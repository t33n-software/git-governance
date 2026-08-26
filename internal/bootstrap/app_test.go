package bootstrap

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

func TestCommandTreeAndVersion(t *testing.T) {
	t.Parallel()

	command := New(BuildInfo{Version: "test-version", Commit: "abc", Date: "2026-07-10"})
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{"--help"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"branch",
		"commit",
		"workflow",
		"validate",
		"auth",
		"config",
		"policy",
		"doctor",
		"completion",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("help output missing %q: %q", expected, output.String())
		}
	}

	versionCommand := New(BuildInfo{Version: "test-version", Commit: "abc", Date: "2026-07-10"})
	versionOutput := &bytes.Buffer{}
	versionCommand.SetOut(versionOutput)
	versionCommand.SetErr(versionOutput)
	versionCommand.SetArgs([]string{"--version"})
	if err := versionCommand.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(versionOutput.String(), "git-governance test-version") {
		t.Fatalf("version output = %q", versionOutput.String())
	}
}

func TestBranchListJSONContract(t *testing.T) {
	t.Parallel()

	command := New(BuildInfo{Version: "test"})
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{"--output", "json", "branch", "list"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`"schemaVersion":1`,
		`"ok":true`,
		`"operation":"branch.list"`,
		`"family":"main"`,
		`"family":"scratch"`,
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("JSON output missing %q: %q", expected, output.String())
		}
	}
}

func TestPrePushCommandValidatesHookTargetRatherThanCurrentBranch(t *testing.T) {
	t.Parallel()

	git := &prePushGit{}
	command := NewWithRuntime(BuildInfo{Version: "test"}, Runtime{
		GitFactory: func(_ time.Duration) port.GitRepository {
			return git
		},
	})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetIn(strings.NewReader(
		"HEAD " + strings.Repeat("a", 40) + " refs/heads/main " + strings.Repeat("b", 40),
	))
	command.SetArgs([]string{"--interactive", "never", "validate", "pre-push"})

	err := command.ExecuteContext(context.Background())
	assertProblemCode(t, err, problem.CodeSharedLineMutationForbidden)
	if git.currentBranchCalls != 0 {
		t.Fatalf("pre-push must not infer the target from the checked-out branch: current branch calls=%d", git.currentBranchCalls)
	}
	if git.validated.String() != "main" {
		t.Fatalf("validated target = %q, want main", git.validated.String())
	}
}

func TestDryRunCommandContractsCoverWorkflowSurfaces(t *testing.T) {
	testCases := []struct {
		name     string
		current  string
		messages []string
		args     []string
	}{
		{
			name:    "branch create",
			current: "feature/ABC-123-add-export",
			args:    []string{"branch", "create", "--family", "feature", "--key", "ABC", "--ticket", "123", "--slug", "new-export"},
		},
		{
			name:    "scratch create",
			current: "feature/ABC-123-add-export",
			args:    []string{"branch", "create", "--family", "scratch", "--key", "ABC", "--ticket", "123", "--slug", "exploration", "--base", "feature/ABC-123-add-export"},
		},
		{
			name:    "commit create",
			current: "feature/ABC-123-add-export",
			args:    []string{"commit", "create", "--type", "feat", "--subject", "add export", "--stage", "README.md"},
		},
		{
			name:    "ticket start",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "ticket", "start", "--family", "feature", "--key", "ABC", "--ticket", "123", "--slug", "new-export", "--scratch"},
		},
		{
			name:    "hotfix start",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "hotfix", "start", "--key", "ABC", "--ticket", "999", "--slug", "payment-timeout", "--affected-line", "main"},
		},
		{
			name:     "hotfix publish",
			current:  "hotfix/ABC-999-payment-timeout",
			messages: []string{"fix(ABC-999): resolve payment timeout"},
			args:     []string{"workflow", "hotfix", "publish", "--affected-line", "main"},
		},
		{
			name:    "hotfix propagation",
			current: "hotfix/ABC-999-payment-timeout",
			args:    []string{"workflow", "hotfix", "propagate", "--target-line", "develop", "--commit", strings.Repeat("a", 40)},
		},
		{
			name:    "release cut",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "release", "cut", "--version", "2.8.0"},
		},
		{
			name:    "release stabilization",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "release", "stabilize", "--release", "release/2.8.0", "--kind", "blocker", "--key", "ABC", "--ticket", "999", "--slug", "release-blocker"},
		},
		{
			name:     "release stabilization publish",
			current:  "fix/ABC-999-release-blocker",
			messages: []string{"fix(ABC-999): resolve release blocker"},
			args:     []string{"workflow", "release", "publish-stabilization", "--release", "release/2.8.0"},
		},
		{
			name:    "release promotion",
			current: "release/2.8.0",
			args:    []string{"workflow", "release", "promote", "--release", "release/2.8.0"},
		},
		{
			name:    "release backmerge",
			current: "release/2.8.0",
			args:    []string{"workflow", "release", "backmerge", "--release", "release/2.8.0"},
		},
		{
			name:    "support preparation",
			current: "main",
			args:    []string{"workflow", "release", "support", "--version", "2.8"},
		},
		{
			name:    "scratch cleanup",
			current: "scratch/ABC-123-experiment",
			args:    []string{"workflow", "cleanup", "--branch", "scratch/ABC-123-experiment"},
		},
		{
			name:    "doctor",
			current: "feature/ABC-123-add-export",
			args:    []string{"doctor"},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			git := newCommandGit(t, testCase.current, testCase.messages)
			command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
			output := &bytes.Buffer{}
			command.SetOut(output)
			command.SetErr(output)
			command.SetArgs(append([]string{"--interactive", "never", "--dry-run", "--output", "json"}, testCase.args...))
			if err := command.ExecuteContext(context.Background()); err != nil {
				t.Fatalf("Execute(%v) error = %v", testCase.args, err)
			}
			if !strings.Contains(output.String(), `"ok":true`) {
				t.Fatalf("command output = %q", output.String())
			}
		})
	}
}

func TestCompletionCommandsGenerateScripts(t *testing.T) {
	t.Parallel()

	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		shell := shell
		t.Run(shell, func(t *testing.T) {
			t.Parallel()
			command := New(BuildInfo{Version: "test"})
			output := &bytes.Buffer{}
			command.SetOut(output)
			command.SetErr(output)
			command.SetArgs([]string{"completion", shell})
			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			if output.Len() == 0 || !strings.Contains(output.String(), "git-governance") {
				t.Fatalf("completion output for %s = %q", shell, output.String())
			}
		})
	}
}

func TestNoPromptAutomationReturnsTypedMissingInput(t *testing.T) {
	t.Parallel()

	command := New(BuildInfo{Version: "test"})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{
		"--interactive", "never",
		"--repo", ".",
		"branch", "create",
		"--family", "feature",
		"--key", "ABC",
		"--ticket", "123",
		"--yes",
	})
	err := command.ExecuteContext(context.Background())
	assertProblemCode(t, err, problem.CodeInvalidInput)
}

func TestOptionAndJSONErrorContracts(t *testing.T) {
	t.Parallel()

	command := New(BuildInfo{Version: "test"})
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	command.SetOut(stdout)
	command.SetErr(stderr)
	command.SetArgs([]string{"--output", "json", "--interactive", "unknown", "branch", "list"})
	err := command.Execute()
	assertProblemCode(t, err, problem.CodeInvalidInput)
	RenderError(command, err)
	if !strings.Contains(stderr.String(), `"schemaVersion":1`) || !strings.Contains(stderr.String(), `"code":"INVALID_INPUT"`) {
		t.Fatalf("JSON error = %q", stderr.String())
	}
}

func TestNonInteractiveConfirmationGuard(t *testing.T) {
	t.Parallel()

	application := newApplication(Runtime{}, &appOptions{
		interactive: "never",
		output:      "human",
		color:       "auto",
		remote:      "origin",
		repository:  ".",
		timeout:     1,
	})
	err := application.confirmMutation(context.Background(), "Mutate", "test")
	assertProblemCode(t, err, problem.CodeInvalidInput)
}

func TestParseBaseAndFooterSpec(t *testing.T) {
	t.Parallel()

	base, err := parseBase("origin/develop", "origin")
	if err != nil || base == nil || base.String() != "origin/develop" {
		t.Fatalf("parseBase() = (%#v, %v)", base, err)
	}
	if _, err := parseBase("upstream/develop", "origin"); err == nil {
		t.Fatal("parseBase accepted another remote")
	}
	footer, err := parseFooterSpec("Refs=#123")
	if err != nil || footer.String() != "Refs: #123" {
		t.Fatalf("parseFooterSpec() = (%q, %v)", footer.String(), err)
	}
	if _, err := parseFooterSpec("invalid"); err == nil {
		t.Fatal("parseFooterSpec accepted invalid input")
	}

	application := newApplication(Runtime{}, &appOptions{
		interactive: "never",
		output:      "human",
		color:       "auto",
		remote:      "origin",
		repository:  ".",
		timeout:     1,
	})
	id, err := ticket.ParseID("ABC-123")
	if err != nil {
		t.Fatal(err)
	}
	scratchBase, err := application.resolveScratchBase(context.Background(), "feature/ABC-123-add-export", "origin", id)
	if err != nil || scratchBase == nil || scratchBase.String() != "feature/ABC-123-add-export" || scratchBase.IsRemoteTracking() {
		t.Fatalf("resolveScratchBase() = (%#v, %v)", scratchBase, err)
	}
	_, err = application.resolveScratchBase(context.Background(), "origin/feature/ABC-123-add-export", "origin", id)
	assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
}

func TestRuntimePromptAndReporterDefaults(t *testing.T) {
	t.Parallel()

	application := newApplication(Runtime{
		PromptFactory: func(bool, string) port.Prompt {
			return nil
		},
	}, &appOptions{
		interactive: "never",
		output:      "human",
		color:       "auto",
		remote:      "origin",
		repository:  ".",
		timeout:     1,
	})
	if application.runtime.KeyPolicy == nil || application.runtime.QualityFactory == nil {
		t.Fatal("runtime defaults were not supplied")
	}
}

func TestInteractiveAlwaysRequiresTerminalAndHumanOutput(t *testing.T) {
	t.Parallel()

	options := &appOptions{
		interactive: "always",
		output:      "human",
		color:       "auto",
		remote:      "origin",
		repository:  ".",
		timeout:     time.Second,
	}
	withoutTerminal := newApplication(Runtime{
		InputIsTerminal:  func() bool { return false },
		OutputIsTerminal: func() bool { return false },
	}, options)
	err := withoutTerminal.validateOptions()
	assertProblemCode(t, err, problem.CodeInvalidInput)

	withTerminal := newApplication(Runtime{
		InputIsTerminal:  func() bool { return true },
		OutputIsTerminal: func() bool { return true },
	}, &appOptions{
		interactive: "always",
		output:      "human",
		color:       "auto",
		remote:      "origin",
		repository:  ".",
		timeout:     time.Second,
	})
	if err := withTerminal.validateOptions(); err != nil {
		t.Fatalf("interactive terminal validation failed: %v", err)
	}

	jsonOutput := newApplication(Runtime{
		InputIsTerminal:  func() bool { return true },
		OutputIsTerminal: func() bool { return true },
	}, &appOptions{
		interactive: "always",
		output:      "json",
		color:       "auto",
		remote:      "origin",
		repository:  ".",
		timeout:     time.Second,
	})
	err = jsonOutput.validateOptions()
	assertProblemCode(t, err, problem.CodeInvalidInput)
}

func TestColorModeControlsReporterOutput(t *testing.T) {
	t.Parallel()

	newOptions := func(color string) *appOptions {
		return &appOptions{
			interactive: "never",
			output:      "human",
			color:       color,
			remote:      "origin",
			repository:  ".",
			timeout:     time.Second,
		}
	}
	always := newApplication(Runtime{
		OutputIsTerminal: func() bool { return false },
	}, newOptions("always"))
	if !always.colorEnabled() {
		t.Fatal("color=always must enable human output color")
	}

	never := newApplication(Runtime{
		OutputIsTerminal: func() bool { return true },
	}, newOptions("never"))
	if never.colorEnabled() {
		t.Fatal("color=never must disable human output color")
	}

	autoTTY := newApplication(Runtime{
		OutputIsTerminal: func() bool { return true },
	}, newOptions("auto"))
	if !autoTTY.colorEnabled() {
		t.Fatal("color=auto must enable color for a terminal output")
	}

	autoPipe := newApplication(Runtime{
		OutputIsTerminal: func() bool { return false },
	}, newOptions("auto"))
	if autoPipe.colorEnabled() {
		t.Fatal("color=auto must disable color for a non-terminal output")
	}
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

type prePushGit struct {
	currentBranchCalls int
	validated          branch.BranchName
}

func (git *prePushGit) Discover(context.Context, string) (port.RepositoryIdentity, error) {
	return port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"}, nil
}

func (*prePushGit) Version(context.Context) (string, error) {
	return "git version test", nil
}

func (*prePushGit) RemoteURL(context.Context, port.RepositoryIdentity) (string, error) {
	return "https://example.invalid/repo.git", nil
}

func (*prePushGit) ActiveOperation(context.Context, port.RepositoryIdentity) (string, bool, error) {
	return "", false, nil
}

func (*prePushGit) HasCommits(context.Context, port.RepositoryIdentity) (bool, error) {
	return true, nil
}

func (*prePushGit) IsWorktreeClean(context.Context, port.RepositoryIdentity) (bool, error) {
	return true, nil
}

func (git *prePushGit) CurrentBranch(context.Context, port.RepositoryIdentity) (branch.BranchName, error) {
	git.currentBranchCalls++
	return branch.ParseName("feature/ABC-123-add-export")
}

func (git *prePushGit) ValidateBranchRef(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName) error {
	git.validated = name
	return nil
}

func (*prePushGit) BranchExists(context.Context, port.RepositoryIdentity, branch.BranchName) (bool, error) {
	return false, nil
}

func (*prePushGit) OfficialBranchesForTicket(context.Context, port.RepositoryIdentity, ticket.ID) ([]branch.BranchName, error) {
	return nil, nil
}

func (*prePushGit) Fetch(context.Context, port.RepositoryIdentity) error {
	return nil
}

func (*prePushGit) TargetBaseExists(context.Context, port.RepositoryIdentity, branch.TargetBase) (bool, error) {
	return true, nil
}

func (*prePushGit) CreateBranch(context.Context, port.RepositoryIdentity, branch.BranchName, branch.TargetBase, bool) error {
	return nil
}

func (*prePushGit) StoreWorkflowBase(context.Context, port.RepositoryIdentity, branch.BranchName, branch.TargetBase) error {
	return nil
}

func (*prePushGit) ClearWorkflowBase(context.Context, port.RepositoryIdentity, branch.BranchName) error {
	return nil
}

func (*prePushGit) WorkflowBase(context.Context, port.RepositoryIdentity, branch.BranchName) (branch.TargetBase, bool, error) {
	return branch.TargetBase{}, false, nil
}

func (*prePushGit) SwitchBranch(context.Context, port.RepositoryIdentity, branch.BranchName) error {
	return nil
}

func (*prePushGit) PublicationState(context.Context, port.RepositoryIdentity, branch.BranchName) (branch.PublicationState, error) {
	return branch.PublicationUnpublished, nil
}

func (*prePushGit) HasMissingBaseCommits(context.Context, port.RepositoryIdentity, branch.TargetBase) (bool, error) {
	return false, nil
}

func (*prePushGit) CommitMessagesSince(context.Context, port.RepositoryIdentity, branch.TargetBase) ([]string, error) {
	return nil, nil
}

func (*prePushGit) Rebase(context.Context, port.RepositoryIdentity, branch.TargetBase) error {
	return nil
}

func (*prePushGit) ContinueRebase(context.Context, port.RepositoryIdentity) error {
	return nil
}

func (*prePushGit) Merge(context.Context, port.RepositoryIdentity, branch.TargetBase, commitmsg.Message) error {
	return nil
}

func (*prePushGit) SquashMerge(context.Context, port.RepositoryIdentity, branch.BranchName) error {
	return nil
}

func (*prePushGit) CherryPick(context.Context, port.RepositoryIdentity, string) error {
	return nil
}

func (*prePushGit) DeleteLocalBranch(context.Context, port.RepositoryIdentity, branch.BranchName, bool) error {
	return nil
}

func (*prePushGit) ReleaseTagsAt(context.Context, port.RepositoryIdentity, string) ([]string, error) {
	return nil, nil
}

func (*prePushGit) HasUnmergedConflicts(context.Context, port.RepositoryIdentity) (bool, error) {
	return false, nil
}

func (*prePushGit) HasStagedChanges(context.Context, port.RepositoryIdentity) (bool, error) {
	return false, nil
}

func (*prePushGit) Stage(context.Context, port.RepositoryIdentity, []string) error {
	return nil
}

func (*prePushGit) Commit(context.Context, port.RepositoryIdentity, commitmsg.Message) error {
	return nil
}

func (*prePushGit) Push(context.Context, port.RepositoryIdentity, branch.BranchName, bool) error {
	return nil
}

func (*prePushGit) InspectPushUpdate(context.Context, port.RepositoryIdentity, branch.TargetBase, string, string) (port.PushUpdateInspection, error) {
	return port.PushUpdateInspection{}, nil
}

var _ port.GitRepository = (*prePushGit)(nil)

type commandGit struct {
	current         branch.BranchName
	messages        []string
	workflowBases   map[string]branch.TargetBase
	remoteURL       string
	remoteURLErr    error
	discoverErr     error
	signingConfig   port.SigningConfiguration
	signingProofErr error
}

func newCommandGit(t *testing.T, current string, messages []string) *commandGit {
	t.Helper()
	name, err := branch.ParseName(current)
	if err != nil {
		t.Fatal(err)
	}
	return &commandGit{
		current:  name,
		messages: append([]string(nil), messages...),
		signingConfig: port.SigningConfiguration{
			SigningEnabled:         true,
			Format:                 "ssh",
			SigningKey:             "C:/keys/signing.key",
			SigningKeyReadable:     true,
			UserEmail:              "lane@example.invalid",
			AllowedSignersFile:     "C:/keys/allowed_signers",
			AllowedSignersReadable: true,
		},
	}
}

func (git *commandGit) Discover(context.Context, string) (port.RepositoryIdentity, error) {
	if git.discoverErr != nil {
		return port.RepositoryIdentity{}, git.discoverErr
	}
	return port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"}, nil
}

func (*commandGit) Version(context.Context) (string, error) {
	return "git version test", nil
}

func (git *commandGit) RemoteURL(context.Context, port.RepositoryIdentity) (string, error) {
	if git.remoteURLErr != nil {
		return "", git.remoteURLErr
	}
	if git.remoteURL != "" {
		return git.remoteURL, nil
	}
	return "https://example.invalid/repo.git", nil
}

func (*commandGit) ActiveOperation(context.Context, port.RepositoryIdentity) (string, bool, error) {
	return "", false, nil
}

func (*commandGit) CheckTransportAuthentication(context.Context, port.RepositoryIdentity) error {
	return nil
}

func (git *commandGit) SigningConfiguration(context.Context, port.RepositoryIdentity) (port.SigningConfiguration, error) {
	return git.signingConfig, nil
}

func (git *commandGit) ProveSigningCapability(context.Context, port.RepositoryIdentity, port.SigningConfiguration) error {
	return git.signingProofErr
}

func (*commandGit) HasCommits(context.Context, port.RepositoryIdentity) (bool, error) {
	return true, nil
}

func (*commandGit) IsWorktreeClean(context.Context, port.RepositoryIdentity) (bool, error) {
	return true, nil
}

func (git *commandGit) CurrentBranch(context.Context, port.RepositoryIdentity) (branch.BranchName, error) {
	return git.current, nil
}

func (*commandGit) ValidateBranchRef(context.Context, port.RepositoryIdentity, branch.BranchName) error {
	return nil
}

func (*commandGit) BranchExists(context.Context, port.RepositoryIdentity, branch.BranchName) (bool, error) {
	return false, nil
}

func (*commandGit) OfficialBranchesForTicket(context.Context, port.RepositoryIdentity, ticket.ID) ([]branch.BranchName, error) {
	return nil, nil
}

func (*commandGit) Fetch(context.Context, port.RepositoryIdentity) error {
	return nil
}

func (*commandGit) TargetBaseExists(context.Context, port.RepositoryIdentity, branch.TargetBase) (bool, error) {
	return true, nil
}

func (*commandGit) CreateBranch(context.Context, port.RepositoryIdentity, branch.BranchName, branch.TargetBase, bool) error {
	return nil
}

func (git *commandGit) StoreWorkflowBase(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName, base branch.TargetBase) error {
	if git.workflowBases == nil {
		git.workflowBases = make(map[string]branch.TargetBase)
	}
	git.workflowBases[name.String()] = base
	return nil
}

func (git *commandGit) ClearWorkflowBase(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName) error {
	delete(git.workflowBases, name.String())
	return nil
}

func (git *commandGit) WorkflowBase(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName) (branch.TargetBase, bool, error) {
	base, found := git.workflowBases[name.String()]
	return base, found, nil
}

func (*commandGit) SwitchBranch(context.Context, port.RepositoryIdentity, branch.BranchName) error {
	return nil
}

func (*commandGit) PublicationState(context.Context, port.RepositoryIdentity, branch.BranchName) (branch.PublicationState, error) {
	return branch.PublicationUnpublished, nil
}

func (*commandGit) HasMissingBaseCommits(context.Context, port.RepositoryIdentity, branch.TargetBase) (bool, error) {
	return false, nil
}

func (git *commandGit) CommitMessagesSince(context.Context, port.RepositoryIdentity, branch.TargetBase) ([]string, error) {
	return append([]string(nil), git.messages...), nil
}

func (*commandGit) Rebase(context.Context, port.RepositoryIdentity, branch.TargetBase) error {
	return nil
}

func (*commandGit) ContinueRebase(context.Context, port.RepositoryIdentity) error {
	return nil
}

func (*commandGit) Merge(context.Context, port.RepositoryIdentity, branch.TargetBase, commitmsg.Message) error {
	return nil
}

func (*commandGit) SquashMerge(context.Context, port.RepositoryIdentity, branch.BranchName) error {
	return nil
}

func (*commandGit) CherryPick(context.Context, port.RepositoryIdentity, string) error {
	return nil
}

func (*commandGit) DeleteLocalBranch(context.Context, port.RepositoryIdentity, branch.BranchName, bool) error {
	return nil
}

func (*commandGit) ReleaseTagsAt(context.Context, port.RepositoryIdentity, string) ([]string, error) {
	return []string{"v2.8.0"}, nil
}

func (*commandGit) HasUnmergedConflicts(context.Context, port.RepositoryIdentity) (bool, error) {
	return false, nil
}

func (*commandGit) HasStagedChanges(context.Context, port.RepositoryIdentity) (bool, error) {
	return true, nil
}

func (*commandGit) Stage(context.Context, port.RepositoryIdentity, []string) error {
	return nil
}

func (*commandGit) Commit(context.Context, port.RepositoryIdentity, commitmsg.Message) error {
	return nil
}

func (*commandGit) Push(context.Context, port.RepositoryIdentity, branch.BranchName, bool) error {
	return nil
}

func (git *commandGit) InspectPushUpdate(context.Context, port.RepositoryIdentity, branch.TargetBase, string, string) (port.PushUpdateInspection, error) {
	return port.PushUpdateInspection{FastForward: true, CommitMessages: append([]string(nil), git.messages...)}, nil
}

type commandStore struct {
	preferences port.Preferences
}

func (store *commandStore) Load(context.Context) (port.Preferences, error) {
	return store.preferences, nil
}

func (store *commandStore) Save(_ context.Context, preferences port.Preferences) error {
	store.preferences = preferences
	return nil
}

type commandTools struct{}

func (commandTools) Platform() (string, string) {
	return "windows", "amd64"
}

func (commandTools) Version(context.Context, string) (string, error) {
	return "lefthook version test", nil
}

func (commandTools) FileExists(string) (bool, error) {
	return true, nil
}

func commandRuntime(git port.GitRepository) Runtime {
	store := &commandStore{preferences: port.Preferences{SchemaVersion: 1}}
	return Runtime{
		GitFactory: func(time.Duration) port.GitRepository {
			return git
		},
		StoreFactory: func(string) port.PreferencesStore {
			return store
		},
		Tools: commandTools{},
	}
}

var _ port.GitRepository = (*commandGit)(nil)
var _ port.PreferencesStore = (*commandStore)(nil)
var _ port.ToolInspector = commandTools{}
