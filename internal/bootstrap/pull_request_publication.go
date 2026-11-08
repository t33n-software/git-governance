package bootstrap

import (
	"context"
	"fmt"
	"strings"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/application/workflow"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// This file owns the pull-request publication funnel: the fail-closed
// validation before any Git mutation, the mandatory description mandate, and
// the interactive publication resolution shared by every publishing
// endpoint. Canonical convention:
// docs/conventions/pull-requests/description-mandate.md.

func (application *application) validatePullRequestPublication(
	services services,
	push, createPullRequest bool,
	body string,
) error {
	if createPullRequest && !push {
		return invalidOption("create-pull-request", "true", "--push")
	}
	if err := validatePullRequestBody(createPullRequest, body); err != nil {
		return err
	}
	if createPullRequest && !application.options.dryRun && !services.tickets.HasPullRequestPublisher() {
		return pullRequestPublisherUnavailable()
	}
	return nil
}

// validatePullRequestBody enforces the mandatory pull-request description
// owned by docs/conventions/pull-requests/description-mandate.md before any
// Git mutation happens: the description must be non-empty and must carry the
// five canonical sections in their immutable order.
func validatePullRequestBody(createPullRequest bool, body string) error {
	if !createPullRequest {
		return nil
	}
	if strings.TrimSpace(body) == "" {
		return invalidOption("body-file", "empty", "a non-empty pull-request description file passed as --body-file when --create-pull-request is set")
	}
	return validatePullRequestDescriptionStructure(body)
}

// pullRequestDescriptionSections is the canonical five-section layout of the
// pull-request description in its immutable order, owned by
// docs/conventions/pull-requests/description-mandate.md.
var pullRequestDescriptionSections = []string{
	"Summary",
	"Scope and Non-Goals",
	"Commit Series",
	"Risk and Rollback",
	"Verification and Review Focus",
}

// validatePullRequestDescriptionStructure enforces the canonical five-section
// description layout: every canonical section appears as a `## <Section Name>`
// Markdown heading in the immutable order, each with non-empty content. The
// check binds structure and presence only — the content inside a section is
// never validated. Additional non-canonical headings are not rejected; the
// mandate binds the canonical sections, not a closed heading set.
func validatePullRequestDescriptionStructure(body string) error {
	type descriptionHeading struct {
		name string
		line int
	}
	lines := strings.Split(body, "\n")
	headings := make([]descriptionHeading, 0, len(pullRequestDescriptionSections))
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "## ") {
			continue
		}
		headings = append(headings, descriptionHeading{
			name: strings.TrimSpace(strings.TrimPrefix(trimmed, "## ")),
			line: index,
		})
	}

	sectionLines := make(map[string]int, len(pullRequestDescriptionSections))
	cursor := 0
	for _, canonical := range pullRequestDescriptionSections {
		found := -1
		for cursor < len(headings) {
			candidate := headings[cursor]
			cursor++
			if candidate.name == canonical {
				found = candidate.line
				break
			}
		}
		if found < 0 {
			found := make([]string, 0, len(headings))
			for _, heading := range headings {
				found = append(found, heading.name)
			}
			return pullRequestDescriptionInvalid(
				fmt.Sprintf("found sections: %s", strings.Join(found, "; ")),
				fmt.Sprintf("the canonical section '%s' is missing or out of order", canonical),
			)
		}
		sectionLines[canonical] = found
	}

	for _, canonical := range pullRequestDescriptionSections {
		start := sectionLines[canonical] + 1
		end := len(lines)
		for _, heading := range headings {
			if heading.line > sectionLines[canonical] {
				end = heading.line
				break
			}
		}
		if strings.TrimSpace(strings.Join(lines[start:end], "\n")) == "" {
			return pullRequestDescriptionInvalid(
				fmt.Sprintf("the section '## %s' has no content", canonical),
				"every canonical section carries non-empty content",
			)
		}
	}
	return nil
}

func pullRequestDescriptionInvalid(actual, rule string) error {
	return problem.New(problem.Details{
		Code:     problem.CodePullRequestDescriptionInvalid,
		Category: problem.CategoryGovernance,
		Field:    "pull request description",
		Actual:   actual,
		Expected: "the five canonical sections as '## <Section Name>' headings in order: " +
			strings.Join(pullRequestDescriptionSections, ", ") +
			", each with non-empty content",
		Rule:        rule,
		Example:     "## Summary\n\nAdd the export capability.",
		Remediation: "compose all five canonical sections in the immutable order with non-empty content and pass the description via --body-file",
	})
}

func (application *application) completePreparedPublication(
	ctx context.Context,
	services services,
	repository port.RepositoryIdentity,
	result *workflow.PublishTicketResult,
	requestedPush, requestedPullRequest bool,
	workflowManaged bool,
) error {
	if result == nil || result.DryRun {
		return nil
	}
	if requestedPullRequest {
		if err := services.tickets.PreflightPullRequest(ctx, repository, result.PullRequest); err != nil {
			return err
		}
	}
	push := requestedPush
	if application.promptAvailable() && !application.options.yes {
		confirmed, err := application.prompt().Confirm(ctx, port.ConfirmRequest{
			Label:       "Push official ticket branch",
			Description: "Push " + result.Branch.String() + " after the completed synchronization? The first push configures its matching upstream branch.",
			Default:     requestedPush,
		})
		if err != nil {
			return err
		}
		push = confirmed
	}
	if !push {
		return nil
	}
	base := result.Sync.Base
	if err := services.tickets.PushPreparedTicket(ctx, repository, result.Branch, &base, workflowManaged); err != nil {
		return err
	}
	result.Pushed = true

	pullRequest, createPullRequest, err := application.resolvePullRequestPublication(
		ctx,
		services,
		result.PullRequest,
		requestedPullRequest,
	)
	if err != nil {
		return err
	}
	if !createPullRequest {
		return nil
	}
	result.PullRequest = pullRequest
	if err := services.tickets.PreflightPullRequest(ctx, repository, result.PullRequest); err != nil {
		return err
	}
	publication, err := services.tickets.PublishPullRequest(ctx, repository, result.PullRequest)
	if err != nil {
		return err
	}
	result.PublishedURL = publication.URL
	result.IntegrationLineReturn = publication.IntegrationLineReturn
	return nil
}

func (application *application) resolvePullRequestPublication(
	ctx context.Context,
	services services,
	request port.PullRequest,
	requested bool,
) (port.PullRequest, bool, error) {
	if application.options.dryRun {
		return request, false, nil
	}
	if !services.tickets.HasPullRequestPublisher() {
		if requested {
			return request, false, pullRequestPublisherUnavailable()
		}
		return request, false, nil
	}
	create := requested
	if application.promptAvailable() && !application.options.yes {
		confirmed, err := application.prompt().Confirm(ctx, port.ConfirmRequest{
			Label: "Create pull request",
			Description: "Create the pull request from " + request.Source.String() +
				" to " + request.Target.String() + " now?",
			Default: requested,
		})
		if err != nil {
			return request, false, err
		}
		create = confirmed
	} else if requested && !application.options.yes {
		return request, false, pullRequestConfirmationRequired()
	}
	if !create {
		return request, false, nil
	}
	return application.resolvePullRequestBody(ctx, request)
}

// resolvePullRequestBody enforces the mandatory pull-request description
// owned by docs/conventions/pull-requests/description-mandate.md: a confirmed
// publication either carries the description already or asks for it
// interactively; non-interactive creation without it fails closed.
func (application *application) resolvePullRequestBody(
	ctx context.Context,
	request port.PullRequest,
) (port.PullRequest, bool, error) {
	if strings.TrimSpace(request.Body) != "" {
		return request, true, nil
	}
	if application.promptAvailable() && !application.options.yes {
		body, err := application.prompt().Input(ctx, port.InputRequest{
			Label: "Pull request description",
			Description: "Describe the change set for " + request.Source.String() + " to " + request.Target.String() +
				": summary, scope and non-goals, commit series, risk and rollback, verification and review focus. The description is mandatory.",
			Required: true,
			Validate: func(value string) error {
				if strings.TrimSpace(value) == "" {
					return problem.New(problem.Details{
						Code:        problem.CodeInvalidInput,
						Category:    problem.CategoryUsage,
						Field:       "pull request description",
						Expected:    "a non-empty description",
						Rule:        "every governed pull request carries a mandatory description",
						Remediation: "enter the canonical pull-request description",
					})
				}
				return nil
			},
		})
		if err != nil {
			return request, false, err
		}
		request.Body = body
		return request, true, nil
	}
	return request, false, invalidOption("body-file", "empty", "a non-empty pull-request description file passed as --body-file")
}

func pullRequestPublisherUnavailable() error {
	return problem.New(problem.Details{
		Code:        problem.CodeExternalCommandFailed,
		Category:    problem.CategoryExternal,
		Field:       "pull request publisher",
		Expected:    "a hosting provider derived from the selected remote and its bound authentication session",
		Rule:        "a real pull request can be created only through the provider adapter derived from the selected remote",
		Remediation: "complete auth login github for the selected remote or configure the managed credential broker",
	})
}

// pullRequestPublicationIntentCause renders the intent-only publication cause
// from the resolved provider posture, so the report vocabulary exists exactly
// once: the selected remote simply carries no supported hosting identity.
func (application *application) pullRequestPublicationIntentCause() string {
	return "intent-only; no supported hosting identity derived from the selected remote"
}

func pullRequestConfirmationRequired() error {
	return problem.New(problem.Details{
		Code:        problem.CodeInvalidInput,
		Category:    problem.CategoryUsage,
		Field:       "confirmation",
		Expected:    "--yes for non-interactive pull-request creation",
		Rule:        "external pull-request creation requires explicit confirmation",
		Example:     "--yes --create-pull-request",
		Remediation: "pass --yes or run in an interactive terminal",
	})
}
