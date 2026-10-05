package bootstrap

import (
	"strconv"

	"github.com/spf13/cobra"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/application/workflow"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// newBranchHygieneCommand owns the governed branch-hygiene endpoints: the
// controlled deletion of one completed local working branch and the
// evidence-based prune pass over the local branch surface.
func newBranchHygieneCommand(application *application) *cobra.Command {
	command := &cobra.Command{
		Use:   "branch",
		Short: "Delete completed local working branches under the branch-hygiene evidence",
	}
	command.AddCommand(
		newBranchRemoveCommand(application),
		newBranchPruneCommand(application),
	)
	return command
}

func newBranchRemoveCommand(application *application) *cobra.Command {
	var branchRaw string
	command := &cobra.Command{
		Use:   "remove",
		Short: "Delete one local official working branch under the branch-hygiene evidence",
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			if branchRaw == "" {
				return branchNameRequired()
			}
			name, err := branch.ParseName(branchRaw)
			if err != nil {
				return err
			}
			inputs.add("branch", name.String())
			if err := application.confirmMutation(command.Context(), "Remove branch", "Delete the local official working branch under the branch-hygiene evidence? Every refusal class stays fail-closed."); err != nil {
				return err
			}
			result, err := services.branchHygiene.RemoveBranch(command.Context(), workflow.RemoveBranchRequest{
				Repository: repository,
				Branch:     name,
				DryRun:     application.options.dryRun,
			})
			if err != nil {
				return err
			}
			fields := map[string]string{
				"branch": result.Branch,
				"ticket": result.Ticket,
				"class":  result.Class,
				"dryRun": boolString(result.DryRun),
			}
			if result.PRState != "" {
				fields["pullRequestState"] = result.PRState
				fields["pullRequestNumber"] = result.PRNumber
			}
			if result.Base != "" {
				fields["base"] = result.Base
				fields["ahead"] = strconv.Itoa(result.Ahead)
			}
			return application.report(command, port.Report{
				Operation: "workflow.branch.remove",
				Summary:   "Branch hygiene removal completed.",
				Fields:    fields,
			})
		}),
	}
	registerBranchReferenceFlag(command, &branchRaw, "branch", "the official working branch to delete under the branch-hygiene evidence")
	return command
}

// newBranchPruneCommand discovers the local official working branches whose
// branch-hygiene evidence is proven and deletes them under the same guard
// matrix as the single-branch removal.
func newBranchPruneCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "prune",
		Short: "Delete the local official working branches whose branch-hygiene evidence is proven",
		RunE: func(command *cobra.Command, _ []string) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			if err := application.confirmMutation(command.Context(), "Prune branches", "Delete every local official working branch whose branch-hygiene evidence is proven? Each candidate must pass the guard matrix."); err != nil {
				return err
			}
			result, err := services.branchHygiene.PruneBranches(command.Context(), workflow.PruneBranchesRequest{
				Repository: repository,
				DryRun:     application.options.dryRun,
			})
			if err != nil {
				return err
			}
			removed := 0
			for _, entry := range result.Entries {
				if entry.Removed {
					removed++
				}
			}
			return application.report(command, port.Report{
				Operation: "workflow.branch.prune",
				Summary:   "Branch hygiene prune completed.",
				Fields: map[string]string{
					"branchCount":  strconv.Itoa(len(result.Entries)),
					"removedCount": strconv.Itoa(removed),
					"dryRun":       boolString(result.DryRun),
				},
				Data: result.Entries,
			})
		},
	}
}

// branchNameRequired is the fail-closed record of a removal dispatch without
// a named branch.
func branchNameRequired() error {
	return problem.New(problem.Details{
		Code:        problem.CodeInvalidInput,
		Category:    problem.CategoryUsage,
		Field:       "branch",
		Expected:    "the canonical branch name to remove",
		Rule:        "the branch hygiene removes one named official working branch; the branch flag is required",
		Example:     "--branch feature/ABC-123-add-export-button",
		Remediation: "pass the canonical branch name with --branch",
	})
}
