package port

import (
	"context"

	"github.com/t33n-software/git-governance/internal/domain/branch"
)

// StagePreviewer is an optional capability for adapters that can resolve the
// effective initial content set of explicit stage paths without mutating the
// index. The preview binds the boundary scan to exactly the paths a later
// stage operation would add.
type StagePreviewer interface {
	PreviewStage(ctx context.Context, repository RepositoryIdentity, paths []string) ([]string, error)
}

// RefExistenceInspector is an optional capability for adapters that can prove
// whether any Git reference exists at all. The governed repository birth
// requires this proof: an unborn repository carries no references.
type RefExistenceInspector interface {
	HasAnyRef(ctx context.Context, repository RepositoryIdentity) (bool, error)
}

// CommitSignatureVerifier is an optional capability for adapters that can
// prove the signature of an existing commit object. The genesis finalizer
// re-proves the signature of the created genesis commit independently of the
// creation path.
type CommitSignatureVerifier interface {
	VerifyCommitSignature(ctx context.Context, repository RepositoryIdentity, revision string) error
}

// HookInstallation proves the installed local hook boundary of a repository.
type HookInstallation struct {
	Directory string
	Hooks     []string
}

// HookInstaller is an optional capability for adapters that can install the
// repository's hook boundary and prove the installed hooks. The governed
// repository birth installs the boundary so every subsequent operation is
// governed from the first commit onward.
type HookInstaller interface {
	InstallHooks(ctx context.Context, repository RepositoryIdentity) (HookInstallation, error)
}

// UnbornStateRestorer is an optional capability for adapters that can restore
// the proven pre-birth state of an unborn repository after an aborted genesis
// step: the index returns to the empty tree and every existing reference is
// deleted, which leaves the symbolic unborn HEAD and the working tree
// untouched. The governed repository birth compensates every aborted genesis
// step through this capability so the retry of the same endpoint stays
// idempotent.
type UnbornStateRestorer interface {
	RestoreUnbornState(ctx context.Context, repository RepositoryIdentity) error
}

// HeadCommitCounter is an optional capability for adapters that can count the
// commits reachable from HEAD. The governed publication resume proves the
// birth topology through it: a born repository carries exactly one genesis
// commit, and any additional commit breaks the resume state.
type HeadCommitCounter interface {
	CountHeadCommits(ctx context.Context, repository RepositoryIdentity) (int, error)
}

// HookBoundaryInspector is an optional capability for adapters that can prove
// the canonical hook boundary is materialized in the repository's Git hooks
// directory. The genesis finalizer installs the boundary; the publication
// resume re-proves it before the shared-line push without reinstalling it.
type HookBoundaryInspector interface {
	HookBoundaryPresent(ctx context.Context, repository RepositoryIdentity) (bool, error)
}

// CherryPickContinuator is consumed only by workflows that must resume a
// user-resolved cherry-pick. Keeping it separate avoids forcing unrelated Git
// adapters and test fakes to implement a mutation they never invoke.
type CherryPickContinuator interface {
	ContinueCherryPick(ctx context.Context, repository RepositoryIdentity) error
}

// MergeContinuator is consumed only by workflows that resume a manually
// resolved merge. Keeping it optional preserves the narrow GitRepository
// contract for workflows that never continue merge state.
type MergeContinuator interface {
	ContinueMerge(ctx context.Context, repository RepositoryIdentity) error
}

// ActiveMergeTargetInspector verifies that an in-progress merge still targets
// the fetched remote-tracking base that the workflow is allowed to integrate.
type ActiveMergeTargetInspector interface {
	ActiveMergeTargetMatches(ctx context.Context, repository RepositoryIdentity, target branch.TargetBase) (bool, error)
}

// PausedRebaseBranchInspector is an optional capability for adapters that can
// resolve the canonical branch carried by a paused rebase state. During a
// paused rebase Git detaches HEAD by design, so the branch survives only in
// the operation state; a resume derives it there instead of requiring a
// checked-out branch. Canonical convention:
// docs/conventions/branches/resume-branch-derivation.md.
type PausedRebaseBranchInspector interface {
	PausedRebaseBranch(ctx context.Context, repository RepositoryIdentity) (branch.BranchName, bool, error)
}

// ReconciliationMergeInspector verifies that the checked-out resolution
// candidate is the exact merge of a delivered release ref and a pinned develop
// ref before a privileged controller publishes it.
type ReconciliationMergeInspector interface {
	ResolveReconciliationBases(
		ctx context.Context,
		repository RepositoryIdentity,
		release branch.TargetBase,
		develop branch.TargetBase,
	) (releaseRevision string, developRevision string, err error)
	HeadIsMergeOf(
		ctx context.Context,
		repository RepositoryIdentity,
		release branch.TargetBase,
		develop branch.TargetBase,
	) (bool, error)
}

// FastForwardOutcome classifies a guarded fast-forward-only update of a local
// branch to its fetched remote-tracking base.
type FastForwardOutcome string

const (
	// FastForwardUpdated reports that the local branch advanced to the base.
	FastForwardUpdated FastForwardOutcome = "updated"
	// FastForwardAlreadyCurrent reports that the local branch already matched
	// the base; no mutation happened.
	FastForwardAlreadyCurrent FastForwardOutcome = "already-current"
	// FastForwardDiverged reports that the local branch cannot fast-forward
	// to the base and was left untouched.
	FastForwardDiverged FastForwardOutcome = "diverged"
)

// BranchFastForwarder is an optional capability for adapters that can advance
// a checked-out local branch to its fetched remote-tracking base when — and
// only when — that update is a fast-forward. The caller owns the
// preconditions: the branch is checked out and the worktree is clean. A
// diverged branch is reported, never forced, and never produces a merge
// commit.
type BranchFastForwarder interface {
	FastForwardBranch(
		ctx context.Context,
		repository RepositoryIdentity,
		name branch.BranchName,
		base branch.TargetBase,
	) (FastForwardOutcome, error)
}

// BranchReferenceFastForwarder is an optional capability for adapters that can
// advance a local branch reference to its fetched remote-tracking base when —
// and only when — that update is a fast-forward. The caller owns the
// preconditions: the branch exists locally and is not checked out in any
// worktree, so the pinned reference update never touches a working tree. A
// diverged branch is reported, never forced, and never produces a merge
// commit.
type BranchReferenceFastForwarder interface {
	FastForwardBranchReference(
		ctx context.Context,
		repository RepositoryIdentity,
		name branch.BranchName,
		base branch.TargetBase,
	) (FastForwardOutcome, error)
}

// BranchAheadCounter is an optional capability for adapters that can count
// the commits a local branch carries that its selected remote-tracking base
// does not. The branch-hygiene guards derive the null-ahead class from the
// count; keeping the capability separate avoids forcing unrelated Git
// adapters and test fakes to implement a comparison they never invoke.
type BranchAheadCounter interface {
	CountBranchAheadCommits(ctx context.Context, repository RepositoryIdentity, name branch.BranchName, base branch.TargetBase) (int, error)
}
