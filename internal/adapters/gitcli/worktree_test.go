package gitcli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

const worktreePorcelainFixture = `worktree C:/repo
HEAD abc123def456abc123def456abc123def456abc1
branch refs/heads/develop

worktree C:/repo-GOV-129
HEAD bbc123def456abc123def456abc123def456abc2
detached

worktree C:/repo-GOV-130
HEAD cbc123def456abc123def456abc123def456abc3
branch refs/heads/feature/GOV-130-add-export

worktree C:/bare
bare
`

func mustWorktreeBranch(raw string) branch.BranchName {
	name, err := branch.ParseName(raw)
	if err != nil {
		panic(err)
	}
	return name
}

func mustWorktreeBase(remote, raw string) branch.TargetBase {
	base, err := branch.NewTargetBase(remote, mustWorktreeBranch(raw))
	if err != nil {
		panic(err)
	}
	return base
}

func TestWorktreeList(t *testing.T) {
	t.Parallel()

	t.Run("parses the porcelain worktree inventory in output order", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{stdout: worktreePorcelainFixture}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		entries, err := repository.WorktreeList(context.Background(), testIdentity())
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 4 {
			t.Fatalf("entry count = %d, want 4", len(entries))
		}
		primary := entries[0]
		if primary.Path != "C:/repo" ||
			primary.Head != "abc123def456abc123def456abc123def456abc1" ||
			primary.Branch != "develop" ||
			primary.Detached ||
			primary.Bare {
			t.Fatalf("primary entry = %#v", primary)
		}
		detached := entries[1]
		if detached.Path != "C:/repo-GOV-129" || !detached.Detached || detached.Branch != "" || detached.Bare {
			t.Fatalf("detached entry = %#v", detached)
		}
		linked := entries[2]
		if linked.Path != "C:/repo-GOV-130" || linked.Branch != "feature/GOV-130-add-export" || linked.Detached {
			t.Fatalf("linked entry = %#v", linked)
		}
		bare := entries[3]
		if bare.Path != "C:/bare" || !bare.Bare || bare.Branch != "" {
			t.Fatalf("bare entry = %#v", bare)
		}
		assertCall(t, runner.calls[0], "C:/repo", "", "worktree", "list", "--porcelain")
	})

	t.Run("keeps the worktree record open until the blank line", func(t *testing.T) {
		t.Parallel()
		entries := parseWorktreeList("worktree C:/repo\nHEAD abc\nbranch refs/heads/main")
		if len(entries) != 1 || entries[0].Branch != "main" || entries[0].Head != "abc" {
			t.Fatalf("entries without a trailing blank line = %#v", entries)
		}
	})

	t.Run("ignores unknown attributes and unattributed lines", func(t *testing.T) {
		t.Parallel()
		entries := parseWorktreeList("worktree C:/repo\nlocked\ncarried-flag value\nHEAD abc\n\nworktree C:/other\nHEAD def\n\n")
		if len(entries) != 2 || entries[0].Head != "abc" || entries[0].Branch != "" || entries[1].Head != "def" {
			t.Fatalf("unknown-attribute entries = %#v", entries)
		}
	})

	t.Run("maps a branch attribute without the refs prefix to no branch", func(t *testing.T) {
		t.Parallel()
		entries := parseWorktreeList("worktree C:/repo\nHEAD abc\nbranch (null)\n")
		if len(entries) != 1 || entries[0].Branch != "" || entries[0].Detached {
			t.Fatalf("unparsable branch attribute = %#v", entries)
		}
	})

	t.Run("returns an empty inventory without records", func(t *testing.T) {
		t.Parallel()
		if entries := parseWorktreeList(""); len(entries) != 0 {
			t.Fatalf("empty inventory = %#v", entries)
		}
	})

	t.Run("fails closed on a Git failure", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{err: errors.New("failed"), exitCode: 128}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		_, err := repository.WorktreeList(context.Background(), testIdentity())
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})
}

func TestWorktreeAddDetached(t *testing.T) {
	t.Parallel()

	t.Run("fetches the remote and creates the detached worktree", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{}, {}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		if err := repository.WorktreeAddDetached(context.Background(), testIdentity(), "C:/repo-GOV-129", mustWorktreeBase("origin", "develop")); err != nil {
			t.Fatal(err)
		}
		if len(runner.calls) != 2 {
			t.Fatalf("call count = %d, want 2", len(runner.calls))
		}
		assertCall(t, runner.calls[0], "C:/repo", "", "fetch", "--prune", "origin")
		assertCall(t, runner.calls[1], "C:/repo", "", "worktree", "add", "--detach", "C:/repo-GOV-129", "origin/develop")
	})

	t.Run("rejects a local target base", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{}
		repository := &Repository{runner: runner, timeout: time.Second}
		local, err := branch.NewLocalBase(mustWorktreeBranch("develop"))
		if err != nil {
			t.Fatal(err)
		}
		if err := repository.WorktreeAddDetached(context.Background(), testIdentity(), "C:/repo-GOV-129", local); err == nil {
			t.Fatal("a local target base must be rejected")
		} else {
			assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
		}
		if len(runner.calls) != 0 {
			t.Fatalf("a rejected base must not invoke Git: %v", runner.calls)
		}
	})

	t.Run("propagates a fetch failure", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{err: errors.New("fetch failed"), exitCode: 128}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		err := repository.WorktreeAddDetached(context.Background(), testIdentity(), "C:/repo-GOV-129", mustWorktreeBase("origin", "develop"))
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
		if len(runner.calls) != 1 {
			t.Fatalf("a failed fetch must not create the worktree: %v", runner.calls)
		}
	})

	t.Run("maps a worktree creation failure", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{}, {err: errors.New("add failed"), exitCode: 128}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		err := repository.WorktreeAddDetached(context.Background(), testIdentity(), "C:/repo-GOV-129", mustWorktreeBase("origin", "develop"))
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("maps an already-existing worktree path to the named conflict", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{}, {
			err:      errors.New("exit status 5"),
			stderr:   "Preparing worktree (detached HEAD c460158)\nfatal: 'C:/repo-GOV-129' already exists",
			exitCode: 128,
		}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		err := repository.WorktreeAddDetached(context.Background(), testIdentity(), "C:/repo-GOV-129", mustWorktreeBase("origin", "develop"))
		assertProblemCode(t, err, problem.CodeWorktreeConflict)
	})
}

func TestWorktreeHeadMatchesBase(t *testing.T) {
	t.Parallel()

	t.Run("resolves the checkout and base revisions as one evidence pair", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{
			{stdout: "c46015869552bc0433fa2a5276713d74bfc73f87\n"},
			{stdout: "c46015869552bc0433fa2a5276713d74bfc73f87\n"},
		}}
		repository := &Repository{runner: runner, timeout: time.Second}
		head, baseRevision, err := repository.WorktreeHeadMatchesBase(context.Background(), testIdentity(), mustWorktreeBase("origin", "develop"))
		if err != nil {
			t.Fatal(err)
		}
		if head != "c46015869552bc0433fa2a5276713d74bfc73f87" || baseRevision != head {
			t.Fatalf("WorktreeHeadMatchesBase() = (%q, %q)", head, baseRevision)
		}
		assertCall(t, runner.calls[0], "C:/repo", "", "rev-parse", "HEAD")
		assertCall(t, runner.calls[1], "C:/repo", "", "rev-parse", "origin/develop")
	})

	t.Run("reports a drifted pair without equalizing it", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{
			{stdout: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n"},
			{stdout: "c46015869552bc0433fa2a5276713d74bfc73f87\n"},
		}}
		repository := &Repository{runner: runner, timeout: time.Second}
		head, baseRevision, err := repository.WorktreeHeadMatchesBase(context.Background(), testIdentity(), mustWorktreeBase("origin", "develop"))
		if err != nil {
			t.Fatal(err)
		}
		if head == baseRevision {
			t.Fatalf("WorktreeHeadMatchesBase() = (%q, %q), want the drifted pair", head, baseRevision)
		}
	})

	t.Run("fails closed when the checkout revision cannot be resolved", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{err: errors.New("rev-parse failed"), exitCode: 128}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		if _, _, err := repository.WorktreeHeadMatchesBase(context.Background(), testIdentity(), mustWorktreeBase("origin", "develop")); err == nil {
			t.Fatal("a failed HEAD resolution must fail closed")
		} else {
			assertProblemCode(t, err, problem.CodeGitCommandFailed)
		}
	})

	t.Run("fails closed when the base revision cannot be resolved", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{
			{stdout: "c46015869552bc0433fa2a5276713d74bfc73f87\n"},
			{err: errors.New("rev-parse failed"), exitCode: 128},
		}}
		repository := &Repository{runner: runner, timeout: time.Second}
		if _, _, err := repository.WorktreeHeadMatchesBase(context.Background(), testIdentity(), mustWorktreeBase("origin", "develop")); err == nil {
			t.Fatal("a failed base resolution must fail closed")
		} else {
			assertProblemCode(t, err, problem.CodeGitCommandFailed)
		}
	})
}

func TestWorktreeRemove(t *testing.T) {
	t.Parallel()

	t.Run("removes a clean worktree", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{stdout: worktreePorcelainFixture}, {}, {}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		if err := repository.WorktreeRemove(context.Background(), testIdentity(), "C:/repo-GOV-129"); err != nil {
			t.Fatal(err)
		}
		if len(runner.calls) != 3 {
			t.Fatalf("call count = %d, want 3", len(runner.calls))
		}
		assertCall(t, runner.calls[0], "C:/repo", "", "worktree", "list", "--porcelain")
		assertCall(t, runner.calls[1], "C:/repo-GOV-129", "", "status", "--porcelain=v1", "--untracked-files=normal")
		assertCall(t, runner.calls[2], "C:/repo", "", "worktree", "remove", "C:/repo-GOV-129")
	})

	t.Run("removes the real registered path of a case-variant target", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{
			stdout: "worktree C:/repo\nbranch refs/heads/develop\n\nworktree C:/repo-gov-129\ndetached\n\n",
		}, {}, {}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		if err := repository.WorktreeRemove(context.Background(), testIdentity(), "C:/repo-GOV-129"); err != nil {
			t.Fatal(err)
		}
		assertCall(t, runner.calls[1], "C:/repo-gov-129", "", "status", "--porcelain=v1", "--untracked-files=normal")
		assertCall(t, runner.calls[2], "C:/repo", "", "worktree", "remove", "C:/repo-gov-129")
	})

	t.Run("fails closed on an unclean target worktree", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{stdout: worktreePorcelainFixture}, {stdout: " M file.txt\n"}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		err := repository.WorktreeRemove(context.Background(), testIdentity(), "C:/repo-GOV-129")
		assertProblemCode(t, err, problem.CodeWorktreeNotClean)
		if len(runner.calls) != 2 {
			t.Fatalf("an unclean worktree must never be removed: %v", runner.calls)
		}
	})

	t.Run("maps a status failure on the target worktree", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{stdout: worktreePorcelainFixture}, {err: errors.New("status failed"), exitCode: 128}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		err := repository.WorktreeRemove(context.Background(), testIdentity(), "C:/repo-GOV-129")
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("maps a removal failure", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{stdout: worktreePorcelainFixture}, {}, {err: errors.New("remove failed"), exitCode: 128}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		err := repository.WorktreeRemove(context.Background(), testIdentity(), "C:/repo-GOV-129")
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("fails closed when the worktree inventory cannot be read", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{err: errors.New("list failed"), exitCode: 128}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		err := repository.WorktreeRemove(context.Background(), testIdentity(), "C:/repo-GOV-129")
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
		if len(runner.calls) != 1 {
			t.Fatalf("an unreadable inventory must never touch a target: %v", runner.calls)
		}
	})
}

func TestLinkedWorktree(t *testing.T) {
	t.Parallel()

	t.Run("reports the primary checkout as not linked", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{stdout: ".git\n"}, {stdout: ".git\n"}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		linked, err := repository.LinkedWorktree(context.Background(), testIdentity())
		if err != nil {
			t.Fatal(err)
		}
		if linked {
			t.Fatal("the primary checkout must not be reported as a linked worktree")
		}
	})

	t.Run("reports a linked task worktree", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{stdout: "C:/repo/.git/worktrees/repo-GOV-129\n"}, {stdout: "C:/repo/.git\n"}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		linked, err := repository.LinkedWorktree(context.Background(), testIdentity())
		if err != nil {
			t.Fatal(err)
		}
		if !linked {
			t.Fatal("a worktree Git directory below the common directory must be reported as linked")
		}
	})

	t.Run("trims the process output before comparing", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{stdout: "  .git\n"}, {stdout: ".git  \n"}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		linked, err := repository.LinkedWorktree(context.Background(), testIdentity())
		if err != nil {
			t.Fatal(err)
		}
		if linked {
			t.Fatal("equal Git directories must not be reported as linked")
		}
	})

	t.Run("fails closed when the Git directory cannot be resolved", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{err: errors.New("failed"), exitCode: 128}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		_, err := repository.LinkedWorktree(context.Background(), testIdentity())
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
		if !strings.Contains(strings.Join(runner.calls[0].arguments, " "), "--git-dir") {
			t.Fatalf("first call must resolve the Git directory: %v", runner.calls)
		}
	})

	t.Run("fails closed when the common Git directory cannot be resolved", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{stdout: ".git\n"}, {err: errors.New("failed"), exitCode: 128}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		_, err := repository.LinkedWorktree(context.Background(), testIdentity())
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})
}

func TestWorktreeAdapterImplementsPort(t *testing.T) {
	t.Parallel()

	if _, ok := interface{}(&Repository{}).(port.WorktreeManager); !ok {
		t.Fatal("the Git CLI adapter must implement the WorktreeManager port")
	}
}
