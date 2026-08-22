package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func TestGenerateWritesCompletionsAndManpages(t *testing.T) {
	t.Parallel()

	output := t.TempDir()
	if err := generate(output); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"git-governance.bash",
		"_git-governance",
		"git-governance.fish",
		"git-governance.ps1",
	} {
		contents, err := os.ReadFile(filepath.Join(output, "completions", name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(contents), "git-governance") {
			t.Fatalf("completion %s does not contain command name", name)
		}
	}

	entries, err := os.ReadDir(filepath.Join(output, "man"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".1") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("generated manpage directory contains no section-1 files: %v", entries)
	}
}

func TestGenerateRejectsEmptyOutputDirectory(t *testing.T) {
	t.Parallel()

	if err := generate(""); err == nil {
		t.Fatal("generate accepted an empty output directory")
	}
}

func TestRunUsesBuildGeneratedDirectoryByDefault(t *testing.T) {
	previous := generateFiles
	t.Cleanup(func() {
		generateFiles = previous
	})

	var outputDirectory string
	generateFiles = func(output string) error {
		outputDirectory = output
		return nil
	}

	if code := run(nil, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("run() exit code = %d", code)
	}
	if outputDirectory != ".build/generated" {
		t.Fatalf("default output directory = %q, want %q", outputDirectory, ".build/generated")
	}
}

func TestRunPrintsTheToolVersion(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	if code := run([]string{"--version"}, stdout, &bytes.Buffer{}); code != 0 {
		t.Fatalf("run --version exit code = %d", code)
	}
	if !strings.Contains(stdout.String(), "generate-docs") {
		t.Fatalf("run --version output = %q", stdout.String())
	}
}

func TestGenerateFilesystemFailurePaths(t *testing.T) {
	t.Parallel()

	t.Run("output is a file", func(t *testing.T) {
		output := filepath.Join(t.TempDir(), "not-a-directory")
		if err := os.WriteFile(output, []byte("file"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := generate(output); err == nil {
			t.Fatal("generate accepted a file as output directory")
		}
	})

	t.Run("man path is a file", func(t *testing.T) {
		output := t.TempDir()
		if err := os.WriteFile(filepath.Join(output, "man"), []byte("file"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := generate(output); err == nil {
			t.Fatal("generate accepted a file as manpage directory")
		}
	})

	for _, completion := range []string{
		"git-governance.bash",
		"_git-governance",
		"git-governance.fish",
		"git-governance.ps1",
	} {
		completion := completion
		t.Run("completion target "+completion, func(t *testing.T) {
			t.Parallel()
			output := t.TempDir()
			directory := filepath.Join(output, "completions", completion)
			if err := os.MkdirAll(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := generate(output); err == nil {
				t.Fatalf("generate accepted directory completion target %s", completion)
			}
		})
	}
}

func TestGenerateReportsManpageFailure(t *testing.T) {
	previous := generateMan
	t.Cleanup(func() {
		generateMan = previous
	})
	generateMan = func(*cobra.Command, *doc.GenManHeader, string) error {
		return errors.New("manpage failed")
	}

	if err := generate(t.TempDir()); err == nil {
		t.Fatal("generate suppressed manpage failure")
	}
}

func TestRunAndMainExitContracts(t *testing.T) {
	output := &bytes.Buffer{}
	if code := run([]string{"--unknown"}, &bytes.Buffer{}, output); code != 2 {
		t.Fatalf("run unknown flag code = %d", code)
	}
	if output.Len() == 0 {
		t.Fatal("flag parse failure produced no diagnostic")
	}

	previousGenerate := generateFiles
	t.Cleanup(func() {
		generateFiles = previousGenerate
	})
	generateFiles = func(string) error {
		return errors.New("generation failed")
	}
	output.Reset()
	if code := run(nil, &bytes.Buffer{}, output); code != 1 || !strings.Contains(output.String(), "generation failed") {
		t.Fatalf("run generation failure = (%d, %q)", code, output.String())
	}

	previousExit, previousArgs := exitProcess, commandArgs
	t.Cleanup(func() {
		exitProcess, commandArgs = previousExit, previousArgs
	})
	exitCode := -1
	exitProcess = func(code int) {
		exitCode = code
	}
	commandArgs = []string{"generate-docs", "--out", t.TempDir()}
	generateFiles = generate
	main()
	if exitCode != 0 {
		t.Fatalf("main exit code = %d", exitCode)
	}
}
