package workflow

import (
	"context"
	"path/filepath"
	"strconv"
	"time"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	commitapp "github.com/t33n-software/git-governance/internal/application/commit"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/application/ticketalloc"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/genesis"
	"github.com/t33n-software/git-governance/internal/domain/problem"
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

// PublishBornLines executes the separately confirmed publication phase: the
// governed remote birth of the shared lines created by Bootstrap. It re-proves
// the born refs before pushing and never pushes anything else.
func (service *BootstrapService) PublishBornLines(ctx context.Context, repository port.RepositoryIdentity) error {
	if service.git == nil {
		return internalDependencyError("bootstrap services")
	}
	repository, err := bootstrapRepository(repository)
	if err != nil {
		return err
	}
	if err := bootstrapContextError(ctx); err != nil {
		return err
	}
	for _, line := range []branch.BranchName{mustMain(), mustDevelop()} {
		exists, err := service.git.BranchExists(ctx, repository, line)
		if err != nil {
			return err
		}
		if !exists {
			return problem.New(problem.Details{
				Code:        problem.CodeRepositoryNotFound,
				Category:    problem.CategoryRepository,
				Field:       "shared line",
				Actual:      line.String(),
				Expected:    "the born shared lines main and develop",
				Rule:        "publication requires the completed local genesis",
				Remediation: "complete workflow bootstrap before publishing the shared lines",
			})
		}
	}
	for _, line := range []branch.BranchName{mustMain(), mustDevelop()} {
		if err := service.git.Push(ctx, repository, line, true); err != nil {
			return err
		}
	}
	return nil
}

// RecoverUnbornRequest describes the governed pre-birth recovery of an
// unborn repository whose index or references carry a foreign or aborted
// pre-staging state.
type RecoverUnbornRequest struct {
	Repository port.RepositoryIdentity
	DryRun     bool
}

// BootstrapRecoveryResult carries the executed or planned recovery steps and
// the proven pre-state facts.
type BootstrapRecoveryResult struct {
	Plan              []branchapp.PlanStep
	DryRun            bool
	IndexEmptied      bool
	ReferencesRemoved bool
}

// RecoverUnborn restores the proven pre-birth state of an unborn repository:
// the read-only proof binds the unborn topology — no commits, the unborn HEAD
// targeting main, no active Git operation — the mutation empties the index
// and removes every reference through the same governed restoration the birth
// compensation uses, and the read-only read-back proves the restored state.
// The working tree is never modified, so foreign working-tree changes survive
// unchanged. A born repository is never recovered; its corrections belong to
// the governed lifecycle, not to a destructive pre-birth reset.
func (service *BootstrapService) RecoverUnborn(ctx context.Context, request RecoverUnbornRequest) (BootstrapRecoveryResult, error) {
	if service.git == nil {
		return BootstrapRecoveryResult{}, internalDependencyError("bootstrap services")
	}
	repository, err := bootstrapRepository(request.Repository)
	if err != nil {
		return BootstrapRecoveryResult{}, err
	}
	if err := bootstrapContextError(ctx); err != nil {
		return BootstrapRecoveryResult{}, err
	}
	refs, restorer, err := resolveRecoveryCapabilities(service.git)
	if err != nil {
		return BootstrapRecoveryResult{}, err
	}
	proof, err := service.proveUnbornRecoveryState(ctx, repository, refs)
	if err != nil {
		return BootstrapRecoveryResult{}, err
	}
	plan := recoveryPlan(proof)
	if request.DryRun {
		return BootstrapRecoveryResult{Plan: plan, DryRun: true}, nil
	}
	if err := restorer.RestoreUnbornState(ctx, repository); err != nil {
		return BootstrapRecoveryResult{}, err
	}
	if err := service.proveRestoredPreState(ctx, repository, refs); err != nil {
		return BootstrapRecoveryResult{}, err
	}
	return BootstrapRecoveryResult{
		Plan:              plan,
		IndexEmptied:      proof.Staged,
		ReferencesRemoved: proof.Referenced,
	}, nil
}

// unbornRecoveryProof carries the read-only state facts the recovery proves
// before and after the restoration.
type unbornRecoveryProof struct {
	Staged     bool
	Referenced bool
}

func (service *BootstrapService) proveUnbornRecoveryState(
	ctx context.Context,
	repository port.RepositoryIdentity,
	refs port.RefExistenceInspector,
) (unbornRecoveryProof, error) {
	hasCommits, err := service.git.HasCommits(ctx, repository)
	if err != nil {
		return unbornRecoveryProof{}, err
	}
	if hasCommits {
		return unbornRecoveryProof{}, recoveryStateInvalid("the repository HEAD already carries a commit")
	}
	current, err := service.git.CurrentBranch(ctx, repository)
	if err != nil {
		return unbornRecoveryProof{}, err
	}
	if current.Family() != branch.FamilyMain {
		return unbornRecoveryProof{}, unbornHeadMismatch(current)
	}
	operation, active, err := service.git.ActiveOperation(ctx, repository)
	if err != nil {
		return unbornRecoveryProof{}, err
	}
	if active {
		return unbornRecoveryProof{}, problem.New(problem.Details{
			Code:        problem.CodeOperationInProgress,
			Category:    problem.CategoryRepository,
			Field:       "git operation",
			Actual:      operation,
			Expected:    "no merge, rebase, or cherry-pick in progress",
			Rule:        "a running Git operation blocks the governed pre-birth recovery",
			Remediation: "complete or abort the active operation and retry the recovery",
		})
	}
	staged, err := service.git.HasStagedChanges(ctx, repository)
	if err != nil {
		return unbornRecoveryProof{}, err
	}
	anyRef, err := refs.HasAnyRef(ctx, repository)
	if err != nil {
		return unbornRecoveryProof{}, err
	}
	return unbornRecoveryProof{Staged: staged, Referenced: anyRef}, nil
}

func (service *BootstrapService) proveRestoredPreState(
	ctx context.Context,
	repository port.RepositoryIdentity,
	refs port.RefExistenceInspector,
) error {
	staged, err := service.git.HasStagedChanges(ctx, repository)
	if err != nil {
		return recoveryReadBackProblem("the staged-state read-back failed", err)
	}
	if staged {
		return recoveryReadBackProblem("the index still carries staged entries after the restoration", nil)
	}
	anyRef, err := refs.HasAnyRef(ctx, repository)
	if err != nil {
		return recoveryReadBackProblem("the reference read-back failed", err)
	}
	if anyRef {
		return recoveryReadBackProblem("the repository still carries references after the restoration", nil)
	}
	return nil
}

// PublishResumeRequest describes the governed publication resume of a
// repository born without --push.
type PublishResumeRequest struct {
	Repository port.RepositoryIdentity
	DryRun     bool
}

// BootstrapPublicationResult carries the executed or planned publication
// steps and the proven genesis revision.
type BootstrapPublicationResult struct {
	Plan     []branchapp.PlanStep
	DryRun   bool
	Revision string
}

// PublishResume re-proves the complete birth topology of a born repository —
// exactly one commit, main and develop on the shared genesis revision, the
// verified genesis signature, and the materialized hook boundary — and then
// publishes the born shared lines through the separately confirmed
// publication. Outside the proven birth state it fails closed; it never
// pushes anything else.
func (service *BootstrapService) PublishResume(ctx context.Context, request PublishResumeRequest) (BootstrapPublicationResult, error) {
	if service.git == nil || service.branches == nil {
		return BootstrapPublicationResult{}, internalDependencyError("bootstrap services")
	}
	repository, err := bootstrapRepository(request.Repository)
	if err != nil {
		return BootstrapPublicationResult{}, err
	}
	if err := bootstrapContextError(ctx); err != nil {
		return BootstrapPublicationResult{}, err
	}
	capabilities, err := branchapp.ResolveBirthTopologyCapabilities(service.git)
	if err != nil {
		return BootstrapPublicationResult{}, err
	}
	revision, err := service.proveBirthTopology(ctx, repository, capabilities)
	if err != nil {
		return BootstrapPublicationResult{}, err
	}
	if _, err := service.git.RemoteURL(ctx, repository); err != nil {
		return BootstrapPublicationResult{}, problem.New(problem.Details{
			Code:        problem.CodeConfigurationUnavailable,
			Category:    problem.CategoryConfig,
			Field:       "remote",
			Actual:      repository.Remote,
			Expected:    "a bound remote for the publication of the born shared lines",
			Rule:        "the publication resume requires a bound remote; the born shared lines publish only through it",
			Remediation: "bind the remote first (git remote add " + repository.Remote + " <url>) and retry the publication resume",
		})
	}
	plan := publicationResumePlan(revision, repository.Remote)
	if request.DryRun {
		return BootstrapPublicationResult{Plan: plan, DryRun: true, Revision: revision}, nil
	}
	if err := service.PublishBornLines(ctx, request.Repository); err != nil {
		return BootstrapPublicationResult{}, err
	}
	return BootstrapPublicationResult{Plan: plan, Revision: revision}, nil
}

func resolveRecoveryCapabilities(git port.GitRepository) (port.RefExistenceInspector, port.UnbornStateRestorer, error) {
	refs, ok := git.(port.RefExistenceInspector)
	if !ok {
		return nil, nil, bootstrapCapabilityRequired("reference inspection")
	}
	restorer, ok := git.(port.UnbornStateRestorer)
	if !ok {
		return nil, nil, bootstrapCapabilityRequired("unborn-state restoration")
	}
	return refs, restorer, nil
}

// proveBirthTopology re-proves the complete birth topology at finalizer
// grade through the shared birth predicate owned by the branch application:
// exactly one commit reachable from HEAD, both shared lines existing and
// pointing at that shared genesis revision, the verified genesis signature,
// and the materialized hook boundary.
func (service *BootstrapService) proveBirthTopology(
	ctx context.Context,
	repository port.RepositoryIdentity,
	capabilities branchapp.BirthTopologyCapabilities,
) (string, error) {
	return branchapp.ProveBirthTopology(ctx, service.git, service.branches, repository, capabilities)
}

// recoveryPlan renders the recovery steps: the unborn proof, the state
// reductions the proof found, and the read-back evidence.
func recoveryPlan(proof unbornRecoveryProof) []branchapp.PlanStep {
	plan := []branchapp.PlanStep{
		{Action: "prove-unborn", Detail: "unborn-state proof: no commits, unborn HEAD on main, no active Git operation"},
	}
	if proof.Staged {
		plan = append(plan, branchapp.PlanStep{Action: "empty-index", Detail: "empty the index; staged content stays in the working tree as untracked files"})
	}
	if proof.Referenced {
		plan = append(plan, branchapp.PlanStep{Action: "remove-references", Detail: "remove every reference of the aborted or foreign pre-staging state"})
	}
	plan = append(plan, branchapp.PlanStep{Action: "prove-pre-state", Detail: "read-back proof: empty index and no references"})
	return plan
}

// publicationResumePlan renders the publication resume steps: the birth
// topology proof and the separately confirmed shared-line publication.
func publicationResumePlan(revision string, remote string) []branchapp.PlanStep {
	return []branchapp.PlanStep{
		{Action: "prove-birth", Detail: "birth-topology proof: exactly one commit, main and develop on the shared genesis revision " + revision + ", verified signature, materialized hook boundary"},
		{Action: "publish", Detail: "push main and develop to " + remote + " after the separate publication confirmation"},
	}
}

// recoveryStateInvalid refuses a recovery outside the unborn pre-birth
// domain: a born repository is never reset.
func recoveryStateInvalid(detail string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeRepositoryAlreadyBorn,
		Category:    problem.CategoryRepository,
		Field:       "repository",
		Actual:      detail,
		Expected:    "an unborn repository without commits",
		Rule:        "the governed pre-birth recovery restores only unborn repositories; a born repository is never reset",
		Remediation: "run the recovery only on a repository without commits",
	})
}

func recoveryReadBackProblem(detail string, cause error) error {
	problemDetails := problem.Details{
		Code:        problem.CodeInternal,
		Category:    problem.CategoryInternal,
		Field:       "recovery read-back",
		Diagnostic:  detail,
		Expected:    "the proven pre-birth state: empty index and no references",
		Rule:        "a failed read-back leaves the repository state in place and blocks the retry",
		Remediation: "review the repository state and retry the recovery",
	}
	if cause == nil {
		return problem.New(problemDetails)
	}
	return problem.Wrap(problemDetails, cause)
}

// runBootstrapPreflight proves every birth precondition without mutating
// anything: the unborn state, the unborn HEAD targeting main, the idle Git
// state, the empty index, the environment, the signing gate with its canary,
// the hook-manager resolution, the policy snapshot, the ticket key policy, and
// the content boundary of the explicit content set.
func (service *BootstrapService) runBootstrapPreflight(
	ctx context.Context,
	repository port.RepositoryIdentity,
	request BootstrapRequest,
	capabilities bootstrapCapabilities,
) (BootstrapPreflight, error) {
	hasCommits, err := service.git.HasCommits(ctx, repository)
	if err != nil {
		return BootstrapPreflight{}, err
	}
	if hasCommits {
		return BootstrapPreflight{}, repositoryAlreadyBorn("the repository HEAD already carries a commit")
	}
	anyRef, err := capabilities.refs.HasAnyRef(ctx, repository)
	if err != nil {
		return BootstrapPreflight{}, err
	}
	if anyRef {
		return BootstrapPreflight{}, repositoryAlreadyBorn("the repository already carries references")
	}
	current, err := service.git.CurrentBranch(ctx, repository)
	if err != nil {
		return BootstrapPreflight{}, err
	}
	if current.Family() != branch.FamilyMain {
		return BootstrapPreflight{}, unbornHeadMismatch(current)
	}
	operation, active, err := service.git.ActiveOperation(ctx, repository)
	if err != nil {
		return BootstrapPreflight{}, err
	}
	if active {
		return BootstrapPreflight{}, problem.New(problem.Details{
			Code:        problem.CodeOperationInProgress,
			Category:    problem.CategoryRepository,
			Field:       "git operation",
			Actual:      operation,
			Expected:    "no merge, rebase, or cherry-pick in progress",
			Rule:        "a running Git operation blocks the governed repository birth",
			Remediation: "complete or abort the active operation and retry the bootstrap",
		})
	}
	staged, err := service.git.HasStagedChanges(ctx, repository)
	if err != nil {
		return BootstrapPreflight{}, err
	}
	if staged {
		return BootstrapPreflight{}, problem.New(problem.Details{
			Code:        problem.CodeWorktreeNotClean,
			Category:    problem.CategoryRepository,
			Field:       "index",
			Expected:    "an empty index before the governed birth",
			Rule:        "the governed genesis stages the content set explicitly; a pre-staged index would bypass the content boundary scan",
			Remediation: "unstage the index and pass the content set through --stage",
		})
	}
	version, err := service.git.Version(ctx)
	if err != nil {
		return BootstrapPreflight{}, err
	}
	configuration, err := capabilities.signer.SigningConfiguration(ctx, repository)
	if err != nil {
		return BootstrapPreflight{}, err
	}
	if err := service.signingReadiness(configuration); err != nil {
		return BootstrapPreflight{}, err
	}
	if configuration.UserName == "" {
		return BootstrapPreflight{}, problem.New(problem.Details{
			Code:        problem.CodeConfigurationUnavailable,
			Category:    problem.CategoryConfig,
			Field:       "user.name",
			Expected:    "a configured user.name",
			Rule:        "the governed genesis requires the committer identity before the mutation",
			Remediation: "configure user.name for the committer identity and retry the bootstrap",
		})
	}
	if err := capabilities.signer.ProveSigningCapability(ctx, repository, configuration); err != nil {
		return BootstrapPreflight{}, err
	}
	hookManager, err := service.tools.Version(ctx, "lefthook")
	if err != nil {
		return BootstrapPreflight{}, problem.Wrap(problem.Details{
			Code:        problem.CodeExternalCommandFailed,
			Category:    problem.CategoryExternal,
			Field:       "hook manager",
			Expected:    "the approved lefthook binary",
			Rule:        "the governed repository birth installs the canonical hook boundary through lefthook",
			Remediation: "install the approved lefthook binary and retry the bootstrap",
		}, err)
	}
	hookConfig, err := service.tools.FileExists(filepath.Join(repository.Root, "lefthook.yml"))
	if err != nil {
		return BootstrapPreflight{}, problem.Wrap(problem.Details{
			Code:        problem.CodeExternalCommandFailed,
			Category:    problem.CategoryExternal,
			Field:       "lefthook.yml",
			Expected:    "a readable hook configuration path",
			Rule:        "the governed repository birth must resolve the hook configuration state",
			Remediation: "repair the repository filesystem and retry the bootstrap",
		}, err)
	}
	if !hookConfig {
		return BootstrapPreflight{}, problem.New(problem.Details{
			Code:        problem.CodeConfigurationUnavailable,
			Category:    problem.CategoryConfig,
			Field:       "lefthook.yml",
			Expected:    "the repository's lefthook.yml hook configuration",
			Rule:        "the governed repository birth requires the canonical hook configuration before the mutation",
			Remediation: "add the canonical commit-msg and pre-push hooks through lefthook.yml and retry the bootstrap",
		})
	}
	snapshot := service.policySnapshot()
	if service.keyPolicy != nil {
		if err := service.keyPolicy.ValidateKey(ctx, repository, request.Ticket.Key()); err != nil {
			return BootstrapPreflight{}, err
		}
	}
	if len(request.StagePaths) == 0 {
		return BootstrapPreflight{}, emptyContentSet()
	}
	preview, err := capabilities.previewer.PreviewStage(ctx, repository, request.StagePaths)
	if err != nil {
		return BootstrapPreflight{}, err
	}
	if len(preview) == 0 {
		return BootstrapPreflight{}, emptyContentSet()
	}
	if err := genesis.ScanContentSet(preview); err != nil {
		return BootstrapPreflight{}, err
	}
	if request.Push {
		if _, err := service.git.RemoteURL(ctx, repository); err != nil {
			return BootstrapPreflight{}, problem.New(problem.Details{
				Code:        problem.CodeConfigurationUnavailable,
				Category:    problem.CategoryConfig,
				Field:       "remote",
				Actual:      repository.Remote,
				Expected:    "a bound remote for the publication of the born shared lines",
				Rule:        "publication requires a bound remote; the local genesis completes without it only when --push is not requested",
				Remediation: "bind the remote first (git remote add " + repository.Remote + " <url>) and retry with --push",
			})
		}
	}
	return BootstrapPreflight{
		ContentFiles:   len(preview),
		PolicySnapshot: snapshot,
		Actor:          configuration.UserEmail,
		GitVersion:     version,
		HookManager:    hookManager,
	}, nil
}

// runGenesisMutation is the single bound transaction of the birth: stage the
// explicit content set, create the signed genesis commit on main, create
// develop from the same revision, and install the hook boundary. The staged
// surface carries the derived mutation budget; the hook installation stays on
// the preflight surface because it is corpus-independent.
func (service *BootstrapService) runGenesisMutation(
	ctx context.Context,
	git port.GitRepository,
	capabilities bootstrapCapabilities,
	repository port.RepositoryIdentity,
	request BootstrapRequest,
	message commitmsg.Message,
) (string, port.HookInstallation, error) {
	if err := git.Stage(ctx, repository, request.StagePaths); err != nil {
		return "", port.HookInstallation{}, err
	}
	staged, err := git.HasStagedChanges(ctx, repository)
	if err != nil {
		return "", port.HookInstallation{}, err
	}
	if !staged {
		return "", port.HookInstallation{}, problem.New(problem.Details{
			Code:        problem.CodeGitCommandFailed,
			Category:    problem.CategoryGit,
			Field:       "index",
			Expected:    "the staged content set after staging the explicit paths",
			Rule:        "the genesis mutation requires the staged content set the preflight proved",
			Remediation: "review the repository state and retry the bootstrap",
		})
	}
	if err := git.Commit(ctx, repository, message); err != nil {
		return "", port.HookInstallation{}, err
	}
	revision, err := capabilities.revisions.ResolveRevision(ctx, repository, "HEAD")
	if err != nil {
		return "", port.HookInstallation{}, err
	}
	// mustMain is the product's fixed production-line taxonomy and therefore a
	// canonical local branch name. NewLocalBase cannot reject that invariant.
	developBase, _ := branch.NewLocalBase(mustMain())
	if err := git.CreateBranch(ctx, repository, mustDevelop(), developBase, false); err != nil {
		return "", port.HookInstallation{}, err
	}
	installation, err := capabilities.installer.InstallHooks(ctx, repository)
	if err != nil {
		return "", port.HookInstallation{}, err
	}
	return revision, installation, nil
}

// runGenesisFinalizer is the read-only closing proof of the birth: the born
// refs exist and satisfy the governed validation, both point at the genesis
// revision, the genesis commit signature verifies, and the hook boundary is
// materialized.
func (service *BootstrapService) runGenesisFinalizer(
	ctx context.Context,
	repository port.RepositoryIdentity,
	capabilities bootstrapCapabilities,
	revision string,
	installation port.HookInstallation,
) error {
	for _, line := range []branch.BranchName{mustMain(), mustDevelop()} {
		if _, err := service.branches.Validate(ctx, branchapp.ValidateRequest{
			Repository: repository,
			Name:       line,
		}); err != nil {
			return err
		}
		exists, err := service.git.BranchExists(ctx, repository, line)
		if err != nil {
			return err
		}
		if !exists {
			return genesisFinalizerProblem("the born ref " + line.String() + " does not exist")
		}
		lineRevision, err := capabilities.revisions.ResolveRevision(ctx, repository, line.String())
		if err != nil {
			return err
		}
		if lineRevision != revision {
			return genesisFinalizerProblem(
				"the born ref " + line.String() + " points at " + lineRevision + " instead of the genesis revision " + revision,
			)
		}
	}
	if err := capabilities.verifier.VerifyCommitSignature(ctx, repository, revision); err != nil {
		return err
	}
	for _, hook := range installation.Hooks {
		exists, err := service.tools.FileExists(filepath.Join(installation.Directory, hook))
		if err != nil {
			return problem.Wrap(problem.Details{
				Code:        problem.CodeExternalCommandFailed,
				Category:    problem.CategoryExternal,
				Field:       "hook boundary",
				Actual:      hook,
				Expected:    "a readable installed hook file",
				Rule:        "the genesis finalizer proves the installed hook boundary",
				Remediation: "repair the hook installation and retry the bootstrap",
			}, err)
		}
		if !exists {
			return genesisFinalizerProblem("the installed hook " + hook + " is missing in " + installation.Directory)
		}
	}
	return nil
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

// composeGenesisMessage assembles the canonical genesis commit: the chore
// family, the explicit ticket scope, the fixed subject, and the canonical
// body. Composition is pure and round-trip validated before any mutation.
func composeGenesisMessage(
	id ticket.ID,
	preflight BootstrapPreflight,
	publish bool,
) (commitmsg.Message, error) {
	return commitapp.Compose(commitapp.Draft{
		Family:  commitmsg.TypeChore,
		Ticket:  id,
		Subject: genesis.CommitSubject,
		Body: genesis.CommitBody(genesis.BodyFacts{
			Ticket:         id,
			ContentFiles:   preflight.ContentFiles,
			PolicySnapshot: preflight.PolicySnapshot,
			Publish:        publish,
		}),
	})
}

// bootstrapPlan renders the full genesis plan: the executed preflight plus
// the planned mutation, finalizer, and — when requested — the separately
// confirmed publication.
func bootstrapPlan(
	repository port.RepositoryIdentity,
	request BootstrapRequest,
	preflight BootstrapPreflight,
	message commitmsg.Message,
) []branchapp.PlanStep {
	plan := []branchapp.PlanStep{
		{Action: "preflight", Detail: "unborn-state proof, signing gate with canary, hook-manager resolution, policy snapshot, content boundary scan"},
		{Action: "stage", Detail: "stage " + strconv.Itoa(preflight.ContentFiles) + " content files from the explicit stage paths"},
		{Action: "commit", Detail: message.Header().String()},
		{Action: "create", Detail: "create develop from the genesis revision"},
		{Action: "install-hooks", Detail: "install the canonical hook boundary (commit-msg, pre-push)"},
		{Action: "finalize", Detail: "prove the born refs, the shared revision, the signature, and the hook boundary"},
	}
	if request.Push {
		plan = append(plan, branchapp.PlanStep{
			Action: "publish",
			Detail: "push main and develop to " + repository.Remote + " after the separate publication confirmation",
		})
	}
	return plan
}

// The genesis mutation budget derives from the proven preflight corpus size
// instead of a flat caller default: the base ceiling covers the commit, the
// branch creation, and the hook installation; the per-file allowance covers
// the linear staging cost of the explicit content set. The ceiling keeps the
// budget a bounded-execution guarantee for every corpus size.
const (
	genesisBaseBudget       = 60 * time.Second
	genesisPerFileAllowance = 10 * time.Millisecond
	genesisMaxBudget        = 30 * time.Minute
)

// deriveGenesisMutationBudget derives the per-process budget of the genesis
// mutation from the proven content-file count of the preflight.
func deriveGenesisMutationBudget(contentFiles int) time.Duration {
	if contentFiles < 0 {
		contentFiles = 0
	}
	budget := genesisBaseBudget + time.Duration(contentFiles)*genesisPerFileAllowance
	if budget > genesisMaxBudget {
		budget = genesisMaxBudget
	}
	return budget
}

// resolveGenesisMutationBudget binds the effective mutation budget: the
// derived budget is the primary protection, and an explicitly supplied caller
// timeout caps it as an upper bound. Zero keeps the derived budget.
func resolveGenesisMutationBudget(contentFiles int, explicitTimeout time.Duration) time.Duration {
	budget := deriveGenesisMutationBudget(contentFiles)
	if explicitTimeout > 0 && explicitTimeout < budget {
		return explicitTimeout
	}
	return budget
}

// scopeMutationGit binds the mutation-phase Git surface: when the adapter
// carries the operation-timeout capability, the derived budget applies to a
// scoped surface; otherwise the configured surface runs unchanged.
func scopeMutationGit(git port.GitRepository, budget time.Duration) port.GitRepository {
	if scoper, ok := git.(port.OperationTimeoutScoper); ok {
		return scoper.WithOperationTimeout(budget)
	}
	return git
}

// compensateGenesisAbortion restores the proven unborn pre-state after an
// aborted genesis step. A successful restoration returns the original cause
// with the compensation fact added to its diagnostic; a failed restoration
// returns a blocking record that names the partial state and its recovery,
// wrapping the original cause. The restoration runs on a detached context, so
// a cancelled caller cannot skip the restoration of the proven pre-state. A
// cause without the typed problem contract carries no diagnostic surface and
// is returned unchanged.
func compensateGenesisAbortion(ctx context.Context, restorer port.UnbornStateRestorer, repository port.RepositoryIdentity, cause error) error {
	if err := restorer.RestoreUnbornState(context.WithoutCancel(ctx), repository); err != nil {
		return problem.Wrap(problem.Details{
			Code:        problem.CodeInternal,
			Category:    problem.CategoryInternal,
			Field:       "genesis compensation",
			Diagnostic:  err.Error(),
			Expected:    "the proven unborn pre-state: empty index, no references, unborn HEAD",
			Rule:        "a failed compensation leaves the partial birth in place and blocks the retry",
			Remediation: "empty the index, delete the born refs, and retry the bootstrap",
		}, cause)
	}
	if typed, ok := problem.As(cause); ok {
		details := typed.Details
		if details.Diagnostic != "" {
			details.Diagnostic += "; "
		}
		details.Diagnostic += "the aborted birth was compensated back to the proven unborn pre-state (empty index, no references, unborn HEAD); the retry is idempotent"
		return problem.Wrap(details, cause)
	}
	return cause
}

func bootstrapRepository(repository port.RepositoryIdentity) (port.RepositoryIdentity, error) {
	if repository.Root == "" {
		return port.RepositoryIdentity{}, problem.New(problem.Details{
			Code:        problem.CodeRepositoryNotFound,
			Category:    problem.CategoryRepository,
			Field:       "repository",
			Expected:    "a discovered local Git repository",
			Rule:        "the governed repository birth requires a repository root",
			Example:     "C:\\work\\repository",
			Remediation: "run from a Git repository or pass --repo",
		})
	}
	if repository.Remote == "" {
		repository.Remote = "origin"
	}
	return repository, nil
}

func bootstrapContextError(ctx context.Context) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	return problem.Wrap(problem.Details{
		Code:        problem.CodeOperationCancelled,
		Category:    problem.CategoryCancelled,
		Field:       "bootstrap operation",
		Expected:    "an active context",
		Rule:        "the governed repository birth stops when the caller cancels its context",
		Remediation: "retry with an active context",
	}, ctx.Err())
}

func repositoryAlreadyBorn(detail string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeRepositoryAlreadyBorn,
		Category:    problem.CategoryRepository,
		Field:       "repository",
		Actual:      detail,
		Expected:    "an unborn repository without commits or references",
		Rule:        "the governed bootstrap births only unborn repositories; a born repository is never re-born or mutated",
		Remediation: "run the bootstrap only on a repository without commits or references",
	})
}

func unbornHeadMismatch(current branch.BranchName) error {
	return problem.New(problem.Details{
		Code:        problem.CodeUnbornHeadMismatch,
		Category:    problem.CategoryRepository,
		Field:       "current branch",
		Actual:      current.String(),
		Expected:    "main",
		Rule:        "the governed genesis births main; the unborn HEAD must target main",
		Example:     "git symbolic-ref HEAD refs/heads/main",
		Remediation: "point the unborn HEAD at main or re-initialize with git init -b main",
	})
}

func emptyContentSet() error {
	return problem.New(problem.Details{
		Code:        problem.CodeInvalidInput,
		Category:    problem.CategoryUsage,
		Field:       "stage paths",
		Expected:    "an explicit, non-empty initial content set",
		Rule:        "the governed genesis requires an explicit content set; an empty genesis is a policy decision, never a default",
		Example:     "--stage .",
		Remediation: "pass at least one stage path that matches repository content",
	})
}

func bootstrapCapabilityRequired(capability string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeConfigurationUnavailable,
		Category:    problem.CategoryConfig,
		Field:       "Git adapter",
		Expected:    "a Git adapter with " + capability + " capability",
		Rule:        "the governed repository birth requires the composed adapter capabilities",
		Remediation: "use the composed Git adapter of the CLI",
	})
}

func genesisFinalizerProblem(detail string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeInternal,
		Category:    problem.CategoryInternal,
		Field:       "genesis finalizer",
		Actual:      detail,
		Expected:    "the born refs, the shared genesis revision, the verified signature, and the installed hook boundary",
		Rule:        "the genesis finalizer proof failed after the mutation; the local birth is incomplete and reversible",
		Remediation: "review the repository state, tear down the local birth, and retry the bootstrap",
	})
}
