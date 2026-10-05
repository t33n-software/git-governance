package workflow

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

const bootstrapTestRevision = "0123456789abcdef0123456789abcdef01234567"

// bootstrapGit is the full-capability fake for the governed birth: the base
// Git surface plus the stage preview, reference inspection, signature
// verification, hook installation, signing inspection, and revision
// resolution capabilities.
type bootstrapGit struct {
	fakeGitRepository
	current       string
	currentErr    error
	versionErr    error
	stagedQueue   []bool
	stagedErrs    []error
	preview       []string
	previewErr    error
	anyRef        bool
	anyRefErr     error
	remoteURL     string
	remoteURLErr  error
	signing       port.SigningConfiguration
	signingErr    error
	proofErr      error
	verifyErr     error
	revisions     map[string]string
	resolveErr    error
	resolveFailOn map[string]error
	exists        map[string]bool
	existsErr     error
	stageErr      error
	commitErr     error
	createErr     error
	installErr    error
	installation  port.HookInstallation
	stagedPaths   []string
	committed     commitmsg.Message
	restored      bool
	restoreErr    error
	headCommits   int
	countHeadErr  error
	hookBoundary  bool
	hookBndryErr  error
	anyRefQueue   []bool
	anyRefErrs    []error
}

func newBootstrapGit() *bootstrapGit {
	return &bootstrapGit{
		current: "main",
		preview: []string{"README.md"},
		signing: port.SigningConfiguration{
			SigningEnabled: true,
			UserName:       "Lane Tester",
			UserEmail:      "lane@example.invalid",
		},
		revisions: map[string]string{
			"HEAD":    bootstrapTestRevision,
			"main":    bootstrapTestRevision,
			"develop": bootstrapTestRevision,
		},
		exists:       map[string]bool{"main": true, "develop": true},
		headCommits:  1,
		hookBoundary: true,
		installation: port.HookInstallation{
			Directory: filepath.Join(testRepository().Root, ".git", "hooks"),
			Hooks:     []string{"commit-msg", "pre-push"},
		},
	}
}

func (fake *bootstrapGit) CurrentBranch(context.Context, port.RepositoryIdentity) (branch.BranchName, error) {
	fake.calls = append(fake.calls, "current-branch")
	if fake.currentErr != nil {
		return branch.BranchName{}, fake.currentErr
	}
	return mustBranch(fake.current), fake.err
}

func (fake *bootstrapGit) HasStagedChanges(context.Context, port.RepositoryIdentity) (bool, error) {
	fake.calls = append(fake.calls, "staged")
	if len(fake.stagedQueue) == 0 {
		return false, nil
	}
	staged := fake.stagedQueue[0]
	fake.stagedQueue = fake.stagedQueue[1:]
	var err error
	if len(fake.stagedErrs) > 0 {
		err = fake.stagedErrs[0]
		fake.stagedErrs = fake.stagedErrs[1:]
	}
	return staged, err
}

func (fake *bootstrapGit) Stage(_ context.Context, _ port.RepositoryIdentity, paths []string) error {
	fake.calls = append(fake.calls, "stage")
	fake.stagedPaths = append([]string(nil), paths...)
	return fake.stageErr
}

func (fake *bootstrapGit) Commit(_ context.Context, _ port.RepositoryIdentity, message commitmsg.Message) error {
	fake.calls = append(fake.calls, "commit")
	fake.committed = message
	return fake.commitErr
}

func (fake *bootstrapGit) CreateBranch(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName, _ branch.TargetBase, _ bool) error {
	fake.calls = append(fake.calls, "create-branch")
	fake.createdNames = append(fake.createdNames, name)
	return fake.createErr
}

func (fake *bootstrapGit) BranchExists(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName) (bool, error) {
	fake.calls = append(fake.calls, "branch-exists")
	if fake.existsErr != nil {
		return false, fake.existsErr
	}
	return fake.exists[name.String()], fake.err
}

func (fake *bootstrapGit) RemoteURL(context.Context, port.RepositoryIdentity) (string, error) {
	fake.calls = append(fake.calls, "remote-url")
	if fake.remoteURLErr != nil {
		return "", fake.remoteURLErr
	}
	if fake.remoteURL != "" {
		return fake.remoteURL, fake.err
	}
	return "https://example.invalid/repo.git", fake.err
}

func (fake *bootstrapGit) PreviewStage(_ context.Context, _ port.RepositoryIdentity, paths []string) ([]string, error) {
	fake.calls = append(fake.calls, "preview-stage")
	if fake.previewErr != nil {
		return nil, fake.previewErr
	}
	return append([]string(nil), fake.preview...), fake.err
}

func (fake *bootstrapGit) HasAnyRef(context.Context, port.RepositoryIdentity) (bool, error) {
	fake.calls = append(fake.calls, "has-any-ref")
	if len(fake.anyRefQueue) > 0 {
		anyRef := fake.anyRefQueue[0]
		fake.anyRefQueue = fake.anyRefQueue[1:]
		var err error
		if len(fake.anyRefErrs) > 0 {
			err = fake.anyRefErrs[0]
			fake.anyRefErrs = fake.anyRefErrs[1:]
		}
		return anyRef, err
	}
	return fake.anyRef, fake.anyRefErr
}

func (fake *bootstrapGit) Version(context.Context) (string, error) {
	fake.calls = append(fake.calls, "version")
	if fake.versionErr != nil {
		return "", fake.versionErr
	}
	return "git version test", fake.err
}

func (fake *bootstrapGit) VerifyCommitSignature(context.Context, port.RepositoryIdentity, string) error {
	fake.calls = append(fake.calls, "verify-signature")
	return fake.verifyErr
}

func (fake *bootstrapGit) InstallHooks(context.Context, port.RepositoryIdentity) (port.HookInstallation, error) {
	fake.calls = append(fake.calls, "install-hooks")
	if fake.installErr != nil {
		return port.HookInstallation{}, fake.installErr
	}
	return fake.installation, fake.err
}

func (fake *bootstrapGit) SigningConfiguration(context.Context, port.RepositoryIdentity) (port.SigningConfiguration, error) {
	fake.calls = append(fake.calls, "signing-configuration")
	return fake.signing, fake.signingErr
}

func (fake *bootstrapGit) ProveSigningCapability(context.Context, port.RepositoryIdentity, port.SigningConfiguration) error {
	fake.calls = append(fake.calls, "prove-signing")
	return fake.proofErr
}

func (fake *bootstrapGit) ResolveRevision(_ context.Context, _ port.RepositoryIdentity, revision string) (string, error) {
	fake.calls = append(fake.calls, "resolve-revision")
	if err, found := fake.resolveFailOn[revision]; found {
		return "", err
	}
	if fake.resolveErr != nil {
		return "", fake.resolveErr
	}
	if resolved, found := fake.revisions[revision]; found {
		return resolved, fake.err
	}
	return "", fake.err
}

func (fake *bootstrapGit) RestoreUnbornState(context.Context, port.RepositoryIdentity) error {
	fake.calls = append(fake.calls, "restore-unborn-state")
	fake.restored = true
	return fake.restoreErr
}

func (fake *bootstrapGit) CountHeadCommits(context.Context, port.RepositoryIdentity) (int, error) {
	fake.calls = append(fake.calls, "count-head-commits")
	return fake.headCommits, fake.countHeadErr
}

func (fake *bootstrapGit) HookBoundaryPresent(context.Context, port.RepositoryIdentity) (bool, error) {
	fake.calls = append(fake.calls, "hook-boundary-present")
	return fake.hookBoundary, fake.hookBndryErr
}

// bootstrapTools fakes the host-tool inspector for the hook-manager
// resolution and the hook-file proof.
type bootstrapTools struct {
	version    string
	versionErr error
	files      map[string]bool
	failOn     map[string]error
	existsErr  error
}

func newBootstrapTools() *bootstrapTools {
	hooksDirectory := filepath.Join(testRepository().Root, ".git", "hooks")
	return &bootstrapTools{
		version: "lefthook version test",
		files: map[string]bool{
			filepath.Join(testRepository().Root, "lefthook.yml"): true,
			filepath.Join(hooksDirectory, "commit-msg"):          true,
			filepath.Join(hooksDirectory, "pre-push"):            true,
		},
	}
}

func (fake *bootstrapTools) Platform() (string, string) {
	return "windows", "amd64"
}

func (fake *bootstrapTools) Version(context.Context, string) (string, error) {
	return fake.version, fake.versionErr
}

func (fake *bootstrapTools) FileExists(path string) (bool, error) {
	if err, found := fake.failOn[path]; found {
		return false, err
	}
	if fake.existsErr != nil {
		return false, fake.existsErr
	}
	return fake.files[path], nil
}

func newBootstrapService(git port.GitRepository, tools port.ToolInspector) *BootstrapService {
	return NewBootstrapService(branchapp.NewService(git, &fakeKeyPolicy{}), git, &fakeKeyPolicy{}, tools).
		WithTicketAllocation(newTestAllocation(&fakeAllocationSurfaces{})).
		WithSigningReadiness(func(configuration port.SigningConfiguration) error {
			if !configuration.SigningEnabled {
				return problem.New(problem.Details{
					Code:        problem.CodeConfigurationUnavailable,
					Category:    problem.CategoryConfig,
					Field:       "commit.gpgsign",
					Expected:    "commit.gpgsign enabled",
					Rule:        "the governed baseline requires commit.gpgsign=true so every commit carries a signature",
					Remediation: "enable commit signing and retry the bootstrap",
				})
			}
			return nil
		}).
		WithPolicySnapshot(func() string {
			return "schemaVersion=1 keyPolicy=syntax-only commitSigning=required"
		}).
		WithClock(func() time.Time { return time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC) })
}

func bootstrapRequest() BootstrapRequest {
	return BootstrapRequest{
		Repository: testRepository(),
		Ticket:     mustTicketID("GOV-116"),
		StagePaths: []string{"."},
	}
}

func mustTicketID(raw string) ticket.ID {
	value, err := ticket.ParseID(raw)
	if err != nil {
		panic(err)
	}
	return value
}

// scopedBudgetGit wraps the bootstrap fake with the operation-timeout
// capability so tests can observe the budget the service applies to the
// genesis mutation surface.
type scopedBudgetGit struct {
	*bootstrapGit
	budgets []time.Duration
}

func (fake *scopedBudgetGit) WithOperationTimeout(timeout time.Duration) port.GitRepository {
	fake.budgets = append(fake.budgets, timeout)
	return fake
}

func TestDeriveGenesisMutationBudget(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		contentFiles int
		explicit     time.Duration
		expected     time.Duration
	}{
		{
			name:         "a small corpus keeps the base ceiling",
			contentFiles: 1,
			explicit:     0,
			expected:     genesisBaseBudget + genesisPerFileAllowance,
		},
		{
			name:         "negative counts clamp to the base ceiling",
			contentFiles: -5,
			explicit:     0,
			expected:     genesisBaseBudget,
		},
		{
			name:         "the reproduced defect corpus derives its staging allowance",
			contentFiles: 18410,
			explicit:     0,
			expected:     genesisBaseBudget + time.Duration(18410)*genesisPerFileAllowance,
		},
		{
			name:         "huge corpora cap at the bounded ceiling",
			contentFiles: 200000,
			explicit:     0,
			expected:     genesisMaxBudget,
		},
		{
			name:         "an explicit caller timeout caps the derived budget",
			contentFiles: 18410,
			explicit:     30 * time.Second,
			expected:     30 * time.Second,
		},
		{
			name:         "an explicit caller timeout above the derivation keeps the derivation",
			contentFiles: 18410,
			explicit:     time.Hour,
			expected:     genesisBaseBudget + time.Duration(18410)*genesisPerFileAllowance,
		},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveGenesisMutationBudget(testCase.contentFiles, testCase.explicit); got != testCase.expected {
				t.Fatalf("resolveGenesisMutationBudget(%d, %v) = %v, want %v", testCase.contentFiles, testCase.explicit, got, testCase.expected)
			}
		})
	}
}

func TestBootstrapScopesTheGenesisMutationBudget(t *testing.T) {
	t.Parallel()

	t.Run("the derived budget applies to the mutation surface", func(t *testing.T) {
		t.Parallel()
		git := &scopedBudgetGit{bootstrapGit: newBootstrapGit()}
		git.stagedQueue = []bool{false, true}
		if _, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), bootstrapRequest()); err != nil {
			t.Fatal(err)
		}
		expected := genesisBaseBudget + time.Duration(1)*genesisPerFileAllowance
		if len(git.budgets) != 1 || git.budgets[0] != expected {
			t.Fatalf("the genesis mutation budget = %v, want [%v]", git.budgets, expected)
		}
	})

	t.Run("an explicit caller timeout caps the applied budget", func(t *testing.T) {
		t.Parallel()
		git := &scopedBudgetGit{bootstrapGit: newBootstrapGit()}
		git.stagedQueue = []bool{false, true}
		request := bootstrapRequest()
		request.MutationTimeout = 30 * time.Second
		if _, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		if len(git.budgets) != 1 || git.budgets[0] != 30*time.Second {
			t.Fatalf("the capped genesis mutation budget = %v, want [30s]", git.budgets)
		}
	})

	t.Run("the mutation still completes through the scoped surface", func(t *testing.T) {
		t.Parallel()
		git := &scopedBudgetGit{bootstrapGit: newBootstrapGit()}
		git.stagedQueue = []bool{false, true}
		result, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), bootstrapRequest())
		if err != nil {
			t.Fatal(err)
		}
		if result.Record.Revision != bootstrapTestRevision {
			t.Fatalf("genesis revision = %q", result.Record.Revision)
		}
		for _, call := range []string{"stage", "commit", "create-branch", "install-hooks"} {
			if !strings.Contains(strings.Join(git.calls, ","), call) {
				t.Fatalf("the scoped mutation missed %q: %v", call, git.calls)
			}
		}
	})
}

func TestBootstrapDependencyGuards(t *testing.T) {
	t.Parallel()

	git := newBootstrapGit()
	tools := newBootstrapTools()
	branches := branchapp.NewService(git, &fakeKeyPolicy{})

	testCases := []struct {
		name    string
		service *BootstrapService
	}{
		{name: "no dependencies", service: NewBootstrapService(nil, nil, nil, nil)},
		{name: "missing signing readiness", service: NewBootstrapService(branches, git, &fakeKeyPolicy{}, tools).
			WithPolicySnapshot(func() string { return "snapshot" })},
		{name: "missing policy snapshot", service: NewBootstrapService(branches, git, &fakeKeyPolicy{}, tools).
			WithSigningReadiness(func(port.SigningConfiguration) error { return nil })},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := testCase.service.Bootstrap(context.Background(), bootstrapRequest())
			assertProblemCode(t, err, problem.CodeInternal)
		})
	}
}

func TestBootstrapInputValidation(t *testing.T) {
	t.Parallel()

	t.Run("repository root is required", func(t *testing.T) {
		t.Parallel()
		request := bootstrapRequest()
		request.Repository = port.RepositoryIdentity{}
		_, err := newBootstrapService(newBootstrapGit(), newBootstrapTools()).Bootstrap(context.Background(), request)
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)
	})

	t.Run("a cancelled context stops the birth", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := newBootstrapService(newBootstrapGit(), newBootstrapTools()).Bootstrap(ctx, bootstrapRequest())
		assertProblemCode(t, err, problem.CodeOperationCancelled)
	})

	t.Run("the ticket is mandatory and explicit", func(t *testing.T) {
		t.Parallel()
		request := bootstrapRequest()
		request.Ticket = ticket.ID{}
		_, err := newBootstrapService(newBootstrapGit(), newBootstrapTools()).Bootstrap(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})
}

type previewCapableGit struct{ fakeGitRepository }

func (fake *previewCapableGit) PreviewStage(context.Context, port.RepositoryIdentity, []string) ([]string, error) {
	return nil, nil
}

type refsCapableGit struct{ previewCapableGit }

func (fake *refsCapableGit) HasAnyRef(context.Context, port.RepositoryIdentity) (bool, error) {
	return false, nil
}

type verifierCapableGit struct{ refsCapableGit }

func (fake *verifierCapableGit) VerifyCommitSignature(context.Context, port.RepositoryIdentity, string) error {
	return nil
}

type installerCapableGit struct{ verifierCapableGit }

func (fake *installerCapableGit) InstallHooks(context.Context, port.RepositoryIdentity) (port.HookInstallation, error) {
	return port.HookInstallation{}, nil
}

type signerCapableGit struct{ installerCapableGit }

func (fake *signerCapableGit) SigningConfiguration(context.Context, port.RepositoryIdentity) (port.SigningConfiguration, error) {
	return port.SigningConfiguration{}, nil
}

func (fake *signerCapableGit) ProveSigningCapability(context.Context, port.RepositoryIdentity, port.SigningConfiguration) error {
	return nil
}

type revisionsCapableGit struct{ signerCapableGit }

func (fake *revisionsCapableGit) ResolveRevision(context.Context, port.RepositoryIdentity, string) (string, error) {
	return "", nil
}

func TestBootstrapCapabilityResolution(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		git  port.GitRepository
	}{
		{name: "stage preview missing", git: &fakeGitRepository{}},
		{name: "reference inspection missing", git: &previewCapableGit{}},
		{name: "signature verification missing", git: &refsCapableGit{}},
		{name: "hook installation missing", git: &verifierCapableGit{}},
		{name: "signing inspection missing", git: &installerCapableGit{}},
		{name: "revision resolution missing", git: &signerCapableGit{}},
		{name: "unborn-state restoration missing", git: &revisionsCapableGit{}},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := newBootstrapService(testCase.git, newBootstrapTools()).Bootstrap(context.Background(), bootstrapRequest())
			assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
		})
	}
}

func TestBootstrapPreflightRefusals(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		mutate func(git *bootstrapGit, tools *bootstrapTools)
		code   problem.Code
	}{
		{
			name:   "a repository with commits is already born",
			mutate: func(git *bootstrapGit, _ *bootstrapTools) { git.hasCommits = true },
			code:   problem.CodeRepositoryAlreadyBorn,
		},
		{
			name:   "a repository with references is already born",
			mutate: func(git *bootstrapGit, _ *bootstrapTools) { git.anyRef = true },
			code:   problem.CodeRepositoryAlreadyBorn,
		},
		{
			name:   "the unborn HEAD must target main",
			mutate: func(git *bootstrapGit, _ *bootstrapTools) { git.current = "develop" },
			code:   problem.CodeUnbornHeadMismatch,
		},
		{
			name: "a running git operation blocks the birth",
			mutate: func(git *bootstrapGit, _ *bootstrapTools) {
				git.active = true
				git.activeOperation = "rebase"
			},
			code: problem.CodeOperationInProgress,
		},
		{
			name:   "a pre-staged index is refused",
			mutate: func(git *bootstrapGit, _ *bootstrapTools) { git.stagedQueue = []bool{true} },
			code:   problem.CodeWorktreeNotClean,
		},
		{
			name:   "the signing gate refuses disabled signing",
			mutate: func(git *bootstrapGit, _ *bootstrapTools) { git.signing.SigningEnabled = false },
			code:   problem.CodeConfigurationUnavailable,
		},
		{
			name: "the committer identity requires user.name",
			mutate: func(git *bootstrapGit, _ *bootstrapTools) {
				git.signing.UserName = ""
			},
			code: problem.CodeConfigurationUnavailable,
		},
		{
			name:   "the hook manager must resolve",
			mutate: func(_ *bootstrapGit, tools *bootstrapTools) { tools.versionErr = errors.New("lefthook missing") },
			code:   problem.CodeExternalCommandFailed,
		},
		{
			name: "the hook configuration must exist",
			mutate: func(_ *bootstrapGit, tools *bootstrapTools) {
				tools.files = map[string]bool{}
			},
			code: problem.CodeConfigurationUnavailable,
		},
		{
			name:   "an empty stage preview is an empty content set",
			mutate: func(git *bootstrapGit, _ *bootstrapTools) { git.preview = nil },
			code:   problem.CodeInvalidInput,
		},
		{
			name:   "boundary artifacts are refused before any mutation",
			mutate: func(git *bootstrapGit, _ *bootstrapTools) { git.preview = []string{"README.md", ".env"} },
			code:   problem.CodeContentBoundaryViolation,
		},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			git := newBootstrapGit()
			tools := newBootstrapTools()
			testCase.mutate(git, tools)
			_, err := newBootstrapService(git, tools).Bootstrap(context.Background(), bootstrapRequest())
			assertProblemCode(t, err, testCase.code)
			for _, call := range git.calls {
				if call == "stage" || call == "commit" || call == "create-branch" || call == "install-hooks" {
					t.Fatalf("preflight refusal mutated the repository: calls = %v", git.calls)
				}
			}
		})
	}
}

func TestBootstrapPreflightErrorPropagation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		mutate func(git *bootstrapGit, tools *bootstrapTools)
	}{
		{name: "has commits", mutate: func(git *bootstrapGit, _ *bootstrapTools) { git.err = errors.New("git failed") }},
		{name: "reference inspection", mutate: func(git *bootstrapGit, _ *bootstrapTools) { git.anyRefErr = errors.New("git failed") }},
		{name: "current branch", mutate: func(git *bootstrapGit, _ *bootstrapTools) { git.currentErr = errors.New("git failed") }},
		{name: "active operation", mutate: func(git *bootstrapGit, _ *bootstrapTools) { git.activeErr = errors.New("git failed") }},
		{name: "staged changes", mutate: func(git *bootstrapGit, _ *bootstrapTools) {
			git.stagedQueue = []bool{false}
			git.stagedErrs = []error{errors.New("git failed")}
		}},
		{name: "git version", mutate: func(git *bootstrapGit, _ *bootstrapTools) { git.versionErr = errors.New("git failed") }},
		{name: "signing configuration", mutate: func(git *bootstrapGit, _ *bootstrapTools) { git.signingErr = errors.New("git failed") }},
		{name: "signing proof", mutate: func(git *bootstrapGit, _ *bootstrapTools) { git.proofErr = errors.New("git failed") }},
		{name: "hook configuration stat", mutate: func(_ *bootstrapGit, tools *bootstrapTools) { tools.existsErr = errors.New("stat failed") }},
		{name: "stage preview", mutate: func(git *bootstrapGit, _ *bootstrapTools) { git.previewErr = errors.New("git failed") }},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			git := newBootstrapGit()
			tools := newBootstrapTools()
			testCase.mutate(git, tools)
			if _, err := newBootstrapService(git, tools).Bootstrap(context.Background(), bootstrapRequest()); err == nil {
				t.Fatal("expected the preflight error to propagate")
			}
		})
	}
}

func TestBootstrapContentSetAndKeyPolicy(t *testing.T) {
	t.Parallel()

	t.Run("missing stage paths are refused", func(t *testing.T) {
		t.Parallel()
		request := bootstrapRequest()
		request.StagePaths = nil
		_, err := newBootstrapService(newBootstrapGit(), newBootstrapTools()).Bootstrap(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("the key policy is enforced", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		service := NewBootstrapService(branchapp.NewService(git, &fakeKeyPolicy{err: errors.New("key rejected")}), git, &fakeKeyPolicy{err: errors.New("key rejected")}, newBootstrapTools()).
			WithTicketAllocation(newTestAllocation(&fakeAllocationSurfaces{})).
			WithSigningReadiness(func(port.SigningConfiguration) error { return nil }).
			WithPolicySnapshot(func() string { return "snapshot" })
		if _, err := service.Bootstrap(context.Background(), bootstrapRequest()); err == nil ||
			!strings.Contains(err.Error(), "key rejected") {
			t.Fatalf("key policy error = %v", err)
		}
	})

	t.Run("publication without a bound remote is refused before the mutation", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.remoteURLErr = errors.New("no such remote")
		request := bootstrapRequest()
		request.Push = true
		_, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), request)
		assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
		for _, call := range git.calls {
			if call == "stage" || call == "commit" {
				t.Fatalf("the refused publication mutated the repository: calls = %v", git.calls)
			}
		}
	})

	t.Run("publication with a bound remote passes the preflight", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.stagedQueue = []bool{false, true}
		request := bootstrapRequest()
		request.Push = true
		result, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if result.Published {
			t.Fatal("Bootstrap must never publish; publication is a separate confirmed step")
		}
	})
}

func TestBootstrapDryRun(t *testing.T) {
	t.Parallel()

	git := newBootstrapGit()
	request := bootstrapRequest()
	request.DryRun = true
	request.Push = true
	result, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun || result.Published {
		t.Fatalf("dry-run result = %#v", result)
	}
	for _, call := range git.calls {
		if call == "stage" || call == "commit" || call == "create-branch" || call == "install-hooks" || call == "push" {
			t.Fatalf("dry-run mutated the repository: calls = %v", git.calls)
		}
	}
	actions := make([]string, 0, len(result.Plan))
	for _, step := range result.Plan {
		actions = append(actions, step.Action)
	}
	joined := strings.Join(actions, ",")
	if joined != "preflight,stage,commit,create,install-hooks,finalize,publish" {
		t.Fatalf("dry-run plan = %q", joined)
	}
}

func TestBootstrapHappyPath(t *testing.T) {
	t.Parallel()

	git := newBootstrapGit()
	git.stagedQueue = []bool{false, true}
	result, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), bootstrapRequest())
	if err != nil {
		t.Fatal(err)
	}

	expectedOrder := []string{
		"has-commits", "has-any-ref", "current-branch", "active-operation", "staged",
		"version", "signing-configuration", "prove-signing", "preview-stage",
		"stage", "staged", "commit", "resolve-revision", "create-branch", "install-hooks",
		"validate-ref", "branch-exists", "resolve-revision", "validate-ref", "branch-exists", "resolve-revision",
		"verify-signature",
	}
	if strings.Join(git.calls, ",") != strings.Join(expectedOrder, ",") {
		t.Fatalf("call order = %v", git.calls)
	}
	if git.committed.Header().String() != "chore(GOV-116): initialize the governed repository" {
		t.Fatalf("genesis commit = %q", git.committed.Header().String())
	}
	if len(git.createdNames) != 1 || git.createdNames[0].String() != "develop" {
		t.Fatalf("created branches = %v", git.createdNames)
	}
	record := result.Record
	if record.Ticket != "GOV-116" || record.Revision != bootstrapTestRevision ||
		len(record.Refs) != 2 || record.Refs[0] != "main" || record.Refs[1] != "develop" ||
		!record.SignatureVerified || record.Actor != "lane@example.invalid" ||
		record.PolicySnapshot != "schemaVersion=1 keyPolicy=syntax-only commitSigning=required" ||
		!record.CreatedAt.Equal(time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("record = %#v", record)
	}
	if result.DryRun || result.Published {
		t.Fatalf("result = %#v", result)
	}
}

func TestBootstrapDefaultsTheRemote(t *testing.T) {
	t.Parallel()

	git := newBootstrapGit()
	git.stagedQueue = []bool{false, true}
	request := bootstrapRequest()
	request.Repository = port.RepositoryIdentity{Root: "C:/repo"}
	result, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Record.Remote != "origin" {
		t.Fatalf("record remote = %q", result.Record.Remote)
	}
}

func TestBootstrapMutationFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		mutate func(git *bootstrapGit)
		code   problem.Code
	}{
		{name: "stage", mutate: func(git *bootstrapGit) { git.stageErr = errors.New("stage failed") }},
		{name: "staged proof", mutate: func(git *bootstrapGit) {
			git.stagedQueue = []bool{false, true}
			git.stagedErrs = []error{nil, errors.New("staged proof failed")}
		}},
		{name: "empty index after staging", mutate: func(git *bootstrapGit) { git.stagedQueue = []bool{false, false} }},
		{name: "commit", mutate: func(git *bootstrapGit) { git.commitErr = errors.New("commit failed") }},
		{name: "revision resolution", mutate: func(git *bootstrapGit) { git.resolveErr = errors.New("resolve failed") }},
		{name: "develop creation", mutate: func(git *bootstrapGit) { git.createErr = errors.New("create failed") }},
		{name: "hook installation", mutate: func(git *bootstrapGit) { git.installErr = errors.New("install failed") }},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			git := newBootstrapGit()
			git.stagedQueue = []bool{false, true}
			testCase.mutate(git)
			_, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), bootstrapRequest())
			if err == nil {
				t.Fatal("expected the mutation failure to propagate")
			}
			if testCase.name == "empty index after staging" {
				assertProblemCode(t, err, problem.CodeGitCommandFailed)
			}
		})
	}
}

func TestBootstrapCompensatesAbortedGenesisSteps(t *testing.T) {
	t.Parallel()

	t.Run("an aborted mutation restores the proven pre-state", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.stagedQueue = []bool{false, true}
		git.stageErr = errors.New("stage failed")
		_, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), bootstrapRequest())
		if err == nil {
			t.Fatal("expected the mutation failure to propagate")
		}
		if !git.restored {
			t.Fatal("the aborted mutation did not restore the proven pre-state")
		}
		if err.Error() != "stage failed" {
			t.Fatalf("the compensation altered the untyped cause: %v", err)
		}
	})

	t.Run("a typed cause carries the compensation fact in its diagnostic", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.stagedQueue = []bool{false, true}
		git.stageErr = problem.New(problem.Details{
			Code:     problem.CodeGitCommandFailed,
			Category: problem.CategoryGit,
			Field:    "index",
			Rule:     "the staging step failed",
		})
		_, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), bootstrapRequest())
		if err == nil {
			t.Fatal("expected the mutation failure to propagate")
		}
		typed, ok := problem.As(err)
		if !ok {
			t.Fatalf("the typed cause lost its contract: %v", err)
		}
		if typed.Code != problem.CodeGitCommandFailed {
			t.Fatalf("the compensation changed the failure code: %v", typed.Code)
		}
		if !strings.Contains(typed.Details.Diagnostic, "compensated back to the proven unborn pre-state") {
			t.Fatalf("the compensation fact is missing: %q", typed.Details.Diagnostic)
		}
	})

	t.Run("an existing diagnostic is preserved and extended", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.stagedQueue = []bool{false, true}
		git.stageErr = problem.New(problem.Details{
			Code:       problem.CodeGitCommandFailed,
			Category:   problem.CategoryGit,
			Field:      "index",
			Diagnostic: "the staging process died",
			Rule:       "the staging step failed",
		})
		_, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), bootstrapRequest())
		if err == nil {
			t.Fatal("expected the mutation failure to propagate")
		}
		typed, _ := problem.As(err)
		if !strings.HasPrefix(typed.Details.Diagnostic, "the staging process died; ") {
			t.Fatalf("the existing diagnostic was not preserved: %q", typed.Details.Diagnostic)
		}
	})

	t.Run("a finalizer failure is compensated", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.stagedQueue = []bool{false, true}
		git.verifyErr = errors.New("unsigned")
		_, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), bootstrapRequest())
		if err == nil {
			t.Fatal("expected the finalizer failure to propagate")
		}
		if !git.restored {
			t.Fatal("the finalizer failure did not restore the proven pre-state")
		}
	})

	t.Run("a failed compensation blocks the retry", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.stagedQueue = []bool{false, true}
		git.stageErr = errors.New("stage failed")
		git.restoreErr = errors.New("index.lock persists")
		_, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), bootstrapRequest())
		if err == nil {
			t.Fatal("expected the compensation failure to block")
		}
		typed, ok := problem.As(err)
		if !ok || typed.Code != problem.CodeInternal {
			t.Fatalf("the failed compensation lost its blocking record: %v", err)
		}
		if typed.Field != "genesis compensation" {
			t.Fatalf("compensation field = %q", typed.Field)
		}
		if !strings.Contains(typed.Details.Diagnostic, "index.lock persists") {
			t.Fatalf("the restoration diagnostic is missing: %q", typed.Details.Diagnostic)
		}
		if !strings.Contains(typed.Details.Rule, "a failed compensation leaves the partial birth in place") {
			t.Fatalf("the blocking rule is missing: %q", typed.Details.Rule)
		}
		if !git.restored {
			t.Fatal("the compensation never attempted the restoration")
		}
	})

	t.Run("a successful birth never compensates", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.stagedQueue = []bool{false, true}
		if _, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), bootstrapRequest()); err != nil {
			t.Fatal(err)
		}
		if git.restored {
			t.Fatal("the successful birth restored the pre-state")
		}
	})
}

func TestBootstrapFinalizerFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		mutate func(git *bootstrapGit, tools *bootstrapTools)
		code   problem.Code
	}{
		{
			name:   "governed validation",
			mutate: func(git *bootstrapGit, _ *bootstrapTools) { git.validateRefErr = errors.New("invalid ref") },
		},
		{
			name: "born ref existence",
			mutate: func(git *bootstrapGit, _ *bootstrapTools) {
				git.exists = map[string]bool{"main": true, "develop": false}
			},
			code: problem.CodeInternal,
		},
		{
			name: "born ref existence check failure",
			mutate: func(git *bootstrapGit, _ *bootstrapTools) {
				git.existsErr = errors.New("git failed")
			},
		},
		{
			name: "finalizer revision resolution failure",
			mutate: func(git *bootstrapGit, _ *bootstrapTools) {
				git.resolveFailOn = map[string]error{"main": errors.New("git failed")}
			},
		},
		{
			name: "shared revision equality",
			mutate: func(git *bootstrapGit, _ *bootstrapTools) {
				git.revisions = map[string]string{
					"HEAD":    bootstrapTestRevision,
					"main":    bootstrapTestRevision,
					"develop": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				}
			},
			code: problem.CodeInternal,
		},
		{
			name:   "signature verification",
			mutate: func(git *bootstrapGit, _ *bootstrapTools) { git.verifyErr = errors.New("unsigned") },
		},
		{
			name: "hook file proof",
			mutate: func(_ *bootstrapGit, tools *bootstrapTools) {
				tools.files[filepath.Join(testRepository().Root, ".git", "hooks", "pre-push")] = false
			},
			code: problem.CodeInternal,
		},
		{
			name: "hook file stat error",
			mutate: func(_ *bootstrapGit, tools *bootstrapTools) {
				tools.failOn = map[string]error{
					filepath.Join(testRepository().Root, ".git", "hooks", "commit-msg"): errors.New("stat failed"),
				}
			},
			code: problem.CodeExternalCommandFailed,
		},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			git := newBootstrapGit()
			git.stagedQueue = []bool{false, true}
			tools := newBootstrapTools()
			testCase.mutate(git, tools)
			_, err := newBootstrapService(git, tools).Bootstrap(context.Background(), bootstrapRequest())
			if err == nil {
				t.Fatal("expected the finalizer failure to propagate")
			}
			if testCase.code != "" {
				assertProblemCode(t, err, testCase.code)
			}
		})
	}
}

func TestBootstrapMessageCompositionFailure(t *testing.T) {
	t.Parallel()

	git := newBootstrapGit()
	service := newBootstrapService(git, newBootstrapTools()).
		WithPolicySnapshot(func() string { return "snapshot-with-\x01-control" })
	_, err := service.Bootstrap(context.Background(), bootstrapRequest())
	assertProblemCode(t, err, problem.CodeCommitDescriptionInvalid)
}

func TestPublishBornLines(t *testing.T) {
	t.Parallel()

	t.Run("guards dependencies and inputs", func(t *testing.T) {
		t.Parallel()
		if err := NewBootstrapService(nil, nil, nil, nil).PublishBornLines(context.Background(), testRepository()); err == nil {
			t.Fatal("expected the missing-dependency guard")
		}
		git := newBootstrapGit()
		if err := newBootstrapService(git, newBootstrapTools()).PublishBornLines(context.Background(), port.RepositoryIdentity{}); err == nil {
			t.Fatal("expected the repository guard")
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := newBootstrapService(git, newBootstrapTools()).PublishBornLines(ctx, testRepository()); err == nil {
			t.Fatal("expected the cancelled context guard")
		}
	})

	t.Run("unborn lines are refused", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.exists = map[string]bool{"main": true, "develop": false}
		err := newBootstrapService(git, newBootstrapTools()).PublishBornLines(context.Background(), testRepository())
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)
	})

	t.Run("existence failures propagate", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.existsErr = errors.New("git failed")
		if err := newBootstrapService(git, newBootstrapTools()).PublishBornLines(context.Background(), testRepository()); err == nil {
			t.Fatal("expected the existence failure to propagate")
		}
	})

	t.Run("push failures propagate", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.pushErr = errors.New("push failed")
		if err := newBootstrapService(git, newBootstrapTools()).PublishBornLines(context.Background(), testRepository()); err == nil {
			t.Fatal("expected the push failure to propagate")
		}
	})

	t.Run("pushes main before develop with upstream configuration", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		if err := newBootstrapService(git, newBootstrapTools()).PublishBornLines(context.Background(), testRepository()); err != nil {
			t.Fatal(err)
		}
		if len(git.pushed) != 2 || git.pushed[0].String() != "main" || git.pushed[1].String() != "develop" {
			t.Fatalf("pushed = %v", git.pushed)
		}
	})
}

// recoveryRefsGit carries only the reference-inspection capability.
type recoveryRefsGit struct{ fakeGitRepository }

func (fake *recoveryRefsGit) HasAnyRef(context.Context, port.RepositoryIdentity) (bool, error) {
	return false, nil
}

// resumeCommitsGit carries only the head-commit-counter capability.
type resumeCommitsGit struct{ fakeGitRepository }

func (fake *resumeCommitsGit) CountHeadCommits(context.Context, port.RepositoryIdentity) (int, error) {
	return 1, nil
}

// resumeRevisionsGit adds the revision-resolution capability.
type resumeRevisionsGit struct{ resumeCommitsGit }

func (fake *resumeRevisionsGit) ResolveRevision(context.Context, port.RepositoryIdentity, string) (string, error) {
	return bootstrapTestRevision, nil
}

// resumeVerifierGit adds the signature-verification capability.
type resumeVerifierGit struct{ resumeRevisionsGit }

func (fake *resumeVerifierGit) VerifyCommitSignature(context.Context, port.RepositoryIdentity, string) error {
	return nil
}

func TestRecoverUnbornCapabilityResolution(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		git  port.GitRepository
	}{
		{name: "reference inspection missing", git: &fakeGitRepository{}},
		{name: "unborn-state restoration missing", git: &recoveryRefsGit{}},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := newBootstrapService(testCase.git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{
				Repository: testRepository(),
			})
			assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
		})
	}
}

func TestRecoverUnbornRefusals(t *testing.T) {
	t.Parallel()

	t.Run("guards dependencies and inputs", func(t *testing.T) {
		t.Parallel()
		if _, err := NewBootstrapService(nil, nil, nil, nil).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()}); err == nil {
			t.Fatal("expected the missing-dependency guard")
		}
		if _, err := newBootstrapService(newBootstrapGit(), newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{}); err == nil {
			t.Fatal("expected the repository guard")
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := newBootstrapService(newBootstrapGit(), newBootstrapTools()).RecoverUnborn(ctx, RecoverUnbornRequest{Repository: testRepository()}); err == nil {
			t.Fatal("expected the cancelled context guard")
		}
	})

	t.Run("a born repository is never recovered", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.hasCommits = true
		_, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeRepositoryAlreadyBorn)
		if git.restored {
			t.Fatal("the refused recovery restored a state")
		}
	})

	t.Run("the unborn HEAD must target main", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.current = "develop"
		_, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeUnbornHeadMismatch)
	})

	t.Run("a running git operation blocks the recovery", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.active = true
		git.activeOperation = "rebase"
		_, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeOperationInProgress)
	})
}

func TestRecoverUnbornProofErrorPropagation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		mutate func(git *bootstrapGit)
	}{
		{name: "has commits", mutate: func(git *bootstrapGit) { git.err = errors.New("git failed") }},
		{name: "current branch", mutate: func(git *bootstrapGit) { git.currentErr = errors.New("git failed") }},
		{name: "active operation", mutate: func(git *bootstrapGit) { git.activeErr = errors.New("git failed") }},
		{name: "staged changes", mutate: func(git *bootstrapGit) {
			git.stagedQueue = []bool{false}
			git.stagedErrs = []error{errors.New("git failed")}
		}},
		{name: "reference inspection", mutate: func(git *bootstrapGit) { git.anyRefErr = errors.New("git failed") }},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			git := newBootstrapGit()
			testCase.mutate(git)
			if _, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()}); err == nil {
				t.Fatal("expected the proof error to propagate")
			}
			if git.restored {
				t.Fatal("the failed proof restored a state")
			}
		})
	}
}

func TestRecoverUnbornExecution(t *testing.T) {
	t.Parallel()

	t.Run("the dry run proves the state without restoring it", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.anyRef = true
		git.stagedQueue = []bool{true}
		result, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{
			Repository: testRepository(),
			DryRun:     true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !result.DryRun || git.restored {
			t.Fatalf("the dry run mutated the repository: result = %#v, restored = %t", result, git.restored)
		}
		actions := make([]string, 0, len(result.Plan))
		for _, step := range result.Plan {
			actions = append(actions, step.Action)
		}
		if strings.Join(actions, ",") != "prove-unborn,empty-index,remove-references,prove-pre-state" {
			t.Fatalf("dry-run plan = %q", strings.Join(actions, ","))
		}
	})

	t.Run("a clean unborn repository recovers idempotently", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		result, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		if !git.restored || result.IndexEmptied || result.ReferencesRemoved {
			t.Fatalf("the clean recovery = %#v, restored = %t", result, git.restored)
		}
	})

	t.Run("a foreign pre-staging state is restored and proven", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.anyRefQueue = []bool{true, false}
		git.stagedQueue = []bool{true, false}
		result, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		if !git.restored || !result.IndexEmptied || !result.ReferencesRemoved {
			t.Fatalf("the recovery result = %#v, restored = %t", result, git.restored)
		}
		actions := make([]string, 0, len(result.Plan))
		for _, step := range result.Plan {
			actions = append(actions, step.Action)
		}
		if strings.Join(actions, ",") != "prove-unborn,empty-index,remove-references,prove-pre-state" {
			t.Fatalf("recovery plan = %q", strings.Join(actions, ","))
		}
	})

	t.Run("a restoration failure propagates", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.stagedQueue = []bool{true}
		git.restoreErr = errors.New("index.lock persists")
		if _, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()}); err == nil {
			t.Fatal("expected the restoration failure to propagate")
		}
	})

	t.Run("a staged read-back blocks the retry", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.stagedQueue = []bool{true, true}
		_, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeInternal)
		typed, ok := problem.As(err)
		if !ok || typed.Field != "recovery read-back" {
			t.Fatalf("the read-back failure lost its blocking record: %v", err)
		}
	})

	t.Run("a reference read-back blocks the retry", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.stagedQueue = []bool{false, false}
		git.anyRef = true
		_, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("a read-back error propagates", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.stagedQueue = []bool{false, false}
		git.stagedErrs = []error{nil, errors.New("git failed")}
		_, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeInternal)
		typed, ok := problem.As(err)
		if !ok || typed.Field != "recovery read-back" {
			t.Fatalf("the read-back failure lost its blocking record: %v", err)
		}
		if !git.restored {
			t.Fatal("the restoration never ran")
		}
	})

	t.Run("a reference read-back error propagates", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.stagedQueue = []bool{false, false}
		git.anyRefQueue = []bool{false}
		git.anyRefErrs = []error{nil}
		git.anyRefErr = errors.New("git failed")
		_, err := newBootstrapService(git, newBootstrapTools()).RecoverUnborn(context.Background(), RecoverUnbornRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeInternal)
		typed, ok := problem.As(err)
		if !ok || typed.Field != "recovery read-back" {
			t.Fatalf("the read-back failure lost its blocking record: %v", err)
		}
		if !git.restored {
			t.Fatal("the restoration never ran")
		}
	})
}

func TestPublishResumeCapabilityResolution(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		git  port.GitRepository
	}{
		{name: "head commit counting missing", git: &fakeGitRepository{}},
		{name: "revision resolution missing", git: &resumeCommitsGit{}},
		{name: "signature verification missing", git: &resumeRevisionsGit{}},
		{name: "hook boundary inspection missing", git: &resumeVerifierGit{}},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := newBootstrapService(testCase.git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{
				Repository: testRepository(),
			})
			assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
		})
	}
}

func TestPublishResumeBirthTopologyProof(t *testing.T) {
	t.Parallel()

	t.Run("guards dependencies and inputs", func(t *testing.T) {
		t.Parallel()
		if _, err := NewBootstrapService(nil, nil, nil, nil).PublishResume(context.Background(), PublishResumeRequest{Repository: testRepository()}); err == nil {
			t.Fatal("expected the missing-dependency guard")
		}
		if _, err := newBootstrapService(newBootstrapGit(), newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{}); err == nil {
			t.Fatal("expected the repository guard")
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := newBootstrapService(newBootstrapGit(), newBootstrapTools()).PublishResume(ctx, PublishResumeRequest{Repository: testRepository()}); err == nil {
			t.Fatal("expected the cancelled context guard")
		}
	})

	t.Run("an unborn repository is refused", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.hasCommits = false
		_, err := newBootstrapService(git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeBirthStateInvalid)
		if len(git.pushed) != 0 {
			t.Fatalf("the refused resume pushed: %v", git.pushed)
		}
	})

	t.Run("a multi-commit repository is refused", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.hasCommits = true
		git.headCommits = 3
		_, err := newBootstrapService(git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeBirthStateInvalid)
	})

	t.Run("a missing shared line is refused", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.hasCommits = true
		git.exists = map[string]bool{"main": true, "develop": false}
		_, err := newBootstrapService(git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeBirthStateInvalid)
	})

	t.Run("a diverged shared line is refused", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.hasCommits = true
		git.revisions = map[string]string{
			"HEAD":    bootstrapTestRevision,
			"main":    bootstrapTestRevision,
			"develop": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}
		_, err := newBootstrapService(git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeBirthStateInvalid)
	})

	t.Run("an unverifiable genesis signature is refused", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.hasCommits = true
		git.verifyErr = problem.New(problem.Details{
			Code:     problem.CodeCommitSignatureRequired,
			Category: problem.CategoryGovernance,
			Field:    "commit signature",
		})
		_, err := newBootstrapService(git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeCommitSignatureRequired)
	})

	t.Run("a missing hook boundary is refused", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.hasCommits = true
		git.hookBoundary = false
		_, err := newBootstrapService(git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeBirthStateInvalid)
	})

	t.Run("an unbound remote is refused before the push", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.hasCommits = true
		git.remoteURLErr = errors.New("no such remote")
		_, err := newBootstrapService(git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
		if len(git.pushed) != 0 {
			t.Fatalf("the refused resume pushed: %v", git.pushed)
		}
	})

	t.Run("proof failures propagate without pushing", func(t *testing.T) {
		t.Parallel()
		testCases := []struct {
			name   string
			mutate func(git *bootstrapGit)
		}{
			{name: "has commits", mutate: func(git *bootstrapGit) { git.err = errors.New("git failed") }},
			{name: "head commit count", mutate: func(git *bootstrapGit) { git.countHeadErr = errors.New("git failed") }},
			{name: "head revision resolution", mutate: func(git *bootstrapGit) { git.resolveErr = errors.New("git failed") }},
			{name: "governed ref validation", mutate: func(git *bootstrapGit) { git.validateRefErr = errors.New("invalid ref") }},
			{name: "born ref existence check", mutate: func(git *bootstrapGit) { git.existsErr = errors.New("git failed") }},
			{name: "shared revision resolution", mutate: func(git *bootstrapGit) {
				git.resolveFailOn = map[string]error{"main": errors.New("git failed")}
			}},
			{name: "hook boundary inspection", mutate: func(git *bootstrapGit) { git.hookBndryErr = errors.New("git failed") }},
		}
		for _, testCase := range testCases {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				t.Parallel()
				git := newBootstrapGit()
				git.hasCommits = true
				testCase.mutate(git)
				if _, err := newBootstrapService(git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{Repository: testRepository()}); err == nil {
					t.Fatal("expected the proof failure to propagate")
				}
				if len(git.pushed) != 0 {
					t.Fatalf("the failed proof pushed: %v", git.pushed)
				}
			})
		}
	})
}

func TestPublishResumeExecution(t *testing.T) {
	t.Parallel()

	t.Run("the dry run proves the birth topology without pushing", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.hasCommits = true
		result, err := newBootstrapService(git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{
			Repository: testRepository(),
			DryRun:     true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !result.DryRun || result.Revision != bootstrapTestRevision || len(git.pushed) != 0 {
			t.Fatalf("the dry-run resume = %#v, pushed = %v", result, git.pushed)
		}
		actions := make([]string, 0, len(result.Plan))
		for _, step := range result.Plan {
			actions = append(actions, step.Action)
		}
		if strings.Join(actions, ",") != "prove-birth,publish" {
			t.Fatalf("dry-run plan = %q", strings.Join(actions, ","))
		}
	})

	t.Run("the proven birth publishes exactly the shared lines", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.hasCommits = true
		result, err := newBootstrapService(git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		if result.DryRun || result.Revision != bootstrapTestRevision {
			t.Fatalf("the resume result = %#v", result)
		}
		if len(git.pushed) != 2 || git.pushed[0].String() != "main" || git.pushed[1].String() != "develop" {
			t.Fatalf("pushed = %v", git.pushed)
		}
	})

	t.Run("a push failure propagates", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.hasCommits = true
		git.pushErr = errors.New("push failed")
		if _, err := newBootstrapService(git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{Repository: testRepository()}); err == nil {
			t.Fatal("expected the push failure to propagate")
		}
	})
}
