package workflow

import (
	"context"
	"fmt"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// Branch-hygiene staleness classes. The two eligible classes carry the
// branch-hygiene convention's evidence conjunction; every other class keeps
// the branch in place with its named reason, and the single-branch removal
// refuses fail-closed.
const (
	hygieneClassWorkedToCompletion = "worked-to-completion"
	hygieneClassNeverPublished     = "never-published"
	hygieneClassPublishedOpen      = "published-open"
	hygieneClassAbandoned          = "abandoned"
	hygieneClassObligationOpen     = "obligation-open"
	hygieneClassCheckedOut         = "checked-out"
	hygieneClassUnpublishedWork    = "unpublished-work"
	hygieneClassActive             = "active"
	hygieneClassOutOfScope         = "out-of-scope"
)

// BranchHygieneService owns the governed branch-hygiene use cases: the
// controlled deletion of one completed local working branch and the
// evidence-based prune pass over the local branch surface. Deletion is
// actor-invoked, evidence-proven, and never automatic; the allocation
// inventory stays a pure read surface the hygiene only consumes.
type BranchHygieneService struct {
	git port.GitRepository
	// pullRequests carries the provider inventory capability whose records
	// prove the review outcome of a ticket. The hygiene fails closed without
	// it: completion is never decided from the local plane alone.
	pullRequests port.PullRequestInventoryLister
}

// NewBranchHygieneService creates the branch-hygiene workflow service.
func NewBranchHygieneService(git port.GitRepository) *BranchHygieneService {
	return &BranchHygieneService{git: git}
}

// WithPullRequestInventory binds the provider capability whose pull-request
// records carry the review-outcome evidence of the branch hygiene.
func (service *BranchHygieneService) WithPullRequestInventory(lister port.PullRequestInventoryLister) *BranchHygieneService {
	service.pullRequests = lister
	return service
}

// RemoveBranchRequest identifies one local official working branch whose
// controlled deletion is requested.
type RemoveBranchRequest struct {
	Repository port.RepositoryIdentity
	Branch     branch.BranchName
	DryRun     bool
}

// RemoveBranchResult reports the classified branch and its removal outcome.
type RemoveBranchResult struct {
	Branch   string
	Ticket   string
	Class    string
	PRState  string
	PRNumber string
	Ahead    int
	Base     string
	Reason   string
	DryRun   bool
	Plan     []branchapp.PlanStep
	Removed  bool
}

// RemoveBranch deletes one local official working branch under the
// branch-hygiene evidence. The removal fails closed on every refusal class:
// an open pull request, an abandoned boundary decision, an open remote
// branch-cleanup obligation, an active checkout, and unpublished work are
// never deleted. The guard matrix proves the null-loss state before the
// reference is deleted; the deletion runs forced because Git's own merge
// check compares against the current checkout state, not against the measured
// lane base the guards evaluated.
func (service *BranchHygieneService) RemoveBranch(ctx context.Context, request RemoveBranchRequest) (RemoveBranchResult, error) {
	repository := normalizeRepositoryIdentity(request.Repository)
	evidence, err := service.collectEvidence(ctx, repository)
	if err != nil {
		return RemoveBranchResult{}, err
	}
	entry := service.classifyBranch(ctx, evidence, repository, request.Branch)
	if entry.Class != hygieneClassWorkedToCompletion && entry.Class != hygieneClassNeverPublished {
		return RemoveBranchResult{}, branchHygieneRefusal(entry)
	}
	result := RemoveBranchResult{
		Branch:   entry.Branch,
		Ticket:   entry.Ticket,
		Class:    entry.Class,
		PRState:  entry.PRState,
		PRNumber: entry.PRNumber,
		Ahead:    entry.Ahead,
		Base:     entry.Base,
		Reason:   entry.Reason,
		DryRun:   request.DryRun,
		Plan: []branchapp.PlanStep{
			{Action: "branch-delete", Detail: "git branch -D " + entry.Branch},
		},
	}
	if request.DryRun {
		return result, nil
	}
	if err := service.git.DeleteLocalBranch(ctx, repository, request.Branch, true); err != nil {
		return RemoveBranchResult{}, err
	}
	result.Removed = true
	return result, nil
}

// PruneBranchesRequest identifies the repository whose local branch surface
// is pruned.
type PruneBranchesRequest struct {
	Repository port.RepositoryIdentity
	DryRun     bool
}

// BranchHygieneEntry reports one local branch of the prune inventory with its
// staleness classification and its removal outcome. Only the two
// stale-eligible classes are removal candidates, and the reason carries the
// evidence basis of the decision.
type BranchHygieneEntry struct {
	Branch   string `json:"branch"`
	Ticket   string `json:"ticket,omitempty"`
	Class    string `json:"class"`
	PRState  string `json:"pullRequestState,omitempty"`
	PRNumber string `json:"pullRequestNumber,omitempty"`
	Ahead    int    `json:"ahead,omitempty"`
	Base     string `json:"base,omitempty"`
	Reason   string `json:"reason"`
	Removed  bool   `json:"removed"`
}

// PruneBranchesResult reports the classified local branch inventory and the
// executed removals of one prune dispatch.
type PruneBranchesResult struct {
	Entries []BranchHygieneEntry
	DryRun  bool
	Plan    []branchapp.PlanStep
}

// PruneBranches discovers the local official working branches whose
// branch-hygiene evidence is proven and deletes them under the same guard
// matrix as the single-branch removal. Every local branch is classified;
// shared lines, scratch branches, and every refusal class stay in place with
// their named reason. The prune never deletes a branch on a pure-local or a
// pure-remote decision.
func (service *BranchHygieneService) PruneBranches(ctx context.Context, request PruneBranchesRequest) (PruneBranchesResult, error) {
	if service.git == nil {
		return PruneBranchesResult{}, internalDependencyError("branch hygiene services")
	}
	locals, ok := service.git.(port.LocalBranchLister)
	if !ok {
		return PruneBranchesResult{}, branchHygieneCapabilityProblem("local branch listing")
	}
	repository := normalizeRepositoryIdentity(request.Repository)
	evidence, err := service.collectEvidence(ctx, repository)
	if err != nil {
		return PruneBranchesResult{}, err
	}
	names, err := locals.LocalBranches(ctx, repository)
	if err != nil {
		return PruneBranchesResult{}, err
	}
	result := PruneBranchesResult{DryRun: request.DryRun}
	for _, name := range names {
		entry := service.classifyBranch(ctx, evidence, repository, name)
		if entry.Class == hygieneClassWorkedToCompletion || entry.Class == hygieneClassNeverPublished {
			result.Plan = append(result.Plan, branchapp.PlanStep{
				Action: "branch-delete",
				Detail: "git branch -D " + entry.Branch,
			})
			if !request.DryRun {
				if err := service.git.DeleteLocalBranch(ctx, repository, name, true); err != nil {
					entry.Reason = "removal refused: " + err.Error()
				} else {
					entry.Removed = true
					entry.Reason = "removed under the branch-hygiene guard matrix"
				}
			}
		}
		result.Entries = append(result.Entries, entry)
	}
	return result, nil
}

// branchHygieneEvidence carries the surfaces one hygiene dispatch measures:
// the worktree inventory, the fetched remote-tracking branch surface, the
// complete pull-request inventory, and the ahead-measurement capability.
type branchHygieneEvidence struct {
	worktrees    []port.WorktreeEntry
	remotes      []branch.BranchName
	pullRequests []port.PullRequestSummary
	ahead        port.BranchAheadCounter
}

// collectEvidence measures the shared evidence surfaces of one hygiene
// dispatch and fails closed when one Git-transport capability is missing.
func (service *BranchHygieneService) collectEvidence(ctx context.Context, repository port.RepositoryIdentity) (branchHygieneEvidence, error) {
	if service.git == nil {
		return branchHygieneEvidence{}, internalDependencyError("branch hygiene services")
	}
	if service.pullRequests == nil {
		return branchHygieneEvidence{}, branchHygieneCapabilityProblem("pull-request state evidence")
	}
	remotes, ok := service.git.(port.RemoteBranchLister)
	if !ok {
		return branchHygieneEvidence{}, branchHygieneCapabilityProblem("remote branch listing")
	}
	worktreeLister, ok := service.git.(port.WorktreeInventoryLister)
	if !ok {
		return branchHygieneEvidence{}, branchHygieneCapabilityProblem("worktree inventory listing")
	}
	ahead, ok := service.git.(port.BranchAheadCounter)
	if !ok {
		return branchHygieneEvidence{}, branchHygieneCapabilityProblem("branch ahead measurement")
	}
	remoteURL, err := service.git.RemoteURL(ctx, repository)
	if err != nil {
		return branchHygieneEvidence{}, err
	}
	inventory, err := service.pullRequests.ListPullRequests(ctx, port.PullRequestInventoryQuery{
		Repository: repository,
		RemoteURL:  remoteURL,
	})
	if err != nil {
		return branchHygieneEvidence{}, err
	}
	remoteRefs, err := remotes.RemoteBranches(ctx, repository)
	if err != nil {
		return branchHygieneEvidence{}, err
	}
	worktrees, err := worktreeLister.WorktreeList(ctx, repository)
	if err != nil {
		return branchHygieneEvidence{}, err
	}
	return branchHygieneEvidence{
		worktrees:    worktrees,
		remotes:      remoteRefs,
		pullRequests: inventory,
		ahead:        ahead,
	}, nil
}

// classifyBranch measures one local branch against the branch-hygiene
// evidence and binds exactly one staleness class with its evidence basis.
// Per-branch measurement failures degrade to the active class and keep the
// branch in place; shared evidence failures fail the whole dispatch closed.
func (service *BranchHygieneService) classifyBranch(ctx context.Context, evidence branchHygieneEvidence, repository port.RepositoryIdentity, name branch.BranchName) BranchHygieneEntry {
	entry := BranchHygieneEntry{Branch: name.String()}
	if !name.Family().IsOfficialWorkingBranch() {
		entry.Class = hygieneClassOutOfScope
		entry.Reason = "only official working branches are hygiene candidates"
		return entry
	}
	id, _ := name.Ticket()
	entry.Ticket = id.String()
	newest, found := newestPullRequestForTicket(evidence.pullRequests, id)
	if found {
		entry.PRState = string(newest.State)
		entry.PRNumber = newest.Number
	}
	switch {
	case found && newest.State == port.PullRequestStateOpen:
		entry.Class = hygieneClassPublishedOpen
		entry.Reason = "the pull request of the ticket is open; the branch is the review continuation context"
		return entry
	case found && newest.State == port.PullRequestStateClosed:
		entry.Class = hygieneClassAbandoned
		entry.Reason = "the newest pull request of the ticket is closed without a merge; the boundary decision belongs to the actor"
		return entry
	}
	onRemote := branchOnRemote(evidence.remotes, name.String())
	if found && newest.State == port.PullRequestStateMerged && onRemote {
		entry.Class = hygieneClassObligationOpen
		entry.Reason = "the branch-cleanup obligation is open: the ticket branch still exists on the remote surface"
		return entry
	}
	if path, checkedOut := port.WorktreeBranchLocation(evidence.worktrees, name); checkedOut {
		entry.Class = hygieneClassCheckedOut
		entry.Reason = "the branch is checked out in " + path + "; an active working context is never deleted"
		return entry
	}
	if found && newest.State == port.PullRequestStateMerged {
		entry.Class = hygieneClassWorkedToCompletion
		entry.Reason = "completion evidence proven: merged pull request, deleted remote branch, no checkout"
		return entry
	}
	if onRemote {
		entry.Class = hygieneClassActive
		entry.Reason = "no pull request records the review outcome of the ticket; the branch still exists on the remote surface"
		return entry
	}
	base, baseFound, err := service.branchAheadBase(ctx, repository, name)
	if err != nil {
		entry.Class = hygieneClassActive
		entry.Reason = "the lane base of the branch is unmeasurable: " + err.Error()
		return entry
	}
	if !baseFound {
		entry.Class = hygieneClassActive
		entry.Reason = "the lane base of the branch is neither recorded nor derivable from its family; the null-ahead evidence is unmeasurable"
		return entry
	}
	entry.Base = base.String()
	ahead, err := evidence.ahead.CountBranchAheadCommits(ctx, repository, name, base)
	if err != nil {
		entry.Class = hygieneClassActive
		entry.Reason = "the ahead evidence of the branch is unmeasurable: " + err.Error()
		return entry
	}
	entry.Ahead = ahead
	if ahead > 0 {
		entry.Class = hygieneClassUnpublishedWork
		entry.Reason = "the branch carries commits its lane base does not; deletion would lose unpublished work"
		return entry
	}
	entry.Class = hygieneClassNeverPublished
	entry.Reason = "never-published hygiene proven: no remote branch, no pull request record, null-ahead against the lane base"
	return entry
}

// branchAheadBase resolves the lane base the null-ahead evidence measures
// against: the recorded workflow base of the branch when one exists, else the
// family's default remote base. A branch with neither stays unmeasurable.
func (service *BranchHygieneService) branchAheadBase(ctx context.Context, repository port.RepositoryIdentity, name branch.BranchName) (branch.TargetBase, bool, error) {
	base, found, err := service.git.WorkflowBase(ctx, repository, name)
	if err != nil || found {
		return base, found, err
	}
	return name.Family().DefaultTargetBase(repository.Remote)
}

// branchHygieneRefusal is the fail-closed record of a single-branch removal
// whose classification is not stale-eligible. The protection classes the
// branch-hygiene convention names carry their own stable codes; every other
// refusal carries its evidence basis in the rule.
func branchHygieneRefusal(entry BranchHygieneEntry) error {
	switch entry.Class {
	case hygieneClassPublishedOpen:
		return problem.New(problem.Details{
			Code:        problem.CodeBranchPullRequestOpen,
			Category:    problem.CategoryGovernance,
			Field:       "branch",
			Actual:      entry.Branch,
			Expected:    "a branch whose newest pull request is merged",
			Rule:        "the branch is the review continuation context of an open pull request",
			Remediation: "complete or close the pull request before the branch hygiene",
		})
	case hygieneClassCheckedOut:
		return problem.New(problem.Details{
			Code:        problem.CodeBranchCheckedOut,
			Category:    problem.CategoryRepository,
			Field:       "branch",
			Actual:      entry.Branch,
			Expected:    "a branch that is not checked out in any worktree",
			Rule:        "an active working context is never deleted",
			Remediation: "finish or detach the checkout that holds the branch, then re-run the branch hygiene",
		})
	case hygieneClassUnpublishedWork:
		return problem.New(problem.Details{
			Code:        problem.CodeBranchAheadUnmerged,
			Category:    problem.CategoryRepository,
			Field:       "branch",
			Actual:      fmt.Sprintf("%d commits ahead of %s", entry.Ahead, entry.Base),
			Expected:    "a branch with no commits its lane base does not carry",
			Rule:        "deletion would lose unpublished work; the loss protection is fail-closed",
			Remediation: "publish the work through the governed ticket workflow or resolve the commits as the actor",
		})
	default:
		return problem.New(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryGovernance,
			Field:       "branch",
			Actual:      entry.Branch,
			Expected:    "a stale-eligible official working branch",
			Rule:        entry.Reason,
			Remediation: branchHygieneRemediation(entry.Class),
		})
	}
}

// branchHygieneRemediation binds the concrete remediation of every refusal
// class the stable codes do not carry.
func branchHygieneRemediation(class string) string {
	switch class {
	case hygieneClassAbandoned:
		return "the boundary decision belongs to the actor; resolve the closed pull request first"
	case hygieneClassObligationOpen:
		return "let the remote branch-cleanup complete, then re-run the branch hygiene"
	case hygieneClassActive:
		return "resolve the unmeasurable evidence, then re-run the branch hygiene"
	default:
		return "the branch hygiene only deletes official working branches"
	}
}

// branchHygieneCapabilityProblem is the fail-closed record of a composition
// whose adapter surface cannot measure the evidence of the branch hygiene.
func branchHygieneCapabilityProblem(capability string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeConfigurationUnavailable,
		Category:    problem.CategoryConfig,
		Field:       "branch hygiene",
		Actual:      capability,
		Expected:    "the evidence capabilities of the branch hygiene",
		Rule:        "the hygiene measures the completion evidence per branch and fails closed when one capability is missing",
		Remediation: "fix the composition so the branch hygiene service receives the " + capability + " capability",
	})
}
