package bootstrap

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/application/workflow"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

const maxCommitMessageBytes = 1 << 20

func (application *application) validateOptions() error {
	switch application.options.interactive {
	case "auto", "always", "never":
	default:
		return invalidOption("interactive", application.options.interactive, "auto, always, or never")
	}
	if _, err := application.outputFormat(); err != nil {
		return err
	}
	switch application.options.color {
	case "auto", "always", "never":
	default:
		return invalidOption("color", application.options.color, "auto, always, or never")
	}
	switch application.options.pullRequestProvider {
	case "", "none", "github":
	default:
		return invalidOption("pull-request-provider", application.options.pullRequestProvider, "none or github")
	}
	if application.options.timeout <= 0 {
		return invalidOption("timeout", application.options.timeout.String(), "a positive duration")
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

func (application *application) validatePullRequestPublication(
	services services,
	push, createPullRequest bool,
	body string,
) error {
	if createPullRequest && !push {
		return invalidOption("create-pull-request", "true", "--push")
	}
	if err := validatePullRequestBody(createPullRequest, body); err != nil {
		return err
	}
	if createPullRequest && !application.options.dryRun && !services.tickets.HasPullRequestPublisher() {
		return pullRequestPublisherUnavailable()
	}
	return nil
}

// validatePullRequestBody enforces the mandatory pull-request description
// owned by docs/conventions/pull-requests/description-mandate.md before any
// Git mutation happens.
func validatePullRequestBody(createPullRequest bool, body string) error {
	if createPullRequest && strings.TrimSpace(body) == "" {
		return invalidOption("body", "empty", "a non-empty pull-request description when --create-pull-request is set")
	}
	return nil
}

func (application *application) completePreparedPublication(
	ctx context.Context,
	services services,
	repository port.RepositoryIdentity,
	result *workflow.PublishTicketResult,
	requestedPush, requestedPullRequest bool,
	workflowManaged bool,
) error {
	if result == nil || result.DryRun {
		return nil
	}
	if requestedPullRequest {
		if err := services.tickets.PreflightPullRequest(ctx, repository, result.PullRequest); err != nil {
			return err
		}
	}
	push := requestedPush
	if application.promptAvailable() && !application.options.yes {
		confirmed, err := application.prompt().Confirm(ctx, port.ConfirmRequest{
			Label:       "Push official ticket branch",
			Description: "Push " + result.Branch.String() + " after the completed synchronization? The first push configures its matching upstream branch.",
			Default:     requestedPush,
		})
		if err != nil {
			return err
		}
		push = confirmed
	}
	if !push {
		return nil
	}
	base := result.Sync.Base
	if err := services.tickets.PushPreparedTicket(ctx, repository, result.Branch, &base, workflowManaged); err != nil {
		return err
	}
	result.Pushed = true

	pullRequest, createPullRequest, err := application.resolvePullRequestPublication(
		ctx,
		services,
		result.PullRequest,
		requestedPullRequest,
	)
	if err != nil {
		return err
	}
	if !createPullRequest {
		return nil
	}
	result.PullRequest = pullRequest
	if err := services.tickets.PreflightPullRequest(ctx, repository, result.PullRequest); err != nil {
		return err
	}
	publishedURL, err := services.tickets.PublishPullRequest(ctx, repository, result.PullRequest)
	if err != nil {
		return err
	}
	result.PublishedURL = publishedURL
	return nil
}

func (application *application) resolvePullRequestPublication(
	ctx context.Context,
	services services,
	request port.PullRequest,
	requested bool,
) (port.PullRequest, bool, error) {
	if application.options.dryRun {
		return request, false, nil
	}
	if !services.tickets.HasPullRequestPublisher() {
		if requested {
			return request, false, pullRequestPublisherUnavailable()
		}
		return request, false, nil
	}
	create := requested
	if application.promptAvailable() && !application.options.yes {
		confirmed, err := application.prompt().Confirm(ctx, port.ConfirmRequest{
			Label: "Create pull request",
			Description: "Create the pull request from " + request.Source.String() +
				" to " + request.Target.String() + " now?",
			Default: requested,
		})
		if err != nil {
			return request, false, err
		}
		create = confirmed
	} else if requested && !application.options.yes {
		return request, false, pullRequestConfirmationRequired()
	}
	if !create {
		return request, false, nil
	}
	return application.resolvePullRequestBody(ctx, request)
}

// resolvePullRequestBody enforces the mandatory pull-request description
// owned by docs/conventions/pull-requests/description-mandate.md: a confirmed
// publication either carries the description already or asks for it
// interactively; non-interactive creation without it fails closed.
func (application *application) resolvePullRequestBody(
	ctx context.Context,
	request port.PullRequest,
) (port.PullRequest, bool, error) {
	if strings.TrimSpace(request.Body) != "" {
		return request, true, nil
	}
	if application.promptAvailable() && !application.options.yes {
		body, err := application.prompt().Input(ctx, port.InputRequest{
			Label: "Pull request description",
			Description: "Describe the change set for " + request.Source.String() + " to " + request.Target.String() +
				": summary, scope and non-goals, commit series, risk and rollback, verification and review focus. The description is mandatory.",
			Required: true,
			Validate: func(value string) error {
				if strings.TrimSpace(value) == "" {
					return problem.New(problem.Details{
						Code:        problem.CodeInvalidInput,
						Category:    problem.CategoryUsage,
						Field:       "pull request description",
						Expected:    "a non-empty description",
						Rule:        "every governed pull request carries a mandatory description",
						Remediation: "enter the canonical pull-request description",
					})
				}
				return nil
			},
		})
		if err != nil {
			return request, false, err
		}
		request.Body = body
		return request, true, nil
	}
	return request, false, invalidOption("body", "empty", "a non-empty pull-request description passed as --body")
}

func pullRequestPublisherUnavailable() error {
	return problem.New(problem.Details{
		Code:        problem.CodeExternalCommandFailed,
		Category:    problem.CategoryExternal,
		Field:       "pull request publisher",
		Expected:    "a configured hosting-provider adapter",
		Rule:        "a real pull request can be created only through an explicit provider adapter",
		Remediation: "set --pull-request-provider github and complete auth login github or configure the managed credential broker",
	})
}

func pullRequestConfirmationRequired() error {
	return problem.New(problem.Details{
		Code:        problem.CodeInvalidInput,
		Category:    problem.CategoryUsage,
		Field:       "confirmation",
		Expected:    "--yes for non-interactive pull-request creation",
		Rule:        "external pull-request creation requires explicit confirmation",
		Example:     "--yes --create-pull-request",
		Remediation: "pass --yes or run in an interactive terminal",
	})
}

func (application *application) resolveKey(ctx context.Context, service services, raw string) (ticket.Key, error) {
	if raw != "" {
		return ticket.ParseKey(raw)
	}
	defaultValue := ""
	preferences, err := service.preferences.List(ctx)
	if err == nil && preferences.DefaultKey != nil {
		defaultValue = preferences.DefaultKey.String()
	}
	if !application.promptAvailable() {
		return ticket.Key{}, missingInput("ticket key")
	}
	value, err := application.prompt().Input(ctx, port.InputRequest{
		Label:       "Ticket key",
		Description: "Enter 1 to 32 uppercase ASCII letters or digits, starting with a letter. Lowercase letters, spaces, and hyphens are not allowed. Examples: ABC, PLATFORM2.",
		Default:     defaultValue,
		Required:    true,
		Validate:    inputValidator(ticket.ParseKey),
	})
	if err != nil {
		return ticket.Key{}, err
	}
	return ticket.ParseKey(value)
}

func (application *application) resolveNumber(ctx context.Context, raw string) (ticket.Number, error) {
	return resolveValidatedInput(
		application,
		ctx,
		raw,
		"Ticket number",
		"Enter 1 to 18 decimal digits, starting with 1 to 9. Leading zeroes, signs, decimals, and spaces are not allowed. Example: 123.",
		ticket.ParseNumber,
	)
}

func (application *application) resolveSlug(ctx context.Context, raw string, label string) (branch.Slug, error) {
	return resolveValidatedInput(
		application,
		ctx,
		raw,
		label,
		"Enter 1 to 100 lowercase ASCII letters or digits. Separate words with exactly one hyphen; do not use spaces, uppercase letters, or leading, trailing, or repeated hyphens. Example: add-export-button.",
		branch.ParseSlug,
	)
}

// resolveScratchMergeMessage binds the squash transfer commit from structured
// values only. The body is mandatory: the transfer commit is the only record
// of the discarded experiment paths. Canonical convention:
// docs/conventions/commits/family-selection.md.
func (application *application) resolveScratchMergeMessage(
	ctx context.Context,
	repository port.RepositoryIdentity,
	target branch.BranchName,
	family string,
	description string,
	body string,
	breaking bool,
	breakingDescription string,
	footerSpecs []string,
) (commitmsg.Message, error) {
	return application.resolveCommitMessage(ctx, commitMessageInput{
		Branch:               target,
		Repository:           repository,
		Family:               family,
		Description:          description,
		Body:                 body,
		Breaking:             breaking,
		BreakingDescription:  breakingDescription,
		FooterSpecifications: footerSpecs,
		RequireBody:          true,
		DescriptionLabel:     "Squash commit description",
		Operation:            "the scratch squash transfer",
		Validate: func(message commitmsg.Message) error {
			return branchapp.ValidateScratchMergeMessage(target, message)
		},
	})
}

func (application *application) resolveFamily(ctx context.Context, raw string, includeSpecial bool) (branch.Family, error) {
	if raw != "" {
		return branch.ParseFamily(raw)
	}
	if !application.promptAvailable() {
		return "", missingInput("branch family")
	}

	families := branchapp.ListFamilies()
	options := make([]port.SelectOption, 0, len(families))
	for _, family := range families {
		if !includeSpecial && !family.DirectlyCreatable {
			continue
		}
		options = append(options, port.SelectOption{
			Value:       family.Family.String(),
			Label:       family.Label,
			Description: family.Description,
		})
	}
	value, err := application.prompt().Select(ctx, port.SelectRequest{
		Label:       "Branch family",
		Description: "Select the branch family that matches the work. The list command explains every family.",
		Options:     options,
		Default:     branch.FamilyFeature.String(),
	})
	if err != nil {
		return "", err
	}
	return branch.ParseFamily(value)
}

func parseTicketParts(keyRaw, numberRaw string) (ticket.ID, error) {
	key, err := ticket.ParseKey(keyRaw)
	if err != nil {
		return ticket.ID{}, err
	}
	number, err := ticket.ParseNumber(numberRaw)
	if err != nil {
		return ticket.ID{}, err
	}
	return ticket.NewID(key, number), nil
}

func parseBase(raw, remote string) (*branch.TargetBase, error) {
	if raw == "" {
		return nil, nil
	}
	nameRaw := raw
	prefix := remote + "/"
	if strings.HasPrefix(raw, prefix) {
		nameRaw = strings.TrimPrefix(raw, prefix)
	} else if strings.Contains(raw, "/") {
		return nil, problem.New(problem.Details{
			Code:        problem.CodeBranchBaseInvalid,
			Category:    problem.CategoryUsage,
			Field:       "base",
			Actual:      raw,
			Expected:    "a canonical branch name or " + prefix + "<branch>",
			Rule:        "explicit bases use the selected remote only",
			Example:     prefix + "develop",
			Remediation: "pass a branch on the selected --remote",
		})
	}
	name, err := branch.ParseName(nameRaw)
	if err != nil {
		return nil, err
	}
	base, err := branch.NewTargetBase(remote, name)
	if err != nil {
		return nil, err
	}
	return &base, nil
}

func (application *application) resolveScratchBase(
	ctx context.Context,
	raw, remote string,
	id ticket.ID,
) (*branch.TargetBase, error) {
	base, err := resolveValidatedInput(
		application,
		ctx,
		raw,
		"Official ticket branch base",
		"Enter the local official feature, fix, docs, refactor, chore, test, perf, or hotfix branch for the same ticket. Do not use origin/, a shared line, or a scratch branch. Example: feature/ABC-123-add-export.",
		func(value string) (branch.TargetBase, error) {
			return parseScratchBase(value, remote, id)
		},
	)
	if err != nil {
		return nil, err
	}
	return &base, nil
}

func parseScratchBase(raw, remote string, id ticket.ID) (branch.TargetBase, error) {
	if strings.HasPrefix(raw, remote+"/") {
		return branch.TargetBase{}, problem.New(problem.Details{
			Code:        problem.CodeBranchBaseInvalid,
			Category:    problem.CategoryGovernance,
			Field:       "scratch base",
			Actual:      raw,
			Expected:    "a local official ticket branch name",
			Rule:        "scratch branches start from the local official branch, not a remote-tracking ref",
			Example:     "feature/ABC-123-add-export",
			Remediation: "provide the checked-out official ticket branch without the remote prefix",
		})
	}
	base, err := branch.ParseLocalBase(raw)
	if err != nil {
		return branch.TargetBase{}, err
	}
	name := base.Branch()
	if !name.Family().IsOfficialWorkingBranch() {
		return branch.TargetBase{}, problem.New(problem.Details{
			Code:        problem.CodeBranchBaseInvalid,
			Category:    problem.CategoryGovernance,
			Field:       "scratch base",
			Actual:      raw,
			Expected:    "a local feature, fix, docs, refactor, chore, test, perf, or hotfix branch",
			Rule:        "scratch branches are private exploration of an official ticket branch",
			Example:     "feature/ABC-123-add-export",
			Remediation: "select the official branch for the same ticket or use workflow ticket start",
		})
	}
	baseTicket, hasTicket := name.Ticket()
	if !hasTicket || baseTicket.String() != id.String() {
		return branch.TargetBase{}, problem.New(problem.Details{
			Code:        problem.CodeBranchBaseInvalid,
			Category:    problem.CategoryGovernance,
			Field:       "scratch base",
			Actual:      raw,
			Expected:    "a local official branch for ticket " + id.String(),
			Rule:        "scratch branches stay attached to the official branch for the same ticket",
			Example:     "feature/" + id.String() + "-add-export",
			Remediation: "select the official branch that carries the current ticket",
		})
	}
	return base, nil
}

func resolveValidatedInput[T any](
	application *application,
	ctx context.Context,
	raw, label, description string,
	parse func(string) (T, error),
) (T, error) {
	if raw != "" {
		return parse(raw)
	}
	value, err := application.requireInput(ctx, "", label, description, inputValidator(parse))
	if err != nil {
		var zero T
		return zero, err
	}
	return parse(value)
}

func inputValidator[T any](parse func(string) (T, error)) port.InputValidator {
	return func(value string) error {
		_, err := parse(value)
		return err
	}
}

func (application *application) resolveAffectedLine(ctx context.Context, raw string) (branch.BranchName, error) {
	return resolveAllowedBranchLine(
		application,
		ctx,
		raw,
		"Affected line",
		"Enter main, release/<semantic-version>, or support/<major.minor>. A hotfix must start from the active line that contains the defect. Examples: main, release/2.8.0, support/2.7.",
		"affected line",
		"main, release/<semver>, or support/<major.minor>",
		"a hotfix must start from the active line that contains the defect",
		"main",
		"select the main, release, or support line that contains the defect",
		branch.FamilyMain,
		branch.FamilyRelease,
		branch.FamilySupport,
	)
}

func (application *application) resolvePropagationTarget(ctx context.Context, raw string) (branch.BranchName, error) {
	return resolveAllowedBranchLine(
		application,
		ctx,
		raw,
		"Propagation target line",
		"Enter main, develop, release/<semantic-version>, or support/<major.minor>. The target must be the active line that also needs the reviewed hotfix. Examples: develop, release/2.8.0.",
		"propagation target line",
		"main, develop, release/<semver>, or support/<major.minor>",
		"hotfix propagation targets another active line",
		"develop",
		"select the active line that also needs the reviewed hotfix",
		branch.FamilyMain,
		branch.FamilyDevelop,
		branch.FamilyRelease,
		branch.FamilySupport,
	)
}

func (application *application) resolveReleaseLine(
	ctx context.Context,
	raw, label, description string,
) (branch.BranchName, error) {
	return resolveAllowedBranchLine(
		application,
		ctx,
		raw,
		label,
		description,
		"release line",
		"release/<semver>",
		"the selected workflow requires a canonical release line",
		"release/2.8.0",
		"enter the release/<semver> line required by this workflow",
		branch.FamilyRelease,
	)
}

func (application *application) resolveReviewedCommit(ctx context.Context, raw string) (string, error) {
	return resolveValidatedInput(
		application,
		ctx,
		raw,
		"Reviewed source commit",
		"Enter the 7 to 64 character hexadecimal SHA of the reviewed hotfix commit. Do not enter a branch name, tag, ref, or spaces. Example: 0123456789abcdef0123456789abcdef01234567.",
		func(value string) (string, error) {
			return value, workflow.ValidateCommitID(value)
		},
	)
}

func (application *application) resolveReleaseVersion(ctx context.Context, raw string) (branch.SemanticVersion, error) {
	return resolveValidatedInput(
		application,
		ctx,
		raw,
		"Release version",
		"Enter Semantic Versioning 2.0.0 without a leading v: major.minor.patch, optionally with pre-release or build metadata. Examples: 2.8.0, 2.8.0-rc.1.",
		branch.ParseSemanticVersion,
	)
}

func (application *application) resolveSupportVersion(ctx context.Context, raw string) (branch.SupportVersion, error) {
	return resolveValidatedInput(
		application,
		ctx,
		raw,
		"Support version",
		"Enter exactly major.minor without a leading v or leading zeroes. Examples: 2.7, 0.9. Patch versions are not allowed.",
		branch.ParseSupportVersion,
	)
}

func (application *application) resolveStabilizationKind(ctx context.Context, raw string) (workflow.ReleaseStabilizationKind, error) {
	return resolveValidatedInput(
		application,
		ctx,
		raw,
		"Stabilization kind",
		"Enter blocker, docs, or release-prep. Frozen release lines do not allow new features or general refactors. Example: blocker.",
		workflow.ParseReleaseStabilizationKind,
	)
}

func resolveAllowedBranchLine(
	application *application,
	ctx context.Context,
	raw, label, description, field, expected, rule, example, remediation string,
	allowed ...branch.Family,
) (branch.BranchName, error) {
	return resolveValidatedInput(
		application,
		ctx,
		raw,
		label,
		description,
		func(value string) (branch.BranchName, error) {
			name, err := branch.ParseName(value)
			if err != nil {
				return branch.BranchName{}, err
			}
			for _, family := range allowed {
				if name.Family() == family {
					return name, nil
				}
			}
			return branch.BranchName{}, problem.New(problem.Details{
				Code:        problem.CodeInvalidInput,
				Category:    problem.CategoryGovernance,
				Field:       field,
				Actual:      value,
				Expected:    expected,
				Rule:        rule,
				Example:     example,
				Remediation: remediation,
			})
		},
	)
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

func readCommitMessage(path, inline string) (string, error) {
	if inline != "" {
		return inline, nil
	}
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
	if info.Size() > maxCommitMessageBytes {
		return "", problem.New(problem.Details{
			Code:        problem.CodeCommitHeaderInvalid,
			Category:    problem.CategoryUsage,
			Field:       "message file",
			Actual:      path,
			Expected:    fmt.Sprintf("at most %d bytes", maxCommitMessageBytes),
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
