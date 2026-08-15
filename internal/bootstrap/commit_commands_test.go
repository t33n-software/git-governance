package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/t33n-software/git-governance/internal/adapters/terminal"
	commitapp "github.com/t33n-software/git-governance/internal/application/commit"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestCommitCommandTreeHonorsRootFlagsForDryRunPush(t *testing.T) {
	git := newCommitCommandGit(t, "feature/ABC-123-add-export")
	command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))

	output, err := executeBootstrapCommand(t, command,
		"--interactive", "never",
		"--dry-run",
		"--output", "json",
		"commit", "create",
		"--subject", "add export",
		"--stage", "README.md",
		"--push",
		"--base", "origin/develop",
	)
	if err != nil {
		t.Fatalf("commit create error = %v", err)
	}
	assertCommitOutputContains(t, output,
		`"operation":"commit.create"`,
		`"message":"feat(ABC-123): add export"`,
		`"dryRun":"true"`,
		"pre-push validation",
	)
	if len(git.stagedPaths) != 0 || len(git.committedMessages) != 0 {
		t.Fatalf("dry run mutated Git: staged=%v commits=%d", git.stagedPaths, len(git.committedMessages))
	}
}

func TestCommitCreateRetriesInteractiveSubjectAndBreakingDescription(t *testing.T) {
	promptOutput := &bytes.Buffer{}
	prompt := terminal.New(terminal.Options{
		Input: strings.NewReader(
			"\n" + // select the default feat type
				" add export\nadd export\n" +
				" clients must migrate\nclients must migrate\n",
		),
		Output: promptOutput,
	})
	git := newCommitCommandGit(t, "feature/ABC-123-add-export")
	runtime := commandRuntime(git)
	runtime.PromptFactory = func(bool, string) port.Prompt {
		return prompt
	}
	runtime.InputIsTerminal = func() bool { return true }
	runtime.OutputIsTerminal = func() bool { return true }
	command := NewWithRuntime(BuildInfo{Version: "test"}, runtime)

	output, err := executeBootstrapCommand(
		t,
		command,
		"--interactive",
		"always",
		"--dry-run",
		"commit",
		"create",
		"--breaking",
		"--stage",
		"README.md",
	)
	if err != nil {
		t.Fatalf("interactive commit create error = %v; prompt=%q; output=%q", err, promptOutput.String(), output)
	}
	if !strings.Contains(output, "Commit creation plan generated.") {
		t.Fatalf("commit output = %q", output)
	}
	for _, expected := range []string{
		"Invalid value for Commit description.",
		"Invalid value for Breaking change impact.",
		"Enter a new value.",
	} {
		if !strings.Contains(promptOutput.String(), expected) {
			t.Fatalf("interactive commit diagnostic missing %q: %q", expected, promptOutput.String())
		}
	}
}

func TestCommitCreateCommandBuildsBreakingMessageAndReportsJSON(t *testing.T) {
	git := newCommitCommandGit(t, "feature/ABC-123-add-export")
	application := newCommitCommandApplication(git, nil)
	application.options.yes = true
	application.options.output = "json"

	output, err := executeBootstrapCommand(t, newCommitCreateCommand(application),
		"--type", "feat",
		"--ticket", "ABC-123",
		"--subject", "add export",
		"--body", "Exports are now available to clients.",
		"--footer", "Refs=#123",
		"--footer", "Reviewed-by=Maintainer",
		"--breaking",
		"--breaking-description", "Clients must use the export endpoint.",
		"--stage", "cmd/export.go",
		"--base", "origin/develop",
	)
	if err != nil {
		t.Fatalf("commit create error = %v", err)
	}
	if got := git.stagedPaths; len(got) != 1 || strings.Join(got[0], ",") != "cmd/export.go" {
		t.Fatalf("staged paths = %v", got)
	}
	if len(git.committedMessages) != 1 {
		t.Fatalf("committed messages = %d, want 1", len(git.committedMessages))
	}
	message := git.committedMessages[0]
	if message.Header().String() != "feat(ABC-123)!: add export" {
		t.Fatalf("commit header = %q", message.Header().String())
	}
	if message.Body() != "Exports are now available to clients." {
		t.Fatalf("commit body = %q", message.Body())
	}
	if got := []string{
		message.Footers()[0].String(),
		message.Footers()[1].String(),
		message.Footers()[2].String(),
	}; strings.Join(got, "|") != "Refs: #123|Reviewed-by: Maintainer|BREAKING CHANGE: Clients must use the export endpoint." {
		t.Fatalf("commit footers = %v", got)
	}
	assertCommitOutputContains(t, output,
		`"operation":"commit.create"`,
		`"summary":"Commit created."`,
		`"committed":"true"`,
		`"pushed":"false"`,
		`"dryRun":"false"`,
	)
}

func TestCommitCreateCommandFailureContracts(t *testing.T) {
	discoverErr := errors.New("repository discovery failed")
	currentErr := errors.New("current branch failed")

	testCases := []struct {
		name      string
		args      []string
		configure func(*commitCommandGit, *application)
		wantCode  problem.Code
		wantErr   error
	}{
		{
			name: "preserves repository discovery errors",
			configure: func(git *commitCommandGit, _ *application) {
				git.discoverErr = discoverErr
			},
			wantErr: discoverErr,
		},
		{
			name: "preserves current branch errors",
			configure: func(git *commitCommandGit, _ *application) {
				git.currentErr = currentErr
			},
			wantErr: currentErr,
		},
		{
			name:     "rejects invalid explicit tickets",
			args:     []string{"--ticket", "ABC123"},
			wantCode: problem.CodeTicketIDInvalid,
		},
		{
			name:     "rejects unsupported commit types",
			args:     []string{"--type", "feature"},
			wantCode: problem.CodeCommitTypeInvalid,
		},
		{
			name:     "requires a subject without a prompt",
			args:     []string{"--type", "feat"},
			wantCode: problem.CodeInvalidInput,
		},
		{
			name:     "validates the constructed header",
			args:     []string{"--subject", " "},
			wantCode: problem.CodeCommitDescriptionInvalid,
		},
		{
			name:     "rejects malformed footer flags",
			args:     []string{"--subject", "add export", "--footer", "invalid"},
			wantCode: problem.CodeCommitDescriptionInvalid,
		},
		{
			name:     "requires a breaking change description",
			args:     []string{"--subject", "add export", "--breaking"},
			wantCode: problem.CodeInvalidInput,
		},
		{
			name:     "validates explicit breaking descriptions",
			args:     []string{"--subject", "add export", "--breaking", "--breaking-description", " "},
			wantCode: problem.CodeCommitDescriptionInvalid,
		},
		{
			name:     "requires a reverted commit reference",
			args:     []string{"--type", "revert", "--subject", "revert export"},
			wantCode: problem.CodeCommitDescriptionInvalid,
		},
		{
			name:     "accepts bases only from the selected remote",
			args:     []string{"--subject", "add export", "--base", "upstream/main"},
			wantCode: problem.CodeBranchBaseInvalid,
		},
		{
			name: "honors a declined confirmation",
			args: []string{"--type", "feat", "--subject", "add export"},
			configure: func(_ *commitCommandGit, application *application) {
				prompt := &commitCommandPrompt{
					confirms: []commitConfirmationReply{{value: false}},
				}
				enableCommitPrompt(application, prompt)
				application.options.yes = false
			},
			wantCode: problem.CodeOperationCancelled,
		},
		{
			name: "returns commit service failures",
			args: []string{"--type", "feat", "--subject", "add export"},
			configure: func(git *commitCommandGit, _ *application) {
				git.staged = false
			},
			wantCode: problem.CodeInvalidInput,
		},
		{
			name:     "uses an explicit ticket before the branch ticket",
			args:     []string{"--ticket", "XYZ-999", "--type", "feat", "--subject", "add export"},
			wantCode: problem.CodeCommitTicketMismatch,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			git := newCommitCommandGit(t, "feature/ABC-123-add-export")
			application := newCommitCommandApplication(git, nil)
			application.options.yes = true
			if testCase.configure != nil {
				testCase.configure(git, application)
			}

			_, err := executeBootstrapCommand(t, newCommitCreateCommand(application), testCase.args...)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("commit create error = %v, want %v", err, testCase.wantErr)
				}
			} else {
				assertProblemCode(t, err, testCase.wantCode)
			}
			if len(git.committedMessages) != 0 {
				t.Fatalf("failed creation committed %d messages", len(git.committedMessages))
			}
		})
	}
}

func TestCommitValidateCommandParsesMessagesAndRespectsFlagPrecedence(t *testing.T) {
	messagePath := filepath.Join(t.TempDir(), "message.txt")
	messageFromFile := "feat(ABC-123)!: add export\n\nExports are available to clients.\n\nBREAKING CHANGE: Clients must use the export endpoint."
	if err := os.WriteFile(messagePath, []byte(messageFromFile), 0o600); err != nil {
		t.Fatal(err)
	}

	git := newCommitCommandGit(t, "main")
	application := newCommitCommandApplication(git, nil)
	application.options.output = "json"
	output, err := executeBootstrapCommand(t, newCommitValidateCommand(application),
		"--branch", "feature/ABC-123-add-export",
		"--message-file", messagePath,
	)
	if err != nil {
		t.Fatalf("message-file validation error = %v", err)
	}
	if git.currentCalls != 0 {
		t.Fatalf("explicit --branch called CurrentBranch %d times", git.currentCalls)
	}
	if len(git.validatedBranches) != 1 || git.validatedBranches[0].String() != "feature/ABC-123-add-export" {
		t.Fatalf("validated branches = %v", git.validatedBranches)
	}
	assertCommitOutputContains(t, output,
		`"operation":"commit.validate"`,
		`"message":"feat(ABC-123)!: add export"`,
		`"breaking":"true"`,
	)

	precedenceGit := newCommitCommandGit(t, "feature/ABC-123-add-export")
	precedenceApplication := newCommitCommandApplication(precedenceGit, nil)
	precedenceOutput, err := executeBootstrapCommand(t, newCommitValidateCommand(precedenceApplication),
		"--message-file", filepath.Join(t.TempDir(), "does-not-exist.txt"),
		"--message", "feat(ABC-123): use inline input",
	)
	if err != nil {
		t.Fatalf("inline message validation error = %v", err)
	}
	if precedenceGit.currentCalls != 1 {
		t.Fatalf("implicit branch calls = %d, want 1", precedenceGit.currentCalls)
	}
	assertCommitOutputContains(t, precedenceOutput, "Commit message is valid.")
}

func TestCommitValidateCommandFailureContracts(t *testing.T) {
	discoverErr := errors.New("repository discovery failed")
	currentErr := errors.New("current branch failed")
	validateErr := errors.New("branch validation failed")

	testCases := []struct {
		name      string
		current   string
		args      []string
		configure func(*commitCommandGit)
		wantCode  problem.Code
		wantErr   error
	}{
		{
			name: "preserves repository discovery errors",
			configure: func(git *commitCommandGit) {
				git.discoverErr = discoverErr
			},
			wantErr: discoverErr,
		},
		{
			name: "preserves current branch errors",
			configure: func(git *commitCommandGit) {
				git.currentErr = currentErr
			},
			wantErr: currentErr,
		},
		{
			name:     "rejects invalid explicit branches",
			args:     []string{"--branch", "not-a-branch"},
			wantCode: problem.CodeBranchNameInvalid,
		},
		{
			name:     "requires a message source",
			args:     []string{"--branch", "feature/ABC-123-add-export"},
			wantCode: problem.CodeInvalidInput,
		},
		{
			name:     "rejects malformed messages",
			args:     []string{"--branch", "feature/ABC-123-add-export", "--message", "not a commit message"},
			wantCode: problem.CodeCommitHeaderInvalid,
		},
		{
			name:     "returns commit validation errors",
			current:  "main",
			args:     []string{"--message", "feat(ABC-123): add export"},
			wantCode: problem.CodeSharedLineMutationForbidden,
		},
		{
			name: "preserves Git reference validation errors",
			args: []string{"--branch", "feature/ABC-123-add-export", "--message", "feat(ABC-123): add export"},
			configure: func(git *commitCommandGit) {
				git.validateErr = validateErr
			},
			wantErr: validateErr,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			current := testCase.current
			if current == "" {
				current = "feature/ABC-123-add-export"
			}
			git := newCommitCommandGit(t, current)
			if testCase.configure != nil {
				testCase.configure(git)
			}
			application := newCommitCommandApplication(git, nil)

			_, err := executeBootstrapCommand(t, newCommitValidateCommand(application), testCase.args...)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("commit validate error = %v, want %v", err, testCase.wantErr)
				}
			} else {
				assertProblemCode(t, err, testCase.wantCode)
			}
		})
	}
}

func TestResolveCommitTicketContracts(t *testing.T) {
	current, err := branch.ParseName("feature/ABC-123-add-export")
	if err != nil {
		t.Fatal(err)
	}
	main, err := branch.ParseName("main")
	if err != nil {
		t.Fatal(err)
	}

	fromBranch, err := resolveCommitTicket(current, "")
	if err != nil || fromBranch.String() != "ABC-123" {
		t.Fatalf("branch ticket = (%q, %v)", fromBranch.String(), err)
	}
	fromMatchingFlag, err := resolveCommitTicket(current, "ABC-123")
	if err != nil || fromMatchingFlag.String() != "ABC-123" {
		t.Fatalf("matching explicit ticket = (%q, %v)", fromMatchingFlag.String(), err)
	}
	if _, err := resolveCommitTicket(current, "ABC123"); err == nil {
		t.Fatal("invalid explicit ticket was accepted")
	}
	_, err = resolveCommitTicket(current, "XYZ-456")
	assertProblemCode(t, err, problem.CodeCommitTicketMismatch)
	_, err = resolveCommitTicket(main, "")
	assertProblemCode(t, err, problem.CodeSharedLineMutationForbidden)
}

func TestResolveStructuredCommitMessageContracts(t *testing.T) {
	feature, err := branch.ParseName("feature/ABC-123-add-export")
	if err != nil {
		t.Fatal(err)
	}

	noninteractive := newCommitCommandApplication(newCommitCommandGit(t, feature.String()), nil)
	message, err := noninteractive.resolveCommitMessage(context.Background(), commitMessageInput{
		Branch:           feature,
		Family:           "fix",
		Description:      "correct export validation",
		RequireFamily:    true,
		DescriptionLabel: "Commit description",
		Operation:        "this commit",
	})
	if err != nil || message.Header().String() != "fix(ABC-123): correct export validation" {
		t.Fatalf("resolveCommitMessage() = (%q, %v)", message.String(), err)
	}

	_, err = noninteractive.resolveCommitMessage(context.Background(), commitMessageInput{
		Branch:        feature,
		Description:   "add export",
		RequireFamily: true,
	})
	assertProblemCode(t, err, problem.CodeInvalidInput)

	_, err = noninteractive.resolveCommitMessage(context.Background(), commitMessageInput{
		Branch:           feature,
		Family:           "feature",
		Description:      "add export",
		RequireFamily:    true,
		DescriptionLabel: "Commit description",
	})
	assertProblemCode(t, err, problem.CodeCommitTypeInvalid)

	prompt := &commitCommandPrompt{
		selects: []commitStringReply{{value: "docs"}},
		inputs:  []commitStringReply{{value: "document export workflow"}},
	}
	interactive := newCommitCommandApplication(newCommitCommandGit(t, feature.String()), prompt)
	enableCommitPrompt(interactive, prompt)
	message, err = interactive.resolveCommitMessage(context.Background(), commitMessageInput{
		Branch:           feature,
		RequireFamily:    true,
		DescriptionLabel: "Commit description",
		Operation:        "this commit",
	})
	if err != nil || message.Header().String() != "docs(ABC-123): document export workflow" {
		t.Fatalf("interactive resolveCommitMessage() = (%q, %v)", message.String(), err)
	}
	if len(prompt.selectRequests) != 1 || len(prompt.inputRequests) != 1 {
		t.Fatalf("commit prompts = selects:%#v inputs:%#v", prompt.selectRequests, prompt.inputRequests)
	}
	selection := prompt.selectRequests[0]
	if selection.Label != "Commit family" || selection.Default != "feat" || len(selection.Options) != len(commitapp.Families()) {
		t.Fatalf("commit family selection = %#v", selection)
	}
	if selection.Options[0].Value != "build" || selection.Options[4].Label != "Feature" {
		t.Fatalf("commit family options = %#v", selection.Options)
	}
	if prompt.inputRequests[0].Label != "Commit description" ||
		!strings.Contains(prompt.inputRequests[0].Description, "ABC-123") ||
		!strings.Contains(prompt.inputRequests[0].Description, "docs(ABC-123): <description>") {
		t.Fatalf("commit description prompt = %#v", prompt.inputRequests[0])
	}
}

func TestCommitCommandFooterAndReportHelpers(t *testing.T) {
	footers, err := parseFooterSpecs([]string{"Refs=#123", "Reviewed-by=Maintainer"})
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{footers[0].String(), footers[1].String()}; strings.Join(got, "|") != "Refs: #123|Reviewed-by: Maintainer" {
		t.Fatalf("footers = %v", got)
	}
	if _, err := parseFooterSpecs([]string{"Refs=#123", "invalid"}); err == nil {
		t.Fatal("invalid footer specifications were accepted")
	}

	for _, testCase := range []struct {
		result commitapp.CreateResult
		want   string
	}{
		{result: commitapp.CreateResult{DryRun: true}, want: "Commit creation plan generated."},
		{result: commitapp.CreateResult{Pushed: true}, want: "Commit created and pushed."},
		{result: commitapp.CreateResult{}, want: "Commit created."},
	} {
		if got := commitCreationSummary(testCase.result); got != testCase.want {
			t.Fatalf("commitCreationSummary(%#v) = %q, want %q", testCase.result, got, testCase.want)
		}
	}

	plan := []commitapp.PlanStep{
		{Action: "stage", Detail: "stage requested paths"},
		{Action: "commit", Detail: "feat(ABC-123): add export"},
	}
	if got := commitPlanText(plan); got != "stage: stage requested paths; commit: feat(ABC-123): add export" {
		t.Fatalf("commitPlanText() = %q", got)
	}
	if got := commitPlanText(nil); got != "" {
		t.Fatalf("empty commit plan = %q", got)
	}
}

func executeBootstrapCommand(t *testing.T, command *cobra.Command, arguments ...string) (string, error) {
	t.Helper()
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs(arguments)
	err := command.ExecuteContext(context.Background())
	return output.String(), err
}

func assertCommitOutputContains(t *testing.T, output string, expected ...string) {
	t.Helper()
	for _, value := range expected {
		if !strings.Contains(output, value) {
			t.Fatalf("command output missing %q: %q", value, output)
		}
	}
}

func newCommitCommandApplication(git port.GitRepository, prompt port.Prompt) *application {
	runtime := commandRuntime(git)
	runtime.PromptFactory = func(bool, string) port.Prompt {
		return prompt
	}
	runtime.InputIsTerminal = func() bool {
		return prompt != nil
	}
	runtime.OutputIsTerminal = func() bool {
		return prompt != nil
	}
	return newApplication(runtime, &appOptions{
		interactive: "never",
		output:      "human",
		color:       "never",
		remote:      "origin",
		repository:  "C:/repo",
		timeout:     time.Second,
	})
}

func enableCommitPrompt(application *application, prompt port.Prompt) {
	application.options.interactive = "auto"
	application.runtime.PromptFactory = func(bool, string) port.Prompt {
		return prompt
	}
	application.runtime.InputIsTerminal = func() bool {
		return true
	}
	application.runtime.OutputIsTerminal = func() bool {
		return true
	}
}

type commitCommandGit struct {
	*commandGit

	discoverErr error
	currentErr  error
	validateErr error
	stageErr    error
	stagedErr   error
	commitErr   error
	staged      bool

	discoverCalls     int
	currentCalls      int
	validatedBranches []branch.BranchName
	stagedPaths       [][]string
	committedMessages []commitmsg.Message
}

func newCommitCommandGit(t *testing.T, current string) *commitCommandGit {
	t.Helper()
	return &commitCommandGit{
		commandGit: newCommandGit(t, current, nil),
		staged:     true,
	}
}

func (git *commitCommandGit) Discover(ctx context.Context, directory string) (port.RepositoryIdentity, error) {
	git.discoverCalls++
	if git.discoverErr != nil {
		return port.RepositoryIdentity{}, git.discoverErr
	}
	return git.commandGit.Discover(ctx, directory)
}

func (git *commitCommandGit) CurrentBranch(_ context.Context, _ port.RepositoryIdentity) (branch.BranchName, error) {
	git.currentCalls++
	if git.currentErr != nil {
		return branch.BranchName{}, git.currentErr
	}
	return git.current, nil
}

func (git *commitCommandGit) ValidateBranchRef(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName) error {
	git.validatedBranches = append(git.validatedBranches, name)
	return git.validateErr
}

func (git *commitCommandGit) Stage(_ context.Context, _ port.RepositoryIdentity, paths []string) error {
	if git.stageErr != nil {
		return git.stageErr
	}
	git.stagedPaths = append(git.stagedPaths, append([]string(nil), paths...))
	return nil
}

func (git *commitCommandGit) HasStagedChanges(context.Context, port.RepositoryIdentity) (bool, error) {
	if git.stagedErr != nil {
		return false, git.stagedErr
	}
	return git.staged, nil
}

func (git *commitCommandGit) Commit(_ context.Context, _ port.RepositoryIdentity, message commitmsg.Message) error {
	if git.commitErr != nil {
		return git.commitErr
	}
	git.committedMessages = append(git.committedMessages, message)
	return nil
}

var _ port.GitRepository = (*commitCommandGit)(nil)

type commitStringReply struct {
	value string
	err   error
}

type commitConfirmationReply struct {
	value bool
	err   error
}

type commitCommandPrompt struct {
	inputs   []commitStringReply
	selects  []commitStringReply
	confirms []commitConfirmationReply

	inputRequests   []port.InputRequest
	selectRequests  []port.SelectRequest
	confirmRequests []port.ConfirmRequest
}

func (prompt *commitCommandPrompt) Input(_ context.Context, request port.InputRequest) (string, error) {
	prompt.inputRequests = append(prompt.inputRequests, request)
	if len(prompt.inputs) == 0 {
		return "", errors.New("unexpected input prompt")
	}
	reply := prompt.inputs[0]
	prompt.inputs = prompt.inputs[1:]
	return reply.value, reply.err
}

func (prompt *commitCommandPrompt) Select(_ context.Context, request port.SelectRequest) (string, error) {
	prompt.selectRequests = append(prompt.selectRequests, request)
	if len(prompt.selects) == 0 {
		return "", errors.New("unexpected select prompt")
	}
	reply := prompt.selects[0]
	prompt.selects = prompt.selects[1:]
	return reply.value, reply.err
}

func (prompt *commitCommandPrompt) Confirm(_ context.Context, request port.ConfirmRequest) (bool, error) {
	prompt.confirmRequests = append(prompt.confirmRequests, request)
	if len(prompt.confirms) == 0 {
		return false, errors.New("unexpected confirmation prompt")
	}
	reply := prompt.confirms[0]
	prompt.confirms = prompt.confirms[1:]
	return reply.value, reply.err
}

var _ port.Prompt = (*commitCommandPrompt)(nil)
