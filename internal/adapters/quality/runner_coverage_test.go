package quality

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestRunnerCoverageNormalizesNilContextAndAppliesDefaultExclusions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeConfig(t, filepath.Join(root, defaultConfigName), `{
  "schemaVersion": 2,
  "defaults": {"excludeFamilies": ["docs"]},
  "gates": [
    {"name":"default","command":"default"},
    {"name":"docs-only","command":"docs","includeFamilies":["docs"]},
    {"name":"feature-only","command":"feature","includeFamilies":["feature"]}
  ]
}`)

	var calls []string
	runner := New(Options{
		Run: func(ctx context.Context, _ string, executable string, _ ...string) error {
			if ctx == nil {
				t.Error("Run() passed a nil context to a gate")
			}
			calls = append(calls, executable)
			return nil
		},
	})

	result, err := runner.Run(testNilContext(), port.RepositoryIdentity{Root: root}, qualityRequest(branch.FamilyFeature))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != port.QualityPassed || strings.Join(calls, ",") != "default,feature" {
		t.Fatalf("Run() = (%#v, %v), want passed default and feature gates", result, calls)
	}
}

func testNilContext() context.Context {
	return nil
}

func TestRunnerCoverageRejectsDuplicateNamesAndTooManyArguments(t *testing.T) {
	t.Parallel()

	arguments := make([]string, maxArgumentCount+1)
	tooManyArguments, err := json.Marshal(config{
		SchemaVersion: currentSchema,
		Gates: []gate{{
			Name:    "many-arguments",
			Command: "tool",
			Args:    arguments,
		}},
	})
	if err != nil {
		t.Fatalf("marshal argument-limit configuration: %v", err)
	}

	testCases := []struct {
		name string
		raw  []byte
		rule string
	}{
		{
			name: "duplicate gate name",
			raw: []byte(`{
  "schemaVersion": 2,
  "gates": [
    {"name":"same","command":"first"},
    {"name":"same","command":"second"}
  ]
}`),
			rule: "gate names must be unique",
		},
		{
			name: "too many arguments",
			raw:  tooManyArguments,
			rule: "each gate may contain at most 64 arguments",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			_, err := decode("quality.json", testCase.raw)
			assertProblemCode(t, err, problem.CodeConfigurationInvalid)

			value, ok := problem.As(err)
			if !ok || value.Rule != testCase.rule {
				t.Fatalf("decode() problem = %#v, want rule %q", value, testCase.rule)
			}
		})
	}
}

func TestRunnerCoverageScopeDecisions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		defaults  familyScope
		gate      gate
		requested []branch.Family
		want      bool
	}{
		{
			name: "empty request never applies",
		},
		{
			name:      "implicit defaults include feature",
			requested: []branch.Family{branch.FamilyFeature},
			want:      true,
		},
		{
			name:      "implicit defaults omit scratch",
			requested: []branch.Family{branch.FamilyScratch},
		},
		{
			name:      "default exclusion applies to gate override",
			defaults:  familyScope{ExcludeFamilies: []branch.Family{branch.FamilyDocs}},
			gate:      gate{IncludeFamilies: []branch.Family{branch.FamilyDocs}},
			requested: []branch.Family{branch.FamilyDocs},
		},
		{
			name:      "gate inclusion replaces default inclusion",
			defaults:  familyScope{IncludeFamilies: []branch.Family{branch.FamilyDocs}},
			gate:      gate{IncludeFamilies: []branch.Family{branch.FamilyFeature}},
			requested: []branch.Family{branch.FamilyFeature},
			want:      true,
		},
		{
			name: "default and gate exclusions are combined",
			defaults: familyScope{
				IncludeFamilies: []branch.Family{branch.FamilyFeature, branch.FamilyDocs},
				ExcludeFamilies: []branch.Family{branch.FamilyDocs},
			},
			gate:      gate{ExcludeFamilies: []branch.Family{branch.FamilyFeature}},
			requested: []branch.Family{branch.FamilyFeature, branch.FamilyDocs},
		},
		{
			name:      "any requested family can apply",
			defaults:  familyScope{IncludeFamilies: []branch.Family{branch.FamilyDocs}},
			requested: []branch.Family{branch.FamilyScratch, branch.FamilyDocs},
			want:      true,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			if got := gateApplies(testCase.defaults, testCase.gate, testCase.requested); got != testCase.want {
				t.Fatalf("gateApplies(%#v, %#v, %#v) = %t, want %t", testCase.defaults, testCase.gate, testCase.requested, got, testCase.want)
			}
		})
	}
}

func TestRunnerCoverageWorkingDirectoryBoundary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	absolute := filepath.Join(t.TempDir(), "outside")
	testCases := []struct {
		name     string
		relative string
		want     string
		rejected bool
	}{
		{name: "empty defaults to root", want: root},
		{name: "clean descendant", relative: filepath.Join("tools", "..", "checks"), want: filepath.Join(root, "checks")},
		{name: "parent traversal", relative: filepath.Join("..", "outside"), rejected: true},
		{name: "absolute path", relative: absolute, rejected: true},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			got, err := resolveWorkingDirectory(root, testCase.relative)
			if testCase.rejected {
				if err == nil {
					t.Fatalf("resolveWorkingDirectory(%q, %q) = %q, want error", root, testCase.relative, got)
				}
				assertProblemCode(t, err, problem.CodeConfigurationInvalid)
				return
			}
			if err != nil || got != testCase.want {
				t.Fatalf("resolveWorkingDirectory(%q, %q) = (%q, %v), want %q", root, testCase.relative, got, err, testCase.want)
			}
		})
	}
}

func TestRunnerCoverageDistinguishesQualityStatuses(t *testing.T) {
	t.Parallel()

	t.Run("unconfigured", func(t *testing.T) {
		result, err := New(Options{
			ReadFile: func(string) ([]byte, error) {
				return nil, os.ErrNotExist
			},
		}).Run(context.Background(), port.RepositoryIdentity{Root: t.TempDir()}, qualityRequest(branch.FamilyFeature))
		if err != nil || result.Status != port.QualityUnconfigured || len(result.Gates) != 0 {
			t.Fatalf("Run() = (%#v, %v), want unconfigured", result, err)
		}
	})

	t.Run("skipped", func(t *testing.T) {
		runner := New(Options{
			ReadFile: func(string) ([]byte, error) {
				return []byte(`{"schemaVersion":2,"gates":[{"name":"docs","command":"docs","includeFamilies":["docs"]}]}`), nil
			},
			Run: func(context.Context, string, string, ...string) error {
				t.Fatal("a skipped quality gate must not run")
				return nil
			},
		})

		result, err := runner.Run(context.Background(), port.RepositoryIdentity{Root: t.TempDir()}, qualityRequest(branch.FamilyFeature))
		if err != nil || result.Status != port.QualitySkipped || len(result.Gates) != 0 {
			t.Fatalf("Run() = (%#v, %v), want skipped", result, err)
		}
	})

	t.Run("passed", func(t *testing.T) {
		root := t.TempDir()
		var calls int
		runner := New(Options{
			ReadFile: func(string) ([]byte, error) {
				return []byte(`{"schemaVersion":2,"gates":[{"name":"verified","command":"tool","args":["verify"]}]}`), nil
			},
			Run: func(_ context.Context, directory, executable string, arguments ...string) error {
				calls++
				if directory != root || executable != "tool" || strings.Join(arguments, ",") != "verify" {
					t.Errorf("gate invocation = (%q, %q, %v)", directory, executable, arguments)
				}
				return nil
			},
		})

		result, err := runner.Run(context.Background(), port.RepositoryIdentity{Root: root}, qualityRequest(branch.FamilyFeature))
		if err != nil || result.Status != port.QualityPassed || calls != 1 || len(result.Gates) != 1 || result.Gates[0].Name != "verified" {
			t.Fatalf("Run() = (%#v, %v), calls = %d; want passed verified gate", result, err, calls)
		}
	})
}

func TestRunnerCoverageHonorsTimeoutAndCancellation(t *testing.T) {
	t.Parallel()

	t.Run("configured timeout reaches command context", func(t *testing.T) {
		runner := New(Options{
			ReadFile: func(string) ([]byte, error) {
				return []byte(`{"schemaVersion":2,"gates":[{"name":"deadline","command":"tool","timeout":"10ms"}]}`), nil
			},
			Run: func(ctx context.Context, _ string, _ string, _ ...string) error {
				<-ctx.Done()
				return ctx.Err()
			},
		})

		_, err := runner.Run(context.Background(), port.RepositoryIdentity{Root: t.TempDir()}, qualityRequest(branch.FamilyFeature))
		assertProblemCode(t, err, problem.CodeExternalCommandFailed)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("timeout error = %v, want context deadline exceeded", err)
		}
	})

	t.Run("cancelled before configuration access", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		runner := New(Options{
			ReadFile: func(string) ([]byte, error) {
				t.Fatal("cancelled Run() must not read configuration")
				return nil, nil
			},
		})

		_, err := runner.Run(ctx, port.RepositoryIdentity{Root: t.TempDir()}, qualityRequest(branch.FamilyFeature))
		assertProblemCode(t, err, problem.CodeOperationCancelled)
	})
}

func TestRunnerCoverageWrapsExecutableFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	missingExecutable := filepath.Join(root, "missing-quality-executable")
	contents, err := json.Marshal(config{
		SchemaVersion: currentSchema,
		Gates: []gate{{
			Name:    "missing-executable",
			Command: missingExecutable,
		}},
	})
	if err != nil {
		t.Fatalf("marshal missing executable configuration: %v", err)
	}

	_, err = New(Options{
		ReadFile:   func(string) ([]byte, error) { return contents, nil },
		Diagnostic: &bytes.Buffer{},
	}).Run(context.Background(), port.RepositoryIdentity{Root: root}, qualityRequest(branch.FamilyFeature))
	assertProblemCode(t, err, problem.CodeExternalCommandFailed)
}

func TestRunnerCoverageRoutesChildOutputAwayFromJSONResult(t *testing.T) {
	root := t.TempDir()
	contents, err := json.Marshal(config{
		SchemaVersion: currentSchema,
		Gates: []gate{{
			Name:    "output-routing",
			Command: "go",
			Args:    []string{"version"},
		}},
	})
	if err != nil {
		t.Fatalf("marshal output configuration: %v", err)
	}

	diagnostic := &bytes.Buffer{}
	result, err := New(Options{
		ReadFile:   func(string) ([]byte, error) { return contents, nil },
		Diagnostic: diagnostic,
	}).Run(context.Background(), port.RepositoryIdentity{Root: root}, qualityRequest(branch.FamilyFeature))
	if err != nil || result.Status != port.QualityPassed {
		t.Fatalf("Run() = (%#v, %v), want passed", result, err)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal quality result: %v", err)
	}
	if !strings.Contains(diagnostic.String(), "go version") {
		t.Fatalf("diagnostic output = %q, missing command output", diagnostic.String())
	}
	if strings.Contains(string(encoded), "go version") {
		t.Fatalf("JSON result leaked child output: %s", encoded)
	}
}
