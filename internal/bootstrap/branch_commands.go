package bootstrap

import (
	"strings"

	"github.com/spf13/cobra"
	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

func newBranchCommand(application *application) *cobra.Command {
	command := &cobra.Command{
		Use:   "branch",
		Short: "List, validate, create, and synchronize governed branches",
	}
	command.AddCommand(
		newBranchListCommand(application),
		newBranchValidateCommand(application),
		newBranchCreateCommand(application),
		newScratchMergeCommand(application),
		newBranchSyncBaseCommand(application),
	)
	return command
}

func newBranchListCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List every supported branch family",
		RunE: func(command *cobra.Command, _ []string) error {
			families := branchapp.ListFamilies()
			fields := make(map[string]string, len(families))
			for _, family := range families {
				fields[family.Family.String()] = family.Pattern + " — " + family.Description
			}
			return application.report(command, port.Report{
				Operation: "branch.list",
				Summary:   "Supported branch families:",
				Fields:    fields,
				Data:      families,
			})
		},
	}
}

func newBranchValidateCommand(application *application) *cobra.Command {
	var nameRaw string
	command := &cobra.Command{
		Use:   "validate",
		Short: "Validate a branch name and its local Git reference",
		RunE: func(command *cobra.Command, _ []string) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			name, err := currentOrSpecified(command.Context(), services, nameRaw, repository)
			if err != nil {
				return err
			}
			result, err := services.branches.Validate(command.Context(), branchapp.ValidateRequest{
				Repository: repository,
				Name:       name,
			})
			if err != nil {
				return err
			}
			return application.report(command, port.Report{
				Operation: "branch.validate",
				Summary:   "Branch is valid.",
				Fields: map[string]string{
					"branch": result.Name.String(),
					"family": result.Name.Family().String(),
				},
			})
		},
	}
	// Flag help texts in this file render the value domains of their endpoint;
	// every flag belongs to exactly one value class and carries its
	// class-specific help duty. Canonical conventions:
	// docs/conventions/cli/value-domain-model.md and
	// docs/conventions/cli/help-contract.md.
	command.Flags().StringVar(&nameRaw, "branch", "", "branch name; defaults to the current branch")
	return command
}

func newBranchCreateCommand(application *application) *cobra.Command {
	var (
		familyRaw string
		keyRaw    string
		numberRaw string
		slugRaw   string
		baseRaw   string
		switchTo  bool
	)
	command := &cobra.Command{
		Use:   "create",
		Short: "Create a regular governed branch",
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			family, err := application.resolveFamily(command.Context(), familyRaw, false)
			if err != nil {
				return err
			}
			inputs.add("branch family", family.String())
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
			slug, err := application.resolveSlug(command.Context(), slugRaw, "Branch description")
			if err != nil {
				return err
			}
			inputs.add("branch description", slug.String())
			var base *branch.TargetBase
			if family == branch.FamilyScratch {
				base, err = application.resolveScratchBase(command.Context(), baseRaw, repository.Remote, id)
			} else {
				base, err = parseBase(baseRaw, repository.Remote)
			}
			if err != nil {
				return err
			}
			if base != nil {
				inputs.add("target base", base.String())
			}
			name, err := branch.NewTicketBranch(family, id, slug)
			if err != nil {
				return err
			}
			if err := application.confirmMutation(command.Context(), "Create branch", "Create "+name.String()+" from the canonical target base?"); err != nil {
				return err
			}
			result, err := services.branches.Create(command.Context(), branchapp.CreateRequest{
				Repository: repository,
				Family:     family,
				Ticket:     id,
				Slug:       slug,
				Base:       base,
				Switch:     &switchTo,
				DryRun:     application.options.dryRun,
			})
			if err != nil {
				return err
			}
			return application.report(command, port.Report{
				Operation: "branch.create",
				Summary: application.withInteractiveFetchSummary(
					branchCreationSummary(result),
					repository.Remote,
					fetchCompleted(result.DryRun, result.Plan),
				),
				Fields: map[string]string{
					"branch":   result.Name.String(),
					"base":     result.Base.String(),
					"switched": boolString(result.Switched),
					"dryRun":   boolString(result.DryRun),
					"plan":     planText(result.Plan),
				},
			})
		}),
	}
	registerBranchFamilyFlag(command, &familyRaw, "")
	registerTicketKeyFlag(command, &keyRaw)
	registerTicketNumberFlag(command, &numberRaw)
	registerSlugFlag(command, &slugRaw, "slug", "")
	command.Flags().StringVar(&baseRaw, "base", "", "explicit base for eligible branch families")
	command.Flags().BoolVar(&switchTo, "switch", true, "switch to the branch after creating it")
	return command
}

func newScratchMergeCommand(application *application) *cobra.Command {
	var (
		sourceRaw            string
		targetRaw            string
		commitFamily         string
		commitSubject        string
		commitBody           string
		commitFooters        []string
		commitBreaking       bool
		commitBreakingImpact string
	)
	command := &cobra.Command{
		Use:   "merge-scratch",
		Short: "Squash a private scratch branch into its official ticket branch",
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			source, err := currentOrSpecified(command.Context(), services, sourceRaw, repository)
			if err != nil {
				return err
			}
			inputs.add("scratch branch", source.String())

			var explicitTarget *branch.BranchName
			if targetRaw != "" {
				target, err := branch.ParseName(targetRaw)
				if err != nil {
					return err
				}
				explicitTarget = &target
			}
			target, err := services.scratch.ResolveTarget(command.Context(), repository, source, explicitTarget)
			if err != nil {
				return err
			}
			inputs.add("official ticket branch", target.String())
			message, err := application.resolveScratchMergeMessage(
				command.Context(),
				repository,
				target,
				commitFamily,
				commitSubject,
				commitBody,
				commitBreaking,
				commitBreakingImpact,
				commitFooters,
			)
			if err != nil {
				return err
			}
			inputs.add("squash commit family", message.Header().Type().String())
			inputs.add("squash commit description", message.Header().Subject())

			if err := application.confirmMutation(
				command.Context(),
				"Squash merge scratch branch",
				"Squash-merge "+source.String()+" into "+target.String()+" as one commit?",
			); err != nil {
				return err
			}
			result, err := services.scratch.Merge(command.Context(), branchapp.ScratchMergeRequest{
				Repository: repository,
				Source:     source,
				Target:     &target,
				Message:    message,
				DryRun:     application.options.dryRun,
			})
			if err != nil {
				return err
			}
			return application.report(command, port.Report{
				Operation: "branch.merge-scratch",
				Summary:   scratchMergeSummary(result),
				Fields: map[string]string{
					"scratchBranch":  result.Source.String(),
					"officialBranch": result.Target.String(),
					"commit":         result.Message.Header().String(),
					"committed":      boolString(result.Committed),
					"dryRun":         boolString(result.DryRun),
					"plan":           planText(result.Plan),
				},
			})
		}),
	}
	command.Flags().StringVar(&sourceRaw, "branch", "", "scratch branch; defaults to the current branch")
	command.Flags().StringVar(&targetRaw, "target", "", "optional local official ticket branch target")
	registerCommitTypeFlag(command, &commitFamily, "for the squashed change")
	registerSubjectFlag(command, &commitSubject, "subject", "for the squashed change")
	registerBodyFlag(command, &commitBody, "body", "documenting the discarded experiment paths (mandatory)")
	registerFooterFlag(command, &commitFooters, "footer", "")
	command.Flags().BoolVar(&commitBreaking, "breaking", false, "mark an incompatible public contract change")
	registerBreakingDescriptionFlag(command, &commitBreakingImpact, "breaking-description", "")
	return command
}

func newBranchSyncBaseCommand(application *application) *cobra.Command {
	var (
		nameRaw             string
		baseRaw             string
		strategyRaw         string
		mergeFamily         string
		mergeSubject        string
		mergeBody           string
		mergeFooters        []string
		mergeBreaking       bool
		mergeBreakingImpact string
		resume              bool
	)
	command := &cobra.Command{
		Use:   "sync-base",
		Short: "Check, rebase, or merge the current branch target base safely",
		RunE: func(command *cobra.Command, _ []string) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			name, err := currentOrSpecified(command.Context(), services, nameRaw, repository)
			if err != nil {
				return err
			}
			if nameRaw != "" {
				current, err := services.git.CurrentBranch(command.Context(), repository)
				if err != nil {
					return err
				}
				if current.String() != name.String() {
					return syncBaseBranchNotCurrent(current, name)
				}
			}
			base, err := parseBase(baseRaw, repository.Remote)
			if err != nil {
				return err
			}
			var result branchapp.SyncResult
			if resume {
				if application.options.dryRun {
					return invalidOption("resume", "true", "a non-dry-run invocation")
				}
				if command.Flags().Changed("strategy") {
					return invalidOption("strategy", strategyRaw, "omitted; --resume continues the paused Git operation")
				}
				if mergeFamily != "" || mergeSubject != "" || mergeBody != "" || len(mergeFooters) > 0 || mergeBreaking || mergeBreakingImpact != "" {
					return invalidOption("merge commit input", "configured", "omitted; --resume continues the paused Git operation")
				}
				if err := application.confirmMutation(
					command.Context(),
					"Resume branch base synchronization",
					"Continue the resolved rebase or merge on "+name.String()+" if policy permits?",
				); err != nil {
					return err
				}
				result, err = services.sync.ResumeSync(command.Context(), branchapp.ResumeSyncRequest{
					Repository: repository,
					Name:       name,
					Base:       base,
				})
				if err != nil {
					return err
				}
			} else {
				strategy := branchapp.SyncStrategy(strategyRaw)
				var parsedMergeMessage *commitmsg.Message
				if strategy == branchapp.SyncMerge {
					message, err := application.resolveCommitMessage(command.Context(), commitMessageInput{
						Branch:               name,
						Repository:           repository,
						Family:               mergeFamily,
						Description:          mergeSubject,
						Body:                 mergeBody,
						Breaking:             mergeBreaking,
						BreakingDescription:  mergeBreakingImpact,
						FooterSpecifications: mergeFooters,
						DescriptionLabel:     "Merge commit description",
						Operation:            "the branch-base synchronization merge",
					})
					if err != nil {
						return err
					}
					parsedMergeMessage = &message
				} else if mergeFamily != "" || mergeSubject != "" || mergeBody != "" || len(mergeFooters) > 0 || mergeBreaking || mergeBreakingImpact != "" {
					return invalidOption("merge commit input", "configured", "merge commit inputs are only supported with --strategy merge")
				}
				if strategy == branchapp.SyncRebase || strategy == branchapp.SyncMerge {
					if err := application.confirmMutation(command.Context(), "Synchronize branch base", "Apply "+strategyRaw+" to "+name.String()+" if policy permits?"); err != nil {
						return err
					}
				}
				result, err = services.sync.Sync(command.Context(), branchapp.SyncRequest{
					Repository:   repository,
					Name:         name,
					Base:         base,
					Strategy:     strategy,
					MergeMessage: parsedMergeMessage,
					DryRun:       application.options.dryRun,
				})
				if err != nil {
					return err
				}
			}
			fields := map[string]string{
				"branch":             result.Name.String(),
				"base":               result.Base.String(),
				"publication":        string(result.Publication),
				"missingBaseCommits": boolString(result.MissingBaseCommits),
				"mutated":            boolString(result.Mutated),
				"recommendedAction":  result.RecommendedAction,
			}
			if result.Quality != nil {
				fields["qualityStatus"] = string(result.Quality.Status)
				fields["qualityDetail"] = result.Quality.Detail
			}
			return application.report(command, port.Report{
				Operation: "branch.sync-base",
				Summary: application.withInteractiveFetchSummary(
					"Branch base synchronization checked.",
					repository.Remote,
					!application.options.dryRun,
				),
				Fields: fields,
			})
		},
	}
	command.Flags().StringVar(&nameRaw, "branch", "", "branch name; must match the current branch when supplied")
	command.Flags().StringVar(&baseRaw, "base", "", "explicit remote target base")
	registerSyncStrategyFlag(command, &strategyRaw)
	registerMergeTypeFlag(command, &mergeFamily)
	registerSubjectFlag(command, &mergeSubject, "merge-subject", "for --strategy merge")
	registerBodyFlag(command, &mergeBody, "merge-body", "for the --strategy merge commit")
	registerFooterFlag(command, &mergeFooters, "merge-footer", "for the merge commit")
	command.Flags().BoolVar(&mergeBreaking, "merge-breaking", false, "mark an incompatible public contract change in the merge commit")
	registerBreakingDescriptionFlag(command, &mergeBreakingImpact, "merge-breaking-description", "for the merge commit")
	command.Flags().BoolVar(&resume, "resume", false, "continue a manually resolved rebase or merge")
	return command
}

func syncBaseBranchNotCurrent(current, requested branch.BranchName) error {
	return problem.New(problem.Details{
		Code:        problem.CodeInvalidInput,
		Category:    problem.CategoryRepository,
		Field:       "branch",
		Actual:      current.String(),
		Expected:    requested.String(),
		Rule:        "branch synchronization may mutate only the checked-out branch",
		Example:     "git governance branch sync-base --strategy merge",
		Remediation: "switch to the requested branch before running branch sync-base",
	})
}

func branchCreationSummary(result branchapp.CreateResult) string {
	if result.DryRun {
		return "Branch creation plan generated."
	}
	return "Branch created."
}

func scratchMergeSummary(result branchapp.ScratchMergeResult) string {
	if result.DryRun {
		return "Scratch squash-merge plan generated."
	}
	return "Scratch branch squashed into the official ticket branch."
}

func planText(steps []branchapp.PlanStep) string {
	parts := make([]string, 0, len(steps))
	for _, step := range steps {
		parts = append(parts, step.String())
	}
	return strings.Join(parts, "; ")
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
