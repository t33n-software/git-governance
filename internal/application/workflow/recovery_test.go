package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// recoveryRefsGit carries only the reference-inspection capability.
type recoveryRefsGit struct{ fakeGitRepository }

func (fake *recoveryRefsGit) HasAnyRef(context.Context, port.RepositoryIdentity) (bool, error) {
	return false, nil
}

func TestRecoverUnbornCapabilityResolution(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		git  port.GitRepository
	}{
		{name: "reference inspection missing", git: &fakeGitRepository{}},
		{name: "unborn-state restoration missing", git: &recoveryRefsGit{}},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := newBootstrapService(testCase.git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{
				Repository: testRepository(),
			})
			assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
		})
	}
}

func TestRecoverUnbornRefusals(t *testing.T) {
	t.Parallel()

	t.Run("guards dependencies and inputs", func(t *testing.T) {
		t.Parallel()
		if _, err := NewBootstrapService(nil, nil, nil, nil).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()}); err == nil {
			t.Fatal("expected the missing-dependency guard")
		}
		if _, err := newBootstrapService(newBootstrapGit(), newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{}); err == nil {
			t.Fatal("expected the repository guard")
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := newBootstrapService(newBootstrapGit(), newBootstrapTools()).RecoverUnborn(ctx, RecoverUnbornRequest{Repository: testRepository()}); err == nil {
			t.Fatal("expected the cancelled context guard")
		}
	})

	t.Run("a born repository is never recovered", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.hasCommits = true
		_, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeRepositoryAlreadyBorn)
		if git.restored {
			t.Fatal("the refused recovery restored a state")
		}
	})

	t.Run("the unborn HEAD must target main", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.current = "develop"
		_, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeUnbornHeadMismatch)
	})

	t.Run("a running git operation blocks the recovery", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.active = true
		git.activeOperation = "rebase"
		_, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeOperationInProgress)
	})
}

func TestRecoverUnbornProofErrorPropagation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		mutate func(git *bootstrapGit)
	}{
		{name: "has commits", mutate: func(git *bootstrapGit) { git.err = errors.New("git failed") }},
		{name: "current branch", mutate: func(git *bootstrapGit) { git.currentErr = errors.New("git failed") }},
		{name: "active operation", mutate: func(git *bootstrapGit) { git.activeErr = errors.New("git failed") }},
		{name: "staged changes", mutate: func(git *bootstrapGit) {
			git.stagedQueue = []bool{false}
			git.stagedErrs = []error{errors.New("git failed")}
		}},
		{name: "reference inspection", mutate: func(git *bootstrapGit) { git.anyRefErr = errors.New("git failed") }},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			git := newBootstrapGit()
			testCase.mutate(git)
			if _, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()}); err == nil {
				t.Fatal("expected the proof error to propagate")
			}
			if git.restored {
				t.Fatal("the failed proof restored a state")
			}
		})
	}
}

func TestRecoverUnbornExecution(t *testing.T) {
	t.Parallel()

	t.Run("the dry run proves the state without restoring it", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.anyRef = true
		git.stagedQueue = []bool{true}
		result, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{
			Repository: testRepository(),
			DryRun:     true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !result.DryRun || git.restored {
			t.Fatalf("the dry run mutated the repository: result = %#v, restored = %t", result, git.restored)
		}
		actions := make([]string, 0, len(result.Plan))
		for _, step := range result.Plan {
			actions = append(actions, step.Action)
		}
		if strings.Join(actions, ",") != "prove-unborn,empty-index,remove-references,prove-pre-state" {
			t.Fatalf("dry-run plan = %q", strings.Join(actions, ","))
		}
	})

	t.Run("a clean unborn repository recovers idempotently", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		result, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		if !git.restored || result.IndexEmptied || result.ReferencesRemoved {
			t.Fatalf("the clean recovery = %#v, restored = %t", result, git.restored)
		}
	})

	t.Run("a foreign pre-staging state is restored and proven", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.anyRefQueue = []bool{true, false}
		git.stagedQueue = []bool{true, false}
		result, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		if !git.restored || !result.IndexEmptied || !result.ReferencesRemoved {
			t.Fatalf("the recovery result = %#v, restored = %t", result, git.restored)
		}
		actions := make([]string, 0, len(result.Plan))
		for _, step := range result.Plan {
			actions = append(actions, step.Action)
		}
		if strings.Join(actions, ",") != "prove-unborn,empty-index,remove-references,prove-pre-state" {
			t.Fatalf("recovery plan = %q", strings.Join(actions, ","))
		}
	})

	t.Run("a restoration failure propagates", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.stagedQueue = []bool{true}
		git.restoreErr = errors.New("index.lock persists")
		if _, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()}); err == nil {
			t.Fatal("expected the restoration failure to propagate")
		}
	})

	t.Run("a staged read-back blocks the retry", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.stagedQueue = []bool{true, true}
		_, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeInternal)
		typed, ok := problem.As(err)
		if !ok || typed.Field != "recovery read-back" {
			t.Fatalf("the read-back failure lost its blocking record: %v", err)
		}
	})

	t.Run("a reference read-back blocks the retry", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.stagedQueue = []bool{false, false}
		git.anyRef = true
		_, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("a read-back error propagates", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.stagedQueue = []bool{false, false}
		git.stagedErrs = []error{nil, errors.New("git failed")}
		_, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeInternal)
		typed, ok := problem.As(err)
		if !ok || typed.Field != "recovery read-back" {
			t.Fatalf("the read-back failure lost its blocking record: %v", err)
		}
		if !git.restored {
			t.Fatal("the restoration never ran")
		}
	})

	t.Run("a reference read-back error propagates", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.stagedQueue = []bool{false, false}
		git.anyRefQueue = []bool{false}
		git.anyRefErrs = []error{nil}
		git.anyRefErr = errors.New("git failed")
		_, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeInternal)
		typed, ok := problem.As(err)
		if !ok || typed.Field != "recovery read-back" {
			t.Fatalf("the read-back failure lost its blocking record: %v", err)
		}
		if !git.restored {
			t.Fatal("the restoration never ran")
		}
	})
}
