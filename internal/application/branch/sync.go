package branchapp

import (
	"context"

	commitapp "github.com/CyberT33N/git-governance/internal/application/commit"
	"github.com/CyberT33N/git-governance/internal/application/port"
	"github.com/CyberT33N/git-governance/internal/domain/branch"
	"github.com/CyberT33N/git-governance/internal/domain/commitmsg"
	"github.com/CyberT33N/git-governance/internal/domain/problem"
)

// SyncStrategy controls how a detected base delta is handled.
type SyncStrategy string

const (
	SyncCheck  SyncStrategy = "check"
	SyncAuto   SyncStrategy = "auto"
	SyncRebase SyncStrategy = "rebase"
	SyncMerge  SyncStrategy = "merge"
)

// Synchronizer centralizes base freshness and rewrite policy.
type Synchronizer struct {
	git          port.GitRepository
	validator    *Service
	quality      port.QualityRunner
	finalQuality *FinalQualityGate
}

// NewSynchronizer creates a synchronization service.
func NewSynchronizer(git port.GitRepository, validator *Service, quality port.QualityRunner) *Synchronizer {
	return &Synchronizer{
		git:       git,
		validator: validator,
		quality:   quality,
	}
}

// WithFinalQualityGate adds revision-bound final quality evidence to pre-push
// validation without changing direct synchronization quality behavior.
func (synchronizer *Synchronizer) WithFinalQualityGate(gate *FinalQualityGate) *Synchronizer {
	synchronizer.finalQuality = gate
	return synchronizer
}

// SyncRequest describes a base-synchronization request.
type SyncRequest struct {
	Repository               port.RepositoryIdentity
	Name                     branch.BranchName
	Base                     *branch.TargetBase
	Strategy                 SyncStrategy
	MergeMessage             *commitmsg.Message
	DryRun                   bool
	SkipFetch                bool
	WorkflowManaged          bool
	DeferPostMutationQuality bool
}

// SyncResult describes the observed state and any mutation performed.
type SyncResult struct {
	Name               branch.BranchName
	Base               branch.TargetBase
	Publication        branch.PublicationState
	MissingBaseCommits bool
	Mutated            bool
	RecommendedAction  string
	Quality            *port.QualityResult
}

// Sync applies the requested policy-safe synchronization strategy.
func (synchronizer *Synchronizer) Sync(ctx context.Context, request SyncRequest) (SyncResult, error) {
	repository, err := normalizeRepository(request.Repository)
	if err != nil {
		return SyncResult{}, err
	}
	if err := contextError(ctx); err != nil {
		return SyncResult{}, err
	}
	if synchronizer.validator == nil {
		return SyncResult{}, internalDependencyError("branch validator")
	}
	if synchronizer.git == nil {
		return SyncResult{}, internalDependencyError("Git repository")
	}
	if _, err := synchronizer.validator.Validate(ctx, ValidateRequest{Repository: repository, Name: request.Name}); err != nil {
		return SyncResult{}, err
	}
	if !request.Name.Family().IsOfficialWorkingBranch() {
		return SyncResult{}, unsupportedSyncFamily(request.Name)
	}

	baseInput, err := synchronizer.workflowBase(ctx, repository, request.Name, request.Base)
	if err != nil {
		return SyncResult{}, err
	}
	base, err := resolveSyncBase(request.Name, repository, baseInput, request.WorkflowManaged)
	if err != nil {
		return SyncResult{}, err
	}
	strategy := request.Strategy
	if strategy == "" {
		strategy = SyncCheck
	}
	switch strategy {
	case SyncCheck, SyncAuto, SyncRebase, SyncMerge:
	default:
		return SyncResult{}, invalidSyncStrategy(strategy)
	}

	clean, err := synchronizer.git.IsWorktreeClean(ctx, repository)
	if err != nil {
		return SyncResult{}, err
	}
	if !clean {
		return SyncResult{}, worktreeNotCleanForSync()
	}
	if !request.SkipFetch && !request.DryRun {
		if err := synchronizer.git.Fetch(ctx, repository); err != nil {
			return SyncResult{}, err
		}
	}

	publication, err := synchronizer.git.PublicationState(ctx, repository, request.Name)
	if err != nil {
		return SyncResult{}, err
	}
	if publication == branch.PublicationUnknown {
		return SyncResult{}, problem.New(problem.Details{
			Code:        problem.CodeBranchPublicationUnknown,
			Category:    problem.CategoryRepository,
			Field:       "branch publication state",
			Actual:      request.Name.String(),
			Expected:    "a known published or unpublished state",
			Rule:        "history rewrites are forbidden when publication state cannot be determined",
			Remediation: "fetch the remote successfully and resolve the branch tracking state",
		})
	}

	missing, err := synchronizer.git.HasMissingBaseCommits(ctx, repository, base)
	if err != nil {
		return SyncResult{}, err
	}
	result := SyncResult{
		Name:               request.Name,
		Base:               base,
		Publication:        publication,
		MissingBaseCommits: missing,
	}
	if !missing {
		if strategy == SyncRebase {
			return SyncResult{}, problem.New(problem.Details{
				Code:        problem.CodeRebaseNotRequired,
				Category:    problem.CategoryGovernance,
				Field:       "target base",
				Actual:      base.String(),
				Expected:    "a target base containing commits missing from the current branch",
				Rule:        "a rebase is performed only when the target base has advanced",
				Remediation: "continue without a rebase; the current branch already contains the target base",
			})
		}
		result.RecommendedAction = "none"
		return result, nil
	}

	if strategy == SyncCheck {
		result.RecommendedAction = recommendedAction(publication)
		return result, nil
	}

	if strategy == SyncAuto {
		if publication == branch.PublicationPublished {
			result.RecommendedAction = "merge"
			return result, nil
		}
		if request.DryRun {
			result.RecommendedAction = "rebase"
			return result, nil
		}
		if err := synchronizer.git.Rebase(ctx, repository, base); err != nil {
			return SyncResult{}, synchronizer.classifyRebaseFailure(ctx, repository, base, err)
		}
		result.Mutated = true
		result.RecommendedAction = "rebased"
		if !request.DeferPostMutationQuality {
			quality, err := synchronizer.validateAfterMutation(ctx, repository, request.Name)
			if err != nil {
				return SyncResult{}, err
			}
			result.Quality = &quality
		}
		return result, nil
	}

	if strategy == SyncRebase {
		if publication != branch.PublicationUnpublished {
			return SyncResult{}, rebaseAfterPublishForbidden(request.Name, base)
		}
		if request.DryRun {
			result.RecommendedAction = "rebase"
			return result, nil
		}
		if err := synchronizer.git.Rebase(ctx, repository, base); err != nil {
			return SyncResult{}, synchronizer.classifyRebaseFailure(ctx, repository, base, err)
		}
		result.Mutated = true
		result.RecommendedAction = "rebased"
		if !request.DeferPostMutationQuality {
			quality, err := synchronizer.validateAfterMutation(ctx, repository, request.Name)
			if err != nil {
				return SyncResult{}, err
			}
			result.Quality = &quality
		}
		return result, nil
	}

	// The strategy was validated above; reaching this point means SyncMerge.
	if publication != branch.PublicationPublished {
		return SyncResult{}, invalidMergeBeforePublish(request.Name)
	}
	if request.MergeMessage == nil {
		return SyncResult{}, problem.New(problem.Details{
			Code:        problem.CodeCommitHeaderInvalid,
			Category:    problem.CategoryGovernance,
			Field:       "merge message",
			Expected:    "a validated Conventional Commit message",
			Rule:        "published branch synchronization creates an explicit governed merge commit",
			Example:     "chore(ABC-123): merge origin/develop",
			Remediation: "supply a validated merge message matching the branch ticket",
		})
	}
	if err := commitapp.ValidateMessageForBranch(request.Name, *request.MergeMessage); err != nil {
		return SyncResult{}, err
	}
	if request.DryRun {
		result.RecommendedAction = "merge"
		return result, nil
	}
	if err := synchronizer.git.Merge(ctx, repository, base, *request.MergeMessage); err != nil {
		return SyncResult{}, err
	}
	result.Mutated = true
	result.RecommendedAction = "merged"
	if !request.DeferPostMutationQuality {
		quality, err := synchronizer.validateAfterMutation(ctx, repository, request.Name)
		if err != nil {
			return SyncResult{}, err
		}
		result.Quality = &quality
	}
	return result, nil
}

// ResumeRebaseRequest describes a previously interrupted policy-approved
// rebase. It deliberately contains no mutation strategy: the only permitted
// action is to continue the in-progress rebase after user conflict resolution.
type ResumeRebaseRequest struct {
	Repository               port.RepositoryIdentity
	Name                     branch.BranchName
	Base                     *branch.TargetBase
	WorkflowManaged          bool
	DeferPostMutationQuality bool
}

// ResumeRebase continues a user-resolved rebase or verifies an externally
// completed one, then reruns branch and quality validation before publication
// can continue.
func (synchronizer *Synchronizer) ResumeRebase(ctx context.Context, request ResumeRebaseRequest) (SyncResult, error) {
	repository, err := normalizeRepository(request.Repository)
	if err != nil {
		return SyncResult{}, err
	}
	if err := contextError(ctx); err != nil {
		return SyncResult{}, err
	}
	if synchronizer.validator == nil {
		return SyncResult{}, internalDependencyError("branch validator")
	}
	if synchronizer.git == nil {
		return SyncResult{}, internalDependencyError("Git repository")
	}
	if _, err := synchronizer.validator.Validate(ctx, ValidateRequest{Repository: repository, Name: request.Name}); err != nil {
		return SyncResult{}, err
	}
	if !request.Name.Family().IsOfficialWorkingBranch() {
		return SyncResult{}, unsupportedSyncFamily(request.Name)
	}

	baseInput, err := synchronizer.workflowBase(ctx, repository, request.Name, request.Base)
	if err != nil {
		return SyncResult{}, err
	}
	base, err := resolveSyncBase(request.Name, repository, baseInput, request.WorkflowManaged)
	if err != nil {
		return SyncResult{}, err
	}
	publication, err := synchronizer.git.PublicationState(ctx, repository, request.Name)
	if err != nil {
		return SyncResult{}, err
	}
	if publication != branch.PublicationUnpublished {
		return SyncResult{}, rebaseAfterPublishForbidden(request.Name, base)
	}

	operation, active, err := synchronizer.git.ActiveOperation(ctx, repository)
	if err != nil {
		return SyncResult{}, err
	}
	if active {
		if operation != "rebase" {
			return SyncResult{}, rebaseResumeUnavailable(operation)
		}
		if err := synchronizer.git.ContinueRebase(ctx, repository); err != nil {
			return SyncResult{}, synchronizer.classifyRebaseFailure(ctx, repository, base, err)
		}
	}

	missing, err := synchronizer.git.HasMissingBaseCommits(ctx, repository, base)
	if err != nil {
		return SyncResult{}, err
	}
	if missing {
		return SyncResult{}, rebaseConflict(base, nil)
	}
	var quality *port.QualityResult
	if !request.DeferPostMutationQuality {
		completed, err := synchronizer.validateAfterMutation(ctx, repository, request.Name)
		if err != nil {
			return SyncResult{}, err
		}
		quality = &completed
	}
	return SyncResult{
		Name:              request.Name,
		Base:              base,
		Publication:       publication,
		Mutated:           true,
		RecommendedAction: "rebased",
		Quality:           quality,
	}, nil
}

// PrePushRequest describes the local governance data checked before a push.
type PrePushRequest struct {
	Repository      port.RepositoryIdentity
	Name            branch.BranchName
	Base            *branch.TargetBase
	WorkflowManaged bool
}

// PrePushResult describes the freshness and publication state checked locally.
type PrePushResult struct {
	Name               branch.BranchName
	Base               *branch.TargetBase
	Publication        branch.PublicationState
	MissingBaseCommits bool
	Quality            port.QualityResult
}

// ValidatePrePush validates an outgoing branch but never rewrites, merges, or
// otherwise mutates local branch history.
func (synchronizer *Synchronizer) ValidatePrePush(ctx context.Context, request PrePushRequest) (PrePushResult, error) {
	repository, err := normalizeRepository(request.Repository)
	if err != nil {
		return PrePushResult{}, err
	}
	if synchronizer.validator == nil {
		return PrePushResult{}, internalDependencyError("branch validator")
	}
	if synchronizer.git == nil {
		return PrePushResult{}, internalDependencyError("Git repository")
	}
	if request.Name.Family().IsSharedLine() {
		return PrePushResult{}, problem.New(problem.Details{
			Code:        problem.CodeSharedLineMutationForbidden,
			Category:    problem.CategoryGovernance,
			Field:       "branch",
			Actual:      request.Name.String(),
			Expected:    "a pull request into a shared line",
			Rule:        "developers do not directly push main, develop, release, or support lines",
			Remediation: "push an official working branch and open a pull request",
		})
	}
	if request.Name.Family() != branch.FamilyScratch && !request.Name.Family().IsOfficialWorkingBranch() {
		return PrePushResult{}, unsupportedSyncFamily(request.Name)
	}
	if _, err := synchronizer.validator.Validate(ctx, ValidateRequest{Repository: repository, Name: request.Name}); err != nil {
		return PrePushResult{}, err
	}
	if request.Name.Family() == branch.FamilyScratch {
		quality, err := synchronizer.prePushQuality(ctx, repository, request.Name, nil)
		if err != nil {
			return PrePushResult{}, err
		}
		return PrePushResult{
			Name:        request.Name,
			Publication: branch.PublicationUnknown,
			Quality:     quality,
		}, nil
	}
	baseInput, err := synchronizer.workflowBase(ctx, repository, request.Name, request.Base)
	if err != nil {
		return PrePushResult{}, err
	}
	base, err := resolveSyncBase(request.Name, repository, baseInput, request.WorkflowManaged)
	if err != nil {
		return PrePushResult{}, err
	}
	if err := synchronizer.git.Fetch(ctx, repository); err != nil {
		return PrePushResult{}, err
	}
	publication, err := synchronizer.git.PublicationState(ctx, repository, request.Name)
	if err != nil {
		return PrePushResult{}, err
	}
	if publication == branch.PublicationUnknown {
		return PrePushResult{}, problem.New(problem.Details{
			Code:        problem.CodeBranchPublicationUnknown,
			Category:    problem.CategoryRepository,
			Field:       "branch publication state",
			Actual:      request.Name.String(),
			Expected:    "a known published or unpublished state",
			Rule:        "pre-push validation must not infer branch history safety from an unknown state",
			Remediation: "fetch the remote successfully and resolve the branch tracking state",
		})
	}
	missing, err := synchronizer.git.HasMissingBaseCommits(ctx, repository, base)
	if err != nil {
		return PrePushResult{}, err
	}
	result := PrePushResult{
		Name:               request.Name,
		Base:               &base,
		Publication:        publication,
		MissingBaseCommits: missing,
	}
	if publication == branch.PublicationUnpublished && missing {
		return PrePushResult{}, problem.New(problem.Details{
			Code:        problem.CodeBranchBaseInvalid,
			Category:    problem.CategoryGovernance,
			Field:       "target base",
			Actual:      base.String(),
			Expected:    "an unpublished branch based on the latest target base",
			Rule:        "before the first push, a branch with missing base commits must be rebased",
			Example:     "git rebase " + base.String(),
			Remediation: "run branch sync-base --strategy rebase, rerun quality checks, then push again",
		})
	}
	quality, err := synchronizer.prePushQuality(ctx, repository, request.Name, &base)
	if err != nil {
		return PrePushResult{}, err
	}
	result.Quality = quality
	return result, nil
}

// ValidatePush exposes pre-push validation as a small cross-use-case contract.
// It deliberately performs no history rewrite, merge, or push itself.
func (synchronizer *Synchronizer) ValidatePush(ctx context.Context, repository port.RepositoryIdentity, name branch.BranchName, base *branch.TargetBase) error {
	_, err := synchronizer.ValidatePrePush(ctx, PrePushRequest{
		Repository: repository,
		Name:       name,
		Base:       base,
	})
	return err
}

func (synchronizer *Synchronizer) runQuality(
	ctx context.Context,
	repository port.RepositoryIdentity,
	families ...branch.Family,
) (port.QualityResult, error) {
	if synchronizer.quality == nil {
		return port.QualityResult{
			Status: port.QualityUnconfigured,
			Detail: "no quality runner is configured",
		}, nil
	}
	return synchronizer.quality.Run(ctx, repository, port.QualityRequest{Families: families})
}

func (synchronizer *Synchronizer) prePushQuality(
	ctx context.Context,
	repository port.RepositoryIdentity,
	name branch.BranchName,
	base *branch.TargetBase,
) (port.QualityResult, error) {
	if synchronizer.finalQuality != nil {
		return synchronizer.finalQuality.ValidateOrRunForBranch(ctx, repository, name, base)
	}
	return synchronizer.runQuality(ctx, repository, name.Family())
}

func (synchronizer *Synchronizer) workflowBase(
	ctx context.Context,
	repository port.RepositoryIdentity,
	name branch.BranchName,
	explicit *branch.TargetBase,
) (*branch.TargetBase, error) {
	if explicit != nil {
		return explicit, nil
	}
	if !name.Family().MayUseWorkflowBase() {
		return nil, nil
	}
	stored, found, err := synchronizer.git.WorkflowBase(ctx, repository, name)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &stored, nil
}

func (synchronizer *Synchronizer) validateAfterMutation(
	ctx context.Context,
	repository port.RepositoryIdentity,
	name branch.BranchName,
) (port.QualityResult, error) {
	if _, err := synchronizer.validator.Validate(ctx, ValidateRequest{
		Repository: repository,
		Name:       name,
	}); err != nil {
		return port.QualityResult{}, err
	}
	return synchronizer.runQuality(ctx, repository, name.Family())
}

func (synchronizer *Synchronizer) classifyRebaseFailure(
	ctx context.Context,
	repository port.RepositoryIdentity,
	base branch.TargetBase,
	cause error,
) error {
	operation, active, err := synchronizer.git.ActiveOperation(ctx, repository)
	if err != nil || !active || operation != "rebase" {
		return cause
	}
	return rebaseConflict(base, cause)
}

func recommendedAction(publication branch.PublicationState) string {
	if publication == branch.PublicationUnpublished {
		return "rebase"
	}
	return "merge"
}

func unsupportedSyncFamily(name branch.BranchName) error {
	return problem.New(problem.Details{
		Code:        problem.CodeBranchFamilyInvalid,
		Category:    problem.CategoryGovernance,
		Field:       "branch family",
		Actual:      name.Family().String(),
		Expected:    "an official working branch",
		Rule:        "base synchronization is defined for official published or unpublished working branches",
		Example:     "feature/ABC-123-add-export-button",
		Remediation: "use the matching workflow for release, support, hotfix, or scratch work",
	})
}

func invalidSyncStrategy(strategy SyncStrategy) error {
	return problem.New(problem.Details{
		Code:        problem.CodeInvalidInput,
		Category:    problem.CategoryUsage,
		Field:       "sync strategy",
		Actual:      string(strategy),
		Expected:    "check, auto, rebase, or merge",
		Rule:        "synchronization uses an explicit strategy",
		Example:     "rebase",
		Remediation: "choose a supported synchronization strategy",
	})
}

func worktreeNotCleanForSync() error {
	return problem.New(problem.Details{
		Code:        problem.CodeWorktreeNotClean,
		Category:    problem.CategoryRepository,
		Field:       "worktree",
		Expected:    "a clean working tree before rebase or merge",
		Rule:        "history operations must not risk uncommitted changes",
		Example:     "git status --porcelain returns no entries",
		Remediation: "commit, stash, or discard local changes before synchronizing",
	})
}

func rebaseAfterPublishForbidden(name branch.BranchName, base branch.TargetBase) error {
	return problem.New(problem.Details{
		Code:        problem.CodeRebaseAfterPublishForbidden,
		Category:    problem.CategoryGovernance,
		Field:       "branch",
		Actual:      name.String(),
		Expected:    "an unpublished official branch for a rebase",
		Rule:        "published official branches are append-only and synchronize with an explicit merge",
		Example:     "chore(ABC-123): merge " + base.String(),
		Remediation: "use --strategy merge with a governed merge message",
	})
}

func rebaseConflict(base branch.TargetBase, cause error) error {
	return problem.Wrap(problem.Details{
		Code:        problem.CodeRebaseConflict,
		Category:    problem.CategoryGit,
		Field:       "rebase",
		Actual:      base.String(),
		Expected:    "a completed rebase without unresolved conflicts",
		Rule:        "the workflow pauses when Git requires the developer to resolve a rebase conflict",
		Example:     "resolve conflicts, stage the resolutions, then select Retry",
		Remediation: "resolve and stage every conflicting file, then select Retry to continue the existing rebase",
	}, cause)
}

func rebaseResumeUnavailable(operation string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeRebaseConflict,
		Category:    problem.CategoryGit,
		Field:       "Git operation state",
		Actual:      operation,
		Expected:    "an in-progress rebase",
		Rule:        "a rebase retry can continue only the paused rebase created by this workflow",
		Remediation: "complete or abort the active Git operation, then restart ticket publication",
	})
}

func invalidMergeBeforePublish(name branch.BranchName) error {
	return problem.New(problem.Details{
		Code:        problem.CodeBranchBaseInvalid,
		Category:    problem.CategoryGovernance,
		Field:       "branch",
		Actual:      name.String(),
		Expected:    "a published official branch for a merge synchronization",
		Rule:        "unpublished branches rebase only when their target base has advanced",
		Remediation: "use --strategy rebase before the first push",
	})
}

func internalDependencyError(name string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeInternal,
		Category:    problem.CategoryInternal,
		Field:       "dependency",
		Actual:      name,
		Expected:    "a configured application dependency",
		Rule:        "application services are composed with their required ports",
		Remediation: "fix the composition root",
	})
}
