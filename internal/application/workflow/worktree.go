package workflow

import (
	"context"
	"path/filepath"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

// WorktreeService owns the task-worktree lifecycle use cases: the governed
// acquisition of a detached task worktree, its inventory, and its removal
// after the ticket work is published.
type WorktreeService struct {
	git port.GitRepository
}

// NewWorktreeService creates the worktree workflow service.
func NewWorktreeService(git port.GitRepository) *WorktreeService {
	return &WorktreeService{git: git}
}

// StartWorktreeRequest describes the acquisition of one detached task
// worktree for one ticket.
type StartWorktreeRequest struct {
	Repository port.RepositoryIdentity
	Ticket     ticket.ID
	DryRun     bool
}

// StartWorktreeResult reports the acquired worktree path and the remote base
// it was created from.
type StartWorktreeResult struct {
	Path   string
	Base   branch.TargetBase
	Ticket ticket.ID
	DryRun bool
	Plan   []branchapp.PlanStep
}

// StartWorktree acquires a detached task worktree for one ticket from the
// current revision of the remote develop integration line. The acquisition
// runs from the primary checkout; the worktree stays detached, and the
// sanctioned first mutation inside it remains the governed ticket-start
// workflow.
func (service *WorktreeService) StartWorktree(ctx context.Context, request StartWorktreeRequest) (StartWorktreeResult, error) {
	manager, err := service.worktreeManager()
	if err != nil {
		return StartWorktreeResult{}, err
	}
	if request.Ticket.IsZero() {
		return StartWorktreeResult{}, invalidWorkflowInput(
			"worktree acquisition requires a ticket",
			"provide the ticket key and number of the task",
		)
	}
	repository := normalizeRepositoryIdentity(request.Repository)
	linked, err := manager.LinkedWorktree(ctx, repository)
	if err != nil {
		return StartWorktreeResult{}, err
	}
	if linked {
		return StartWorktreeResult{}, problem.New(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryGovernance,
			Field:       "worktree",
			Actual:      repository.Root,
			Expected:    "the primary checkout of the repository",
			Rule:        "task worktrees are acquired from the primary checkout, never nested inside another worktree",
			Remediation: "run the acquisition from the primary checkout",
		})
	}
	base, err := branch.NewTargetBase(repository.Remote, mustDevelop())
	if err != nil {
		return StartWorktreeResult{}, err
	}
	path := worktreePath(repository.Root, request.Ticket)
	result := StartWorktreeResult{
		Path:   path,
		Base:   base,
		Ticket: request.Ticket,
		DryRun: request.DryRun,
		Plan: []branchapp.PlanStep{
			{Action: "fetch", Detail: "git fetch --prune " + repository.Remote},
			{Action: "worktree-add", Detail: "git worktree add --detach " + path + " " + base.String()},
		},
	}
	if request.DryRun {
		return result, nil
	}
	if err := manager.WorktreeAddDetached(ctx, repository, path, base); err != nil {
		return StartWorktreeResult{}, err
	}
	return result, nil
}

// ListWorktreesRequest identifies the repository whose worktrees are
// inventoried.
type ListWorktreesRequest struct {
	Repository port.RepositoryIdentity
}

// ListWorktreesResult reports every worktree and the checkout the inventory
// ran in.
type ListWorktreesResult struct {
	Worktrees []port.WorktreeEntry
	Current   string
}

// ListWorktrees inventories every local worktree of the repository.
func (service *WorktreeService) ListWorktrees(ctx context.Context, request ListWorktreesRequest) (ListWorktreesResult, error) {
	manager, err := service.worktreeManager()
	if err != nil {
		return ListWorktreesResult{}, err
	}
	repository := normalizeRepositoryIdentity(request.Repository)
	entries, err := manager.WorktreeList(ctx, repository)
	if err != nil {
		return ListWorktreesResult{}, err
	}
	return ListWorktreesResult{
		Worktrees: entries,
		Current:   filepath.Clean(repository.Root),
	}, nil
}

// RemoveWorktreeRequest identifies the completed ticket whose task worktree
// is removed.
type RemoveWorktreeRequest struct {
	Repository port.RepositoryIdentity
	Ticket     ticket.ID
	DryRun     bool
}

// RemoveWorktreeResult reports the removed worktree path.
type RemoveWorktreeResult struct {
	Path   string
	Ticket ticket.ID
	DryRun bool
	Plan   []branchapp.PlanStep
}

// RemoveWorktree removes the task worktree of a completed ticket. The removal
// fails closed: the target must be a registered task worktree and its working
// tree must be clean. Official and remote branches are never touched.
func (service *WorktreeService) RemoveWorktree(ctx context.Context, request RemoveWorktreeRequest) (RemoveWorktreeResult, error) {
	manager, err := service.worktreeManager()
	if err != nil {
		return RemoveWorktreeResult{}, err
	}
	if request.Ticket.IsZero() {
		return RemoveWorktreeResult{}, invalidWorkflowInput(
			"worktree removal requires a ticket",
			"provide the ticket key and number of the completed task",
		)
	}
	repository := normalizeRepositoryIdentity(request.Repository)
	path := worktreePath(repository.Root, request.Ticket)
	entries, err := manager.WorktreeList(ctx, repository)
	if err != nil {
		return RemoveWorktreeResult{}, err
	}
	if !registeredWorktree(entries, path) {
		return RemoveWorktreeResult{}, problem.New(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryRepository,
			Field:       "worktree",
			Actual:      path,
			Expected:    "a registered task worktree",
			Rule:        "worktree removal only removes the registered task worktree of the requested ticket",
			Example:     path,
			Remediation: "verify the ticket or list the existing worktrees with workflow worktree list",
		})
	}
	result := RemoveWorktreeResult{
		Path:   path,
		Ticket: request.Ticket,
		DryRun: request.DryRun,
		Plan: []branchapp.PlanStep{
			{Action: "worktree-remove", Detail: "git worktree remove " + path},
		},
	}
	if request.DryRun {
		return result, nil
	}
	if err := manager.WorktreeRemove(ctx, repository, path); err != nil {
		return RemoveWorktreeResult{}, err
	}
	return result, nil
}

// worktreeManager resolves the optional worktree capability of the composed
// Git adapter. A composition without the capability cannot run any worktree
// lifecycle use case.
func (service *WorktreeService) worktreeManager() (port.WorktreeManager, error) {
	if service.git == nil {
		return nil, internalDependencyError("worktree workflow services")
	}
	manager, ok := service.git.(port.WorktreeManager)
	if !ok {
		return nil, internalDependencyError("worktree manager")
	}
	return manager, nil
}

// registeredWorktree reports whether the derived task-worktree path is part
// of the porcelain worktree inventory.
func registeredWorktree(entries []port.WorktreeEntry, path string) bool {
	cleaned := filepath.Clean(path)
	for _, entry := range entries {
		if filepath.Clean(entry.Path) == cleaned {
			return true
		}
	}
	return false
}

// worktreePath derives the canonical task-worktree directory of one ticket
// from the repository root: a sibling directory named after the repository
// and the ticket.
func worktreePath(root string, id ticket.ID) string {
	return filepath.Join(filepath.Dir(root), filepath.Base(root)+"-"+id.String())
}

// acceptDetachedTaskWorktree enforces the branch-context matrix at ticket
// start. A fresh detached task worktree is a legitimate pre-start context for
// every actor when it is a linked worktree with a clean working tree; every
// other detached context stays blocked. Compositions whose Git adapter does
// not offer the worktree capability keep the unchanged behavior.
func (service *TicketService) acceptDetachedTaskWorktree(ctx context.Context, repository port.RepositoryIdentity) error {
	if _, err := service.git.CurrentBranch(ctx, repository); err == nil {
		return nil
	} else if classified, ok := problem.As(err); !ok || classified.Code != problem.CodeBranchNameInvalid {
		return err
	}
	manager, ok := service.git.(port.WorktreeManager)
	if !ok {
		return detachedTaskWorktreeBlocked()
	}
	linked, err := manager.LinkedWorktree(ctx, repository)
	if err != nil {
		return err
	}
	if !linked {
		return detachedTaskWorktreeBlocked()
	}
	clean, err := service.git.IsWorktreeClean(ctx, repository)
	if err != nil {
		return err
	}
	if !clean {
		return problem.New(problem.Details{
			Code:        problem.CodeWorktreeNotClean,
			Category:    problem.CategoryRepository,
			Field:       "worktree",
			Expected:    "a clean task worktree before the ticket workflow starts",
			Rule:        "the detached task worktree is a pre-start context only in its fresh, clean form",
			Example:     "git status --porcelain returns no entries",
			Remediation: "clean the worktree before starting the ticket workflow",
		})
	}
	return nil
}

// detachedTaskWorktreeBlocked is the fail-closed record for a detached HEAD
// that is not a fresh task worktree.
func detachedTaskWorktreeBlocked() error {
	return problem.New(problem.Details{
		Code:        problem.CodeBranchNameInvalid,
		Category:    problem.CategoryRepository,
		Field:       "current branch",
		Expected:    "a checked-out canonical branch or a fresh detached task worktree",
		Rule:        "a detached HEAD is a valid ticket-start context only inside a fresh task worktree",
		Example:     "workflow worktree start",
		Remediation: "acquire a task worktree with workflow worktree start or switch to a canonical branch",
	})
}

// normalizeRepositoryIdentity applies the shared remote and root defaults of
// the workflow package to one repository identity.
func normalizeRepositoryIdentity(repository port.RepositoryIdentity) port.RepositoryIdentity {
	if repository.Remote == "" {
		repository.Remote = "origin"
	}
	return repository
}
