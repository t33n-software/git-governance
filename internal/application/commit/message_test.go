package commitapp

import (
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

func TestFamiliesReflectTheCanonicalCommitTypes(t *testing.T) {
	t.Parallel()

	types := commitmsg.Types()
	families := Families()
	if len(families) != len(types) {
		t.Fatalf("Families() length = %d, want %d", len(families), len(types))
	}

	for index, kind := range types {
		family := families[index]
		if family.Type != kind || family.Label == "" || family.Description == "" {
			t.Fatalf("Families()[%d] = %#v, want populated metadata for %q", index, family, kind)
		}
	}

	if families[4].Label != "Feature" || families[4].Description != "New product functionality." {
		t.Fatalf("feature family = %#v", families[4])
	}
	if familyLabel(commitmsg.Type("unknown")) != "unknown" {
		t.Fatalf("unknown family label = %q", familyLabel(commitmsg.Type("unknown")))
	}
}

func TestDefaultFamilyMapsBranchContextsWithoutRestrictingSelection(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		family branch.Family
		want   commitmsg.Type
	}{
		{family: branch.FamilyFeature, want: commitmsg.TypeFeat},
		{family: branch.FamilyFix, want: commitmsg.TypeFix},
		{family: branch.FamilyHotfix, want: commitmsg.TypeFix},
		{family: branch.FamilyDocs, want: commitmsg.TypeDocs},
		{family: branch.FamilyRefactor, want: commitmsg.TypeRefactor},
		{family: branch.FamilyChore, want: commitmsg.TypeChore},
		{family: branch.FamilyTest, want: commitmsg.TypeTest},
		{family: branch.FamilyPerf, want: commitmsg.TypePerf},
		{family: branch.FamilyMain, want: commitmsg.TypeChore},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.family.String(), func(t *testing.T) {
			if actual := DefaultFamily(testCase.family); actual != testCase.want {
				t.Fatalf("DefaultFamily(%q) = %q, want %q", testCase.family, actual, testCase.want)
			}
		})
	}
}

func TestComposeBuildsValidatedCompleteMessages(t *testing.T) {
	t.Parallel()

	id, err := ticket.ParseID("ABC-123")
	if err != nil {
		t.Fatal(err)
	}
	footer, err := commitmsg.NewFooter("Refs", "#123")
	if err != nil {
		t.Fatal(err)
	}

	message, err := Compose(Draft{
		Family:  commitmsg.TypeFeat,
		Ticket:  id,
		Subject: "add export",
		Body:    "Exports are available to clients.",
		Footers: []commitmsg.Footer{footer},
	})
	if err != nil {
		t.Fatal(err)
	}
	if actual := message.String(); actual != "feat(ABC-123): add export\n\nExports are available to clients.\n\nRefs: #123" {
		t.Fatalf("Compose() = %q", actual)
	}

	_, err = Compose(Draft{Family: commitmsg.Type("unknown"), Ticket: id, Subject: "add export"})
	if err == nil || !strings.Contains(err.Error(), "COMMIT_TYPE_INVALID") {
		t.Fatalf("Compose(invalid family) error = %v", err)
	}
}

func TestComposeRejectsBodiesCollidingWithTheFooterGrammar(t *testing.T) {
	t.Parallel()

	id, err := ticket.ParseID("ABC-123")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Compose(Draft{
		Family:  commitmsg.TypeFeat,
		Ticket:  id,
		Subject: "add export",
		Body:    "Motivation: exports are needed.\n\nBehavioral change: clients can export.",
	})
	if err == nil || !strings.Contains(err.Error(), "COMMIT_DESCRIPTION_INVALID") {
		t.Fatalf("Compose(footer-colliding body) error = %v", err)
	}

	message, err := Compose(Draft{
		Family:  commitmsg.TypeFeat,
		Ticket:  id,
		Subject: "add export",
		Body:    "## Motivation\n\nExports are needed.\n\n## Verification\n\nCovered by tests.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message.Body(), "## Motivation") || !strings.Contains(message.Body(), "## Verification") {
		t.Fatalf("Compose() body = %q", message.Body())
	}

	_, err = Compose(Draft{
		Family:  commitmsg.TypeFeat,
		Ticket:  id,
		Subject: "add export",
		Body:    "invalid\x00body",
	})
	if err == nil || !strings.Contains(err.Error(), "COMMIT_DESCRIPTION_INVALID") {
		t.Fatalf("Compose(control-character body) error = %v", err)
	}
}

func TestValidateMessageForBranchEnforcesTicketOwnership(t *testing.T) {
	t.Parallel()

	name, err := branch.ParseName("feature/ABC-123-add-export")
	if err != nil {
		t.Fatal(err)
	}
	id, err := ticket.ParseID("ABC-123")
	if err != nil {
		t.Fatal(err)
	}
	message, err := Compose(Draft{Family: commitmsg.TypeFeat, Ticket: id, Subject: "add export"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateMessageForBranch(name, message); err != nil {
		t.Fatalf("ValidateMessageForBranch() error = %v", err)
	}

	other, err := ticket.ParseID("ABC-124")
	if err != nil {
		t.Fatal(err)
	}
	mismatched, err := Compose(Draft{Family: commitmsg.TypeFeat, Ticket: other, Subject: "add export"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateMessageForBranch(name, mismatched); err == nil || !strings.Contains(err.Error(), "COMMIT_TICKET_MISMATCH") {
		t.Fatalf("ValidateMessageForBranch(mismatch) error = %v", err)
	}

	if err := ValidateMessageForBranch(name, commitmsg.Message{}); err == nil || !strings.Contains(err.Error(), "COMMIT_HEADER_INVALID") {
		t.Fatalf("ValidateMessageForBranch(empty) error = %v", err)
	}

	if err := ValidateMessageForBranch(branch.BranchName{}, message); err != nil {
		t.Fatalf("ValidateMessageForBranch(unscoped) error = %v", err)
	}
}
