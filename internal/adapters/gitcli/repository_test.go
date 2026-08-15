package gitcli

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

type recordedCall struct {
	directory string
	stdin     string
	arguments []string
}

type fakeRunner struct {
	results []processResult
	calls   []recordedCall
}

func (runner *fakeRunner) run(_ context.Context, directory string, stdin io.Reader, arguments ...string) processResult {
	var input string
	if stdin != nil {
		bytes, _ := io.ReadAll(stdin)
		input = string(bytes)
	}
	runner.calls = append(runner.calls, recordedCall{
		directory: directory,
		stdin:     input,
		arguments: append([]string(nil), arguments...),
	})
	if len(runner.results) == 0 {
		return processResult{}
	}
	result := runner.results[0]
	runner.results = runner.results[1:]
	return result
}

type waitingRunner struct{}

func (waitingRunner) run(ctx context.Context, _ string, _ io.Reader, _ ...string) processResult {
	<-ctx.Done()
	return processResult{err: ctx.Err(), exitCode: -1}
}

type environmentFakeRunner struct {
	*fakeRunner
	environments [][]string
}

func (runner *environmentFakeRunner) runWithEnvironment(
	ctx context.Context,
	directory string,
	stdin io.Reader,
	environment []string,
	arguments ...string,
) processResult {
	runner.environments = append(runner.environments, append([]string(nil), environment...))
	return runner.fakeRunner.run(ctx, directory, stdin, arguments...)
}

func TestRepositoryReadOperations(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		call   func(*Repository, port.RepositoryIdentity) error
		result processResult
		code   problem.Code
	}{
		{
			name: "fetch failure",
			call: func(repository *Repository, identity port.RepositoryIdentity) error {
				return repository.Fetch(context.Background(), identity)
			},
			result: processResult{err: errors.New("failed"), exitCode: 128},
			code:   problem.CodeGitCommandFailed,
		},
		{
			name: "invalid ref failure",
			call: func(repository *Repository, identity port.RepositoryIdentity) error {
				name, _ := branch.ParseName("feature/ABC-123-add-export")
				return repository.ValidateBranchRef(context.Background(), identity, name)
			},
			result: processResult{err: errors.New("invalid ref"), exitCode: 1},
			code:   problem.CodeBranchRefInvalid,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runner := &fakeRunner{results: []processResult{testCase.result}}
			repository := &Repository{runner: runner, timeout: time.Second}
			err := testCase.call(repository, testIdentity())
			assertProblemCode(t, err, testCase.code)
		})
	}
}

func TestDiscoverAndBasicStates(t *testing.T) {
	t.Parallel()

	t.Run("discover", func(t *testing.T) {
		runner := &fakeRunner{results: []processResult{{stdout: "C:/work/repo\n"}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		actual, err := repository.Discover(context.Background(), "C:/work/repo/subdir")
		if err != nil {
			t.Fatal(err)
		}
		if actual.Root != "C:/work/repo" || actual.Remote != "origin" {
			t.Fatalf("Discover() = %#v", actual)
		}
		assertCall(t, runner.calls[0], "C:/work/repo/subdir", "", "rev-parse", "--show-toplevel")
	})

	t.Run("missing repository", func(t *testing.T) {
		repository := &Repository{runner: &fakeRunner{results: []processResult{{err: errors.New("not a repo"), exitCode: 128}}}, timeout: time.Second}
		_, err := repository.Discover(context.Background(), "C:/not-a-repo")
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)
	})

	t.Run("has commits", func(t *testing.T) {
		repository := &Repository{runner: &fakeRunner{results: []processResult{{}}}, timeout: time.Second}
		actual, err := repository.HasCommits(context.Background(), testIdentity())
		if err != nil || !actual {
			t.Fatalf("HasCommits() = (%t, %v)", actual, err)
		}
	})

	t.Run("no commits", func(t *testing.T) {
		repository := &Repository{runner: &fakeRunner{results: []processResult{{err: errors.New("missing HEAD"), exitCode: 1}}}, timeout: time.Second}
		actual, err := repository.HasCommits(context.Background(), testIdentity())
		if err != nil || actual {
			t.Fatalf("HasCommits() = (%t, %v)", actual, err)
		}
	})

	t.Run("worktree clean", func(t *testing.T) {
		repository := &Repository{runner: &fakeRunner{results: []processResult{{stdout: ""}}}, timeout: time.Second}
		actual, err := repository.IsWorktreeClean(context.Background(), testIdentity())
		if err != nil || !actual {
			t.Fatalf("IsWorktreeClean() = (%t, %v)", actual, err)
		}
	})

	t.Run("worktree dirty", func(t *testing.T) {
		repository := &Repository{runner: &fakeRunner{results: []processResult{{stdout: " M README.md\n"}}}, timeout: time.Second}
		actual, err := repository.IsWorktreeClean(context.Background(), testIdentity())
		if err != nil || actual {
			t.Fatalf("IsWorktreeClean() = (%t, %v)", actual, err)
		}
	})
}

func TestDoctorGitOperations(t *testing.T) {
	t.Parallel()

	t.Run("version and remote", func(t *testing.T) {
		runner := &fakeRunner{results: []processResult{
			{stdout: "git version 2.53.0\n"},
			{stdout: "https://example.invalid/repo.git\n"},
		}}
		repository := &Repository{runner: runner, timeout: time.Second}
		version, err := repository.Version(context.Background())
		if err != nil || version != "git version 2.53.0" {
			t.Fatalf("Version() = (%q, %v)", version, err)
		}
		url, err := repository.RemoteURL(context.Background(), testIdentity())
		if err != nil || url != "https://example.invalid/repo.git" {
			t.Fatalf("RemoteURL() = (%q, %v)", url, err)
		}
		assertCall(t, runner.calls[0], "", "", "--version")
		assertCall(t, runner.calls[1], "C:/repo", "", "remote", "get-url", "origin")
	})

	t.Run("active merge", func(t *testing.T) {
		root := t.TempDir()
		gitDirectory := filepath.Join(root, ".git")
		if err := os.MkdirAll(gitDirectory, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(gitDirectory, "MERGE_HEAD"), []byte("merge"), 0o600); err != nil {
			t.Fatal(err)
		}
		runner := &fakeRunner{results: []processResult{
			{stdout: ".git/rebase-merge\n"},
			{stdout: ".git/rebase-apply\n"},
			{stdout: ".git/MERGE_HEAD\n"},
		}}
		repository := &Repository{runner: runner, timeout: time.Second}
		operation, active, err := repository.ActiveOperation(context.Background(), port.RepositoryIdentity{Root: root, Remote: "origin"})
		if err != nil || !active || operation != "merge" {
			t.Fatalf("ActiveOperation() = (%q, %t, %v)", operation, active, err)
		}
	})
}

func TestCheckTransportAuthenticationUsesNonInteractiveDryRunPush(t *testing.T) {
	identity := testIdentity()
	t.Run("succeeds through the environment-aware process runner", func(t *testing.T) {
		runner := &environmentFakeRunner{fakeRunner: &fakeRunner{results: []processResult{
			{stdout: "feature/ABC-123-add-export\n"},
			{},
		}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		if err := repository.CheckTransportAuthentication(context.Background(), identity); err != nil {
			t.Fatalf("CheckTransportAuthentication() error = %v", err)
		}
		if len(runner.calls) != 2 || len(runner.environments) != 2 {
			t.Fatalf("authentication calls=%#v environments=%#v", runner.calls, runner.environments)
		}
		assertCall(t, runner.calls[0], identity.Root, "", "branch", "--show-current")
		assertCall(
			t,
			runner.calls[1],
			identity.Root,
			"",
			"push",
			"--dry-run",
			"--no-verify",
			"--porcelain",
			identity.Remote,
			"HEAD:refs/heads/feature/ABC-123-add-export",
		)
		for _, environment := range runner.environments {
			if strings.Join(environment, ",") != strings.Join(noPromptGitEnvironment, ",") {
				t.Fatalf("non-interactive environment = %#v", environment)
			}
		}
	})

	t.Run("falls back to a regular runner and rejects a detached HEAD", func(t *testing.T) {
		runner := &fakeRunner{results: []processResult{{stdout: ""}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		err := repository.CheckTransportAuthentication(context.Background(), identity)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
		if len(runner.calls) != 1 {
			t.Fatalf("fallback authentication calls = %#v", runner.calls)
		}
	})

	for _, testCase := range []struct {
		name    string
		results []processResult
		code    problem.Code
	}{
		{
			name: "branch lookup failure",
			results: []processResult{{
				err:      errors.New("branch failed"),
				exitCode: 1,
			}},
			code: problem.CodeGitCommandFailed,
		},
		{
			name: "push authentication failure",
			results: []processResult{
				{stdout: "feature/ABC-123-add-export"},
				{err: errors.New("authentication failed"), exitCode: 128},
			},
			code: problem.CodeGitCommandFailed,
		},
		{
			name: "push timeout",
			results: []processResult{
				{stdout: "feature/ABC-123-add-export"},
				{err: context.DeadlineExceeded, exitCode: -1},
			},
			code: problem.CodeExternalCommandFailed,
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			runner := &fakeRunner{results: testCase.results}
			repository := &Repository{runner: runner, timeout: time.Second}
			err := repository.CheckTransportAuthentication(context.Background(), identity)
			assertProblemCode(t, err, testCase.code)
			value, _ := problem.As(err)
			if value.Field != "Git authentication" || strings.Contains(value.Error(), "authentication failed") {
				t.Fatalf("authentication problem = %#v", value)
			}
		})
	}
}

func TestInvokeNoPromptUsesBackgroundForNilContext(t *testing.T) {
	runner := &fakeRunner{results: []processResult{{stdout: "ok"}}}
	repository := &Repository{runner: runner, timeout: time.Second}
	result := repository.invokeNoPrompt(testNilContext(), "C:/repo", nil, "--version")
	if result.stdout != "ok" || len(runner.calls) != 1 {
		t.Fatalf("invokeNoPrompt() = %#v, calls=%#v", result, runner.calls)
	}
}

func TestContinueCherryPick(t *testing.T) {
	t.Parallel()

	t.Run("continues with a non-interactive editor", func(t *testing.T) {
		runner := &fakeRunner{results: []processResult{{}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		if err := repository.ContinueCherryPick(context.Background(), testIdentity()); err != nil {
			t.Fatal(err)
		}
		assertCall(t, runner.calls[0], "C:/repo", "", "-c", "core.editor=true", "cherry-pick", "--continue")
	})

	t.Run("classifies continuation failures", func(t *testing.T) {
		repository := &Repository{
			runner:  &fakeRunner{results: []processResult{{err: errors.New("continue failed"), exitCode: 1}}},
			timeout: time.Second,
		}
		err := repository.ContinueCherryPick(context.Background(), testIdentity())
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})
}

func TestBranchStateOperations(t *testing.T) {
	t.Parallel()

	name, err := branch.ParseName("feature/ABC-123-add-export")
	if err != nil {
		t.Fatal(err)
	}
	develop, _ := branch.ParseName("develop")
	base, err := branch.NewTargetBase("origin", develop)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("current branch", func(t *testing.T) {
		repository := &Repository{runner: &fakeRunner{results: []processResult{{stdout: name.String() + "\n"}}}, timeout: time.Second}
		actual, err := repository.CurrentBranch(context.Background(), testIdentity())
		if err != nil || actual.String() != name.String() {
			t.Fatalf("CurrentBranch() = (%q, %v)", actual.String(), err)
		}
	})

	t.Run("detached head", func(t *testing.T) {
		repository := &Repository{runner: &fakeRunner{results: []processResult{{stdout: "\n"}}}, timeout: time.Second}
		_, err := repository.CurrentBranch(context.Background(), testIdentity())
		assertProblemCode(t, err, problem.CodeBranchNameInvalid)
	})

	t.Run("branch existence", func(t *testing.T) {
		repository := &Repository{runner: &fakeRunner{results: []processResult{{}}}, timeout: time.Second}
		actual, err := repository.BranchExists(context.Background(), testIdentity(), name)
		if err != nil || !actual {
			t.Fatalf("BranchExists() = (%t, %v)", actual, err)
		}
	})

	t.Run("missing branch", func(t *testing.T) {
		repository := &Repository{runner: &fakeRunner{results: []processResult{{err: errors.New("missing"), exitCode: 1}}}, timeout: time.Second}
		actual, err := repository.BranchExists(context.Background(), testIdentity(), name)
		if err != nil || actual {
			t.Fatalf("BranchExists() = (%t, %v)", actual, err)
		}
	})

	t.Run("create and switch", func(t *testing.T) {
		runner := &fakeRunner{results: []processResult{{}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		if err := repository.CreateBranch(context.Background(), testIdentity(), name, base, true); err != nil {
			t.Fatal(err)
		}
		assertCall(t, runner.calls[0], "C:/repo", "", "switch", "-c", name.String(), base.String())
	})

	t.Run("create without switch", func(t *testing.T) {
		runner := &fakeRunner{results: []processResult{{}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		if err := repository.CreateBranch(context.Background(), testIdentity(), name, base, false); err != nil {
			t.Fatal(err)
		}
		assertCall(t, runner.calls[0], "C:/repo", "", "branch", name.String(), base.String())
	})

	t.Run("publication state", func(t *testing.T) {
		repository := &Repository{runner: &fakeRunner{results: []processResult{{}}}, timeout: time.Second}
		actual, err := repository.PublicationState(context.Background(), testIdentity(), name)
		if err != nil || actual != branch.PublicationPublished {
			t.Fatalf("PublicationState() = (%q, %v)", actual, err)
		}
	})

	t.Run("unpublished state", func(t *testing.T) {
		repository := &Repository{runner: &fakeRunner{results: []processResult{{err: errors.New("missing"), exitCode: 1}}}, timeout: time.Second}
		actual, err := repository.PublicationState(context.Background(), testIdentity(), name)
		if err != nil || actual != branch.PublicationUnpublished {
			t.Fatalf("PublicationState() = (%q, %v)", actual, err)
		}
	})

	t.Run("base delta", func(t *testing.T) {
		repository := &Repository{runner: &fakeRunner{results: []processResult{{stdout: "2\n"}}}, timeout: time.Second}
		actual, err := repository.HasMissingBaseCommits(context.Background(), testIdentity(), base)
		if err != nil || !actual {
			t.Fatalf("HasMissingBaseCommits() = (%t, %v)", actual, err)
		}
	})

	t.Run("malformed base count", func(t *testing.T) {
		repository := &Repository{runner: &fakeRunner{results: []processResult{{stdout: "not-a-number"}}}, timeout: time.Second}
		_, err := repository.HasMissingBaseCommits(context.Background(), testIdentity(), base)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})
}

func TestOfficialBranchesForTicketFindsLocalAndRemoteCanonicalBranches(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{results: []processResult{{
		stdout: strings.Join([]string{
			"refs/heads/feature/ABC-123-add-export",
			"refs/heads/scratch/ABC-123-experiment",
			"refs/heads/feature/ABC-124-other-ticket",
			"refs/heads/noncanonical-local-branch",
			"refs/remotes/origin/HEAD",
			"refs/remotes/origin/fix/ABC-123-correct-export",
			"refs/remotes/upstream/feature/ABC-123-other-remote",
		}, "\n") + "\n",
	}}}
	repository := &Repository{runner: runner, timeout: time.Second}
	id, err := ticket.ParseID("ABC-123")
	if err != nil {
		t.Fatal(err)
	}

	actual, err := repository.OfficialBranchesForTicket(context.Background(), testIdentity(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(actual) != 2 || actual[0].String() != "feature/ABC-123-add-export" || actual[1].String() != "fix/ABC-123-correct-export" {
		t.Fatalf("OfficialBranchesForTicket() = %v", actual)
	}
	assertCall(
		t,
		runner.calls[0],
		"C:/repo",
		"",
		"for-each-ref",
		"--format=%(refname)",
		"refs/heads",
		"refs/remotes/origin",
	)
}

func TestMutationOperationsUseArgumentArraysAndStdin(t *testing.T) {
	t.Parallel()

	identity := testIdentity()
	name, _ := branch.ParseName("feature/ABC-123-add-export")
	develop, _ := branch.ParseName("develop")
	base, _ := branch.NewTargetBase("origin", develop)
	id, _ := ticket.ParseID("ABC-123")
	header, _ := commitmsg.NewHeader(commitmsg.TypeChore, id, "merge origin/develop", false)
	message, _ := commitmsg.NewMessage(header, "", nil)

	runner := &fakeRunner{results: []processResult{{}, {}, {}, {}, {}, {}, {}}}
	repository := &Repository{runner: runner, timeout: time.Second}

	if err := repository.Rebase(context.Background(), identity, base); err != nil {
		t.Fatal(err)
	}
	if err := repository.Merge(context.Background(), identity, base, message); err != nil {
		t.Fatal(err)
	}
	if err := repository.CherryPick(context.Background(), identity, strings.Repeat("a", 40)); err != nil {
		t.Fatal(err)
	}
	if err := repository.Stage(context.Background(), identity, []string{"safe.go", ";not-a-shell-command"}); err != nil {
		t.Fatal(err)
	}
	if err := repository.Commit(context.Background(), identity, message); err != nil {
		t.Fatal(err)
	}
	if err := repository.Push(context.Background(), identity, name, true); err != nil {
		t.Fatal(err)
	}
	if err := repository.SwitchBranch(context.Background(), identity, name); err != nil {
		t.Fatal(err)
	}

	assertCall(t, runner.calls[0], "C:/repo", "", "rebase", "origin/develop")
	assertCall(t, runner.calls[1], "C:/repo", "", "merge", "--no-ff", "--no-edit", "-m", message.String(), "origin/develop")
	assertCall(t, runner.calls[2], "C:/repo", "", "cherry-pick", "-x", strings.Repeat("a", 40))
	assertCall(t, runner.calls[3], "C:/repo", "", "add", "--", "safe.go", ";not-a-shell-command")
	assertCall(t, runner.calls[4], "C:/repo", message.String()+"\n", "commit", "--file=-")
	assertCall(t, runner.calls[5], "C:/repo", "", "push", "--set-upstream", "origin", name.String())
	assertCall(t, runner.calls[6], "C:/repo", "", "switch", name.String())

	if err := repository.Stage(context.Background(), identity, nil); err == nil {
		t.Fatal("Stage accepted empty paths")
	} else {
		assertProblemCode(t, err, problem.CodeInvalidInput)
	}
}

func TestContinueRebaseUsesAControlledEditor(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{results: []processResult{{}}}
	repository := &Repository{runner: runner, timeout: time.Second}
	if err := repository.ContinueRebase(context.Background(), testIdentity()); err != nil {
		t.Fatal(err)
	}
	assertCall(t, runner.calls[0], "C:/repo", "", "-c", "core.editor=true", "rebase", "--continue")

	failing := &Repository{
		runner:  &fakeRunner{results: []processResult{{err: errors.New("continue failed"), exitCode: 1}}},
		timeout: time.Second,
	}
	err := failing.ContinueRebase(context.Background(), testIdentity())
	assertProblemCode(t, err, problem.CodeGitCommandFailed)
}

func TestSquashMergeUsesArgumentArrays(t *testing.T) {
	t.Parallel()

	source, err := branch.ParseName("scratch/ABC-123-export-exploration")
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{results: []processResult{{}}}
	repository := &Repository{runner: runner, timeout: time.Second}
	if err := repository.SquashMerge(context.Background(), testIdentity(), source); err != nil {
		t.Fatal(err)
	}
	assertCall(
		t,
		runner.calls[0],
		"C:/repo",
		"",
		"merge",
		"--squash",
		"--no-commit",
		source.String(),
	)

	failing := &Repository{
		runner:  &fakeRunner{results: []processResult{{err: errors.New("conflict"), exitCode: 1}}},
		timeout: time.Second,
	}
	err = failing.SquashMerge(context.Background(), testIdentity(), source)
	assertProblemCode(t, err, problem.CodeGitCommandFailed)
}

func TestInspectPushUpdateUsesExactObjectIDs(t *testing.T) {
	t.Parallel()

	identity := testIdentity()
	develop, err := branch.ParseName("develop")
	if err != nil {
		t.Fatal(err)
	}
	base, err := branch.NewTargetBase("origin", develop)
	if err != nil {
		t.Fatal(err)
	}
	localObjectID := strings.Repeat("a", 40)
	remoteObjectID := strings.Repeat("b", 40)
	runner := &fakeRunner{results: []processResult{
		{stdout: "0\n"},
		{stdout: "\x1efeat(ABC-123): add export\x00"},
		{},
	}}
	repository := &Repository{runner: runner, timeout: time.Second}

	inspection, err := repository.InspectPushUpdate(context.Background(), identity, base, localObjectID, remoteObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.MissingBaseCommits || !inspection.FastForward || len(inspection.CommitMessages) != 1 {
		t.Fatalf("InspectPushUpdate() = %#v", inspection)
	}
	assertCall(t, runner.calls[0], "C:/repo", "", "rev-list", "--count", localObjectID+"..origin/develop")
	assertCall(t, runner.calls[1], "C:/repo", "", "log", "--format=%x1e%B%x00", "origin/develop.."+localObjectID)
	assertCall(t, runner.calls[2], "C:/repo", "", "merge-base", "--is-ancestor", remoteObjectID, localObjectID)
}

func TestInspectPushUpdateTreatsZeroRemoteIDAsBranchCreation(t *testing.T) {
	t.Parallel()

	develop, _ := branch.ParseName("develop")
	base, _ := branch.NewTargetBase("origin", develop)
	localObjectID := strings.Repeat("a", 40)
	runner := &fakeRunner{results: []processResult{
		{stdout: "0\n"},
		{stdout: "\x1efeat(ABC-123): add export\x00"},
	}}
	repository := &Repository{runner: runner, timeout: time.Second}

	inspection, err := repository.InspectPushUpdate(
		context.Background(),
		testIdentity(),
		base,
		localObjectID,
		strings.Repeat("0", 40),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !inspection.FastForward || len(runner.calls) != 2 {
		t.Fatalf("InspectPushUpdate() = %#v, calls=%d", inspection, len(runner.calls))
	}
}

func TestInspectPushUpdateDetectsNonFastForward(t *testing.T) {
	t.Parallel()

	develop, _ := branch.ParseName("develop")
	base, _ := branch.NewTargetBase("origin", develop)
	runner := &fakeRunner{results: []processResult{
		{stdout: "0\n"},
		{stdout: "\x1efeat(ABC-123): add export\x00"},
		{err: errors.New("not an ancestor"), exitCode: 1},
	}}
	repository := &Repository{runner: runner, timeout: time.Second}

	inspection, err := repository.InspectPushUpdate(
		context.Background(),
		testIdentity(),
		base,
		strings.Repeat("a", 40),
		strings.Repeat("b", 40),
	)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.FastForward {
		t.Fatalf("InspectPushUpdate() reported a non-fast-forward update as fast-forward: %#v", inspection)
	}
}

func TestBranchCleanupAndReleaseTagOperationsUseArgumentArrays(t *testing.T) {
	t.Parallel()

	name, err := branch.ParseName("hotfix/ABC-999-payment-timeout")
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{results: []processResult{
		{},
		{stdout: `{"hotfix/ABC-999-payment-timeout":"origin/main"}`},
		{},
		{stdout: "v2.8.0\nv2.8.1\n"},
	}}
	repository := &Repository{runner: runner, timeout: time.Second}
	if err := repository.DeleteLocalBranch(context.Background(), testIdentity(), name, false); err != nil {
		t.Fatal(err)
	}
	if err := repository.ClearWorkflowBase(context.Background(), testIdentity(), name); err != nil {
		t.Fatal(err)
	}
	tags, err := repository.ReleaseTagsAt(context.Background(), testIdentity(), "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(tags, ",") != "v2.8.0,v2.8.1" {
		t.Fatalf("ReleaseTagsAt() = %v", tags)
	}
	assertCall(t, runner.calls[0], "C:/repo", "", "branch", "-d", name.String())
	assertCall(t, runner.calls[1], "C:/repo", "", "config", "--local", "--get", workflowBasesConfigKey)
	assertCall(t, runner.calls[2], "C:/repo", "", "config", "--local", workflowBasesConfigKey, `{}`)
	assertCall(t, runner.calls[3], "C:/repo", "", "tag", "--points-at", "origin/main")
}

func TestWorkflowBaseMetadataUsesLocalGitConfiguration(t *testing.T) {
	t.Parallel()

	name, err := branch.ParseName("hotfix/ABC-999-payment-timeout")
	if err != nil {
		t.Fatal(err)
	}
	main, _ := branch.ParseName("main")
	base, err := branch.NewTargetBase("origin", main)
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{results: []processResult{
		{err: errors.New("missing"), exitCode: 1},
		{},
		{stdout: `{"hotfix/ABC-999-payment-timeout":"origin/main"}`},
	}}
	repository := &Repository{runner: runner, timeout: time.Second}
	if err := repository.StoreWorkflowBase(context.Background(), testIdentity(), name, base); err != nil {
		t.Fatal(err)
	}
	actual, found, err := repository.WorkflowBase(context.Background(), testIdentity(), name)
	if err != nil || !found || actual.String() != "origin/main" {
		t.Fatalf("WorkflowBase() = (%q, %t, %v)", actual.String(), found, err)
	}
	assertCall(t, runner.calls[0], "C:/repo", "", "config", "--local", "--get", workflowBasesConfigKey)
	assertCall(t, runner.calls[1], "C:/repo", "", "config", "--local", workflowBasesConfigKey, `{"hotfix/ABC-999-payment-timeout":"origin/main"}`)
	assertCall(t, runner.calls[2], "C:/repo", "", "config", "--local", "--get", workflowBasesConfigKey)
}

func TestHasStagedChanges(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name    string
		result  processResult
		staged  bool
		hasCode problem.Code
	}{
		{name: "none", result: processResult{}, staged: false},
		{name: "has changes", result: processResult{err: errors.New("different"), exitCode: 1}, staged: true},
		{name: "unexpected error", result: processResult{err: errors.New("failed"), exitCode: 128}, hasCode: problem.CodeGitCommandFailed},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			repository := &Repository{runner: &fakeRunner{results: []processResult{testCase.result}}, timeout: time.Second}
			actual, err := repository.HasStagedChanges(context.Background(), testIdentity())
			if testCase.hasCode != "" {
				assertProblemCode(t, err, testCase.hasCode)
				return
			}
			if err != nil || actual != testCase.staged {
				t.Fatalf("HasStagedChanges() = (%t, %v)", actual, err)
			}
		})
	}
}

func TestHasUnmergedConflicts(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name      string
		result    processResult
		conflicts bool
		code      problem.Code
	}{
		{name: "none", result: processResult{}, conflicts: false},
		{name: "unmerged paths", result: processResult{stdout: "conflict.txt\n"}, conflicts: true},
		{name: "command failure", result: processResult{err: errors.New("diff failed"), exitCode: 128}, code: problem.CodeGitCommandFailed},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			repository, runner := coverageRepository(testCase.result)
			conflicts, err := repository.HasUnmergedConflicts(context.Background(), testIdentity())
			if testCase.code != "" {
				assertProblemCode(t, err, testCase.code)
				return
			}
			if err != nil || conflicts != testCase.conflicts {
				t.Fatalf("HasUnmergedConflicts() = (%t, %v)", conflicts, err)
			}
			assertCall(t, runner.calls[0], "C:/repo", "", "diff", "--name-only", "--diff-filter=U")
		})
	}
}

func TestInvokeHonorsCancellation(t *testing.T) {
	t.Parallel()

	repository := &Repository{runner: waitingRunner{}, timeout: time.Millisecond}
	err := repository.Fetch(context.Background(), testIdentity())
	actual, ok := problem.As(err)
	if !ok {
		t.Fatalf("Fetch() error %T does not carry a problem", err)
	}
	if actual.Code != problem.CodeExternalCommandFailed || actual.Category != problem.CategoryExternal {
		t.Fatalf("Fetch() problem = %#v", actual.Details)
	}
}

func TestBoundedBuffer(t *testing.T) {
	t.Parallel()

	buffer := newBoundedBuffer(3)
	if written, err := buffer.Write([]byte("abcdef")); err != nil || written != 6 {
		t.Fatalf("Write() = (%d, %v)", written, err)
	}
	if buffer.String() != "abc" || !buffer.Truncated() {
		t.Fatalf("bounded buffer = (%q, %t)", buffer.String(), buffer.Truncated())
	}
	if written, err := buffer.Write([]byte("g")); err != nil || written != 1 {
		t.Fatalf("second Write() = (%d, %v)", written, err)
	}
}

func TestExecRunnerProcessContract(t *testing.T) {
	t.Parallel()

	runner := execRunner{binary: "go", maxOutputBytes: 1024}
	result := runner.run(context.Background(), "", nil, "version")
	if result.err != nil || result.exitCode != 0 {
		t.Fatalf("execRunner.run() = %#v", result)
	}
	if !strings.Contains(result.stdout, "go version") {
		t.Fatalf("stdout = %q", result.stdout)
	}
}

func testIdentity() port.RepositoryIdentity {
	return port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"}
}

func assertCall(t *testing.T, actual recordedCall, directory, stdin string, arguments ...string) {
	t.Helper()
	if actual.directory != directory {
		t.Fatalf("directory = %q, want %q", actual.directory, directory)
	}
	if actual.stdin != stdin {
		t.Fatalf("stdin = %q, want %q", actual.stdin, stdin)
	}
	if len(actual.arguments) != len(arguments) {
		t.Fatalf("arguments = %v, want %v", actual.arguments, arguments)
	}
	for index, expected := range arguments {
		if actual.arguments[index] != expected {
			t.Fatalf("arguments[%d] = %q, want %q", index, actual.arguments[index], expected)
		}
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
