package quality

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CyberT33N/git-governance/internal/application/port"
	"github.com/CyberT33N/git-governance/internal/domain/branch"
	"github.com/CyberT33N/git-governance/internal/domain/problem"
)

type recordedCommand struct {
	directory  string
	executable string
	arguments  []string
}

func TestRunReportsUnconfiguredRepository(t *testing.T) {
	t.Parallel()

	runner := New(Options{})
	result, err := runner.Run(context.Background(), port.RepositoryIdentity{Root: t.TempDir()}, qualityRequest(branch.FamilyFeature))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != port.QualityUnconfigured || len(result.Gates) != 0 {
		t.Fatalf("Run() = %#v", result)
	}
}

func TestRunExecutesConfiguredArgumentArrays(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, defaultConfigName)
	writeConfig(t, configPath, `{
  "schemaVersion": 2,
  "gates": [
    {"name":"unit-tests","command":"go","args":["test","./..."],"timeout":"2m"},
    {"name":"lint","command":"tool","args":["check"],"workingDirectory":"tools","timeout":"30s"}
  ]
}`)
	var calls []recordedCommand
	runner := New(Options{
		Run: func(_ context.Context, directory, executable string, arguments ...string) error {
			calls = append(calls, recordedCommand{
				directory:  directory,
				executable: executable,
				arguments:  append([]string(nil), arguments...),
			})
			return nil
		},
	})

	result, err := runner.Run(context.Background(), port.RepositoryIdentity{Root: root}, qualityRequest(branch.FamilyFeature))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != port.QualityPassed || len(result.Gates) != 2 {
		t.Fatalf("Run() = %#v", result)
	}
	if len(calls) != 2 ||
		calls[0].directory != root ||
		calls[0].executable != "go" ||
		strings.Join(calls[0].arguments, ",") != "test,./..." ||
		calls[1].directory != filepath.Join(root, "tools") {
		t.Fatalf("quality calls = %#v", calls)
	}
}

func TestRunWithFingerprintBindsTheExecutedConfiguration(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	contents := `{
  "schemaVersion": 2,
  "gates": [
    {"name":"baseline","command":"baseline"},
    {"name":"documentation","command":"docs","includeFamilies":["docs"]}
  ]
}`
	writeConfig(t, filepath.Join(root, defaultConfigName), contents)
	runner := New(Options{
		Run: func(context.Context, string, string, ...string) error {
			return nil
		},
	})
	request := qualityRequest(branch.FamilyDocs)

	result, fingerprint, err := runner.RunWithFingerprint(context.Background(), port.RepositoryIdentity{Root: root}, request)
	if err != nil || result.Status != port.QualityPassed || strings.Join(fingerprint.Gates, ",") != "baseline,documentation" ||
		len(fingerprint.ConfigurationDigest) != 64 || fingerprint.Toolchain == "" {
		t.Fatalf("RunWithFingerprint() = (%#v, %#v, %v)", result, fingerprint, err)
	}
	current, err := runner.Fingerprint(context.Background(), port.RepositoryIdentity{Root: root}, request)
	if err != nil ||
		current.ConfigurationDigest != fingerprint.ConfigurationDigest ||
		current.Toolchain != fingerprint.Toolchain ||
		strings.Join(current.Gates, ",") != strings.Join(fingerprint.Gates, ",") ||
		!strings.Contains(current.Toolchain, "/") {
		t.Fatalf("Fingerprint() = (%#v, %v), want %#v", current, err, fingerprint)
	}
	current, err = runner.Fingerprint(testNilContext(), port.RepositoryIdentity{Root: root}, request)
	if err != nil || current.ConfigurationDigest != fingerprint.ConfigurationDigest {
		t.Fatalf("nil-context Fingerprint() = (%#v, %v)", current, err)
	}

	unconfigured, err := New(Options{}).Fingerprint(
		context.Background(),
		port.RepositoryIdentity{Root: t.TempDir()},
		qualityRequest(branch.FamilyFeature),
	)
	if err != nil || unconfigured.ConfigurationDigest != "" || len(unconfigured.Gates) != 0 || unconfigured.Toolchain == "" {
		t.Fatalf("unconfigured Fingerprint() = (%#v, %v)", unconfigured, err)
	}

	_, err = New(Options{
		ReadFile: func(string) ([]byte, error) {
			return nil, errors.New("read denied")
		},
	}).Fingerprint(context.Background(), port.RepositoryIdentity{Root: root}, request)
	assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
}

func TestRunScopesGatesByBranchFamily(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeConfig(t, filepath.Join(root, defaultConfigName), `{
  "schemaVersion": 2,
  "defaults": {"includeFamilies": ["feature", "docs", "perf"]},
  "gates": [
    {"name":"baseline","command":"baseline"},
    {"name":"documentation","command":"docs","includeFamilies":["docs"]},
    {"name":"stress","command":"stress","includeFamilies":["feature","perf"]},
    {"name":"integration","command":"integration","excludeFamilies":["docs"]},
    {"name":"scratch-check","command":"scratch","includeFamilies":["scratch"]}
  ]
}`)
	var calls []string
	runner := New(Options{
		Run: func(_ context.Context, _ string, executable string, _ ...string) error {
			calls = append(calls, executable)
			return nil
		},
	})

	result, err := runner.Run(
		context.Background(),
		port.RepositoryIdentity{Root: root},
		qualityRequest(branch.FamilyDocs),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != port.QualityPassed || strings.Join(calls, ",") != "baseline,docs" {
		t.Fatalf("docs Run() = (%#v, %v)", result, calls)
	}

	calls = nil
	result, err = runner.Run(
		context.Background(),
		port.RepositoryIdentity{Root: root},
		qualityRequest(branch.FamilyFeature, branch.FamilyDocs),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != port.QualityPassed || strings.Join(calls, ",") != "baseline,docs,stress,integration" {
		t.Fatalf("multi-family Run() = (%#v, %v)", result, calls)
	}

	calls = nil
	result, err = runner.Run(
		context.Background(),
		port.RepositoryIdentity{Root: root},
		qualityRequest(branch.FamilyScratch),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != port.QualityPassed || strings.Join(calls, ",") != "scratch" {
		t.Fatalf("scratch Run() = (%#v, %v)", result, calls)
	}
}

func TestRunReportsSkippedWhenNoGateApplies(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeConfig(t, filepath.Join(root, defaultConfigName), `{
  "schemaVersion": 2,
  "gates": [{"name":"baseline","command":"baseline"}]
}`)
	runner := New(Options{
		Run: func(context.Context, string, string, ...string) error {
			t.Fatal("a skipped gate must not execute")
			return nil
		},
	})

	result, err := runner.Run(
		context.Background(),
		port.RepositoryIdentity{Root: root},
		qualityRequest(branch.FamilyScratch),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != port.QualitySkipped || len(result.Gates) != 0 {
		t.Fatalf("Run() = %#v", result)
	}
}

func TestRunnerErrorAndScopeHelperPaths(t *testing.T) {
	t.Parallel()

	t.Run("repository required", func(t *testing.T) {
		_, err := New(Options{}).Run(context.Background(), port.RepositoryIdentity{}, qualityRequest(branch.FamilyFeature))
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)
	})

	t.Run("configuration unavailable", func(t *testing.T) {
		_, err := New(Options{
			ReadFile: func(string) ([]byte, error) {
				return nil, errors.New("read denied")
			},
		}).Run(context.Background(), port.RepositoryIdentity{Root: t.TempDir()}, qualityRequest(branch.FamilyFeature))
		assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
	})

	t.Run("configuration too large", func(t *testing.T) {
		_, err := New(Options{
			ReadFile: func(string) ([]byte, error) {
				return make([]byte, maxConfigBytes+1), nil
			},
		}).Run(context.Background(), port.RepositoryIdentity{Root: t.TempDir()}, qualityRequest(branch.FamilyFeature))
		assertProblemCode(t, err, problem.CodeConfigurationInvalid)
	})

	t.Run("unknown requested family", func(t *testing.T) {
		root := t.TempDir()
		writeConfig(t, filepath.Join(root, defaultConfigName), `{"schemaVersion":2,"gates":[{"name":"test","command":"go"}]}`)
		_, err := New(Options{}).Run(context.Background(), port.RepositoryIdentity{Root: root}, qualityRequest(branch.Family("unknown")))
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("empty request skips", func(t *testing.T) {
		root := t.TempDir()
		writeConfig(t, filepath.Join(root, defaultConfigName), `{"schemaVersion":2,"gates":[{"name":"test","command":"go"}]}`)
		result, err := New(Options{
			Run: func(context.Context, string, string, ...string) error {
				t.Fatal("empty family request must not execute a gate")
				return nil
			},
		}).Run(context.Background(), port.RepositoryIdentity{Root: root}, port.QualityRequest{})
		if err != nil || result.Status != port.QualitySkipped {
			t.Fatalf("Run() = (%#v, %v)", result, err)
		}
	})

	t.Run("paths and scopes", func(t *testing.T) {
		runner := New(Options{Path: "nested/quality.json"})
		if got := runner.configPath("C:/repo"); got != filepath.Join("C:/repo", "nested", "quality.json") {
			t.Fatalf("relative config path = %q", got)
		}
		absolute, err := filepath.Abs("quality.json")
		if err != nil {
			t.Fatal(err)
		}
		runner = New(Options{Path: absolute})
		if got := runner.configPath("C:/repo"); got != filepath.Clean(absolute) {
			t.Fatalf("absolute config path = %q", got)
		}
		if gateApplies(familyScope{}, gate{}, []branch.Family{branch.FamilyScratch}) {
			t.Fatal("default scope must not include scratch")
		}
		effective := effectiveFamilies(
			familyScope{IncludeFamilies: []branch.Family{branch.FamilyFeature, branch.FamilyDocs}},
			familyScope{ExcludeFamilies: []branch.Family{branch.FamilyDocs}},
		)
		if len(effective) != 1 || effective[0] != branch.FamilyFeature {
			t.Fatalf("effective families = %v", effective)
		}
	})
}

func TestRunRejectsUnsafeOrInvalidConfigurations(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		contents string
	}{
		{
			name:     "unknown field",
			contents: `{"schemaVersion":2,"gates":[{"name":"test","command":"go","unknown":true}]}`,
		},
		{
			name:     "empty gates",
			contents: `{"schemaVersion":2,"gates":[]}`,
		},
		{
			name:     "escaping directory",
			contents: `{"schemaVersion":2,"gates":[{"name":"test","command":"go","workingDirectory":"../outside"}]}`,
		},
		{
			name:     "shell control argument",
			contents: "{\"schemaVersion\":2,\"gates\":[{\"name\":\"test\",\"command\":\"go\",\"args\":[\"test\\n./...\"]}]}",
		},
		{
			name:     "unknown family",
			contents: `{"schemaVersion":2,"gates":[{"name":"test","command":"go","includeFamilies":["unknown"]}]}`,
		},
		{
			name:     "scope overlap",
			contents: `{"schemaVersion":2,"gates":[{"name":"test","command":"go","includeFamilies":["feature"],"excludeFamilies":["feature"]}]}`,
		},
		{
			name:     "old schema",
			contents: `{"schemaVersion":1,"gates":[{"name":"test","command":"go"}]}`,
		},
		{
			name:     "duplicate scope family",
			contents: `{"schemaVersion":2,"defaults":{"includeFamilies":["feature","feature"]},"gates":[{"name":"test","command":"go"}]}`,
		},
		{
			name:     "empty command",
			contents: `{"schemaVersion":2,"gates":[{"name":"test","command":" "}]}`,
		},
		{
			name:     "multiple documents",
			contents: `{"schemaVersion":2,"gates":[{"name":"test","command":"go"}]} {}`,
		},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeConfig(t, filepath.Join(root, defaultConfigName), testCase.contents)
			_, err := New(Options{}).Run(context.Background(), port.RepositoryIdentity{Root: root}, qualityRequest(branch.FamilyFeature))
			assertProblemCode(t, err, problem.CodeConfigurationInvalid)
		})
	}
}

func TestRunClassifiesGateFailureAndCancellation(t *testing.T) {
	t.Parallel()

	t.Run("gate failure", func(t *testing.T) {
		root := t.TempDir()
		writeConfig(t, filepath.Join(root, defaultConfigName), `{"schemaVersion":2,"gates":[{"name":"test","command":"go"}]}`)
		_, err := New(Options{
			Run: func(context.Context, string, string, ...string) error {
				return errors.New("exit status 1")
			},
		}).Run(context.Background(), port.RepositoryIdentity{Root: root}, qualityRequest(branch.FamilyFeature))
		assertProblemCode(t, err, problem.CodeExternalCommandFailed)
	})

	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := New(Options{}).Run(ctx, port.RepositoryIdentity{Root: t.TempDir()}, qualityRequest(branch.FamilyFeature))
		assertProblemCode(t, err, problem.CodeOperationCancelled)
	})
}

func TestInternalTimeoutAndPathHelpers(t *testing.T) {
	t.Parallel()

	if timeout, err := gateTimeout("", time.Second); err != nil || timeout != time.Second {
		t.Fatalf("gateTimeout default = (%s, %v)", timeout, err)
	}
	if _, err := gateTimeout("zero", time.Second); err == nil {
		t.Fatal("gateTimeout accepted invalid value")
	}
	outside, err := filepath.Abs("outside")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolveWorkingDirectory(".", outside); err == nil {
		t.Fatal("resolveWorkingDirectory accepted absolute path")
	}
}

func TestRunCommandWritesChildOutputToDiagnosticWriter(t *testing.T) {
	t.Parallel()

	diagnostic := &bytes.Buffer{}
	if err := runCommand(diagnostic, context.Background(), "", "go", "version"); err != nil {
		t.Fatalf("runCommand() error = %v", err)
	}
	if !strings.Contains(diagnostic.String(), "go version") {
		t.Fatalf("diagnostic output = %q", diagnostic.String())
	}
}

func FuzzDecodeQualityConfiguration(f *testing.F) {
	for _, seed := range []string{
		`{"schemaVersion":2,"gates":[{"name":"unit-tests","command":"go","args":["test","./..."],"timeout":"2m"}]}`,
		`{"schemaVersion":2,"defaults":{"includeFamilies":["feature"]},"gates":[{"name":"stress","command":"tool","includeFamilies":["perf"]}]}`,
		`{"schemaVersion":2,"gates":[]}`,
		`{"schemaVersion":2,"gates":[{"name":"test","command":"go","workingDirectory":"../outside"}]}`,
		`{`,
		"",
		"\x00",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		_, _ = decode("fuzz-quality.json", []byte(raw))
	})
}

func writeConfig(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func qualityRequest(families ...branch.Family) port.QualityRequest {
	return port.QualityRequest{Families: families}
}

func assertProblemCode(t *testing.T, err error, expected problem.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected problem code %q, got nil", expected)
	}
	actual, ok := problem.As(err)
	if !ok {
		t.Fatalf("error %T does not carry a problem: %v", err, err)
	}
	if actual.Code != expected {
		t.Fatalf("problem code = %q, want %q", actual.Code, expected)
	}
}
