package cliparam

import (
	"strings"
	"testing"
)

func TestValueListRendersCanonicalEnumeration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{name: "empty set", values: nil, want: ""},
		{name: "single value", values: []string{"feat"}, want: "feat"},
		{name: "two values", values: []string{"human", "json"}, want: "human or json"},
		{
			name:   "three values",
			values: []string{"auto", "always", "never"},
			want:   "auto, always, or never",
		},
		{
			name: "eleven values keep the canonical order with oxford or",
			values: []string{
				"build", "chore", "ci", "docs", "feat", "fix",
				"perf", "refactor", "revert", "style", "test",
			},
			want: "build, chore, ci, docs, feat, fix, perf, refactor, revert, style, or test",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			domain := Domain{Values: test.values}
			if got := domain.ValueList(); got != test.want {
				t.Fatalf("ValueList() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestHelpTextRendersClosedEnumValueList(t *testing.T) {
	t.Parallel()

	domain := Domain{
		Concept: "commit family",
		Class:   ClassClosedEnum,
		Values:  []string{"feat", "fix"},
		Example: "feat",
	}
	want := "commit family: feat or fix; example: feat"
	if got := domain.HelpText(""); got != want {
		t.Fatalf("HelpText() = %q, want %q", got, want)
	}
}

func TestHelpTextAppendsClosedEnumRuleAsSeparateSection(t *testing.T) {
	t.Parallel()

	domain := Domain{
		Concept: "interactive mode",
		Class:   ClassClosedEnum,
		Values:  []string{"auto", "always", "never"},
		Rule:    "always is rejected when --output json is selected or no terminal is available",
	}
	want := "interactive mode: auto, always, or never; " +
		"always is rejected when --output json is selected or no terminal is available"
	if got := domain.HelpText(""); got != want {
		t.Fatalf("HelpText() = %q, want %q", got, want)
	}
}

func TestHelpTextRendersRuleConventionAndExampleWithLabels(t *testing.T) {
	t.Parallel()

	domain := Domain{
		Concept:    "commit description",
		Class:      ClassFreeConstrained,
		Rule:       "1-200 runes, unpadded (rejected by validation)",
		Convention: "filler formulas without a named behavior",
		Example:    "add export button",
	}
	got := domain.HelpText("")

	for _, part := range []string{
		"commit description: 1-200 runes, unpadded (rejected by validation)",
		"convention-violating: filler formulas without a named behavior",
		"example: add export button",
	} {
		if !strings.Contains(got, part) {
			t.Fatalf("HelpText() = %q, missing %q", got, part)
		}
	}
	if strings.Index(got, "(rejected by validation)") > strings.Index(got, "convention-violating:") {
		t.Fatalf("HelpText() = %q, rejection label must precede the convention label", got)
	}
}

func TestHelpTextExtendsTheConceptWithEndpointContext(t *testing.T) {
	t.Parallel()

	domain := Domain{
		Concept: "commit family",
		Class:   ClassClosedEnum,
		Values:  []string{"feat", "fix"},
	}
	got := domain.HelpText("for the squashed change")
	want := "commit family for the squashed change: feat or fix"
	if got != want {
		t.Fatalf("HelpText(context) = %q, want %q", got, want)
	}
}

func TestHelpTextSkipsAbsentOptionalSections(t *testing.T) {
	t.Parallel()

	domain := Domain{
		Concept: "timeout for external Git processes",
		Class:   ClassScalarBounded,
		Rule:    "a positive duration (rejected otherwise)",
	}
	want := "timeout for external Git processes: a positive duration (rejected otherwise)"
	if got := domain.HelpText(""); got != want {
		t.Fatalf("HelpText() = %q, want %q", got, want)
	}
}

func TestHelpTextOnZeroDomainIsEmpty(t *testing.T) {
	t.Parallel()

	if got := (Domain{}).HelpText(""); got != "" {
		t.Fatalf("HelpText() on zero Domain = %q, want empty", got)
	}
}

func TestCompleteFiltersValuesAndPrefixesByTypedPrefix(t *testing.T) {
	t.Parallel()

	domain := Domain{
		Class:    ClassShaped,
		Values:   []string{"main"},
		Prefixes: []string{"release/", "support/"},
	}

	tests := []struct {
		name       string
		toComplete string
		want       []string
	}{
		{name: "empty prefix returns every candidate", toComplete: "", want: []string{"main", "release/", "support/"}},
		{name: "prefix filters values and prefixes", toComplete: "re", want: []string{"release/"}},
		{name: "exact value prefix", toComplete: "ma", want: []string{"main"}},
		{name: "no match yields an empty result", toComplete: "zzz", want: []string{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := domain.Complete(test.toComplete)
			if len(got) != len(test.want) {
				t.Fatalf("Complete(%q) = %v, want %v", test.toComplete, got, test.want)
			}
			for index, value := range test.want {
				if got[index] != value {
					t.Fatalf("Complete(%q)[%d] = %q, want %q", test.toComplete, index, got[index], value)
				}
			}
		})
	}
}

func TestCompleteDoesNotAliasOrMutateTheDescriptor(t *testing.T) {
	t.Parallel()

	domain := Domain{
		Class:    ClassClosedEnum,
		Values:   []string{"feat", "fix"},
		Prefixes: []string{"release/"},
	}
	first := domain.Complete("")
	if len(first) != 3 {
		t.Fatalf("Complete(\"\") returned %d candidates, want 3", len(first))
	}
	first[0] = "mutated"
	if domain.Values[0] != "feat" {
		t.Fatalf("Complete mutated the descriptor values: %v", domain.Values)
	}
	second := domain.Complete("")
	if second[0] != "feat" {
		t.Fatalf("Complete result leaked into the next call: %v", second)
	}
}
