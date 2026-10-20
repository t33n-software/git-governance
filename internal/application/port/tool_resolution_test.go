package port

import (
	"context"
	"errors"
	"testing"
)

// plainInspector implements only the base ToolInspector surface.
type plainInspector struct {
	version string
	err     error
}

func (plainInspector) Platform() (string, string) { return "linux", "amd64" }

func (inspector plainInspector) Version(context.Context, string) (string, error) {
	return inspector.version, inspector.err
}

func (plainInspector) FileExists(string) (bool, error) { return true, nil }

// declaringInspector additionally implements the project-tool capability and
// records the arguments the resolution chain passes to it.
type declaringInspector struct {
	plainInspector
	declared  bool
	declErr   error
	project   string
	seenRoot  string
	seenTool  string
	callCount int
}

func (inspector *declaringInspector) ResolveProjectToolVersion(
	_ context.Context,
	repositoryRoot string,
	toolName string,
) (string, bool, error) {
	inspector.callCount++
	inspector.seenRoot = repositoryRoot
	inspector.seenTool = toolName
	if inspector.declErr != nil {
		return "", false, inspector.declErr
	}
	if !inspector.declared {
		return "", false, nil
	}
	return inspector.project, true, nil
}

func TestResolveToolVersionChain(t *testing.T) {
	t.Parallel()

	t.Run("a missing inspector fails closed", func(t *testing.T) {
		t.Parallel()
		_, _, err := ResolveToolVersion(context.Background(), nil, "/repo", "lefthook")
		if !errors.Is(err, ErrToolInspectorMissing) {
			t.Fatalf("ResolveToolVersion(nil) error = %v", err)
		}
	})

	t.Run("a declared project tool wins over the host path", func(t *testing.T) {
		t.Parallel()
		inspector := &declaringInspector{
			plainInspector: plainInspector{version: "host should not be consulted"},
			declared:       true,
			project:        "lefthook version 2.2.2",
		}
		version, projectResolved, err := ResolveToolVersion(context.Background(), inspector, "/repo", "lefthook")
		if err != nil || !projectResolved || version != "lefthook version 2.2.2" {
			t.Fatalf("ResolveToolVersion() = (%q, %t, %v)", version, projectResolved, err)
		}
		if inspector.seenRoot != "/repo" || inspector.seenTool != "lefthook" || inspector.callCount != 1 {
			t.Fatalf("resolver call = %q %q count %d", inspector.seenRoot, inspector.seenTool, inspector.callCount)
		}
	})

	t.Run("a broken declaration surface fails closed without falling back", func(t *testing.T) {
		t.Parallel()
		expected := errors.New("go.mod unreadable")
		inspector := &declaringInspector{
			plainInspector: plainInspector{version: "host fallback must not mask the failure"},
			declErr:        expected,
		}
		_, projectResolved, err := ResolveToolVersion(context.Background(), inspector, "/repo", "lefthook")
		if !errors.Is(err, expected) || projectResolved {
			t.Fatalf("ResolveToolVersion() = (%t, %v), want the declaration failure", projectResolved, err)
		}
	})

	t.Run("an absent declaration falls back to the host path", func(t *testing.T) {
		t.Parallel()
		inspector := &declaringInspector{
			plainInspector: plainInspector{version: "lefthook version 2.1.8"},
		}
		version, projectResolved, err := ResolveToolVersion(context.Background(), inspector, "/repo", "lefthook")
		if err != nil || projectResolved || version != "lefthook version 2.1.8" {
			t.Fatalf("ResolveToolVersion() = (%q, %t, %v)", version, projectResolved, err)
		}
	})

	t.Run("an empty repository root skips the declaration probe", func(t *testing.T) {
		t.Parallel()
		inspector := &declaringInspector{
			plainInspector: plainInspector{version: "lefthook version 2.1.8"},
			declared:       true,
		}
		version, projectResolved, err := ResolveToolVersion(context.Background(), inspector, "", "lefthook")
		if err != nil || projectResolved || version != "lefthook version 2.1.8" {
			t.Fatalf("ResolveToolVersion() = (%q, %t, %v)", version, projectResolved, err)
		}
		if inspector.callCount != 0 {
			t.Fatalf("the resolver ran without a repository root: calls = %d", inspector.callCount)
		}
	})

	t.Run("an inspector without the capability uses the host path", func(t *testing.T) {
		t.Parallel()
		version, projectResolved, err := ResolveToolVersion(
			context.Background(),
			plainInspector{version: "lefthook version 2.1.8"},
			"/repo",
			"lefthook",
		)
		if err != nil || projectResolved || version != "lefthook version 2.1.8" {
			t.Fatalf("ResolveToolVersion() = (%q, %t, %v)", version, projectResolved, err)
		}
	})

	t.Run("a host path failure propagates unchanged", func(t *testing.T) {
		t.Parallel()
		expected := errors.New("lefthook missing")
		_, _, err := ResolveToolVersion(context.Background(), plainInspector{err: expected}, "/repo", "lefthook")
		if !errors.Is(err, expected) {
			t.Fatalf("ResolveToolVersion() error = %v, want %v", err, expected)
		}
	})
}
