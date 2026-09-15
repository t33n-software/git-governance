package branchapp

import (
	"context"
	"strings"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// SharedLineRefresh records the guarded fast-forward outcome of one local
// shared-line checkout against its fetched remote-tracking reference.
type SharedLineRefresh struct {
	Name    branch.BranchName
	Base    branch.TargetBase
	Outcome port.FastForwardOutcome
}

// RefreshSharedLinesRequest describes an on-demand refresh of local
// shared-line checkouts. An empty Lines selects every locally present shared
// line; an explicit selection is validated fail-closed against the shared-line
// set.
type RefreshSharedLinesRequest struct {
	Repository port.RepositoryIdentity
	Lines      []branch.BranchName
	DryRun     bool
}

// RefreshSharedLinesResult reports the fetch state, the plan, and the
// per-line outcomes of one refresh run.
type RefreshSharedLinesResult struct {
	Lines   []SharedLineRefresh
	Fetched bool
	DryRun  bool
	Plan    []PlanStep
}

// SharedLineRefresher owns the on-demand, fail-closed fast-forward refresh of
// local shared-line checkouts: never a merge commit, never a force, never a
// remote mutation, never a checkout switch, and never the creation of a
// missing line.
type SharedLineRefresher struct {
	git port.GitRepository
}

// NewSharedLineRefresher creates the shared-line refresh use case.
func NewSharedLineRefresher(git port.GitRepository) *SharedLineRefresher {
	return &SharedLineRefresher{git: git}
}

// Refresh fast-forwards every bound local shared-line checkout to its fetched
// remote-tracking reference. A dry run validates the target set against the
// last fetched state and reports the plan without any mutation. The real run
// fetches first, rejects any in-progress Git operation, requires a clean
// worktree when the checked-out branch is a target, and reports every per-line
// outcome; a diverged line is left untouched and fails the run closed.
func (refresher *SharedLineRefresher) Refresh(ctx context.Context, request RefreshSharedLinesRequest) (RefreshSharedLinesResult, error) {
	repository, err := normalizeRepository(request.Repository)
	if err != nil {
		return RefreshSharedLinesResult{}, err
	}
	if err := contextError(ctx); err != nil {
		return RefreshSharedLinesResult{}, err
	}
	if refresher.git == nil {
		return RefreshSharedLinesResult{}, internalDependencyError("Git repository")
	}

	targets, err := refresher.resolveTargets(ctx, repository, request.Lines)
	if err != nil {
		return RefreshSharedLinesResult{}, err
	}
	if len(targets) == 0 {
		// No local shared-line checkout exists: nothing to plan, fetch, or
		// refresh, and no guard may run against a nonexistent target.
		return RefreshSharedLinesResult{
			Lines:  make([]SharedLineRefresh, 0),
			DryRun: request.DryRun,
		}, nil
	}

	bases := make([]branch.TargetBase, 0, len(targets))
	plan := make([]PlanStep, 0, len(targets)+1)
	if !request.DryRun {
		plan = append(plan, PlanStep{Action: "fetch", Detail: "git fetch --prune " + repository.Remote})
	}
	for _, target := range targets {
		base, err := branch.NewTargetBase(repository.Remote, target)
		if err != nil {
			return RefreshSharedLinesResult{}, err
		}
		bases = append(bases, base)
		plan = append(plan, PlanStep{Action: "refresh", Detail: target.String() + " to " + base.String()})
	}
	result := RefreshSharedLinesResult{
		Lines:  make([]SharedLineRefresh, 0, len(targets)),
		DryRun: request.DryRun,
		Plan:   plan,
	}

	if request.DryRun {
		// The dry run validates the target set against the last fetched local
		// state and never fetches or mutates.
		if err := refresher.requireBases(ctx, repository, targets, bases); err != nil {
			return RefreshSharedLinesResult{}, err
		}
		return result, nil
	}

	if err := refresher.git.Fetch(ctx, repository); err != nil {
		return RefreshSharedLinesResult{}, err
	}
	result.Fetched = true

	// After the fetch, the remote-tracking references are authoritative.
	if err := refresher.requireBases(ctx, repository, targets, bases); err != nil {
		return RefreshSharedLinesResult{}, err
	}

	operation, active, err := refresher.git.ActiveOperation(ctx, repository)
	if err != nil {
		return RefreshSharedLinesResult{}, err
	}
	if active {
		return RefreshSharedLinesResult{}, operationInProgress(operation)
	}

	current, err := refresher.git.CurrentBranch(ctx, repository)
	if err != nil {
		return RefreshSharedLinesResult{}, err
	}
	if containsBranch(targets, current) {
		clean, err := refresher.git.IsWorktreeClean(ctx, repository)
		if err != nil {
			return RefreshSharedLinesResult{}, err
		}
		if !clean {
			return RefreshSharedLinesResult{}, worktreeNotCleanForRefresh(current)
		}
	}

	// The required capabilities bind all-or-nothing before any mutation: a
	// composition that lacks one of them fails closed instead of refreshing a
	// subset.
	needsReference := false
	for _, target := range targets {
		if target.String() != current.String() {
			needsReference = true
		}
	}
	checkedOutForwarder, checkedOutOK := refresher.git.(port.BranchFastForwarder)
	referenceForwarder, referenceOK := refresher.git.(port.BranchReferenceFastForwarder)
	if containsBranch(targets, current) && !checkedOutOK {
		return RefreshSharedLinesResult{}, internalDependencyError("checked-out branch fast-forwarder")
	}
	if needsReference && !referenceOK {
		return RefreshSharedLinesResult{}, internalDependencyError("branch reference fast-forwarder")
	}

	for index, target := range targets {
		var outcome port.FastForwardOutcome
		if target.String() == current.String() {
			outcome, err = checkedOutForwarder.FastForwardBranch(ctx, repository, target, bases[index])
		} else {
			outcome, err = referenceForwarder.FastForwardBranchReference(ctx, repository, target, bases[index])
		}
		if err != nil {
			return RefreshSharedLinesResult{}, err
		}
		result.Lines = append(result.Lines, SharedLineRefresh{Name: target, Base: bases[index], Outcome: outcome})
	}

	for _, line := range result.Lines {
		if line.Outcome == port.FastForwardDiverged {
			return result, sharedLineDiverged(line.Name, result.Lines)
		}
	}
	return result, nil
}

// resolveTargets binds the requested or the default target set: an explicit
// selection is validated fail-closed against the shared-line set and the
// locally present branches; an empty selection enumerates every locally
// present shared line.
func (refresher *SharedLineRefresher) resolveTargets(
	ctx context.Context,
	repository port.RepositoryIdentity,
	selected []branch.BranchName,
) ([]branch.BranchName, error) {
	if len(selected) == 0 {
		lister, ok := refresher.git.(port.LocalBranchLister)
		if !ok {
			return nil, internalDependencyError("local branch lister")
		}
		local, err := lister.LocalBranches(ctx, repository)
		if err != nil {
			return nil, err
		}
		targets := make([]branch.BranchName, 0, len(local))
		for _, name := range local {
			if name.Family().IsSharedLine() {
				targets = append(targets, name)
			}
		}
		return targets, nil
	}

	seen := make(map[string]struct{}, len(selected))
	targets := make([]branch.BranchName, 0, len(selected))
	for _, name := range selected {
		if !name.Family().IsSharedLine() {
			return nil, unsupportedSharedLineRefresh(name)
		}
		if _, found := seen[name.String()]; found {
			continue
		}
		seen[name.String()] = struct{}{}
		exists, err := refresher.git.BranchExists(ctx, repository, name)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, sharedLineNotPresent(name)
		}
		targets = append(targets, name)
	}
	return targets, nil
}

// requireBases proves that every target line carries a fetched remote-tracking
// reference before any mutation happens.
func (refresher *SharedLineRefresher) requireBases(
	ctx context.Context,
	repository port.RepositoryIdentity,
	targets []branch.BranchName,
	bases []branch.TargetBase,
) error {
	for index, base := range bases {
		exists, err := refresher.git.TargetBaseExists(ctx, repository, base)
		if err != nil {
			return err
		}
		if !exists {
			return sharedLineBaseMissing(targets[index], base)
		}
	}
	return nil
}

func containsBranch(names []branch.BranchName, candidate branch.BranchName) bool {
	for _, name := range names {
		if name.String() == candidate.String() {
			return true
		}
	}
	return false
}

func unsupportedSharedLineRefresh(name branch.BranchName) error {
	return problem.New(problem.Details{
		Code:        problem.CodeBranchFamilyInvalid,
		Category:    problem.CategoryGovernance,
		Field:       "shared line",
		Actual:      name.String(),
		Expected:    "main, develop, release/<semver>, or support/<major.minor>",
		Rule:        "an on-demand refresh targets only local shared-line checkouts",
		Example:     "develop",
		Remediation: "synchronize an official working branch with branch sync-base",
	})
}

func sharedLineNotPresent(name branch.BranchName) error {
	return problem.New(problem.Details{
		Code:        problem.CodeBranchBaseInvalid,
		Category:    problem.CategoryRepository,
		Field:       "shared line",
		Actual:      name.String(),
		Expected:    "a locally present shared-line checkout",
		Rule:        "a shared-line refresh advances only existing local checkouts and never creates a missing line",
		Example:     "develop",
		Remediation: "switch to the line once or fetch the selected remote before refreshing it",
	})
}

func sharedLineBaseMissing(name branch.BranchName, base branch.TargetBase) error {
	return problem.New(problem.Details{
		Code:        problem.CodeBranchBaseInvalid,
		Category:    problem.CategoryRepository,
		Field:       "shared line base",
		Actual:      base.String(),
		Expected:    "an existing fetched remote-tracking reference for " + name.String(),
		Rule:        "a shared-line refresh binds to the fetched remote-tracking reference of the line",
		Example:     "origin/develop",
		Remediation: "fetch the selected remote and verify the line exists upstream before refreshing",
	})
}

func operationInProgress(operation string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeOperationInProgress,
		Category:    problem.CategoryRepository,
		Field:       "Git operation state",
		Actual:      operation,
		Expected:    "no in-progress merge, rebase, or cherry-pick",
		Rule:        "a shared-line refresh never runs while another Git operation is in progress",
		Remediation: "complete or abort the active operation, then refresh again",
	})
}

func worktreeNotCleanForRefresh(name branch.BranchName) error {
	return problem.New(problem.Details{
		Code:        problem.CodeWorktreeNotClean,
		Category:    problem.CategoryRepository,
		Field:       "worktree",
		Actual:      name.String(),
		Expected:    "a clean working tree before refreshing the checked-out shared line",
		Rule:        "the checked-out shared line fast-forwards only when no uncommitted changes are at risk",
		Example:     "git status --porcelain returns no entries",
		Remediation: "commit, stash, or discard local changes before refreshing the checked-out line",
	})
}

func sharedLineDiverged(name branch.BranchName, lines []SharedLineRefresh) error {
	refreshed := make([]string, 0, len(lines))
	for _, line := range lines {
		if line.Outcome == port.FastForwardUpdated {
			refreshed = append(refreshed, line.Name.String())
		}
	}
	contextDetail := ""
	if len(refreshed) > 0 {
		contextDetail = "already refreshed in this run: " + strings.Join(refreshed, ", ")
	}
	return problem.New(problem.Details{
		Code:        problem.CodeSharedLineDiverged,
		Category:    problem.CategoryRepository,
		Field:       "shared line",
		Actual:      name.String(),
		Context:     contextDetail,
		Expected:    "a local shared-line checkout that can fast-forward to its fetched remote-tracking reference",
		Rule:        "a diverged local shared line is never forced and never merge-committed by a refresh",
		Remediation: "inspect the local-only commits and reconcile them through the governed workflow before refreshing",
	})
}
