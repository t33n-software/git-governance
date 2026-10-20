package system

import (
	"context"
	"errors"
	"os"
	"testing"
)

func testProjectToolInspector(options Options) *Inspector {
	if options.ReadFile == nil {
		options.ReadFile = func(string) ([]byte, error) {
			return nil, os.ErrNotExist
		}
	}
	if options.LookPath == nil {
		options.LookPath = func(string) (string, error) { return "/usr/local/bin/go", nil }
	}
	if options.Run == nil {
		options.Run = func(context.Context, string, ...string) ([]byte, error) {
			return []byte("lefthook version 2.2.2\n"), nil
		}
	}
	return New(options)
}

func TestGoModDeclaresTool(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		contents string
		tool     string
		expected bool
	}{
		{
			name:     "single-line tool directive",
			contents: "module example.com/repo\n\ngo 1.27\n\ntool github.com/evilmartians/lefthook/v2\n",
			tool:     "lefthook",
			expected: true,
		},
		{
			name:     "block tool directive",
			contents: "tool (\n\tgithub.com/evilmartians/lefthook/v2\n\tgolang.org/x/vuln/cmd/govulncheck\n)\n",
			tool:     "govulncheck",
			expected: true,
		},
		{
			name:     "major suffix removal maps the module path",
			contents: "tool github.com/evilmartians/lefthook/v2\n",
			tool:     "lefthook",
			expected: true,
		},
		{
			name:     "unrelated tool does not match",
			contents: "tool honnef.co/go/tools/cmd/staticcheck\n",
			tool:     "lefthook",
			expected: false,
		},
		{
			name:     "no tool directive",
			contents: "module example.com/repo\n\ngo 1.27\n",
			tool:     "lefthook",
			expected: false,
		},
		{
			name:     "tool word inside a comment line does not match",
			contents: "// tool github.com/evilmartians/lefthook/v2\n",
			tool:     "lefthook",
			expected: false,
		},
		{
			name:     "a closed block without a match does not match",
			contents: "tool (\n\thonnef.co/go/tools/cmd/staticcheck\n)\n\nrequire github.com/example/thing v1.0.0\n",
			tool:     "lefthook",
			expected: false,
		},
		{
			name:     "a bare major marker is not a version segment",
			contents: "tool github.com/evilmartians/lefthook/v\n",
			tool:     "lefthook",
			expected: false,
		},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := goModDeclaresTool([]byte(testCase.contents), testCase.tool); got != testCase.expected {
				t.Fatalf("goModDeclaresTool() = %t, want %t", got, testCase.expected)
			}
		})
	}
}

func TestResolveProjectToolVersion(t *testing.T) {
	t.Parallel()

	t.Run("missing arguments resolve as an absent declaration", func(t *testing.T) {
		t.Parallel()
		inspector := testProjectToolInspector(Options{})
		if _, declared, err := inspector.ResolveProjectToolVersion(context.Background(), "", "lefthook"); err != nil || declared {
			t.Fatalf("empty root = (%t, %v)", declared, err)
		}
		if _, declared, err := inspector.ResolveProjectToolVersion(context.Background(), "/repo", ""); err != nil || declared {
			t.Fatalf("empty tool = (%t, %v)", declared, err)
		}
	})

	t.Run("an absent go.mod is an absent declaration", func(t *testing.T) {
		t.Parallel()
		inspector := testProjectToolInspector(Options{})
		version, declared, err := inspector.ResolveProjectToolVersion(context.Background(), "/repo", "lefthook")
		if err != nil || declared || version != "" {
			t.Fatalf("ResolveProjectToolVersion() = (%q, %t, %v)", version, declared, err)
		}
	})

	t.Run("a go.mod without the tool directive is an absent declaration", func(t *testing.T) {
		t.Parallel()
		inspector := testProjectToolInspector(Options{
			ReadFile: func(string) ([]byte, error) {
				return []byte("module example.com/repo\n\ngo 1.27\n"), nil
			},
		})
		_, declared, err := inspector.ResolveProjectToolVersion(context.Background(), "/repo", "lefthook")
		if err != nil || declared {
			t.Fatalf("undeclared tool = (%t, %v)", declared, err)
		}
	})

	t.Run("a declared tool resolves through the project tool surface", func(t *testing.T) {
		t.Parallel()
		var (
			resolved  string
			arguments []string
		)
		inspector := testProjectToolInspector(Options{
			ReadFile: func(string) ([]byte, error) {
				return []byte("tool github.com/evilmartians/lefthook/v2\n"), nil
			},
			Run: func(_ context.Context, executable string, values ...string) ([]byte, error) {
				resolved = executable
				arguments = append([]string(nil), values...)
				return []byte("lefthook version 2.2.2\nextra"), nil
			},
		})
		version, declared, err := inspector.ResolveProjectToolVersion(context.Background(), "/repo", "lefthook")
		if err != nil || !declared || version != "lefthook version 2.2.2" {
			t.Fatalf("ResolveProjectToolVersion() = (%q, %t, %v)", version, declared, err)
		}
		if resolved != "/usr/local/bin/go" || len(arguments) != 3 ||
			arguments[0] != "tool" || arguments[1] != "lefthook" || arguments[2] != "--version" {
			t.Fatalf("project tool invocation = %q %v", resolved, arguments)
		}
	})

	t.Run("an unreadable go.mod fails closed", func(t *testing.T) {
		t.Parallel()
		inspector := testProjectToolInspector(Options{
			ReadFile: func(string) ([]byte, error) { return nil, errors.New("permission denied") },
		})
		_, declared, err := inspector.ResolveProjectToolVersion(context.Background(), "/repo", "lefthook")
		if err == nil || declared {
			t.Fatalf("unreadable go.mod = (%t, %v)", declared, err)
		}
	})

	t.Run("a failing tool run reports the declared failure", func(t *testing.T) {
		t.Parallel()
		inspector := testProjectToolInspector(Options{
			ReadFile: func(string) ([]byte, error) {
				return []byte("tool github.com/evilmartians/lefthook/v2\n"), nil
			},
			Run: func(context.Context, string, ...string) ([]byte, error) {
				return nil, errors.New("build failed")
			},
		})
		_, declared, err := inspector.ResolveProjectToolVersion(context.Background(), "/repo", "lefthook")
		if err == nil || !declared {
			t.Fatalf("failing tool run = (%t, %v)", declared, err)
		}
	})

	t.Run("a missing go toolchain lookup reports the declared failure", func(t *testing.T) {
		t.Parallel()
		inspector := testProjectToolInspector(Options{
			ReadFile: func(string) ([]byte, error) {
				return []byte("tool github.com/evilmartians/lefthook/v2\n"), nil
			},
			LookPath: func(string) (string, error) { return "", errors.New("go not found") },
		})
		_, declared, err := inspector.ResolveProjectToolVersion(context.Background(), "/repo", "lefthook")
		if err == nil || !declared {
			t.Fatalf("missing go lookup = (%t, %v)", declared, err)
		}
	})

	t.Run("an empty tool output fails closed", func(t *testing.T) {
		t.Parallel()
		inspector := testProjectToolInspector(Options{
			ReadFile: func(string) ([]byte, error) {
				return []byte("tool github.com/evilmartians/lefthook/v2\n"), nil
			},
			Run: func(context.Context, string, ...string) ([]byte, error) { return []byte("\n"), nil },
		})
		_, declared, err := inspector.ResolveProjectToolVersion(context.Background(), "/repo", "lefthook")
		if err == nil || !declared {
			t.Fatalf("empty tool output = (%t, %v)", declared, err)
		}
	})
}
