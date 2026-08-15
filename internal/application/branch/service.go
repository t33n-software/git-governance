package branchapp

import (
	"context"
	"fmt"
	"strings"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

// Service owns canonical branch validation and creation orchestration.
type Service struct {
	git       port.GitRepository
	keyPolicy port.KeyPolicy
}

// NewService creates a branch application service.
func NewService(git port.GitRepository, keyPolicy port.KeyPolicy) *Service {
	return &Service{
		git:       git,
		keyPolicy: keyPolicy,
	}
}

// ValidateRequest describes a branch validation request.
type ValidateRequest struct {
	Repository port.RepositoryIdentity
	Name       branch.BranchName
}

// ValidateResult is a successfully validated branch.
type ValidateResult struct {
	Name branch.BranchName
}

// Validate checks domain, policy, and Git-ref invariants without mutation.
func (service *Service) Validate(ctx context.Context, request ValidateRequest) (ValidateResult, error) {
	repository, err := normalizeRepository(request.Repository)
	if err != nil {
		return ValidateResult{}, err
	}
	if request.Name.IsZero() {
		return ValidateResult{}, invalidBranchInput("a canonical branch name is required")
	}
	if err := service.validateDomainAndPolicy(ctx, repository, request.Name); err != nil {
		return ValidateResult{}, err
	}
	return ValidateResult{Name: request.Name}, nil
}

// CreateRequest describes a new ticket-scoped working branch.
type CreateRequest struct {
	Repository      port.RepositoryIdentity
	Family          branch.Family
	Ticket          ticket.ID
	Slug            branch.Slug
	Base            *branch.TargetBase
	Switch          *bool
	DryRun          bool
	WorkflowManaged bool
	SkipFetch       bool
}

// PlanStep describes one planned or executed mutation.
type PlanStep struct {
	Action string
	Detail string
}

// CreateResult provides the resolved name, base, and plan.
type CreateResult struct {
	Name     branch.BranchName
	Base     branch.TargetBase
	Switched bool
	DryRun   bool
	Plan     []PlanStep
}

// Create validates and creates a canonical branch. Regular ticket branches
// start from origin/develop; special workflows must set WorkflowManaged and an
// explicit base.
func (service *Service) Create(ctx context.Context, request CreateRequest) (CreateResult, error) {
	repository, err := normalizeRepository(request.Repository)
	if err != nil {
		return CreateResult{}, err
	}
	if err := contextError(ctx); err != nil {
		return CreateResult{}, err
	}

	name, base, err := resolveCreation(request, repository)
	if err != nil {
		return CreateResult{}, err
	}
	if err := service.validateDomainAndPolicy(ctx, repository, name); err != nil {
		return CreateResult{}, err
	}

	hasCommits, err := service.git.HasCommits(ctx, repository)
	if err != nil {
		return CreateResult{}, err
	}
	if !hasCommits {
		return CreateResult{}, problem.New(problem.Details{
			Code:        problem.CodeRepositoryHasNoCommits,
			Category:    problem.CategoryRepository,
			Field:       "repository",
			Expected:    "at least one commit before creating a branch",
			Rule:        "branch creation does not create an implicit initial commit",
			Example:     "git commit -m \"chore(ABC-123): initialize repository\"",
			Remediation: "bootstrap the repository with an explicit reviewed commit, then retry",
		})
	}

	switched := true
	if request.Switch != nil {
		switched = *request.Switch
	}
	plan := make([]PlanStep, 0, 3)
	if !request.SkipFetch {
		plan = append(plan, PlanStep{Action: "fetch", Detail: "git fetch --prune " + repository.Remote})
	}
	plan = append(plan, PlanStep{Action: "create", Detail: name.String() + " from " + base.String()})
	result := CreateResult{
		Name:     name,
		Base:     base,
		Switched: switched,
		DryRun:   request.DryRun,
		Plan:     plan,
	}
	if switched {
		result.Plan = append(result.Plan, PlanStep{Action: "switch", Detail: "switch to " + name.String()})
	}
	if request.DryRun {
		if err := service.ensureBranchAvailability(ctx, repository, request, name); err != nil {
			return CreateResult{}, err
		}
		return result, nil
	}

	clean, err := service.git.IsWorktreeClean(ctx, repository)
	if err != nil {
		return CreateResult{}, err
	}
	if !clean {
		return CreateResult{}, problem.New(problem.Details{
			Code:        problem.CodeWorktreeNotClean,
			Category:    problem.CategoryRepository,
			Field:       "worktree",
			Expected:    "a clean working tree before switching branches",
			Rule:        "branch creation must not risk uncommitted work",
			Example:     "git status --porcelain returns no entries",
			Remediation: "commit, stash, or discard local changes before creating the branch",
		})
	}
	if !request.SkipFetch {
		if err := service.git.Fetch(ctx, repository); err != nil {
			return CreateResult{}, err
		}
	}
	if base.IsRemoteTracking() {
		exists, err := service.git.TargetBaseExists(ctx, repository, base)
		if err != nil {
			return CreateResult{}, err
		}
		if !exists {
			return CreateResult{}, unavailableTargetBase(base)
		}
	}
	if err := service.ensureBranchAvailability(ctx, repository, request, name); err != nil {
		return CreateResult{}, err
	}
	if err := service.git.CreateBranch(ctx, repository, name, base, switched); err != nil {
		return CreateResult{}, err
	}
	return result, nil
}

func (service *Service) ensureBranchAvailability(
	ctx context.Context,
	repository port.RepositoryIdentity,
	request CreateRequest,
	name branch.BranchName,
) error {
	exists, err := service.git.BranchExists(ctx, repository, name)
	if err != nil {
		return err
	}
	if exists {
		return problem.New(problem.Details{
			Code:        problem.CodeBranchAlreadyExists,
			Category:    problem.CategoryRepository,
			Field:       "branch",
			Actual:      name.String(),
			Expected:    "a branch name not already present locally",
			Rule:        "branch creation must not overwrite an existing branch",
			Example:     "feature/ABC-123-add-export-button",
			Remediation: "choose a new branch name or switch to the existing branch",
		})
	}
	if !requiresExclusiveTicketBranch(request.Family, request.WorkflowManaged) {
		return nil
	}

	existing, err := service.git.OfficialBranchesForTicket(ctx, repository, request.Ticket)
	if err != nil {
		return err
	}
	if len(existing) == 0 {
		return nil
	}

	names := make([]string, 0, len(existing))
	for _, candidate := range existing {
		names = append(names, candidate.String())
	}
	return problem.New(problem.Details{
		Code:        problem.CodeTicketBranchAlreadyExists,
		Category:    problem.CategoryGovernance,
		Field:       "ticket",
		Actual:      request.Ticket.String() + " on " + strings.Join(names, ", "),
		Expected:    "no existing official regular branch for the same ticket",
		Rule:        "normal ticket work uses exactly one official branch per ticket",
		Example:     "feature/ABC-123-add-export-button",
		Remediation: "continue on the existing official branch or close it before starting unrelated ticket work",
	})
}

func requiresExclusiveTicketBranch(family branch.Family, workflowManaged bool) bool {
	if workflowManaged {
		return false
	}
	switch family {
	case branch.FamilyFeature, branch.FamilyFix, branch.FamilyDocs, branch.FamilyRefactor,
		branch.FamilyChore, branch.FamilyTest, branch.FamilyPerf:
		return true
	default:
		return false
	}
}

func (service *Service) validateDomainAndPolicy(ctx context.Context, repository port.RepositoryIdentity, name branch.BranchName) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if scopedTicket, ok := name.Ticket(); ok && service.keyPolicy != nil {
		if err := service.keyPolicy.ValidateKey(ctx, repository, scopedTicket.Key()); err != nil {
			return err
		}
	}
	return service.git.ValidateBranchRef(ctx, repository, name)
}

type ticketBranchFactory func(branch.Family, ticket.ID, branch.Slug) (branch.BranchName, error)

func resolveCreation(request CreateRequest, repository port.RepositoryIdentity) (branch.BranchName, branch.TargetBase, error) {
	return resolveCreationWithFactory(request, repository, branch.NewTicketBranch)
}

func resolveCreationWithFactory(
	request CreateRequest,
	repository port.RepositoryIdentity,
	createTicketBranch ticketBranchFactory,
) (branch.BranchName, branch.TargetBase, error) {
	if !request.Family.IsKnown() {
		return branch.BranchName{}, branch.TargetBase{}, invalidBranchInput("a supported branch family is required")
	}
	if request.Family == branch.FamilyRelease || request.Family == branch.FamilySupport {
		return branch.BranchName{}, branch.TargetBase{}, specialWorkflowRequired(request.Family)
	}
	if request.Family.IsSharedLine() {
		return branch.BranchName{}, branch.TargetBase{}, problem.New(problem.Details{
			Code:        problem.CodeSharedLineMutationForbidden,
			Category:    problem.CategoryGovernance,
			Field:       "branch family",
			Actual:      request.Family.String(),
			Expected:    "a non-shared working branch family",
			Rule:        "shared lines are protected and cannot be created as ordinary work branches",
			Example:     "feature",
			Remediation: "select a ticket branch family or use the relevant release or support workflow",
		})
	}
	if request.Family == branch.FamilyHotfix && !request.WorkflowManaged {
		return branch.BranchName{}, branch.TargetBase{}, specialWorkflowRequired(request.Family)
	}
	if request.Ticket.IsZero() || request.Slug.String() == "" {
		return branch.BranchName{}, branch.TargetBase{}, invalidBranchInput("ticket-scoped branches require both a ticket and a kebab-case slug")
	}

	name, err := createTicketBranch(request.Family, request.Ticket, request.Slug)
	if err != nil {
		return branch.BranchName{}, branch.TargetBase{}, err
	}

	if defaultBase, found, err := request.Family.DefaultTargetBase(repository.Remote); err != nil {
		return branch.BranchName{}, branch.TargetBase{}, err
	} else if found {
		if request.Base != nil && request.Base.String() != defaultBase.String() {
			if request.WorkflowManaged &&
				(request.Family == branch.FamilyFix ||
					request.Family == branch.FamilyDocs ||
					request.Family == branch.FamilyChore) {
				if err := validateSpecialBase(request.Family, *request.Base); err != nil {
					return branch.BranchName{}, branch.TargetBase{}, err
				}
				return name, *request.Base, nil
			}
			return branch.BranchName{}, branch.TargetBase{}, invalidBase(
				request.Base.String(),
				"regular ticket work must start from the configured remote develop branch",
				defaultBase.String(),
			)
		}
		return name, defaultBase, nil
	}

	if request.Base == nil {
		return branch.BranchName{}, branch.TargetBase{}, invalidBase(
			"",
			"this branch family requires an explicit target base",
			"origin/main, origin/release/<semver>, origin/support/<major.minor>, or its official ticket branch",
		)
	}
	if err := validateSpecialBase(request.Family, *request.Base); err != nil {
		return branch.BranchName{}, branch.TargetBase{}, err
	}
	if request.Family == branch.FamilyScratch {
		if request.Base.IsRemoteTracking() {
			return branch.BranchName{}, branch.TargetBase{}, invalidBase(
				request.Base.String(),
				"a scratch branch must use a local official branch as its base",
				"feature/"+request.Ticket.String()+"-<slug> or another local official branch with the same ticket",
			)
		}
		baseTicket, hasBaseTicket := request.Base.Branch().Ticket()
		if !hasBaseTicket || baseTicket.String() != request.Ticket.String() {
			return branch.BranchName{}, branch.TargetBase{}, invalidBase(
				request.Base.String(),
				"a scratch branch must use the official branch for the same ticket",
				"feature/"+request.Ticket.String()+"-<slug> or another official branch with the same ticket",
			)
		}
	}
	return name, *request.Base, nil
}

func validateSpecialBase(family branch.Family, base branch.TargetBase) error {
	baseFamily := base.Branch().Family()
	switch family {
	case branch.FamilyHotfix:
		if baseFamily == branch.FamilyMain || baseFamily == branch.FamilyRelease || baseFamily == branch.FamilySupport {
			return nil
		}
		return invalidBase(
			base.String(),
			"a hotfix must start from the active line that contains the defect",
			"origin/main, origin/release/<semver>, or origin/support/<major.minor>",
		)
	case branch.FamilyScratch:
		if baseFamily.IsOfficialWorkingBranch() {
			return nil
		}
		return invalidBase(
			base.String(),
			"a scratch branch must start from its official ticket branch",
			"origin/feature/<ticket>-<slug> or another official ticket branch",
		)
	case branch.FamilyFix:
		switch baseFamily {
		case branch.FamilyMain, branch.FamilyDevelop, branch.FamilyRelease, branch.FamilySupport:
			return nil
		default:
			return invalidBase(
				base.String(),
				"a workflow-managed fix must start from an active shared line",
				"origin/develop, origin/main, origin/release/<semver>, or origin/support/<major.minor>",
			)
		}
	case branch.FamilyDocs, branch.FamilyChore:
		if baseFamily == branch.FamilyRelease {
			return nil
		}
		return invalidBase(
			base.String(),
			"release documentation and release-preparation work must start from the frozen release line",
			"origin/release/<semver>",
		)
	default:
		return nil
	}
}

func normalizeRepository(repository port.RepositoryIdentity) (port.RepositoryIdentity, error) {
	if repository.Root == "" {
		return port.RepositoryIdentity{}, problem.New(problem.Details{
			Code:        problem.CodeRepositoryNotFound,
			Category:    problem.CategoryRepository,
			Field:       "repository",
			Expected:    "a discovered local Git repository",
			Rule:        "branch operations require a repository root",
			Example:     "C:\\work\\repository",
			Remediation: "run from a Git repository or pass --repo",
		})
	}
	if repository.Remote == "" {
		repository.Remote = "origin"
	}
	return repository, nil
}

func specialWorkflowRequired(family branch.Family) error {
	return problem.New(problem.Details{
		Code:        problem.CodeBranchFamilyInvalid,
		Category:    problem.CategoryGovernance,
		Field:       "branch family",
		Actual:      family.String(),
		Expected:    "a branch family supported by the normal creation command",
		Rule:        family.String() + " requires its dedicated workflow",
		Example:     "workflow " + family.String() + " ...",
		Remediation: "use the dedicated workflow so its required base and safety checks are applied",
	})
}

func invalidBranchInput(rule string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeBranchNameInvalid,
		Category:    problem.CategoryGovernance,
		Field:       "branch",
		Expected:    "a complete canonical branch request",
		Rule:        rule,
		Example:     "feature/ABC-123-add-export-button",
		Remediation: "select a supported family and provide all required branch components",
	})
}

func invalidBase(actual, rule, expected string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeBranchBaseInvalid,
		Category:    problem.CategoryGovernance,
		Field:       "target base",
		Actual:      actual,
		Expected:    expected,
		Rule:        rule,
		Example:     "origin/develop",
		Remediation: "select the canonical base required by the branch family",
	})
}

func unavailableTargetBase(base branch.TargetBase) error {
	return problem.New(problem.Details{
		Code:        problem.CodeBranchBaseInvalid,
		Category:    problem.CategoryRepository,
		Field:       "target base",
		Actual:      base.String(),
		Expected:    "an existing fetched remote-tracking branch",
		Rule:        "branch creation requires the selected target base to exist after fetch",
		Example:     "origin/develop",
		Remediation: "create or fetch the required remote branch, select the correct --remote, or use the workflow for the intended base",
	})
}

func contextError(ctx context.Context) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	return problem.Wrap(problem.Details{
		Code:        problem.CodeOperationCancelled,
		Category:    problem.CategoryCancelled,
		Field:       "branch operation",
		Expected:    "an active context",
		Rule:        "branch operations stop when the caller cancels their context",
		Remediation: "retry with an active context",
	}, ctx.Err())
}

func (step PlanStep) String() string {
	return fmt.Sprintf("%s: %s", step.Action, step.Detail)
}
