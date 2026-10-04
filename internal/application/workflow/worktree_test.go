package workflow

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// fakeWorktreeGit extends the shared workflow fake with the optional
// worktree-management capability and records every worktree mutation.
type fakeWorktreeGit struct {
	*fakeGitRepository
	entries      []port.WorktreeEntry
	listErr      error
	addErr       error
	removeErr    error
	linked       bool
	linkedErr    error
	head         string
	baseRevision string
	headBaseErr  error
	addedPaths   []string
	addedBases   []branch.TargetBase
	removedPaths []string
}

func (fake *fakeWorktreeGit) WorktreeList(context.Context, port.RepositoryIdentity) ([]port.WorktreeEntry, error) {
	fake.calls = append(fake.calls, "worktree-list")
	if fake.listErr != nil {
		return nil, fake.listErr
	}
	return fake.entries, nil
}

func (fake *fakeWorktreeGit) WorktreeAddDetached(_ context.Context, _ port.RepositoryIdentity, path string, base branch.TargetBase) error {
	fake.calls = append(fake.calls, "worktree-add-detached")
	fake.addedPaths = append(fake.addedPaths, path)
	fake.addedBases = append(fake.addedBases, base)
	return fake.addErr
}

func (fake *fakeWorktreeGit) WorktreeRemove(_ context.Context, _ port.RepositoryIdentity, path string) error {
	fake.calls = append(fake.calls, "worktree-remove")
	fake.removedPaths = append(fake.removedPaths, path)
	return fake.removeErr
}

func (fake *fakeWorktreeGit) LinkedWorktree(context.Context, port.RepositoryIdentity) (bool, error) {
	fake.calls = append(fake.calls, "linked-worktree")
	return fake.linked, fake.linkedErr
}

func (fake *fakeWorktreeGit) WorktreeHeadMatchesBase(_ context.Context, _ port.RepositoryIdentity, _ branch.TargetBase) (string, string, error) {
	fake.calls = append(fake.calls, "worktree-head-matches-base")
	if fake.headBaseErr != nil {
		return "", "", fake.headBaseErr
	}
	return fake.head, fake.baseRevision, nil
}

// detachedTaskWorktreeGit reports the detached-HEAD classification the Git CLI
// adapter produces for a worktree without a checked-out branch.
type detachedTaskWorktreeGit struct {
	*fakeWorktreeGit
}

func (git *detachedTaskWorktreeGit) CurrentBranch(context.Context, port.RepositoryIdentity) (branch.BranchName, error) {
	git.calls = append(git.calls, "current-branch")
	return branch.BranchName{}, detachedCurrentBranchProblem()
}

// detachedPrimaryGit reports a detached HEAD while carrying no worktree
// capability, modeling a composition without the optional port.
type detachedPrimaryGit struct {
	*fakeGitRepository
}

func (git *detachedPrimaryGit) CurrentBranch(context.Context, port.RepositoryIdentity) (branch.BranchName, error) {
	git.calls = append(git.calls, "current-branch")
	return branch.BranchName{}, detachedCurrentBranchProblem()
}

func detachedCurrentBranchProblem() error {
	return problem.New(problem.Details{
		Code:        problem.CodeBranchNameInvalid,
		Category:    problem.CategoryRepository,
		Field:       "current branch",
		Expected:    "a checked-out canonical branch",
		Rule:        "detached HEAD cannot satisfy governed branch workflows",
		Remediation: "switch to a canonical branch before continuing",
	})
}

// worktreeFormGit overrides the worktree-form inspection of the
// integration-line transition fake while carrying the complete worktree
// capability.
type worktreeFormGit struct {
	*fakeWorktreeGit
	current     branch.BranchName
	currentErr  error
	worktreeErr error
	switchErr   error
	switchedTo  []branch.BranchName
}

func (git *worktreeFormGit) CurrentBranch(context.Context, port.RepositoryIdentity) (branch.BranchName, error) {
	git.calls = append(git.calls, "current-branch")
	return git.current, git.currentErr
}

func (git *worktreeFormGit) IsWorktreeClean(context.Context, port.RepositoryIdentity) (bool, error) {
	git.calls = append(git.calls, "worktree-clean")
	return git.clean, git.worktreeErr
}

func (git *worktreeFormGit) SwitchBranch(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName) error {
	git.calls = append(git.calls, "switch")
	git.switchedTo = append(git.switchedTo, name)
	return git.switchErr
}

func TestStartWorktree(t *testing.T) {
	t.Parallel()

	t.Run("fetches and creates the detached worktree from the remote develop revision", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{}}
		service := NewWorktreeService(git)
		result, err := service.StartWorktree(context.Background(), StartWorktreeRequest{
			Repository: testRepository(),
			Ticket:     mustTicket("GOV-129"),
		})
		if err != nil {
			t.Fatal(err)
		}
		expectedPath := filepath.Join(filepath.Dir(testRepository().Root), "repo-GOV-129")
		if result.Path != expectedPath || result.Base.String() != "origin/develop" || result.DryRun {
			t.Fatalf("StartWorktree() = %#v", result)
		}
		if result.Ticket.String() != "GOV-129" || len(result.Plan) != 2 {
			t.Fatalf("StartWorktree() = %#v", result)
		}
		if len(git.addedPaths) != 1 || git.addedPaths[0] != expectedPath || git.addedBases[0].String() != "origin/develop" {
			t.Fatalf("worktree additions = %v with bases %v", git.addedPaths, git.addedBases)
		}
		if countCall(git.calls, "linked-worktree") != 1 {
			t.Fatalf("the acquisition must verify the primary-checkout form: %v", git.calls)
		}
	})

	t.Run("plans the acquisition without mutating during dry-run", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{}}
		service := NewWorktreeService(git)
		result, err := service.StartWorktree(context.Background(), StartWorktreeRequest{
			Repository: testRepository(),
			Ticket:     mustTicket("GOV-129"),
			DryRun:     true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !result.DryRun || len(result.Plan) != 2 || result.Plan[0].Action != "fetch" || result.Plan[1].Action != "worktree-add" {
			t.Fatalf("dry-run result = %#v", result)
		}
		if len(git.addedPaths) != 0 {
			t.Fatalf("a dry run must not create the worktree: %v", git.addedPaths)
		}
	})

	t.Run("rejects a missing ticket before any Git interaction", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{}}
		service := NewWorktreeService(git)
		_, err := service.StartWorktree(context.Background(), StartWorktreeRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeInvalidInput)
		if len(git.calls) != 0 {
			t.Fatalf("a rejected request must not invoke Git: %v", git.calls)
		}
	})

	t.Run("rejects a nested acquisition from inside a linked worktree", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{}, linked: true}
		service := NewWorktreeService(git)
		_, err := service.StartWorktree(context.Background(), StartWorktreeRequest{
			Repository: testRepository(),
			Ticket:     mustTicket("GOV-129"),
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)
		if len(git.addedPaths) != 0 {
			t.Fatalf("a nested acquisition must not create a worktree: %v", git.addedPaths)
		}
	})

	t.Run("propagates a worktree-form inspection failure", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{}, linkedErr: errors.New("inspection failed")}
		service := NewWorktreeService(git)
		_, err := service.StartWorktree(context.Background(), StartWorktreeRequest{
			Repository: testRepository(),
			Ticket:     mustTicket("GOV-129"),
		})
		if err == nil || !strings.Contains(err.Error(), "inspection failed") {
			t.Fatalf("StartWorktree() error = %v, want the propagated inspection failure", err)
		}
	})

	t.Run("propagates an acquisition failure", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{}, addErr: errors.New("add failed")}
		service := NewWorktreeService(git)
		_, err := service.StartWorktree(context.Background(), StartWorktreeRequest{
			Repository: testRepository(),
			Ticket:     mustTicket("GOV-129"),
		})
		if err == nil || !strings.Contains(err.Error(), "add failed") {
			t.Fatalf("StartWorktree() error = %v, want the propagated acquisition failure", err)
		}
	})

	t.Run("fails closed when the composed adapter carries no worktree capability", func(t *testing.T) {
		t.Parallel()
		service := NewWorktreeService(&fakeGitRepository{})
		_, err := service.StartWorktree(context.Background(), StartWorktreeRequest{
			Repository: testRepository(),
			Ticket:     mustTicket("GOV-129"),
		})
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("fails closed without a composed Git adapter", func(t *testing.T) {
		t.Parallel()
		service := NewWorktreeService(nil)
		_, err := service.StartWorktree(context.Background(), StartWorktreeRequest{
			Repository: testRepository(),
			Ticket:     mustTicket("GOV-129"),
		})
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("propagates an invalid remote base construction", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{}}
		service := NewWorktreeService(git)
		_, err := service.StartWorktree(context.Background(), StartWorktreeRequest{
			Repository: port.RepositoryIdentity{Root: "C:/repo", Remote: "invalid remote"},
			Ticket:     mustTicket("GOV-129"),
		})
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
		if len(git.addedPaths) != 0 {
			t.Fatalf("an invalid base must not create the worktree: %v", git.addedPaths)
		}
	})

	t.Run("defaults the remote to origin when the repository carries none", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{}}
		service := NewWorktreeService(git)
		result, err := service.StartWorktree(context.Background(), StartWorktreeRequest{
			Repository: port.RepositoryIdentity{Root: "C:/repo"},
			Ticket:     mustTicket("GOV-129"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.Base.String() != "origin/develop" {
			t.Fatalf("StartWorktree() base = %q, want origin/develop", result.Base.String())
		}
	})
}

func TestListWorktrees(t *testing.T) {
	t.Parallel()

	t.Run("inventories the worktrees and marks the current checkout", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{}, entries: []port.WorktreeEntry{
			{Path: "C:/repo", Head: "abc", Branch: "develop"},
			{Path: "C:/repo-GOV-129", Head: "def", Detached: true},
		}}
		service := NewWorktreeService(git)
		result, err := service.ListWorktrees(context.Background(), ListWorktreesRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Worktrees) != 2 || result.Current != filepath.Clean("C:/repo") {
			t.Fatalf("ListWorktrees() = %#v", result)
		}
		if countCall(git.calls, "worktree-list") != 1 {
			t.Fatalf("inventory calls = %v", git.calls)
		}
	})

	t.Run("propagates an inventory failure", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{}, listErr: errors.New("list failed")}
		service := NewWorktreeService(git)
		_, err := service.ListWorktrees(context.Background(), ListWorktreesRequest{Repository: testRepository()})
		if err == nil || !strings.Contains(err.Error(), "list failed") {
			t.Fatalf("ListWorktrees() error = %v, want the propagated inventory failure", err)
		}
	})

	t.Run("fails closed when the composed adapter carries no worktree capability", func(t *testing.T) {
		t.Parallel()
		service := NewWorktreeService(&fakeGitRepository{})
		_, err := service.ListWorktrees(context.Background(), ListWorktreesRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeInternal)
	})
}

func TestRemoveWorktree(t *testing.T) {
	t.Parallel()

	t.Run("removes the registered task worktree of the ticket", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{}, entries: []port.WorktreeEntry{
			{Path: "C:/repo", Head: "abc", Branch: "develop"},
			{Path: "C:/repo-GOV-129", Head: "def", Detached: true},
		}}
		service := NewWorktreeService(git)
		result, err := service.RemoveWorktree(context.Background(), RemoveWorktreeRequest{
			Repository: testRepository(),
			Ticket:     mustTicket("GOV-129"),
		})
		if err != nil {
			t.Fatal(err)
		}
		expectedPath := filepath.Join(filepath.Dir(testRepository().Root), "repo-GOV-129")
		if result.Path != expectedPath || result.DryRun || len(result.Plan) != 1 {
			t.Fatalf("RemoveWorktree() = %#v", result)
		}
		if len(git.removedPaths) != 1 || git.removedPaths[0] != expectedPath {
			t.Fatalf("worktree removals = %v", git.removedPaths)
		}
	})

	t.Run("plans the removal without mutating during dry-run", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{}, entries: []port.WorktreeEntry{
			{Path: "C:/repo-GOV-129", Head: "def", Detached: true},
		}}
		service := NewWorktreeService(git)
		result, err := service.RemoveWorktree(context.Background(), RemoveWorktreeRequest{
			Repository: testRepository(),
			Ticket:     mustTicket("GOV-129"),
			DryRun:     true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !result.DryRun || len(result.Plan) != 1 {
			t.Fatalf("dry-run result = %#v", result)
		}
		if len(git.removedPaths) != 0 {
			t.Fatalf("a dry run must not remove the worktree: %v", git.removedPaths)
		}
	})

	t.Run("fails closed for an unregistered worktree", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{}, entries: []port.WorktreeEntry{
			{Path: "C:/repo", Head: "abc", Branch: "develop"},
		}}
		service := NewWorktreeService(git)
		_, err := service.RemoveWorktree(context.Background(), RemoveWorktreeRequest{
			Repository: testRepository(),
			Ticket:     mustTicket("GOV-129"),
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)
		if len(git.removedPaths) != 0 {
			t.Fatalf("an unregistered worktree must never be removed: %v", git.removedPaths)
		}
	})

	t.Run("rejects a missing ticket before any Git interaction", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{}}
		service := NewWorktreeService(git)
		_, err := service.RemoveWorktree(context.Background(), RemoveWorktreeRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeInvalidInput)
		if len(git.calls) != 0 {
			t.Fatalf("a rejected request must not invoke Git: %v", git.calls)
		}
	})

	t.Run("fails closed when the composed adapter carries no worktree capability", func(t *testing.T) {
		t.Parallel()
		service := NewWorktreeService(&fakeGitRepository{})
		_, err := service.RemoveWorktree(context.Background(), RemoveWorktreeRequest{
			Repository: testRepository(),
			Ticket:     mustTicket("GOV-129"),
		})
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("propagates an inventory failure of the registration check", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{}, listErr: errors.New("list failed")}
		service := NewWorktreeService(git)
		_, err := service.RemoveWorktree(context.Background(), RemoveWorktreeRequest{
			Repository: testRepository(),
			Ticket:     mustTicket("GOV-129"),
		})
		if err == nil || !strings.Contains(err.Error(), "list failed") {
			t.Fatalf("RemoveWorktree() error = %v, want the propagated inventory failure", err)
		}
		if len(git.removedPaths) != 0 {
			t.Fatalf("a closed inventory must never remove a worktree: %v", git.removedPaths)
		}
	})

	t.Run("propagates a removal failure", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{
			fakeGitRepository: &fakeGitRepository{},
			entries:           []port.WorktreeEntry{{Path: "C:/repo-GOV-129", Head: "def", Detached: true}},
			removeErr:         errors.New("remove failed"),
		}
		service := NewWorktreeService(git)
		_, err := service.RemoveWorktree(context.Background(), RemoveWorktreeRequest{
			Repository: testRepository(),
			Ticket:     mustTicket("GOV-129"),
		})
		if err == nil || !strings.Contains(err.Error(), "remove failed") {
			t.Fatalf("RemoveWorktree() error = %v, want the propagated removal failure", err)
		}
	})
}

func TestStartTicketAcceptsFreshDetachedTaskWorktree(t *testing.T) {
	t.Parallel()

	request := func() StartTicketRequest {
		return StartTicketRequest{
			Repository: testRepository(),
			Family:     branch.FamilyFeature,
			Ticket:     mustTicket("GOV-129"),
			Slug:       mustSlug("add-export"),
		}
	}

	t.Run("starts the ticket workflow inside a clean linked task worktree", func(t *testing.T) {
		t.Parallel()
		git := &detachedTaskWorktreeGit{fakeWorktreeGit: &fakeWorktreeGit{
			fakeGitRepository: &fakeGitRepository{hasCommits: true, clean: true},
			linked:            true,
		}}
		service := newTicketServiceWithGit(git, nil, nil)
		result, err := service.StartTicket(context.Background(), request())
		if err != nil {
			t.Fatal(err)
		}
		if result.Official.Name.String() != "feature/GOV-129-add-export" {
			t.Fatalf("StartTicket() = %#v", result)
		}
		if countCall(git.calls, "linked-worktree") != 1 {
			t.Fatalf("the acceptance must verify the worktree form: %v", git.calls)
		}
		if countCall(git.calls, "worktree-clean") < 1 {
			t.Fatalf("the acceptance must verify the clean worktree: %v", git.calls)
		}
	})

	t.Run("blocks a dirty linked task worktree before any branch creation", func(t *testing.T) {
		t.Parallel()
		git := &detachedTaskWorktreeGit{fakeWorktreeGit: &fakeWorktreeGit{
			fakeGitRepository: &fakeGitRepository{hasCommits: true, clean: false},
			linked:            true,
		}}
		service := newTicketServiceWithGit(git, nil, nil)
		_, err := service.StartTicket(context.Background(), request())
		assertProblemCode(t, err, problem.CodeWorktreeNotClean)
		if countCall(git.calls, "create-branch") != 0 {
			t.Fatalf("a dirty worktree must never receive branch creation: %v", git.calls)
		}
	})

	t.Run("blocks ticket work outside a linked task worktree", func(t *testing.T) {
		t.Parallel()
		git := &detachedTaskWorktreeGit{fakeWorktreeGit: &fakeWorktreeGit{
			fakeGitRepository: &fakeGitRepository{hasCommits: true, clean: true},
			linked:            false,
		}}
		service := newTicketServiceWithGit(git, nil, nil)
		_, err := service.StartTicket(context.Background(), request())
		assertProblemCode(t, err, problem.CodeWorktreeRequired)
		if countCall(git.calls, "create-branch") != 0 {
			t.Fatalf("a blocked context must never create a branch: %v", git.calls)
		}
	})

	t.Run("keeps the unenforced legacy behavior when no worktree capability is composed", func(t *testing.T) {
		t.Parallel()
		git := &detachedPrimaryGit{fakeGitRepository: &fakeGitRepository{hasCommits: true, clean: true}}
		service := newTicketServiceWithGit(git, nil, nil)
		if _, err := service.StartTicket(context.Background(), request()); err != nil {
			t.Fatal(err)
		}
		if countCall(git.calls, "create-branch") != 1 {
			t.Fatalf("a composition without the worktree capability keeps the legacy behavior: %v", git.calls)
		}
	})

	t.Run("propagates a worktree-form inspection failure", func(t *testing.T) {
		t.Parallel()
		git := &detachedTaskWorktreeGit{fakeWorktreeGit: &fakeWorktreeGit{
			fakeGitRepository: &fakeGitRepository{hasCommits: true, clean: true},
			linkedErr:         errors.New("inspection failed"),
		}}
		service := newTicketServiceWithGit(git, nil, nil)
		_, err := service.StartTicket(context.Background(), request())
		if err == nil || !strings.Contains(err.Error(), "inspection failed") {
			t.Fatalf("StartTicket() error = %v, want the propagated inspection failure", err)
		}
		if countCall(git.calls, "create-branch") != 0 {
			t.Fatalf("an unverified worktree form must never create a branch: %v", git.calls)
		}
	})

	t.Run("propagates a clean-inspection failure of a linked task worktree", func(t *testing.T) {
		t.Parallel()
		git := &detachedTaskWorktreeGit{fakeWorktreeGit: &fakeWorktreeGit{
			fakeGitRepository: &fakeGitRepository{hasCommits: true, clean: true, err: errors.New("clean inspection failed")},
			linked:            true,
		}}
		service := newTicketServiceWithGit(git, nil, nil)
		_, err := service.StartTicket(context.Background(), request())
		if err == nil || !strings.Contains(err.Error(), "clean inspection failed") {
			t.Fatalf("StartTicket() error = %v, want the propagated inspection failure", err)
		}
		if countCall(git.calls, "create-branch") != 0 {
			t.Fatalf("an unverified worktree must never create a branch: %v", git.calls)
		}
	})

	t.Run("propagates a non-detached current-branch failure", func(t *testing.T) {
		t.Parallel()
		git := &worktreeFormGit{
			fakeWorktreeGit: &fakeWorktreeGit{
				fakeGitRepository: &fakeGitRepository{hasCommits: true, clean: true},
				linked:            true,
			},
			currentErr: errors.New("current branch unavailable"),
		}
		service := newTicketServiceWithGit(git, nil, nil)
		_, err := service.StartTicket(context.Background(), request())
		if err == nil || !strings.Contains(err.Error(), "current branch unavailable") {
			t.Fatalf("StartTicket() error = %v, want the propagated Git failure", err)
		}
	})

	t.Run("requires the worktree even for a checked-out branch", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{hasCommits: true, clean: true}}
		service := newTicketServiceWithGit(git, nil, nil)
		_, err := service.StartTicket(context.Background(), request())
		assertProblemCode(t, err, problem.CodeWorktreeRequired)
		if countCall(git.calls, "linked-worktree") != 1 {
			t.Fatalf("the enforcement must verify the worktree form: %v", git.calls)
		}
		if countCall(git.calls, "create-branch") != 0 {
			t.Fatalf("a blocked context must never create a branch: %v", git.calls)
		}
	})

	t.Run("accepts a checked-out branch inside a linked task worktree", func(t *testing.T) {
		t.Parallel()
		git := &worktreeFormGit{
			fakeWorktreeGit: &fakeWorktreeGit{
				fakeGitRepository: &fakeGitRepository{hasCommits: true, clean: true},
				linked:            true,
			},
			current: mustBranch("feature/GOV-129-add-export"),
		}
		service := newTicketServiceWithGit(git, nil, nil)
		if _, err := service.StartTicket(context.Background(), request()); err != nil {
			t.Fatal(err)
		}
		if countCall(git.calls, "current-branch") != 1 {
			t.Fatalf("the continuing form must resolve the checked-out branch: %v", git.calls)
		}
		if countCall(git.calls, "create-branch") != 1 {
			t.Fatalf("the continuing form must proceed to branch creation: %v", git.calls)
		}
	})
}

func TestReturnToIntegrationLineSkipsTaskWorktree(t *testing.T) {
	t.Parallel()

	t.Run("reports the linked worktree form without switching", func(t *testing.T) {
		t.Parallel()
		git := &worktreeFormGit{
			fakeWorktreeGit: &fakeWorktreeGit{
				fakeGitRepository: &fakeGitRepository{clean: true},
				linked:            true,
			},
			current: mustBranch("feature/GOV-129-add-export"),
		}
		result := returnToIntegrationLine(context.Background(), git, testRepository())
		if result.Status != IntegrationLineReturnSkippedWorktree ||
			result.Branch.String() != "develop" ||
			!strings.Contains(result.Detail, "linked task worktree") {
			t.Fatalf("returnToIntegrationLine() = %#v", result)
		}
		if len(git.switchedTo) != 0 {
			t.Fatalf("a task worktree must never be switched: %v", git.switchedTo)
		}
		if strings.Contains(strings.Join(git.calls, ","), "current-branch") {
			t.Fatalf("the worktree form must be inspected before the branch: %v", git.calls)
		}
		if strings.Contains(strings.Join(git.calls, ","), "worktree-clean") {
			t.Fatalf("a skipped transition must not inspect cleanliness: %v", git.calls)
		}
	})

	t.Run("reports a failed worktree-form inspection without questioning the publication", func(t *testing.T) {
		t.Parallel()
		git := &worktreeFormGit{
			fakeWorktreeGit: &fakeWorktreeGit{
				fakeGitRepository: &fakeGitRepository{clean: true},
				linkedErr:         errors.New("inspection failed"),
			},
			current: mustBranch("feature/GOV-129-add-export"),
		}
		result := returnToIntegrationLine(context.Background(), git, testRepository())
		if result.Status != IntegrationLineReturnFailed ||
			!strings.Contains(result.Detail, "inspect the worktree form") {
			t.Fatalf("returnToIntegrationLine() = %#v", result)
		}
		if len(git.switchedTo) != 0 {
			t.Fatalf("an unverified worktree form must never be switched: %v", git.switchedTo)
		}
	})
}

func TestStartWorktreeLaneAwareBase(t *testing.T) {
	t.Parallel()

	t.Run("acquires from an explicit protected main base", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{}}
		service := NewWorktreeService(git)
		result, err := service.StartWorktree(context.Background(), StartWorktreeRequest{
			Repository: testRepository(),
			Ticket:     mustTicket("GOV-129"),
			BaseLine:   "main",
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.Base.String() != "origin/main" {
			t.Fatalf("StartWorktree() base = %q, want origin/main", result.Base.String())
		}
		if len(git.addedBases) != 1 || git.addedBases[0].String() != "origin/main" {
			t.Fatalf("worktree additions = %v with bases %v", git.addedPaths, git.addedBases)
		}
		if result.Plan[1].Detail != "git worktree add --detach "+result.Path+" origin/main" {
			t.Fatalf("the plan must bind the lane base: %q", result.Plan[1].Detail)
		}
	})

	t.Run("acquires from an explicit frozen release base", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{}}
		service := NewWorktreeService(git)
		result, err := service.StartWorktree(context.Background(), StartWorktreeRequest{
			Repository: testRepository(),
			Ticket:     mustTicket("GOV-129"),
			BaseLine:   "release/1.2.0",
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.Base.String() != "origin/release/1.2.0" {
			t.Fatalf("StartWorktree() base = %q, want origin/release/1.2.0", result.Base.String())
		}
	})

	t.Run("acquires from an explicit protected support base", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{}}
		service := NewWorktreeService(git)
		result, err := service.StartWorktree(context.Background(), StartWorktreeRequest{
			Repository: testRepository(),
			Ticket:     mustTicket("GOV-129"),
			BaseLine:   "support/1.2",
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.Base.String() != "origin/support/1.2" {
			t.Fatalf("StartWorktree() base = %q, want origin/support/1.2", result.Base.String())
		}
	})

	t.Run("rejects a working-family base as a lane violation", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{}}
		service := NewWorktreeService(git)
		_, err := service.StartWorktree(context.Background(), StartWorktreeRequest{
			Repository: testRepository(),
			Ticket:     mustTicket("GOV-129"),
			BaseLine:   "feature/GOV-129-add-export",
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)
		if len(git.addedPaths) != 0 || countCall(git.calls, "worktree-list") != 0 {
			t.Fatalf("a rejected base must not inventory or mutate: %v", git.calls)
		}
	})

	t.Run("rejects a non-canonical base name", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{}}
		service := NewWorktreeService(git)
		_, err := service.StartWorktree(context.Background(), StartWorktreeRequest{
			Repository: testRepository(),
			Ticket:     mustTicket("GOV-129"),
			BaseLine:   "not a branch",
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)
		if len(git.addedPaths) != 0 || countCall(git.calls, "worktree-list") != 0 {
			t.Fatalf("a rejected base must not inventory or mutate: %v", git.calls)
		}
	})
}

func TestStartWorktreeConflictPrecheck(t *testing.T) {
	t.Parallel()

	t.Run("fails closed when the derived worktree path is already registered", func(t *testing.T) {
		t.Parallel()
		existing := filepath.Join(filepath.Dir(testRepository().Root), "repo-GOV-129")
		git := &fakeWorktreeGit{
			fakeGitRepository: &fakeGitRepository{},
			entries: []port.WorktreeEntry{
				{Path: existing, Detached: true},
			},
		}
		service := NewWorktreeService(git)
		_, err := service.StartWorktree(context.Background(), StartWorktreeRequest{
			Repository: testRepository(),
			Ticket:     mustTicket("GOV-129"),
		})
		assertProblemCode(t, err, problem.CodeWorktreeConflict)
		if len(git.addedPaths) != 0 {
			t.Fatalf("a conflicted acquisition must not create a worktree: %v", git.addedPaths)
		}
	})

	t.Run("acquires when no registered worktree occupies the derived path", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{
			fakeGitRepository: &fakeGitRepository{},
			entries: []port.WorktreeEntry{
				{Path: testRepository().Root, Branch: "develop"},
			},
		}
		service := NewWorktreeService(git)
		result, err := service.StartWorktree(context.Background(), StartWorktreeRequest{
			Repository: testRepository(),
			Ticket:     mustTicket("GOV-129"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(git.addedPaths) != 1 {
			t.Fatalf("an unoccupied path must acquire the worktree: %v", git.addedPaths)
		}
		if result.Path == testRepository().Root {
			t.Fatalf("StartWorktree() path = %q, want the derived sibling path", result.Path)
		}
	})

	t.Run("propagates a registration inventory failure", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{
			fakeGitRepository: &fakeGitRepository{},
			listErr:           errors.New("inventory failed"),
		}
		service := NewWorktreeService(git)
		_, err := service.StartWorktree(context.Background(), StartWorktreeRequest{
			Repository: testRepository(),
			Ticket:     mustTicket("GOV-129"),
		})
		if err == nil || !strings.Contains(err.Error(), "inventory failed") {
			t.Fatalf("StartWorktree() error = %v, want the propagated inventory failure", err)
		}
	})
}

func TestRequireTaskWorktreeBaseGuard(t *testing.T) {
	t.Parallel()

	t.Run("accepts a detached pre-start form at its acquired base revision", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{
			fakeGitRepository: &fakeGitRepository{clean: true},
			linked:            true,
			head:              "c46015869552bc0433fa2a5276713d74bfc73f87",
			baseRevision:      "c46015869552bc0433fa2a5276713d74bfc73f87",
		}
		detached := &detachedTaskWorktreeGit{fakeWorktreeGit: git}
		guard := &TicketService{git: detached}
		if err := guard.requireTaskWorktree(context.Background(), testRepository()); err != nil {
			t.Fatal(err)
		}
		if countCall(detached.calls, "worktree-head-matches-base") != 1 {
			t.Fatalf("the detached pre-start form must measure the base revisions: %v", detached.calls)
		}
	})

	t.Run("fails closed on a drifted detached head", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{
			fakeGitRepository: &fakeGitRepository{clean: true},
			linked:            true,
			head:              "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			baseRevision:      "c46015869552bc0433fa2a5276713d74bfc73f87",
		}
		detached := &detachedTaskWorktreeGit{fakeWorktreeGit: git}
		guard := &TicketService{git: detached}
		err := guard.requireTaskWorktree(context.Background(), testRepository())
		assertProblemCode(t, err, problem.CodeWorktreeBaseDrift)
	})

	t.Run("propagates a base-revision measurement failure", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{
			fakeGitRepository: &fakeGitRepository{clean: true},
			linked:            true,
			headBaseErr:       errors.New("revision measurement failed"),
		}
		detached := &detachedTaskWorktreeGit{fakeWorktreeGit: git}
		guard := &TicketService{git: detached}
		err := guard.requireTaskWorktree(context.Background(), testRepository())
		if err == nil || !strings.Contains(err.Error(), "revision measurement failed") {
			t.Fatalf("requireTaskWorktree() error = %v, want the propagated measurement failure", err)
		}
	})

	t.Run("does not measure the base in the continuing branch form", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{
			fakeGitRepository: &fakeGitRepository{clean: true},
			linked:            true,
		}
		guard := &TicketService{git: git}
		err := guard.requireTaskWorktree(context.Background(), testRepository())
		if err != nil {
			t.Fatal(err)
		}
		if countCall(git.calls, "worktree-head-matches-base") != 0 {
			t.Fatalf("the continuing form must not run the base guard: %v", git.calls)
		}
	})
}

// fakePruneGit extends the shared workflow fake with the complete worktree
// capability plus the remote branch surface the prune measures, and records
// every removal. The cleanliness of each worktree path is controllable
// individually so per-entry evidence degradation stays testable.
type fakePruneGit struct {
	*fakeGitRepository
	entries        []port.WorktreeEntry
	listErr        error
	cleanFor       map[string]bool
	cleanErrFor    map[string]error
	remoteBranches []branch.BranchName
	remoteErr      error
	removeErr      error
	removedPaths   []string
}

func (fake *fakePruneGit) WorktreeList(context.Context, port.RepositoryIdentity) ([]port.WorktreeEntry, error) {
	fake.calls = append(fake.calls, "worktree-list")
	if fake.listErr != nil {
		return nil, fake.listErr
	}
	return fake.entries, nil
}

func (fake *fakePruneGit) WorktreeAddDetached(context.Context, port.RepositoryIdentity, string, branch.TargetBase) error {
	return nil
}

func (fake *fakePruneGit) WorktreeRemove(_ context.Context, _ port.RepositoryIdentity, path string) error {
	fake.calls = append(fake.calls, "worktree-remove")
	if fake.removeErr != nil {
		return fake.removeErr
	}
	fake.removedPaths = append(fake.removedPaths, path)
	return nil
}

func (fake *fakePruneGit) LinkedWorktree(context.Context, port.RepositoryIdentity) (bool, error) {
	return false, nil
}

func (fake *fakePruneGit) WorktreeHeadMatchesBase(context.Context, port.RepositoryIdentity, branch.TargetBase) (string, string, error) {
	return "", "", nil
}

func (fake *fakePruneGit) RemoteBranches(context.Context, port.RepositoryIdentity) ([]branch.BranchName, error) {
	fake.calls = append(fake.calls, "remote-branches")
	if fake.remoteErr != nil {
		return nil, fake.remoteErr
	}
	return fake.remoteBranches, nil
}

func (fake *fakePruneGit) IsWorktreeClean(_ context.Context, identity port.RepositoryIdentity) (bool, error) {
	fake.calls = append(fake.calls, "worktree-clean")
	if err, bound := fake.cleanErrFor[identity.Root]; bound {
		return false, err
	}
	if clean, bound := fake.cleanFor[identity.Root]; bound {
		return clean, nil
	}
	return fake.clean, nil
}

// fakeWorktreeManagerOnlyGit carries the worktree capability without the
// remote branch surface, modeling a composition whose adapter cannot measure
// the branch-cleanup obligation of the prune.
type fakeWorktreeManagerOnlyGit struct {
	*fakeGitRepository
	entries []port.WorktreeEntry
	listErr error
}

func (fake *fakeWorktreeManagerOnlyGit) WorktreeList(context.Context, port.RepositoryIdentity) ([]port.WorktreeEntry, error) {
	fake.calls = append(fake.calls, "worktree-list")
	return fake.entries, fake.listErr
}

func (fake *fakeWorktreeManagerOnlyGit) WorktreeAddDetached(context.Context, port.RepositoryIdentity, string, branch.TargetBase) error {
	return nil
}

func (fake *fakeWorktreeManagerOnlyGit) WorktreeRemove(context.Context, port.RepositoryIdentity, string) error {
	return nil
}

func (fake *fakeWorktreeManagerOnlyGit) LinkedWorktree(context.Context, port.RepositoryIdentity) (bool, error) {
	return false, nil
}

func (fake *fakeWorktreeManagerOnlyGit) WorktreeHeadMatchesBase(context.Context, port.RepositoryIdentity, branch.TargetBase) (string, string, error) {
	return "", "", nil
}

// fakePullRequestInventory carries the provider records the prune matches
// through the canonical title grammars.
type fakePullRequestInventory struct {
	records []port.PullRequestSummary
	err     error
}

func (fake *fakePullRequestInventory) ListPullRequests(context.Context, port.PullRequestInventoryQuery) ([]port.PullRequestSummary, error) {
	if fake.err != nil {
		return nil, fake.err
	}
	return fake.records, nil
}

func TestPruneWorktrees(t *testing.T) {
	t.Parallel()

	const taskPath = "C:/repo-GOV-129"
	const taskBranch = "feature/GOV-129-add-export"
	primaryEntry := port.WorktreeEntry{Path: testRepository().Root, Head: "abc", Branch: "develop"}
	taskEntry := port.WorktreeEntry{Path: taskPath, Head: "def", Branch: taskBranch}
	mergedRecord := port.PullRequestSummary{Number: "12", Title: "GOV-129: add-export", State: port.PullRequestStateMerged}
	envelopeRecord := func(state port.PullRequestState, number string) port.PullRequestSummary {
		return port.PullRequestSummary{Number: number, Title: "feat(GOV-129): add export button", State: state}
	}
	service := func(git port.GitRepository, records []port.PullRequestSummary) *WorktreeService {
		return NewWorktreeService(git).WithPullRequestInventory(&fakePullRequestInventory{records: records})
	}
	entryByPath := func(entries []WorktreePruneEntry, path string) WorktreePruneEntry {
		for _, entry := range entries {
			if entry.Path == path {
				return entry
			}
		}
		return WorktreePruneEntry{}
	}

	t.Run("removes the proven-complete worktree and keeps the others", func(t *testing.T) {
		t.Parallel()
		git := &fakePruneGit{
			fakeGitRepository: &fakeGitRepository{clean: true},
			entries: []port.WorktreeEntry{
				primaryEntry,
				taskEntry,
				{Path: "C:/repo-GOV-131", Head: "ghi", Branch: "docs/GOV-131-modularize-workflow-metadata"},
			},
			remoteBranches: []branch.BranchName{},
		}
		result, err := service(git, []port.PullRequestSummary{
			mergedRecord,
			{Number: "13", Title: "docs(GOV-131): modularize workflow metadata", State: port.PullRequestStateOpen},
		}).PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		removed := entryByPath(result.Entries, taskPath)
		if removed.Class != pruneClassStaleEligible || !removed.Removed || removed.Ticket != "GOV-129" ||
			removed.PRState != "merged" || removed.PRNumber != "12" {
			t.Fatalf("the proven-complete entry = %#v", removed)
		}
		kept := entryByPath(result.Entries, "C:/repo-GOV-131")
		if kept.Class != pruneClassPublishedOpen || kept.Removed || kept.PRState != "open" {
			t.Fatalf("the published-open entry = %#v", kept)
		}
		primary := entryByPath(result.Entries, testRepository().Root)
		if primary.Class != pruneClassOutOfScope || primary.Removed {
			t.Fatalf("the primary checkout entry = %#v", primary)
		}
		if len(git.removedPaths) != 1 || git.removedPaths[0] != taskPath {
			t.Fatalf("worktree removals = %v", git.removedPaths)
		}
		if countCall(git.calls, "worktree-remove") != 1 {
			t.Fatalf("prune calls = %v", git.calls)
		}
	})

	t.Run("keeps a worktree whose branch-cleanup obligation is open", func(t *testing.T) {
		t.Parallel()
		git := &fakePruneGit{
			fakeGitRepository: &fakeGitRepository{clean: true},
			entries:           []port.WorktreeEntry{taskEntry},
			remoteBranches:    []branch.BranchName{mustBranch(taskBranch)},
		}
		result, err := service(git, []port.PullRequestSummary{mergedRecord}).PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		entry := result.Entries[0]
		if entry.Class != pruneClassActive || entry.Removed ||
			!strings.Contains(entry.Reason, "branch-cleanup obligation") {
			t.Fatalf("the obligation-open entry = %#v", entry)
		}
		if len(git.removedPaths) != 0 {
			t.Fatalf("an obligation-open worktree must never be removed: %v", git.removedPaths)
		}
	})

	t.Run("keeps a worktree that carries uncommitted state", func(t *testing.T) {
		t.Parallel()
		git := &fakePruneGit{
			fakeGitRepository: &fakeGitRepository{},
			entries:           []port.WorktreeEntry{taskEntry},
			cleanFor:          map[string]bool{taskPath: false},
		}
		result, err := service(git, []port.PullRequestSummary{mergedRecord}).PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		entry := result.Entries[0]
		if entry.Class != pruneClassActive || entry.Removed ||
			!strings.Contains(entry.Reason, "uncommitted state") {
			t.Fatalf("the dirty entry = %#v", entry)
		}
		if len(git.removedPaths) != 0 {
			t.Fatalf("a dirty worktree must never be removed: %v", git.removedPaths)
		}
	})

	t.Run("keeps a worktree held by an active governed operation", func(t *testing.T) {
		t.Parallel()
		git := &fakePruneGit{
			fakeGitRepository: &fakeGitRepository{clean: true, active: true, activeOperation: "rebase"},
			entries:           []port.WorktreeEntry{taskEntry},
		}
		result, err := service(git, []port.PullRequestSummary{mergedRecord}).PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		entry := result.Entries[0]
		if entry.Class != pruneClassActive || entry.Removed ||
			!strings.Contains(entry.Reason, "rebase") {
			t.Fatalf("the held entry = %#v", entry)
		}
		if len(git.removedPaths) != 0 {
			t.Fatalf("a held worktree must never be removed: %v", git.removedPaths)
		}
	})

	t.Run("keeps an abandoned worktree for the actor decision", func(t *testing.T) {
		t.Parallel()
		git := &fakePruneGit{
			fakeGitRepository: &fakeGitRepository{clean: true},
			entries:           []port.WorktreeEntry{taskEntry},
		}
		result, err := service(git, []port.PullRequestSummary{envelopeRecord(port.PullRequestStateClosed, "12")}).
			PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		entry := result.Entries[0]
		if entry.Class != pruneClassAbandoned || entry.Removed || entry.PRState != "closed" ||
			!strings.Contains(entry.Reason, "belongs to the actor") {
			t.Fatalf("the abandoned entry = %#v", entry)
		}
		if len(git.removedPaths) != 0 {
			t.Fatalf("an abandoned worktree must never be removed: %v", git.removedPaths)
		}
	})

	t.Run("keeps a worktree without a pull-request record", func(t *testing.T) {
		t.Parallel()
		git := &fakePruneGit{
			fakeGitRepository: &fakeGitRepository{clean: true},
			entries:           []port.WorktreeEntry{taskEntry},
		}
		result, err := service(git, []port.PullRequestSummary{
			{Number: "5", Title: "KHUB-21: other work", State: port.PullRequestStateMerged},
		}).PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		entry := result.Entries[0]
		if entry.Class != pruneClassActive || entry.Removed || entry.PRState != "" ||
			!strings.Contains(entry.Reason, "no pull request records") {
			t.Fatalf("the unpublished entry = %#v", entry)
		}
	})

	t.Run("matches records only through the canonical title grammars", func(t *testing.T) {
		t.Parallel()
		git := &fakePruneGit{
			fakeGitRepository: &fakeGitRepository{clean: true},
			entries:           []port.WorktreeEntry{taskEntry},
		}
		result, err := service(git, []port.PullRequestSummary{
			{Number: "7", Title: "unrelated GOV-129 mention", State: port.PullRequestStateMerged},
		}).PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		entry := result.Entries[0]
		if entry.Class != pruneClassActive || entry.Removed {
			t.Fatalf("a non-grammar mention must never bind the record: %#v", entry)
		}
	})

	t.Run("classifies detached worktrees by their local form", func(t *testing.T) {
		t.Parallel()
		fresh := port.WorktreeEntry{Path: "C:/repo-GOV-129", Head: "def", Detached: true}
		dirty := port.WorktreeEntry{Path: "C:/repo-GOV-130", Head: "ghi", Detached: true}
		orphaned := port.WorktreeEntry{Path: "C:/repo-GOV-131", Head: "jkl", Detached: true}
		git := &fakePruneGit{
			fakeGitRepository: &fakeGitRepository{clean: true},
			entries:           []port.WorktreeEntry{fresh, dirty, orphaned},
			cleanFor:          map[string]bool{"C:/repo-GOV-130": false},
			cleanErrFor:       map[string]error{"C:/repo-GOV-131": errors.New("orphaned worktree registration")},
		}
		result, err := service(git, nil).PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		if entry := entryByPath(result.Entries, "C:/repo-GOV-129"); entry.Class != pruneClassPreStart || entry.Removed || entry.Ticket != "GOV-129" {
			t.Fatalf("the pre-start entry = %#v", entry)
		}
		if entry := entryByPath(result.Entries, "C:/repo-GOV-130"); entry.Class != pruneClassActive || entry.Removed {
			t.Fatalf("the dirty detached entry = %#v", entry)
		}
		if entry := entryByPath(result.Entries, "C:/repo-GOV-131"); entry.Class != pruneClassActive || entry.Removed ||
			!strings.Contains(entry.Reason, "unmeasurable") {
			t.Fatalf("the unmeasurable detached entry = %#v", entry)
		}
		if len(git.removedPaths) != 0 {
			t.Fatalf("a pre-start classification must never remove: %v", git.removedPaths)
		}
	})

	t.Run("classifies non-registry paths as out of scope", func(t *testing.T) {
		t.Parallel()
		git := &fakePruneGit{
			fakeGitRepository: &fakeGitRepository{clean: true},
			entries: []port.WorktreeEntry{
				{Path: "C:/repo-main-control", Head: "abc", Branch: "hotfix/GOV-27-publish-prepared-reconciliation-candidate"},
				{Path: "C:/bare", Bare: true},
			},
		}
		result, err := service(git, nil).PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range result.Entries {
			if entry.Class != pruneClassOutOfScope || entry.Removed {
				t.Fatalf("an out-of-scope entry = %#v", entry)
			}
		}
		if len(git.removedPaths) != 0 {
			t.Fatalf("an out-of-scope worktree must never be removed: %v", git.removedPaths)
		}
	})

	t.Run("the newest pull-request record decides the review state", func(t *testing.T) {
		t.Parallel()
		t.Run("an open record keeps the worktree published-open", func(t *testing.T) {
			t.Parallel()
			git := &fakePruneGit{
				fakeGitRepository: &fakeGitRepository{clean: true},
				entries:           []port.WorktreeEntry{taskEntry},
			}
			result, err := service(git, []port.PullRequestSummary{
				mergedRecord,
				{Number: "13", Title: "GOV-129: add-export", State: port.PullRequestStateOpen},
			}).PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
			if err != nil {
				t.Fatal(err)
			}
			entry := result.Entries[0]
			if entry.Class != pruneClassPublishedOpen || entry.Removed || entry.PRNumber != "13" {
				t.Fatalf("the newest-open entry = %#v", entry)
			}
		})

		t.Run("a merged record proves completion", func(t *testing.T) {
			t.Parallel()
			git := &fakePruneGit{
				fakeGitRepository: &fakeGitRepository{clean: true},
				entries:           []port.WorktreeEntry{taskEntry},
			}
			result, err := service(git, []port.PullRequestSummary{
				{Number: "12", Title: "GOV-129: add-export", State: port.PullRequestStateOpen},
				{Number: "13", Title: "GOV-129: add-export", State: port.PullRequestStateMerged},
			}).PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
			if err != nil {
				t.Fatal(err)
			}
			entry := result.Entries[0]
			if entry.Class != pruneClassStaleEligible || !entry.Removed || entry.PRNumber != "13" || entry.PRState != "merged" {
				t.Fatalf("the newest-merged entry = %#v", entry)
			}
		})
	})

	t.Run("plans the removal without mutating during dry-run", func(t *testing.T) {
		t.Parallel()
		git := &fakePruneGit{
			fakeGitRepository: &fakeGitRepository{clean: true},
			entries:           []port.WorktreeEntry{taskEntry},
		}
		result, err := service(git, []port.PullRequestSummary{mergedRecord}).PruneWorktrees(context.Background(), PruneWorktreesRequest{
			Repository: testRepository(),
			DryRun:     true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !result.DryRun || len(result.Plan) != 1 || result.Plan[0].Action != "worktree-remove" ||
			result.Plan[0].Detail != "git worktree remove "+taskPath {
			t.Fatalf("dry-run result = %#v", result)
		}
		if entry := result.Entries[0]; entry.Class != pruneClassStaleEligible || entry.Removed {
			t.Fatalf("the dry-run entry = %#v", entry)
		}
		if len(git.removedPaths) != 0 {
			t.Fatalf("a dry run must not remove the worktree: %v", git.removedPaths)
		}
	})

	t.Run("keeps a worktree whose local evidence is unmeasurable and removes the next candidate", func(t *testing.T) {
		t.Parallel()
		git := &fakePruneGit{
			fakeGitRepository: &fakeGitRepository{clean: true},
			entries: []port.WorktreeEntry{
				{Path: "C:/repo-GOV-130", Head: "ghi", Branch: "docs/GOV-130-derive-the-genesis-timeout-budget"},
				taskEntry,
			},
			cleanErrFor: map[string]error{"C:/repo-GOV-130": errors.New("orphaned worktree registration")},
		}
		result, err := service(git, []port.PullRequestSummary{
			{Number: "9", Title: "docs(GOV-130): derive the genesis timeout budget", State: port.PullRequestStateMerged},
			mergedRecord,
		}).PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		unmeasurable := entryByPath(result.Entries, "C:/repo-GOV-130")
		if unmeasurable.Class != pruneClassActive || unmeasurable.Removed ||
			!strings.Contains(unmeasurable.Reason, "unmeasurable") {
			t.Fatalf("the unmeasurable entry = %#v", unmeasurable)
		}
		removed := entryByPath(result.Entries, taskPath)
		if removed.Class != pruneClassStaleEligible || !removed.Removed {
			t.Fatalf("the next candidate = %#v", removed)
		}
		if len(git.removedPaths) != 1 || git.removedPaths[0] != taskPath {
			t.Fatalf("worktree removals = %v", git.removedPaths)
		}
	})

	t.Run("records a refused removal fail-closed", func(t *testing.T) {
		t.Parallel()
		git := &fakePruneGit{
			fakeGitRepository: &fakeGitRepository{clean: true},
			entries:           []port.WorktreeEntry{taskEntry},
			removeErr:         errors.New("remove failed"),
		}
		result, err := service(git, []port.PullRequestSummary{mergedRecord}).PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		entry := result.Entries[0]
		if entry.Removed || !strings.Contains(entry.Reason, "removal refused") {
			t.Fatalf("the refused entry = %#v", entry)
		}
		if len(git.removedPaths) != 0 {
			t.Fatalf("a refused removal must not record a removal: %v", git.removedPaths)
		}
	})

	t.Run("keeps a worktree whose operation inspection is unmeasurable", func(t *testing.T) {
		t.Parallel()
		git := &fakePruneGit{
			fakeGitRepository: &fakeGitRepository{clean: true, activeErr: errors.New("operation inspection failed")},
			entries:           []port.WorktreeEntry{taskEntry},
		}
		result, err := service(git, []port.PullRequestSummary{mergedRecord}).PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		entry := result.Entries[0]
		if entry.Class != pruneClassActive || entry.Removed ||
			!strings.Contains(entry.Reason, "unmeasurable") {
			t.Fatalf("the unmeasurable-operation entry = %#v", entry)
		}
		if len(git.removedPaths) != 0 {
			t.Fatalf("an unverified worktree must never be removed: %v", git.removedPaths)
		}
	})

	t.Run("orders the pull-request records of the ticket deterministically", func(t *testing.T) {
		t.Parallel()
		t.Run("an unparseable number sorts below every parseable number", func(t *testing.T) {
			t.Parallel()
			git := &fakePruneGit{
				fakeGitRepository: &fakeGitRepository{clean: true},
				entries:           []port.WorktreeEntry{taskEntry},
			}
			result, err := service(git, []port.PullRequestSummary{
				{Number: "abc", Title: "GOV-129: add-export", State: port.PullRequestStateMerged},
				{Number: "14", Title: "GOV-129: add-export", State: port.PullRequestStateOpen},
			}).PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
			if err != nil {
				t.Fatal(err)
			}
			entry := result.Entries[0]
			if entry.PRNumber != "14" || entry.Class != pruneClassPublishedOpen {
				t.Fatalf("the parseable newest record must decide: %#v", entry)
			}
		})

		t.Run("a non-numeric newest candidate never replaces a parsed record", func(t *testing.T) {
			t.Parallel()
			git := &fakePruneGit{
				fakeGitRepository: &fakeGitRepository{clean: true},
				entries:           []port.WorktreeEntry{taskEntry},
			}
			result, err := service(git, []port.PullRequestSummary{
				mergedRecord,
				{Number: "abc", Title: "GOV-129: add-export", State: port.PullRequestStateOpen},
			}).PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
			if err != nil {
				t.Fatal(err)
			}
			entry := result.Entries[0]
			if entry.PRNumber != "12" || entry.Class != pruneClassStaleEligible || !entry.Removed {
				t.Fatalf("the parsed newest record must decide: %#v", entry)
			}
		})

		t.Run("records in descending order keep the highest number", func(t *testing.T) {
			t.Parallel()
			git := &fakePruneGit{
				fakeGitRepository: &fakeGitRepository{clean: true},
				entries:           []port.WorktreeEntry{taskEntry},
			}
			result, err := service(git, []port.PullRequestSummary{
				{Number: "13", Title: "GOV-129: add-export", State: port.PullRequestStateMerged},
				{Number: "12", Title: "GOV-129: add-export", State: port.PullRequestStateOpen},
			}).PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
			if err != nil {
				t.Fatal(err)
			}
			entry := result.Entries[0]
			if entry.PRNumber != "13" || entry.Class != pruneClassStaleEligible || !entry.Removed {
				t.Fatalf("the highest record must decide: %#v", entry)
			}
		})
	})

	t.Run("propagates a worktree inventory failure", func(t *testing.T) {
		t.Parallel()
		git := &fakePruneGit{
			fakeGitRepository: &fakeGitRepository{},
			listErr:           errors.New("list failed"),
		}
		_, err := service(git, nil).PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
		if err == nil || !strings.Contains(err.Error(), "list failed") {
			t.Fatalf("PruneWorktrees() error = %v, want the propagated inventory failure", err)
		}
	})

	t.Run("propagates a remote URL failure", func(t *testing.T) {
		t.Parallel()
		git := &fakePruneGit{
			fakeGitRepository: &fakeGitRepository{err: errors.New("remote url failed")},
		}
		_, err := service(git, nil).PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
		if err == nil || !strings.Contains(err.Error(), "remote url failed") {
			t.Fatalf("PruneWorktrees() error = %v, want the propagated remote URL failure", err)
		}
	})

	t.Run("propagates a pull-request inventory failure", func(t *testing.T) {
		t.Parallel()
		git := &fakePruneGit{fakeGitRepository: &fakeGitRepository{clean: true}}
		inventory := &fakePullRequestInventory{err: errors.New("provider unreachable")}
		_, err := NewWorktreeService(git).WithPullRequestInventory(inventory).
			PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
		if err == nil || !strings.Contains(err.Error(), "provider unreachable") {
			t.Fatalf("PruneWorktrees() error = %v, want the propagated provider failure", err)
		}
	})

	t.Run("propagates a remote branch listing failure", func(t *testing.T) {
		t.Parallel()
		git := &fakePruneGit{
			fakeGitRepository: &fakeGitRepository{clean: true},
			remoteErr:         errors.New("remote branches failed"),
		}
		_, err := service(git, nil).PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
		if err == nil || !strings.Contains(err.Error(), "remote branches failed") {
			t.Fatalf("PruneWorktrees() error = %v, want the propagated remote branch failure", err)
		}
	})

	t.Run("fails closed without pull-request state evidence", func(t *testing.T) {
		t.Parallel()
		git := &fakePruneGit{fakeGitRepository: &fakeGitRepository{}}
		_, err := NewWorktreeService(git).PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
		if countCall(git.calls, "worktree-list") != 0 {
			t.Fatalf("a blocked prune must not inventory: %v", git.calls)
		}
	})

	t.Run("fails closed without the remote branch listing capability", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeManagerOnlyGit{fakeGitRepository: &fakeGitRepository{}}
		_, err := NewWorktreeService(git).WithPullRequestInventory(&fakePullRequestInventory{}).
			PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
	})

	t.Run("fails closed when the composed adapter carries no worktree capability", func(t *testing.T) {
		t.Parallel()
		_, err := NewWorktreeService(&fakeGitRepository{}).WithPullRequestInventory(&fakePullRequestInventory{}).
			PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("fails closed without a composed Git adapter", func(t *testing.T) {
		t.Parallel()
		_, err := NewWorktreeService(nil).WithPullRequestInventory(&fakePullRequestInventory{}).
			PruneWorktrees(context.Background(), PruneWorktreesRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeInternal)
	})
}
