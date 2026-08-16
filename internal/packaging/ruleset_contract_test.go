package packaging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProtectedLineRulesetsAllowInitialCreation(t *testing.T) {
	t.Parallel()

	expectedChecks := sharedLineStatusChecks(t)
	for _, fileName := range []string{"04-release.json", "05-support.json"} {
		fileName := fileName
		t.Run(fileName, func(t *testing.T) {
			t.Parallel()

			doNotEnforceOnCreate, got := rulesetStatusChecks(t, fileName)
			if !doNotEnforceOnCreate {
				t.Fatal("required status checks must not be enforced when the protected line is first created")
			}
			if !equalStrings(got, expectedChecks) {
				t.Fatalf("required status checks = %#v, want %#v", got, expectedChecks)
			}
		})
	}
}

func TestSharedLineRulesetsMatchRequiredChecks(t *testing.T) {
	t.Parallel()

	expectedChecks := sharedLineStatusChecks(t)
	for _, fileName := range []string{
		"02-develop.json",
		"03-main.json",
		"04-release.json",
		"05-support.json",
	} {
		fileName := fileName
		t.Run(fileName, func(t *testing.T) {
			t.Parallel()

			_, got := rulesetStatusChecks(t, fileName)
			if !equalStrings(got, expectedChecks) {
				t.Fatalf("required status checks = %#v, want shared-line checks %#v", got, expectedChecks)
			}
		})
	}
}

func TestDependencyAdmissionReviewTargetsEverySharedLine(t *testing.T) {
	t.Parallel()

	workflow := strings.ReplaceAll(readWorkflow(t, "dependency-review.yml"), "\r\n", "\n")
	const pullRequestContract = `on:
  pull_request:
    branches:
      - develop
      - main
      - release/**
      - support/**`
	if !strings.Contains(workflow, pullRequestContract) {
		t.Fatalf("dependency-review workflow must target every shared line:\n%s", pullRequestContract)
	}
	if !strings.Contains(workflow, "\n  dependency-review:\n    name: "+dependencyAdmissionReviewStatusCheck+"\n") {
		t.Fatalf("dependency-review workflow must publish %q", dependencyAdmissionReviewStatusCheck)
	}
}

func TestPushProtectionsRulesetBlocksCredentialShapedArtifacts(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile(filepath.Join(
		repositoryRoot(t),
		"docs",
		"hosting-platforms",
		"github",
		"rulesets",
		"00-push-protections.json",
	))
	if err != nil {
		t.Fatal(err)
	}
	push := string(contents)

	for _, required := range []string{
		`"name": "push-protections: block secret and key shaped artifacts"`,
		`"target": "push"`,
		`"source": "t33n-software/git-governance"`,
		`"enforcement": "active"`,
		`"conditions": null`,
		`"bypass_actors": []`,
		"file_extension_restriction",
		"restricted_file_extensions",
		"file_path_restriction",
		"restricted_file_paths",
	} {
		if !strings.Contains(push, required) {
			t.Fatalf("00-push-protections.json does not contain %q", required)
		}
	}
	for _, extension := range []string{"pem", "key", "p12", "pfx", "jks", "keystore", "kdbx", "ppk", "gpg"} {
		if !strings.Contains(push, `"*.`+extension+`"`) {
			t.Fatalf("00-push-protections.json does not restrict the %q extension in glob form", extension)
		}
	}
	for _, restrictedPath := range []string{"**/.env", "**/.env.*", "**/credentials", "**/credentials.*", "**/*.tfstate", "**/*.tfstate.*"} {
		if !strings.Contains(push, `"`+restrictedPath+`"`) {
			t.Fatalf("00-push-protections.json does not restrict the %q path", restrictedPath)
		}
	}
	for _, forbidden := range []string{"ref_name", "required_status_checks", "code_scanning", "code_quality", "code_coverage"} {
		if strings.Contains(push, forbidden) {
			t.Fatalf("00-push-protections.json unexpectedly contains %q; a push ruleset has no branch targets or check bindings", forbidden)
		}
	}

	readme := normalizeWhitespace(readRepositoryDocument(t, filepath.Join("docs", "hosting-platforms", "github", "rulesets", "README.md")))
	for _, required := range []string{"00-push-protections.json", "fork network", "Team plan", "public"} {
		if !strings.Contains(readme, required) {
			t.Fatalf("Ruleset README does not document the push protections token %q", required)
		}
	}
}

func TestSharedLineRulesetsRequireCodeOwnerReview(t *testing.T) {
	t.Parallel()

	for _, fileName := range []string{
		"02-develop.json",
		"03-main.json",
		"04-release.json",
		"05-support.json",
	} {
		fileName := fileName
		t.Run(fileName, func(t *testing.T) {
			t.Parallel()

			if !rulesetRequiresCodeOwnerReview(t, fileName) {
				t.Fatal("shared-line ruleset must require code owner review")
			}
		})
	}
}

func TestCodeOwnersContractBindsTheMaintainer(t *testing.T) {
	t.Parallel()

	contents := readRepositoryDocument(t, filepath.Join(".github", "CODEOWNERS"))

	defaultFound := false
	for _, line := range strings.Split(strings.ReplaceAll(contents, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			t.Fatalf("CODEOWNERS entry %q must map a path pattern to at least one owner", line)
		}
		for _, owner := range fields[1:] {
			if !strings.HasPrefix(owner, "@") {
				t.Fatalf("CODEOWNERS owner %q must be a GitHub user or team reference", owner)
			}
		}
		if fields[0] == "*" {
			defaultFound = true
			if fields[1] != "@CyberT33N" {
				t.Fatalf("CODEOWNERS default owner = %q, want @CyberT33N", fields[1])
			}
		}
	}
	if !defaultFound {
		t.Fatal("CODEOWNERS must define the default * ownership entry")
	}

	readme := normalizeWhitespace(readRepositoryDocument(t, filepath.Join("docs", "hosting-platforms", "github", "rulesets", "README.md")))
	for _, required := range []string{"CODEOWNERS", "require_code_owner_review", "@CyberT33N"} {
		if !strings.Contains(readme, required) {
			t.Fatalf("Ruleset README does not document the code owner token %q", required)
		}
	}
}

func readRepositoryDocument(t *testing.T, path string) string {
	t.Helper()

	contents, err := os.ReadFile(filepath.Join(repositoryRoot(t), filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func normalizeWhitespace(content string) string {
	return strings.Join(strings.Fields(content), " ")
}

func rulesetStatusChecks(t *testing.T, fileName string) (bool, []string) {
	t.Helper()

	contents, err := os.ReadFile(filepath.Join(
		repositoryRoot(t),
		"docs",
		"hosting-platforms",
		"github",
		"rulesets",
		fileName,
	))
	if err != nil {
		t.Fatal(err)
	}

	var ruleset struct {
		Rules []struct {
			Type       string `json:"type"`
			Parameters struct {
				DoNotEnforceOnCreate bool `json:"do_not_enforce_on_create"`
				RequiredStatusChecks []struct {
					Context string `json:"context"`
				} `json:"required_status_checks"`
			} `json:"parameters"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(contents, &ruleset); err != nil {
		t.Fatal(err)
	}

	for _, rule := range ruleset.Rules {
		if rule.Type == "required_status_checks" {
			return rule.Parameters.DoNotEnforceOnCreate, statusCheckContexts(rule.Parameters.RequiredStatusChecks)
		}
	}
	t.Fatal("ruleset does not define required status checks")
	return false, nil
}

func rulesetRequiresCodeOwnerReview(t *testing.T, fileName string) bool {
	t.Helper()

	contents, err := os.ReadFile(filepath.Join(
		repositoryRoot(t),
		"docs",
		"hosting-platforms",
		"github",
		"rulesets",
		fileName,
	))
	if err != nil {
		t.Fatal(err)
	}

	var ruleset struct {
		Rules []struct {
			Type       string `json:"type"`
			Parameters struct {
				RequireCodeOwnerReview bool `json:"require_code_owner_review"`
			} `json:"parameters"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(contents, &ruleset); err != nil {
		t.Fatal(err)
	}

	for _, rule := range ruleset.Rules {
		if rule.Type == "pull_request" {
			return rule.Parameters.RequireCodeOwnerReview
		}
	}
	t.Fatal("ruleset does not define a pull_request rule")
	return false
}

const dependencyAdmissionReviewStatusCheck = "Dependency admission review"

func sharedLineStatusChecks(t *testing.T) []string {
	t.Helper()

	return append(ciQualityStatusChecks(t), dependencyAdmissionReviewStatusCheck)
}

func ciQualityStatusChecks(t *testing.T) []string {
	t.Helper()

	workflow := strings.ReplaceAll(readWorkflow(t, "ci.yml"), "\r\n", "\n")
	const (
		qualityJobStart     = "\n  quality:\n"
		nativeSmokeJobStart = "\n  native-smoke:\n"
		matrixEntryPrefix   = "          - name: "
	)
	start := strings.Index(workflow, qualityJobStart)
	if start == -1 {
		t.Fatal("CI workflow does not define the quality job")
	}
	end := strings.Index(workflow[start+len(qualityJobStart):], nativeSmokeJobStart)
	if end == -1 {
		t.Fatal("CI workflow does not define the native-smoke job after the quality job")
	}
	qualityJob := workflow[start : start+len(qualityJobStart)+end]
	if !strings.Contains(qualityJob, "name: Quality gates (${{ matrix.name }})") {
		t.Fatal("CI quality job must publish matrix-specific Quality gates check names")
	}

	checks := make([]string, 0)
	seen := make(map[string]struct{})
	for _, line := range strings.Split(qualityJob, "\n") {
		if !strings.HasPrefix(line, matrixEntryPrefix) {
			continue
		}
		matrixName := strings.TrimPrefix(line, matrixEntryPrefix)
		if matrixName == "" {
			t.Fatal("CI quality matrix contains an empty check-name suffix")
		}
		if _, duplicate := seen[matrixName]; duplicate {
			t.Fatalf("CI quality matrix repeats check-name suffix %q", matrixName)
		}
		seen[matrixName] = struct{}{}
		checks = append(checks, fmt.Sprintf("Quality gates (%s)", matrixName))
	}
	if len(checks) == 0 {
		t.Fatal("CI quality matrix does not define any required status checks")
	}
	return checks
}

func statusCheckContexts(checks []struct {
	Context string `json:"context"`
}) []string {
	contexts := make([]string, len(checks))
	for index, check := range checks {
		contexts[index] = check.Context
	}
	return contexts
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
