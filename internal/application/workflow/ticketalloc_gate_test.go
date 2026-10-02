package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/application/ticketalloc"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/releaserequest"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

// fakeAllocationSurfaces is the configurable allocation-surface fake for the
// workflow package gates. The default has empty git-core surfaces and no
// platform provider, so every number is free.
type fakeAllocationSurfaces struct {
	localBranches  []string
	remoteBranches []string
	subjects       []string
	requestRecords []port.ProtectedLineRequestRecord
}

func (fake *fakeAllocationSurfaces) LocalBranches(context.Context, port.RepositoryIdentity) ([]branch.BranchName, error) {
	return parseAllocationBranches(fake.localBranches), nil
}

func (fake *fakeAllocationSurfaces) RemoteBranches(context.Context, port.RepositoryIdentity) ([]branch.BranchName, error) {
	return parseAllocationBranches(fake.remoteBranches), nil
}

func (fake *fakeAllocationSurfaces) CommitSubjects(context.Context, port.RepositoryIdentity) ([]string, error) {
	return fake.subjects, nil
}

func (fake *fakeAllocationSurfaces) ListHotfixReleaseRecords(context.Context, port.RepositoryIdentity) ([]port.HotfixReleaseRecord, error) {
	return nil, nil
}

func (fake *fakeAllocationSurfaces) RemoteURL(context.Context, port.RepositoryIdentity) (string, error) {
	return "https://example.invalid/governance.git", nil
}

func (fake *fakeAllocationSurfaces) ListPullRequests(context.Context, port.PullRequestInventoryQuery) ([]port.PullRequestSummary, error) {
	return nil, nil
}

func (fake *fakeAllocationSurfaces) ListProtectedLineRequests(context.Context, port.ProtectedLineRequestInventoryQuery) ([]port.ProtectedLineRequestRecord, error) {
	return fake.requestRecords, nil
}

func (fake *fakeAllocationSurfaces) InspectAppPermissions(context.Context, port.AppPermissionQuery) (port.AppPermissionSnapshot, error) {
	return port.AppPermissionSnapshot{
		Slug:        "gate-app",
		Permissions: map[string]string{"pull_requests": "write", "deployments": "write"},
	}, nil
}

func parseAllocationBranches(raw []string) []branch.BranchName {
	names := make([]branch.BranchName, 0, len(raw))
	for _, value := range raw {
		names = append(names, mustBranch(value))
	}
	return names
}

// newTestAllocation creates the allocation service over the configurable
// surface fake.
func newTestAllocation(surfaces *fakeAllocationSurfaces) *ticketalloc.Service {
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

// allocationRequestRecord builds one valid durable request record for gate
// fixtures. Its ticket is the number holder no pull-request scan can see.
func allocationRequestRecord(t *testing.T, id ticket.ID) port.ProtectedLineRequestRecord {
	releaseBranch, err := branch.NewReleaseBranch(mustReleaseVersion(t, "1.2.0"))
	if err != nil {
		panic(err)
	}
	request, err := releaserequest.New(releaserequest.Input{
		ID:               "req-allocation-1",
		Repository:       "acme/governance",
		Operation:        releaserequest.OperationRelease,
		Ticket:           id,
		Version:          "1.2.0",
		Target:           releaseBranch,
		Source:           mustBranch("develop"),
		SourceSHA:        strings.Repeat("a", 40),
		Requester:        "release-controller",
		ExpectedExecutor: "execute-protected-line-request.yml",
		ParentRunID:      "12345",
		ExpiresAt:        time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
		IdempotencyKey:   strings.Repeat("b", 64),
		DeploymentID:     424242,
		State:            releaserequest.StateRequestAuthorized,
	}, time.Date(2026, 9, 30, 16, 53, 44, 0, time.UTC))
	if err != nil {
		panic(err)
	}
	return port.ProtectedLineRequestRecord{
		Request:   request,
		CreatedAt: time.Date(2026, 9, 30, 16, 53, 44, 0, time.UTC),
	}
}

// TestStartTicketGateBlocksAllocatedNumbers proves the intake gate of
// workflow ticket start: the requested number is validated against a fresh
// full-surface inventory immediately before the branch binds it.
func TestStartTicketGateBlocksAllocatedNumbers(t *testing.T) {
	t.Parallel()

	surfaces := &fakeAllocationSurfaces{remoteBranches: []string{"feature/ABC-123-add-export"}}
	service := newTicketServiceWithAllocation(newTicketGateGit(), nil, nil, newTestAllocation(surfaces))
	result, err := service.StartTicket(context.Background(), StartTicketRequest{
		Repository: testRepository(),
		Family:     branch.FamilyFeature,
		Ticket:     mustTicket("ABC-123"),
		Slug:       mustSlug("add-export"),
	})
	if result.Official.Name.String() != "" {
		t.Fatalf("StartTicket() created a branch despite the allocation gate: %#v", result)
	}
	assertProblemCode(t, err, problem.CodeTicketNumberAlreadyAllocated)
	if typed, ok := problem.As(err); !ok || !strings.Contains(typed.Context, "remote branch ref") {
		t.Fatalf("collision evidence missing the holder surface: %#v", typed)
	}
}

// TestStartTicketGateAcceptsFreeNumbers proves the gate passes for a number
// no surface holds, including surfaces carrying other keys.
func TestStartTicketGateAcceptsFreeNumbers(t *testing.T) {
	t.Parallel()

	surfaces := &fakeAllocationSurfaces{
		localBranches:  []string{"feature/XYZ-9-other-key"},
		remoteBranches: []string{"release/1.2.0"},
		subjects:       []string{"feat(XYZ-9): unrelated work", "chore(deps): bump actions"},
	}
	service := newTicketServiceWithAllocation(newTicketGateGit(), nil, nil, newTestAllocation(surfaces))
	result, err := service.StartTicket(context.Background(), StartTicketRequest{
		Repository: testRepository(),
		Family:     branch.FamilyFeature,
		Ticket:     mustTicket("ABC-124"),
		Slug:       mustSlug("add-export"),
	})
	if err != nil {
		t.Fatalf("StartTicket() error = %v", err)
	}
	if result.Official.Name.String() != "feature/ABC-124-add-export" {
		t.Fatalf("StartTicket() = %#v", result)
	}
}

// TestStartTicketGateFailsClosedWhenUnwired proves an unwired gate fails
// closed instead of allocating an unverified number.
func TestStartTicketGateFailsClosedWhenUnwired(t *testing.T) {
	t.Parallel()

	service := newTicketServiceWithAllocation(newTicketGateGit(), nil, nil, nil)
	_, err := service.StartTicket(context.Background(), StartTicketRequest{
		Repository: testRepository(),
		Family:     branch.FamilyFeature,
		Ticket:     mustTicket("ABC-124"),
		Slug:       mustSlug("add-export"),
	})
	assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
}

// TestRequestProtectedLineGateBlocksRequestRecordNumbers is the RG-37
// regression: a ticket number held only by a durable protected-line request
// record is invisible to every branch and pull-request scan, and the gate
// blocks it anyway.
func TestRequestProtectedLineGateBlocksRequestRecordNumbers(t *testing.T) {
	t.Parallel()

	surfaces := &fakeAllocationSurfaces{
		requestRecords: []port.ProtectedLineRequestRecord{allocationRequestRecord(t, mustTicket("ABC-37"))},
	}
	git := newReleaseWhiteboxGit()
	provider := &fakeProtectedLineRequestProvider{}
	service := NewReleaseService(
		branchapp.NewService(git, &fakeKeyPolicy{}),
		git,
		nil,
	).WithTicketAllocation(newTestAllocation(surfaces)).WithProtectedLineRequestProvider(provider)
	_, err := service.RequestProtectedLine(context.Background(), RequestProtectedLineRequest{
		Repository:  testRepository(),
		Ticket:      mustTicket("ABC-37"),
		Operation:   releaserequest.OperationRelease,
		Version:     "1.2.0",
		Requester:   "release-controller",
		ParentRunID: "12345",
	})
	if provider.authorized {
		t.Fatal("RequestProtectedLine() persisted the request despite the allocation gate")
	}
	assertProblemCode(t, err, problem.CodeTicketNumberAlreadyAllocated)
	if typed, ok := problem.As(err); !ok || !strings.Contains(typed.Context, ticketalloc.SurfaceProtectedLineRequests) {
		t.Fatalf("collision evidence missing the request-record surface: %#v", typed)
	}
}

// TestRequestProtectedLineGateFailsClosedWhenUnwired proves the protected-line
// request path fails closed without the allocation gate.
func TestRequestProtectedLineGateFailsClosedWhenUnwired(t *testing.T) {
	t.Parallel()

	git := newReleaseWhiteboxGit()
	service := NewReleaseService(
		branchapp.NewService(git, &fakeKeyPolicy{}),
		git,
		nil,
	).WithProtectedLineRequestProvider(&fakeProtectedLineRequestProvider{})
	_, err := service.RequestProtectedLine(context.Background(), RequestProtectedLineRequest{
		Repository:  testRepository(),
		Ticket:      mustTicket("ABC-37"),
		Operation:   releaserequest.OperationRelease,
		Version:     "1.2.0",
		Requester:   "release-controller",
		ParentRunID: "12345",
	})
	assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
}

// TestBootstrapGenesisGateBlocksAllocatedNumbers proves the genesis ticket is
// validated against a fresh full-surface inventory before the genesis commit
// binds it.
func TestBootstrapGenesisGateBlocksAllocatedNumbers(t *testing.T) {
	t.Parallel()

	surfaces := &fakeAllocationSurfaces{remoteBranches: []string{"feature/GOV-116-workflow-bootstrap"}}
	service := NewBootstrapService(
		branchapp.NewService(newBootstrapGit(), &fakeKeyPolicy{}),
		newBootstrapGit(),
		&fakeKeyPolicy{},
		newBootstrapTools(),
	).WithTicketAllocation(newTestAllocation(surfaces)).
		WithSigningReadiness(func(port.SigningConfiguration) error { return nil }).
		WithPolicySnapshot(func() string { return "snapshot" }).
		WithClock(func() time.Time { return time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC) })
	_, err := service.Bootstrap(context.Background(), bootstrapRequest())
	assertProblemCode(t, err, problem.CodeTicketNumberAlreadyAllocated)
}

// TestBootstrapGenesisGateFailsClosedWhenUnwired proves the repository birth
// fails closed without the allocation gate.
func TestBootstrapGenesisGateFailsClosedWhenUnwired(t *testing.T) {
	t.Parallel()

	git := newBootstrapGit()
	service := NewBootstrapService(
		branchapp.NewService(git, &fakeKeyPolicy{}),
		git,
		&fakeKeyPolicy{},
		newBootstrapTools(),
	).WithSigningReadiness(func(port.SigningConfiguration) error { return nil }).
		WithPolicySnapshot(func() string { return "snapshot" })
	_, err := service.Bootstrap(context.Background(), bootstrapRequest())
	assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
}

// newTicketServiceWithAllocation builds a ticket service whose branch service
// carries the given allocation gate; see ticketalloc_gate_test.go.
func newTicketServiceWithAllocation(
	git *fakeGitRepository,
	quality port.QualityRunner,
	publisher port.PullRequestPublisher,
	allocation *ticketalloc.Service,
) *TicketService {
	keys := &fakeKeyPolicy{}
	branches := branchapp.NewService(git, keys).WithTicketAllocation(allocation)
	sync := branchapp.NewSynchronizer(git, branches, quality)
	return NewTicketService(branches, sync, git, quality, publisher)
}

// ticketGateGit is the minimal fake for gate-path branch creations: a born,
// clean repository.
func newTicketGateGit() *fakeGitRepository {
	return &fakeGitRepository{hasCommits: true, clean: true}
}

// fakeProtectedLineRequestProvider records authorized protected-line requests.
type fakeProtectedLineRequestProvider struct {
	authorized bool
}

func (provider *fakeProtectedLineRequestProvider) AuthorizeProtectedLineRequest(
	context.Context,
	port.ProtectedLineRequestAuthorization,
) (port.ProtectedLineRequestResult, error) {
	provider.authorized = true
	return port.ProtectedLineRequestResult{}, errors.New("must not authorize an allocated ticket number")
}

func (provider *fakeProtectedLineRequestProvider) AuthorizeProtectedLineExecution(
	context.Context,
	port.ProtectedLineExecutionAuthorization,
) (port.ProtectedLineExecutionPlan, error) {
	return port.ProtectedLineExecutionPlan{}, nil
}

func (provider *fakeProtectedLineRequestProvider) FinalizeProtectedLineRequest(
	context.Context,
	port.ProtectedLineFinalizationRequest,
) (port.ProtectedLineFinalizationResult, error) {
	return port.ProtectedLineFinalizationResult{}, nil
}
