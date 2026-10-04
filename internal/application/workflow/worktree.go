package workflow

import (
	"context"
	"path/filepath"
	"strconv"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/application/ticketalloc"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

// WorktreeService owns the task-worktree lifecycle use cases: the governed
// acquisition of a detached task worktree, its inventory, its removal after
// the ticket work is published, and the evidence-based prune of completed
// worktrees.
type WorktreeService struct {
	git port.GitRepository
	// pullRequests carries the provider inventory capability whose records
	// prove the review outcome of a ticket. The prune fails closed without
	// it: completion is never decided from the local plane alone.
	pullRequests port.PullRequestInventoryLister
}

// NewWorktreeService creates the worktree workflow service.
func NewWorktreeService(git port.GitRepository) *WorktreeService {
	return &WorktreeService{git: git}
}

// WithPullRequestInventory binds the provider capability whose pull-request
// records carry the review-outcome evidence of the worktree prune.
func (service *WorktreeService) WithPullRequestInventory(lister port.PullRequestInventoryLister) *WorktreeService {
	service.pullRequests = lister
	return service
}

// StartWorktreeRequest describes the acquisition of one detached task
// worktree for one ticket.
type StartWorktreeRequest struct {
	Repository port.RepositoryIdentity
	Ticket     ticket.ID
	// BaseLine carries the protected base line of the lane whose work the
	// worktree hosts: main, release/<semver>, or support/<major.minor> for
	// hotfix and stabilization lanes. An empty value acquires from the
	// develop integration line of regular ticket work.
	BaseLine string
	DryRun   bool
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
	baseName, err := worktreeAcquisitionBase(request.BaseLine)
	if err != nil {
		return StartWorktreeResult{}, err
	}
	base, err := branch.NewTargetBase(repository.Remote, baseName)
	if err != nil {
		return StartWorktreeResult{}, err
	}
	path := worktreePath(repository.Root, request.Ticket)
	entries, err := manager.WorktreeList(ctx, repository)
	if err != nil {
		return StartWorktreeResult{}, err
	}
	if registeredWorktree(entries, path) {
		return StartWorktreeResult{}, worktreeConflict(path)
	}
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

// Staleness classes of the prune inventory. The names follow the worktree
// lifecycle convention: every task worktree is in exactly one class, and
// only the stale-eligible class is a removal candidate.
const (
	pruneClassActive        = "active"
	pruneClassPreStart      = "pre-start"
	pruneClassPublishedOpen = "published-open"
	pruneClassStaleEligible = "stale-eligible"
	pruneClassAbandoned     = "abandoned"
	pruneClassOutOfScope    = "out-of-scope"
)

// PruneWorktreesRequest identifies the repository whose completed task
// worktrees are pruned.
type PruneWorktreesRequest struct {
	Repository port.RepositoryIdentity
	DryRun     bool
}

// WorktreePruneEntry reports one worktree of the prune inventory with its
// staleness classification and its removal outcome. The class follows the
// worktree lifecycle convention; only a stale-eligible entry is a removal
// candidate, and the reason carries the evidence basis of the decision.
type WorktreePruneEntry struct {
	Path     string `json:"path"`
	Ticket   string `json:"ticket,omitempty"`
	Branch   string `json:"branch,omitempty"`
	Class    string `json:"class"`
	PRState  string `json:"pullRequestState,omitempty"`
	PRNumber string `json:"pullRequestNumber,omitempty"`
	Reason   string `json:"reason"`
	Removed  bool   `json:"removed"`
}

// PruneWorktreesResult reports the classified worktree inventory and the
// executed removals of one prune dispatch.
type PruneWorktreesResult struct {
	Entries []WorktreePruneEntry
	DryRun  bool
	Plan    []branchapp.PlanStep
}

// PruneWorktrees discovers the task worktrees whose completion evidence is
// proven and removes them under the same registered-and-clean guards as the
// single-ticket removal. Completion is a hybrid conjunction measured per
// worktree: the newest pull-request record of the ticket is merged, the
// branch-cleanup obligation is satisfied on the remote branch surface, the
// worktree is locally clean, and no governed operation holds it. Every other
// staleness class stays in place with its named reason; the prune never
// removes a worktree on a pure-remote or a pure-local decision.
func (service *WorktreeService) PruneWorktrees(ctx context.Context, request PruneWorktreesRequest) (PruneWorktreesResult, error) {
	manager, err := service.worktreeManager()
	if err != nil {
		return PruneWorktreesResult{}, err
	}
	remoteBranches, ok := service.git.(port.RemoteBranchLister)
	if !ok {
		return PruneWorktreesResult{}, worktreeCapabilityProblem("remote branch listing")
	}
	if service.pullRequests == nil {
		return PruneWorktreesResult{}, worktreeCapabilityProblem("pull-request state evidence")
	}
	repository := normalizeRepositoryIdentity(request.Repository)
	entries, err := manager.WorktreeList(ctx, repository)
	if err != nil {
		return PruneWorktreesResult{}, err
	}
	remoteURL, err := service.git.RemoteURL(ctx, repository)
	if err != nil {
		return PruneWorktreesResult{}, err
	}
	inventory, err := service.pullRequests.ListPullRequests(ctx, port.PullRequestInventoryQuery{
		Repository: repository,
		RemoteURL:  remoteURL,
	})
	if err != nil {
		return PruneWorktreesResult{}, err
	}
	remoteRefs, err := remoteBranches.RemoteBranches(ctx, repository)
	if err != nil {
		return PruneWorktreesResult{}, err
	}
	result := PruneWorktreesResult{DryRun: request.DryRun}
	for _, entry := range entries {
		classified := service.classifyWorktree(ctx, repository, remoteRefs, inventory, entry)
		if classified.Class == pruneClassStaleEligible {
			result.Plan = append(result.Plan, branchapp.PlanStep{
				Action: "worktree-remove",
				Detail: "git worktree remove " + entry.Path,
			})
			if !request.DryRun {
				if err := manager.WorktreeRemove(ctx, repository, entry.Path); err != nil {
					classified.Reason = "removal refused: " + err.Error()
				} else {
					classified.Removed = true
					classified.Reason = "removed under the registered-and-clean guards"
				}
			}
		}
		result.Entries = append(result.Entries, classified)
	}
	return result, nil
}

// classifyWorktree measures one worktree entry against the hybrid completion
// evidence and binds exactly one staleness class with its evidence basis.
func (service *WorktreeService) classifyWorktree(
	ctx context.Context,
	repository port.RepositoryIdentity,
	remoteRefs []branch.BranchName,
	inventory []port.PullRequestSummary,
	entry port.WorktreeEntry,
) WorktreePruneEntry {
	classified := WorktreePruneEntry{Path: entry.Path, Branch: entry.Branch}
	if entry.Bare {
		classified.Class = pruneClassOutOfScope
		classified.Reason = "bare worktree without a working tree"
		return classified
	}
	id, isTask := ticketalloc.TicketFromWorktreePath(entry.Path)
	if !isTask {
		classified.Class = pruneClassOutOfScope
		classified.Reason = "the path does not match the task-worktree registry grammar"
		return classified
	}
	classified.Ticket = id.String()
	if entry.Detached {
		clean, err := service.git.IsWorktreeClean(ctx, worktreeIdentity(repository, entry.Path))
		switch {
		case err != nil:
			classified.Class = pruneClassActive
			classified.Reason = "the local evidence of the worktree is unmeasurable: " + err.Error()
		case clean:
			classified.Class = pruneClassPreStart
			classified.Reason = "fresh detached acquisition; the sanctioned first mutation is the governed ticket start"
		default:
			classified.Class = pruneClassActive
			classified.Reason = "the detached worktree carries uncommitted state"
		}
		return classified
	}
	newest, found := newestPullRequestForTicket(inventory, id)
	if !found {
		classified.Class = pruneClassActive
		classified.Reason = "no pull request records the review outcome of the ticket"
		return classified
	}
	classified.PRState = string(newest.State)
	classified.PRNumber = newest.Number
	switch newest.State {
	case port.PullRequestStateOpen:
		classified.Class = pruneClassPublishedOpen
		classified.Reason = "the pull request of the ticket is open; the worktree is the review continuation context"
		return classified
	case port.PullRequestStateClosed:
		classified.Class = pruneClassAbandoned
		classified.Reason = "the newest pull request of the ticket is closed without a merge; the boundary decision belongs to the actor"
		return classified
	}
	if branchOnRemote(remoteRefs, entry.Branch) {
		classified.Class = pruneClassActive
		classified.Reason = "the branch-cleanup obligation is open: the ticket branch still exists on the remote surface"
		return classified
	}
	clean, err := service.git.IsWorktreeClean(ctx, worktreeIdentity(repository, entry.Path))
	if err != nil {
		classified.Class = pruneClassActive
		classified.Reason = "the local evidence of the worktree is unmeasurable: " + err.Error()
		return classified
	}
	if !clean {
		classified.Class = pruneClassActive
		classified.Reason = "the worktree carries uncommitted state"
		return classified
	}
	name, active, err := service.git.ActiveOperation(ctx, worktreeIdentity(repository, entry.Path))
	if err != nil {
		classified.Class = pruneClassActive
		classified.Reason = "the local evidence of the worktree is unmeasurable: " + err.Error()
		return classified
	}
	if active {
		classified.Class = pruneClassActive
		classified.Reason = "the governed operation " + name + " holds the worktree"
		return classified
	}
	classified.Class = pruneClassStaleEligible
	classified.Reason = "completion evidence proven: merged pull request, deleted remote branch, clean worktree, no active operation"
	return classified
}

// newestPullRequestForTicket resolves the newest pull-request record of one
// ticket from the complete provider surface. Records are matched through the
// canonical title grammars, never through string containment; the highest
// parsed record number carries the current review state of the ticket.
func newestPullRequestForTicket(inventory []port.PullRequestSummary, id ticket.ID) (port.PullRequestSummary, bool) {
	newest := port.PullRequestSummary{}
	found := false
	for _, summary := range inventory {
		recordID, matches := ticketalloc.TicketFromTitle(summary.Title)
		if !matches || recordID.String() != id.String() {
			continue
		}
		if !found || pullRequestNumberNewer(summary.Number, newest.Number) {
			newest = summary
			found = true
		}
	}
	return newest, found
}

// pullRequestNumberNewer compares two provider record numbers numerically; a
// number that does not parse sorts below every parseable number.
func pullRequestNumberNewer(candidate string, current string) bool {
	candidateValue, candidateErr := strconv.Atoi(candidate)
	currentValue, currentErr := strconv.Atoi(current)
	if candidateErr != nil {
		return false
	}
	if currentErr != nil {
		return true
	}
	return candidateValue > currentValue
}

// branchOnRemote reports whether one canonical branch name still exists on
// the fetched remote-tracking branch surface.
func branchOnRemote(remoteRefs []branch.BranchName, name string) bool {
	for _, ref := range remoteRefs {
		if ref.String() == name {
			return true
		}
	}
	return false
}

// worktreeIdentity binds one worktree path into a repository identity so the
// per-worktree cleanliness and operation measurements run inside the target
// worktree.
func worktreeIdentity(repository port.RepositoryIdentity, path string) port.RepositoryIdentity {
	return port.RepositoryIdentity{Root: path, Remote: repository.Remote}
}

// worktreeCapabilityProblem is the fail-closed record of a composition whose
// adapter surface cannot measure the completion evidence of the prune.
func worktreeCapabilityProblem(capability string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeConfigurationUnavailable,
		Category:    problem.CategoryConfig,
		Field:       "worktree lifecycle",
		Actual:      capability,
		Expected:    "the evidence capabilities of the worktree prune",
		Rule:        "the prune measures the hybrid completion evidence per worktree and fails closed when one capability is missing",
		Remediation: "fix the composition so the worktree workflow service receives the " + capability + " capability",
	})
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

// worktreeAcquisitionBase resolves the acquisition base of the lane whose
// work the worktree hosts. An empty base line is the regular ticket lane and
// acquires from the develop integration line; an explicit base must be the
// develop line itself or a protected line (main, release, support) per the
// lane-to-base mapping convention.
func worktreeAcquisitionBase(baseLine string) (branch.BranchName, error) {
	if baseLine == "" {
		return mustDevelop(), nil
	}
	parsed, err := branch.ParseName(baseLine)
	if err != nil {
		return branch.BranchName{}, invalidWorkflowInput(
			"the worktree acquisition base must be a canonical branch name",
			"provide the base line of the work lane: develop, main, release/<semver>, or support/<major.minor>",
		)
	}
	switch parsed.Family() {
	case branch.FamilyDevelop, branch.FamilyMain, branch.FamilyRelease, branch.FamilySupport:
		return parsed, nil
	default:
		return branch.BranchName{}, problem.New(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryGovernance,
			Field:       "base",
			Actual:      baseLine,
			Expected:    "the base line of a governed acquisition lane",
			Rule:        "the acquisition base follows the lane-to-base mapping: develop for regular ticket work, the protected base line for hotfix work, the frozen release line for stabilization work",
			Example:     "--base main",
			Remediation: "acquire from the lane's own base line",
		})
	}
}

// worktreeConflict is the fail-closed record of an occupied task-worktree
// path: the derived worktree of the ticket is already registered, so the
// acquisition refuses to mutate instead of failing with an opaque Git
// diagnostic.
func worktreeConflict(path string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeWorktreeConflict,
		Category:    problem.CategoryRepository,
		Field:       "worktree",
		Actual:      path,
		Expected:    "an unoccupied task-worktree path",
		Rule:        "task worktrees are acquired once per ticket; the derived worktree of the ticket already exists",
		Example:     "workflow worktree list",
		Remediation: "continue in the existing worktree by running the ticket workflow inside it, or inventory the worktrees with workflow worktree list",
	})
}

// worktreeBaseDrift is the fail-closed record of a drifted detached pre-start
// worktree: its HEAD no longer resolves to the acquired base revision, so the
// pre-start form would hide an ungoverned detached commit.
func worktreeBaseDrift(head string, baseRevision string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeWorktreeBaseDrift,
		Category:    problem.CategoryRepository,
		Field:       "worktree",
		Actual:      "HEAD " + head,
		Expected:    "the acquired base revision " + baseRevision,
		Rule:        "the detached pre-start form is legitimate only at its acquired base revision; a drifted HEAD hides an ungoverned detached commit",
		Example:     "workflow worktree start",
		Remediation: "resolve the detached commit, then re-acquire a fresh worktree with workflow worktree start",
	})
}

// requireTaskWorktree enforces the task-binding law at ticket start: ticket
// work happens inside a linked task worktree. A fresh detached task worktree
// is the legitimate pre-start form and must be clean; a checked-out branch
// inside the worktree is the continuing form. Compositions whose Git adapter
// carries no worktree capability keep the unenforced legacy behavior; the
// shipped composition always carries the capability.
func (service *TicketService) requireTaskWorktree(ctx context.Context, repository port.RepositoryIdentity) error {
	manager, ok := service.git.(port.WorktreeManager)
	if !ok {
		return nil
	}
	linked, err := manager.LinkedWorktree(ctx, repository)
	if err != nil {
		return err
	}
	if !linked {
		return taskWorktreeRequired()
	}
	if _, err := service.git.CurrentBranch(ctx, repository); err == nil {
		return nil
	} else if classified, ok := problem.As(err); !ok || classified.Code != problem.CodeBranchNameInvalid {
		return err
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
	// The detached pre-start form is legitimate only at its acquired base
	// revision. A HEAD that drifted between the worktree acquisition and the
	// ticket start hides an ungoverned detached commit behind the clean
	// state, so the guard re-measures both revisions fail-closed.
	// mustDevelop and the upstream-validated remote cannot fail the base
	// construction.
	base, _ := branch.NewTargetBase(repository.Remote, mustDevelop())
	head, baseRevision, err := manager.WorktreeHeadMatchesBase(ctx, repository, base)
	if err != nil {
		return err
	}
	if head != baseRevision {
		return worktreeBaseDrift(head, baseRevision)
	}
	return nil
}

// taskWorktreeRequired is the fail-closed record for ticket work outside a
// linked task worktree.
func taskWorktreeRequired() error {
	return problem.New(problem.Details{
		Code:        problem.CodeWorktreeRequired,
		Category:    problem.CategoryGovernance,
		Field:       "worktree",
		Expected:    "a linked task worktree bound to the ticket",
		Rule:        "the task-binding law hosts every ticket work inside its task worktree; the primary checkout never hosts ticket branch work",
		Example:     "workflow worktree start",
		Remediation: "acquire a task worktree with workflow worktree start, then run ticket start inside it",
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
