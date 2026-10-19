package workflow

import (
	"context"
	"time"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/application/ticketalloc"
	"github.com/t33n-software/git-governance/internal/domain/genesis"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

// BootstrapService owns the governed birth of an unborn repository: the
// read-only fail-closed preflight, the single bound genesis mutation, the
// read-only finalizer proof, the evidence emission, and the separately
// confirmed publication of the born shared lines.
type BootstrapService struct {
	branches         *branchapp.Service
	git              port.GitRepository
	keyPolicy        port.KeyPolicy
	tools            port.ToolInspector
	allocation       *ticketalloc.Service
	signingReadiness func(port.SigningConfiguration) error
	policySnapshot   func() string
	now              func() time.Time
}

// NewBootstrapService creates the repository birth workflow service.
func NewBootstrapService(
	branches *branchapp.Service,
	git port.GitRepository,
	keyPolicy port.KeyPolicy,
	tools port.ToolInspector,
) *BootstrapService {
	return &BootstrapService{
		branches:  branches,
		git:       git,
		keyPolicy: keyPolicy,
		tools:     tools,
		now:       time.Now,
	}
}

// WithSigningReadiness binds the governed signing-readiness rule. The rule is
// owned by the policy package and injected by the composition root, so the
// workflow package never depends on the policy package.
func (service *BootstrapService) WithSigningReadiness(readiness func(port.SigningConfiguration) error) *BootstrapService {
	service.signingReadiness = readiness
	return service
}

// WithPolicySnapshot binds the policy snapshot source recorded in the genesis
// evidence. The snapshot content is owned by the policy package and rendered
// by the composition root.
func (service *BootstrapService) WithPolicySnapshot(snapshot func() string) *BootstrapService {
	service.policySnapshot = snapshot
	return service
}

// WithClock binds the evidence timestamp source. Tests use it to pin the
// record timestamp; production keeps the system clock.
func (service *BootstrapService) WithClock(now func() time.Time) *BootstrapService {
	service.now = now
	return service
}

// WithTicketAllocation wires the fail-closed ticket-number allocation gate
// into the governed repository birth: the genesis ticket is validated
// against a fresh full-surface inventory immediately before the genesis
// commit binds it. An unwired gate fails closed.
func (service *BootstrapService) WithTicketAllocation(allocation *ticketalloc.Service) *BootstrapService {
	service.allocation = allocation
	return service
}

// BootstrapRequest describes the governed birth of an unborn repository.
type BootstrapRequest struct {
	Repository port.RepositoryIdentity
	Ticket     ticket.ID
	StagePaths []string
	Push       bool
	DryRun     bool
	// MutationTimeout caps the derived genesis mutation budget as an explicit
	// caller upper bound. Zero derives the budget from the proven preflight
	// corpus size alone, so the caller never needs a timeout for large
	// content sets.
	MutationTimeout time.Duration
}

// BootstrapPreflight carries the proven read-only preflight facts into the
// plan, the genesis commit body, and the evidence record.
type BootstrapPreflight struct {
	ContentFiles   int
	PolicySnapshot string
	Actor          string
	GitVersion     string
	HookManager    string
}

// BootstrapResult carries the evidence record of the completed birth, the
// executed or planned steps, and the publication state.
type BootstrapResult struct {
	Record    genesis.Record
	Plan      []branchapp.PlanStep
	Published bool
	DryRun    bool
}

// bootstrapCapabilities binds the optional adapter capabilities the governed
// birth requires. A missing capability is a composition defect and fails
// closed before any mutation.
type bootstrapCapabilities struct {
	previewer port.StagePreviewer
	refs      port.RefExistenceInspector
	verifier  port.CommitSignatureVerifier
	installer port.HookInstaller
	signer    port.GitSigningInspector
	revisions port.RevisionResolver
	restorer  port.UnbornStateRestorer
}

// Bootstrap executes the governed birth through its bound phases: preflight
// (read-only, fail-closed), the single genesis mutation, the finalizer proof,
// and the evidence emission. Publication is never a side effect of the local
// genesis; it runs only through PublishBornLines after its separate
// confirmation.
func (service *BootstrapService) Bootstrap(ctx context.Context, request BootstrapRequest) (BootstrapResult, error) {
	if service.branches == nil || service.git == nil || service.tools == nil ||
		service.signingReadiness == nil || service.policySnapshot == nil {
		return BootstrapResult{}, internalDependencyError("bootstrap services")
	}
	repository, err := bootstrapRepository(request.Repository)
	if err != nil {
		return BootstrapResult{}, err
	}
	if err := bootstrapContextError(ctx); err != nil {
		return BootstrapResult{}, err
	}
	if request.Ticket.IsZero() {
		return BootstrapResult{}, invalidWorkflowInput(
			"repository bootstrap requires an explicit ticket",
			"pass --key and --ticket of the repository setup or governance ticket",
		)
	}
	if service.allocation == nil {
		return BootstrapResult{}, ticketalloc.GateUnavailable()
	}
	if err := service.allocation.ValidateFree(ctx, repository, request.Ticket); err != nil {
		return BootstrapResult{}, err
	}
	capabilities, err := resolveBootstrapCapabilities(service.git)
	if err != nil {
		return BootstrapResult{}, err
	}
	preflight, err := service.runBootstrapPreflight(ctx, repository, request, capabilities)
	if err != nil {
		return BootstrapResult{}, err
	}
	message, err := composeGenesisMessage(request.Ticket, preflight, request.Push)
	if err != nil {
		return BootstrapResult{}, err
	}
	plan := bootstrapPlan(repository, request, preflight, message)
	if request.DryRun {
		return BootstrapResult{Plan: plan, DryRun: true}, nil
	}

	revision, installation, err := service.runGenesisMutation(
		ctx,
		scopeMutationGit(service.git, resolveGenesisMutationBudget(preflight.ContentFiles, request.MutationTimeout)),
		capabilities,
		repository,
		request,
		message,
	)
	if err != nil {
		return BootstrapResult{}, compensateGenesisAbortion(ctx, capabilities.restorer, repository, err)
	}
	if err := service.runGenesisFinalizer(ctx, repository, capabilities, revision, installation); err != nil {
		return BootstrapResult{}, compensateGenesisAbortion(ctx, capabilities.restorer, repository, err)
	}
	record := genesis.NewRecord(
		repository.Root,
		repository.Remote,
		request.Ticket,
		revision,
		[]string{mustMain().String(), mustDevelop().String()},
		preflight.PolicySnapshot,
		preflight.Actor,
		service.now().UTC(),
	)
	return BootstrapResult{Record: record, Plan: plan}, nil
}

// resolveBootstrapCapabilities binds the required optional adapter
// capabilities fail-closed before any mutation.
func resolveBootstrapCapabilities(git port.GitRepository) (bootstrapCapabilities, error) {
	previewer, ok := git.(port.StagePreviewer)
	if !ok {
		return bootstrapCapabilities{}, bootstrapCapabilityRequired("stage preview")
	}
	refs, ok := git.(port.RefExistenceInspector)
	if !ok {
		return bootstrapCapabilities{}, bootstrapCapabilityRequired("reference inspection")
	}
	verifier, ok := git.(port.CommitSignatureVerifier)
	if !ok {
		return bootstrapCapabilities{}, bootstrapCapabilityRequired("commit signature verification")
	}
	installer, ok := git.(port.HookInstaller)
	if !ok {
		return bootstrapCapabilities{}, bootstrapCapabilityRequired("hook installation")
	}
	signer, ok := git.(port.GitSigningInspector)
	if !ok {
		return bootstrapCapabilities{}, bootstrapCapabilityRequired("commit signing inspection")
	}
	revisions, ok := git.(port.RevisionResolver)
	if !ok {
		return bootstrapCapabilities{}, bootstrapCapabilityRequired("revision resolution")
	}
	restorer, ok := git.(port.UnbornStateRestorer)
	if !ok {
		return bootstrapCapabilities{}, bootstrapCapabilityRequired("unborn-state restoration")
	}
	return bootstrapCapabilities{
		previewer: previewer,
		refs:      refs,
		verifier:  verifier,
		installer: installer,
		signer:    signer,
		revisions: revisions,
		restorer:  restorer,
	}, nil
}
