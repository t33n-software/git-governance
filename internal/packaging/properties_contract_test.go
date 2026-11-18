package packaging

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

type propertyDefinition struct {
	ValueType        string   `json:"value_type"`
	Required         bool     `json:"required"`
	DefaultValue     string   `json:"default_value"`
	Description      string   `json:"description"`
	AllowedValues    []string `json:"allowed_values"`
	ValuesEditableBy string   `json:"values_editable_by"`
}

func TestQualityGatesPropertyDefinitionMatchesRulesetClassPartition(t *testing.T) {
	t.Parallel()

	definition := parsePropertyDefinition(t, "quality-gates.json")

	if definition.ValueType != "single_select" {
		t.Fatalf("quality-gates value_type = %q, want %q", definition.ValueType, "single_select")
	}
	if !definition.Required {
		t.Fatal("quality-gates must be a required property so every repository carries an explicit classification")
	}
	if definition.ValuesEditableBy != "org_actors" {
		t.Fatalf("quality-gates values_editable_by = %q, want %q; a repository must never reclassify itself into a weaker ruleset class", definition.ValuesEditableBy, "org_actors")
	}
	if definition.DefaultValue != "pending" {
		t.Fatalf("quality-gates default_value = %q, want the onboarding value %q", definition.DefaultValue, "pending")
	}
	// The custom-property REST surface rejects a description longer than 255
	// characters (HTTP 422), proven live; the canonical artifact must apply
	// without a manual shortening at import time.
	if definition.Description == "" || len(definition.Description) > 255 {
		t.Fatalf("quality-gates description length = %d, want a non-empty description within the 255-character provider limit", len(definition.Description))
	}

	classes := map[string]struct{}{}
	for _, fileName := range append(
		append([]string{}, sharedLineRulesetFullFiles...),
		sharedLineRulesetLinuxOnlyFiles...,
	) {
		classes[rulesetClassFromFileName(t, fileName)] = struct{}{}
	}

	allowed := map[string]struct{}{}
	for _, value := range definition.AllowedValues {
		if _, duplicate := allowed[value]; duplicate {
			t.Fatalf("quality-gates allowed_values repeats %q", value)
		}
		allowed[value] = struct{}{}
	}

	for class := range classes {
		if _, ok := allowed[class]; !ok {
			t.Fatalf("quality-gates allowed_values misses the ruleset class %q", class)
		}
	}
	for value := range allowed {
		if _, isClass := classes[value]; !isClass && value != "pending" {
			t.Fatalf("quality-gates allowed_values contains %q, which is neither a ruleset class nor the onboarding value %q", value, "pending")
		}
	}

	for _, fileName := range append(
		append([]string{}, sharedLineRulesetFullFiles...),
		sharedLineRulesetLinuxOnlyFiles...,
	) {
		fileName := fileName
		t.Run(fileName, func(t *testing.T) {
			t.Parallel()

			ruleset := parseRuleset(t, fileName)
			includes := ruleset.Conditions.RepositoryProperty.Include
			if len(includes) != 1 {
				t.Fatalf("shared-line ruleset must bind exactly one repository property selector, got %d", len(includes))
			}
			for _, value := range includes[0].PropertyValues {
				if _, ok := allowed[value]; !ok {
					t.Fatalf("ruleset selector value %q is not an allowed value of the quality-gates property definition", value)
				}
			}
		})
	}
}

func TestQualityGatesPropertyDefinitionIsDocumented(t *testing.T) {
	t.Parallel()

	propertiesReadme := normalizeWhitespace(readRepositoryDocument(t, filepath.Join("properties", "github", "README.md")))
	for _, required := range []string{"quality-gates", "single_select", "org_actors", "pending"} {
		if !strings.Contains(propertiesReadme, required) {
			t.Fatalf("properties README does not document the token %q", required)
		}
	}

	rulesetsReadme := normalizeWhitespace(readRepositoryDocument(t, filepath.Join("rulesets", "github", "README.md")))
	if !strings.Contains(rulesetsReadme, "properties/github") {
		t.Fatal("ruleset README must reference the canonical properties/github definition artifact")
	}

	convention := normalizeWhitespace(readRepositoryDocument(t, filepath.Join("docs", "conventions", "hosting-platforms", "github", "custom-properties", "README.md")))
	for _, required := range []string{"positive-list", "org_actors", "pending"} {
		if !strings.Contains(convention, required) {
			t.Fatalf("custom-properties convention does not document the token %q", required)
		}
	}
}

func TestSecurityScanningPropertyDefinitionMatchesCapabilitySelector(t *testing.T) {
	t.Parallel()

	definition := parsePropertyDefinition(t, "security-scanning.json")

	if definition.ValueType != "single_select" {
		t.Fatalf("security-scanning value_type = %q, want %q", definition.ValueType, "single_select")
	}
	if !definition.Required {
		t.Fatal("security-scanning must be a required property so every repository carries an explicit capability decision")
	}
	if definition.ValuesEditableBy != "org_actors" {
		t.Fatalf("security-scanning values_editable_by = %q, want %q; a repository must never opt itself out of the security capability", definition.ValuesEditableBy, "org_actors")
	}
	if definition.DefaultValue != "enabled" {
		t.Fatalf("security-scanning default_value = %q, want the target-architecture value %q; a cost or plan reason must never be burned into the canonical default", definition.DefaultValue, "enabled")
	}
	// The custom-property REST surface rejects a description longer than 255
	// characters (HTTP 422), proven live; the canonical artifact must apply
	// without a manual shortening at import time.
	if definition.Description == "" || len(definition.Description) > 255 {
		t.Fatalf("security-scanning description length = %d, want a non-empty description within the 255-character provider limit", len(definition.Description))
	}

	allowed := map[string]struct{}{}
	for _, value := range definition.AllowedValues {
		if _, duplicate := allowed[value]; duplicate {
			t.Fatalf("security-scanning allowed_values repeats %q", value)
		}
		allowed[value] = struct{}{}
	}
	for _, value := range []string{"enabled", "disabled"} {
		if _, ok := allowed[value]; !ok {
			t.Fatalf("security-scanning allowed_values misses %q", value)
		}
	}
	if _, ok := allowed["pending"]; ok {
		t.Fatal("security-scanning must not carry the onboarding value pending; the capability axis has no workflow-alignment state")
	}

	ruleset := parseRuleset(t, securityScanningRulesetFile)
	includes := ruleset.Conditions.RepositoryProperty.Include
	if len(includes) != 1 {
		t.Fatalf("capability ruleset must bind exactly one repository property selector, got %d", len(includes))
	}
	for _, value := range includes[0].PropertyValues {
		if _, ok := allowed[value]; !ok {
			t.Fatalf("capability selector value %q is not an allowed value of the security-scanning property definition", value)
		}
	}
}

func TestSecurityScanningPropertyDefinitionIsDocumented(t *testing.T) {
	t.Parallel()

	propertiesReadme := normalizeWhitespace(readRepositoryDocument(t, filepath.Join("properties", "github", "README.md")))
	for _, required := range []string{"security-scanning", "enabled", "disabled", "09-security-scanning.json"} {
		if !strings.Contains(propertiesReadme, required) {
			t.Fatalf("properties README does not document the capability token %q", required)
		}
	}

	convention := normalizeWhitespace(readRepositoryDocument(t, filepath.Join("docs", "conventions", "hosting-platforms", "github", "rulesets", "security-scanning-capability.md")))
	for _, required := range []string{"09-security-scanning.json", "default_value: enabled", "do_not_enforce_on_create", "emitter-consistency"} {
		if !strings.Contains(convention, required) {
			t.Fatalf("security-scanning capability convention does not document the token %q", required)
		}
	}
}

func TestCanonicalConformancePropertyDefinitionMatchesCapabilitySelector(t *testing.T) {
	t.Parallel()

	definition := parsePropertyDefinition(t, "canonical-conformance.json")

	if definition.ValueType != "single_select" {
		t.Fatalf("canonical-conformance value_type = %q, want %q", definition.ValueType, "single_select")
	}
	if !definition.Required {
		t.Fatal("canonical-conformance must be a required property so every repository carries an explicit capability decision")
	}
	if definition.ValuesEditableBy != "org_actors" {
		t.Fatalf("canonical-conformance values_editable_by = %q, want %q; a repository must never opt itself out of the conformance capability", definition.ValuesEditableBy, "org_actors")
	}
	if definition.DefaultValue != "enabled" {
		t.Fatalf("canonical-conformance default_value = %q, want the target-architecture value %q; the structural exception must never be burned into the canonical default", definition.DefaultValue, "enabled")
	}
	// The custom-property REST surface rejects a description longer than 255
	// characters (HTTP 422), proven live; the canonical artifact must apply
	// without a manual shortening at import time.
	if definition.Description == "" || len(definition.Description) > 255 {
		t.Fatalf("canonical-conformance description length = %d, want a non-empty description within the 255-character provider limit", len(definition.Description))
	}

	allowed := map[string]struct{}{}
	for _, value := range definition.AllowedValues {
		if _, duplicate := allowed[value]; duplicate {
			t.Fatalf("canonical-conformance allowed_values repeats %q", value)
		}
		allowed[value] = struct{}{}
	}
	for _, value := range []string{"enabled", "disabled"} {
		if _, ok := allowed[value]; !ok {
			t.Fatalf("canonical-conformance allowed_values misses %q", value)
		}
	}
	if _, ok := allowed["pending"]; ok {
		t.Fatal("canonical-conformance must not carry the onboarding value pending; the emitter-less state is the positive disabled assignment of this capability axis, never a third value")
	}

	ruleset := parseRuleset(t, canonicalConformanceRulesetFile)
	includes := ruleset.Conditions.RepositoryProperty.Include
	if len(includes) != 1 {
		t.Fatalf("capability ruleset must bind exactly one repository property selector, got %d", len(includes))
	}
	for _, value := range includes[0].PropertyValues {
		if _, ok := allowed[value]; !ok {
			t.Fatalf("capability selector value %q is not an allowed value of the canonical-conformance property definition", value)
		}
	}
}

func TestCanonicalConformancePropertyDefinitionIsDocumented(t *testing.T) {
	t.Parallel()

	propertiesReadme := normalizeWhitespace(readRepositoryDocument(t, filepath.Join("properties", "github", "README.md")))
	for _, required := range []string{"canonical-conformance", "enabled", "disabled", "10-canonical-conformance.json"} {
		if !strings.Contains(propertiesReadme, required) {
			t.Fatalf("properties README does not document the capability token %q", required)
		}
	}

	convention := normalizeWhitespace(readRepositoryDocument(t, filepath.Join("docs", "conventions", "hosting-platforms", "github", "rulesets", "canonical-conformance-capability.md")))
	for _, required := range []string{"10-canonical-conformance.json", "default_value: enabled", "do_not_enforce_on_create", "emitter-consistency", "verifier-owning canonical home", "phantom-context"} {
		if !strings.Contains(convention, required) {
			t.Fatalf("canonical-conformance capability convention does not document the token %q", required)
		}
	}
}

func parsePropertyDefinition(t *testing.T, fileName string) propertyDefinition {
	t.Helper()

	contents := readRepositoryDocument(t, filepath.Join("properties", "github", fileName))
	var definition propertyDefinition
	if err := json.Unmarshal([]byte(contents), &definition); err != nil {
		t.Fatal(err)
	}
	return definition
}
