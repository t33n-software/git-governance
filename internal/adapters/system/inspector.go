// Package system provides read-only host diagnostics for tools and files.
package system

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/t33n-software/git-governance/internal/application/port"
)

const defaultTimeout = 5 * time.Second

type commandRunner func(ctx context.Context, executable string, arguments ...string) ([]byte, error)

// Options configures a read-only system inspector. The functions are injectable
// so whitebox tests can cover lookup, process, timeout, and filesystem paths.
type Options struct {
	Timeout  time.Duration
	LookPath func(string) (string, error)
	Run      commandRunner
	Stat     func(string) (os.FileInfo, error)
}

// Inspector implements bounded diagnostic checks without changing the host.
type Inspector struct {
	timeout  time.Duration
	lookPath func(string) (string, error)
	run      commandRunner
	stat     func(string) (os.FileInfo, error)
}

// New constructs a system inspector with production defaults.
func New(options Options) *Inspector {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	lookPath := options.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	run := options.Run
	if run == nil {
		run = runCommand
	}
	stat := options.Stat
	if stat == nil {
		stat = os.Stat
	}
	return &Inspector{
		timeout:  timeout,
		lookPath: lookPath,
		run:      run,
		stat:     stat,
	}
}

// Platform reports the current native runtime target without probing external
// tools or changing host state.
func (*Inspector) Platform() (string, string) {
	return runtime.GOOS, runtime.GOARCH
}

// Version resolves an executable and returns its first version-output line.
func (inspector *Inspector) Version(ctx context.Context, executable string) (string, error) {
	if executable == "" {
		return "", errors.New("executable name is required")
	}
	path, err := inspector.lookPath(executable)
	if err != nil {
		return "", err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, inspector.timeout)
	defer cancel()
	output, err := inspector.run(ctx, path, "--version")
	if err != nil {
		return "", err
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(output)), "\n")
	if line == "" {
		return "", errors.New("version command produced no output")
	}
	return line, nil
}

// FileExists reports whether a path exists without opening or changing it.
func (inspector *Inspector) FileExists(path string) (bool, error) {
	_, err := inspector.stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func runCommand(ctx context.Context, executable string, arguments ...string) ([]byte, error) {
	return exec.CommandContext(ctx, executable, arguments...).CombinedOutput()
}

var _ port.ToolInspector = (*Inspector)(nil)
