package cliparam

import (
	"testing"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/workflow"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

// This file is the property-based contract between the canonical register and
// the domain validation: every documented value must be accepted, and every
// undocumented value must be rejected with the correct error code. A drift
// between a projection and its source fails here. Canonical convention:
// docs/conventions/cli/testing-and-verification.md.

func TestClosedEnumValuesAreAcceptedByTheDomain(t *testing.T) {
	t.Parallel()

	t.Run("commit family", func(t *testing.T) {
		t.Parallel()
		for _, value := range CommitType().Values {
			if _, err := commitmsg.ParseType(value); err != nil {
				t.Fatalf("documented commit family %q rejected: %v", value, err)
			}
		}
		_, err := commitmsg.ParseType("feature")
		assertProblemCode(t, err, problem.CodeCommitTypeInvalid)
	})

	t.Run("branch family", func(t *testing.T) {
		t.Parallel()
		for _, value := range BranchFamily().Values {
			if _, err := branch.ParseFamily(value); err != nil {
				t.Fatalf("documented branch family %q rejected: %v", value, err)
			}
		}
		_, err := branch.ParseFamily("feat")
		assertProblemCode(t, err, problem.CodeBranchFamilyInvalid)
	})

	t.Run("directly creatable branch family subset", func(t *testing.T) {
		t.Parallel()
		catalog := branchapp.ListFamilies()
		creatable := map[string]bool{}
		for _, info := range catalog {
			creatable[info.Family.String()] = info.DirectlyCreatable
		}
		for _, value := range DirectlyCreatableBranchFamily().Values {
			if !creatable[value] {
				t.Fatalf("documented creatable family %q is not directly creatable in the catalog", value)
			}
		}
		for _, info := range catalog {
			if !info.DirectlyCreatable {
				for _, value := range DirectlyCreatableBranchFamily().Values {
					if value == info.Family.String() {
						t.Fatalf("workflow-bound family %q appears in the directly creatable subset", value)
					}
				}
			}
		}

		id, err := ticket.ParseID("ABC-123")
		if err != nil {
			t.Fatal(err)
		}
		slug, err := branch.ParseSlug("add-export-button")
		if err != nil {
			t.Fatal(err)
		}
		_, err = branch.NewTicketBranch(branch.FamilyMain, id, slug)
		assertProblemCode(t, err, problem.CodeBranchFamilyInvalid)
	})

	t.Run("release stabilization kind", func(t *testing.T) {
		t.Parallel()
		for _, value := range ReleaseStabilizationKind().Values {
			if _, err := workflow.ParseReleaseStabilizationKind(value); err != nil {
				t.Fatalf("documented stabilization kind %q rejected: %v", value, err)
			}
		}
		_, err := workflow.ParseReleaseStabilizationKind("feature")
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})
}

// TestDocumentedRulesAcceptAndReject pins at least one pass and one fail case
// per documented rule of the free-constrained and shaped domains, including
// the canonical failure scenarios from the discoverable-closed analysis.
func TestDocumentedRulesAcceptAndReject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		validate  func() error
		wantCode  problem.Code
		wantError bool
	}{
		{name: "key accepts the documented example", validate: func() error { _, err := ticket.ParseKey("ABC"); return err }},
		{name: "key rejects lowercase", validate: func() error { _, err := ticket.ParseKey("abc"); return err }, wantError: true, wantCode: problem.CodeTicketKeyInvalid},
		{name: "key rejects hyphens", validate: func() error { _, err := ticket.ParseKey("ABC-DEF"); return err }, wantError: true, wantCode: problem.CodeTicketKeyInvalid},
		{name: "number accepts the documented example", validate: func() error { _, err := ticket.ParseNumber("123"); return err }},
		{name: "number rejects leading zero", validate: func() error { _, err := ticket.ParseNumber("007"); return err }, wantError: true, wantCode: problem.CodeTicketNumberInvalid},
		{name: "ticket id accepts the documented form", validate: func() error { _, err := ticket.ParseID("ABC-123"); return err }},
		{name: "ticket id rejects missing separator", validate: func() error { _, err := ticket.ParseID("ABC"); return err }, wantError: true, wantCode: problem.CodeTicketIDInvalid},
		{name: "ticket id rejects double separator", validate: func() error { _, err := ticket.ParseID("ABC-123-4"); return err }, wantError: true, wantCode: problem.CodeTicketIDInvalid},
		{name: "slug accepts the documented example", validate: func() error { _, err := branch.ParseSlug("add-export-button"); return err }},
		{name: "slug rejects mixed case and repeated separators", validate: func() error { _, err := branch.ParseSlug("Add_Export--Button"); return err }, wantError: true, wantCode: problem.CodeBranchSlugInvalid},
		{name: "subject accepts the documented example", validate: func() error {
			id, _ := ticket.ParseID("ABC-123")
			_, err := commitmsg.NewHeader(commitmsg.TypeFeat, id, "add export button", false)
			return err
		}},
		{name: "subject rejects the assembly envelope", validate: func() error {
			id, _ := ticket.ParseID("ABC-123")
			_, err := commitmsg.NewHeader(commitmsg.TypeFeat, id, "feat(RG-26): add x", false)
			return err
		}, wantError: true, wantCode: problem.CodeCommitDescriptionInvalid},
		{name: "release version accepts the documented example", validate: func() error { _, err := branch.ParseSemanticVersion("2.8.0-rc.1"); return err }},
		{name: "release version rejects a leading v", validate: func() error { _, err := branch.ParseSemanticVersion("v2.8.0"); return err }, wantError: true, wantCode: problem.CodeBranchNameInvalid},
		{name: "release version rejects a missing patch component", validate: func() error { _, err := branch.ParseSemanticVersion("2.8"); return err }, wantError: true, wantCode: problem.CodeBranchNameInvalid},
		{name: "support version accepts the documented example", validate: func() error { _, err := branch.ParseSupportVersion("2.7"); return err }},
		{name: "support version rejects leading zeroes", validate: func() error { _, err := branch.ParseSupportVersion("02.7"); return err }, wantError: true, wantCode: problem.CodeBranchNameInvalid},
		{name: "release line accepts the documented form", validate: func() error { _, err := branch.ParseName("release/2.8.0"); return err }},
		{name: "release line rejects a missing patch component", validate: func() error { _, err := branch.ParseName("release/2.8"); return err }, wantError: true, wantCode: problem.CodeBranchNameInvalid},
		{name: "commit sha accepts the documented example", validate: func() error { return workflow.ValidateCommitID("9fceb02") }},
		{name: "commit sha rejects a branch name", validate: func() error { return workflow.ValidateCommitID("main") }, wantError: true, wantCode: problem.CodeInvalidInput},
		{name: "commit sha rejects a six character form", validate: func() error { return workflow.ValidateCommitID("abc123") }, wantError: true, wantCode: problem.CodeInvalidInput},
		{name: "remote accepts the documented example", validate: func() error {
			develop, _ := branch.ParseName("develop")
			_, err := branch.NewTargetBase("origin", develop)
			return err
		}},
		{name: "remote rejects spaces", validate: func() error {
			develop, _ := branch.ParseName("develop")
			_, err := branch.NewTargetBase("my remote", develop)
			return err
		}, wantError: true, wantCode: problem.CodeBranchBaseInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.validate()
			if !test.wantError {
				if err != nil {
					t.Fatalf("documented value rejected: %v", err)
				}
				return
			}
			assertProblemCode(t, err, test.wantCode)
		})
	}
}

func assertProblemCode(t *testing.T, err error, code problem.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected problem %q, got nil", code)
	}
	typed, ok := problem.As(err)
	if !ok {
		t.Fatalf("error %T does not carry a problem: %v", err, err)
	}
	if typed.Code != code {
		t.Fatalf("problem code = %q, want %q (error: %v)", typed.Code, code, err)
	}
}
