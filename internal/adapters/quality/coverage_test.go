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

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestQualityCoverageHelperPaths(t *testing.T) {
	t.Parallel()

	t.Run("default construction and config paths", func(t *testing.T) {
		runner := New(Options{})
		if runner.defaultTimeout != defaultTimeout {
			t.Fatalf("default timeout = %s, want %s", runner.defaultTimeout, defaultTimeout)
		}
		if got := runner.configPath("C:/repo"); got != filepath.Join("C:/repo", defaultConfigName) {
			t.Fatalf("default config path = %q", got)
		}

		absolute := filepath.Join(t.TempDir(), "quality.json")
		runner = New(Options{Path: absolute, DefaultTimeout: time.Second})
		if got := runner.configPath("C:/repo"); got != filepath.Clean(absolute) {
			t.Fatalf("absolute config path = %q", got)
		}
	})

	t.Run("scope and request validation failures", func(t *testing.T) {
		for _, scope := range []familyScope{
			{ExcludeFamilies: []branch.Family{branch.FamilyFeature, branch.FamilyFeature}},
			{IncludeFamilies: []branch.Family{branch.FamilyFeature}, ExcludeFamilies: []branch.Family{branch.FamilyFeature}},
			{ExcludeFamilies: []branch.Family{branch.Family("unknown")}},
		} {
			if err := validateScope("quality.json", "scope", scope); err == nil {
				t.Fatalf("validateScope(%#v) unexpectedly succeeded", scope)
			}
		}

		requested, err := normalizeRequestedFamilies([]branch.Family{branch.FamilyFeature, branch.FamilyFeature})
		if err != nil || len(requested) != 1 || requested[0] != branch.FamilyFeature {
			t.Fatalf("normalizeRequestedFamilies() = (%v, %v)", requested, err)
		}
	})

	t.Run("decode and process error helpers", func(t *testing.T) {
		if _, err := decode("quality.json", []byte(`{"schemaVersion":3,"toolchain":{"goVersion":"1.26.6"},"gates":[{"name":"bad\nname","command":"go"}]}`)); err == nil {
			t.Fatal("decode accepted a newline in a gate name")
		}
		if _, err := decode("quality.json", []byte(`{"schemaVersion":3,"toolchain":{"goVersion":"1.26.6"},"gates":[{"name":"test","command":"go","args":["ok"],"timeout":"0s"}]}`)); err == nil {
			t.Fatal("decode accepted a non-positive timeout")
		}
		if _, err := gateTimeout("-1s", time.Second); err == nil {
			t.Fatal("gateTimeout accepted a negative duration")
		}
		if err := unavailable("quality.json", "read", errors.New("denied")); err == nil {
			t.Fatal("unavailable returned nil")
		}

		diagnostic := &bytes.Buffer{}
		if err := runCommand(diagnostic, context.Background(), "", "go", "definitely-not-a-go-command"); err == nil {
			t.Fatal("runCommand unexpectedly succeeded")
		}
	})

	t.Run("configured gate output uses diagnostic stream", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, defaultConfigName), []byte(`{
  "schemaVersion": 3,
  "toolchain": {"goVersion":"1.26.6"},
  "gates": [{"name":"version","command":"go","args":["version"]}]
}`), 0o600); err != nil {
			t.Fatal(err)
		}
		diagnostic := &bytes.Buffer{}
		runner := New(Options{Diagnostic: diagnostic})
		result, err := runner.Run(context.Background(), port.RepositoryIdentity{Root: root}, qualityRequest(branch.FamilyFeature))
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != port.QualityPassed || !strings.Contains(diagnostic.String(), "go version") {
			t.Fatalf("quality result = %#v, diagnostic = %q", result, diagnostic.String())
		}
	})
}

func TestQualityErrorHelpersRemainTyped(t *testing.T) {
	t.Parallel()

	for _, err := range []error{
		cancelled(context.Canceled),
		unavailable("quality.json", "read", errors.New("denied")),
		invalid("quality.json", "invalid", errors.New("bad")),
	} {
		if _, ok := problem.As(err); !ok {
			t.Fatalf("helper error %T is not a problem", err)
		}
	}
}
