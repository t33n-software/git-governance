package workflow

import (
	"context"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// RecoverUnbornRequest describes the governed pre-birth recovery of an
// unborn repository whose index or references carry a foreign or aborted
// pre-staging state.
type RecoverUnbornRequest struct {
	Repository port.RepositoryIdentity
	DryRun     bool
}

// BootstrapRecoveryResult carries the executed or planned recovery steps and
// the proven pre-state facts.
type BootstrapRecoveryResult struct {
	Plan              []branchapp.PlanStep
	DryRun            bool
	IndexEmptied      bool
	ReferencesRemoved bool
}

// RecoverUnborn restores the proven pre-birth state of an unborn repository:
// the read-only proof binds the unborn topology — no commits, the unborn HEAD
// targeting main, no active Git operation — the mutation empties the index
// and removes every reference through the same governed restoration the birth
// compensation uses, and the read-only read-back proves the restored state.
// The working tree is never modified, so foreign working-tree changes survive
// unchanged. A born repository is never recovered; its corrections belong to
// the governed lifecycle, not to a destructive pre-birth reset.
func (service *BootstrapService) RecoverUnborn(ctx context.Context, request RecoverUnbornRequest) (BootstrapRecoveryResult, error) {
	if service.git == nil {
		return BootstrapRecoveryResult{}, internalDependencyError("bootstrap services")
	}
	repository, err := bootstrapRepository(request.Repository)
	if err != nil {
		return BootstrapRecoveryResult{}, err
	}
	if err := bootstrapContextError(ctx); err != nil {
		return BootstrapRecoveryResult{}, err
	}
	refs, restorer, err := resolveRecoveryCapabilities(service.git)
	if err != nil {
		return BootstrapRecoveryResult{}, err
	}
	proof, err := service.proveUnbornRecoveryState(ctx, repository, refs)
	if err != nil {
		return BootstrapRecoveryResult{}, err
	}
	plan := recoveryPlan(proof)
	if request.DryRun {
		return BootstrapRecoveryResult{Plan: plan, DryRun: true}, nil
	}
	if err := restorer.RestoreUnbornState(ctx, repository); err != nil {
		return BootstrapRecoveryResult{}, err
	}
	if err := service.proveRestoredPreState(ctx, repository, refs); err != nil {
		return BootstrapRecoveryResult{}, err
	}
	return BootstrapRecoveryResult{
		Plan:              plan,
		IndexEmptied:      proof.Staged,
		ReferencesRemoved: proof.Referenced,
	}, nil
}

// unbornRecoveryProof carries the read-only state facts the recovery proves
// before and after the restoration.
type unbornRecoveryProof struct {
	Staged     bool
	Referenced bool
}

func (service *BootstrapService) proveUnbornRecoveryState(
	ctx context.Context,
	repository port.RepositoryIdentity,
	refs port.RefExistenceInspector,
) (unbornRecoveryProof, error) {
	hasCommits, err := service.git.HasCommits(ctx, repository)
	if err != nil {
		return unbornRecoveryProof{}, err
	}
	if hasCommits {
		return unbornRecoveryProof{}, recoveryStateInvalid("the repository HEAD already carries a commit")
	}
	current, err := service.git.CurrentBranch(ctx, repository)
	if err != nil {
		return unbornRecoveryProof{}, err
	}
	if current.Family() != branch.FamilyMain {
		return unbornRecoveryProof{}, unbornHeadMismatch(current)
	}
	operation, active, err := service.git.ActiveOperation(ctx, repository)
	if err != nil {
		return unbornRecoveryProof{}, err
	}
	if active {
		return unbornRecoveryProof{}, problem.New(problem.Details{
			Code:        problem.CodeOperationInProgress,
			Category:    problem.CategoryRepository,
			Field:       "git operation",
			Actual:      operation,
			Expected:    "no merge, rebase, or cherry-pick in progress",
			Rule:        "a running Git operation blocks the governed pre-birth recovery",
			Remediation: "complete or abort the active operation and retry the recovery",
		})
	}
	staged, err := service.git.HasStagedChanges(ctx, repository)
	if err != nil {
		return unbornRecoveryProof{}, err
	}
	anyRef, err := refs.HasAnyRef(ctx, repository)
	if err != nil {
		return unbornRecoveryProof{}, err
	}
	return unbornRecoveryProof{Staged: staged, Referenced: anyRef}, nil
}

func (service *BootstrapService) proveRestoredPreState(
	ctx context.Context,
	repository port.RepositoryIdentity,
	refs port.RefExistenceInspector,
) error {
	staged, err := service.git.HasStagedChanges(ctx, repository)
	if err != nil {
		return recoveryReadBackProblem("the staged-state read-back failed", err)
	}
	if staged {
		return recoveryReadBackProblem("the index still carries staged entries after the restoration", nil)
	}
	anyRef, err := refs.HasAnyRef(ctx, repository)
	if err != nil {
		return recoveryReadBackProblem("the reference read-back failed", err)
	}
	if anyRef {
		return recoveryReadBackProblem("the repository still carries references after the restoration", nil)
	}
	return nil
}

func resolveRecoveryCapabilities(git port.GitRepository) (port.RefExistenceInspector, port.UnbornStateRestorer, error) {
	refs, ok := git.(port.RefExistenceInspector)
	if !ok {
		return nil, nil, bootstrapCapabilityRequired("reference inspection")
	}
	restorer, ok := git.(port.UnbornStateRestorer)
	if !ok {
		return nil, nil, bootstrapCapabilityRequired("unborn-state restoration")
	}
	return refs, restorer, nil
}

// recoveryPlan renders the recovery steps: the unborn proof, the state
// reductions the proof found, and the read-back evidence.
func recoveryPlan(proof unbornRecoveryProof) []branchapp.PlanStep {
	plan := []branchapp.PlanStep{
		{Action: "prove-unborn", Detail: "unborn-state proof: no commits, unborn HEAD on main, no active Git operation"},
	}
	if proof.Staged {
		plan = append(plan, branchapp.PlanStep{Action: "empty-index", Detail: "empty the index; staged content stays in the working tree as untracked files"})
	}
	if proof.Referenced {
		plan = append(plan, branchapp.PlanStep{Action: "remove-references", Detail: "remove every reference of the aborted or foreign pre-staging state"})
	}
	plan = append(plan, branchapp.PlanStep{Action: "prove-pre-state", Detail: "read-back proof: empty index and no references"})
	return plan
}

// recoveryStateInvalid refuses a recovery outside the unborn pre-birth
// domain: a born repository is never reset.
func recoveryStateInvalid(detail string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeRepositoryAlreadyBorn,
		Category:    problem.CategoryRepository,
		Field:       "repository",
		Actual:      detail,
		Expected:    "an unborn repository without commits",
		Rule:        "the governed pre-birth recovery restores only unborn repositories; a born repository is never reset",
		Remediation: "run the recovery only on a repository without commits",
	})
}

func recoveryReadBackProblem(detail string, cause error) error {
	problemDetails := problem.Details{
		Code:        problem.CodeInternal,
		Category:    problem.CategoryInternal,
		Field:       "recovery read-back",
		Diagnostic:  detail,
		Expected:    "the proven pre-birth state: empty index and no references",
		Rule:        "a failed read-back leaves the repository state in place and blocks the retry",
		Remediation: "review the repository state and retry the recovery",
	}
	if cause == nil {
		return problem.New(problemDetails)
	}
	return problem.Wrap(problemDetails, cause)
}
