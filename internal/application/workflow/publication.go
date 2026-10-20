package workflow

import (
	"context"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

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

// publicationResumePlan renders the publication resume steps: the birth
// topology proof and the separately confirmed shared-line publication.
func publicationResumePlan(revision string, remote string) []branchapp.PlanStep {
	return []branchapp.PlanStep{
		{Action: "prove-birth", Detail: "birth-topology proof: exactly one commit, main and develop on the shared genesis revision " + revision + ", verified signature, materialized hook boundary"},
		{Action: "publish", Detail: "push main and develop to " + remote + " after the separate publication confirmation"},
	}
}
