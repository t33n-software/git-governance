package packaging

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var (
	sharedLineRulesetFullFiles = []string{
		"02-develop.quality-gates-full.json",
		"03-main.quality-gates-full.json",
		"04-release.quality-gates-full.json",
		"05-support.quality-gates-full.json",
	}
	sharedLineRulesetLinuxOnlyFiles = []string{
		"02-develop.quality-gates-linux-only.json",
		"03-main.quality-gates-linux-only.json",
		"04-release.quality-gates-linux-only.json",
		"05-support.quality-gates-linux-only.json",
	}
	protectedLineRulesetFiles = []string{
		"04-release.quality-gates-full.json",
		"04-release.quality-gates-linux-only.json",
		"05-support.quality-gates-full.json",
		"05-support.quality-gates-linux-only.json",
	}
)

func TestProtectedLineRulesetsAllowInitialCreation(t *testing.T) {
	t.Parallel()

	for _, fileName := range protectedLineRulesetFiles {
		fileName := fileName
		t.Run(fileName, func(t *testing.T) {
			t.Parallel()

			doNotEnforceOnCreate, got := rulesetStatusChecks(t, fileName)
			if !doNotEnforceOnCreate {
				t.Fatal("required status checks must not be enforced when the protected line is first created")
			}
			expectedChecks := expectedSharedLineChecks(t, fileName)
			if !equalStrings(got, expectedChecks) {
				t.Fatalf("required status checks = %#v, want %#v", got, expectedChecks)
			}
		})
	}
}

func TestSharedLineRulesetsMatchRequiredChecks(t *testing.T) {
	t.Parallel()

	for _, fileName := range append(
		append([]string{}, sharedLineRulesetFullFiles...),
		sharedLineRulesetLinuxOnlyFiles...,
	) {
		fileName := fileName
		t.Run(fileName, func(t *testing.T) {
			t.Parallel()

			_, got := rulesetStatusChecks(t, fileName)
			expectedChecks := expectedSharedLineChecks(t, fileName)
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
    branches: [main, develop, "release/**", "support/**"]`
	if !strings.Contains(workflow, pullRequestContract) {
		t.Fatalf("dependency-review caller must target every shared line:\n%s", pullRequestContract)
	}
	if !strings.Contains(workflow, "\n  dependency-review:\n    name: Dependency review\n") {
		t.Fatal("dependency-review caller must publish the lane identity \"Dependency review\"")
	}
}

func TestPushProtectionsRulesetBlocksCredentialShapedArtifacts(t *testing.T) {
	t.Parallel()

	push := rulesetDocument(t, "00-push-protections.json")

	for _, required := range []string{
		`"name": "push-protections: secret artifact boundary"`,
		`"target": "push"`,
		`"source": "t33n-software"`,
		`"enforcement": "active"`,
		`"bypass_actors": []`,
		"repository_property",
		`"name": "visibility"`,
		`"private"`,
		`"internal"`,
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

	readme := normalizeWhitespace(readRepositoryDocument(t, filepath.Join("rulesets", "github", "README.md")))
	for _, required := range []string{"00-push-protections.json", "fork network", "Team plan", "public"} {
		if !strings.Contains(readme, required) {
			t.Fatalf("Ruleset README does not document the push protections token %q", required)
		}
	}
}

func TestTagRulesetsBindTheVersionTagNamespace(t *testing.T) {
	t.Parallel()

	versionTags := rulesetDocument(t, "07-release-version-tags.json")
	for _, required := range []string{
		`"name": "tag-governance: release version tags"`,
		`"target": "tag"`,
		`"source": "t33n-software"`,
		`"refs/tags/v*"`,
		`"type": "creation"`,
		`"type": "update"`,
		`"type": "deletion"`,
		`"actor_type": "Integration"`,
		`"actor_type": "OrganizationAdmin"`,
		`"bypass_mode": "always"`,
	} {
		if !strings.Contains(versionTags, required) {
			t.Fatalf("07-release-version-tags.json does not contain %q", required)
		}
	}
	for _, forbidden := range []string{"exempt", "required_status_checks", "pull_request", "quality-gates"} {
		if strings.Contains(versionTags, forbidden) {
			t.Fatalf("07-release-version-tags.json unexpectedly contains %q", forbidden)
		}
	}

	floor := rulesetDocument(t, "08-tag-namespace-floor.json")
	for _, required := range []string{
		`"name": "tag-governance: tag namespace floor"`,
		`"target": "tag"`,
		`"refs/tags/*"`,
		`"refs/tags/v*"`,
		`"type": "creation"`,
		`"type": "update"`,
		`"actor_type": "OrganizationAdmin"`,
		`"bypass_mode": "always"`,
	} {
		if !strings.Contains(floor, required) {
			t.Fatalf("08-tag-namespace-floor.json does not contain %q", required)
		}
	}
	for _, forbidden := range []string{`"type": "deletion"`, "exempt", "Integration", "quality-gates"} {
		if strings.Contains(floor, forbidden) {
			t.Fatalf("08-tag-namespace-floor.json unexpectedly contains %q", forbidden)
		}
	}

	readme := normalizeWhitespace(readRepositoryDocument(t, filepath.Join("rulesets", "github", "README.md")))
	for _, required := range []string{"07-release-version-tags.json", "08-tag-namespace-floor.json", "disabled", "break-glass"} {
		if !strings.Contains(readme, required) {
			t.Fatalf("Ruleset README does not document the tag governance token %q", required)
		}
	}
}

func TestSharedLineRulesetsRequireCodeOwnerReview(t *testing.T) {
	t.Parallel()

	for _, fileName := range append(
		append([]string{}, sharedLineRulesetFullFiles...),
		sharedLineRulesetLinuxOnlyFiles...,
	) {
		fileName := fileName
		t.Run(fileName, func(t *testing.T) {
			t.Parallel()

			if !rulesetRequiresCodeOwnerReview(t, fileName) {
				t.Fatal("shared-line ruleset must require code owner review")
			}
		})
	}
}

func TestRulesetIdentityTripleBindsTitleSelectorAndFile(t *testing.T) {
	t.Parallel()

	for _, fileName := range append(
		append([]string{}, sharedLineRulesetFullFiles...),
		sharedLineRulesetLinuxOnlyFiles...,
	) {
		fileName := fileName
		t.Run(fileName, func(t *testing.T) {
			t.Parallel()

			class := rulesetClassFromFileName(t, fileName)
			ruleset := parseRuleset(t, fileName)

			if !strings.HasSuffix(ruleset.Name, "(quality-gates="+class+")") {
				t.Fatalf("ruleset name %q must declare its class as (quality-gates=%s)", ruleset.Name, class)
			}

			includes := ruleset.Conditions.RepositoryProperty.Include
			if len(includes) != 1 {
				t.Fatalf("shared-line ruleset must bind exactly one repository property selector, got %d", len(includes))
			}
			selector := includes[0]
			if selector.Name != "quality-gates" {
				t.Fatalf("repository property selector = %q, want %q", selector.Name, "quality-gates")
			}
			if len(selector.PropertyValues) != 1 || selector.PropertyValues[0] != class {
				t.Fatalf("repository property values = %#v, want [%q]", selector.PropertyValues, class)
			}
		})
	}

	t.Run("01-ticket-working-branches.json", func(t *testing.T) {
		t.Parallel()

		ruleset := parseRuleset(t, "01-ticket-working-branches.json")
		if strings.Contains(ruleset.Name, "quality-gates") {
			t.Fatalf("classless ruleset name %q must not declare a quality-gates class", ruleset.Name)
		}
		includes := ruleset.Conditions.RepositoryName.Include
		if len(includes) != 1 || includes[0] != "~ALL" {
			t.Fatalf("classless ruleset must target ~ALL repositories, got %#v", includes)
		}
	})
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

	readme := normalizeWhitespace(readRepositoryDocument(t, filepath.Join("rulesets", "github", "README.md")))
	for _, required := range []string{"CODEOWNERS", "require_code_owner_review", "@CyberT33N"} {
		if !strings.Contains(readme, required) {
			t.Fatalf("Ruleset README does not document the code owner token %q", required)
		}
	}
}

func rulesetClassFromFileName(t *testing.T, fileName string) string {
	t.Helper()

	const (
		fullSuffix      = ".quality-gates-full.json"
		linuxOnlySuffix = ".quality-gates-linux-only.json"
	)
	switch {
	case strings.HasSuffix(fileName, fullSuffix):
		return "full"
	case strings.HasSuffix(fileName, linuxOnlySuffix):
		return "linux-only"
	default:
		t.Fatalf("file name %q does not carry a quality-gates class suffix", fileName)
		return ""
	}
}

func expectedSharedLineChecks(t *testing.T, fileName string) []string {
	t.Helper()

	if rulesetClassFromFileName(t, fileName) == "linux-only" {
		return linuxOnlyStatusChecks()
	}
	return sharedLineStatusChecks()
}

func readRepositoryDocument(t *testing.T, path string) string {
	t.Helper()

	contents, err := os.ReadFile(filepath.Join(repositoryRoot(t), filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func rulesetDocument(t *testing.T, fileName string) string {
	t.Helper()

	return readRepositoryDocument(t, filepath.Join("rulesets", "github", fileName))
}

func normalizeWhitespace(content string) string {
	return strings.Join(strings.Fields(content), " ")
}

type rulesetDefinition struct {
	Name       string `json:"name"`
	Conditions struct {
		RepositoryProperty struct {
			Include []struct {
				Name           string   `json:"name"`
				PropertyValues []string `json:"property_values"`
			} `json:"include"`
		} `json:"repository_property"`
		RepositoryName struct {
			Include []string `json:"include"`
		} `json:"repository_name"`
	} `json:"conditions"`
	Rules []struct {
		Type       string `json:"type"`
		Parameters struct {
			DoNotEnforceOnCreate bool `json:"do_not_enforce_on_create"`
			RequiredStatusChecks []struct {
				Context string `json:"context"`
			} `json:"required_status_checks"`
			RequireCodeOwnerReview bool `json:"require_code_owner_review"`
		} `json:"parameters"`
	} `json:"rules"`
}

func parseRuleset(t *testing.T, fileName string) rulesetDefinition {
	t.Helper()

	var ruleset rulesetDefinition
	if err := json.Unmarshal([]byte(rulesetDocument(t, fileName)), &ruleset); err != nil {
		t.Fatal(err)
	}
	return ruleset
}

func rulesetStatusChecks(t *testing.T, fileName string) (bool, []string) {
	t.Helper()

	ruleset := parseRuleset(t, fileName)
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

	ruleset := parseRuleset(t, fileName)
	for _, rule := range ruleset.Rules {
		if rule.Type == "pull_request" {
			return rule.Parameters.RequireCodeOwnerReview
		}
	}
	t.Fatal("ruleset does not define a pull_request rule")
	return false
}

const dependencyAdmissionReviewStatusCheck = "Dependency admission review"

// The canonical composite check contexts bound by the linux-only shared-line
// rulesets. The canonical callers (repository-governance) emit them per the
// naming law (GIT_GITHUB_ACTIONS_NAMING_CONVENTIONS_001): the caller job
// carries the lane identity, the callee job carries the gate or variant
// identity.
const (
	compositeQualityGateLinuxStatusCheck = "Quality gates / linux-amd64"
	compositeDependencyReviewStatusCheck = "Dependency review / Dependency admission review"
)

// The inline-era check contexts the full-class shared-line rulesets keep
// until their composite migration (GOV-98). They are static on purpose: the
// dogfooded thin caller no longer carries the inline matrix they were derived
// from, and the ruleset sources migrate only after the composite producer has
// been proven on the real dogfooding pull request (the proof-before-binding
// sequencing).
var inlineEraFullClassQualityStatusChecks = []string{
	"Quality gates (linux-amd64)",
	"Quality gates (macos-arm64)",
	"Quality gates (windows-amd64)",
}

func sharedLineStatusChecks() []string {
	checks := append([]string{}, inlineEraFullClassQualityStatusChecks...)
	return append(checks, dependencyAdmissionReviewStatusCheck)
}

func linuxOnlyStatusChecks() []string {
	return []string{compositeQualityGateLinuxStatusCheck, compositeDependencyReviewStatusCheck}
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
