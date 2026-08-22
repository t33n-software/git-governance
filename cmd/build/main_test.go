package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

type invocation struct {
	executable string
	arguments  []string
}

func TestRunExecutesEveryQualityGateBeforeBuilding(t *testing.T) {
	t.Parallel()

	var calls []invocation
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run(
		context.Background(),
		nil,
		stdout,
		stderr,
		successfulRunner(t, &calls),
		func(string) ([]string, error) {
			return []string{"first.go", "second.go"}, nil
		},
		func(path string, permission os.FileMode) error {
			if path != filepath.Join(".build", "bin") || permission != 0o755 {
				t.Fatalf("create build directory = (%q, %o)", path, permission)
			}
			return nil
		},
	)

	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Build completed successfully.") {
		t.Fatalf("run() stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("run() stderr = %q", stderr.String())
	}

	want := expectedInvocations([]string{"first.go", "second.go"})
	if !slices.EqualFunc(calls, want, func(actual, expected invocation) bool {
		return actual.executable == expected.executable && slices.Equal(actual.arguments, expected.arguments)
	}) {
		t.Fatalf("run() invocations = %#v, want %#v", calls, want)
	}
}

func TestRunRejectsArguments(t *testing.T) {
	t.Parallel()

	stderr := &bytes.Buffer{}
	exitCode := run(
		context.Background(),
		[]string{"unexpected"},
		&bytes.Buffer{},
		stderr,
		func(context.Context, string, ...string) ([]byte, error) {
			t.Fatal("runner must not execute for invalid arguments")
			return nil, nil
		},
		func(string) ([]string, error) {
			t.Fatal("file finder must not execute for invalid arguments")
			return nil, nil
		},
		func(string, os.FileMode) error {
			t.Fatal("directory creator must not execute for invalid arguments")
			return nil
		},
	)

	if exitCode != 2 || !strings.Contains(stderr.String(), "usage: build") {
		t.Fatalf("run() = (%d, %q)", exitCode, stderr.String())
	}
}

func TestRunPrintsTheToolVersion(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	exitCode := run(
		context.Background(),
		[]string{"--version"},
		stdout,
		&bytes.Buffer{},
		func(context.Context, string, ...string) ([]byte, error) {
			t.Fatal("runner must not execute for the version flag")
			return nil, nil
		},
		func(string) ([]string, error) {
			t.Fatal("file finder must not execute for the version flag")
			return nil, nil
		},
		func(string, os.FileMode) error {
			t.Fatal("directory creator must not execute for the version flag")
			return nil
		},
	)
	if exitCode != 0 || !strings.Contains(stdout.String(), "build devel") {
		t.Fatalf("run --version = (%d, %q)", exitCode, stdout.String())
	}
}

func TestRunNormalizesNilContext(t *testing.T) {
	t.Parallel()

	var calls []invocation
	exitCode := run(
		testNilContext(),
		nil,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(ctx context.Context, executable string, arguments ...string) ([]byte, error) {
			if ctx == nil {
				t.Fatal("run() passed a nil context to a command")
			}
			calls = append(calls, invocation{executable: executable, arguments: slices.Clone(arguments)})
			return nil, nil
		},
		func(string) ([]string, error) {
			return []string{"main.go"}, nil
		},
		func(string, os.FileMode) error {
			return nil
		},
	)

	if exitCode != 0 || len(calls) == 0 {
		t.Fatalf("run() = (%d, %#v)", exitCode, calls)
	}
}

func TestRunStopsOnDependencyFailure(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runErr := errors.New("download failed")
	exitCode := run(
		context.Background(),
		nil,
		stdout,
		stderr,
		func(context.Context, string, ...string) ([]byte, error) {
			return []byte("dependency output\n"), runErr
		},
		func(string) ([]string, error) {
			t.Fatal("file finder must not execute after a dependency failure")
			return nil, nil
		},
		func(string, os.FileMode) error {
			t.Fatal("directory creator must not execute after a dependency failure")
			return nil
		},
	)

	if exitCode != 1 || !strings.Contains(stdout.String(), "dependency output") || !strings.Contains(stderr.String(), runErr.Error()) {
		t.Fatalf("run() = (%d, %q, %q)", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunStopsWhenFormattingCannotListFiles(t *testing.T) {
	t.Parallel()

	listErr := errors.New("list failure")
	stderr := &bytes.Buffer{}
	exitCode := run(
		context.Background(),
		nil,
		&bytes.Buffer{},
		stderr,
		func(context.Context, string, ...string) ([]byte, error) {
			return nil, nil
		},
		func(string) ([]string, error) {
			return nil, listErr
		},
		func(string, os.FileMode) error {
			t.Fatal("directory creator must not execute after a file-listing failure")
			return nil
		},
	)

	if exitCode != 1 || !strings.Contains(stderr.String(), listErr.Error()) {
		t.Fatalf("run() = (%d, %q)", exitCode, stderr.String())
	}
}

func TestCheckFormattingRejectsGofmtFailure(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runErr := errors.New("gofmt failed")
	ok := checkFormatting(
		context.Background(),
		stdout,
		stderr,
		func(context.Context, string, ...string) ([]byte, error) {
			return []byte("gofmt output\n"), runErr
		},
		func(root string) ([]string, error) {
			if root != "." {
				t.Fatalf("format root = %q", root)
			}
			return []string{"main.go"}, nil
		},
	)

	if ok || !strings.Contains(stdout.String(), "gofmt output") || !strings.Contains(stderr.String(), runErr.Error()) {
		t.Fatalf("checkFormatting() = (%t, %q, %q)", ok, stdout.String(), stderr.String())
	}
}

func TestCheckFormattingRejectsUnformattedFiles(t *testing.T) {
	t.Parallel()

	stderr := &bytes.Buffer{}
	ok := checkFormatting(
		context.Background(),
		&bytes.Buffer{},
		stderr,
		func(_ context.Context, executable string, arguments ...string) ([]byte, error) {
			if executable != "gofmt" || !slices.Equal(arguments, []string{"-l", "main.go"}) {
				t.Fatalf("gofmt invocation = (%q, %#v)", executable, arguments)
			}
			return []byte("main.go\n"), nil
		},
		func(string) ([]string, error) {
			return []string{"main.go"}, nil
		},
	)

	if ok || !strings.Contains(stderr.String(), "main.go") {
		t.Fatalf("checkFormatting() = (%t, %q)", ok, stderr.String())
	}
}

func TestRunStopsOnQualityFailure(t *testing.T) {
	t.Parallel()

	stderr := &bytes.Buffer{}
	exitCode := run(
		context.Background(),
		nil,
		&bytes.Buffer{},
		stderr,
		func(_ context.Context, executable string, arguments ...string) ([]byte, error) {
			if executable == "go" && slices.Equal(arguments, []string{"tool", "-modfile", "tools/go.mod", "staticcheck", "./..."}) {
				return nil, errors.New("lint failed")
			}
			return nil, nil
		},
		func(string) ([]string, error) {
			return []string{"main.go"}, nil
		},
		func(string, os.FileMode) error {
			t.Fatal("directory creator must not execute after a quality failure")
			return nil
		},
	)

	if exitCode != 1 || !strings.Contains(stderr.String(), "lint failed") {
		t.Fatalf("run() = (%d, %q)", exitCode, stderr.String())
	}
}

func TestRunStopsWhenBuildDirectoryCannotBeCreated(t *testing.T) {
	t.Parallel()

	directoryErr := errors.New("cannot create build directory")
	stderr := &bytes.Buffer{}
	exitCode := run(
		context.Background(),
		nil,
		&bytes.Buffer{},
		stderr,
		successfulRunner(t, nil),
		func(string) ([]string, error) {
			return []string{"main.go"}, nil
		},
		func(string, os.FileMode) error {
			return directoryErr
		},
	)

	if exitCode != 1 || !strings.Contains(stderr.String(), directoryErr.Error()) {
		t.Fatalf("run() = (%d, %q)", exitCode, stderr.String())
	}
}

func TestRunStopsOnBuildFailure(t *testing.T) {
	t.Parallel()

	buildErr := errors.New("build failed")
	stderr := &bytes.Buffer{}
	exitCode := run(
		context.Background(),
		nil,
		&bytes.Buffer{},
		stderr,
		func(_ context.Context, executable string, arguments ...string) ([]byte, error) {
			if executable == "go" && len(arguments) > 0 && arguments[0] == "build" {
				return []byte("build output\n"), buildErr
			}
			return nil, nil
		},
		func(string) ([]string, error) {
			return []string{"main.go"}, nil
		},
		func(string, os.FileMode) error {
			return nil
		},
	)

	if exitCode != 1 || !strings.Contains(stderr.String(), buildErr.Error()) {
		t.Fatalf("run() = (%d, %q)", exitCode, stderr.String())
	}
}

func TestGoFilesSkipsBuildOutputsAndSortsSourceFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, path := range []string{
		"z.go",
		"nested/a.go",
		".build/ignored.go",
		".git/ignored.go",
		".cache/ignored.go",
		"coverage/ignored.go",
		"dist/ignored.go",
		"vendor/ignored.go",
		"nested/readme.txt",
	} {
		writeTestFile(t, filepath.Join(root, path))
	}

	files, err := goFiles(root)
	if err != nil {
		t.Fatalf("goFiles() error = %v", err)
	}
	want := []string{
		filepath.Join(root, "nested", "a.go"),
		filepath.Join(root, "z.go"),
	}
	if !slices.Equal(files, want) {
		t.Fatalf("goFiles() = %#v, want %#v", files, want)
	}
}

func TestGoFilesReturnsWalkErrors(t *testing.T) {
	t.Parallel()

	_, err := goFiles(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("goFiles() error = nil, want an error for a missing root")
	}
}

func TestIgnoredDirectory(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name string
		want bool
	}{
		{name: ".build", want: true},
		{name: ".git", want: true},
		{name: ".cache", want: true},
		{name: "coverage", want: true},
		{name: "dist", want: true},
		{name: "vendor", want: true},
		{name: "internal", want: false},
	} {
		if got := ignoredDirectory(testCase.name); got != testCase.want {
			t.Errorf("ignoredDirectory(%q) = %t, want %t", testCase.name, got, testCase.want)
		}
	}
}

func TestBinaryPathFor(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		goos string
		want string
	}{
		{name: "Windows", goos: "windows", want: filepath.Join(".build", "bin", "git-governance.exe")},
		{name: "Linux", goos: "linux", want: filepath.Join(".build", "bin", "git-governance")},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			if got := binaryPathFor(testCase.goos); got != testCase.want {
				t.Fatalf("binaryPathFor(%q) = %q, want %q", testCase.goos, got, testCase.want)
			}
		})
	}
}

func TestBinaryPathUsesCurrentPlatform(t *testing.T) {
	t.Parallel()

	if got, want := binaryPath(), binaryPathFor(runtime.GOOS); got != want {
		t.Fatalf("binaryPath() = %q, want %q", got, want)
	}
}

func TestMainDelegatesToRun(t *testing.T) {
	originalArgs := commandArgs
	originalExit := exitProcess
	originalRunner := runExternalCommand
	originalFinder := findGoFiles
	originalCreator := createDirectory
	t.Cleanup(func() {
		commandArgs = originalArgs
		exitProcess = originalExit
		runExternalCommand = originalRunner
		findGoFiles = originalFinder
		createDirectory = originalCreator
	})

	commandArgs = []string{"build"}
	runExternalCommand = successfulRunner(t, nil)
	findGoFiles = func(string) ([]string, error) {
		return []string{"main.go"}, nil
	}
	createDirectory = func(string, os.FileMode) error {
		return nil
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

func TestRunCommandUsesRequestedExecutable(t *testing.T) {
	t.Parallel()

	output, err := runCommand(context.Background(), os.Args[0], "-test.run=^$")
	if err != nil {
		t.Fatalf("runCommand() error = %v", err)
	}
	if output == nil {
		t.Fatal("runCommand() returned a nil output slice")
	}
}

func successfulRunner(t *testing.T, calls *[]invocation) commandRunner {
	t.Helper()

	return func(ctx context.Context, executable string, arguments ...string) ([]byte, error) {
		if ctx == nil {
			t.Fatal("runner received a nil context")
		}
		if calls != nil {
			*calls = append(*calls, invocation{
				executable: executable,
				arguments:  slices.Clone(arguments),
			})
		}
		if executable == "gofmt" {
			return nil, nil
		}
		return []byte("completed\n"), nil
	}
}

func testNilContext() context.Context {
	return nil
}

func expectedInvocations(files []string) []invocation {
	artifact := binaryPath()
	return []invocation{
		{
			executable: "go",
			arguments:  []string{"mod", "download"},
		},
		{
			executable: "go",
			arguments:  []string{"mod", "verify"},
		},
		{
			executable: "go",
			arguments:  []string{"mod", "tidy", "-diff"},
		},
		{
			executable: "go",
			arguments:  []string{"-C", "tools", "mod", "download"},
		},
		{
			executable: "go",
			arguments:  []string{"-C", "tools", "mod", "verify"},
		},
		{
			executable: "go",
			arguments:  []string{"-C", "tools", "mod", "tidy", "-diff"},
		},
		{
			executable: "gofmt",
			arguments:  append([]string{"-l"}, files...),
		},
		{
			executable: "go",
			arguments:  []string{"tool", "-modfile", "tools/go.mod", "staticcheck", "./..."},
		},
		{
			executable: "go",
			arguments:  []string{"test", "-mod=readonly", "-run=^$", "./..."},
		},
		{
			executable: "go",
			arguments:  []string{"test", "-mod=readonly", "./..."},
		},
		{
			executable: "go",
			arguments:  []string{"test", "-mod=readonly", "./internal/integration", "-count=1"},
		},
		{
			executable: "go",
			arguments:  []string{"run", "-mod=readonly", "./cmd/check-coverage"},
		},
		{
			executable: "go",
			arguments:  []string{"test", "-mod=readonly", "-race", "./..."},
		},
		{
			executable: "go",
			arguments:  []string{"vet", "./..."},
		},
		{
			executable: "go",
			arguments:  []string{"tool", "-modfile", "tools/go.mod", "govulncheck", "./..."},
		},
		{
			executable: "go",
			arguments:  []string{"test", "-mod=readonly", "./internal/domain/ticket", "-run=^$", "-fuzz=FuzzParseTicketValues", "-fuzztime=50000x", "-parallel=1"},
		},
		{
			executable: "go",
			arguments:  []string{"test", "-mod=readonly", "./internal/domain/branch", "-run=^$", "-fuzz=FuzzParseBranchValues", "-fuzztime=50000x", "-parallel=1"},
		},
		{
			executable: "go",
			arguments:  []string{"test", "-mod=readonly", "./internal/domain/commitmsg", "-run=^$", "-fuzz=FuzzParseCommitMessage", "-fuzztime=50000x", "-parallel=1"},
		},
		{
			executable: "go",
			arguments:  []string{"test", "-mod=readonly", "./internal/adapters/configfs", "-run=^$", "-fuzz=FuzzDecodePreferences", "-fuzztime=50000x", "-parallel=1"},
		},
		{
			executable: "go",
			arguments:  []string{"test", "-mod=readonly", "./internal/adapters/quality", "-run=^$", "-fuzz=FuzzDecodeQualityConfiguration", "-fuzztime=50000x", "-parallel=1"},
		},
		{
			executable: "go",
			arguments:  []string{"tool", "-modfile", "tools/go.mod", "lefthook", "validate"},
		},
		invocation{
			executable: "go",
			arguments:  []string{"build", "-mod=readonly", "-trimpath", "-o", artifact, "./cmd/git-governance"},
		},
		invocation{
			executable: "go",
			arguments:  []string{"version", "-m", artifact},
		},
		invocation{
			executable: artifact,
			arguments:  []string{"--version"},
		},
		invocation{
			executable: artifact,
			arguments:  []string{"--output", "json", "branch", "list"},
		},
		invocation{
			executable: artifact,
			arguments:  []string{"--output", "json", "policy", "describe"},
		},
	}
}

func writeTestFile(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent directory for %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte("package testdata\n"), 0o600); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}
