package workflow

import (
	"context"
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
