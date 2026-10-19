package port

import (
	"context"
	"errors"
	"time"

	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/hotfix"
	"github.com/t33n-software/git-governance/internal/domain/releaserequest"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

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
