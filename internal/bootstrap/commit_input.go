package bootstrap

import (
	"context"
	"strings"

	"github.com/t33n-software/git-governance/internal/application/cliparam"
	commitapp "github.com/t33n-software/git-governance/internal/application/commit"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

// commitMessageInput is the shared delivery-level input model for every
// command that creates a governed commit. Creation speaks structured values
// only; a raw complete message exists exclusively at the verification
// boundary (commit validate and the commit-msg hook), never as a creation
// input. Canonical convention:
// docs/conventions/commits/family-selection.md.
type commitMessageInput struct {
	Branch               branch.BranchName
	Repository           port.RepositoryIdentity
	Family               string
	Description          string
	Body                 string
	Breaking             bool
	BreakingDescription  string
	FooterSpecifications []string
	DefaultFamily        commitmsg.Type
	RequireBody          bool
	DescriptionLabel     string
	Operation            string
	Validate             func(commitmsg.Message) error
}

func (application *application) resolveCommitMessage(
	ctx context.Context,
	input commitMessageInput,
) (commitmsg.Message, error) {
	ticketID, ticketScoped := input.Branch.Ticket()
	if !ticketScoped {
		return commitmsg.Message{}, missingCommitContext(input.Branch)
	}

	family, err := application.resolveCommitFamily(ctx, input, ticketID)
	if err != nil {
		return commitmsg.Message{}, err
	}
	description, err := application.resolveCommitDescription(ctx, input, family, ticketID)
	if err != nil {
		return commitmsg.Message{}, err
	}
	footers, err := parseFooterSpecs(input.FooterSpecifications)
	if err != nil {
		return commitmsg.Message{}, err
	}
	if input.Breaking {
		breakingDescription := input.BreakingDescription
		if breakingDescription == "" {
			breakingDescription, err = application.requireInput(
				ctx,
				"",
				"Breaking change impact",
				cliparam.BreakingDescription().PromptText(),
				func(value string) error {
					_, validationErr := commitmsg.NewFooter("BREAKING CHANGE", value)
					return validationErr
				},
			)
			if err != nil {
				return commitmsg.Message{}, err
			}
		}
		breakingFooter, err := commitmsg.NewFooter("BREAKING CHANGE", breakingDescription)
		if err != nil {
			return commitmsg.Message{}, err
		}
		footers = append(footers, breakingFooter)
	}
	body, err := application.resolveCommitBody(ctx, input)
	if err != nil {
		return commitmsg.Message{}, err
	}
	message, err := commitapp.Compose(commitapp.Draft{
		Family:   family,
		Ticket:   ticketID,
		Subject:  description,
		Breaking: input.Breaking,
		Body:     body,
		Footers:  footers,
	})
	if err != nil {
		return commitmsg.Message{}, err
	}
	return validateResolvedCommitMessage(input, message)
}

// resolveCommitFamily binds the commit family as an explicit author decision.
// Non-interactive invocation without --type fails closed: the family steers
// body duty, release semantics, and history readability, so it is never
// silently derived from the branch family. The branch-family default remains
// as the preselected proposal in the interactive select, which the author
// confirms explicitly. Canonical convention:
// docs/conventions/commits/family-selection.md. The interactive select renders
// the same value domain as the static help surfaces:
// docs/conventions/cli/interaction-model.md.
func (application *application) resolveCommitFamily(
	ctx context.Context,
	input commitMessageInput,
	ticketID ticket.ID,
) (commitmsg.Type, error) {
	if input.Family != "" {
		return commitmsg.ParseType(input.Family)
	}
	if !application.promptAvailable() {
		return "", missingInput("commit family")
	}

	families := commitapp.Families()
	options := make([]port.SelectOption, 0, len(families))
	for _, family := range families {
		options = append(options, port.SelectOption{
			Value:       family.Type.String(),
			Label:       family.Label,
			Description: family.Description,
		})
	}
	value, err := application.prompt().Select(ctx, port.SelectRequest{
		Label:       "Commit family",
		Description: commitContextDescription(input.Operation, input.Branch, ticketID.String(), ""),
		Options:     options,
		Default:     defaultCommitFamily(input).String(),
	})
	if err != nil {
		return "", err
	}
	return commitmsg.ParseType(value)
}

func (application *application) resolveCommitDescription(
	ctx context.Context,
	input commitMessageInput,
	family commitmsg.Type,
	ticketID ticket.ID,
) (string, error) {
	label := input.DescriptionLabel
	if label == "" {
		label = "Commit description"
	}
	return resolveValidatedInput(
		application,
		ctx,
		input.Description,
		label,
		commitContextDescription(input.Operation, input.Branch, ticketID.String(), family.String())+
			"\nEnter only the description after ': ' in "+family.String()+"("+ticketID.String()+"): <description>. "+
			cliparam.CommitSubject().PromptText(),
		func(value string) (string, error) {
			_, err := commitmsg.NewHeader(family, ticketID, value, input.Breaking)
			return value, err
		},
	)
}

// resolveCommitBody binds the commit body. The body is the default, not an
// option: wherever the body-duty matrix marks the context as always mandatory
// and that context is machine-known — a breaking change, the scratch squash
// transfer, the hotfix lane, or a release-line stabilization branch — a
// missing body is prompted for interactively or fails closed. All other units
// keep the matrix as a content contract. Canonical convention:
// docs/conventions/commits/family-selection.md.
func (application *application) resolveCommitBody(
	ctx context.Context,
	input commitMessageInput,
) (string, error) {
	rule, required, err := application.bodyDutyRule(ctx, input)
	if err != nil {
		return "", err
	}
	if !required || input.Body != "" {
		return input.Body, nil
	}
	if !application.promptAvailable() {
		return "", bodyRequired(input, rule)
	}
	return application.requireInput(
		ctx,
		"",
		"Commit body",
		rule+" Provide the canonical body layout (Motivation, Behavioral Change, Contracts and Invariants, Verification, Risks and Follow-ups — only the applicable categories, in this order).",
		func(value string) error {
			if value == "" || strings.TrimSpace(value) != value {
				return bodyRequired(input, rule)
			}
			return nil
		},
	)
}

// bodyDutyRule reports whether the body-duty matrix marks the current context
// as always mandatory using only machine-known signals: the breaking marker,
// the scratch squash transfer operation, the hotfix lane, and the recorded
// workflow base of the branch for release-line stabilization.
func (application *application) bodyDutyRule(
	ctx context.Context,
	input commitMessageInput,
) (string, bool, error) {
	if input.Breaking {
		return "a breaking change carries the migration impact in the body.", true, nil
	}
	if input.RequireBody {
		return "the scratch squash transfer documents the discarded experiment paths in the body.", true, nil
	}
	if input.Branch.Family() == branch.FamilyHotfix {
		return "the hotfix lane carries incident context, root cause, and risk in the body.", true, nil
	}
	storedBase, found, err := application.services().git.WorkflowBase(ctx, input.Repository, input.Branch)
	if err != nil {
		return "", false, err
	}
	if found && storedBase.Branch().Family() == branch.FamilyRelease {
		return "release-line stabilization carries the frozen-line burden of proof in the body.", true, nil
	}
	return "", false, nil
}

func bodyRequired(input commitMessageInput, rule string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeCommitBodyRequired,
		Category:    problem.CategoryGovernance,
		Field:       "commit body",
		Expected:    "a body for " + input.Operation,
		Rule:        rule,
		Example:     "## Motivation\n\nWhy the unit changes.",
		Remediation: "supply the body via the body input of this command",
	})
}

func validateResolvedCommitMessage(input commitMessageInput, message commitmsg.Message) (commitmsg.Message, error) {
	if input.Validate != nil {
		if err := input.Validate(message); err != nil {
			return commitmsg.Message{}, err
		}
	}
	if err := commitapp.ValidateMessageForBranch(input.Branch, message); err != nil {
		return commitmsg.Message{}, err
	}
	return message, nil
}

func defaultCommitFamily(input commitMessageInput) commitmsg.Type {
	if input.DefaultFamily.IsKnown() {
		return input.DefaultFamily
	}
	return commitapp.DefaultFamily(input.Branch.Family())
}

func commitContextDescription(operation string, name branch.BranchName, ticketID, family string) string {
	if operation == "" {
		operation = "this commit"
	}
	lines := []string{
		"Fixed commit context for " + operation + ":",
		"  Branch: " + name.String(),
		"  Ticket key: " + strings.SplitN(ticketID, "-", 2)[0],
		"  Ticket ID: " + ticketID,
		"These values are derived from the governed branch and cannot be changed here.",
	}
	if family == "" {
		lines = append(lines, "Select the semantic commit family for the change.")
	} else {
		lines = append(lines, "Selected commit family: "+family+".")
	}
	return strings.Join(lines, "\n")
}

func missingCommitContext(name branch.BranchName) error {
	return problem.New(problem.Details{
		Code:        problem.CodeSharedLineMutationForbidden,
		Category:    problem.CategoryGovernance,
		Field:       "branch",
		Actual:      name.String(),
		Expected:    "a ticket-scoped working branch",
		Rule:        "governed commit creation derives its ticket context from the current working branch",
		Example:     "feature/ABC-123-add-export",
		Remediation: "switch to an official ticket branch or use the relevant governed workflow",
	})
}
