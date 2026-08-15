package branchapp

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

const (
	finalQualityEvidenceSchemaVersion = 1
	defaultFinalQualityEvidenceTTL    = 15 * time.Minute
)

// FinalQualityGate binds one successful local quality suite to the exact
// outgoing revisions it validated. The record is an optimization for a later
// pre-push hook, never a substitute for remote CI or protected-branch policy.
type FinalQualityGate struct {
	git       port.GitRepository
	quality   port.QualityEvidenceRunner
	evidence  port.FinalQualityEvidenceStore
	revisions port.RevisionResolver
	now       func() time.Time
	ttl       time.Duration
}

// NewFinalQualityGate creates a final-quality coordinator from optional Git
// metadata capabilities. Runtime composition must provide all capabilities;
// a missing one fails closed when final publication is attempted.
func NewFinalQualityGate(git port.GitRepository, quality port.QualityRunner) *FinalQualityGate {
	evidence, _ := git.(port.FinalQualityEvidenceStore)
	revisions, _ := git.(port.RevisionResolver)
	evidenceRunner, _ := quality.(port.QualityEvidenceRunner)
	return newFinalQualityGate(git, evidenceRunner, evidence, revisions, time.Now, defaultFinalQualityEvidenceTTL)
}

// Available reports whether the active runtime can create and validate
// revision-bound local evidence. Test and compatibility runtimes may omit the
// optional Git metadata capabilities and retain the conservative full-suite
// fallback instead.
func (gate *FinalQualityGate) Available() bool {
	return gate != nil &&
		gate.git != nil &&
		gate.quality != nil &&
		gate.evidence != nil &&
		gate.revisions != nil
}

func newFinalQualityGate(
	git port.GitRepository,
	quality port.QualityEvidenceRunner,
	evidence port.FinalQualityEvidenceStore,
	revisions port.RevisionResolver,
	now func() time.Time,
	ttl time.Duration,
) *FinalQualityGate {
	if now == nil {
		now = time.Now
	}
	if ttl <= 0 {
		ttl = defaultFinalQualityEvidenceTTL
	}
	return &FinalQualityGate{
		git:       git,
		quality:   quality,
		evidence:  evidence,
		revisions: revisions,
		now:       now,
		ttl:       ttl,
	}
}

// RunAndRecord executes final quality only after synchronization has produced
// the actual publish candidate and stores its local Git metadata record.
func (gate *FinalQualityGate) RunAndRecord(
	ctx context.Context,
	repository port.RepositoryIdentity,
	name branch.BranchName,
	base branch.TargetBase,
) (port.QualityResult, error) {
	return gate.validateOrRun(ctx, repository, []finalQualityCandidate{{
		LocalRef:  "refs/heads/" + name.String(),
		RemoteRef: "refs/heads/" + name.String(),
		Branch:    name,
		Base:      &base,
	}}, []branch.Family{name.Family()}, false)
}

// ValidateOrRunForBranch validates the structural pre-push result's exact
// candidate branch and reuses final evidence only when every binding matches.
func (gate *FinalQualityGate) ValidateOrRunForBranch(
	ctx context.Context,
	repository port.RepositoryIdentity,
	name branch.BranchName,
	base *branch.TargetBase,
) (port.QualityResult, error) {
	if err := gate.requireDependencies(); err != nil {
		return port.QualityResult{}, err
	}
	localRef := "refs/heads/" + name.String()
	revision, err := gate.revisions.ResolveRevision(ctx, repository, localRef)
	if err != nil {
		return port.QualityResult{}, err
	}
	return gate.validateOrRun(ctx, repository, []finalQualityCandidate{{
		LocalRef:      localRef,
		LocalObjectID: revision,
		RemoteRef:     localRef,
		Branch:        name,
		Base:          base,
	}}, []branch.Family{name.Family()}, true)
}

// ValidateOrRunForUpdates validates a complete Git pre-push batch. Structural
// checks occur in the caller for every update; this method shares one full
// local suite when no matching batch evidence exists.
func (gate *FinalQualityGate) ValidateOrRunForUpdates(
	ctx context.Context,
	repository port.RepositoryIdentity,
	results []PrePushUpdateResult,
) (port.QualityResult, error) {
	candidates := make([]finalQualityCandidate, 0, len(results))
	families := make([]branch.Family, 0, len(results))
	for _, result := range results {
		if !result.Update.GovernedBranch || result.Update.Action == PushActionDelete {
			continue
		}
		candidates = append(candidates, finalQualityCandidate{
			LocalRef:      result.Update.LocalRef,
			LocalObjectID: result.Update.LocalObjectID,
			RemoteRef:     result.Update.RemoteRef,
			Branch:        result.Update.Target,
			Base:          result.Base,
		})
		families = append(families, result.Update.Target.Family())
	}
	if len(candidates) == 0 {
		return port.QualityResult{
			Status: port.QualitySkipped,
			Detail: "no outgoing governed branch update requires quality gates",
		}, nil
	}
	return gate.validateOrRun(ctx, repository, candidates, families, true)
}

type finalQualityCandidate struct {
	LocalRef      string
	LocalObjectID string
	RemoteRef     string
	Branch        branch.BranchName
	Base          *branch.TargetBase
}

func (gate *FinalQualityGate) validateOrRun(
	ctx context.Context,
	repository port.RepositoryIdentity,
	candidates []finalQualityCandidate,
	families []branch.Family,
	allowReuse bool,
) (port.QualityResult, error) {
	if err := gate.requireDependencies(); err != nil {
		return port.QualityResult{}, err
	}
	if allowReuse {
		matched, result, err := gate.matches(ctx, repository, candidates, families)
		if err != nil {
			return port.QualityResult{}, err
		}
		if matched {
			return result, nil
		}
	}
	return gate.runAndRecord(ctx, repository, candidates, families)
}

func (gate *FinalQualityGate) runAndRecord(
	ctx context.Context,
	repository port.RepositoryIdentity,
	candidates []finalQualityCandidate,
	families []branch.Family,
) (port.QualityResult, error) {
	if err := gate.requireCleanWorktree(ctx, repository); err != nil {
		return port.QualityResult{}, err
	}
	result, fingerprint, err := gate.quality.RunWithFingerprint(ctx, repository, port.QualityRequest{Families: families})
	if err != nil {
		return port.QualityResult{}, err
	}
	if result.Status != port.QualityPassed {
		return result, nil
	}
	if err := gate.requireCleanWorktree(ctx, repository); err != nil {
		return port.QualityResult{}, err
	}
	updates, err := gate.evidenceUpdates(ctx, repository, candidates)
	if err != nil {
		return port.QualityResult{}, err
	}
	evidence := port.FinalQualityEvidence{
		SchemaVersion:       finalQualityEvidenceSchemaVersion,
		Remote:              repository.Remote,
		ConfigurationDigest: fingerprint.ConfigurationDigest,
		Gates:               append([]string(nil), fingerprint.Gates...),
		Toolchain:           fingerprint.Toolchain,
		WorktreeClean:       true,
		CreatedAt:           gate.now().UTC(),
		Updates:             updates,
	}
	if err := gate.evidence.StoreFinalQualityEvidence(ctx, repository, evidence); err != nil {
		return port.QualityResult{}, err
	}
	return result, nil
}

func (gate *FinalQualityGate) matches(
	ctx context.Context,
	repository port.RepositoryIdentity,
	candidates []finalQualityCandidate,
	families []branch.Family,
) (bool, port.QualityResult, error) {
	if err := gate.requireCleanWorktree(ctx, repository); err != nil {
		return false, port.QualityResult{}, err
	}
	evidence, found, err := gate.evidence.LoadFinalQualityEvidence(ctx, repository)
	if err != nil {
		return false, port.QualityResult{}, err
	}
	if !found {
		return false, port.QualityResult{}, nil
	}
	if err := gate.validateEvidence(evidence, repository); err != nil {
		return false, port.QualityResult{}, err
	}
	now := gate.now().UTC()
	if now.Before(evidence.CreatedAt) || now.Sub(evidence.CreatedAt) > gate.ttl {
		return false, port.QualityResult{}, nil
	}
	fingerprint, err := gate.quality.Fingerprint(ctx, repository, port.QualityRequest{Families: families})
	if err != nil {
		return false, port.QualityResult{}, err
	}
	if evidence.ConfigurationDigest != fingerprint.ConfigurationDigest ||
		evidence.Toolchain != fingerprint.Toolchain ||
		!sameStrings(evidence.Gates, fingerprint.Gates) {
		return false, port.QualityResult{}, nil
	}
	updates, err := gate.evidenceUpdates(ctx, repository, candidates)
	if err != nil {
		return false, port.QualityResult{}, err
	}
	if !sameEvidenceUpdates(evidence.Updates, updates) {
		return false, port.QualityResult{}, nil
	}
	return true, port.QualityResult{
		Status: port.QualityPassed,
		Detail: "reused final local quality evidence bound to the outgoing revisions",
		Gates:  qualityGateResults(evidence.Gates),
	}, nil
}

func (gate *FinalQualityGate) evidenceUpdates(
	ctx context.Context,
	repository port.RepositoryIdentity,
	candidates []finalQualityCandidate,
) ([]port.FinalQualityEvidenceUpdate, error) {
	updates := make([]port.FinalQualityEvidenceUpdate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.LocalRef == "" || candidate.RemoteRef == "" || candidate.Branch.IsZero() {
			return nil, finalQualityCandidateInvalid()
		}
		localObjectID := strings.ToLower(candidate.LocalObjectID)
		if localObjectID == "" {
			resolved, err := gate.revisions.ResolveRevision(ctx, repository, candidate.LocalRef)
			if err != nil {
				return nil, err
			}
			localObjectID = resolved
		}
		update := port.FinalQualityEvidenceUpdate{
			LocalRef:      candidate.LocalRef,
			LocalObjectID: localObjectID,
			RemoteRef:     candidate.RemoteRef,
			Branch:        candidate.Branch.String(),
		}
		if candidate.Base != nil {
			baseRevision, err := gate.revisions.ResolveRevision(ctx, repository, candidate.Base.String())
			if err != nil {
				return nil, err
			}
			update.Base = candidate.Base.String()
			update.BaseRevision = baseRevision
		}
		updates = append(updates, update)
	}
	sort.Slice(updates, func(left, right int) bool {
		return updates[left].RemoteRef < updates[right].RemoteRef
	})
	return updates, nil
}

func (gate *FinalQualityGate) requireDependencies() error {
	if !gate.Available() {
		return problem.New(problem.Details{
			Code:        problem.CodeInternal,
			Category:    problem.CategoryInternal,
			Field:       "final quality evidence",
			Expected:    "quality runner, Git revision resolver, and local Git metadata store",
			Rule:        "final publish quality evidence fails closed when its local dependencies are unavailable",
			Remediation: "repair runtime composition before attempting publication",
		})
	}
	return nil
}

func (gate *FinalQualityGate) requireCleanWorktree(ctx context.Context, repository port.RepositoryIdentity) error {
	clean, err := gate.git.IsWorktreeClean(ctx, repository)
	if err != nil {
		return err
	}
	if !clean {
		return problem.New(problem.Details{
			Code:        problem.CodeWorktreeNotClean,
			Category:    problem.CategoryRepository,
			Field:       "worktree",
			Expected:    "a clean worktree while final publish quality is recorded or reused",
			Rule:        "local quality evidence must describe the exact outgoing revision rather than uncommitted work",
			Remediation: "commit, stash, or discard unrelated changes, then rerun final quality",
		})
	}
	return nil
}

func (gate *FinalQualityGate) validateEvidence(
	evidence port.FinalQualityEvidence,
	repository port.RepositoryIdentity,
) error {
	if evidence.SchemaVersion != finalQualityEvidenceSchemaVersion ||
		evidence.Remote != repository.Remote ||
		evidence.ConfigurationDigest == "" ||
		evidence.Toolchain == "" ||
		!evidence.WorktreeClean ||
		evidence.CreatedAt.IsZero() ||
		len(evidence.Gates) == 0 ||
		len(evidence.Updates) == 0 {
		return finalQualityEvidenceInvalid()
	}
	for _, update := range evidence.Updates {
		if update.LocalRef == "" ||
			update.LocalObjectID == "" ||
			update.RemoteRef == "" ||
			update.Branch == "" ||
			(update.Base == "") != (update.BaseRevision == "") {
			return finalQualityEvidenceInvalid()
		}
	}
	return nil
}

func sameEvidenceUpdates(left, right []port.FinalQualityEvidenceUpdate) bool {
	if len(left) != len(right) {
		return false
	}
	left = append([]port.FinalQualityEvidenceUpdate(nil), left...)
	right = append([]port.FinalQualityEvidenceUpdate(nil), right...)
	sort.Slice(left, func(first, second int) bool {
		return left[first].RemoteRef < left[second].RemoteRef
	})
	sort.Slice(right, func(first, second int) bool {
		return right[first].RemoteRef < right[second].RemoteRef
	})
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func qualityGateResults(names []string) []port.QualityGateResult {
	results := make([]port.QualityGateResult, 0, len(names))
	for _, name := range names {
		results = append(results, port.QualityGateResult{Name: name})
	}
	return results
}

func finalQualityCandidateInvalid() error {
	return problem.New(problem.Details{
		Code:        problem.CodeInvalidInput,
		Category:    problem.CategoryGovernance,
		Field:       "final quality candidate",
		Expected:    "a canonical outgoing branch ref, revision, and optional target base",
		Rule:        "final quality evidence is bound to actual Git ref updates",
		Remediation: "retry the push through the governed publish workflow or Git pre-push hook",
	})
}

func finalQualityEvidenceInvalid() error {
	return problem.New(problem.Details{
		Code:        problem.CodeConfigurationInvalid,
		Category:    problem.CategoryConfig,
		Field:       "final quality evidence",
		Expected:    "a complete current repository-local final-quality record",
		Rule:        "corrupted or incomplete final-quality evidence fails closed",
		Remediation: "remove the corrupted local Git metadata record and rerun the final quality suite",
	})
}
