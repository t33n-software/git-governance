package gitcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestInstallHooks(t *testing.T) {
	t.Parallel()

	identity := func(root string) port.RepositoryIdentity {
		return port.RepositoryIdentity{Root: root, Remote: "origin"}
	}
	writeHooks := func(t *testing.T, hooksDirectory string, hooks ...string) {
		t.Helper()
		if err := os.MkdirAll(hooksDirectory, 0o700); err != nil {
			t.Fatal(err)
		}
		for _, hook := range hooks {
			if err := os.WriteFile(filepath.Join(hooksDirectory, hook), []byte("#!/bin/sh\n"), 0o700); err != nil {
				t.Fatal(err)
			}
		}
	}
	// captureProgram wires a program runner that records the executable the
	// adapter resolves for each invocation.
	captureProgram := func(program *fakeRunner) (func(string) processRunner, *string) {
		captured := ""
		return func(executable string) processRunner {
			captured = executable
			return program
		}, &captured
	}

	t.Run("installs through the repository tool declaration", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		gitDirectory := filepath.Join(root, ".git")
		writeHooks(t, filepath.Join(gitDirectory, "hooks"), "commit-msg", "pre-push")

		runner := &fakeRunner{results: []processResult{{stdout: gitDirectory + "\n"}}}
		program := &fakeRunner{results: []processResult{{}}}
		programRunner, captured := captureProgram(program)
		repository := &Repository{
			runner:        runner,
			timeout:       time.Second,
			programRunner: programRunner,
		}
		installation, err := repository.InstallHooks(
			context.Background(),
			identity(root),
			port.HookManagerInvocation{Executable: "go", Arguments: []string{"tool", "lefthook"}},
		)
		if err != nil {
			t.Fatal(err)
		}
		if installation.Directory != filepath.Join(gitDirectory, "hooks") ||
			len(installation.Hooks) != 2 || installation.Hooks[0] != "commit-msg" || installation.Hooks[1] != "pre-push" {
			t.Fatalf("InstallHooks() = %#v", installation)
		}
		if *captured != "go" {
			t.Fatalf("resolved executable = %q, want go", *captured)
		}
		if program.calls[0].directory != root ||
			strings.Join(program.calls[0].arguments, " ") != "tool lefthook install" {
			t.Fatalf("project tool call = %#v", program.calls[0])
		}
	})

	t.Run("installs through the host executable for the zero invocation", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		gitDirectory := filepath.Join(root, ".git")
		writeHooks(t, filepath.Join(gitDirectory, "hooks"), "commit-msg", "pre-push")

		runner := &fakeRunner{results: []processResult{{stdout: gitDirectory + "\n"}}}
		program := &fakeRunner{results: []processResult{{}}}
		programRunner, captured := captureProgram(program)
		repository := &Repository{
			runner:        runner,
			timeout:       time.Second,
			programRunner: programRunner,
		}
		installation, err := repository.InstallHooks(context.Background(), identity(root), port.HookManagerInvocation{})
		if err != nil {
			t.Fatal(err)
		}
		if installation.Directory != filepath.Join(gitDirectory, "hooks") {
			t.Fatalf("InstallHooks() directory = %q", installation.Directory)
		}
		if *captured != "lefthook" {
			t.Fatalf("resolved executable = %q, want lefthook", *captured)
		}
		if strings.Join(program.calls[0].arguments, " ") != "install" {
			t.Fatalf("host executable call = %#v", program.calls[0])
		}
	})

	t.Run("resolves a relative git directory against the repository root", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeHooks(t, filepath.Join(root, ".git", "hooks"), "commit-msg", "pre-push")

		runner := &fakeRunner{results: []processResult{{stdout: ".git\n"}}}
		program := &fakeRunner{results: []processResult{{}}}
		repository := &Repository{
			runner:        runner,
			timeout:       time.Second,
			programRunner: func(string) processRunner { return program },
		}
		installation, err := repository.InstallHooks(context.Background(), identity(root), port.HookManagerInvocation{})
		if err != nil {
			t.Fatal(err)
		}
		if installation.Directory != filepath.Join(root, ".git", "hooks") {
			t.Fatalf("InstallHooks() directory = %q", installation.Directory)
		}
	})

	t.Run("git directory resolution failures fail closed", func(t *testing.T) {
		t.Parallel()
		repository := &Repository{
			runner:  &fakeRunner{results: []processResult{{err: errors.New("failed"), exitCode: 128}}},
			timeout: time.Second,
		}
		_, err := repository.InstallHooks(context.Background(), identity(t.TempDir()), port.HookManagerInvocation{})
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("an empty git directory answer fails closed", func(t *testing.T) {
		t.Parallel()
		repository := &Repository{
			runner:  &fakeRunner{results: []processResult{{stdout: "\n"}}},
			timeout: time.Second,
		}
		_, err := repository.InstallHooks(context.Background(), identity(t.TempDir()), port.HookManagerInvocation{})
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("a failed hook manager installation fails closed", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		gitDirectory := filepath.Join(root, ".git")
		runner := &fakeRunner{results: []processResult{{stdout: gitDirectory + "\n"}}}
		program := &fakeRunner{results: []processResult{{stderr: "lefthook failed", err: errors.New("failed"), exitCode: 1}}}
		repository := &Repository{
			runner:        runner,
			timeout:       time.Second,
			programRunner: func(string) processRunner { return program },
		}
		_, err := repository.InstallHooks(context.Background(), identity(root), port.HookManagerInvocation{})
		assertProblemCode(t, err, problem.CodeExternalCommandFailed)
	})

	t.Run("a missing canonical hook file fails the proof", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		gitDirectory := filepath.Join(root, ".git")
		writeHooks(t, filepath.Join(gitDirectory, "hooks"), "commit-msg")

		runner := &fakeRunner{results: []processResult{{stdout: gitDirectory + "\n"}}}
		program := &fakeRunner{results: []processResult{{}}}
		repository := &Repository{
			runner:        runner,
			timeout:       time.Second,
			programRunner: func(string) processRunner { return program },
		}
		_, err := repository.InstallHooks(context.Background(), identity(root), port.HookManagerInvocation{})
		assertProblemCode(t, err, problem.CodeExternalCommandFailed)
	})
}
