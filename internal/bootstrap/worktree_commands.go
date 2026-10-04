package bootstrap

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/application/workflow"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

func newWorktreeCommand(application *application) *cobra.Command {
	command := &cobra.Command{
		Use:   "worktree",
		Short: "Acquire, inventory, and remove detached task worktrees",
	}
	command.AddCommand(
		newWorktreeStartCommand(application),
		newWorktreeListCommand(application),
		newWorktreeRemoveCommand(application),
		newWorktreePruneCommand(application),
	)
	return command
}

func newWorktreeStartCommand(application *application) *cobra.Command {
	var (
		keyRaw    string
		numberRaw string
		baseLine  string
	)
	command := &cobra.Command{
		Use:   "start",
		Short: "Acquire a detached task worktree for one ticket from the current revision of the selected base line",
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			key, err := application.resolveKey(command.Context(), services, keyRaw)
			if err != nil {
				return err
			}
			inputs.add("ticket key", key.String())
			number, err := application.resolveNumber(command.Context(), numberRaw)
			if err != nil {
				return err
			}
			inputs.add("ticket number", number.String())
			id := ticket.NewID(key, number)
			inputs.add("worktree ticket", id.String())
			if baseLine != "" {
				inputs.add("acquisition base", baseLine)
			}
			if err := application.confirmMutation(command.Context(), "Acquire task worktree", "Fetch the selected remote and create the detached task worktree from the current revision of the lane's base line?"); err != nil {
				return err
			}
			result, err := services.worktrees.StartWorktree(command.Context(), workflow.StartWorktreeRequest{
				Repository: repository,
				Ticket:     id,
				BaseLine:   baseLine,
				DryRun:     application.options.dryRun,
			})
			if err != nil {
				return err
			}
			return application.report(command, port.Report{
				Operation: "workflow.worktree.start",
				Summary: application.withInteractiveFetchSummary(
					"Task worktree acquisition completed.",
					repository.Remote,
					fetchCompleted(result.DryRun, result.Plan),
				),
				Fields: map[string]string{
					"worktreePath": result.Path,
					"base":         result.Base.String(),
					"ticket":       result.Ticket.String(),
					"dryRun":       boolString(result.DryRun),
				},
			})
		}),
	}
	registerTicketKeyFlag(command, &keyRaw)
	registerTicketNumberFlag(command, &numberRaw)
	registerAcquisitionBaseFlag(command, &baseLine)
	return command
}

func newWorktreeListCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Inventory the task worktrees of the repository",
		RunE: func(command *cobra.Command, _ []string) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			result, err := services.worktrees.ListWorktrees(command.Context(), workflow.ListWorktreesRequest{
				Repository: repository,
			})
			if err != nil {
				return err
			}
			fields := make(map[string]string, len(result.Worktrees)+1)
			fields["worktreeCount"] = strconv.Itoa(len(result.Worktrees))
			for _, entry := range result.Worktrees {
				fields[entry.Path] = worktreeEntrySummary(entry, result.Current)
			}
			return application.report(command, port.Report{
				Operation: "workflow.worktree.list",
				Summary:   "Worktree inventory completed.",
				Fields:    fields,
				Data:      result.Worktrees,
			})
		},
	}
}

func newWorktreeRemoveCommand(application *application) *cobra.Command {
	var (
		keyRaw    string
		numberRaw string
	)
	command := &cobra.Command{
		Use:   "remove",
		Short: "Remove the task worktree of a completed ticket",
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			key, err := application.resolveKey(command.Context(), services, keyRaw)
			if err != nil {
				return err
			}
			inputs.add("ticket key", key.String())
			number, err := application.resolveNumber(command.Context(), numberRaw)
			if err != nil {
				return err
			}
			inputs.add("ticket number", number.String())
			id := ticket.NewID(key, number)
			inputs.add("worktree ticket", id.String())
			if err := application.confirmMutation(command.Context(), "Remove task worktree", "Remove the task worktree of the completed ticket? Its working tree must be clean."); err != nil {
				return err
			}
			result, err := services.worktrees.RemoveWorktree(command.Context(), workflow.RemoveWorktreeRequest{
				Repository: repository,
				Ticket:     id,
				DryRun:     application.options.dryRun,
			})
			if err != nil {
				return err
			}
			return application.report(command, port.Report{
				Operation: "workflow.worktree.remove",
				Summary:   "Task worktree removal completed.",
				Fields: map[string]string{
					"worktreePath": result.Path,
					"ticket":       result.Ticket.String(),
					"dryRun":       boolString(result.DryRun),
				},
			})
		}),
	}
	registerTicketKeyFlag(command, &keyRaw)
	registerTicketNumberFlag(command, &numberRaw)
	return command
}

// newWorktreePruneCommand discovers the task worktrees whose completion
// evidence is proven and removes them under the registered-and-clean guards.
func newWorktreePruneCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "prune",
		Short: "Remove the task worktrees whose completion evidence is proven",
		RunE: func(command *cobra.Command, _ []string) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			if err := application.confirmMutation(command.Context(), "Prune task worktrees", "Remove every task worktree whose completion evidence is proven? Each candidate must be registered and clean."); err != nil {
				return err
			}
			result, err := services.worktrees.PruneWorktrees(command.Context(), workflow.PruneWorktreesRequest{
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
				Operation: "workflow.worktree.prune",
				Summary:   "Task worktree prune completed.",
				Fields: map[string]string{
					"worktreeCount": strconv.Itoa(len(result.Entries)),
					"removedCount":  strconv.Itoa(removed),
					"dryRun":        boolString(result.DryRun),
				},
				Data: result.Entries,
			})
		},
	}
}

// worktreeEntrySummary renders one inventory entry as a single human-facing
// line: the resolved commit, the checkout form, and the current marker.
func worktreeEntrySummary(entry port.WorktreeEntry, current string) string {
	parts := make([]string, 0, 3)
	if entry.Head != "" {
		parts = append(parts, entry.Head)
	}
	switch {
	case entry.Bare:
		parts = append(parts, "bare")
	case entry.Detached:
		parts = append(parts, "detached")
	case entry.Branch != "":
		parts = append(parts, "branch "+entry.Branch)
	}
	if filepath.Clean(entry.Path) == current {
		parts = append(parts, "(current)")
	}
	return strings.Join(parts, " ")
}
