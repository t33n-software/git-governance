package gitcli

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// WorktreeList inventories every worktree of the repository from the
// porcelain worktree format. The inventory preserves Git's stable output
// order: the primary checkout first, every linked worktree after it.
func (repository *Repository) WorktreeList(ctx context.Context, identity port.RepositoryIdentity) ([]port.WorktreeEntry, error) {
	result := repository.invoke(ctx, identity.Root, nil, "worktree", "list", "--porcelain")
	if result.err != nil {
		return nil, repository.commandProblem(problem.CodeGitCommandFailed, identity, "list worktrees", result)
	}
	return parseWorktreeList(result.stdout), nil
}

// parseWorktreeList parses the porcelain worktree format. Every record starts
// with a worktree attribute and is terminated by a blank line; HEAD, branch,
// detached, and bare attributes refine the record.
func parseWorktreeList(raw string) []port.WorktreeEntry {
	entries := make([]port.WorktreeEntry, 0)
	var current *port.WorktreeEntry
	flush := func() {
		if current != nil {
			entries = append(entries, *current)
			current = nil
		}
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			flush()
			continue
		}
		attribute, value, _ := strings.Cut(line, " ")
		switch attribute {
		case "worktree":
			flush()
			current = &port.WorktreeEntry{Path: value}
		case "HEAD":
			if current != nil {
				current.Head = value
			}
		case "branch":
			if current != nil {
				if name, found := strings.CutPrefix(value, "refs/heads/"); found {
					current.Branch = name
				}
			}
		case "detached":
			if current != nil {
				current.Detached = true
			}
		case "bare":
			if current != nil {
				current.Bare = true
			}
		}
	}
	flush()
	return entries
}

// WorktreeAddDetached acquires a new detached task worktree from a fetched
// remote-tracking base. The fetch of the remote base and the detached
// worktree creation are one internal acquisition step: the created worktree
// always starts from the current revision of the selected remote base.
func (repository *Repository) WorktreeAddDetached(ctx context.Context, identity port.RepositoryIdentity, path string, base branch.TargetBase) error {
	if !base.IsRemoteTracking() {
		return problem.New(problem.Details{
			Code:        problem.CodeBranchBaseInvalid,
			Category:    problem.CategoryRepository,
			Field:       "target base",
			Actual:      base.String(),
			Expected:    "a remote-tracking target base",
			Rule:        "task worktrees are acquired from the fetched selected remote",
			Example:     identity.Remote + "/develop",
			Remediation: "select a canonical remote target base",
		})
	}
	if err := repository.Fetch(ctx, identity); err != nil {
		return err
	}
	result := repository.invoke(ctx, identity.Root, nil, "worktree", "add", "--detach", path, base.String())
	if result.err != nil {
		if strings.Contains(result.stderr, "already exists") {
			return worktreeConflictProblem(path)
		}
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "create the detached task worktree", result)
	}
	return nil
}

// WorktreeHeadMatchesBase resolves the HEAD revision of the checkout and the
// revision of the acquired remote-tracking base as one measured evidence
// pair. Both revisions resolve from the running checkout: a linked worktree
// resolves its own HEAD and the shared remote-tracking reference of the
// repository.
func (repository *Repository) WorktreeHeadMatchesBase(ctx context.Context, identity port.RepositoryIdentity, base branch.TargetBase) (string, string, error) {
	head := repository.invoke(ctx, identity.Root, nil, "rev-parse", "HEAD")
	if head.err != nil {
		return "", "", repository.commandProblem(problem.CodeGitCommandFailed, identity, "resolve the checkout revision", head)
	}
	baseRevision := repository.invoke(ctx, identity.Root, nil, "rev-parse", base.String())
	if baseRevision.err != nil {
		return "", "", repository.commandProblem(problem.CodeGitCommandFailed, identity, "resolve the acquired base revision", baseRevision)
	}
	return strings.TrimSpace(head.stdout), strings.TrimSpace(baseRevision.stdout), nil
}

// worktreeConflictProblem is the named conflict record of an occupied
// task-worktree path: the directory or worktree of the derived ticket path
// already exists, so the acquisition refuses to mutate instead of failing
// with an opaque Git diagnostic.
func worktreeConflictProblem(path string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeWorktreeConflict,
		Category:    problem.CategoryRepository,
		Field:       "worktree",
		Actual:      path,
		Expected:    "an unoccupied task-worktree path",
		Rule:        "task worktrees are acquired once per ticket; an existing worktree or directory at the derived path is a conflict",
		Example:     "workflow worktree list",
		Remediation: "continue in the existing worktree by running the ticket workflow inside it, or inventory the worktrees with workflow worktree list",
	})
}

// WorktreeRemove removes a task worktree fail-closed. The removal addresses
// the real registered path of the logical worktree name: the target is
// resolved case-insensitively against the porcelain worktree inventory, so a
// case-variant spelling of the same logical name stays addressable through
// the governed endpoint. The working tree of the resolved target must be
// clean before the removal mutates anything.
func (repository *Repository) WorktreeRemove(ctx context.Context, identity port.RepositoryIdentity, path string) error {
	listing := repository.invoke(ctx, identity.Root, nil, "worktree", "list", "--porcelain")
	if listing.err != nil {
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "resolve the registered task worktree", listing)
	}
	target := path
	for _, entry := range parseWorktreeList(listing.stdout) {
		if strings.EqualFold(filepath.Clean(entry.Path), filepath.Clean(path)) {
			target = entry.Path
			break
		}
	}
	status := repository.invoke(ctx, target, nil, "status", "--porcelain=v1", "--untracked-files=normal")
	if status.err != nil {
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "inspect the target worktree", status)
	}
	if strings.TrimSpace(status.stdout) != "" {
		return problem.New(problem.Details{
			Code:        problem.CodeWorktreeNotClean,
			Category:    problem.CategoryRepository,
			Field:       "worktree",
			Actual:      target,
			Expected:    "a clean target worktree before removal",
			Rule:        "worktree removal must not discard uncommitted work",
			Example:     "git status --porcelain returns no entries",
			Remediation: "commit, publish, or clean the worktree before removing it",
		})
	}
	result := repository.invoke(ctx, identity.Root, nil, "worktree", "remove", target)
	if result.err != nil {
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "remove the task worktree", result)
	}
	return nil
}

// LinkedWorktree reports whether the checkout at the repository root is a
// linked worktree rather than the primary checkout. The Git directory of a
// linked worktree lives below the common Git directory of the repository.
func (repository *Repository) LinkedWorktree(ctx context.Context, identity port.RepositoryIdentity) (bool, error) {
	gitDirectory := repository.invoke(ctx, identity.Root, nil, "rev-parse", "--git-dir")
	if gitDirectory.err != nil {
		return false, repository.commandProblem(problem.CodeGitCommandFailed, identity, "resolve the Git directory", gitDirectory)
	}
	commonDirectory := repository.invoke(ctx, identity.Root, nil, "rev-parse", "--git-common-dir")
	if commonDirectory.err != nil {
		return false, repository.commandProblem(problem.CodeGitCommandFailed, identity, "resolve the common Git directory", commonDirectory)
	}
	return strings.TrimSpace(gitDirectory.stdout) != strings.TrimSpace(commonDirectory.stdout), nil
}

var _ port.WorktreeManager = (*Repository)(nil)
var _ port.WorktreeInventoryLister = (*Repository)(nil)
