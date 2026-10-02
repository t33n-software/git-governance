// Package port contains outbound contracts owned by the application layer.
// Adapters implement these contracts; domain packages never import them.
package port

import (
	"context"
	"errors"
	"time"

	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/hotfix"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/releaserequest"
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

// StagePreviewer is an optional capability for adapters that can resolve the
// effective initial content set of explicit stage paths without mutating the
// index. The preview binds the boundary scan to exactly the paths a later
// stage operation would add.
type StagePreviewer interface {
	PreviewStage(ctx context.Context, repository RepositoryIdentity, paths []string) ([]string, error)
}

// RefExistenceInspector is an optional capability for adapters that can prove
// whether any Git reference exists at all. The governed repository birth
// requires this proof: an unborn repository carries no references.
type RefExistenceInspector interface {
	HasAnyRef(ctx context.Context, repository RepositoryIdentity) (bool, error)
}

// CommitSignatureVerifier is an optional capability for adapters that can
// prove the signature of an existing commit object. The genesis finalizer
// re-proves the signature of the created genesis commit independently of the
// creation path.
type CommitSignatureVerifier interface {
	VerifyCommitSignature(ctx context.Context, repository RepositoryIdentity, revision string) error
}

// HookInstallation proves the installed local hook boundary of a repository.
type HookInstallation struct {
	Directory string
	Hooks     []string
}

// HookInstaller is an optional capability for adapters that can install the
// repository's hook boundary and prove the installed hooks. The governed
// repository birth installs the boundary so every subsequent operation is
// governed from the first commit onward.
type HookInstaller interface {
	InstallHooks(ctx context.Context, repository RepositoryIdentity) (HookInstallation, error)
}

// CherryPickContinuator is consumed only by workflows that must resume a
// user-resolved cherry-pick. Keeping it separate avoids forcing unrelated Git
// adapters and test fakes to implement a mutation they never invoke.
type CherryPickContinuator interface {
	ContinueCherryPick(ctx context.Context, repository RepositoryIdentity) error
}

// MergeContinuator is consumed only by workflows that resume a manually
// resolved merge. Keeping it optional preserves the narrow GitRepository
// contract for workflows that never continue merge state.
type MergeContinuator interface {
	ContinueMerge(ctx context.Context, repository RepositoryIdentity) error
}

// ActiveMergeTargetInspector verifies that an in-progress merge still targets
// the fetched remote-tracking base that the workflow is allowed to integrate.
type ActiveMergeTargetInspector interface {
	ActiveMergeTargetMatches(ctx context.Context, repository RepositoryIdentity, target branch.TargetBase) (bool, error)
}

// ReconciliationMergeInspector verifies that the checked-out resolution
// candidate is the exact merge of a delivered release ref and a pinned develop
// ref before a privileged controller publishes it.
type ReconciliationMergeInspector interface {
	ResolveReconciliationBases(
		ctx context.Context,
		repository RepositoryIdentity,
		release branch.TargetBase,
		develop branch.TargetBase,
	) (releaseRevision string, developRevision string, err error)
	HeadIsMergeOf(
		ctx context.Context,
		repository RepositoryIdentity,
		release branch.TargetBase,
		develop branch.TargetBase,
	) (bool, error)
}

// FastForwardOutcome classifies a guarded fast-forward-only update of a local
// branch to its fetched remote-tracking base.
type FastForwardOutcome string

const (
	// FastForwardUpdated reports that the local branch advanced to the base.
	FastForwardUpdated FastForwardOutcome = "updated"
	// FastForwardAlreadyCurrent reports that the local branch already matched
	// the base; no mutation happened.
	FastForwardAlreadyCurrent FastForwardOutcome = "already-current"
	// FastForwardDiverged reports that the local branch cannot fast-forward
	// to the base and was left untouched.
	FastForwardDiverged FastForwardOutcome = "diverged"
)

// BranchFastForwarder is an optional capability for adapters that can advance
// a checked-out local branch to its fetched remote-tracking base when — and
// only when — that update is a fast-forward. The caller owns the
// preconditions: the branch is checked out and the worktree is clean. A
// diverged branch is reported, never forced, and never produces a merge
// commit.
type BranchFastForwarder interface {
	FastForwardBranch(
		ctx context.Context,
		repository RepositoryIdentity,
		name branch.BranchName,
		base branch.TargetBase,
	) (FastForwardOutcome, error)
}

// LocalBranchLister is an optional capability for adapters that can enumerate
// the local canonical branches of a repository. Keeping it separate avoids
// forcing unrelated Git adapters and test fakes to implement a read they never
// invoke.
type LocalBranchLister interface {
	LocalBranches(ctx context.Context, repository RepositoryIdentity) ([]branch.BranchName, error)
}

// RemoteBranchLister is an optional capability for adapters that can enumerate
// the fetched remote-tracking canonical branches of the selected remote. The
// allocation inventory reads the branch-ref surface through it; keeping the
// capability separate avoids forcing unrelated adapters and test fakes to
// implement a read they never invoke.
type RemoteBranchLister interface {
	RemoteBranches(ctx context.Context, repository RepositoryIdentity) ([]branch.BranchName, error)
}

// CommitSubjectLister is an optional capability for adapters that can read the
// complete commit subject history across every reachable reference, including
// unborn repositories that carry none. The allocation inventory parses those
// subjects with the canonical commit header grammar; it never string-matches.
type CommitSubjectLister interface {
	CommitSubjects(ctx context.Context, repository RepositoryIdentity) ([]string, error)
}

// BranchReferenceFastForwarder is an optional capability for adapters that can
// advance a local branch reference to its fetched remote-tracking base when —
// and only when — that update is a fast-forward. The caller owns the
// preconditions: the branch exists locally and is not checked out in any
// worktree, so the pinned reference update never touches a working tree. A
// diverged branch is reported, never forced, and never produces a merge
// commit.
type BranchReferenceFastForwarder interface {
	FastForwardBranchReference(
		ctx context.Context,
		repository RepositoryIdentity,
		name branch.BranchName,
		base branch.TargetBase,
	) (FastForwardOutcome, error)
}

// KeyPolicy validates a syntactically valid key against the active local
// policy. The first implementation only checks syntax; a bundle adapter can
// add repository authorization later.
type KeyPolicy interface {
	ValidateKey(ctx context.Context, repository RepositoryIdentity, key ticket.Key) error
}

// PolicyStatus describes the active local policy mode without exposing policy
// implementation details to diagnostics consumers.
type PolicyStatus struct {
	Mode          string
	BundlePresent bool
	BundleFresh   bool
	Detail        string
}

// PolicyInspector reports read-only status for the active policy adapter.
type PolicyInspector interface {
	Status(ctx context.Context, repository RepositoryIdentity) (PolicyStatus, error)
}

// ToolInspector performs bounded read-only diagnostics for external tools and
// repository-local configuration files.
type ToolInspector interface {
	Platform() (operatingSystem string, architecture string)
	Version(ctx context.Context, executable string) (string, error)
	FileExists(path string) (bool, error)
}

// Preferences are user-scoped UX preferences, never organizational policy or
// secrets.
type Preferences struct {
	SchemaVersion int
	KnownKeys     []ticket.Key
	DefaultKey    *ticket.Key
	Accessible    bool
}

// PreferencesStore persists user-scoped preferences.
type PreferencesStore interface {
	Load(ctx context.Context) (Preferences, error)
	Save(ctx context.Context, preferences Preferences) error
}

// Prompt is the inbound interactive terminal boundary.
type Prompt interface {
	Input(ctx context.Context, request InputRequest) (string, error)
	Select(ctx context.Context, request SelectRequest) (string, error)
	Confirm(ctx context.Context, request ConfirmRequest) (bool, error)
}

// InputValidator validates one interactive input candidate. It returns an
// error that the terminal adapter can render before asking again.
type InputValidator func(string) error

// InputRequest describes an explanatory text input.
type InputRequest struct {
	Label       string
	Description string
	Default     string
	Required    bool
	Validate    InputValidator
	Sensitive   bool
}

// SelectRequest describes an explanatory single-value choice.
type SelectRequest struct {
	Label       string
	Description string
	Options     []SelectOption
	Default     string
}

// SelectOption is a stable machine value and a human-facing explanation.
type SelectOption struct {
	Value       string
	Label       string
	Description string
}

// ConfirmRequest describes a consequential action requiring explicit consent.
type ConfirmRequest struct {
	Label       string
	Description string
	Default     bool
}

// Report is the delivery-neutral result of a use case.
type Report struct {
	Operation string
	Summary   string
	Fields    map[string]string
	Data      any
	Problem   *problem.Problem
}

// Reporter renders application results as human or machine output.
type Reporter interface {
	Report(ctx context.Context, result Report) error
}

// QualityStatus classifies whether repository-local quality gates were
// configured and successfully executed.
type QualityStatus string

const (
	QualityUnconfigured QualityStatus = "unconfigured"
	QualitySkipped      QualityStatus = "skipped"
	QualityPassed       QualityStatus = "passed"
)

// QualityGateResult records one successfully completed configured gate without
// exposing its process output, which may contain sensitive project data.
type QualityGateResult struct {
	Name string
}

// QualityResult reports the actual quality-gate outcome. An unconfigured
// repository is not reported as a passed suite.
type QualityResult struct {
	Status QualityStatus
	Detail string
	Gates  []QualityGateResult
}

// QualityRequest identifies the branch families relevant to one
// publication-affecting operation. The runner evaluates each configured gate
// once against this complete set, so a multi-ref push cannot duplicate work.
type QualityRequest struct {
	Families []branch.Family
}

// QualityRunner runs repository-defined local quality gates before a
// publication-affecting operation.
type QualityRunner interface {
	Run(ctx context.Context, repository RepositoryIdentity, request QualityRequest) (QualityResult, error)
}

// QualityFingerprint binds a successful quality run to the configuration and
// toolchain that selected its gates. It contains no process output or secrets.
type QualityFingerprint struct {
	ConfigurationDigest string
	Gates               []string
	Toolchain           string
}

// QualityEvidenceRunner executes configured quality gates and returns the
// matching fingerprint from the same configuration snapshot.
type QualityEvidenceRunner interface {
	QualityRunner
	RunWithFingerprint(
		ctx context.Context,
		repository RepositoryIdentity,
		request QualityRequest,
	) (QualityResult, QualityFingerprint, error)
	Fingerprint(
		ctx context.Context,
		repository RepositoryIdentity,
		request QualityRequest,
	) (QualityFingerprint, error)
}

// FinalQualityEvidenceUpdate identifies one exact branch update covered by a
// local final-quality result. BaseRevision is empty only when no target base
// applies, such as a private scratch branch.
type FinalQualityEvidenceUpdate struct {
	LocalRef      string
	LocalObjectID string
	RemoteRef     string
	Branch        string
	Base          string
	BaseRevision  string
}

// FinalQualityEvidence is local Git metadata used only to deduplicate an
// identical quality run for an outgoing publish candidate. It is never a
// server-side approval or authorization artifact.
type FinalQualityEvidence struct {
	SchemaVersion       int
	Remote              string
	ConfigurationDigest string
	Gates               []string
	Toolchain           string
	WorktreeClean       bool
	CreatedAt           time.Time
	Updates             []FinalQualityEvidenceUpdate
}

// FinalQualityEvidenceStore persists a short-lived final-quality record in
// repository-local Git metadata rather than in the working tree.
type FinalQualityEvidenceStore interface {
	LoadFinalQualityEvidence(
		ctx context.Context,
		repository RepositoryIdentity,
	) (FinalQualityEvidence, bool, error)
	StoreFinalQualityEvidence(
		ctx context.Context,
		repository RepositoryIdentity,
		evidence FinalQualityEvidence,
	) error
}

// RevisionResolver resolves a ref to the exact commit object used for
// revision-bound local quality evidence.
type RevisionResolver interface {
	ResolveRevision(ctx context.Context, repository RepositoryIdentity, revision string) (string, error)
}

// HotfixReleaseRecordStore loads a reviewed hotfix release record from the
// repository without allowing callers to escape its controlled record area.
type HotfixReleaseRecordStore interface {
	LoadHotfixReleaseRecord(
		ctx context.Context,
		repository RepositoryIdentity,
		id ticket.ID,
		location string,
	) (hotfix.ReleaseRecord, error)
}

// HotfixReleaseRecord binds one reviewed release record to its
// repository-relative location for the allocation inventory.
type HotfixReleaseRecord struct {
	Record   hotfix.ReleaseRecord
	Location string
}

// HotfixReleaseRecordLister is an optional capability for adapters that can
// enumerate every reviewed release record below the controlled repository
// record directory. The allocation inventory derives hotfix record numbers
// from the schema-validated records themselves.
type HotfixReleaseRecordLister interface {
	ListHotfixReleaseRecords(ctx context.Context, repository RepositoryIdentity) ([]HotfixReleaseRecord, error)
}

// HotfixManifestProgress records one in-progress ordered propagation in
// repository-local Git metadata. It is not a reviewed release record and
// never grants publication authority.
type HotfixManifestProgress struct {
	Branch   branch.BranchName
	Source   branch.BranchName
	Target   branch.BranchName
	Manifest []string
	Next     int
}

// HotfixManifestProgressStore persists the local state needed to continue an
// explicitly user-resolved ordered cherry-pick without guessing its position.
type HotfixManifestProgressStore interface {
	LoadHotfixManifestProgress(
		ctx context.Context,
		repository RepositoryIdentity,
	) (HotfixManifestProgress, bool, error)
	StoreHotfixManifestProgress(
		ctx context.Context,
		repository RepositoryIdentity,
		progress HotfixManifestProgress,
	) error
	ClearHotfixManifestProgress(
		ctx context.Context,
		repository RepositoryIdentity,
	) error
}

// PullRequest describes a provider-neutral pull request intent. Body carries
// the mandatory pull-request description; it is required whenever the intent
// is published through a provider. The mandate and its canonical structure
// are owned by docs/conventions/pull-requests/description-mandate.md.
type PullRequest struct {
	Source branch.BranchName
	Target branch.BranchName
	Ticket ticket.ID
	Title  string
	Body   string
	Draft  bool
}

// PullRequestPublication carries provider-neutral pull-request intent together
// with the selected Git remote needed by a hosting adapter. RemoteURL is never
// included in the CLI result contract.
type PullRequestPublication struct {
	Repository  RepositoryIdentity
	RemoteURL   string
	PullRequest PullRequest
}

// PublishedPullRequest represents an optional provider-specific result.
type PublishedPullRequest struct {
	URL string
}

// PullRequestPublisher is optional: the product always emits provider-neutral
// pull-request data and publishes it only through an explicitly configured host.
type PullRequestPublisher interface {
	Publish(ctx context.Context, publication PullRequestPublication) (PublishedPullRequest, error)
}

// PullRequestPublisherPreflight is an optional capability for adapters that
// can validate credentials and routing before a Git branch is pushed.
type PullRequestPublisherPreflight interface {
	Validate(ctx context.Context, publication PullRequestPublication) error
}

// PullRequestInventoryQuery identifies the repository whose complete open and
// closed pull-request surface is enumerated through a hosting provider.
type PullRequestInventoryQuery struct {
	Repository RepositoryIdentity
	RemoteURL  string
}

// PullRequestSummary carries one provider-neutral pull-request record of the
// allocation inventory.
type PullRequestSummary struct {
	Number    string
	Title     string
	Author    string
	CreatedAt time.Time
}

// PullRequestInventoryLister is an optional capability for hosting adapters
// that can enumerate the complete pull-request surface of one repository
// through pagination. The allocation inventory parses titles with the
// canonical ticket grammars; it never string-matches, and a surface that
// cannot be read completely fails the inventory closed.
type PullRequestInventoryLister interface {
	ListPullRequests(ctx context.Context, query PullRequestInventoryQuery) ([]PullRequestSummary, error)
}

// SharedLineDispatchRequest describes a provider-owned request to create a
// protected release or support line. The application never directly pushes a
// shared line.
type SharedLineDispatchRequest struct {
	Repository RepositoryIdentity
	RemoteURL  string
	Workflow   string
	Ref        string
	Inputs     map[string]string
	Branch     branch.BranchName
}

// SharedLineDispatchResult records the completed provider workflow and the
// protected line it created.
type SharedLineDispatchResult struct {
	WorkflowRunURL string
	Branch         branch.BranchName
}

// ReleaseReconciliationRequest identifies one released line that must be
// checked against the integration line before a conditional backmerge.
type ReleaseReconciliationRequest struct {
	Repository RepositoryIdentity
	RemoteURL  string
	Release    branch.BranchName
}

// ReleaseReconciliationEvidence proves the causal release-delivery gates and
// reports whether a release-only effective delta remains for develop.
type ReleaseReconciliationEvidence struct {
	PromotionPullRequestURL string
	PromotionMergeCommit    string
	Tag                     string
	ReleaseURL              string
	EffectiveDelta          bool
}

// ReleaseLifecycleProvider is an optional hosting capability. It dispatches
// protected-line creation and verifies the delivery evidence required before a
// release-to-develop reconciliation can create a pull request.
type ReleaseLifecycleProvider interface {
	DispatchSharedLine(ctx context.Context, request SharedLineDispatchRequest) (SharedLineDispatchResult, error)
	VerifyReleaseReconciliation(ctx context.Context, request ReleaseReconciliationRequest) (ReleaseReconciliationEvidence, error)
}

// ProtectedLineRequestAuthorization binds one request-controller approval to
// the exact release or support line the provider may later execute.
type ProtectedLineRequestAuthorization struct {
	Repository  RepositoryIdentity
	RemoteURL   string
	Ticket      ticket.ID
	Operation   releaserequest.Operation
	Version     string
	Source      branch.BranchName
	Target      branch.BranchName
	Requester   string
	ParentRunID string
}

// ProtectedLineRequestResult identifies the durable request record and its
// asynchronous execution state. Dispatch acceptance is intentionally not
// represented as protected-line completion.
type ProtectedLineRequestResult struct {
	Request releaserequest.Request
}

// ProtectedLineExecutionAuthorization binds one execution workflow run to the
// durable request it is allowed to mutate exactly once.
type ProtectedLineExecutionAuthorization struct {
	Repository    RepositoryIdentity
	RemoteURL     string
	RequestID     string
	ExecutorRunID string
}

// ProtectedLineExecutionPlan exposes only the immutable source and target
// facts that the trusted executor needs for its one permitted mutation.
type ProtectedLineExecutionPlan struct {
	Request       releaserequest.Request
	NeedsMutation bool
}

// ProtectedLineFinalizationRequest correlates a read-only finalizer with one
// execution run. Recovery permits only a record already awaiting finalization.
type ProtectedLineFinalizationRequest struct {
	Repository    RepositoryIdentity
	RemoteURL     string
	RequestID     string
	ExecutorRunID string
	Recovery      bool
}

// ProtectedLineFinalizationResult exposes the durable final request state.
type ProtectedLineFinalizationResult struct {
	Request releaserequest.Request
}

// ProtectedLineRequestProvider owns durable request persistence, dispatch
// correlation, execution authorization, and read-only finalization facts for
// protected release and support-line creation.
type ProtectedLineRequestProvider interface {
	AuthorizeProtectedLineRequest(
		ctx context.Context,
		request ProtectedLineRequestAuthorization,
	) (ProtectedLineRequestResult, error)
	AuthorizeProtectedLineExecution(
		ctx context.Context,
		request ProtectedLineExecutionAuthorization,
	) (ProtectedLineExecutionPlan, error)
	FinalizeProtectedLineRequest(
		ctx context.Context,
		request ProtectedLineFinalizationRequest,
	) (ProtectedLineFinalizationResult, error)
}

// ProtectedLineRequestInventoryQuery identifies the repository whose durable
// protected-line request records are enumerated through a hosting provider.
type ProtectedLineRequestInventoryQuery struct {
	Repository RepositoryIdentity
	RemoteURL  string
}

// ProtectedLineRequestRecord binds one durable request record to its provider
// creation time for the allocation inventory.
type ProtectedLineRequestRecord struct {
	Request   releaserequest.Request
	CreatedAt time.Time
}

// ProtectedLineRequestInventoryLister is an optional capability for hosting
// adapters that can enumerate every durable protected-line request record of
// one repository. These records consume ticket numbers without any pull
// request, so they are allocation evidence no pull-request scan can see.
type ProtectedLineRequestInventoryLister interface {
	ListProtectedLineRequests(
		ctx context.Context,
		query ProtectedLineRequestInventoryQuery,
	) ([]ProtectedLineRequestRecord, error)
}

// CredentialCapability names a closed set of provider permission classes the
// CLI distinguishes when selecting a credential. The names match the public
// app registration permission keys; the allocation inventory owns the
// classification of a capability-scoped absence.
type CredentialCapability string

const (
	// CapabilityPullRequests selects a session whose app class carries the
	// pull requests permission at read or write level.
	CapabilityPullRequests CredentialCapability = "pull_requests"
	// CapabilityDeployments selects a session whose app class carries the
	// deployments permission at read or write level.
	CapabilityDeployments CredentialCapability = "deployments"
)

// Valid reports whether the capability is one of the governed permission
// classes.
func (capability CredentialCapability) Valid() bool {
	switch capability {
	case CapabilityPullRequests, CapabilityDeployments:
		return true
	default:
		return false
	}
}

// ErrCapabilitySessionMissing marks the named verdict that no stored session
// covering the target repository carries the requested permission class.
// Resolvers wrap it into a reported problem; callers distinguish it through
// errors.Is from genuine infrastructure failures. It is the classification
// basis of a capability-scoped absent surface, never a silent skip.
var ErrCapabilitySessionMissing = errors.New("no configured GitHub App session carries the requested permission class")

// AppPermissionCarries reports whether a permission snapshot carries one
// permission class at read or write level. A missing or unknown key means the
// app class does not carry the capability.
func AppPermissionCarries(permissions map[string]string, name string) bool {
	level, carried := permissions[name]
	return carried && (level == "read" || level == "write")
}

// AppPermissionQuery identifies the repository context whose configured
// provider identity is inspected for its permission class. Capability names
// the permission class whose serving session is measured; an empty or unknown
// capability is rejected before any request.
type AppPermissionQuery struct {
	Repository RepositoryIdentity
	RemoteURL  string
	Capability CredentialCapability
}

// AppPermissionSnapshot carries the cache-free permission facts of the
// configured provider app identity. Permissions maps the public registration
// permission names to their read or write level; a missing key means the app
// class does not carry the capability.
type AppPermissionSnapshot struct {
	Slug        string
	Permissions map[string]string
}

// AppPermissionInspector is an optional capability for hosting adapters that
// can read the public app registration of the configured provider identity.
// The inspection is a fresh measurement per call and is never cached: every
// inventory invocation re-reads the registration, so the capability
// classification stays a measurement and never becomes a stored claim state.
type AppPermissionInspector interface {
	InspectAppPermissions(ctx context.Context, query AppPermissionQuery) (AppPermissionSnapshot, error)
}

// MainHotfixDeliveryRequest binds a reviewed record to its repository before a
// production hotfix controller can create or verify a patch delivery.
type MainHotfixDeliveryRequest struct {
	Repository RepositoryIdentity
	RemoteURL  string
	Record     hotfix.ReleaseRecord
}

// MainHotfixMergeEvidence proves the exact same-repository hotfix pull
// request and merge that a delivery controller may tag.
type MainHotfixMergeEvidence struct {
	PullRequestURL string
	MergeCommit    string
	Tag            string
}

// MainHotfixDeliveryEvidence adds the published release and successful
// artifact workflow that bind the patch delivery evidence to the merge.
type MainHotfixDeliveryEvidence struct {
	MainHotfixMergeEvidence
	ReleaseURL     string
	WorkflowRunURL string
}

// MainHotfixLifecycleProvider verifies provider-owned facts for a main hotfix
// before immutable tagging and after artifact delivery. It is intentionally a
// read-only contract; the trusted workflow owns tag creation and dispatch.
type MainHotfixLifecycleProvider interface {
	VerifyMainHotfixMerge(ctx context.Context, request MainHotfixDeliveryRequest) (MainHotfixMergeEvidence, error)
	VerifyMainHotfixDelivery(ctx context.Context, request MainHotfixDeliveryRequest) (MainHotfixDeliveryEvidence, error)
}
