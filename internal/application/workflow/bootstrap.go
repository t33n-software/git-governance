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

	revision, installation, err := service.runGenesisMutation(ctx, repository, request, capabilities, message)
	if err != nil {
		return BootstrapResult{}, err
	}
	if err := service.runGenesisFinalizer(ctx, repository, capabilities, revision, installation); err != nil {
		return BootstrapResult{}, err
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
// develop from the same revision, and install the hook boundary.
func (service *BootstrapService) runGenesisMutation(
	ctx context.Context,
	repository port.RepositoryIdentity,
	request BootstrapRequest,
	capabilities bootstrapCapabilities,
	message commitmsg.Message,
) (string, port.HookInstallation, error) {
	if err := service.git.Stage(ctx, repository, request.StagePaths); err != nil {
		return "", port.HookInstallation{}, err
	}
	staged, err := service.git.HasStagedChanges(ctx, repository)
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
	if err := service.git.Commit(ctx, repository, message); err != nil {
		return "", port.HookInstallation{}, err
	}
	revision, err := capabilities.revisions.ResolveRevision(ctx, repository, "HEAD")
	if err != nil {
		return "", port.HookInstallation{}, err
	}
	// mustMain is the product's fixed production-line taxonomy and therefore a
	// canonical local branch name. NewLocalBase cannot reject that invariant.
	developBase, _ := branch.NewLocalBase(mustMain())
	if err := service.git.CreateBranch(ctx, repository, mustDevelop(), developBase, false); err != nil {
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
	return bootstrapCapabilities{
		previewer: previewer,
		refs:      refs,
		verifier:  verifier,
		installer: installer,
		signer:    signer,
		revisions: revisions,
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
