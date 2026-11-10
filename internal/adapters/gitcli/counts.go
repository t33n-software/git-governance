package gitcli

import (
	"context"
	"strconv"
	"strings"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// CountCommitsBehindBase counts the commits the current HEAD lacks from the
// given remote-tracking base. It is the read-only counting surface behind the
// doctor shared-line freshness check; the governed mutation paths keep using
// their own base-freshness bindings and never derive a gate from this count.
func (repository *Repository) CountCommitsBehindBase(
	ctx context.Context,
	identity port.RepositoryIdentity,
	base branch.TargetBase,
) (int, error) {
	result := repository.invoke(ctx, identity.Root, nil, "rev-list", "--count", "HEAD.."+base.String())
	if result.err != nil {
		return 0, repository.commandProblem(
			problem.CodeBranchBaseInvalid,
			identity,
			"count the commits the checked-out line lacks from its remote-tracking base",
			result,
		)
	}
	count, err := strconv.Atoi(strings.TrimSpace(result.stdout))
	if err != nil || count < 0 {
		return 0, problem.Wrap(problem.Details{
			Code:        problem.CodeBranchBaseInvalid,
			Category:    problem.CategoryRepository,
			Field:       "target base",
			Actual:      base.String(),
			Expected:    "a comparable remote-tracking base",
			Rule:        "Git must return a non-negative behind-commit count",
			Example:     "origin/develop",
			Remediation: "fetch the remote and verify the selected target base exists",
		}, err)
	}
	return count, nil
}

var _ port.CommitsBehindBaseCounter = (*Repository)(nil)
