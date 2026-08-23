// Package workflow composes branch and commit use cases into bounded,
// resumable Git workflows without spawning the CLI recursively.
package workflow

import (
	"context"
	"strings"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

// TicketService owns the bounded ticket start and publish workflows.
type TicketService struct {
	branches     *branchapp.Service
	sync         *branchapp.Synchronizer
	scratch      *branchapp.ScratchMerger
	git          port.GitRepository
	quality      port.QualityRunner
	finalQuality *branchapp.FinalQualityGate
	publisher    port.PullRequestPublisher
}

// NewTicketService creates the ticket workflow service.
func NewTicketService(
	branches *branchapp.Service,
	sync *branchapp.Synchronizer,
	git port.GitRepository,
	quality port.QualityRunner,
	publisher port.PullRequestPublisher,
) *TicketService {
	return &TicketService{
		branches:  branches,
		sync:      sync,
		git:       git,
		quality:   quality,
		publisher: publisher,
	}
}

// WithScratchMerger adds the reusable scratch-transfer use case to ticket
// publication without changing the regular ticket workflow composition.
func (service *TicketService) WithScratchMerger(merger *branchapp.ScratchMerger) *TicketService {
	service.scratch = merger
	return service
}

// WithFinalQualityGate ensures final publication quality is bound to the
// post-synchronization candidate before pre-push validation can reuse it.
func (service *TicketService) WithFinalQualityGate(gate *branchapp.FinalQualityGate) *TicketService {
	service.finalQuality = gate
	return service
}

// StartTicketRequest describes normal ticket work from develop.
type StartTicketRequest struct {
	Repository    port.RepositoryIdentity
	Family        branch.Family
	Ticket        ticket.ID
	Slug          branch.Slug
	CreateScratch bool
	ScratchSlug   branch.Slug
	DryRun        bool
}

// StartTicketResult identifies the official and optional scratch branches.
type StartTicketResult struct {
	Official branchapp.CreateResult
	Scratch  *branchapp.CreateResult
	Active   branch.BranchName
}

// StartTicket creates one official regular ticket branch and, optionally, a
// private scratch branch from it. It ends at the active branch and deliberately
// does not continue into the development or pull-request phase.
func (service *TicketService) StartTicket(ctx context.Context, request StartTicketRequest) (StartTicketResult, error) {
	if service.branches == nil {
		return StartTicketResult{}, internalDependencyError("branch service")
	}
	if request.Family != branch.FamilyFeature &&
		request.Family != branch.FamilyFix &&
		request.Family != branch.FamilyDocs &&
		request.Family != branch.FamilyRefactor &&
		request.Family != branch.FamilyChore &&
		request.Family != branch.FamilyTest &&
		request.Family != branch.FamilyPerf {
		return StartTicketResult{}, invalidWorkflowInput(
			"regular ticket start accepts feature, fix, docs, refactor, chore, test, or perf",
			"select a regular ticket family or use the hotfix/release workflow",
		)
	}

	switchToOfficial := true
	official, err := service.branches.Create(ctx, branchapp.CreateRequest{
		Repository: request.Repository,
		Family:     request.Family,
		Ticket:     request.Ticket,
		Slug:       request.Slug,
		Switch:     &switchToOfficial,
		DryRun:     request.DryRun,
	})
	if err != nil {
		return StartTicketResult{}, err
	}
	result := StartTicketResult{
		Official: official,
		Active:   official.Name,
	}
	if !request.CreateScratch {
		return result, nil
	}

	scratchSlug := request.ScratchSlug
	if scratchSlug.String() == "" {
		scratchSlug, err = branch.ParseSlug(request.Slug.String() + "-exploration")
		if err != nil {
			return StartTicketResult{}, err
		}
	}
	// official.Name was just created by the branch service and is therefore a
	// canonical local branch name. NewLocalBase cannot reject that invariant.
	localBase, _ := branch.NewLocalBase(official.Name)
	switchToScratch := true
	scratch, err := service.branches.Create(ctx, branchapp.CreateRequest{
		Repository: request.Repository,
		Family:     branch.FamilyScratch,
		Ticket:     request.Ticket,
		Slug:       scratchSlug,
		Base:       &localBase,
		Switch:     &switchToScratch,
		DryRun:     request.DryRun,
		SkipFetch:  true,
	})
	if err != nil {
		return StartTicketResult{}, err
	}
	result.Scratch = &scratch
	result.Active = scratch.Name
	return result, nil
}

// PublishTicketRequest describes the handoff from completed local work to a
// push and provider-neutral pull request. Body carries the mandatory
// pull-request description required by
// docs/conventions/pull-requests/description-mandate.md whenever
// CreatePullRequest is set.
type PublishTicketRequest struct {
	Repository        port.RepositoryIdentity
	Branch            branch.BranchName
	Base              *branch.TargetBase
	Target            *branch.BranchName
	ScratchTarget     *branch.BranchName
	ScratchMessage    *commitmsg.Message
	WorkflowManaged   bool
	Push              bool
	CreatePullRequest bool
	Draft             bool
	Body              string
	DryRun            bool
}

// PublishTicketResult contains the push status and provider-neutral PR intent.
type PublishTicketResult struct {
	Branch              branch.BranchName
	Sync                branchapp.SyncResult
	Pushed              bool
	PullRequest         port.PullRequest
	PublishedURL        string
	DryRun              bool
	ScratchMerge        *branchapp.ScratchMergeResult
	Quality             port.QualityResult
	PostMutationQuality *port.QualityResult
}

// PublishTicket validates the complete local commit series, runs quality
// gates, synchronizes the base safely, and emits a pull request intent.
// Programmatic callers may explicitly request its push and provider publication.
func (service *TicketService) PublishTicket(ctx context.Context, request PublishTicketRequest) (PublishTicketResult, error) {
	if service.branches == nil || service.sync == nil || service.git == nil {
		return PublishTicketResult{}, internalDependencyError("ticket workflow services")
	}
	if request.Branch.IsZero() {
		return PublishTicketResult{}, invalidWorkflowInput(
			"ticket publish requires a canonical ticket branch",
			"run this workflow from a scratch or official ticket branch",
		)
	}
	if request.Branch.Family() != branch.FamilyScratch && !request.Branch.Family().IsOfficialWorkingBranch() {
		return PublishTicketResult{}, invalidWorkflowInput(
			"ticket publish requires an official ticket branch",
			"run this workflow from feature, fix, docs, refactor, chore, test, perf, or hotfix work",
		)
	}
	if request.CreatePullRequest && !request.Push {
		return PublishTicketResult{}, invalidWorkflowInput(
			"pull-request creation requires an explicit branch push",
			"set Push before requesting provider pull-request creation",
		)
	}
	if request.CreatePullRequest && strings.TrimSpace(request.Body) == "" {
		return PublishTicketResult{}, pullRequestBodyRequired()
	}

	repository := request.Repository
	if repository.Remote == "" {
		repository.Remote = "origin"
	}
	if repository.Root == "" {
		return PublishTicketResult{}, repositoryRequired()
	}

	var scratchMerge *branchapp.ScratchMergeResult
	if request.Branch.Family() == branch.FamilyScratch {
		if service.scratch == nil {
			return PublishTicketResult{}, internalDependencyError("scratch merger")
		}
		if request.ScratchMessage == nil {
			return PublishTicketResult{}, invalidWorkflowInput(
				"publishing from scratch requires a validated squash commit message",
				"provide a Conventional Commit message for the transfer into the official branch",
			)
		}
		merged, err := service.scratch.Merge(ctx, branchapp.ScratchMergeRequest{
			Repository: repository,
			Source:     request.Branch,
			Target:     request.ScratchTarget,
			Message:    *request.ScratchMessage,
			DryRun:     request.DryRun,
		})
		if err != nil {
			return PublishTicketResult{}, err
		}
		scratchMerge = &merged
		request.Branch = merged.Target
	}

	validation, err := service.branches.Validate(ctx, branchapp.ValidateRequest{
		Repository: repository,
		Name:       request.Branch,
	})
	if err != nil {
		return PublishTicketResult{}, err
	}
	baseInput := request.Base
	if baseInput == nil && validation.Name.Family().MayUseWorkflowBase() {
		storedBase, found, err := service.git.WorkflowBase(ctx, repository, validation.Name)
		if err != nil {
			return PublishTicketResult{}, err
		}
		if found {
			baseInput = &storedBase
		}
	}
	base, err := resolveTicketBase(validation.Name, repository, baseInput, request.WorkflowManaged)
	if err != nil {
		return PublishTicketResult{}, err
	}
	target, err := resolvePullRequestTarget(validation.Name, base, request.Target, request.WorkflowManaged)
	if err != nil {
		return PublishTicketResult{}, err
	}
	pullRequest := newTicketPullRequest(validation.Name, target, request.Draft, request.Body)
	if scratchMerge != nil && request.DryRun {
		return PublishTicketResult{
			Branch:       validation.Name,
			PullRequest:  pullRequest,
			DryRun:       true,
			ScratchMerge: scratchMerge,
			Sync: branchapp.SyncResult{
				Name:              validation.Name,
				RecommendedAction: "planned",
			},
			Quality: port.QualityResult{
				Status: port.QualitySkipped,
				Detail: "quality gates are not executed during dry-run",
			},
		}, nil
	}

	if !request.DryRun {
		if err := service.git.Fetch(ctx, repository); err != nil {
			return PublishTicketResult{}, err
		}
	}
	if err := service.validateCommitSeries(ctx, repository, validation.Name, base); err != nil {
		return PublishTicketResult{}, err
	}
	syncResult, err := service.sync.Sync(ctx, branchapp.SyncRequest{
		Repository:               repository,
		Name:                     validation.Name,
		Base:                     &base,
		Strategy:                 branchapp.SyncAuto,
		DryRun:                   request.DryRun,
		SkipFetch:                true,
		WorkflowManaged:          request.WorkflowManaged,
		DeferPostMutationQuality: true,
	})
	if err != nil {
		return PublishTicketResult{}, err
	}
	if syncResult.Mutated {
		if err := service.validateCommitSeries(ctx, repository, validation.Name, base); err != nil {
			return PublishTicketResult{}, err
		}
	}
	quality := port.QualityResult{
		Status: port.QualitySkipped,
		Detail: "quality gates are not executed during dry-run",
	}
	if !request.DryRun {
		quality, err = service.runFinalQuality(ctx, repository, validation.Name, base)
		if err != nil {
			return PublishTicketResult{}, err
		}
	}

	result := PublishTicketResult{
		Branch:       validation.Name,
		Sync:         syncResult,
		PullRequest:  pullRequest,
		DryRun:       request.DryRun,
		ScratchMerge: scratchMerge,
		Quality:      quality,
	}
	if syncResult.Mutated {
		result.PostMutationQuality = &quality
	}
	if request.DryRun {
		return result, nil
	}
	if request.Push {
		if err := service.PushPreparedTicket(ctx, repository, validation.Name, &syncResult.Base, request.WorkflowManaged); err != nil {
			return PublishTicketResult{}, err
		}
		result.Pushed = true
	}
	if request.CreatePullRequest {
		publishedURL, err := service.PublishPullRequest(ctx, repository, pullRequest)
		if err != nil {
			return PublishTicketResult{}, err
		}
		result.PublishedURL = publishedURL
	}
	return result, nil
}

// ResumeTicketPublishRequest identifies a ticket publication that paused on a
// rebase conflict. The branch must be the official branch being published; a
// scratch transfer is completed before the rebase can start. Body carries the
// mandatory pull-request description required by
// docs/conventions/pull-requests/description-mandate.md for the continued
// publication.
type ResumeTicketPublishRequest struct {
	Repository      port.RepositoryIdentity
	Branch          branch.BranchName
	Base            *branch.TargetBase
	Target          *branch.BranchName
	WorkflowManaged bool
	Draft           bool
	Body            string
}

// ResumeTicketPublish continues an already resolved rebase and revalidates the
// resulting branch before the caller offers the next publication step.
func (service *TicketService) ResumeTicketPublish(ctx context.Context, request ResumeTicketPublishRequest) (PublishTicketResult, error) {
	if service.branches == nil || service.sync == nil || service.git == nil {
		return PublishTicketResult{}, internalDependencyError("ticket workflow services")
	}
	if request.Branch.IsZero() || !request.Branch.Family().IsOfficialWorkingBranch() {
		return PublishTicketResult{}, invalidWorkflowInput(
			"rebase resumption requires an official ticket branch",
			"resolve the active rebase on the official ticket branch, then select Retry",
		)
	}
	repository := request.Repository
	if repository.Remote == "" {
		repository.Remote = "origin"
	}
	if repository.Root == "" {
		return PublishTicketResult{}, repositoryRequired()
	}

	validation, err := service.branches.Validate(ctx, branchapp.ValidateRequest{
		Repository: repository,
		Name:       request.Branch,
	})
	if err != nil {
		return PublishTicketResult{}, err
	}
	baseInput := request.Base
	if baseInput == nil && validation.Name.Family().MayUseWorkflowBase() {
		storedBase, found, err := service.git.WorkflowBase(ctx, repository, validation.Name)
		if err != nil {
			return PublishTicketResult{}, err
		}
		if found {
			baseInput = &storedBase
		}
	}
	base, err := resolveTicketBase(validation.Name, repository, baseInput, request.WorkflowManaged)
	if err != nil {
		return PublishTicketResult{}, err
	}
	target, err := resolvePullRequestTarget(validation.Name, base, request.Target, request.WorkflowManaged)
	if err != nil {
		return PublishTicketResult{}, err
	}
	syncResult, err := service.sync.ResumeRebase(ctx, branchapp.ResumeRebaseRequest{
		Repository:               repository,
		Name:                     validation.Name,
		Base:                     &base,
		WorkflowManaged:          request.WorkflowManaged,
		DeferPostMutationQuality: true,
	})
	if err != nil {
		return PublishTicketResult{}, err
	}
	if err := service.validateCommitSeries(ctx, repository, validation.Name, base); err != nil {
		return PublishTicketResult{}, err
	}
	quality, err := service.runFinalQuality(ctx, repository, validation.Name, base)
	if err != nil {
		return PublishTicketResult{}, err
	}
	result := PublishTicketResult{
		Branch:              validation.Name,
		Sync:                syncResult,
		PullRequest:         newTicketPullRequest(validation.Name, target, request.Draft, request.Body),
		Quality:             quality,
		PostMutationQuality: &quality,
	}
	return result, nil
}

// PushPreparedTicket performs the final pre-push validation and configures the
// upstream only when the branch has not yet been published.
func (service *TicketService) PushPreparedTicket(
	ctx context.Context,
	repository port.RepositoryIdentity,
	name branch.BranchName,
	base *branch.TargetBase,
	workflowManaged bool,
) error {
	if service.sync == nil || service.git == nil {
		return internalDependencyError("ticket publication services")
	}
	if !name.Family().IsOfficialWorkingBranch() {
		return invalidWorkflowInput(
			"only official ticket branches can be pushed by ticket publication",
			"complete any scratch transfer before publishing",
		)
	}
	validation, err := service.sync.ValidatePrePush(ctx, branchapp.PrePushRequest{
		Repository:      repository,
		Name:            name,
		Base:            base,
		WorkflowManaged: workflowManaged,
	})
	if err != nil {
		return err
	}
	return service.git.Push(ctx, repository, name, validation.Publication == branch.PublicationUnpublished)
}

// HasPullRequestPublisher reports whether this process can create a real
// provider-specific pull request rather than only emit its portable intent.
func (service *TicketService) HasPullRequestPublisher() bool {
	return service != nil && service.publisher != nil
}

// PublishPullRequest invokes the configured provider adapter for an already
// prepared pull-request intent and the selected Git remote.
func (service *TicketService) PublishPullRequest(
	ctx context.Context,
	repository port.RepositoryIdentity,
	request port.PullRequest,
) (string, error) {
	if service == nil {
		return "", pullRequestPublisherUnavailable()
	}
	return publishPullRequest(ctx, service.git, service.publisher, repository, request)
}

func publishPullRequest(
	ctx context.Context,
	git port.GitRepository,
	publisher port.PullRequestPublisher,
	repository port.RepositoryIdentity,
	request port.PullRequest,
) (string, error) {
	if publisher == nil || git == nil {
		return "", pullRequestPublisherUnavailable()
	}
	publication, err := pullRequestPublication(ctx, git, repository, request)
	if err != nil {
		return "", err
	}
	published, err := publisher.Publish(ctx, publication)
	if err != nil {
		return "", err
	}
	return published.URL, nil
}

// PreflightPullRequest validates optional hosting configuration before a
// publication-affecting Git push. Adapters without a preflight capability
// remain supported and are invoked only during actual publication.
func (service *TicketService) PreflightPullRequest(
	ctx context.Context,
	repository port.RepositoryIdentity,
	request port.PullRequest,
) error {
	if service == nil || service.publisher == nil || service.git == nil {
		return pullRequestPublisherUnavailable()
	}
	publication, err := pullRequestPublication(ctx, service.git, repository, request)
	if err != nil {
		return err
	}
	validator, ok := service.publisher.(port.PullRequestPublisherPreflight)
	if !ok {
		return nil
	}
	return validator.Validate(ctx, publication)
}

func pullRequestPublication(
	ctx context.Context,
	git port.GitRepository,
	repository port.RepositoryIdentity,
	request port.PullRequest,
) (port.PullRequestPublication, error) {
	// The pull-request description is mandatory for every provider
	// publication; see docs/conventions/pull-requests/description-mandate.md.
	if strings.TrimSpace(request.Body) == "" {
		return port.PullRequestPublication{}, pullRequestBodyRequired()
	}
	remoteURL, err := git.RemoteURL(ctx, repository)
	if err != nil {
		return port.PullRequestPublication{}, err
	}
	return port.PullRequestPublication{
		Repository:  repository,
		RemoteURL:   remoteURL,
		PullRequest: request,
	}, nil
}

func pullRequestPublisherUnavailable() error {
	return problem.New(problem.Details{
		Code:        problem.CodeExternalCommandFailed,
		Category:    problem.CategoryExternal,
		Field:       "pull request publisher",
		Expected:    "a configured hosting-provider adapter",
		Rule:        "a real pull request can be created only through an explicit provider adapter",
		Remediation: "configure a supported hosting-provider adapter or create the displayed provider-neutral pull-request intent manually",
	})
}

// pullRequestBodyRequired enforces the mandatory pull-request description
// owned by docs/conventions/pull-requests/description-mandate.md.
func pullRequestBodyRequired() error {
	return problem.New(problem.Details{
		Code:        problem.CodeInvalidInput,
		Category:    problem.CategoryGovernance,
		Field:       "pull request body",
		Expected:    "a non-empty pull-request description",
		Rule:        "every governed pull request carries a mandatory description",
		Example:     "--body \"Summary: ...\\n\\nScope and Non-Goals: ...\"",
		Remediation: "compose the canonical pull-request description and pass it as the body input",
	})
}

func newTicketPullRequest(name, target branch.BranchName, draft bool, body string) port.PullRequest {
	branchTicket, _ := name.Ticket()
	branchSlug, _ := name.Slug()
	return port.PullRequest{
		Source: name,
		Target: target,
		Ticket: branchTicket,
		Title:  branchTicket.String() + ": " + branchSlug.String(),
		Body:   body,
		Draft:  draft,
	}
}

func (service *TicketService) validateCommitSeries(ctx context.Context, repository port.RepositoryIdentity, name branch.BranchName, base branch.TargetBase) error {
	messages, err := service.git.CommitMessagesSince(ctx, repository, base)
	if err != nil {
		return err
	}
	return branchapp.ValidateCommitSeries(name, messages)
}

func (service *TicketService) runFinalQuality(
	ctx context.Context,
	repository port.RepositoryIdentity,
	name branch.BranchName,
	base branch.TargetBase,
) (port.QualityResult, error) {
	if service.finalQuality != nil {
		return service.finalQuality.RunAndRecord(ctx, repository, name, base)
	}
	if service.quality == nil {
		return port.QualityResult{
			Status: port.QualityUnconfigured,
			Detail: "no quality runner is configured",
		}, nil
	}
	return service.quality.Run(ctx, repository, port.QualityRequest{
		Families: []branch.Family{name.Family()},
	})
}

func resolveTicketBase(
	name branch.BranchName,
	repository port.RepositoryIdentity,
	explicit *branch.TargetBase,
	workflowManaged bool,
) (branch.TargetBase, error) {
	if base, found, err := name.Family().DefaultTargetBase(repository.Remote); err != nil {
		return branch.TargetBase{}, err
	} else if found {
		if explicit != nil && explicit.String() != base.String() {
			if workflowManaged && isWorkflowManagedTicketBase(name.Family(), *explicit) {
				return *explicit, nil
			}
			return branch.TargetBase{}, invalidWorkflowInput(
				"regular ticket publication uses origin/develop unless a dedicated workflow selected another active line",
				"remove --base or use the dedicated stabilization or propagation workflow",
			)
		}
		return base, nil
	}
	if explicit != nil {
		if name.Family() == branch.FamilyHotfix && !isHotfixBase(*explicit) {
			return branch.TargetBase{}, invalidWorkflowInput(
				"a hotfix publish target must be main, release/<semver>, or support/<major.minor>",
				"provide the same active line from which the hotfix branch was created",
			)
		}
		return *explicit, nil
	}
	return branch.TargetBase{}, invalidWorkflowInput(
		"this ticket branch family requires an explicit target base",
		"provide --base for the actual hotfix target line",
	)
}

func resolvePullRequestTarget(
	name branch.BranchName,
	base branch.TargetBase,
	explicit *branch.BranchName,
	workflowManaged bool,
) (branch.BranchName, error) {
	target := mustDevelop()
	if name.Family() == branch.FamilyHotfix ||
		(workflowManaged &&
			(name.Family() == branch.FamilyFix ||
				name.Family() == branch.FamilyDocs ||
				name.Family() == branch.FamilyChore) &&
			base.Branch().Family() != branch.FamilyDevelop) {
		target = base.Branch()
	}
	if explicit != nil && explicit.String() != target.String() {
		return branch.BranchName{}, invalidWorkflowInput(
			"the pull request target must match the branch workflow target line",
			"remove --target or supply the actual affected or integration line",
		)
	}
	return target, nil
}

func isHotfixBase(base branch.TargetBase) bool {
	switch base.Branch().Family() {
	case branch.FamilyMain, branch.FamilyRelease, branch.FamilySupport:
		return true
	default:
		return false
	}
}

func isSharedLineBase(base branch.TargetBase) bool {
	switch base.Branch().Family() {
	case branch.FamilyMain, branch.FamilyDevelop, branch.FamilyRelease, branch.FamilySupport:
		return true
	default:
		return false
	}
}

func isWorkflowManagedTicketBase(family branch.Family, base branch.TargetBase) bool {
	switch family {
	case branch.FamilyFix:
		return isSharedLineBase(base)
	case branch.FamilyDocs, branch.FamilyChore:
		return base.Branch().Family() == branch.FamilyRelease
	default:
		return false
	}
}

func mustDevelop() branch.BranchName {
	// This literal is part of the product's fixed branch taxonomy and is
	// independently validated by the branch domain tests.
	name, _ := branch.ParseName("develop")
	return name
}

func invalidWorkflowInput(rule, remediation string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeInvalidInput,
		Category:    problem.CategoryGovernance,
		Field:       "workflow",
		Expected:    "a valid workflow request",
		Rule:        rule,
		Remediation: remediation,
	})
}

func repositoryRequired() error {
	return problem.New(problem.Details{
		Code:        problem.CodeRepositoryNotFound,
		Category:    problem.CategoryRepository,
		Field:       "repository",
		Expected:    "a discovered local Git repository",
		Rule:        "workflow operations require a repository root",
		Remediation: "run from a Git repository or pass --repo",
	})
}

func internalDependencyError(name string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeInternal,
		Category:    problem.CategoryInternal,
		Field:       "dependency",
		Actual:      name,
		Expected:    "a configured workflow dependency",
		Rule:        "workflow services are composed with required use cases and adapters",
		Remediation: "fix the composition root",
	})
}
