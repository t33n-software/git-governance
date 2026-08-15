// Package commitapp contains use cases for governed local commits.
package commitapp

import (
	"context"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// PushValidator gates an optional push after a successful local commit.
type PushValidator interface {
	ValidatePush(ctx context.Context, repository port.RepositoryIdentity, name branch.BranchName, base *branch.TargetBase) error
}

// Service owns commit validation, explicit staging, and optional validated
// pushes.
type Service struct {
	git           port.GitRepository
	keyPolicy     port.KeyPolicy
	pushValidator PushValidator
}

// NewService creates a commit application service.
func NewService(git port.GitRepository, keyPolicy port.KeyPolicy, pushValidator PushValidator) *Service {
	return &Service{
		git:           git,
		keyPolicy:     keyPolicy,
		pushValidator: pushValidator,
	}
}

// ValidateRequest describes a message validation request for a branch.
type ValidateRequest struct {
	Repository port.RepositoryIdentity
	Branch     branch.BranchName
	Message    commitmsg.Message
}

// ValidateResult confirms a valid message for the specified branch.
type ValidateResult struct {
	Branch  branch.BranchName
	Message commitmsg.Message
}

// Validate checks commit syntax, ticket ownership, key policy, Git-ref
// validity, and shared-line restrictions without mutating Git.
func (service *Service) Validate(ctx context.Context, request ValidateRequest) (ValidateResult, error) {
	repository, err := normalizeRepository(request.Repository)
	if err != nil {
		return ValidateResult{}, err
	}
	if err := contextError(ctx); err != nil {
		return ValidateResult{}, err
	}
	if request.Branch.IsZero() {
		return ValidateResult{}, invalidCommitInput("a current canonical branch is required")
	}
	if request.Branch.Family().IsSharedLine() {
		return ValidateResult{}, sharedLineForbidden(request.Branch)
	}
	if err := ValidateMessageForBranch(request.Branch, request.Message); err != nil {
		return ValidateResult{}, err
	}
	if err := service.git.ValidateBranchRef(ctx, repository, request.Branch); err != nil {
		return ValidateResult{}, err
	}

	messageTicket := request.Message.Header().Ticket()
	if service.keyPolicy != nil {
		if err := service.keyPolicy.ValidateKey(ctx, repository, messageTicket.Key()); err != nil {
			return ValidateResult{}, err
		}
	}
	return ValidateResult{Branch: request.Branch, Message: request.Message}, nil
}

// CreateRequest describes a local commit request.
type CreateRequest struct {
	Repository port.RepositoryIdentity
	Branch     branch.BranchName
	Message    commitmsg.Message
	StagePaths []string
	Base       *branch.TargetBase
	Push       bool
	DryRun     bool
}

// PlanStep describes an explicit commit operation.
type PlanStep struct {
	Action string
	Detail string
}

// CreateResult describes the resulting or planned commit operation.
type CreateResult struct {
	Branch    branch.BranchName
	Message   commitmsg.Message
	Committed bool
	Pushed    bool
	DryRun    bool
	Plan      []PlanStep
}

// Create validates and creates a commit. It never stages all files implicitly,
// amends commits, or force-pushes branches.
func (service *Service) Create(ctx context.Context, request CreateRequest) (CreateResult, error) {
	validation, err := service.Validate(ctx, ValidateRequest{
		Repository: request.Repository,
		Branch:     request.Branch,
		Message:    request.Message,
	})
	if err != nil {
		return CreateResult{}, err
	}
	repository := request.Repository
	if repository.Remote == "" {
		repository.Remote = "origin"
	}

	result := CreateResult{
		Branch:  validation.Branch,
		Message: validation.Message,
		DryRun:  request.DryRun,
	}
	if len(request.StagePaths) > 0 {
		result.Plan = append(result.Plan, PlanStep{
			Action: "stage",
			Detail: "stage explicitly requested paths",
		})
	}
	result.Plan = append(result.Plan, PlanStep{
		Action: "commit",
		Detail: validation.Message.Header().String(),
	})
	if request.Push {
		result.Plan = append(result.Plan, PlanStep{
			Action: "pre-push validation",
			Detail: "validate branch policy before pushing",
		})
		result.Plan = append(result.Plan, PlanStep{
			Action: "push",
			Detail: "push " + validation.Branch.String(),
		})
	}
	if request.DryRun {
		return result, nil
	}

	if len(request.StagePaths) > 0 {
		if err := service.git.Stage(ctx, repository, request.StagePaths); err != nil {
			return CreateResult{}, err
		}
	}
	staged, err := service.git.HasStagedChanges(ctx, repository)
	if err != nil {
		return CreateResult{}, err
	}
	if !staged {
		return CreateResult{}, problem.New(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryUsage,
			Field:       "staged changes",
			Expected:    "at least one staged change before committing",
			Rule:        "the CLI does not implicitly stage all repository changes",
			Example:     "git add -- cmd/git-governance/main.go",
			Remediation: "stage explicit paths with --stage or use Git before creating the commit",
		})
	}
	if err := service.git.Commit(ctx, repository, validation.Message); err != nil {
		return CreateResult{}, err
	}
	result.Committed = true

	if !request.Push {
		return result, nil
	}
	if service.pushValidator == nil {
		return CreateResult{}, internalDependencyError("push validator")
	}
	if err := service.pushValidator.ValidatePush(ctx, repository, validation.Branch, request.Base); err != nil {
		return CreateResult{}, err
	}
	publication, err := service.git.PublicationState(ctx, repository, validation.Branch)
	if err != nil {
		return CreateResult{}, err
	}
	if publication == branch.PublicationUnknown {
		return CreateResult{}, problem.New(problem.Details{
			Code:        problem.CodeBranchPublicationUnknown,
			Category:    problem.CategoryRepository,
			Field:       "branch publication state",
			Actual:      validation.Branch.String(),
			Expected:    "a known published or unpublished state",
			Rule:        "push setup must not infer upstream behavior from an unknown state",
			Remediation: "fetch the remote and resolve the tracking state before pushing",
		})
	}
	if err := service.git.Push(ctx, repository, validation.Branch, publication == branch.PublicationUnpublished); err != nil {
		return CreateResult{}, err
	}
	result.Pushed = true
	return result, nil
}

func normalizeRepository(repository port.RepositoryIdentity) (port.RepositoryIdentity, error) {
	if repository.Root == "" {
		return port.RepositoryIdentity{}, problem.New(problem.Details{
			Code:        problem.CodeRepositoryNotFound,
			Category:    problem.CategoryRepository,
			Field:       "repository",
			Expected:    "a discovered local Git repository",
			Rule:        "commit operations require a repository root",
			Example:     "C:\\work\\repository",
			Remediation: "run from a Git repository or pass --repo",
		})
	}
	if repository.Remote == "" {
		repository.Remote = "origin"
	}
	return repository, nil
}

func sharedLineForbidden(name branch.BranchName) error {
	return problem.New(problem.Details{
		Code:        problem.CodeSharedLineMutationForbidden,
		Category:    problem.CategoryGovernance,
		Field:       "branch",
		Actual:      name.String(),
		Expected:    "an official working branch",
		Rule:        "developers do not create direct local commits on shared lines",
		Example:     "feature/ABC-123-add-export-button",
		Remediation: "switch to an official ticket branch and open a pull request for the shared line",
	})
}

func invalidCommitInput(rule string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeCommitHeaderInvalid,
		Category:    problem.CategoryGovernance,
		Field:       "commit",
		Expected:    "a validated ticket-scoped Conventional Commit message",
		Rule:        rule,
		Example:     "feat(ABC-123): add export button",
		Remediation: "provide a valid branch and commit message",
	})
}

func contextError(ctx context.Context) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	return problem.Wrap(problem.Details{
		Code:        problem.CodeOperationCancelled,
		Category:    problem.CategoryCancelled,
		Field:       "commit operation",
		Expected:    "an active context",
		Rule:        "commit operations stop when their context is cancelled",
		Remediation: "retry with an active context",
	}, ctx.Err())
}

func internalDependencyError(name string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeInternal,
		Category:    problem.CategoryInternal,
		Field:       "dependency",
		Actual:      name,
		Expected:    "a configured application dependency",
		Rule:        "application services are composed with their required ports",
		Remediation: "fix the composition root",
	})
}
