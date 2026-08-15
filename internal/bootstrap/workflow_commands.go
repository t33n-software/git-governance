package bootstrap

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/application/workflow"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/hotfix"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/releaserequest"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

func newWorkflowCommand(application *application) *cobra.Command {
	command := &cobra.Command{
		Use:   "workflow",
		Short: "Run bounded governed Git workflows",
	}
	command.AddCommand(
		newTicketWorkflowCommand(application),
		newHotfixWorkflowCommand(application),
		newReleaseWorkflowCommand(application),
		newCleanupWorkflowCommand(application),
	)
	return command
}

func newTicketWorkflowCommand(application *application) *cobra.Command {
	command := &cobra.Command{
		Use:   "ticket",
		Short: "Start and publish regular ticket work",
	}
	command.AddCommand(
		newTicketStartCommand(application),
		newTicketPublishCommand(application),
	)
	return command
}

func newTicketStartCommand(application *application) *cobra.Command {
	var (
		familyRaw     string
		keyRaw        string
		numberRaw     string
		slugRaw       string
		createScratch bool
		scratchSlug   string
	)
	command := &cobra.Command{
		Use:   "start",
		Short: "Create a regular ticket branch and optionally a private scratch branch",
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
			if !createScratch && application.promptAvailable() && !application.options.yes {
				createScratch, err = application.prompt().Confirm(command.Context(), port.ConfirmRequest{
					Label:       "Create a private scratch branch?",
					Description: "Scratch branches are for uncertain exploration only. Do not open a pull request from them; move stable work to the official ticket branch.",
					Default:     false,
				})
				if err != nil {
					return err
				}
			}
			inputs.add("create scratch branch", boolString(createScratch))
			var parsedScratchSlug branch.Slug
			if scratchSlug != "" {
				parsedScratchSlug, err = branch.ParseSlug(scratchSlug)
				if err != nil {
					return err
				}
				inputs.add("scratch branch description", parsedScratchSlug.String())
			}
			if err := application.confirmMutation(command.Context(), "Start ticket workflow", "Create the official ticket branch and any selected scratch branch?"); err != nil {
				return err
			}
			result, err := services.tickets.StartTicket(command.Context(), workflow.StartTicketRequest{
				Repository:    repository,
				Family:        family,
				Ticket:        id,
				Slug:          slug,
				CreateScratch: createScratch,
				ScratchSlug:   parsedScratchSlug,
				DryRun:        application.options.dryRun,
			})
			if err != nil {
				return err
			}
			fields := map[string]string{
				"officialBranch": result.Official.Name.String(),
				"activeBranch":   result.Active.String(),
				"dryRun":         boolString(application.options.dryRun),
			}
			if result.Scratch != nil {
				fields["scratchBranch"] = result.Scratch.Name.String()
			}
			return application.report(command, port.Report{
				Operation: "workflow.ticket.start",
				Summary: application.withInteractiveFetchSummary(
					"Ticket workflow start completed.",
					repository.Remote,
					fetchCompleted(result.Official.DryRun, result.Official.Plan),
				),
				Fields: fields,
			})
		}),
	}
	command.Flags().StringVar(&familyRaw, "family", "", "regular ticket branch family")
	command.Flags().StringVar(&keyRaw, "key", "", "ticket key")
	command.Flags().StringVar(&numberRaw, "ticket", "", "ticket number")
	command.Flags().StringVar(&slugRaw, "slug", "", "kebab-case branch description")
	command.Flags().BoolVar(&createScratch, "scratch", false, "create a private scratch branch")
	command.Flags().StringVar(&scratchSlug, "scratch-slug", "", "optional scratch branch slug")
	return command
}

func newTicketPublishCommand(application *application) *cobra.Command {
	var (
		branchRaw         string
		baseRaw           string
		scratchTargetRaw  string
		scratchMessageRaw string
		scratchFamilyRaw  string
		scratchSubjectRaw string
		push              bool
		createPullRequest bool
		draft             bool
		resume            bool
	)
	command := &cobra.Command{
		Use:   "publish",
		Short: "Validate, synchronize, optionally push, and prepare a ticket pull request",
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			name, err := currentOrSpecified(command.Context(), services, branchRaw, repository)
			if err != nil {
				return err
			}
			inputs.add("ticket branch", name.String())
			var (
				scratchTarget  *branch.BranchName
				scratchMessage *commitmsg.Message
			)
			if name.Family() == branch.FamilyScratch {
				var explicitTarget *branch.BranchName
				if scratchTargetRaw != "" {
					target, err := branch.ParseName(scratchTargetRaw)
					if err != nil {
						return err
					}
					explicitTarget = &target
				}
				target, err := services.scratch.ResolveTarget(command.Context(), repository, name, explicitTarget)
				if err != nil {
					return err
				}
				scratchTarget = &target
				inputs.add("official ticket branch", target.String())
				message, err := application.resolveScratchMergeMessage(
					command.Context(),
					scratchMessageRaw,
					scratchFamilyRaw,
					scratchSubjectRaw,
					target,
				)
				if err != nil {
					return err
				}
				scratchMessage = &message
				inputs.add("squash commit family", message.Header().Type().String())
				inputs.add("squash commit description", message.Header().Subject())
			} else if scratchTargetRaw != "" || scratchMessageRaw != "" || scratchFamilyRaw != "" || scratchSubjectRaw != "" {
				return invalidOption("scratch transfer", "configured", "--target, --message, --type, and --subject are only supported when publishing from scratch")
			}
			base, err := parseBase(baseRaw, repository.Remote)
			if err != nil {
				return err
			}
			if base != nil {
				inputs.add("target base", base.String())
			}
			if resume && application.options.dryRun {
				return invalidOption("resume", "true", "a non-dry-run invocation")
			}
			if err := application.validatePullRequestPublication(services, push, createPullRequest); err != nil {
				return err
			}
			label := "Publish ticket workflow"
			description := "Validate the commit series, synchronize safely, and optionally push the branch?"
			if resume {
				label = "Resume ticket publication"
				description = "Continue the resolved Git operation, revalidate the official branch, and optionally push it?"
			}
			if scratchTarget != nil && scratchMessage != nil {
				label = "Publish ticket workflow from scratch"
				description = "You are on private scratch branch " + name.String() +
					". Squash-merge it into " + scratchTarget.String() + " as " +
					scratchMessage.Header().String() +
					", then validate, synchronize safely, and optionally push the official branch?"
				if resume {
					label = "Resume ticket publication from scratch"
					description = "Commit the resolved scratch transfer into " + scratchTarget.String() +
						", then validate, synchronize safely, and optionally push the official branch?"
				}
			}
			if err := application.confirmMutation(command.Context(), label, description); err != nil {
				return err
			}
			var result workflow.PublishTicketResult
			if resume {
				if name.Family() == branch.FamilyScratch {
					scratchMerge, resumeErr := services.scratch.Resume(command.Context(), branchapp.ScratchMergeRequest{
						Repository: repository,
						Source:     name,
						Target:     scratchTarget,
						Message:    *scratchMessage,
					})
					if resumeErr != nil {
						return resumeErr
					}
					result, err = services.tickets.PublishTicket(command.Context(), workflow.PublishTicketRequest{
						Repository: repository,
						Branch:     scratchMerge.Target,
						Base:       base,
						Draft:      draft,
					})
					if err == nil {
						result.ScratchMerge = &scratchMerge
					}
				} else {
					result, err = services.tickets.ResumeTicketPublish(command.Context(), workflow.ResumeTicketPublishRequest{
						Repository: repository,
						Branch:     name,
						Base:       base,
						Draft:      draft,
					})
				}
			} else {
				result, err = services.tickets.PublishTicket(command.Context(), workflow.PublishTicketRequest{
					Repository:     repository,
					Branch:         name,
					Base:           base,
					ScratchTarget:  scratchTarget,
					ScratchMessage: scratchMessage,
					Draft:          draft,
					DryRun:         application.options.dryRun,
				})
			}
			if err != nil {
				if !resume && isScratchMergeConflict(err) && application.promptAvailable() && scratchTarget != nil && scratchMessage != nil {
					scratchMerge, resumeErr := application.resumeScratchMergeAfterConflict(
						command.Context(),
						services,
						repository,
						name,
						*scratchTarget,
						*scratchMessage,
					)
					if resumeErr != nil {
						return resumeErr
					}
					result, err = services.tickets.PublishTicket(command.Context(), workflow.PublishTicketRequest{
						Repository: repository,
						Branch:     *scratchTarget,
						Base:       base,
						Draft:      draft,
						DryRun:     application.options.dryRun,
					})
					if err == nil {
						result.ScratchMerge = &scratchMerge
					}
				}
			}
			if err != nil {
				if resume || !application.promptAvailable() || !isRebaseConflict(err) {
					return err
				}
				resumeBranch := name
				if scratchTarget != nil {
					resumeBranch = *scratchTarget
				}
				result, err = application.resumeTicketPublishAfterRebaseConflict(
					command.Context(),
					services,
					repository,
					resumeBranch,
					base,
					draft,
				)
				if err != nil {
					return err
				}
				if scratchTarget != nil && scratchMessage != nil {
					result.ScratchMerge = &branchapp.ScratchMergeResult{
						Source:    name,
						Target:    *scratchTarget,
						Message:   *scratchMessage,
						Committed: true,
					}
				}
			}
			if err := application.reportTicketSynchronization(command, result.Sync, result.DryRun); err != nil {
				return err
			}
			if err := application.completeTicketPublishInteraction(
				command.Context(),
				services,
				repository,
				&result,
				push,
				createPullRequest,
				false,
			); err != nil {
				return err
			}
			fields := map[string]string{
				"branch":               result.Branch.String(),
				"pushed":               boolString(result.Pushed),
				"syncAction":           result.Sync.RecommendedAction,
				"pullRequestSource":    result.PullRequest.Source.String(),
				"pullRequestTarget":    result.PullRequest.Target.String(),
				"pullRequestTitle":     result.PullRequest.Title,
				"publishedPullRequest": result.PublishedURL,
			}
			switch {
			case result.PublishedURL != "":
				fields["pullRequestPublication"] = "created"
			case services.tickets.HasPullRequestPublisher():
				fields["pullRequestPublication"] = "not requested"
			default:
				fields["pullRequestPublication"] = "intent-only; no hosting-provider adapter is configured"
			}
			if result.ScratchMerge != nil {
				fields["scratchBranch"] = result.ScratchMerge.Source.String()
				fields["squashMerged"] = boolString(result.ScratchMerge.Committed)
				fields["squashCommit"] = result.ScratchMerge.Message.Header().String()
			}
			addQualityFields(fields, result)
			return application.report(command, port.Report{
				Operation: "workflow.ticket.publish",
				Summary: application.withInteractiveFetchSummary(
					"Ticket publish workflow completed.",
					repository.Remote,
					!result.DryRun,
				),
				Fields: fields,
				Data:   result.PullRequest,
			})
		}),
	}
	command.Flags().StringVar(&branchRaw, "branch", "", "ticket branch; defaults to the current branch")
	command.Flags().StringVar(&baseRaw, "base", "", "explicit base for hotfix ticket publication")
	command.Flags().StringVar(&scratchTargetRaw, "target", "", "optional local official target when publishing from scratch")
	command.Flags().StringVar(&scratchFamilyRaw, "type", "", "commit family for a scratch squash transfer")
	command.Flags().StringVar(&scratchSubjectRaw, "subject", "", "commit description for a scratch squash transfer")
	command.Flags().StringVar(&scratchMessageRaw, "message", "", "complete commit message compatibility input for a scratch squash transfer")
	command.Flags().BoolVar(&push, "push", false, "push the branch after validation")
	command.Flags().BoolVar(&createPullRequest, "create-pull-request", false, "create the pull request through the configured provider after pushing")
	command.Flags().BoolVar(&draft, "draft", false, "mark the pull request intent as a draft")
	command.Flags().BoolVar(&resume, "resume", false, "continue a manually resolved rebase or scratch transfer")
	return command
}

func (application *application) reportTicketSynchronization(
	command *cobra.Command,
	result branchapp.SyncResult,
	dryRun bool,
) error {
	if dryRun || !application.promptAvailable() {
		return nil
	}
	summary := "Target-base synchronization completed without a rebase."
	switch result.RecommendedAction {
	case "rebased":
		summary = "Rebase completed successfully; the official branch is synchronized with its target base."
	case "none":
		summary = "No rebase was performed because the target base has no commits missing from the branch."
	case "merge":
		summary = "No rebase was performed because the branch is already published; a controlled merge is required if its target base advanced."
	}
	return application.report(command, port.Report{
		Operation: "workflow.ticket.publish.sync",
		Summary:   summary,
		Fields: map[string]string{
			"branch":     result.Name.String(),
			"targetBase": result.Base.String(),
			"syncAction": result.RecommendedAction,
		},
	})
}

func (application *application) resumeTicketPublishAfterRebaseConflict(
	ctx context.Context,
	services services,
	repository port.RepositoryIdentity,
	name branch.BranchName,
	base *branch.TargetBase,
	draft bool,
) (workflow.PublishTicketResult, error) {
	for {
		action, err := application.prompt().Select(ctx, port.SelectRequest{
			Label: "Rebase conflict requires resolution",
			Description: "Git paused the rebase because conflicts remain. Resolve every conflict, stage the resolutions, then select Retry. " +
				"Selecting Cancel leaves the Git rebase untouched.",
			Options: []port.SelectOption{
				{Value: "retry", Label: "Retry", Description: "Continue the resolved rebase and resume this ticket publication."},
				{Value: "cancel", Label: "Cancel", Description: "Leave the rebase paused for manual resolution."},
			},
			Default: "retry",
		})
		if err != nil {
			return workflow.PublishTicketResult{}, err
		}
		if action == "cancel" {
			return workflow.PublishTicketResult{}, problem.New(problem.Details{
				Code:        problem.CodeOperationCancelled,
				Category:    problem.CategoryCancelled,
				Field:       "rebase retry",
				Expected:    "Retry after resolving the rebase conflicts",
				Rule:        "the workflow leaves unresolved Git conflicts for explicit user resolution",
				Remediation: "resolve and stage the conflicts, then rerun ticket publish to resume the paused rebase",
			})
		}
		result, err := services.tickets.ResumeTicketPublish(ctx, workflow.ResumeTicketPublishRequest{
			Repository: repository,
			Branch:     name,
			Base:       base,
			Draft:      draft,
		})
		if err == nil {
			return result, nil
		}
		if !isRebaseConflict(err) {
			return workflow.PublishTicketResult{}, err
		}
	}
}

func (application *application) resumeScratchMergeAfterConflict(
	ctx context.Context,
	services services,
	repository port.RepositoryIdentity,
	source, target branch.BranchName,
	message commitmsg.Message,
) (branchapp.ScratchMergeResult, error) {
	for {
		action, err := application.prompt().Select(ctx, port.SelectRequest{
			Label: "Scratch merge conflict requires resolution",
			Description: "Git paused the scratch squash transfer because conflicts remain. Resolve every conflict, stage the resolutions, then select Retry. " +
				"Selecting Cancel leaves the squash transfer untouched.",
			Options: []port.SelectOption{
				{Value: "retry", Label: "Retry", Description: "Commit the resolved squash transfer and continue ticket publication."},
				{Value: "cancel", Label: "Cancel", Description: "Leave the unresolved scratch transfer for manual resolution."},
			},
			Default: "retry",
		})
		if err != nil {
			return branchapp.ScratchMergeResult{}, err
		}
		if action == "cancel" {
			return branchapp.ScratchMergeResult{}, problem.New(problem.Details{
				Code:        problem.CodeOperationCancelled,
				Category:    problem.CategoryCancelled,
				Field:       "scratch merge retry",
				Expected:    "Retry after resolving the scratch merge conflicts",
				Rule:        "the workflow leaves unresolved Git conflicts for explicit user resolution",
				Remediation: "resolve and stage the conflicts, then rerun ticket publish to resume the scratch transfer",
			})
		}
		result, err := services.scratch.Resume(ctx, branchapp.ScratchMergeRequest{
			Repository: repository,
			Source:     source,
			Target:     &target,
			Message:    message,
		})
		if err == nil {
			return result, nil
		}
		if !isScratchMergeConflict(err) {
			return branchapp.ScratchMergeResult{}, err
		}
	}
}

func (application *application) completeTicketPublishInteraction(
	ctx context.Context,
	services services,
	repository port.RepositoryIdentity,
	result *workflow.PublishTicketResult,
	requestedPush bool,
	requestedPullRequest bool,
	workflowManaged bool,
) error {
	return application.completePreparedPublication(
		ctx,
		services,
		repository,
		result,
		requestedPush,
		requestedPullRequest,
		workflowManaged,
	)
}

func isRebaseConflict(err error) bool {
	typed, ok := problem.As(err)
	return ok && typed.Code == problem.CodeRebaseConflict
}

func isScratchMergeConflict(err error) bool {
	typed, ok := problem.As(err)
	return ok && typed.Code == problem.CodeScratchMergeConflict
}

func newHotfixWorkflowCommand(application *application) *cobra.Command {
	var (
		keyRaw      string
		numberRaw   string
		slugRaw     string
		affectedRaw string
	)
	command := &cobra.Command{
		Use:   "hotfix",
		Short: "Start a hotfix from the active line that contains the defect",
	}
	start := &cobra.Command{
		Use:   "start",
		Short: "Create a hotfix branch from main, release, or support",
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
			slug, err := application.resolveSlug(command.Context(), slugRaw, "Hotfix description")
			if err != nil {
				return err
			}
			inputs.add("hotfix description", slug.String())
			affected, err := application.resolveAffectedLine(command.Context(), affectedRaw)
			if err != nil {
				return err
			}
			inputs.add("affected line", affected.String())
			if err := application.confirmMutation(command.Context(), "Start hotfix", "Create a hotfix from "+affected.String()+"?"); err != nil {
				return err
			}
			result, err := services.releases.StartHotfix(command.Context(), workflow.StartHotfixRequest{
				Repository:   repository,
				Ticket:       ticket.NewID(key, number),
				Slug:         slug,
				AffectedLine: affected,
				DryRun:       application.options.dryRun,
			})
			if err != nil {
				return err
			}
			return application.report(command, port.Report{
				Operation: "workflow.hotfix.start",
				Summary: application.withInteractiveFetchSummary(
					"Hotfix branch created.",
					repository.Remote,
					fetchCompleted(result.DryRun, result.Plan),
				),
				Fields: map[string]string{
					"branch": result.Name.String(),
					"base":   result.Base.String(),
					"dryRun": boolString(result.DryRun),
				},
			})
		}),
	}
	start.Flags().StringVar(&keyRaw, "key", "", "ticket key")
	start.Flags().StringVar(&numberRaw, "ticket", "", "ticket number")
	start.Flags().StringVar(&slugRaw, "slug", "", "kebab-case hotfix description")
	start.Flags().StringVar(&affectedRaw, "affected-line", "", "main, release/<semver>, or support/<major.minor>")
	command.AddCommand(
		start,
		newHotfixValidateRecordCommand(application),
		newHotfixVerifyMergeCommand(application),
		newHotfixVerifyDeliveryCommand(application),
		newHotfixPublishCommand(application),
		newHotfixPropagateCommand(application),
		newHotfixPropagateManifestCommand(application),
	)
	return command
}

func newHotfixValidateRecordCommand(application *application) *cobra.Command {
	var (
		branchRaw string
		recordRaw string
	)
	command := &cobra.Command{
		Use:   "validate-record",
		Short: "Validate the reviewed release record for a main hotfix",
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			name, err := currentOrSpecified(command.Context(), services, branchRaw, repository)
			if err != nil {
				return err
			}
			inputs.add("hotfix branch", name.String())
			if name.Family() != branch.FamilyHotfix {
				return invalidOption("branch", name.String(), "a hotfix/<ticket>-<slug> branch")
			}
			result, err := services.releases.ValidateMainHotfixRecord(command.Context(), workflow.ValidateMainHotfixRecordRequest{
				Repository: repository,
				Branch:     name,
				Location:   recordRaw,
			})
			if err != nil {
				return err
			}
			manifest := result.Record.Manifest()
			targets := result.Record.PropagationTargets()
			targetValues := make([]string, 0, len(targets))
			for _, target := range targets {
				targetValues = append(targetValues, target.String())
			}
			return application.report(command, port.Report{
				Operation: "workflow.hotfix.validate-record",
				Summary:   "Main hotfix release record is valid.",
				Fields: map[string]string{
					"ticket":                  result.Record.Ticket().String(),
					"incident":                result.Record.Incident(),
					"source":                  result.Record.ExpectedSource().String(),
					"affectedLine":            result.Record.AffectedLine().String(),
					"targetVersion":           result.Record.TargetVersion().String(),
					"previousTag":             result.Record.PreviousTag(),
					"manifestCommitCount":     strconv.Itoa(len(manifest)),
					"commitBudgetException":   result.Record.CommitBudgetException(),
					"scopeEscalationApproval": result.Record.ScopeEscalationApproval(),
					"propagationTargets":      strings.Join(targetValues, ","),
				},
			})
		}),
	}
	command.Flags().StringVar(&branchRaw, "branch", "", "hotfix branch; defaults to the current branch")
	command.Flags().StringVar(&recordRaw, "record", "", "repository-relative hotfix release record; defaults to the ticket record path")
	return command
}

func newHotfixVerifyMergeCommand(application *application) *cobra.Command {
	var (
		branchRaw string
		recordRaw string
	)
	command := &cobra.Command{
		Use:   "verify-merge",
		Short: "Verify merged main-hotfix evidence before immutable tagging",
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			name, err := currentOrSpecified(command.Context(), services, branchRaw, repository)
			if err != nil {
				return err
			}
			inputs.add("hotfix branch", name.String())
			result, err := services.releases.VerifyMainHotfixMerge(command.Context(), workflow.VerifyMainHotfixMergeRequest{
				Repository: repository,
				Branch:     name,
				Location:   recordRaw,
			})
			if err != nil {
				return err
			}
			fields := mainHotfixRecordFields(result.Record)
			fields["pullRequestURL"] = result.Evidence.PullRequestURL
			fields["mergeCommit"] = result.Evidence.MergeCommit
			fields["tag"] = result.Evidence.Tag
			return application.report(command, port.Report{
				Operation: "workflow.hotfix.verify-merge",
				Summary:   "Merged main hotfix evidence is valid for immutable patch tagging.",
				Fields:    fields,
			})
		}),
	}
	command.Flags().StringVar(&branchRaw, "branch", "", "merged hotfix branch; defaults to the current branch")
	command.Flags().StringVar(&recordRaw, "record", "", "repository-relative hotfix release record; defaults to the ticket record path")
	return command
}

func newHotfixVerifyDeliveryCommand(application *application) *cobra.Command {
	var (
		branchRaw string
		recordRaw string
	)
	command := &cobra.Command{
		Use:   "verify-delivery",
		Short: "Verify immutable main-hotfix patch delivery evidence",
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			name, err := currentOrSpecified(command.Context(), services, branchRaw, repository)
			if err != nil {
				return err
			}
			inputs.add("hotfix branch", name.String())
			result, err := services.releases.VerifyMainHotfixDelivery(command.Context(), workflow.VerifyMainHotfixDeliveryRequest{
				Repository: repository,
				Branch:     name,
				Location:   recordRaw,
			})
			if err != nil {
				return err
			}
			fields := mainHotfixRecordFields(result.Record)
			fields["pullRequestURL"] = result.Evidence.PullRequestURL
			fields["mergeCommit"] = result.Evidence.MergeCommit
			fields["tag"] = result.Evidence.Tag
			fields["releaseURL"] = result.Evidence.ReleaseURL
			fields["workflowRunURL"] = result.Evidence.WorkflowRunURL
			return application.report(command, port.Report{
				Operation: "workflow.hotfix.verify-delivery",
				Summary:   "Main hotfix patch delivery evidence is complete.",
				Fields:    fields,
			})
		}),
	}
	command.Flags().StringVar(&branchRaw, "branch", "", "merged hotfix branch; defaults to the current branch")
	command.Flags().StringVar(&recordRaw, "record", "", "repository-relative hotfix release record; defaults to the ticket record path")
	return command
}

func mainHotfixRecordFields(record hotfix.ReleaseRecord) map[string]string {
	manifest := record.Manifest()
	targets := record.PropagationTargets()
	targetValues := make([]string, 0, len(targets))
	for _, target := range targets {
		targetValues = append(targetValues, target.String())
	}
	return map[string]string{
		"ticket":                  record.Ticket().String(),
		"incident":                record.Incident(),
		"source":                  record.ExpectedSource().String(),
		"affectedLine":            record.AffectedLine().String(),
		"targetVersion":           record.TargetVersion().String(),
		"previousTag":             record.PreviousTag(),
		"manifestCommitCount":     strconv.Itoa(len(manifest)),
		"commitBudgetException":   record.CommitBudgetException(),
		"scopeEscalationApproval": record.ScopeEscalationApproval(),
		"propagationTargets":      strings.Join(targetValues, ","),
	}
}

func newHotfixPublishCommand(application *application) *cobra.Command {
	var (
		branchRaw         string
		affectedRaw       string
		push              bool
		createPullRequest bool
		draft             bool
		resume            bool
	)
	command := &cobra.Command{
		Use:   "publish",
		Short: "Validate, publish, and prepare a pull request for a hotfix",
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			name, err := currentOrSpecified(command.Context(), services, branchRaw, repository)
			if err != nil {
				return err
			}
			inputs.add("hotfix branch", name.String())
			if name.Family() != branch.FamilyHotfix {
				return invalidOption("branch", name.String(), "a hotfix/<ticket>-<slug> branch")
			}
			affected, err := application.resolveAffectedLine(command.Context(), affectedRaw)
			if err != nil {
				return err
			}
			inputs.add("affected line", affected.String())
			base, err := branch.NewTargetBase(repository.Remote, affected)
			if err != nil {
				return err
			}
			if resume && application.options.dryRun {
				return invalidOption("resume", "true", "a non-dry-run invocation")
			}
			if err := application.validatePullRequestPublication(services, push, createPullRequest); err != nil {
				return err
			}
			label := "Publish hotfix"
			description := "Validate the hotfix and prepare its pull request for " + affected.String() + "?"
			if resume {
				label = "Resume hotfix publication"
				description = "Continue the resolved hotfix rebase, revalidate the branch, and optionally push it?"
			}
			if err := application.confirmMutation(
				command.Context(),
				label,
				description,
			); err != nil {
				return err
			}
			var result workflow.PublishTicketResult
			if resume {
				result, err = services.tickets.ResumeTicketPublish(command.Context(), workflow.ResumeTicketPublishRequest{
					Repository: repository,
					Branch:     name,
					Base:       &base,
					Draft:      draft,
				})
			} else {
				result, err = services.tickets.PublishTicket(command.Context(), workflow.PublishTicketRequest{
					Repository: repository,
					Branch:     name,
					Base:       &base,
					Draft:      draft,
					DryRun:     application.options.dryRun,
				})
			}
			if err != nil {
				return err
			}
			if err := application.completePreparedPublication(
				command.Context(),
				services,
				repository,
				&result,
				push,
				createPullRequest,
				false,
			); err != nil {
				return err
			}
			fields := map[string]string{
				"branch":               result.Branch.String(),
				"affectedLine":         affected.String(),
				"pushed":               boolString(result.Pushed),
				"pullRequestSource":    result.PullRequest.Source.String(),
				"pullRequestTarget":    result.PullRequest.Target.String(),
				"pullRequestTitle":     result.PullRequest.Title,
				"publishedPullRequest": result.PublishedURL,
			}
			addQualityFields(fields, result)
			return application.report(command, port.Report{
				Operation: "workflow.hotfix.publish",
				Summary: application.withInteractiveFetchSummary(
					"Hotfix publish workflow completed.",
					repository.Remote,
					!result.DryRun,
				),
				Fields: fields,
				Data:   result.PullRequest,
			})
		}),
	}
	command.Flags().StringVar(&branchRaw, "branch", "", "hotfix branch; defaults to the current branch")
	command.Flags().StringVar(&affectedRaw, "affected-line", "", "main, release/<semver>, or support/<major.minor>")
	command.Flags().BoolVar(&push, "push", false, "push the hotfix branch after validation")
	command.Flags().BoolVar(&createPullRequest, "create-pull-request", false, "create the pull request through the configured provider after pushing")
	command.Flags().BoolVar(&draft, "draft", false, "mark the pull request intent as a draft")
	command.Flags().BoolVar(&resume, "resume", false, "continue a manually resolved rebase")
	return command
}

func newHotfixPropagateCommand(application *application) *cobra.Command {
	var (
		sourceRaw         string
		targetRaw         string
		commitID          string
		slugRaw           string
		branchRaw         string
		push              bool
		createPullRequest bool
		draft             bool
		resume            bool
	)
	command := &cobra.Command{
		Use:   "propagate",
		Short: "Forward-port or backport one reviewed hotfix commit",
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			if resume && application.options.dryRun {
				return invalidOption("resume", "true", "a non-dry-run invocation")
			}
			if err := application.validatePullRequestPublication(services, push, createPullRequest); err != nil {
				return err
			}
			if resume && sourceRaw == "" {
				return missingInput("hotfix source branch")
			}
			source, err := currentOrSpecified(command.Context(), services, sourceRaw, repository)
			if err != nil {
				return err
			}
			inputs.add("source branch", source.String())
			if source.Family() != branch.FamilyHotfix {
				return invalidOption("source", source.String(), "a hotfix/<ticket>-<slug> branch")
			}
			target, err := application.resolvePropagationTarget(command.Context(), targetRaw)
			if err != nil {
				return err
			}
			inputs.add("target line", target.String())
			if resume {
				if branchRaw == "" {
					return missingInput("propagation branch")
				}
				propagationBranch, err := branch.ParseName(branchRaw)
				if err != nil {
					return err
				}
				inputs.add("propagation branch", propagationBranch.String())
				if err := application.confirmMutation(
					command.Context(),
					"Resume hotfix propagation",
					"Continue the resolved cherry-pick on "+propagationBranch.String()+", then validate and optionally push it?",
				); err != nil {
					return err
				}
				result, err := services.releases.ResumeHotfixPropagation(command.Context(), workflow.ResumeHotfixPropagationRequest{
					Repository: repository,
					Source:     source,
					TargetLine: target,
					Branch:     propagationBranch,
					Draft:      draft,
				})
				if err != nil {
					return err
				}
				if err := application.completePreparedPublication(
					command.Context(),
					services,
					repository,
					&result.Publication,
					push,
					createPullRequest,
					true,
				); err != nil {
					return err
				}
				return application.report(command, port.Report{
					Operation: "workflow.hotfix.propagate",
					Summary: application.withInteractiveFetchSummary(
						"Hotfix propagation workflow resumed.",
						repository.Remote,
						true,
					),
					Fields: map[string]string{
						"source":               source.String(),
						"target":               target.String(),
						"branch":               result.Branch.Name.String(),
						"cherryPicked":         boolString(result.CherryPicked),
						"pushed":               boolString(result.Publication.Pushed),
						"pullRequestSource":    result.Publication.PullRequest.Source.String(),
						"pullRequestTarget":    result.Publication.PullRequest.Target.String(),
						"publishedPullRequest": result.Publication.PublishedURL,
					},
					Data: result.Publication.PullRequest,
				})
			}
			commitID, err = application.resolveReviewedCommit(command.Context(), commitID)
			if err != nil {
				return err
			}
			inputs.add("reviewed source commit", commitID)
			var slug branch.Slug
			if slugRaw != "" {
				slug, err = branch.ParseSlug(slugRaw)
				if err != nil {
					return err
				}
				inputs.add("branch description", slug.String())
			}
			if err := application.confirmMutation(
				command.Context(),
				"Propagate hotfix",
				"Create a controlled fix branch from "+target.String()+" and cherry-pick "+commitID+" with -x?",
			); err != nil {
				return err
			}
			result, err := services.releases.PropagateHotfix(command.Context(), workflow.PropagateHotfixRequest{
				Repository: repository,
				Source:     source,
				TargetLine: target,
				CommitID:   commitID,
				Slug:       slug,
				Draft:      draft,
				DryRun:     application.options.dryRun,
			})
			if err != nil {
				return err
			}
			if err := application.completePreparedPublication(
				command.Context(),
				services,
				repository,
				&result.Publication,
				push,
				createPullRequest,
				true,
			); err != nil {
				return err
			}
			return application.report(command, port.Report{
				Operation: "workflow.hotfix.propagate",
				Summary: application.withInteractiveFetchSummary(
					"Hotfix propagation workflow completed.",
					repository.Remote,
					fetchCompleted(result.Branch.DryRun, result.Branch.Plan) || !result.Publication.DryRun,
				),
				Fields: map[string]string{
					"source":               source.String(),
					"target":               target.String(),
					"branch":               result.Branch.Name.String(),
					"cherryPicked":         boolString(result.CherryPicked),
					"pushed":               boolString(result.Publication.Pushed),
					"pullRequestSource":    result.Publication.PullRequest.Source.String(),
					"pullRequestTarget":    result.Publication.PullRequest.Target.String(),
					"publishedPullRequest": result.Publication.PublishedURL,
				},
				Data: result.Publication.PullRequest,
			})
		}),
	}
	command.Flags().StringVar(&sourceRaw, "source", "", "hotfix source branch; defaults to the current branch")
	command.Flags().StringVar(&targetRaw, "target-line", "", "main, develop, release/<semver>, or support/<major.minor>")
	command.Flags().StringVar(&commitID, "commit", "", "reviewed source commit SHA")
	command.Flags().StringVar(&slugRaw, "slug", "", "optional kebab-case propagation branch description")
	command.Flags().StringVar(&branchRaw, "branch", "", "generated propagation branch; required with --resume")
	command.Flags().BoolVar(&push, "push", false, "push the propagation branch after validation")
	command.Flags().BoolVar(&createPullRequest, "create-pull-request", false, "create the pull request through the configured provider after pushing")
	command.Flags().BoolVar(&draft, "draft", false, "mark the pull request intent as a draft")
	command.Flags().BoolVar(&resume, "resume", false, "continue a manually resolved cherry-pick")
	return command
}

func newHotfixPropagateManifestCommand(application *application) *cobra.Command {
	var (
		sourceRaw string
		targetRaw string
		recordRaw string
		slugRaw   string
		branchRaw string
		resume    bool
		publish   bool
	)
	command := &cobra.Command{
		Use:   "propagate-manifest",
		Short: "Prepare or publish an ordered multi-commit hotfix propagation candidate",
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			if resume && application.options.dryRun {
				return invalidOption("resume", "true", "a non-dry-run invocation")
			}
			if publish && !application.hotfixManifestPublisherEnabled() {
				return hotfixManifestPublisherUnavailable()
			}
			if resume && sourceRaw == "" {
				return missingInput("hotfix source branch")
			}
			source, err := currentOrSpecified(command.Context(), services, sourceRaw, repository)
			if err != nil {
				return err
			}
			inputs.add("source branch", source.String())
			if source.Family() != branch.FamilyHotfix {
				return invalidOption("source", source.String(), "a hotfix/<ticket>-<slug> branch")
			}
			target, err := application.resolvePropagationTarget(command.Context(), targetRaw)
			if err != nil {
				return err
			}
			inputs.add("target line", target.String())
			if resume {
				if branchRaw == "" {
					return missingInput("propagation branch")
				}
				candidate, err := branch.ParseName(branchRaw)
				if err != nil {
					return err
				}
				inputs.add("propagation branch", candidate.String())
				label := "Resume hotfix manifest propagation"
				description := "Continue the resolved ordered cherry-pick on " + candidate.String() + " without publishing it?"
				if publish {
					label = "Publish resumed hotfix manifest propagation"
					description = "Continue the resolved ordered cherry-pick on " + candidate.String() + " and publish it only through the dedicated server-side publisher?"
				}
				if err := application.confirmMutation(
					command.Context(),
					label,
					description,
				); err != nil {
					return err
				}
				result, err := services.releases.ResumeHotfixManifestPropagation(
					command.Context(),
					workflow.ResumeHotfixManifestPropagationRequest{
						Repository: repository,
						Source:     source,
						TargetLine: target,
						Branch:     candidate,
						Location:   recordRaw,
						Publish:    publish,
					},
				)
				if err != nil {
					return err
				}
				summary := "Hotfix manifest propagation candidate resumed and verified."
				if publish {
					summary = "Hotfix manifest propagation candidate resumed, verified, and published."
				}
				return application.report(command, port.Report{
					Operation: "workflow.hotfix.propagate-manifest",
					Summary:   summary,
					Fields:    manifestPropagationFields(result, true),
				})
			}

			var slug branch.Slug
			if slugRaw != "" {
				slug, err = branch.ParseSlug(slugRaw)
				if err != nil {
					return err
				}
				inputs.add("propagation description", slug.String())
			}
			label := "Prepare hotfix manifest propagation"
			description := "Create a controlled local fix branch from " + target.String() + " and apply the reviewed manifest without publishing it?"
			if publish {
				label = "Publish hotfix manifest propagation"
				description = "Create a controlled fix branch from " + target.String() + ", apply the reviewed manifest, and publish it only through the dedicated server-side publisher?"
			}
			if err := application.confirmMutation(
				command.Context(),
				label,
				description,
			); err != nil {
				return err
			}
			result, err := services.releases.PropagateHotfixManifest(
				command.Context(),
				workflow.PropagateHotfixManifestRequest{
					Repository: repository,
					Source:     source,
					TargetLine: target,
					Location:   recordRaw,
					Slug:       slug,
					Publish:    publish,
					DryRun:     application.options.dryRun,
				},
			)
			if err != nil {
				return err
			}
			summary := "Hotfix manifest propagation candidate prepared without publication."
			if publish {
				summary = "Hotfix manifest propagation candidate prepared, verified, and published."
			}
			return application.report(command, port.Report{
				Operation: "workflow.hotfix.propagate-manifest",
				Summary:   summary,
				Fields:    manifestPropagationFields(result, false),
			})
		}),
	}
	command.Flags().StringVar(&sourceRaw, "source", "", "hotfix source branch; defaults to the current branch")
	command.Flags().StringVar(&targetRaw, "target-line", "", "declared develop, release/<semver>, or support/<major.minor> target")
	command.Flags().StringVar(&recordRaw, "record", "", "repository-relative hotfix release record; defaults to the ticket record path")
	command.Flags().StringVar(&slugRaw, "slug", "", "optional kebab-case propagation branch description")
	command.Flags().StringVar(&branchRaw, "branch", "", "generated fix branch; required with --resume")
	command.Flags().BoolVar(&resume, "resume", false, "continue a manually resolved manifest cherry-pick")
	command.Flags().BoolVar(&publish, "publish", false, "publish only through the dedicated server-side hotfix propagation publisher")
	return command
}

func manifestPropagationFields(result workflow.PropagateHotfixManifestResult, resumed bool) map[string]string {
	pushed := "false"
	publishedPullRequest := ""
	if result.Publication != nil {
		pushed = boolString(result.Publication.Pushed)
		publishedPullRequest = result.Publication.PublishedURL
	}
	fields := map[string]string{
		"source":               result.Record.ExpectedSource().String(),
		"target":               result.Branch.Base.Branch().String(),
		"branch":               result.Branch.Name.String(),
		"cherryPickCount":      strconv.Itoa(result.CherryPickCount),
		"manifestCommitCount":  strconv.Itoa(len(result.Record.Manifest())),
		"pushed":               pushed,
		"publishedPullRequest": publishedPullRequest,
		"resumed":              boolString(resumed),
		"dryRun":               boolString(result.DryRun),
	}
	if result.Quality != nil {
		fields["qualityStatus"] = string(result.Quality.Status)
		fields["qualityDetail"] = result.Quality.Detail
	}
	return fields
}

func (application *application) hotfixManifestPublisherEnabled() bool {
	return application.runtime.HotfixPropagationPublisherEnabled != nil &&
		application.runtime.HotfixPropagationPublisherEnabled()
}

func (application *application) protectedLineRequestControllerEnabled() bool {
	return application.runtime.GitHubWorkflowTokenEnabled != nil &&
		application.runtime.GitHubWorkflowTokenEnabled() &&
		application.runtime.GitHubWorkflowToken != nil &&
		strings.TrimSpace(application.runtime.GitHubWorkflowToken()) != ""
}

func protectedLineRequestControllerUnavailable() error {
	return problem.New(problem.Details{
		Code:        problem.CodeConfigurationUnavailable,
		Category:    problem.CategoryConfig,
		Field:       "protected-line request controller",
		Expected:    "the designated GitHub Actions request, execution, or finalizer job with an ephemeral job token",
		Rule:        "protected-line requests and execution are authorized only by their separate server-side controllers",
		Remediation: "use the protected release-request, release-execution, or recovery workflow instead of a local CLI invocation",
	})
}

func protectedLineDirectDispatchDisabled() error {
	return problem.New(problem.Details{
		Code:        problem.CodeConfigurationUnavailable,
		Category:    problem.CategoryGovernance,
		Field:       "protected-line executor dispatch",
		Expected:    "a durable request authorized by the protected release-request controller",
		Rule:        "normal protected-line execution cannot be dispatched directly from a local CLI invocation",
		Remediation: "request the release or support line through the protected release-control workflow",
	})
}

func hotfixManifestPublisherUnavailable() error {
	return problem.New(problem.Details{
		Code:        problem.CodeConfigurationUnavailable,
		Category:    problem.CategoryConfig,
		Field:       "hotfix propagation publisher",
		Expected:    "the dedicated server-side hotfix propagation publisher boundary",
		Rule:        "manifest candidates may be published only by the protected hotfix propagation controller",
		Remediation: "run the protected hotfix propagation workflow after its broker, workload identity, and publisher application are configured",
	})
}

func newReleaseWorkflowCommand(application *application) *cobra.Command {
	command := &cobra.Command{
		Use:   "release",
		Short: "Cut and reconcile governed release lines",
	}
	command.AddCommand(
		newReleaseRequestCommand(application),
		newReleaseExecuteRequestCommand(application),
		newReleaseFinalizeRequestCommand(application),
		newReleaseCutCommand(application),
		newReleaseStabilizeCommand(application),
		newReleasePublishStabilizationCommand(application),
		newReleaseAlignPromotionBaseCommand(application),
		newReleasePromotionCommand(application),
		newReleaseBackmergeCommand(application),
		newReleaseAlignReconciliationBaseCommand(application),
		newSupportPrepareCommand(application),
	)
	return command
}

func newCleanupWorkflowCommand(application *application) *cobra.Command {
	var branchRaw string
	command := &cobra.Command{
		Use:   "cleanup",
		Short: "Delete a local private scratch branch without deleting a remote branch",
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			name, err := currentOrSpecified(command.Context(), services, branchRaw, repository)
			if err != nil {
				return err
			}
			inputs.add("scratch branch", name.String())
			if err := application.confirmMutation(
				command.Context(),
				"Clean up scratch branch",
				"Delete "+name.String()+" locally? Official branch lifecycle and remote deletion remain GitHub, GitLab, or CI responsibilities.",
			); err != nil {
				return err
			}
			result, err := services.releases.CleanupBranch(command.Context(), workflow.CleanupBranchRequest{
				Repository: repository,
				Branch:     name,
				DryRun:     application.options.dryRun,
			})
			if err != nil {
				return err
			}
			return application.report(command, port.Report{
				Operation: "workflow.cleanup",
				Summary:   "Branch cleanup completed.",
				Fields: map[string]string{
					"branch":          result.Branch.String(),
					"deletedLocal":    boolString(result.DeletedLocal),
					"metadataCleared": boolString(result.MetadataCleared),
					"dryRun":          boolString(result.DryRun),
				},
			})
		}),
	}
	command.Flags().StringVar(&branchRaw, "branch", "", "local scratch branch; defaults to the current branch")
	return command
}

func addQualityFields(fields map[string]string, result workflow.PublishTicketResult) {
	fields["qualityStatus"] = string(result.Quality.Status)
	fields["qualityDetail"] = result.Quality.Detail
	if result.PostMutationQuality != nil {
		fields["postMutationQualityStatus"] = string(result.PostMutationQuality.Status)
		fields["postMutationQualityDetail"] = result.PostMutationQuality.Detail
	}
}

func newReleaseRequestCommand(application *application) *cobra.Command {
	var (
		kindRaw      string
		versionRaw   string
		keyRaw       string
		numberRaw    string
		requesterRaw string
		parentRunRaw string
	)
	command := &cobra.Command{
		Use:   "request",
		Short: "Authorize and dispatch one bound protected release or support-line request",
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			if !application.protectedLineRequestControllerEnabled() {
				return protectedLineRequestControllerUnavailable()
			}
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			operation, err := parseProtectedLineOperation(kindRaw)
			if err != nil {
				return err
			}
			inputs.add("protected-line operation", string(operation))
			version, err := application.resolveProtectedLineVersion(command.Context(), operation, versionRaw)
			if err != nil {
				return err
			}
			inputs.add("protected-line version", version)
			key, err := application.resolveKey(command.Context(), services, keyRaw)
			if err != nil {
				return err
			}
			number, err := application.resolveNumber(command.Context(), numberRaw)
			if err != nil {
				return err
			}
			id := ticket.NewID(key, number)
			inputs.add("ticket", id.String())
			requester, err := application.requireInput(command.Context(), requesterRaw, "Release request requester", "Enter the GitHub actor authorized by the request environment.")
			if err != nil {
				return err
			}
			inputs.add("requester", requester)
			parentRun, err := application.requireInput(command.Context(), parentRunRaw, "Release request controller run", "Enter the current GitHub Actions request-controller run ID.")
			if err != nil {
				return err
			}
			inputs.add("request controller run", parentRun)
			if err := application.confirmMutation(
				command.Context(),
				"Authorize protected-line request",
				"Persist and dispatch one protected-line request without directly mutating a shared line?",
			); err != nil {
				return err
			}
			result, err := services.releases.RequestProtectedLine(command.Context(), workflow.RequestProtectedLineRequest{
				Repository:  repository,
				Ticket:      id,
				Operation:   operation,
				Version:     version,
				Requester:   requester,
				ParentRunID: parentRun,
				DryRun:      application.options.dryRun,
			})
			if err != nil {
				return err
			}
			fields := map[string]string{
				"operation": result.Intent.Kind,
				"branch":    result.Intent.Branch.String(),
				"base":      result.Intent.Source.String(),
				"dryRun":    boolString(result.DryRun),
			}
			if !result.DryRun {
				fields["requestID"] = result.Request.Request.ID()
				fields["deploymentID"] = strconv.FormatInt(result.Request.Request.DeploymentID(), 10)
				fields["state"] = string(result.Request.Request.State())
				fields["expiresAt"] = result.Request.Request.ExpiresAt().Format(time.RFC3339)
			}
			return application.report(command, port.Report{
				Operation: "workflow.release.request",
				Summary:   "Protected-line request authorized and execution dispatched.",
				Fields:    fields,
				Data:      result.Intent,
			})
		}),
	}
	command.Flags().StringVar(&kindRaw, "kind", "", "protected line operation: release or support")
	command.Flags().StringVar(&versionRaw, "version", "", "release semantic version or support major.minor version")
	command.Flags().StringVar(&keyRaw, "key", "", "ticket key")
	command.Flags().StringVar(&numberRaw, "ticket", "", "ticket number")
	command.Flags().StringVar(&requesterRaw, "requester", "", "request-authority actor")
	command.Flags().StringVar(&parentRunRaw, "parent-run", "", "request-controller workflow run ID")
	return command
}

func newReleaseExecuteRequestCommand(application *application) *cobra.Command {
	var (
		requestIDRaw   string
		executorRunRaw string
	)
	command := &cobra.Command{
		Use:    "execute-request",
		Short:  "Validate one bound protected-line request before its workflow mutation",
		Hidden: true,
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			if !application.protectedLineRequestControllerEnabled() {
				return protectedLineRequestControllerUnavailable()
			}
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			requestID, err := application.requireInput(command.Context(), requestIDRaw, "Protected-line request ID", "Enter the durable request ID dispatched by the request controller.")
			if err != nil {
				return err
			}
			executorRun, err := application.requireInput(command.Context(), executorRunRaw, "Protected-line executor run", "Enter the current execution workflow run ID.")
			if err != nil {
				return err
			}
			inputs.add("request ID", requestID)
			inputs.add("executor run", executorRun)
			if err := application.confirmMutation(
				command.Context(),
				"Authorize protected-line execution",
				"Bind this executor run to one authorized protected-line request?",
			); err != nil {
				return err
			}
			plan, err := services.releases.AuthorizeProtectedLineExecution(command.Context(), repository, requestID, executorRun)
			if err != nil {
				return err
			}
			return application.report(command, port.Report{
				Operation: "workflow.release.execute-request",
				Summary:   "Protected-line execution request validated.",
				Fields: map[string]string{
					"requestID":     plan.Request.ID(),
					"source":        plan.Request.Source().String(),
					"sourceSHA":     plan.Request.SourceSHA(),
					"target":        plan.Request.Target().String(),
					"needsMutation": boolString(plan.NeedsMutation),
					"state":         string(plan.Request.State()),
				},
			})
		}),
	}
	command.Flags().StringVar(&requestIDRaw, "request-id", "", "durable protected-line request ID")
	command.Flags().StringVar(&executorRunRaw, "executor-run", "", "current protected-line executor workflow run ID")
	return command
}

func newReleaseFinalizeRequestCommand(application *application) *cobra.Command {
	var (
		requestIDRaw   string
		executorRunRaw string
		recovery       bool
	)
	command := &cobra.Command{
		Use:    "finalize-request",
		Short:  "Read-only finalize one protected-line execution request",
		Hidden: true,
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			if !application.protectedLineRequestControllerEnabled() {
				return protectedLineRequestControllerUnavailable()
			}
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			requestID, err := application.requireInput(command.Context(), requestIDRaw, "Protected-line request ID", "Enter the durable request ID to finalize.")
			if err != nil {
				return err
			}
			executorRun := strings.TrimSpace(executorRunRaw)
			if !recovery {
				executorRun, err = application.requireInput(command.Context(), executorRunRaw, "Protected-line executor run", "Enter the correlated execution workflow run ID.")
				if err != nil {
					return err
				}
			}
			inputs.add("request ID", requestID)
			inputs.add("recovery", boolString(recovery))
			if executorRun != "" {
				inputs.add("executor run", executorRun)
			}
			if err := application.confirmMutation(
				command.Context(),
				"Finalize protected-line request",
				"Write only the independently verified audit state for this protected-line request?",
			); err != nil {
				return err
			}
			result, err := services.releases.FinalizeProtectedLineRequest(command.Context(), repository, requestID, executorRun, recovery)
			if err != nil {
				return err
			}
			return application.report(command, port.Report{
				Operation: "workflow.release.finalize-request",
				Summary:   "Protected-line request finalization completed.",
				Fields: map[string]string{
					"requestID":    result.Request.ID(),
					"state":        string(result.Request.State()),
					"target":       result.Request.Target().String(),
					"sourceSHA":    result.Request.SourceSHA(),
					"executorRun":  result.Request.ExecutorRunID(),
					"deploymentID": strconv.FormatInt(result.Request.DeploymentID(), 10),
				},
			})
		}),
	}
	command.Flags().StringVar(&requestIDRaw, "request-id", "", "durable protected-line request ID")
	command.Flags().StringVar(&executorRunRaw, "executor-run", "", "correlated protected-line executor workflow run ID")
	command.Flags().BoolVar(&recovery, "recovery", false, "allow only read-only finalization of a verification-pending request")
	return command
}

func parseProtectedLineOperation(raw string) (releaserequest.Operation, error) {
	switch releaserequest.Operation(raw) {
	case releaserequest.OperationRelease, releaserequest.OperationSupport:
		return releaserequest.Operation(raw), nil
	default:
		return "", problem.New(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryGovernance,
			Field:       "protected-line operation",
			Actual:      raw,
			Expected:    "release or support",
			Rule:        "request authorization binds one protected release or support-line operation",
			Remediation: "select release for develop-derived cuts or support for a released main line",
		})
	}
}

func (application *application) resolveProtectedLineVersion(
	ctx context.Context,
	operation releaserequest.Operation,
	raw string,
) (string, error) {
	switch operation {
	case releaserequest.OperationRelease:
		version, err := application.resolveReleaseVersion(ctx, raw)
		if err != nil {
			return "", err
		}
		return version.String(), nil
	case releaserequest.OperationSupport:
		version, err := application.resolveSupportVersion(ctx, raw)
		if err != nil {
			return "", err
		}
		return version.String(), nil
	default:
		_, err := parseProtectedLineOperation(string(operation))
		return "", err
	}
}

func newReleaseCutCommand(application *application) *cobra.Command {
	var (
		versionRaw string
		dispatch   bool
	)
	command := &cobra.Command{
		Use:   "cut",
		Short: "Prepare a protected CI request for release/<semver> from develop",
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			version, err := application.resolveReleaseVersion(command.Context(), versionRaw)
			if err != nil {
				return err
			}
			inputs.add("release version", version.String())
			if dispatch && !application.options.dryRun {
				return protectedLineDirectDispatchDisabled()
			}
			if err := application.confirmMutation(command.Context(), "Cut release", "Create release/"+version.String()+" from origin/develop?"); err != nil {
				return err
			}
			result, err := services.releases.CutRelease(command.Context(), workflow.CutReleaseRequest{
				Repository: repository,
				Version:    version,
				DryRun:     application.options.dryRun,
			})
			if err != nil {
				return err
			}
			return application.report(command, port.Report{
				Operation: "workflow.release.cut",
				Summary: application.withInteractiveFetchSummary(
					"Protected release-line creation intent prepared.",
					repository.Remote,
					fetchCompleted(result.DryRun, result.Plan),
				),
				Fields: map[string]string{
					"branch":            result.Intent.Branch.String(),
					"base":              result.Intent.Source.String(),
					"workflow":          result.Intent.Workflow,
					"dispatchRequested": boolString(dispatch),
					"dryRun":            boolString(result.DryRun),
				},
				Data: result.Intent,
			})
		}),
	}
	command.Flags().StringVar(&versionRaw, "version", "", "release semantic version")
	command.Flags().BoolVar(&dispatch, "dispatch", false, "request direct executor dispatch (rejected outside dry-run)")
	return command
}

func newReleaseStabilizeCommand(application *application) *cobra.Command {
	var (
		releaseRaw string
		kindRaw    string
		keyRaw     string
		numberRaw  string
		slugRaw    string
		switchTo   bool
	)
	command := &cobra.Command{
		Use:   "stabilize",
		Short: "Create a permitted stabilization branch from a frozen release line",
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			release, err := application.resolveReleaseLine(
				command.Context(),
				releaseRaw,
				"Release line",
				"Enter release/<semantic-version> for the frozen line that contains this stabilization task. Examples: release/2.8.0, release/2.8.0-rc.1.",
			)
			if err != nil {
				return err
			}
			inputs.add("release line", release.String())
			kind, err := application.resolveStabilizationKind(command.Context(), kindRaw)
			if err != nil {
				return err
			}
			inputs.add("stabilization kind", string(kind))
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
			slug, err := application.resolveSlug(command.Context(), slugRaw, "Stabilization description")
			if err != nil {
				return err
			}
			inputs.add("stabilization description", slug.String())
			if err := application.confirmMutation(
				command.Context(),
				"Create release stabilization branch",
				"Create a "+kindRaw+" stabilization branch from "+release.String()+"?",
			); err != nil {
				return err
			}
			result, err := services.releases.CreateReleaseStabilization(command.Context(), workflow.CreateReleaseStabilizationRequest{
				Repository: repository,
				Release:    release,
				Ticket:     ticket.NewID(key, number),
				Slug:       slug,
				Kind:       kind,
				Switch:     &switchTo,
				DryRun:     application.options.dryRun,
			})
			if err != nil {
				return err
			}
			return application.report(command, port.Report{
				Operation: "workflow.release.stabilize",
				Summary: application.withInteractiveFetchSummary(
					"Release stabilization branch created.",
					repository.Remote,
					fetchCompleted(result.DryRun, result.Plan),
				),
				Fields: map[string]string{
					"release": release.String(),
					"branch":  result.Name.String(),
					"base":    result.Base.String(),
					"kind":    string(kind),
					"dryRun":  boolString(result.DryRun),
				},
			})
		}),
	}
	command.Flags().StringVar(&releaseRaw, "release", "", "release/<semver> line")
	command.Flags().StringVar(&kindRaw, "kind", "", "blocker, docs, or release-prep")
	command.Flags().StringVar(&keyRaw, "key", "", "ticket key")
	command.Flags().StringVar(&numberRaw, "ticket", "", "ticket number")
	command.Flags().StringVar(&slugRaw, "slug", "", "kebab-case stabilization description")
	command.Flags().BoolVar(&switchTo, "switch", true, "switch to the stabilization branch after creating it")
	return command
}

func newReleasePublishStabilizationCommand(application *application) *cobra.Command {
	var (
		branchRaw         string
		releaseRaw        string
		push              bool
		createPullRequest bool
		draft             bool
		resume            bool
	)
	command := &cobra.Command{
		Use:   "publish-stabilization",
		Short: "Validate and prepare a stabilization pull request for its release line",
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			name, err := currentOrSpecified(command.Context(), services, branchRaw, repository)
			if err != nil {
				return err
			}
			inputs.add("stabilization branch", name.String())
			switch name.Family() {
			case branch.FamilyFix, branch.FamilyDocs, branch.FamilyChore:
			default:
				return invalidOption("branch", name.String(), "a release stabilization fix, docs, or chore branch")
			}
			release, err := application.resolveReleaseLine(
				command.Context(),
				releaseRaw,
				"Release line",
				"Enter the release/<semantic-version> line from which the stabilization branch was created. Example: release/2.8.0.",
			)
			if err != nil {
				return err
			}
			inputs.add("release line", release.String())
			base, err := branch.NewTargetBase(repository.Remote, release)
			if err != nil {
				return err
			}
			if resume && application.options.dryRun {
				return invalidOption("resume", "true", "a non-dry-run invocation")
			}
			if err := application.validatePullRequestPublication(services, push, createPullRequest); err != nil {
				return err
			}
			label := "Publish release stabilization"
			description := "Validate the stabilization branch and prepare its pull request for " + release.String() + "?"
			if resume {
				label = "Resume release stabilization publication"
				description = "Continue the resolved stabilization rebase, revalidate the branch, and optionally push it?"
			}
			if err := application.confirmMutation(
				command.Context(),
				label,
				description,
			); err != nil {
				return err
			}
			var result workflow.PublishTicketResult
			if resume {
				result, err = services.tickets.ResumeTicketPublish(command.Context(), workflow.ResumeTicketPublishRequest{
					Repository:      repository,
					Branch:          name,
					Base:            &base,
					WorkflowManaged: true,
					Draft:           draft,
				})
			} else {
				result, err = services.tickets.PublishTicket(command.Context(), workflow.PublishTicketRequest{
					Repository:      repository,
					Branch:          name,
					Base:            &base,
					WorkflowManaged: true,
					Draft:           draft,
					DryRun:          application.options.dryRun,
				})
			}
			if err != nil {
				return err
			}
			if err := application.completePreparedPublication(
				command.Context(),
				services,
				repository,
				&result,
				push,
				createPullRequest,
				true,
			); err != nil {
				return err
			}
			fields := map[string]string{
				"branch":               result.Branch.String(),
				"release":              release.String(),
				"pushed":               boolString(result.Pushed),
				"pullRequestSource":    result.PullRequest.Source.String(),
				"pullRequestTarget":    result.PullRequest.Target.String(),
				"publishedPullRequest": result.PublishedURL,
			}
			addQualityFields(fields, result)
			return application.report(command, port.Report{
				Operation: "workflow.release.publish-stabilization",
				Summary: application.withInteractiveFetchSummary(
					"Release stabilization pull request prepared.",
					repository.Remote,
					!result.DryRun,
				),
				Fields: fields,
				Data:   result.PullRequest,
			})
		}),
	}
	command.Flags().StringVar(&branchRaw, "branch", "", "stabilization branch; defaults to the current branch")
	command.Flags().StringVar(&releaseRaw, "release", "", "release/<semver> target line")
	command.Flags().BoolVar(&push, "push", false, "push the stabilization branch after validation")
	command.Flags().BoolVar(&createPullRequest, "create-pull-request", false, "create the pull request through the configured provider after pushing")
	command.Flags().BoolVar(&draft, "draft", false, "mark the pull request intent as a draft")
	command.Flags().BoolVar(&resume, "resume", false, "continue a manually resolved rebase")
	return command
}

func newReleaseAlignPromotionBaseCommand(application *application) *cobra.Command {
	var (
		branchRaw         string
		releaseRaw        string
		push              bool
		createPullRequest bool
		draft             bool
		resume            bool
	)
	command := &cobra.Command{
		Use:   "align-promotion-base",
		Short: "Align a release-preparation branch with main before promotion",
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			name, err := currentOrSpecified(command.Context(), services, branchRaw, repository)
			if err != nil {
				return err
			}
			if name.Family() != branch.FamilyChore {
				return invalidOption("branch", name.String(), "a chore release-preparation branch")
			}
			inputs.add("release-preparation branch", name.String())
			release, err := application.resolveReleaseLine(
				command.Context(),
				releaseRaw,
				"Release line",
				"Enter the frozen release/<semantic-version> line whose main promotion must be aligned. Example: release/2.8.0.",
			)
			if err != nil {
				return err
			}
			inputs.add("release line", release.String())
			if err := application.validatePullRequestPublication(services, push, createPullRequest); err != nil {
				return err
			}
			if err := application.confirmMutation(
				command.Context(),
				"Align release promotion base",
				"Merge origin/main into "+name.String()+" and optionally publish its stabilization pull request to "+release.String()+"?",
			); err != nil {
				return err
			}
			result, err := services.releases.AlignReleasePromotionBase(command.Context(), workflow.AlignReleasePromotionBaseRequest{
				Repository:        repository,
				Release:           release,
				Branch:            name,
				Push:              push,
				CreatePullRequest: createPullRequest,
				Draft:             draft,
				Resume:            resume,
				DryRun:            application.options.dryRun,
			})
			if err != nil {
				return err
			}
			fields := map[string]string{
				"branch":               result.Branch.String(),
				"release":              result.Release.String(),
				"main":                 result.Main.String(),
				"missingMainCommits":   boolString(result.MissingMainCommits),
				"merged":               boolString(result.Merged),
				"resumed":              boolString(result.Resumed),
				"pushed":               boolString(result.Pushed),
				"publishedPullRequest": result.PublishedURL,
				"dryRun":               boolString(result.DryRun),
			}
			if result.Quality != nil {
				fields["qualityStatus"] = string(result.Quality.Status)
				fields["qualityDetail"] = result.Quality.Detail
			}
			return application.report(command, port.Report{
				Operation: "workflow.release.align-promotion-base",
				Summary:   "Release promotion base alignment completed.",
				Fields:    fields,
				Data:      result.PullRequest,
			})
		}),
	}
	command.Flags().StringVar(&branchRaw, "branch", "", "release-preparation branch; defaults to the current branch")
	command.Flags().StringVar(&releaseRaw, "release", "", "release/<semver> target line")
	command.Flags().BoolVar(&push, "push", false, "push the aligned release-preparation branch")
	command.Flags().BoolVar(&createPullRequest, "create-pull-request", false, "create the release stabilization pull request after pushing")
	command.Flags().BoolVar(&draft, "draft", false, "mark the pull request intent as a draft")
	command.Flags().BoolVar(&resume, "resume", false, "continue a manually resolved promotion-base merge")
	return command
}

func newReleasePromotionCommand(application *application) *cobra.Command {
	var (
		releaseRaw        string
		createPullRequest bool
		draft             bool
	)
	command := &cobra.Command{
		Use:   "promote",
		Short: "Prepare the release/<semver> to main pull request",
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			release, err := application.resolveReleaseLine(
				command.Context(),
				releaseRaw,
				"Release branch",
				"Enter the approved release/<semantic-version> branch to promote to main. Example: release/2.8.0.",
			)
			if err != nil {
				return err
			}
			inputs.add("release branch", release.String())
			result, err := services.releases.PrepareReleasePromotion(command.Context(), workflow.PrepareReleasePromotionRequest{
				Repository: repository,
				Release:    release,
				Draft:      draft,
				DryRun:     application.options.dryRun,
			})
			if err != nil {
				return err
			}
			create, err := application.resolvePullRequestPublication(
				command.Context(),
				services,
				result.PullRequest,
				createPullRequest,
			)
			if err != nil {
				return err
			}
			if create {
				if err := services.tickets.PreflightPullRequest(command.Context(), repository, result.PullRequest); err != nil {
					return err
				}
				publishedURL, err := services.tickets.PublishPullRequest(command.Context(), repository, result.PullRequest)
				if err != nil {
					return err
				}
				result.PublishedURL = publishedURL
			}
			return application.report(command, port.Report{
				Operation: "workflow.release.promote",
				Summary:   "Release promotion pull request prepared.",
				Fields: map[string]string{
					"source": result.PullRequest.Source.String(),
					"target": result.PullRequest.Target.String(),
					"title":  result.PullRequest.Title,
					"url":    result.PublishedURL,
				},
				Data: result.PullRequest,
			})
		}),
	}
	command.Flags().StringVar(&releaseRaw, "release", "", "release/<semver> branch")
	command.Flags().BoolVar(&createPullRequest, "create-pull-request", false, "create the pull request through the configured provider")
	command.Flags().BoolVar(&draft, "draft", false, "mark the pull request intent as a draft")
	return command
}

func newReleaseBackmergeCommand(application *application) *cobra.Command {
	var releaseRaw string
	var createPullRequest bool
	var draft bool
	command := &cobra.Command{
		Use:   "backmerge",
		Short: "Verify release delivery and conditionally prepare a develop backmerge",
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			release, err := application.resolveReleaseLine(
				command.Context(),
				releaseRaw,
				"Release branch",
				"Enter the delivered release/<semantic-version> branch to reconcile with develop. Example: release/2.8.0.",
			)
			if err != nil {
				return err
			}
			inputs.add("release branch", release.String())
			result, err := services.releases.AssessReleaseBackmerge(command.Context(), workflow.AssessReleaseBackmergeRequest{
				Repository: repository,
				Release:    release,
				Draft:      draft,
				DryRun:     application.options.dryRun,
			})
			if err != nil {
				return err
			}
			fields := map[string]string{
				"status":                  string(result.Status),
				"promotionPullRequestURL": result.Evidence.PromotionPullRequestURL,
				"promotionMergeCommit":    result.Evidence.PromotionMergeCommit,
				"tag":                     result.Evidence.Tag,
				"releaseURL":              result.Evidence.ReleaseURL,
				"effectiveDelta":          boolString(result.Evidence.EffectiveDelta),
			}
			if result.PullRequest == nil {
				return application.report(command, port.Report{
					Operation: "workflow.release.backmerge",
					Summary:   "Release reconciliation completed; no backmerge pull request is required.",
					Fields:    fields,
					Data:      result,
				})
			}
			create, err := application.resolvePullRequestPublication(
				command.Context(),
				services,
				*result.PullRequest,
				createPullRequest,
			)
			if err != nil {
				return err
			}
			if create {
				if err := services.tickets.PreflightPullRequest(command.Context(), repository, *result.PullRequest); err != nil {
					return err
				}
				publishedURL, err := services.tickets.PublishPullRequest(command.Context(), repository, *result.PullRequest)
				if err != nil {
					return err
				}
				fields["url"] = publishedURL
			}
			fields["source"] = result.PullRequest.Source.String()
			fields["target"] = result.PullRequest.Target.String()
			fields["title"] = result.PullRequest.Title
			summary := "Release reconciliation requires a backmerge pull request."
			if result.Status == workflow.ReleaseBackmergePlanned {
				summary = "Release reconciliation plan prepared."
			}
			return application.report(command, port.Report{
				Operation: "workflow.release.backmerge",
				Summary:   summary,
				Fields:    fields,
				Data:      result,
			})
		}),
	}
	command.Flags().StringVar(&releaseRaw, "release", "", "delivered release/<semver> branch")
	command.Flags().BoolVar(&createPullRequest, "create-pull-request", false, "create the pull request through the configured provider")
	command.Flags().BoolVar(&draft, "draft", false, "mark the pull request intent as a draft")
	return command
}

func newReleaseAlignReconciliationBaseCommand(application *application) *cobra.Command {
	var (
		branchRaw         string
		releaseRaw        string
		push              bool
		createPullRequest bool
		draft             bool
		resume            bool
		prepared          bool
	)
	command := &cobra.Command{
		Use:   "align-reconciliation-base",
		Short: "Align a release-preparation branch with develop before reconciliation",
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			name, err := currentOrSpecified(command.Context(), services, branchRaw, repository)
			if err != nil {
				return err
			}
			inputs.add("reconciliation-preparation branch", name.String())
			if name.Family() != branch.FamilyChore {
				return invalidOption("branch", name.String(), "a chore release-preparation branch")
			}
			release, err := application.resolveReleaseLine(
				command.Context(),
				releaseRaw,
				"Release line",
				"Enter the delivered release/<semantic-version> line to reconcile with develop. Example: release/2.8.0.",
			)
			if err != nil {
				return err
			}
			inputs.add("release line", release.String())
			if err := application.validatePullRequestPublication(services, push, createPullRequest); err != nil {
				return err
			}
			if err := application.confirmMutation(
				command.Context(),
				"Align release reconciliation base",
				"Merge current develop into "+name.String()+" without mutating "+release.String()+"?",
			); err != nil {
				return err
			}
			result, err := services.releases.AlignReleaseReconciliationBase(
				command.Context(),
				workflow.AlignReleaseReconciliationBaseRequest{
					Repository:        repository,
					Release:           release,
					Branch:            name,
					Push:              push,
					CreatePullRequest: createPullRequest,
					Draft:             draft,
					Resume:            resume,
					Prepared:          prepared,
					DryRun:            application.options.dryRun,
				},
			)
			if err != nil {
				return err
			}
			fields := map[string]string{
				"branch":                result.Branch.String(),
				"release":               result.Release.String(),
				"develop":               result.Develop.String(),
				"missingDevelopCommits": boolString(result.MissingDevelopCommits),
				"merged":                boolString(result.Merged),
				"resumed":               boolString(result.Resumed),
				"prepared":              boolString(result.Prepared),
				"pushed":                boolString(result.Pushed),
				"pullRequestSource":     result.PullRequest.Source.String(),
				"pullRequestTarget":     result.PullRequest.Target.String(),
				"publishedPullRequest":  result.PublishedURL,
				"dryRun":                boolString(result.DryRun),
			}
			if result.Quality != nil {
				fields["qualityStatus"] = string(result.Quality.Status)
				fields["qualityDetail"] = result.Quality.Detail
			}
			return application.report(command, port.Report{
				Operation: "workflow.release.align-reconciliation-base",
				Summary:   "Release reconciliation base alignment completed.",
				Fields:    fields,
				Data:      result.PullRequest,
			})
		}),
	}
	command.Flags().StringVar(&branchRaw, "branch", "", "reconciliation-preparation branch; defaults to the current branch")
	command.Flags().StringVar(&releaseRaw, "release", "", "delivered release/<semver> line")
	command.Flags().BoolVar(&push, "push", false, "push the preparation branch after validation")
	command.Flags().BoolVar(&createPullRequest, "create-pull-request", false, "create the pull request through the configured provider after pushing")
	command.Flags().BoolVar(&draft, "draft", false, "mark the pull request intent as a draft")
	command.Flags().BoolVar(&resume, "resume", false, "continue a manually resolved reconciliation merge")
	command.Flags().BoolVar(&prepared, "prepared", false, "validate and publish a resolved reconciliation preparation branch")
	return command
}

func newSupportPrepareCommand(application *application) *cobra.Command {
	var (
		versionRaw string
		dispatch   bool
	)
	command := &cobra.Command{
		Use:   "support",
		Short: "Prepare a protected CI request for support/<major.minor> from main",
		RunE: withWorkflowInputs(func(command *cobra.Command, inputs *workflowInputSummary) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			version, err := application.resolveSupportVersion(command.Context(), versionRaw)
			if err != nil {
				return err
			}
			inputs.add("support version", version.String())
			if dispatch && !application.options.dryRun {
				return protectedLineDirectDispatchDisabled()
			}
			if err := application.confirmMutation(command.Context(), "Create support line", "Create support/"+version.String()+" from origin/main?"); err != nil {
				return err
			}
			result, err := services.releases.PrepareSupport(command.Context(), workflow.PrepareSupportRequest{
				Repository: repository,
				Version:    version,
				DryRun:     application.options.dryRun,
			})
			if err != nil {
				return err
			}
			return application.report(command, port.Report{
				Operation: "workflow.release.support",
				Summary: application.withInteractiveFetchSummary(
					"Protected support-line creation intent prepared.",
					repository.Remote,
					fetchCompleted(result.DryRun, result.Plan),
				),
				Fields: map[string]string{
					"branch":            result.Intent.Branch.String(),
					"base":              result.Intent.Source.String(),
					"workflow":          result.Intent.Workflow,
					"dispatchRequested": boolString(dispatch),
					"dryRun":            boolString(result.DryRun),
				},
				Data: result.Intent,
			})
		}),
	}
	command.Flags().StringVar(&versionRaw, "version", "", "support major.minor version")
	command.Flags().BoolVar(&dispatch, "dispatch", false, "request direct executor dispatch (rejected outside dry-run)")
	return command
}
