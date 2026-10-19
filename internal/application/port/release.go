package port

import (
	"context"
	"time"

	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/hotfix"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

// Report is the delivery-neutral result of a use case.
type Report struct {
	Operation string
	Summary   string
	Fields    map[string]string
	Data      any
	Problem   *problem.Problem
}

// Reporter renders application results as human or machine output.
type Reporter interface {
	Report(ctx context.Context, result Report) error
}

// HotfixReleaseRecordStore loads a reviewed hotfix release record from the
// repository without allowing callers to escape its controlled record area.
type HotfixReleaseRecordStore interface {
	LoadHotfixReleaseRecord(
		ctx context.Context,
		repository RepositoryIdentity,
		id ticket.ID,
		location string,
	) (hotfix.ReleaseRecord, error)
}

// HotfixReleaseRecord binds one reviewed release record to its
// repository-relative location for the allocation inventory.
type HotfixReleaseRecord struct {
	Record   hotfix.ReleaseRecord
	Location string
}

// HotfixReleaseRecordLister is an optional capability for adapters that can
// enumerate every reviewed release record below the controlled repository
// record directory. The allocation inventory derives hotfix record numbers
// from the schema-validated records themselves.
type HotfixReleaseRecordLister interface {
	ListHotfixReleaseRecords(ctx context.Context, repository RepositoryIdentity) ([]HotfixReleaseRecord, error)
}

// HotfixManifestProgress records one in-progress ordered propagation in
// repository-local Git metadata. It is not a reviewed release record and
// never grants publication authority.
type HotfixManifestProgress struct {
	Branch   branch.BranchName
	Source   branch.BranchName
	Target   branch.BranchName
	Manifest []string
	Next     int
}

// HotfixManifestProgressStore persists the local state needed to continue an
// explicitly user-resolved ordered cherry-pick without guessing its position.
type HotfixManifestProgressStore interface {
	LoadHotfixManifestProgress(
		ctx context.Context,
		repository RepositoryIdentity,
	) (HotfixManifestProgress, bool, error)
	StoreHotfixManifestProgress(
		ctx context.Context,
		repository RepositoryIdentity,
		progress HotfixManifestProgress,
	) error
	ClearHotfixManifestProgress(
		ctx context.Context,
		repository RepositoryIdentity,
	) error
}

// PullRequest describes a provider-neutral pull request intent. Body carries
// the mandatory pull-request description; it is required whenever the intent
// is published through a provider. The mandate and its canonical structure
// are owned by docs/conventions/pull-requests/description-mandate.md.
type PullRequest struct {
	Source branch.BranchName
	Target branch.BranchName
	Ticket ticket.ID
	Title  string
	Body   string
	Draft  bool
}

// PullRequestPublication carries provider-neutral pull-request intent together
// with the selected Git remote needed by a hosting adapter. RemoteURL is never
// included in the CLI result contract.
type PullRequestPublication struct {
	Repository  RepositoryIdentity
	RemoteURL   string
	PullRequest PullRequest
}

// PublishedPullRequest represents an optional provider-specific result.
type PublishedPullRequest struct {
	URL string
}

// PullRequestPublisher is optional: the product always emits provider-neutral
// pull-request data and publishes it only through an explicitly configured host.
type PullRequestPublisher interface {
	Publish(ctx context.Context, publication PullRequestPublication) (PublishedPullRequest, error)
}

// PullRequestPublisherPreflight is an optional capability for adapters that
// can validate credentials and routing before a Git branch is pushed.
type PullRequestPublisherPreflight interface {
	Validate(ctx context.Context, publication PullRequestPublication) error
}

// PullRequestInventoryQuery identifies the repository whose complete open and
// closed pull-request surface is enumerated through a hosting provider.
type PullRequestInventoryQuery struct {
	Repository RepositoryIdentity
	RemoteURL  string
}

// PullRequestSummary carries one provider-neutral pull-request record of the
// allocation inventory.
type PullRequestSummary struct {
	Number    string
	Title     string
	Author    string
	CreatedAt time.Time
	// State classifies the provider lifecycle of the record so completion
	// evidence never needs a second read: merged is measured from the
	// provider merge fact, closed from the raw state, and everything else
	// stays open.
	State PullRequestState
}

// PullRequestState classifies the lifecycle state of one inventory pull
// request record.
type PullRequestState string

const (
	// PullRequestStateOpen reports a pull request that is neither merged
	// nor closed.
	PullRequestStateOpen PullRequestState = "open"
	// PullRequestStateMerged reports a pull request the provider recorded
	// as merged.
	PullRequestStateMerged PullRequestState = "merged"
	// PullRequestStateClosed reports a pull request closed without a merge.
	PullRequestStateClosed PullRequestState = "closed"
)

// PullRequestInventoryLister is an optional capability for hosting adapters
// that can enumerate the complete pull-request surface of one repository
// through pagination. The allocation inventory parses titles with the
// canonical ticket grammars; it never string-matches, and a surface that
// cannot be read completely fails the inventory closed.
type PullRequestInventoryLister interface {
	ListPullRequests(ctx context.Context, query PullRequestInventoryQuery) ([]PullRequestSummary, error)
}
