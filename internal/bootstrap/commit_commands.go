package bootstrap

import (
	"strings"

	"github.com/spf13/cobra"
	commitapp "github.com/t33n-software/git-governance/internal/application/commit"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

func newCommitCommand(application *application) *cobra.Command {
	command := &cobra.Command{
		Use:   "commit",
		Short: "Create and validate governed commits",
	}
	command.AddCommand(
		newCommitCreateCommand(application),
		newCommitValidateCommand(application),
	)
	return command
}

func newCommitCreateCommand(application *application) *cobra.Command {
	var (
		typeRaw             string
		ticketRaw           string
		subject             string
		body                string
		breaking            bool
		breakingDescription string
		footerSpecs         []string
		stagePaths          []string
		push                bool
		baseRaw             string
	)
	command := &cobra.Command{
		Use:   "create",
		Short: "Create a governed commit from explicit staged paths",
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			currentBranch, err := services.git.CurrentBranch(command.Context(), repository)
			if err != nil {
				return err
			}
			commitTicket, err := resolveCommitTicket(currentBranch, ticketRaw)
			if err != nil {
				return err
			}
			inputs.add("ticket", commitTicket.String())
			message, err := application.resolveCommitMessage(command.Context(), commitMessageInput{
				Branch:               currentBranch,
				Repository:           repository,
				Family:               typeRaw,
				Description:          subject,
				Body:                 body,
				Breaking:             breaking,
				BreakingDescription:  breakingDescription,
				FooterSpecifications: footerSpecs,
				DescriptionLabel:     "Commit description",
				Operation:            "this commit",
			})
			if err != nil {
				return err
			}
			inputs.add("commit family", message.Header().Type().String())
			inputs.add("commit description", message.Header().Subject())
			if message.IsBreaking() {
				inputs.add("breaking change", "true")
			}
			base, err := parseBase(baseRaw, repository.Remote)
			if err != nil {
				return err
			}
			if err := application.confirmMutation(command.Context(), "Create commit", "Create "+message.Header().String()+"?"); err != nil {
				return err
			}
			result, err := services.commits.Create(command.Context(), commitapp.CreateRequest{
				Repository: repository,
				Branch:     currentBranch,
				Message:    message,
				StagePaths: stagePaths,
				Base:       base,
				Push:       push,
				DryRun:     application.options.dryRun,
			})
			if err != nil {
				return err
			}
			return application.report(command, port.Report{
				Operation: "commit.create",
				Summary:   commitCreationSummary(result),
				Fields: map[string]string{
					"branch":    result.Branch.String(),
					"message":   result.Message.Header().String(),
					"committed": boolString(result.Committed),
					"pushed":    boolString(result.Pushed),
					"dryRun":    boolString(result.DryRun),
					"plan":      commitPlanText(result.Plan),
				},
			})
		}),
	}
	command.Flags().StringVar(&typeRaw, "type", "", "commit family")
	command.Flags().StringVar(&ticketRaw, "ticket", "", "ticket ID compatibility check; the current branch is authoritative")
	command.Flags().StringVar(&subject, "subject", "", "commit description; never carries the type/ticket envelope")
	command.Flags().StringVar(&body, "body", "", "commit body; mandatory for breaking changes, hotfix-lane commits, release-stabilization branches, and the scratch squash transfer")
	command.Flags().BoolVar(&breaking, "breaking", false, "mark an incompatible public contract change")
	command.Flags().StringVar(&breakingDescription, "breaking-description", "", "breaking change migration impact")
	command.Flags().StringSliceVar(&footerSpecs, "footer", nil, "footer as TOKEN=VALUE; repeatable")
	command.Flags().StringSliceVar(&stagePaths, "stage", nil, "explicit path to stage; repeatable")
	command.Flags().BoolVar(&push, "push", false, "validate and push after committing")
	command.Flags().StringVar(&baseRaw, "base", "", "explicit base for pre-push validation")
	return command
}

func newCommitValidateCommand(application *application) *cobra.Command {
	var (
		messageFile string
		messageRaw  string
		branchRaw   string
	)
	command := &cobra.Command{
		Use:   "validate",
		Short: "Validate a complete commit message for the current branch",
		RunE: func(command *cobra.Command, _ []string) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			currentBranch, err := currentOrSpecified(command.Context(), services, branchRaw, repository)
			if err != nil {
				return err
			}
			raw, err := readCommitMessage(messageFile, messageRaw)
			if err != nil {
				return err
			}
			message, err := commitmsg.Parse(raw)
			if err != nil {
				return err
			}
			result, err := services.commits.Validate(command.Context(), commitapp.ValidateRequest{
				Repository: repository,
				Branch:     currentBranch,
				Message:    message,
			})
			if err != nil {
				return err
			}
			return application.report(command, port.Report{
				Operation: "commit.validate",
				Summary:   "Commit message is valid.",
				Fields: map[string]string{
					"branch":   result.Branch.String(),
					"message":  result.Message.Header().String(),
					"breaking": boolString(result.Message.IsBreaking()),
				},
			})
		},
	}
	command.Flags().StringVar(&messageFile, "message-file", "", "file containing the complete commit message")
	command.Flags().StringVar(&messageRaw, "message", "", "complete commit message")
	command.Flags().StringVar(&branchRaw, "branch", "", "branch name; defaults to the current branch")
	return command
}

func resolveCommitTicket(current branch.BranchName, raw string) (ticket.ID, error) {
	fromBranch, ticketScoped := current.Ticket()
	if !ticketScoped {
		return ticket.ID{}, missingCommitContext(current)
	}
	if raw == "" {
		return fromBranch, nil
	}
	explicit, err := ticket.ParseID(raw)
	if err != nil {
		return ticket.ID{}, err
	}
	if explicit.String() == fromBranch.String() {
		return fromBranch, nil
	}
	return ticket.ID{}, problem.New(problem.Details{
		Code:        problem.CodeCommitTicketMismatch,
		Category:    problem.CategoryGovernance,
		Field:       "ticket",
		Actual:      explicit.String(),
		Expected:    fromBranch.String(),
		Rule:        "governed commit creation derives the ticket from the current branch",
		Example:     "feat(" + fromBranch.String() + "): add export button",
		Remediation: "remove --ticket or supply the ticket derived from the current branch",
	})
}

func parseFooterSpecs(values []string) ([]commitmsg.Footer, error) {
	footers := make([]commitmsg.Footer, 0, len(values))
	for _, value := range values {
		footer, err := parseFooterSpec(value)
		if err != nil {
			return nil, err
		}
		footers = append(footers, footer)
	}
	return footers, nil
}

func commitCreationSummary(result commitapp.CreateResult) string {
	if result.DryRun {
		return "Commit creation plan generated."
	}
	if result.Pushed {
		return "Commit created and pushed."
	}
	return "Commit created."
}

func commitPlanText(steps []commitapp.PlanStep) string {
	parts := make([]string, 0, len(steps))
	for _, step := range steps {
		parts = append(parts, step.Action+": "+step.Detail)
	}
	return strings.Join(parts, "; ")
}
