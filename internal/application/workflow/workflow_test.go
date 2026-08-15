package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

type fakeGitRepository struct {
	hasCommits      bool
	clean           bool
	exists          bool
	publication     branch.PublicationState
	missing         bool
	messages        []string
	messageBatches  [][]string
	err             error
	validateRefErr  error
	workflowBaseErr error
	activeErr       error
	continueErr     error
	publicationErr  error
	missingErr      error
	pushErr         error
	activeOperation string
	active          bool
	calls           []string
	createdNames    []branch.BranchName
	createdBases    []branch.TargetBase
	createdSwitches []bool
	pushed          []branch.BranchName
	cherryPicked    []string
	releaseTags     []string
	workflowBases   map[string]branch.TargetBase
}

func TestParseReleaseStabilizationKind(t *testing.T) {
	t.Parallel()

	for _, value := range []ReleaseStabilizationKind{
		ReleaseStabilizationBlocker,
		ReleaseStabilizationDocs,
		ReleaseStabilizationPrep,
	} {
		value := value
		t.Run(string(value), func(t *testing.T) {
			actual, err := ParseReleaseStabilizationKind(string(value))
			if err != nil || actual != value {
				t.Fatalf("ParseReleaseStabilizationKind(%q) = (%q, %v)", value, actual, err)
			}
		})
	}
	_, err := ParseReleaseStabilizationKind("feature")
	if err == nil {
		t.Fatal("ParseReleaseStabilizationKind accepted an unsupported category")
	}
	actual, ok := problem.As(err)
	if !ok || actual.Field != "stabilization kind" {
		t.Fatalf("invalid stabilization kind error = %#v", err)
	}
}

func (fake *fakeGitRepository) Discover(context.Context, string) (port.RepositoryIdentity, error) {
	fake.calls = append(fake.calls, "discover")
	return testRepository(), fake.err
}

func (fake *fakeGitRepository) Version(context.Context) (string, error) {
	fake.calls = append(fake.calls, "version")
	return "git version test", fake.err
}

func (fake *fakeGitRepository) RemoteURL(context.Context, port.RepositoryIdentity) (string, error) {
	fake.calls = append(fake.calls, "remote-url")
	return "https://example.invalid/repo.git", fake.err
}

func (fake *fakeGitRepository) ActiveOperation(context.Context, port.RepositoryIdentity) (string, bool, error) {
	fake.calls = append(fake.calls, "active-operation")
	if fake.activeErr != nil {
		return "", false, fake.activeErr
	}
	return fake.activeOperation, fake.active, fake.err
}

func (fake *fakeGitRepository) HasCommits(context.Context, port.RepositoryIdentity) (bool, error) {
	fake.calls = append(fake.calls, "has-commits")
	return fake.hasCommits, fake.err
}

func (fake *fakeGitRepository) IsWorktreeClean(context.Context, port.RepositoryIdentity) (bool, error) {
	fake.calls = append(fake.calls, "worktree-clean")
	return fake.clean, fake.err
}

func (fake *fakeGitRepository) CurrentBranch(context.Context, port.RepositoryIdentity) (branch.BranchName, error) {
	fake.calls = append(fake.calls, "current-branch")
	return mustBranch("feature/ABC-123-add-export"), fake.err
}

func (fake *fakeGitRepository) ValidateBranchRef(context.Context, port.RepositoryIdentity, branch.BranchName) error {
	fake.calls = append(fake.calls, "validate-ref")
	if fake.validateRefErr != nil {
		return fake.validateRefErr
	}
	return fake.err
}

func (fake *fakeGitRepository) BranchExists(context.Context, port.RepositoryIdentity, branch.BranchName) (bool, error) {
	fake.calls = append(fake.calls, "branch-exists")
	return fake.exists, fake.err
}

func (fake *fakeGitRepository) OfficialBranchesForTicket(context.Context, port.RepositoryIdentity, ticket.ID) ([]branch.BranchName, error) {
	fake.calls = append(fake.calls, "official-branches-for-ticket")
	return nil, fake.err
}

func (fake *fakeGitRepository) Fetch(context.Context, port.RepositoryIdentity) error {
	fake.calls = append(fake.calls, "fetch")
	return fake.err
}

func (fake *fakeGitRepository) TargetBaseExists(context.Context, port.RepositoryIdentity, branch.TargetBase) (bool, error) {
	fake.calls = append(fake.calls, "target-base-exists")
	return true, fake.err
}

func (fake *fakeGitRepository) CreateBranch(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName, base branch.TargetBase, switchTo bool) error {
	fake.calls = append(fake.calls, "create-branch")
	fake.createdNames = append(fake.createdNames, name)
	fake.createdBases = append(fake.createdBases, base)
	fake.createdSwitches = append(fake.createdSwitches, switchTo)
	return fake.err
}

func (fake *fakeGitRepository) StoreWorkflowBase(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName, base branch.TargetBase) error {
	fake.calls = append(fake.calls, "store-workflow-base")
	if fake.workflowBases == nil {
		fake.workflowBases = make(map[string]branch.TargetBase)
	}
	fake.workflowBases[name.String()] = base
	return fake.err
}

func (fake *fakeGitRepository) ClearWorkflowBase(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName) error {
	fake.calls = append(fake.calls, "clear-workflow-base")
	delete(fake.workflowBases, name.String())
	return fake.err
}

func (fake *fakeGitRepository) WorkflowBase(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName) (branch.TargetBase, bool, error) {
	fake.calls = append(fake.calls, "workflow-base")
	if fake.workflowBaseErr != nil {
		return branch.TargetBase{}, false, fake.workflowBaseErr
	}
	base, found := fake.workflowBases[name.String()]
	return base, found, fake.err
}

func (fake *fakeGitRepository) SwitchBranch(context.Context, port.RepositoryIdentity, branch.BranchName) error {
	fake.calls = append(fake.calls, "switch")
	return fake.err
}

func (fake *fakeGitRepository) PublicationState(context.Context, port.RepositoryIdentity, branch.BranchName) (branch.PublicationState, error) {
	fake.calls = append(fake.calls, "publication")
	if fake.publicationErr != nil {
		return branch.PublicationUnknown, fake.publicationErr
	}
	return fake.publication, fake.err
}

func (fake *fakeGitRepository) HasMissingBaseCommits(context.Context, port.RepositoryIdentity, branch.TargetBase) (bool, error) {
	fake.calls = append(fake.calls, "missing-base")
	if fake.missingErr != nil {
		return false, fake.missingErr
	}
	return fake.missing, fake.err
}

func (fake *fakeGitRepository) CommitMessagesSince(context.Context, port.RepositoryIdentity, branch.TargetBase) ([]string, error) {
	fake.calls = append(fake.calls, "commit-messages")
	if len(fake.messageBatches) > 0 {
		messages := fake.messageBatches[0]
		fake.messageBatches = fake.messageBatches[1:]
		return append([]string(nil), messages...), fake.err
	}
	return append([]string(nil), fake.messages...), fake.err
}

func (fake *fakeGitRepository) Rebase(context.Context, port.RepositoryIdentity, branch.TargetBase) error {
	fake.calls = append(fake.calls, "rebase")
	return fake.err
}

func (fake *fakeGitRepository) ContinueRebase(context.Context, port.RepositoryIdentity) error {
	fake.calls = append(fake.calls, "continue-rebase")
	if fake.continueErr != nil {
		return fake.continueErr
	}
	fake.active = false
	fake.activeOperation = ""
	fake.missing = false
	return fake.err
}

func (fake *fakeGitRepository) Merge(context.Context, port.RepositoryIdentity, branch.TargetBase, commitmsg.Message) error {
	fake.calls = append(fake.calls, "merge")
	return fake.err
}

func (fake *fakeGitRepository) SquashMerge(context.Context, port.RepositoryIdentity, branch.BranchName) error {
	fake.calls = append(fake.calls, "squash-merge")
	return fake.err
}

func (fake *fakeGitRepository) CherryPick(_ context.Context, _ port.RepositoryIdentity, commitID string) error {
	fake.calls = append(fake.calls, "cherry-pick")
	fake.cherryPicked = append(fake.cherryPicked, commitID)
	return fake.err
}

func (fake *fakeGitRepository) ContinueCherryPick(context.Context, port.RepositoryIdentity) error {
	fake.calls = append(fake.calls, "continue-cherry-pick")
	if fake.continueErr != nil {
		return fake.continueErr
	}
	fake.active = false
	fake.activeOperation = ""
	return fake.err
}

func (fake *fakeGitRepository) DeleteLocalBranch(context.Context, port.RepositoryIdentity, branch.BranchName, bool) error {
	fake.calls = append(fake.calls, "delete-local-branch")
	return fake.err
}

func (fake *fakeGitRepository) ReleaseTagsAt(context.Context, port.RepositoryIdentity, string) ([]string, error) {
	fake.calls = append(fake.calls, "release-tags")
	return append([]string(nil), fake.releaseTags...), fake.err
}

func (fake *fakeGitRepository) HasUnmergedConflicts(context.Context, port.RepositoryIdentity) (bool, error) {
	fake.calls = append(fake.calls, "unmerged-conflicts")
	return false, fake.err
}

func (fake *fakeGitRepository) HasStagedChanges(context.Context, port.RepositoryIdentity) (bool, error) {
	fake.calls = append(fake.calls, "staged")
	return true, fake.err
}

func (fake *fakeGitRepository) Stage(context.Context, port.RepositoryIdentity, []string) error {
	fake.calls = append(fake.calls, "stage")
	return fake.err
}

func (fake *fakeGitRepository) Commit(context.Context, port.RepositoryIdentity, commitmsg.Message) error {
	fake.calls = append(fake.calls, "commit")
	return fake.err
}

func (fake *fakeGitRepository) Push(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName, _ bool) error {
	fake.calls = append(fake.calls, "push")
	fake.pushed = append(fake.pushed, name)
	if fake.pushErr != nil {
		return fake.pushErr
	}
	return fake.err
}

func (fake *fakeGitRepository) InspectPushUpdate(context.Context, port.RepositoryIdentity, branch.TargetBase, string, string) (port.PushUpdateInspection, error) {
	fake.calls = append(fake.calls, "inspect-push")
	return port.PushUpdateInspection{}, fake.err
}

type fakeKeyPolicy struct {
	err error
}

func (fake *fakeKeyPolicy) ValidateKey(context.Context, port.RepositoryIdentity, ticket.Key) error {
	return fake.err
}

type fakeQualityRunner struct {
	calls int
	err   error
}

func (fake *fakeQualityRunner) Run(context.Context, port.RepositoryIdentity, port.QualityRequest) (port.QualityResult, error) {
	fake.calls++
	return port.QualityResult{Status: port.QualityPassed}, fake.err
}

type fakePublisher struct {
	calls   int
	request port.PullRequest
	result  port.PublishedPullRequest
	err     error
}

func (fake *fakePublisher) Publish(_ context.Context, publication port.PullRequestPublication) (port.PublishedPullRequest, error) {
	fake.calls++
	fake.request = publication.PullRequest
	return fake.result, fake.err
}

func TestStartTicketCreatesOfficialAndScratchWithoutSecondFetch(t *testing.T) {
	t.Parallel()

	git := &fakeGitRepository{hasCommits: true, clean: true}
	service := newTicketService(git, nil, nil)
	result, err := service.StartTicket(context.Background(), StartTicketRequest{
		Repository:    testRepository(),
		Family:        branch.FamilyFeature,
		Ticket:        mustTicket("ABC-123"),
		Slug:          mustSlug("add-export"),
		CreateScratch: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Official.Name.String() != "feature/ABC-123-add-export" || result.Scratch == nil {
		t.Fatalf("StartTicket() = %#v", result)
	}
	if result.Scratch.Name.String() != "scratch/ABC-123-add-export-exploration" || result.Active.String() != result.Scratch.Name.String() {
		t.Fatalf("scratch result = %#v", result)
	}
	if len(git.createdBases) != 2 || git.createdBases[0].String() != "origin/develop" || git.createdBases[1].String() != "feature/ABC-123-add-export" {
		t.Fatalf("created bases = %v", git.createdBases)
	}
	if countCall(git.calls, "fetch") != 1 {
		t.Fatalf("fetch calls = %v", git.calls)
	}
}

func TestStartTicketRejectsNonRegularFamily(t *testing.T) {
	t.Parallel()

	git := &fakeGitRepository{}
	service := newTicketService(git, nil, nil)
	_, err := service.StartTicket(context.Background(), StartTicketRequest{
		Repository: testRepository(),
		Family:     branch.FamilyHotfix,
		Ticket:     mustTicket("ABC-123"),
		Slug:       mustSlug("payment-timeout"),
	})
	assertProblemCode(t, err, problem.CodeInvalidInput)
	if len(git.calls) != 0 {
		t.Fatalf("invalid workflow must not invoke Git: %v", git.calls)
	}
}

func TestPublishTicketValidatesSeriesAndBuildsPullRequest(t *testing.T) {
	t.Parallel()

	git := &fakeGitRepository{
		clean:       true,
		publication: branch.PublicationUnpublished,
		messages:    []string{"feat(ABC-123): add export"},
	}
	quality := &fakeQualityRunner{}
	publisher := &fakePublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/1"}}
	service := newTicketService(git, quality, publisher)
	name := mustBranch("feature/ABC-123-add-export")
	result, err := service.PublishTicket(context.Background(), PublishTicketRequest{
		Repository:        testRepository(),
		Branch:            name,
		Push:              true,
		CreatePullRequest: true,
		Draft:             true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Pushed || result.PullRequest.Source.String() != name.String() || result.PullRequest.Target.String() != "develop" || !result.PullRequest.Draft {
		t.Fatalf("PublishTicket() = %#v", result)
	}
	if result.PublishedURL != "https://example.invalid/pr/1" || publisher.calls != 1 || quality.calls != 2 {
		t.Fatalf("publish result = %#v, publisher=%d, quality=%d", result, publisher.calls, quality.calls)
	}
	expected := "validate-ref,fetch,commit-messages,validate-ref,worktree-clean,publication,missing-base,validate-ref,fetch,publication,missing-base,push,remote-url"
	if got := strings.Join(git.calls, ","); got != expected {
		t.Fatalf("calls = %q, want %q", got, expected)
	}
}

func TestPublishTicketStopsOnInvalidSeries(t *testing.T) {
	t.Parallel()

	t.Run("no commits", func(t *testing.T) {
		git := &fakeGitRepository{clean: true, messages: nil}
		quality := &fakeQualityRunner{}
		service := newTicketService(git, quality, nil)
		_, err := service.PublishTicket(context.Background(), PublishTicketRequest{
			Repository: testRepository(),
			Branch:     mustBranch("feature/ABC-123-add-export"),
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)
		if quality.calls != 0 {
			t.Fatal("quality must not run without a commit series")
		}
	})

	t.Run("mixed ticket", func(t *testing.T) {
		git := &fakeGitRepository{clean: true, messages: []string{"feat(ABC-124): unrelated work"}}
		service := newTicketService(git, nil, nil)
		_, err := service.PublishTicket(context.Background(), PublishTicketRequest{
			Repository: testRepository(),
			Branch:     mustBranch("feature/ABC-123-add-export"),
		})
		assertProblemCode(t, err, problem.CodeCommitTicketMismatch)
	})
}

func TestPublishTicketRevalidatesAfterRebase(t *testing.T) {
	t.Parallel()

	git := &fakeGitRepository{
		clean:       true,
		publication: branch.PublicationUnpublished,
		missing:     true,
		messageBatches: [][]string{
			{"feat(ABC-123): add export"},
			{"feat(ABC-123): add export"},
		},
	}
	quality := &fakeQualityRunner{}
	service := newTicketService(git, quality, nil)

	result, err := service.PublishTicket(context.Background(), PublishTicketRequest{
		Repository: testRepository(),
		Branch:     mustBranch("feature/ABC-123-add-export"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Sync.Mutated || quality.calls != 1 {
		t.Fatalf("PublishTicket() = %#v, quality calls=%d", result, quality.calls)
	}
	if countCall(git.calls, "commit-messages") != 2 {
		t.Fatalf("commit-series validation calls = %v", git.calls)
	}
	if countCall(git.calls, "validate-ref") != 2 {
		t.Fatalf("branch/policy validation calls = %v", git.calls)
	}
}

func TestPublishTicketBlocksPushWhenPostRebaseValidationFails(t *testing.T) {
	t.Parallel()

	git := &fakeGitRepository{
		clean:       true,
		publication: branch.PublicationUnpublished,
		missing:     true,
		messageBatches: [][]string{
			{"feat(ABC-123): add export"},
			{"feat(ABC-124): invalid after rebase"},
		},
	}
	service := newTicketService(git, &fakeQualityRunner{}, nil)
	_, err := service.PublishTicket(context.Background(), PublishTicketRequest{
		Repository: testRepository(),
		Branch:     mustBranch("feature/ABC-123-add-export"),
		Push:       true,
	})
	assertProblemCode(t, err, problem.CodeCommitTicketMismatch)
	if strings.Contains(strings.Join(git.calls, ","), "push") {
		t.Fatalf("post-rebase validation failure must stop before push: %v", git.calls)
	}
}

func TestTicketPublicationResumePushAndPullRequestBoundaries(t *testing.T) {
	t.Parallel()

	name := mustBranch("feature/ABC-123-add-export")
	base := mustBase("origin", "develop")
	git := &fakeGitRepository{
		clean:       true,
		publication: branch.PublicationUnpublished,
		messages:    []string{"feat(ABC-123): add export"},
	}
	quality := &fakeQualityRunner{}
	service := newTicketService(git, quality, nil)

	resumed, err := service.ResumeTicketPublish(context.Background(), ResumeTicketPublishRequest{
		Repository: testRepository(),
		Branch:     name,
		Base:       &base,
	})
	if err != nil || resumed.Branch != name || resumed.Sync.RecommendedAction != "rebased" ||
		resumed.PostMutationQuality == nil || quality.calls != 1 {
		t.Fatalf("ResumeTicketPublish() = (%#v, %v), quality=%d", resumed, err, quality.calls)
	}

	if err := service.PushPreparedTicket(context.Background(), testRepository(), name, &base, false); err != nil {
		t.Fatalf("PushPreparedTicket() error = %v", err)
	}
	if len(git.pushed) != 1 || git.pushed[0] != name {
		t.Fatalf("prepared push = %v", git.pushed)
	}
	if service.HasPullRequestPublisher() {
		t.Fatal("service unexpectedly reports a publisher")
	}
	_, err = service.PublishPullRequest(context.Background(), testRepository(), resumed.PullRequest)
	assertProblemCode(t, err, problem.CodeExternalCommandFailed)

	publisher := &fakePublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/2"}}
	service = newTicketService(git, nil, publisher)
	if !service.HasPullRequestPublisher() {
		t.Fatal("service did not report the configured publisher")
	}
	url, err := service.PublishPullRequest(context.Background(), testRepository(), resumed.PullRequest)
	if err != nil || url != "https://example.invalid/pr/2" || publisher.calls != 1 {
		t.Fatalf("PublishPullRequest() = (%q, %v), calls=%d", url, err, publisher.calls)
	}
}

func TestTicketPublicationResumeAndPushWhiteboxFailurePaths(t *testing.T) {
	name := mustBranch("feature/ABC-123-add-export")
	base := mustBase("origin", "develop")
	request := ResumeTicketPublishRequest{
		Repository: testRepository(),
		Branch:     name,
		Base:       &base,
	}

	t.Run("guards composed services repository and branch inputs", func(t *testing.T) {
		_, err := (&TicketService{}).ResumeTicketPublish(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInternal)

		git := &fakeGitRepository{}
		service := newTicketService(git, nil, nil)
		invalid := request
		invalid.Branch = mustBranch("scratch/ABC-123-experiment")
		_, err = service.ResumeTicketPublish(context.Background(), invalid)
		assertProblemCode(t, err, problem.CodeInvalidInput)

		invalid = request
		invalid.Repository = port.RepositoryIdentity{}
		_, err = service.ResumeTicketPublish(context.Background(), invalid)
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)
	})

	t.Run("resolves branch validation workflow base target and series failures", func(t *testing.T) {
		validationErr := errors.New("branch validation failed")
		git := &fakeGitRepository{validateRefErr: validationErr}
		_, err := newTicketService(git, nil, nil).ResumeTicketPublish(context.Background(), request)
		if !errors.Is(err, validationErr) {
			t.Fatalf("validation error = %v, want %v", err, validationErr)
		}

		hotfix := mustBranch("hotfix/ABC-123-payment-timeout")
		main := mustBase("origin", "main")
		git = &fakeGitRepository{
			publication: branch.PublicationUnpublished,
			messages:    []string{"fix(ABC-123): resolve timeout"},
			workflowBases: map[string]branch.TargetBase{
				hotfix.String(): main,
			},
		}
		result, err := newTicketService(git, nil, nil).ResumeTicketPublish(context.Background(), ResumeTicketPublishRequest{
			Repository: port.RepositoryIdentity{Root: testRepository().Root},
			Branch:     hotfix,
		})
		if err != nil || result.PullRequest.Target.String() != "main" {
			t.Fatalf("hotfix resume = (%#v, %v)", result, err)
		}

		workflowBaseErr := errors.New("workflow base unavailable")
		git = &fakeGitRepository{workflowBaseErr: workflowBaseErr}
		_, err = newTicketService(git, nil, nil).ResumeTicketPublish(context.Background(), ResumeTicketPublishRequest{
			Repository: testRepository(),
			Branch:     hotfix,
		})
		if !errors.Is(err, workflowBaseErr) {
			t.Fatalf("workflow base error = %v, want %v", err, workflowBaseErr)
		}

		git = &fakeGitRepository{}
		_, err = newTicketService(git, nil, nil).ResumeTicketPublish(context.Background(), ResumeTicketPublishRequest{
			Repository: testRepository(),
			Branch:     hotfix,
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)

		wrongTarget := mustBranch("main")
		_, err = newTicketService(&fakeGitRepository{publication: branch.PublicationUnpublished}, nil, nil).ResumeTicketPublish(context.Background(), ResumeTicketPublishRequest{
			Repository: testRepository(),
			Branch:     name,
			Base:       &base,
			Target:     &wrongTarget,
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)

		git = &fakeGitRepository{publication: branch.PublicationUnpublished}
		_, err = newTicketService(git, nil, nil).ResumeTicketPublish(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("propagates synchronization and post-sync failures", func(t *testing.T) {
		git := &fakeGitRepository{
			publication: branch.PublicationUnpublished,
			missing:     true,
		}
		_, err := newTicketService(git, nil, nil).ResumeTicketPublish(context.Background(), request)
		assertProblemCode(t, err, problem.CodeRebaseConflict)

		git = &fakeGitRepository{
			publication: branch.PublicationUnpublished,
			messages:    []string{"feat(ABC-124): wrong ticket"},
		}
		_, err = newTicketService(git, nil, nil).ResumeTicketPublish(context.Background(), request)
		assertProblemCode(t, err, problem.CodeCommitTicketMismatch)

		git = &fakeGitRepository{
			publication: branch.PublicationUnpublished,
			messages:    []string{"feat(ABC-123): add export"},
		}
		result, err := newTicketService(git, nil, nil).ResumeTicketPublish(context.Background(), request)
		if err != nil || result.PostMutationQuality == nil || result.Quality.Status != port.QualityUnconfigured {
			t.Fatalf("resume without quality runner = (%#v, %v)", result, err)
		}
	})

	t.Run("push protects all preconditions and preserves adapter failures", func(t *testing.T) {
		err := (&TicketService{}).PushPreparedTicket(context.Background(), testRepository(), name, &base, false)
		assertProblemCode(t, err, problem.CodeInternal)

		git := &fakeGitRepository{}
		service := newTicketService(git, nil, nil)
		err = service.PushPreparedTicket(context.Background(), testRepository(), mustBranch("scratch/ABC-123-experiment"), &base, false)
		assertProblemCode(t, err, problem.CodeInvalidInput)

		git = &fakeGitRepository{publication: branch.PublicationUnpublished, missing: true}
		err = newTicketService(git, nil, nil).PushPreparedTicket(context.Background(), testRepository(), name, &base, false)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)

		publicationErr := errors.New("publication failed")
		git = &fakeGitRepository{publicationErr: publicationErr}
		err = newTicketService(git, nil, nil).PushPreparedTicket(context.Background(), testRepository(), name, &base, false)
		if !errors.Is(err, publicationErr) {
			t.Fatalf("publication error = %v, want %v", err, publicationErr)
		}

		git = &fakeGitRepository{publication: branch.PublicationUnknown}
		err = newTicketService(git, nil, nil).PushPreparedTicket(context.Background(), testRepository(), name, &base, false)
		assertProblemCode(t, err, problem.CodeBranchPublicationUnknown)

		pushErr := errors.New("push failed")
		git = &fakeGitRepository{publication: branch.PublicationUnpublished, pushErr: pushErr}
		err = newTicketService(git, nil, nil).PushPreparedTicket(context.Background(), testRepository(), name, &base, false)
		if !errors.Is(err, pushErr) {
			t.Fatalf("push error = %v, want %v", err, pushErr)
		}
	})

	t.Run("pull request publisher errors are preserved", func(t *testing.T) {
		publishErr := errors.New("publisher failed")
		service := newTicketService(&fakeGitRepository{}, nil, &fakePublisher{err: publishErr})
		_, err := service.PublishPullRequest(context.Background(), testRepository(), port.PullRequest{})
		if !errors.Is(err, publishErr) {
			t.Fatalf("publisher error = %v, want %v", err, publishErr)
		}
	})
}

func TestPublishHotfixTargetsAffectedLine(t *testing.T) {
	t.Parallel()

	git := &fakeGitRepository{
		clean:       true,
		publication: branch.PublicationUnpublished,
		messages:    []string{"fix(ABC-999): resolve payment timeout"},
	}
	service := newTicketService(git, nil, nil)
	name := mustBranch("hotfix/ABC-999-payment-timeout")
	affected := mustBase("origin", "main")

	result, err := service.PublishTicket(context.Background(), PublishTicketRequest{
		Repository: testRepository(),
		Branch:     name,
		Base:       &affected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.PullRequest.Target.String() != "main" {
		t.Fatalf("hotfix target = %q, want main", result.PullRequest.Target.String())
	}
}

func TestPublishHotfixUsesStoredWorkflowBase(t *testing.T) {
	t.Parallel()

	name := mustBranch("hotfix/ABC-999-payment-timeout")
	base := mustBase("origin", "main")
	git := &fakeGitRepository{
		clean:       true,
		publication: branch.PublicationUnpublished,
		messages:    []string{"fix(ABC-999): resolve payment timeout"},
		workflowBases: map[string]branch.TargetBase{
			name.String(): base,
		},
	}
	service := newTicketService(git, nil, nil)
	result, err := service.PublishTicket(context.Background(), PublishTicketRequest{
		Repository: testRepository(),
		Branch:     name,
	})
	if err != nil || result.PullRequest.Target.String() != "main" {
		t.Fatalf("PublishTicket() = (%#v, %v)", result, err)
	}
}

func TestPublishHotfixRejectsIncorrectAffectedLine(t *testing.T) {
	t.Parallel()

	git := &fakeGitRepository{clean: true, messages: []string{"fix(ABC-999): resolve payment timeout"}}
	service := newTicketService(git, nil, nil)
	invalidBase := mustBase("origin", "develop")
	_, err := service.PublishTicket(context.Background(), PublishTicketRequest{
		Repository: testRepository(),
		Branch:     mustBranch("hotfix/ABC-999-payment-timeout"),
		Base:       &invalidBase,
	})
	assertProblemCode(t, err, problem.CodeInvalidInput)
}

func TestTicketWorkflowHelperContracts(t *testing.T) {
	t.Parallel()

	feature := mustBranch("feature/ABC-123-add-export")
	develop := mustBase("origin", "develop")
	main := mustBase("origin", "main")
	release := mustBase("origin", "release/2.8.0")

	t.Run("ticket bases distinguish regular, hotfix, and managed stabilization work", func(t *testing.T) {
		resolved, err := resolveTicketBase(feature, testRepository(), nil, false)
		if err != nil || resolved.String() != develop.String() {
			t.Fatalf("regular base = (%q, %v)", resolved, err)
		}
		if _, err := resolveTicketBase(feature, testRepository(), &main, false); err == nil {
			t.Fatal("regular ticket accepted main base")
		}

		hotfix := mustBranch("hotfix/ABC-123-payment-timeout")
		resolved, err = resolveTicketBase(hotfix, testRepository(), &main, false)
		if err != nil || resolved.String() != main.String() {
			t.Fatalf("hotfix base = (%q, %v)", resolved, err)
		}
		if _, err := resolveTicketBase(hotfix, testRepository(), &develop, false); err == nil {
			t.Fatal("hotfix accepted develop base")
		}
		if _, err := resolveTicketBase(hotfix, testRepository(), nil, false); err == nil {
			t.Fatal("hotfix accepted missing base")
		}

		resolved, err = resolveTicketBase(mustBranch("docs/ABC-123-release-docs"), testRepository(), &release, true)
		if err != nil || resolved.String() != release.String() {
			t.Fatalf("managed docs base = (%q, %v)", resolved, err)
		}
	})

	t.Run("pull request targets cannot diverge from workflow targets", func(t *testing.T) {
		target, err := resolvePullRequestTarget(feature, develop, nil, false)
		if err != nil || target.String() != "develop" {
			t.Fatalf("regular target = (%q, %v)", target, err)
		}
		target, err = resolvePullRequestTarget(mustBranch("hotfix/ABC-123-payment-timeout"), main, nil, false)
		if err != nil || target.String() != "main" {
			t.Fatalf("hotfix target = (%q, %v)", target, err)
		}
		target, err = resolvePullRequestTarget(mustBranch("fix/ABC-123-release-blocker"), release, nil, true)
		if err != nil || target.String() != "release/2.8.0" {
			t.Fatalf("managed fix target = (%q, %v)", target, err)
		}
		for _, name := range []branch.BranchName{
			mustBranch("docs/ABC-123-release-docs"),
			mustBranch("chore/ABC-123-release-prep"),
		} {
			target, err = resolvePullRequestTarget(name, release, nil, true)
			if err != nil || target.String() != "release/2.8.0" {
				t.Fatalf("managed %s target = (%q, %v)", name.Family(), target, err)
			}
		}
		wrong := mustBranch("main")
		if _, err := resolvePullRequestTarget(feature, develop, &wrong, false); err == nil {
			t.Fatal("regular ticket accepted a mismatched PR target")
		}
	})

	t.Run("family and problem helpers cover both outcomes", func(t *testing.T) {
		for _, base := range []branch.TargetBase{main, develop, release, mustBase("origin", "support/2.7")} {
			if !isSharedLineBase(base) {
				t.Fatalf("shared base %q was not recognized", base)
			}
		}
		if isSharedLineBase(mustBase("origin", "feature/ABC-123-add-export")) {
			t.Fatal("ticket branch was treated as shared")
		}
		if !isWorkflowManagedTicketBase(branch.FamilyFix, main) ||
			!isWorkflowManagedTicketBase(branch.FamilyDocs, release) ||
			!isWorkflowManagedTicketBase(branch.FamilyChore, release) ||
			isWorkflowManagedTicketBase(branch.FamilyFeature, release) {
			t.Fatal("workflow-managed family classification is incorrect")
		}
		if _, ok := problem.As(repositoryRequired()); !ok {
			t.Fatal("repositoryRequired is not a problem")
		}
		if _, ok := problem.As(internalDependencyError("workflow")); !ok {
			t.Fatal("internalDependencyError is not a problem")
		}
		if mustDevelop().String() != "develop" {
			t.Fatal("mustDevelop did not return develop")
		}
	})
}

func TestReleaseWorkflows(t *testing.T) {
	t.Parallel()

	t.Run("hotfix starts from main", func(t *testing.T) {
		git := &fakeGitRepository{hasCommits: true, clean: true}
		service := newReleaseService(git, nil)
		result, err := service.StartHotfix(context.Background(), StartHotfixRequest{
			Repository:   testRepository(),
			Ticket:       mustTicket("ABC-999"),
			Slug:         mustSlug("payment-timeout"),
			AffectedLine: mustBranch("main"),
		})
		if err != nil || result.Name.String() != "hotfix/ABC-999-payment-timeout" || result.Base.String() != "origin/main" {
			t.Fatalf("StartHotfix() = (%#v, %v)", result, err)
		}
		if stored, found := git.workflowBases[result.Name.String()]; !found || stored.String() != "origin/main" {
			t.Fatalf("hotfix workflow base metadata = %#v", git.workflowBases)
		}
	})

	t.Run("hotfix rejects develop", func(t *testing.T) {
		service := newReleaseService(&fakeGitRepository{}, nil)
		_, err := service.StartHotfix(context.Background(), StartHotfixRequest{
			Repository:   testRepository(),
			Ticket:       mustTicket("ABC-999"),
			Slug:         mustSlug("payment-timeout"),
			AffectedLine: mustBranch("develop"),
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("release cut starts from develop", func(t *testing.T) {
		git := &fakeGitRepository{hasCommits: true, clean: true}
		service := newReleaseService(git, nil)
		version := mustReleaseVersion(t, "2.8.0")
		result, err := service.CutRelease(context.Background(), CutReleaseRequest{
			Repository: testRepository(),
			Version:    version,
		})
		if err != nil || result.Intent.Branch.String() != "release/2.8.0" || result.Intent.Source.String() != "origin/develop" {
			t.Fatalf("CutRelease() = (%#v, %v)", result, err)
		}
		if result.Intent.Workflow != "execute-protected-line-request.yml" || result.Intent.Kind != "release" {
			t.Fatalf("release intent = %#v", result.Intent)
		}
		if strings.Contains(strings.Join(git.calls, ","), "create-branch") {
			t.Fatalf("release intent must not create a local shared line: %v", git.calls)
		}
	})

	t.Run("support starts from main", func(t *testing.T) {
		git := &fakeGitRepository{hasCommits: true, clean: true, releaseTags: []string{"v2.8.0"}}
		service := newReleaseService(git, nil)
		version := mustSupportVersion(t, "2.8")
		result, err := service.PrepareSupport(context.Background(), PrepareSupportRequest{
			Repository: testRepository(),
			Version:    version,
		})
		if err != nil || result.Intent.Branch.String() != "support/2.8" || result.Intent.Source.String() != "origin/main" {
			t.Fatalf("PrepareSupport() = (%#v, %v)", result, err)
		}
		if result.Intent.Workflow != "execute-protected-line-request.yml" || result.Intent.Kind != "support" {
			t.Fatalf("support intent = %#v", result.Intent)
		}
	})
}

func TestPrepareReleaseBackmerge(t *testing.T) {
	t.Parallel()

	publisher := &fakePublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/backmerge"}}
	service := newReleaseService(&fakeGitRepository{}, publisher)
	result, err := service.PrepareReleaseBackmerge(context.Background(), PrepareReleaseBackmergeRequest{
		Repository:        testRepository(),
		Release:           mustBranch("release/2.8.0"),
		CreatePullRequest: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.PullRequest.Source.String() != "release/2.8.0" || result.PullRequest.Target.String() != "develop" || result.PublishedURL == "" {
		t.Fatalf("PrepareReleaseBackmerge() = %#v", result)
	}
	if publisher.calls != 1 {
		t.Fatal("publisher was not invoked")
	}
}

func TestReleaseStabilizationConstrainsFamiliesAndBases(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		kind   ReleaseStabilizationKind
		family branch.Family
	}{
		{ReleaseStabilizationBlocker, branch.FamilyFix},
		{ReleaseStabilizationDocs, branch.FamilyDocs},
		{ReleaseStabilizationPrep, branch.FamilyChore},
	} {
		testCase := testCase
		t.Run(string(testCase.kind), func(t *testing.T) {
			t.Parallel()
			git := &fakeGitRepository{hasCommits: true, clean: true}
			service := newReleaseService(git, nil)
			result, err := service.CreateReleaseStabilization(context.Background(), CreateReleaseStabilizationRequest{
				Repository: testRepository(),
				Release:    mustBranch("release/2.8.0"),
				Ticket:     mustTicket("ABC-999"),
				Slug:       mustSlug("release-blocker-timeout"),
				Kind:       testCase.kind,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Name.Family() != testCase.family || result.Base.String() != "origin/release/2.8.0" {
				t.Fatalf("CreateReleaseStabilization() = %#v", result)
			}
		})
	}

	service := newReleaseService(&fakeGitRepository{}, nil)
	_, err := service.CreateReleaseStabilization(context.Background(), CreateReleaseStabilizationRequest{
		Repository: testRepository(),
		Release:    mustBranch("release/2.8.0"),
		Ticket:     mustTicket("ABC-999"),
		Slug:       mustSlug("new-feature"),
		Kind:       "feature",
	})
	assertProblemCode(t, err, problem.CodeInvalidInput)
}

func TestReleasePromotionAndSupportProvenance(t *testing.T) {
	t.Parallel()

	t.Run("release promotion targets main", func(t *testing.T) {
		publisher := &fakePublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/release"}}
		service := newReleaseService(&fakeGitRepository{}, publisher)
		result, err := service.PrepareReleasePromotion(context.Background(), PrepareReleasePromotionRequest{
			Repository:        testRepository(),
			Release:           mustBranch("release/2.8.0"),
			CreatePullRequest: true,
		})
		if err != nil || result.PullRequest.Target.String() != "main" || result.PublishedURL == "" {
			t.Fatalf("PrepareReleasePromotion() = (%#v, %v)", result, err)
		}
	})

	t.Run("support requires matching release tag", func(t *testing.T) {
		git := &fakeGitRepository{hasCommits: true, clean: true, releaseTags: []string{"v2.9.0"}}
		service := newReleaseService(git, nil)
		_, err := service.PrepareSupport(context.Background(), PrepareSupportRequest{
			Repository: testRepository(),
			Version:    mustSupportVersion(t, "2.8"),
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})
}

func TestCleanupBranchLimitsDeletionToPermittedLocalBranches(t *testing.T) {
	t.Parallel()

	t.Run("official branches are never local cleanup targets", func(t *testing.T) {
		git := &fakeGitRepository{}
		service := newReleaseService(git, nil)
		_, err := service.CleanupBranch(context.Background(), CleanupBranchRequest{
			Repository: testRepository(),
			Branch:     mustBranch("hotfix/ABC-999-payment-timeout"),
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)
		if len(git.calls) != 0 {
			t.Fatalf("official cleanup must not call Git: %v", git.calls)
		}
	})

	t.Run("scratch cleanup is private and direct", func(t *testing.T) {
		git := &fakeGitRepository{}
		service := newReleaseService(git, nil)
		result, err := service.CleanupBranch(context.Background(), CleanupBranchRequest{
			Repository: testRepository(),
			Branch:     mustBranch("scratch/ABC-123-experiment"),
		})
		if err != nil || !result.DeletedLocal || !result.MetadataCleared {
			t.Fatalf("CleanupBranch() = (%#v, %v)", result, err)
		}
	})

	t.Run("shared and release lines are protected", func(t *testing.T) {
		service := newReleaseService(&fakeGitRepository{}, nil)
		for _, name := range []string{"develop", "release/2.8.0", "support/2.8"} {
			_, err := service.CleanupBranch(context.Background(), CleanupBranchRequest{
				Repository: testRepository(),
				Branch:     mustBranch(name),
			})
			assertProblemCode(t, err, problem.CodeInvalidInput)
		}
	})
}

func TestPropagateHotfixUsesCherryPickProvenanceAndTargetLine(t *testing.T) {
	t.Parallel()

	git := &fakeGitRepository{
		hasCommits:  true,
		clean:       true,
		publication: branch.PublicationUnpublished,
		messages:    []string{"fix(ABC-999): resolve payment timeout"},
	}
	service := newReleaseService(git, nil)
	commitID := strings.Repeat("a", 40)

	result, err := service.PropagateHotfix(context.Background(), PropagateHotfixRequest{
		Repository: testRepository(),
		Source:     mustBranch("hotfix/ABC-999-payment-timeout"),
		TargetLine: mustBranch("support/2.7"),
		CommitID:   commitID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.CherryPicked || len(git.cherryPicked) != 1 || git.cherryPicked[0] != commitID {
		t.Fatalf("cherry-pick result = %#v, commits=%v", result, git.cherryPicked)
	}
	if result.Branch.Name.String() != "fix/ABC-999-forward-port-payment-timeout" {
		t.Fatalf("propagation branch = %q", result.Branch.Name.String())
	}
	if result.Publication.PullRequest.Target.String() != "support/2.7" {
		t.Fatalf("propagation target = %q", result.Publication.PullRequest.Target.String())
	}
}

func TestPropagateHotfixRejectsInvalidRequestsBeforeMutation(t *testing.T) {
	t.Parallel()

	git := &fakeGitRepository{}
	service := newReleaseService(git, nil)
	_, err := service.PropagateHotfix(context.Background(), PropagateHotfixRequest{
		Repository: testRepository(),
		Source:     mustBranch("feature/ABC-999-payment-timeout"),
		TargetLine: mustBranch("develop"),
		CommitID:   strings.Repeat("a", 40),
	})
	assertProblemCode(t, err, problem.CodeInvalidInput)
	if len(git.calls) != 0 {
		t.Fatalf("invalid propagation must not call Git: %v", git.calls)
	}
}

func newTicketService(git *fakeGitRepository, quality port.QualityRunner, publisher port.PullRequestPublisher) *TicketService {
	return newTicketServiceWithGit(git, quality, publisher)
}

func newTicketServiceWithGit(git port.GitRepository, quality port.QualityRunner, publisher port.PullRequestPublisher) *TicketService {
	keys := &fakeKeyPolicy{}
	branches := branchapp.NewService(git, keys)
	sync := branchapp.NewSynchronizer(git, branches, quality)
	return NewTicketService(branches, sync, git, quality, publisher)
}

func newReleaseService(git *fakeGitRepository, publisher port.PullRequestPublisher) *ReleaseService {
	branches := branchapp.NewService(git, &fakeKeyPolicy{})
	sync := branchapp.NewSynchronizer(git, branches, nil)
	tickets := NewTicketService(branches, sync, git, nil, publisher)
	return NewReleaseService(branches, git, publisher).WithTicketService(tickets)
}

func testRepository() port.RepositoryIdentity {
	return port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"}
}

func mustBranch(raw string) branch.BranchName {
	value, err := branch.ParseName(raw)
	if err != nil {
		panic(err)
	}
	return value
}

func mustBase(remote, raw string) branch.TargetBase {
	base, err := branch.NewTargetBase(remote, mustBranch(raw))
	if err != nil {
		panic(err)
	}
	return base
}

func mustTicket(raw string) ticket.ID {
	value, err := ticket.ParseID(raw)
	if err != nil {
		panic(err)
	}
	return value
}

func mustSlug(raw string) branch.Slug {
	value, err := branch.ParseSlug(raw)
	if err != nil {
		panic(err)
	}
	return value
}

func mustReleaseVersion(t *testing.T, raw string) branch.SemanticVersion {
	t.Helper()
	value, err := branch.ParseSemanticVersion(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func mustSupportVersion(t *testing.T, raw string) branch.SupportVersion {
	t.Helper()
	value, err := branch.ParseSupportVersion(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func countCall(calls []string, expected string) int {
	count := 0
	for _, actual := range calls {
		if actual == expected {
			count++
		}
	}
	return count
}

func assertProblemCode(t *testing.T, err error, expected problem.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected problem code %q, got nil", expected)
	}
	actual, ok := problem.As(err)
	if !ok {
		t.Fatalf("error %T does not carry a problem: %v", err, err)
	}
	if actual.Code != expected {
		t.Fatalf("problem code = %q, want %q", actual.Code, expected)
	}
}

var _ port.GitRepository = (*fakeGitRepository)(nil)
var _ port.KeyPolicy = (*fakeKeyPolicy)(nil)
var _ port.QualityRunner = (*fakeQualityRunner)(nil)
var _ port.PullRequestPublisher = (*fakePublisher)(nil)
