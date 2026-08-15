package branch

import (
	"regexp"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

func TestFamiliesAreCompleteAndDefensivelyCopied(t *testing.T) {
	t.Parallel()

	actual := Families()
	if len(actual) != 13 {
		t.Fatalf("Families() length = %d, want 13", len(actual))
	}
	if actual[0] != FamilyMain || actual[len(actual)-1] != FamilyScratch {
		t.Fatalf("Families() order = %v", actual)
	}

	actual[0] = "mutated"
	if Families()[0] != FamilyMain {
		t.Fatal("Families() returned mutable shared backing storage")
	}
}

func TestFamilyPolicies(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		family           Family
		shared           bool
		ticketScoped     bool
		official         bool
		privateRewrites  bool
		requiresWorkflow bool
		workflowBase     bool
	}{
		{FamilyMain, true, false, false, false, false, false},
		{FamilyDevelop, true, false, false, false, false, false},
		{FamilyRelease, true, false, false, false, true, false},
		{FamilySupport, true, false, false, false, true, false},
		{FamilyFeature, false, true, true, false, false, false},
		{FamilyFix, false, true, true, false, false, true},
		{FamilyDocs, false, true, true, false, false, true},
		{FamilyRefactor, false, true, true, false, false, false},
		{FamilyChore, false, true, true, false, false, true},
		{FamilyTest, false, true, true, false, false, false},
		{FamilyPerf, false, true, true, false, false, false},
		{FamilyHotfix, false, true, true, false, true, true},
		{FamilyScratch, false, true, false, true, false, false},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.family.String(), func(t *testing.T) {
			t.Parallel()
			if testCase.family.IsSharedLine() != testCase.shared {
				t.Fatalf("IsSharedLine() = %t", testCase.family.IsSharedLine())
			}
			if testCase.family.IsTicketScoped() != testCase.ticketScoped {
				t.Fatalf("IsTicketScoped() = %t", testCase.family.IsTicketScoped())
			}
			if testCase.family.IsOfficialWorkingBranch() != testCase.official {
				t.Fatalf("IsOfficialWorkingBranch() = %t", testCase.family.IsOfficialWorkingBranch())
			}
			if testCase.family.AllowsPrivateRewrites() != testCase.privateRewrites {
				t.Fatalf("AllowsPrivateRewrites() = %t", testCase.family.AllowsPrivateRewrites())
			}
			if testCase.family.RequiresWorkflow() != testCase.requiresWorkflow {
				t.Fatalf("RequiresWorkflow() = %t", testCase.family.RequiresWorkflow())
			}
			if testCase.family.MayUseWorkflowBase() != testCase.workflowBase {
				t.Fatalf("MayUseWorkflowBase() = %t", testCase.family.MayUseWorkflowBase())
			}
		})
	}
}

func TestParseSlug(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		raw   string
		valid bool
	}{
		{raw: "add-export-button", valid: true},
		{raw: "oauth2-token-refresh", valid: true},
		{raw: strings.Repeat("a", maxSlugLength), valid: true},
		{raw: "", valid: false},
		{raw: "Add-export", valid: false},
		{raw: "add--export", valid: false},
		{raw: "-add-export", valid: false},
		{raw: "add-export-", valid: false},
		{raw: "add/export", valid: false},
		{raw: strings.Repeat("a", maxSlugLength+1), valid: false},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.raw, func(t *testing.T) {
			t.Parallel()
			actual, err := ParseSlug(testCase.raw)
			if testCase.valid {
				if err != nil || actual.String() != testCase.raw {
					t.Fatalf("ParseSlug(%q) = (%q, %v)", testCase.raw, actual.String(), err)
				}
				return
			}
			assertProblemCode(t, err, problem.CodeBranchSlugInvalid)
		})
	}
}

func TestParseSemanticVersion(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		raw   string
		valid bool
	}{
		{raw: "0.1.0", valid: true},
		{raw: "2.8.0", valid: true},
		{raw: "2.8.0-rc.1", valid: true},
		{raw: "2.8.0-rc.1+build.7", valid: true},
		{raw: "1.0.0-0.3.7", valid: true},
		{raw: "v2.8.0", valid: false},
		{raw: "02.8.0", valid: false},
		{raw: "2.08.0", valid: false},
		{raw: "2.8", valid: false},
		{raw: "2.8.0-", valid: false},
		{raw: "2.8.0+build..1", valid: false},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.raw, func(t *testing.T) {
			t.Parallel()
			actual, err := ParseSemanticVersion(testCase.raw)
			if testCase.valid {
				if err != nil || actual.String() != testCase.raw {
					t.Fatalf("ParseSemanticVersion(%q) = (%q, %v)", testCase.raw, actual.String(), err)
				}
				return
			}
			assertProblemCode(t, err, problem.CodeBranchNameInvalid)
		})
	}
}

func TestParseSupportVersion(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		raw   string
		valid bool
	}{
		{raw: "0.9", valid: true},
		{raw: "2.7", valid: true},
		{raw: "02.7", valid: false},
		{raw: "2.07", valid: false},
		{raw: "2", valid: false},
		{raw: "2.7.1", valid: false},
	} {
		testCase := testCase
		t.Run(testCase.raw, func(t *testing.T) {
			t.Parallel()
			actual, err := ParseSupportVersion(testCase.raw)
			if testCase.valid {
				if err != nil || actual.String() != testCase.raw {
					t.Fatalf("ParseSupportVersion(%q) = (%q, %v)", testCase.raw, actual.String(), err)
				}
				return
			}
			assertProblemCode(t, err, problem.CodeBranchNameInvalid)
		})
	}
}

func TestParseNameAllFamilies(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		raw    string
		family Family
		ticket bool
	}{
		{raw: "main", family: FamilyMain},
		{raw: "develop", family: FamilyDevelop},
		{raw: "release/2.8.0-rc.1", family: FamilyRelease},
		{raw: "support/2.7", family: FamilySupport},
		{raw: "feature/ABC-123-add-export-button", family: FamilyFeature, ticket: true},
		{raw: "fix/ABC-123-handle-null", family: FamilyFix, ticket: true},
		{raw: "docs/ABC-123-update-guide", family: FamilyDocs, ticket: true},
		{raw: "refactor/ABC-123-split-service", family: FamilyRefactor, ticket: true},
		{raw: "chore/ABC-123-update-tooling", family: FamilyChore, ticket: true},
		{raw: "test/ABC-123-cover-flow", family: FamilyTest, ticket: true},
		{raw: "perf/ABC-123-reduce-latency", family: FamilyPerf, ticket: true},
		{raw: "hotfix/ABC-123-payment-timeout", family: FamilyHotfix, ticket: true},
		{raw: "scratch/ABC-123-experiment", family: FamilyScratch, ticket: true},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.raw, func(t *testing.T) {
			t.Parallel()
			actual, err := ParseName(testCase.raw)
			if err != nil {
				t.Fatalf("ParseName(%q) error = %v", testCase.raw, err)
			}
			if actual.String() != testCase.raw || actual.Family() != testCase.family {
				t.Fatalf("ParseName(%q) = (%q, %q)", testCase.raw, actual.String(), actual.Family())
			}
			_, hasTicket := actual.Ticket()
			if hasTicket != testCase.ticket {
				t.Fatalf("Ticket() presence = %t, want %t", hasTicket, testCase.ticket)
			}
		})
	}
}

func TestParseNameRejectsNonCanonicalValues(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"feat/ABC-123-add-export",
		"feature/ABC-123-add--export",
		"feature/frontend/ABC-123-add-export",
		"feature/abc-123-add-export",
		"feature/ABC-001-add-export",
		"release/v2.8.0",
		"release/2.8.0/extra",
		"support/2.7/extra",
		"main/extra",
	} {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			_, err := ParseName(raw)
			if err == nil {
				t.Fatalf("ParseName(%q) unexpectedly succeeded", raw)
			}
		})
	}
}

func TestTargetBaseAndPublicationState(t *testing.T) {
	t.Parallel()

	develop, err := ParseName("develop")
	if err != nil {
		t.Fatal(err)
	}
	base, err := NewTargetBase("origin", develop)
	if err != nil {
		t.Fatal(err)
	}
	if base.String() != "origin/develop" {
		t.Fatalf("TargetBase.String() = %q", base.String())
	}
	if !base.IsRemoteTracking() {
		t.Fatal("remote target base must report IsRemoteTracking")
	}
	local, err := NewLocalBase(mustParseBranch(t, "feature/ABC-123-add-export"))
	if err != nil {
		t.Fatal(err)
	}
	if local.String() != "feature/ABC-123-add-export" || local.IsRemoteTracking() {
		t.Fatalf("local target base = (%q, remote=%t)", local.String(), local.IsRemoteTracking())
	}
	if _, err := NewTargetBase("../origin", develop); err == nil {
		t.Fatal("NewTargetBase accepted invalid remote")
	}

	if !PublicationUnpublished.CanRewriteHistory(FamilyFeature) {
		t.Fatal("unpublished feature branch should be rewriteable")
	}
	if PublicationPublished.CanRewriteHistory(FamilyFeature) {
		t.Fatal("published feature branch must not be rewriteable")
	}
	if PublicationUnpublished.CanRewriteHistory(FamilyScratch) {
		t.Fatal("scratch branch uses its separate private-rewrite policy")
	}
	if PublicationUnknown.CanRewriteHistory(FamilyFeature) {
		t.Fatal("unknown publication state must be safe by default")
	}
}

func TestParseLocalBasePreservesBranchValidation(t *testing.T) {
	t.Parallel()

	base, err := ParseLocalBase("feature/ABC-123-add-export")
	if err != nil {
		t.Fatal(err)
	}
	if base.String() != "feature/ABC-123-add-export" || base.IsRemoteTracking() {
		t.Fatalf("ParseLocalBase() = (%q, remote=%t)", base.String(), base.IsRemoteTracking())
	}

	_, err = ParseLocalBase("not-a-branch")
	assertProblemCode(t, err, problem.CodeBranchNameInvalid)
}

func TestDefaultTargetBase(t *testing.T) {
	t.Parallel()

	base, found, err := FamilyFeature.DefaultTargetBase("origin")
	if err != nil || !found || base.String() != "origin/develop" {
		t.Fatalf("DefaultTargetBase() = (%q, %t, %v)", base.String(), found, err)
	}
	if _, found, err := FamilyHotfix.DefaultTargetBase("origin"); err != nil || found {
		t.Fatalf("hotfix DefaultTargetBase() found = %t, err = %v", found, err)
	}
}

func TestNewTicketBranch(t *testing.T) {
	t.Parallel()

	id, err := ticket.ParseID("ABC-123")
	if err != nil {
		t.Fatal(err)
	}
	slug, err := ParseSlug("add-export-button")
	if err != nil {
		t.Fatal(err)
	}
	actual, err := NewTicketBranch(FamilyFeature, id, slug)
	if err != nil {
		t.Fatal(err)
	}
	if actual.String() != "feature/ABC-123-add-export-button" {
		t.Fatalf("NewTicketBranch() = %q", actual.String())
	}
	if _, err := NewTicketBranch(FamilyRelease, id, slug); err == nil {
		t.Fatal("NewTicketBranch accepted release")
	}
}

func TestBranchValueObjectErrorAndAccessorPaths(t *testing.T) {
	t.Parallel()

	if _, err := ParseFamily("unknown"); err == nil {
		t.Fatal("ParseFamily accepted an unknown family")
	}
	if (Family("unknown")).IsKnown() {
		t.Fatal("unknown family reported known")
	}

	var zeroName BranchName
	if !zeroName.IsZero() {
		t.Fatal("zero branch name must report IsZero")
	}
	if _, ok := zeroName.Slug(); ok {
		t.Fatal("zero branch name unexpectedly has a slug")
	}
	if _, ok := zeroName.ReleaseVersion(); ok {
		t.Fatal("zero branch name unexpectedly has a release version")
	}
	if _, ok := zeroName.SupportVersion(); ok {
		t.Fatal("zero branch name unexpectedly has a support version")
	}
	if _, err := NewTicketBranch(FamilyFeature, ticket.ID{}, Slug{}); err == nil {
		t.Fatal("NewTicketBranch accepted a zero ticket and slug")
	}
	validTicket, err := ticket.ParseID("ABC-123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewTicketBranch(FamilyFeature, validTicket, Slug{}); err == nil {
		t.Fatal("NewTicketBranch accepted a zero slug")
	}
	if _, err := NewReleaseBranch(SemanticVersion{}); err == nil {
		t.Fatal("NewReleaseBranch accepted a zero version")
	}
	if _, err := NewSupportBranch(SupportVersion{}); err == nil {
		t.Fatal("NewSupportBranch accepted a zero version")
	}
	if _, err := NewTargetBase("origin", BranchName{}); err == nil {
		t.Fatal("NewTargetBase accepted a zero branch")
	}
	if _, err := NewLocalBase(BranchName{}); err == nil {
		t.Fatal("NewLocalBase accepted a zero branch")
	}

	name := mustParseBranch(t, "feature/ABC-123-add-export")
	base, err := NewTargetBase("origin", name)
	if err != nil {
		t.Fatal(err)
	}
	if base.Remote() != "origin" || base.Branch().String() != name.String() {
		t.Fatalf("base accessors = (%q, %q)", base.Remote(), base.Branch())
	}
	if (TargetBase{}).String() != "" {
		t.Fatal("zero target base must render as empty")
	}
	for _, raw := range []string{"release/", "support/", "feature/ABC-123-", "feature/ABC-123-add-export/extra"} {
		if _, err := ParseName(raw); err == nil {
			t.Fatalf("ParseName(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestParseNameDefensiveComponentFailures(t *testing.T) {
	original := ticketBranchPattern
	t.Cleanup(func() {
		ticketBranchPattern = original
	})

	testCases := []struct {
		name    string
		pattern string
		raw     string
		code    problem.Code
	}{
		{
			name:    "unknown family",
			pattern: `^(unknown)/(ABC-123)-(valid)$`,
			raw:     "unknown/ABC-123-valid",
			code:    problem.CodeBranchFamilyInvalid,
		},
		{
			name:    "invalid ticket",
			pattern: `^(feature)/(abc-123)-(valid)$`,
			raw:     "feature/abc-123-valid",
			code:    problem.CodeTicketKeyInvalid,
		},
		{
			name:    "invalid slug",
			pattern: `^(feature)/(ABC-123)-(bad--slug)$`,
			raw:     "feature/ABC-123-bad--slug",
			code:    problem.CodeBranchSlugInvalid,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ticketBranchPattern = regexp.MustCompile(testCase.pattern)
			_, err := ParseName(testCase.raw)
			assertProblemCode(t, err, testCase.code)
		})
	}
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

func mustParseBranch(t *testing.T, raw string) BranchName {
	t.Helper()
	actual, err := ParseName(raw)
	if err != nil {
		t.Fatal(err)
	}
	return actual
}

func FuzzParseBranchValues(f *testing.F) {
	for _, seed := range []string{
		"feature/ABC-123-add-export",
		"release/2.8.0-rc.1",
		"support/2.7",
		"scratch/ABC-123-experiment",
		"feat/ABC-123-invalid",
		"",
		"\x00",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if name, err := ParseName(raw); err == nil {
			roundTrip, err := ParseName(name.String())
			if err != nil || roundTrip.String() != name.String() {
				t.Fatalf("branch round-trip for %q = (%q, %v)", raw, roundTrip.String(), err)
			}
		}
		if version, err := ParseSemanticVersion(raw); err == nil && version.String() != raw {
			t.Fatalf("semantic version changed %q to %q", raw, version.String())
		}
		if slug, err := ParseSlug(raw); err == nil && slug.String() != raw {
			t.Fatalf("slug changed %q to %q", raw, slug.String())
		}
	})
}
