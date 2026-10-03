package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
)

// worktreeCommandGit extends the shared command fake with the worktree
// capability the worktree workflow commands consume.
type worktreeCommandGit struct {
	*commandGit
	entries    []port.WorktreeEntry
	linked     bool
	linkedErr  error
	listErr    error
	addErr     error
	addedPaths []string
	removed    []string
}

func (git *worktreeCommandGit) WorktreeList(context.Context, port.RepositoryIdentity) ([]port.WorktreeEntry, error) {
	return git.entries, git.listErr
}

func (git *worktreeCommandGit) WorktreeAddDetached(_ context.Context, _ port.RepositoryIdentity, path string, _ branch.TargetBase) error {
	if git.addErr != nil {
		return git.addErr
	}
	git.addedPaths = append(git.addedPaths, path)
	return nil
}

func (git *worktreeCommandGit) WorktreeRemove(_ context.Context, _ port.RepositoryIdentity, path string) error {
	git.removed = append(git.removed, path)
	return nil
}

func (git *worktreeCommandGit) LinkedWorktree(context.Context, port.RepositoryIdentity) (bool, error) {
	return git.linked, git.linkedErr
}

func TestWorktreeStartCommand(t *testing.T) {
	t.Parallel()

	t.Run("acquires the detached task worktree and reports the ticket binding", func(t *testing.T) {
		t.Parallel()
		git := &worktreeCommandGit{commandGit: newCommandGit(t, "develop", nil)}
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		output, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "--yes", "workflow", "worktree", "start", "--key", "GOV", "--ticket", "129")
		if err != nil {
			t.Fatalf("worktree start error = %v; output=%q", err, output)
		}
		expectedPath := filepath.Join(filepath.Dir("C:/repo"), "repo-GOV-129")
		for _, expected := range []string{
			`"operation":"workflow.worktree.start"`,
			`"base":"origin/develop"`,
			`"ticket":"GOV-129"`,
			"repo-GOV-129",
		} {
			if !strings.Contains(output, expected) {
				t.Fatalf("worktree start output missing %q: %q", expected, output)
			}
		}
		if len(git.addedPaths) != 1 || git.addedPaths[0] != expectedPath {
			t.Fatalf("worktree additions = %v", git.addedPaths)
		}
	})

	t.Run("plans the acquisition without mutating during dry-run", func(t *testing.T) {
		t.Parallel()
		git := &worktreeCommandGit{commandGit: newCommandGit(t, "develop", nil)}
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		output, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "--dry-run", "workflow", "worktree", "start", "--key", "GOV", "--ticket", "129")
		if err != nil {
			t.Fatalf("worktree start error = %v; output=%q", err, output)
		}
		if !strings.Contains(output, `"dryRun":"true"`) {
			t.Fatalf("dry-run output missing the dry-run marker: %q", output)
		}
		if len(git.addedPaths) != 0 {
			t.Fatalf("a dry run must not create the worktree: %v", git.addedPaths)
		}
	})

	t.Run("requires a ticket key in non-interactive runs", func(t *testing.T) {
		t.Parallel()
		git := &worktreeCommandGit{commandGit: newCommandGit(t, "develop", nil)}
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		_, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "workflow", "worktree", "start")
		if err == nil {
			t.Fatal("worktree start without a key unexpectedly succeeded")
		}
		if !strings.Contains(err.Error(), "ticket key") {
			t.Fatalf("missing-key failure must name the ticket key input: %v", err)
		}
	})

	t.Run("rejects a nested acquisition from inside a linked worktree", func(t *testing.T) {
		t.Parallel()
		git := &worktreeCommandGit{commandGit: newCommandGit(t, "develop", nil), linked: true}
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		if _, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "--yes", "workflow", "worktree", "start", "--key", "GOV", "--ticket", "129"); err == nil {
			t.Fatal("a nested acquisition unexpectedly succeeded")
		}
		if len(git.addedPaths) != 0 {
			t.Fatalf("a nested acquisition must not create a worktree: %v", git.addedPaths)
		}
	})

	t.Run("fails when the repository cannot be discovered", func(t *testing.T) {
		t.Parallel()
		git := &worktreeCommandGit{commandGit: newCommandGit(t, "develop", nil)}
		git.discoverErr = errors.New("discover failed")
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		if _, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "--yes", "workflow", "worktree", "start", "--key", "GOV", "--ticket", "129"); err == nil {
			t.Fatal("worktree start unexpectedly succeeded with a discovery failure")
		}
	})

	t.Run("requires a ticket number in non-interactive runs", func(t *testing.T) {
		t.Parallel()
		git := &worktreeCommandGit{commandGit: newCommandGit(t, "develop", nil)}
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		if _, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "workflow", "worktree", "start", "--key", "GOV"); err == nil {
			t.Fatal("worktree start without a ticket number unexpectedly succeeded")
		}
		if len(git.addedPaths) != 0 {
			t.Fatalf("a rejected request must not create a worktree: %v", git.addedPaths)
		}
	})

	t.Run("requires --yes for a non-interactive acquisition", func(t *testing.T) {
		t.Parallel()
		git := &worktreeCommandGit{commandGit: newCommandGit(t, "develop", nil)}
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		if _, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "workflow", "worktree", "start", "--key", "GOV", "--ticket", "129"); err == nil {
			t.Fatal("a non-interactive acquisition without --yes unexpectedly succeeded")
		}
		if len(git.addedPaths) != 0 {
			t.Fatalf("an unconfirmed acquisition must never mutate: %v", git.addedPaths)
		}
	})
}

func TestWorktreeListCommand(t *testing.T) {
	t.Parallel()

	t.Run("inventories the worktrees as JSON", func(t *testing.T) {
		t.Parallel()
		git := &worktreeCommandGit{commandGit: newCommandGit(t, "develop", nil), entries: []port.WorktreeEntry{
			{Path: "C:/repo", Head: "abc", Branch: "develop"},
			{Path: "C:/repo-GOV-129", Head: "def", Detached: true},
		}}
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		output, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "workflow", "worktree", "list")
		if err != nil {
			t.Fatalf("worktree list error = %v; output=%q", err, output)
		}
		for _, expected := range []string{
			`"operation":"workflow.worktree.list"`,
			`"worktreeCount":"2"`,
			`"path":"C:/repo"`,
			`"branch":"develop"`,
			`"path":"C:/repo-GOV-129"`,
			`"detached":true`,
		} {
			if !strings.Contains(output, expected) {
				t.Fatalf("worktree list output missing %q: %q", expected, output)
			}
		}
	})

	t.Run("fails closed on an inventory failure", func(t *testing.T) {
		t.Parallel()
		git := &worktreeCommandGit{commandGit: newCommandGit(t, "develop", nil), listErr: errWorktreeListFailed}
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		if _, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "workflow", "worktree", "list"); err == nil {
			t.Fatal("worktree list unexpectedly succeeded with an inventory failure")
		}
	})

	t.Run("fails when the repository cannot be discovered", func(t *testing.T) {
		t.Parallel()
		git := &worktreeCommandGit{commandGit: newCommandGit(t, "develop", nil)}
		git.discoverErr = errors.New("discover failed")
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		if _, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "workflow", "worktree", "list"); err == nil {
			t.Fatal("worktree list unexpectedly succeeded with a discovery failure")
		}
	})

	t.Run("renders bare worktrees without a branch", func(t *testing.T) {
		t.Parallel()
		git := &worktreeCommandGit{commandGit: newCommandGit(t, "develop", nil), entries: []port.WorktreeEntry{
			{Path: "C:/bare", Bare: true},
		}}
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		output, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "workflow", "worktree", "list")
		if err != nil {
			t.Fatalf("worktree list error = %v; output=%q", err, output)
		}
		for _, expected := range []string{
			`"path":"C:/bare"`,
			`"bare":true`,
			`"C:/bare":"bare"`,
		} {
			if !strings.Contains(output, expected) {
				t.Fatalf("bare worktree output missing %q: %q", expected, output)
			}
		}
	})
}

var errWorktreeListFailed = errWorktreeListFailedValue{}

type errWorktreeListFailedValue struct{}

func (errWorktreeListFailedValue) Error() string { return "worktree list failed" }

func TestWorktreeRemoveCommand(t *testing.T) {
	t.Parallel()

	t.Run("removes the registered task worktree of the completed ticket", func(t *testing.T) {
		t.Parallel()
		expectedPath := filepath.Join(filepath.Dir("C:/repo"), "repo-GOV-129")
		git := &worktreeCommandGit{commandGit: newCommandGit(t, "develop", nil), entries: []port.WorktreeEntry{
			{Path: "C:/repo", Head: "abc", Branch: "develop"},
			{Path: expectedPath, Head: "def", Detached: true},
		}}
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		output, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "--yes", "workflow", "worktree", "remove", "--key", "GOV", "--ticket", "129")
		if err != nil {
			t.Fatalf("worktree remove error = %v; output=%q", err, output)
		}
		for _, expected := range []string{
			`"operation":"workflow.worktree.remove"`,
			`"ticket":"GOV-129"`,
		} {
			if !strings.Contains(output, expected) {
				t.Fatalf("worktree remove output missing %q: %q", expected, output)
			}
		}
		if len(git.removed) != 1 || git.removed[0] != expectedPath {
			t.Fatalf("worktree removals = %v", git.removed)
		}
	})

	t.Run("fails closed for an unregistered worktree", func(t *testing.T) {
		t.Parallel()
		git := &worktreeCommandGit{commandGit: newCommandGit(t, "develop", nil), entries: []port.WorktreeEntry{
			{Path: "C:/repo", Head: "abc", Branch: "develop"},
		}}
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		if _, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "--yes", "workflow", "worktree", "remove", "--key", "GOV", "--ticket", "129"); err == nil {
			t.Fatal("removing an unregistered worktree unexpectedly succeeded")
		}
		if len(git.removed) != 0 {
			t.Fatalf("an unregistered worktree must never be removed: %v", git.removed)
		}
	})

	t.Run("requires explicit confirmation without --yes in non-interactive runs", func(t *testing.T) {
		t.Parallel()
		git := &worktreeCommandGit{commandGit: newCommandGit(t, "develop", nil), entries: []port.WorktreeEntry{
			{Path: "C:/repo-GOV-129", Head: "def", Detached: true},
		}}
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		if _, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "workflow", "worktree", "remove", "--key", "GOV", "--ticket", "129"); err == nil {
			t.Fatal("a non-interactive removal without --yes unexpectedly succeeded")
		}
		if len(git.removed) != 0 {
			t.Fatalf("an unconfirmed removal must never mutate: %v", git.removed)
		}
	})

	t.Run("fails when the repository cannot be discovered", func(t *testing.T) {
		t.Parallel()
		git := &worktreeCommandGit{commandGit: newCommandGit(t, "develop", nil)}
		git.discoverErr = errors.New("discover failed")
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		if _, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "--yes", "workflow", "worktree", "remove", "--key", "GOV", "--ticket", "129"); err == nil {
			t.Fatal("worktree remove unexpectedly succeeded with a discovery failure")
		}
		if len(git.removed) != 0 {
			t.Fatalf("a failed discovery must never remove a worktree: %v", git.removed)
		}
	})

	t.Run("requires a ticket key in non-interactive runs", func(t *testing.T) {
		t.Parallel()
		git := &worktreeCommandGit{commandGit: newCommandGit(t, "develop", nil)}
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		if _, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "workflow", "worktree", "remove", "--ticket", "129"); err == nil {
			t.Fatal("worktree remove without a ticket key unexpectedly succeeded")
		}
		if len(git.removed) != 0 {
			t.Fatalf("a rejected request must not remove a worktree: %v", git.removed)
		}
	})

	t.Run("requires a ticket number in non-interactive runs", func(t *testing.T) {
		t.Parallel()
		git := &worktreeCommandGit{commandGit: newCommandGit(t, "develop", nil)}
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		if _, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "workflow", "worktree", "remove", "--key", "GOV"); err == nil {
			t.Fatal("worktree remove without a ticket number unexpectedly succeeded")
		}
		if len(git.removed) != 0 {
			t.Fatalf("a rejected request must not remove a worktree: %v", git.removed)
		}
	})
}
