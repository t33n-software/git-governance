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

	convention := normalizeWhitespace(readRepositoryDocument(t, filepath.Join("docs", "conventions", "hosting-plattform", "github", "custom-properties", "README.md")))
	for _, required := range []string{"Positive-List", "org_actors", "pending"} {
		if !strings.Contains(convention, required) {
			t.Fatalf("custom-properties convention does not document the token %q", required)
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
