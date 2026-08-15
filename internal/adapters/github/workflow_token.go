package github

import (
	"context"
	"strings"

	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// WorkflowTokenResolver exposes only the ephemeral job token injected by a
// GitHub Actions controller. The token remains adapter-private and is never
// rendered into reports, errors, or persisted state.
type WorkflowTokenResolver struct {
	token string
}

// NewWorkflowTokenResolver constructs a resolver for one ephemeral workflow
// job token.
func NewWorkflowTokenResolver(token string) *WorkflowTokenResolver {
	return &WorkflowTokenResolver{token: token}
}

// Resolve returns the token only to the GitHub HTTP adapter after the caller
// has already bound the target repository.
func (resolver *WorkflowTokenResolver) Resolve(ctx context.Context, _ CredentialTarget) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if resolver == nil || strings.TrimSpace(resolver.token) == "" {
		return "", problem.New(problem.Details{
			Code:        problem.CodeConfigurationUnavailable,
			Category:    problem.CategoryConfig,
			Field:       "GitHub Actions workflow token",
			Expected:    "an ephemeral GITHUB_TOKEN available only inside the protected controller job",
			Rule:        "protected-line request, execution, and finalization controllers use job-scoped ephemeral credentials",
			Remediation: "run the command from its designated GitHub Actions controller instead of a local shell",
		})
	}
	return resolver.token, nil
}

var _ CredentialResolver = (*WorkflowTokenResolver)(nil)
