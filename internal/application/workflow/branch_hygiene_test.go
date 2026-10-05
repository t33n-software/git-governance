package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// fakeHygieneGit extends the shared workflow fake with the evidence surfaces
// the branch hygiene measures and records every deletion with its force form.
type fakeHygieneGit struct {
	*fakeGitRepository
	localBranches  []branch.BranchName
	localErr       error
	remoteBranches []branch.BranchName
	remoteErr      error
	worktrees      []port.WorktreeEntry
	worktreeErr    error
	aheadFor       map[string]int
	aheadErrFor    map[string]error
	deleted        []branch.BranchName
	deletedForce   []bool
	deleteErr      error
}

func (fake *fakeHygieneGit) LocalBranches(context.Context, port.RepositoryIdentity) ([]branch.BranchName, error) {
	fake.calls = append(fake.calls, "local-branches")
	if fake.localErr != nil {
		return nil, fake.localErr
	}
	return fake.localBranches, nil
}

func (fake *fakeHygieneGit) RemoteBranches(context.Context, port.RepositoryIdentity) ([]branch.BranchName, error) {
	fake.calls = append(fake.calls, "remote-branches")
	if fake.remoteErr != nil {
		return nil, fake.remoteErr
	}
	return fake.remoteBranches, nil
}

func (fake *fakeHygieneGit) WorktreeList(context.Context, port.RepositoryIdentity) ([]port.WorktreeEntry, error) {
	fake.calls = append(fake.calls, "worktree-list")
	if fake.worktreeErr != nil {
		return nil, fake.worktreeErr
	}
	return fake.worktrees, nil
}

func (fake *fakeHygieneGit) CountBranchAheadCommits(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName, _ branch.TargetBase) (int, error) {
	fake.calls = append(fake.calls, "branch-ahead")
	if err, bound := fake.aheadErrFor[name.String()]; bound {
		return 0, err
	}
	if ahead, bound := fake.aheadFor[name.String()]; bound {
		return ahead, nil
	}
	return 0, nil
}

func (fake *fakeHygieneGit) DeleteLocalBranch(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName, force bool) error {
	fake.calls = append(fake.calls, "delete-local-branch")
	if fake.deleteErr != nil {
		return fake.deleteErr
	}
	fake.deleted = append(fake.deleted, name)
	fake.deletedForce = append(fake.deletedForce, force)
	return nil
}

// fakeHygieneNoRemotesGit carries the worktree inventory without the remote
// branch surface, modeling a composition whose adapter cannot measure the
// branch-cleanup obligation.
type fakeHygieneNoRemotesGit struct {
	*fakeGitRepository
	worktrees []port.WorktreeEntry
}

func (fake *fakeHygieneNoRemotesGit) WorktreeList(context.Context, port.RepositoryIdentity) ([]port.WorktreeEntry, error) {
	return fake.worktrees, nil
}

// fakeHygieneNoWorktreesGit carries the remote branch surface without the
// worktree inventory, modeling a composition whose adapter cannot measure the
// checkout guard.
type fakeHygieneNoWorktreesGit struct {
	*fakeGitRepository
	remoteBranches []branch.BranchName
}

func (fake *fakeHygieneNoWorktreesGit) RemoteBranches(context.Context, port.RepositoryIdentity) ([]branch.BranchName, error) {
	return fake.remoteBranches, nil
}

// fakeHygieneNoAheadGit carries the remote and worktree surfaces without the
// ahead measurement, modeling a composition whose adapter cannot measure the
// null-ahead evidence.
type fakeHygieneNoAheadGit struct {
	*fakeGitRepository
	remoteBranches []branch.BranchName
	worktrees      []port.WorktreeEntry
}

func (fake *fakeHygieneNoAheadGit) RemoteBranches(context.Context, port.RepositoryIdentity) ([]branch.BranchName, error) {
	return fake.remoteBranches, nil
}

func (fake *fakeHygieneNoAheadGit) WorktreeList(context.Context, port.RepositoryIdentity) ([]port.WorktreeEntry, error) {
	return fake.worktrees, nil
}

func hygieneService(git port.GitRepository, records []port.PullRequestSummary) *BranchHygieneService {
	return NewBranchHygieneService(git).WithPullRequestInventory(&fakePullRequestInventory{records: records})
}

func hygieneEntryByBranch(entries []BranchHygieneEntry, name string) BranchHygieneEntry {
	for _, entry := range entries {
		if entry.Branch == name {
			return entry
		}
	}
	return BranchHygieneEntry{}
}

func TestRemoveBranch(t *testing.T) {
	t.Parallel()

	const taskBranch = "feature/GOV-138-branch-hygiene-endpoints"
	const hotfixBranch = "hotfix/GOV-139-patch-release-line"
	mergedRecord := port.PullRequestSummary{Number: "21", Title: "GOV-138: branch-hygiene-endpoints", State: port.PullRequestStateMerged}
	primaryEntry := port.WorktreeEntry{Path: testRepository().Root, Head: "abc", Branch: "develop"}

	t.Run("removes the worked-to-completion branch under the proven guard matrix", func(t *testing.T) {
		t.Parallel()
		git := &fakeHygieneGit{
			fakeGitRepository: &fakeGitRepository{},
			remoteBranches:    []branch.BranchName{},
			worktrees:         []port.WorktreeEntry{primaryEntry},
		}
		result, err := hygieneService(git, []port.PullRequestSummary{mergedRecord}).RemoveBranch(context.Background(), RemoveBranchRequest{
			Repository: testRepository(),
			Branch:     mustBranch(taskBranch),
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.Class != hygieneClassWorkedToCompletion || !result.Removed || result.Ticket != "GOV-138" ||
			result.PRState != "merged" || result.PRNumber != "21" || result.DryRun {
			t.Fatalf("the removal result = %#v", result)
		}
		if len(git.deleted) != 1 || git.deleted[0].String() != taskBranch {
			t.Fatalf("deletions = %v", git.deleted)
		}
		if len(git.deletedForce) != 1 || !git.deletedForce[0] {
			t.Fatalf("the guard-proven deletion runs forced: %v", git.deletedForce)
		}
		if countCall(git.calls, "delete-local-branch") != 1 {
			t.Fatalf("hygiene calls = %v", git.calls)
		}
	})

	t.Run("plans the removal without mutating during dry-run", func(t *testing.T) {
		t.Parallel()
		git := &fakeHygieneGit{
			fakeGitRepository: &fakeGitRepository{},
			remoteBranches:    []branch.BranchName{},
		}
		result, err := hygieneService(git, []port.PullRequestSummary{mergedRecord}).RemoveBranch(context.Background(), RemoveBranchRequest{
			Repository: testRepository(),
			Branch:     mustBranch(taskBranch),
			DryRun:     true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !result.DryRun || result.Removed || len(result.Plan) != 1 ||
			result.Plan[0].Action != "branch-delete" || result.Plan[0].Detail != "git branch -D "+taskBranch {
			t.Fatalf("dry-run result = %#v", result)
		}
		if len(git.deleted) != 0 {
			t.Fatalf("a dry run must not delete the branch: %v", git.deleted)
		}
	})

	t.Run("refuses the review continuation context of an open pull request", func(t *testing.T) {
		t.Parallel()
		git := &fakeHygieneGit{fakeGitRepository: &fakeGitRepository{}}
		_, err := hygieneService(git, []port.PullRequestSummary{
			{Number: "22", Title: "GOV-138: branch-hygiene-endpoints", State: port.PullRequestStateOpen},
		}).RemoveBranch(context.Background(), RemoveBranchRequest{Repository: testRepository(), Branch: mustBranch(taskBranch)})
		assertProblemCode(t, err, problem.CodeBranchPullRequestOpen)
		if len(git.deleted) != 0 {
			t.Fatalf("a published-open branch must never be deleted: %v", git.deleted)
		}
	})

	t.Run("refuses an abandoned branch for the actor decision", func(t *testing.T) {
		t.Parallel()
		git := &fakeHygieneGit{fakeGitRepository: &fakeGitRepository{}}
		_, err := hygieneService(git, []port.PullRequestSummary{
			{Number: "23", Title: "GOV-138: branch-hygiene-endpoints", State: port.PullRequestStateClosed},
		}).RemoveBranch(context.Background(), RemoveBranchRequest{Repository: testRepository(), Branch: mustBranch(taskBranch)})
		assertProblemCode(t, err, problem.CodeInvalidInput)
		classified, _ := problem.As(err)
		if !strings.Contains(classified.Rule, "belongs to the actor") {
			t.Fatalf("the abandoned refusal = %#v", classified)
		}
		if len(git.deleted) != 0 {
			t.Fatalf("an abandoned branch must never be deleted: %v", git.deleted)
		}
	})

	t.Run("refuses a branch whose remote branch-cleanup obligation is open", func(t *testing.T) {
		t.Parallel()
		git := &fakeHygieneGit{
			fakeGitRepository: &fakeGitRepository{},
			remoteBranches:    []branch.BranchName{mustBranch(taskBranch)},
		}
		_, err := hygieneService(git, []port.PullRequestSummary{mergedRecord}).RemoveBranch(context.Background(), RemoveBranchRequest{
			Repository: testRepository(),
			Branch:     mustBranch(taskBranch),
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)
		classified, _ := problem.As(err)
		if !strings.Contains(classified.Rule, "branch-cleanup obligation") {
			t.Fatalf("the obligation-open refusal = %#v", classified)
		}
		if len(git.deleted) != 0 {
			t.Fatalf("an obligation-open branch must never be deleted: %v", git.deleted)
		}
	})

	t.Run("refuses a branch that is checked out in a worktree", func(t *testing.T) {
		t.Parallel()
		git := &fakeHygieneGit{
			fakeGitRepository: &fakeGitRepository{},
			remoteBranches:    []branch.BranchName{},
			worktrees:         []port.WorktreeEntry{{Path: "C:/repo-GOV-138", Head: "def", Branch: taskBranch}},
		}
		_, err := hygieneService(git, []port.PullRequestSummary{mergedRecord}).RemoveBranch(context.Background(), RemoveBranchRequest{
			Repository: testRepository(),
			Branch:     mustBranch(taskBranch),
		})
		assertProblemCode(t, err, problem.CodeBranchCheckedOut)
		classified, _ := problem.As(err)
		if !strings.Contains(classified.Rule, "active working context") {
			t.Fatalf("the checked-out refusal = %#v", classified)
		}
		if len(git.deleted) != 0 {
			t.Fatalf("a checked-out branch must never be deleted: %v", git.deleted)
		}
	})

	t.Run("refuses deletion that would lose unpublished work", func(t *testing.T) {
		t.Parallel()
		git := &fakeHygieneGit{
			fakeGitRepository: &fakeGitRepository{},
			remoteBranches:    []branch.BranchName{},
			aheadFor:          map[string]int{taskBranch: 3},
		}
		_, err := hygieneService(git, nil).RemoveBranch(context.Background(), RemoveBranchRequest{
			Repository: testRepository(),
			Branch:     mustBranch(taskBranch),
		})
		assertProblemCode(t, err, problem.CodeBranchAheadUnmerged)
		classified, _ := problem.As(err)
		if !strings.Contains(classified.Actual, "3 commits ahead of origin/develop") {
			t.Fatalf("the ahead refusal = %#v", classified)
		}
		if len(git.deleted) != 0 {
			t.Fatalf("unpublished work must never be deleted: %v", git.deleted)
		}
	})

	t.Run("removes the never-published branch at null-ahead against the recorded lane base", func(t *testing.T) {
		t.Parallel()
		git := &fakeHygieneGit{
			fakeGitRepository: &fakeGitRepository{
				workflowBases: map[string]branch.TargetBase{taskBranch: mustBase("origin", "main")},
			},
			remoteBranches: []branch.BranchName{},
		}
		result, err := hygieneService(git, nil).RemoveBranch(context.Background(), RemoveBranchRequest{
			Repository: testRepository(),
			Branch:     mustBranch(taskBranch),
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.Class != hygieneClassNeverPublished || !result.Removed || result.Base != "origin/main" || result.Ahead != 0 {
			t.Fatalf("the never-published removal = %#v", result)
		}
	})

	t.Run("refuses a branch whose lane base is neither recorded nor derivable", func(t *testing.T) {
		t.Parallel()
		git := &fakeHygieneGit{
			fakeGitRepository: &fakeGitRepository{},
			remoteBranches:    []branch.BranchName{},
		}
		_, err := hygieneService(git, nil).RemoveBranch(context.Background(), RemoveBranchRequest{
			Repository: testRepository(),
			Branch:     mustBranch(hotfixBranch),
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)
		classified, _ := problem.As(err)
		if !strings.Contains(classified.Rule, "neither recorded nor derivable") {
			t.Fatalf("the unmeasurable-base refusal = %#v", classified)
		}
		if len(git.deleted) != 0 {
			t.Fatalf("an unmeasurable branch must never be deleted: %v", git.deleted)
		}
	})

	t.Run("keeps a pushed branch whose review outcome is unrecorded", func(t *testing.T) {
		t.Parallel()
		git := &fakeHygieneGit{
			fakeGitRepository: &fakeGitRepository{},
			remoteBranches:    []branch.BranchName{mustBranch(taskBranch)},
		}
		_, err := hygieneService(git, nil).RemoveBranch(context.Background(), RemoveBranchRequest{
			Repository: testRepository(),
			Branch:     mustBranch(taskBranch),
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)
		classified, _ := problem.As(err)
		if !strings.Contains(classified.Rule, "no pull request records the review outcome") ||
			!strings.Contains(classified.Rule, "remote surface") {
			t.Fatalf("the unrecorded-review refusal = %#v", classified)
		}
		if len(git.deleted) != 0 {
			t.Fatalf("an unreviewed branch must never be deleted: %v", git.deleted)
		}
	})

	t.Run("refuses shared lines and scratch branches as out of scope", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{"develop", "scratch/GOV-138-experiment"} {
			git := &fakeHygieneGit{fakeGitRepository: &fakeGitRepository{}}
			_, err := hygieneService(git, nil).RemoveBranch(context.Background(), RemoveBranchRequest{
				Repository: testRepository(),
				Branch:     mustBranch(name),
			})
			assertProblemCode(t, err, problem.CodeInvalidInput)
			classified, _ := problem.As(err)
			if !strings.Contains(classified.Rule, "only official working branches") {
				t.Fatalf("the out-of-scope refusal for %s = %#v", name, classified)
			}
			if len(git.deleted) != 0 {
				t.Fatalf("%s must never be deleted: %v", name, git.deleted)
			}
		}
	})

	t.Run("fails closed when the pull-request inventory capability is missing", func(t *testing.T) {
		t.Parallel()
		git := &fakeHygieneGit{fakeGitRepository: &fakeGitRepository{}}
		_, err := NewBranchHygieneService(git).RemoveBranch(context.Background(), RemoveBranchRequest{
			Repository: testRepository(),
			Branch:     mustBranch(taskBranch),
		})
		assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
		classified, _ := problem.As(err)
		if classified.Actual != "pull-request state evidence" {
			t.Fatalf("the capability refusal = %#v", classified)
		}
	})

	t.Run("fails closed when one evidence capability is missing", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name       string
			git        port.GitRepository
			capability string
		}{
			{"remote branch listing", &fakeHygieneNoRemotesGit{fakeGitRepository: &fakeGitRepository{}}, "remote branch listing"},
			{"worktree inventory listing", &fakeHygieneNoWorktreesGit{fakeGitRepository: &fakeGitRepository{}}, "worktree inventory listing"},
			{"branch ahead measurement", &fakeHygieneNoAheadGit{fakeGitRepository: &fakeGitRepository{}}, "branch ahead measurement"},
		}
		for _, testCase := range cases {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				t.Parallel()
				_, err := hygieneService(testCase.git, nil).RemoveBranch(context.Background(), RemoveBranchRequest{
					Repository: testRepository(),
					Branch:     mustBranch(taskBranch),
				})
				assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
				classified, _ := problem.As(err)
				if classified.Actual != testCase.capability {
					t.Fatalf("the capability refusal = %#v", classified)
				}
			})
		}
	})

	t.Run("propagates the shared evidence failures", func(t *testing.T) {
		t.Parallel()
		t.Run("remote url", func(t *testing.T) {
			t.Parallel()
			git := &fakeHygieneGit{fakeGitRepository: &fakeGitRepository{err: errors.New("remote-url failed")}}
			_, err := hygieneService(git, nil).RemoveBranch(context.Background(), RemoveBranchRequest{
				Repository: testRepository(),
				Branch:     mustBranch(taskBranch),
			})
			if err == nil || !strings.Contains(err.Error(), "remote-url failed") {
				t.Fatalf("the remote-url failure = %v", err)
			}
		})
		t.Run("provider inventory", func(t *testing.T) {
			t.Parallel()
			git := &fakeHygieneGit{fakeGitRepository: &fakeGitRepository{}}
			service := NewBranchHygieneService(git).WithPullRequestInventory(&fakePullRequestInventory{err: errors.New("provider failed")})
			_, err := service.RemoveBranch(context.Background(), RemoveBranchRequest{Repository: testRepository(), Branch: mustBranch(taskBranch)})
			if err == nil || !strings.Contains(err.Error(), "provider failed") {
				t.Fatalf("the provider failure = %v", err)
			}
		})
		t.Run("remote surface", func(t *testing.T) {
			t.Parallel()
			git := &fakeHygieneGit{
				fakeGitRepository: &fakeGitRepository{},
				remoteErr:         errors.New("remote failed"),
			}
			_, err := hygieneService(git, nil).RemoveBranch(context.Background(), RemoveBranchRequest{
				Repository: testRepository(),
				Branch:     mustBranch(taskBranch),
			})
			if err == nil || !strings.Contains(err.Error(), "remote failed") {
				t.Fatalf("the remote failure = %v", err)
			}
		})
		t.Run("worktree inventory", func(t *testing.T) {
			t.Parallel()
			git := &fakeHygieneGit{
				fakeGitRepository: &fakeGitRepository{},
				worktreeErr:       errors.New("worktrees failed"),
			}
			_, err := hygieneService(git, nil).RemoveBranch(context.Background(), RemoveBranchRequest{
				Repository: testRepository(),
				Branch:     mustBranch(taskBranch),
			})
			if err == nil || !strings.Contains(err.Error(), "worktrees failed") {
				t.Fatalf("the worktree failure = %v", err)
			}
		})
	})

	t.Run("keeps a branch whose lane-base measurement fails and names the cause", func(t *testing.T) {
		t.Parallel()
		git := &fakeHygieneGit{
			fakeGitRepository: &fakeGitRepository{workflowBaseErr: errors.New("base metadata failed")},
			remoteBranches:    []branch.BranchName{},
		}
		_, err := hygieneService(git, nil).RemoveBranch(context.Background(), RemoveBranchRequest{
			Repository: testRepository(),
			Branch:     mustBranch(taskBranch),
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)
		classified, _ := problem.As(err)
		if !strings.Contains(classified.Rule, "unmeasurable") || !strings.Contains(classified.Rule, "base metadata failed") {
			t.Fatalf("the unmeasurable-base refusal = %#v", classified)
		}
		if len(git.deleted) != 0 {
			t.Fatalf("an unmeasurable branch must never be deleted: %v", git.deleted)
		}
	})

	t.Run("propagates the deletion failure", func(t *testing.T) {
		t.Parallel()
		git := &fakeHygieneGit{
			fakeGitRepository: &fakeGitRepository{},
			remoteBranches:    []branch.BranchName{},
			deleteErr:         errors.New("delete failed"),
		}
		_, err := hygieneService(git, []port.PullRequestSummary{mergedRecord}).RemoveBranch(context.Background(), RemoveBranchRequest{
			Repository: testRepository(),
			Branch:     mustBranch(taskBranch),
		})
		if err == nil || !strings.Contains(err.Error(), "delete failed") {
			t.Fatalf("the deletion failure = %v", err)
		}
		if len(git.deleted) != 0 {
			t.Fatalf("a failed deletion must not record a removal: %v", git.deleted)
		}
	})

	t.Run("fails closed without the git surface", func(t *testing.T) {
		t.Parallel()
		_, err := NewBranchHygieneService(nil).RemoveBranch(context.Background(), RemoveBranchRequest{
			Repository: testRepository(),
			Branch:     mustBranch(taskBranch),
		})
		assertProblemCode(t, err, problem.CodeInternal)
	})
}

func TestPruneBranches(t *testing.T) {
	t.Parallel()

	const taskBranch = "feature/GOV-138-branch-hygiene-endpoints"
	const docsBranch = "docs/GOV-139-modularize-branch-metadata"
	const choreBranch = "chore/GOV-140-tidy-branch-metadata"
	const fixBranch = "fix/GOV-141-stabilize-release-line"
	primaryEntry := port.WorktreeEntry{Path: testRepository().Root, Head: "abc", Branch: "develop"}

	inventory := func(git *fakeHygieneGit, records []port.PullRequestSummary) *BranchHygieneService {
		return hygieneService(git, records)
	}
	pruneState := func() *fakeHygieneGit {
		return &fakeHygieneGit{
			fakeGitRepository: &fakeGitRepository{},
			localBranches: []branch.BranchName{
				mustBranch("develop"),
				mustBranch(taskBranch),
				mustBranch(docsBranch),
				mustBranch(choreBranch),
				mustBranch(fixBranch),
			},
			remoteBranches: []branch.BranchName{},
			worktrees:      []port.WorktreeEntry{primaryEntry},
			aheadFor:       map[string]int{fixBranch: 2},
		}
	}
	pruneRecords := []port.PullRequestSummary{
		{Number: "21", Title: "GOV-138: branch-hygiene-endpoints", State: port.PullRequestStateMerged},
		{Number: "22", Title: "docs(GOV-139): modularize branch metadata", State: port.PullRequestStateOpen},
	}

	t.Run("removes the proven-complete branches and keeps the others", func(t *testing.T) {
		t.Parallel()
		git := pruneState()
		result, err := inventory(git, pruneRecords).PruneBranches(context.Background(), PruneBranchesRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		shared := hygieneEntryByBranch(result.Entries, "develop")
		if shared.Class != hygieneClassOutOfScope || shared.Removed {
			t.Fatalf("the shared-line entry = %#v", shared)
		}
		complete := hygieneEntryByBranch(result.Entries, taskBranch)
		if complete.Class != hygieneClassWorkedToCompletion || !complete.Removed ||
			complete.PRState != "merged" || complete.PRNumber != "21" {
			t.Fatalf("the worked-to-completion entry = %#v", complete)
		}
		open := hygieneEntryByBranch(result.Entries, docsBranch)
		if open.Class != hygieneClassPublishedOpen || open.Removed || open.PRState != "open" {
			t.Fatalf("the published-open entry = %#v", open)
		}
		empty := hygieneEntryByBranch(result.Entries, choreBranch)
		if empty.Class != hygieneClassNeverPublished || !empty.Removed || empty.Base != "origin/develop" {
			t.Fatalf("the never-published entry = %#v", empty)
		}
		unpublished := hygieneEntryByBranch(result.Entries, fixBranch)
		if unpublished.Class != hygieneClassUnpublishedWork || unpublished.Removed || unpublished.Ahead != 2 {
			t.Fatalf("the unpublished-work entry = %#v", unpublished)
		}
		if len(git.deleted) != 2 || git.deleted[0].String() != taskBranch || git.deleted[1].String() != choreBranch {
			t.Fatalf("deletions = %v", git.deleted)
		}
		for _, force := range git.deletedForce {
			if !force {
				t.Fatalf("every guard-proven deletion runs forced: %v", git.deletedForce)
			}
		}
	})

	t.Run("plans without mutating during dry-run", func(t *testing.T) {
		t.Parallel()
		git := pruneState()
		result, err := inventory(git, pruneRecords).PruneBranches(context.Background(), PruneBranchesRequest{
			Repository: testRepository(),
			DryRun:     true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !result.DryRun || len(result.Plan) != 2 ||
			result.Plan[0].Detail != "git branch -D "+taskBranch ||
			result.Plan[1].Detail != "git branch -D "+choreBranch {
			t.Fatalf("dry-run plan = %#v", result.Plan)
		}
		for _, entry := range result.Entries {
			if entry.Removed {
				t.Fatalf("a dry run must not delete: %#v", entry)
			}
		}
		if len(git.deleted) != 0 {
			t.Fatalf("a dry run must not delete any branch: %v", git.deleted)
		}
	})

	t.Run("records a refused removal fail-closed", func(t *testing.T) {
		t.Parallel()
		git := pruneState()
		git.deleteErr = errors.New("delete failed")
		result, err := inventory(git, pruneRecords).PruneBranches(context.Background(), PruneBranchesRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		complete := hygieneEntryByBranch(result.Entries, taskBranch)
		if complete.Removed || !strings.Contains(complete.Reason, "removal refused") {
			t.Fatalf("the refused entry = %#v", complete)
		}
		empty := hygieneEntryByBranch(result.Entries, choreBranch)
		if empty.Removed || !strings.Contains(empty.Reason, "removal refused") {
			t.Fatalf("the refused entry = %#v", empty)
		}
		if len(git.deleted) != 0 {
			t.Fatalf("a refused removal must not record a removal: %v", git.deleted)
		}
	})

	t.Run("keeps a branch whose ahead evidence is unmeasurable and removes the next candidate", func(t *testing.T) {
		t.Parallel()
		git := pruneState()
		git.aheadErrFor = map[string]error{choreBranch: errors.New("ahead failed")}
		result, err := inventory(git, pruneRecords).PruneBranches(context.Background(), PruneBranchesRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		unmeasurable := hygieneEntryByBranch(result.Entries, choreBranch)
		if unmeasurable.Class != hygieneClassActive || unmeasurable.Removed ||
			!strings.Contains(unmeasurable.Reason, "unmeasurable") {
			t.Fatalf("the unmeasurable entry = %#v", unmeasurable)
		}
		complete := hygieneEntryByBranch(result.Entries, taskBranch)
		if complete.Class != hygieneClassWorkedToCompletion || !complete.Removed {
			t.Fatalf("the next candidate = %#v", complete)
		}
		if len(git.deleted) != 1 || git.deleted[0].String() != taskBranch {
			t.Fatalf("deletions = %v", git.deleted)
		}
	})

	t.Run("fails closed when the local branch listing capability is missing", func(t *testing.T) {
		t.Parallel()
		git := &fakeHygieneNoAheadGit{fakeGitRepository: &fakeGitRepository{}}
		_, err := hygieneService(git, nil).PruneBranches(context.Background(), PruneBranchesRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
		classified, _ := problem.As(err)
		if classified.Actual != "local branch listing" {
			t.Fatalf("the capability refusal = %#v", classified)
		}
	})

	t.Run("propagates the local branch listing failure", func(t *testing.T) {
		t.Parallel()
		git := &fakeHygieneGit{
			fakeGitRepository: &fakeGitRepository{},
			localErr:          errors.New("locals failed"),
		}
		_, err := hygieneService(git, nil).PruneBranches(context.Background(), PruneBranchesRequest{Repository: testRepository()})
		if err == nil || !strings.Contains(err.Error(), "locals failed") {
			t.Fatalf("the listing failure = %v", err)
		}
	})

	t.Run("propagates a shared evidence failure", func(t *testing.T) {
		t.Parallel()
		git := pruneState()
		git.remoteErr = errors.New("remote failed")
		_, err := inventory(git, pruneRecords).PruneBranches(context.Background(), PruneBranchesRequest{Repository: testRepository()})
		if err == nil || !strings.Contains(err.Error(), "remote failed") {
			t.Fatalf("the shared evidence failure = %v", err)
		}
	})

	t.Run("inventories an empty branch surface", func(t *testing.T) {
		t.Parallel()
		git := &fakeHygieneGit{
			fakeGitRepository: &fakeGitRepository{},
			localBranches:     []branch.BranchName{},
		}
		result, err := hygieneService(git, nil).PruneBranches(context.Background(), PruneBranchesRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Entries) != 0 || len(result.Plan) != 0 {
			t.Fatalf("the empty inventory = %#v", result)
		}
		if len(git.deleted) != 0 {
			t.Fatalf("an empty surface must never delete: %v", git.deleted)
		}
	})

	t.Run("fails closed without the git surface", func(t *testing.T) {
		t.Parallel()
		_, err := NewBranchHygieneService(nil).PruneBranches(context.Background(), PruneBranchesRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeInternal)
	})
}
