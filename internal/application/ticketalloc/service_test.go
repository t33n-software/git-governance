package ticketalloc

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/hotfix"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/releaserequest"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

// fakeSurfaces is the configurable allocation-surface fake: every surface is
// settable independently, so each inventory branch has a direct fixture. The
// permission fake carries the provider app class; an unset permission map
// defaults to the full-carry class so the scanned-surface fixtures stay
// focused on their own arrangement.
type fakeSurfaces struct {
	localBranchNames      []string
	remoteBranchNames     []string
	subjects              []string
	recordDocuments       []string
	recordLocations       []string
	worktrees             []port.WorktreeEntry
	pullRequests          []port.PullRequestSummary
	requestRecords        []port.ProtectedLineRequestRecord
	permissions           map[string]string
	remoteURLErr          error
	localErr              error
	remoteErr             error
	subjectsErr           error
	recordsErr            error
	worktreesErr          error
	pullRequestsErr       error
	requestRecordsErr     error
	permissionsErr        error
	permissionInspections int
	inspectedCapabilities []port.CredentialCapability
	permissionMissing     port.CredentialCapability
	deploymentsErr        error
}

func (fake *fakeSurfaces) LocalBranches(context.Context, port.RepositoryIdentity) ([]branch.BranchName, error) {
	if fake.localErr != nil {
		return nil, fake.localErr
	}
	return parseNames(fake.localBranchNames), nil
}

func (fake *fakeSurfaces) RemoteBranches(context.Context, port.RepositoryIdentity) ([]branch.BranchName, error) {
	if fake.remoteErr != nil {
		return nil, fake.remoteErr
	}
	return parseNames(fake.remoteBranchNames), nil
}

func (fake *fakeSurfaces) CommitSubjects(context.Context, port.RepositoryIdentity) ([]string, error) {
	if fake.subjectsErr != nil {
		return nil, fake.subjectsErr
	}
	return fake.subjects, nil
}

func (fake *fakeSurfaces) ListHotfixReleaseRecords(context.Context, port.RepositoryIdentity) ([]port.HotfixReleaseRecord, error) {
	if fake.recordsErr != nil {
		return nil, fake.recordsErr
	}
	records := make([]port.HotfixReleaseRecord, 0, len(fake.recordDocuments))
	for index, document := range fake.recordDocuments {
		parsed, err := hotfix.ParseRecord([]byte(document))
		if err != nil {
			return nil, err
		}
		location := parsed.Ticket().String() + ".json"
		if index < len(fake.recordLocations) {
			location = fake.recordLocations[index]
		}
		records = append(records, port.HotfixReleaseRecord{Record: parsed, Location: location})
	}
	return records, nil
}

// WorktreeList serves the configurable task-worktree surface fixture.
func (fake *fakeSurfaces) WorktreeList(context.Context, port.RepositoryIdentity) ([]port.WorktreeEntry, error) {
	if fake.worktreesErr != nil {
		return nil, fake.worktreesErr
	}
	return fake.worktrees, nil
}

func (fake *fakeSurfaces) RemoteURL(context.Context, port.RepositoryIdentity) (string, error) {
	if fake.remoteURLErr != nil {
		return "", fake.remoteURLErr
	}
	return "https://example.invalid/governance.git", nil
}

func (fake *fakeSurfaces) ListPullRequests(context.Context, port.PullRequestInventoryQuery) ([]port.PullRequestSummary, error) {
	if fake.pullRequestsErr != nil {
		return nil, fake.pullRequestsErr
	}
	return fake.pullRequests, nil
}

func (fake *fakeSurfaces) ListProtectedLineRequests(context.Context, port.ProtectedLineRequestInventoryQuery) ([]port.ProtectedLineRequestRecord, error) {
	if fake.requestRecordsErr != nil {
		return nil, fake.requestRecordsErr
	}
	return fake.requestRecords, nil
}

func (fake *fakeSurfaces) InspectAppPermissions(_ context.Context, query port.AppPermissionQuery) (port.AppPermissionSnapshot, error) {
	fake.permissionInspections++
	fake.inspectedCapabilities = append(fake.inspectedCapabilities, query.Capability)
	if fake.permissionsErr != nil {
		return port.AppPermissionSnapshot{}, fake.permissionsErr
	}
	if query.Capability != "" && query.Capability == fake.permissionMissing {
		return port.AppPermissionSnapshot{}, problem.Wrap(problem.Details{
			Code:        problem.CodeConfigurationUnavailable,
			Category:    problem.CategoryConfig,
			Field:       "GitHub App session capability",
			Actual:      string(query.Capability),
			Expected:    "a GitHub App session whose app class carries the " + string(query.Capability) + " permission",
			Rule:        "capability-scoped session selection binds only an app class that carries the requested permission",
			Remediation: "run auth login github with the GitHub App whose class carries the requested permission",
		}, port.ErrCapabilitySessionMissing)
	}
	if query.Capability == port.CapabilityDeployments && fake.deploymentsErr != nil {
		return port.AppPermissionSnapshot{}, fake.deploymentsErr
	}
	if fake.permissions == nil {
		return port.AppPermissionSnapshot{
			Slug:        "fixture-app",
			Permissions: map[string]string{"pull_requests": "write", "deployments": "write"},
		}, nil
	}
	return port.AppPermissionSnapshot{Slug: "fixture-app", Permissions: fake.permissions}, nil
}

func parseNames(raw []string) []branch.BranchName {
	names := make([]branch.BranchName, 0, len(raw))
	for _, value := range raw {
		name, err := branch.ParseName(value)
		if err != nil {
			panic(err)
		}
		names = append(names, name)
	}
	return names
}

const validRecordDocument = `{"schemaVersion":1,"ticket":"ABC-9","incident":"INC-42",` +
	`"affectedLine":"main","targetVersion":"1.2.1","previousTag":"v1.2.0",` +
	`"expectedPullRequest":{"source":"hotfix/ABC-9-payment-timeout","target":"main"},` +
	`"manifest":["aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"],` +
	`"commitBudgetException":"","scopeEscalationApproval":"","propagationTargets":[]}`

// requestRecord builds one valid durable protected-line request record whose
// ticket number no pull request, branch, or envelope carries.
func requestRecord(rawTicket string) port.ProtectedLineRequestRecord {
	id := mustID(rawTicket)
	releaseBranch, err := branch.NewReleaseBranch(mustVersion("1.1.1"))
	if err != nil {
		panic(err)
	}
	request, err := releaserequest.New(releaserequest.Input{
		ID:               "req-" + id.String(),
		Repository:       "acme/governance",
		Operation:        releaserequest.OperationRelease,
		Ticket:           id,
		Version:          "1.1.1",
		Target:           releaseBranch,
		Source:           mustName("develop"),
		SourceSHA:        strings.Repeat("c", 40),
		Requester:        "d-demand-priv",
		ExpectedExecutor: "execute-protected-line-request.yml",
		ParentRunID:      "6765149073",
		ExpiresAt:        time.Date(2026, 9, 30, 20, 53, 44, 0, time.UTC),
		IdempotencyKey:   strings.Repeat("d", 64),
		DeploymentID:     6765149073,
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

func TestInventoryDerivesAllocationFromEverySurface(t *testing.T) {
	t.Parallel()

	surfaces := &fakeSurfaces{
		localBranchNames:  []string{"feature/ABC-2-foundation", "feature/XYZ-5-other-key"},
		remoteBranchNames: []string{"scratch/ABC-45-exploration", "release/1.1.1"},
		subjects: []string{
			"feat(ABC-10): add export button",
			"chore(ABC-120): align gitattributes with the git lfs class policy",
			"chore(BOOTSTRAP-1): establish git-governance CLI",
			"chore(deps): bump actions/checkout",
			"added unit tests",
			"feat(XYZ-5): unrelated key",
		},
		recordDocuments: []string{validRecordDocument},
		pullRequests: []port.PullRequestSummary{
			{Number: "55", Title: "ABC-37: allow-git-lfs-in-canonical-gitattributes", Author: "CyberT33N", CreatedAt: time.Date(2026, 9, 30, 20, 2, 32, 0, time.UTC)},
			{Number: "12", Title: "fix(ABC-11): synchronize protected workflow contract", Author: "CyberT33N", CreatedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)},
			{Number: "17", Title: "Release 1.0.0 into main", Author: "CyberT33N", CreatedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)},
			{Number: "18", Title: "chore(deps): bump actions/attest", Author: "dependabot", CreatedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)},
			{Number: "19", Title: "XYZ-8: other-key-work", Author: "someone", CreatedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)},
		},
		requestRecords: []port.ProtectedLineRequestRecord{requestRecord("ABC-35")},
	}
	service := New(Dependencies{
		LocalBranches:         surfaces,
		RemoteBranches:        surfaces,
		CommitSubjects:        surfaces,
		HotfixRecords:         surfaces,
		Worktrees:             surfaces,
		RemoteURL:             surfaces.RemoteURL,
		PullRequests:          surfaces,
		ProtectedLineRequests: surfaces,
		AppPermissions:        surfaces,
	})

	allocation, err := service.Inventory(context.Background(), testRepository(), mustKey("ABC"))
	if err != nil {
		t.Fatal(err)
	}

	expectedSurfaces := []string{
		SurfaceBranchRefs,
		SurfaceCommitEnvelopes,
		SurfaceHotfixRecords,
		SurfaceWorktreeRegistry,
		SurfacePullRequestTitles,
		SurfaceProtectedLineRequests,
	}
	for _, expected := range expectedSurfaces {
		found := false
		for _, status := range allocation.Surfaces {
			if status.Surface == expected && status.State == SurfaceScanned {
				found = true
			}
		}
		if !found {
			t.Fatalf("surface %q is not reported as scanned: %#v", expected, allocation.Surfaces)
		}
	}

	expectedHolders := map[string]int{
		"2":   1,
		"10":  1,
		"11":  1,
		"35":  1,
		"37":  1,
		"45":  1,
		"120": 1,
		"9":   1,
	}
	for number, count := range expectedHolders {
		if len(allocation.Holders[number]) != count {
			t.Fatalf("holders for %q = %#v", number, allocation.Holders[number])
		}
	}
	if len(allocation.Holders) != len(expectedHolders) {
		t.Fatalf("holders = %#v", allocation.Holders)
	}
	if allocation.NextFree != "121" {
		t.Fatalf("next free = %q; want 121 derived numerically above the highest holder", allocation.NextFree)
	}
	if allocation.Key != "ABC" {
		t.Fatalf("key = %q", allocation.Key)
	}
}

func TestInventoryProviderNoneNamesAbsentPlatformSurfaces(t *testing.T) {
	t.Parallel()

	surfaces := &fakeSurfaces{localBranchNames: []string{"feature/ABC-2-foundation"}}
	service := New(Dependencies{
		LocalBranches:  surfaces,
		RemoteBranches: surfaces,
		CommitSubjects: surfaces,
		HotfixRecords:  surfaces,
		Worktrees:      surfaces,
	})

	allocation, err := service.Inventory(context.Background(), testRepository(), mustKey("ABC"))
	if err != nil {
		t.Fatal(err)
	}
	absent := map[string]bool{}
	for _, status := range allocation.Surfaces {
		if status.State == SurfaceAbsent {
			absent[status.Surface] = true
		}
	}
	if !absent[SurfacePullRequestTitles] || !absent[SurfaceProtectedLineRequests] {
		t.Fatalf("provider-less inventory must name both absent platform surfaces: %#v", allocation.Surfaces)
	}
	if allocation.NextFree != "3" {
		t.Fatalf("next free = %q", allocation.NextFree)
	}
}

func TestInventoryFailsClosedOnUnreadableSurfaces(t *testing.T) {
	t.Parallel()

	failure := errors.New("surface unavailable")
	readFailures := []struct {
		name     string
		surfaces *fakeSurfaces
	}{
		{name: "local branch read failure", surfaces: &fakeSurfaces{localErr: failure}},
		{name: "remote branch read failure", surfaces: &fakeSurfaces{remoteErr: failure}},
		{name: "commit subject read failure", surfaces: &fakeSurfaces{subjectsErr: failure}},
		{name: "hotfix record read failure", surfaces: &fakeSurfaces{recordsErr: failure}},
		{name: "worktree read failure", surfaces: &fakeSurfaces{worktreesErr: failure}},
		{name: "pull request read failure", surfaces: &fakeSurfaces{pullRequestsErr: failure}},
		{name: "request record read failure", surfaces: &fakeSurfaces{requestRecordsErr: failure}},
	}
	for _, testCase := range readFailures {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			service := fullService(testCase.surfaces)
			_, err := service.Inventory(context.Background(), testRepository(), mustKey("ABC"))
			if !errors.Is(err, failure) {
				t.Fatalf("Inventory() error = %v; want the propagated surface failure", err)
			}
		})
	}
}

func TestInventoryFailsClosedOnMissingCapabilities(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		capability string
		configure  func(*Dependencies, *fakeSurfaces)
	}{
		{
			name:       "missing local branch capability",
			capability: "local branch listing",
			configure:  func(dependencies *Dependencies, _ *fakeSurfaces) { dependencies.LocalBranches = nil },
		},
		{
			name:       "missing remote branch capability",
			capability: "remote branch listing",
			configure:  func(dependencies *Dependencies, _ *fakeSurfaces) { dependencies.RemoteBranches = nil },
		},
		{
			name:       "missing commit subject capability",
			capability: "commit subject history",
			configure:  func(dependencies *Dependencies, _ *fakeSurfaces) { dependencies.CommitSubjects = nil },
		},
		{
			name:       "missing hotfix record capability",
			capability: "hotfix release record listing",
			configure:  func(dependencies *Dependencies, _ *fakeSurfaces) { dependencies.HotfixRecords = nil },
		},
		{
			name:       "missing worktree inventory capability",
			capability: "worktree inventory listing",
			configure:  func(dependencies *Dependencies, _ *fakeSurfaces) { dependencies.Worktrees = nil },
		},
		{
			name:       "missing remote URL resolution",
			capability: "remote URL resolution",
			configure:  func(dependencies *Dependencies, _ *fakeSurfaces) { dependencies.RemoteURL = nil },
		},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			surfaces := &fakeSurfaces{}
			dependencies := Dependencies{
				LocalBranches:         surfaces,
				RemoteBranches:        surfaces,
				CommitSubjects:        surfaces,
				HotfixRecords:         surfaces,
				Worktrees:             surfaces,
				RemoteURL:             surfaces.RemoteURL,
				PullRequests:          surfaces,
				ProtectedLineRequests: surfaces,
			}
			testCase.configure(&dependencies, surfaces)
			_, err := New(dependencies).Inventory(context.Background(), testRepository(), mustKey("ABC"))
			assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
			typed, ok := problem.As(err)
			if !ok || typed.Actual != testCase.capability {
				t.Fatalf("failure must name the missing capability %q: %#v", testCase.capability, typed)
			}
		})
	}
}

func TestValidateFreePassesUnheldNumbers(t *testing.T) {
	t.Parallel()

	surfaces := &fakeSurfaces{localBranchNames: []string{"feature/ABC-2-foundation"}}
	service := fullService(surfaces)
	if err := service.ValidateFree(context.Background(), testRepository(), mustID("ABC-3")); err != nil {
		t.Fatalf("ValidateFree() error = %v", err)
	}
}

// TestValidateFreeBlocksNumbersHeldOnlyByRequestRecords is the RG-37
// regression: the durable protected-line request record consumes a ticket
// number without any pull request, so no branch, envelope, or pull-request
// scan can see it; the gate still blocks the number with holder evidence.
func TestValidateFreeBlocksNumbersHeldOnlyByRequestRecords(t *testing.T) {
	t.Parallel()

	surfaces := &fakeSurfaces{requestRecords: []port.ProtectedLineRequestRecord{requestRecord("ABC-37")}}
	service := fullService(surfaces)
	err := service.ValidateFree(context.Background(), testRepository(), mustID("ABC-37"))
	assertProblemCode(t, err, problem.CodeTicketNumberAlreadyAllocated)
	typed, ok := problem.As(err)
	if !ok {
		t.Fatal("collision must be a typed problem")
	}
	if !strings.Contains(typed.Context, SurfaceProtectedLineRequests) ||
		!strings.Contains(typed.Context, "req-ABC-37") ||
		!strings.Contains(typed.Context, "d-demand-priv") {
		t.Fatalf("collision context must carry the holder evidence: %q", typed.Context)
	}
	if typed.Example != "ABC-38" {
		t.Fatalf("example must be the next free number above the held 37: %q", typed.Example)
	}
	if typed.Actual != "ABC-37" {
		t.Fatalf("actual = %q", typed.Actual)
	}
}

// TestValidateFreeBlocksNumbersHeldOnlyByTaskWorktrees is the pre-start
// regression: a freshly acquired detached task worktree consumes its ticket
// number through the path convention before any branch, commit, or pull
// request exists, so a scan without the worktree surface would report the
// number as free and a duplicate allocation could bind it.
func TestValidateFreeBlocksNumbersHeldOnlyByTaskWorktrees(t *testing.T) {
	t.Parallel()

	surfaces := &fakeSurfaces{
		worktrees: []port.WorktreeEntry{
			{Path: "C:/work/git-governance-ABC-37", Detached: true},
			{Path: "C:/work/git-governance-XYZ-9", Detached: true},
			{Path: "C:/work/git-governance-main-current", Detached: true},
			{Path: "C:/work/git-governance", Head: "abc"},
		},
	}
	service := fullService(surfaces)
	err := service.ValidateFree(context.Background(), testRepository(), mustID("ABC-37"))
	assertProblemCode(t, err, problem.CodeTicketNumberAlreadyAllocated)
	typed, ok := problem.As(err)
	if !ok {
		t.Fatal("collision must be a typed problem")
	}
	if !strings.Contains(typed.Context, SurfaceWorktreeRegistry) ||
		!strings.Contains(typed.Context, "git-governance-ABC-37") {
		t.Fatalf("collision context must carry the worktree holder evidence: %q", typed.Context)
	}
	if typed.Example != "ABC-38" {
		t.Fatalf("example must be the next free number above the held 37: %q", typed.Example)
	}
}

func TestTicketFromWorktreePath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		path     string
		expected ticket.ID
		found    bool
	}{
		{"a sibling task worktree binds its ticket", "C:/work/git-governance-GOV-132", mustID("GOV-132"), true},
		{"a repo basename without a ticket suffix is not a holder", "C:/work/git-governance", ticket.ID{}, false},
		{"a basename without any separator is not a holder", "C:/work/repository", ticket.ID{}, false},
		{"a foreign suffix is not a holder", "C:/work/git-governance-main-current", ticket.ID{}, false},
		{"a lowercase derived key is not a holder", "C:/work/git-governance-132", ticket.ID{}, false},
		{"a basename with a single separator is not a holder", "C:/work/repo-132", ticket.ID{}, false},
		{"a lowercase key is not a holder", "C:/work/git-governance-gov-132", ticket.ID{}, false},
		{"a leading-zero number is not a holder", "C:/work/git-governance-ABC-007", ticket.ID{}, false},
		{"an eighteen-digit number binds", "C:/work/repo-ABC-123456789012345678", mustID("ABC-123456789012345678"), true},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			id, found := ticketFromWorktreePath(testCase.path)
			if found != testCase.found {
				t.Fatalf("ticketFromWorktreePath(%q) found = %t", testCase.path, found)
			}
			if found && id.String() != testCase.expected.String() {
				t.Fatalf("ticketFromWorktreePath(%q) = %q; want %q", testCase.path, id.String(), testCase.expected.String())
			}
		})
	}
}

func TestValidateFreeBlocksBranchAndEnvelopeHolders(t *testing.T) {
	t.Parallel()

	surfaces := &fakeSurfaces{
		remoteBranchNames: []string{"feature/ABC-7-existing"},
		subjects:          []string{"feat(ABC-8): earlier work"},
	}
	service := fullService(surfaces)
	for _, rawTicket := range []string{"ABC-7", "ABC-8"} {
		err := service.ValidateFree(context.Background(), testRepository(), mustID(rawTicket))
		assertProblemCode(t, err, problem.CodeTicketNumberAlreadyAllocated)
	}
}

func TestNextFreeDerivesOneAboveHighestHolder(t *testing.T) {
	t.Parallel()

	empty := fullService(&fakeSurfaces{})
	number, err := empty.NextFree(context.Background(), testRepository(), mustKey("ABC"))
	if err != nil || number.String() != "1" {
		t.Fatalf("NextFree() = (%q, %v); want 1", number.String(), err)
	}

	surfaces := &fakeSurfaces{localBranchNames: []string{
		"feature/ABC-9-foundation",
		"feature/ABC-10-export",
		"feature/ABC-2-initial",
	}}
	number, err = fullService(surfaces).NextFree(context.Background(), testRepository(), mustKey("ABC"))
	if err != nil || number.String() != "11" {
		t.Fatalf("NextFree() = (%q, %v); want 11 ordered numerically", number.String(), err)
	}
}

func TestGateUnavailableFailsClosed(t *testing.T) {
	t.Parallel()

	err := GateUnavailable()
	assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
}

func TestInventoryStopsOnCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := fullService(&fakeSurfaces{})
	_, err := service.Inventory(ctx, testRepository(), mustKey("ABC"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Inventory() error = %v", err)
	}
}

func TestInventoryStopsOnNilService(t *testing.T) {
	t.Parallel()

	var service *Service
	_, err := service.Inventory(context.Background(), testRepository(), mustKey("ABC"))
	assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
}

func TestInventoryFailsClosedOnRemoteURLFailure(t *testing.T) {
	t.Parallel()

	failure := errors.New("remote URL unavailable")
	surfaces := &fakeSurfaces{remoteURLErr: failure}
	_, err := fullService(surfaces).Inventory(context.Background(), testRepository(), mustKey("ABC"))
	if !errors.Is(err, failure) {
		t.Fatalf("Inventory() error = %v; want the propagated remote URL failure", err)
	}
}

func TestValidateFreePropagatesInventoryFailure(t *testing.T) {
	t.Parallel()

	failure := errors.New("surface unavailable")
	err := fullService(&fakeSurfaces{localErr: failure}).ValidateFree(
		context.Background(), testRepository(), mustID("ABC-3"))
	if !errors.Is(err, failure) {
		t.Fatalf("ValidateFree() error = %v; want the propagated inventory failure", err)
	}
}

func TestNextFreePropagatesInventoryFailure(t *testing.T) {
	t.Parallel()

	failure := errors.New("surface unavailable")
	_, err := fullService(&fakeSurfaces{localErr: failure}).NextFree(
		context.Background(), testRepository(), mustKey("ABC"))
	if !errors.Is(err, failure) {
		t.Fatalf("NextFree() error = %v; want the propagated inventory failure", err)
	}
}

func TestNextFreeNumberSkipsInvalidHolderKeys(t *testing.T) {
	t.Parallel()

	holders := map[string][]Holder{
		"7":            {{Surface: SurfaceBranchRefs}},
		"not-a-number": {{Surface: SurfaceBranchRefs}},
	}
	if got := nextFreeNumber(holders); got != "8" {
		t.Fatalf("nextFreeNumber() = %q, want 8 above the only valid holder", got)
	}
}

func TestFormatTimestamp(t *testing.T) {
	t.Parallel()

	if got := formatTimestamp(time.Time{}); got != "" {
		t.Fatalf("formatTimestamp(zero) = %q, want empty", got)
	}
	stamp := time.Date(2026, 9, 30, 16, 53, 44, 0, time.UTC)
	if got := formatTimestamp(stamp); got != "2026-09-30T16:53:44Z" {
		t.Fatalf("formatTimestamp() = %q, want the RFC3339 UTC form", got)
	}
}

// TestInventoryReportsMixedPlatformCapabilities proves the two platform
// surfaces are reported independently: a pull-request-only provider names the
// request-record surface absent, a request-record-only provider names the
// pull-request surface absent, and a foreign-key request record never holds a
// number of the inventoried key.
func TestInventoryReportsMixedPlatformCapabilities(t *testing.T) {
	t.Parallel()

	pullRequestOnly := &fakeSurfaces{pullRequests: []port.PullRequestSummary{
		{Number: "55", Title: "ABC-37: allow-git-lfs", Author: "CyberT33N", CreatedAt: time.Date(2026, 9, 30, 20, 2, 32, 0, time.UTC)},
	}}
	requestOnly := &fakeSurfaces{requestRecords: []port.ProtectedLineRequestRecord{requestRecord("XYZ-5")}}

	pullService := New(Dependencies{
		LocalBranches:  pullRequestOnly,
		RemoteBranches: pullRequestOnly,
		CommitSubjects: pullRequestOnly,
		HotfixRecords:  pullRequestOnly,
		Worktrees:      pullRequestOnly,
		RemoteURL:      pullRequestOnly.RemoteURL,
		PullRequests:   pullRequestOnly,
		AppPermissions: pullRequestOnly,
	})
	allocation, err := pullService.Inventory(context.Background(), testRepository(), mustKey("ABC"))
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]string{}
	for _, status := range allocation.Surfaces {
		states[status.Surface] = status.State
	}
	if states[SurfacePullRequestTitles] != SurfaceScanned || states[SurfaceProtectedLineRequests] != SurfaceAbsent {
		t.Fatalf("pull-request-only surfaces = %#v", allocation.Surfaces)
	}
	if len(allocation.Holders["37"]) != 1 {
		t.Fatalf("pull-request holder missing: %#v", allocation.Holders)
	}

	requestService := New(Dependencies{
		LocalBranches:         requestOnly,
		RemoteBranches:        requestOnly,
		CommitSubjects:        requestOnly,
		HotfixRecords:         requestOnly,
		Worktrees:             requestOnly,
		RemoteURL:             requestOnly.RemoteURL,
		ProtectedLineRequests: requestOnly,
		AppPermissions:        requestOnly,
	})
	allocation, err = requestService.Inventory(context.Background(), testRepository(), mustKey("ABC"))
	if err != nil {
		t.Fatal(err)
	}
	states = map[string]string{}
	for _, status := range allocation.Surfaces {
		states[status.Surface] = status.State
	}
	if states[SurfacePullRequestTitles] != SurfaceAbsent || states[SurfaceProtectedLineRequests] != SurfaceScanned {
		t.Fatalf("request-record-only surfaces = %#v", allocation.Surfaces)
	}
	if len(allocation.Holders) != 0 {
		t.Fatalf("a foreign-key request record must not hold an ABC number: %#v", allocation.Holders)
	}
	if allocation.NextFree != "1" {
		t.Fatalf("next free = %q, want 1 for an empty ABC surface set", allocation.NextFree)
	}
}

// TestInventoryClassifiesPlatformSurfacesByAppPermissionClass proves the
// capability matrix: a carried read or write permission scans the surface, a
// missing or unknown permission names the surface absent with the capability
// reason, and an absent surface never contributes holders.
func TestInventoryClassifiesPlatformSurfacesByAppPermissionClass(t *testing.T) {
	t.Parallel()

	pullRequests := []port.PullRequestSummary{
		{Number: "55", Title: "ABC-37: allow-git-lfs", Author: "CyberT33N", CreatedAt: time.Date(2026, 9, 30, 20, 2, 32, 0, time.UTC)},
	}
	requestRecords := []port.ProtectedLineRequestRecord{requestRecord("ABC-35")}

	testCases := []struct {
		name                    string
		permissions             map[string]string
		wantPullRequestState    string
		wantRequestRecordState  string
		wantPullRequestReason   bool
		wantRequestRecordReason bool
	}{
		{
			name:                   "carried write permissions scan both surfaces",
			permissions:            map[string]string{"pull_requests": "write", "deployments": "write"},
			wantPullRequestState:   SurfaceScanned,
			wantRequestRecordState: SurfaceScanned,
		},
		{
			name:                   "carried read permissions scan both surfaces",
			permissions:            map[string]string{"pull_requests": "read", "deployments": "read"},
			wantPullRequestState:   SurfaceScanned,
			wantRequestRecordState: SurfaceScanned,
		},
		{
			name:                    "a missing deployments permission names the record surface absent",
			permissions:             map[string]string{"pull_requests": "write", "metadata": "read"},
			wantPullRequestState:    SurfaceScanned,
			wantRequestRecordState:  SurfaceAbsent,
			wantRequestRecordReason: true,
		},
		{
			name:                   "a missing pull requests permission names the title surface absent",
			permissions:            map[string]string{"deployments": "write"},
			wantPullRequestState:   SurfaceAbsent,
			wantPullRequestReason:  true,
			wantRequestRecordState: SurfaceScanned,
		},
		{
			name:                    "an app class without either permission names both surfaces absent",
			permissions:             map[string]string{"contents": "write", "metadata": "read"},
			wantPullRequestState:    SurfaceAbsent,
			wantPullRequestReason:   true,
			wantRequestRecordState:  SurfaceAbsent,
			wantRequestRecordReason: true,
		},
		{
			name:                    "unknown permission levels do not carry the surfaces",
			permissions:             map[string]string{"pull_requests": "admin", "deployments": "none"},
			wantPullRequestState:    SurfaceAbsent,
			wantPullRequestReason:   true,
			wantRequestRecordState:  SurfaceAbsent,
			wantRequestRecordReason: true,
		},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			surfaces := &fakeSurfaces{
				permissions:    testCase.permissions,
				pullRequests:   pullRequests,
				requestRecords: requestRecords,
			}
			allocation, err := fullService(surfaces).Inventory(context.Background(), testRepository(), mustKey("ABC"))
			if err != nil {
				t.Fatal(err)
			}
			states := map[string]SurfaceStatus{}
			for _, status := range allocation.Surfaces {
				states[status.Surface] = status
			}
			pullStatus := states[SurfacePullRequestTitles]
			recordStatus := states[SurfaceProtectedLineRequests]
			if pullStatus.State != testCase.wantPullRequestState || recordStatus.State != testCase.wantRequestRecordState {
				t.Fatalf("surface states = %#v", allocation.Surfaces)
			}
			if (pullStatus.Reason != "") != testCase.wantPullRequestReason ||
				(recordStatus.Reason != "") != testCase.wantRequestRecordReason {
				t.Fatalf("absent reasons = %q / %q", pullStatus.Reason, recordStatus.Reason)
			}
			if testCase.wantRequestRecordState == SurfaceAbsent && len(allocation.Holders["35"]) != 0 {
				t.Fatalf("an absent surface must not contribute holders: %#v", allocation.Holders)
			}
			if testCase.wantPullRequestState == SurfaceScanned && len(allocation.Holders["37"]) != 1 {
				t.Fatalf("the scanned pull-request surface must carry its holder: %#v", allocation.Holders)
			}
		})
	}
}

// TestInventoryMeasuresAppPermissionsOncePerInvocation proves the cache-free
// measurement duty: one fresh inspection per inventory invocation even when
// both platform surfaces consume its classification.
func TestInventoryMeasuresEachSurfaceCapabilityFreshPerInvocation(t *testing.T) {
	t.Parallel()

	surfaces := &fakeSurfaces{
		permissions:    map[string]string{"pull_requests": "read", "deployments": "read"},
		pullRequests:   []port.PullRequestSummary{{Number: "1", Title: "ABC-1: first", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}},
		requestRecords: []port.ProtectedLineRequestRecord{requestRecord("ABC-35")},
	}
	if _, err := fullService(surfaces).Inventory(context.Background(), testRepository(), mustKey("ABC")); err != nil {
		t.Fatal(err)
	}
	if surfaces.permissionInspections != 2 ||
		surfaces.inspectedCapabilities[0] != port.CapabilityPullRequests ||
		surfaces.inspectedCapabilities[1] != port.CapabilityDeployments {
		t.Fatalf("permission inspections = %d (%v); want one fresh measurement per wired platform surface capability",
			surfaces.permissionInspections, surfaces.inspectedCapabilities)
	}
	// A second invocation re-measures both capability cards: the
	// classification is a measurement, never a stored claim state.
	if _, err := fullService(surfaces).Inventory(context.Background(), testRepository(), mustKey("ABC")); err != nil {
		t.Fatal(err)
	}
	if surfaces.permissionInspections != 4 {
		t.Fatalf("permission inspections after the second invocation = %d; want a fresh re-measurement", surfaces.permissionInspections)
	}
}

// TestInventoryClassifiesUnboundSessionClassAsNamedAbsent proves the
// capability-missing verdict of the serving session class is the named
// degradation basis: the surface is absent with the session-class reason, the
// carried capability still scans, and a genuine read failure keeps failing
// closed.
func TestInventoryClassifiesUnboundSessionClassAsNamedAbsent(t *testing.T) {
	t.Parallel()

	surfaces := &fakeSurfaces{
		permissions:       map[string]string{"pull_requests": "write"},
		permissionMissing: port.CapabilityDeployments,
		pullRequests:      []port.PullRequestSummary{{Number: "1", Title: "ABC-1: first", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}},
		requestRecords:    []port.ProtectedLineRequestRecord{requestRecord("ABC-35")},
	}
	allocation, err := fullService(surfaces).Inventory(context.Background(), testRepository(), mustKey("ABC"))
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]SurfaceStatus{}
	for _, status := range allocation.Surfaces {
		states[status.Surface] = status
	}
	if states[SurfacePullRequestTitles].State != SurfaceScanned {
		t.Fatalf("pull-request surface = %#v", states[SurfacePullRequestTitles])
	}
	if states[SurfaceProtectedLineRequests].State != SurfaceAbsent ||
		states[SurfaceProtectedLineRequests].Reason != "no configured provider session carries the deployments read permission" {
		t.Fatalf("protected-line surface = %#v", states[SurfaceProtectedLineRequests])
	}
	// The degraded scan never reports the platform-bound record number as a
	// scanned holder; the named absence stays the visible evidence.
	if holders := allocation.Holders["35"]; len(holders) != 0 {
		t.Fatalf("degraded surface holders = %#v", holders)
	}

	failing := &fakeSurfaces{
		permissions:       map[string]string{"pull_requests": "write"},
		permissionMissing: port.CapabilityDeployments,
		permissionsErr:    errors.New("registration unreachable"),
	}
	if _, err := fullService(failing).Inventory(context.Background(), testRepository(), mustKey("ABC")); err == nil {
		t.Fatal("a genuine discovery failure was classified as a named absence")
	}

	// A genuine failure of the deployments-serving measurement fails the
	// inventory closed even when the carried capability scans.
	failingDeployments := &fakeSurfaces{
		permissions:    map[string]string{"pull_requests": "write"},
		deploymentsErr: errors.New("registration unreachable"),
		pullRequests:   []port.PullRequestSummary{{Number: "1", Title: "ABC-1: first", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}},
	}
	if _, err := fullService(failingDeployments).Inventory(context.Background(), testRepository(), mustKey("ABC")); err == nil {
		t.Fatal("a genuine deployments measurement failure was swallowed")
	}
}

// TestInventoryProviderNoneSkipsPermissionDiscovery proves the provider-less
// path never measures the app registration: the named provider-less absence
// needs no capability evidence.
func TestInventoryProviderNoneSkipsPermissionDiscovery(t *testing.T) {
	t.Parallel()

	surfaces := &fakeSurfaces{localBranchNames: []string{"feature/ABC-2-foundation"}}
	service := New(Dependencies{
		LocalBranches:  surfaces,
		RemoteBranches: surfaces,
		CommitSubjects: surfaces,
		HotfixRecords:  surfaces,
		Worktrees:      surfaces,
		AppPermissions: surfaces,
	})
	if _, err := service.Inventory(context.Background(), testRepository(), mustKey("ABC")); err != nil {
		t.Fatal(err)
	}
	if surfaces.permissionInspections != 0 {
		t.Fatalf("permission inspections = %d; the provider-less inventory must not measure the app registration", surfaces.permissionInspections)
	}
}

func TestInventoryFailsClosedOnPermissionDiscoveryFailure(t *testing.T) {
	t.Parallel()

	failure := errors.New("app registration unavailable")
	surfaces := &fakeSurfaces{permissionsErr: failure}
	_, err := fullService(surfaces).Inventory(context.Background(), testRepository(), mustKey("ABC"))
	if !errors.Is(err, failure) {
		t.Fatalf("Inventory() error = %v; want the propagated permission discovery failure", err)
	}
}

func TestInventoryFailsClosedOnMissingPermissionDiscoveryCapability(t *testing.T) {
	t.Parallel()

	surfaces := &fakeSurfaces{}
	dependencies := Dependencies{
		LocalBranches:         surfaces,
		RemoteBranches:        surfaces,
		CommitSubjects:        surfaces,
		HotfixRecords:         surfaces,
		Worktrees:             surfaces,
		RemoteURL:             surfaces.RemoteURL,
		PullRequests:          surfaces,
		ProtectedLineRequests: surfaces,
	}
	_, err := New(dependencies).Inventory(context.Background(), testRepository(), mustKey("ABC"))
	assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
	typed, ok := problem.As(err)
	if !ok || typed.Actual != "provider app permission discovery" {
		t.Fatalf("failure must name the missing discovery capability: %#v", typed)
	}
}

func fullService(surfaces *fakeSurfaces) *Service {
	return New(Dependencies{
		LocalBranches:         surfaces,
		RemoteBranches:        surfaces,
		CommitSubjects:        surfaces,
		HotfixRecords:         surfaces,
		Worktrees:             surfaces,
		RemoteURL:             surfaces.RemoteURL,
		PullRequests:          surfaces,
		ProtectedLineRequests: surfaces,
		AppPermissions:        surfaces,
	})
}

func testRepository() port.RepositoryIdentity {
	return port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"}
}

func mustKey(raw string) ticket.Key {
	key, err := ticket.ParseKey(raw)
	if err != nil {
		panic(err)
	}
	return key
}

func mustID(raw string) ticket.ID {
	id, err := ticket.ParseID(raw)
	if err != nil {
		panic(err)
	}
	return id
}

func mustName(raw string) branch.BranchName {
	name, err := branch.ParseName(raw)
	if err != nil {
		panic(err)
	}
	return name
}

func mustVersion(raw string) branch.SemanticVersion {
	version, err := branch.ParseSemanticVersion(raw)
	if err != nil {
		panic(err)
	}
	return version
}

func assertProblemCode(t *testing.T, err error, code problem.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected problem code %q, got nil", code)
	}
	typed, ok := problem.As(err)
	if !ok {
		t.Fatalf("error %v is not a typed problem", err)
	}
	if typed.Code != code {
		t.Fatalf("problem code = %q; want %q (details: %#v)", typed.Code, code, typed.Details)
	}
}
