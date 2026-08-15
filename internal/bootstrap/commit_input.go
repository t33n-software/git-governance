package bootstrap

import (
	"context"
	"strings"

	commitapp "github.com/t33n-software/git-governance/internal/application/commit"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

// commitMessageInput is the shared delivery-level input model for every
// command that creates a governed commit. CompleteMessage is retained only for
// compatibility and machine automation that needs bodies or footers.
type commitMessageInput struct {
	Branch               branch.BranchName
	CompleteMessage      string
	Family               string
	Description          string
	Body                 string
	Breaking             bool
	BreakingDescription  string
	FooterSpecifications []string
	DefaultFamily        commitmsg.Type
	RequireFamily        bool
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
	if input.CompleteMessage != "" {
		if hasStructuredCommitInput(input) {
			return commitmsg.Message{}, conflictingCommitInput()
		}
		message, err := commitmsg.Parse(input.CompleteMessage)
		if err != nil {
			return commitmsg.Message{}, err
		}
		return validateResolvedCommitMessage(input, message)
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
				"Describe the incompatible public contract change and the concrete migration impact without leading or trailing whitespace. Example: clients must use the versioned export endpoint.",
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
	message, err := commitapp.Compose(commitapp.Draft{
		Family:   family,
		Ticket:   ticketID,
		Subject:  description,
		Breaking: input.Breaking,
		Body:     input.Body,
		Footers:  footers,
	})
	if err != nil {
		return commitmsg.Message{}, err
	}
	return validateResolvedCommitMessage(input, message)
}

func (application *application) resolveCommitFamily(
	ctx context.Context,
	input commitMessageInput,
	ticketID ticket.ID,
) (commitmsg.Type, error) {
	if input.Family != "" {
		return commitmsg.ParseType(input.Family)
	}
	if !application.promptAvailable() {
		if input.RequireFamily {
			return "", missingInput("commit family")
		}
		return defaultCommitFamily(input), nil
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
			"It must be one non-empty, unpadded line of at most 200 Unicode characters and must not contain control characters. "+
			"Example: add export button.",
		func(value string) (string, error) {
			_, err := commitmsg.NewHeader(family, ticketID, value, input.Breaking)
			return value, err
		},
	)
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

func hasStructuredCommitInput(input commitMessageInput) bool {
	return input.Family != "" ||
		input.Description != "" ||
		input.Body != "" ||
		input.Breaking ||
		input.BreakingDescription != "" ||
		len(input.FooterSpecifications) > 0
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

func conflictingCommitInput() error {
	return problem.New(problem.Details{
		Code:        problem.CodeInvalidInput,
		Category:    problem.CategoryUsage,
		Field:       "commit message",
		Expected:    "either a complete --message or structured family and description inputs",
		Rule:        "a commit is composed from exactly one input representation",
		Example:     "--type feat --subject \"add export button\"",
		Remediation: "remove --message or remove the structured commit flags",
	})
}
