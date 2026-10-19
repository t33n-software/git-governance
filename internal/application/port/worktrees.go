package port

import (
	"context"

	"github.com/t33n-software/git-governance/internal/domain/branch"
)

// LocalBranchLister is an optional capability for adapters that can enumerate
// the local canonical branches of a repository. Keeping it separate avoids
// forcing unrelated Git adapters and test fakes to implement a read they never
// invoke.
type LocalBranchLister interface {
	LocalBranches(ctx context.Context, repository RepositoryIdentity) ([]branch.BranchName, error)
}

// RemoteBranchLister is an optional capability for adapters that can enumerate
// the fetched remote-tracking canonical branches of the selected remote. The
// allocation inventory reads the branch-ref surface through it; keeping the
// capability separate avoids forcing unrelated adapters and test fakes to
// implement a read they never invoke.
type RemoteBranchLister interface {
	RemoteBranches(ctx context.Context, repository RepositoryIdentity) ([]branch.BranchName, error)
}

// CommitSubjectLister is an optional capability for adapters that can read the
// complete commit subject history across every reachable reference, including
// unborn repositories that carry none. The allocation inventory parses those
// subjects with the canonical commit header grammar; it never string-matches.
type CommitSubjectLister interface {
	CommitSubjects(ctx context.Context, repository RepositoryIdentity) ([]string, error)
}

// WorktreeEntry describes one worktree of the Git worktree inventory.
type WorktreeEntry struct {
	// Path is the absolute worktree directory.
	Path string `json:"path"`
	// Head is the commit object the worktree currently resolves, when known.
	Head string `json:"head"`
	// Branch is the checked-out canonical branch name; empty when the
	// worktree is detached or bare.
	Branch string `json:"branch"`
	// Detached reports a worktree whose HEAD points at a commit instead of a
	// branch.
	Detached bool `json:"detached"`
	// Bare reports a bare worktree without a working tree.
	Bare bool `json:"bare"`
}

// WorktreeManager is an optional capability for adapters that can manage task
// worktrees: the porcelain inventory, the detached acquisition of a new
// worktree from a fetched remote-tracking base, the fail-closed removal of a
// clean worktree, and the linked-worktree form of the running checkout.
// Keeping the capability separate avoids forcing unrelated Git adapters and
// test fakes to implement worktree mutations they never invoke.
type WorktreeManager interface {
	WorktreeList(ctx context.Context, repository RepositoryIdentity) ([]WorktreeEntry, error)
	WorktreeAddDetached(ctx context.Context, repository RepositoryIdentity, path string, base branch.TargetBase) error
	WorktreeRemove(ctx context.Context, repository RepositoryIdentity, path string) error
	LinkedWorktree(ctx context.Context, repository RepositoryIdentity) (bool, error)
	// WorktreeHeadMatchesBase resolves the HEAD revision of the checkout and
	// the revision of the acquired remote-tracking base as one measured
	// evidence pair. The ticket-start guard compares the pair so the detached
	// pre-start form stays legitimate only at its acquired base revision.
	WorktreeHeadMatchesBase(ctx context.Context, repository RepositoryIdentity, base branch.TargetBase) (head string, baseRevision string, err error)
}

// WorktreeBranchLocation reports the worktree path where the branch is
// currently checked out, derived from the porcelain worktree inventory. A
// detached or bare worktree never carries a branch.
func WorktreeBranchLocation(entries []WorktreeEntry, name branch.BranchName) (string, bool) {
	for _, entry := range entries {
		if !entry.Detached && !entry.Bare && entry.Branch == name.String() {
			return entry.Path, true
		}
	}
	return "", false
}

// WorktreeInventoryLister is an optional capability for adapters that can
// enumerate the worktree inventory of a repository. The allocation inventory
// reads the registered task-worktree surface through it; keeping the
// capability separate avoids forcing unrelated Git adapters and test fakes
// to implement worktree mutations they never invoke.
type WorktreeInventoryLister interface {
	WorktreeList(ctx context.Context, repository RepositoryIdentity) ([]WorktreeEntry, error)
}
