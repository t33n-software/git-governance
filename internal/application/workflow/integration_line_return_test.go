package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
)

// integrationLineReturnGit controls exactly the three Git capabilities the
// post-publication integration-line transition depends on and records every
// switch attempt.
type integrationLineReturnGit struct {
	*fakeGitRepository
	current     branch.BranchName
	currentErr  error
	worktreeErr error
	switchErr   error
	switchedTo  []branch.BranchName
}

func (git *integrationLineReturnGit) CurrentBranch(context.Context, port.RepositoryIdentity) (branch.BranchName, error) {
	git.calls = append(git.calls, "current-branch")
	return git.current, git.currentErr
}

func (git *integrationLineReturnGit) IsWorktreeClean(context.Context, port.RepositoryIdentity) (bool, error) {
	git.calls = append(git.calls, "worktree-clean")
	return git.clean, git.worktreeErr
}

func (git *integrationLineReturnGit) SwitchBranch(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName) error {
	git.calls = append(git.calls, "switch")
	git.switchedTo = append(git.switchedTo, name)
	return git.switchErr
}

// integrationLineReturnRefreshGit adds the optional fast-forward capability
// to the transition fake and records every refresh attempt.
type integrationLineReturnRefreshGit struct {
	*integrationLineReturnGit
	refreshOutcome port.FastForwardOutcome
	refreshErr     error
	refreshCalls   int
}

func (git *integrationLineReturnRefreshGit) FastForwardBranch(_ context.Context, _ port.RepositoryIdentity, _ branch.BranchName, _ branch.TargetBase) (port.FastForwardOutcome, error) {
	git.calls = append(git.calls, "fast-forward")
	git.refreshCalls++
	return git.refreshOutcome, git.refreshErr
}

// publishRefreshGit adds the optional fast-forward capability to the shared
// fake so publication-level tests can observe the refresh outcome.
type publishRefreshGit struct {
	*fakeGitRepository
	outcome port.FastForwardOutcome
}

func (git *publishRefreshGit) FastForwardBranch(_ context.Context, _ port.RepositoryIdentity, _ branch.BranchName, _ branch.TargetBase) (port.FastForwardOutcome, error) {
	git.calls = append(git.calls, "fast-forward")
	return git.outcome, nil
}

func TestReturnToIntegrationLine(t *testing.T) {
	t.Parallel()

	t.Run("fails when the current branch cannot be determined", func(t *testing.T) {
		t.Parallel()
		git := &integrationLineReturnGit{
			fakeGitRepository: &fakeGitRepository{clean: true},
			currentErr:        errors.New("current branch unavailable"),
		}
		result := returnToIntegrationLine(context.Background(), git, testRepository())
		if result.Status != IntegrationLineReturnFailed ||
			result.Branch.String() != "develop" ||
			!strings.Contains(result.Detail, "determine the current branch") {
			t.Fatalf("returnToIntegrationLine() = %#v", result)
		}
		if len(git.switchedTo) != 0 {
			t.Fatalf("unexpected switch attempt: %v", git.switchedTo)
		}
	})

	t.Run("reports already-home without inspecting or switching", func(t *testing.T) {
		t.Parallel()
		git := &integrationLineReturnGit{
			fakeGitRepository: &fakeGitRepository{clean: true},
			current:           mustBranch("develop"),
		}
		result := returnToIntegrationLine(context.Background(), git, testRepository())
		if result.Status != IntegrationLineReturnAlreadyHome || result.Branch.String() != "develop" || result.Detail != "" {
			t.Fatalf("returnToIntegrationLine() = %#v", result)
		}
		calls := strings.Join(git.calls, ",")
		if strings.Contains(calls, "worktree-clean") || strings.Contains(calls, "switch") {
			t.Fatalf("already-home must not inspect or switch: %v", git.calls)
		}
	})

	t.Run("fails when the worktree state cannot be inspected", func(t *testing.T) {
		t.Parallel()
		git := &integrationLineReturnGit{
			fakeGitRepository: &fakeGitRepository{clean: true},
			current:           mustBranch("feature/ABC-123-add-export"),
			worktreeErr:       errors.New("worktree inspection failed"),
		}
		result := returnToIntegrationLine(context.Background(), git, testRepository())
		if result.Status != IntegrationLineReturnFailed ||
			!strings.Contains(result.Detail, "inspect the worktree state") {
			t.Fatalf("returnToIntegrationLine() = %#v", result)
		}
		if len(git.switchedTo) != 0 {
			t.Fatalf("unexpected switch attempt: %v", git.switchedTo)
		}
	})

	t.Run("skips the transition and preserves a dirty worktree", func(t *testing.T) {
		t.Parallel()
		git := &integrationLineReturnGit{
			fakeGitRepository: &fakeGitRepository{clean: false},
			current:           mustBranch("feature/ABC-123-add-export"),
		}
		result := returnToIntegrationLine(context.Background(), git, testRepository())
		if result.Status != IntegrationLineReturnSkippedDirtyWorktree ||
			result.Branch.String() != "develop" ||
			!strings.Contains(result.Detail, "preserved") {
			t.Fatalf("returnToIntegrationLine() = %#v", result)
		}
		if len(git.switchedTo) != 0 {
			t.Fatalf("a dirty worktree must never be switched: %v", git.switchedTo)
		}
	})

	t.Run("reports a failed switch without questioning the publication", func(t *testing.T) {
		t.Parallel()
		git := &integrationLineReturnGit{
			fakeGitRepository: &fakeGitRepository{clean: true},
			current:           mustBranch("feature/ABC-123-add-export"),
			switchErr:         errors.New("switch failed"),
		}
		result := returnToIntegrationLine(context.Background(), git, testRepository())
		if result.Status != IntegrationLineReturnFailed ||
			!strings.Contains(result.Detail, "switch to the integration line") {
			t.Fatalf("returnToIntegrationLine() = %#v", result)
		}
		if len(git.switchedTo) != 1 || git.switchedTo[0].String() != "develop" {
			t.Fatalf("switch attempts = %v", git.switchedTo)
		}
	})

	t.Run("switches back to the integration line", func(t *testing.T) {
		t.Parallel()
		git := &integrationLineReturnGit{
			fakeGitRepository: &fakeGitRepository{clean: true},
			current:           mustBranch("feature/ABC-123-add-export"),
		}
		result := returnToIntegrationLine(context.Background(), git, testRepository())
		if result.Status != IntegrationLineReturnSwitched || result.Branch.String() != "develop" || result.Detail != "" {
			t.Fatalf("returnToIntegrationLine() = %#v", result)
		}
		if len(git.switchedTo) != 1 || git.switchedTo[0].String() != "develop" {
			t.Fatalf("switch attempts = %v", git.switchedTo)
		}
	})
}

func TestReturnToIntegrationLineRefresh(t *testing.T) {
	t.Parallel()

	t.Run("refreshes the local integration line after the switch", func(t *testing.T) {
		t.Parallel()
		git := &integrationLineReturnRefreshGit{
			integrationLineReturnGit: &integrationLineReturnGit{
				fakeGitRepository: &fakeGitRepository{clean: true},
				current:           mustBranch("feature/ABC-123-add-export"),
			},
			refreshOutcome: port.FastForwardUpdated,
		}
		result := returnToIntegrationLine(context.Background(), git, testRepository())
		if result.Status != IntegrationLineReturnSwitched ||
			result.Refresh != IntegrationLineRefreshUpdated ||
			result.RefreshDetail != "" {
			t.Fatalf("returnToIntegrationLine() = %#v", result)
		}
		if git.refreshCalls != 1 {
			t.Fatalf("refresh attempts = %d", git.refreshCalls)
		}
	})

	t.Run("reports an already current integration line", func(t *testing.T) {
		t.Parallel()
		git := &integrationLineReturnRefreshGit{
			integrationLineReturnGit: &integrationLineReturnGit{
				fakeGitRepository: &fakeGitRepository{clean: true},
				current:           mustBranch("feature/ABC-123-add-export"),
			},
			refreshOutcome: port.FastForwardAlreadyCurrent,
		}
		result := returnToIntegrationLine(context.Background(), git, testRepository())
		if result.Status != IntegrationLineReturnSwitched || result.Refresh != IntegrationLineRefreshAlreadyCurrent {
			t.Fatalf("returnToIntegrationLine() = %#v", result)
		}
	})

	t.Run("leaves a diverged integration line untouched", func(t *testing.T) {
		t.Parallel()
		git := &integrationLineReturnRefreshGit{
			integrationLineReturnGit: &integrationLineReturnGit{
				fakeGitRepository: &fakeGitRepository{clean: true},
				current:           mustBranch("feature/ABC-123-add-export"),
			},
			refreshOutcome: port.FastForwardDiverged,
		}
		result := returnToIntegrationLine(context.Background(), git, testRepository())
		if result.Status != IntegrationLineReturnSwitched ||
			result.Refresh != IntegrationLineRefreshDiverged ||
			!strings.Contains(result.RefreshDetail, "left untouched") {
			t.Fatalf("returnToIntegrationLine() = %#v", result)
		}
	})

	t.Run("keeps the transition valid when the refresh fails", func(t *testing.T) {
		t.Parallel()
		git := &integrationLineReturnRefreshGit{
			integrationLineReturnGit: &integrationLineReturnGit{
				fakeGitRepository: &fakeGitRepository{clean: true},
				current:           mustBranch("feature/ABC-123-add-export"),
			},
			refreshErr: errors.New("merge refused"),
		}
		result := returnToIntegrationLine(context.Background(), git, testRepository())
		if result.Status != IntegrationLineReturnSwitched ||
			result.Refresh != IntegrationLineRefreshFailed ||
			!strings.Contains(result.RefreshDetail, "refresh the local integration line") {
			t.Fatalf("returnToIntegrationLine() = %#v", result)
		}
	})

	t.Run("refreshes when the workspace is already home", func(t *testing.T) {
		t.Parallel()
		git := &integrationLineReturnRefreshGit{
			integrationLineReturnGit: &integrationLineReturnGit{
				fakeGitRepository: &fakeGitRepository{clean: true},
				current:           mustBranch("develop"),
			},
			refreshOutcome: port.FastForwardUpdated,
		}
		result := returnToIntegrationLine(context.Background(), git, testRepository())
		if result.Status != IntegrationLineReturnAlreadyHome || result.Refresh != IntegrationLineRefreshUpdated {
			t.Fatalf("returnToIntegrationLine() = %#v", result)
		}
		if len(git.switchedTo) != 0 {
			t.Fatalf("already-home must not switch: %v", git.switchedTo)
		}
	})

	t.Run("does not refresh a dirty worktree when already home", func(t *testing.T) {
		t.Parallel()
		git := &integrationLineReturnRefreshGit{
			integrationLineReturnGit: &integrationLineReturnGit{
				fakeGitRepository: &fakeGitRepository{clean: false},
				current:           mustBranch("develop"),
			},
			refreshOutcome: port.FastForwardUpdated,
		}
		result := returnToIntegrationLine(context.Background(), git, testRepository())
		if result.Status != IntegrationLineReturnAlreadyHome ||
			result.Refresh != "" ||
			!strings.Contains(result.RefreshDetail, "not refreshed") {
			t.Fatalf("returnToIntegrationLine() = %#v", result)
		}
		if git.refreshCalls != 0 {
			t.Fatalf("a dirty worktree must never be refreshed: %d", git.refreshCalls)
		}
	})

	t.Run("reports a failed worktree inspection before the refresh", func(t *testing.T) {
		t.Parallel()
		git := &integrationLineReturnRefreshGit{
			integrationLineReturnGit: &integrationLineReturnGit{
				fakeGitRepository: &fakeGitRepository{clean: true},
				current:           mustBranch("develop"),
				worktreeErr:       errors.New("worktree inspection failed"),
			},
			refreshOutcome: port.FastForwardUpdated,
		}
		result := returnToIntegrationLine(context.Background(), git, testRepository())
		if result.Status != IntegrationLineReturnAlreadyHome ||
			result.Refresh != IntegrationLineRefreshFailed ||
			!strings.Contains(result.RefreshDetail, "inspect the worktree state") {
			t.Fatalf("returnToIntegrationLine() = %#v", result)
		}
		if git.refreshCalls != 0 {
			t.Fatalf("refresh attempts = %d", git.refreshCalls)
		}
	})

	t.Run("never refreshes when the transition was skipped", func(t *testing.T) {
		t.Parallel()
		git := &integrationLineReturnRefreshGit{
			integrationLineReturnGit: &integrationLineReturnGit{
				fakeGitRepository: &fakeGitRepository{clean: false},
				current:           mustBranch("feature/ABC-123-add-export"),
			},
			refreshOutcome: port.FastForwardUpdated,
		}
		result := returnToIntegrationLine(context.Background(), git, testRepository())
		if result.Status != IntegrationLineReturnSkippedDirtyWorktree || result.Refresh != "" {
			t.Fatalf("returnToIntegrationLine() = %#v", result)
		}
		if git.refreshCalls != 0 {
			t.Fatalf("a skipped transition must never refresh: %d", git.refreshCalls)
		}
	})

	t.Run("never refreshes after a failed switch", func(t *testing.T) {
		t.Parallel()
		git := &integrationLineReturnRefreshGit{
			integrationLineReturnGit: &integrationLineReturnGit{
				fakeGitRepository: &fakeGitRepository{clean: true},
				current:           mustBranch("feature/ABC-123-add-export"),
				switchErr:         errors.New("switch failed"),
			},
			refreshOutcome: port.FastForwardUpdated,
		}
		result := returnToIntegrationLine(context.Background(), git, testRepository())
		if result.Status != IntegrationLineReturnFailed || result.Refresh != "" {
			t.Fatalf("returnToIntegrationLine() = %#v", result)
		}
		if git.refreshCalls != 0 {
			t.Fatalf("a failed switch must never refresh: %d", git.refreshCalls)
		}
	})
}

func TestPublishPullRequestIntegrationLineReturn(t *testing.T) {
	t.Parallel()

	request := port.PullRequest{
		Source: mustBranch("feature/ABC-123-add-export"),
		Target: mustBranch("develop"),
		Title:  "ABC-123: add-export",
		Body:   "Summary: Add the export button.",
	}

	t.Run("leaves the workspace untouched when the transition is disabled", func(t *testing.T) {
		t.Parallel()
		git := &fakeGitRepository{clean: true}
		publisher := &fakePublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/1"}}
		result, err := publishPullRequest(context.Background(), git, publisher, testRepository(), request, false)
		if err != nil {
			t.Fatal(err)
		}
		if result.URL != "https://example.invalid/pr/1" || result.IntegrationLineReturn != nil {
			t.Fatalf("publishPullRequest() = %#v", result)
		}
		if strings.Contains(strings.Join(git.calls, ","), "switch") {
			t.Fatalf("disabled transition must not switch: %v", git.calls)
		}
	})

	t.Run("returns the workspace to the integration line when enabled", func(t *testing.T) {
		t.Parallel()
		git := &fakeGitRepository{clean: true}
		publisher := &fakePublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/1"}}
		result, err := publishPullRequest(context.Background(), git, publisher, testRepository(), request, true)
		if err != nil {
			t.Fatal(err)
		}
		if result.IntegrationLineReturn == nil ||
			result.IntegrationLineReturn.Status != IntegrationLineReturnSwitched ||
			result.IntegrationLineReturn.Branch.String() != "develop" {
			t.Fatalf("publishPullRequest() = %#v", result)
		}
		if !strings.Contains(strings.Join(git.calls, ","), "switch") {
			t.Fatalf("enabled transition must switch: %v", git.calls)
		}
	})

	t.Run("never transitions after a failed publication", func(t *testing.T) {
		t.Parallel()
		publishErr := errors.New("publisher failed")
		git := &fakeGitRepository{clean: true}
		_, err := publishPullRequest(context.Background(), git, &fakePublisher{err: publishErr}, testRepository(), request, true)
		if !errors.Is(err, publishErr) {
			t.Fatalf("publishPullRequest() error = %v, want %v", err, publishErr)
		}
		if strings.Contains(strings.Join(git.calls, ","), "switch") {
			t.Fatalf("a failed publication must not transition: %v", git.calls)
		}
	})

	t.Run("carries the refresh outcome when the adapter offers the capability", func(t *testing.T) {
		t.Parallel()
		git := &publishRefreshGit{fakeGitRepository: &fakeGitRepository{clean: true}, outcome: port.FastForwardUpdated}
		publisher := &fakePublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/1"}}
		result, err := publishPullRequest(context.Background(), git, publisher, testRepository(), request, true)
		if err != nil {
			t.Fatal(err)
		}
		if result.IntegrationLineReturn == nil ||
			result.IntegrationLineReturn.Status != IntegrationLineReturnSwitched ||
			result.IntegrationLineReturn.Refresh != IntegrationLineRefreshUpdated {
			t.Fatalf("publishPullRequest() = %#v", result)
		}
	})
}

func TestTicketServicePublishPullRequestIntegrationLineReturn(t *testing.T) {
	t.Parallel()

	request := port.PullRequest{
		Source: mustBranch("feature/ABC-123-add-export"),
		Target: mustBranch("develop"),
		Title:  "ABC-123: add-export",
		Body:   "Summary: Add the export button.",
	}

	t.Run("keeps the transition disabled by default", func(t *testing.T) {
		t.Parallel()
		git := &fakeGitRepository{clean: true}
		service := newTicketService(git, nil, &fakePublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/1"}})
		result, err := service.PublishPullRequest(context.Background(), testRepository(), request)
		if err != nil {
			t.Fatal(err)
		}
		if result.IntegrationLineReturn != nil {
			t.Fatalf("unexpected transition: %#v", result.IntegrationLineReturn)
		}
	})

	t.Run("builder toggles the transition in both directions", func(t *testing.T) {
		t.Parallel()
		git := &fakeGitRepository{clean: true}
		service := newTicketService(git, nil, &fakePublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/1"}})
		if returned := service.WithIntegrationLineReturn(true); returned != service {
			t.Fatal("WithIntegrationLineReturn must return the same service for chaining")
		}
		result, err := service.PublishPullRequest(context.Background(), testRepository(), request)
		if err != nil {
			t.Fatal(err)
		}
		if result.IntegrationLineReturn == nil || result.IntegrationLineReturn.Status != IntegrationLineReturnSwitched {
			t.Fatalf("PublishPullRequest() = %#v", result)
		}
		service.WithIntegrationLineReturn(false)
		result, err = service.PublishPullRequest(context.Background(), testRepository(), request)
		if err != nil {
			t.Fatal(err)
		}
		if result.IntegrationLineReturn != nil {
			t.Fatalf("transition still active after disable: %#v", result.IntegrationLineReturn)
		}
	})
}

func TestPublishTicketIntegrationLineReturn(t *testing.T) {
	t.Parallel()

	publish := func(service *TicketService) PublishTicketResult {
		result, err := service.PublishTicket(context.Background(), PublishTicketRequest{
			Repository:        testRepository(),
			Branch:            mustBranch("feature/ABC-123-add-export"),
			Push:              true,
			CreatePullRequest: true,
			Body:              "Summary: Add the export button.",
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	t.Run("carries the transition outcome when enabled", func(t *testing.T) {
		t.Parallel()
		git := &fakeGitRepository{
			clean:       true,
			publication: branch.PublicationUnpublished,
			messages:    []string{"feat(ABC-123): add export"},
		}
		service := newTicketService(git, &fakeQualityRunner{}, &fakePublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/1"}}).
			WithIntegrationLineReturn(true)
		result := publish(service)
		if result.PublishedURL != "https://example.invalid/pr/1" ||
			result.IntegrationLineReturn == nil ||
			result.IntegrationLineReturn.Status != IntegrationLineReturnSwitched {
			t.Fatalf("PublishTicket() = %#v", result)
		}
	})

	t.Run("omits the transition when disabled", func(t *testing.T) {
		t.Parallel()
		git := &fakeGitRepository{
			clean:       true,
			publication: branch.PublicationUnpublished,
			messages:    []string{"feat(ABC-123): add export"},
		}
		service := newTicketService(git, &fakeQualityRunner{}, &fakePublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/1"}})
		result := publish(service)
		if result.PublishedURL != "https://example.invalid/pr/1" || result.IntegrationLineReturn != nil {
			t.Fatalf("PublishTicket() = %#v", result)
		}
	})
}

func TestPrepareReleasePromotionIntegrationLineReturn(t *testing.T) {
	t.Parallel()

	request := func() PrepareReleasePromotionRequest {
		return PrepareReleasePromotionRequest{
			Repository:        testRepository(),
			Release:           mustBranch("release/2.8.0"),
			CreatePullRequest: true,
			Body:              "Summary: Promote release 2.8.0 into main.",
		}
	}

	t.Run("returns the workspace to the integration line when enabled", func(t *testing.T) {
		t.Parallel()
		git := newReleaseWhiteboxGit()
		publisher := &releaseWhiteboxPublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/promotion"}}
		service := newReleaseWhiteboxService(git, publisher)
		if returned := service.WithIntegrationLineReturn(true); returned != service {
			t.Fatal("WithIntegrationLineReturn must return the same service for chaining")
		}
		result, err := service.PrepareReleasePromotion(context.Background(), request())
		if err != nil {
			t.Fatal(err)
		}
		if result.PublishedURL != "https://example.invalid/pr/promotion" ||
			result.IntegrationLineReturn == nil ||
			result.IntegrationLineReturn.Status != IntegrationLineReturnSwitched {
			t.Fatalf("PrepareReleasePromotion() = %#v", result)
		}
	})

	t.Run("omits the transition when disabled", func(t *testing.T) {
		t.Parallel()
		git := newReleaseWhiteboxGit()
		publisher := &releaseWhiteboxPublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/promotion"}}
		service := newReleaseWhiteboxService(git, publisher)
		result, err := service.PrepareReleasePromotion(context.Background(), request())
		if err != nil {
			t.Fatal(err)
		}
		if result.PublishedURL != "https://example.invalid/pr/promotion" || result.IntegrationLineReturn != nil {
			t.Fatalf("PrepareReleasePromotion() = %#v", result)
		}
	})
}

func TestPrepareReleaseBackmergeIntegrationLineReturn(t *testing.T) {
	t.Parallel()

	request := func() PrepareReleaseBackmergeRequest {
		return PrepareReleaseBackmergeRequest{
			Repository:        testRepository(),
			Release:           mustBranch("release/2.8.0"),
			CreatePullRequest: true,
			Body:              "Summary: Backmerge release 2.8.0 into develop.",
		}
	}

	t.Run("returns the workspace to the integration line when enabled", func(t *testing.T) {
		t.Parallel()
		git := newReleaseWhiteboxGit()
		publisher := &releaseWhiteboxPublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/backmerge"}}
		service := newReleaseWhiteboxService(git, publisher).WithIntegrationLineReturn(true)
		result, err := service.PrepareReleaseBackmerge(context.Background(), request())
		if err != nil {
			t.Fatal(err)
		}
		if result.PublishedURL != "https://example.invalid/pr/backmerge" ||
			result.IntegrationLineReturn == nil ||
			result.IntegrationLineReturn.Status != IntegrationLineReturnSwitched {
			t.Fatalf("PrepareReleaseBackmerge() = %#v", result)
		}
	})

	t.Run("omits the transition when disabled", func(t *testing.T) {
		t.Parallel()
		git := newReleaseWhiteboxGit()
		publisher := &releaseWhiteboxPublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/backmerge"}}
		service := newReleaseWhiteboxService(git, publisher)
		result, err := service.PrepareReleaseBackmerge(context.Background(), request())
		if err != nil {
			t.Fatal(err)
		}
		if result.PublishedURL != "https://example.invalid/pr/backmerge" || result.IntegrationLineReturn != nil {
			t.Fatalf("PrepareReleaseBackmerge() = %#v", result)
		}
	})
}

func TestPromotionAlignmentIntegrationLineReturn(t *testing.T) {
	t.Parallel()

	git := newPromotionAlignmentGit(t)
	quality := &fakeQualityRunner{}
	publisher := &promotionAlignmentPublisher{
		releaseWhiteboxPublisher: releaseWhiteboxPublisher{
			result: port.PublishedPullRequest{URL: "https://example.invalid/pr/alignment"},
		},
	}
	branches := branchapp.NewService(git, &fakeKeyPolicy{})
	tickets := NewTicketService(branches, branchapp.NewSynchronizer(git, branches, quality), git, quality, publisher).
		WithIntegrationLineReturn(true)
	service := NewReleaseService(branches, git, publisher).
		WithTicketService(tickets).
		WithQualityRunner(quality)

	request := promotionAlignmentRequest()
	request.Push = true
	request.CreatePullRequest = true
	result, err := service.AlignReleasePromotionBase(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.PublishedURL != "https://example.invalid/pr/alignment" ||
		result.IntegrationLineReturn == nil ||
		result.IntegrationLineReturn.Status != IntegrationLineReturnSwitched {
		t.Fatalf("AlignReleasePromotionBase() = %#v", result)
	}
}

func TestReconciliationAlignmentIntegrationLineReturn(t *testing.T) {
	t.Parallel()

	git := newReconciliationAlignmentGit(t)
	quality := &fakeQualityRunner{}
	publisher := &reconciliationAlignmentPublisher{
		releaseWhiteboxPublisher: releaseWhiteboxPublisher{
			result: port.PublishedPullRequest{URL: "https://example.invalid/pr/reconciliation"},
		},
	}
	lifecycle := &releaseWhiteboxLifecycle{evidence: requiredReconciliationEvidence()}
	branches := branchapp.NewService(git, &fakeKeyPolicy{})
	tickets := NewTicketService(branches, branchapp.NewSynchronizer(git, branches, quality), git, quality, publisher).
		WithIntegrationLineReturn(true)
	service := NewReleaseService(branches, git, publisher).
		WithTicketService(tickets).
		WithQualityRunner(quality).
		WithReleaseLifecycleProvider(lifecycle)

	request := reconciliationAlignmentRequest()
	request.Push = true
	request.CreatePullRequest = true
	result, err := service.AlignReleaseReconciliationBase(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.PublishedURL != "https://example.invalid/pr/reconciliation" ||
		result.IntegrationLineReturn == nil ||
		result.IntegrationLineReturn.Status != IntegrationLineReturnSwitched {
		t.Fatalf("AlignReleaseReconciliationBase() = %#v", result)
	}
}
