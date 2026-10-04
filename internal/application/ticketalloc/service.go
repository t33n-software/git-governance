// Package ticketalloc derives the governed ticket-number allocation
// inventory from every allocation surface of a repository. Allocation truth is
// a measurement of the current surfaces, never a stored claim state; the
// canonical convention docs/conventions/tickets/ticket-allocation.md owns the
// invariant.
package ticketalloc

import (
	"context"
	"errors"
	"math/big"
	"path/filepath"
	"strings"
	"time"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

// Allocation surface names of the closed inventory. The names follow the
// canonical convention TAL-R002.
const (
	SurfaceBranchRefs            = "branch refs"
	SurfaceCommitEnvelopes       = "commit envelopes"
	SurfaceHotfixRecords         = "hotfix release records"
	SurfaceWorktreeRegistry      = "worktree registry"
	SurfacePullRequestTitles     = "pull request titles"
	SurfaceProtectedLineRequests = "protected-line request records"
)

// Surface scan states of the inventory report.
const (
	SurfaceScanned = "scanned"
	SurfaceAbsent  = "absent"
)

// Provider app permission names of the platform surface read classes. The
// permission discovery maps the public app registration onto these names.
const (
	appPermissionPullRequests = "pull_requests"
	appPermissionDeployments  = "deployments"
)

// Dependencies binds the read-only adapter capabilities the inventory derives
// its measurement from. The Git-transport surfaces are required; the platform
// surfaces are present exactly when a hosting provider is configured. A
// provider-less repository names their absence and never silently treats the
// platform surfaces as empty.
type Dependencies struct {
	LocalBranches  port.LocalBranchLister
	RemoteBranches port.RemoteBranchLister
	CommitSubjects port.CommitSubjectLister
	HotfixRecords  port.HotfixReleaseRecordLister
	// Worktrees lists the local task worktrees whose registered
	// acquisition consumes a ticket number before any branch, commit, or
	// pull request exists. It is a Git-transport surface: a composition
	// without it fails the inventory closed instead of silently treating
	// the surface as empty.
	Worktrees    port.WorktreeInventoryLister
	RemoteURL    func(context.Context, port.RepositoryIdentity) (string, error)
	PullRequests port.PullRequestInventoryLister
	// ProtectedLineRequests lists the durable request records that consume
	// ticket numbers without any pull request.
	ProtectedLineRequests port.ProtectedLineRequestInventoryLister
	// AppPermissions measures the permission class of the configured
	// provider app identity fresh per inventory invocation. It is required
	// whenever a platform surface port is wired, because the classification
	// of a configured platform surface is decided from the app class and
	// never from a read failure.
	AppPermissions port.AppPermissionInspector
}

// Service owns the derived allocation inventory and the fail-closed ticket
// intake gates. It is a read model: every call measures the current surfaces
// and no allocation state is ever stored.
type Service struct {
	dependencies Dependencies
}

// New creates the allocation inventory service from resolved adapter
// capabilities. A missing required capability is a composition defect that
// fails closed at scan time with the named capability.
func New(dependencies Dependencies) *Service {
	return &Service{dependencies: dependencies}
}

// Holder records one allocation evidence on one governed surface.
type Holder struct {
	Surface   string `json:"surface"`
	Locator   string `json:"locator"`
	Actor     string `json:"actor,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

// SurfaceStatus names one inventory surface, whether it was scanned or is
// absent by configuration, and the named reason of an absence. A
// capability-scoped absence carries the permission-class reason of the
// provider app; a provider-less absence stays unnamed.
type SurfaceStatus struct {
	Surface string `json:"surface"`
	State   string `json:"state"`
	Reason  string `json:"reason,omitempty"`
}

// Allocation is the derived allocation inventory of one ticket-key
// namespace.
type Allocation struct {
	Key      string              `json:"key"`
	Holders  map[string][]Holder `json:"holders"`
	NextFree string              `json:"nextFree"`
	Surfaces []SurfaceStatus     `json:"surfaces"`
}

// Inventory derives the complete allocation inventory for one ticket key
// from every governed surface of the repository. Surfaces that cannot be
// read completely fail the inventory closed.
func (service *Service) Inventory(ctx context.Context, repository port.RepositoryIdentity, key ticket.Key) (Allocation, error) {
	if service == nil {
		return Allocation{}, unavailableCapabilityProblem("allocation inventory")
	}
	if err := ctx.Err(); err != nil {
		return Allocation{}, err
	}

	allocation := Allocation{
		Key:     key.String(),
		Holders: map[string][]Holder{},
	}

	local, err := service.localBranches(ctx, repository)
	if err != nil {
		return Allocation{}, err
	}
	for _, name := range local {
		recordBranchHolder(allocation.Holders, key, name, "local branch ref")
	}

	remote, err := service.remoteBranches(ctx, repository)
	if err != nil {
		return Allocation{}, err
	}
	for _, name := range remote {
		recordBranchHolder(allocation.Holders, key, name, "remote branch ref")
	}
	allocation.Surfaces = append(allocation.Surfaces, SurfaceStatus{
		Surface: SurfaceBranchRefs,
		State:   SurfaceScanned,
	})

	subjects, err := service.commitSubjects(ctx, repository)
	if err != nil {
		return Allocation{}, err
	}
	for _, subject := range subjects {
		header, err := commitmsg.ParseHeader(subject)
		if err != nil {
			continue
		}
		if header.Ticket().Key().String() == key.String() {
			addHolder(allocation.Holders, header.Ticket().Number().String(), Holder{
				Surface: SurfaceCommitEnvelopes,
				Locator: header.String(),
			})
		}
	}
	allocation.Surfaces = append(allocation.Surfaces, SurfaceStatus{
		Surface: SurfaceCommitEnvelopes,
		State:   SurfaceScanned,
	})

	records, err := service.hotfixRecords(ctx, repository)
	if err != nil {
		return Allocation{}, err
	}
	for _, record := range records {
		if record.Record.Ticket().Key().String() == key.String() {
			addHolder(allocation.Holders, record.Record.Ticket().Number().String(), Holder{
				Surface: SurfaceHotfixRecords,
				Locator: record.Location,
			})
		}
	}
	allocation.Surfaces = append(allocation.Surfaces, SurfaceStatus{
		Surface: SurfaceHotfixRecords,
		State:   SurfaceScanned,
	})

	entries, err := service.worktrees(ctx, repository)
	if err != nil {
		return Allocation{}, err
	}
	for _, entry := range entries {
		id, found := ticketFromWorktreePath(entry.Path)
		if !found || id.Key().String() != key.String() {
			continue
		}
		addHolder(allocation.Holders, id.Number().String(), Holder{
			Surface: SurfaceWorktreeRegistry,
			Locator: "task worktree " + filepath.Base(entry.Path),
		})
	}
	allocation.Surfaces = append(allocation.Surfaces, SurfaceStatus{
		Surface: SurfaceWorktreeRegistry,
		State:   SurfaceScanned,
	})

	if service.dependencies.PullRequests == nil && service.dependencies.ProtectedLineRequests == nil {
		allocation.Surfaces = append(allocation.Surfaces,
			SurfaceStatus{Surface: SurfacePullRequestTitles, State: SurfaceAbsent},
			SurfaceStatus{Surface: SurfaceProtectedLineRequests, State: SurfaceAbsent},
		)
		allocation.NextFree = nextFreeNumber(allocation.Holders)
		return allocation, nil
	}

	remoteURL, err := service.remoteURL(ctx, repository)
	if err != nil {
		return Allocation{}, err
	}

	if service.dependencies.PullRequests != nil {
		permissions, err := service.appPermissions(ctx, repository, remoteURL, port.CapabilityPullRequests)
		if err != nil && !errors.Is(err, port.ErrCapabilitySessionMissing) {
			return Allocation{}, err
		}
		if !appPermissionCarries(permissions, appPermissionPullRequests) {
			allocation.Surfaces = append(allocation.Surfaces, SurfaceStatus{
				Surface: SurfacePullRequestTitles,
				State:   SurfaceAbsent,
				Reason:  absentReason(err, appPermissionPullRequests),
			})
		} else {
			pullRequests, err := service.dependencies.PullRequests.ListPullRequests(ctx, port.PullRequestInventoryQuery{
				Repository: repository,
				RemoteURL:  remoteURL,
			})
			if err != nil {
				return Allocation{}, err
			}
			for _, summary := range pullRequests {
				id, found := ticketFromTitle(summary.Title)
				if !found || id.Key().String() != key.String() {
					continue
				}
				addHolder(allocation.Holders, id.Number().String(), Holder{
					Surface:   SurfacePullRequestTitles,
					Locator:   "pull request #" + summary.Number,
					Actor:     summary.Author,
					Timestamp: formatTimestamp(summary.CreatedAt),
				})
			}
			allocation.Surfaces = append(allocation.Surfaces, SurfaceStatus{
				Surface: SurfacePullRequestTitles,
				State:   SurfaceScanned,
			})
		}
	} else {
		allocation.Surfaces = append(allocation.Surfaces, SurfaceStatus{
			Surface: SurfacePullRequestTitles,
			State:   SurfaceAbsent,
		})
	}

	if service.dependencies.ProtectedLineRequests != nil {
		permissions, err := service.appPermissions(ctx, repository, remoteURL, port.CapabilityDeployments)
		if err != nil && !errors.Is(err, port.ErrCapabilitySessionMissing) {
			return Allocation{}, err
		}
		if !appPermissionCarries(permissions, appPermissionDeployments) {
			allocation.Surfaces = append(allocation.Surfaces, SurfaceStatus{
				Surface: SurfaceProtectedLineRequests,
				State:   SurfaceAbsent,
				Reason:  absentReason(err, appPermissionDeployments),
			})
		} else {
			requestRecords, err := service.dependencies.ProtectedLineRequests.ListProtectedLineRequests(ctx, port.ProtectedLineRequestInventoryQuery{
				Repository: repository,
				RemoteURL:  remoteURL,
			})
			if err != nil {
				return Allocation{}, err
			}
			for _, record := range requestRecords {
				id := record.Request.Ticket()
				if id.Key().String() != key.String() {
					continue
				}
				addHolder(allocation.Holders, id.Number().String(), Holder{
					Surface:   SurfaceProtectedLineRequests,
					Locator:   "request " + record.Request.ID(),
					Actor:     record.Request.Requester(),
					Timestamp: formatTimestamp(record.CreatedAt),
				})
			}
			allocation.Surfaces = append(allocation.Surfaces, SurfaceStatus{
				Surface: SurfaceProtectedLineRequests,
				State:   SurfaceScanned,
			})
		}
	} else {
		allocation.Surfaces = append(allocation.Surfaces, SurfaceStatus{
			Surface: SurfaceProtectedLineRequests,
			State:   SurfaceAbsent,
		})
	}

	allocation.NextFree = nextFreeNumber(allocation.Holders)
	return allocation, nil
}

// ValidateFree fails closed with TICKET_NUMBER_ALREADY_ALLOCATED when the
// requested ticket number is held on any governed surface. The collision
// record carries the holder evidence and the next free number of the same key.
func (service *Service) ValidateFree(ctx context.Context, repository port.RepositoryIdentity, id ticket.ID) error {
	allocation, err := service.Inventory(ctx, repository, id.Key())
	if err != nil {
		return err
	}
	holders := allocation.Holders[id.Number().String()]
	if len(holders) == 0 {
		return nil
	}
	return problem.New(problem.Details{
		Code:     problem.CodeTicketNumberAlreadyAllocated,
		Category: problem.CategoryGovernance,
		Field:    "ticket number",
		Actual:   id.String(),
		Context:  holdersContext(holders),
		Expected: "a ticket number not allocated on any governed surface",
		Rule:     "every ticket number within a ticket-key namespace is allocated exactly once across all governed allocation surfaces",
		Example:  id.Key().String() + "-" + allocation.NextFree,
		Remediation: "continue the existing allocation, or claim the next free number " +
			id.Key().String() + "-" + allocation.NextFree,
	})
}

// NextFree derives the next free number of one ticket key: one above the
// highest allocated number across all governed surfaces.
func (service *Service) NextFree(ctx context.Context, repository port.RepositoryIdentity, key ticket.Key) (ticket.Number, error) {
	allocation, err := service.Inventory(ctx, repository, key)
	if err != nil {
		return ticket.Number{}, err
	}
	return ticket.ParseNumber(allocation.NextFree)
}

// GateUnavailable is the fail-closed problem of a ticket-consuming endpoint
// whose allocation gate has not been wired.
func GateUnavailable() error {
	return problem.New(problem.Details{
		Code:     problem.CodeConfigurationUnavailable,
		Category: problem.CategoryConfig,
		Field:    "ticket allocation gate",
		Expected: "the allocation inventory service wired into every ticket-consuming endpoint",
		Rule:     "every ticket-consuming endpoint validates the requested number against a fresh full-surface inventory before binding",
		Remediation: "fix the composition so every ticket-consuming endpoint receives the " +
			"allocation inventory service",
	})
}

// ticketFromTitle parses the canonical ticket grammars of a pull-request
// title: the governed plain form KEY-NUMBER: slug and the historical envelope
// form type(KEY-NUMBER): subject. Parsing uses the canonical domain grammars;
// it never string-matches.
func ticketFromTitle(title string) (ticket.ID, bool) {
	if header, err := commitmsg.ParseHeader(title); err == nil {
		return header.Ticket(), true
	}
	prefix, _, found := strings.Cut(title, ":")
	if !found {
		return ticket.ID{}, false
	}
	id, err := ticket.ParseID(strings.TrimSpace(prefix))
	if err != nil {
		return ticket.ID{}, false
	}
	return id, true
}

// ticketFromWorktreePath derives the ticket binding of one task-worktree
// path from its basename. The acquisition convention names sibling
// directories `<repo>-<KEY>-<NUMBER>`, so a registered task worktree
// consumes its number before any branch, commit, or pull request exists;
// a basename without that grammar is not a task-worktree holder.
func ticketFromWorktreePath(path string) (ticket.ID, bool) {
	name := filepath.Base(path)
	numberSeparator := strings.LastIndex(name, "-")
	if numberSeparator < 0 {
		return ticket.ID{}, false
	}
	prefix := name[:numberSeparator]
	keySeparator := strings.LastIndex(prefix, "-")
	if keySeparator < 0 {
		return ticket.ID{}, false
	}
	key, err := ticket.ParseKey(prefix[keySeparator+1:])
	if err != nil {
		return ticket.ID{}, false
	}
	number, err := ticket.ParseNumber(name[numberSeparator+1:])
	if err != nil {
		return ticket.ID{}, false
	}
	return ticket.NewID(key, number), true
}

func recordBranchHolder(holders map[string][]Holder, key ticket.Key, name branch.BranchName, surface string) {
	scopedTicket, found := name.Ticket()
	if !found || scopedTicket.Key().String() != key.String() {
		return
	}
	addHolder(holders, scopedTicket.Number().String(), Holder{
		Surface: SurfaceBranchRefs,
		Locator: surface + " " + name.String(),
	})
}

func addHolder(holders map[string][]Holder, number string, holder Holder) {
	holders[number] = append(holders[number], holder)
}

func holdersContext(holders []Holder) string {
	parts := make([]string, 0, len(holders))
	for _, holder := range holders {
		entry := holder.Surface + " " + holder.Locator
		if holder.Actor != "" {
			entry += " (" + holder.Actor + ")"
		}
		if holder.Timestamp != "" {
			entry += " at " + holder.Timestamp
		}
		parts = append(parts, entry)
	}
	return "held by " + strings.Join(parts, "; ")
}

// nextFreeNumber derives one above the highest allocated number using
// arbitrary-precision arithmetic so the eighteen-digit ticket grammar cannot
// overflow.
func nextFreeNumber(holders map[string][]Holder) string {
	highest := new(big.Int)
	allocated := false
	for number := range holders {
		value := new(big.Int)
		if _, valid := value.SetString(number, 10); !valid {
			continue
		}
		if !allocated || value.Cmp(highest) > 0 {
			highest = new(big.Int).Set(value)
			allocated = true
		}
	}
	if !allocated {
		return "1"
	}
	return highest.Add(highest, big.NewInt(1)).String()
}

func formatTimestamp(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.UTC().Format(time.RFC3339)
}

func (service *Service) localBranches(ctx context.Context, repository port.RepositoryIdentity) ([]branch.BranchName, error) {
	if service.dependencies.LocalBranches == nil {
		return nil, unavailableCapabilityProblem("local branch listing")
	}
	return service.dependencies.LocalBranches.LocalBranches(ctx, repository)
}

func (service *Service) remoteBranches(ctx context.Context, repository port.RepositoryIdentity) ([]branch.BranchName, error) {
	if service.dependencies.RemoteBranches == nil {
		return nil, unavailableCapabilityProblem("remote branch listing")
	}
	return service.dependencies.RemoteBranches.RemoteBranches(ctx, repository)
}

func (service *Service) commitSubjects(ctx context.Context, repository port.RepositoryIdentity) ([]string, error) {
	if service.dependencies.CommitSubjects == nil {
		return nil, unavailableCapabilityProblem("commit subject history")
	}
	return service.dependencies.CommitSubjects.CommitSubjects(ctx, repository)
}

func (service *Service) hotfixRecords(ctx context.Context, repository port.RepositoryIdentity) ([]port.HotfixReleaseRecord, error) {
	if service.dependencies.HotfixRecords == nil {
		return nil, unavailableCapabilityProblem("hotfix release record listing")
	}
	return service.dependencies.HotfixRecords.ListHotfixReleaseRecords(ctx, repository)
}

func (service *Service) worktrees(ctx context.Context, repository port.RepositoryIdentity) ([]port.WorktreeEntry, error) {
	if service.dependencies.Worktrees == nil {
		return nil, unavailableCapabilityProblem("worktree inventory listing")
	}
	return service.dependencies.Worktrees.WorktreeList(ctx, repository)
}

func (service *Service) remoteURL(ctx context.Context, repository port.RepositoryIdentity) (string, error) {
	if service.dependencies.RemoteURL == nil {
		return "", unavailableCapabilityProblem("remote URL resolution")
	}
	return service.dependencies.RemoteURL(ctx, repository)
}

// appPermissions measures the permission card of the session that would
// serve one capability class, fresh per inventory invocation. The measurement
// is never cached: every gate invocation re-reads the public app registration
// of the serving session, so the capability classification stays a
// measurement and never becomes a stored claim state. The named
// capability-missing verdict of the class propagates as the classification
// basis of a capability-scoped absent surface.
func (service *Service) appPermissions(
	ctx context.Context,
	repository port.RepositoryIdentity,
	remoteURL string,
	capability port.CredentialCapability,
) (port.AppPermissionSnapshot, error) {
	if service.dependencies.AppPermissions == nil {
		return port.AppPermissionSnapshot{}, unavailableCapabilityProblem("provider app permission discovery")
	}
	return service.dependencies.AppPermissions.InspectAppPermissions(ctx, port.AppPermissionQuery{
		Repository: repository,
		RemoteURL:  remoteURL,
		Capability: capability,
	})
}

// appPermissionCarries reports whether the provider app's permission map
// carries the read class of one platform surface through the shared port
// invariant. A missing or unknown key means the app class does not carry the
// capability.
func appPermissionCarries(permissions port.AppPermissionSnapshot, name string) bool {
	return port.AppPermissionCarries(permissions.Permissions, name)
}

// absentReason names the degradation form of a capability-scoped absent
// surface: a capability-missing verdict of the serving session class names
// the unbound session class, a measured card without the permission names the
// app-class fact.
func absentReason(err error, permission string) string {
	if errors.Is(err, port.ErrCapabilitySessionMissing) {
		return absentSessionCapabilityReason(permission)
	}
	return absentCapabilityReason(permission)
}

// absentCapabilityReason is the named degradation form of a capability-scoped
// absent surface whose serving session was measured: the configured provider
// identity's app class structurally does not carry the surface's read
// permission.
func absentCapabilityReason(permission string) string {
	return "provider app class does not carry the " + permission + " read permission"
}

// absentSessionCapabilityReason is the named degradation form of a
// capability-scoped absent surface whose session class is not bound: no
// configured provider session of the repository carries the surface's read
// permission.
func absentSessionCapabilityReason(permission string) string {
	return "no configured provider session carries the " + permission + " read permission"
}

func unavailableCapabilityProblem(capability string) error {
	return problem.New(problem.Details{
		Code:     problem.CodeConfigurationUnavailable,
		Category: problem.CategoryConfig,
		Field:    "allocation inventory",
		Actual:   capability,
		Expected: "the read capability of every governed allocation surface",
		Rule:     "the allocation inventory measures every governed surface and fails closed when one cannot be read",
		Remediation: "fix the composition so the allocation inventory receives the " + capability +
			" capability",
	})
}
