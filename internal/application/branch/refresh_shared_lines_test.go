package branchapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/port"
	domainbranch "github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// refreshSharedLinesGit extends the shared fake with the three capabilities
// the refresh use case consumes: local branch listing, checked-out
// fast-forward, and reference fast-forward.
type refreshSharedLinesGit struct {
	*fakeGitRepository
	current          domainbranch.BranchName
	currentErr       error
	existsByName     map[string]bool
	existsErr        error
	localList        []domainbranch.BranchName
	localListErr     error
	checkoutOutcome  port.FastForwardOutcome
	checkoutErr      error
	checkoutCalls    []string
	referenceOutcome port.FastForwardOutcome
	referenceErr     error
	referenceCalls   []string
}

func newRefreshSharedLinesGit() *refreshSharedLinesGit {
	return &refreshSharedLinesGit{
		fakeGitRepository: &fakeGitRepository{clean: true},
		current:           mustBranch("feature/ABC-123-add-export"),
	}
}

func (git *refreshSharedLinesGit) CurrentBranch(context.Context, port.RepositoryIdentity) (domainbranch.BranchName, error) {
	git.calls = append(git.calls, "current-branch")
	return git.current, git.currentErr
}

func (git *refreshSharedLinesGit) BranchExists(_ context.Context, _ port.RepositoryIdentity, name domainbranch.BranchName) (bool, error) {
	git.calls = append(git.calls, "branch-exists")
	if git.existsErr != nil {
		return false, git.existsErr
	}
	return git.existsByName[name.String()], nil
}

func (git *refreshSharedLinesGit) LocalBranches(context.Context, port.RepositoryIdentity) ([]domainbranch.BranchName, error) {
	git.calls = append(git.calls, "local-branches")
	return append([]domainbranch.BranchName(nil), git.localList...), git.localListErr
}

func (git *refreshSharedLinesGit) FastForwardBranch(_ context.Context, _ port.RepositoryIdentity, name domainbranch.BranchName, _ domainbranch.TargetBase) (port.FastForwardOutcome, error) {
	git.calls = append(git.calls, "fast-forward")
	git.checkoutCalls = append(git.checkoutCalls, name.String())
	return git.checkoutOutcome, git.checkoutErr
}

func (git *refreshSharedLinesGit) FastForwardBranchReference(_ context.Context, _ port.RepositoryIdentity, name domainbranch.BranchName, _ domainbranch.TargetBase) (port.FastForwardOutcome, error) {
	git.calls = append(git.calls, "fast-forward-reference")
	git.referenceCalls = append(git.referenceCalls, name.String())
	return git.referenceOutcome, git.referenceErr
}

// refreshWithoutCheckedOutPortGit lacks the checked-out fast-forward
// capability while offering listing and reference refresh.
type refreshWithoutCheckedOutPortGit struct {
	*fakeGitRepository
	localList []domainbranch.BranchName
}

func (git *refreshWithoutCheckedOutPortGit) CurrentBranch(context.Context, port.RepositoryIdentity) (domainbranch.BranchName, error) {
	return mustBranch("develop"), nil
}

func (git *refreshWithoutCheckedOutPortGit) LocalBranches(context.Context, port.RepositoryIdentity) ([]domainbranch.BranchName, error) {
	return append([]domainbranch.BranchName(nil), git.localList...), nil
}

func (git *refreshWithoutCheckedOutPortGit) FastForwardBranchReference(context.Context, port.RepositoryIdentity, domainbranch.BranchName, domainbranch.TargetBase) (port.FastForwardOutcome, error) {
	return port.FastForwardUpdated, nil
}

// refreshWithoutReferencePortGit lacks the reference fast-forward capability
// while offering listing and checked-out refresh.
type refreshWithoutReferencePortGit struct {
	*fakeGitRepository
	localList []domainbranch.BranchName
}

func (git *refreshWithoutReferencePortGit) LocalBranches(context.Context, port.RepositoryIdentity) ([]domainbranch.BranchName, error) {
	return append([]domainbranch.BranchName(nil), git.localList...), nil
}

func (git *refreshWithoutReferencePortGit) FastForwardBranch(context.Context, port.RepositoryIdentity, domainbranch.BranchName, domainbranch.TargetBase) (port.FastForwardOutcome, error) {
	return port.FastForwardUpdated, nil
}

func TestRefreshSharedLinesInputGuards(t *testing.T) {
	t.Parallel()

	t.Run("requires a repository and an active context", func(t *testing.T) {
		t.Parallel()
		_, err := NewSharedLineRefresher(&fakeGitRepository{}).Refresh(context.Background(), RefreshSharedLinesRequest{})
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = NewSharedLineRefresher(&fakeGitRepository{}).Refresh(ctx, RefreshSharedLinesRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeOperationCancelled)

		_, err = NewSharedLineRefresher(nil).Refresh(context.Background(), RefreshSharedLinesRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("requires the local branch lister for the default selection", func(t *testing.T) {
		t.Parallel()
		_, err := NewSharedLineRefresher(&fakeGitRepository{}).Refresh(context.Background(), RefreshSharedLinesRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("propagates a local branch listing failure", func(t *testing.T) {
		t.Parallel()
		listErr := errors.New("listing failed")
		git := newRefreshSharedLinesGit()
		git.localListErr = listErr
		_, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{Repository: testRepository()})
		if !errors.Is(err, listErr) {
			t.Fatalf("Refresh() error = %v, want %v", err, listErr)
		}
	})

	t.Run("rejects a non-shared explicit line before any Git call", func(t *testing.T) {
		t.Parallel()
		git := newRefreshSharedLinesGit()
		_, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{
			Repository: testRepository(),
			Lines:      []domainbranch.BranchName{mustBranch("feature/ABC-123-add-export")},
		})
		assertProblemCode(t, err, problem.CodeBranchFamilyInvalid)
		if len(git.calls) != 0 {
			t.Fatalf("a non-shared line must stop before Git calls: %v", git.calls)
		}
	})

	t.Run("rejects an explicit line that is not present locally", func(t *testing.T) {
		t.Parallel()
		git := newRefreshSharedLinesGit()
		git.existsByName = map[string]bool{"develop": false}
		_, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{
			Repository: testRepository(),
			Lines:      []domainbranch.BranchName{mustBranch("develop")},
		})
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})

	t.Run("propagates a branch existence failure", func(t *testing.T) {
		t.Parallel()
		existsErr := errors.New("existence check failed")
		git := newRefreshSharedLinesGit()
		git.existsErr = existsErr
		_, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{
			Repository: testRepository(),
			Lines:      []domainbranch.BranchName{mustBranch("develop")},
		})
		if !errors.Is(err, existsErr) {
			t.Fatalf("Refresh() error = %v, want %v", err, existsErr)
		}
	})

	t.Run("rejects an invalid remote name at base construction", func(t *testing.T) {
		t.Parallel()
		git := newRefreshSharedLinesGit()
		git.existsByName = map[string]bool{"develop": true}
		repository := testRepository()
		repository.Remote = "bad/ref"
		_, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{
			Repository: repository,
			Lines:      []domainbranch.BranchName{mustBranch("develop")},
		})
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})

	t.Run("deduplicates an explicit selection", func(t *testing.T) {
		t.Parallel()
		git := newRefreshSharedLinesGit()
		git.existsByName = map[string]bool{"develop": true}
		result, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{
			Repository: testRepository(),
			Lines:      []domainbranch.BranchName{mustBranch("develop"), mustBranch("develop")},
			DryRun:     true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Plan) != 1 {
			t.Fatalf("duplicated selection produced %d plan steps, want 1", len(result.Plan))
		}
	})
}

func TestRefreshSharedLinesDryRun(t *testing.T) {
	t.Parallel()

	t.Run("plans without fetching or mutating", func(t *testing.T) {
		t.Parallel()
		git := newRefreshSharedLinesGit()
		git.localList = []domainbranch.BranchName{
			mustBranch("develop"),
			mustBranch("feature/ABC-123-add-export"),
			mustBranch("main"),
		}
		result, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{
			Repository: testRepository(),
			DryRun:     true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !result.DryRun || result.Fetched {
			t.Fatalf("dry-run result = %#v", result)
		}
		if len(result.Plan) != 2 || result.Plan[0].Action != "refresh" || result.Plan[0].Detail != "develop to origin/develop" ||
			result.Plan[1].Detail != "main to origin/main" {
			t.Fatalf("dry-run plan = %#v", result.Plan)
		}
		calls := strings.Join(git.calls, ",")
		if strings.Contains(calls, "fetch") || strings.Contains(calls, "fast-forward") {
			t.Fatalf("dry-run fetched or mutated: %v", git.calls)
		}
		if !strings.Contains(calls, "target-base-exists") {
			t.Fatalf("dry-run must validate the bases against the last fetched state: %v", git.calls)
		}
	})

	t.Run("rejects a missing base against the last fetched state", func(t *testing.T) {
		t.Parallel()
		git := newRefreshSharedLinesGit()
		git.localList = []domainbranch.BranchName{mustBranch("develop")}
		git.targetBaseMissing = true
		_, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{
			Repository: testRepository(),
			DryRun:     true,
		})
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})

	t.Run("an empty local shared-line set is a no-op without a fetch", func(t *testing.T) {
		t.Parallel()
		git := newRefreshSharedLinesGit()
		git.localList = []domainbranch.BranchName{mustBranch("feature/ABC-123-add-export")}
		for _, dryRun := range []bool{true, false} {
			result, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{
				Repository: testRepository(),
				DryRun:     dryRun,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Lines) != 0 || len(result.Plan) != 0 || result.Fetched {
				t.Fatalf("empty target set result = %#v", result)
			}
		}
		if strings.Contains(strings.Join(git.calls, ","), "fetch") {
			t.Fatalf("an empty target set must never fetch: %v", git.calls)
		}
	})
}

func TestRefreshSharedLinesExecution(t *testing.T) {
	t.Parallel()

	t.Run("propagates a fetch failure", func(t *testing.T) {
		t.Parallel()
		fetchErr := errors.New("fetch failed")
		git := newRefreshSharedLinesGit()
		git.localList = []domainbranch.BranchName{mustBranch("develop")}
		git.fetchErr = fetchErr
		_, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{Repository: testRepository()})
		if !errors.Is(err, fetchErr) {
			t.Fatalf("Refresh() error = %v, want %v", err, fetchErr)
		}
	})

	t.Run("rejects a missing base after the fetch", func(t *testing.T) {
		t.Parallel()
		git := newRefreshSharedLinesGit()
		git.localList = []domainbranch.BranchName{mustBranch("develop")}
		git.targetBaseMissing = true
		_, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})

	t.Run("propagates a base availability failure", func(t *testing.T) {
		t.Parallel()
		baseErr := errors.New("base check failed")
		git := newRefreshSharedLinesGit()
		git.localList = []domainbranch.BranchName{mustBranch("develop")}
		git.targetBaseErr = baseErr
		_, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{Repository: testRepository()})
		if !errors.Is(err, baseErr) {
			t.Fatalf("Refresh() error = %v, want %v", err, baseErr)
		}
	})

	t.Run("rejects an in-progress Git operation before any refresh", func(t *testing.T) {
		t.Parallel()
		git := newRefreshSharedLinesGit()
		git.localList = []domainbranch.BranchName{mustBranch("develop")}
		git.active = true
		git.activeOperation = "merge"
		_, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeOperationInProgress)
		if len(git.checkoutCalls) != 0 || len(git.referenceCalls) != 0 {
			t.Fatalf("an active operation must block every refresh: %v %v", git.checkoutCalls, git.referenceCalls)
		}
	})

	t.Run("propagates an operation inspection failure", func(t *testing.T) {
		t.Parallel()
		operationErr := errors.New("operation inspection failed")
		git := newRefreshSharedLinesGit()
		git.localList = []domainbranch.BranchName{mustBranch("develop")}
		git.activeOperationErr = operationErr
		_, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{Repository: testRepository()})
		if !errors.Is(err, operationErr) {
			t.Fatalf("Refresh() error = %v, want %v", err, operationErr)
		}
	})

	t.Run("propagates a current branch failure", func(t *testing.T) {
		t.Parallel()
		currentErr := errors.New("current branch failed")
		git := newRefreshSharedLinesGit()
		git.localList = []domainbranch.BranchName{mustBranch("develop")}
		git.currentErr = currentErr
		_, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{Repository: testRepository()})
		if !errors.Is(err, currentErr) {
			t.Fatalf("Refresh() error = %v, want %v", err, currentErr)
		}
	})

	t.Run("rejects a dirty worktree when the checked-out line is a target", func(t *testing.T) {
		t.Parallel()
		git := newRefreshSharedLinesGit()
		git.current = mustBranch("develop")
		git.clean = false
		git.localList = []domainbranch.BranchName{mustBranch("develop")}
		_, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeWorktreeNotClean)
		if len(git.checkoutCalls) != 0 {
			t.Fatalf("a dirty worktree must never fast-forward: %v", git.checkoutCalls)
		}
	})

	t.Run("propagates a worktree inspection failure", func(t *testing.T) {
		t.Parallel()
		worktreeErr := errors.New("worktree inspection failed")
		git := newRefreshSharedLinesGit()
		git.current = mustBranch("develop")
		git.worktreeCleanErr = worktreeErr
		git.localList = []domainbranch.BranchName{mustBranch("develop")}
		_, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{Repository: testRepository()})
		if !errors.Is(err, worktreeErr) {
			t.Fatalf("Refresh() error = %v, want %v", err, worktreeErr)
		}
	})

	t.Run("requires the checked-out capability when the current line is a target", func(t *testing.T) {
		t.Parallel()
		git := &refreshWithoutCheckedOutPortGit{
			fakeGitRepository: &fakeGitRepository{clean: true},
			localList:         []domainbranch.BranchName{mustBranch("develop")},
		}
		git.fakeGitRepository.err = nil
		_, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("requires the reference capability when a non-checked-out line is a target", func(t *testing.T) {
		t.Parallel()
		git := &refreshWithoutReferencePortGit{
			fakeGitRepository: &fakeGitRepository{clean: true},
			localList:         []domainbranch.BranchName{mustBranch("develop"), mustBranch("main")},
		}
		_, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("refreshes the checked-out line and a reference line together", func(t *testing.T) {
		t.Parallel()
		git := newRefreshSharedLinesGit()
		git.current = mustBranch("develop")
		git.localList = []domainbranch.BranchName{mustBranch("develop"), mustBranch("main")}
		git.checkoutOutcome = port.FastForwardUpdated
		git.referenceOutcome = port.FastForwardUpdated
		result, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		if !result.Fetched || len(result.Lines) != 2 {
			t.Fatalf("Refresh() = %#v", result)
		}
		if result.Lines[0].Outcome != port.FastForwardUpdated || result.Lines[1].Outcome != port.FastForwardUpdated {
			t.Fatalf("outcomes = %#v", result.Lines)
		}
		if len(git.checkoutCalls) != 1 || git.checkoutCalls[0] != "develop" {
			t.Fatalf("checked-out calls = %v", git.checkoutCalls)
		}
		if len(git.referenceCalls) != 1 || git.referenceCalls[0] != "main" {
			t.Fatalf("reference calls = %v", git.referenceCalls)
		}
	})

	t.Run("reports already-current lines without mutation semantics", func(t *testing.T) {
		t.Parallel()
		git := newRefreshSharedLinesGit()
		git.current = mustBranch("develop")
		git.localList = []domainbranch.BranchName{mustBranch("develop"), mustBranch("main")}
		git.checkoutOutcome = port.FastForwardAlreadyCurrent
		git.referenceOutcome = port.FastForwardAlreadyCurrent
		result, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range result.Lines {
			if line.Outcome != port.FastForwardAlreadyCurrent {
				t.Fatalf("outcome = %q, want already-current", line.Outcome)
			}
		}
	})

	t.Run("propagates a checked-out refresh failure", func(t *testing.T) {
		t.Parallel()
		refreshErr := errors.New("merge refused")
		git := newRefreshSharedLinesGit()
		git.current = mustBranch("develop")
		git.localList = []domainbranch.BranchName{mustBranch("develop")}
		git.checkoutErr = refreshErr
		_, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{Repository: testRepository()})
		if !errors.Is(err, refreshErr) {
			t.Fatalf("Refresh() error = %v, want %v", err, refreshErr)
		}
	})

	t.Run("propagates a reference refresh failure", func(t *testing.T) {
		t.Parallel()
		refreshErr := errors.New("update-ref refused")
		git := newRefreshSharedLinesGit()
		git.localList = []domainbranch.BranchName{mustBranch("main")}
		git.referenceErr = refreshErr
		_, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{Repository: testRepository()})
		if !errors.Is(err, refreshErr) {
			t.Fatalf("Refresh() error = %v, want %v", err, refreshErr)
		}
	})

	t.Run("fails closed on a diverged line and reports the partial stand", func(t *testing.T) {
		t.Parallel()
		git := newRefreshSharedLinesGit()
		git.current = mustBranch("develop")
		git.localList = []domainbranch.BranchName{mustBranch("develop"), mustBranch("main")}
		git.checkoutOutcome = port.FastForwardDiverged
		git.referenceOutcome = port.FastForwardUpdated
		result, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeSharedLineDiverged)
		value, _ := problem.As(err)
		if value.Actual != "develop" || !strings.Contains(value.Context, "main") {
			t.Fatalf("diverged problem = %#v", value)
		}
		if len(result.Lines) != 2 {
			t.Fatalf("the partial stand must stay observable: %#v", result.Lines)
		}
	})

	t.Run("a solely diverged line carries no refreshed context", func(t *testing.T) {
		t.Parallel()
		git := newRefreshSharedLinesGit()
		git.current = mustBranch("develop")
		git.localList = []domainbranch.BranchName{mustBranch("develop")}
		git.checkoutOutcome = port.FastForwardDiverged
		_, err := NewSharedLineRefresher(git).Refresh(context.Background(), RefreshSharedLinesRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeSharedLineDiverged)
		value, _ := problem.As(err)
		if value.Context != "" {
			t.Fatalf("unexpected refreshed context: %q", value.Context)
		}
	})
}
