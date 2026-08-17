package branchapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/port"
	domainbranch "github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

type fakeGitRepository struct {
	hasCommits              bool
	clean                   bool
	exists                  bool
	publication             domainbranch.PublicationState
	missingBase             bool
	err                     error
	hasCommitsErr           error
	worktreeCleanErr        error
	validateRefErr          error
	validateRefErrors       []error
	branchExistsErr         error
	officialBranchesErr     error
	fetchErr                error
	targetBaseErr           error
	targetBaseMissing       bool
	createBranchErr         error
	workflowBaseErr         error
	publicationErr          error
	missingBaseErr          error
	unmergedConflictsErr    error
	rebaseErr               error
	continueRebaseErr       error
	continueMergeErr        error
	activeOperationErr      error
	mergeErr                error
	inspectionErr           error
	mergeTargetMatches      bool
	mergeTargetMatchesErr   error
	activeOperationSequence []activeOperationAnswer
	inspections             []port.PushUpdateInspection
	official                []domainbranch.BranchName
	workflowBase            *domainbranch.TargetBase
	activeOperation         string
	active                  bool
	unmergedConflicts       bool
	calls                   []string
	createdName             domainbranch.BranchName
	createdBase             domainbranch.TargetBase
	createdSwitch           bool
	mergedMessage           commitmsg.Message
}

func (fake *fakeGitRepository) Discover(context.Context, string) (port.RepositoryIdentity, error) {
	fake.calls = append(fake.calls, "discover")
	return port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"}, fake.err
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
	if len(fake.activeOperationSequence) > 0 {
		answer := fake.activeOperationSequence[0]
		fake.activeOperationSequence = fake.activeOperationSequence[1:]
		return answer.operation, answer.active, answer.err
	}
	if fake.activeOperationErr != nil {
		return "", false, fake.activeOperationErr
	}
	return fake.activeOperation, fake.active, fake.err
}

func (fake *fakeGitRepository) HasCommits(context.Context, port.RepositoryIdentity) (bool, error) {
	fake.calls = append(fake.calls, "has-commits")
	return fake.hasCommits, fake.methodError(fake.hasCommitsErr)
}

func (fake *fakeGitRepository) IsWorktreeClean(context.Context, port.RepositoryIdentity) (bool, error) {
	fake.calls = append(fake.calls, "worktree-clean")
	return fake.clean, fake.methodError(fake.worktreeCleanErr)
}

func (fake *fakeGitRepository) CurrentBranch(context.Context, port.RepositoryIdentity) (domainbranch.BranchName, error) {
	fake.calls = append(fake.calls, "current-branch")
	return mustBranch("feature/ABC-123-add-export"), fake.err
}

func (fake *fakeGitRepository) ValidateBranchRef(context.Context, port.RepositoryIdentity, domainbranch.BranchName) error {
	fake.calls = append(fake.calls, "validate-ref")
	if len(fake.validateRefErrors) > 0 {
		err := fake.validateRefErrors[0]
		fake.validateRefErrors = fake.validateRefErrors[1:]
		return err
	}
	return fake.methodError(fake.validateRefErr)
}

func (fake *fakeGitRepository) BranchExists(context.Context, port.RepositoryIdentity, domainbranch.BranchName) (bool, error) {
	fake.calls = append(fake.calls, "branch-exists")
	return fake.exists, fake.methodError(fake.branchExistsErr)
}

func (fake *fakeGitRepository) OfficialBranchesForTicket(_ context.Context, _ port.RepositoryIdentity, _ ticket.ID) ([]domainbranch.BranchName, error) {
	fake.calls = append(fake.calls, "official-branches-for-ticket")
	return append([]domainbranch.BranchName(nil), fake.official...), fake.methodError(fake.officialBranchesErr)
}

func (fake *fakeGitRepository) Fetch(context.Context, port.RepositoryIdentity) error {
	fake.calls = append(fake.calls, "fetch")
	return fake.methodError(fake.fetchErr)
}

func (fake *fakeGitRepository) TargetBaseExists(context.Context, port.RepositoryIdentity, domainbranch.TargetBase) (bool, error) {
	fake.calls = append(fake.calls, "target-base-exists")
	return !fake.targetBaseMissing, fake.methodError(fake.targetBaseErr)
}

func (fake *fakeGitRepository) CreateBranch(_ context.Context, _ port.RepositoryIdentity, name domainbranch.BranchName, base domainbranch.TargetBase, switchTo bool) error {
	fake.calls = append(fake.calls, "create-branch")
	fake.createdName = name
	fake.createdBase = base
	fake.createdSwitch = switchTo
	return fake.methodError(fake.createBranchErr)
}

func (fake *fakeGitRepository) methodError(specific error) error {
	if specific != nil {
		return specific
	}
	return fake.err
}

func (fake *fakeGitRepository) StoreWorkflowBase(_ context.Context, _ port.RepositoryIdentity, _ domainbranch.BranchName, base domainbranch.TargetBase) error {
	fake.calls = append(fake.calls, "store-workflow-base")
	copy := base
	fake.workflowBase = &copy
	return fake.err
}

func (fake *fakeGitRepository) ClearWorkflowBase(context.Context, port.RepositoryIdentity, domainbranch.BranchName) error {
	fake.calls = append(fake.calls, "clear-workflow-base")
	fake.workflowBase = nil
	return fake.err
}

func (fake *fakeGitRepository) WorkflowBase(context.Context, port.RepositoryIdentity, domainbranch.BranchName) (domainbranch.TargetBase, bool, error) {
	fake.calls = append(fake.calls, "workflow-base")
	if fake.workflowBase == nil {
		return domainbranch.TargetBase{}, false, fake.methodError(fake.workflowBaseErr)
	}
	return *fake.workflowBase, true, fake.methodError(fake.workflowBaseErr)
}

func (fake *fakeGitRepository) SwitchBranch(context.Context, port.RepositoryIdentity, domainbranch.BranchName) error {
	fake.calls = append(fake.calls, "switch")
	return fake.err
}

func (fake *fakeGitRepository) PublicationState(context.Context, port.RepositoryIdentity, domainbranch.BranchName) (domainbranch.PublicationState, error) {
	fake.calls = append(fake.calls, "publication-state")
	return fake.publication, fake.methodError(fake.publicationErr)
}

func (fake *fakeGitRepository) HasMissingBaseCommits(context.Context, port.RepositoryIdentity, domainbranch.TargetBase) (bool, error) {
	fake.calls = append(fake.calls, "missing-base")
	return fake.missingBase, fake.methodError(fake.missingBaseErr)
}

func (fake *fakeGitRepository) CommitMessagesSince(context.Context, port.RepositoryIdentity, domainbranch.TargetBase) ([]string, error) {
	fake.calls = append(fake.calls, "commit-messages")
	return nil, fake.err
}

func (fake *fakeGitRepository) Rebase(context.Context, port.RepositoryIdentity, domainbranch.TargetBase) error {
	fake.calls = append(fake.calls, "rebase")
	return fake.methodError(fake.rebaseErr)
}

func (fake *fakeGitRepository) ContinueRebase(context.Context, port.RepositoryIdentity) error {
	fake.calls = append(fake.calls, "continue-rebase")
	return fake.methodError(fake.continueRebaseErr)
}

func (fake *fakeGitRepository) ContinueMerge(context.Context, port.RepositoryIdentity) error {
	fake.calls = append(fake.calls, "continue-merge")
	return fake.methodError(fake.continueMergeErr)
}

func (fake *fakeGitRepository) ActiveMergeTargetMatches(context.Context, port.RepositoryIdentity, domainbranch.TargetBase) (bool, error) {
	fake.calls = append(fake.calls, "merge-target-matches")
	return fake.mergeTargetMatches, fake.methodError(fake.mergeTargetMatchesErr)
}

func (fake *fakeGitRepository) Merge(_ context.Context, _ port.RepositoryIdentity, _ domainbranch.TargetBase, message commitmsg.Message) error {
	fake.calls = append(fake.calls, "merge")
	fake.mergedMessage = message
	return fake.methodError(fake.mergeErr)
}

func (fake *fakeGitRepository) SquashMerge(context.Context, port.RepositoryIdentity, domainbranch.BranchName) error {
	fake.calls = append(fake.calls, "squash-merge")
	return fake.err
}

func (fake *fakeGitRepository) CherryPick(context.Context, port.RepositoryIdentity, string) error {
	fake.calls = append(fake.calls, "cherry-pick")
	return fake.err
}

func (fake *fakeGitRepository) DeleteLocalBranch(context.Context, port.RepositoryIdentity, domainbranch.BranchName, bool) error {
	fake.calls = append(fake.calls, "delete-local-branch")
	return fake.err
}

func (fake *fakeGitRepository) ReleaseTagsAt(context.Context, port.RepositoryIdentity, string) ([]string, error) {
	fake.calls = append(fake.calls, "release-tags")
	return nil, fake.err
}

func (fake *fakeGitRepository) HasUnmergedConflicts(context.Context, port.RepositoryIdentity) (bool, error) {
	fake.calls = append(fake.calls, "unmerged-conflicts")
	return fake.unmergedConflicts, fake.methodError(fake.unmergedConflictsErr)
}

func (fake *fakeGitRepository) HasStagedChanges(context.Context, port.RepositoryIdentity) (bool, error) {
	fake.calls = append(fake.calls, "staged")
	return false, fake.err
}

func (fake *fakeGitRepository) Stage(context.Context, port.RepositoryIdentity, []string) error {
	fake.calls = append(fake.calls, "stage")
	return fake.err
}

func (fake *fakeGitRepository) Commit(context.Context, port.RepositoryIdentity, commitmsg.Message) error {
	fake.calls = append(fake.calls, "commit")
	return fake.err
}

func (fake *fakeGitRepository) Push(context.Context, port.RepositoryIdentity, domainbranch.BranchName, bool) error {
	fake.calls = append(fake.calls, "push")
	return fake.err
}

func (fake *fakeGitRepository) InspectPushUpdate(context.Context, port.RepositoryIdentity, domainbranch.TargetBase, string, string) (port.PushUpdateInspection, error) {
	fake.calls = append(fake.calls, "inspect-push")
	if fake.inspectionErr != nil {
		return port.PushUpdateInspection{}, fake.inspectionErr
	}
	if len(fake.inspections) == 0 {
		return port.PushUpdateInspection{}, nil
	}
	inspection := fake.inspections[0]
	fake.inspections = fake.inspections[1:]
	return inspection, nil
}

type fakeKeyPolicy struct {
	err  error
	keys []string
}

func (fake *fakeKeyPolicy) ValidateKey(_ context.Context, _ port.RepositoryIdentity, key ticket.Key) error {
	fake.keys = append(fake.keys, key.String())
	return fake.err
}

type fakeQualityRunner struct {
	calls    int
	err      error
	requests []port.QualityRequest
}

func (fake *fakeQualityRunner) Run(_ context.Context, _ port.RepositoryIdentity, request port.QualityRequest) (port.QualityResult, error) {
	fake.calls++
	fake.requests = append(fake.requests, port.QualityRequest{
		Families: append([]domainbranch.Family(nil), request.Families...),
	})
	return port.QualityResult{Status: port.QualityPassed}, fake.err
}

func TestListFamiliesIsCompleteAndIndependent(t *testing.T) {
	t.Parallel()

	actual := ListFamilies()
	if len(actual) != 13 {
		t.Fatalf("ListFamilies() length = %d, want 13", len(actual))
	}
	if actual[0].Family != domainbranch.FamilyMain || actual[len(actual)-1].Family != domainbranch.FamilyScratch {
		t.Fatalf("ListFamilies() = %#v", actual)
	}
	actual[0].Label = "changed"
	if ListFamilies()[0].Label == "changed" {
		t.Fatal("ListFamilies returned shared backing storage")
	}
}

func TestValidateAllowsSharedLinesButChecksRefAndPolicy(t *testing.T) {
	t.Parallel()

	git := &fakeGitRepository{}
	keys := &fakeKeyPolicy{}
	service := NewService(git, keys)

	main := mustBranch("main")
	if _, err := service.Validate(context.Background(), ValidateRequest{Repository: testRepository(), Name: main}); err != nil {
		t.Fatalf("Validate(main) error = %v", err)
	}
	if got := strings.Join(git.calls, ","); got != "validate-ref" {
		t.Fatalf("calls = %q", got)
	}

	feature := mustBranch("feature/ABC-123-add-export")
	git.calls = nil
	if _, err := service.Validate(context.Background(), ValidateRequest{Repository: testRepository(), Name: feature}); err != nil {
		t.Fatalf("Validate(feature) error = %v", err)
	}
	if got := strings.Join(keys.keys, ","); got != "ABC" {
		t.Fatalf("policy keys = %q", got)
	}
	if got := strings.Join(git.calls, ","); got != "validate-ref" {
		t.Fatalf("calls = %q", got)
	}
}

func TestValidateRejectsInvalidStateAndPropagatesDependencies(t *testing.T) {
	t.Parallel()

	feature := mustBranch("feature/ABC-123-add-export")
	t.Run("repository root is required", func(t *testing.T) {
		_, err := NewService(&fakeGitRepository{}, &fakeKeyPolicy{}).Validate(
			context.Background(),
			ValidateRequest{Name: feature},
		)
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)
	})
	t.Run("branch name is required", func(t *testing.T) {
		_, err := NewService(&fakeGitRepository{}, &fakeKeyPolicy{}).Validate(
			context.Background(),
			ValidateRequest{Repository: testRepository()},
		)
		assertProblemCode(t, err, problem.CodeBranchNameInvalid)
	})
	t.Run("cancelled context stops validation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := NewService(&fakeGitRepository{}, &fakeKeyPolicy{}).Validate(
			ctx,
			ValidateRequest{Repository: testRepository(), Name: feature},
		)
		assertProblemCode(t, err, problem.CodeOperationCancelled)
	})
	t.Run("key policy failure is preserved", func(t *testing.T) {
		expected := errors.New("policy denied")
		_, err := NewService(&fakeGitRepository{}, &fakeKeyPolicy{err: expected}).Validate(
			context.Background(),
			ValidateRequest{Repository: testRepository(), Name: feature},
		)
		if !errors.Is(err, expected) {
			t.Fatalf("Validate() error = %v, want %v", err, expected)
		}
	})
	t.Run("git ref failure is preserved", func(t *testing.T) {
		expected := errors.New("invalid ref")
		_, err := NewService(&fakeGitRepository{validateRefErr: expected}, &fakeKeyPolicy{}).Validate(
			context.Background(),
			ValidateRequest{Repository: testRepository(), Name: feature},
		)
		if !errors.Is(err, expected) {
			t.Fatalf("Validate() error = %v, want %v", err, expected)
		}
	})
	t.Run("nil key policy remains optional", func(t *testing.T) {
		if _, err := NewService(&fakeGitRepository{}, nil).Validate(
			context.Background(),
			ValidateRequest{Repository: testRepository(), Name: feature},
		); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})
}

func TestCreateRegularBranch(t *testing.T) {
	t.Parallel()

	git := &fakeGitRepository{hasCommits: true, clean: true}
	keys := &fakeKeyPolicy{}
	service := NewService(git, keys)
	request := regularRequest()

	actual, err := service.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if actual.Name.String() != "feature/ABC-123-add-export" || actual.Base.String() != "origin/develop" || !actual.Switched {
		t.Fatalf("Create() = %#v", actual)
	}
	if git.createdName.String() != actual.Name.String() || git.createdBase.String() != "origin/develop" || !git.createdSwitch {
		t.Fatalf("create call = (%q, %q, %t)", git.createdName, git.createdBase, git.createdSwitch)
	}
	expectedCalls := "validate-ref,has-commits,worktree-clean,fetch,target-base-exists,branch-exists,official-branches-for-ticket,create-branch"
	if got := strings.Join(git.calls, ","); got != expectedCalls {
		t.Fatalf("calls = %q, want %q", got, expectedCalls)
	}
}

func TestCreateDryRunNeverMutates(t *testing.T) {
	t.Parallel()

	git := &fakeGitRepository{hasCommits: true, clean: true}
	service := NewService(git, &fakeKeyPolicy{})
	request := regularRequest()
	request.DryRun = true
	switchTo := false
	request.Switch = &switchTo

	actual, err := service.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !actual.DryRun || actual.Switched || len(actual.Plan) != 2 {
		t.Fatalf("Create() = %#v", actual)
	}
	if got := strings.Join(git.calls, ","); got != "validate-ref,has-commits,branch-exists,official-branches-for-ticket" {
		t.Fatalf("dry-run calls = %q", got)
	}
}

func TestCreateHonorsSkipFetchAndExplicitNoSwitch(t *testing.T) {
	t.Parallel()

	git := &fakeGitRepository{hasCommits: true, clean: true}
	service := NewService(git, &fakeKeyPolicy{})
	request := regularRequest()
	request.SkipFetch = true
	switchTo := false
	request.Switch = &switchTo

	result, err := service.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Switched || len(result.Plan) != 1 || result.Plan[0].Action != "create" {
		t.Fatalf("Create() = %#v", result)
	}
	if strings.Contains(strings.Join(git.calls, ","), "fetch") || git.createdSwitch {
		t.Fatalf("Create() did not honor skip fetch/no switch: %v", git.calls)
	}
}

func TestCreateStopsBeforeMutationOnInvalidState(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		git     *fakeGitRepository
		request CreateRequest
		code    problem.Code
		calls   string
	}{
		{
			name:    "no commits",
			git:     &fakeGitRepository{hasCommits: false},
			request: regularRequest(),
			code:    problem.CodeRepositoryHasNoCommits,
			calls:   "validate-ref,has-commits",
		},
		{
			name:    "branch exists",
			git:     &fakeGitRepository{hasCommits: true, clean: true, exists: true},
			request: regularRequest(),
			code:    problem.CodeBranchAlreadyExists,
			calls:   "validate-ref,has-commits,worktree-clean,fetch,target-base-exists,branch-exists",
		},
		{
			name:    "target base is absent after fetch",
			git:     &fakeGitRepository{hasCommits: true, clean: true, targetBaseMissing: true},
			request: regularRequest(),
			code:    problem.CodeBranchBaseInvalid,
			calls:   "validate-ref,has-commits,worktree-clean,fetch,target-base-exists",
		},
		{
			name:    "dirty worktree",
			git:     &fakeGitRepository{hasCommits: true, clean: false},
			request: regularRequest(),
			code:    problem.CodeWorktreeNotClean,
			calls:   "validate-ref,has-commits,worktree-clean",
		},
		{
			name:    "hotfix outside workflow",
			git:     &fakeGitRepository{},
			request: hotfixRequest(false),
			code:    problem.CodeBranchFamilyInvalid,
			calls:   "",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			service := NewService(testCase.git, &fakeKeyPolicy{})
			_, err := service.Create(context.Background(), testCase.request)
			assertProblemCode(t, err, testCase.code)
			if got := strings.Join(testCase.git.calls, ","); got != testCase.calls {
				t.Fatalf("calls = %q, want %q", got, testCase.calls)
			}
		})
	}
}

func TestCreatePropagatesEveryGitDependencyFailure(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		git  *fakeGitRepository
	}{
		{name: "has commits", git: &fakeGitRepository{hasCommitsErr: errors.New("head failed")}},
		{name: "worktree", git: &fakeGitRepository{hasCommits: true, worktreeCleanErr: errors.New("status failed")}},
		{name: "fetch", git: &fakeGitRepository{hasCommits: true, clean: true, fetchErr: errors.New("fetch failed")}},
		{name: "target base", git: &fakeGitRepository{hasCommits: true, clean: true, targetBaseErr: errors.New("base failed")}},
		{name: "branch exists", git: &fakeGitRepository{hasCommits: true, clean: true, branchExistsErr: errors.New("exists failed")}},
		{name: "ticket lookup", git: &fakeGitRepository{hasCommits: true, clean: true, officialBranchesErr: errors.New("lookup failed")}},
		{name: "create", git: &fakeGitRepository{hasCommits: true, clean: true, createBranchErr: errors.New("create failed")}},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewService(testCase.git, &fakeKeyPolicy{}).Create(context.Background(), regularRequest())
			if err == nil {
				t.Fatal("Create() error = nil")
			}
		})
	}

	t.Run("dry run propagates availability failure without mutation", func(t *testing.T) {
		git := &fakeGitRepository{hasCommits: true, branchExistsErr: errors.New("exists failed")}
		request := regularRequest()
		request.DryRun = true
		_, err := NewService(git, &fakeKeyPolicy{}).Create(context.Background(), request)
		if err == nil {
			t.Fatal("Create() error = nil")
		}
		if strings.Contains(strings.Join(git.calls, ","), "fetch") || strings.Contains(strings.Join(git.calls, ","), "create-branch") {
			t.Fatalf("dry run mutated through calls %v", git.calls)
		}
	})

	t.Run("missing repository and cancelled context stop before Git", func(t *testing.T) {
		service := NewService(&fakeGitRepository{}, &fakeKeyPolicy{})
		request := regularRequest()
		request.Repository = port.RepositoryIdentity{}
		_, err := service.Create(context.Background(), request)
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = service.Create(ctx, regularRequest())
		assertProblemCode(t, err, problem.CodeOperationCancelled)
	})

	t.Run("policy and ref failures stop before repository mutation", func(t *testing.T) {
		expected := errors.New("policy failed")
		_, err := NewService(&fakeGitRepository{}, &fakeKeyPolicy{err: expected}).Create(context.Background(), regularRequest())
		if !errors.Is(err, expected) {
			t.Fatalf("Create() error = %v, want %v", err, expected)
		}

		expected = errors.New("ref failed")
		_, err = NewService(&fakeGitRepository{validateRefErr: expected}, &fakeKeyPolicy{}).Create(context.Background(), regularRequest())
		if !errors.Is(err, expected) {
			t.Fatalf("Create() error = %v, want %v", err, expected)
		}
	})
}

func TestCreateSpecialBranchesRequireCorrectBases(t *testing.T) {
	t.Parallel()

	t.Run("workflow hotfix from main", func(t *testing.T) {
		git := &fakeGitRepository{hasCommits: true, clean: true}
		service := NewService(git, &fakeKeyPolicy{})
		actual, err := service.Create(context.Background(), hotfixRequest(true))
		if err != nil {
			t.Fatal(err)
		}
		if actual.Base.String() != "origin/main" {
			t.Fatalf("hotfix base = %q", actual.Base.String())
		}
	})

	t.Run("scratch requires official base", func(t *testing.T) {
		git := &fakeGitRepository{}
		service := NewService(git, &fakeKeyPolicy{})
		request := regularRequest()
		request.Family = domainbranch.FamilyScratch
		request.Base = nil
		_, err := service.Create(context.Background(), request)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})

	t.Run("scratch uses matching local official base", func(t *testing.T) {
		git := &fakeGitRepository{hasCommits: true, clean: true}
		service := NewService(git, &fakeKeyPolicy{})
		request := regularRequest()
		request.Family = domainbranch.FamilyScratch
		request.Slug = mustSlug("exploration")
		base, err := domainbranch.NewLocalBase(mustBranch("feature/ABC-123-add-export"))
		if err != nil {
			t.Fatal(err)
		}
		request.Base = &base
		actual, err := service.Create(context.Background(), request)
		if err != nil || actual.Base.String() != "feature/ABC-123-add-export" {
			t.Fatalf("Create() = (%#v, %v)", actual, err)
		}
	})

	t.Run("scratch rejects a different ticket base", func(t *testing.T) {
		git := &fakeGitRepository{}
		service := NewService(git, &fakeKeyPolicy{})
		request := regularRequest()
		request.Family = domainbranch.FamilyScratch
		request.Slug = mustSlug("exploration")
		base, err := domainbranch.NewLocalBase(mustBranch("feature/ABC-124-other-ticket"))
		if err != nil {
			t.Fatal(err)
		}
		request.Base = &base
		_, err = service.Create(context.Background(), request)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})

	t.Run("scratch rejects a remote tracking base", func(t *testing.T) {
		git := &fakeGitRepository{}
		service := NewService(git, &fakeKeyPolicy{})
		request := regularRequest()
		request.Family = domainbranch.FamilyScratch
		request.Slug = mustSlug("exploration")
		base := mustBase("origin", "feature/ABC-123-add-export")
		request.Base = &base

		_, err := service.Create(context.Background(), request)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
		if len(git.calls) != 0 {
			t.Fatalf("remote scratch base must stop before Git calls: %v", git.calls)
		}
	})
}

func TestCreateRejectsAnotherOfficialRegularBranchForTheSameTicket(t *testing.T) {
	t.Parallel()

	git := &fakeGitRepository{
		hasCommits: true,
		clean:      true,
		official:   []domainbranch.BranchName{mustBranch("fix/ABC-123-existing-ticket-work")},
	}
	service := NewService(git, &fakeKeyPolicy{})

	_, err := service.Create(context.Background(), regularRequest())
	assertProblemCode(t, err, problem.CodeTicketBranchAlreadyExists)
	if strings.Contains(strings.Join(git.calls, ","), "create-branch") {
		t.Fatalf("duplicate ticket must stop before branch creation: %v", git.calls)
	}
}

func TestCreateAllowsWorkflowManagedBranchesToReuseTicketAcrossActiveLines(t *testing.T) {
	t.Parallel()

	git := &fakeGitRepository{
		hasCommits: true,
		clean:      true,
		official:   []domainbranch.BranchName{mustBranch("feature/ABC-123-existing-ticket-work")},
	}
	service := NewService(git, &fakeKeyPolicy{})
	base := mustBase("origin", "release/2.8.0")
	request := CreateRequest{
		Repository:      testRepository(),
		Family:          domainbranch.FamilyFix,
		Ticket:          mustTicket("ABC-123"),
		Slug:            mustSlug("release-blocker"),
		Base:            &base,
		WorkflowManaged: true,
	}

	result, err := service.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Name.String() != "fix/ABC-123-release-blocker" {
		t.Fatalf("Create() branch = %q", result.Name.String())
	}
	if strings.Contains(strings.Join(git.calls, ","), "official-branches-for-ticket") {
		t.Fatalf("workflow-managed branch must not apply regular-ticket exclusivity: %v", git.calls)
	}
}

func TestBranchCreationHelperContracts(t *testing.T) {
	t.Parallel()

	t.Run("exclusive ticket branch applies only to regular work", func(t *testing.T) {
		for _, testCase := range []struct {
			family          domainbranch.Family
			workflowManaged bool
			want            bool
		}{
			{domainbranch.FamilyFeature, false, true},
			{domainbranch.FamilyFix, false, true},
			{domainbranch.FamilyDocs, false, true},
			{domainbranch.FamilyRefactor, false, true},
			{domainbranch.FamilyChore, false, true},
			{domainbranch.FamilyTest, false, true},
			{domainbranch.FamilyPerf, false, true},
			{domainbranch.FamilyHotfix, false, false},
			{domainbranch.FamilyScratch, false, false},
			{domainbranch.FamilyFeature, true, false},
		} {
			if got := requiresExclusiveTicketBranch(testCase.family, testCase.workflowManaged); got != testCase.want {
				t.Fatalf("requiresExclusiveTicketBranch(%q, %t) = %t, want %t", testCase.family, testCase.workflowManaged, got, testCase.want)
			}
		}
	})

	t.Run("normalization defaults remote and rejects missing root", func(t *testing.T) {
		if _, err := normalizeRepository(port.RepositoryIdentity{}); err == nil {
			t.Fatal("normalizeRepository accepted empty root")
		}
		repository, err := normalizeRepository(port.RepositoryIdentity{Root: "C:/repo"})
		if err != nil || repository.Remote != "origin" {
			t.Fatalf("normalizeRepository() = (%#v, %v)", repository, err)
		}
	})

	t.Run("resolver rejects invalid workflow families and inputs", func(t *testing.T) {
		for _, request := range []CreateRequest{
			{Family: domainbranch.Family("unknown")},
			{Family: domainbranch.FamilyRelease},
			{Family: domainbranch.FamilySupport},
			{Family: domainbranch.FamilyMain},
			hotfixRequest(false),
			{Family: domainbranch.FamilyFeature, Ticket: ticket.ID{}},
		} {
			if _, _, err := resolveCreation(request, testRepository()); err == nil {
				t.Fatalf("resolveCreation(%#v) error = nil", request)
			}
		}
	})

	t.Run("regular base cannot be overridden outside workflow", func(t *testing.T) {
		request := regularRequest()
		base := mustBase("origin", "main")
		request.Base = &base
		if _, _, err := resolveCreation(request, testRepository()); err == nil {
			t.Fatal("resolveCreation accepted main base for regular feature branch")
		}
	})

	t.Run("invalid remote and special base errors are preserved", func(t *testing.T) {
		request := regularRequest()
		if _, _, err := resolveCreation(request, port.RepositoryIdentity{Root: "C:/repo", Remote: "bad/ref"}); err == nil {
			t.Fatal("resolveCreation accepted an invalid remote name")
		}

		request = regularRequest()
		request.Family = domainbranch.FamilyFix
		request.WorkflowManaged = true
		base := mustBase("origin", "feature/ABC-123-add-export")
		request.Base = &base
		if _, _, err := resolveCreation(request, testRepository()); err == nil {
			t.Fatal("resolveCreation accepted a workflow-managed fix from a ticket branch")
		}

		request = hotfixRequest(true)
		base = mustBase("origin", "develop")
		request.Base = &base
		if _, _, err := resolveCreation(request, testRepository()); err == nil {
			t.Fatal("resolveCreation accepted a hotfix from develop")
		}
	})

	t.Run("factory errors remain visible", func(t *testing.T) {
		expected := errors.New("factory failed")
		_, _, err := resolveCreationWithFactory(
			regularRequest(),
			testRepository(),
			func(domainbranch.Family, ticket.ID, domainbranch.Slug) (domainbranch.BranchName, error) {
				return domainbranch.BranchName{}, expected
			},
		)
		if !errors.Is(err, expected) {
			t.Fatalf("resolveCreationWithFactory() error = %v, want %v", err, expected)
		}
	})

	t.Run("workflow managed release stabilization bases are accepted", func(t *testing.T) {
		for _, family := range []domainbranch.Family{domainbranch.FamilyFix, domainbranch.FamilyDocs, domainbranch.FamilyChore} {
			request := regularRequest()
			request.Family = family
			request.WorkflowManaged = true
			base := mustBase("origin", "release/2.8.0")
			request.Base = &base
			_, resolved, err := resolveCreation(request, testRepository())
			if err != nil || resolved.String() != "origin/release/2.8.0" {
				t.Fatalf("resolveCreation(%q) = (%q, %v)", family, resolved, err)
			}
		}
	})

	t.Run("hotfix accepts every real affected shared line", func(t *testing.T) {
		for _, raw := range []string{"main", "release/2.8.0", "support/2.7"} {
			request := hotfixRequest(true)
			base := mustBase("origin", raw)
			request.Base = &base
			_, resolved, err := resolveCreation(request, testRepository())
			if err != nil || resolved.String() != "origin/"+raw {
				t.Fatalf("resolveCreation(%q) = (%q, %v)", raw, resolved, err)
			}
		}
	})

	t.Run("workflow managed feature cannot override develop", func(t *testing.T) {
		request := regularRequest()
		request.WorkflowManaged = true
		base := mustBase("origin", "main")
		request.Base = &base
		if _, _, err := resolveCreation(request, testRepository()); err == nil {
			t.Fatal("resolveCreation accepted a non-develop feature base")
		}
	})

	t.Run("special base rules reject incorrect lines", func(t *testing.T) {
		testCases := []struct {
			family domainbranch.Family
			base   domainbranch.TargetBase
		}{
			{domainbranch.FamilyHotfix, mustBase("origin", "develop")},
			{domainbranch.FamilyScratch, mustBase("origin", "main")},
			{domainbranch.FamilyFix, mustBase("origin", "feature/ABC-123-add-export")},
			{domainbranch.FamilyDocs, mustBase("origin", "develop")},
			{domainbranch.FamilyChore, mustBase("origin", "develop")},
		}
		for _, testCase := range testCases {
			if err := validateSpecialBase(testCase.family, testCase.base); err == nil {
				t.Fatalf("validateSpecialBase(%q, %q) error = nil", testCase.family, testCase.base)
			}
		}
		if err := validateSpecialBase(domainbranch.FamilyFeature, mustBase("origin", "develop")); err != nil {
			t.Fatalf("validateSpecialBase(feature) error = %v", err)
		}
	})

	t.Run("problem helpers and plans remain actionable", func(t *testing.T) {
		for _, err := range []error{
			specialWorkflowRequired(domainbranch.FamilyRelease),
			invalidBranchInput("missing branch"),
			invalidBase("origin/main", "wrong base", "origin/develop"),
		} {
			if _, ok := problem.As(err); !ok {
				t.Fatalf("helper error %T is not a problem", err)
			}
		}
		if got := (PlanStep{Action: "fetch", Detail: "origin"}).String(); got != "fetch: origin" {
			t.Fatalf("PlanStep.String() = %q", got)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		assertProblemCode(t, contextError(ctx), problem.CodeOperationCancelled)
		if err := contextError(testNilContext()); err != nil {
			t.Fatalf("contextError(nil) = %v", err)
		}
	})
}

func TestSyncPolicy(t *testing.T) {
	t.Parallel()

	name := mustBranch("feature/ABC-123-add-export")
	base := mustBase("origin", "develop")
	message := mustMessage(t, "chore(ABC-123): merge origin/develop")

	t.Run("up to date check", func(t *testing.T) {
		git := &fakeGitRepository{
			publication: domainbranch.PublicationUnpublished,
			clean:       true,
			missingBase: false,
		}
		sync := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		actual, err := sync.Sync(context.Background(), SyncRequest{
			Repository: testRepository(),
			Name:       name,
			Base:       &base,
			Strategy:   SyncCheck,
		})
		if err != nil || actual.RecommendedAction != "none" || actual.Mutated {
			t.Fatalf("Sync() = (%#v, %v)", actual, err)
		}
	})

	t.Run("unpublished auto rebase", func(t *testing.T) {
		git := &fakeGitRepository{
			publication: domainbranch.PublicationUnpublished,
			clean:       true,
			missingBase: true,
		}
		quality := &fakeQualityRunner{}
		sync := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), quality)
		actual, err := sync.Sync(context.Background(), SyncRequest{
			Repository: testRepository(),
			Name:       name,
			Base:       &base,
			Strategy:   SyncAuto,
		})
		if err != nil || !actual.Mutated || actual.RecommendedAction != "rebased" || quality.calls != 1 {
			t.Fatalf("Sync() = (%#v, %v), quality=%d", actual, err, quality.calls)
		}
		if !strings.Contains(strings.Join(git.calls, ","), "rebase") {
			t.Fatalf("calls = %v", git.calls)
		}
	})

	t.Run("published auto recommends merge", func(t *testing.T) {
		git := &fakeGitRepository{
			publication: domainbranch.PublicationPublished,
			clean:       true,
			missingBase: true,
		}
		sync := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		actual, err := sync.Sync(context.Background(), SyncRequest{
			Repository: testRepository(),
			Name:       name,
			Base:       &base,
			Strategy:   SyncAuto,
		})
		if err != nil || actual.Mutated || actual.RecommendedAction != "merge" {
			t.Fatalf("Sync() = (%#v, %v)", actual, err)
		}
		if strings.Contains(strings.Join(git.calls, ","), "merge") {
			t.Fatalf("auto must not merge published branch: %v", git.calls)
		}
	})

	t.Run("published rebase is forbidden", func(t *testing.T) {
		git := &fakeGitRepository{
			publication: domainbranch.PublicationPublished,
			clean:       true,
			missingBase: true,
		}
		sync := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		_, err := sync.Sync(context.Background(), SyncRequest{
			Repository: testRepository(),
			Name:       name,
			Base:       &base,
			Strategy:   SyncRebase,
		})
		assertProblemCode(t, err, problem.CodeRebaseAfterPublishForbidden)
		if strings.Contains(strings.Join(git.calls, ","), "rebase") {
			t.Fatalf("published branch must not rebase: %v", git.calls)
		}
	})

	t.Run("published merge", func(t *testing.T) {
		git := &fakeGitRepository{
			publication: domainbranch.PublicationPublished,
			clean:       true,
			missingBase: true,
		}
		quality := &fakeQualityRunner{}
		sync := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), quality)
		actual, err := sync.Sync(context.Background(), SyncRequest{
			Repository:   testRepository(),
			Name:         name,
			Base:         &base,
			Strategy:     SyncMerge,
			MergeMessage: &message,
		})
		if err != nil || !actual.Mutated || actual.RecommendedAction != "merged" || quality.calls != 1 {
			t.Fatalf("Sync() = (%#v, %v), quality=%d", actual, err, quality.calls)
		}
		if git.mergedMessage.String() != message.String() {
			t.Fatalf("merge message = %q", git.mergedMessage.String())
		}

		wrongTicket := mustMessage(t, "chore(ABC-124): merge origin/develop")
		git = &fakeGitRepository{
			publication: domainbranch.PublicationPublished,
			clean:       true,
			missingBase: true,
		}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).Sync(context.Background(), SyncRequest{
			Repository:   testRepository(),
			Name:         name,
			Base:         &base,
			Strategy:     SyncMerge,
			MergeMessage: &wrongTicket,
		})
		assertProblemCode(t, err, problem.CodeCommitTicketMismatch)
		if strings.Contains(strings.Join(git.calls, ","), "merge") {
			t.Fatalf("mismatched merge ticket invoked Git merge: %v", git.calls)
		}
	})
}

func TestSynchronizerSyncWhiteboxPaths(t *testing.T) {
	t.Parallel()

	name := mustBranch("feature/ABC-123-add-export")
	base := mustBase("origin", "develop")
	request := func(strategy SyncStrategy) SyncRequest {
		return SyncRequest{
			Repository: testRepository(),
			Name:       name,
			Base:       &base,
			Strategy:   strategy,
		}
	}

	t.Run("input and dependency guards", func(t *testing.T) {
		_, err := NewSynchronizer(&fakeGitRepository{}, NewService(&fakeGitRepository{}, &fakeKeyPolicy{}), nil).Sync(
			context.Background(),
			SyncRequest{Name: name, Base: &base},
		)
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = NewSynchronizer(&fakeGitRepository{}, NewService(&fakeGitRepository{}, &fakeKeyPolicy{}), nil).Sync(ctx, request(SyncCheck))
		assertProblemCode(t, err, problem.CodeOperationCancelled)

		_, err = NewSynchronizer(&fakeGitRepository{}, nil, nil).Sync(context.Background(), request(SyncCheck))
		assertProblemCode(t, err, problem.CodeInternal)
		_, err = NewSynchronizer(nil, NewService(&fakeGitRepository{}, &fakeKeyPolicy{}), nil).Sync(context.Background(), request(SyncCheck))
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("validation and branch family failures", func(t *testing.T) {
		refErr := errors.New("ref validation failed")
		git := &fakeGitRepository{validateRefErr: refErr}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).Sync(context.Background(), request(SyncCheck))
		if !errors.Is(err, refErr) {
			t.Fatalf("Sync() error = %v", err)
		}

		git = &fakeGitRepository{}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).Sync(context.Background(), SyncRequest{
			Repository: testRepository(),
			Name:       mustBranch("scratch/ABC-123-experiment"),
		})
		assertProblemCode(t, err, problem.CodeBranchFamilyInvalid)
	})

	t.Run("base and strategy errors stop before mutation", func(t *testing.T) {
		git := &fakeGitRepository{clean: true, publication: domainbranch.PublicationUnpublished, missingBase: true}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).Sync(context.Background(), request(SyncStrategy("unknown")))
		assertProblemCode(t, err, problem.CodeInvalidInput)

		hotfix := mustBranch("hotfix/ABC-123-payment-timeout")
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).Sync(context.Background(), SyncRequest{
			Repository: testRepository(),
			Name:       hotfix,
			Strategy:   SyncCheck,
		})
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})

	t.Run("repository state errors propagate", func(t *testing.T) {
		testCases := []struct {
			name string
			git  *fakeGitRepository
			code problem.Code
		}{
			{name: "dirty", git: &fakeGitRepository{clean: false}, code: problem.CodeWorktreeNotClean},
			{name: "unknown publication", git: &fakeGitRepository{clean: true, publication: domainbranch.PublicationUnknown}, code: problem.CodeBranchPublicationUnknown},
		}
		for _, testCase := range testCases {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				_, err := NewSynchronizer(testCase.git, NewService(testCase.git, &fakeKeyPolicy{}), nil).Sync(context.Background(), request(SyncCheck))
				assertProblemCode(t, err, testCase.code)
			})
		}

		for _, testCase := range []struct {
			name string
			git  *fakeGitRepository
			err  error
		}{
			{name: "worktree", git: &fakeGitRepository{worktreeCleanErr: errors.New("status failed")}, err: errors.New("placeholder")},
			{name: "fetch", git: &fakeGitRepository{clean: true, fetchErr: errors.New("fetch failed")}, err: errors.New("placeholder")},
			{name: "publication", git: &fakeGitRepository{clean: true, publicationErr: errors.New("publication failed")}, err: errors.New("placeholder")},
			{name: "missing base", git: &fakeGitRepository{clean: true, missingBaseErr: errors.New("missing-base failed")}, err: errors.New("placeholder")},
		} {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				var expected error
				switch testCase.name {
				case "worktree":
					expected = testCase.git.worktreeCleanErr
				case "fetch":
					expected = testCase.git.fetchErr
				case "publication":
					expected = testCase.git.publicationErr
				case "missing base":
					expected = testCase.git.missingBaseErr
				}
				_, err := NewSynchronizer(testCase.git, NewService(testCase.git, &fakeKeyPolicy{}), nil).Sync(context.Background(), request(SyncCheck))
				if !errors.Is(err, expected) {
					t.Fatalf("Sync() error = %v, want %v", err, expected)
				}
			})
		}
	})

	t.Run("strategy behavior and post-mutation failures", func(t *testing.T) {
		baseGit := func() *fakeGitRepository {
			return &fakeGitRepository{clean: true, publication: domainbranch.PublicationUnpublished, missingBase: true}
		}

		git := baseGit()
		result, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).Sync(context.Background(), request(SyncCheck))
		if err != nil || result.RecommendedAction != "rebase" {
			t.Fatalf("check result = (%#v, %v)", result, err)
		}

		git = baseGit()
		dryRequest := request(SyncAuto)
		dryRequest.DryRun = true
		result, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).Sync(context.Background(), dryRequest)
		if err != nil || result.RecommendedAction != "rebase" || result.Mutated {
			t.Fatalf("dry auto result = (%#v, %v)", result, err)
		}

		git = baseGit()
		git.rebaseErr = errors.New("rebase failed")
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).Sync(context.Background(), request(SyncAuto))
		if !errors.Is(err, git.rebaseErr) {
			t.Fatalf("auto rebase error = %v", err)
		}

		git = baseGit()
		dryRequest = request(SyncRebase)
		dryRequest.DryRun = true
		result, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).Sync(context.Background(), dryRequest)
		if err != nil || result.RecommendedAction != "rebase" {
			t.Fatalf("dry rebase result = (%#v, %v)", result, err)
		}

		git = baseGit()
		quality := &fakeQualityRunner{err: errors.New("quality failed")}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), quality).Sync(context.Background(), request(SyncRebase))
		if err == nil {
			t.Fatal("post-rebase quality failure was accepted")
		}

		published := &fakeGitRepository{clean: true, publication: domainbranch.PublicationPublished, missingBase: true}
		mergeRequest := request(SyncMerge)
		_, err = NewSynchronizer(published, NewService(published, &fakeKeyPolicy{}), nil).Sync(context.Background(), mergeRequest)
		assertProblemCode(t, err, problem.CodeCommitHeaderInvalid)

		message := mustMessage(t, "chore(ABC-123): merge origin/develop")
		mergeRequest.MergeMessage = &message
		mergeRequest.DryRun = true
		result, err = NewSynchronizer(published, NewService(published, &fakeKeyPolicy{}), nil).Sync(context.Background(), mergeRequest)
		if err != nil || result.RecommendedAction != "merge" || result.Mutated {
			t.Fatalf("dry merge result = (%#v, %v)", result, err)
		}

		published = &fakeGitRepository{clean: true, publication: domainbranch.PublicationPublished, missingBase: true, mergeErr: errors.New("merge failed")}
		mergeRequest = request(SyncMerge)
		mergeRequest.MergeMessage = &message
		_, err = NewSynchronizer(published, NewService(published, &fakeKeyPolicy{}), nil).Sync(context.Background(), mergeRequest)
		if !errors.Is(err, published.mergeErr) {
			t.Fatalf("merge error = %v", err)
		}

		unpublished := &fakeGitRepository{clean: true, publication: domainbranch.PublicationUnpublished, missingBase: true}
		mergeRequest = request(SyncMerge)
		mergeRequest.MergeMessage = &message
		_, err = NewSynchronizer(unpublished, NewService(unpublished, &fakeKeyPolicy{}), nil).Sync(context.Background(), mergeRequest)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})

	t.Run("remaining strategies and revalidation branches", func(t *testing.T) {
		upToDate := &fakeGitRepository{clean: true, publication: domainbranch.PublicationUnpublished}
		result, err := NewSynchronizer(upToDate, NewService(upToDate, &fakeKeyPolicy{}), nil).Sync(context.Background(), request(""))
		if err != nil || result.RecommendedAction != "none" {
			t.Fatalf("default strategy = (%#v, %v)", result, err)
		}

		upToDate = &fakeGitRepository{clean: true, publication: domainbranch.PublicationUnpublished}
		_, err = NewSynchronizer(upToDate, NewService(upToDate, &fakeKeyPolicy{}), nil).Sync(context.Background(), request(SyncRebase))
		assertProblemCode(t, err, problem.CodeRebaseNotRequired)

		published := &fakeGitRepository{clean: true, publication: domainbranch.PublicationPublished, missingBase: true}
		result, err = NewSynchronizer(published, NewService(published, &fakeKeyPolicy{}), nil).Sync(context.Background(), request(SyncCheck))
		if err != nil || result.RecommendedAction != "merge" {
			t.Fatalf("published check = (%#v, %v)", result, err)
		}

		invalid := &fakeGitRepository{clean: true, publication: domainbranch.PublicationPublished, missingBase: true}
		_, err = NewSynchronizer(invalid, NewService(invalid, &fakeKeyPolicy{}), nil).Sync(context.Background(), request(SyncStrategy("unknown")))
		assertProblemCode(t, err, problem.CodeInvalidInput)

		rebaseFailure := &fakeGitRepository{clean: true, publication: domainbranch.PublicationUnpublished, missingBase: true, rebaseErr: errors.New("rebase failed")}
		_, err = NewSynchronizer(rebaseFailure, NewService(rebaseFailure, &fakeKeyPolicy{}), nil).Sync(context.Background(), request(SyncRebase))
		if !errors.Is(err, rebaseFailure.rebaseErr) {
			t.Fatalf("explicit rebase error = %v", err)
		}

		rebaseSuccess := &fakeGitRepository{clean: true, publication: domainbranch.PublicationUnpublished, missingBase: true}
		result, err = NewSynchronizer(rebaseSuccess, NewService(rebaseSuccess, &fakeKeyPolicy{}), nil).Sync(context.Background(), request(SyncRebase))
		if err != nil || !result.Mutated || result.RecommendedAction != "rebased" {
			t.Fatalf("explicit rebase result = (%#v, %v)", result, err)
		}

		postValidationErr := errors.New("post-rebase validation failed")
		postValidation := &fakeGitRepository{
			clean:             true,
			publication:       domainbranch.PublicationUnpublished,
			missingBase:       true,
			validateRefErrors: []error{nil, postValidationErr},
		}
		_, err = NewSynchronizer(postValidation, NewService(postValidation, &fakeKeyPolicy{}), nil).Sync(context.Background(), request(SyncRebase))
		if !errors.Is(err, postValidationErr) {
			t.Fatalf("post-rebase validation error = %v", err)
		}

		mergeQuality := &fakeQualityRunner{err: errors.New("post-merge quality failed")}
		published = &fakeGitRepository{clean: true, publication: domainbranch.PublicationPublished, missingBase: true}
		message := mustMessage(t, "chore(ABC-123): merge origin/develop")
		mergeRequest := request(SyncMerge)
		mergeRequest.MergeMessage = &message
		_, err = NewSynchronizer(published, NewService(published, &fakeKeyPolicy{}), mergeQuality).Sync(context.Background(), mergeRequest)
		if err == nil {
			t.Fatal("post-merge quality failure was accepted")
		}

		skipFetch := &fakeGitRepository{clean: true, publication: domainbranch.PublicationUnpublished}
		skipRequest := request(SyncCheck)
		skipRequest.SkipFetch = true
		_, err = NewSynchronizer(skipFetch, NewService(skipFetch, &fakeKeyPolicy{}), nil).Sync(context.Background(), skipRequest)
		if err != nil || strings.Contains(strings.Join(skipFetch.calls, ","), "fetch") {
			t.Fatalf("skip fetch result = (%v, %v)", skipFetch.calls, err)
		}
	})

	t.Run("auto workflow base and post-mutation errors propagate", func(t *testing.T) {
		hotfix := mustBranch("hotfix/ABC-123-payment-timeout")
		workflowErr := errors.New("workflow base failed")
		git := &fakeGitRepository{workflowBaseErr: workflowErr}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).Sync(context.Background(), SyncRequest{
			Repository: testRepository(),
			Name:       hotfix,
			Strategy:   SyncAuto,
		})
		if !errors.Is(err, workflowErr) {
			t.Fatalf("workflow base error = %v", err)
		}

		qualityErr := errors.New("auto quality failed")
		git = &fakeGitRepository{clean: true, publication: domainbranch.PublicationUnpublished, missingBase: true}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), &fakeQualityRunner{err: qualityErr}).Sync(context.Background(), request(SyncAuto))
		if !errors.Is(err, qualityErr) {
			t.Fatalf("auto quality error = %v", err)
		}
	})
}

func TestValidatePrePush(t *testing.T) {
	t.Parallel()

	name := mustBranch("feature/ABC-123-add-export")
	base := mustBase("origin", "develop")

	t.Run("first push with missing base is blocked without rebase", func(t *testing.T) {
		git := &fakeGitRepository{
			publication: domainbranch.PublicationUnpublished,
			missingBase: true,
		}
		sync := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		_, err := sync.ValidatePrePush(context.Background(), PrePushRequest{
			Repository: testRepository(),
			Name:       name,
			Base:       &base,
		})
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
		if strings.Contains(strings.Join(git.calls, ","), "rebase") {
			t.Fatalf("pre-push must never rebase: %v", git.calls)
		}
	})

	t.Run("published branch allows validation", func(t *testing.T) {
		git := &fakeGitRepository{
			publication: domainbranch.PublicationPublished,
			missingBase: true,
		}
		sync := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		actual, err := sync.ValidatePrePush(context.Background(), PrePushRequest{
			Repository: testRepository(),
			Name:       name,
			Base:       &base,
		})
		if err != nil || actual.Publication != domainbranch.PublicationPublished || !actual.MissingBaseCommits {
			t.Fatalf("ValidatePrePush() = (%#v, %v)", actual, err)
		}
	})

	t.Run("shared line is blocked", func(t *testing.T) {
		git := &fakeGitRepository{}
		sync := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		_, err := sync.ValidatePrePush(context.Background(), PrePushRequest{
			Repository: testRepository(),
			Name:       mustBranch("develop"),
		})
		assertProblemCode(t, err, problem.CodeSharedLineMutationForbidden)
	})

	t.Run("scratch has no first-push base rule", func(t *testing.T) {
		git := &fakeGitRepository{}
		sync := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		actual, err := sync.ValidatePrePush(context.Background(), PrePushRequest{
			Repository: testRepository(),
			Name:       mustBranch("scratch/ABC-123-experiment"),
		})
		if err != nil || actual.Publication != domainbranch.PublicationUnknown {
			t.Fatalf("ValidatePrePush() = (%#v, %v)", actual, err)
		}
	})
}

func TestValidatePrePushWhiteboxPaths(t *testing.T) {
	t.Parallel()

	name := mustBranch("feature/ABC-123-add-export")
	base := mustBase("origin", "develop")
	request := func() PrePushRequest {
		return PrePushRequest{Repository: testRepository(), Name: name, Base: &base}
	}

	t.Run("input and dependency guards", func(t *testing.T) {
		invalid := request()
		invalid.Repository = port.RepositoryIdentity{}
		_, err := NewSynchronizer(&fakeGitRepository{}, NewService(&fakeGitRepository{}, &fakeKeyPolicy{}), nil).ValidatePrePush(context.Background(), invalid)
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)

		_, err = NewSynchronizer(&fakeGitRepository{}, nil, nil).ValidatePrePush(context.Background(), request())
		assertProblemCode(t, err, problem.CodeInternal)
		_, err = NewSynchronizer(nil, NewService(&fakeGitRepository{}, &fakeKeyPolicy{}), nil).ValidatePrePush(context.Background(), request())
		assertProblemCode(t, err, problem.CodeInternal)

		_, err = NewSynchronizer(&fakeGitRepository{}, NewService(&fakeGitRepository{}, &fakeKeyPolicy{}), nil).ValidatePrePush(context.Background(), PrePushRequest{
			Repository: testRepository(),
		})
		assertProblemCode(t, err, problem.CodeBranchFamilyInvalid)
	})

	t.Run("validation, base, repository, and quality failures propagate", func(t *testing.T) {
		refErr := errors.New("ref failed")
		git := &fakeGitRepository{validateRefErr: refErr}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ValidatePrePush(context.Background(), request())
		if !errors.Is(err, refErr) {
			t.Fatalf("ref error = %v", err)
		}

		hotfix := PrePushRequest{Repository: testRepository(), Name: mustBranch("hotfix/ABC-123-payment-timeout")}
		git = &fakeGitRepository{}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ValidatePrePush(context.Background(), hotfix)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)

		for _, testCase := range []struct {
			name string
			git  *fakeGitRepository
			want error
		}{
			{name: "fetch", git: &fakeGitRepository{fetchErr: errors.New("fetch failed")}},
			{name: "publication", git: &fakeGitRepository{publicationErr: errors.New("publication failed")}},
			{name: "missing-base", git: &fakeGitRepository{missingBaseErr: errors.New("missing failed")}},
		} {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				var expected error
				switch testCase.name {
				case "fetch":
					expected = testCase.git.fetchErr
				case "publication":
					expected = testCase.git.publicationErr
				case "missing-base":
					expected = testCase.git.missingBaseErr
				}
				_, err := NewSynchronizer(testCase.git, NewService(testCase.git, &fakeKeyPolicy{}), nil).ValidatePrePush(context.Background(), request())
				if !errors.Is(err, expected) {
					t.Fatalf("ValidatePrePush() error = %v, want %v", err, expected)
				}
			})
		}

		unknown := &fakeGitRepository{publication: domainbranch.PublicationUnknown}
		_, err = NewSynchronizer(unknown, NewService(unknown, &fakeKeyPolicy{}), nil).ValidatePrePush(context.Background(), request())
		assertProblemCode(t, err, problem.CodeBranchPublicationUnknown)

		qualityErr := errors.New("quality failed")
		git = &fakeGitRepository{publication: domainbranch.PublicationPublished}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), &fakeQualityRunner{err: qualityErr}).ValidatePrePush(context.Background(), request())
		if !errors.Is(err, qualityErr) {
			t.Fatalf("quality error = %v", err)
		}
	})

	t.Run("scratch quality and cross-use-case wrapper execute", func(t *testing.T) {
		scratch := mustBranch("scratch/ABC-123-experiment")
		quality := &fakeQualityRunner{}
		git := &fakeGitRepository{}
		result, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), quality).ValidatePrePush(context.Background(), PrePushRequest{
			Repository: testRepository(),
			Name:       scratch,
		})
		if err != nil || quality.calls != 1 || result.Quality.Status != port.QualityPassed {
			t.Fatalf("scratch pre-push = (%#v, %v), quality=%d", result, err, quality.calls)
		}

		qualityErr := errors.New("scratch quality failed")
		git = &fakeGitRepository{}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), &fakeQualityRunner{err: qualityErr}).ValidatePrePush(context.Background(), PrePushRequest{
			Repository: testRepository(),
			Name:       scratch,
		})
		if !errors.Is(err, qualityErr) {
			t.Fatalf("scratch quality error = %v", err)
		}

		git = &fakeGitRepository{publication: domainbranch.PublicationPublished}
		if err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ValidatePush(context.Background(), testRepository(), name, &base); err != nil {
			t.Fatalf("ValidatePush() error = %v", err)
		}
	})

	t.Run("hotfix workflow base paths remain explicit", func(t *testing.T) {
		hotfix := mustBranch("hotfix/ABC-123-payment-timeout")
		workflowErr := errors.New("workflow metadata failed")
		git := &fakeGitRepository{workflowBaseErr: workflowErr}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ValidatePrePush(context.Background(), PrePushRequest{
			Repository: testRepository(),
			Name:       hotfix,
		})
		if !errors.Is(err, workflowErr) {
			t.Fatalf("hotfix workflow base error = %v", err)
		}

		git = &fakeGitRepository{}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ValidatePrePush(context.Background(), PrePushRequest{
			Repository: testRepository(),
			Name:       hotfix,
		})
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})
}

func TestSynchronizerRebaseConflictAndResumePaths(t *testing.T) {
	name := mustBranch("feature/ABC-123-add-export")
	base := mustBase("origin", "develop")
	request := ResumeRebaseRequest{
		Repository: testRepository(),
		Name:       name,
		Base:       &base,
	}

	t.Run("classifies a paused rebase conflict", func(t *testing.T) {
		rebaseErr := errors.New("conflict")
		git := &fakeGitRepository{
			clean:           true,
			publication:     domainbranch.PublicationUnpublished,
			missingBase:     true,
			rebaseErr:       rebaseErr,
			active:          true,
			activeOperation: "rebase",
		}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).Sync(context.Background(), SyncRequest{
			Repository: testRepository(),
			Name:       name,
			Base:       &base,
			Strategy:   SyncAuto,
		})
		assertProblemCode(t, err, problem.CodeRebaseConflict)
		if !errors.Is(err, rebaseErr) {
			t.Fatalf("conflict error = %v, want %v", err, rebaseErr)
		}
	})

	t.Run("continues a resolved rebase and reruns validation", func(t *testing.T) {
		git := &fakeGitRepository{
			publication:     domainbranch.PublicationUnpublished,
			active:          true,
			activeOperation: "rebase",
		}
		quality := &fakeQualityRunner{}
		result, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), quality).ResumeRebase(context.Background(), request)
		if err != nil || !result.Mutated || result.RecommendedAction != "rebased" || result.Quality == nil || quality.calls != 1 {
			t.Fatalf("ResumeRebase() = (%#v, %v), quality=%d", result, err, quality.calls)
		}
		if !strings.Contains(strings.Join(git.calls, ","), "continue-rebase") {
			t.Fatalf("resume calls = %v", git.calls)
		}
	})

	t.Run("accepts a manually completed rebase only when the base is current", func(t *testing.T) {
		git := &fakeGitRepository{publication: domainbranch.PublicationUnpublished}
		result, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeRebase(context.Background(), request)
		if err != nil || result.RecommendedAction != "rebased" {
			t.Fatalf("manually completed ResumeRebase() = (%#v, %v)", result, err)
		}

		git = &fakeGitRepository{publication: domainbranch.PublicationUnpublished, missingBase: true}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeRebase(context.Background(), request)
		assertProblemCode(t, err, problem.CodeRebaseConflict)
	})

	t.Run("keeps retry available while Git still cannot continue", func(t *testing.T) {
		continueErr := errors.New("unresolved conflicts")
		git := &fakeGitRepository{
			publication:       domainbranch.PublicationUnpublished,
			active:            true,
			activeOperation:   "rebase",
			continueRebaseErr: continueErr,
		}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeRebase(context.Background(), request)
		assertProblemCode(t, err, problem.CodeRebaseConflict)
		if !errors.Is(err, continueErr) {
			t.Fatalf("continue error = %v, want %v", err, continueErr)
		}
	})

	t.Run("rejects invalid retry state and preserves inspection errors", func(t *testing.T) {
		git := &fakeGitRepository{
			publication:     domainbranch.PublicationUnpublished,
			active:          true,
			activeOperation: "merge",
		}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeRebase(context.Background(), request)
		assertProblemCode(t, err, problem.CodeRebaseConflict)

		activeErr := errors.New("operation inspection failed")
		git = &fakeGitRepository{
			publication:        domainbranch.PublicationUnpublished,
			activeOperationErr: activeErr,
		}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeRebase(context.Background(), request)
		if !errors.Is(err, activeErr) {
			t.Fatalf("operation inspection error = %v, want %v", err, activeErr)
		}

		git = &fakeGitRepository{publication: domainbranch.PublicationPublished}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeRebase(context.Background(), request)
		assertProblemCode(t, err, problem.CodeRebaseAfterPublishForbidden)
	})
}

func TestSynchronizerResumeRebaseWhiteboxFailurePaths(t *testing.T) {
	name := mustBranch("feature/ABC-123-add-export")
	base := mustBase("origin", "develop")
	request := ResumeRebaseRequest{Repository: testRepository(), Name: name, Base: &base}

	t.Run("guards repository context and dependencies", func(t *testing.T) {
		invalidRepository := request
		invalidRepository.Repository = port.RepositoryIdentity{}
		_, err := NewSynchronizer(&fakeGitRepository{}, NewService(&fakeGitRepository{}, &fakeKeyPolicy{}), nil).ResumeRebase(context.Background(), invalidRepository)
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		git := &fakeGitRepository{}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeRebase(ctx, request)
		assertProblemCode(t, err, problem.CodeOperationCancelled)

		_, err = NewSynchronizer(&fakeGitRepository{}, nil, nil).ResumeRebase(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInternal)
		_, err = NewSynchronizer(nil, NewService(&fakeGitRepository{}, &fakeKeyPolicy{}), nil).ResumeRebase(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("rejects invalid branches and propagates validation errors", func(t *testing.T) {
		git := &fakeGitRepository{}
		invalid := request
		invalid.Name = mustBranch("scratch/ABC-123-experiment")
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeRebase(context.Background(), invalid)
		assertProblemCode(t, err, problem.CodeBranchFamilyInvalid)

		validationErr := errors.New("validation failed")
		git = &fakeGitRepository{validateRefErr: validationErr}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeRebase(context.Background(), request)
		if !errors.Is(err, validationErr) {
			t.Fatalf("validation error = %v, want %v", err, validationErr)
		}
	})

	t.Run("propagates base and publication resolution failures", func(t *testing.T) {
		hotfix := mustBranch("hotfix/ABC-123-payment-timeout")
		hotfixRequest := ResumeRebaseRequest{Repository: testRepository(), Name: hotfix}
		workflowBaseErr := errors.New("workflow base failed")
		git := &fakeGitRepository{workflowBaseErr: workflowBaseErr}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeRebase(context.Background(), hotfixRequest)
		if !errors.Is(err, workflowBaseErr) {
			t.Fatalf("workflow base error = %v, want %v", err, workflowBaseErr)
		}

		git = &fakeGitRepository{}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeRebase(context.Background(), hotfixRequest)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)

		publicationErr := errors.New("publication failed")
		git = &fakeGitRepository{publicationErr: publicationErr}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeRebase(context.Background(), request)
		if !errors.Is(err, publicationErr) {
			t.Fatalf("publication error = %v, want %v", err, publicationErr)
		}
	})

	t.Run("propagates final comparison and validation failures", func(t *testing.T) {
		missingErr := errors.New("base comparison failed")
		git := &fakeGitRepository{
			publication:    domainbranch.PublicationUnpublished,
			missingBaseErr: missingErr,
		}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeRebase(context.Background(), request)
		if !errors.Is(err, missingErr) {
			t.Fatalf("missing-base error = %v, want %v", err, missingErr)
		}

		postValidationErr := errors.New("post-rebase validation failed")
		git = &fakeGitRepository{
			publication:       domainbranch.PublicationUnpublished,
			validateRefErrors: []error{nil, postValidationErr},
		}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeRebase(context.Background(), request)
		if !errors.Is(err, postValidationErr) {
			t.Fatalf("post-validation error = %v, want %v", err, postValidationErr)
		}

		qualityErr := errors.New("quality failed")
		git = &fakeGitRepository{publication: domainbranch.PublicationUnpublished}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), &fakeQualityRunner{err: qualityErr}).ResumeRebase(context.Background(), request)
		if !errors.Is(err, qualityErr) {
			t.Fatalf("quality error = %v, want %v", err, qualityErr)
		}
	})
}

type activeOperationAnswer struct {
	operation string
	active    bool
	err       error
}

type gitWithoutMergeResumePorts struct {
	port.GitRepository
}

type gitWithMergeInspectorOnly struct {
	port.GitRepository
	matches bool
	err     error
}

func (git gitWithMergeInspectorOnly) ActiveMergeTargetMatches(context.Context, port.RepositoryIdentity, domainbranch.TargetBase) (bool, error) {
	return git.matches, git.err
}

func TestSynchronizerSyncClassifiesMergeConflicts(t *testing.T) {
	name := mustBranch("feature/ABC-123-add-export")
	base := mustBase("origin", "develop")

	t.Run("classifies a paused merge conflict", func(t *testing.T) {
		mergeErr := errors.New("conflict")
		git := &fakeGitRepository{
			clean:           true,
			publication:     domainbranch.PublicationPublished,
			missingBase:     true,
			mergeErr:        mergeErr,
			active:          true,
			activeOperation: "merge",
		}
		message := mustMessage(t, "chore(ABC-123): merge origin/develop")
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).Sync(context.Background(), SyncRequest{
			Repository:   testRepository(),
			Name:         name,
			Base:         &base,
			Strategy:     SyncMerge,
			MergeMessage: &message,
		})
		assertProblemCode(t, err, problem.CodeMergeConflict)
		if !errors.Is(err, mergeErr) {
			t.Fatalf("conflict error = %v, want %v", err, mergeErr)
		}
	})

	t.Run("preserves merge failures without an active merge", func(t *testing.T) {
		mergeErr := errors.New("merge failed")
		git := &fakeGitRepository{
			clean:       true,
			publication: domainbranch.PublicationPublished,
			missingBase: true,
			mergeErr:    mergeErr,
		}
		message := mustMessage(t, "chore(ABC-123): merge origin/develop")
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).Sync(context.Background(), SyncRequest{
			Repository:   testRepository(),
			Name:         name,
			Base:         &base,
			Strategy:     SyncMerge,
			MergeMessage: &message,
		})
		if !errors.Is(err, mergeErr) {
			t.Fatalf("merge error = %v, want %v", err, mergeErr)
		}
	})
}

func TestSynchronizerResumeSyncDispatchAndOutcomes(t *testing.T) {
	name := mustBranch("feature/ABC-123-add-export")
	base := mustBase("origin", "develop")
	request := ResumeSyncRequest{
		Repository: testRepository(),
		Name:       name,
		Base:       &base,
	}

	t.Run("continues a resolved rebase and reruns validation and quality", func(t *testing.T) {
		git := &fakeGitRepository{
			publication:     domainbranch.PublicationUnpublished,
			active:          true,
			activeOperation: "rebase",
		}
		quality := &fakeQualityRunner{}
		result, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), quality).ResumeSync(context.Background(), request)
		if err != nil || !result.Mutated || result.RecommendedAction != "rebased" || result.Quality == nil || quality.calls != 1 {
			t.Fatalf("ResumeSync() = (%#v, %v), quality=%d", result, err, quality.calls)
		}
		if !strings.Contains(strings.Join(git.calls, ","), "continue-rebase") {
			t.Fatalf("resume calls = %v", git.calls)
		}
	})

	t.Run("continues a resolved merge with target provenance and quality", func(t *testing.T) {
		git := &fakeGitRepository{
			publication:        domainbranch.PublicationPublished,
			active:             true,
			activeOperation:    "merge",
			mergeTargetMatches: true,
		}
		quality := &fakeQualityRunner{}
		result, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), quality).ResumeSync(context.Background(), request)
		if err != nil || !result.Mutated || result.RecommendedAction != "merged" || result.Quality == nil || quality.calls != 1 {
			t.Fatalf("ResumeSync() merge = (%#v, %v), quality=%d", result, err, quality.calls)
		}
		joined := strings.Join(git.calls, ",")
		for _, expected := range []string{"unmerged-conflicts", "fetch", "target-base-exists", "merge-target-matches", "continue-merge", "missing-base"} {
			if !strings.Contains(joined, expected) {
				t.Fatalf("merge resume calls missing %q: %v", expected, git.calls)
			}
		}
	})

	t.Run("verifies an externally completed synchronization by publication state", func(t *testing.T) {
		git := &fakeGitRepository{publication: domainbranch.PublicationUnpublished}
		result, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		if err != nil || !result.Mutated || result.RecommendedAction != "rebased" {
			t.Fatalf("external rebase ResumeSync() = (%#v, %v)", result, err)
		}

		git = &fakeGitRepository{publication: domainbranch.PublicationPublished}
		result, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		if err != nil || !result.Mutated || result.RecommendedAction != "merged" {
			t.Fatalf("external merge ResumeSync() = (%#v, %v)", result, err)
		}
	})

	t.Run("defers post-mutation quality when requested", func(t *testing.T) {
		git := &fakeGitRepository{publication: domainbranch.PublicationUnpublished}
		quality := &fakeQualityRunner{}
		deferred := request
		deferred.DeferPostMutationQuality = true
		result, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), quality).ResumeSync(context.Background(), deferred)
		if err != nil || result.Quality != nil || quality.calls != 0 {
			t.Fatalf("deferred ResumeSync() = (%#v, %v), quality=%d", result, err, quality.calls)
		}
	})

	t.Run("rejects a foreign active operation", func(t *testing.T) {
		git := &fakeGitRepository{
			publication:     domainbranch.PublicationUnpublished,
			active:          true,
			activeOperation: "cherry-pick",
		}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("keeps a remaining base delta a conflict after resume", func(t *testing.T) {
		git := &fakeGitRepository{
			publication:     domainbranch.PublicationUnpublished,
			active:          true,
			activeOperation: "rebase",
			missingBase:     true,
		}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		assertProblemCode(t, err, problem.CodeRebaseConflict)

		git = &fakeGitRepository{
			publication:        domainbranch.PublicationPublished,
			active:             true,
			activeOperation:    "merge",
			mergeTargetMatches: true,
			missingBase:        true,
		}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		assertProblemCode(t, err, problem.CodeMergeConflict)
	})
}

func TestSynchronizerResumeSyncWhiteboxFailurePaths(t *testing.T) {
	name := mustBranch("feature/ABC-123-add-export")
	base := mustBase("origin", "develop")
	request := ResumeSyncRequest{Repository: testRepository(), Name: name, Base: &base}

	t.Run("guards repository context and dependencies", func(t *testing.T) {
		invalidRepository := request
		invalidRepository.Repository = port.RepositoryIdentity{}
		_, err := NewSynchronizer(&fakeGitRepository{}, NewService(&fakeGitRepository{}, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), invalidRepository)
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		git := &fakeGitRepository{}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(ctx, request)
		assertProblemCode(t, err, problem.CodeOperationCancelled)

		_, err = NewSynchronizer(&fakeGitRepository{}, nil, nil).ResumeSync(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInternal)
		_, err = NewSynchronizer(nil, NewService(&fakeGitRepository{}, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("rejects invalid branches and propagates validation errors", func(t *testing.T) {
		git := &fakeGitRepository{}
		invalid := request
		invalid.Name = mustBranch("scratch/ABC-123-experiment")
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), invalid)
		assertProblemCode(t, err, problem.CodeBranchFamilyInvalid)

		validationErr := errors.New("validation failed")
		git = &fakeGitRepository{validateRefErr: validationErr}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		if !errors.Is(err, validationErr) {
			t.Fatalf("validation error = %v, want %v", err, validationErr)
		}
	})

	t.Run("propagates base and publication resolution failures", func(t *testing.T) {
		hotfix := mustBranch("hotfix/ABC-123-payment-timeout")
		hotfixRequest := ResumeSyncRequest{Repository: testRepository(), Name: hotfix}
		workflowBaseErr := errors.New("workflow base failed")
		git := &fakeGitRepository{workflowBaseErr: workflowBaseErr}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), hotfixRequest)
		if !errors.Is(err, workflowBaseErr) {
			t.Fatalf("workflow base error = %v, want %v", err, workflowBaseErr)
		}

		git = &fakeGitRepository{}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), hotfixRequest)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)

		publicationErr := errors.New("publication failed")
		git = &fakeGitRepository{publicationErr: publicationErr}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		if !errors.Is(err, publicationErr) {
			t.Fatalf("publication error = %v, want %v", err, publicationErr)
		}

		git = &fakeGitRepository{publication: domainbranch.PublicationUnknown}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		assertProblemCode(t, err, problem.CodeBranchPublicationUnknown)
	})

	t.Run("propagates operation inspection failures", func(t *testing.T) {
		activeErr := errors.New("operation inspection failed")
		git := &fakeGitRepository{
			publication:        domainbranch.PublicationUnpublished,
			activeOperationErr: activeErr,
		}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		if !errors.Is(err, activeErr) {
			t.Fatalf("operation inspection error = %v, want %v", err, activeErr)
		}
	})

	t.Run("rejects a published branch for a paused rebase", func(t *testing.T) {
		git := &fakeGitRepository{
			publication:     domainbranch.PublicationPublished,
			active:          true,
			activeOperation: "rebase",
		}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		assertProblemCode(t, err, problem.CodeRebaseAfterPublishForbidden)
	})

	t.Run("classifies a failing rebase continuation", func(t *testing.T) {
		continueErr := errors.New("unresolved conflicts")
		git := &fakeGitRepository{
			publication:       domainbranch.PublicationUnpublished,
			active:            true,
			activeOperation:   "rebase",
			continueRebaseErr: continueErr,
		}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		assertProblemCode(t, err, problem.CodeRebaseConflict)
		if !errors.Is(err, continueErr) {
			t.Fatalf("continue error = %v, want %v", err, continueErr)
		}

		git = &fakeGitRepository{
			publication:       domainbranch.PublicationUnpublished,
			continueRebaseErr: continueErr,
			activeOperationSequence: []activeOperationAnswer{
				{operation: "rebase", active: true},
				{operation: "", active: false},
			},
		}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		if !errors.Is(err, continueErr) {
			t.Fatalf("externally resolved continue error = %v, want %v", err, continueErr)
		}
	})

	t.Run("rejects a paused merge on an unpublished branch", func(t *testing.T) {
		git := &fakeGitRepository{
			publication:     domainbranch.PublicationUnpublished,
			active:          true,
			activeOperation: "merge",
		}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})

	t.Run("fails closed while merge conflicts remain unresolved", func(t *testing.T) {
		git := &fakeGitRepository{
			publication:       domainbranch.PublicationPublished,
			active:            true,
			activeOperation:   "merge",
			unmergedConflicts: true,
		}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		assertProblemCode(t, err, problem.CodeMergeConflict)
		if strings.Contains(strings.Join(git.calls, ","), "continue-merge") {
			t.Fatalf("unresolved merge continued: %v", git.calls)
		}

		conflictsErr := errors.New("conflict inspection failed")
		git = &fakeGitRepository{
			publication:          domainbranch.PublicationPublished,
			active:               true,
			activeOperation:      "merge",
			unmergedConflictsErr: conflictsErr,
		}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		if !errors.Is(err, conflictsErr) {
			t.Fatalf("conflict inspection error = %v, want %v", err, conflictsErr)
		}
	})

	t.Run("propagates merge resume fetch and base failures", func(t *testing.T) {
		fetchErr := errors.New("fetch failed")
		git := &fakeGitRepository{
			publication:     domainbranch.PublicationPublished,
			active:          true,
			activeOperation: "merge",
			fetchErr:        fetchErr,
		}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		if !errors.Is(err, fetchErr) {
			t.Fatalf("fetch error = %v, want %v", err, fetchErr)
		}

		git = &fakeGitRepository{
			publication:       domainbranch.PublicationPublished,
			active:            true,
			activeOperation:   "merge",
			targetBaseMissing: true,
		}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)

		targetBaseErr := errors.New("target base failed")
		git = &fakeGitRepository{
			publication:     domainbranch.PublicationPublished,
			active:          true,
			activeOperation: "merge",
			targetBaseErr:   targetBaseErr,
		}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		if !errors.Is(err, targetBaseErr) {
			t.Fatalf("target base error = %v, want %v", err, targetBaseErr)
		}
	})

	t.Run("requires the merge target inspector and continuator ports", func(t *testing.T) {
		git := &fakeGitRepository{
			publication:     domainbranch.PublicationPublished,
			active:          true,
			activeOperation: "merge",
		}
		_, err := NewSynchronizer(gitWithoutMergeResumePorts{git}, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInternal)

		inspectorOnly := gitWithMergeInspectorOnly{
			GitRepository: git,
			matches:       true,
		}
		_, err = NewSynchronizer(inspectorOnly, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("propagates merge target inspection failures and rejects stale targets", func(t *testing.T) {
		matchesErr := errors.New("merge target inspection failed")
		git := &fakeGitRepository{
			publication:           domainbranch.PublicationPublished,
			active:                true,
			activeOperation:       "merge",
			mergeTargetMatchesErr: matchesErr,
		}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		if !errors.Is(err, matchesErr) {
			t.Fatalf("merge target inspection error = %v, want %v", err, matchesErr)
		}

		git = &fakeGitRepository{
			publication:     domainbranch.PublicationPublished,
			active:          true,
			activeOperation: "merge",
		}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		assertProblemCode(t, err, problem.CodeMergeConflict)
	})

	t.Run("classifies a failing merge continuation", func(t *testing.T) {
		continueErr := errors.New("unresolved merge conflicts")
		git := &fakeGitRepository{
			publication:        domainbranch.PublicationPublished,
			active:             true,
			activeOperation:    "merge",
			mergeTargetMatches: true,
			continueMergeErr:   continueErr,
		}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		assertProblemCode(t, err, problem.CodeMergeConflict)
		if !errors.Is(err, continueErr) {
			t.Fatalf("continue error = %v, want %v", err, continueErr)
		}

		git = &fakeGitRepository{
			publication:        domainbranch.PublicationPublished,
			mergeTargetMatches: true,
			continueMergeErr:   continueErr,
			activeOperationSequence: []activeOperationAnswer{
				{operation: "merge", active: true},
				{operation: "", active: false},
			},
		}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		if !errors.Is(err, continueErr) {
			t.Fatalf("externally resolved continue error = %v, want %v", err, continueErr)
		}
	})

	t.Run("propagates final comparison and validation failures", func(t *testing.T) {
		missingErr := errors.New("base comparison failed")
		git := &fakeGitRepository{
			publication:    domainbranch.PublicationUnpublished,
			missingBaseErr: missingErr,
		}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		if !errors.Is(err, missingErr) {
			t.Fatalf("missing-base error = %v, want %v", err, missingErr)
		}

		postValidationErr := errors.New("post-resume validation failed")
		git = &fakeGitRepository{
			publication:       domainbranch.PublicationUnpublished,
			validateRefErrors: []error{nil, postValidationErr},
		}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ResumeSync(context.Background(), request)
		if !errors.Is(err, postValidationErr) {
			t.Fatalf("post-validation error = %v, want %v", err, postValidationErr)
		}

		qualityErr := errors.New("quality failed")
		git = &fakeGitRepository{publication: domainbranch.PublicationUnpublished}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), &fakeQualityRunner{err: qualityErr}).ResumeSync(context.Background(), request)
		if !errors.Is(err, qualityErr) {
			t.Fatalf("quality error = %v, want %v", err, qualityErr)
		}
	})
}

func regularRequest() CreateRequest {
	return CreateRequest{
		Repository: testRepository(),
		Family:     domainbranch.FamilyFeature,
		Ticket:     mustTicket("ABC-123"),
		Slug:       mustSlug("add-export"),
	}
}

func hotfixRequest(workflowManaged bool) CreateRequest {
	base := mustBase("origin", "main")
	return CreateRequest{
		Repository:      testRepository(),
		Family:          domainbranch.FamilyHotfix,
		Ticket:          mustTicket("ABC-123"),
		Slug:            mustSlug("payment-timeout"),
		Base:            &base,
		WorkflowManaged: workflowManaged,
	}
}

func testRepository() port.RepositoryIdentity {
	return port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"}
}

func mustBranch(raw string) domainbranch.BranchName {
	name, err := domainbranch.ParseName(raw)
	if err != nil {
		panic(err)
	}
	return name
}

func mustBase(remote, raw string) domainbranch.TargetBase {
	base, err := domainbranch.NewTargetBase(remote, mustBranch(raw))
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

func mustSlug(raw string) domainbranch.Slug {
	value, err := domainbranch.ParseSlug(raw)
	if err != nil {
		panic(err)
	}
	return value
}

func mustMessage(t *testing.T, raw string) commitmsg.Message {
	t.Helper()
	value, err := commitmsg.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
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
var _ port.QualityRunner = (*fakeQualityRunner)(nil)
