package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
)

// branchHygieneCommandGit extends the shared command fake with the evidence
// surfaces the branch-hygiene endpoints consume and records every deletion
// with its force form.
type branchHygieneCommandGit struct {
	*commandGit
	localBranches  []branch.BranchName
	localErr       error
	remoteBranches []branch.BranchName
	remoteErr      error
	worktrees      []port.WorktreeEntry
	worktreeErr    error
	aheadFor       map[string]int
	aheadErr       error
	deleted        []branch.BranchName
	deletedForce   []bool
	deleteErr      error
}

func (git *branchHygieneCommandGit) LocalBranches(context.Context, port.RepositoryIdentity) ([]branch.BranchName, error) {
	return git.localBranches, git.localErr
}

func (git *branchHygieneCommandGit) RemoteBranches(context.Context, port.RepositoryIdentity) ([]branch.BranchName, error) {
	return git.remoteBranches, git.remoteErr
}

func (git *branchHygieneCommandGit) WorktreeList(context.Context, port.RepositoryIdentity) ([]port.WorktreeEntry, error) {
	return git.worktrees, git.worktreeErr
}

func (git *branchHygieneCommandGit) CountBranchAheadCommits(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName, _ branch.TargetBase) (int, error) {
	if git.aheadErr != nil {
		return 0, git.aheadErr
	}
	if ahead, bound := git.aheadFor[name.String()]; bound {
		return ahead, nil
	}
	return 0, nil
}

func (git *branchHygieneCommandGit) DeleteLocalBranch(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName, force bool) error {
	if git.deleteErr != nil {
		return git.deleteErr
	}
	git.deleted = append(git.deleted, name)
	git.deletedForce = append(git.deletedForce, force)
	return nil
}

func TestBranchRemoveCommand(t *testing.T) {
	t.Parallel()

	const taskBranch = "feature/GOV-138-branch-hygiene-endpoints"
	mergedRecord := port.PullRequestSummary{Number: "21", Title: "GOV-138: branch-hygiene-endpoints", State: port.PullRequestStateMerged}

	t.Run("deletes the worked-to-completion branch under the proven evidence", func(t *testing.T) {
		t.Parallel()
		git := &branchHygieneCommandGit{commandGit: newCommandGit(t, "develop", nil), remoteBranches: []branch.BranchName{}}
		command := NewWithRuntime(BuildInfo{Version: "test"}, pruneCommandRuntime(git, []port.PullRequestSummary{mergedRecord}))
		output, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "--yes", "workflow", "branch", "remove", "--branch", taskBranch)
		if err != nil {
			t.Fatalf("branch remove error = %v; output=%q", err, output)
		}
		for _, expected := range []string{
			`"operation":"workflow.branch.remove"`,
			`"branch":"` + taskBranch + `"`,
			`"ticket":"GOV-138"`,
			`"class":"worked-to-completion"`,
			`"pullRequestState":"merged"`,
			`"pullRequestNumber":"21"`,
		} {
			if !strings.Contains(output, expected) {
				t.Fatalf("branch remove output missing %q: %q", expected, output)
			}
		}
		if len(git.deleted) != 1 || git.deleted[0].String() != taskBranch {
			t.Fatalf("deletions = %v", git.deleted)
		}
		if len(git.deletedForce) != 1 || !git.deletedForce[0] {
			t.Fatalf("the guard-proven deletion runs forced: %v", git.deletedForce)
		}
	})

	t.Run("plans the removal without mutating during dry-run", func(t *testing.T) {
		t.Parallel()
		git := &branchHygieneCommandGit{commandGit: newCommandGit(t, "develop", nil), remoteBranches: []branch.BranchName{}}
		command := NewWithRuntime(BuildInfo{Version: "test"}, pruneCommandRuntime(git, []port.PullRequestSummary{mergedRecord}))
		output, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "--dry-run", "workflow", "branch", "remove", "--branch", taskBranch)
		if err != nil {
			t.Fatalf("branch remove error = %v; output=%q", err, output)
		}
		if !strings.Contains(output, `"dryRun":"true"`) {
			t.Fatalf("dry-run output missing the dry-run marker: %q", output)
		}
		if len(git.deleted) != 0 {
			t.Fatalf("a dry run must not delete the branch: %v", git.deleted)
		}
	})

	t.Run("refuses the review continuation context of an open pull request", func(t *testing.T) {
		t.Parallel()
		git := &branchHygieneCommandGit{commandGit: newCommandGit(t, "develop", nil)}
		command := NewWithRuntime(BuildInfo{Version: "test"}, pruneCommandRuntime(git, []port.PullRequestSummary{
			{Number: "22", Title: "GOV-138: branch-hygiene-endpoints", State: port.PullRequestStateOpen},
		}))
		if _, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "--yes", "workflow", "branch", "remove", "--branch", taskBranch); err == nil {
			t.Fatal("the removal of a published-open branch unexpectedly succeeded")
		}
		if len(git.deleted) != 0 {
			t.Fatalf("a published-open branch must never be deleted: %v", git.deleted)
		}
	})

	t.Run("requires the branch flag", func(t *testing.T) {
		t.Parallel()
		git := &branchHygieneCommandGit{commandGit: newCommandGit(t, "develop", nil)}
		command := NewWithRuntime(BuildInfo{Version: "test"}, pruneCommandRuntime(git, nil))
		_, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "--yes", "workflow", "branch", "remove")
		if err == nil {
			t.Fatal("branch remove without a branch unexpectedly succeeded")
		}
		if !strings.Contains(err.Error(), "branch flag is required") {
			t.Fatalf("the missing-branch failure must name the input: %v", err)
		}
	})

	t.Run("requires --yes for a non-interactive removal", func(t *testing.T) {
		t.Parallel()
		git := &branchHygieneCommandGit{commandGit: newCommandGit(t, "develop", nil), remoteBranches: []branch.BranchName{}}
		command := NewWithRuntime(BuildInfo{Version: "test"}, pruneCommandRuntime(git, []port.PullRequestSummary{mergedRecord}))
		if _, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "workflow", "branch", "remove", "--branch", taskBranch); err == nil {
			t.Fatal("a non-interactive removal without --yes unexpectedly succeeded")
		}
		if len(git.deleted) != 0 {
			t.Fatalf("an unconfirmed removal must never mutate: %v", git.deleted)
		}
	})

	t.Run("rejects a non-canonical branch name", func(t *testing.T) {
		t.Parallel()
		git := &branchHygieneCommandGit{commandGit: newCommandGit(t, "develop", nil)}
		command := NewWithRuntime(BuildInfo{Version: "test"}, pruneCommandRuntime(git, nil))
		if _, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "--yes", "workflow", "branch", "remove", "--branch", "not-a-canonical-branch"); err == nil {
			t.Fatal("a non-canonical branch name unexpectedly succeeded")
		}
		if len(git.deleted) != 0 {
			t.Fatalf("an invalid branch name must never delete: %v", git.deleted)
		}
	})

	t.Run("fails closed when the ahead measurement capability is missing", func(t *testing.T) {
		t.Parallel()
		git := newCommandGit(t, "develop", nil)
		command := NewWithRuntime(BuildInfo{Version: "test"}, pruneCommandRuntime(git, nil))
		if _, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "--yes", "workflow", "branch", "remove", "--branch", taskBranch); err == nil {
			t.Fatal("the removal without the ahead capability unexpectedly succeeded")
		}
	})

	t.Run("reports the null-ahead evidence of a never-published removal", func(t *testing.T) {
		t.Parallel()
		git := &branchHygieneCommandGit{commandGit: newCommandGit(t, "develop", nil), remoteBranches: []branch.BranchName{}}
		command := NewWithRuntime(BuildInfo{Version: "test"}, pruneCommandRuntime(git, nil))
		output, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "--yes", "workflow", "branch", "remove", "--branch", taskBranch)
		if err != nil {
			t.Fatalf("branch remove error = %v; output=%q", err, output)
		}
		for _, expected := range []string{
			`"class":"never-published"`,
			`"base":"origin/develop"`,
			`"ahead":"0"`,
		} {
			if !strings.Contains(output, expected) {
				t.Fatalf("branch remove output missing %q: %q", expected, output)
			}
		}
		if len(git.deleted) != 1 {
			t.Fatalf("the never-published removal must delete: %v", git.deleted)
		}
	})

	t.Run("fails when the repository cannot be discovered", func(t *testing.T) {
		t.Parallel()
		git := &branchHygieneCommandGit{commandGit: newCommandGit(t, "develop", nil)}
		git.discoverErr = errors.New("discover failed")
		command := NewWithRuntime(BuildInfo{Version: "test"}, pruneCommandRuntime(git, nil))
		if _, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "--yes", "workflow", "branch", "remove", "--branch", taskBranch); err == nil {
			t.Fatal("branch remove unexpectedly succeeded with a discovery failure")
		}
	})
}

func TestBranchPruneCommand(t *testing.T) {
	t.Parallel()

	const taskBranch = "feature/GOV-138-branch-hygiene-endpoints"
	const docsBranch = "docs/GOV-139-modularize-branch-metadata"
	pruneState := func() *branchHygieneCommandGit {
		return &branchHygieneCommandGit{
			commandGit:     newCommandGit(t, "develop", nil),
			localBranches:  []branch.BranchName{mustBranchName(t, "develop"), mustBranchName(t, taskBranch), mustBranchName(t, docsBranch)},
			remoteBranches: []branch.BranchName{},
		}
	}
	pruneRecords := []port.PullRequestSummary{
		{Number: "21", Title: "GOV-138: branch-hygiene-endpoints", State: port.PullRequestStateMerged},
		{Number: "22", Title: "docs(GOV-139): modularize branch metadata", State: port.PullRequestStateOpen},
	}

	t.Run("deletes the proven-complete branches and keeps the others", func(t *testing.T) {
		t.Parallel()
		git := pruneState()
		command := NewWithRuntime(BuildInfo{Version: "test"}, pruneCommandRuntime(git, pruneRecords))
		output, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "--yes", "workflow", "branch", "prune")
		if err != nil {
			t.Fatalf("branch prune error = %v; output=%q", err, output)
		}
		for _, expected := range []string{
			`"operation":"workflow.branch.prune"`,
			`"branchCount":"3"`,
			`"removedCount":"1"`,
			`"branch":"` + taskBranch + `"`,
			`"class":"worked-to-completion"`,
			`"removed":true`,
		} {
			if !strings.Contains(output, expected) {
				t.Fatalf("branch prune output missing %q: %q", expected, output)
			}
		}
		if len(git.deleted) != 1 || git.deleted[0].String() != taskBranch {
			t.Fatalf("deletions = %v", git.deleted)
		}
	})

	t.Run("plans without mutating during dry-run", func(t *testing.T) {
		t.Parallel()
		git := pruneState()
		command := NewWithRuntime(BuildInfo{Version: "test"}, pruneCommandRuntime(git, pruneRecords))
		output, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "--dry-run", "workflow", "branch", "prune")
		if err != nil {
			t.Fatalf("branch prune error = %v; output=%q", err, output)
		}
		if !strings.Contains(output, `"removedCount":"0"`) || !strings.Contains(output, `"dryRun":"true"`) {
			t.Fatalf("dry-run output missing the plan markers: %q", output)
		}
		if len(git.deleted) != 0 {
			t.Fatalf("a dry run must not delete any branch: %v", git.deleted)
		}
	})

	t.Run("records a refused removal fail-closed", func(t *testing.T) {
		t.Parallel()
		git := pruneState()
		git.deleteErr = errors.New("delete failed")
		command := NewWithRuntime(BuildInfo{Version: "test"}, pruneCommandRuntime(git, pruneRecords))
		output, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "--yes", "workflow", "branch", "prune")
		if err != nil {
			t.Fatalf("branch prune error = %v; output=%q", err, output)
		}
		if !strings.Contains(output, `"removedCount":"0"`) || !strings.Contains(output, "removal refused") {
			t.Fatalf("the refused-removal output = %q", output)
		}
		if len(git.deleted) != 0 {
			t.Fatalf("a refused removal must not record a removal: %v", git.deleted)
		}
	})

	t.Run("requires --yes for a non-interactive prune", func(t *testing.T) {
		t.Parallel()
		git := pruneState()
		command := NewWithRuntime(BuildInfo{Version: "test"}, pruneCommandRuntime(git, pruneRecords))
		if _, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "workflow", "branch", "prune"); err == nil {
			t.Fatal("a non-interactive prune without --yes unexpectedly succeeded")
		}
		if len(git.deleted) != 0 {
			t.Fatalf("an unconfirmed prune must never mutate: %v", git.deleted)
		}
	})

	t.Run("fails closed on an inventory failure", func(t *testing.T) {
		t.Parallel()
		git := pruneState()
		git.localErr = errors.New("locals failed")
		command := NewWithRuntime(BuildInfo{Version: "test"}, pruneCommandRuntime(git, pruneRecords))
		if _, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "--yes", "workflow", "branch", "prune"); err == nil {
			t.Fatal("branch prune unexpectedly succeeded with an inventory failure")
		}
		if len(git.deleted) != 0 {
			t.Fatalf("a failed inventory must never delete: %v", git.deleted)
		}
	})

	t.Run("inventories an empty branch surface", func(t *testing.T) {
		t.Parallel()
		git := &branchHygieneCommandGit{commandGit: newCommandGit(t, "develop", nil), localBranches: []branch.BranchName{}}
		command := NewWithRuntime(BuildInfo{Version: "test"}, pruneCommandRuntime(git, nil))
		output, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "--yes", "workflow", "branch", "prune")
		if err != nil {
			t.Fatalf("branch prune error = %v; output=%q", err, output)
		}
		if !strings.Contains(output, `"branchCount":"0"`) {
			t.Fatalf("the empty inventory output = %q", output)
		}
		if len(git.deleted) != 0 {
			t.Fatalf("an empty surface must never delete: %v", git.deleted)
		}
	})

	t.Run("fails when the repository cannot be discovered", func(t *testing.T) {
		t.Parallel()
		git := &branchHygieneCommandGit{commandGit: newCommandGit(t, "develop", nil)}
		git.discoverErr = errors.New("discover failed")
		command := NewWithRuntime(BuildInfo{Version: "test"}, pruneCommandRuntime(git, nil))
		if _, err := executeBootstrapCommand(t, command, "--interactive", "never", "--output", "json", "--yes", "workflow", "branch", "prune"); err == nil {
			t.Fatal("branch prune unexpectedly succeeded with a discovery failure")
		}
	})
}

// mustBranchName parses one canonical branch name for the command tests.
func mustBranchName(t *testing.T, raw string) branch.BranchName {
	t.Helper()
	name, err := branch.ParseName(raw)
	if err != nil {
		t.Fatal(err)
	}
	return name
}
