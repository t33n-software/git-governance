package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestIncompletePackages(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		output string
		want   []string
	}{
		{
			name: "all executable packages are complete",
			output: strings.Join([]string{
				"ok example.com/complete coverage: 100.0% of statements",
				"? example.com/interfaces [no test files]",
				"ok example.com/integration coverage: [no statements]",
			}, "\r\n"),
		},
		{
			name:   "reports incomplete executable package",
			output: "ok example.com/incomplete coverage: 99.9% of statements\n",
			want:   []string{"ok example.com/incomplete coverage: 99.9% of statements"},
		},
		{
			name:   "reports malformed coverage result",
			output: "ok example.com/malformed coverage: unavailable\n",
			want:   []string{"ok example.com/malformed coverage: unavailable"},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			actual := incompletePackages(testCase.output)
			if strings.Join(actual, "\n") != strings.Join(testCase.want, "\n") {
				t.Fatalf("incompletePackages() = %q, want %q", actual, testCase.want)
			}
		})
	}
}

func TestPackagesWithoutTests(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		output string
		want   []string
	}{
		{
			name: "all packages contain tests",
			output: strings.Join([]string{
				"ok example.com/complete coverage: 100.0% of statements",
				"ok example.com/integration coverage: [no statements]",
			}, "\r\n"),
		},
		{
			name:   "reports package without test files",
			output: "? example.com/port [no test files]\n",
			want:   []string{"? example.com/port [no test files]"},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			actual := packagesWithoutTests(testCase.output)
			if strings.Join(actual, "\n") != strings.Join(testCase.want, "\n") {
				t.Fatalf("packagesWithoutTests() = %q, want %q", actual, testCase.want)
			}
		})
	}
}

func TestRun(t *testing.T) {
	complete := []byte("ok example.com/complete coverage: 100.0% of statements\n")

	t.Run("accepts complete coverage", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		exitCode := run(context.Background(), nil, stdout, stderr, func(context.Context, string, ...string) ([]byte, error) {
			return complete, nil
		})
		if exitCode != 0 || !strings.Contains(stdout.String(), "All Go packages contain test files") || stderr.Len() != 0 {
			t.Fatalf("run() = (%d, %q, %q)", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("rejects incomplete coverage", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		exitCode := run(context.Background(), nil, stdout, stderr, func(context.Context, string, ...string) ([]byte, error) {
			return []byte("ok example.com/incomplete coverage: 80.0% of statements\n"), nil
		})
		if exitCode != 1 || !strings.Contains(stderr.String(), "80.0%") || !strings.Contains(stdout.String(), "80.0%") {
			t.Fatalf("run() = (%d, %q, %q)", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("rejects packages without test files", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		exitCode := run(context.Background(), nil, stdout, stderr, func(context.Context, string, ...string) ([]byte, error) {
			return []byte("? example.com/port [no test files]\n"), nil
		})
		if exitCode != 1 || !strings.Contains(stderr.String(), "_test.go") || !strings.Contains(stderr.String(), "example.com/port") || !strings.Contains(stdout.String(), "example.com/port") {
			t.Fatalf("run() = (%d, %q, %q)", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("reports missing tests and incomplete coverage together", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		exitCode := run(context.Background(), nil, stdout, stderr, func(context.Context, string, ...string) ([]byte, error) {
			return []byte(strings.Join([]string{
				"? example.com/port [no test files]",
				"ok example.com/incomplete coverage: 80.0% of statements",
			}, "\n")), nil
		})
		if exitCode != 1 || !strings.Contains(stderr.String(), "_test.go") || !strings.Contains(stderr.String(), "100.0%") {
			t.Fatalf("run() = (%d, %q, %q)", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("preserves Go command failure output", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		runErr := errors.New("go test failed")
		exitCode := run(context.Background(), nil, stdout, stderr, func(context.Context, string, ...string) ([]byte, error) {
			return []byte("failure output\n"), runErr
		})
		if exitCode != 1 || !strings.Contains(stdout.String(), "failure output") || !strings.Contains(stderr.String(), runErr.Error()) {
			t.Fatalf("run() = (%d, %q, %q)", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("rejects command arguments", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		exitCode := run(context.Background(), []string{"unexpected"}, stdout, stderr, func(context.Context, string, ...string) ([]byte, error) {
			t.Fatal("coverage command must not run for invalid arguments")
			return nil, nil
		})
		if exitCode != 2 || !strings.Contains(stderr.String(), "usage: check-coverage") {
			t.Fatalf("run() = (%d, %q)", exitCode, stderr.String())
		}
	})

	t.Run("normalizes a nil context", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		exitCode := run(testNilContext(), nil, stdout, &bytes.Buffer{}, func(ctx context.Context, executable string, arguments ...string) ([]byte, error) {
			if ctx == nil || executable != "go" || strings.Join(arguments, " ") != "test -count=1 -p=1 -cover -covermode=atomic ./..." {
				t.Fatalf("coverage invocation = (%v, %q, %v)", ctx, executable, arguments)
			}
			return complete, nil
		})
		if exitCode != 0 {
			t.Fatalf("run() exit code = %d", exitCode)
		}
	})
}

func TestRunPrintsTheToolVersion(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	exitCode := run(context.Background(), []string{"--version"}, stdout, &bytes.Buffer{}, func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("runner must not execute for the version flag")
		return nil, nil
	})
	if exitCode != 0 || !strings.Contains(stdout.String(), "check-coverage devel") {
		t.Fatalf("run --version = (%d, %q)", exitCode, stdout.String())
	}
}

func TestMainDelegatesToRun(t *testing.T) {
	originalArgs := commandArgs
	originalExit := exitProcess
	originalRun := runCommand
	t.Cleanup(func() {
		commandArgs = originalArgs
		exitProcess = originalExit
		runCommand = originalRun
	})

	commandArgs = []string{"check-coverage"}
	runCommand = func(context.Context, string, ...string) ([]byte, error) {
		return []byte("ok example.com/complete coverage: 100.0% of statements\n"), nil
	}
	exitCode := -1
	exitProcess = func(code int) {
		exitCode = code
	}

	main()
	if exitCode != 0 {
		t.Fatalf("main() exit code = %d", exitCode)
	}
}

func TestRunGoCommandUsesRequestedExecutable(t *testing.T) {
	output, err := runGoCommand(context.Background(), os.Args[0], "-test.run=^$")
	if err != nil {
		t.Fatalf("runGoCommand() error = %v", err)
	}
	if output == nil {
		t.Fatal("runGoCommand() returned a nil output slice")
	}
}

func testNilContext() context.Context {
	return nil
}
