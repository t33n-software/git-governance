package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	branchapp "github.com/CyberT33N/git-governance/internal/application/branch"
	"github.com/CyberT33N/git-governance/internal/application/port"
	"github.com/CyberT33N/git-governance/internal/domain/branch"
	"github.com/CyberT33N/git-governance/internal/domain/problem"
)

func TestReleaseWhiteboxPublishesManifestOnlyThroughDedicatedBoundary(t *testing.T) {
	source := mustBranch("hotfix/ABC-999-payment-timeout")
	target := mustBranch("develop")
	manifest := []string{strings.Repeat("a", 40), strings.Repeat("b", 40)}
	record := releaseRecordWithManifest(t, source.String(), "main", manifest, []string{"develop"})
	request := PropagateHotfixManifestRequest{
		Repository: testRepository(),
		Source:     source,
		TargetLine: target,
		Publish:    true,
	}

	t.Run("rejects publication before candidate mutation without publisher boundary", func(t *testing.T) {
		git := &releaseManifestGit{releaseWhiteboxGit: newReleaseWhiteboxGit()}
		_, err := newReleaseWhiteboxService(git, &releaseWhiteboxPublisher{}).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithQualityRunner(&releaseWhiteboxQuality{}).
			PropagateHotfixManifest(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInvalidInput)
		if len(git.createdNames) != 0 {
			t.Fatalf("publication guard created branches: %#v", git.createdNames)
		}
	})

	t.Run("rejects publication when no provider is configured", func(t *testing.T) {
		git := &releaseManifestGit{releaseWhiteboxGit: newReleaseWhiteboxGit()}
		_, err := newReleaseWhiteboxService(git, nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithQualityRunner(&releaseWhiteboxQuality{}).
			WithHotfixManifestPublication(true).
			PropagateHotfixManifest(context.Background(), request)
		assertProblemCode(t, err, problem.CodeExternalCommandFailed)
	})

	t.Run("publishes the exact target-derived candidate after one final quality run", func(t *testing.T) {
		git := &releaseManifestGit{releaseWhiteboxGit: newReleaseWhiteboxGit()}
		quality := &releaseWhiteboxQuality{result: port.QualityResult{Status: port.QualityPassed}}
		publisher := &releaseWhiteboxPublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/999"}}
		service := newReleaseWhiteboxService(git, publisher)
		service.tickets.quality = quality

		result, err := service.
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithQualityRunner(quality).
			WithHotfixManifestPublication(true).
			PropagateHotfixManifest(context.Background(), request)
		if err != nil {
			t.Fatalf("PropagateHotfixManifest() error = %v", err)
		}
		if result.Publication == nil ||
			!result.Publication.Pushed ||
			result.Publication.PublishedURL != "https://example.invalid/pr/999" ||
			result.Quality == nil ||
			result.Quality.Status != port.QualityPassed {
			t.Fatalf("publication result = %#v", result)
		}
		if len(quality.requests) != 1 || quality.requests[0].Families[0] != branch.FamilyFix {
			t.Fatalf("quality requests = %#v", quality.requests)
		}
		if len(publisher.requests) != 1 ||
			publisher.requests[0].Source.String() != "fix/ABC-999-propagate-to-develop" ||
			publisher.requests[0].Target.String() != "develop" {
			t.Fatalf("publisher requests = %#v", publisher.requests)
		}
	})

	t.Run("propagates provider publication failure", func(t *testing.T) {
		git := &releaseManifestGit{releaseWhiteboxGit: newReleaseWhiteboxGit()}
		publicationFailure := errors.New("provider publication failed")
		quality := &releaseWhiteboxQuality{result: port.QualityResult{Status: port.QualityPassed}}
		service := newReleaseWhiteboxService(git, &releaseWhiteboxPublisher{err: publicationFailure})
		service.tickets.quality = quality
		_, err := service.
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithQualityRunner(quality).
			WithHotfixManifestPublication(true).
			PropagateHotfixManifest(context.Background(), request)
		if !errors.Is(err, publicationFailure) {
			t.Fatalf("publication error = %v, want %v", err, publicationFailure)
		}
	})
}

func TestReleaseWhiteboxPublishesResumedManifestOnlyThroughDedicatedBoundary(t *testing.T) {
	source := mustBranch("hotfix/ABC-999-payment-timeout")
	target := mustBranch("develop")
	candidate := mustBranch("fix/ABC-999-propagate-to-develop")
	manifest := []string{strings.Repeat("a", 40), strings.Repeat("b", 40)}
	record := releaseRecordWithManifest(t, source.String(), "main", manifest, []string{"develop"})
	base := newReleaseWhiteboxGit()
	base.active = true
	base.activeOperation = "cherry-pick"
	git := &releaseManifestGit{
		releaseWhiteboxGit: base,
		current:            candidate,
		progressFound:      true,
		progress: port.HotfixManifestProgress{
			Branch:   candidate,
			Source:   source,
			Target:   target,
			Manifest: manifest,
		},
	}
	quality := &releaseWhiteboxQuality{result: port.QualityResult{Status: port.QualityPassed}}
	publisher := &releaseWhiteboxPublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/1000"}}
	service := newReleaseWhiteboxService(git, publisher)
	service.tickets.quality = quality

	result, err := service.
		WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
		WithQualityRunner(quality).
		WithHotfixManifestPublication(true).
		ResumeHotfixManifestPropagation(context.Background(), ResumeHotfixManifestPropagationRequest{
			Repository: testRepository(),
			Source:     source,
			TargetLine: target,
			Branch:     candidate,
			Publish:    true,
		})
	if err != nil {
		t.Fatalf("ResumeHotfixManifestPropagation() error = %v", err)
	}
	if result.Publication == nil || !result.Publication.Pushed || result.Publication.PublishedURL == "" {
		t.Fatalf("resume publication = %#v", result)
	}
}

func TestReleaseWhiteboxHotfixManifestPublicationFailurePaths(t *testing.T) {
	source := mustBranch("hotfix/ABC-999-payment-timeout")
	target := mustBranch("develop")
	candidate := mustBranch("fix/ABC-999-propagate-to-develop")
	manifest := []string{strings.Repeat("a", 40), strings.Repeat("b", 40)}
	record := releaseRecordWithManifest(t, source.String(), "main", manifest, []string{"develop"})
	request := PropagateHotfixManifestRequest{
		Repository: testRepository(),
		Source:     source,
		TargetLine: target,
		Publish:    true,
	}

	newServiceWithoutTickets := func(git port.GitRepository) *ReleaseService {
		return NewReleaseService(branchapp.NewService(git, &fakeKeyPolicy{}), git, nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithQualityRunner(&releaseWhiteboxQuality{}).
			WithHotfixManifestPublication(true)
	}

	t.Run("requires a publication service for new and resumed candidates", func(t *testing.T) {
		git := &releaseManifestGit{releaseWhiteboxGit: newReleaseWhiteboxGit()}
		_, err := newServiceWithoutTickets(git).PropagateHotfixManifest(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInternal)

		_, err = newServiceWithoutTickets(git).ResumeHotfixManifestPropagation(context.Background(), ResumeHotfixManifestPropagationRequest{
			Repository: testRepository(),
			Source:     source,
			TargetLine: target,
			Branch:     candidate,
			Publish:    true,
		})
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("rejects resumed publication without the dedicated boundary", func(t *testing.T) {
		git := &releaseManifestGit{releaseWhiteboxGit: newReleaseWhiteboxGit()}
		_, err := newReleaseWhiteboxService(git, &releaseWhiteboxPublisher{}).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithQualityRunner(&releaseWhiteboxQuality{}).
			ResumeHotfixManifestPropagation(context.Background(), ResumeHotfixManifestPropagationRequest{
				Repository: testRepository(),
				Source:     source,
				TargetLine: target,
				Branch:     candidate,
				Publish:    true,
			})
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("preserves candidate and publisher failures after a resumed manifest", func(t *testing.T) {
		newResumedGit := func() *releaseManifestGit {
			base := newReleaseWhiteboxGit()
			base.active = true
			base.activeOperation = "cherry-pick"
			return &releaseManifestGit{
				releaseWhiteboxGit: base,
				current:            candidate,
				progressFound:      true,
				progress: port.HotfixManifestProgress{
					Branch:   candidate,
					Source:   source,
					Target:   target,
					Manifest: manifest,
				},
			}
		}

		clearFailure := errors.New("clear progress")
		git := newResumedGit()
		git.clearErr = clearFailure
		service := newReleaseWhiteboxService(git, &releaseWhiteboxPublisher{})
		service.tickets.quality = &releaseWhiteboxQuality{result: port.QualityResult{Status: port.QualityPassed}}
		_, err := service.
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithQualityRunner(service.tickets.quality).
			WithHotfixManifestPublication(true).
			ResumeHotfixManifestPropagation(context.Background(), ResumeHotfixManifestPropagationRequest{
				Repository: testRepository(),
				Source:     source,
				TargetLine: target,
				Branch:     candidate,
				Publish:    true,
			})
		if !errors.Is(err, clearFailure) {
			t.Fatalf("clear error = %v, want %v", err, clearFailure)
		}

		publicationFailure := errors.New("publisher failure")
		git = newResumedGit()
		service = newReleaseWhiteboxService(git, &releaseWhiteboxPublisher{err: publicationFailure})
		service.tickets.quality = &releaseWhiteboxQuality{result: port.QualityResult{Status: port.QualityPassed}}
		_, err = service.
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithQualityRunner(service.tickets.quality).
			WithHotfixManifestPublication(true).
			ResumeHotfixManifestPropagation(context.Background(), ResumeHotfixManifestPropagationRequest{
				Repository: testRepository(),
				Source:     source,
				TargetLine: target,
				Branch:     candidate,
				Publish:    true,
			})
		if !errors.Is(err, publicationFailure) {
			t.Fatalf("publication error = %v, want %v", err, publicationFailure)
		}
	})

	t.Run("fails closed for direct publication helper misuse", func(t *testing.T) {
		service := newReleaseWhiteboxService(newReleaseWhiteboxGit(), &releaseWhiteboxPublisher{})
		_, err := service.publishHotfixManifestCandidate(context.Background(), testRepository(), candidate, target)
		assertProblemCode(t, err, problem.CodeInvalidInput)

		service = NewReleaseService(branchapp.NewService(newReleaseWhiteboxGit(), &fakeKeyPolicy{}), newReleaseWhiteboxGit(), nil).
			WithHotfixManifestPublication(true)
		_, err = service.publishHotfixManifestCandidate(context.Background(), testRepository(), candidate, target)
		assertProblemCode(t, err, problem.CodeInternal)

		service = newReleaseWhiteboxService(newReleaseWhiteboxGit(), &hotfixPropagationPreflightPublisher{err: errors.New("preflight failed")}).
			WithHotfixManifestPublication(true)
		_, err = service.publishHotfixManifestCandidate(context.Background(), testRepository(), candidate, branch.BranchName{})
		if err == nil {
			t.Fatal("publication helper accepted an invalid target")
		}
		_, err = service.publishHotfixManifestCandidate(context.Background(), testRepository(), candidate, target)
		if err == nil || !strings.Contains(err.Error(), "preflight failed") {
			t.Fatalf("preflight error = %v", err)
		}
	})
}

type hotfixPropagationPreflightPublisher struct {
	err error
}

func (publisher *hotfixPropagationPreflightPublisher) Publish(context.Context, port.PullRequestPublication) (port.PublishedPullRequest, error) {
	return port.PublishedPullRequest{}, publisher.err
}

func (publisher *hotfixPropagationPreflightPublisher) Validate(context.Context, port.PullRequestPublication) error {
	return publisher.err
}
