package workflow

import (
	"context"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func bootstrapRepository(repository port.RepositoryIdentity) (port.RepositoryIdentity, error) {
	if repository.Root == "" {
		return port.RepositoryIdentity{}, problem.New(problem.Details{
			Code:        problem.CodeRepositoryNotFound,
			Category:    problem.CategoryRepository,
			Field:       "repository",
			Expected:    "a discovered local Git repository",
			Rule:        "the governed repository birth requires a repository root",
			Example:     "C:\\work\\repository",
			Remediation: "run from a Git repository or pass --repo",
		})
	}
	if repository.Remote == "" {
		repository.Remote = "origin"
	}
	return repository, nil
}

func bootstrapContextError(ctx context.Context) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	return problem.Wrap(problem.Details{
		Code:        problem.CodeOperationCancelled,
		Category:    problem.CategoryCancelled,
		Field:       "bootstrap operation",
		Expected:    "an active context",
		Rule:        "the governed repository birth stops when the caller cancels its context",
		Remediation: "retry with an active context",
	}, ctx.Err())
}

func repositoryAlreadyBorn(detail string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeRepositoryAlreadyBorn,
		Category:    problem.CategoryRepository,
		Field:       "repository",
		Actual:      detail,
		Expected:    "an unborn repository without commits or references",
		Rule:        "the governed bootstrap births only unborn repositories; a born repository is never re-born or mutated",
		Remediation: "run the bootstrap only on a repository without commits or references",
	})
}

func unbornHeadMismatch(current branch.BranchName) error {
	return problem.New(problem.Details{
		Code:        problem.CodeUnbornHeadMismatch,
		Category:    problem.CategoryRepository,
		Field:       "current branch",
		Actual:      current.String(),
		Expected:    "main",
		Rule:        "the governed genesis births main; the unborn HEAD must target main",
		Example:     "git symbolic-ref HEAD refs/heads/main",
		Remediation: "point the unborn HEAD at main or re-initialize with git init -b main",
	})
}

func emptyContentSet() error {
	return problem.New(problem.Details{
		Code:        problem.CodeInvalidInput,
		Category:    problem.CategoryUsage,
		Field:       "stage paths",
		Expected:    "an explicit, non-empty initial content set",
		Rule:        "the governed genesis requires an explicit content set; an empty genesis is a policy decision, never a default",
		Example:     "--stage .",
		Remediation: "pass at least one stage path that matches repository content",
	})
}

func bootstrapCapabilityRequired(capability string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeConfigurationUnavailable,
		Category:    problem.CategoryConfig,
		Field:       "Git adapter",
		Expected:    "a Git adapter with " + capability + " capability",
		Rule:        "the governed repository birth requires the composed adapter capabilities",
		Remediation: "use the composed Git adapter of the CLI",
	})
}

func genesisFinalizerProblem(detail string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeInternal,
		Category:    problem.CategoryInternal,
		Field:       "genesis finalizer",
		Actual:      detail,
		Expected:    "the born refs, the shared genesis revision, the verified signature, and the installed hook boundary",
		Rule:        "the genesis finalizer proof failed after the mutation; the local birth is incomplete and reversible",
		Remediation: "review the repository state, tear down the local birth, and retry the bootstrap",
	})
}
