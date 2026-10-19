package port

import (
	"context"
	"errors"
)

// ErrToolInspectorMissing marks the fail-closed verdict of a tool resolution
// attempt without a configured tool inspector.
var ErrToolInspectorMissing = errors.New("tool inspector is not configured")

// ProjectToolVersionResolver is an optional capability for host inspectors
// that can resolve an external tool through the repository's own declaration
// before the host PATH is consulted. A repository that declares the tool as a
// Go tool in its native manifest resolves through that declaration, so the
// version the governed operation reports is the version the repository pins.
// The boolean result reports whether the repository declares the tool at all;
// a false result without an error is an absent declaration, never a silent
// failure.
type ProjectToolVersionResolver interface {
	ResolveProjectToolVersion(
		ctx context.Context,
		repositoryRoot string,
		toolName string,
	) (version string, declared bool, err error)
}

// ResolveToolVersion is the single source of truth for the governed
// hook-manager resolution chain: a repository-declared project tool wins, and
// the host PATH lookup is the fallback for repositories without a declaration.
// A broken declaration surface fails closed: when the resolver reports an
// error, the resolution reports it instead of silently falling back, because
// an unreadable declaration cannot prove the absence of a pinned tool.
func ResolveToolVersion(
	ctx context.Context,
	tools ToolInspector,
	repositoryRoot string,
	executable string,
) (version string, projectResolved bool, err error) {
	if tools == nil {
		return "", false, ErrToolInspectorMissing
	}
	if resolver, ok := tools.(ProjectToolVersionResolver); ok && repositoryRoot != "" {
		projectVersion, declared, resolveErr := resolver.ResolveProjectToolVersion(ctx, repositoryRoot, executable)
		if resolveErr != nil {
			return "", false, resolveErr
		}
		if declared {
			return projectVersion, true, nil
		}
	}
	hostVersion, hostErr := tools.Version(ctx, executable)
	if hostErr != nil {
		return "", false, hostErr
	}
	return hostVersion, false, nil
}
