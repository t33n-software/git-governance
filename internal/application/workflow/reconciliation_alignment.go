package workflow

import (
	"context"
	"errors"

	branchapp "github.com/CyberT33N/git-governance/internal/application/branch"
	"github.com/CyberT33N/git-governance/internal/application/port"
	"github.com/CyberT33N/git-governance/internal/domain/branch"
	"github.com/CyberT33N/git-governance/internal/domain/commitmsg"
	"github.com/CyberT33N/git-governance/internal/domain/problem"
)

// AlignReleaseReconciliationBaseRequest describes a controlled merge of the
// current integration line into a release-derived reconciliation branch.
type AlignReleaseReconciliationBaseRequest struct {
	Repository        port.RepositoryIdentity
	Release           branch.BranchName
	Branch            branch.BranchName
	Push              bool
	CreatePullRequest bool
	Draft             bool
	Resume            bool
	Prepared          bool
	DryRun            bool
}

// AlignReleaseReconciliationBaseResult records a preparation branch that is
// current with develop without mutating the delivered release line.
type AlignReleaseReconciliationBaseResult struct {
	Branch                branch.BranchName
	Release               branch.BranchName
	Develop               branch.BranchName
	PullRequest           port.PullRequest
	Evidence              port.ReleaseReconciliationEvidence
	MissingDevelopCommits bool
	Merged                bool
	Resumed               bool
	Prepared              bool
	Pushed                bool
	PublishedURL          string
	Quality               *port.QualityResult
	DryRun                bool
}

// AlignReleaseReconciliationBase merges develop into a ticket-bound,
// release-derived working branch. It preserves the delivered release ref and
// prepares a reviewed merge-commit pull request to develop.
func (service *ReleaseService) AlignReleaseReconciliationBase(
	ctx context.Context,
	request AlignReleaseReconciliationBaseRequest,
) (AlignReleaseReconciliationBaseResult, error) {
	if service.branches == nil || service.git == nil {
		return AlignReleaseReconciliationBaseResult{}, internalDependencyError("reconciliation-base alignment services")
	}
	if request.Release.Family() != branch.FamilyRelease {
		return AlignReleaseReconciliationBaseResult{}, invalidWorkflowInput(
			"reconciliation-base alignment requires a release/<semver> line",
			"provide the delivered release line that must be reconciled with develop",
		)
	}
	if request.Branch.Family() != branch.FamilyChore {
		return AlignReleaseReconciliationBaseResult{}, invalidWorkflowInput(
			"reconciliation-base alignment requires a chore preparation branch",
			"create a release-preparation branch before aligning the develop base",
		)
	}
	if request.CreatePullRequest && !request.Push {
		return AlignReleaseReconciliationBaseResult{}, invalidWorkflowInput(
			"pull-request creation requires an explicit branch push",
			"set Push before requesting provider pull-request creation",
		)
	}
	if request.Resume && request.Prepared {
		return AlignReleaseReconciliationBaseResult{}, invalidWorkflowInput(
			"reconciliation-base alignment cannot resume and validate a prepared branch simultaneously",
			"continue the active merge first or submit an already committed prepared branch",
		)
	}
	repository, err := normalizeWorkflowRepository(request.Repository)
	if err != nil {
		return AlignReleaseReconciliationBaseResult{}, err
	}
	if _, err := service.branches.Validate(ctx, branchapp.ValidateRequest{
		Repository: repository,
		Name:       request.Branch,
	}); err != nil {
		return AlignReleaseReconciliationBaseResult{}, err
	}
	current, err := service.git.CurrentBranch(ctx, repository)
	if err != nil {
		return AlignReleaseReconciliationBaseResult{}, err
	}
	if current.String() != request.Branch.String() {
		return AlignReleaseReconciliationBaseResult{}, invalidWorkflowInput(
			"reconciliation-base alignment may mutate only the checked-out preparation branch",
			"switch to the requested reconciliation-preparation branch before retrying",
		)
	}
	releaseBase, err := branch.NewTargetBase(repository.Remote, request.Release)
	if err != nil {
		return AlignReleaseReconciliationBaseResult{}, err
	}
	storedBase, found, err := service.git.WorkflowBase(ctx, repository, request.Branch)
	if err != nil {
		return AlignReleaseReconciliationBaseResult{}, err
	}
	if found && storedBase.String() != releaseBase.String() {
		return AlignReleaseReconciliationBaseResult{}, invalidWorkflowInput(
			"reconciliation-base alignment requires a branch created from the requested release line",
			"create the preparation branch with workflow release stabilize and use its recorded release base",
		)
	}
	if !found && !request.Prepared {
		return AlignReleaseReconciliationBaseResult{}, invalidWorkflowInput(
			"reconciliation-base alignment requires a branch created from the requested release line",
			"create the preparation branch with workflow release stabilize and use its recorded release base",
		)
	}
	develop := mustDevelop()
	developBase, _ := branch.NewTargetBase(repository.Remote, develop)
	result := AlignReleaseReconciliationBaseResult{
		Branch:      request.Branch,
		Release:     request.Release,
		Develop:     develop,
		PullRequest: reconciliationBaseAlignmentPullRequest(request.Branch, develop, request.Draft),
		Prepared:    request.Prepared,
		DryRun:      request.DryRun,
	}
	if request.DryRun {
		exists, err := service.git.TargetBaseExists(ctx, repository, developBase)
		if err != nil {
			return AlignReleaseReconciliationBaseResult{}, err
		}
		if !exists {
			return AlignReleaseReconciliationBaseResult{}, invalidWorkflowInput(
				"the current develop target base must exist before reconciliation-base alignment",
				"fetch the selected remote and verify develop before retrying",
			)
		}
		missing, err := service.git.HasMissingBaseCommits(ctx, repository, developBase)
		if err != nil {
			return AlignReleaseReconciliationBaseResult{}, err
		}
		result.MissingDevelopCommits = missing
		return result, nil
	}
	if request.CreatePullRequest {
		if service.tickets == nil {
			return AlignReleaseReconciliationBaseResult{}, internalDependencyError("ticket publication service")
		}
		if err := service.tickets.PreflightPullRequest(ctx, repository, result.PullRequest); err != nil {
			return AlignReleaseReconciliationBaseResult{}, err
		}
	}
	if request.Resume {
		operation, active, err := service.git.ActiveOperation(ctx, repository)
		if err != nil {
			return AlignReleaseReconciliationBaseResult{}, err
		}
		if !active || operation != "merge" {
			return AlignReleaseReconciliationBaseResult{}, invalidWorkflowInput(
				"reconciliation merge resume requires an active merge operation",
				"resolve and stage the existing reconciliation merge before retrying with --resume",
			)
		}
		conflicts, err := service.git.HasUnmergedConflicts(ctx, repository)
		if err != nil {
			return AlignReleaseReconciliationBaseResult{}, err
		}
		if conflicts {
			return AlignReleaseReconciliationBaseResult{}, invalidWorkflowInput(
				"reconciliation merge resume requires every conflict to be resolved and staged",
				"resolve exact conflicted paths, stage them explicitly, then retry with --resume",
			)
		}
		continuator, ok := service.git.(port.MergeContinuator)
		if !ok {
			return AlignReleaseReconciliationBaseResult{}, internalDependencyError("reconciliation merge continuator")
		}
		if err := continuator.ContinueMerge(ctx, repository); err != nil {
			return AlignReleaseReconciliationBaseResult{}, err
		}
		result.Resumed = true
		result.Merged = true
	}
	clean, err := service.git.IsWorktreeClean(ctx, repository)
	if err != nil {
		return AlignReleaseReconciliationBaseResult{}, err
	}
	if !clean {
		return AlignReleaseReconciliationBaseResult{}, invalidWorkflowInput(
			"reconciliation-base alignment requires a clean working tree",
			"commit, explicitly stash, or otherwise safely handle local changes before retrying",
		)
	}
	if err := service.git.Fetch(ctx, repository); err != nil {
		return AlignReleaseReconciliationBaseResult{}, err
	}
	exists, err := service.git.TargetBaseExists(ctx, repository, developBase)
	if err != nil {
		return AlignReleaseReconciliationBaseResult{}, err
	}
	if !exists {
		return AlignReleaseReconciliationBaseResult{}, invalidWorkflowInput(
			"the current develop target base must exist before reconciliation-base alignment",
			"fetch the selected remote and verify develop before retrying",
		)
	}
	missing, err := service.git.HasMissingBaseCommits(ctx, repository, developBase)
	if err != nil {
		return AlignReleaseReconciliationBaseResult{}, err
	}
	result.MissingDevelopCommits = missing
	assessment, err := service.AssessReleaseBackmerge(ctx, AssessReleaseBackmergeRequest{
		Repository: repository,
		Release:    request.Release,
	})
	if err != nil {
		return AlignReleaseReconciliationBaseResult{}, err
	}
	if assessment.Status != ReleaseBackmergeRequired {
		return AlignReleaseReconciliationBaseResult{}, invalidWorkflowInput(
			"reconciliation-base alignment requires an effective release-only delta",
			"record the verified not-required reconciliation result instead of preparing a backmerge branch",
		)
	}
	result.Evidence = assessment.Evidence
	if request.Prepared {
		if missing {
			return AlignReleaseReconciliationBaseResult{}, invalidWorkflowInput(
				"prepared reconciliation branch does not contain the current develop base",
				"resolve the manifest-pinned develop ref again before requesting privileged publication",
			)
		}
		inspector, ok := service.git.(port.ReconciliationMergeInspector)
		if !ok {
			return AlignReleaseReconciliationBaseResult{}, internalDependencyError("reconciliation merge inspector")
		}
		matches, err := inspector.HeadIsMergeOf(ctx, repository, releaseBase, developBase)
		if err != nil {
			return AlignReleaseReconciliationBaseResult{}, err
		}
		if !matches {
			return AlignReleaseReconciliationBaseResult{}, invalidWorkflowInput(
				"prepared reconciliation branch lacks the exact release-to-develop merge provenance",
				"submit only a ticket-bound branch whose HEAD merges the immutable release ref with the pinned develop ref",
			)
		}
		result.Merged = true
	}
	if request.Resume && missing {
		return AlignReleaseReconciliationBaseResult{}, invalidWorkflowInput(
			"develop advanced after the reconciliation merge was resolved",
			"start a new reconciliation alignment against the current develop base",
		)
	}
	if !missing && !request.Push && !request.Prepared && !request.Resume {
		return result, nil
	}
	if service.quality == nil {
		return AlignReleaseReconciliationBaseResult{}, internalDependencyError("quality runner")
	}
	if missing && !request.Prepared && !request.Resume {
		message := reconciliationBaseAlignmentMergeMessage(request.Branch, request.Release)
		if err := service.git.Merge(ctx, repository, developBase, message); err != nil {
			conflicts, inspectErr := service.git.HasUnmergedConflicts(ctx, repository)
			if inspectErr != nil {
				return AlignReleaseReconciliationBaseResult{}, inspectErr
			}
			if conflicts {
				inspector, ok := service.git.(port.ReconciliationMergeInspector)
				if !ok {
					return AlignReleaseReconciliationBaseResult{}, internalDependencyError("reconciliation merge inspector")
				}
				releaseRevision, developRevision, resolveErr := inspector.ResolveReconciliationBases(ctx, repository, releaseBase, developBase)
				if resolveErr != nil {
					return AlignReleaseReconciliationBaseResult{}, resolveErr
				}
				return AlignReleaseReconciliationBaseResult{}, reconciliationMergeConflictProblem(
					request.Branch,
					request.Release,
					developBase,
					releaseRevision,
					developRevision,
					err,
				)
			}
			return AlignReleaseReconciliationBaseResult{}, err
		}
		result.Merged = true
	}
	if _, err := service.branches.Validate(ctx, branchapp.ValidateRequest{
		Repository: repository,
		Name:       request.Branch,
	}); err != nil {
		return AlignReleaseReconciliationBaseResult{}, err
	}
	quality, err := service.quality.Run(ctx, repository, port.QualityRequest{
		Families: []branch.Family{request.Branch.Family()},
	})
	if err != nil {
		return AlignReleaseReconciliationBaseResult{}, err
	}
	result.Quality = &quality
	if !request.Push {
		return result, nil
	}
	publication, err := service.git.PublicationState(ctx, repository, request.Branch)
	if err != nil {
		return AlignReleaseReconciliationBaseResult{}, err
	}
	if publication == branch.PublicationUnknown {
		return AlignReleaseReconciliationBaseResult{}, invalidWorkflowInput(
			"reconciliation-base alignment requires a known branch publication state",
			"fetch the remote and resolve the branch tracking state before retrying",
		)
	}
	if err := service.git.Push(ctx, repository, request.Branch, publication == branch.PublicationUnpublished); err != nil {
		return AlignReleaseReconciliationBaseResult{}, err
	}
	result.Pushed = true
	if !request.CreatePullRequest {
		return result, nil
	}
	publishedURL, err := service.tickets.PublishPullRequest(ctx, repository, result.PullRequest)
	if err != nil {
		return AlignReleaseReconciliationBaseResult{}, err
	}
	result.PublishedURL = publishedURL
	return result, nil
}

func reconciliationBaseAlignmentPullRequest(worker, develop branch.BranchName, draft bool) port.PullRequest {
	id, _ := worker.Ticket()
	slug, _ := worker.Slug()
	return port.PullRequest{
		Source: worker,
		Target: develop,
		Ticket: id,
		Title:  id.String() + ": " + slug.String(),
		Draft:  draft,
	}
}

func reconciliationBaseAlignmentMergeMessage(worker, release branch.BranchName) commitmsg.Message {
	id, _ := worker.Ticket()
	version, _ := release.ReleaseVersion()
	header, _ := commitmsg.NewHeader(
		commitmsg.TypeChore,
		id,
		"align release "+version.String()+" with develop for reconciliation",
		false,
	)
	message, _ := commitmsg.NewMessage(header, "", nil)
	return message
}

func reconciliationMergeConflictProblem(
	worker branch.BranchName,
	release branch.BranchName,
	develop branch.TargetBase,
	releaseRevision string,
	developRevision string,
	cause error,
) error {
	var causeProblem *problem.Problem
	context := ""
	diagnostic := ""
	if errors.As(cause, &causeProblem) {
		context = causeProblem.Context
		diagnostic = causeProblem.Diagnostic
	}
	return problem.Wrap(problem.Details{
		Code:        problem.CodeMergeConflict,
		Category:    problem.CategoryGit,
		Field:       "reconciliation conflict",
		Actual:      "merge " + develop.String() + "@" + developRevision + " into " + worker.String(),
		Context:     context,
		Diagnostic:  diagnostic,
		Expected:    "a conflict-free merge in the ticket-bound reconciliation preparation branch",
		Rule:        "reconciliation conflicts must remain fail-closed until a provenance-validated resolution is available",
		Remediation: "record the release/develop conflict context, resolve exact paths in the preparation branch, then continue with --resume or submit a validated prepared branch",
		WorkflowInputs: []problem.WorkflowInput{
			{Field: "reconciliation-preparation branch", Value: worker.String()},
			{Field: "release line", Value: release.String() + "@" + releaseRevision},
			{Field: "develop base", Value: develop.String() + "@" + developRevision},
		},
	}, cause)
}
