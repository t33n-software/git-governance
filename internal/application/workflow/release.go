package workflow

import (
	"context"
	"regexp"
	"strings"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/hotfix"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/releaserequest"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

// ReleaseService owns the bounded hotfix, release, support, and release
// backmerge workflows.
type ReleaseService struct {
	branches            *branchapp.Service
	git                 port.GitRepository
	publisher           port.PullRequestPublisher
	lifecycle           port.ReleaseLifecycleProvider
	protectedRequests   port.ProtectedLineRequestProvider
	hotfix              port.MainHotfixLifecycleProvider
	tickets             *TicketService
	quality             port.QualityRunner
	records             port.HotfixReleaseRecordStore
	manifestPublication bool
}

var commitIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)

// ValidateCommitID verifies the bounded hexadecimal identifier accepted by
// the controlled hotfix propagation workflow.
func ValidateCommitID(raw string) error {
	if commitIDPattern.MatchString(raw) {
		return nil
	}
	return problem.New(problem.Details{
		Code:        problem.CodeInvalidInput,
		Category:    problem.CategoryGovernance,
		Field:       "reviewed source commit",
		Actual:      raw,
		Expected:    "a 7 to 64 character hexadecimal commit ID",
		Rule:        "hotfix propagation cherry-picks one reviewed Git commit",
		Example:     "0123456789abcdef0123456789abcdef01234567",
		Remediation: "provide the reviewed source commit SHA without spaces or a ref name",
	})
}

// NewReleaseService creates a release workflow service.
func NewReleaseService(branches *branchapp.Service, git port.GitRepository, publisher port.PullRequestPublisher) *ReleaseService {
	return &ReleaseService{
		branches:  branches,
		git:       git,
		publisher: publisher,
	}
}

// WithTicketService wires publication behavior into release workflows without
// making the release service depend on the CLI delivery layer.
func (service *ReleaseService) WithTicketService(tickets *TicketService) *ReleaseService {
	service.tickets = tickets
	return service
}

// WithQualityRunner wires repository quality gates into release preparation
// workflows that mutate a working branch before publication.
func (service *ReleaseService) WithQualityRunner(quality port.QualityRunner) *ReleaseService {
	service.quality = quality
	return service
}

// WithHotfixManifestPublication enables publication only for the dedicated
// server-side hotfix propagation publisher boundary.
func (service *ReleaseService) WithHotfixManifestPublication(enabled bool) *ReleaseService {
	service.manifestPublication = enabled
	return service
}

// WithHotfixReleaseRecordStore wires the repository-local, reviewed release
// record reader into main hotfix publication validation.
func (service *ReleaseService) WithHotfixReleaseRecordStore(records port.HotfixReleaseRecordStore) *ReleaseService {
	service.records = records
	return service
}

// WithReleaseLifecycleProvider wires provider-owned protected-line dispatch
// and release-delivery verification into release workflows.
func (service *ReleaseService) WithReleaseLifecycleProvider(provider port.ReleaseLifecycleProvider) *ReleaseService {
	service.lifecycle = provider
	return service
}

// WithProtectedLineRequestProvider wires the durable request, execution, and
// finalization boundary into protected release and support-line workflows.
func (service *ReleaseService) WithProtectedLineRequestProvider(provider port.ProtectedLineRequestProvider) *ReleaseService {
	service.protectedRequests = provider
	return service
}

// WithMainHotfixLifecycleProvider wires read-only provider evidence checks
// into the production main-hotfix delivery workflow.
func (service *ReleaseService) WithMainHotfixLifecycleProvider(provider port.MainHotfixLifecycleProvider) *ReleaseService {
	service.hotfix = provider
	return service
}

// StartHotfixRequest describes the affected line and ticket for a hotfix.
type StartHotfixRequest struct {
	Repository   port.RepositoryIdentity
	Ticket       ticket.ID
	Slug         branch.Slug
	AffectedLine branch.BranchName
	DryRun       bool
}

// StartHotfix creates a hotfix directly from the active line that contains the
// defect.
func (service *ReleaseService) StartHotfix(ctx context.Context, request StartHotfixRequest) (branchapp.CreateResult, error) {
	if service.branches == nil {
		return branchapp.CreateResult{}, internalDependencyError("branch service")
	}
	if request.AffectedLine.IsZero() {
		return branchapp.CreateResult{}, invalidWorkflowInput(
			"an affected main, release, or support line is required",
			"select the line that actually contains the defect",
		)
	}
	affectedFamily := request.AffectedLine.Family()
	if affectedFamily != branch.FamilyMain && affectedFamily != branch.FamilyRelease && affectedFamily != branch.FamilySupport {
		return branchapp.CreateResult{}, invalidWorkflowInput(
			"a hotfix starts from main, release/<semver>, or support/<major.minor>",
			"do not start a hotfix from develop or a regular ticket branch",
		)
	}
	repository, err := normalizeWorkflowRepository(request.Repository)
	if err != nil {
		return branchapp.CreateResult{}, err
	}
	base, err := branch.NewTargetBase(repository.Remote, request.AffectedLine)
	if err != nil {
		return branchapp.CreateResult{}, err
	}
	if !request.DryRun && service.git == nil {
		return branchapp.CreateResult{}, internalDependencyError("Git repository")
	}
	switchToBranch := true
	result, err := service.branches.Create(ctx, branchapp.CreateRequest{
		Repository:      repository,
		Family:          branch.FamilyHotfix,
		Ticket:          request.Ticket,
		Slug:            request.Slug,
		Base:            &base,
		Switch:          &switchToBranch,
		DryRun:          request.DryRun,
		WorkflowManaged: true,
	})
	if err != nil {
		return branchapp.CreateResult{}, err
	}
	if !request.DryRun {
		if err := service.git.StoreWorkflowBase(ctx, repository, result.Name, base); err != nil {
			return branchapp.CreateResult{}, err
		}
	}
	return result, nil
}

// ValidateMainHotfixRecordRequest identifies the hotfix branch and optional
// repository-relative record location to validate before a main hotfix can be
// published for review.
type ValidateMainHotfixRecordRequest struct {
	Repository port.RepositoryIdentity
	Branch     branch.BranchName
	Location   string
}

// ValidateMainHotfixRecordResult exposes only the non-secret facts a caller
// needs to bind a main hotfix publication to its reviewed release record.
type ValidateMainHotfixRecordResult struct {
	Record hotfix.ReleaseRecord
}

// ValidateMainHotfixRecord verifies that a reviewed record belongs to the
// selected hotfix branch and satisfies main patch-delivery invariants.
func (service *ReleaseService) ValidateMainHotfixRecord(
	ctx context.Context,
	request ValidateMainHotfixRecordRequest,
) (ValidateMainHotfixRecordResult, error) {
	if service.records == nil {
		return ValidateMainHotfixRecordResult{}, internalDependencyError("hotfix release record store")
	}
	if request.Branch.Family() != branch.FamilyHotfix {
		return ValidateMainHotfixRecordResult{}, invalidWorkflowInput(
			"main hotfix record validation requires a hotfix/<ticket>-<slug> branch",
			"select the ticket-bound hotfix branch reviewed for main delivery",
		)
	}
	id, _ := request.Branch.Ticket()
	repository, err := normalizeWorkflowRepository(request.Repository)
	if err != nil {
		return ValidateMainHotfixRecordResult{}, err
	}
	record, err := service.records.LoadHotfixReleaseRecord(ctx, repository, id, request.Location)
	if err != nil {
		return ValidateMainHotfixRecordResult{}, err
	}
	if record.Ticket().String() != id.String() || record.ExpectedSource().String() != request.Branch.String() {
		return ValidateMainHotfixRecordResult{}, invalidWorkflowInput(
			"the hotfix release record must bind the selected ticket and source branch",
			"update the reviewed record to match the exact hotfix branch before publishing",
		)
	}
	if err := record.ValidateMainPatchDelivery(); err != nil {
		return ValidateMainHotfixRecordResult{}, err
	}
	return ValidateMainHotfixRecordResult{Record: record}, nil
}

// VerifyMainHotfixMergeRequest identifies the reviewed record and hotfix
// branch that a trusted controller wants to validate before immutable tagging.
type VerifyMainHotfixMergeRequest struct {
	Repository port.RepositoryIdentity
	Branch     branch.BranchName
	Location   string
}

// VerifyMainHotfixMergeResult binds the validated record to provider evidence
// for the exact merged main hotfix.
type VerifyMainHotfixMergeResult struct {
	Record   hotfix.ReleaseRecord
	Evidence port.MainHotfixMergeEvidence
}

// VerifyMainHotfixMerge independently proves the reviewed same-repository PR,
// its exact merge commit, and the ordered manifest before a controller tags it.
func (service *ReleaseService) VerifyMainHotfixMerge(
	ctx context.Context,
	request VerifyMainHotfixMergeRequest,
) (VerifyMainHotfixMergeResult, error) {
	if service.git == nil {
		return VerifyMainHotfixMergeResult{}, internalDependencyError("Git repository")
	}
	if service.hotfix == nil {
		return VerifyMainHotfixMergeResult{}, internalDependencyError("main hotfix lifecycle provider")
	}
	record, repository, err := service.validatedMainHotfixRecord(ctx, request.Repository, request.Branch, request.Location)
	if err != nil {
		return VerifyMainHotfixMergeResult{}, err
	}
	remoteURL, err := service.git.RemoteURL(ctx, repository)
	if err != nil {
		return VerifyMainHotfixMergeResult{}, err
	}
	evidence, err := service.hotfix.VerifyMainHotfixMerge(ctx, port.MainHotfixDeliveryRequest{
		Repository: repository,
		RemoteURL:  remoteURL,
		Record:     record,
	})
	if err != nil {
		return VerifyMainHotfixMergeResult{}, err
	}
	if evidence.Tag != "v"+record.TargetVersion().String() || evidence.MergeCommit == "" || evidence.PullRequestURL == "" {
		return VerifyMainHotfixMergeResult{}, invalidWorkflowInput(
			"main hotfix merge evidence must bind the expected patch tag, merge commit, and pull request",
			"repair the record or provider evidence before creating an immutable tag",
		)
	}
	return VerifyMainHotfixMergeResult{Record: record, Evidence: evidence}, nil
}

// VerifyMainHotfixDeliveryRequest identifies a main hotfix whose tag and
// artifact delivery must be independently verified after release automation.
type VerifyMainHotfixDeliveryRequest struct {
	Repository port.RepositoryIdentity
	Branch     branch.BranchName
	Location   string
}

// VerifyMainHotfixDeliveryResult captures the record and complete provider
// evidence required before the hotfix can be considered delivered.
type VerifyMainHotfixDeliveryResult struct {
	Record   hotfix.ReleaseRecord
	Evidence port.MainHotfixDeliveryEvidence
}

// VerifyMainHotfixDelivery proves that the immutable patch tag, published
// release, and successful artifact workflow all bind to the reviewed merge.
func (service *ReleaseService) VerifyMainHotfixDelivery(
	ctx context.Context,
	request VerifyMainHotfixDeliveryRequest,
) (VerifyMainHotfixDeliveryResult, error) {
	if service.git == nil {
		return VerifyMainHotfixDeliveryResult{}, internalDependencyError("Git repository")
	}
	if service.hotfix == nil {
		return VerifyMainHotfixDeliveryResult{}, internalDependencyError("main hotfix lifecycle provider")
	}
	record, repository, err := service.validatedMainHotfixRecord(ctx, request.Repository, request.Branch, request.Location)
	if err != nil {
		return VerifyMainHotfixDeliveryResult{}, err
	}
	remoteURL, err := service.git.RemoteURL(ctx, repository)
	if err != nil {
		return VerifyMainHotfixDeliveryResult{}, err
	}
	evidence, err := service.hotfix.VerifyMainHotfixDelivery(ctx, port.MainHotfixDeliveryRequest{
		Repository: repository,
		RemoteURL:  remoteURL,
		Record:     record,
	})
	if err != nil {
		return VerifyMainHotfixDeliveryResult{}, err
	}
	if evidence.Tag != "v"+record.TargetVersion().String() ||
		evidence.MergeCommit == "" ||
		evidence.PullRequestURL == "" ||
		evidence.ReleaseURL == "" ||
		evidence.WorkflowRunURL == "" {
		return VerifyMainHotfixDeliveryResult{}, invalidWorkflowInput(
			"main hotfix delivery evidence must bind the patch tag, merge, release, and artifact workflow",
			"wait for the immutable patch delivery to complete before marking the hotfix delivered",
		)
	}
	return VerifyMainHotfixDeliveryResult{Record: record, Evidence: evidence}, nil
}

func (service *ReleaseService) validatedMainHotfixRecord(
	ctx context.Context,
	repository port.RepositoryIdentity,
	name branch.BranchName,
	location string,
) (hotfix.ReleaseRecord, port.RepositoryIdentity, error) {
	normalized, err := normalizeWorkflowRepository(repository)
	if err != nil {
		return hotfix.ReleaseRecord{}, port.RepositoryIdentity{}, err
	}
	result, err := service.ValidateMainHotfixRecord(ctx, ValidateMainHotfixRecordRequest{
		Repository: normalized,
		Branch:     name,
		Location:   location,
	})
	if err != nil {
		return hotfix.ReleaseRecord{}, port.RepositoryIdentity{}, err
	}
	return result.Record, normalized, nil
}

// CutReleaseRequest describes an intentional release cut from develop.
type CutReleaseRequest struct {
	Repository port.RepositoryIdentity
	Version    branch.SemanticVersion
	DryRun     bool
}

// RequestProtectedLineRequest describes a request-controller authorization for
// one release or support-line mutation.
type RequestProtectedLineRequest struct {
	Repository  port.RepositoryIdentity
	Ticket      ticket.ID
	Operation   releaserequest.Operation
	Version     string
	Requester   string
	ParentRunID string
	DryRun      bool
}

// RequestProtectedLineResult exposes the durable request record and the
// derived protected-line intent. It deliberately does not claim that the
// protected line already exists.
type RequestProtectedLineResult struct {
	Intent  SharedLineIntent
	Request port.ProtectedLineRequestResult
	DryRun  bool
}

// RequestProtectedLine authorizes and persists one request before dispatching
// the execution workflow. The request controller cannot mutate a shared line.
func (service *ReleaseService) RequestProtectedLine(
	ctx context.Context,
	request RequestProtectedLineRequest,
) (RequestProtectedLineResult, error) {
	if service.git == nil {
		return RequestProtectedLineResult{}, internalDependencyError("Git repository")
	}
	if service.protectedRequests == nil {
		return RequestProtectedLineResult{}, internalDependencyError("protected-line request provider")
	}
	if request.Ticket.Key().String() == "" || request.Ticket.Number().String() == "" {
		return RequestProtectedLineResult{}, invalidWorkflowInput(
			"a ticket-bound release request",
			"bind the protected-line request to its governing ticket before authorization",
		)
	}
	repository, err := normalizeWorkflowRepository(request.Repository)
	if err != nil {
		return RequestProtectedLineResult{}, err
	}
	intent, err := service.protectedLineIntent(ctx, repository, request.Operation, request.Version, request.DryRun)
	if err != nil {
		return RequestProtectedLineResult{}, err
	}
	if request.DryRun {
		return RequestProtectedLineResult{Intent: intent, DryRun: true}, nil
	}
	remoteURL, err := service.git.RemoteURL(ctx, repository)
	if err != nil {
		return RequestProtectedLineResult{}, err
	}
	authorized, err := service.protectedRequests.AuthorizeProtectedLineRequest(ctx, port.ProtectedLineRequestAuthorization{
		Repository:  repository,
		RemoteURL:   remoteURL,
		Ticket:      request.Ticket,
		Operation:   request.Operation,
		Version:     request.Version,
		Source:      intent.Source.Branch(),
		Target:      intent.Branch,
		Requester:   request.Requester,
		ParentRunID: request.ParentRunID,
	})
	if err != nil {
		return RequestProtectedLineResult{}, err
	}
	return RequestProtectedLineResult{Intent: intent, Request: authorized}, nil
}

// AuthorizeProtectedLineExecution validates the durable request immediately
// before the executor is permitted to mutate its one protected target.
func (service *ReleaseService) AuthorizeProtectedLineExecution(
	ctx context.Context,
	repository port.RepositoryIdentity,
	requestID string,
	executorRunID string,
) (port.ProtectedLineExecutionPlan, error) {
	if service.git == nil {
		return port.ProtectedLineExecutionPlan{}, internalDependencyError("Git repository")
	}
	if service.protectedRequests == nil {
		return port.ProtectedLineExecutionPlan{}, internalDependencyError("protected-line request provider")
	}
	normalized, err := normalizeWorkflowRepository(repository)
	if err != nil {
		return port.ProtectedLineExecutionPlan{}, err
	}
	remoteURL, err := service.git.RemoteURL(ctx, normalized)
	if err != nil {
		return port.ProtectedLineExecutionPlan{}, err
	}
	return service.protectedRequests.AuthorizeProtectedLineExecution(ctx, port.ProtectedLineExecutionAuthorization{
		Repository:    normalized,
		RemoteURL:     remoteURL,
		RequestID:     requestID,
		ExecutorRunID: executorRunID,
	})
}

// FinalizeProtectedLineRequest performs a read-only provider verification of
// one correlated execution and persists only its final audit state.
func (service *ReleaseService) FinalizeProtectedLineRequest(
	ctx context.Context,
	repository port.RepositoryIdentity,
	requestID string,
	executorRunID string,
	recovery bool,
) (port.ProtectedLineFinalizationResult, error) {
	if service.git == nil {
		return port.ProtectedLineFinalizationResult{}, internalDependencyError("Git repository")
	}
	if service.protectedRequests == nil {
		return port.ProtectedLineFinalizationResult{}, internalDependencyError("protected-line request provider")
	}
	normalized, err := normalizeWorkflowRepository(repository)
	if err != nil {
		return port.ProtectedLineFinalizationResult{}, err
	}
	remoteURL, err := service.git.RemoteURL(ctx, normalized)
	if err != nil {
		return port.ProtectedLineFinalizationResult{}, err
	}
	return service.protectedRequests.FinalizeProtectedLineRequest(ctx, port.ProtectedLineFinalizationRequest{
		Repository:    normalized,
		RemoteURL:     remoteURL,
		RequestID:     requestID,
		ExecutorRunID: executorRunID,
		Recovery:      recovery,
	})
}

func (service *ReleaseService) protectedLineIntent(
	ctx context.Context,
	repository port.RepositoryIdentity,
	operation releaserequest.Operation,
	version string,
	dryRun bool,
) (SharedLineIntent, error) {
	switch operation {
	case releaserequest.OperationRelease:
		releaseVersion, err := branch.ParseSemanticVersion(version)
		if err != nil {
			return SharedLineIntent{}, err
		}
		result, err := service.CutRelease(ctx, CutReleaseRequest{
			Repository: repository,
			Version:    releaseVersion,
			DryRun:     dryRun,
		})
		if err != nil {
			return SharedLineIntent{}, err
		}
		return result.Intent, nil
	case releaserequest.OperationSupport:
		supportVersion, err := branch.ParseSupportVersion(version)
		if err != nil {
			return SharedLineIntent{}, err
		}
		result, err := service.PrepareSupport(ctx, PrepareSupportRequest{
			Repository: repository,
			Version:    supportVersion,
			DryRun:     dryRun,
		})
		if err != nil {
			return SharedLineIntent{}, err
		}
		return result.Intent, nil
	default:
		return SharedLineIntent{}, invalidWorkflowInput(
			"a release or support protected-line operation",
			"select release for develop-derived cuts or support for a released main line",
		)
	}
}

// SharedLineIntent describes a privileged CI operation that creates a remote
// protected release or support line. The local CLI never pushes shared lines.
type SharedLineIntent struct {
	Workflow string            `json:"workflow"`
	Kind     string            `json:"kind"`
	Branch   branch.BranchName `json:"branch"`
	Source   branch.TargetBase `json:"source"`
	Inputs   map[string]string `json:"inputs"`
}

// SharedLineIntentResult contains the prepared CI/hosting operation and the
// read-only validation plan that produced it.
type SharedLineIntentResult struct {
	Intent SharedLineIntent     `json:"intent"`
	DryRun bool                 `json:"dryRun"`
	Plan   []branchapp.PlanStep `json:"plan"`
}

// CutRelease creates release/<semver> directly from origin/develop. It does
// not tag, publish artifacts, or merge into main; those are separate release
// approval and pipeline responsibilities.
func (service *ReleaseService) CutRelease(ctx context.Context, request CutReleaseRequest) (SharedLineIntentResult, error) {
	name, err := branch.NewReleaseBranch(request.Version)
	if err != nil {
		return SharedLineIntentResult{}, err
	}
	develop := mustDevelop()
	return service.prepareSharedLine(
		ctx,
		request.Repository,
		name,
		develop,
		"release",
		request.Version.String(),
		request.DryRun,
	)
}

// DispatchSharedLine delegates protected-line creation to the configured
// hosting provider, then verifies that the provider-created remote line is
// available as a fetched remote-tracking reference.
func (service *ReleaseService) DispatchSharedLine(
	ctx context.Context,
	repository port.RepositoryIdentity,
	intent SharedLineIntent,
) (port.SharedLineDispatchResult, error) {
	if service.git == nil {
		return port.SharedLineDispatchResult{}, internalDependencyError("Git repository")
	}
	if service.lifecycle == nil {
		return port.SharedLineDispatchResult{}, internalDependencyError("release lifecycle provider")
	}
	if intent.Branch.IsZero() || strings.TrimSpace(intent.Workflow) == "" || strings.TrimSpace(intent.Kind) == "" {
		return port.SharedLineDispatchResult{}, invalidWorkflowInput(
			"a complete protected shared-line intent is required",
			"prepare the release or support intent before requesting provider dispatch",
		)
	}
	normalized, err := normalizeWorkflowRepository(repository)
	if err != nil {
		return port.SharedLineDispatchResult{}, err
	}
	remoteURL, err := service.git.RemoteURL(ctx, normalized)
	if err != nil {
		return port.SharedLineDispatchResult{}, err
	}
	result, err := service.lifecycle.DispatchSharedLine(ctx, port.SharedLineDispatchRequest{
		Repository: normalized,
		RemoteURL:  remoteURL,
		Workflow:   intent.Workflow,
		Ref:        mustMain().String(),
		Inputs:     intent.Inputs,
		Branch:     intent.Branch,
	})
	if err != nil {
		return port.SharedLineDispatchResult{}, err
	}
	if result.Branch.IsZero() {
		result.Branch = intent.Branch
	}
	if result.Branch.String() != intent.Branch.String() {
		return port.SharedLineDispatchResult{}, invalidWorkflowInput(
			"the provider-created line must equal the requested protected line",
			"retry the dispatch with the exact release or support branch from the prepared intent",
		)
	}
	if err := service.git.Fetch(ctx, normalized); err != nil {
		return port.SharedLineDispatchResult{}, err
	}
	base, err := branch.NewTargetBase(normalized.Remote, intent.Branch)
	if err != nil {
		return port.SharedLineDispatchResult{}, err
	}
	exists, err := service.git.TargetBaseExists(ctx, normalized, base)
	if err != nil {
		return port.SharedLineDispatchResult{}, err
	}
	if !exists {
		return port.SharedLineDispatchResult{}, invalidWorkflowInput(
			"the provider workflow must create the requested protected remote line",
			"inspect the provider workflow result and verify the release or support branch exists on the selected remote",
		)
	}
	return result, nil
}

// PrepareSupportRequest describes a support-line creation from a released
// main-line version.
type PrepareSupportRequest struct {
	Repository port.RepositoryIdentity
	Version    branch.SupportVersion
	DryRun     bool
}

// PrepareSupport creates support/<major.minor> directly from origin/main only
// when that main revision carries a matching released version tag.
func (service *ReleaseService) PrepareSupport(ctx context.Context, request PrepareSupportRequest) (SharedLineIntentResult, error) {
	name, err := branch.NewSupportBranch(request.Version)
	if err != nil {
		return SharedLineIntentResult{}, err
	}
	repository, err := normalizeWorkflowRepository(request.Repository)
	if err != nil {
		return SharedLineIntentResult{}, err
	}
	main := mustMain()
	if !request.DryRun {
		if service.git == nil {
			return SharedLineIntentResult{}, internalDependencyError("Git repository")
		}
		if err := service.git.Fetch(ctx, repository); err != nil {
			return SharedLineIntentResult{}, err
		}
		tags, err := service.git.ReleaseTagsAt(ctx, repository, repository.Remote+"/"+main.String())
		if err != nil {
			return SharedLineIntentResult{}, err
		}
		if !hasMatchingSupportReleaseTag(tags, request.Version) {
			return SharedLineIntentResult{}, invalidWorkflowInput(
				"support lines can be created only from a released main revision with a matching v<major.minor.patch> tag",
				"release and tag the matching version on main before creating its support line",
			)
		}
	}
	return service.prepareSharedLine(
		ctx,
		repository,
		name,
		main,
		"support",
		request.Version.String(),
		request.DryRun,
	)
}

// ReleaseStabilizationKind constrains change categories allowed after a release
// line has been cut.
type ReleaseStabilizationKind string

const (
	ReleaseStabilizationBlocker ReleaseStabilizationKind = "blocker"
	ReleaseStabilizationDocs    ReleaseStabilizationKind = "docs"
	ReleaseStabilizationPrep    ReleaseStabilizationKind = "release-prep"
)

// ParseReleaseStabilizationKind validates the constrained release-change
// category before a workflow begins.
func ParseReleaseStabilizationKind(raw string) (ReleaseStabilizationKind, error) {
	kind := ReleaseStabilizationKind(raw)
	if _, err := stabilizationFamily(kind); err != nil {
		return "", err
	}
	return kind, nil
}

// CreateReleaseStabilizationRequest describes an explicitly permitted short
// working branch from a frozen release line.
type CreateReleaseStabilizationRequest struct {
	Repository port.RepositoryIdentity
	Release    branch.BranchName
	Ticket     ticket.ID
	Slug       branch.Slug
	Kind       ReleaseStabilizationKind
	Switch     *bool
	DryRun     bool
}

// CreateReleaseStabilization creates a controlled fix, docs, or chore branch
// from origin/release/<semver>. New features and refactors are deliberately
// not expressible through this workflow.
func (service *ReleaseService) CreateReleaseStabilization(ctx context.Context, request CreateReleaseStabilizationRequest) (branchapp.CreateResult, error) {
	if service.branches == nil {
		return branchapp.CreateResult{}, internalDependencyError("branch service")
	}
	if request.Release.Family() != branch.FamilyRelease {
		return branchapp.CreateResult{}, invalidWorkflowInput(
			"release stabilization requires a release/<semver> line",
			"select the frozen release line that contains the blocker or release task",
		)
	}
	family, err := stabilizationFamily(request.Kind)
	if err != nil {
		return branchapp.CreateResult{}, err
	}
	repository, err := normalizeWorkflowRepository(request.Repository)
	if err != nil {
		return branchapp.CreateResult{}, err
	}
	base, err := branch.NewTargetBase(repository.Remote, request.Release)
	if err != nil {
		return branchapp.CreateResult{}, err
	}
	if !request.DryRun && service.git == nil {
		return branchapp.CreateResult{}, internalDependencyError("Git repository")
	}
	result, err := service.branches.Create(ctx, branchapp.CreateRequest{
		Repository:      repository,
		Family:          family,
		Ticket:          request.Ticket,
		Slug:            request.Slug,
		Base:            &base,
		Switch:          request.Switch,
		DryRun:          request.DryRun,
		WorkflowManaged: true,
	})
	if err != nil {
		return branchapp.CreateResult{}, err
	}
	if !request.DryRun {
		if err := service.git.StoreWorkflowBase(ctx, repository, result.Name, base); err != nil {
			return branchapp.CreateResult{}, err
		}
	}
	return result, nil
}

// PrepareReleasePromotionRequest describes a provider-neutral release-to-main
// pull request after release stabilization and approval. Body carries the
// mandatory pull-request description required by
// docs/conventions/pull-requests/description-mandate.md whenever
// CreatePullRequest is set.
type PrepareReleasePromotionRequest struct {
	Repository        port.RepositoryIdentity
	Release           branch.BranchName
	CreatePullRequest bool
	Draft             bool
	Body              string
	DryRun            bool
}

// PrepareReleasePromotionResult exposes the release-to-main pull request
// intent and optional provider result.
type PrepareReleasePromotionResult struct {
	PullRequest  port.PullRequest
	PublishedURL string
	DryRun       bool
}

// PrepareReleasePromotion prepares release/<semver> -> main. It does not tag,
// merge, or publish artifacts; those remain protected CI and hosting actions.
func (service *ReleaseService) PrepareReleasePromotion(ctx context.Context, request PrepareReleasePromotionRequest) (PrepareReleasePromotionResult, error) {
	if request.Release.Family() != branch.FamilyRelease {
		return PrepareReleasePromotionResult{}, invalidWorkflowInput(
			"release promotion requires a release/<semver> branch",
			"select the frozen release line approved for promotion",
		)
	}
	if request.CreatePullRequest && strings.TrimSpace(request.Body) == "" {
		return PrepareReleasePromotionResult{}, pullRequestBodyRequired()
	}
	repository, err := normalizeWorkflowRepository(request.Repository)
	if err != nil {
		return PrepareReleasePromotionResult{}, err
	}
	version, _ := request.Release.ReleaseVersion()
	result := PrepareReleasePromotionResult{
		PullRequest: port.PullRequest{
			Source: request.Release,
			Target: mustMain(),
			Title:  "Release " + version.String() + " into main",
			Body:   request.Body,
			Draft:  request.Draft,
		},
		DryRun: request.DryRun,
	}
	if request.DryRun || !request.CreatePullRequest {
		return result, nil
	}
	publishedURL, err := publishPullRequest(ctx, service.git, service.publisher, repository, result.PullRequest)
	if err != nil {
		return PrepareReleasePromotionResult{}, err
	}
	result.PublishedURL = publishedURL
	return result, nil
}

// PrepareReleaseBackmergeRequest describes provider-neutral release backmerge
// preparation after a release has been approved. Body carries the mandatory
// pull-request description required by
// docs/conventions/pull-requests/description-mandate.md whenever
// CreatePullRequest is set.
type PrepareReleaseBackmergeRequest struct {
	Repository        port.RepositoryIdentity
	Release           branch.BranchName
	CreatePullRequest bool
	Draft             bool
	Body              string
	DryRun            bool
}

// PrepareReleaseBackmergeResult exposes the PR intent and optional published
// URL. The workflow never directly mutates develop.
type PrepareReleaseBackmergeResult struct {
	PullRequest  port.PullRequest
	PublishedURL string
	DryRun       bool
}

// ReleaseBackmergeStatus distinguishes a dry-run plan, an actionable
// backmerge, and an audited no-op result.
type ReleaseBackmergeStatus string

const (
	ReleaseBackmergePlanned     ReleaseBackmergeStatus = "planned"
	ReleaseBackmergeRequired    ReleaseBackmergeStatus = "required"
	ReleaseBackmergeNotRequired ReleaseBackmergeStatus = "not-required"
)

// AssessReleaseBackmergeRequest describes the delivery-gated reconciliation
// decision for a completed release line. Body carries the mandatory
// pull-request description required by
// docs/conventions/pull-requests/description-mandate.md for the case that the
// assessment yields a required backmerge pull request.
type AssessReleaseBackmergeRequest struct {
	Repository port.RepositoryIdentity
	Release    branch.BranchName
	Draft      bool
	Body       string
	DryRun     bool
}

// AssessReleaseBackmergeResult captures the provider-verified delivery
// evidence and, only when needed, the pull-request intent.
type AssessReleaseBackmergeResult struct {
	Status      ReleaseBackmergeStatus
	Evidence    port.ReleaseReconciliationEvidence
	PullRequest *port.PullRequest
	DryRun      bool
}

// PrepareReleaseBackmerge prepares release/<semver> -> develop.
func (service *ReleaseService) PrepareReleaseBackmerge(ctx context.Context, request PrepareReleaseBackmergeRequest) (PrepareReleaseBackmergeResult, error) {
	if request.Release.Family() != branch.FamilyRelease {
		return PrepareReleaseBackmergeResult{}, invalidWorkflowInput(
			"release backmerge requires a release/<semver> branch",
			"select the completed release branch to merge back into develop",
		)
	}
	if request.CreatePullRequest && strings.TrimSpace(request.Body) == "" {
		return PrepareReleaseBackmergeResult{}, pullRequestBodyRequired()
	}
	repository, err := normalizeWorkflowRepository(request.Repository)
	if err != nil {
		return PrepareReleaseBackmergeResult{}, err
	}
	pullRequest := releaseBackmergePullRequest(request.Release, request.Draft, request.Body)
	result := PrepareReleaseBackmergeResult{
		PullRequest: pullRequest,
		DryRun:      request.DryRun,
	}
	if request.DryRun || !request.CreatePullRequest {
		return result, nil
	}
	publishedURL, err := publishPullRequest(ctx, service.git, service.publisher, repository, pullRequest)
	if err != nil {
		return PrepareReleaseBackmergeResult{}, err
	}
	result.PublishedURL = publishedURL
	return result, nil
}

// AssessReleaseBackmerge verifies the causal release-delivery gate, then
// creates a backmerge intent only when the hosting provider reports an
// effective release-only delta for develop.
func (service *ReleaseService) AssessReleaseBackmerge(
	ctx context.Context,
	request AssessReleaseBackmergeRequest,
) (AssessReleaseBackmergeResult, error) {
	if request.Release.Family() != branch.FamilyRelease {
		return AssessReleaseBackmergeResult{}, invalidWorkflowInput(
			"release backmerge requires a release/<semver> branch",
			"select the completed release branch to reconcile with develop",
		)
	}
	repository, err := normalizeWorkflowRepository(request.Repository)
	if err != nil {
		return AssessReleaseBackmergeResult{}, err
	}
	prepared := PrepareReleaseBackmergeResult{
		PullRequest: releaseBackmergePullRequest(request.Release, request.Draft, request.Body),
		DryRun:      request.DryRun,
	}
	if request.DryRun {
		return AssessReleaseBackmergeResult{
			Status:      ReleaseBackmergePlanned,
			PullRequest: &prepared.PullRequest,
			DryRun:      true,
		}, nil
	}
	if service.git == nil {
		return AssessReleaseBackmergeResult{}, internalDependencyError("Git repository")
	}
	if service.lifecycle == nil {
		return AssessReleaseBackmergeResult{}, internalDependencyError("release lifecycle provider")
	}
	remoteURL, err := service.git.RemoteURL(ctx, repository)
	if err != nil {
		return AssessReleaseBackmergeResult{}, err
	}
	evidence, err := service.lifecycle.VerifyReleaseReconciliation(ctx, port.ReleaseReconciliationRequest{
		Repository: repository,
		RemoteURL:  remoteURL,
		Release:    request.Release,
	})
	if err != nil {
		return AssessReleaseBackmergeResult{}, err
	}
	version, _ := request.Release.ReleaseVersion()
	if evidence.PromotionMergeCommit == "" || evidence.Tag != "v"+version.String() || evidence.ReleaseURL == "" {
		return AssessReleaseBackmergeResult{}, invalidWorkflowInput(
			"the release-to-main merge, exact immutable tag, and published release evidence are required before reconciliation",
			"complete release delivery and retry the release backmerge assessment",
		)
	}
	if !evidence.EffectiveDelta {
		return AssessReleaseBackmergeResult{
			Status:   ReleaseBackmergeNotRequired,
			Evidence: evidence,
		}, nil
	}
	return AssessReleaseBackmergeResult{
		Status:      ReleaseBackmergeRequired,
		Evidence:    evidence,
		PullRequest: &prepared.PullRequest,
	}, nil
}

func releaseBackmergePullRequest(release branch.BranchName, draft bool, body string) port.PullRequest {
	releaseVersion, _ := release.ReleaseVersion()
	return port.PullRequest{
		Source: release,
		Target: mustDevelop(),
		Title:  "Backmerge release " + releaseVersion.String() + " into develop",
		Body:   body,
		Draft:  draft,
	}
}

// PropagateHotfixRequest describes an explicit forward-port or backport of one
// already-reviewed hotfix commit into another active line. Body carries the
// mandatory pull-request description required by
// docs/conventions/pull-requests/description-mandate.md whenever
// CreatePullRequest is set.
type PropagateHotfixRequest struct {
	Repository        port.RepositoryIdentity
	Source            branch.BranchName
	TargetLine        branch.BranchName
	CommitID          string
	Slug              branch.Slug
	Push              bool
	CreatePullRequest bool
	Draft             bool
	Body              string
	DryRun            bool
}

// PropagateHotfixResult describes the derived fix branch, cherry-pick, and
// provider-neutral pull request intent.
type PropagateHotfixResult struct {
	Branch       branchapp.CreateResult
	CherryPicked bool
	Publication  PublishTicketResult
}

// PropagateHotfix creates a short-lived fix branch from the target line,
// cherry-picks the requested commit with -x, and prepares the resulting pull
// request. The workflow never assumes that a hotfix automatically reaches
// another active line.
func (service *ReleaseService) PropagateHotfix(ctx context.Context, request PropagateHotfixRequest) (PropagateHotfixResult, error) {
	if service.branches == nil || service.git == nil || service.tickets == nil {
		return PropagateHotfixResult{}, internalDependencyError("hotfix propagation services")
	}
	if request.Source.Family() != branch.FamilyHotfix {
		return PropagateHotfixResult{}, invalidWorkflowInput(
			"hotfix propagation requires a hotfix/<ticket>-<slug> source branch",
			"select the reviewed hotfix branch that contains the commit to propagate",
		)
	}
	switch request.TargetLine.Family() {
	case branch.FamilyMain, branch.FamilyDevelop, branch.FamilyRelease, branch.FamilySupport:
	default:
		return PropagateHotfixResult{}, invalidWorkflowInput(
			"hotfix propagation targets main, develop, release/<semver>, or support/<major.minor>",
			"select the active line that also needs the reviewed hotfix",
		)
	}
	if err := ValidateCommitID(request.CommitID); err != nil {
		return PropagateHotfixResult{}, err
	}
	if request.CreatePullRequest && strings.TrimSpace(request.Body) == "" {
		return PropagateHotfixResult{}, pullRequestBodyRequired()
	}
	repository, err := normalizeWorkflowRepository(request.Repository)
	if err != nil {
		return PropagateHotfixResult{}, err
	}
	sourceTicket, _ := request.Source.Ticket()
	slug := request.Slug
	if slug.String() == "" {
		sourceSlug, _ := request.Source.Slug()
		slug, err = branch.ParseSlug("forward-port-" + sourceSlug.String())
		if err != nil {
			return PropagateHotfixResult{}, err
		}
	}
	base, err := branch.NewTargetBase(repository.Remote, request.TargetLine)
	if err != nil {
		return PropagateHotfixResult{}, err
	}
	switchToBranch := true
	created, err := service.branches.Create(ctx, branchapp.CreateRequest{
		Repository:      repository,
		Family:          branch.FamilyFix,
		Ticket:          sourceTicket,
		Slug:            slug,
		Base:            &base,
		Switch:          &switchToBranch,
		DryRun:          request.DryRun,
		WorkflowManaged: true,
	})
	if err != nil {
		return PropagateHotfixResult{}, err
	}
	result := PropagateHotfixResult{Branch: created}
	if request.DryRun {
		result.Publication = PublishTicketResult{
			Branch: created.Name,
			PullRequest: port.PullRequest{
				Source: created.Name,
				Target: request.TargetLine,
				Ticket: sourceTicket,
				Title:  sourceTicket.String() + ": " + slug.String(),
				Body:   request.Body,
				Draft:  request.Draft,
			},
			DryRun: true,
		}
		return result, nil
	}
	if err := service.git.StoreWorkflowBase(ctx, repository, created.Name, base); err != nil {
		return PropagateHotfixResult{}, err
	}
	if err := service.git.CherryPick(ctx, repository, request.CommitID); err != nil {
		return PropagateHotfixResult{}, service.classifyCherryPickFailure(ctx, repository, err)
	}
	result.CherryPicked = true
	target := request.TargetLine
	publication, err := service.tickets.PublishTicket(ctx, PublishTicketRequest{
		Repository:        repository,
		Branch:            created.Name,
		Base:              &base,
		Target:            &target,
		WorkflowManaged:   true,
		Push:              request.Push,
		CreatePullRequest: request.CreatePullRequest,
		Draft:             request.Draft,
		Body:              request.Body,
	})
	if err != nil {
		return PropagateHotfixResult{}, err
	}
	result.Publication = publication
	return result, nil
}

// PropagateHotfixManifestRequest describes the local preparation of one
// ordered, reviewed multi-commit propagation candidate. Publication remains
// owned by the separate server-side hotfix publisher boundary.
type PropagateHotfixManifestRequest struct {
	Repository port.RepositoryIdentity
	Source     branch.BranchName
	TargetLine branch.BranchName
	Location   string
	Slug       branch.Slug
	Publish    bool
	DryRun     bool
}

// PropagateHotfixManifestResult records a declared propagation candidate. Local
// callers prepare without publication; the dedicated server-side publisher can
// additionally publish the validated candidate.
type PropagateHotfixManifestResult struct {
	Branch          branchapp.CreateResult
	Record          hotfix.ReleaseRecord
	CherryPickCount int
	Quality         *port.QualityResult
	Publication     *PublishTicketResult
	DryRun          bool
}

// PropagateHotfixManifest creates a target-derived fix branch and applies the
// exact ordered full-SHA manifest under a resumable local cursor. Publication
// is available only when the service was composed for the dedicated hotfix
// propagation publisher boundary.
func (service *ReleaseService) PropagateHotfixManifest(
	ctx context.Context,
	request PropagateHotfixManifestRequest,
) (PropagateHotfixManifestResult, error) {
	if service.branches == nil || service.git == nil || service.quality == nil {
		return PropagateHotfixManifestResult{}, internalDependencyError("hotfix manifest propagation services")
	}
	if request.Publish && !service.manifestPublication {
		return PropagateHotfixManifestResult{}, hotfixManifestPublicationUnavailable()
	}
	if request.Publish && service.tickets == nil {
		return PropagateHotfixManifestResult{}, internalDependencyError("hotfix manifest publication service")
	}
	progressStore, ok := service.git.(port.HotfixManifestProgressStore)
	if !ok {
		return PropagateHotfixManifestResult{}, internalDependencyError("hotfix manifest propagation progress store")
	}
	record, repository, err := service.validatedMainHotfixRecord(ctx, request.Repository, request.Source, request.Location)
	if err != nil {
		return PropagateHotfixManifestResult{}, err
	}
	if err := validateManifestTarget(record, request.TargetLine); err != nil {
		return PropagateHotfixManifestResult{}, err
	}
	slug := resolveManifestPropagationSlug(request.Slug, request.TargetLine)
	base, _ := branch.NewTargetBase(repository.Remote, request.TargetLine)
	sourceTicket, _ := request.Source.Ticket()
	switchToBranch := true
	created, err := service.branches.Create(ctx, branchapp.CreateRequest{
		Repository:      repository,
		Family:          branch.FamilyFix,
		Ticket:          sourceTicket,
		Slug:            slug,
		Base:            &base,
		Switch:          &switchToBranch,
		DryRun:          request.DryRun,
		WorkflowManaged: true,
	})
	if err != nil {
		return PropagateHotfixManifestResult{}, err
	}
	result := PropagateHotfixManifestResult{
		Branch: created,
		Record: record,
		DryRun: request.DryRun,
	}
	if request.DryRun {
		return result, nil
	}
	if err := service.git.StoreWorkflowBase(ctx, repository, created.Name, base); err != nil {
		return PropagateHotfixManifestResult{}, err
	}
	progress := port.HotfixManifestProgress{
		Branch:   created.Name,
		Source:   request.Source,
		Target:   request.TargetLine,
		Manifest: record.Manifest(),
	}
	if err := progressStore.StoreHotfixManifestProgress(ctx, repository, progress); err != nil {
		return PropagateHotfixManifestResult{}, err
	}
	result, err = service.applyHotfixManifest(ctx, repository, record, progress, progressStore, result, !request.Publish)
	if err != nil {
		return PropagateHotfixManifestResult{}, err
	}
	if request.Publish {
		publication, err := service.publishHotfixManifestCandidate(ctx, repository, record, result.Branch.Name, request.TargetLine)
		if err != nil {
			return PropagateHotfixManifestResult{}, err
		}
		result.Publication = &publication
		result.Quality = &publication.Quality
	}
	return result, nil
}

// ResumeHotfixManifestPropagationRequest identifies an existing local
// candidate with a user-resolved active cherry-pick.
type ResumeHotfixManifestPropagationRequest struct {
	Repository port.RepositoryIdentity
	Source     branch.BranchName
	TargetLine branch.BranchName
	Branch     branch.BranchName
	Location   string
	Publish    bool
}

// ResumeHotfixManifestPropagation continues exactly the paused manifest item,
// then applies the remaining ordered commits and re-runs quality gates. Only
// the dedicated server-side publisher may publish the resumed candidate.
func (service *ReleaseService) ResumeHotfixManifestPropagation(
	ctx context.Context,
	request ResumeHotfixManifestPropagationRequest,
) (PropagateHotfixManifestResult, error) {
	if service.branches == nil || service.git == nil || service.quality == nil {
		return PropagateHotfixManifestResult{}, internalDependencyError("hotfix manifest propagation services")
	}
	if request.Publish && !service.manifestPublication {
		return PropagateHotfixManifestResult{}, hotfixManifestPublicationUnavailable()
	}
	if request.Publish && service.tickets == nil {
		return PropagateHotfixManifestResult{}, internalDependencyError("hotfix manifest publication service")
	}
	progressStore, ok := service.git.(port.HotfixManifestProgressStore)
	if !ok {
		return PropagateHotfixManifestResult{}, internalDependencyError("hotfix manifest propagation progress store")
	}
	record, repository, err := service.validatedMainHotfixRecord(ctx, request.Repository, request.Source, request.Location)
	if err != nil {
		return PropagateHotfixManifestResult{}, err
	}
	if err := validateManifestTarget(record, request.TargetLine); err != nil {
		return PropagateHotfixManifestResult{}, err
	}
	if request.Branch.Family() != branch.FamilyFix {
		return PropagateHotfixManifestResult{}, invalidWorkflowInput(
			"hotfix manifest propagation resumption requires a generated fix branch",
			"provide the exact target-derived fix branch created by workflow hotfix propagate-manifest",
		)
	}
	current, err := service.git.CurrentBranch(ctx, repository)
	if err != nil {
		return PropagateHotfixManifestResult{}, err
	}
	if current.String() != request.Branch.String() {
		return PropagateHotfixManifestResult{}, invalidWorkflowInput(
			"hotfix manifest propagation may resume only on the checked-out candidate branch",
			"switch to the generated fix branch before resuming the resolved cherry-pick",
		)
	}
	progress, found, err := progressStore.LoadHotfixManifestProgress(ctx, repository)
	if err != nil {
		return PropagateHotfixManifestResult{}, err
	}
	if !found || !matchesManifestProgress(progress, request, record.Manifest()) {
		return PropagateHotfixManifestResult{}, invalidWorkflowInput(
			"hotfix manifest propagation resumption requires matching local progress metadata",
			"resume the original candidate with its exact source, target, branch, and reviewed manifest",
		)
	}
	operation, active, err := service.git.ActiveOperation(ctx, repository)
	if err != nil {
		return PropagateHotfixManifestResult{}, err
	}
	if !active || operation != "cherry-pick" {
		return PropagateHotfixManifestResult{}, invalidWorkflowInput(
			"hotfix manifest propagation can resume only an in-progress resolved cherry-pick",
			"resolve and stage the current conflict without changing the manifest, then resume immediately",
		)
	}
	continuator, ok := service.git.(port.CherryPickContinuator)
	if !ok {
		return PropagateHotfixManifestResult{}, internalDependencyError("cherry-pick continuator")
	}
	if err := continuator.ContinueCherryPick(ctx, repository); err != nil {
		return PropagateHotfixManifestResult{}, service.classifyCherryPickFailure(ctx, repository, err)
	}
	progress.Next++
	if err := progressStore.StoreHotfixManifestProgress(ctx, repository, progress); err != nil {
		return PropagateHotfixManifestResult{}, err
	}
	base, _ := branch.NewTargetBase(repository.Remote, request.TargetLine)
	result := PropagateHotfixManifestResult{
		Branch: branchapp.CreateResult{
			Name: request.Branch,
			Base: base,
		},
		Record:          record,
		CherryPickCount: progress.Next,
	}
	result, err = service.applyHotfixManifest(ctx, repository, record, progress, progressStore, result, !request.Publish)
	if err != nil {
		return PropagateHotfixManifestResult{}, err
	}
	if request.Publish {
		publication, err := service.publishHotfixManifestCandidate(ctx, repository, record, result.Branch.Name, request.TargetLine)
		if err != nil {
			return PropagateHotfixManifestResult{}, err
		}
		result.Publication = &publication
		result.Quality = &publication.Quality
	}
	return result, nil
}

func (service *ReleaseService) applyHotfixManifest(
	ctx context.Context,
	repository port.RepositoryIdentity,
	record hotfix.ReleaseRecord,
	progress port.HotfixManifestProgress,
	progressStore port.HotfixManifestProgressStore,
	result PropagateHotfixManifestResult,
	runQuality bool,
) (PropagateHotfixManifestResult, error) {
	for index := progress.Next; index < len(progress.Manifest); index++ {
		if err := service.git.CherryPick(ctx, repository, progress.Manifest[index]); err != nil {
			return PropagateHotfixManifestResult{}, service.classifyCherryPickFailure(ctx, repository, err)
		}
		progress.Next = index + 1
		if err := progressStore.StoreHotfixManifestProgress(ctx, repository, progress); err != nil {
			return PropagateHotfixManifestResult{}, err
		}
		result.CherryPickCount = progress.Next
	}
	if err := progressStore.ClearHotfixManifestProgress(ctx, repository); err != nil {
		return PropagateHotfixManifestResult{}, err
	}
	if _, err := service.branches.Validate(ctx, branchapp.ValidateRequest{
		Repository: repository,
		Name:       result.Branch.Name,
	}); err != nil {
		return PropagateHotfixManifestResult{}, err
	}
	if !runQuality {
		result.Record = record
		return result, nil
	}
	quality, err := service.quality.Run(ctx, repository, port.QualityRequest{
		Families: []branch.Family{result.Branch.Name.Family()},
	})
	if err != nil {
		return PropagateHotfixManifestResult{}, err
	}
	result.Quality = &quality
	result.Record = record
	return result, nil
}

func (service *ReleaseService) publishHotfixManifestCandidate(
	ctx context.Context,
	repository port.RepositoryIdentity,
	record hotfix.ReleaseRecord,
	candidate branch.BranchName,
	target branch.BranchName,
) (PublishTicketResult, error) {
	if !service.manifestPublication {
		return PublishTicketResult{}, hotfixManifestPublicationUnavailable()
	}
	if service.tickets == nil {
		return PublishTicketResult{}, internalDependencyError("hotfix manifest publication service")
	}
	if !service.tickets.HasPullRequestPublisher() {
		return PublishTicketResult{}, pullRequestPublisherUnavailable()
	}
	base, err := branch.NewTargetBase(repository.Remote, target)
	if err != nil {
		return PublishTicketResult{}, err
	}
	body := hotfixManifestPullRequestBody(record, target)
	pullRequest := newTicketPullRequest(candidate, target, false, body)
	if err := service.tickets.PreflightPullRequest(ctx, repository, pullRequest); err != nil {
		return PublishTicketResult{}, err
	}
	return service.tickets.PublishTicket(ctx, PublishTicketRequest{
		Repository:        repository,
		Branch:            candidate,
		Base:              &base,
		Target:            &target,
		WorkflowManaged:   true,
		Push:              true,
		CreatePullRequest: true,
		Body:              body,
	})
}

// hotfixManifestPullRequestBody composes the mandatory pull-request
// description from the reviewed release record; the record is the content
// source of truth for the server-side propagation publisher boundary. The
// canonical section layout is owned by
// docs/conventions/pull-requests/description-mandate.md.
func hotfixManifestPullRequestBody(record hotfix.ReleaseRecord, target branch.BranchName) string {
	manifest := record.Manifest()
	series := make([]string, 0, len(manifest))
	for _, commitID := range manifest {
		series = append(series, "- "+commitID)
	}
	return "## Summary\n\nPropagate the reviewed hotfix " + record.Ticket().String() + " manifest into " + target.String() + ".\n\n" +
		"## Scope and Non-Goals\n\nApplies exactly the ordered reviewed manifest onto " + target.String() + "; no new changes are introduced.\n\n" +
		"## Commit Series\n\n" + strings.Join(series, "\n") + "\n\n" +
		"## Risk and Rollback\n\nControlled propagation of the reviewed hotfix for incident " + record.Incident() + "; rollback reverts the propagated series on " + target.String() + ".\n\n" +
		"## Verification and Review Focus\n\nThe reviewed release record, the ordered manifest, and the final quality gate bound the candidate; review the manifest order and the target-line selection."
}

func hotfixManifestPublicationUnavailable() error {
	return invalidWorkflowInput(
		"hotfix manifest candidate publication requires the dedicated server-side hotfix propagation publisher",
		"run the protected hotfix propagation controller; local manifest preparation remains non-publishing",
	)
}

func validateManifestTarget(record hotfix.ReleaseRecord, target branch.BranchName) error {
	switch target.Family() {
	case branch.FamilyDevelop, branch.FamilyRelease, branch.FamilySupport:
	default:
		return invalidWorkflowInput(
			"hotfix manifest propagation targets develop, release/<semver>, or support/<major.minor>",
			"select a declared additional active line instead of main or a working branch",
		)
	}
	for _, declared := range record.PropagationTargets() {
		if declared.String() == target.String() {
			return nil
		}
	}
	return invalidWorkflowInput(
		"hotfix manifest propagation target must be declared in the reviewed release record",
		"add the active target line to the reviewed record before preparing its propagation candidate",
	)
}

func resolveManifestPropagationSlug(value branch.Slug, target branch.BranchName) branch.Slug {
	if value.String() != "" {
		return value
	}
	normalized := strings.NewReplacer("/", "-", ".", "-").Replace(target.String())
	slug, _ := branch.ParseSlug("propagate-to-" + normalized)
	return slug
}

func matchesManifestProgress(
	progress port.HotfixManifestProgress,
	request ResumeHotfixManifestPropagationRequest,
	manifest []string,
) bool {
	if progress.Branch.String() != request.Branch.String() ||
		progress.Source.String() != request.Source.String() ||
		progress.Target.String() != request.TargetLine.String() ||
		len(progress.Manifest) != len(manifest) {
		return false
	}
	for index := range manifest {
		if progress.Manifest[index] != manifest[index] {
			return false
		}
	}
	return progress.Next >= 0 && progress.Next < len(manifest)
}

// ResumeHotfixPropagation continues a manually resolved cherry-pick and then
// resumes validation, optional push, and optional pull-request publication for
// the already-created propagation branch.
type ResumeHotfixPropagationRequest struct {
	Repository        port.RepositoryIdentity
	Source            branch.BranchName
	TargetLine        branch.BranchName
	Branch            branch.BranchName
	Push              bool
	CreatePullRequest bool
	Draft             bool
	Body              string
}

// ResumeHotfixPropagation continues only a known propagation branch whose
// stored workflow base matches the requested target line.
func (service *ReleaseService) ResumeHotfixPropagation(
	ctx context.Context,
	request ResumeHotfixPropagationRequest,
) (PropagateHotfixResult, error) {
	if service.branches == nil || service.git == nil || service.tickets == nil {
		return PropagateHotfixResult{}, internalDependencyError("hotfix propagation services")
	}
	if request.Source.Family() != branch.FamilyHotfix {
		return PropagateHotfixResult{}, invalidWorkflowInput(
			"hotfix propagation resumption requires the original hotfix source branch",
			"provide --source hotfix/<ticket>-<slug>",
		)
	}
	switch request.TargetLine.Family() {
	case branch.FamilyMain, branch.FamilyDevelop, branch.FamilyRelease, branch.FamilySupport:
	default:
		return PropagateHotfixResult{}, invalidWorkflowInput(
			"hotfix propagation resumption targets main, develop, release/<semver>, or support/<major.minor>",
			"provide the target line originally selected for the propagation",
		)
	}
	if request.Branch.Family() != branch.FamilyFix {
		return PropagateHotfixResult{}, invalidWorkflowInput(
			"hotfix propagation resumption requires the generated fix branch",
			"provide --branch fix/<ticket>-<slug>",
		)
	}
	sourceTicket, _ := request.Source.Ticket()
	branchTicket, hasBranchTicket := request.Branch.Ticket()
	if !hasBranchTicket || branchTicket.String() != sourceTicket.String() {
		return PropagateHotfixResult{}, invalidWorkflowInput(
			"the resumed propagation branch must carry the source hotfix ticket",
			"provide the fix branch created for the same hotfix ticket",
		)
	}
	if request.CreatePullRequest && !request.Push {
		return PropagateHotfixResult{}, invalidWorkflowInput(
			"pull-request creation requires an explicit propagation branch push",
			"set Push before requesting provider pull-request creation",
		)
	}
	if request.CreatePullRequest && strings.TrimSpace(request.Body) == "" {
		return PropagateHotfixResult{}, pullRequestBodyRequired()
	}
	repository, err := normalizeWorkflowRepository(request.Repository)
	if err != nil {
		return PropagateHotfixResult{}, err
	}
	base, err := branch.NewTargetBase(repository.Remote, request.TargetLine)
	if err != nil {
		return PropagateHotfixResult{}, err
	}
	storedBase, found, err := service.git.WorkflowBase(ctx, repository, request.Branch)
	if err != nil {
		return PropagateHotfixResult{}, err
	}
	if !found || storedBase.String() != base.String() {
		return PropagateHotfixResult{}, invalidWorkflowInput(
			"hotfix propagation resumption requires the recorded workflow base for the selected target line",
			"resume the original propagation branch with its original --target-line",
		)
	}
	operation, active, err := service.git.ActiveOperation(ctx, repository)
	if err != nil {
		return PropagateHotfixResult{}, err
	}
	if active {
		if operation != "cherry-pick" {
			return PropagateHotfixResult{}, invalidWorkflowInput(
				"hotfix propagation can resume only an in-progress cherry-pick",
				"complete or abort the active Git operation before resuming propagation",
			)
		}
		continuator, ok := service.git.(port.CherryPickContinuator)
		if !ok {
			return PropagateHotfixResult{}, internalDependencyError("cherry-pick continuator")
		}
		if err := continuator.ContinueCherryPick(ctx, repository); err != nil {
			return PropagateHotfixResult{}, service.classifyCherryPickFailure(ctx, repository, err)
		}
	}
	target := request.TargetLine
	publication, err := service.tickets.PublishTicket(ctx, PublishTicketRequest{
		Repository:        repository,
		Branch:            request.Branch,
		Base:              &base,
		Target:            &target,
		WorkflowManaged:   true,
		Push:              request.Push,
		CreatePullRequest: request.CreatePullRequest,
		Draft:             request.Draft,
		Body:              request.Body,
	})
	if err != nil {
		return PropagateHotfixResult{}, err
	}
	return PropagateHotfixResult{
		Branch: branchapp.CreateResult{
			Name: request.Branch,
			Base: base,
		},
		CherryPicked: true,
		Publication:  publication,
	}, nil
}

func (service *ReleaseService) classifyCherryPickFailure(
	ctx context.Context,
	repository port.RepositoryIdentity,
	cause error,
) error {
	operation, active, err := service.git.ActiveOperation(ctx, repository)
	if err != nil || !active || operation != "cherry-pick" {
		return cause
	}
	return problem.Wrap(problem.Details{
		Code:        problem.CodeCherryPickConflict,
		Category:    problem.CategoryGit,
		Field:       "cherry-pick",
		Expected:    "a completed cherry-pick without unresolved conflicts",
		Rule:        "hotfix propagation pauses while Git requires manual conflict resolution",
		Example:     "resolve conflicts, stage the resolutions, then rerun workflow hotfix propagate --resume",
		Remediation: "resolve and stage every conflicting file, then resume the existing propagation branch",
	}, cause)
}

// CleanupBranchRequest describes a local cleanup. Remote branch retention and
// deletion remain hosting or CI responsibilities.
type CleanupBranchRequest struct {
	Repository port.RepositoryIdentity
	Branch     branch.BranchName
	DryRun     bool
}

// CleanupBranchResult records the local cleanup and metadata removal outcome.
type CleanupBranchResult struct {
	Branch          branch.BranchName
	DeletedLocal    bool
	MetadataCleared bool
	DryRun          bool
}

// CleanupBranch removes a local private scratch branch. It never deletes remote
// branches or official working branches because their lifecycle belongs to
// hosting and CI automation.
func (service *ReleaseService) CleanupBranch(ctx context.Context, request CleanupBranchRequest) (CleanupBranchResult, error) {
	if service.git == nil {
		return CleanupBranchResult{}, internalDependencyError("Git repository")
	}
	family := request.Branch.Family()
	if family != branch.FamilyScratch {
		return CleanupBranchResult{}, invalidWorkflowInput(
			"cleanup accepts only a private scratch branch",
			"let GitHub, GitLab, or CI own every official branch lifecycle; use the CLI only to delete a local scratch branch",
		)
	}
	repository, err := normalizeWorkflowRepository(request.Repository)
	if err != nil {
		return CleanupBranchResult{}, err
	}
	result := CleanupBranchResult{
		Branch: request.Branch,
		DryRun: request.DryRun,
	}
	if request.DryRun {
		return result, nil
	}
	if err := service.git.DeleteLocalBranch(ctx, repository, request.Branch, true); err != nil {
		return CleanupBranchResult{}, err
	}
	result.DeletedLocal = true
	if err := service.git.ClearWorkflowBase(ctx, repository, request.Branch); err != nil {
		return CleanupBranchResult{}, err
	}
	result.MetadataCleared = true
	return result, nil
}

func stabilizationFamily(kind ReleaseStabilizationKind) (branch.Family, error) {
	switch kind {
	case ReleaseStabilizationBlocker:
		return branch.FamilyFix, nil
	case ReleaseStabilizationDocs:
		return branch.FamilyDocs, nil
	case ReleaseStabilizationPrep:
		return branch.FamilyChore, nil
	default:
		return "", invalidReleaseStabilizationKind(string(kind))
	}
}

func invalidReleaseStabilizationKind(actual string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeInvalidInput,
		Category:    problem.CategoryGovernance,
		Field:       "stabilization kind",
		Actual:      actual,
		Expected:    "blocker, docs, or release-prep",
		Rule:        "frozen release lines accept only release-blocking fixes, documentation, or release preparation",
		Example:     "blocker",
		Remediation: "select blocker, docs, or release-prep",
	})
}

func hasMatchingSupportReleaseTag(tags []string, version branch.SupportVersion) bool {
	for _, tag := range tags {
		raw := strings.TrimPrefix(tag, "v")
		semantic, err := branch.ParseSemanticVersion(raw)
		if err == nil && strings.HasPrefix(semantic.String(), version.String()+".") {
			return true
		}
	}
	return false
}

func (service *ReleaseService) prepareSharedLine(
	ctx context.Context,
	identity port.RepositoryIdentity,
	name branch.BranchName,
	baseName branch.BranchName,
	lineKind string,
	version string,
	dryRun bool,
) (SharedLineIntentResult, error) {
	if service.git == nil {
		return SharedLineIntentResult{}, internalDependencyError("Git repository")
	}
	repository, err := normalizeWorkflowRepository(identity)
	if err != nil {
		return SharedLineIntentResult{}, err
	}
	if err := service.git.ValidateBranchRef(ctx, repository, name); err != nil {
		return SharedLineIntentResult{}, err
	}
	base, err := branch.NewTargetBase(repository.Remote, baseName)
	if err != nil {
		return SharedLineIntentResult{}, err
	}
	result := SharedLineIntentResult{
		Intent: SharedLineIntent{
			Workflow: "execute-protected-line-request.yml",
			Kind:     lineKind,
			Branch:   name,
			Source:   base,
			Inputs: map[string]string{
				"kind":    lineKind,
				"version": version,
			},
		},
		DryRun: dryRun,
		Plan: []branchapp.PlanStep{
			{Action: "fetch", Detail: "git fetch --prune " + repository.Remote},
			{Action: "dispatch", Detail: "authorized CI creates " + name.String() + " from " + base.String()},
		},
	}
	if dryRun {
		return result, nil
	}
	if err := service.git.Fetch(ctx, repository); err != nil {
		return SharedLineIntentResult{}, err
	}
	hasCommits, err := service.git.HasCommits(ctx, repository)
	if err != nil {
		return SharedLineIntentResult{}, err
	}
	if !hasCommits {
		return SharedLineIntentResult{}, problem.New(problem.Details{
			Code:        problem.CodeRepositoryHasNoCommits,
			Category:    problem.CategoryRepository,
			Field:       "repository",
			Expected:    "at least one commit before preparing a protected shared line",
			Rule:        "release and support lines do not implicitly bootstrap repositories",
			Remediation: "create an explicit initial commit before requesting the protected line",
		})
	}
	return result, nil
}

func normalizeWorkflowRepository(repository port.RepositoryIdentity) (port.RepositoryIdentity, error) {
	if repository.Root == "" {
		return port.RepositoryIdentity{}, repositoryRequired()
	}
	if repository.Remote == "" {
		repository.Remote = "origin"
	}
	return repository, nil
}

func mustMain() branch.BranchName {
	// This literal is part of the product's fixed branch taxonomy and is
	// independently validated by the branch domain tests.
	name, _ := branch.ParseName("main")
	return name
}
