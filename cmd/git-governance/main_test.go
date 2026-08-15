package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestNewCommandUsesBuildMetadata(t *testing.T) {
	previousVersion, previousCommit, previousDate := version, commit, date
	t.Cleanup(func() {
		version, commit, date = previousVersion, previousCommit, previousDate
	})
	version, commit, date = "1.2.3", "abc123", "2026-07-10T12:00:00Z"

	command := newCommand()
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{"--version"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"git-governance 1.2.3", "commit: abc123", "built: 2026-07-10T12:00:00Z"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("version output missing %q: %q", expected, output.String())
		}
	}
}

func TestExecuteAndMainExitContract(t *testing.T) {
	command := &cobra.Command{
		Use: "test",
		RunE: func(*cobra.Command, []string) error {
			return problem.New(problem.Details{
				Code:     problem.CodeInvalidInput,
				Category: problem.CategoryUsage,
			})
		},
	}
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(output)
	if code := execute(context.Background(), command); code != problem.ExitUsage {
		t.Fatalf("execute() = %d, want %d", code, problem.ExitUsage)
	}

	previousExit, previousBuilder := exitProcess, buildCommand
	t.Cleanup(func() {
		exitProcess, buildCommand = previousExit, previousBuilder
	})
	exitCode := -1
	exitProcess = func(code int) {
		exitCode = code
	}
	buildCommand = func() *cobra.Command {
		return &cobra.Command{
			Use: "test",
			RunE: func(*cobra.Command, []string) error {
				return nil
			},
		}
	}
	main()
	if exitCode != problem.ExitSuccess {
		t.Fatalf("main exit code = %d", exitCode)
	}

	failing := &cobra.Command{
		Use: "test",
		RunE: func(*cobra.Command, []string) error {
			return errors.New("unexpected")
		},
	}
	failing.SetErr(&bytes.Buffer{})
	if code := execute(context.Background(), failing); code != problem.ExitInternal {
		t.Fatalf("untyped execute() = %d", code)
	}
}
