package workflow

import (
	"context"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
)

// IntegrationLineReturnStatus classifies the governed post-publication
// transition of the local workspace back to the develop integration line.
type IntegrationLineReturnStatus string

const (
	// IntegrationLineReturnSwitched reports that the workspace was switched
	// back to the integration line after the pull request was created.
	IntegrationLineReturnSwitched IntegrationLineReturnStatus = "switched"
	// IntegrationLineReturnAlreadyHome reports that the workspace already was
	// on the integration line when the pull request was created.
	IntegrationLineReturnAlreadyHome IntegrationLineReturnStatus = "already-home"
	// IntegrationLineReturnSkippedDirtyWorktree reports that uncommitted
	// changes kept the workspace on the publishing branch; every foreign or
	// uncommitted change is preserved untouched.
	IntegrationLineReturnSkippedDirtyWorktree IntegrationLineReturnStatus = "skipped-dirty-worktree"
	// IntegrationLineReturnFailed reports that the transition failed after the
	// pull request had already been created successfully; the publication
	// itself remains valid.
	IntegrationLineReturnFailed IntegrationLineReturnStatus = "failed"
)

// IntegrationLineRefresh classifies the guarded fast-forward-only refresh of
// the local integration line against its fetched remote-tracking reference
// after the workspace transition.
type IntegrationLineRefresh string

const (
	// IntegrationLineRefreshUpdated reports that the local integration line
	// advanced to its fetched remote-tracking reference.
	IntegrationLineRefreshUpdated IntegrationLineRefresh = "updated"
	// IntegrationLineRefreshAlreadyCurrent reports that the local integration
	// line already matched its fetched remote-tracking reference.
	IntegrationLineRefreshAlreadyCurrent IntegrationLineRefresh = "already-current"
	// IntegrationLineRefreshDiverged reports that the local integration line
	// could not be fast-forwarded and was left untouched.
	IntegrationLineRefreshDiverged IntegrationLineRefresh = "diverged"
	// IntegrationLineRefreshFailed reports that the refresh attempt itself
	// failed; the completed publication and the transition remain valid.
	IntegrationLineRefreshFailed IntegrationLineRefresh = "failed"
)

// IntegrationLineReturn records the post-publication workspace transition
// outcome. The pull request is always created before the transition runs, so
// a skipped or failed transition never questions the publication itself.
type IntegrationLineReturn struct {
	Status IntegrationLineReturnStatus
	Branch branch.BranchName
	Detail string
	// Refresh reports the guarded fast-forward-only refresh of the local
	// integration line after the transition. It is empty when no refresh was
	// attempted: the transition was skipped or failed, the worktree was not
	// clean, or the composed Git adapter does not offer the capability.
	Refresh IntegrationLineRefresh
	// RefreshDetail carries the reason a refresh was skipped or failed.
	RefreshDetail string
}

// PublicationResult carries the published pull-request URL together with the
// post-publication workspace transition outcome.
type PublicationResult struct {
	URL string
	// IntegrationLineReturn is nil when the post-publication transition is
	// disabled for the current context, for example on server-side
	// controllers whose ephemeral checkout must never be switched.
	IntegrationLineReturn *IntegrationLineReturn
}

// returnToIntegrationLine switches the local workspace back to the develop
// integration line after a successfully created pull request and then attempts
// the guarded fast-forward-only refresh of that local checkout against its
// fetched remote-tracking reference. The transition is a local
// workspace-hygiene step: it never mutates the published branch, never
// discards uncommitted changes, and reports every outcome as a status instead
// of failing the already completed publication.
func returnToIntegrationLine(
	ctx context.Context,
	git port.GitRepository,
	repository port.RepositoryIdentity,
) IntegrationLineReturn {
	home := mustDevelop()
	current, err := git.CurrentBranch(ctx, repository)
	if err != nil {
		return IntegrationLineReturn{
			Status: IntegrationLineReturnFailed,
			Branch: home,
			Detail: "determine the current branch: " + err.Error(),
		}
	}
	if current.String() == home.String() {
		return refreshIntegrationLine(ctx, git, repository, IntegrationLineReturn{
			Status: IntegrationLineReturnAlreadyHome,
			Branch: home,
		})
	}
	clean, err := git.IsWorktreeClean(ctx, repository)
	if err != nil {
		return IntegrationLineReturn{
			Status: IntegrationLineReturnFailed,
			Branch: home,
			Detail: "inspect the worktree state: " + err.Error(),
		}
	}
	if !clean {
		return IntegrationLineReturn{
			Status: IntegrationLineReturnSkippedDirtyWorktree,
			Branch: home,
			Detail: "uncommitted changes are preserved; handle them and switch to the integration line manually",
		}
	}
	if err := git.SwitchBranch(ctx, repository, home); err != nil {
		return IntegrationLineReturn{
			Status: IntegrationLineReturnFailed,
			Branch: home,
			Detail: "switch to the integration line: " + err.Error(),
		}
	}
	return refreshIntegrationLine(ctx, git, repository, IntegrationLineReturn{
		Status: IntegrationLineReturnSwitched,
		Branch: home,
	})
}

// refreshIntegrationLine attempts the guarded fast-forward-only refresh of the
// local integration line after a completed transition. The refresh verifies
// its own clean-worktree precondition at its boundary, is fail-open against
// the already completed publication and transition, and leaves a diverged line
// untouched. When the composed Git adapter does not offer the capability, the
// transition result is returned unchanged.
func refreshIntegrationLine(
	ctx context.Context,
	git port.GitRepository,
	repository port.RepositoryIdentity,
	result IntegrationLineReturn,
) IntegrationLineReturn {
	refresher, ok := git.(port.BranchFastForwarder)
	if !ok {
		return result
	}
	clean, err := git.IsWorktreeClean(ctx, repository)
	if err != nil {
		result.Refresh = IntegrationLineRefreshFailed
		result.RefreshDetail = "inspect the worktree state: " + err.Error()
		return result
	}
	if !clean {
		result.RefreshDetail = "uncommitted changes are preserved; the local integration line was not refreshed"
		return result
	}
	// result.Branch is the canonical integration line produced by mustDevelop
	// and the selected remote is validated upstream, so this base construction
	// cannot fail.
	base, _ := branch.NewTargetBase(repository.Remote, result.Branch)
	outcome, err := refresher.FastForwardBranch(ctx, repository, result.Branch, base)
	if err != nil {
		result.Refresh = IntegrationLineRefreshFailed
		result.RefreshDetail = "refresh the local integration line: " + err.Error()
		return result
	}
	switch outcome {
	case port.FastForwardUpdated:
		result.Refresh = IntegrationLineRefreshUpdated
	case port.FastForwardAlreadyCurrent:
		result.Refresh = IntegrationLineRefreshAlreadyCurrent
	default:
		result.Refresh = IntegrationLineRefreshDiverged
		result.RefreshDetail = "the local integration line has diverged from its fetched remote-tracking reference and was left untouched"
	}
	return result
}
