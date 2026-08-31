package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/hotfix"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

type releaseWhiteboxGit struct {
	*fakeGitRepository

	validateErr       error
	validateErrors    []error
	hasCommitsErr     error
	cleanErr          error
	branchExistsErr   error
	createErr         error
	storeErr          error
	releaseTagsErr    error
	cherryPickErr     error
	deleteErr         error
	clearErr          error
	commitMessagesErr error
	fetchErrors       []error

	validateContexts    []context.Context
	hasCommitsContexts  []context.Context
	fetchContexts       []context.Context
	storeContexts       []context.Context
	cherryPickContexts  []context.Context
	deleteContexts      []context.Context
	clearContexts       []context.Context
	releaseTagRevisions []string
	deletedBranches     []branch.BranchName
	deleteForces        []bool
}

func newReleaseWhiteboxGit() *releaseWhiteboxGit {
	return &releaseWhiteboxGit{
		fakeGitRepository: &fakeGitRepository{
			hasCommits:  true,
			clean:       true,
			publication: branch.PublicationUnpublished,
			messages:    []string{"fix(ABC-999): resolve payment timeout"},
		},
	}
}

func (git *releaseWhiteboxGit) ValidateBranchRef(ctx context.Context, repository port.RepositoryIdentity, name branch.BranchName) error {
	git.validateContexts = append(git.validateContexts, ctx)
	if len(git.validateErrors) > 0 {
		err := git.validateErrors[0]
		git.validateErrors = git.validateErrors[1:]
		if err != nil {
			git.calls = append(git.calls, "validate-ref")
			return err
		}
	}
	if git.validateErr != nil {
		git.calls = append(git.calls, "validate-ref")
		return git.validateErr
	}
	return git.fakeGitRepository.ValidateBranchRef(ctx, repository, name)
}

func (git *releaseWhiteboxGit) HasCommits(ctx context.Context, repository port.RepositoryIdentity) (bool, error) {
	git.hasCommitsContexts = append(git.hasCommitsContexts, ctx)
	if git.hasCommitsErr != nil {
		git.calls = append(git.calls, "has-commits")
		return false, git.hasCommitsErr
	}
	return git.fakeGitRepository.HasCommits(ctx, repository)
}

func (git *releaseWhiteboxGit) IsWorktreeClean(ctx context.Context, repository port.RepositoryIdentity) (bool, error) {
	if git.cleanErr != nil {
		git.calls = append(git.calls, "worktree-clean")
		return false, git.cleanErr
	}
	return git.fakeGitRepository.IsWorktreeClean(ctx, repository)
}

func (git *releaseWhiteboxGit) BranchExists(ctx context.Context, repository port.RepositoryIdentity, name branch.BranchName) (bool, error) {
	if git.branchExistsErr != nil {
		git.calls = append(git.calls, "branch-exists")
		return false, git.branchExistsErr
	}
	return git.fakeGitRepository.BranchExists(ctx, repository, name)
}

func (git *releaseWhiteboxGit) Fetch(ctx context.Context, repository port.RepositoryIdentity) error {
	git.fetchContexts = append(git.fetchContexts, ctx)
	if len(git.fetchErrors) > 0 {
		err := git.fetchErrors[0]
		git.fetchErrors = git.fetchErrors[1:]
		if err != nil {
			git.calls = append(git.calls, "fetch")
			return err
		}
	}
	return git.fakeGitRepository.Fetch(ctx, repository)
}

func (git *releaseWhiteboxGit) CreateBranch(
	ctx context.Context,
	repository port.RepositoryIdentity,
	name branch.BranchName,
	base branch.TargetBase,
	switchTo bool,
) error {
	if git.createErr != nil {
		git.calls = append(git.calls, "create-branch")
		return git.createErr
	}
	return git.fakeGitRepository.CreateBranch(ctx, repository, name, base, switchTo)
}

func (git *releaseWhiteboxGit) StoreWorkflowBase(
	ctx context.Context,
	repository port.RepositoryIdentity,
	name branch.BranchName,
	base branch.TargetBase,
) error {
	git.storeContexts = append(git.storeContexts, ctx)
	if git.storeErr != nil {
		git.calls = append(git.calls, "store-workflow-base")
		return git.storeErr
	}
	return git.fakeGitRepository.StoreWorkflowBase(ctx, repository, name, base)
}

func (git *releaseWhiteboxGit) ReleaseTagsAt(
	ctx context.Context,
	repository port.RepositoryIdentity,
	revision string,
) ([]string, error) {
	git.releaseTagRevisions = append(git.releaseTagRevisions, revision)
	if git.releaseTagsErr != nil {
		git.calls = append(git.calls, "release-tags")
		return nil, git.releaseTagsErr
	}
	return git.fakeGitRepository.ReleaseTagsAt(ctx, repository, revision)
}

func (git *releaseWhiteboxGit) CherryPick(ctx context.Context, repository port.RepositoryIdentity, commitID string) error {
	git.cherryPickContexts = append(git.cherryPickContexts, ctx)
	if git.cherryPickErr != nil {
		git.calls = append(git.calls, "cherry-pick")
		return git.cherryPickErr
	}
	return git.fakeGitRepository.CherryPick(ctx, repository, commitID)
}

func (git *releaseWhiteboxGit) CommitMessagesSince(
	ctx context.Context,
	repository port.RepositoryIdentity,
	base branch.TargetBase,
) ([]string, error) {
	if git.commitMessagesErr != nil {
		git.calls = append(git.calls, "commit-messages")
		return nil, git.commitMessagesErr
	}
	return git.fakeGitRepository.CommitMessagesSince(ctx, repository, base)
}

func (git *releaseWhiteboxGit) DeleteLocalBranch(
	ctx context.Context,
	repository port.RepositoryIdentity,
	name branch.BranchName,
	force bool,
) error {
	git.deleteContexts = append(git.deleteContexts, ctx)
	git.deletedBranches = append(git.deletedBranches, name)
	git.deleteForces = append(git.deleteForces, force)
	if git.deleteErr != nil {
		git.calls = append(git.calls, "delete-local-branch")
		return git.deleteErr
	}
	return git.fakeGitRepository.DeleteLocalBranch(ctx, repository, name, force)
}

func (git *releaseWhiteboxGit) ClearWorkflowBase(
	ctx context.Context,
	repository port.RepositoryIdentity,
	name branch.BranchName,
) error {
	git.clearContexts = append(git.clearContexts, ctx)
	if git.clearErr != nil {
		git.calls = append(git.calls, "clear-workflow-base")
		return git.clearErr
	}
	return git.fakeGitRepository.ClearWorkflowBase(ctx, repository, name)
}

type releaseWhiteboxPublisher struct {
	result   port.PublishedPullRequest
	err      error
	contexts []context.Context
	requests []port.PullRequest
}

type releaseWhiteboxLifecycle struct {
	dispatchResult port.SharedLineDispatchResult
	dispatchErr    error
	evidence       port.ReleaseReconciliationEvidence
	verifyErr      error
	dispatches     []port.SharedLineDispatchRequest
	reconciles     []port.ReleaseReconciliationRequest
}

type releaseWhiteboxRecordStore struct {
	record     hotfix.ReleaseRecord
	err        error
	repository port.RepositoryIdentity
	ticket     ticket.ID
	location   string
	calls      int
}

func (store *releaseWhiteboxRecordStore) LoadHotfixReleaseRecord(
	_ context.Context,
	repository port.RepositoryIdentity,
	id ticket.ID,
	location string,
) (hotfix.ReleaseRecord, error) {
	store.calls++
	store.repository = repository
	store.ticket = id
	store.location = location
	if store.err != nil {
		return hotfix.ReleaseRecord{}, store.err
	}
	return store.record, nil
}

func (lifecycle *releaseWhiteboxLifecycle) DispatchSharedLine(
	ctx context.Context,
	request port.SharedLineDispatchRequest,
) (port.SharedLineDispatchResult, error) {
	lifecycle.dispatches = append(lifecycle.dispatches, request)
	if lifecycle.dispatchErr != nil {
		return port.SharedLineDispatchResult{}, lifecycle.dispatchErr
	}
	return lifecycle.dispatchResult, nil
}

func (lifecycle *releaseWhiteboxLifecycle) VerifyReleaseReconciliation(
	ctx context.Context,
	request port.ReleaseReconciliationRequest,
) (port.ReleaseReconciliationEvidence, error) {
	lifecycle.reconciles = append(lifecycle.reconciles, request)
	if lifecycle.verifyErr != nil {
		return port.ReleaseReconciliationEvidence{}, lifecycle.verifyErr
	}
	return lifecycle.evidence, nil
}

type releaseWhiteboxHotfixLifecycle struct {
	mergeResult    port.MainHotfixMergeEvidence
	mergeErr       error
	deliveryResult port.MainHotfixDeliveryEvidence
	deliveryErr    error
	mergeRequests  []port.MainHotfixDeliveryRequest
	deliveryCalls  []port.MainHotfixDeliveryRequest
}

func (lifecycle *releaseWhiteboxHotfixLifecycle) VerifyMainHotfixMerge(
	_ context.Context,
	request port.MainHotfixDeliveryRequest,
) (port.MainHotfixMergeEvidence, error) {
	lifecycle.mergeRequests = append(lifecycle.mergeRequests, request)
	if lifecycle.mergeErr != nil {
		return port.MainHotfixMergeEvidence{}, lifecycle.mergeErr
	}
	return lifecycle.mergeResult, nil
}

func (lifecycle *releaseWhiteboxHotfixLifecycle) VerifyMainHotfixDelivery(
	_ context.Context,
	request port.MainHotfixDeliveryRequest,
) (port.MainHotfixDeliveryEvidence, error) {
	lifecycle.deliveryCalls = append(lifecycle.deliveryCalls, request)
	if lifecycle.deliveryErr != nil {
		return port.MainHotfixDeliveryEvidence{}, lifecycle.deliveryErr
	}
	return lifecycle.deliveryResult, nil
}

type releaseWhiteboxQuality struct {
	result   port.QualityResult
	err      error
	requests []port.QualityRequest
}

func (quality *releaseWhiteboxQuality) Run(
	_ context.Context,
	_ port.RepositoryIdentity,
	request port.QualityRequest,
) (port.QualityResult, error) {
	quality.requests = append(quality.requests, request)
	if quality.err != nil {
		return port.QualityResult{}, quality.err
	}
	return quality.result, nil
}

type releaseManifestGit struct {
	*releaseWhiteboxGit

	current       branch.BranchName
	currentErr    error
	progress      port.HotfixManifestProgress
	progressFound bool
	loadErr       error
	storeErr      error
	storeErrors   []error
	clearErr      error
	stored        []port.HotfixManifestProgress
	cleared       int
}

func (git *releaseManifestGit) CurrentBranch(context.Context, port.RepositoryIdentity) (branch.BranchName, error) {
	if git.currentErr != nil {
		return branch.BranchName{}, git.currentErr
	}
	if git.current.IsZero() {
		return git.releaseWhiteboxGit.CurrentBranch(context.Background(), testRepository())
	}
	return git.current, nil
}

func (git *releaseManifestGit) LoadHotfixManifestProgress(
	context.Context,
	port.RepositoryIdentity,
) (port.HotfixManifestProgress, bool, error) {
	if git.loadErr != nil {
		return port.HotfixManifestProgress{}, false, git.loadErr
	}
	return git.progress, git.progressFound, nil
}

func (git *releaseManifestGit) StoreHotfixManifestProgress(
	_ context.Context,
	_ port.RepositoryIdentity,
	progress port.HotfixManifestProgress,
) error {
	if len(git.storeErrors) > 0 {
		err := git.storeErrors[0]
		git.storeErrors = git.storeErrors[1:]
		if err != nil {
			return err
		}
	}
	if git.storeErr != nil {
		return git.storeErr
	}
	git.progress = progress
	git.progressFound = true
	git.stored = append(git.stored, progress)
	return nil
}

func (git *releaseManifestGit) ClearHotfixManifestProgress(context.Context, port.RepositoryIdentity) error {
	if git.clearErr != nil {
		return git.clearErr
	}
	git.cleared++
	git.progress = port.HotfixManifestProgress{}
	git.progressFound = false
	return nil
}

type manifestNoContinueGit struct {
	port.GitRepository
	state *releaseManifestGit
}

func (git *manifestNoContinueGit) CurrentBranch(ctx context.Context, repository port.RepositoryIdentity) (branch.BranchName, error) {
	return git.state.CurrentBranch(ctx, repository)
}

func (git *manifestNoContinueGit) LoadHotfixManifestProgress(
	ctx context.Context,
	repository port.RepositoryIdentity,
) (port.HotfixManifestProgress, bool, error) {
	return git.state.LoadHotfixManifestProgress(ctx, repository)
}

func (git *manifestNoContinueGit) StoreHotfixManifestProgress(
	ctx context.Context,
	repository port.RepositoryIdentity,
	progress port.HotfixManifestProgress,
) error {
	return git.state.StoreHotfixManifestProgress(ctx, repository, progress)
}

func (git *manifestNoContinueGit) ClearHotfixManifestProgress(ctx context.Context, repository port.RepositoryIdentity) error {
	return git.state.ClearHotfixManifestProgress(ctx, repository)
}

type releaseLifecycleGit struct {
	*releaseWhiteboxGit

	remoteURLErr error
	targetExists bool
	targetErr    error
}

func (git *releaseLifecycleGit) RemoteURL(ctx context.Context, repository port.RepositoryIdentity) (string, error) {
	if git.remoteURLErr != nil {
		return "", git.remoteURLErr
	}
	return git.releaseWhiteboxGit.RemoteURL(ctx, repository)
}

func (git *releaseLifecycleGit) TargetBaseExists(
	ctx context.Context,
	repository port.RepositoryIdentity,
	base branch.TargetBase,
) (bool, error) {
	if git.targetErr != nil {
		return false, git.targetErr
	}
	if !git.targetExists {
		return false, nil
	}
	return git.releaseWhiteboxGit.TargetBaseExists(ctx, repository, base)
}

type nonContinuingGit struct {
	port.GitRepository
}

func (publisher *releaseWhiteboxPublisher) Publish(
	ctx context.Context,
	publication port.PullRequestPublication,
) (port.PublishedPullRequest, error) {
	publisher.contexts = append(publisher.contexts, ctx)
	publisher.requests = append(publisher.requests, publication.PullRequest)
	return publisher.result, publisher.err
}

func newReleaseWhiteboxService(git port.GitRepository, publisher port.PullRequestPublisher) *ReleaseService {
	branches := branchapp.NewService(git, &fakeKeyPolicy{})
	sync := branchapp.NewSynchronizer(git, branches, nil)
	tickets := NewTicketService(branches, sync, git, nil, publisher)
	return NewReleaseService(branches, git, publisher).WithTicketService(tickets)
}

func newReleaseWhiteboxServiceWithoutTickets(git port.GitRepository) *ReleaseService {
	branches := branchapp.NewService(git, &fakeKeyPolicy{})
	return NewReleaseService(branches, git, nil)
}

func releaseHotfixRequest() StartHotfixRequest {
	return StartHotfixRequest{
		Repository:   testRepository(),
		Ticket:       mustTicket("ABC-999"),
		Slug:         mustSlug("payment-timeout"),
		AffectedLine: mustBranch("main"),
	}
}

func TestReleaseWhiteboxValidateMainHotfixRecord(t *testing.T) {
	source := mustBranch("hotfix/ABC-999-payment-timeout")
	request := ValidateMainHotfixRecordRequest{
		Repository: testRepository(),
		Branch:     source,
		Location:   ".git-governance/hotfix-release-records/ABC-999.json",
	}

	t.Run("requires a record store", func(t *testing.T) {
		_, err := newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil).ValidateMainHotfixRecord(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("rejects non hotfix branches before loading", func(t *testing.T) {
		store := &releaseWhiteboxRecordStore{}
		request := request
		request.Branch = mustBranch("feature/ABC-999-payment-timeout")
		_, err := newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil).
			WithHotfixReleaseRecordStore(store).
			ValidateMainHotfixRecord(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInvalidInput)
		if store.calls != 0 {
			t.Fatalf("record store calls = %d, want 0", store.calls)
		}
	})

	t.Run("rejects an invalid repository before loading", func(t *testing.T) {
		store := &releaseWhiteboxRecordStore{}
		request := request
		request.Repository = port.RepositoryIdentity{}
		_, err := newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil).
			WithHotfixReleaseRecordStore(store).
			ValidateMainHotfixRecord(context.Background(), request)
		if err == nil {
			t.Fatal("ValidateMainHotfixRecord unexpectedly accepted an invalid repository")
		}
		if store.calls != 0 {
			t.Fatalf("record store calls = %d, want 0", store.calls)
		}
	})

	t.Run("preserves record-store failures", func(t *testing.T) {
		recordFailure := errors.New("record unavailable")
		store := &releaseWhiteboxRecordStore{err: recordFailure}
		_, err := newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil).
			WithHotfixReleaseRecordStore(store).
			ValidateMainHotfixRecord(context.Background(), request)
		if !errors.Is(err, recordFailure) {
			t.Fatalf("ValidateMainHotfixRecord() error = %v, want %v", err, recordFailure)
		}
	})

	t.Run("rejects records bound to another hotfix branch", func(t *testing.T) {
		store := &releaseWhiteboxRecordStore{record: releaseRecord(t, "hotfix/ABC-999-other-hotfix", "main")}
		_, err := newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil).
			WithHotfixReleaseRecordStore(store).
			ValidateMainHotfixRecord(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("rejects non main patch records", func(t *testing.T) {
		store := &releaseWhiteboxRecordStore{record: releaseRecord(t, source.String(), "release/1.0.2")}
		_, err := newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil).
			WithHotfixReleaseRecordStore(store).
			ValidateMainHotfixRecord(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("returns a matching reviewed main record", func(t *testing.T) {
		record := releaseRecord(t, source.String(), "main")
		store := &releaseWhiteboxRecordStore{record: record}
		result, err := newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil).
			WithHotfixReleaseRecordStore(store).
			ValidateMainHotfixRecord(context.Background(), request)
		if err != nil || result.Record.Ticket().String() != "ABC-999" {
			t.Fatalf("ValidateMainHotfixRecord() = (%#v, %v)", result, err)
		}
		if store.calls != 1 || store.ticket.String() != "ABC-999" || store.location != request.Location {
			t.Fatalf("record load = %#v", store)
		}
	})
}

func TestReleaseWhiteboxVerifyMainHotfixDelivery(t *testing.T) {
	source := mustBranch("hotfix/ABC-999-payment-timeout")
	record := releaseRecord(t, source.String(), "main")
	request := VerifyMainHotfixMergeRequest{
		Repository: testRepository(),
		Branch:     source,
		Location:   ".git-governance/hotfix-release-records/ABC-999.json",
	}

	t.Run("requires Git and a lifecycle provider", func(t *testing.T) {
		store := &releaseWhiteboxRecordStore{record: record}
		_, err := NewReleaseService(nil, nil, nil).
			WithHotfixReleaseRecordStore(store).
			VerifyMainHotfixMerge(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInternal)

		_, err = newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil).
			WithHotfixReleaseRecordStore(store).
			VerifyMainHotfixMerge(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("preserves record and remote failures before provider use", func(t *testing.T) {
		storeFailure := errors.New("record failure")
		provider := &releaseWhiteboxHotfixLifecycle{}
		_, err := newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{err: storeFailure}).
			WithMainHotfixLifecycleProvider(provider).
			VerifyMainHotfixMerge(context.Background(), request)
		if !errors.Is(err, storeFailure) {
			t.Fatalf("record error = %v, want %v", err, storeFailure)
		}
		if len(provider.mergeRequests) != 0 {
			t.Fatalf("provider merge requests = %#v", provider.mergeRequests)
		}

		remoteFailure := errors.New("remote URL failure")
		git := &releaseLifecycleGit{releaseWhiteboxGit: newReleaseWhiteboxGit(), remoteURLErr: remoteFailure}
		_, err = newReleaseWhiteboxService(git, nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithMainHotfixLifecycleProvider(provider).
			VerifyMainHotfixMerge(context.Background(), request)
		if !errors.Is(err, remoteFailure) {
			t.Fatalf("remote error = %v, want %v", err, remoteFailure)
		}
	})

	t.Run("rejects incomplete merge evidence", func(t *testing.T) {
		provider := &releaseWhiteboxHotfixLifecycle{
			mergeResult: port.MainHotfixMergeEvidence{Tag: "v1.0.2"},
		}
		_, err := newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithMainHotfixLifecycleProvider(provider).
			VerifyMainHotfixMerge(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("returns validated merge evidence", func(t *testing.T) {
		provider := &releaseWhiteboxHotfixLifecycle{
			mergeResult: port.MainHotfixMergeEvidence{
				PullRequestURL: "https://example.invalid/pr/999",
				MergeCommit:    strings.Repeat("a", 40),
				Tag:            "v1.0.2",
			},
		}
		result, err := newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithMainHotfixLifecycleProvider(provider).
			VerifyMainHotfixMerge(context.Background(), request)
		if err != nil || result.Evidence.MergeCommit != strings.Repeat("a", 40) {
			t.Fatalf("VerifyMainHotfixMerge() = (%#v, %v)", result, err)
		}
		if len(provider.mergeRequests) != 1 || provider.mergeRequests[0].Record.Ticket().String() != "ABC-999" {
			t.Fatalf("provider merge requests = %#v", provider.mergeRequests)
		}
	})

	t.Run("preserves main hotfix provider merge failures", func(t *testing.T) {
		mergeFailure := errors.New("merge evidence failure")
		_, err := newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithMainHotfixLifecycleProvider(&releaseWhiteboxHotfixLifecycle{mergeErr: mergeFailure}).
			VerifyMainHotfixMerge(context.Background(), request)
		if !errors.Is(err, mergeFailure) {
			t.Fatalf("merge evidence error = %v, want %v", err, mergeFailure)
		}
	})

	t.Run("verifies complete delivery evidence", func(t *testing.T) {
		provider := &releaseWhiteboxHotfixLifecycle{
			deliveryResult: port.MainHotfixDeliveryEvidence{
				MainHotfixMergeEvidence: port.MainHotfixMergeEvidence{
					PullRequestURL: "https://example.invalid/pr/999",
					MergeCommit:    strings.Repeat("a", 40),
					Tag:            "v1.0.2",
				},
				ReleaseURL:     "https://example.invalid/releases/v1.0.2",
				WorkflowRunURL: "https://example.invalid/actions/99",
			},
		}
		result, err := newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithMainHotfixLifecycleProvider(provider).
			VerifyMainHotfixDelivery(context.Background(), VerifyMainHotfixDeliveryRequest(request))
		if err != nil || result.Evidence.WorkflowRunURL == "" {
			t.Fatalf("VerifyMainHotfixDelivery() = (%#v, %v)", result, err)
		}
		if len(provider.deliveryCalls) != 1 {
			t.Fatalf("provider delivery calls = %#v", provider.deliveryCalls)
		}
	})

	t.Run("rejects incomplete or failed delivery provider evidence", func(t *testing.T) {
		deliveryFailure := errors.New("delivery provider failure")
		provider := &releaseWhiteboxHotfixLifecycle{deliveryErr: deliveryFailure}
		_, err := newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithMainHotfixLifecycleProvider(provider).
			VerifyMainHotfixDelivery(context.Background(), VerifyMainHotfixDeliveryRequest(request))
		if !errors.Is(err, deliveryFailure) {
			t.Fatalf("delivery error = %v, want %v", err, deliveryFailure)
		}

		provider = &releaseWhiteboxHotfixLifecycle{
			deliveryResult: port.MainHotfixDeliveryEvidence{
				MainHotfixMergeEvidence: port.MainHotfixMergeEvidence{
					PullRequestURL: "https://example.invalid/pr/999",
					MergeCommit:    strings.Repeat("a", 40),
					Tag:            "v1.0.2",
				},
			},
		}
		_, err = newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithMainHotfixLifecycleProvider(provider).
			VerifyMainHotfixDelivery(context.Background(), VerifyMainHotfixDeliveryRequest(request))
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("requires Git and lifecycle before delivery verification", func(t *testing.T) {
		_, err := NewReleaseService(nil, nil, nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			VerifyMainHotfixDelivery(context.Background(), VerifyMainHotfixDeliveryRequest(request))
		assertProblemCode(t, err, problem.CodeInternal)

		_, err = newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			VerifyMainHotfixDelivery(context.Background(), VerifyMainHotfixDeliveryRequest(request))
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("preserves delivery record and remote failures", func(t *testing.T) {
		recordFailure := errors.New("delivery record failure")
		provider := &releaseWhiteboxHotfixLifecycle{}
		_, err := newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{err: recordFailure}).
			WithMainHotfixLifecycleProvider(provider).
			VerifyMainHotfixDelivery(context.Background(), VerifyMainHotfixDeliveryRequest(request))
		if !errors.Is(err, recordFailure) {
			t.Fatalf("delivery record error = %v", err)
		}

		remoteFailure := errors.New("delivery remote failure")
		git := &releaseLifecycleGit{releaseWhiteboxGit: newReleaseWhiteboxGit(), remoteURLErr: remoteFailure}
		_, err = newReleaseWhiteboxService(git, nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithMainHotfixLifecycleProvider(provider).
			VerifyMainHotfixDelivery(context.Background(), VerifyMainHotfixDeliveryRequest(request))
		if !errors.Is(err, remoteFailure) {
			t.Fatalf("delivery remote error = %v", err)
		}

		_, err = newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithMainHotfixLifecycleProvider(provider).
			VerifyMainHotfixDelivery(context.Background(), VerifyMainHotfixDeliveryRequest{
				Repository: port.RepositoryIdentity{},
				Branch:     source,
			})
		if err == nil {
			t.Fatal("delivery verification unexpectedly accepted an invalid repository")
		}
	})
}

func TestReleaseWhiteboxPropagateHotfixManifest(t *testing.T) {
	source := mustBranch("hotfix/ABC-999-payment-timeout")
	target := mustBranch("develop")
	manifest := []string{strings.Repeat("a", 40), strings.Repeat("b", 40)}
	record := releaseRecordWithManifest(t, source.String(), "main", manifest, []string{"develop"})
	request := PropagateHotfixManifestRequest{
		Repository: testRepository(),
		Source:     source,
		TargetLine: target,
	}

	t.Run("requires complete dependencies and progress storage", func(t *testing.T) {
		_, err := (&ReleaseService{}).PropagateHotfixManifest(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInternal)

		_, err = newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithQualityRunner(&releaseWhiteboxQuality{}).
			PropagateHotfixManifest(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("rejects undeclared targets before branch mutation", func(t *testing.T) {
		git := &releaseManifestGit{releaseWhiteboxGit: newReleaseWhiteboxGit()}
		request := request
		request.TargetLine = mustBranch("support/1.0")
		_, err := newReleaseWhiteboxService(git, nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithQualityRunner(&releaseWhiteboxQuality{}).
			PropagateHotfixManifest(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInvalidInput)
		if len(git.createdNames) != 0 {
			t.Fatalf("created branches = %#v", git.createdNames)
		}
	})

	t.Run("plans a dry run without local propagation state", func(t *testing.T) {
		git := &releaseManifestGit{releaseWhiteboxGit: newReleaseWhiteboxGit()}
		request := request
		request.DryRun = true
		result, err := newReleaseWhiteboxService(git, nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithQualityRunner(&releaseWhiteboxQuality{}).
			PropagateHotfixManifest(context.Background(), request)
		if err != nil || !result.DryRun || result.Branch.Name.String() != "fix/ABC-999-propagate-to-develop" {
			t.Fatalf("PropagateHotfixManifest() = (%#v, %v)", result, err)
		}
		if len(git.stored) != 0 || len(git.cherryPicked) != 0 {
			t.Fatalf("dry run mutated progress=%#v cherry=%#v", git.stored, git.cherryPicked)
		}
	})

	t.Run("creates and verifies a local ordered candidate without publication", func(t *testing.T) {
		git := &releaseManifestGit{releaseWhiteboxGit: newReleaseWhiteboxGit()}
		quality := &releaseWhiteboxQuality{result: port.QualityResult{Status: port.QualityPassed}}
		result, err := newReleaseWhiteboxService(git, nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithQualityRunner(quality).
			PropagateHotfixManifest(context.Background(), request)
		if err != nil ||
			result.Branch.Name.String() != "fix/ABC-999-propagate-to-develop" ||
			result.CherryPickCount != 2 ||
			result.Quality == nil ||
			len(git.cherryPicked) != 2 ||
			git.cherryPicked[0] != manifest[0] ||
			git.cherryPicked[1] != manifest[1] ||
			git.cleared != 1 {
			t.Fatalf("PropagateHotfixManifest() = (%#v, %v), git=%#v", result, err, git)
		}
		if len(quality.requests) != 1 || len(quality.requests[0].Families) != 1 || quality.requests[0].Families[0] != branch.FamilyFix {
			t.Fatalf("quality requests = %#v", quality.requests)
		}
	})

	t.Run("preserves a paused manifest cursor for conflict recovery", func(t *testing.T) {
		cherryPickFailure := errors.New("cherry pick conflict")
		base := newReleaseWhiteboxGit()
		base.cherryPickErr = cherryPickFailure
		base.active = true
		base.activeOperation = "cherry-pick"
		git := &releaseManifestGit{releaseWhiteboxGit: base}
		_, err := newReleaseWhiteboxService(git, nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithQualityRunner(&releaseWhiteboxQuality{}).
			PropagateHotfixManifest(context.Background(), request)
		assertProblemCode(t, err, problem.CodeCherryPickConflict)
		if !git.progressFound || git.progress.Next != 0 || git.cleared != 0 {
			t.Fatalf("conflict progress = %#v", git)
		}
	})
}

func TestReleaseWhiteboxResumeHotfixManifestPropagation(t *testing.T) {
	source := mustBranch("hotfix/ABC-999-payment-timeout")
	target := mustBranch("develop")
	candidate := mustBranch("fix/ABC-999-propagate-to-develop")
	manifest := []string{strings.Repeat("a", 40), strings.Repeat("b", 40)}
	record := releaseRecordWithManifest(t, source.String(), "main", manifest, []string{"develop"})
	request := ResumeHotfixManifestPropagationRequest{
		Repository: testRepository(),
		Source:     source,
		TargetLine: target,
		Branch:     candidate,
	}

	t.Run("requires dependencies and matching state", func(t *testing.T) {
		_, err := (&ReleaseService{}).ResumeHotfixManifestPropagation(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInternal)

		_, err = newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithQualityRunner(&releaseWhiteboxQuality{}).
			ResumeHotfixManifestPropagation(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("rejects candidate and progress mismatches before continuation", func(t *testing.T) {
		base := newReleaseWhiteboxGit()
		base.active = true
		base.activeOperation = "cherry-pick"
		git := &releaseManifestGit{
			releaseWhiteboxGit: base,
			current:            candidate,
			progressFound:      true,
			progress: port.HotfixManifestProgress{
				Branch:   mustBranch("fix/ABC-999-wrong"),
				Source:   source,
				Target:   target,
				Manifest: manifest,
			},
		}
		_, err := newReleaseWhiteboxService(git, nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithQualityRunner(&releaseWhiteboxQuality{}).
			ResumeHotfixManifestPropagation(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("continues the resolved item then verifies remaining ordered commits", func(t *testing.T) {
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
				Next:     0,
			},
		}
		quality := &releaseWhiteboxQuality{result: port.QualityResult{Status: port.QualityPassed}}
		result, err := newReleaseWhiteboxService(git, nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithQualityRunner(quality).
			ResumeHotfixManifestPropagation(context.Background(), request)
		if err != nil || result.CherryPickCount != 2 || len(git.cherryPicked) != 1 || git.cherryPicked[0] != manifest[1] || git.cleared != 1 {
			t.Fatalf("ResumeHotfixManifestPropagation() = (%#v, %v), git=%#v", result, err, git)
		}
	})
}

func TestReleaseWhiteboxManifestPropagationFailurePaths(t *testing.T) {
	source := mustBranch("hotfix/ABC-999-payment-timeout")
	target := mustBranch("develop")
	candidate := mustBranch("fix/ABC-999-propagate-to-develop")
	manifest := []string{strings.Repeat("a", 40), strings.Repeat("b", 40)}
	record := releaseRecordWithManifest(t, source.String(), "main", manifest, []string{"develop"})
	request := PropagateHotfixManifestRequest{Repository: testRepository(), Source: source, TargetLine: target}
	resume := ResumeHotfixManifestPropagationRequest{Repository: testRepository(), Source: source, TargetLine: target, Branch: candidate}

	newService := func(git port.GitRepository, store *releaseWhiteboxRecordStore, quality *releaseWhiteboxQuality) *ReleaseService {
		return newReleaseWhiteboxService(git, nil).
			WithHotfixReleaseRecordStore(store).
			WithQualityRunner(quality)
	}

	t.Run("preserves record, branch, workflow-base, progress, clear, validation, and quality errors", func(t *testing.T) {
		recordFailure := errors.New("record failure")
		git := &releaseManifestGit{releaseWhiteboxGit: newReleaseWhiteboxGit()}
		_, err := newService(git, &releaseWhiteboxRecordStore{err: recordFailure}, &releaseWhiteboxQuality{}).
			PropagateHotfixManifest(context.Background(), request)
		if !errors.Is(err, recordFailure) {
			t.Fatalf("record error = %v", err)
		}

		base := newReleaseWhiteboxGit()
		createFailure := errors.New("create failure")
		base.createErr = createFailure
		git = &releaseManifestGit{releaseWhiteboxGit: base}
		_, err = newService(git, &releaseWhiteboxRecordStore{record: record}, &releaseWhiteboxQuality{}).
			PropagateHotfixManifest(context.Background(), request)
		if !errors.Is(err, createFailure) {
			t.Fatalf("create error = %v", err)
		}

		base = newReleaseWhiteboxGit()
		workflowBaseFailure := errors.New("workflow base failure")
		base.storeErr = workflowBaseFailure
		git = &releaseManifestGit{releaseWhiteboxGit: base}
		_, err = newService(git, &releaseWhiteboxRecordStore{record: record}, &releaseWhiteboxQuality{}).
			PropagateHotfixManifest(context.Background(), request)
		if !errors.Is(err, workflowBaseFailure) {
			t.Fatalf("workflow base error = %v", err)
		}

		git = &releaseManifestGit{releaseWhiteboxGit: newReleaseWhiteboxGit(), storeErr: errors.New("progress store failure")}
		_, err = newService(git, &releaseWhiteboxRecordStore{record: record}, &releaseWhiteboxQuality{}).
			PropagateHotfixManifest(context.Background(), request)
		if err == nil {
			t.Fatal("progress store failure was lost")
		}

		git = &releaseManifestGit{releaseWhiteboxGit: newReleaseWhiteboxGit(), clearErr: errors.New("clear failure")}
		_, err = newService(git, &releaseWhiteboxRecordStore{record: record}, &releaseWhiteboxQuality{}).
			PropagateHotfixManifest(context.Background(), request)
		if err == nil {
			t.Fatal("clear failure was lost")
		}

		qualityFailure := errors.New("quality failure")
		git = &releaseManifestGit{releaseWhiteboxGit: newReleaseWhiteboxGit()}
		_, err = newService(git, &releaseWhiteboxRecordStore{record: record}, &releaseWhiteboxQuality{err: qualityFailure}).
			PropagateHotfixManifest(context.Background(), request)
		if !errors.Is(err, qualityFailure) {
			t.Fatalf("quality error = %v", err)
		}

		branchValidationFailure := errors.New("post-manifest validation failure")
		base = newReleaseWhiteboxGit()
		base.validateErrors = []error{nil, branchValidationFailure}
		git = &releaseManifestGit{releaseWhiteboxGit: base}
		_, err = newService(git, &releaseWhiteboxRecordStore{record: record}, &releaseWhiteboxQuality{}).
			PropagateHotfixManifest(context.Background(), request)
		if !errors.Is(err, branchValidationFailure) {
			t.Fatalf("post-manifest validation error = %v", err)
		}

		git = &releaseManifestGit{
			releaseWhiteboxGit: newReleaseWhiteboxGit(),
			storeErrors:        []error{nil, errors.New("post-pick progress failure")},
		}
		_, err = newService(git, &releaseWhiteboxRecordStore{record: record}, &releaseWhiteboxQuality{}).
			PropagateHotfixManifest(context.Background(), request)
		if err == nil {
			t.Fatal("post-pick progress failure was lost")
		}
	})

	t.Run("enforces target and default or explicit slug contracts", func(t *testing.T) {
		if err := validateManifestTarget(record, mustBranch("main")); err == nil {
			t.Fatal("validateManifestTarget accepted main")
		}
		if err := validateManifestTarget(record, mustBranch("support/1.0")); err == nil {
			t.Fatal("validateManifestTarget accepted undeclared support")
		}
		if err := validateManifestTarget(record, target); err != nil {
			t.Fatal(err)
		}
		if got := resolveManifestPropagationSlug(mustSlug("custom-propagation"), target); got.String() != "custom-propagation" {
			t.Fatalf("explicit slug = %q", got)
		}
		if got := resolveManifestPropagationSlug(branch.Slug{}, mustBranch("release/1.0.2")); got.String() != "propagate-to-release-1-0-2" {
			t.Fatalf("default slug = %q", got)
		}
	})

	t.Run("rejects invalid resume state before and during continuation", func(t *testing.T) {
		base := newReleaseWhiteboxGit()
		base.active = true
		base.activeOperation = "cherry-pick"
		state := &releaseManifestGit{
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

		wrongBranch := resume
		wrongBranch.Branch = mustBranch("feature/ABC-999-wrong")
		_, err := newService(state, &releaseWhiteboxRecordStore{record: record}, &releaseWhiteboxQuality{}).
			ResumeHotfixManifestPropagation(context.Background(), wrongBranch)
		assertProblemCode(t, err, problem.CodeInvalidInput)

		state.current = mustBranch("fix/ABC-999-other")
		_, err = newService(state, &releaseWhiteboxRecordStore{record: record}, &releaseWhiteboxQuality{}).
			ResumeHotfixManifestPropagation(context.Background(), resume)
		assertProblemCode(t, err, problem.CodeInvalidInput)

		state.current = candidate
		state.currentErr = errors.New("current branch failure")
		_, err = newService(state, &releaseWhiteboxRecordStore{record: record}, &releaseWhiteboxQuality{}).
			ResumeHotfixManifestPropagation(context.Background(), resume)
		if err == nil {
			t.Fatal("current branch failure was lost")
		}

		state.currentErr = nil
		state.loadErr = errors.New("progress load failure")
		_, err = newService(state, &releaseWhiteboxRecordStore{record: record}, &releaseWhiteboxQuality{}).
			ResumeHotfixManifestPropagation(context.Background(), resume)
		if err == nil {
			t.Fatal("progress load failure was lost")
		}

		state.loadErr = nil
		state.progressFound = false
		_, err = newService(state, &releaseWhiteboxRecordStore{record: record}, &releaseWhiteboxQuality{}).
			ResumeHotfixManifestPropagation(context.Background(), resume)
		assertProblemCode(t, err, problem.CodeInvalidInput)

		state.progressFound = true
		base.activeErr = errors.New("active operation failure")
		_, err = newService(state, &releaseWhiteboxRecordStore{record: record}, &releaseWhiteboxQuality{}).
			ResumeHotfixManifestPropagation(context.Background(), resume)
		if err == nil {
			t.Fatal("active operation failure was lost")
		}

		base.activeErr = nil
		base.active = false
		_, err = newService(state, &releaseWhiteboxRecordStore{record: record}, &releaseWhiteboxQuality{}).
			ResumeHotfixManifestPropagation(context.Background(), resume)
		assertProblemCode(t, err, problem.CodeInvalidInput)

		recordFailure := errors.New("resume record failure")
		_, err = newService(state, &releaseWhiteboxRecordStore{err: recordFailure}, &releaseWhiteboxQuality{}).
			ResumeHotfixManifestPropagation(context.Background(), resume)
		if !errors.Is(err, recordFailure) {
			t.Fatalf("resume record error = %v", err)
		}

		undeclared := resume
		undeclared.TargetLine = mustBranch("support/1.0")
		_, err = newService(state, &releaseWhiteboxRecordStore{record: record}, &releaseWhiteboxQuality{}).
			ResumeHotfixManifestPropagation(context.Background(), undeclared)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("requires a continuator and preserves continuation and progress failures", func(t *testing.T) {
		base := newReleaseWhiteboxGit()
		base.active = true
		base.activeOperation = "cherry-pick"
		state := &releaseManifestGit{
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
		withoutContinuator := &manifestNoContinueGit{GitRepository: state.releaseWhiteboxGit, state: state}
		_, err := newService(withoutContinuator, &releaseWhiteboxRecordStore{record: record}, &releaseWhiteboxQuality{}).
			ResumeHotfixManifestPropagation(context.Background(), resume)
		assertProblemCode(t, err, problem.CodeInternal)

		base.continueErr = errors.New("continue failure")
		_, err = newService(state, &releaseWhiteboxRecordStore{record: record}, &releaseWhiteboxQuality{}).
			ResumeHotfixManifestPropagation(context.Background(), resume)
		assertProblemCode(t, err, problem.CodeCherryPickConflict)

		base.continueErr = nil
		base.active = true
		base.activeOperation = "cherry-pick"
		state.storeErr = errors.New("resume progress failure")
		_, err = newService(state, &releaseWhiteboxRecordStore{record: record}, &releaseWhiteboxQuality{}).
			ResumeHotfixManifestPropagation(context.Background(), resume)
		if err == nil {
			t.Fatal("resume progress failure was lost")
		}
	})

	t.Run("matches only exact ordered progress", func(t *testing.T) {
		progress := port.HotfixManifestProgress{Branch: candidate, Source: source, Target: target, Manifest: manifest}
		if !matchesManifestProgress(progress, resume, manifest) {
			t.Fatal("exact progress did not match")
		}
		for _, mutate := range []func(*port.HotfixManifestProgress){
			func(value *port.HotfixManifestProgress) { value.Branch = mustBranch("fix/ABC-999-other") },
			func(value *port.HotfixManifestProgress) { value.Source = mustBranch("hotfix/ABC-999-other") },
			func(value *port.HotfixManifestProgress) { value.Target = mustBranch("support/1.0") },
			func(value *port.HotfixManifestProgress) { value.Manifest = []string{manifest[1], manifest[0]} },
			func(value *port.HotfixManifestProgress) { value.Next = len(manifest) },
		} {
			value := progress
			value.Manifest = append([]string(nil), progress.Manifest...)
			mutate(&value)
			if matchesManifestProgress(value, resume, manifest) {
				t.Fatalf("mismatched progress accepted: %#v", value)
			}
		}
	})
}

func releaseRecord(t *testing.T, source, affected string) hotfix.ReleaseRecord {
	t.Helper()

	return releaseRecordWithManifest(
		t,
		source,
		affected,
		[]string{strings.Repeat("a", 40)},
		[]string{"develop"},
	)
}

func releaseRecordWithManifest(
	t *testing.T,
	source, affected string,
	manifest, targets []string,
) hotfix.ReleaseRecord {
	t.Helper()

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	targetsJSON, err := json.Marshal(targets)
	if err != nil {
		t.Fatal(err)
	}
	contents := fmt.Sprintf(
		`{"schemaVersion":1,"ticket":"ABC-999","incident":"INC-999","affectedLine":%q,"targetVersion":"1.0.2","previousTag":"v1.0.1","expectedPullRequest":{"source":%q,"target":%q},"manifest":%s,"commitBudgetException":"","propagationTargets":%s}`,
		affected,
		source,
		affected,
		manifestJSON,
		targetsJSON,
	)
	record, err := hotfix.ParseRecord([]byte(contents))
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func releaseStabilizationRequest() CreateReleaseStabilizationRequest {
	return CreateReleaseStabilizationRequest{
		Repository: testRepository(),
		Release:    mustBranch("release/2.8.0"),
		Ticket:     mustTicket("ABC-999"),
		Slug:       mustSlug("release-blocker"),
		Kind:       ReleaseStabilizationBlocker,
	}
}

func releasePropagationRequest() PropagateHotfixRequest {
	return PropagateHotfixRequest{
		Repository: testRepository(),
		Source:     mustBranch("hotfix/ABC-999-payment-timeout"),
		TargetLine: mustBranch("main"),
		CommitID:   strings.Repeat("a", 40),
		Slug:       mustSlug("forward-port-payment-timeout"),
		Body:       "Summary: Forward-port the reviewed payment-timeout hotfix to main.\n\nScope and Non-Goals: The single reviewed commit only.\n\nCommit Series:\n- fix(ABC-999): resolve payment timeout\n\nRisk and Rollback: Reviewed hotfix; rollback reverts the cherry-pick.\n\nVerification and Review Focus: Reviewed source commit; review the target-line fit.",
	}
}

func assertReleaseErrorIs(t *testing.T, got error, want error) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("error = %v, want %v", got, want)
	}
}

func assertReleaseNoCall(t *testing.T, calls []string, forbidden string) {
	t.Helper()
	if countCall(calls, forbidden) != 0 {
		t.Fatalf("calls %v include forbidden %q", calls, forbidden)
	}
}

func TestReleaseWhiteboxStartHotfixBoundaries(t *testing.T) {
	t.Run("requires a composed branch service", func(t *testing.T) {
		_, err := (&ReleaseService{}).StartHotfix(context.Background(), releaseHotfixRequest())
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("rejects absent, non-shared, and integration-line affected lines before Git", func(t *testing.T) {
		for _, affected := range []branch.BranchName{{}, mustBranch("feature/ABC-999-payment-timeout"), mustBranch("develop")} {
			git := newReleaseWhiteboxGit()
			request := releaseHotfixRequest()
			request.AffectedLine = affected

			_, err := newReleaseWhiteboxService(git, nil).StartHotfix(context.Background(), request)
			assertProblemCode(t, err, problem.CodeInvalidInput)
			if len(git.calls) != 0 {
				t.Fatalf("invalid input called Git: %v", git.calls)
			}
		}
	})

	t.Run("rejects missing repositories and invalid remotes before creation", func(t *testing.T) {
		for _, repository := range []port.RepositoryIdentity{
			{},
			{Root: testRepository().Root, Remote: "invalid remote"},
		} {
			git := newReleaseWhiteboxGit()
			request := releaseHotfixRequest()
			request.Repository = repository

			_, err := newReleaseWhiteboxService(git, nil).StartHotfix(context.Background(), request)
			if repository.Root == "" {
				assertProblemCode(t, err, problem.CodeRepositoryNotFound)
			} else {
				assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
			}
			if len(git.calls) != 0 {
				t.Fatalf("invalid repository called Git: %v", git.calls)
			}
		}
	})

	t.Run("honors a cancelled context before adapter interactions", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		git := newReleaseWhiteboxGit()

		_, err := newReleaseWhiteboxService(git, nil).StartHotfix(ctx, releaseHotfixRequest())
		assertProblemCode(t, err, problem.CodeOperationCancelled)
		if len(git.calls) != 0 {
			t.Fatalf("cancelled operation called Git: %v", git.calls)
		}
	})

	t.Run("propagates validation and branch creation failures", func(t *testing.T) {
		validationFailure := errors.New("validate hotfix ref")
		git := newReleaseWhiteboxGit()
		git.validateErr = validationFailure
		_, err := newReleaseWhiteboxService(git, nil).StartHotfix(context.Background(), releaseHotfixRequest())
		assertReleaseErrorIs(t, err, validationFailure)
		assertReleaseNoCall(t, git.calls, "create-branch")

		createFailure := errors.New("create hotfix")
		git = newReleaseWhiteboxGit()
		git.createErr = createFailure
		_, err = newReleaseWhiteboxService(git, nil).StartHotfix(context.Background(), releaseHotfixRequest())
		assertReleaseErrorIs(t, err, createFailure)
		assertReleaseNoCall(t, git.calls, "store-workflow-base")
	})

	t.Run("preflights the metadata dependency before local mutation", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		branches := branchapp.NewService(git, &fakeKeyPolicy{})
		service := NewReleaseService(branches, nil, nil)

		_, err := service.StartHotfix(context.Background(), releaseHotfixRequest())
		assertProblemCode(t, err, problem.CodeInternal)
		assertReleaseNoCall(t, git.calls, "create-branch")
	})

	t.Run("returns a plan without mutations during dry run", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		request := releaseHotfixRequest()
		request.DryRun = true

		result, err := newReleaseWhiteboxService(git, nil).StartHotfix(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if !result.DryRun || result.Name.String() != "hotfix/ABC-999-payment-timeout" || result.Base.String() != "origin/main" {
			t.Fatalf("StartHotfix() = %#v", result)
		}
		assertReleaseNoCall(t, git.calls, "create-branch")
		assertReleaseNoCall(t, git.calls, "store-workflow-base")
		assertReleaseNoCall(t, git.calls, "push")
	})

	t.Run("stores the hotfix provenance and forwards context", func(t *testing.T) {
		type contextKey struct{}
		ctx := context.WithValue(context.Background(), contextKey{}, "hotfix")
		git := newReleaseWhiteboxGit()

		result, err := newReleaseWhiteboxService(git, nil).StartHotfix(ctx, releaseHotfixRequest())
		if err != nil {
			t.Fatal(err)
		}
		if stored := git.workflowBases[result.Name.String()]; stored.String() != "origin/main" {
			t.Fatalf("stored base = %q, want origin/main", stored)
		}
		if len(git.validateContexts) == 0 || git.validateContexts[0] != ctx {
			t.Fatalf("validation contexts = %v, want %v", git.validateContexts, ctx)
		}
		if len(git.storeContexts) != 1 || git.storeContexts[0] != ctx {
			t.Fatalf("store contexts = %v, want %v", git.storeContexts, ctx)
		}

		storeFailure := errors.New("store workflow base")
		git = newReleaseWhiteboxGit()
		git.storeErr = storeFailure
		_, err = newReleaseWhiteboxService(git, nil).StartHotfix(context.Background(), releaseHotfixRequest())
		assertReleaseErrorIs(t, err, storeFailure)
	})
}

func TestReleaseWhiteboxCutReleaseCreatesOnlyCIIntent(t *testing.T) {
	version := mustReleaseVersion(t, "2.8.0")

	t.Run("rejects an absent version and missing Git adapter", func(t *testing.T) {
		_, err := (&ReleaseService{}).CutRelease(context.Background(), CutReleaseRequest{
			Repository: testRepository(),
		})
		assertProblemCode(t, err, problem.CodeBranchNameInvalid)

		_, err = (&ReleaseService{}).CutRelease(context.Background(), CutReleaseRequest{
			Repository: testRepository(),
			Version:    version,
		})
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("propagates validation and source resolution errors", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		_, err := newReleaseWhiteboxService(git, nil).CutRelease(context.Background(), CutReleaseRequest{
			Repository: port.RepositoryIdentity{},
			Version:    version,
		})
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)

		validationFailure := errors.New("validate release intent")
		git = newReleaseWhiteboxGit()
		git.validateErr = validationFailure
		_, err = newReleaseWhiteboxService(git, nil).CutRelease(context.Background(), CutReleaseRequest{
			Repository: testRepository(),
			Version:    version,
		})
		assertReleaseErrorIs(t, err, validationFailure)

		git = newReleaseWhiteboxGit()
		_, err = newReleaseWhiteboxService(git, nil).CutRelease(context.Background(), CutReleaseRequest{
			Repository: port.RepositoryIdentity{Root: testRepository().Root, Remote: "bad remote"},
			Version:    version,
		})
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})

	t.Run("plans dry runs without fetching or mutating a shared line", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		result, err := newReleaseWhiteboxService(git, nil).CutRelease(context.Background(), CutReleaseRequest{
			Repository: testRepository(),
			Version:    version,
			DryRun:     true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !result.DryRun || result.Intent.Workflow != "execute-protected-line-request.yml" ||
			result.Intent.Kind != "release" || result.Intent.Branch.String() != "release/2.8.0" ||
			result.Intent.Source.String() != "origin/develop" || result.Intent.Inputs["version"] != "2.8.0" ||
			len(result.Plan) != 2 {
			t.Fatalf("CutRelease() = %#v", result)
		}
		assertReleaseNoCall(t, git.calls, "fetch")
		assertReleaseNoCall(t, git.calls, "create-branch")
		assertReleaseNoCall(t, git.calls, "push")
	})

	t.Run("propagates fetch and commit inspection failures", func(t *testing.T) {
		fetchFailure := errors.New("fetch release source")
		git := newReleaseWhiteboxGit()
		git.fetchErrors = []error{fetchFailure}
		_, err := newReleaseWhiteboxService(git, nil).CutRelease(context.Background(), CutReleaseRequest{
			Repository: testRepository(),
			Version:    version,
		})
		assertReleaseErrorIs(t, err, fetchFailure)

		commitInspectionFailure := errors.New("inspect release source")
		git = newReleaseWhiteboxGit()
		git.hasCommitsErr = commitInspectionFailure
		_, err = newReleaseWhiteboxService(git, nil).CutRelease(context.Background(), CutReleaseRequest{
			Repository: testRepository(),
			Version:    version,
		})
		assertReleaseErrorIs(t, err, commitInspectionFailure)

		git = newReleaseWhiteboxGit()
		git.hasCommits = false
		_, err = newReleaseWhiteboxService(git, nil).CutRelease(context.Background(), CutReleaseRequest{
			Repository: testRepository(),
			Version:    version,
		})
		assertProblemCode(t, err, problem.CodeRepositoryHasNoCommits)
	})

	t.Run("forwards context to read-only adapters and never pushes", func(t *testing.T) {
		type contextKey struct{}
		ctx := context.WithValue(context.Background(), contextKey{}, "release")
		git := newReleaseWhiteboxGit()

		result, err := newReleaseWhiteboxService(git, nil).CutRelease(ctx, CutReleaseRequest{
			Repository: testRepository(),
			Version:    version,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(git.validateContexts) != 1 || git.validateContexts[0] != ctx ||
			len(git.fetchContexts) != 1 || git.fetchContexts[0] != ctx ||
			len(git.hasCommitsContexts) != 1 || git.hasCommitsContexts[0] != ctx {
			t.Fatalf("release contexts were not forwarded: validate=%v fetch=%v commits=%v",
				git.validateContexts, git.fetchContexts, git.hasCommitsContexts)
		}
		if result.Intent.Branch.Family() != branch.FamilyRelease {
			t.Fatalf("release intent branch = %q", result.Intent.Branch)
		}
		assertReleaseNoCall(t, git.calls, "create-branch")
		assertReleaseNoCall(t, git.calls, "push")
	})
}

func TestReleaseWhiteboxDispatchAndAssessReleaseLifecycle(t *testing.T) {
	release := mustBranch("release/2.8.0")
	version := mustReleaseVersion(t, "2.8.0")

	t.Run("dispatches a prepared protected line and verifies its fetched reference", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		lifecycle := &releaseWhiteboxLifecycle{
			dispatchResult: port.SharedLineDispatchResult{
				WorkflowRunURL: "https://example.invalid/actions/42",
				Branch:         release,
			},
		}
		service := newReleaseWhiteboxService(git, nil).WithReleaseLifecycleProvider(lifecycle)
		intent, err := service.CutRelease(context.Background(), CutReleaseRequest{
			Repository: testRepository(),
			Version:    version,
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := service.DispatchSharedLine(context.Background(), testRepository(), intent.Intent)
		if err != nil || result != lifecycle.dispatchResult || len(lifecycle.dispatches) != 1 {
			t.Fatalf("DispatchSharedLine() = (%#v, %v), dispatches=%#v", result, err, lifecycle.dispatches)
		}
		request := lifecycle.dispatches[0]
		if request.Workflow != "execute-protected-line-request.yml" || request.Ref != "main" ||
			request.Inputs["kind"] != "release" || request.Inputs["version"] != "2.8.0" ||
			request.Branch != release || request.RemoteURL == "" {
			t.Fatalf("dispatch request = %#v", request)
		}
		if countCall(git.calls, "fetch") != 2 {
			t.Fatalf("dispatch fetch calls = %v", git.calls)
		}
	})

	t.Run("rejects incomplete dispatch dependencies, intents, provider failures, and mismatched lines", func(t *testing.T) {
		intent := SharedLineIntent{
			Workflow: "execute-protected-line-request.yml",
			Kind:     "release",
			Branch:   release,
			Inputs:   map[string]string{"kind": "release", "version": "2.8.0"},
		}
		_, err := (&ReleaseService{}).DispatchSharedLine(context.Background(), testRepository(), intent)
		assertProblemCode(t, err, problem.CodeInternal)

		git := newReleaseWhiteboxGit()
		_, err = (&ReleaseService{git: git}).DispatchSharedLine(context.Background(), testRepository(), intent)
		assertProblemCode(t, err, problem.CodeInternal)

		service := newReleaseWhiteboxService(git, nil).WithReleaseLifecycleProvider(&releaseWhiteboxLifecycle{})
		_, err = service.DispatchSharedLine(context.Background(), testRepository(), SharedLineIntent{})
		assertProblemCode(t, err, problem.CodeInvalidInput)

		dispatchFailure := errors.New("dispatch failed")
		service = newReleaseWhiteboxService(git, nil).WithReleaseLifecycleProvider(&releaseWhiteboxLifecycle{dispatchErr: dispatchFailure})
		_, err = service.DispatchSharedLine(context.Background(), testRepository(), intent)
		assertReleaseErrorIs(t, err, dispatchFailure)

		service = newReleaseWhiteboxService(git, nil).WithReleaseLifecycleProvider(&releaseWhiteboxLifecycle{
			dispatchResult: port.SharedLineDispatchResult{Branch: mustBranch("release/2.9.0")},
		})
		_, err = service.DispatchSharedLine(context.Background(), testRepository(), intent)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("covers dispatch repository, remote, fetch, target, and zero-value result paths", func(t *testing.T) {
		intent := SharedLineIntent{
			Workflow: "execute-protected-line-request.yml",
			Kind:     "release",
			Branch:   release,
			Inputs:   map[string]string{"kind": "release", "version": "2.8.0"},
		}
		newService := func(git *releaseLifecycleGit) *ReleaseService {
			return newReleaseWhiteboxService(git, nil).WithReleaseLifecycleProvider(&releaseWhiteboxLifecycle{
				dispatchResult: port.SharedLineDispatchResult{WorkflowRunURL: "https://example.invalid/actions/42"},
			})
		}

		git := &releaseLifecycleGit{releaseWhiteboxGit: newReleaseWhiteboxGit(), targetExists: true}
		service := newService(git)
		result, err := service.DispatchSharedLine(context.Background(), testRepository(), intent)
		if err != nil || result.Branch != release {
			t.Fatalf("zero-value provider branch result = (%#v, %v)", result, err)
		}

		_, err = service.DispatchSharedLine(context.Background(), port.RepositoryIdentity{}, intent)
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)

		git = &releaseLifecycleGit{
			releaseWhiteboxGit: newReleaseWhiteboxGit(),
			remoteURLErr:       errors.New("remote unavailable"),
			targetExists:       true,
		}
		_, err = newService(git).DispatchSharedLine(context.Background(), testRepository(), intent)
		if err == nil || !strings.Contains(err.Error(), "remote unavailable") {
			t.Fatalf("remote URL error = %v", err)
		}

		git = &releaseLifecycleGit{releaseWhiteboxGit: newReleaseWhiteboxGit(), targetExists: true}
		git.fetchErrors = []error{errors.New("fetch dispatched line")}
		_, err = newService(git).DispatchSharedLine(context.Background(), testRepository(), intent)
		if err == nil || !strings.Contains(err.Error(), "fetch dispatched line") {
			t.Fatalf("dispatch fetch error = %v", err)
		}

		git = &releaseLifecycleGit{releaseWhiteboxGit: newReleaseWhiteboxGit(), targetExists: false}
		_, err = newService(git).DispatchSharedLine(context.Background(), testRepository(), intent)
		assertProblemCode(t, err, problem.CodeInvalidInput)

		git = &releaseLifecycleGit{
			releaseWhiteboxGit: newReleaseWhiteboxGit(),
			targetExists:       true,
			targetErr:          errors.New("target lookup failed"),
		}
		_, err = newService(git).DispatchSharedLine(context.Background(), testRepository(), intent)
		if err == nil || !strings.Contains(err.Error(), "target lookup failed") {
			t.Fatalf("dispatch target error = %v", err)
		}

		git = &releaseLifecycleGit{releaseWhiteboxGit: newReleaseWhiteboxGit(), targetExists: true}
		_, err = newService(git).DispatchSharedLine(context.Background(), port.RepositoryIdentity{
			Root:   testRepository().Root,
			Remote: "invalid remote",
		}, intent)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})

	t.Run("plans a dry run and enforces lifecycle evidence for actual reconciliation", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		service := newReleaseWhiteboxService(git, nil)
		planned, err := service.AssessReleaseBackmerge(context.Background(), AssessReleaseBackmergeRequest{
			Repository: testRepository(),
			Release:    release,
			DryRun:     true,
		})
		if err != nil || planned.Status != ReleaseBackmergePlanned || planned.PullRequest == nil ||
			planned.PullRequest.Target.String() != "develop" {
			t.Fatalf("dry-run assessment = (%#v, %v)", planned, err)
		}

		_, err = service.AssessReleaseBackmerge(context.Background(), AssessReleaseBackmergeRequest{
			Repository: testRepository(),
			Release:    release,
		})
		assertProblemCode(t, err, problem.CodeInternal)

		_, err = (&ReleaseService{lifecycle: &releaseWhiteboxLifecycle{}}).AssessReleaseBackmerge(
			context.Background(),
			AssessReleaseBackmergeRequest{Repository: testRepository(), Release: release},
		)
		assertProblemCode(t, err, problem.CodeInternal)

		service = newReleaseWhiteboxService(git, nil).WithReleaseLifecycleProvider(&releaseWhiteboxLifecycle{})
		_, err = service.AssessReleaseBackmerge(context.Background(), AssessReleaseBackmergeRequest{
			Repository: port.RepositoryIdentity{},
			Release:    release,
		})
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)
	})

	t.Run("returns a no-op result only after verified delivery and creates an intent for an effective delta", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		evidence := port.ReleaseReconciliationEvidence{
			PromotionPullRequestURL: "https://example.invalid/pr/8",
			PromotionMergeCommit:    strings.Repeat("a", 40),
			Tag:                     "v2.8.0",
			ReleaseURL:              "https://example.invalid/releases/v2.8.0",
			EffectiveDelta:          false,
		}
		lifecycle := &releaseWhiteboxLifecycle{evidence: evidence}
		service := newReleaseWhiteboxService(git, nil).WithReleaseLifecycleProvider(lifecycle)
		result, err := service.AssessReleaseBackmerge(context.Background(), AssessReleaseBackmergeRequest{
			Repository: testRepository(),
			Release:    release,
		})
		if err != nil || result.Status != ReleaseBackmergeNotRequired || result.PullRequest != nil ||
			len(lifecycle.reconciles) != 1 {
			t.Fatalf("no-op assessment = (%#v, %v), calls=%#v", result, err, lifecycle.reconciles)
		}

		lifecycle.evidence.EffectiveDelta = true
		result, err = service.AssessReleaseBackmerge(context.Background(), AssessReleaseBackmergeRequest{
			Repository: testRepository(),
			Release:    release,
			Draft:      true,
		})
		if err != nil || result.Status != ReleaseBackmergeRequired || result.PullRequest == nil ||
			result.PullRequest.Target.String() != "develop" || !result.PullRequest.Draft {
			t.Fatalf("required assessment = (%#v, %v)", result, err)
		}
	})

	t.Run("rejects invalid reconciliation evidence and provider failures", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		verifyFailure := errors.New("verify delivery")
		service := newReleaseWhiteboxService(git, nil).WithReleaseLifecycleProvider(&releaseWhiteboxLifecycle{verifyErr: verifyFailure})
		_, err := service.AssessReleaseBackmerge(context.Background(), AssessReleaseBackmergeRequest{
			Repository: testRepository(),
			Release:    release,
		})
		assertReleaseErrorIs(t, err, verifyFailure)

		service = newReleaseWhiteboxService(git, nil).WithReleaseLifecycleProvider(&releaseWhiteboxLifecycle{
			evidence: port.ReleaseReconciliationEvidence{
				PromotionMergeCommit: strings.Repeat("a", 40),
				Tag:                  "v2.9.0",
				ReleaseURL:           "https://example.invalid/releases/v2.9.0",
			},
		})
		_, err = service.AssessReleaseBackmerge(context.Background(), AssessReleaseBackmergeRequest{
			Repository: testRepository(),
			Release:    release,
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)

		_, err = service.AssessReleaseBackmerge(context.Background(), AssessReleaseBackmergeRequest{
			Repository: testRepository(),
			Release:    mustBranch("main"),
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)

		lifecycleGit := &releaseLifecycleGit{
			releaseWhiteboxGit: newReleaseWhiteboxGit(),
			remoteURLErr:       errors.New("reconciliation remote unavailable"),
			targetExists:       true,
		}
		service = newReleaseWhiteboxService(lifecycleGit, nil).WithReleaseLifecycleProvider(&releaseWhiteboxLifecycle{})
		_, err = service.AssessReleaseBackmerge(context.Background(), AssessReleaseBackmergeRequest{
			Repository: testRepository(),
			Release:    release,
		})
		if err == nil || !strings.Contains(err.Error(), "reconciliation remote unavailable") {
			t.Fatalf("reconciliation remote error = %v", err)
		}
	})
}

func TestReleaseWhiteboxPrepareSupportProvenance(t *testing.T) {
	version := mustSupportVersion(t, "2.8")

	t.Run("rejects malformed requests and missing dependencies", func(t *testing.T) {
		_, err := (&ReleaseService{}).PrepareSupport(context.Background(), PrepareSupportRequest{
			Repository: testRepository(),
		})
		assertProblemCode(t, err, problem.CodeBranchNameInvalid)

		git := newReleaseWhiteboxGit()
		_, err = newReleaseWhiteboxService(git, nil).PrepareSupport(context.Background(), PrepareSupportRequest{
			Repository: port.RepositoryIdentity{},
			Version:    version,
		})
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)

		_, err = (&ReleaseService{}).PrepareSupport(context.Background(), PrepareSupportRequest{
			Repository: testRepository(),
			Version:    version,
		})
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("stops on source fetch, tag lookup, and protected-line fetch failures", func(t *testing.T) {
		fetchFailure := errors.New("fetch main")
		git := newReleaseWhiteboxGit()
		git.fetchErrors = []error{fetchFailure}
		_, err := newReleaseWhiteboxService(git, nil).PrepareSupport(context.Background(), PrepareSupportRequest{
			Repository: testRepository(),
			Version:    version,
		})
		assertReleaseErrorIs(t, err, fetchFailure)

		tagFailure := errors.New("read release tags")
		git = newReleaseWhiteboxGit()
		git.releaseTagsErr = tagFailure
		_, err = newReleaseWhiteboxService(git, nil).PrepareSupport(context.Background(), PrepareSupportRequest{
			Repository: testRepository(),
			Version:    version,
		})
		assertReleaseErrorIs(t, err, tagFailure)

		sharedLineFetchFailure := errors.New("fetch support source")
		git = newReleaseWhiteboxGit()
		git.releaseTags = []string{"v2.8.0"}
		git.fetchErrors = []error{nil, sharedLineFetchFailure}
		_, err = newReleaseWhiteboxService(git, nil).PrepareSupport(context.Background(), PrepareSupportRequest{
			Repository: testRepository(),
			Version:    version,
		})
		assertReleaseErrorIs(t, err, sharedLineFetchFailure)
	})

	t.Run("requires a matching main release tag", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		git.releaseTags = []string{"v2.9.0", "not-a-version"}

		_, err := newReleaseWhiteboxService(git, nil).PrepareSupport(context.Background(), PrepareSupportRequest{
			Repository: testRepository(),
			Version:    version,
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)
		assertReleaseNoCall(t, git.calls, "validate-ref")
	})

	t.Run("skips tag inspection only for dry runs", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		result, err := newReleaseWhiteboxService(git, nil).PrepareSupport(context.Background(), PrepareSupportRequest{
			Repository: testRepository(),
			Version:    version,
			DryRun:     true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !result.DryRun || result.Intent.Kind != "support" || result.Intent.Source.String() != "origin/main" {
			t.Fatalf("PrepareSupport() = %#v", result)
		}
		assertReleaseNoCall(t, git.calls, "fetch")
		assertReleaseNoCall(t, git.calls, "release-tags")
		assertReleaseNoCall(t, git.calls, "create-branch")
		assertReleaseNoCall(t, git.calls, "push")
	})

	t.Run("requires commits after provenance is established", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		git.releaseTags = []string{"v2.8.0"}
		git.hasCommits = false

		_, err := newReleaseWhiteboxService(git, nil).PrepareSupport(context.Background(), PrepareSupportRequest{
			Repository: testRepository(),
			Version:    version,
		})
		assertProblemCode(t, err, problem.CodeRepositoryHasNoCommits)
	})

	t.Run("creates a CI-owned intent from the tagged main line", func(t *testing.T) {
		type contextKey struct{}
		ctx := context.WithValue(context.Background(), contextKey{}, "support")
		git := newReleaseWhiteboxGit()
		git.releaseTags = []string{"v2.8.0"}

		result, err := newReleaseWhiteboxService(git, nil).PrepareSupport(ctx, PrepareSupportRequest{
			Repository: testRepository(),
			Version:    version,
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.Intent.Branch.String() != "support/2.8" || result.Intent.Source.String() != "origin/main" ||
			result.Intent.Inputs["kind"] != "support" || countCall(git.calls, "fetch") != 2 ||
			len(git.releaseTagRevisions) != 1 || git.releaseTagRevisions[0] != "origin/main" {
			t.Fatalf("PrepareSupport() = %#v, calls=%v, revisions=%v", result, git.calls, git.releaseTagRevisions)
		}
		if len(git.fetchContexts) != 2 || git.fetchContexts[0] != ctx || git.fetchContexts[1] != ctx {
			t.Fatalf("fetch contexts = %v, want %v", git.fetchContexts, ctx)
		}
		assertReleaseNoCall(t, git.calls, "create-branch")
		assertReleaseNoCall(t, git.calls, "push")
	})
}

func TestReleaseWhiteboxReleaseStabilizationBoundaries(t *testing.T) {
	t.Run("requires branches and a release line", func(t *testing.T) {
		_, err := (&ReleaseService{}).CreateReleaseStabilization(context.Background(), releaseStabilizationRequest())
		assertProblemCode(t, err, problem.CodeInternal)

		git := newReleaseWhiteboxGit()
		request := releaseStabilizationRequest()
		request.Release = mustBranch("main")
		_, err = newReleaseWhiteboxService(git, nil).CreateReleaseStabilization(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInvalidInput)
		assertReleaseNoCall(t, git.calls, "validate-ref")
	})

	t.Run("rejects unsupported kinds, repositories, and remotes before creation", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		request := releaseStabilizationRequest()
		request.Kind = "feature"
		_, err := newReleaseWhiteboxService(git, nil).CreateReleaseStabilization(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInvalidInput)

		git = newReleaseWhiteboxGit()
		request = releaseStabilizationRequest()
		request.Repository = port.RepositoryIdentity{}
		_, err = newReleaseWhiteboxService(git, nil).CreateReleaseStabilization(context.Background(), request)
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)

		git = newReleaseWhiteboxGit()
		request = releaseStabilizationRequest()
		request.Repository = port.RepositoryIdentity{Root: testRepository().Root, Remote: "bad remote"}
		_, err = newReleaseWhiteboxService(git, nil).CreateReleaseStabilization(context.Background(), request)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})

	t.Run("honors cancellation and propagates creation validation failures", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		git := newReleaseWhiteboxGit()
		_, err := newReleaseWhiteboxService(git, nil).CreateReleaseStabilization(ctx, releaseStabilizationRequest())
		assertProblemCode(t, err, problem.CodeOperationCancelled)

		validationFailure := errors.New("validate stabilization ref")
		git = newReleaseWhiteboxGit()
		git.validateErr = validationFailure
		_, err = newReleaseWhiteboxService(git, nil).CreateReleaseStabilization(context.Background(), releaseStabilizationRequest())
		assertReleaseErrorIs(t, err, validationFailure)
	})

	t.Run("uses only a dry-run plan when requested", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		switchToBranch := false
		request := releaseStabilizationRequest()
		request.Switch = &switchToBranch
		request.DryRun = true

		result, err := newReleaseWhiteboxService(git, nil).CreateReleaseStabilization(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if !result.DryRun || result.Switched || result.Base.String() != "origin/release/2.8.0" {
			t.Fatalf("CreateReleaseStabilization() = %#v", result)
		}
		assertReleaseNoCall(t, git.calls, "create-branch")
		assertReleaseNoCall(t, git.calls, "store-workflow-base")
	})

	t.Run("preflights the metadata dependency before local mutation", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		branches := branchapp.NewService(git, &fakeKeyPolicy{})
		service := NewReleaseService(branches, nil, nil)

		_, err := service.CreateReleaseStabilization(context.Background(), releaseStabilizationRequest())
		assertProblemCode(t, err, problem.CodeInternal)
		assertReleaseNoCall(t, git.calls, "create-branch")
	})

	t.Run("maps each permitted kind and records the release provenance", func(t *testing.T) {
		for _, testCase := range []struct {
			kind   ReleaseStabilizationKind
			family branch.Family
		}{
			{kind: ReleaseStabilizationBlocker, family: branch.FamilyFix},
			{kind: ReleaseStabilizationDocs, family: branch.FamilyDocs},
			{kind: ReleaseStabilizationPrep, family: branch.FamilyChore},
		} {
			t.Run(string(testCase.kind), func(t *testing.T) {
				git := newReleaseWhiteboxGit()
				request := releaseStabilizationRequest()
				request.Kind = testCase.kind

				result, err := newReleaseWhiteboxService(git, nil).CreateReleaseStabilization(context.Background(), request)
				if err != nil {
					t.Fatal(err)
				}
				if result.Name.Family() != testCase.family || git.workflowBases[result.Name.String()].String() != "origin/release/2.8.0" {
					t.Fatalf("CreateReleaseStabilization() = %#v, bases=%v", result, git.workflowBases)
				}
			})
		}
	})

	t.Run("propagates provenance storage failures", func(t *testing.T) {
		storeFailure := errors.New("store stabilization base")
		git := newReleaseWhiteboxGit()
		git.storeErr = storeFailure

		_, err := newReleaseWhiteboxService(git, nil).CreateReleaseStabilization(context.Background(), releaseStabilizationRequest())
		assertReleaseErrorIs(t, err, storeFailure)
	})
}

func TestReleaseWhiteboxPromotionAndBackmergePublication(t *testing.T) {
	release := mustBranch("release/2.8.0")

	t.Run("promotion validates inputs and supports no-publisher paths", func(t *testing.T) {
		_, err := (&ReleaseService{}).PrepareReleasePromotion(context.Background(), PrepareReleasePromotionRequest{
			Repository: testRepository(),
			Release:    mustBranch("main"),
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)

		_, err = (&ReleaseService{}).PrepareReleasePromotion(context.Background(), PrepareReleasePromotionRequest{
			Repository: port.RepositoryIdentity{},
			Release:    release,
		})
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)

		result, err := (&ReleaseService{}).PrepareReleasePromotion(context.Background(), PrepareReleasePromotionRequest{
			Repository: testRepository(),
			Release:    release,
			DryRun:     true,
		})
		if err != nil || !result.DryRun || result.PublishedURL != "" || result.PullRequest.Target.String() != "main" {
			t.Fatalf("dry-run promotion = (%#v, %v)", result, err)
		}

		result, err = (&ReleaseService{}).PrepareReleasePromotion(context.Background(), PrepareReleasePromotionRequest{
			Repository: testRepository(),
			Release:    release,
		})
		if err != nil || result.PublishedURL != "" {
			t.Fatalf("unpublished promotion = (%#v, %v)", result, err)
		}
	})

	t.Run("promotion requires the mandatory description before publication", func(t *testing.T) {
		_, err := (&ReleaseService{git: &fakeGitRepository{}, publisher: &releaseWhiteboxPublisher{}}).PrepareReleasePromotion(context.Background(), PrepareReleasePromotionRequest{
			Repository:        testRepository(),
			Release:           release,
			CreatePullRequest: true,
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("promotion propagates publisher errors and emits a complete PR intent", func(t *testing.T) {
		publishFailure := errors.New("publish promotion")
		publisher := &releaseWhiteboxPublisher{err: publishFailure}
		_, err := (&ReleaseService{git: &fakeGitRepository{}, publisher: publisher}).PrepareReleasePromotion(context.Background(), PrepareReleasePromotionRequest{
			Repository:        testRepository(),
			Release:           release,
			CreatePullRequest: true,
			Body:              "Summary: Promote release 2.8.0 into main.\n\nScope and Non-Goals: The frozen release line only.\n\nCommit Series:\n- release 2.8.0 stabilization\n\nRisk and Rollback: Production promotion; rollback reverts the merge.\n\nVerification and Review Focus: Stabilization evidence; review the release contents.",
		})
		assertReleaseErrorIs(t, err, publishFailure)

		type contextKey struct{}
		ctx := context.WithValue(context.Background(), contextKey{}, "promotion")
		publisher = &releaseWhiteboxPublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/promotion"}}
		result, err := (&ReleaseService{git: &fakeGitRepository{}, publisher: publisher}).PrepareReleasePromotion(ctx, PrepareReleasePromotionRequest{
			Repository:        testRepository(),
			Release:           release,
			CreatePullRequest: true,
			Draft:             true,
			Body:              "Summary: Promote release 2.8.0 into main.\n\nScope and Non-Goals: The frozen release line only.\n\nCommit Series:\n- release 2.8.0 stabilization\n\nRisk and Rollback: Production promotion; rollback reverts the merge.\n\nVerification and Review Focus: Stabilization evidence; review the release contents.",
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.PublishedURL == "" || result.PullRequest.Source.String() != "release/2.8.0" ||
			result.PullRequest.Target.String() != "main" || result.PullRequest.Title != "Release 2.8.0 into main" ||
			!result.PullRequest.Draft || len(publisher.contexts) != 1 || publisher.contexts[0] != ctx {
			t.Fatalf("promotion result = %#v, publisher=%#v", result, publisher)
		}
	})

	t.Run("backmerge validates inputs and supports no-publisher paths", func(t *testing.T) {
		_, err := (&ReleaseService{}).PrepareReleaseBackmerge(context.Background(), PrepareReleaseBackmergeRequest{
			Repository: testRepository(),
			Release:    mustBranch("main"),
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)

		_, err = (&ReleaseService{}).PrepareReleaseBackmerge(context.Background(), PrepareReleaseBackmergeRequest{
			Repository: port.RepositoryIdentity{},
			Release:    release,
		})
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)

		result, err := (&ReleaseService{}).PrepareReleaseBackmerge(context.Background(), PrepareReleaseBackmergeRequest{
			Repository: testRepository(),
			Release:    release,
			DryRun:     true,
		})
		if err != nil || !result.DryRun || result.PublishedURL != "" || result.PullRequest.Target.String() != "develop" {
			t.Fatalf("dry-run backmerge = (%#v, %v)", result, err)
		}

		result, err = (&ReleaseService{}).PrepareReleaseBackmerge(context.Background(), PrepareReleaseBackmergeRequest{
			Repository: testRepository(),
			Release:    release,
		})
		if err != nil || result.PublishedURL != "" {
			t.Fatalf("unpublished backmerge = (%#v, %v)", result, err)
		}
	})

	t.Run("backmerge requires the mandatory description before publication", func(t *testing.T) {
		_, err := (&ReleaseService{git: &fakeGitRepository{}, publisher: &releaseWhiteboxPublisher{}}).PrepareReleaseBackmerge(context.Background(), PrepareReleaseBackmergeRequest{
			Repository:        testRepository(),
			Release:           release,
			CreatePullRequest: true,
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("backmerge propagates publisher errors and emits a complete PR intent", func(t *testing.T) {
		publishFailure := errors.New("publish backmerge")
		publisher := &releaseWhiteboxPublisher{err: publishFailure}
		_, err := (&ReleaseService{git: &fakeGitRepository{}, publisher: publisher}).PrepareReleaseBackmerge(context.Background(), PrepareReleaseBackmergeRequest{
			Repository:        testRepository(),
			Release:           release,
			CreatePullRequest: true,
			Body:              "Summary: Backmerge release 2.8.0 into develop.\n\nScope and Non-Goals: The effective release delta only.\n\nCommit Series:\n- release 2.8.0 changes\n\nRisk and Rollback: Low; the release is delivered.\n\nVerification and Review Focus: Delivery evidence verified; review the delta.",
		})
		assertReleaseErrorIs(t, err, publishFailure)

		type contextKey struct{}
		ctx := context.WithValue(context.Background(), contextKey{}, "backmerge")
		publisher = &releaseWhiteboxPublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/backmerge"}}
		result, err := (&ReleaseService{git: &fakeGitRepository{}, publisher: publisher}).PrepareReleaseBackmerge(ctx, PrepareReleaseBackmergeRequest{
			Repository:        testRepository(),
			Release:           release,
			CreatePullRequest: true,
			Draft:             true,
			Body:              "Summary: Backmerge release 2.8.0 into develop.\n\nScope and Non-Goals: The effective release delta only.\n\nCommit Series:\n- release 2.8.0 changes\n\nRisk and Rollback: Low; the release is delivered.\n\nVerification and Review Focus: Delivery evidence verified; review the delta.",
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.PublishedURL == "" || result.PullRequest.Source.String() != "release/2.8.0" ||
			result.PullRequest.Target.String() != "develop" || result.PullRequest.Title != "Backmerge release 2.8.0 into develop" ||
			!result.PullRequest.Draft || len(publisher.contexts) != 1 || publisher.contexts[0] != ctx {
			t.Fatalf("backmerge result = %#v, publisher=%#v", result, publisher)
		}
	})
}

func TestReleaseWhiteboxPropagateHotfixBoundaries(t *testing.T) {
	t.Run("requires every composed workflow dependency", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		_, err := NewReleaseService(nil, git, nil).WithTicketService(&TicketService{}).PropagateHotfix(context.Background(), releasePropagationRequest())
		assertProblemCode(t, err, problem.CodeInternal)

		branches := branchapp.NewService(git, &fakeKeyPolicy{})
		_, err = NewReleaseService(branches, nil, nil).WithTicketService(&TicketService{}).PropagateHotfix(context.Background(), releasePropagationRequest())
		assertProblemCode(t, err, problem.CodeInternal)

		_, err = newReleaseWhiteboxServiceWithoutTickets(git).PropagateHotfix(context.Background(), releasePropagationRequest())
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("rejects source, target, commit, and repository inputs before mutations", func(t *testing.T) {
		for _, mutate := range []func(*PropagateHotfixRequest){
			func(request *PropagateHotfixRequest) { request.Source = mustBranch("feature/ABC-999-payment-timeout") },
			func(request *PropagateHotfixRequest) { request.TargetLine = mustBranch("feature/ABC-998-another-line") },
			func(request *PropagateHotfixRequest) { request.CommitID = "not-a-sha" },
			func(request *PropagateHotfixRequest) { request.Repository = port.RepositoryIdentity{} },
			func(request *PropagateHotfixRequest) {
				request.Repository = port.RepositoryIdentity{Root: testRepository().Root, Remote: "bad remote"}
			},
		} {
			git := newReleaseWhiteboxGit()
			request := releasePropagationRequest()
			mutate(&request)

			_, err := newReleaseWhiteboxService(git, nil).PropagateHotfix(context.Background(), request)
			if err == nil {
				t.Fatal("invalid propagation succeeded")
			}
			assertReleaseNoCall(t, git.calls, "create-branch")
			assertReleaseNoCall(t, git.calls, "store-workflow-base")
			assertReleaseNoCall(t, git.calls, "cherry-pick")
		}
	})

	t.Run("requires the mandatory description before any propagation mutation", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		request := releasePropagationRequest()
		request.Push = true
		request.CreatePullRequest = true
		request.Body = "  "

		_, err := newReleaseWhiteboxService(git, nil).PropagateHotfix(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInvalidInput)
		assertReleaseNoCall(t, git.calls, "create-branch")
		assertReleaseNoCall(t, git.calls, "store-workflow-base")
		assertReleaseNoCall(t, git.calls, "cherry-pick")
	})

	t.Run("fails safely when the derived forward-port slug is invalid", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		request := releasePropagationRequest()
		request.Source = mustBranch("hotfix/ABC-999-" + strings.Repeat("a", 100))
		request.Slug = branch.Slug{}

		_, err := newReleaseWhiteboxService(git, nil).PropagateHotfix(context.Background(), request)
		assertProblemCode(t, err, problem.CodeBranchSlugInvalid)
		assertReleaseNoCall(t, git.calls, "create-branch")
	})

	t.Run("propagates working-branch creation failures", func(t *testing.T) {
		createFailure := errors.New("create forward-port branch")
		git := newReleaseWhiteboxGit()
		git.createErr = createFailure

		_, err := newReleaseWhiteboxService(git, nil).PropagateHotfix(context.Background(), releasePropagationRequest())
		assertReleaseErrorIs(t, err, createFailure)
		assertReleaseNoCall(t, git.calls, "store-workflow-base")
		assertReleaseNoCall(t, git.calls, "cherry-pick")
	})

	t.Run("supports every active target line in dry-run mode", func(t *testing.T) {
		for _, target := range []string{"main", "develop", "release/2.8.0", "support/2.8"} {
			t.Run(target, func(t *testing.T) {
				git := newReleaseWhiteboxGit()
				request := releasePropagationRequest()
				request.TargetLine = mustBranch(target)
				request.DryRun = true

				result, err := newReleaseWhiteboxService(git, nil).PropagateHotfix(context.Background(), request)
				if err != nil {
					t.Fatal(err)
				}
				if !result.Branch.DryRun || !result.Publication.DryRun ||
					result.Publication.PullRequest.Target.String() != target ||
					result.Publication.PullRequest.Source.String() != result.Branch.Name.String() {
					t.Fatalf("dry-run propagation = %#v", result)
				}
				assertReleaseNoCall(t, git.calls, "create-branch")
				assertReleaseNoCall(t, git.calls, "store-workflow-base")
				assertReleaseNoCall(t, git.calls, "cherry-pick")
				assertReleaseNoCall(t, git.calls, "push")
			})
		}
	})

	t.Run("derives a default propagation slug", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		request := releasePropagationRequest()
		request.Slug = branch.Slug{}
		request.TargetLine = mustBranch("support/2.8")
		request.DryRun = true

		result, err := newReleaseWhiteboxService(git, nil).PropagateHotfix(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if result.Branch.Name.String() != "fix/ABC-999-forward-port-payment-timeout" {
			t.Fatalf("derived branch = %q", result.Branch.Name)
		}
	})

	t.Run("propagates metadata, cherry-pick, and publication failures", func(t *testing.T) {
		storeFailure := errors.New("store propagation base")
		git := newReleaseWhiteboxGit()
		git.storeErr = storeFailure
		_, err := newReleaseWhiteboxService(git, nil).PropagateHotfix(context.Background(), releasePropagationRequest())
		assertReleaseErrorIs(t, err, storeFailure)
		assertReleaseNoCall(t, git.calls, "cherry-pick")

		cherryPickFailure := errors.New("cherry-pick propagation")
		git = newReleaseWhiteboxGit()
		git.cherryPickErr = cherryPickFailure
		_, err = newReleaseWhiteboxService(git, nil).PropagateHotfix(context.Background(), releasePropagationRequest())
		assertReleaseErrorIs(t, err, cherryPickFailure)

		publicationFailure := errors.New("validate propagated commit series")
		git = newReleaseWhiteboxGit()
		git.commitMessagesErr = publicationFailure
		_, err = newReleaseWhiteboxService(git, nil).PropagateHotfix(context.Background(), releasePropagationRequest())
		assertReleaseErrorIs(t, err, publicationFailure)
	})

	t.Run("pushes only the derived working branch and publishes its PR", func(t *testing.T) {
		type contextKey struct{}
		ctx := context.WithValue(context.Background(), contextKey{}, "propagation")
		git := newReleaseWhiteboxGit()
		publisher := &releaseWhiteboxPublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/forward-port"}}
		request := releasePropagationRequest()
		request.Push = true
		request.CreatePullRequest = true

		result, err := newReleaseWhiteboxService(git, publisher).PropagateHotfix(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		if !result.CherryPicked || !result.Publication.Pushed || result.Publication.PublishedURL == "" ||
			len(git.pushed) != 1 || git.pushed[0].String() != result.Branch.Name.String() ||
			git.pushed[0].Family() != branch.FamilyFix || len(publisher.requests) != 1 ||
			publisher.requests[0].Target.String() != "main" {
			t.Fatalf("propagation = %#v, pushes=%v, publications=%#v", result, git.pushed, publisher.requests)
		}
		if len(git.storeContexts) != 1 || git.storeContexts[0] != ctx ||
			len(git.cherryPickContexts) != 1 || git.cherryPickContexts[0] != ctx ||
			len(publisher.contexts) != 1 || publisher.contexts[0] != ctx {
			t.Fatalf("propagation contexts were not forwarded")
		}
		if git.pushed[0].String() == request.TargetLine.String() {
			t.Fatalf("protected target line %q was pushed directly", request.TargetLine)
		}
	})
}

func TestReleaseWhiteboxResumeHotfixPropagation(t *testing.T) {
	source := mustBranch("hotfix/ABC-999-payment-timeout")
	target := mustBranch("main")
	propagation := mustBranch("fix/ABC-999-forward-port-payment-timeout")
	base := mustBase("origin", "main")
	request := func() ResumeHotfixPropagationRequest {
		return ResumeHotfixPropagationRequest{
			Repository: testRepository(),
			Source:     source,
			TargetLine: target,
			Branch:     propagation,
		}
	}
	configure := func(git *releaseWhiteboxGit) {
		git.workflowBases = map[string]branch.TargetBase{propagation.String(): base}
	}

	t.Run("rejects dependencies and invalid inputs before mutation", func(t *testing.T) {
		_, err := (&ReleaseService{}).ResumeHotfixPropagation(context.Background(), request())
		assertProblemCode(t, err, problem.CodeInternal)

		for _, mutate := range []func(*ResumeHotfixPropagationRequest){
			func(value *ResumeHotfixPropagationRequest) { value.Source = mustBranch("feature/ABC-999-not-hotfix") },
			func(value *ResumeHotfixPropagationRequest) {
				value.TargetLine = mustBranch("feature/ABC-998-not-a-line")
			},
			func(value *ResumeHotfixPropagationRequest) {
				value.Branch = mustBranch("feature/ABC-999-not-a-propagation")
			},
			func(value *ResumeHotfixPropagationRequest) { value.Branch = mustBranch("fix/ABC-998-wrong-ticket") },
			func(value *ResumeHotfixPropagationRequest) { value.CreatePullRequest = true },
			func(value *ResumeHotfixPropagationRequest) {
				value.Push = true
				value.CreatePullRequest = true
				value.Body = ""
			},
		} {
			git := newReleaseWhiteboxGit()
			configure(git)
			candidate := request()
			mutate(&candidate)
			_, err := newReleaseWhiteboxService(git, nil).ResumeHotfixPropagation(context.Background(), candidate)
			assertProblemCode(t, err, problem.CodeInvalidInput)
			assertReleaseNoCall(t, git.calls, "continue-cherry-pick")
		}

		git := newReleaseWhiteboxGit()
		configure(git)
		candidate := request()
		candidate.Repository = port.RepositoryIdentity{}
		_, err = newReleaseWhiteboxService(git, nil).ResumeHotfixPropagation(context.Background(), candidate)
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)

		git = newReleaseWhiteboxGit()
		configure(git)
		candidate = request()
		candidate.Repository.Remote = "bad remote"
		_, err = newReleaseWhiteboxService(git, nil).ResumeHotfixPropagation(context.Background(), candidate)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})

	t.Run("requires matching stored workflow provenance", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		_, err := newReleaseWhiteboxService(git, nil).ResumeHotfixPropagation(context.Background(), request())
		assertProblemCode(t, err, problem.CodeInvalidInput)

		git = newReleaseWhiteboxGit()
		configure(git)
		git.workflowBaseErr = errors.New("workflow base unavailable")
		_, err = newReleaseWhiteboxService(git, nil).ResumeHotfixPropagation(context.Background(), request())
		if !strings.Contains(err.Error(), "workflow base unavailable") {
			t.Fatalf("workflow-base error = %v", err)
		}

		git = newReleaseWhiteboxGit()
		git.workflowBases = map[string]branch.TargetBase{propagation.String(): mustBase("origin", "develop")}
		_, err = newReleaseWhiteboxService(git, nil).ResumeHotfixPropagation(context.Background(), request())
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("requires a resumable cherry-pick when Git is paused", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		configure(git)
		git.active = true
		git.activeOperation = "merge"
		_, err := newReleaseWhiteboxService(git, nil).ResumeHotfixPropagation(context.Background(), request())
		assertProblemCode(t, err, problem.CodeInvalidInput)

		git = newReleaseWhiteboxGit()
		configure(git)
		git.active = true
		git.activeOperation = "cherry-pick"
		_, err = newReleaseWhiteboxService(nonContinuingGit{GitRepository: git}, nil).ResumeHotfixPropagation(context.Background(), request())
		assertProblemCode(t, err, problem.CodeInternal)

		git = newReleaseWhiteboxGit()
		configure(git)
		git.active = true
		git.activeOperation = "cherry-pick"
		git.continueErr = errors.New("still conflicted")
		_, err = newReleaseWhiteboxService(git, nil).ResumeHotfixPropagation(context.Background(), request())
		assertProblemCode(t, err, problem.CodeCherryPickConflict)

		git = newReleaseWhiteboxGit()
		configure(git)
		git.activeErr = errors.New("inspect active operation")
		_, err = newReleaseWhiteboxService(git, nil).ResumeHotfixPropagation(context.Background(), request())
		if !strings.Contains(err.Error(), "inspect active operation") {
			t.Fatalf("active-operation error = %v", err)
		}
	})

	t.Run("continues a paused or already-completed cherry-pick", func(t *testing.T) {
		for _, paused := range []bool{true, false} {
			git := newReleaseWhiteboxGit()
			configure(git)
			git.active = paused
			if paused {
				git.activeOperation = "cherry-pick"
			}
			result, err := newReleaseWhiteboxService(git, nil).ResumeHotfixPropagation(context.Background(), request())
			if err != nil || !result.CherryPicked || result.Branch.Name != propagation ||
				result.Publication.PullRequest.Target != target {
				t.Fatalf("ResumeHotfixPropagation() = (%#v, %v)", result, err)
			}
			if paused && countCall(git.calls, "continue-cherry-pick") != 1 {
				t.Fatalf("continuation calls = %v", git.calls)
			}
		}

		git := newReleaseWhiteboxGit()
		configure(git)
		git.messages = nil
		_, err := newReleaseWhiteboxService(git, nil).ResumeHotfixPropagation(context.Background(), request())
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("classifies only paused cherry-pick failures", func(t *testing.T) {
		cause := errors.New("cherry-pick failed")
		git := newReleaseWhiteboxGit()
		configure(git)
		if err := newReleaseWhiteboxService(git, nil).classifyCherryPickFailure(context.Background(), testRepository(), cause); !errors.Is(err, cause) {
			t.Fatalf("inactive failure = %v, want %v", err, cause)
		}

		git.active = true
		git.activeOperation = "cherry-pick"
		err := newReleaseWhiteboxService(git, nil).classifyCherryPickFailure(context.Background(), testRepository(), cause)
		assertProblemCode(t, err, problem.CodeCherryPickConflict)
	})
}

func TestReleaseWhiteboxCleanupBranchBoundaries(t *testing.T) {
	scratch := mustBranch("scratch/ABC-999-cleanup")

	t.Run("requires Git and accepts only private scratch branches", func(t *testing.T) {
		_, err := (&ReleaseService{}).CleanupBranch(context.Background(), CleanupBranchRequest{
			Repository: testRepository(),
			Branch:     scratch,
		})
		assertProblemCode(t, err, problem.CodeInternal)

		git := newReleaseWhiteboxGit()
		_, err = newReleaseWhiteboxService(git, nil).CleanupBranch(context.Background(), CleanupBranchRequest{
			Repository: testRepository(),
			Branch:     mustBranch("release/2.8.0"),
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)
		assertReleaseNoCall(t, git.calls, "delete-local-branch")
	})

	t.Run("rejects missing repositories before deletion", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		_, err := newReleaseWhiteboxService(git, nil).CleanupBranch(context.Background(), CleanupBranchRequest{
			Repository: port.RepositoryIdentity{},
			Branch:     scratch,
		})
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)
		assertReleaseNoCall(t, git.calls, "delete-local-branch")
	})

	t.Run("does not mutate during a dry run", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		result, err := newReleaseWhiteboxService(git, nil).CleanupBranch(context.Background(), CleanupBranchRequest{
			Repository: testRepository(),
			Branch:     scratch,
			DryRun:     true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !result.DryRun || result.DeletedLocal || result.MetadataCleared {
			t.Fatalf("dry-run cleanup = %#v", result)
		}
		assertReleaseNoCall(t, git.calls, "delete-local-branch")
		assertReleaseNoCall(t, git.calls, "clear-workflow-base")
	})

	t.Run("propagates deletion and metadata cleanup failures", func(t *testing.T) {
		deleteFailure := errors.New("delete scratch")
		git := newReleaseWhiteboxGit()
		git.deleteErr = deleteFailure
		_, err := newReleaseWhiteboxService(git, nil).CleanupBranch(context.Background(), CleanupBranchRequest{
			Repository: testRepository(),
			Branch:     scratch,
		})
		assertReleaseErrorIs(t, err, deleteFailure)
		assertReleaseNoCall(t, git.calls, "clear-workflow-base")

		clearFailure := errors.New("clear scratch metadata")
		git = newReleaseWhiteboxGit()
		git.clearErr = clearFailure
		_, err = newReleaseWhiteboxService(git, nil).CleanupBranch(context.Background(), CleanupBranchRequest{
			Repository: testRepository(),
			Branch:     scratch,
		})
		assertReleaseErrorIs(t, err, clearFailure)
		if countCall(git.calls, "delete-local-branch") != 1 {
			t.Fatalf("metadata failure did not follow local deletion: %v", git.calls)
		}
	})

	t.Run("deletes only the local scratch branch and its metadata", func(t *testing.T) {
		type contextKey struct{}
		ctx := context.WithValue(context.Background(), contextKey{}, "cleanup")
		git := newReleaseWhiteboxGit()
		git.workflowBases = map[string]branch.TargetBase{scratch.String(): mustBase("origin", "develop")}

		result, err := newReleaseWhiteboxService(git, nil).CleanupBranch(ctx, CleanupBranchRequest{
			Repository: testRepository(),
			Branch:     scratch,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !result.DeletedLocal || !result.MetadataCleared || len(git.deletedBranches) != 1 ||
			git.deletedBranches[0].String() != scratch.String() || !git.deleteForces[0] ||
			len(git.deleteContexts) != 1 || git.deleteContexts[0] != ctx ||
			len(git.clearContexts) != 1 || git.clearContexts[0] != ctx {
			t.Fatalf("cleanup result = %#v, deleted=%v", result, git.deletedBranches)
		}
		if _, found := git.workflowBases[scratch.String()]; found {
			t.Fatalf("workflow metadata for %q was retained", scratch)
		}
		assertReleaseNoCall(t, git.calls, "push")
	})
}

func TestReleaseWhiteboxHelperBranches(t *testing.T) {
	t.Run("stabilization kinds map to the only permitted families", func(t *testing.T) {
		for _, testCase := range []struct {
			kind   ReleaseStabilizationKind
			family branch.Family
		}{
			{ReleaseStabilizationBlocker, branch.FamilyFix},
			{ReleaseStabilizationDocs, branch.FamilyDocs},
			{ReleaseStabilizationPrep, branch.FamilyChore},
		} {
			family, err := stabilizationFamily(testCase.kind)
			if err != nil || family != testCase.family {
				t.Fatalf("stabilizationFamily(%q) = (%q, %v)", testCase.kind, family, err)
			}
		}
		_, err := stabilizationFamily("feature")
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("matches only released support line tags", func(t *testing.T) {
		version := mustSupportVersion(t, "2.8")
		if hasMatchingSupportReleaseTag([]string{"invalid", "v2.9.0", "2.8.0"}, version) != true {
			t.Fatal("matching support tag was not recognized")
		}
		if hasMatchingSupportReleaseTag([]string{"vv2.8.0", "v2.8"}, version) {
			t.Fatal("invalid tags matched a support line")
		}
		if hasMatchingSupportReleaseTag(nil, version) {
			t.Fatal("empty tag set matched a support line")
		}
	})

	t.Run("normalizes the default remote and preserves explicit input", func(t *testing.T) {
		_, err := normalizeWorkflowRepository(port.RepositoryIdentity{})
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)

		normalized, err := normalizeWorkflowRepository(port.RepositoryIdentity{Root: testRepository().Root})
		if err != nil || normalized.Remote != "origin" {
			t.Fatalf("default normalization = (%#v, %v)", normalized, err)
		}

		normalized, err = normalizeWorkflowRepository(port.RepositoryIdentity{Root: testRepository().Root, Remote: "upstream"})
		if err != nil || normalized.Remote != "upstream" {
			t.Fatalf("explicit normalization = (%#v, %v)", normalized, err)
		}
	})

	if mustMain().String() != "main" {
		t.Fatalf("mustMain() = %q", mustMain())
	}
}

var _ port.GitRepository = (*releaseWhiteboxGit)(nil)
var _ port.PullRequestPublisher = (*releaseWhiteboxPublisher)(nil)
