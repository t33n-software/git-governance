package cliparam

import (
	"strings"
	"testing"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	commitapp "github.com/t33n-software/git-governance/internal/application/commit"
	"github.com/t33n-software/git-governance/internal/application/workflow"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/releaserequest"
)

func TestCommitTypeDerivesFromTheDomainRegistry(t *testing.T) {
	t.Parallel()

	types := commitmsg.Types()
	want := make([]string, 0, len(types))
	for _, kind := range types {
		want = append(want, kind.String())
	}

	domain := CommitType()
	assertStringsEqual(t, "CommitType().Values", domain.Values, want)
	if domain.Class != ClassClosedEnum {
		t.Fatalf("CommitType().Class = %q, want %q", domain.Class, ClassClosedEnum)
	}
	if got, wantList := domain.ValueList(), "build, chore, ci, docs, feat, fix, perf, refactor, revert, style, or test"; got != wantList {
		t.Fatalf("CommitType().ValueList() = %q, want %q", got, wantList)
	}
}

func TestBranchFamilyDerivesFromTheDomainRegistry(t *testing.T) {
	t.Parallel()

	families := branch.Families()
	want := make([]string, 0, len(families))
	for _, family := range families {
		want = append(want, family.String())
	}

	domain := BranchFamily()
	assertStringsEqual(t, "BranchFamily().Values", domain.Values, want)
	if domain.Class != ClassClosedEnum {
		t.Fatalf("BranchFamily().Class = %q, want %q", domain.Class, ClassClosedEnum)
	}
}

func TestDirectlyCreatableBranchFamilyMatchesTheDeclaredFilter(t *testing.T) {
	t.Parallel()

	var wantCreatable []string
	var wantBound []string
	for _, info := range branchapp.ListFamilies() {
		if info.DirectlyCreatable {
			wantCreatable = append(wantCreatable, info.Family.String())
		} else {
			wantBound = append(wantBound, info.Family.String())
		}
	}

	domain := DirectlyCreatableBranchFamily()
	assertStringsEqual(t, "DirectlyCreatableBranchFamily().Values", domain.Values, wantCreatable)
	wantRule := "not directly creatable: " + strings.Join(wantBound, ", ") +
		" (shared lines and special-flow families are created by their governed workflows)"
	if domain.Rule != wantRule {
		t.Fatalf("DirectlyCreatableBranchFamily().Rule = %q, want %q", domain.Rule, wantRule)
	}
}

func TestSyncStrategyDerivesFromTheApplicationContract(t *testing.T) {
	t.Parallel()

	want := []string{
		string(branchapp.SyncCheck),
		string(branchapp.SyncAuto),
		string(branchapp.SyncRebase),
		string(branchapp.SyncMerge),
	}
	assertStringsEqual(t, "SyncStrategy().Values", SyncStrategy().Values, want)
}

func TestReleaseRequestKindDerivesFromTheDomain(t *testing.T) {
	t.Parallel()

	want := []string{
		string(releaserequest.OperationRelease),
		string(releaserequest.OperationSupport),
	}
	assertStringsEqual(t, "ReleaseRequestKind().Values", ReleaseRequestKind().Values, want)
}

func TestReleaseStabilizationKindDerivesFromTheWorkflowContract(t *testing.T) {
	t.Parallel()

	want := []string{
		string(workflow.ReleaseStabilizationBlocker),
		string(workflow.ReleaseStabilizationDocs),
		string(workflow.ReleaseStabilizationPrep),
	}
	assertStringsEqual(t, "ReleaseStabilizationKind().Values", ReleaseStabilizationKind().Values, want)
}

func TestCommitTypeMetadataRendersTheCanonicalSurface(t *testing.T) {
	t.Parallel()

	want := commitapp.Families()
	got := CommitTypeMetadata()
	if len(got) != len(want) {
		t.Fatalf("CommitTypeMetadata() returned %d entries, want %d", len(got), len(want))
	}
	for index, family := range want {
		if got[index] != family {
			t.Fatalf("CommitTypeMetadata()[%d] = %+v, want %+v", index, got[index], family)
		}
	}
}

func TestBranchFamilyMetadataRendersTheCanonicalCatalog(t *testing.T) {
	t.Parallel()

	want := branchapp.ListFamilies()
	got := BranchFamilyMetadata()
	if len(got) != len(want) {
		t.Fatalf("BranchFamilyMetadata() returned %d entries, want %d", len(got), len(want))
	}
	for index, info := range want {
		if got[index] != info {
			t.Fatalf("BranchFamilyMetadata()[%d] = %+v, want %+v", index, got[index], info)
		}
	}
}

func TestGlobalEnumDomainsPinTheCliOwnedValueSets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		domain Domain
		want   []string
	}{
		{name: "color", domain: ColorMode(), want: []string{"auto", "always", "never"}},
		{name: "interactive", domain: InteractiveMode(), want: []string{"auto", "always", "never"}},
		{name: "output", domain: OutputMode(), want: []string{"human", "json"}},
		{name: "pull request provider", domain: PullRequestProvider(), want: []string{"none", "github"}},
		{name: "completion shell", domain: CompletionShell(), want: []string{"bash", "zsh", "fish", "powershell"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if test.domain.Class != ClassClosedEnum {
				t.Fatalf("Class = %q, want %q", test.domain.Class, ClassClosedEnum)
			}
			assertStringsEqual(t, test.name+" values", test.domain.Values, test.want)
		})
	}
}

func TestInteractiveAndOutputCarryTheCombinationRule(t *testing.T) {
	t.Parallel()

	if rule := InteractiveMode().Rule; !strings.Contains(rule, "--output json") {
		t.Fatalf("InteractiveMode().Rule = %q, want the --output json combination rule", rule)
	}
	if rule := OutputMode().Rule; !strings.Contains(rule, "--interactive always") {
		t.Fatalf("OutputMode().Rule = %q, want the --interactive always combination rule", rule)
	}
}

func TestAllEnumeratesEveryCanonicalDescriptor(t *testing.T) {
	t.Parallel()

	wantConcepts := []string{
		"commit family",
		"branch family",
		"branch family",
		"sync strategy",
		"protected line operation",
		"stabilization kind",
		"color mode",
		"interactive mode",
		"output mode",
		"pull request provider",
		"shell",
		"release semantic version",
		"support major.minor version",
		"protected line version",
		"release line",
		"affected line",
		"target line",
		"declared target line",
		"ticket `key`",
		"ticket `number`",
		"ticket",
		"kebab-case branch `description`",
		"commit description",
		"commit body",
		"commit footer",
		"breaking change migration impact",
		"`commit`",
		"Git remote `name`",
		"durable protected-line request `ID`",
		"workflow run `ID`",
		"request-authority actor",
		"complete commit message",
		"explicit base",
		"branch",
		"repository-relative hotfix release record",
		"file containing the complete commit message",
		"explicit path to stage",
		"timeout for external Git processes",
	}

	domains := All()
	if len(domains) != len(wantConcepts) {
		t.Fatalf("All() returned %d descriptors, want %d", len(domains), len(wantConcepts))
	}
	for index, concept := range wantConcepts {
		if domains[index].Concept != concept {
			t.Fatalf("All()[%d].Concept = %q, want %q", index, domains[index].Concept, concept)
		}
	}
}

func TestEveryDescriptorSatisfiesTheClassifiedHelpDuty(t *testing.T) {
	t.Parallel()

	knownClasses := map[Class]bool{
		ClassClosedEnum:          true,
		ClassShaped:              true,
		ClassFreeConstrained:     true,
		ClassStructuralReference: true,
		ClassScalarBounded:       true,
		ClassBooleanSwitch:       true,
		ClassCompositeToken:      true,
		ClassSecretReference:     true,
	}

	for _, domain := range All() {
		if domain.Concept == "" {
			t.Fatalf("descriptor with class %q has an empty concept", domain.Class)
		}
		if !knownClasses[domain.Class] {
			t.Fatalf("descriptor %q has unclassified class %q", domain.Concept, domain.Class)
		}

		switch domain.Class {
		case ClassClosedEnum:
			if len(domain.Values) == 0 {
				t.Fatalf("closed-enum descriptor %q carries no values", domain.Concept)
			}
			if !strings.Contains(domain.HelpText(""), domain.ValueList()) {
				t.Fatalf("closed-enum descriptor %q help does not render its value list", domain.Concept)
			}
		case ClassFreeConstrained, ClassShaped, ClassCompositeToken, ClassScalarBounded:
			if !strings.Contains(domain.Rule, "rejected") {
				t.Fatalf("%s descriptor %q rule %q misses the rejection label", domain.Class, domain.Concept, domain.Rule)
			}
		case ClassStructuralReference:
			if !strings.Contains(domain.Rule, "resolved at runtime") {
				t.Fatalf("structural-reference descriptor %q rule %q misses the runtime resolution rule", domain.Concept, domain.Rule)
			}
		case ClassBooleanSwitch, ClassSecretReference:
			// No descriptor currently occupies these classes; the model keeps
			// them for completeness of the eight-class taxonomy.
		}

		if domain.Convention != "" && !strings.Contains(domain.HelpText(""), "convention-violating: "+domain.Convention) {
			t.Fatalf("descriptor %q help misses the convention-violating label", domain.Concept)
		}
	}
}

// TestDescriptorTextsAreMarkdownSafeForTheManPipeline pins the markdown-safety
// contract of the register: the man-page pipeline parses help texts as
// markdown, and an unescaped regex anchor construct (caret followed by a
// character class) is parsed as a malformed reference link that crashes the
// roff renderer. Every regex in a descriptor text must therefore be wrapped
// in backticks. This regression test fails if an unprotected anchor returns.
func TestDescriptorTextsAreMarkdownSafeForTheManPipeline(t *testing.T) {
	t.Parallel()

	for _, domain := range All() {
		texts := []string{
			domain.HelpText(""),
			domain.HelpText("for the squashed change"),
			domain.PromptText(),
		}
		for _, text := range texts {
			for index := 0; index+1 < len(text); index++ {
				if text[index] == '^' && text[index+1] == '[' && (index == 0 || text[index-1] != '`') {
					t.Fatalf("descriptor %q carries an unprotected regex anchor in %q; wrap the regex in backticks", domain.Concept, text)
				}
			}
		}
	}
}

// TestDescriptorHelpTextsKeepRegexesOutOfTheFlagPlaceholder pins the pflag
// placeholder contract: the flag framework claims the first backtick pair of
// a usage text as the flag's value placeholder, so a descriptor that quotes
// its regex must quote a natural placeholder word first; otherwise the regex
// itself would surface as the placeholder in the terminal help.
func TestDescriptorHelpTextsKeepRegexesOutOfTheFlagPlaceholder(t *testing.T) {
	t.Parallel()

	for _, domain := range All() {
		for _, text := range []string{domain.HelpText(""), domain.HelpText("for the squashed change")} {
			regexIndex := strings.Index(text, "`^")
			if regexIndex < 0 {
				continue
			}
			if strings.Count(text[:regexIndex], "`") < 2 {
				t.Fatalf("descriptor %q quotes the regex before any placeholder word: %q", domain.Concept, text)
			}
		}
	}
}

func TestDescriptorValuesAreDefensivelyIsolatedPerCall(t *testing.T) {
	t.Parallel()

	first := CommitType()
	first.Values[0] = "mutated"
	second := CommitType()
	if second.Values[0] == "mutated" {
		t.Fatalf("CommitType() returned a shared backing array: %v", second.Values)
	}
}

func assertStringsEqual(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("%s[%d] = %q, want %q (got %v, want %v)", label, index, got[index], want[index], got, want)
		}
	}
}
