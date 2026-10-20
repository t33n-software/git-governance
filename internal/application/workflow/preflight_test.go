package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

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

func (fake *installerCapableGit) InstallHooks(
	context.Context,
	port.RepositoryIdentity,
	port.HookManagerInvocation,
) (port.HookInstallation, error) {
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

// declaringBootstrapTools adds the project-tool capability to the tool fake so
// the preflight resolves the hook manager through the repository declaration.
type declaringBootstrapTools struct {
	*bootstrapTools
	declared bool
	version  string
}

func (tools *declaringBootstrapTools) ResolveProjectToolVersion(
	context.Context,
	string,
	string,
) (string, bool, error) {
	if !tools.declared {
		return "", false, nil
	}
	return tools.version, true, nil
}

// invocationRecordingGit records the hook-manager invocation the birth passes
// to the installation capability.
type invocationRecordingGit struct {
	*bootstrapGit
	invocation port.HookManagerInvocation
}

func (fake *invocationRecordingGit) InstallHooks(
	_ context.Context,
	_ port.RepositoryIdentity,
	invocation port.HookManagerInvocation,
) (port.HookInstallation, error) {
	fake.calls = append(fake.calls, "install-hooks")
	fake.invocation = invocation
	return fake.installation, fake.err
}

func TestBootstrapPreflightResolvesTheProjectHookTool(t *testing.T) {
	t.Parallel()

	t.Run("a declared project tool installs through the project form", func(t *testing.T) {
		t.Parallel()
		git := &invocationRecordingGit{bootstrapGit: newBootstrapGit()}
		git.stagedQueue = []bool{false, true}
		tools := &declaringBootstrapTools{
			bootstrapTools: newBootstrapTools(),
			declared:       true,
			version:        "lefthook version 2.2.2",
		}
		result, err := newBootstrapService(git, tools).Bootstrap(context.Background(), bootstrapRequest())
		if err != nil {
			t.Fatal(err)
		}
		if result.Record.Revision != bootstrapTestRevision {
			t.Fatalf("record = %#v", result.Record)
		}
		if git.invocation.Executable != "go" ||
			strings.Join(git.invocation.Arguments, " ") != "tool lefthook" {
			t.Fatalf("install invocation = %#v, want the project tool form", git.invocation)
		}
	})

	t.Run("an undeclared repository installs through the host form", func(t *testing.T) {
		t.Parallel()
		git := &invocationRecordingGit{bootstrapGit: newBootstrapGit()}
		git.stagedQueue = []bool{false, true}
		tools := &declaringBootstrapTools{bootstrapTools: newBootstrapTools()}
		if _, err := newBootstrapService(git, tools).Bootstrap(context.Background(), bootstrapRequest()); err != nil {
			t.Fatal(err)
		}
		if git.invocation.Executable != "lefthook" || len(git.invocation.Arguments) != 0 {
			t.Fatalf("install invocation = %#v, want the host form", git.invocation)
		}
	})
}
