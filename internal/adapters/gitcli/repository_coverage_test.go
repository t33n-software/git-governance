package gitcli

import (
	"context"
	"errors"
	"fmt"
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

type contextCaptureRunner struct {
	received context.Context
	result   processResult
}

func (runner *contextCaptureRunner) run(ctx context.Context, _ string, _ io.Reader, _ ...string) processResult {
	runner.received = ctx
	return runner.result
}

func coverageRepository(results ...processResult) (*Repository, *fakeRunner) {
	runner := &fakeRunner{results: results}
	return &Repository{runner: runner, timeout: time.Second}, runner
}

func coverageBranch(t *testing.T, raw string) branch.BranchName {
	t.Helper()

	name, err := branch.ParseName(raw)
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func coverageBase(t *testing.T, remote, raw string) branch.TargetBase {
	t.Helper()

	base, err := branch.NewTargetBase(remote, coverageBranch(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	return base
}

func coverageMessage(t *testing.T) commitmsg.Message {
	t.Helper()

	id, err := ticket.ParseID("ABC-123")
	if err != nil {
		t.Fatal(err)
	}
	header, err := commitmsg.NewHeader(commitmsg.TypeChore, id, "merge origin/develop", false)
	if err != nil {
		t.Fatal(err)
	}
	message, err := commitmsg.NewMessage(header, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return message
}

func assertProblemCategory(t *testing.T, err error, code problem.Code, category problem.Category) *problem.Problem {
	t.Helper()

	assertProblemCode(t, err, code)
	actual, ok := problem.As(err)
	if !ok {
		t.Fatalf("error %T does not carry a problem: %v", err, err)
	}
	if actual.Category != category {
		t.Fatalf("problem category = %q, want %q", actual.Category, category)
	}
	return actual
}

func TestRepositoryCoverageNewAndNilContext(t *testing.T) {
	t.Parallel()

	t.Run("new applies defaults", func(t *testing.T) {
		repository := New(Options{})
		runner, ok := repository.runner.(execRunner)
		if !ok {
			t.Fatalf("New() runner = %T, want execRunner", repository.runner)
		}
		if runner.binary != "git" || runner.maxOutputBytes != 0 || repository.timeout != defaultTimeout {
			t.Fatalf("New() = %#v, runner = %#v", repository, runner)
		}
	})

	t.Run("new preserves explicit options", func(t *testing.T) {
		repository := New(Options{
			Binary:         "git-custom",
			Timeout:        7 * time.Second,
			MaxOutputBytes: 512,
		})
		runner, ok := repository.runner.(execRunner)
		if !ok {
			t.Fatalf("New() runner = %T, want execRunner", repository.runner)
		}
		if runner.binary != "git-custom" || runner.maxOutputBytes != 512 || repository.timeout != 7*time.Second {
			t.Fatalf("New() = %#v, runner = %#v", repository, runner)
		}
	})

	t.Run("nil context is replaced with a timed context", func(t *testing.T) {
		runner := &contextCaptureRunner{result: processResult{stdout: "git version test\n"}}
		repository := &Repository{runner: runner, timeout: time.Second}

		version, err := repository.Version(testNilContext())
		if err != nil || version != "git version test" {
			t.Fatalf("Version() = (%q, %v)", version, err)
		}
		if runner.received == nil {
			t.Fatal("runner received a nil context")
		}
		if _, found := runner.received.Deadline(); !found {
			t.Fatal("runner context has no timeout deadline")
		}
	})
}

func testNilContext() context.Context {
	return nil
}

func TestRepositoryCoverageTargetBaseAvailabilityAndDiagnosticSanitization(t *testing.T) {
	t.Parallel()

	base := coverageBase(t, "origin", "develop")

	t.Run("target base availability distinguishes absent and failed checks", func(t *testing.T) {
		repository, runner := coverageRepository(processResult{})
		exists, err := repository.TargetBaseExists(context.Background(), testIdentity(), base)
		if err != nil || !exists {
			t.Fatalf("TargetBaseExists(success) = (%t, %v)", exists, err)
		}
		assertCall(t, runner.calls[0], "C:/repo", "", "show-ref", "--verify", "--quiet", "refs/remotes/origin/develop")

		repository, _ = coverageRepository(processResult{err: errors.New("missing ref"), exitCode: 1})
		exists, err = repository.TargetBaseExists(context.Background(), testIdentity(), base)
		if err != nil || exists {
			t.Fatalf("TargetBaseExists(absent) = (%t, %v)", exists, err)
		}

		repository, _ = coverageRepository(processResult{err: errors.New("show-ref failed"), exitCode: 128})
		_, err = repository.TargetBaseExists(context.Background(), testIdentity(), base)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)

		local, parseErr := branch.ParseLocalBase("feature/ABC-123-add-export")
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		repository, _ = coverageRepository()
		_, err = repository.TargetBaseExists(context.Background(), testIdentity(), local)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})

	t.Run("diagnostics are bounded and redact credentials", func(t *testing.T) {
		redacted := commandDiagnostic(processResult{
			stderr: "fatal: https://user:token@example.invalid/repository\nTOKEN=secret-value",
		})
		if strings.Contains(redacted, "token") || strings.Contains(redacted, "secret-value") {
			t.Fatalf("credential diagnostic leaked secret: %q", redacted)
		}
		for _, expected := range []string{"https://[redacted]@example.invalid", "TOKEN=[redacted]"} {
			if !strings.Contains(redacted, expected) {
				t.Fatalf("credential diagnostic missing %q: %q", expected, redacted)
			}
		}

		if actual := commandDiagnostic(processResult{stdout: "fallback output"}); actual != "fallback output" {
			t.Fatalf("stdout diagnostic = %q", actual)
		}
		if actual := commandDiagnostic(processResult{stderr: "short", truncated: true}); actual != "short\n[Git diagnostic output truncated]" {
			t.Fatalf("truncated diagnostic = %q", actual)
		}
		if actual := commandDiagnostic(processResult{}); actual != "" {
			t.Fatalf("empty diagnostic = %q", actual)
		}
		if actual := commandDiagnostic(processResult{stderr: strings.Repeat("a", maxDiagnosticBytes+1)}); !strings.HasSuffix(actual, "\n[Git diagnostic output truncated]") {
			t.Fatalf("oversized diagnostic = %q", actual)
		}
	})
}

func TestRepositoryCoverageBasicFailureBranches(t *testing.T) {
	t.Parallel()

	identity := testIdentity()

	t.Run("discover rejects empty root output", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{stdout: " \t\n"})
		_, err := repository.Discover(context.Background(), "C:/empty-root")
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)
	})

	t.Run("version classifies command failure", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{err: errors.New("missing executable"), exitCode: -1})
		_, err := repository.Version(context.Background())
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("version rejects empty output", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{stdout: "\n"})
		_, err := repository.Version(context.Background())
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("remote URL classifies command failure", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{err: errors.New("missing remote"), exitCode: 2})
		_, err := repository.RemoteURL(context.Background(), identity)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("remote URL rejects empty output", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{stdout: "  \r\n"})
		_, err := repository.RemoteURL(context.Background(), identity)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("has commits classifies unexpected failure", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{err: errors.New("bad repository"), exitCode: 128})
		_, err := repository.HasCommits(context.Background(), identity)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("worktree status classifies failure", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{err: errors.New("status failure"), exitCode: 128})
		_, err := repository.IsWorktreeClean(context.Background(), identity)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("current branch classifies command failure", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{err: errors.New("branch failure"), exitCode: 128})
		_, err := repository.CurrentBranch(context.Background(), identity)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("current branch rejects malformed Git output", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{stdout: "not a canonical branch\n"})
		_, err := repository.CurrentBranch(context.Background(), identity)
		assertProblemCode(t, err, problem.CodeBranchNameInvalid)
	})

	t.Run("valid branch ref succeeds", func(t *testing.T) {
		name := coverageBranch(t, "feature/ABC-123-add-export")
		repository, runner := coverageRepository(processResult{})
		if err := repository.ValidateBranchRef(context.Background(), identity, name); err != nil {
			t.Fatal(err)
		}
		assertCall(t, runner.calls[0], identity.Root, "", "check-ref-format", "--branch", name.String())
	})

	t.Run("branch existence classifies unexpected failure", func(t *testing.T) {
		name := coverageBranch(t, "feature/ABC-123-add-export")
		repository, _ := coverageRepository(processResult{err: errors.New("show-ref failure"), exitCode: 128})
		_, err := repository.BranchExists(context.Background(), identity, name)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})
}

func TestRepositoryCoverageActiveOperationBranches(t *testing.T) {
	t.Parallel()

	t.Run("no operation markers are active", func(t *testing.T) {
		root := t.TempDir()
		repository, runner := coverageRepository(
			processResult{stdout: ".git/rebase-merge\n"},
			processResult{stdout: ".git/rebase-apply\n"},
			processResult{stdout: ".git/MERGE_HEAD\n"},
			processResult{stdout: ".git/CHERRY_PICK_HEAD\n"},
		)

		operation, active, err := repository.ActiveOperation(context.Background(), port.RepositoryIdentity{Root: root, Remote: "origin"})
		if err != nil || active || operation != "" {
			t.Fatalf("ActiveOperation() = (%q, %t, %v)", operation, active, err)
		}
		if len(runner.calls) != 4 {
			t.Fatalf("ActiveOperation() calls = %d, want 4", len(runner.calls))
		}
	})

	t.Run("git path lookup failure is classified", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{err: errors.New("rev-parse failed"), exitCode: 128})
		_, _, err := repository.ActiveOperation(context.Background(), testIdentity())
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("empty marker path is malformed Git output", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{stdout: " \n"})
		_, _, err := repository.ActiveOperation(context.Background(), testIdentity())
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("inaccessible marker path is classified", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{stdout: "\x00"})
		_, _, err := repository.ActiveOperation(context.Background(), port.RepositoryIdentity{Root: t.TempDir(), Remote: "origin"})
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("absolute marker path is honored", func(t *testing.T) {
		marker := filepath.Join(t.TempDir(), "rebase-merge")
		if err := os.WriteFile(marker, []byte("rebase"), 0o600); err != nil {
			t.Fatal(err)
		}
		repository, _ := coverageRepository(processResult{stdout: marker + "\n"})

		operation, active, err := repository.ActiveOperation(context.Background(), testIdentity())
		if err != nil || !active || operation != "rebase" {
			t.Fatalf("ActiveOperation() = (%q, %t, %v)", operation, active, err)
		}
	})
}

func TestRepositoryCoverageOfficialBranchLookupFailures(t *testing.T) {
	t.Parallel()

	id, err := ticket.ParseID("ABC-123")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("command failure is classified", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{err: errors.New("for-each-ref failed"), exitCode: 128})
		_, err := repository.OfficialBranchesForTicket(context.Background(), testIdentity(), id)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("blank records are ignored", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{
			stdout: "refs/heads/feature/ABC-123-add-export\n \nrefs/remotes/origin/fix/ABC-123-correct-export\n",
		})
		actual, err := repository.OfficialBranchesForTicket(context.Background(), testIdentity(), id)
		if err != nil {
			t.Fatal(err)
		}
		if len(actual) != 2 || actual[0].String() != "feature/ABC-123-add-export" || actual[1].String() != "fix/ABC-123-correct-export" {
			t.Fatalf("OfficialBranchesForTicket() = %v", actual)
		}
	})
}

func TestRepositoryCoverageMutationFailureBranches(t *testing.T) {
	t.Parallel()

	identity := testIdentity()
	name := coverageBranch(t, "feature/ABC-123-add-export")
	base := coverageBase(t, "origin", "develop")
	message := coverageMessage(t)
	commitID := strings.Repeat("a", 40)

	operations := []struct {
		name      string
		call      func(*Repository) error
		stdin     string
		arguments []string
	}{
		{
			name:      "fetch",
			call:      func(repository *Repository) error { return repository.Fetch(context.Background(), identity) },
			arguments: []string{"fetch", "--prune", "origin"},
		},
		{
			name: "create branch",
			call: func(repository *Repository) error {
				return repository.CreateBranch(context.Background(), identity, name, base, false)
			},
			arguments: []string{"branch", name.String(), base.String()},
		},
		{
			name: "switch branch",
			call: func(repository *Repository) error {
				return repository.SwitchBranch(context.Background(), identity, name)
			},
			arguments: []string{"switch", name.String()},
		},
		{
			name:      "rebase",
			call:      func(repository *Repository) error { return repository.Rebase(context.Background(), identity, base) },
			arguments: []string{"rebase", base.String()},
		},
		{
			name: "merge",
			call: func(repository *Repository) error {
				return repository.Merge(context.Background(), identity, base, message)
			},
			arguments: []string{"merge", "--no-ff", "--no-edit", "-m", message.String(), base.String()},
		},
		{
			name:      "continue merge",
			call:      func(repository *Repository) error { return repository.ContinueMerge(context.Background(), identity) },
			arguments: []string{"-c", "core.editor=true", "merge", "--continue"},
		},
		{
			name: "cherry-pick",
			call: func(repository *Repository) error {
				return repository.CherryPick(context.Background(), identity, commitID)
			},
			arguments: []string{"cherry-pick", "-x", commitID},
		},
		{
			name: "force delete local branch",
			call: func(repository *Repository) error {
				return repository.DeleteLocalBranch(context.Background(), identity, name, true)
			},
			arguments: []string{"branch", "-D", name.String()},
		},
		{
			name: "stage explicit paths",
			call: func(repository *Repository) error {
				return repository.Stage(context.Background(), identity, []string{"--looks-like-an-argument"})
			},
			arguments: []string{"add", "--", "--looks-like-an-argument"},
		},
		{
			name:      "commit",
			call:      func(repository *Repository) error { return repository.Commit(context.Background(), identity, message) },
			stdin:     message.String() + "\n",
			arguments: []string{"commit", "--file=-"},
		},
		{
			name: "push without upstream",
			call: func(repository *Repository) error {
				return repository.Push(context.Background(), identity, name, false)
			},
			arguments: []string{"push", "origin", name.String()},
		},
		{
			name: "release tags",
			call: func(repository *Repository) error {
				_, err := repository.ReleaseTagsAt(context.Background(), identity, "origin/main")
				return err
			},
			arguments: []string{"tag", "--points-at", "origin/main"},
		},
	}

	for _, operation := range operations {
		operation := operation
		t.Run(operation.name, func(t *testing.T) {
			repository, runner := coverageRepository(processResult{err: errors.New("command failed"), exitCode: 128})

			err := operation.call(repository)
			assertProblemCode(t, err, problem.CodeGitCommandFailed)
			if len(runner.calls) != 1 {
				t.Fatalf("%s call count = %d, want 1", operation.name, len(runner.calls))
			}
			assertCall(t, runner.calls[0], identity.Root, operation.stdin, operation.arguments...)
		})
	}

	t.Run("fetch succeeds", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{})
		if err := repository.Fetch(context.Background(), identity); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("continue merge succeeds", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{})
		if err := repository.ContinueMerge(context.Background(), identity); err != nil {
			t.Fatal(err)
		}
	})
}

func TestRepositoryHeadIsMergeOf(t *testing.T) {
	t.Parallel()

	identity := testIdentity()
	release := coverageBase(t, "origin", "release/1.0.1")
	develop := coverageBase(t, "origin", "develop")

	t.Run("accepts the exact release and develop merge parents", func(t *testing.T) {
		repository, runner := coverageRepository(
			processResult{stdout: "release-sha\n"},
			processResult{stdout: "develop-sha\n"},
			processResult{stdout: "release-sha develop-sha\n"},
		)
		matches, err := repository.HeadIsMergeOf(context.Background(), identity, release, develop)
		if err != nil || !matches {
			t.Fatalf("HeadIsMergeOf() = (%t, %v)", matches, err)
		}
		assertCall(t, runner.calls[0], identity.Root, "", "rev-parse", "--verify", release.String()+"^{commit}")
		assertCall(t, runner.calls[1], identity.Root, "", "rev-parse", "--verify", develop.String()+"^{commit}")
		assertCall(t, runner.calls[2], identity.Root, "", "show", "-s", "--format=%P", "HEAD")
	})

	t.Run("rejects a branch with different merge topology", func(t *testing.T) {
		repository, _ := coverageRepository(
			processResult{stdout: "release-sha\n"},
			processResult{stdout: "develop-sha\n"},
			processResult{stdout: "develop-sha release-sha\n"},
		)
		matches, err := repository.HeadIsMergeOf(context.Background(), identity, release, develop)
		if err != nil || matches {
			t.Fatalf("HeadIsMergeOf() = (%t, %v)", matches, err)
		}
	})

	for _, testCase := range []struct {
		name    string
		results []processResult
	}{
		{
			name:    "release lookup failure",
			results: []processResult{{err: errors.New("release lookup"), exitCode: 128}},
		},
		{
			name: "develop lookup failure",
			results: []processResult{
				{stdout: "release-sha\n"},
				{err: errors.New("develop lookup"), exitCode: 128},
			},
		},
		{
			name: "merge parent lookup failure",
			results: []processResult{
				{stdout: "release-sha\n"},
				{stdout: "develop-sha\n"},
				{err: errors.New("parents lookup"), exitCode: 128},
			},
		},
		{
			name:    "empty resolved revision",
			results: []processResult{{}},
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			repository, _ := coverageRepository(testCase.results...)
			if _, err := repository.HeadIsMergeOf(context.Background(), identity, release, develop); err == nil {
				t.Fatal("invalid merge provenance was accepted")
			}
		})
	}
}

func TestRepositoryCoverageWorkflowBaseBranches(t *testing.T) {
	t.Parallel()

	identity := testIdentity()
	name := coverageBranch(t, "hotfix/ABC-999-payment-timeout")
	base := coverageBase(t, "origin", "main")

	t.Run("store rejects a local base", func(t *testing.T) {
		localBase, err := branch.NewLocalBase(coverageBranch(t, "main"))
		if err != nil {
			t.Fatal(err)
		}
		repository, runner := coverageRepository()

		err = repository.StoreWorkflowBase(context.Background(), identity, name, localBase)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
		if len(runner.calls) != 0 {
			t.Fatalf("StoreWorkflowBase() made %d calls for an invalid base", len(runner.calls))
		}
	})

	t.Run("store classifies metadata read failure", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{err: errors.New("config read failed"), exitCode: 128})
		err := repository.StoreWorkflowBase(context.Background(), identity, name, base)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("store classifies metadata write failure", func(t *testing.T) {
		repository, _ := coverageRepository(
			processResult{err: errors.New("key not found"), exitCode: 1},
			processResult{err: errors.New("config write failed"), exitCode: 128},
		)
		err := repository.StoreWorkflowBase(context.Background(), identity, name, base)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("store preserves existing metadata", func(t *testing.T) {
		repository, runner := coverageRepository(
			processResult{stdout: `{"feature/ABC-123-existing":"origin/develop"}`},
			processResult{},
		)
		if err := repository.StoreWorkflowBase(context.Background(), identity, name, base); err != nil {
			t.Fatal(err)
		}
		assertCall(t, runner.calls[0], identity.Root, "", "config", "--local", "--get", workflowBasesConfigKey)
		assertCall(
			t,
			runner.calls[1],
			identity.Root,
			"",
			"config",
			"--local",
			workflowBasesConfigKey,
			`{"feature/ABC-123-existing":"origin/develop","hotfix/ABC-999-payment-timeout":"origin/main"}`,
		)
	})

	t.Run("clear classifies metadata read failure", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{err: errors.New("config read failed"), exitCode: 128})
		err := repository.ClearWorkflowBase(context.Background(), identity, name)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("clear absent entry avoids a write", func(t *testing.T) {
		repository, runner := coverageRepository(processResult{stdout: `{}`})
		if err := repository.ClearWorkflowBase(context.Background(), identity, name); err != nil {
			t.Fatal(err)
		}
		if len(runner.calls) != 1 {
			t.Fatalf("ClearWorkflowBase() calls = %d, want 1", len(runner.calls))
		}
	})

	t.Run("clear classifies metadata write failure", func(t *testing.T) {
		repository, _ := coverageRepository(
			processResult{stdout: `{"hotfix/ABC-999-payment-timeout":"origin/main"}`},
			processResult{err: errors.New("config write failed"), exitCode: 128},
		)
		err := repository.ClearWorkflowBase(context.Background(), identity, name)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("clear preserves other entries", func(t *testing.T) {
		repository, runner := coverageRepository(
			processResult{stdout: `{"feature/ABC-123-existing":"origin/develop","hotfix/ABC-999-payment-timeout":"origin/main"}`},
			processResult{},
		)
		if err := repository.ClearWorkflowBase(context.Background(), identity, name); err != nil {
			t.Fatal(err)
		}
		assertCall(
			t,
			runner.calls[1],
			identity.Root,
			"",
			"config",
			"--local",
			workflowBasesConfigKey,
			`{"feature/ABC-123-existing":"origin/develop"}`,
		)
	})

	t.Run("workflow base classifies metadata read failure", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{err: errors.New("config read failed"), exitCode: 128})
		_, _, err := repository.WorkflowBase(context.Background(), identity, name)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("workflow base reports an absent entry", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{stdout: `{}`})
		actual, found, err := repository.WorkflowBase(context.Background(), identity, name)
		if err != nil || found || actual.String() != "" {
			t.Fatalf("WorkflowBase() = (%q, %t, %v)", actual.String(), found, err)
		}
	})

	t.Run("workflow base rejects malformed JSON", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{stdout: `not-json`})
		_, _, err := repository.WorkflowBase(context.Background(), identity, name)
		assertProblemCode(t, err, problem.CodeConfigurationInvalid)
	})

	t.Run("workflow base treats null JSON as empty metadata", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{stdout: `null`})
		actual, found, err := repository.WorkflowBase(context.Background(), identity, name)
		if err != nil || found || actual.String() != "" {
			t.Fatalf("WorkflowBase() = (%q, %t, %v)", actual.String(), found, err)
		}
	})

	t.Run("workflow base rejects another remote", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{
			stdout: `{"hotfix/ABC-999-payment-timeout":"upstream/main"}`,
		})
		_, _, err := repository.WorkflowBase(context.Background(), identity, name)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})

	t.Run("workflow base rejects malformed branch metadata", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{
			stdout: `{"hotfix/ABC-999-payment-timeout":"origin/not-a-canonical-branch"}`,
		})
		_, _, err := repository.WorkflowBase(context.Background(), identity, name)
		assertProblemCode(t, err, problem.CodeBranchNameInvalid)
	})
}

func TestRepositoryCoverageReadParsingAndClassification(t *testing.T) {
	t.Parallel()

	identity := testIdentity()
	name := coverageBranch(t, "feature/ABC-123-add-export")
	base := coverageBase(t, "origin", "develop")

	t.Run("publication state classifies an indeterminate result", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{err: errors.New("show-ref failed"), exitCode: 128})
		actual, err := repository.PublicationState(context.Background(), identity, name)
		if actual != branch.PublicationUnknown {
			t.Fatalf("PublicationState() = %q, want unknown", actual)
		}
		assertProblemCategory(t, err, problem.CodeBranchPublicationUnknown, problem.CategoryRepository)
	})

	t.Run("missing base comparison classifies command failure", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{err: errors.New("rev-list failed"), exitCode: 128})
		_, err := repository.HasMissingBaseCommits(context.Background(), identity, base)
		assertProblemCategory(t, err, problem.CodeBranchBaseInvalid, problem.CategoryRepository)
	})

	t.Run("missing base comparison rejects a negative count", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{stdout: "-1\n"})
		_, err := repository.HasMissingBaseCommits(context.Background(), identity, base)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})

	t.Run("commit messages preserve complete messages", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{
			stdout: "\n\x1efirst line\nsecond line\n\x00\x1esecond message\x00",
		})
		actual, err := repository.CommitMessagesSince(context.Background(), identity, base)
		if err != nil {
			t.Fatal(err)
		}
		if len(actual) != 2 || actual[0] != "first line\nsecond line" || actual[1] != "second message" {
			t.Fatalf("CommitMessagesSince() = %#v", actual)
		}
	})

	t.Run("empty commit history output is empty", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{})
		actual, err := repository.CommitMessagesSince(context.Background(), identity, base)
		if err != nil || actual != nil {
			t.Fatalf("CommitMessagesSince() = %#v, %v", actual, err)
		}
	})

	t.Run("commit messages classify command failure", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{err: errors.New("log failed"), exitCode: 128})
		_, err := repository.CommitMessagesSince(context.Background(), identity, base)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("commit messages reject malformed Git output", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{stdout: "not record separated\x00"})
		_, err := repository.CommitMessagesSince(context.Background(), identity, base)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("release tags ignore blank output records", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{stdout: "\n v2.8.0 \n \n v2.8.1 \n"})
		actual, err := repository.ReleaseTagsAt(context.Background(), identity, "origin/main")
		if err != nil {
			t.Fatal(err)
		}
		if len(actual) != 2 || actual[0] != "v2.8.0" || actual[1] != "v2.8.1" {
			t.Fatalf("ReleaseTagsAt() = %#v", actual)
		}
	})
}

func TestRepositoryCoveragePushInspectionFailuresAndObjectIDs(t *testing.T) {
	t.Parallel()

	identity := testIdentity()
	base := coverageBase(t, "origin", "develop")
	localObjectID := strings.Repeat("a", 40)

	t.Run("missing base lookup failure short-circuits", func(t *testing.T) {
		repository, runner := coverageRepository(processResult{err: errors.New("rev-list failed"), exitCode: 128})
		_, err := repository.InspectPushUpdate(context.Background(), identity, base, localObjectID, strings.Repeat("b", 40))
		assertProblemCategory(t, err, problem.CodeBranchBaseInvalid, problem.CategoryRepository)
		if len(runner.calls) != 1 {
			t.Fatalf("InspectPushUpdate() calls = %d, want 1", len(runner.calls))
		}
	})

	t.Run("negative missing base count is rejected", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{stdout: "-1"})
		_, err := repository.InspectPushUpdate(context.Background(), identity, base, localObjectID, strings.Repeat("b", 40))
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})

	t.Run("commit log command failure short-circuits", func(t *testing.T) {
		repository, runner := coverageRepository(
			processResult{stdout: "0"},
			processResult{err: errors.New("log failed"), exitCode: 128},
		)
		_, err := repository.InspectPushUpdate(context.Background(), identity, base, localObjectID, strings.Repeat("b", 40))
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
		if len(runner.calls) != 2 {
			t.Fatalf("InspectPushUpdate() calls = %d, want 2", len(runner.calls))
		}
	})

	t.Run("malformed commit log output short-circuits", func(t *testing.T) {
		repository, _ := coverageRepository(
			processResult{stdout: "0"},
			processResult{stdout: "malformed log output\x00"},
		)
		_, err := repository.InspectPushUpdate(context.Background(), identity, base, localObjectID, strings.Repeat("b", 40))
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("unexpected ancestor failure is classified", func(t *testing.T) {
		repository, _ := coverageRepository(
			processResult{stdout: "0"},
			processResult{stdout: "\x1efeat(ABC-123): add export\x00"},
			processResult{err: errors.New("merge-base failed"), exitCode: 128},
		)
		_, err := repository.InspectPushUpdate(context.Background(), identity, base, localObjectID, strings.Repeat("b", 40))
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("empty remote ID is inspected rather than treated as branch creation", func(t *testing.T) {
		repository, runner := coverageRepository(
			processResult{stdout: "0"},
			processResult{stdout: "\x1efeat(ABC-123): add export\x00"},
			processResult{err: errors.New("not ancestor"), exitCode: 1},
		)
		inspection, err := repository.InspectPushUpdate(context.Background(), identity, base, localObjectID, "")
		if err != nil || inspection.FastForward {
			t.Fatalf("InspectPushUpdate() = %#v, %v", inspection, err)
		}
		if len(runner.calls) != 3 {
			t.Fatalf("InspectPushUpdate() calls = %d, want 3", len(runner.calls))
		}
		assertCall(t, runner.calls[2], identity.Root, "", "merge-base", "--is-ancestor", "", localObjectID)
	})

	for _, testCase := range []struct {
		name  string
		value string
		want  bool
	}{
		{name: "empty", value: "", want: false},
		{name: "nonzero", value: strings.Repeat("a", 40), want: false},
		{name: "sha1 zero", value: strings.Repeat("0", 40), want: true},
		{name: "sha256 zero", value: strings.Repeat("0", 64), want: true},
	} {
		testCase := testCase
		t.Run("zero object ID "+testCase.name, func(t *testing.T) {
			if actual := zeroObjectID(testCase.value); actual != testCase.want {
				t.Fatalf("zeroObjectID(%q) = %t, want %t", testCase.value, actual, testCase.want)
			}
		})
	}
}

func TestRepositoryCoverageCommandDiagnosticsAndCancellation(t *testing.T) {
	t.Parallel()

	t.Run("command cause handles all result forms", func(t *testing.T) {
		if cause := commandCause(processResult{}); cause != nil {
			t.Fatalf("commandCause(success) = %v, want nil", cause)
		}
		if cause := commandCause(processResult{err: errors.New("failed"), exitCode: 42}); cause == nil || cause.Error() != "git exited with status 42" {
			t.Fatalf("commandCause(exit) = %v", cause)
		}
		if cause := commandCause(processResult{err: errors.New("missing executable"), exitCode: -1}); cause == nil || cause.Error() != "git process could not be started" {
			t.Fatalf("commandCause(start) = %v", cause)
		}
	})

	t.Run("truncated diagnostics retain bounded context", func(t *testing.T) {
		repository, _ := coverageRepository()
		err := repository.commandProblem(
			problem.CodeBranchPublicationUnknown,
			testIdentity(),
			"inspect publication",
			processResult{err: errors.New("failed"), exitCode: 128, truncated: true},
		)
		actual := assertProblemCategory(t, err, problem.CodeBranchPublicationUnknown, problem.CategoryRepository)
		for _, expected := range []string{
			"action=inspect publication",
			"repository=C:/repo",
			"remote=origin",
		} {
			if !strings.Contains(actual.Context, expected) {
				t.Fatalf("problem context %q does not contain %q", actual.Context, expected)
			}
		}
		if actual.Diagnostic != "[Git diagnostic output was truncated]" {
			t.Fatalf("problem diagnostic = %q", actual.Diagnostic)
		}
	})

	t.Run("caller cancellation is externally classified", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		repository := &Repository{runner: waitingRunner{}, timeout: time.Second}

		err := repository.Fetch(ctx, testIdentity())
		assertProblemCategory(t, err, problem.CodeExternalCommandFailed, problem.CategoryExternal)
	})

	t.Run("wrapped cancellation is externally classified", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{
			err:      fmt.Errorf("runner failed: %w", context.Canceled),
			exitCode: -1,
		})

		err := repository.Fetch(context.Background(), testIdentity())
		assertProblemCategory(t, err, problem.CodeExternalCommandFailed, problem.CategoryExternal)
	})
}

func TestRepositoryCoveragePromotionMergeRecovery(t *testing.T) {
	t.Parallel()

	identity := testIdentity()
	main := coverageBase(t, "origin", "main")

	t.Run("continues a resolved merge without an interactive editor", func(t *testing.T) {
		repository, runner := coverageRepository(processResult{})
		if err := repository.ContinueMerge(context.Background(), identity); err != nil {
			t.Fatal(err)
		}
		assertCall(t, runner.calls[0], "C:/repo", "", "-c", "core.editor=true", "merge", "--continue")

		repository, _ = coverageRepository(processResult{err: errors.New("continue failed"), exitCode: 1})
		err := repository.ContinueMerge(context.Background(), identity)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("compares the active merge target with the selected base", func(t *testing.T) {
		repository, runner := coverageRepository(
			processResult{stdout: "abc123\n"},
			processResult{stdout: "abc123\n"},
		)
		matches, err := repository.ActiveMergeTargetMatches(context.Background(), identity, main)
		if err != nil || !matches {
			t.Fatalf("ActiveMergeTargetMatches(match) = (%t, %v)", matches, err)
		}
		assertCall(t, runner.calls[0], "C:/repo", "", "rev-parse", "--verify", "MERGE_HEAD^{commit}")
		assertCall(t, runner.calls[1], "C:/repo", "", "rev-parse", "--verify", "origin/main^{commit}")

		repository, _ = coverageRepository(
			processResult{stdout: "abc123\n"},
			processResult{stdout: "def456\n"},
		)
		matches, err = repository.ActiveMergeTargetMatches(context.Background(), identity, main)
		if err != nil || matches {
			t.Fatalf("ActiveMergeTargetMatches(mismatch) = (%t, %v)", matches, err)
		}

		repository, _ = coverageRepository(processResult{err: errors.New("missing merge head"), exitCode: 1})
		_, err = repository.ActiveMergeTargetMatches(context.Background(), identity, main)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)

		repository, _ = coverageRepository(processResult{})
		_, err = repository.ActiveMergeTargetMatches(context.Background(), identity, main)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)

		repository, _ = coverageRepository(
			processResult{stdout: "abc123\n"},
			processResult{err: errors.New("missing main"), exitCode: 1},
		)
		_, err = repository.ActiveMergeTargetMatches(context.Background(), identity, main)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("proves exact reconciliation merge provenance", func(t *testing.T) {
		release := coverageBase(t, "origin", "release/1.0.1")
		develop := coverageBase(t, "origin", "develop")
		repository, runner := coverageRepository(
			processResult{stdout: "release-sha\n"},
			processResult{stdout: "develop-sha\n"},
			processResult{stdout: "release-sha develop-sha\n"},
		)

		matches, err := repository.HeadIsMergeOf(context.Background(), identity, release, develop)
		if err != nil || !matches {
			t.Fatalf("HeadIsMergeOf(match) = (%t, %v)", matches, err)
		}
		assertCall(t, runner.calls[0], "C:/repo", "", "rev-parse", "--verify", "origin/release/1.0.1^{commit}")
		assertCall(t, runner.calls[1], "C:/repo", "", "rev-parse", "--verify", "origin/develop^{commit}")
		assertCall(t, runner.calls[2], "C:/repo", "", "show", "-s", "--format=%P", "HEAD")

		repository, _ = coverageRepository(
			processResult{stdout: "release-sha\n"},
			processResult{stdout: "develop-sha\n"},
			processResult{stdout: "develop-sha release-sha\n"},
		)
		matches, err = repository.HeadIsMergeOf(context.Background(), identity, release, develop)
		if err != nil || matches {
			t.Fatalf("HeadIsMergeOf(mismatch) = (%t, %v)", matches, err)
		}

		repository, _ = coverageRepository(processResult{err: errors.New("missing release"), exitCode: 1})
		_, _, err = repository.ResolveReconciliationBases(context.Background(), identity, release, develop)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)

		repository, _ = coverageRepository(
			processResult{stdout: "release-sha\n"},
			processResult{err: errors.New("missing develop"), exitCode: 1},
		)
		_, _, err = repository.ResolveReconciliationBases(context.Background(), identity, release, develop)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)

		repository, _ = coverageRepository(
			processResult{stdout: "release-sha\n"},
			processResult{stdout: "develop-sha\n"},
			processResult{err: errors.New("cannot inspect parents"), exitCode: 1},
		)
		_, err = repository.HeadIsMergeOf(context.Background(), identity, release, develop)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)

		repository, _ = coverageRepository(processResult{err: errors.New("missing release"), exitCode: 1})
		_, err = repository.HeadIsMergeOf(context.Background(), identity, release, develop)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})
}
