package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// bootstrapCommandGit binds the unborn repository state the governed birth
// requires: no commits, no references, HEAD on main, and the born refs
// existing after the mutation.
func bootstrapCommandGit(t *testing.T) *commandGit {
	t.Helper()
	git := newCommandGit(t, "main", nil)
	git.hasCommits = false
	git.stagedQueue = []bool{false, true}
	git.existingBranches = map[string]bool{"main": true, "develop": true}
	return git
}

func TestBootstrapCommandHelpSurface(t *testing.T) {
	t.Parallel()

	command := New(BuildInfo{Version: "test"})
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{"workflow", "bootstrap", "--help"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"--key", "--ticket", "--stage", "--push"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("bootstrap help missing %q: %q", expected, output.String())
		}
	}
}

func TestBootstrapCommandInputAndConfirmationGates(t *testing.T) {
	t.Parallel()

	t.Run("discovery failures propagate", func(t *testing.T) {
		t.Parallel()
		git := bootstrapCommandGit(t)
		git.discoverErr = errors.New("no repository")
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs([]string{"--interactive", "never", "workflow", "bootstrap", "--key", "ABC", "--ticket", "1", "--stage", "."})
		if err := command.ExecuteContext(context.Background()); err == nil {
			t.Fatal("expected the discovery failure to propagate")
		}
	})

	t.Run("the ticket key is mandatory non-interactively", func(t *testing.T) {
		t.Parallel()
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(bootstrapCommandGit(t)))
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs([]string{"--interactive", "never", "workflow", "bootstrap", "--ticket", "1", "--stage", "."})
		assertProblemCode(t, command.ExecuteContext(context.Background()), problem.CodeInvalidInput)
	})

	t.Run("the ticket number is mandatory non-interactively", func(t *testing.T) {
		t.Parallel()
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(bootstrapCommandGit(t)))
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs([]string{"--interactive", "never", "workflow", "bootstrap", "--key", "ABC", "--stage", "."})
		assertProblemCode(t, command.ExecuteContext(context.Background()), problem.CodeInvalidInput)
	})

	t.Run("the mutation requires confirmation", func(t *testing.T) {
		t.Parallel()
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(bootstrapCommandGit(t)))
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs([]string{"--interactive", "never", "workflow", "bootstrap", "--key", "ABC", "--ticket", "1", "--stage", "."})
		assertProblemCode(t, command.ExecuteContext(context.Background()), problem.CodeInvalidInput)
	})

	t.Run("a born repository is refused", func(t *testing.T) {
		t.Parallel()
		git := bootstrapCommandGit(t)
		git.anyRef = true
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs([]string{"--interactive", "never", "--yes", "workflow", "bootstrap", "--key", "ABC", "--ticket", "1", "--stage", "."})
		assertProblemCode(t, command.ExecuteContext(context.Background()), problem.CodeRepositoryAlreadyBorn)
	})
}

func TestBootstrapCommandDryRunContract(t *testing.T) {
	t.Parallel()

	git := bootstrapCommandGit(t)
	command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{
		"--interactive", "never", "--dry-run", "--output", "json",
		"workflow", "bootstrap", "--key", "ABC", "--ticket", "1", "--stage", ".", "--push",
	})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`"ok":true`,
		`"operation":"workflow.bootstrap"`,
		`"dryRun":"true"`,
		`"published":"false"`,
		"preflight",
		"publish",
		"chore(ABC-1): initialize the governed repository",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("dry-run output missing %q: %q", expected, output.String())
		}
	}
	if strings.Contains(output.String(), `"revision"`) {
		t.Fatalf("dry-run output must not carry a birth record: %q", output.String())
	}
	if len(git.pushed) != 0 {
		t.Fatalf("dry-run pushed %v", git.pushed)
	}
}

func TestBootstrapCommandCompletesTheLocalGenesis(t *testing.T) {
	t.Parallel()

	git := bootstrapCommandGit(t)
	command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{
		"--interactive", "never", "--output", "json", "--yes",
		"workflow", "bootstrap", "--key", "ABC", "--ticket", "1", "--stage", ".",
	})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`"ok":true`,
		`"operation":"workflow.bootstrap"`,
		`"revision":"0123456789abcdef0123456789abcdef01234567"`,
		`"signatureVerified":"true"`,
		`"actor":"lane@example.invalid"`,
		`"published":"false"`,
		`"ticket":"ABC-1"`,
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("bootstrap output missing %q: %q", expected, output.String())
		}
	}
	if len(git.pushed) != 0 {
		t.Fatalf("the local genesis must not publish: pushed %v", git.pushed)
	}
	if len(git.operationTimeouts) != 1 {
		t.Fatalf("the genesis mutation budget was not applied: %v", git.operationTimeouts)
	}
	if git.operationTimeouts[0] <= 5*time.Second {
		t.Fatalf("the derived mutation budget collapsed to the flat default: %v", git.operationTimeouts[0])
	}
}

func TestBootstrapCommandCapsTheDerivedBudgetWithTheExplicitTimeout(t *testing.T) {
	t.Parallel()

	git := bootstrapCommandGit(t)
	command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{
		"--interactive", "never", "--output", "json", "--yes", "--timeout", "5s",
		"workflow", "bootstrap", "--key", "ABC", "--ticket", "1", "--stage", ".",
	})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(git.operationTimeouts) != 1 || git.operationTimeouts[0] != 5*time.Second {
		t.Fatalf("the explicit timeout cap did not reach the genesis mutation: %v", git.operationTimeouts)
	}
}

func TestBootstrapCommandPublication(t *testing.T) {
	t.Parallel()

	t.Run("non-interactive push publishes both shared lines", func(t *testing.T) {
		t.Parallel()
		git := bootstrapCommandGit(t)
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		output := &bytes.Buffer{}
		command.SetOut(output)
		command.SetErr(output)
		command.SetArgs([]string{
			"--interactive", "never", "--output", "json", "--yes",
			"workflow", "bootstrap", "--key", "ABC", "--ticket", "1", "--stage", ".", "--push",
		})
		if err := command.ExecuteContext(context.Background()); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), `"published":"true"`) {
			t.Fatalf("publication output = %q", output.String())
		}
		if len(git.pushed) != 2 || git.pushed[0].String() != "main" || git.pushed[1].String() != "develop" {
			t.Fatalf("pushed = %v", git.pushed)
		}
	})

	t.Run("the interactive publication is separately confirmed", func(t *testing.T) {
		t.Parallel()
		git := bootstrapCommandGit(t)
		prompt := &commandHelperPrompt{confirms: []commandHelperConfirmReply{{value: true}, {value: true}}}
		runtime := commandRuntime(git)
		runtime.PromptFactory = func(bool, string) port.Prompt { return prompt }
		runtime.InputIsTerminal = func() bool { return true }
		runtime.OutputIsTerminal = func() bool { return true }
		command := NewWithRuntime(BuildInfo{Version: "test"}, runtime)
		output := &bytes.Buffer{}
		command.SetOut(output)
		command.SetErr(output)
		command.SetArgs([]string{"--color", "never", "workflow", "bootstrap", "--key", "ABC", "--ticket", "1", "--stage", ".", "--push"})
		if err := command.ExecuteContext(context.Background()); err != nil {
			t.Fatal(err)
		}
		if len(prompt.confirmRequests) != 2 {
			t.Fatalf("expected the genesis and the publication confirmation, got %d", len(prompt.confirmRequests))
		}
		if len(git.pushed) != 2 {
			t.Fatalf("pushed = %v", git.pushed)
		}
	})

	t.Run("a declined publication keeps the local genesis", func(t *testing.T) {
		t.Parallel()
		git := bootstrapCommandGit(t)
		prompt := &commandHelperPrompt{confirms: []commandHelperConfirmReply{{value: true}, {value: false}}}
		runtime := commandRuntime(git)
		runtime.PromptFactory = func(bool, string) port.Prompt { return prompt }
		runtime.InputIsTerminal = func() bool { return true }
		runtime.OutputIsTerminal = func() bool { return true }
		command := NewWithRuntime(BuildInfo{Version: "test"}, runtime)
		output := &bytes.Buffer{}
		command.SetOut(output)
		command.SetErr(output)
		command.SetArgs([]string{"--color", "never", "workflow", "bootstrap", "--key", "ABC", "--ticket", "1", "--stage", ".", "--push"})
		if err := command.ExecuteContext(context.Background()); err != nil {
			t.Fatal(err)
		}
		if len(git.pushed) != 0 {
			t.Fatalf("a declined publication must not push: %v", git.pushed)
		}
		if !strings.Contains(output.String(), "published: false") {
			t.Fatalf("declined publication output = %q", output.String())
		}
	})

	t.Run("a publication confirmation failure propagates", func(t *testing.T) {
		t.Parallel()
		git := bootstrapCommandGit(t)
		prompt := &commandHelperPrompt{confirms: []commandHelperConfirmReply{{value: true}, {err: errors.New("prompt failed")}}}
		runtime := commandRuntime(git)
		runtime.PromptFactory = func(bool, string) port.Prompt { return prompt }
		runtime.InputIsTerminal = func() bool { return true }
		runtime.OutputIsTerminal = func() bool { return true }
		command := NewWithRuntime(BuildInfo{Version: "test"}, runtime)
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs([]string{"workflow", "bootstrap", "--key", "ABC", "--ticket", "1", "--stage", ".", "--push"})
		if err := command.ExecuteContext(context.Background()); err == nil {
			t.Fatal("expected the confirmation failure to propagate")
		}
	})

	t.Run("a publication failure propagates", func(t *testing.T) {
		t.Parallel()
		git := bootstrapCommandGit(t)
		git.pushErr = problem.New(problem.Details{
			Code:        problem.CodeGitCommandFailed,
			Category:    problem.CategoryGit,
			Field:       "git operation",
			Expected:    "a successful Git operation",
			Rule:        "Git operations must complete successfully before the workflow can continue",
			Remediation: "review the Git diagnostic, correct the repository state, and retry",
		})
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs([]string{
			"--interactive", "never", "--yes",
			"workflow", "bootstrap", "--key", "ABC", "--ticket", "1", "--stage", ".", "--push",
		})
		assertProblemCode(t, command.ExecuteContext(context.Background()), problem.CodeGitCommandFailed)
	})
}

// bornCommandGit binds the born repository state the publication resume
// requires: exactly one genesis commit, both shared lines on the shared
// revision, and the materialized hook boundary.
func bornCommandGit(t *testing.T) *commandGit {
	t.Helper()
	git := newCommandGit(t, "main", nil)
	git.existingBranches = map[string]bool{"main": true, "develop": true}
	return git
}

func TestBootstrapSubcommandHelpSurface(t *testing.T) {
	t.Parallel()

	command := New(BuildInfo{Version: "test"})
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{"workflow", "bootstrap", "--help"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"recover", "publish"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("bootstrap help missing the %q subcommand: %q", expected, output.String())
		}
	}
}

func TestBootstrapRecoverCommand(t *testing.T) {
	t.Parallel()

	t.Run("the mutation requires confirmation", func(t *testing.T) {
		t.Parallel()
		git := newCommandGit(t, "main", nil)
		git.hasCommits = false
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs([]string{"--interactive", "never", "workflow", "bootstrap", "recover"})
		assertProblemCode(t, command.ExecuteContext(context.Background()), problem.CodeInvalidInput)
	})

	t.Run("a born repository is refused", func(t *testing.T) {
		t.Parallel()
		git := newCommandGit(t, "main", nil)
		git.hasCommits = true
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs([]string{"--interactive", "never", "--yes", "workflow", "bootstrap", "recover"})
		assertProblemCode(t, command.ExecuteContext(context.Background()), problem.CodeRepositoryAlreadyBorn)
	})

	t.Run("the dry run proves the state without restoring it", func(t *testing.T) {
		t.Parallel()
		git := newCommandGit(t, "main", nil)
		git.hasCommits = false
		git.stagedQueue = []bool{true}
		git.anyRef = true
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		output := &bytes.Buffer{}
		command.SetOut(output)
		command.SetErr(output)
		command.SetArgs([]string{
			"--interactive", "never", "--dry-run", "--output", "json",
			"workflow", "bootstrap", "recover",
		})
		if err := command.ExecuteContext(context.Background()); err != nil {
			t.Fatal(err)
		}
		for _, expected := range []string{
			`"ok":true`,
			`"operation":"workflow.bootstrap.recover"`,
			`"dryRun":"true"`,
			"prove-unborn",
			"empty-index",
			"remove-references",
			"prove-pre-state",
		} {
			if !strings.Contains(output.String(), expected) {
				t.Fatalf("recovery dry-run output missing %q: %q", expected, output.String())
			}
		}
	})

	t.Run("the recovery restores and proves the pre-state", func(t *testing.T) {
		t.Parallel()
		git := newCommandGit(t, "main", nil)
		git.hasCommits = false
		git.stagedQueue = []bool{true, false}
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		output := &bytes.Buffer{}
		command.SetOut(output)
		command.SetErr(output)
		command.SetArgs([]string{
			"--interactive", "never", "--output", "json", "--yes",
			"workflow", "bootstrap", "recover",
		})
		if err := command.ExecuteContext(context.Background()); err != nil {
			t.Fatal(err)
		}
		for _, expected := range []string{
			`"ok":true`,
			`"operation":"workflow.bootstrap.recover"`,
			`"indexEmptied":"true"`,
			`"dryRun":"false"`,
		} {
			if !strings.Contains(output.String(), expected) {
				t.Fatalf("recovery output missing %q: %q", expected, output.String())
			}
		}
	})

	t.Run("a restoration failure propagates", func(t *testing.T) {
		t.Parallel()
		git := newCommandGit(t, "main", nil)
		git.hasCommits = false
		git.stagedQueue = []bool{true}
		git.restoreErr = errors.New("index.lock persists")
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs([]string{"--interactive", "never", "--yes", "workflow", "bootstrap", "recover"})
		if err := command.ExecuteContext(context.Background()); err == nil {
			t.Fatal("expected the restoration failure to propagate")
		}
	})

	t.Run("discovery failures propagate", func(t *testing.T) {
		t.Parallel()
		git := newCommandGit(t, "main", nil)
		git.discoverErr = errors.New("no repository")
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs([]string{"--interactive", "never", "--yes", "workflow", "bootstrap", "recover"})
		if err := command.ExecuteContext(context.Background()); err == nil {
			t.Fatal("expected the discovery failure to propagate")
		}
	})

	t.Run("a confirmation failure propagates", func(t *testing.T) {
		t.Parallel()
		git := newCommandGit(t, "main", nil)
		git.hasCommits = false
		prompt := &commandHelperPrompt{confirms: []commandHelperConfirmReply{{err: errors.New("prompt failed")}}}
		runtime := commandRuntime(git)
		runtime.PromptFactory = func(bool, string) port.Prompt { return prompt }
		runtime.InputIsTerminal = func() bool { return true }
		runtime.OutputIsTerminal = func() bool { return true }
		command := NewWithRuntime(BuildInfo{Version: "test"}, runtime)
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs([]string{"workflow", "bootstrap", "recover"})
		if err := command.ExecuteContext(context.Background()); err == nil {
			t.Fatal("expected the confirmation failure to propagate")
		}
	})
}

func TestBootstrapPublishCommand(t *testing.T) {
	t.Parallel()

	t.Run("the publication requires confirmation", func(t *testing.T) {
		t.Parallel()
		git := bornCommandGit(t)
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs([]string{"--interactive", "never", "workflow", "bootstrap", "publish"})
		assertProblemCode(t, command.ExecuteContext(context.Background()), problem.CodeInvalidInput)
	})

	t.Run("a repository outside the birth topology is refused", func(t *testing.T) {
		t.Parallel()
		git := bornCommandGit(t)
		git.headCommits = 4
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs([]string{"--interactive", "never", "--yes", "workflow", "bootstrap", "publish"})
		assertProblemCode(t, command.ExecuteContext(context.Background()), problem.CodeBirthStateInvalid)
		if len(git.pushed) != 0 {
			t.Fatalf("the refused resume pushed: %v", git.pushed)
		}
	})

	t.Run("the dry run proves the birth topology without pushing", func(t *testing.T) {
		t.Parallel()
		git := bornCommandGit(t)
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		output := &bytes.Buffer{}
		command.SetOut(output)
		command.SetErr(output)
		command.SetArgs([]string{
			"--interactive", "never", "--dry-run", "--output", "json",
			"workflow", "bootstrap", "publish",
		})
		if err := command.ExecuteContext(context.Background()); err != nil {
			t.Fatal(err)
		}
		for _, expected := range []string{
			`"ok":true`,
			`"operation":"workflow.bootstrap.publish"`,
			`"dryRun":"true"`,
			`"published":"false"`,
			`"revision":"0123456789abcdef0123456789abcdef01234567"`,
			"prove-birth",
			"publish",
		} {
			if !strings.Contains(output.String(), expected) {
				t.Fatalf("publication dry-run output missing %q: %q", expected, output.String())
			}
		}
		if len(git.pushed) != 0 {
			t.Fatalf("the dry run pushed %v", git.pushed)
		}
	})

	t.Run("the confirmed resume publishes both shared lines", func(t *testing.T) {
		t.Parallel()
		git := bornCommandGit(t)
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		output := &bytes.Buffer{}
		command.SetOut(output)
		command.SetErr(output)
		command.SetArgs([]string{
			"--interactive", "never", "--output", "json", "--yes",
			"workflow", "bootstrap", "publish",
		})
		if err := command.ExecuteContext(context.Background()); err != nil {
			t.Fatal(err)
		}
		for _, expected := range []string{
			`"ok":true`,
			`"operation":"workflow.bootstrap.publish"`,
			`"published":"true"`,
			`"dryRun":"false"`,
		} {
			if !strings.Contains(output.String(), expected) {
				t.Fatalf("publication output missing %q: %q", expected, output.String())
			}
		}
		if len(git.pushed) != 2 || git.pushed[0].String() != "main" || git.pushed[1].String() != "develop" {
			t.Fatalf("pushed = %v", git.pushed)
		}
	})

	t.Run("a push failure propagates", func(t *testing.T) {
		t.Parallel()
		git := bornCommandGit(t)
		git.pushErr = problem.New(problem.Details{
			Code:        problem.CodeGitCommandFailed,
			Category:    problem.CategoryGit,
			Field:       "git operation",
			Expected:    "a successful Git operation",
			Rule:        "Git operations must complete successfully before the workflow can continue",
			Remediation: "review the Git diagnostic, correct the repository state, and retry",
		})
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs([]string{"--interactive", "never", "--yes", "workflow", "bootstrap", "publish"})
		assertProblemCode(t, command.ExecuteContext(context.Background()), problem.CodeGitCommandFailed)
	})

	t.Run("discovery failures propagate", func(t *testing.T) {
		t.Parallel()
		git := bornCommandGit(t)
		git.discoverErr = errors.New("no repository")
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs([]string{"--interactive", "never", "--yes", "workflow", "bootstrap", "publish"})
		if err := command.ExecuteContext(context.Background()); err == nil {
			t.Fatal("expected the discovery failure to propagate")
		}
	})

	t.Run("a confirmation failure propagates", func(t *testing.T) {
		t.Parallel()
		git := bornCommandGit(t)
		prompt := &commandHelperPrompt{confirms: []commandHelperConfirmReply{{err: errors.New("prompt failed")}}}
		runtime := commandRuntime(git)
		runtime.PromptFactory = func(bool, string) port.Prompt { return prompt }
		runtime.InputIsTerminal = func() bool { return true }
		runtime.OutputIsTerminal = func() bool { return true }
		command := NewWithRuntime(BuildInfo{Version: "test"}, runtime)
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs([]string{"workflow", "bootstrap", "publish"})
		if err := command.ExecuteContext(context.Background()); err == nil {
			t.Fatal("expected the confirmation failure to propagate")
		}
	})
}
