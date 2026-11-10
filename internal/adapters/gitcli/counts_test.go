package gitcli

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestCountCommitsBehindBaseMeasuresTheCheckedOutLag(t *testing.T) {
	t.Parallel()

	develop, _ := branch.ParseName("develop")
	base, err := branch.NewTargetBase("origin", develop)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("current line", func(t *testing.T) {
		runner := &fakeRunner{results: []processResult{{stdout: "0\n"}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		actual, err := repository.CountCommitsBehindBase(context.Background(), testIdentity(), base)
		if err != nil || actual != 0 {
			t.Fatalf("CountCommitsBehindBase() = (%d, %v)", actual, err)
		}
		assertCall(t, runner.calls[0], "C:/repo", "", "rev-list", "--count", "HEAD..origin/develop")
	})

	t.Run("lagging line", func(t *testing.T) {
		repository := &Repository{runner: &fakeRunner{results: []processResult{{stdout: "7\n"}}}, timeout: time.Second}
		actual, err := repository.CountCommitsBehindBase(context.Background(), testIdentity(), base)
		if err != nil || actual != 7 {
			t.Fatalf("CountCommitsBehindBase() = (%d, %v)", actual, err)
		}
	})

	t.Run("malformed count", func(t *testing.T) {
		repository := &Repository{runner: &fakeRunner{results: []processResult{{stdout: "not-a-number"}}}, timeout: time.Second}
		_, err := repository.CountCommitsBehindBase(context.Background(), testIdentity(), base)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})

	t.Run("command failure", func(t *testing.T) {
		repository := &Repository{
			runner:  &fakeRunner{results: []processResult{{err: errors.New("rev-list failed"), exitCode: 128}}},
			timeout: time.Second,
		}
		_, err := repository.CountCommitsBehindBase(context.Background(), testIdentity(), base)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})
}
