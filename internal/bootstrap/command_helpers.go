package bootstrap

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/cliparam"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// This file owns the command plumbing shared by every endpoint: global option
// validation, the report surface, the fetch summary decoration, repository
// discovery, the mutation-confirmation law, the workflow input summary, and
// the branch/message/footer parsing helpers. The pull-request publication
// funnel lives in pull_request_publication.go, the interactive resolution
// funnels in interactive_resolution.go, and the line and version
// resolutions in line_version_resolution.go. Canonical conventions:
// docs/conventions/cli/value-domain-model.md,
// docs/conventions/cli/error-philosophy.md, and
// docs/conventions/cli/interaction-model.md.

// validateOptions rejects invalid global option values and combinations
// fail-closed before any workflow runs, including the --interactive always /
// --output json combination rule. Canonical conventions:
// docs/conventions/cli/value-domain-model.md and
// docs/conventions/cli/error-philosophy.md.
func (application *application) validateOptions() error {
	if !slices.Contains(cliparam.InteractiveMode().Values, application.options.interactive) {
		return invalidOption("interactive", application.options.interactive, cliparam.InteractiveMode().ValueList())
	}
	if _, err := application.outputFormat(); err != nil {
		return err
	}
	if !slices.Contains(cliparam.ColorMode().Values, application.options.color) {
		return invalidOption("color", application.options.color, cliparam.ColorMode().ValueList())
	}
	if application.options.pullRequestProvider != "" && !slices.Contains(cliparam.PullRequestProvider().Values, application.options.pullRequestProvider) {
		return invalidOption("pull-request-provider", application.options.pullRequestProvider, cliparam.PullRequestProvider().ValueList())
	}
	if application.options.timeout <= 0 {
		return invalidOption("timeout", application.options.timeout.String(), cliparam.Timeout().Rule)
	}
	if application.options.interactive == "always" {
		if application.options.output == "json" {
			return invalidOption("interactive", application.options.interactive, "auto or never when --output json is selected")
		}
		if !application.inputIsTerminal() || !application.outputIsTerminal() {
			return problem.New(problem.Details{
				Code:        problem.CodeInvalidInput,
				Category:    problem.CategoryUsage,
				Field:       "interactive",
				Actual:      "always",
				Expected:    "an interactive terminal connected to both standard input and standard output",
				Rule:        "--interactive always must fail early when no terminal is available",
				Remediation: "run from a terminal, use --interactive auto, or supply all required flags with --interactive never",
			})
		}
	}
	return nil
}

func (application *application) report(command *cobra.Command, result port.Report) error {
	return application.reporter(command.OutOrStdout()).Report(command.Context(), result)
}

func (application *application) withInteractiveFetchSummary(summary, remote string, fetched bool) string {
	if !fetched || !application.promptAvailable() {
		return summary
	}
	return "🟢 Remote references fetched and stale references pruned from " + remote + " before this operation.\n" + summary
}

func fetchCompleted(dryRun bool, plan []branchapp.PlanStep) bool {
	if dryRun {
		return false
	}
	for _, step := range plan {
		if step.Action == "fetch" {
			return true
		}
	}
	return false
}

func (application *application) discover(ctx context.Context, service services) (port.RepositoryIdentity, error) {
	identity, err := service.git.Discover(ctx, application.options.repository)
	if err != nil {
		return port.RepositoryIdentity{}, err
	}
	identity.Remote = application.options.remote
	return identity, nil
}

// confirmMutation enforces the mutation-confirmation law: mutating operations
// require explicit confirmation or --yes, and every mutating command offers
// --dry-run. Canonical convention:
// docs/conventions/cli/interaction-model.md.
func (application *application) confirmMutation(ctx context.Context, label, description string) error {
	if application.options.dryRun || application.options.yes {
		return nil
	}
	if !application.promptAvailable() {
		return problem.New(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryUsage,
			Field:       "confirmation",
			Expected:    "--yes for non-interactive mutations",
			Rule:        "mutating Git operations require explicit confirmation",
			Example:     "--yes",
			Remediation: "pass --yes or run in an interactive terminal",
		})
	}
	confirmed, err := application.prompt().Confirm(ctx, port.ConfirmRequest{
		Label:       label,
		Description: description,
		Default:     false,
	})
	if err != nil {
		return err
	}
	if confirmed {
		return nil
	}
	return problem.New(problem.Details{
		Code:        problem.CodeOperationCancelled,
		Category:    problem.CategoryCancelled,
		Field:       "confirmation",
		Expected:    "an explicit yes",
		Rule:        "the user declined the mutation",
		Remediation: "rerun the command and confirm the planned operation",
	})
}

type workflowInputSummary struct {
	inputs []problem.WorkflowInput
}

func (summary *workflowInputSummary) add(field, value string) {
	if field == "" || value == "" {
		return
	}
	for index := range summary.inputs {
		if summary.inputs[index].Field == field {
			summary.inputs[index].Value = value
			return
		}
	}
	summary.inputs = append(summary.inputs, problem.WorkflowInput{
		Field: field,
		Value: value,
	})
}

func (summary *workflowInputSummary) attach(err error) error {
	return problem.WithWorkflowInputs(err, summary.inputs)
}

func withWorkflowInputs(
	run func(command *cobra.Command, inputs *workflowInputSummary) error,
) func(command *cobra.Command, arguments []string) error {
	return func(command *cobra.Command, _ []string) error {
		inputs := &workflowInputSummary{}
		return inputs.attach(run(command, inputs))
	}
}

func currentOrSpecified(ctx context.Context, service services, raw string, repository port.RepositoryIdentity) (branch.BranchName, error) {
	if raw != "" {
		return branch.ParseName(raw)
	}
	return service.git.CurrentBranch(ctx, repository)
}

// resumeBranch resolves the branch that a --resume invocation continues. A
// resume never takes a branch input: the command boundary rejects a supplied
// --branch fail-closed, mirroring the existing --resume rejection of
// --strategy and merge inputs, because during a paused rebase the checkout is
// detached by design and the branch survives only in the operation state.
// With no paused rebase the checked-out branch stays the resolution source
// (the externally-completed verification path).
// Canonical convention: docs/conventions/branches/resume-branch-derivation.md.
func resumeBranch(ctx context.Context, service services, repository port.RepositoryIdentity) (branch.BranchName, error) {
	inspector, ok := service.git.(port.PausedRebaseBranchInspector)
	if !ok {
		return service.git.CurrentBranch(ctx, repository)
	}
	derived, found, err := inspector.PausedRebaseBranch(ctx, repository)
	if err != nil {
		return branch.BranchName{}, err
	}
	if found {
		return derived, nil
	}
	return service.git.CurrentBranch(ctx, repository)
}

func readCommitMessage(path string) (string, error) {
	if path == "" {
		return "", missingInput("commit message")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", problem.Wrap(problem.Details{
			Code:        problem.CodeConfigurationUnavailable,
			Category:    problem.CategoryUsage,
			Field:       "message file",
			Actual:      path,
			Expected:    "a readable commit message file",
			Rule:        "commit validation reads the exact message supplied by Git",
			Remediation: "provide --message or an existing --message-file",
		}, err)
	}
	if info.Size() > maxMessageFileBytes {
		return "", problem.New(problem.Details{
			Code:        problem.CodeCommitHeaderInvalid,
			Category:    problem.CategoryUsage,
			Field:       "message file",
			Actual:      path,
			Expected:    fmt.Sprintf("at most %d bytes", maxMessageFileBytes),
			Rule:        "commit message input is size-bounded",
			Remediation: "reduce the commit message file size",
		})
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", problem.Wrap(problem.Details{
			Code:        problem.CodeConfigurationUnavailable,
			Category:    problem.CategoryUsage,
			Field:       "message file",
			Actual:      path,
			Expected:    "a readable commit message file",
			Rule:        "commit validation reads the exact message supplied by Git",
			Remediation: "check the path and file permissions",
		}, err)
	}
	return string(contents), nil
}

func parseFooterSpec(raw string) (commitmsg.Footer, error) {
	token, value, found := strings.Cut(raw, "=")
	if !found {
		return commitmsg.Footer{}, problem.New(problem.Details{
			Code:        problem.CodeCommitDescriptionInvalid,
			Category:    problem.CategoryUsage,
			Field:       "footer",
			Actual:      raw,
			Expected:    "TOKEN=VALUE",
			Rule:        "CLI footer flags use an equals separator",
			Example:     "--footer Refs=#123",
			Remediation: "provide one footer token and one non-empty value",
		})
	}
	return commitmsg.NewFooter(token, value)
}

// invalidOption builds the coded usage-failure record for an unsupported
// option value. Its remediation must stay truthful: it may only reference
// surfaces that actually carry the answer. Canonical conventions:
// docs/conventions/cli/output-contract.md and
// docs/conventions/cli/error-philosophy.md.
func invalidOption(field, actual, expected string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeInvalidInput,
		Category:    problem.CategoryUsage,
		Field:       field,
		Actual:      actual,
		Expected:    expected,
		Rule:        "CLI option values must use a supported value",
		Remediation: "use --help to see supported values",
	})
}
