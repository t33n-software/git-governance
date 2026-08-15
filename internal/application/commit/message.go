package commitapp

import (
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

// Family describes one supported Conventional Commit family for consumers that
// need a stable selection surface without duplicating the commit grammar.
type Family struct {
	Type        commitmsg.Type
	Label       string
	Description string
}

// Families returns every supported commit family in the commit domain's
// canonical order.
func Families() []Family {
	types := commitmsg.Types()
	families := make([]Family, len(types))
	for index, kind := range types {
		families[index] = Family{
			Type:        kind,
			Label:       familyLabel(kind),
			Description: familyDescription(kind),
		}
	}
	return families
}

// DefaultFamily proposes the usual commit family for a branch family. The
// caller may select any supported commit family because branch and commit
// taxonomies intentionally remain separate.
func DefaultFamily(family branch.Family) commitmsg.Type {
	switch family {
	case branch.FamilyFeature:
		return commitmsg.TypeFeat
	case branch.FamilyFix, branch.FamilyHotfix:
		return commitmsg.TypeFix
	case branch.FamilyDocs:
		return commitmsg.TypeDocs
	case branch.FamilyRefactor:
		return commitmsg.TypeRefactor
	case branch.FamilyChore:
		return commitmsg.TypeChore
	case branch.FamilyTest:
		return commitmsg.TypeTest
	case branch.FamilyPerf:
		return commitmsg.TypePerf
	default:
		return commitmsg.TypeChore
	}
}

// Draft contains the structured components used to create a complete
// Conventional Commit message.
type Draft struct {
	Family   commitmsg.Type
	Ticket   ticket.ID
	Subject  string
	Breaking bool
	Body     string
	Footers  []commitmsg.Footer
}

// Compose creates a validated complete commit message from structured input.
func Compose(draft Draft) (commitmsg.Message, error) {
	header, err := commitmsg.NewHeader(draft.Family, draft.Ticket, draft.Subject, draft.Breaking)
	if err != nil {
		return commitmsg.Message{}, err
	}
	return commitmsg.NewMessage(header, draft.Body, draft.Footers)
}

// ValidateMessageForBranch ensures that a ticket-scoped branch and commit use
// the same ticket identity.
func ValidateMessageForBranch(name branch.BranchName, message commitmsg.Message) error {
	if message.Header().Type() == "" {
		return problem.New(problem.Details{
			Code:        problem.CodeCommitHeaderInvalid,
			Category:    problem.CategoryGovernance,
			Field:       "commit",
			Expected:    "a validated ticket-scoped Conventional Commit message",
			Rule:        "commit creation requires a complete valid message",
			Example:     "feat(ABC-123): add export button",
			Remediation: "provide a supported family and a valid description",
		})
	}
	branchTicket, ticketScoped := name.Ticket()
	if !ticketScoped {
		return nil
	}
	if message.Header().Ticket().String() == branchTicket.String() {
		return nil
	}
	return problem.New(problem.Details{
		Code:        problem.CodeCommitTicketMismatch,
		Category:    problem.CategoryGovernance,
		Field:       "ticket",
		Actual:      message.Header().Ticket().String(),
		Expected:    branchTicket.String(),
		Rule:        "ticket-scoped branch commits use the branch ticket",
		Example:     message.Header().Type().String() + "(" + branchTicket.String() + "): " + message.Header().Subject(),
		Remediation: "use the ticket from the current branch or switch to the matching branch",
	})
}

func familyLabel(kind commitmsg.Type) string {
	switch kind {
	case commitmsg.TypeBuild:
		return "Build"
	case commitmsg.TypeChore:
		return "Chore"
	case commitmsg.TypeCI:
		return "CI"
	case commitmsg.TypeDocs:
		return "Docs"
	case commitmsg.TypeFeat:
		return "Feature"
	case commitmsg.TypeFix:
		return "Fix"
	case commitmsg.TypePerf:
		return "Performance"
	case commitmsg.TypeRefactor:
		return "Refactor"
	case commitmsg.TypeRevert:
		return "Revert"
	case commitmsg.TypeStyle:
		return "Style"
	case commitmsg.TypeTest:
		return "Test"
	default:
		return kind.String()
	}
}

func familyDescription(kind commitmsg.Type) string {
	switch kind {
	case commitmsg.TypeBuild:
		return "Build system or dependency change."
	case commitmsg.TypeCI:
		return "Continuous-integration configuration."
	case commitmsg.TypeDocs:
		return "Documentation-only change."
	case commitmsg.TypeFeat:
		return "New product functionality."
	case commitmsg.TypeFix:
		return "A defect correction."
	case commitmsg.TypePerf:
		return "Measured performance improvement."
	case commitmsg.TypeRefactor:
		return "Internal restructuring without a feature or fix."
	case commitmsg.TypeRevert:
		return "A deliberate revert with a commit reference."
	case commitmsg.TypeStyle:
		return "Formatting with no semantic effect."
	case commitmsg.TypeTest:
		return "Test work."
	default:
		return "Maintenance or tooling work."
	}
}
