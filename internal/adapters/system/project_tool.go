package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/t33n-software/git-governance/internal/application/port"
)

// ResolveProjectToolVersion reports whether the repository's go.mod declares
// the named tool through a Go tool directive and, when it does, resolves the
// tool version through "go tool <name> --version" inside the repository. The
// declaration is the binding supply-chain fact of the repository: the pinned
// version of the project module wins over any host PATH installation. An
// absent go.mod or an absent tool directive is a proven absent declaration,
// not a failure; the caller then falls back to the host PATH. A go.mod that
// exists but cannot be read fails closed, because its declaration state
// cannot be proven.
func (inspector *Inspector) ResolveProjectToolVersion(
	ctx context.Context,
	repositoryRoot string,
	toolName string,
) (string, bool, error) {
	if repositoryRoot == "" || toolName == "" {
		return "", false, nil
	}
	contents, err := inspector.readFile(filepath.Join(repositoryRoot, "go.mod"))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !goModDeclaresTool(contents, toolName) {
		return "", false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, inspector.timeout)
	defer cancel()
	path, err := inspector.lookPath("go")
	if err != nil {
		return "", true, err
	}
	output, err := inspector.run(ctx, path, "tool", toolName, "--version")
	if err != nil {
		return "", true, err
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(output)), "\n")
	if line == "" {
		return "", true, errors.New("tool version command produced no output")
	}
	return line, true, nil
}

// goModDeclaresTool proves the tool directive surface of a go.mod document.
// It accepts the single-line form "tool <module-path>" and the block form
// "tool ( … )". A module path maps to its tool name by removing a trailing
// major-version segment such as "/v2" and taking the final path element.
func goModDeclaresTool(contents []byte, toolName string) bool {
	inBlock := false
	for _, raw := range strings.Split(string(contents), "\n") {
		fields := strings.Fields(raw)
		if len(fields) == 0 {
			continue
		}
		if inBlock {
			if fields[0] == ")" {
				inBlock = false
				continue
			}
			if toolModuleNames(fields[0], toolName) {
				return true
			}
			continue
		}
		if fields[0] != "tool" {
			continue
		}
		if len(fields) >= 2 && toolModuleNames(fields[1], toolName) {
			return true
		}
		if strings.Contains(raw, "(") {
			inBlock = true
		}
	}
	return false
}

// toolModuleNames maps one declared module path to its tool name and compares
// it with the requested tool name.
func toolModuleNames(modulePath, toolName string) bool {
	trimmed := modulePath
	if index := strings.LastIndex(trimmed, "/v"); index >= 0 && allDigits(trimmed[index+2:]) {
		trimmed = trimmed[:index]
	}
	base := trimmed[strings.LastIndex(trimmed, "/")+1:]
	return base == toolName
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

var _ port.ProjectToolVersionResolver = (*Inspector)(nil)
