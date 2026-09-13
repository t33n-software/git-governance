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

// IntegrationLineReturn records the post-publication workspace transition
// outcome. The pull request is always created before the transition runs, so
// a skipped or failed transition never questions the publication itself.
type IntegrationLineReturn struct {
	Status IntegrationLineReturnStatus
	Branch branch.BranchName
	Detail string
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
// integration line after a successfully created pull request. The transition
// is a local workspace-hygiene step: it never mutates the published branch,
// never discards uncommitted changes, and reports every outcome as a status
// instead of failing the already completed publication.
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
		return IntegrationLineReturn{Status: IntegrationLineReturnAlreadyHome, Branch: home}
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
	return IntegrationLineReturn{Status: IntegrationLineReturnSwitched, Branch: home}
}
