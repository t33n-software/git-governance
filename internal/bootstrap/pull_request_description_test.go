package bootstrap

import (
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// pullRequestDescriptionFiveSections is the canonical five-section
// description body shared by the pull-request publication tests: the
// description-mandate validation binds this exact structure.
var pullRequestDescriptionFiveSections = "## Summary\n\n" +
	"Add the export capability to the report view.\n\n" +
	"## Scope and Non-Goals\n\n" +
	"In scope: the export action. Out of scope: streaming exports.\n\n" +
	"## Commit Series\n\n" +
	"- feat(ABC-123): add export button\n" +
	"- test(ABC-123): cover the export action\n\n" +
	"## Risk and Rollback\n\n" +
	"Low risk; rollback reverts the series.\n\n" +
	"## Verification and Review Focus\n\n" +
	"Unit and integration tests cover the action."

func TestValidatePullRequestBodyAcceptsTheCanonicalStructure(t *testing.T) {
	t.Parallel()

	if err := validatePullRequestBody(true, pullRequestDescriptionFiveSections); err != nil {
		t.Fatalf("validatePullRequestBody rejected the canonical five-section description: %v", err)
	}
	if err := validatePullRequestBody(false, pullRequestDescriptionFiveSections); err != nil {
		t.Fatalf("validatePullRequestBody blocked an intent-only plan: %v", err)
	}
	if err := validatePullRequestBody(true, ""); err == nil {
		t.Fatal("validatePullRequestBody accepted an empty description")
	}
	if err := validatePullRequestBody(true, "   "); err == nil {
		t.Fatal("validatePullRequestBody accepted a whitespace description")
	}
}

func TestValidatePullRequestBodyRejectsMissingOrDisorderedSections(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		body string
	}{
		{
			name: "single paragraph without headings",
			body: "Summary: Add the export button.",
		},
		{
			name: "missing Scope and Non-Goals",
			body: "## Summary\n\nAdd the export capability.\n\n" +
				"## Commit Series\n\n- feat(ABC-123): add export button\n\n" +
				"## Risk and Rollback\n\nLow risk.\n\n" +
				"## Verification and Review Focus\n\nUnit tests.",
		},
		{
			name: "sections out of order",
			body: "## Scope and Non-Goals\n\nIn scope: the export action.\n\n" +
				"## Summary\n\nAdd the export capability.\n\n" +
				"## Commit Series\n\n- feat(ABC-123): add export button\n\n" +
				"## Risk and Rollback\n\nLow risk.\n\n" +
				"## Verification and Review Focus\n\nUnit tests.",
		},
		{
			name: "empty section content",
			body: "## Summary\n\nAdd the export capability.\n\n" +
				"## Scope and Non-Goals\n\nIn scope: the export action.\n\n" +
				"## Commit Series\n\n\n" +
				"## Risk and Rollback\n\nLow risk.\n\n" +
				"## Verification and Review Focus\n\nUnit tests.",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := validatePullRequestBody(true, testCase.body)
			assertPullRequestDescriptionProblem(t, err)
		})
	}
}

func TestValidatePullRequestBodyStructureErrorCarriesTheFoundSections(t *testing.T) {
	t.Parallel()

	body := "## Summary\n\nAdd the export capability.\n\n" +
		"## Risk and Rollback\n\nLow risk.\n\n" +
		"## Verification and Review Focus\n\nUnit tests."
	err := validatePullRequestBody(true, body)
	assertPullRequestDescriptionProblem(t, err)
	typed, ok := problem.As(err)
	if !ok {
		t.Fatalf("error %T does not carry a problem: %v", err, err)
	}
	if !strings.Contains(typed.Actual, "Summary") || !strings.Contains(typed.Actual, "Risk and Rollback") {
		t.Fatalf("Actual = %q, want the found section names", typed.Actual)
	}
	if !strings.Contains(typed.Rule, "Scope and Non-Goals") {
		t.Fatalf("Rule = %q, want the missing canonical section", typed.Rule)
	}
	if !strings.Contains(typed.Expected, "Commit Series") {
		t.Fatalf("Expected = %q, want the canonical order list", typed.Expected)
	}
}

func TestValidatePullRequestBodyToleratesAdditionalHeadings(t *testing.T) {
	t.Parallel()

	body := "## Summary\n\nAdd the export capability.\n\n" +
		"## Notes\n\nFree-form context heading.\n\n" +
		"## Scope and Non-Goals\n\nIn scope: the export action.\n\n" +
		"## Commit Series\n\n- feat(ABC-123): add export button\n\n" +
		"## Risk and Rollback\n\nLow risk.\n\n" +
		"## Verification and Review Focus\n\nUnit tests."
	if err := validatePullRequestBody(true, body); err != nil {
		t.Fatalf("validatePullRequestBody rejected an additional non-canonical heading: %v", err)
	}
}

func assertPullRequestDescriptionProblem(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a pull-request description problem, got nil")
	}
	typed, ok := problem.As(err)
	if !ok {
		t.Fatalf("error %T does not carry a problem: %v", err, err)
	}
	if typed.Code != problem.CodePullRequestDescriptionInvalid {
		t.Fatalf("problem code = %q, want %q", typed.Code, problem.CodePullRequestDescriptionInvalid)
	}
	if typed.Field != "pull request description" {
		t.Fatalf("problem field = %q, want %q", typed.Field, "pull request description")
	}
}
