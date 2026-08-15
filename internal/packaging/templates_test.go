package packaging

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPackageManagerTemplatesMatchGoReleaserArchiveNames(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	testCases := []struct {
		path      string
		required  []string
		forbidden []string
	}{
		{
			path: filepath.Join("packaging", "homebrew", "git-governance.rb.tmpl"),
			required: []string{
				"git-governance_{{VERSION}}_darwin_amd64.tar.gz",
				"git-governance_{{VERSION}}_darwin_arm64.tar.gz",
			},
			forbidden: []string{"_Darwin_", "_x86_64"},
		},
		{
			path: filepath.Join("packaging", "scoop", "git-governance.json.tmpl"),
			required: []string{
				"git-governance_{{VERSION}}_windows_amd64.zip",
				"git-governance_{{VERSION}}_windows_arm64.zip",
				"git-governance_$version_windows_amd64.zip",
				"git-governance_$version_windows_arm64.zip",
			},
			forbidden: []string{"_Windows_", "_x86_64"},
		},
		{
			path: filepath.Join("packaging", "winget", "t33n-software.GitGovernance.installer.yaml.tmpl"),
			required: []string{
				"git-governance_{{VERSION}}_windows_amd64.zip",
				"git-governance_{{VERSION}}_windows_arm64.zip",
			},
			forbidden: []string{"_Windows_", "_x86_64"},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(filepath.Base(testCase.path), func(t *testing.T) {
			t.Parallel()

			contents, err := os.ReadFile(filepath.Join(root, testCase.path))
			if err != nil {
				t.Fatal(err)
			}
			text := string(contents)
			for _, required := range testCase.required {
				if !strings.Contains(text, required) {
					t.Fatalf("%s does not reference %q", testCase.path, required)
				}
			}
			for _, forbidden := range testCase.forbidden {
				if strings.Contains(text, forbidden) {
					t.Fatalf("%s still contains obsolete artifact identifier %q", testCase.path, forbidden)
				}
			}
		})
	}
}

func TestGoReleaserArchiveTemplateUsesGoTargetIdentifiers(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".goreleaser.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	if !strings.Contains(text, `name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"`) {
		t.Fatal("GoReleaser archive template must use .Os and .Arch identifiers")
	}
}

func TestGoReleaserGeneratedDocsUseBuildWorkspace(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	contents, err := os.ReadFile(filepath.Join(root, ".goreleaser.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, expected := range []string{
		"go run -mod=readonly ./cmd/generate-docs --out .build/generated",
		".build/generated/completions/*",
		".build/generated/man/*",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("GoReleaser configuration must include generated documentation path %q", expected)
		}
	}
	for _, forbidden := range []string{
		"--out dist/generated",
		"dist/generated/completions/*",
		"dist/generated/man/*",
		".release/generated",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("GoReleaser configuration must not use generated documentation path %q", forbidden)
		}
	}

	ignored, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ignored), "/.build/") {
		t.Fatal("build workspace must be ignored")
	}
	if strings.Contains(string(ignored), "/.release/") {
		t.Fatal("obsolete release staging directory must not be ignored")
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not determine the test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
