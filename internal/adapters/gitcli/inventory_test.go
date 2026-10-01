package gitcli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestRemoteBranches(t *testing.T) {
	t.Parallel()

	t.Run("enumerates the selected remote's canonical branches deterministically", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{stdout: strings.Join([]string{
			"refs/remotes/origin/HEAD",
			"refs/remotes/origin/release/1.1.0",
			"refs/remotes/origin/feature/ABC-10-export",
			"refs/remotes/origin/feature/ABC-2-button",
			"refs/remotes/origin/not-a-canonical-family/odd",
		}, "\n")}}}
		repository := &Repository{runner: runner, timeout: time.Second}

		branches, err := repository.RemoteBranches(context.Background(), testIdentity())
		if err != nil {
			t.Fatal(err)
		}
		names := make([]string, 0, len(branches))
		for _, name := range branches {
			names = append(names, name.String())
		}
		want := "feature/ABC-10-export,feature/ABC-2-button,release/1.1.0"
		if strings.Join(names, ",") != want {
			t.Fatalf("RemoteBranches() = %q, want %q", strings.Join(names, ","), want)
		}
		assertCall(t, runner.calls[0], testIdentity().Root, "", "for-each-ref", "--format=%(refname)", "refs/remotes/origin/")
	})

	t.Run("an empty remote-tracking surface is a valid empty result", func(t *testing.T) {
		t.Parallel()
		repository := &Repository{runner: &fakeRunner{results: []processResult{{}}}, timeout: time.Second}
		branches, err := repository.RemoteBranches(context.Background(), testIdentity())
		if err != nil || len(branches) != 0 {
			t.Fatalf("RemoteBranches() = (%#v, %v)", branches, err)
		}
	})

	t.Run("a failed listing is a typed Git failure", func(t *testing.T) {
		t.Parallel()
		repository := &Repository{
			runner:  &fakeRunner{results: []processResult{{err: errors.New("failed"), exitCode: 128}}},
			timeout: time.Second,
		}
		_, err := repository.RemoteBranches(context.Background(), testIdentity())
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("collapses duplicate remote-tracking refs deterministically", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{stdout: strings.Join([]string{
			"refs/remotes/origin/feature/ABC-10-export",
			"refs/remotes/origin/feature/ABC-10-export",
			"refs/remotes/origin/feature/ABC-10-export",
		}, "\n")}}}
		repository := &Repository{runner: runner, timeout: time.Second}

		branches, err := repository.RemoteBranches(context.Background(), testIdentity())
		if err != nil {
			t.Fatal(err)
		}
		if len(branches) != 1 || branches[0].String() != "feature/ABC-10-export" {
			t.Fatalf("RemoteBranches() = %#v", branches)
		}
	})
}

func TestCommitSubjects(t *testing.T) {
	t.Parallel()

	t.Run("reads the complete subject history across all refs", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{stdout: "\x1efeat(ABC-2): add export button\x00\x1echore(deps): bump actions\x00\x1eunstructured legacy subject\x00"}}}
		repository := &Repository{runner: runner, timeout: time.Second}

		subjects, err := repository.CommitSubjects(context.Background(), testIdentity())
		if err != nil {
			t.Fatal(err)
		}
		want := "feat(ABC-2): add export button|chore(deps): bump actions|unstructured legacy subject"
		if strings.Join(subjects, "|") != want {
			t.Fatalf("CommitSubjects() = %q, want %q", strings.Join(subjects, "|"), want)
		}
		assertCall(t, runner.calls[0], testIdentity().Root, "", "log", "--all", "--format=%x1e%s%x00")
	})

	t.Run("an unborn repository carries no subject history", func(t *testing.T) {
		t.Parallel()
		repository := &Repository{runner: &fakeRunner{results: []processResult{{}}}, timeout: time.Second}
		subjects, err := repository.CommitSubjects(context.Background(), testIdentity())
		if err != nil || len(subjects) != 0 {
			t.Fatalf("CommitSubjects() = (%#v, %v)", subjects, err)
		}
	})

	t.Run("a failed history read is a typed Git failure", func(t *testing.T) {
		t.Parallel()
		repository := &Repository{
			runner:  &fakeRunner{results: []processResult{{err: errors.New("failed"), exitCode: 128}}},
			timeout: time.Second,
		}
		_, err := repository.CommitSubjects(context.Background(), testIdentity())
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})
}
