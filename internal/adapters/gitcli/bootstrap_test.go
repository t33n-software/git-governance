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

func TestRestoreUnbornState(t *testing.T) {
	t.Parallel()

	t.Run("empties the index and deletes every born reference", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{
			{stdout: ""},
			{stdout: "refs/heads/main\nrefs/heads/develop\n"},
			{stdout: ""},
			{stdout: ""},
		}}
		repository := &Repository{runner: runner, timeout: time.Second}
		if err := repository.RestoreUnbornState(context.Background(), testIdentity()); err != nil {
			t.Fatal(err)
		}
		if len(runner.calls) != 4 {
			t.Fatalf("recorded calls = %#v", runner.calls)
		}
		if strings.Join(runner.calls[0].arguments, " ") != "read-tree --empty" {
			t.Fatalf("index restoration = %#v", runner.calls[0].arguments)
		}
		if strings.Join(runner.calls[1].arguments, " ") != "for-each-ref --format=%(refname)" {
			t.Fatalf("reference enumeration = %#v", runner.calls[1].arguments)
		}
		if strings.Join(runner.calls[2].arguments, " ") != "update-ref -d refs/heads/main" {
			t.Fatalf("first deletion = %#v", runner.calls[2].arguments)
		}
		if strings.Join(runner.calls[3].arguments, " ") != "update-ref -d refs/heads/develop" {
			t.Fatalf("second deletion = %#v", runner.calls[3].arguments)
		}
	})

	t.Run("an empty reference set deletes nothing", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{
			{stdout: ""},
			{stdout: "\n"},
		}}
		repository := &Repository{runner: runner, timeout: time.Second}
		if err := repository.RestoreUnbornState(context.Background(), testIdentity()); err != nil {
			t.Fatal(err)
		}
		if len(runner.calls) != 2 {
			t.Fatalf("an empty reference set must not delete: %#v", runner.calls)
		}
	})

	t.Run("index restoration failures fail closed", func(t *testing.T) {
		t.Parallel()
		repository := &Repository{
			runner:  &fakeRunner{results: []processResult{{err: errors.New("failed"), exitCode: 128}}},
			timeout: time.Second,
		}
		err := repository.RestoreUnbornState(context.Background(), testIdentity())
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("reference enumeration failures fail closed", func(t *testing.T) {
		t.Parallel()
		repository := &Repository{
			runner: &fakeRunner{results: []processResult{
				{stdout: ""},
				{err: errors.New("failed"), exitCode: 128},
			}},
			timeout: time.Second,
		}
		err := repository.RestoreUnbornState(context.Background(), testIdentity())
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("reference deletion failures fail closed", func(t *testing.T) {
		t.Parallel()
		repository := &Repository{
			runner: &fakeRunner{results: []processResult{
				{stdout: ""},
				{stdout: "refs/heads/main\n"},
				{err: errors.New("failed"), exitCode: 128},
			}},
			timeout: time.Second,
		}
		err := repository.RestoreUnbornState(context.Background(), testIdentity())
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})
}

func TestPreviewStage(t *testing.T) {
	t.Parallel()

	t.Run("rejects an empty path set", func(t *testing.T) {
		t.Parallel()
		repository := &Repository{runner: &fakeRunner{}, timeout: time.Second}
		_, err := repository.PreviewStage(context.Background(), testIdentity(), nil)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("parses the NUL-separated untracked-file preview", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{stdout: "README.md\x00docs/guide.md\x00"}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		preview, err := repository.PreviewStage(context.Background(), testIdentity(), []string{"."})
		if err != nil {
			t.Fatal(err)
		}
		if len(preview) != 2 || preview[0] != "README.md" || preview[1] != "docs/guide.md" {
			t.Fatalf("PreviewStage() = %#v", preview)
		}
		call := runner.calls[0]
		joined := strings.Join(call.arguments, " ")
		if !strings.Contains(joined, "ls-files") || !strings.Contains(joined, "--others") ||
			!strings.Contains(joined, "--exclude-standard") || !strings.Contains(joined, "-z") {
			t.Fatalf("PreviewStage() arguments = %#v", call.arguments)
		}
	})

	t.Run("an empty expansion is an empty preview", func(t *testing.T) {
		t.Parallel()
		repository := &Repository{runner: &fakeRunner{results: []processResult{{stdout: ""}}}, timeout: time.Second}
		preview, err := repository.PreviewStage(context.Background(), testIdentity(), []string{"."})
		if err != nil || len(preview) != 0 {
			t.Fatalf("PreviewStage() = (%#v, %v)", preview, err)
		}
	})

	t.Run("git failures fail closed", func(t *testing.T) {
		t.Parallel()
		repository := &Repository{
			runner:  &fakeRunner{results: []processResult{{err: errors.New("failed"), exitCode: 128}}},
			timeout: time.Second,
		}
		_, err := repository.PreviewStage(context.Background(), testIdentity(), []string{"."})
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})
}

func TestHasAnyRef(t *testing.T) {
	t.Parallel()

	t.Run("no references on an unborn repository", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{stdout: ""}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		anyRef, err := repository.HasAnyRef(context.Background(), testIdentity())
		if err != nil || anyRef {
			t.Fatalf("HasAnyRef() = (%t, %v)", anyRef, err)
		}
		if strings.Join(runner.calls[0].arguments, " ") != "for-each-ref" {
			t.Fatalf("HasAnyRef() arguments = %#v", runner.calls[0].arguments)
		}
	})

	t.Run("references prove a born repository", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{stdout: "refs/heads/main\n"}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		anyRef, err := repository.HasAnyRef(context.Background(), testIdentity())
		if err != nil || !anyRef {
			t.Fatalf("HasAnyRef() = (%t, %v)", anyRef, err)
		}
	})

	t.Run("git failures fail closed", func(t *testing.T) {
		t.Parallel()
		repository := &Repository{
			runner:  &fakeRunner{results: []processResult{{err: errors.New("failed"), exitCode: 128}}},
			timeout: time.Second,
		}
		_, err := repository.HasAnyRef(context.Background(), testIdentity())
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})
}

func TestVerifyCommitSignature(t *testing.T) {
	t.Parallel()

	t.Run("rejects an empty revision", func(t *testing.T) {
		t.Parallel()
		repository := &Repository{runner: &fakeRunner{}, timeout: time.Second}
		err := repository.VerifyCommitSignature(context.Background(), testIdentity(), "  ")
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("a verifying commit passes", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		revision := strings.Repeat("a", 40)
		if err := repository.VerifyCommitSignature(context.Background(), testIdentity(), revision); err != nil {
			t.Fatal(err)
		}
		if strings.Join(runner.calls[0].arguments, " ") != "verify-commit "+revision {
			t.Fatalf("VerifyCommitSignature() arguments = %#v", runner.calls[0].arguments)
		}
	})

	t.Run("an unverifiable commit fails closed", func(t *testing.T) {
		t.Parallel()
		repository := &Repository{
			runner:  &fakeRunner{results: []processResult{{stderr: "no signature found", err: errors.New("failed"), exitCode: 1}}},
			timeout: time.Second,
		}
		err := repository.VerifyCommitSignature(context.Background(), testIdentity(), strings.Repeat("a", 40))
		assertProblemCode(t, err, problem.CodeCommitSignatureRequired)
	})
}

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

	t.Run("installs and proves the canonical hook set", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		gitDirectory := filepath.Join(root, ".git")
		writeHooks(t, filepath.Join(gitDirectory, "hooks"), "commit-msg", "pre-push")

		runner := &fakeRunner{results: []processResult{{stdout: gitDirectory + "\n"}}}
		program := &fakeRunner{results: []processResult{{}}}
		repository := &Repository{
			runner:        runner,
			timeout:       time.Second,
			programRunner: func(string) processRunner { return program },
		}
		installation, err := repository.InstallHooks(context.Background(), identity(root))
		if err != nil {
			t.Fatal(err)
		}
		if installation.Directory != filepath.Join(gitDirectory, "hooks") ||
			len(installation.Hooks) != 2 || installation.Hooks[0] != "commit-msg" || installation.Hooks[1] != "pre-push" {
			t.Fatalf("InstallHooks() = %#v", installation)
		}
		if program.calls[0].directory != root || strings.Join(program.calls[0].arguments, " ") != "install" {
			t.Fatalf("lefthook call = %#v", program.calls[0])
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
		installation, err := repository.InstallHooks(context.Background(), identity(root))
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
		_, err := repository.InstallHooks(context.Background(), identity(t.TempDir()))
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("an empty git directory answer fails closed", func(t *testing.T) {
		t.Parallel()
		repository := &Repository{
			runner:  &fakeRunner{results: []processResult{{stdout: "\n"}}},
			timeout: time.Second,
		}
		_, err := repository.InstallHooks(context.Background(), identity(t.TempDir()))
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
		_, err := repository.InstallHooks(context.Background(), identity(root))
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
		_, err := repository.InstallHooks(context.Background(), identity(root))
		assertProblemCode(t, err, problem.CodeExternalCommandFailed)
	})
}
