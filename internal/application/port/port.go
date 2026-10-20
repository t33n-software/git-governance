// Package port contains outbound contracts owned by the application layer.
// Adapters implement these contracts; domain packages never import them.
//
// The contracts are organized by bounded surface in sibling files: hooks.go
// (birth and operation-state capabilities), worktrees.go (worktree and
// branch inventory), policy.go (policy, preferences, and prompt surfaces),
// quality.go (quality gates and revision evidence), release.go (reports,
// hotfix records, and pull-request publication), and hosting.go (provider
// lifecycle, protected lines, and app permissions).
package port

import (
	"context"
	"time"

	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

// RepositoryIdentity identifies the local repository and its selected remote.
type RepositoryIdentity struct {
	Root   string
	Remote string
}

// PushUpdateInspection contains Git-derived facts about one outgoing branch
// update. The application supplies the exact object IDs received from the
// pre-push hook; the adapter never infers them from the checked-out branch.
type PushUpdateInspection struct {
	MissingBaseCommits bool
	FastForward        bool
	CommitMessages     []string
}

// GitRepository is the explicit boundary for Git process operations. It is not
// a generic persistence repository.
type GitRepository interface {
	Discover(ctx context.Context, directory string) (RepositoryIdentity, error)
	Version(ctx context.Context) (string, error)
	RemoteURL(ctx context.Context, repository RepositoryIdentity) (string, error)
	ActiveOperation(ctx context.Context, repository RepositoryIdentity) (string, bool, error)
	HasCommits(ctx context.Context, repository RepositoryIdentity) (bool, error)
	IsWorktreeClean(ctx context.Context, repository RepositoryIdentity) (bool, error)
	CurrentBranch(ctx context.Context, repository RepositoryIdentity) (branch.BranchName, error)
	ValidateBranchRef(ctx context.Context, repository RepositoryIdentity, name branch.BranchName) error
	BranchExists(ctx context.Context, repository RepositoryIdentity, name branch.BranchName) (bool, error)
	OfficialBranchesForTicket(ctx context.Context, repository RepositoryIdentity, id ticket.ID) ([]branch.BranchName, error)
	Fetch(ctx context.Context, repository RepositoryIdentity) error
	TargetBaseExists(ctx context.Context, repository RepositoryIdentity, base branch.TargetBase) (bool, error)
	CreateBranch(ctx context.Context, repository RepositoryIdentity, name branch.BranchName, base branch.TargetBase, switchTo bool) error
	StoreWorkflowBase(ctx context.Context, repository RepositoryIdentity, name branch.BranchName, base branch.TargetBase) error
	ClearWorkflowBase(ctx context.Context, repository RepositoryIdentity, name branch.BranchName) error
	WorkflowBase(ctx context.Context, repository RepositoryIdentity, name branch.BranchName) (branch.TargetBase, bool, error)
	SwitchBranch(ctx context.Context, repository RepositoryIdentity, name branch.BranchName) error
	PublicationState(ctx context.Context, repository RepositoryIdentity, name branch.BranchName) (branch.PublicationState, error)
	HasMissingBaseCommits(ctx context.Context, repository RepositoryIdentity, base branch.TargetBase) (bool, error)
	CommitMessagesSince(ctx context.Context, repository RepositoryIdentity, base branch.TargetBase) ([]string, error)
	Rebase(ctx context.Context, repository RepositoryIdentity, base branch.TargetBase) error
	ContinueRebase(ctx context.Context, repository RepositoryIdentity) error
	Merge(ctx context.Context, repository RepositoryIdentity, base branch.TargetBase, message commitmsg.Message) error
	SquashMerge(ctx context.Context, repository RepositoryIdentity, source branch.BranchName) error
	CherryPick(ctx context.Context, repository RepositoryIdentity, commitID string) error
	DeleteLocalBranch(ctx context.Context, repository RepositoryIdentity, name branch.BranchName, force bool) error
	ReleaseTagsAt(ctx context.Context, repository RepositoryIdentity, revision string) ([]string, error)
	HasUnmergedConflicts(ctx context.Context, repository RepositoryIdentity) (bool, error)
	HasStagedChanges(ctx context.Context, repository RepositoryIdentity) (bool, error)
	Stage(ctx context.Context, repository RepositoryIdentity, paths []string) error
	Commit(ctx context.Context, repository RepositoryIdentity, message commitmsg.Message) error
	Push(ctx context.Context, repository RepositoryIdentity, name branch.BranchName, setUpstream bool) error
	InspectPushUpdate(
		ctx context.Context,
		repository RepositoryIdentity,
		base branch.TargetBase,
		localObjectID string,
		remoteObjectID string,
	) (PushUpdateInspection, error)
}

// OperationTimeoutScoper is an optional capability for adapters that can scope
// the per-process timeout of the returned Git surface. The governed repository
// birth derives its mutation budget from the proven preflight corpus size;
// this capability applies that budget to the genesis mutation without widening
// the shared GitRepository contract for adapters and test fakes that never
// scope a timeout.
type OperationTimeoutScoper interface {
	WithOperationTimeout(timeout time.Duration) GitRepository
}

// GitTransportAuthenticator is an optional diagnostic capability. It verifies
// that the configured Git transport can authenticate and authorize a
// non-interactive dry-run creation of the reserved probe reference without
// mutating remote references. The probe binds only to transport
// authentication and write authorization — never to the checked-out branch,
// its freshness, or the checkout shape; the behavioral contract is documented
// in docs/usage/diagnostics.md.
type GitTransportAuthenticator interface {
	CheckTransportAuthentication(context.Context, RepositoryIdentity) error
}

// SigningConfiguration carries the effective commit-signing facts of one
// repository context. It never carries key material, only references and
// readiness facts.
type SigningConfiguration struct {
	SigningEnabled         bool
	Format                 string
	SigningKey             string
	SigningKeyReadable     bool
	UserName               string
	UserEmail              string
	SignProgram            string
	AllowedSignersFile     string
	AllowedSignersReadable bool
	LaneInjection          bool
	InjectedSigningKeys    []string
}

// GitSigningInspector is an optional diagnostic capability. It reads the
// effective commit-signing configuration and proves signing capability through
// a non-mutating canary signature that is verified against the configured
// allowed-signers surface.
type GitSigningInspector interface {
	SigningConfiguration(ctx context.Context, repository RepositoryIdentity) (SigningConfiguration, error)
	ProveSigningCapability(ctx context.Context, repository RepositoryIdentity, configuration SigningConfiguration) error
}
