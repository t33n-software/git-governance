package commitapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

type fakeGitRepository struct {
	staged         bool
	publication    branch.PublicationState
	err            error
	validateRefErr error
	stageErr       error
	stagedErr      error
	commitErr      error
	publicationErr error
	pushErr        error
	calls          []string
	stagedPaths    []string
	committed      commitmsg.Message
	pushedName     branch.BranchName
	setUpstream    bool
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
	return "", false, fake.err
}

func (fake *fakeGitRepository) HasCommits(context.Context, port.RepositoryIdentity) (bool, error) {
	fake.calls = append(fake.calls, "has-commits")
	return true, fake.err
}

func (fake *fakeGitRepository) IsWorktreeClean(context.Context, port.RepositoryIdentity) (bool, error) {
	fake.calls = append(fake.calls, "worktree-clean")
	return true, fake.err
}

func (fake *fakeGitRepository) CurrentBranch(context.Context, port.RepositoryIdentity) (branch.BranchName, error) {
	fake.calls = append(fake.calls, "current-branch")
	return mustBranch("feature/ABC-123-add-export"), fake.err
}

func (fake *fakeGitRepository) ValidateBranchRef(context.Context, port.RepositoryIdentity, branch.BranchName) error {
	fake.calls = append(fake.calls, "validate-ref")
	return fake.methodError(fake.validateRefErr)
}

func (fake *fakeGitRepository) BranchExists(context.Context, port.RepositoryIdentity, branch.BranchName) (bool, error) {
	fake.calls = append(fake.calls, "branch-exists")
	return false, fake.err
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

func (fake *fakeGitRepository) CreateBranch(context.Context, port.RepositoryIdentity, branch.BranchName, branch.TargetBase, bool) error {
	fake.calls = append(fake.calls, "create-branch")
	return fake.err
}

func (fake *fakeGitRepository) StoreWorkflowBase(context.Context, port.RepositoryIdentity, branch.BranchName, branch.TargetBase) error {
	fake.calls = append(fake.calls, "store-workflow-base")
	return fake.err
}

func (fake *fakeGitRepository) ClearWorkflowBase(context.Context, port.RepositoryIdentity, branch.BranchName) error {
	fake.calls = append(fake.calls, "clear-workflow-base")
	return fake.err
}

func (fake *fakeGitRepository) WorkflowBase(context.Context, port.RepositoryIdentity, branch.BranchName) (branch.TargetBase, bool, error) {
	fake.calls = append(fake.calls, "workflow-base")
	return branch.TargetBase{}, false, fake.err
}

func (fake *fakeGitRepository) SwitchBranch(context.Context, port.RepositoryIdentity, branch.BranchName) error {
	fake.calls = append(fake.calls, "switch")
	return fake.err
}

func (fake *fakeGitRepository) PublicationState(context.Context, port.RepositoryIdentity, branch.BranchName) (branch.PublicationState, error) {
	fake.calls = append(fake.calls, "publication")
	return fake.publication, fake.methodError(fake.publicationErr)
}

func (fake *fakeGitRepository) HasMissingBaseCommits(context.Context, port.RepositoryIdentity, branch.TargetBase) (bool, error) {
	fake.calls = append(fake.calls, "missing-base")
	return false, fake.err
}

func (fake *fakeGitRepository) CommitMessagesSince(context.Context, port.RepositoryIdentity, branch.TargetBase) ([]string, error) {
	fake.calls = append(fake.calls, "commit-messages")
	return nil, fake.err
}

func (fake *fakeGitRepository) Rebase(context.Context, port.RepositoryIdentity, branch.TargetBase) error {
	fake.calls = append(fake.calls, "rebase")
	return fake.err
}

func (fake *fakeGitRepository) ContinueRebase(context.Context, port.RepositoryIdentity) error {
	fake.calls = append(fake.calls, "continue-rebase")
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

func (fake *fakeGitRepository) CherryPick(context.Context, port.RepositoryIdentity, string) error {
	fake.calls = append(fake.calls, "cherry-pick")
	return fake.err
}

func (fake *fakeGitRepository) DeleteLocalBranch(context.Context, port.RepositoryIdentity, branch.BranchName, bool) error {
	fake.calls = append(fake.calls, "delete-local-branch")
	return fake.err
}

func (fake *fakeGitRepository) ReleaseTagsAt(context.Context, port.RepositoryIdentity, string) ([]string, error) {
	fake.calls = append(fake.calls, "release-tags")
	return nil, fake.err
}

func (fake *fakeGitRepository) HasUnmergedConflicts(context.Context, port.RepositoryIdentity) (bool, error) {
	fake.calls = append(fake.calls, "unmerged-conflicts")
	return false, fake.err
}

func (fake *fakeGitRepository) HasStagedChanges(context.Context, port.RepositoryIdentity) (bool, error) {
	fake.calls = append(fake.calls, "has-staged")
	return fake.staged, fake.methodError(fake.stagedErr)
}

func (fake *fakeGitRepository) Stage(_ context.Context, _ port.RepositoryIdentity, paths []string) error {
	fake.calls = append(fake.calls, "stage")
	fake.stagedPaths = append([]string(nil), paths...)
	return fake.methodError(fake.stageErr)
}

func (fake *fakeGitRepository) Commit(_ context.Context, _ port.RepositoryIdentity, message commitmsg.Message) error {
	fake.calls = append(fake.calls, "commit")
	fake.committed = message
	return fake.methodError(fake.commitErr)
}

func (fake *fakeGitRepository) Push(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName, setUpstream bool) error {
	fake.calls = append(fake.calls, "push")
	fake.pushedName = name
	fake.setUpstream = setUpstream
	return fake.methodError(fake.pushErr)
}

func (fake *fakeGitRepository) methodError(specific error) error {
	if specific != nil {
		return specific
	}
	return fake.err
}

func (fake *fakeGitRepository) InspectPushUpdate(context.Context, port.RepositoryIdentity, branch.TargetBase, string, string) (port.PushUpdateInspection, error) {
	fake.calls = append(fake.calls, "inspect-push")
	return port.PushUpdateInspection{}, fake.err
}

type fakeKeyPolicy struct {
	err  error
	keys []string
}

func (fake *fakeKeyPolicy) ValidateKey(_ context.Context, _ port.RepositoryIdentity, key ticket.Key) error {
	fake.keys = append(fake.keys, key.String())
	return fake.err
}

type fakePushValidator struct {
	err   error
	calls int
	name  branch.BranchName
	base  *branch.TargetBase
}

func (fake *fakePushValidator) ValidatePush(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName, base *branch.TargetBase) error {
	fake.calls++
	fake.name = name
	fake.base = base
	return fake.err
}

func TestValidateCommit(t *testing.T) {
	t.Parallel()

	git := &fakeGitRepository{}
	keys := &fakeKeyPolicy{}
	service := NewService(git, keys, nil)
	request := validRequest()

	actual, err := service.Validate(context.Background(), ValidateRequest{
		Repository: request.Repository,
		Branch:     request.Branch,
		Message:    request.Message,
	})
	if err != nil {
		t.Fatal(err)
	}
	if actual.Message.String() != request.Message.String() || strings.Join(keys.keys, ",") != "ABC" {
		t.Fatalf("Validate() = %#v, keys=%v", actual, keys.keys)
	}
	if strings.Join(git.calls, ",") != "validate-ref" {
		t.Fatalf("calls = %v", git.calls)
	}
}

func TestValidateCommitRejectsMismatchesAndSharedLines(t *testing.T) {
	t.Parallel()

	t.Run("ticket mismatch", func(t *testing.T) {
		git := &fakeGitRepository{}
		service := NewService(git, &fakeKeyPolicy{}, nil)
		message := mustMessage("feat(ABC-124): add export")
		_, err := service.Validate(context.Background(), ValidateRequest{
			Repository: testRepository(),
			Branch:     mustBranch("feature/ABC-123-add-export"),
			Message:    message,
		})
		assertProblemCode(t, err, problem.CodeCommitTicketMismatch)
	})

	t.Run("shared line", func(t *testing.T) {
		git := &fakeGitRepository{}
		service := NewService(git, &fakeKeyPolicy{}, nil)
		_, err := service.Validate(context.Background(), ValidateRequest{
			Repository: testRepository(),
			Branch:     mustBranch("develop"),
			Message:    mustMessage("feat(ABC-123): add export"),
		})
		assertProblemCode(t, err, problem.CodeSharedLineMutationForbidden)
		if len(git.calls) != 0 {
			t.Fatalf("shared line must stop before Git calls: %v", git.calls)
		}
	})
}

func TestValidateCommitWhiteboxPaths(t *testing.T) {
	t.Parallel()

	request := validRequest()
	t.Run("repository, context, branch, and message guards", func(t *testing.T) {
		_, err := NewService(&fakeGitRepository{}, &fakeKeyPolicy{}, nil).Validate(context.Background(), ValidateRequest{
			Branch:  request.Branch,
			Message: request.Message,
		})
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = NewService(&fakeGitRepository{}, &fakeKeyPolicy{}, nil).Validate(ctx, ValidateRequest{
			Repository: request.Repository,
			Branch:     request.Branch,
			Message:    request.Message,
		})
		assertProblemCode(t, err, problem.CodeOperationCancelled)

		_, err = NewService(&fakeGitRepository{}, &fakeKeyPolicy{}, nil).Validate(context.Background(), ValidateRequest{
			Repository: request.Repository,
			Message:    request.Message,
		})
		assertProblemCode(t, err, problem.CodeCommitHeaderInvalid)

		_, err = NewService(&fakeGitRepository{}, &fakeKeyPolicy{}, nil).Validate(context.Background(), ValidateRequest{
			Repository: request.Repository,
			Branch:     request.Branch,
		})
		assertProblemCode(t, err, problem.CodeCommitHeaderInvalid)
	})

	t.Run("git and policy errors are preserved", func(t *testing.T) {
		refErr := errors.New("ref failed")
		_, err := NewService(&fakeGitRepository{validateRefErr: refErr}, &fakeKeyPolicy{}, nil).Validate(context.Background(), ValidateRequest{
			Repository: request.Repository,
			Branch:     request.Branch,
			Message:    request.Message,
		})
		if !errors.Is(err, refErr) {
			t.Fatalf("Validate() error = %v, want %v", err, refErr)
		}

		policyErr := errors.New("policy failed")
		_, err = NewService(&fakeGitRepository{}, &fakeKeyPolicy{err: policyErr}, nil).Validate(context.Background(), ValidateRequest{
			Repository: request.Repository,
			Branch:     request.Branch,
			Message:    request.Message,
		})
		if !errors.Is(err, policyErr) {
			t.Fatalf("Validate() error = %v, want %v", err, policyErr)
		}

		if _, err := NewService(&fakeGitRepository{}, nil, nil).Validate(context.Background(), ValidateRequest{
			Repository: request.Repository,
			Branch:     request.Branch,
			Message:    request.Message,
		}); err != nil {
			t.Fatalf("nil policy Validate() error = %v", err)
		}
	})
}

func TestCreateCommitWithExplicitStage(t *testing.T) {
	t.Parallel()

	git := &fakeGitRepository{staged: true}
	keys := &fakeKeyPolicy{}
	service := NewService(git, keys, nil)
	request := validRequest()
	request.StagePaths = []string{"cmd/git-governance/main.go", "README.md"}

	actual, err := service.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !actual.Committed || actual.Pushed {
		t.Fatalf("Create() = %#v", actual)
	}
	if got := strings.Join(git.stagedPaths, ","); got != "cmd/git-governance/main.go,README.md" {
		t.Fatalf("StagePaths = %q", got)
	}
	if git.committed.String() != request.Message.String() {
		t.Fatalf("committed message = %q", git.committed.String())
	}
	if got := strings.Join(git.calls, ","); got != "validate-ref,stage,has-staged,commit" {
		t.Fatalf("calls = %q", got)
	}
}

func TestCreateCommitDryRunAndNoStagedChanges(t *testing.T) {
	t.Parallel()

	t.Run("dry run avoids mutation", func(t *testing.T) {
		git := &fakeGitRepository{}
		service := NewService(git, &fakeKeyPolicy{}, nil)
		request := validRequest()
		request.StagePaths = []string{"README.md"}
		request.DryRun = true
		actual, err := service.Create(context.Background(), request)
		if err != nil || !actual.DryRun || actual.Committed || len(actual.Plan) != 2 {
			t.Fatalf("Create() = (%#v, %v)", actual, err)
		}
		if got := strings.Join(git.calls, ","); got != "validate-ref" {
			t.Fatalf("calls = %q", got)
		}
	})

	t.Run("no staged changes", func(t *testing.T) {
		git := &fakeGitRepository{staged: false}
		service := NewService(git, &fakeKeyPolicy{}, nil)
		_, err := service.Create(context.Background(), validRequest())
		assertProblemCode(t, err, problem.CodeInvalidInput)
		if got := strings.Join(git.calls, ","); got != "validate-ref,has-staged" {
			t.Fatalf("calls = %q", got)
		}
	})
}

func TestCreateCommitWhiteboxPaths(t *testing.T) {
	t.Parallel()

	t.Run("validation failure stops before planning or mutation", func(t *testing.T) {
		git := &fakeGitRepository{}
		request := validRequest()
		request.Branch = branch.BranchName{}
		_, err := NewService(git, &fakeKeyPolicy{}, nil).Create(context.Background(), request)
		assertProblemCode(t, err, problem.CodeCommitHeaderInvalid)
		if len(git.calls) != 0 {
			t.Fatalf("validation failure must not call Git: %v", git.calls)
		}
	})

	t.Run("dry run includes push plan without mutation", func(t *testing.T) {
		git := &fakeGitRepository{}
		service := NewService(git, &fakeKeyPolicy{}, &fakePushValidator{})
		request := validRequest()
		request.Push = true
		request.DryRun = true
		result, err := service.Create(context.Background(), request)
		if err != nil || len(result.Plan) != 3 || result.Committed || result.Pushed {
			t.Fatalf("Create() = (%#v, %v)", result, err)
		}
		if got := strings.Join(git.calls, ","); got != "validate-ref" {
			t.Fatalf("dry-run calls = %q", got)
		}
	})

	t.Run("create applies the default remote after validation", func(t *testing.T) {
		git := &fakeGitRepository{staged: true}
		request := validRequest()
		request.Repository.Remote = ""
		result, err := NewService(git, &fakeKeyPolicy{}, nil).Create(context.Background(), request)
		if err != nil || !result.Committed {
			t.Fatalf("Create() = (%#v, %v)", result, err)
		}
	})

	t.Run("stage, staged, and commit failures are preserved", func(t *testing.T) {
		testCases := []struct {
			name string
			git  *fakeGitRepository
		}{
			{name: "stage", git: &fakeGitRepository{stageErr: errors.New("stage failed")}},
			{name: "staged", git: &fakeGitRepository{stagedErr: errors.New("staged failed")}},
			{name: "commit", git: &fakeGitRepository{staged: true, commitErr: errors.New("commit failed")}},
		}
		for _, testCase := range testCases {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				request := validRequest()
				if testCase.name == "stage" {
					request.StagePaths = []string{"README.md"}
				}
				_, err := NewService(testCase.git, &fakeKeyPolicy{}, nil).Create(context.Background(), request)
				if err == nil {
					t.Fatal("Create() error = nil")
				}
			})
		}
	})

	t.Run("push dependencies and publication paths are enforced", func(t *testing.T) {
		request := validRequest()
		request.Push = true

		git := &fakeGitRepository{staged: true}
		_, err := NewService(git, &fakeKeyPolicy{}, nil).Create(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInternal)

		publicationErr := errors.New("publication failed")
		git = &fakeGitRepository{staged: true, publicationErr: publicationErr}
		_, err = NewService(git, &fakeKeyPolicy{}, &fakePushValidator{}).Create(context.Background(), request)
		if !errors.Is(err, publicationErr) {
			t.Fatalf("publication error = %v", err)
		}

		pushErr := errors.New("push failed")
		git = &fakeGitRepository{staged: true, publication: branch.PublicationPublished, pushErr: pushErr}
		_, err = NewService(git, &fakeKeyPolicy{}, &fakePushValidator{}).Create(context.Background(), request)
		if !errors.Is(err, pushErr) {
			t.Fatalf("push error = %v", err)
		}
		if git.setUpstream {
			t.Fatal("published branch push must not set upstream")
		}
	})

	t.Run("helper contracts are actionable", func(t *testing.T) {
		repository, err := normalizeRepository(port.RepositoryIdentity{Root: "C:/repo"})
		if err != nil || repository.Remote != "origin" {
			t.Fatalf("normalizeRepository() = (%#v, %v)", repository, err)
		}
		for _, err := range []error{
			sharedLineForbidden(mustBranch("main")),
			invalidCommitInput("invalid input"),
			internalDependencyError("push validator"),
		} {
			if _, ok := problem.As(err); !ok {
				t.Fatalf("helper error %T is not a problem", err)
			}
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		assertProblemCode(t, contextError(ctx), problem.CodeOperationCancelled)
		if err := contextError(testNilContext()); err != nil {
			t.Fatalf("contextError(nil) = %v", err)
		}
	})
}

func TestCreateCommitPushesOnlyAfterValidation(t *testing.T) {
	t.Parallel()

	t.Run("first push configures upstream", func(t *testing.T) {
		git := &fakeGitRepository{
			staged:      true,
			publication: branch.PublicationUnpublished,
		}
		validator := &fakePushValidator{}
		service := NewService(git, &fakeKeyPolicy{}, validator)
		request := validRequest()
		request.Push = true
		base := mustBase("origin", "develop")
		request.Base = &base

		actual, err := service.Create(context.Background(), request)
		if err != nil || !actual.Committed || !actual.Pushed || validator.calls != 1 {
			t.Fatalf("Create() = (%#v, %v), validation calls=%d", actual, err, validator.calls)
		}
		if !git.setUpstream || git.pushedName.String() != request.Branch.String() {
			t.Fatalf("push = (%q, upstream=%t)", git.pushedName, git.setUpstream)
		}
		if got := strings.Join(git.calls, ","); got != "validate-ref,has-staged,commit,publication,push" {
			t.Fatalf("calls = %q", got)
		}
	})

	t.Run("validation failure stops before publication and push", func(t *testing.T) {
		git := &fakeGitRepository{staged: true}
		validator := &fakePushValidator{err: errorsForTest("pre-push failed")}
		service := NewService(git, &fakeKeyPolicy{}, validator)
		request := validRequest()
		request.Push = true
		_, err := service.Create(context.Background(), request)
		if err == nil {
			t.Fatal("Create() unexpectedly succeeded")
		}
		if strings.Contains(strings.Join(git.calls, ","), "push") || strings.Contains(strings.Join(git.calls, ","), "publication") {
			t.Fatalf("push state must not be queried after validation failure: %v", git.calls)
		}
	})

	t.Run("unknown publication blocks push", func(t *testing.T) {
		git := &fakeGitRepository{staged: true, publication: branch.PublicationUnknown}
		validator := &fakePushValidator{}
		service := NewService(git, &fakeKeyPolicy{}, validator)
		request := validRequest()
		request.Push = true
		_, err := service.Create(context.Background(), request)
		assertProblemCode(t, err, problem.CodeBranchPublicationUnknown)
		if strings.Contains(strings.Join(git.calls, ","), "push") {
			t.Fatalf("unknown publication must not push: %v", git.calls)
		}
	})
}

func validRequest() CreateRequest {
	return CreateRequest{
		Repository: testRepository(),
		Branch:     mustBranch("feature/ABC-123-add-export"),
		Message:    mustMessage("feat(ABC-123): add export"),
	}
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

func mustMessage(raw string) commitmsg.Message {
	value, err := commitmsg.Parse(raw)
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

type errorsForTest string

func (value errorsForTest) Error() string {
	return string(value)
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

func testNilContext() context.Context {
	return nil
}

var _ port.GitRepository = (*fakeGitRepository)(nil)
var _ port.KeyPolicy = (*fakeKeyPolicy)(nil)
var _ PushValidator = (*fakePushValidator)(nil)
