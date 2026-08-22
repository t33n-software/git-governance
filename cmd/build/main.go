// Command build runs the repository quality gates before building a native binary.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type commandRunner func(context.Context, string, ...string) ([]byte, error)

type goFileFinder func(string) ([]string, error)

type directoryCreator func(string, os.FileMode) error

type step struct {
	name       string
	executable string
	arguments  []string
}

var (
	exitProcess        = os.Exit
	commandArgs        = os.Args
	runExternalCommand = runCommand
	findGoFiles        = goFiles
	createDirectory    = os.MkdirAll
	version            = "devel"
	dependencySteps    = []step{
		{
			name:       "download module dependencies",
			executable: "go",
			arguments:  []string{"mod", "download"},
		},
		{
			name:       "verify module dependencies",
			executable: "go",
			arguments:  []string{"mod", "verify"},
		},
		{
			name:       "verify module metadata",
			executable: "go",
			arguments:  []string{"mod", "tidy", "-diff"},
		},
		{
			name:       "download build tool dependencies",
			executable: "go",
			arguments:  []string{"-C", "tools", "mod", "download"},
		},
		{
			name:       "verify build tool dependencies",
			executable: "go",
			arguments:  []string{"-C", "tools", "mod", "verify"},
		},
		{
			name:       "verify build tool metadata",
			executable: "go",
			arguments:  []string{"-C", "tools", "mod", "tidy", "-diff"},
		},
	}
	qualitySteps = []step{
		{
			name:       "run lint",
			executable: "go",
			arguments:  []string{"tool", "-modfile", "tools/go.mod", "staticcheck", "./..."},
		},
		{
			name:       "typecheck packages and tests",
			executable: "go",
			arguments:  []string{"test", "-mod=readonly", "-run=^$", "./..."},
		},
		{
			name:       "run unit, contract, and integration tests",
			executable: "go",
			arguments:  []string{"test", "-mod=readonly", "./..."},
		},
		{
			name:       "run uncached integration tests",
			executable: "go",
			arguments:  []string{"test", "-mod=readonly", "./internal/integration", "-count=1"},
		},
		{
			name:       "enforce complete statement coverage",
			executable: "go",
			arguments:  []string{"run", "-mod=readonly", "./cmd/check-coverage"},
		},
		{
			name:       "run race detector",
			executable: "go",
			arguments:  []string{"test", "-mod=readonly", "-race", "./..."},
		},
		{
			name:       "run static analysis",
			executable: "go",
			arguments:  []string{"vet", "./..."},
		},
		{
			name:       "run vulnerability analysis",
			executable: "go",
			arguments:  []string{"tool", "-modfile", "tools/go.mod", "govulncheck", "./..."},
		},
		{
			name:       "fuzz ticket parser",
			executable: "go",
			arguments:  []string{"test", "-mod=readonly", "./internal/domain/ticket", "-run=^$", "-fuzz=FuzzParseTicketValues", "-fuzztime=50000x", "-parallel=1"},
		},
		{
			name:       "fuzz branch parser",
			executable: "go",
			arguments:  []string{"test", "-mod=readonly", "./internal/domain/branch", "-run=^$", "-fuzz=FuzzParseBranchValues", "-fuzztime=50000x", "-parallel=1"},
		},
		{
			name:       "fuzz commit-message parser",
			executable: "go",
			arguments:  []string{"test", "-mod=readonly", "./internal/domain/commitmsg", "-run=^$", "-fuzz=FuzzParseCommitMessage", "-fuzztime=50000x", "-parallel=1"},
		},
		{
			name:       "fuzz preferences decoder",
			executable: "go",
			arguments:  []string{"test", "-mod=readonly", "./internal/adapters/configfs", "-run=^$", "-fuzz=FuzzDecodePreferences", "-fuzztime=50000x", "-parallel=1"},
		},
		{
			name:       "fuzz quality configuration decoder",
			executable: "go",
			arguments:  []string{"test", "-mod=readonly", "./internal/adapters/quality", "-run=^$", "-fuzz=FuzzDecodeQualityConfiguration", "-fuzztime=50000x", "-parallel=1"},
		},
		{
			name:       "validate Lefthook configuration",
			executable: "go",
			arguments:  []string{"tool", "-modfile", "tools/go.mod", "lefthook", "validate"},
		},
	}
)

func main() {
	exitProcess(run(
		context.Background(),
		commandArgs[1:],
		os.Stdout,
		os.Stderr,
		runExternalCommand,
		findGoFiles,
		createDirectory,
	))
}

func run(
	ctx context.Context,
	arguments []string,
	stdout io.Writer,
	stderr io.Writer,
	execute commandRunner,
	locateGoFiles goFileFinder,
	makeDirectory directoryCreator,
) int {
	if len(arguments) == 1 && arguments[0] == "--version" {
		fmt.Fprintf(stdout, "build %s\n", version)
		return 0
	}
	if len(arguments) != 0 {
		fmt.Fprintln(stderr, "usage: build")
		return 2
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if !runSteps(ctx, dependencySteps, stdout, stderr, execute) {
		return 1
	}
	if !checkFormatting(ctx, stdout, stderr, execute, locateGoFiles) {
		return 1
	}
	if !runSteps(ctx, qualitySteps, stdout, stderr, execute) {
		return 1
	}
	artifact := binaryPath()
	if err := makeDirectory(filepath.Dir(artifact), 0o755); err != nil {
		fmt.Fprintln(stderr, "create build directory:", err)
		return 1
	}

	buildSteps := []step{
		{
			name:       "build native binary",
			executable: "go",
			arguments:  []string{"build", "-mod=readonly", "-trimpath", "-o", artifact, "./cmd/git-governance"},
		},
		{
			name:       "record embedded module provenance",
			executable: "go",
			arguments:  []string{"version", "-m", artifact},
		},
		{
			name:       "smoke test version",
			executable: artifact,
			arguments:  []string{"--version"},
		},
		{
			name:       "smoke test branch list",
			executable: artifact,
			arguments:  []string{"--output", "json", "branch", "list"},
		},
		{
			name:       "smoke test policy description",
			executable: artifact,
			arguments:  []string{"--output", "json", "policy", "describe"},
		},
	}
	if !runSteps(ctx, buildSteps, stdout, stderr, execute) {
		return 1
	}

	fmt.Fprintln(stdout, "Build completed successfully.")
	return 0
}

func runSteps(
	ctx context.Context,
	steps []step,
	stdout io.Writer,
	stderr io.Writer,
	execute commandRunner,
) bool {
	for _, step := range steps {
		if !runStep(ctx, step, stdout, stderr, execute) {
			return false
		}
	}
	return true
}

func runStep(
	ctx context.Context,
	step step,
	stdout io.Writer,
	stderr io.Writer,
	execute commandRunner,
) bool {
	fmt.Fprintln(stdout, "==>", step.name)
	output, err := execute(ctx, step.executable, step.arguments...)
	if len(output) > 0 {
		_, _ = stdout.Write(output)
	}
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", step.name, err)
		return false
	}
	return true
}

func checkFormatting(
	ctx context.Context,
	stdout io.Writer,
	stderr io.Writer,
	execute commandRunner,
	locateGoFiles goFileFinder,
) bool {
	files, err := locateGoFiles(".")
	if err != nil {
		fmt.Fprintln(stderr, "list Go files:", err)
		return false
	}

	fmt.Fprintln(stdout, "==> check Go formatting")
	arguments := append([]string{"-l"}, files...)
	output, err := execute(ctx, "gofmt", arguments...)
	if len(output) > 0 {
		_, _ = stdout.Write(output)
	}
	if err != nil {
		fmt.Fprintln(stderr, "check Go formatting:", err)
		return false
	}
	if unformatted := strings.TrimSpace(string(output)); unformatted != "" {
		fmt.Fprintln(stderr, "the following files require gofmt:")
		fmt.Fprintln(stderr, unformatted)
		return false
	}
	return true
}

func goFiles(root string) ([]string, error) {
	files := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if ignoredDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) == ".go" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func ignoredDirectory(name string) bool {
	switch name {
	case ".build", ".git", ".cache", "coverage", "dist", "vendor":
		return true
	default:
		return false
	}
}

func binaryPath() string {
	return binaryPathFor(runtime.GOOS)
}

func binaryPathFor(goos string) string {
	name := "git-governance"
	if goos == "windows" {
		name += ".exe"
	}
	return filepath.Join(".build", "bin", name)
}

func runCommand(ctx context.Context, executable string, arguments ...string) ([]byte, error) {
	return exec.CommandContext(ctx, executable, arguments...).CombinedOutput()
}
