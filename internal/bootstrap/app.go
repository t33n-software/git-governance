// Package bootstrap composes delivery adapters with application services.
package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/t33n-software/git-governance/internal/adapters/report"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// BuildInfo identifies a particular immutable build artifact.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// New constructs the production command tree.
func New(build BuildInfo) *cobra.Command {
	return NewWithRuntime(build, defaultRuntime())
}

// NewWithRuntime constructs the command tree with injected dependencies. It is
// used by CLI contract tests and keeps adapters out of application services.
func NewWithRuntime(build BuildInfo, runtime Runtime) *cobra.Command {
	version := build.Version
	if version == "" {
		version = "devel"
	}
	options := &appOptions{
		interactive:         "auto",
		output:              "human",
		color:               "auto",
		remote:              "origin",
		repository:          ".",
		pullRequestProvider: "none",
		timeout:             30 * time.Second,
	}
	application := newApplication(runtime, options)

	command := &cobra.Command{
		Use:           "git-governance",
		Short:         "Create and validate governed Git branches and commits",
		Long:          "git-governance is a cross-platform CLI for governed Git branch, commit, and workflow operations.",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
	}
	command.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		return application.validateOptions()
	}
	command.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return problem.Wrap(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryUsage,
			Field:       "command options",
			Expected:    "valid command-line flags",
			Rule:        "command-line parsing must succeed before any workflow runs",
			Remediation: "run the command with --help and correct the supplied flags",
		}, err)
	})

	command.Version = version
	command.SetVersionTemplate(fmt.Sprintf(
		"git-governance %s\ncommit: %s\nbuilt: %s\n",
		version,
		nonEmpty(build.Commit, "unknown"),
		nonEmpty(build.Date, "unknown"),
	))
	command.PersistentFlags().StringVar(&options.interactive, "interactive", options.interactive, "auto, always, or never")
	command.PersistentFlags().StringVar(&options.output, "output", options.output, "human or json")
	command.PersistentFlags().BoolVar(&options.quiet, "quiet", false, "suppress successful human output")
	command.PersistentFlags().StringVar(&options.color, "color", options.color, "auto, always, or never")
	command.PersistentFlags().BoolVar(&options.accessible, "accessible", false, "use accessible line-oriented forms")
	command.PersistentFlags().StringVar(&options.remote, "remote", options.remote, "Git remote name")
	command.PersistentFlags().StringVar(&options.repository, "repo", options.repository, "repository directory")
	command.PersistentFlags().StringVar(&options.config, "config", "", "user preferences configuration path")
	command.PersistentFlags().StringVar(&options.qualityConfig, "quality-config", "", "repository-local quality gate configuration path")
	command.PersistentFlags().StringVar(&options.pullRequestProvider, "pull-request-provider", options.pullRequestProvider, "none or github")
	command.PersistentFlags().BoolVar(&options.dryRun, "dry-run", false, "show a plan without mutating Git")
	command.PersistentFlags().BoolVar(&options.yes, "yes", false, "confirm mutating operations non-interactively")
	command.PersistentFlags().DurationVar(&options.timeout, "timeout", options.timeout, "timeout for external Git processes")

	command.AddCommand(newCompletionCommand(command))
	command.AddCommand(
		newBranchCommand(application),
		newCommitCommand(application),
		newWorkflowCommand(application),
		newValidateCommand(application),
		newAuthCommand(application),
		newConfigCommand(application),
		newPolicyCommand(application),
		newDoctorCommand(application),
	)

	return command
}

func nonEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// RenderError emits a stable error result using the selected output format.
func RenderError(command *cobra.Command, err error) {
	format := report.FormatHuman
	if output, flagErr := command.Root().PersistentFlags().GetString("output"); flagErr == nil && output == "json" {
		format = report.FormatJSON
	}
	color := false
	if format == report.FormatHuman {
		if mode, flagErr := command.Root().PersistentFlags().GetString("color"); flagErr == nil {
			color = mode == "always" || (mode == "auto" && stderrIsTerminal())
		}
	}

	typed, ok := problem.As(err)
	if !ok {
		typed = problem.Wrap(problem.Details{
			Code:        problem.CodeInternal,
			Category:    problem.CategoryInternal,
			Field:       "operation",
			Expected:    "a classified product error",
			Rule:        "unclassified failures are reported as internal errors",
			Remediation: "review diagnostics and report the failure",
		}, err)
	}
	_ = report.New(report.Options{
		Writer: command.ErrOrStderr(),
		Format: format,
		Color:  color,
	}).Report(context.Background(), port.Report{Problem: typed})
}
