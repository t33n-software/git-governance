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

type ticketCoverageGit struct {
	*fakeGitRepository

	validateRefErr     error
	fetchErr           error
	workflowBaseErr    error
	commitMessagesErr  error
	pushErr            error
	branchExists       []bool
	branchExistsCursor int
	createErrors       []error
	createCursor       int
}

type scratchTicketWorkflowGit struct {
	*fakeGitRepository

	localBranches map[string]bool
	official      []branch.BranchName
	squashErr     error

	switched  []branch.BranchName
	squashed  []branch.BranchName
	committed []string
}

type ticketPreflightPublisher struct {
	publishErr   error
	validateErr  error
	publications []port.PullRequestPublication
}

func (publisher *ticketPreflightPublisher) Publish(
	_ context.Context,
	publication port.PullRequestPublication,
) (port.PublishedPullRequest, error) {
	publisher.publications = append(publisher.publications, publication)
	return port.PublishedPullRequest{URL: "https://example.invalid/pr/preflight"}, publisher.publishErr
}

func (publisher *ticketPreflightPublisher) Validate(
	_ context.Context,
	publication port.PullRequestPublication,
) error {
	publisher.publications = append(publisher.publications, publication)
	return publisher.validateErr
}

func newScratchTicketWorkflowGit(source, target branch.BranchName) *scratchTicketWorkflowGit {
	return &scratchTicketWorkflowGit{
		fakeGitRepository: &fakeGitRepository{
			clean:       true,
			publication: branch.PublicationUnpublished,
			messages:    []string{"feat(ABC-123): add export"},
		},
		localBranches: map[string]bool{
			source.String(): true,
			target.String(): true,
		},
		official: []branch.BranchName{target},
	}
}

func (git *scratchTicketWorkflowGit) BranchExists(
	_ context.Context,
	_ port.RepositoryIdentity,
	branchName branch.BranchName,
) (bool, error) {
	git.calls = append(git.calls, "branch-exists")
	return git.localBranches[branchName.String()], nil
}

func (git *scratchTicketWorkflowGit) OfficialBranchesForTicket(
	_ context.Context,
	_ port.RepositoryIdentity,
	_ ticket.ID,
) ([]branch.BranchName, error) {
	git.calls = append(git.calls, "official-branches-for-ticket")
	return append([]branch.BranchName(nil), git.official...), nil
}

func (git *scratchTicketWorkflowGit) SwitchBranch(
	_ context.Context,
	_ port.RepositoryIdentity,
	name branch.BranchName,
) error {
	git.calls = append(git.calls, "switch")
	git.switched = append(git.switched, name)
	return nil
}

func (git *scratchTicketWorkflowGit) SquashMerge(
	_ context.Context,
	_ port.RepositoryIdentity,
	source branch.BranchName,
) error {
	git.calls = append(git.calls, "squash-merge")
	if git.squashErr != nil {
		return git.squashErr
	}
	git.squashed = append(git.squashed, source)
	return nil
}

func (git *scratchTicketWorkflowGit) Commit(
	_ context.Context,
	_ port.RepositoryIdentity,
	message commitmsg.Message,
) error {
	git.calls = append(git.calls, "commit")
	git.committed = append(git.committed, message.String())
	return nil
}

func (git *ticketCoverageGit) ValidateBranchRef(context.Context, port.RepositoryIdentity, branch.BranchName) error {
	git.calls = append(git.calls, "validate-ref")
	return git.validateRefErr
}

func (git *ticketCoverageGit) Fetch(context.Context, port.RepositoryIdentity) error {
	git.calls = append(git.calls, "fetch")
	return git.fetchErr
}

func (git *ticketCoverageGit) WorkflowBase(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName) (branch.TargetBase, bool, error) {
	git.calls = append(git.calls, "workflow-base")
	if git.workflowBaseErr != nil {
		return branch.TargetBase{}, false, git.workflowBaseErr
	}
	base, found := git.workflowBases[name.String()]
	return base, found, nil
}

func (git *ticketCoverageGit) CommitMessagesSince(context.Context, port.RepositoryIdentity, branch.TargetBase) ([]string, error) {
	git.calls = append(git.calls, "commit-messages")
	if git.commitMessagesErr != nil {
		return nil, git.commitMessagesErr
	}
	return git.fakeGitRepository.CommitMessagesSince(context.Background(), port.RepositoryIdentity{}, branch.TargetBase{})
}

func (git *ticketCoverageGit) Push(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName, _ bool) error {
	git.calls = append(git.calls, "push")
	if git.pushErr != nil {
		return git.pushErr
	}
	git.pushed = append(git.pushed, name)
	return nil
}

func (git *ticketCoverageGit) BranchExists(context.Context, port.RepositoryIdentity, branch.BranchName) (bool, error) {
	git.calls = append(git.calls, "branch-exists")
	if git.branchExistsCursor < len(git.branchExists) {
		value := git.branchExists[git.branchExistsCursor]
		git.branchExistsCursor++
		return value, nil
	}
	return false, nil
}

func (git *ticketCoverageGit) CreateBranch(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName, base branch.TargetBase, switchTo bool) error {
	git.calls = append(git.calls, "create-branch")
	git.createdNames = append(git.createdNames, name)
	git.createdBases = append(git.createdBases, base)
	git.createdSwitches = append(git.createdSwitches, switchTo)
	if git.createCursor < len(git.createErrors) {
		err := git.createErrors[git.createCursor]
		git.createCursor++
		return err
	}
	return nil
}

func newTicketCoverageGit() *ticketCoverageGit {
	return &ticketCoverageGit{
		fakeGitRepository: &fakeGitRepository{
			hasCommits:  true,
			clean:       true,
			publication: branch.PublicationUnpublished,
			messages:    []string{"feat(ABC-123): add export"},
		},
	}
}

func TestTicketServiceCoverageStartFailuresAndBranches(t *testing.T) {
	request := StartTicketRequest{
		Repository: testRepository(),
		Family:     branch.FamilyFeature,
		Ticket:     mustTicket("ABC-123"),
		Slug:       mustSlug("add-export"),
	}

	t.Run("requires a branch service", func(t *testing.T) {
		_, err := (&TicketService{}).StartTicket(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("returns successfully without scratch", func(t *testing.T) {
		git := newTicketCoverageGit()
		result, err := newTicketServiceWithGit(git, nil, nil).StartTicket(context.Background(), request)
		if err != nil || result.Scratch != nil || result.Active.String() != result.Official.Name.String() {
			t.Fatalf("StartTicket() = (%#v, %v)", result, err)
		}
	})

	t.Run("propagates official branch creation failure", func(t *testing.T) {
		createErr := errors.New("official branch creation failed")
		git := newTicketCoverageGit()
		git.createErrors = []error{createErr}
		_, err := newTicketServiceWithGit(git, nil, nil).StartTicket(context.Background(), request)
		if !errors.Is(err, createErr) {
			t.Fatalf("StartTicket() error = %v, want %v", err, createErr)
		}
	})

	t.Run("rejects a generated scratch slug that exceeds the domain limit", func(t *testing.T) {
		git := newTicketCoverageGit()
		tooLong, err := branch.ParseSlug(strings.Repeat("a", 100))
		if err != nil {
			t.Fatal(err)
		}
		_, err = newTicketServiceWithGit(git, nil, nil).StartTicket(context.Background(), StartTicketRequest{
			Repository:    testRepository(),
			Family:        branch.FamilyFeature,
			Ticket:        mustTicket("ABC-123"),
			Slug:          tooLong,
			CreateScratch: true,
		})
		assertProblemCode(t, err, problem.CodeBranchSlugInvalid)
	})

	t.Run("propagates scratch branch creation failure", func(t *testing.T) {
		createErr := errors.New("scratch branch creation failed")
		git := newTicketCoverageGit()
		git.createErrors = []error{nil, createErr}
		request.CreateScratch = true
		_, err := newTicketServiceWithGit(git, nil, nil).StartTicket(context.Background(), request)
		if !errors.Is(err, createErr) {
			t.Fatalf("StartTicket() error = %v, want %v", err, createErr)
		}
	})
}

func TestTicketServiceCoveragePublishFailuresAndBranches(t *testing.T) {
	feature := mustBranch("feature/ABC-123-add-export")
	validRequest := func() PublishTicketRequest {
		return PublishTicketRequest{Repository: testRepository(), Branch: feature}
	}

	t.Run("requires composed services", func(t *testing.T) {
		_, err := (&TicketService{}).PublishTicket(context.Background(), validRequest())
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("requires a canonical branch", func(t *testing.T) {
		request := validRequest()
		request.Branch = branch.BranchName{}
		_, err := newTicketServiceWithGit(newTicketCoverageGit(), nil, nil).PublishTicket(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("rejects non-official branches", func(t *testing.T) {
		request := validRequest()
		request.Branch = mustBranch("develop")
		_, err := newTicketServiceWithGit(newTicketCoverageGit(), nil, nil).PublishTicket(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("propagates branch validation failure", func(t *testing.T) {
		validateErr := errors.New("branch ref validation failed")
		git := newTicketCoverageGit()
		git.validateRefErr = validateErr
		_, err := newTicketServiceWithGit(git, nil, nil).PublishTicket(context.Background(), validRequest())
		if !errors.Is(err, validateErr) {
			t.Fatalf("PublishTicket() error = %v, want %v", err, validateErr)
		}
	})

	t.Run("defaults an omitted remote", func(t *testing.T) {
		request := validRequest()
		request.Repository.Remote = ""
		request.DryRun = true
		result, err := newTicketServiceWithGit(newTicketCoverageGit(), nil, nil).PublishTicket(context.Background(), request)
		if err != nil || result.PullRequest.Target.String() != "develop" {
			t.Fatalf("PublishTicket() = (%#v, %v)", result, err)
		}
	})

	t.Run("requires a repository root", func(t *testing.T) {
		request := validRequest()
		request.Repository.Root = ""
		_, err := newTicketServiceWithGit(newTicketCoverageGit(), nil, nil).PublishTicket(context.Background(), request)
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)
	})

	t.Run("propagates stored workflow base lookup failure", func(t *testing.T) {
		baseErr := errors.New("workflow base unavailable")
		git := newTicketCoverageGit()
		git.workflowBaseErr = baseErr
		request := validRequest()
		request.Branch = mustBranch("hotfix/ABC-123-payment-timeout")
		_, err := newTicketServiceWithGit(git, nil, nil).PublishTicket(context.Background(), request)
		if !errors.Is(err, baseErr) {
			t.Fatalf("PublishTicket() error = %v, want %v", err, baseErr)
		}
	})

	t.Run("rejects an explicit mismatched pull request target", func(t *testing.T) {
		target := mustBranch("main")
		request := validRequest()
		request.Target = &target
		_, err := newTicketServiceWithGit(newTicketCoverageGit(), nil, nil).PublishTicket(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("propagates fetch failure", func(t *testing.T) {
		fetchErr := errors.New("fetch failed")
		git := newTicketCoverageGit()
		git.fetchErr = fetchErr
		_, err := newTicketServiceWithGit(git, nil, nil).PublishTicket(context.Background(), validRequest())
		if !errors.Is(err, fetchErr) {
			t.Fatalf("PublishTicket() error = %v, want %v", err, fetchErr)
		}
	})

	t.Run("propagates quality failure", func(t *testing.T) {
		qualityErr := errors.New("quality failed")
		_, err := newTicketServiceWithGit(newTicketCoverageGit(), &fakeQualityRunner{err: qualityErr}, nil).PublishTicket(context.Background(), validRequest())
		if !errors.Is(err, qualityErr) {
			t.Fatalf("PublishTicket() error = %v, want %v", err, qualityErr)
		}
	})

	t.Run("propagates synchronizer failure", func(t *testing.T) {
		git := newTicketCoverageGit()
		git.clean = false
		_, err := newTicketServiceWithGit(git, nil, nil).PublishTicket(context.Background(), validRequest())
		assertProblemCode(t, err, problem.CodeWorktreeNotClean)
	})

	t.Run("returns a dry-run intent before publication", func(t *testing.T) {
		request := validRequest()
		request.DryRun = true
		result, err := newTicketServiceWithGit(newTicketCoverageGit(), nil, nil).PublishTicket(context.Background(), request)
		if err != nil || !result.DryRun || result.Pushed {
			t.Fatalf("PublishTicket() = (%#v, %v)", result, err)
		}
	})

	t.Run("propagates push failure", func(t *testing.T) {
		pushErr := errors.New("push failed")
		git := newTicketCoverageGit()
		git.pushErr = pushErr
		request := validRequest()
		request.Push = true
		_, err := newTicketServiceWithGit(git, nil, nil).PublishTicket(context.Background(), request)
		if !errors.Is(err, pushErr) {
			t.Fatalf("PublishTicket() error = %v, want %v", err, pushErr)
		}
	})

	t.Run("propagates publisher failure", func(t *testing.T) {
		publishErr := errors.New("publisher failed")
		request := validRequest()
		request.Push = true
		request.CreatePullRequest = true
		request.Body = "Summary: Add the export button.\n\nScope and Non-Goals: The export button only.\n\nCommit Series:\n- feat(ABC-123): add export button\n\nRisk and Rollback: Low; revert the commit.\n\nVerification and Review Focus: Unit tests; review the button wiring."
		_, err := newTicketServiceWithGit(
			newTicketCoverageGit(),
			nil,
			&fakePublisher{err: publishErr},
		).PublishTicket(context.Background(), request)
		if !errors.Is(err, publishErr) {
			t.Fatalf("PublishTicket() error = %v, want %v", err, publishErr)
		}
	})

	t.Run("rejects an invalid configured remote before Git mutation", func(t *testing.T) {
		request := validRequest()
		request.Repository.Remote = "invalid remote"
		_, err := newTicketServiceWithGit(newTicketCoverageGit(), nil, nil).PublishTicket(context.Background(), request)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})
}

func TestPublishTicketTransfersScratchThroughSharedMerger(t *testing.T) {
	source := mustBranch("scratch/ABC-123-export-exploration")
	target := mustBranch("feature/ABC-123-add-export")
	message := mustScratchCommitMessage(t, "feat(ABC-123): add export")
	git := newScratchTicketWorkflowGit(source, target)
	branches := branchapp.NewService(git, &fakeKeyPolicy{})
	sync := branchapp.NewSynchronizer(git, branches, nil)
	service := NewTicketService(branches, sync, git, nil, nil).
		WithScratchMerger(branchapp.NewScratchMerger(git, branches))

	result, err := service.PublishTicket(context.Background(), PublishTicketRequest{
		Repository:     testRepository(),
		Branch:         source,
		ScratchMessage: &message,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Branch != target || result.ScratchMerge == nil || !result.ScratchMerge.Committed {
		t.Fatalf("PublishTicket() = %#v", result)
	}
	if result.PullRequest.Source != target || result.PullRequest.Target.String() != "develop" {
		t.Fatalf("pull request = %#v", result.PullRequest)
	}
	if len(git.switched) != 1 || git.switched[0] != target ||
		len(git.squashed) != 1 || git.squashed[0] != source ||
		len(git.committed) != 1 || git.committed[0] != message.String() {
		t.Fatalf(
			"scratch transfer calls = switched:%#v squashed:%#v committed:%#v",
			git.switched,
			git.squashed,
			git.committed,
		)
	}
}

func TestPublishTicketPlansScratchTransferDuringDryRun(t *testing.T) {
	source := mustBranch("scratch/ABC-123-export-exploration")
	target := mustBranch("feature/ABC-123-add-export")
	message := mustScratchCommitMessage(t, "feat(ABC-123): add export")
	git := newScratchTicketWorkflowGit(source, target)
	branches := branchapp.NewService(git, &fakeKeyPolicy{})
	sync := branchapp.NewSynchronizer(git, branches, nil)
	service := NewTicketService(branches, sync, git, nil, nil).
		WithScratchMerger(branchapp.NewScratchMerger(git, branches))

	result, err := service.PublishTicket(context.Background(), PublishTicketRequest{
		Repository:     testRepository(),
		Branch:         source,
		ScratchMessage: &message,
		DryRun:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun || result.Branch != target || result.ScratchMerge == nil ||
		!result.ScratchMerge.DryRun || result.ScratchMerge.Committed ||
		result.Sync.RecommendedAction != "planned" ||
		result.Quality.Status != port.QualitySkipped {
		t.Fatalf("dry PublishTicket() = %#v", result)
	}
	for _, prohibited := range []string{"switch", "squash-merge", "commit", "fetch"} {
		if strings.Contains(strings.Join(git.calls, ","), prohibited) {
			t.Fatalf("dry scratch publish called %q: %v", prohibited, git.calls)
		}
	}
}

func TestPublishTicketScratchTransferFailurePaths(t *testing.T) {
	source := mustBranch("scratch/ABC-123-export-exploration")
	target := mustBranch("feature/ABC-123-add-export")
	message := mustScratchCommitMessage(t, "feat(ABC-123): add export")
	request := func() PublishTicketRequest {
		return PublishTicketRequest{
			Repository:     testRepository(),
			Branch:         source,
			ScratchMessage: &message,
		}
	}

	t.Run("requires the composed scratch merger", func(t *testing.T) {
		git := newScratchTicketWorkflowGit(source, target)
		branches := branchapp.NewService(git, &fakeKeyPolicy{})
		service := NewTicketService(branches, branchapp.NewSynchronizer(git, branches, nil), git, nil, nil)

		_, err := service.PublishTicket(context.Background(), request())
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("requires a scratch commit message", func(t *testing.T) {
		git := newScratchTicketWorkflowGit(source, target)
		branches := branchapp.NewService(git, &fakeKeyPolicy{})
		service := NewTicketService(branches, branchapp.NewSynchronizer(git, branches, nil), git, nil, nil).
			WithScratchMerger(branchapp.NewScratchMerger(git, branches))
		missingMessage := request()
		missingMessage.ScratchMessage = nil

		_, err := service.PublishTicket(context.Background(), missingMessage)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("propagates a squash conflict", func(t *testing.T) {
		squashErr := errors.New("squash conflict")
		git := newScratchTicketWorkflowGit(source, target)
		git.squashErr = squashErr
		branches := branchapp.NewService(git, &fakeKeyPolicy{})
		service := NewTicketService(branches, branchapp.NewSynchronizer(git, branches, nil), git, nil, nil).
			WithScratchMerger(branchapp.NewScratchMerger(git, branches))

		_, err := service.PublishTicket(context.Background(), request())
		if !errors.Is(err, squashErr) {
			t.Fatalf("PublishTicket() error = %v, want %v", err, squashErr)
		}
	})
}

func TestTicketServiceExplicitPullRequestPublicationContracts(t *testing.T) {
	t.Run("requires a push before provider publication", func(t *testing.T) {
		request := PublishTicketRequest{
			Repository:        testRepository(),
			Branch:            mustBranch("feature/ABC-123-add-export"),
			CreatePullRequest: true,
		}
		_, err := newTicketServiceWithGit(newTicketCoverageGit(), nil, &fakePublisher{}).PublishTicket(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("requires the mandatory description before provider publication", func(t *testing.T) {
		request := PublishTicketRequest{
			Repository:        testRepository(),
			Branch:            mustBranch("feature/ABC-123-add-export"),
			Push:              true,
			CreatePullRequest: true,
		}
		_, err := newTicketServiceWithGit(newTicketCoverageGit(), nil, &fakePublisher{}).PublishTicket(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("validates publication dependencies and remote lookup", func(t *testing.T) {
		request := port.PullRequest{
			Source: mustBranch("feature/ABC-123-add-export"),
			Target: mustBranch("develop"),
			Title:  "ABC-123: add export",
			Body:   "Summary: Add the export button.",
		}
		var nilService *TicketService
		_, err := nilService.PublishPullRequest(context.Background(), testRepository(), request)
		assertProblemCode(t, err, problem.CodeExternalCommandFailed)

		_, err = (&TicketService{publisher: &fakePublisher{}}).PublishPullRequest(context.Background(), testRepository(), request)
		assertProblemCode(t, err, problem.CodeExternalCommandFailed)

		remoteErr := errors.New("remote URL unavailable")
		git := &fakeGitRepository{err: remoteErr}
		_, err = (&TicketService{git: git, publisher: &fakePublisher{}}).PublishPullRequest(context.Background(), testRepository(), request)
		if !errors.Is(err, remoteErr) {
			t.Fatalf("remote lookup error = %v, want %v", err, remoteErr)
		}
	})

	t.Run("preflights provider routing before a push", func(t *testing.T) {
		request := port.PullRequest{
			Source: mustBranch("feature/ABC-123-add-export"),
			Target: mustBranch("develop"),
			Title:  "ABC-123: add export",
			Body:   "Summary: Add the export button.",
		}
		git := &fakeGitRepository{}
		service := &TicketService{git: git, publisher: &fakePublisher{}}
		if err := service.PreflightPullRequest(context.Background(), testRepository(), request); err != nil {
			t.Fatalf("generic provider preflight error = %v", err)
		}

		preflightErr := errors.New("invalid provider configuration")
		publisher := &ticketPreflightPublisher{validateErr: preflightErr}
		service = &TicketService{git: git, publisher: publisher}
		err := service.PreflightPullRequest(context.Background(), testRepository(), request)
		if !errors.Is(err, preflightErr) || len(publisher.publications) != 1 {
			t.Fatalf("provider preflight = (%v, %#v)", err, publisher.publications)
		}

		remoteErr := errors.New("remote URL unavailable")
		service = &TicketService{git: &fakeGitRepository{err: remoteErr}, publisher: &ticketPreflightPublisher{}}
		err = service.PreflightPullRequest(context.Background(), testRepository(), request)
		if !errors.Is(err, remoteErr) {
			t.Fatalf("preflight remote lookup error = %v, want %v", err, remoteErr)
		}

		err = (&TicketService{}).PreflightPullRequest(context.Background(), testRepository(), request)
		assertProblemCode(t, err, problem.CodeExternalCommandFailed)
	})
}

func mustScratchCommitMessage(t *testing.T, raw string) commitmsg.Message {
	t.Helper()
	message, err := commitmsg.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return message
}

var _ port.GitRepository = (*ticketCoverageGit)(nil)
var _ port.GitRepository = (*scratchTicketWorkflowGit)(nil)
