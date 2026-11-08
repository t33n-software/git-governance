package bootstrap

import (
	"context"
	"strings"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/cliparam"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

// This file owns the interactive resolution funnels: shared validated input,
// ticket identity, branch slug and family selection, base parsing, and the
// scratch transfer message binding. Prompts carry the same rules as the
// static help. Canonical convention: docs/conventions/cli/interaction-model.md.

func (application *application) resolveKey(ctx context.Context, service services, raw string) (ticket.Key, error) {
	if raw != "" {
		return ticket.ParseKey(raw)
	}
	defaultValue := ""
	preferences, err := service.preferences.List(ctx)
	if err == nil && preferences.DefaultKey != nil {
		defaultValue = preferences.DefaultKey.String()
	}
	if !application.promptAvailable() {
		return ticket.Key{}, missingInput("ticket key")
	}
	value, err := application.prompt().Input(ctx, port.InputRequest{
		Label:       "Ticket key",
		Description: cliparam.TicketKey().PromptText(),
		Default:     defaultValue,
		Required:    true,
		Validate:    inputValidator(ticket.ParseKey),
	})
	if err != nil {
		return ticket.Key{}, err
	}
	return ticket.ParseKey(value)
}

func (application *application) resolveNumber(ctx context.Context, raw string) (ticket.Number, error) {
	return resolveValidatedInput(
		application,
		ctx,
		raw,
		"Ticket number",
		cliparam.TicketNumber().PromptText(),
		ticket.ParseNumber,
	)
}

func (application *application) resolveSlug(ctx context.Context, raw string, label string) (branch.Slug, error) {
	return resolveValidatedInput(
		application,
		ctx,
		raw,
		label,
		cliparam.BranchSlug().PromptText(),
		branch.ParseSlug,
	)
}

// resolveScratchMergeMessage binds the squash transfer commit from structured
// values only. The body is mandatory: the transfer commit is the only record
// of the discarded experiment paths. Canonical convention:
// docs/conventions/commits/family-selection.md.
func (application *application) resolveScratchMergeMessage(
	ctx context.Context,
	repository port.RepositoryIdentity,
	target branch.BranchName,
	family string,
	description string,
	body string,
	breaking bool,
	breakingDescription string,
	footerSpecs []string,
) (commitmsg.Message, error) {
	return application.resolveCommitMessage(ctx, commitMessageInput{
		Branch:               target,
		Repository:           repository,
		Family:               family,
		Description:          description,
		Body:                 body,
		Breaking:             breaking,
		BreakingDescription:  breakingDescription,
		FooterSpecifications: footerSpecs,
		RequireBody:          true,
		DescriptionLabel:     "Squash commit description",
		Operation:            "the scratch squash transfer",
		Validate: func(message commitmsg.Message) error {
			return branchapp.ValidateScratchMergeMessage(target, message)
		},
	})
}

func (application *application) resolveFamily(ctx context.Context, raw string, includeSpecial bool) (branch.Family, error) {
	if raw != "" {
		return branch.ParseFamily(raw)
	}
	if !application.promptAvailable() {
		return "", missingInput("branch family")
	}

	families := branchapp.ListFamilies()
	options := make([]port.SelectOption, 0, len(families))
	for _, family := range families {
		if !includeSpecial && !family.DirectlyCreatable {
			continue
		}
		options = append(options, port.SelectOption{
			Value:       family.Family.String(),
			Label:       family.Label,
			Description: family.Description,
		})
	}
	value, err := application.prompt().Select(ctx, port.SelectRequest{
		Label:       "Branch family",
		Description: "Select the branch family that matches the work. The list command explains every family.",
		Options:     options,
		Default:     branch.FamilyFeature.String(),
	})
	if err != nil {
		return "", err
	}
	return branch.ParseFamily(value)
}

func parseTicketParts(keyRaw, numberRaw string) (ticket.ID, error) {
	key, err := ticket.ParseKey(keyRaw)
	if err != nil {
		return ticket.ID{}, err
	}
	number, err := ticket.ParseNumber(numberRaw)
	if err != nil {
		return ticket.ID{}, err
	}
	return ticket.NewID(key, number), nil
}

func parseBase(raw, remote string) (*branch.TargetBase, error) {
	if raw == "" {
		return nil, nil
	}
	nameRaw := raw
	prefix := remote + "/"
	if strings.HasPrefix(raw, prefix) {
		nameRaw = strings.TrimPrefix(raw, prefix)
	} else if strings.Contains(raw, "/") {
		return nil, problem.New(problem.Details{
			Code:        problem.CodeBranchBaseInvalid,
			Category:    problem.CategoryUsage,
			Field:       "base",
			Actual:      raw,
			Expected:    "a canonical branch name or " + prefix + "<branch>",
			Rule:        "explicit bases use the selected remote only",
			Example:     prefix + "develop",
			Remediation: "pass a branch on the selected --remote",
		})
	}
	name, err := branch.ParseName(nameRaw)
	if err != nil {
		return nil, err
	}
	base, err := branch.NewTargetBase(remote, name)
	if err != nil {
		return nil, err
	}
	return &base, nil
}

func (application *application) resolveScratchBase(
	ctx context.Context,
	raw, remote string,
	id ticket.ID,
) (*branch.TargetBase, error) {
	base, err := resolveValidatedInput(
		application,
		ctx,
		raw,
		"Official ticket branch base",
		"Enter the local official feature, fix, docs, refactor, chore, test, perf, or hotfix branch for the same ticket. Do not use origin/, a shared line, or a scratch branch. Example: feature/ABC-123-add-export.",
		func(value string) (branch.TargetBase, error) {
			return parseScratchBase(value, remote, id)
		},
	)
	if err != nil {
		return nil, err
	}
	return &base, nil
}

func parseScratchBase(raw, remote string, id ticket.ID) (branch.TargetBase, error) {
	if strings.HasPrefix(raw, remote+"/") {
		return branch.TargetBase{}, problem.New(problem.Details{
			Code:        problem.CodeBranchBaseInvalid,
			Category:    problem.CategoryGovernance,
			Field:       "scratch base",
			Actual:      raw,
			Expected:    "a local official ticket branch name",
			Rule:        "scratch branches start from the local official branch, not a remote-tracking ref",
			Example:     "feature/ABC-123-add-export",
			Remediation: "provide the checked-out official ticket branch without the remote prefix",
		})
	}
	base, err := branch.ParseLocalBase(raw)
	if err != nil {
		return branch.TargetBase{}, err
	}
	name := base.Branch()
	if !name.Family().IsOfficialWorkingBranch() {
		return branch.TargetBase{}, problem.New(problem.Details{
			Code:        problem.CodeBranchBaseInvalid,
			Category:    problem.CategoryGovernance,
			Field:       "scratch base",
			Actual:      raw,
			Expected:    "a local feature, fix, docs, refactor, chore, test, perf, or hotfix branch",
			Rule:        "scratch branches are private exploration of an official ticket branch",
			Example:     "feature/ABC-123-add-export",
			Remediation: "select the official branch for the same ticket or use workflow ticket start",
		})
	}
	baseTicket, hasTicket := name.Ticket()
	if !hasTicket || baseTicket.String() != id.String() {
		return branch.TargetBase{}, problem.New(problem.Details{
			Code:        problem.CodeBranchBaseInvalid,
			Category:    problem.CategoryGovernance,
			Field:       "scratch base",
			Actual:      raw,
			Expected:    "a local official branch for ticket " + id.String(),
			Rule:        "scratch branches stay attached to the official branch for the same ticket",
			Example:     "feature/" + id.String() + "-add-export",
			Remediation: "select the official branch that carries the current ticket",
		})
	}
	return base, nil
}

// resolveValidatedInput is the shared funnel for validated interactive
// inputs: prompts carry the same rules as the static help and validate live.
// Canonical convention: docs/conventions/cli/interaction-model.md.
func resolveValidatedInput[T any](
	application *application,
	ctx context.Context,
	raw, label, description string,
	parse func(string) (T, error),
) (T, error) {
	if raw != "" {
		return parse(raw)
	}
	value, err := application.requireInput(ctx, "", label, description, inputValidator(parse))
	if err != nil {
		var zero T
		return zero, err
	}
	return parse(value)
}

func inputValidator[T any](parse func(string) (T, error)) port.InputValidator {
	return func(value string) error {
		_, err := parse(value)
		return err
	}
}
