package port

import (
	"context"
	"time"

	"github.com/t33n-software/git-governance/internal/domain/branch"
)

// QualityStatus classifies whether repository-local quality gates were
// configured and successfully executed.
type QualityStatus string

const (
	QualityUnconfigured QualityStatus = "unconfigured"
	QualitySkipped      QualityStatus = "skipped"
	QualityPassed       QualityStatus = "passed"
)

// QualityGateResult records one successfully completed configured gate without
// exposing its process output, which may contain sensitive project data.
type QualityGateResult struct {
	Name string
}

// QualityResult reports the actual quality-gate outcome. An unconfigured
// repository is not reported as a passed suite.
type QualityResult struct {
	Status QualityStatus
	Detail string
	Gates  []QualityGateResult
}

// QualityRequest identifies the branch families relevant to one
// publication-affecting operation. The runner evaluates each configured gate
// once against this complete set, so a multi-ref push cannot duplicate work.
type QualityRequest struct {
	Families []branch.Family
}

// QualityRunner runs repository-defined local quality gates before a
// publication-affecting operation.
type QualityRunner interface {
	Run(ctx context.Context, repository RepositoryIdentity, request QualityRequest) (QualityResult, error)
}

// QualityFingerprint binds a successful quality run to the configuration and
// toolchain that selected its gates. It contains no process output or secrets.
type QualityFingerprint struct {
	ConfigurationDigest string
	Gates               []string
	Toolchain           string
}

// QualityEvidenceRunner executes configured quality gates and returns the
// matching fingerprint from the same configuration snapshot.
type QualityEvidenceRunner interface {
	QualityRunner
	RunWithFingerprint(
		ctx context.Context,
		repository RepositoryIdentity,
		request QualityRequest,
	) (QualityResult, QualityFingerprint, error)
	Fingerprint(
		ctx context.Context,
		repository RepositoryIdentity,
		request QualityRequest,
	) (QualityFingerprint, error)
}

// FinalQualityEvidenceUpdate identifies one exact branch update covered by a
// local final-quality result. BaseRevision is empty only when no target base
// applies, such as a private scratch branch.
type FinalQualityEvidenceUpdate struct {
	LocalRef      string
	LocalObjectID string
	RemoteRef     string
	Branch        string
	Base          string
	BaseRevision  string
}

// FinalQualityEvidence is local Git metadata used only to deduplicate an
// identical quality run for an outgoing publish candidate. It is never a
// server-side approval or authorization artifact.
type FinalQualityEvidence struct {
	SchemaVersion       int
	Remote              string
	ConfigurationDigest string
	Gates               []string
	Toolchain           string
	WorktreeClean       bool
	CreatedAt           time.Time
	Updates             []FinalQualityEvidenceUpdate
}

// FinalQualityEvidenceStore persists a short-lived final-quality record in
// repository-local Git metadata rather than in the working tree.
type FinalQualityEvidenceStore interface {
	LoadFinalQualityEvidence(
		ctx context.Context,
		repository RepositoryIdentity,
	) (FinalQualityEvidence, bool, error)
	StoreFinalQualityEvidence(
		ctx context.Context,
		repository RepositoryIdentity,
		evidence FinalQualityEvidence,
	) error
}

// RevisionResolver resolves a ref to the exact commit object used for
// revision-bound local quality evidence.
type RevisionResolver interface {
	ResolveRevision(ctx context.Context, repository RepositoryIdentity, revision string) (string, error)
}
