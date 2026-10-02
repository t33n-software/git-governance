package branchapp

import (
	"context"
	"errors"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/application/ticketalloc"
	domainbranch "github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// allocationSurfaces is the configurable allocation-surface fake for the
// branch package gates. The default holds nothing, so every number is free.
type allocationSurfaces struct {
	branchNames []string
}

func (surfaces *allocationSurfaces) LocalBranches(context.Context, port.RepositoryIdentity) ([]domainbranch.BranchName, error) {
	return parseAllocationBranches(surfaces.branchNames), nil
}

func (surfaces *allocationSurfaces) RemoteBranches(context.Context, port.RepositoryIdentity) ([]domainbranch.BranchName, error) {
	return nil, nil
}

func (surfaces *allocationSurfaces) CommitSubjects(context.Context, port.RepositoryIdentity) ([]string, error) {
	return nil, nil
}

func (surfaces *allocationSurfaces) ListHotfixReleaseRecords(context.Context, port.RepositoryIdentity) ([]port.HotfixReleaseRecord, error) {
	return nil, nil
}

func (surfaces *allocationSurfaces) RemoteURL(context.Context, port.RepositoryIdentity) (string, error) {
	return "https://example.invalid/repo.git", nil
}

func (surfaces *allocationSurfaces) ListPullRequests(context.Context, port.PullRequestInventoryQuery) ([]port.PullRequestSummary, error) {
	return nil, nil
}

func (surfaces *allocationSurfaces) ListProtectedLineRequests(context.Context, port.ProtectedLineRequestInventoryQuery) ([]port.ProtectedLineRequestRecord, error) {
	return nil, nil
}

func (surfaces *allocationSurfaces) InspectAppPermissions(context.Context, port.AppPermissionQuery) (port.AppPermissionSnapshot, error) {
	return port.AppPermissionSnapshot{
		Slug:        "gate-app",
		Permissions: map[string]string{"pull_requests": "write", "deployments": "write"},
	}, nil
}

func parseAllocationBranches(raw []string) []domainbranch.BranchName {
	names := make([]domainbranch.BranchName, 0, len(raw))
	for _, value := range raw {
		names = append(names, mustBranch(value))
	}
	return names
}

// newTestAllocation creates the allocation service over the surface fake.
func newTestAllocation(surfaces *allocationSurfaces) *ticketalloc.Service {
	return ticketalloc.New(ticketalloc.Dependencies{
		LocalBranches:         surfaces,
		RemoteBranches:        surfaces,
		CommitSubjects:        surfaces,
		HotfixRecords:         surfaces,
		RemoteURL:             surfaces.RemoteURL,
		PullRequests:          surfaces,
		ProtectedLineRequests: surfaces,
		AppPermissions:        surfaces,
	})
}

// gatedService creates the standard test service with the allocation gate.
func gatedService(git port.GitRepository, surfaces *allocationSurfaces) *Service {
	return NewService(git, &fakeKeyPolicy{}).WithTicketAllocation(newTestAllocation(surfaces))
}

// TestCreateGateBlocksAllocatedTicketNumbers proves the intake gate of branch
// creation: the requested number is validated against a fresh full-surface
// inventory immediately before the branch binds it, and the collision record
// carries the holder evidence.
func TestCreateGateBlocksAllocatedTicketNumbers(t *testing.T) {
	t.Parallel()

	surfaces := &allocationSurfaces{branchNames: []string{"feature/ABC-123-add-export"}}
	service := gatedService(&fakeGitRepository{hasCommits: true, clean: true}, surfaces)
	_, err := service.Create(context.Background(), regularRequest())
	assertProblemCode(t, err, problem.CodeTicketNumberAlreadyAllocated)
}

// TestCreateGateAcceptsFreeNumbers proves the gate passes for numbers no
// surface holds.
func TestCreateGateAcceptsFreeNumbers(t *testing.T) {
	t.Parallel()

	surfaces := &allocationSurfaces{branchNames: []string{"feature/XYZ-7-other-work"}}
	git := &fakeGitRepository{hasCommits: true, clean: true}
	service := gatedService(git, surfaces)
	result, err := service.Create(context.Background(), regularRequest())
	if err != nil {
		t.Fatal(err)
	}
	if result.Name.String() != "feature/ABC-123-add-export" {
		t.Fatalf("Create() branch = %q", result.Name.String())
	}
}

// TestCreateGateSkipsSubordinateScratchBranches proves a scratch branch from
// an already gated same-ticket official branch is not gated again: its number
// is the official branch's own allocation.
func TestCreateGateSkipsSubordinateScratchBranches(t *testing.T) {
	t.Parallel()

	surfaces := &allocationSurfaces{branchNames: []string{"feature/ABC-123-add-export"}}
	git := &fakeGitRepository{hasCommits: true, clean: true}
	service := gatedService(git, surfaces)
	base, err := domainbranch.NewLocalBase(mustBranch("feature/ABC-123-add-export"))
	if err != nil {
		t.Fatal(err)
	}
	request := regularRequest()
	request.Family = domainbranch.FamilyScratch
	request.Slug = mustSlug("exploration")
	request.Base = &base
	result, err := service.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Name.String() != "scratch/ABC-123-exploration" {
		t.Fatalf("Create() branch = %q", result.Name.String())
	}
}

// TestCreateGateFailsClosedWhenUnwired proves an unwired gate fails closed
// instead of allocating an unverified number.
func TestCreateGateFailsClosedWhenUnwired(t *testing.T) {
	t.Parallel()

	service := NewService(&fakeGitRepository{hasCommits: true, clean: true}, &fakeKeyPolicy{})
	_, err := service.Create(context.Background(), regularRequest())
	assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
}

// TestCreateGateFollowsFreshWorkAndContinuationSemantics proves the gate
// binds fresh numbers strictly: regular non-managed work and workflow-managed
// hotfix starts require a free number, while workflow-managed fix work
// continues its ticket's stabilization or propagation across active lines.
func TestCreateGateFollowsFreshWorkAndContinuationSemantics(t *testing.T) {
	t.Parallel()

	surfaces := &allocationSurfaces{branchNames: []string{"feature/ABC-123-add-export"}}
	creations := []struct {
		name            string
		family          domainbranch.Family
		workflowManaged bool
		wantBlocked     bool
	}{
		{name: "regular feature work", family: domainbranch.FamilyFeature, wantBlocked: true},
		{name: "workflow-managed hotfix start", family: domainbranch.FamilyHotfix, workflowManaged: true, wantBlocked: true},
		{name: "workflow-managed fix continuation", family: domainbranch.FamilyFix, workflowManaged: true},
		{name: "workflow-managed docs continuation", family: domainbranch.FamilyDocs, workflowManaged: true},
	}
	for _, creation := range creations {
		creation := creation
		t.Run(creation.name, func(t *testing.T) {
			t.Parallel()
			git := &fakeGitRepository{hasCommits: true, clean: true}
			service := gatedService(git, surfaces)
			request := regularRequest()
			request.Family = creation.family
			request.WorkflowManaged = creation.workflowManaged
			if creation.workflowManaged {
				base := mustBase("origin", "release/2.8.0")
				request.Base = &base
			}
			_, err := service.Create(context.Background(), request)
			if creation.wantBlocked {
				assertProblemCode(t, err, problem.CodeTicketNumberAlreadyAllocated)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestCreateDryRunGateBlocksAllocatedTicketNumbers proves the dry-run plan
// gates the requested number against a fresh full-surface allocation
// inventory as well: a planned creation never bypasses the intake gate.
func TestCreateDryRunGateBlocksAllocatedTicketNumbers(t *testing.T) {
	t.Parallel()

	surfaces := &allocationSurfaces{branchNames: []string{"feature/ABC-123-add-export"}}
	service := gatedService(&fakeGitRepository{hasCommits: true, clean: true}, surfaces)
	request := regularRequest()
	request.DryRun = true
	_, err := service.Create(context.Background(), request)
	assertProblemCode(t, err, problem.CodeTicketNumberAlreadyAllocated)
}

// TestCreateStopsOnCancelledContext proves a cancelled context stops branch
// creation before any mutation or inventory read.
func TestCreateStopsOnCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := gatedService(&fakeGitRepository{hasCommits: true, clean: true}, &allocationSurfaces{})
	_, err := service.Create(ctx, regularRequest())
	assertProblemCode(t, err, problem.CodeOperationCancelled)
}

// TestCreatePropagatesBranchCreationFailure proves a failing Git branch
// creation propagates its typed failure after the intake gate passed.
func TestCreatePropagatesBranchCreationFailure(t *testing.T) {
	t.Parallel()

	failure := errors.New("branch creation failed")
	git := &fakeGitRepository{hasCommits: true, clean: true, createBranchErr: failure}
	service := gatedService(git, &allocationSurfaces{})
	_, err := service.Create(context.Background(), regularRequest())
	if !errors.Is(err, failure) {
		t.Fatalf("Create() error = %v; want the propagated creation failure", err)
	}
}
