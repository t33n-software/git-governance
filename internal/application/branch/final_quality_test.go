package branchapp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/application/port"
	domainbranch "github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

type finalQualityGit struct {
	*fakeGitRepository

	revisions     map[string]string
	revisionErr   error
	evidence      port.FinalQualityEvidence
	evidenceFound bool
	evidenceErr   error
	storeErr      error
	cleanSequence []bool
	cleanCursor   int
}

func (git *finalQualityGit) IsWorktreeClean(ctx context.Context, repository port.RepositoryIdentity) (bool, error) {
	if len(git.cleanSequence) > 0 {
		value := git.cleanSequence[git.cleanCursor%len(git.cleanSequence)]
		git.cleanCursor++
		return value, nil
	}
	return git.fakeGitRepository.IsWorktreeClean(ctx, repository)
}

func (git *finalQualityGit) ResolveRevision(_ context.Context, _ port.RepositoryIdentity, revision string) (string, error) {
	if git.revisionErr != nil {
		return "", git.revisionErr
	}
	value, found := git.revisions[revision]
	if !found {
		return "", errors.New("missing revision " + revision)
	}
	return strings.ToLower(value), nil
}

func (git *finalQualityGit) LoadFinalQualityEvidence(
	context.Context,
	port.RepositoryIdentity,
) (port.FinalQualityEvidence, bool, error) {
	if git.evidenceErr != nil {
		return port.FinalQualityEvidence{}, false, git.evidenceErr
	}
	return git.evidence, git.evidenceFound, nil
}

func (git *finalQualityGit) StoreFinalQualityEvidence(
	_ context.Context,
	_ port.RepositoryIdentity,
	evidence port.FinalQualityEvidence,
) error {
	if git.storeErr != nil {
		return git.storeErr
	}
	git.evidence = evidence
	git.evidenceFound = true
	return nil
}

type finalQualityRunner struct {
	result           port.QualityResult
	fingerprint      port.QualityFingerprint
	runErr           error
	fingerprintErr   error
	runCalls         int
	fingerprintCalls int
	requests         []port.QualityRequest
}

func (runner *finalQualityRunner) Run(
	ctx context.Context,
	repository port.RepositoryIdentity,
	request port.QualityRequest,
) (port.QualityResult, error) {
	result, _, err := runner.RunWithFingerprint(ctx, repository, request)
	return result, err
}

func (runner *finalQualityRunner) RunWithFingerprint(
	_ context.Context,
	_ port.RepositoryIdentity,
	request port.QualityRequest,
) (port.QualityResult, port.QualityFingerprint, error) {
	runner.runCalls++
	runner.requests = append(runner.requests, cloneFinalQualityRequest(request))
	if runner.runErr != nil {
		return port.QualityResult{}, port.QualityFingerprint{}, runner.runErr
	}
	return runner.result, runner.fingerprint, nil
}

func (runner *finalQualityRunner) Fingerprint(
	_ context.Context,
	_ port.RepositoryIdentity,
	request port.QualityRequest,
) (port.QualityFingerprint, error) {
	runner.fingerprintCalls++
	runner.requests = append(runner.requests, cloneFinalQualityRequest(request))
	if runner.fingerprintErr != nil {
		return port.QualityFingerprint{}, runner.fingerprintErr
	}
	return runner.fingerprint, nil
}

func TestFinalQualityGateRecordsAndReusesExactCandidate(t *testing.T) {
	now := time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC)
	name := mustBranch("feature/ABC-123-add-export")
	base := mustBase("origin", "develop")
	git := newFinalQualityGit(name, base)
	runner := newFinalQualityRunner()
	gate := newFinalQualityGate(git, runner, git, git, func() time.Time { return now }, time.Minute)

	result, err := gate.RunAndRecord(context.Background(), testRepository(), name, base)
	if err != nil || result.Status != port.QualityPassed || runner.runCalls != 1 {
		t.Fatalf("RunAndRecord() = (%#v, %v), runs=%d", result, err, runner.runCalls)
	}
	evidence := git.evidence
	if !git.evidenceFound ||
		evidence.SchemaVersion != finalQualityEvidenceSchemaVersion ||
		evidence.Remote != "origin" ||
		evidence.CreatedAt != now ||
		len(evidence.Updates) != 1 ||
		evidence.Updates[0].LocalObjectID != strings.Repeat("a", 40) ||
		evidence.Updates[0].BaseRevision != strings.Repeat("b", 40) {
		t.Fatalf("stored final quality evidence = %#v", evidence)
	}

	result, err = gate.ValidateOrRunForBranch(context.Background(), testRepository(), name, &base)
	if err != nil || result.Status != port.QualityPassed || runner.runCalls != 1 || runner.fingerprintCalls != 1 {
		t.Fatalf(
			"ValidateOrRunForBranch() = (%#v, %v), runs=%d fingerprints=%d",
			result,
			err,
			runner.runCalls,
			runner.fingerprintCalls,
		)
	}
	if !strings.Contains(result.Detail, "reused final local quality evidence") {
		t.Fatalf("reuse detail = %q", result.Detail)
	}
}

func TestFinalQualityGateFallsBackForExpiredOrMismatchedEvidence(t *testing.T) {
	now := time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC)
	name := mustBranch("feature/ABC-123-add-export")
	base := mustBase("origin", "develop")

	for _, testCase := range []struct {
		name      string
		configure func(*finalQualityGit, *finalQualityRunner)
	}{
		{
			name: "missing evidence",
			configure: func(git *finalQualityGit, _ *finalQualityRunner) {
				git.evidenceFound = false
			},
		},
		{
			name: "expired evidence",
			configure: func(git *finalQualityGit, _ *finalQualityRunner) {
				git.evidence = finalQualityEvidence(now.Add(-2*time.Minute), name, base)
				git.evidenceFound = true
			},
		},
		{
			name: "future evidence",
			configure: func(git *finalQualityGit, _ *finalQualityRunner) {
				git.evidence = finalQualityEvidence(now.Add(time.Minute), name, base)
				git.evidenceFound = true
			},
		},
		{
			name: "head changed",
			configure: func(git *finalQualityGit, _ *finalQualityRunner) {
				git.evidence = finalQualityEvidence(now, name, base)
				git.evidenceFound = true
				git.revisions["refs/heads/"+name.String()] = strings.Repeat("c", 40)
			},
		},
		{
			name: "base changed",
			configure: func(git *finalQualityGit, _ *finalQualityRunner) {
				git.evidence = finalQualityEvidence(now, name, base)
				git.evidenceFound = true
				git.revisions[base.String()] = strings.Repeat("d", 40)
			},
		},
		{
			name: "configuration changed",
			configure: func(git *finalQualityGit, runner *finalQualityRunner) {
				git.evidence = finalQualityEvidence(now, name, base)
				git.evidenceFound = true
				runner.fingerprint.ConfigurationDigest = "new-config"
			},
		},
		{
			name: "toolchain changed",
			configure: func(git *finalQualityGit, runner *finalQualityRunner) {
				git.evidence = finalQualityEvidence(now, name, base)
				git.evidenceFound = true
				runner.fingerprint.Toolchain = "go-test/other"
			},
		},
		{
			name: "gate selection changed",
			configure: func(git *finalQualityGit, runner *finalQualityRunner) {
				git.evidence = finalQualityEvidence(now, name, base)
				git.evidenceFound = true
				runner.fingerprint.Gates = []string{"other-gate"}
			},
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			git := newFinalQualityGit(name, base)
			runner := newFinalQualityRunner()
			testCase.configure(git, runner)
			gate := newFinalQualityGate(git, runner, git, git, func() time.Time { return now }, time.Minute)

			result, err := gate.ValidateOrRunForBranch(context.Background(), testRepository(), name, &base)
			if err != nil || result.Status != port.QualityPassed || runner.runCalls != 1 {
				t.Fatalf("ValidateOrRunForBranch() = (%#v, %v), runs=%d", result, err, runner.runCalls)
			}
		})
	}
}

func TestFinalQualityGateFailsClosedForInvalidDependenciesEvidenceAndWorktree(t *testing.T) {
	now := time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC)
	name := mustBranch("feature/ABC-123-add-export")
	base := mustBase("origin", "develop")

	t.Run("missing dependencies", func(t *testing.T) {
		_, err := (&FinalQualityGate{}).RunAndRecord(context.Background(), testRepository(), name, base)
		assertProblemCode(t, err, problem.CodeInternal)
		_, err = (&FinalQualityGate{}).ValidateOrRunForBranch(context.Background(), testRepository(), name, &base)
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("corrupted evidence", func(t *testing.T) {
		git := newFinalQualityGit(name, base)
		git.evidence = port.FinalQualityEvidence{SchemaVersion: finalQualityEvidenceSchemaVersion}
		git.evidenceFound = true
		runner := newFinalQualityRunner()
		gate := newFinalQualityGate(git, runner, git, git, func() time.Time { return now }, time.Minute)

		_, err := gate.ValidateOrRunForBranch(context.Background(), testRepository(), name, &base)
		assertProblemCode(t, err, problem.CodeConfigurationInvalid)
		if runner.runCalls != 0 {
			t.Fatalf("corrupted evidence unexpectedly ran quality: %d", runner.runCalls)
		}
	})

	t.Run("evidence store failure", func(t *testing.T) {
		git := newFinalQualityGit(name, base)
		git.storeErr = errors.New("metadata write failed")
		gate := newFinalQualityGate(git, newFinalQualityRunner(), git, git, func() time.Time { return now }, time.Minute)

		_, err := gate.RunAndRecord(context.Background(), testRepository(), name, base)
		if !errors.Is(err, git.storeErr) {
			t.Fatalf("store error = %v, want %v", err, git.storeErr)
		}
	})

	t.Run("evidence read failure", func(t *testing.T) {
		git := newFinalQualityGit(name, base)
		git.evidenceErr = errors.New("metadata read failed")
		gate := newFinalQualityGate(git, newFinalQualityRunner(), git, git, func() time.Time { return now }, time.Minute)

		_, err := gate.ValidateOrRunForBranch(context.Background(), testRepository(), name, &base)
		if !errors.Is(err, git.evidenceErr) {
			t.Fatalf("read error = %v, want %v", err, git.evidenceErr)
		}
	})

	t.Run("revision failure", func(t *testing.T) {
		git := newFinalQualityGit(name, base)
		git.revisionErr = errors.New("revision unavailable")
		gate := newFinalQualityGate(git, newFinalQualityRunner(), git, git, func() time.Time { return now }, time.Minute)

		_, err := gate.ValidateOrRunForBranch(context.Background(), testRepository(), name, &base)
		if !errors.Is(err, git.revisionErr) {
			t.Fatalf("revision error = %v, want %v", err, git.revisionErr)
		}
	})

	t.Run("recording preserves a post-quality revision failure", func(t *testing.T) {
		git := newFinalQualityGit(name, base)
		git.revisionErr = errors.New("revision unavailable")
		runner := newFinalQualityRunner()
		gate := newFinalQualityGate(git, runner, git, git, func() time.Time { return now }, time.Minute)

		_, err := gate.RunAndRecord(context.Background(), testRepository(), name, base)
		if !errors.Is(err, git.revisionErr) || runner.runCalls != 1 {
			t.Fatalf("post-quality revision error = (%v, runs=%d)", err, runner.runCalls)
		}
	})

	t.Run("dirty before quality", func(t *testing.T) {
		git := newFinalQualityGit(name, base)
		git.clean = false
		runner := newFinalQualityRunner()
		gate := newFinalQualityGate(git, runner, git, git, func() time.Time { return now }, time.Minute)

		_, err := gate.RunAndRecord(context.Background(), testRepository(), name, base)
		assertProblemCode(t, err, problem.CodeWorktreeNotClean)
		if runner.runCalls != 0 {
			t.Fatalf("dirty worktree unexpectedly ran quality: %d", runner.runCalls)
		}
	})

	t.Run("dirty after quality", func(t *testing.T) {
		git := newFinalQualityGit(name, base)
		git.cleanSequence = []bool{true, false}
		runner := newFinalQualityRunner()
		gate := newFinalQualityGate(git, runner, git, git, func() time.Time { return now }, time.Minute)

		_, err := gate.RunAndRecord(context.Background(), testRepository(), name, base)
		assertProblemCode(t, err, problem.CodeWorktreeNotClean)
		if runner.runCalls != 1 || git.evidenceFound {
			t.Fatalf("post-quality dirty state = runs:%d evidence:%t", runner.runCalls, git.evidenceFound)
		}
	})

	t.Run("runner and fingerprint failures are preserved", func(t *testing.T) {
		git := newFinalQualityGit(name, base)
		runner := newFinalQualityRunner()
		runner.runErr = errors.New("quality failed")
		gate := newFinalQualityGate(git, runner, git, git, func() time.Time { return now }, time.Minute)

		_, err := gate.RunAndRecord(context.Background(), testRepository(), name, base)
		if !errors.Is(err, runner.runErr) {
			t.Fatalf("quality error = %v, want %v", err, runner.runErr)
		}

		git = newFinalQualityGit(name, base)
		git.evidence = finalQualityEvidence(now, name, base)
		git.evidenceFound = true
		runner = newFinalQualityRunner()
		runner.fingerprintErr = errors.New("fingerprint failed")
		gate = newFinalQualityGate(git, runner, git, git, func() time.Time { return now }, time.Minute)
		_, err = gate.ValidateOrRunForBranch(context.Background(), testRepository(), name, &base)
		if !errors.Is(err, runner.fingerprintErr) {
			t.Fatalf("fingerprint error = %v, want %v", err, runner.fingerprintErr)
		}
	})

	t.Run("quality statuses without a full suite are not recorded", func(t *testing.T) {
		for _, status := range []port.QualityStatus{port.QualitySkipped, port.QualityUnconfigured} {
			git := newFinalQualityGit(name, base)
			runner := newFinalQualityRunner()
			runner.result.Status = status
			gate := newFinalQualityGate(git, runner, git, git, func() time.Time { return now }, time.Minute)

			result, err := gate.RunAndRecord(context.Background(), testRepository(), name, base)
			if err != nil || result.Status != status || git.evidenceFound {
				t.Fatalf("status %q = (%#v, %v), evidence=%t", status, result, err, git.evidenceFound)
			}
		}
	})

	t.Run("individual incomplete update is rejected", func(t *testing.T) {
		git := newFinalQualityGit(name, base)
		git.evidence = finalQualityEvidence(now, name, base)
		git.evidence.Updates[0].BaseRevision = ""
		git.evidenceFound = true
		gate := newFinalQualityGate(git, newFinalQualityRunner(), git, git, func() time.Time { return now }, time.Minute)

		_, err := gate.ValidateOrRunForBranch(context.Background(), testRepository(), name, &base)
		assertProblemCode(t, err, problem.CodeConfigurationInvalid)
	})

	t.Run("worktree inspection failures are preserved", func(t *testing.T) {
		git := newFinalQualityGit(name, base)
		git.worktreeCleanErr = errors.New("status failed")
		gate := newFinalQualityGate(git, newFinalQualityRunner(), git, git, func() time.Time { return now }, time.Minute)

		_, err := gate.RunAndRecord(context.Background(), testRepository(), name, base)
		if !errors.Is(err, git.worktreeCleanErr) {
			t.Fatalf("worktree error = %v, want %v", err, git.worktreeCleanErr)
		}
	})

	t.Run("dirty evidence lookup and base resolution failures are preserved", func(t *testing.T) {
		git := newFinalQualityGit(name, base)
		git.clean = false
		git.evidence = finalQualityEvidence(now, name, base)
		git.evidenceFound = true
		gate := newFinalQualityGate(git, newFinalQualityRunner(), git, git, func() time.Time { return now }, time.Minute)
		_, err := gate.ValidateOrRunForBranch(context.Background(), testRepository(), name, &base)
		assertProblemCode(t, err, problem.CodeWorktreeNotClean)

		git = newFinalQualityGit(name, base)
		git.evidence = finalQualityEvidence(now, name, base)
		git.evidenceFound = true
		delete(git.revisions, base.String())
		gate = newFinalQualityGate(git, newFinalQualityRunner(), git, git, func() time.Time { return now }, time.Minute)
		_, err = gate.ValidateOrRunForBranch(context.Background(), testRepository(), name, &base)
		if err == nil {
			t.Fatal("missing base revision was accepted")
		}
	})
}

func TestFinalQualityGateRunsOneBatchFallbackAndReusesMatchingBatch(t *testing.T) {
	now := time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC)
	first := mustBranch("feature/ABC-123-add-export")
	second := mustBranch("docs/ABC-124-update-guide")
	base := mustBase("origin", "develop")
	git := newFinalQualityGit(first, base)
	git.revisions["refs/heads/"+second.String()] = strings.Repeat("c", 40)
	runner := newFinalQualityRunner()
	gate := newFinalQualityGate(git, runner, git, git, func() time.Time { return now }, time.Minute)
	results := []PrePushUpdateResult{
		finalQualityPushUpdate(first, strings.Repeat("a", 40), &base),
		finalQualityPushUpdate(second, strings.Repeat("c", 40), &base),
	}

	result, err := gate.ValidateOrRunForUpdates(context.Background(), testRepository(), results)
	if err != nil || result.Status != port.QualityPassed || runner.runCalls != 1 {
		t.Fatalf("batch fallback = (%#v, %v), runs=%d", result, err, runner.runCalls)
	}
	if len(runner.requests) != 1 || len(runner.requests[0].Families) != 2 {
		t.Fatalf("batch quality requests = %#v", runner.requests)
	}
	if len(git.evidence.Updates) != 2 || git.evidence.Updates[0].RemoteRef > git.evidence.Updates[1].RemoteRef {
		t.Fatalf("batch evidence updates = %#v", git.evidence.Updates)
	}

	result, err = gate.ValidateOrRunForUpdates(context.Background(), testRepository(), results)
	if err != nil || result.Status != port.QualityPassed || runner.runCalls != 1 || runner.fingerprintCalls != 1 {
		t.Fatalf("batch reuse = (%#v, %v), runs=%d fingerprints=%d", result, err, runner.runCalls, runner.fingerprintCalls)
	}

	scratch := mustBranch("scratch/ABC-125-experiment")
	scratchResult, err := gate.ValidateOrRunForUpdates(context.Background(), testRepository(), []PrePushUpdateResult{{
		Update: PushUpdate{
			LocalRef:       "refs/heads/" + scratch.String(),
			LocalObjectID:  strings.Repeat("d", 40),
			RemoteRef:      "refs/heads/" + scratch.String(),
			RemoteObjectID: strings.Repeat("0", 40),
			Target:         scratch,
			Action:         PushActionCreate,
			GovernedBranch: true,
		},
	}})
	if err != nil || scratchResult.Status != port.QualityPassed || runner.runCalls != 2 || git.evidence.Updates[0].Base != "" {
		t.Fatalf("scratch batch = (%#v, %v), runs=%d evidence=%#v", scratchResult, err, runner.runCalls, git.evidence)
	}
}

func TestSynchronizerPrePushRetainsStructuralChecksWhileReusingFinalEvidence(t *testing.T) {
	now := time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC)
	name := mustBranch("feature/ABC-123-add-export")
	base := mustBase("origin", "develop")
	git := newFinalQualityGit(name, base)
	git.evidence = finalQualityEvidence(now, name, base)
	git.evidenceFound = true
	git.inspections = []port.PushUpdateInspection{{
		FastForward:    true,
		CommitMessages: []string{"feat(ABC-123): add export"},
	}}
	runner := newFinalQualityRunner()
	gate := newFinalQualityGate(git, runner, git, git, func() time.Time { return now }, time.Minute)
	synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), runner).
		WithFinalQualityGate(gate)
	update := PushUpdate{
		LocalRef:       "refs/heads/" + name.String(),
		LocalObjectID:  strings.Repeat("a", 40),
		RemoteRef:      "refs/heads/" + name.String(),
		RemoteObjectID: strings.Repeat("0", 40),
		Target:         name,
		Action:         PushActionCreate,
		GovernedBranch: true,
	}

	result, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), []PushUpdate{update}, nil)
	if err != nil || result.Quality.Status != port.QualityPassed || runner.runCalls != 0 || runner.fingerprintCalls != 1 {
		t.Fatalf(
			"ValidatePrePushUpdates() = (%#v, %v), runs=%d fingerprints=%d",
			result,
			err,
			runner.runCalls,
			runner.fingerprintCalls,
		)
	}
	if !strings.Contains(strings.Join(git.calls, ","), "fetch,validate-ref,inspect-push") {
		t.Fatalf("structural pre-push calls = %v", git.calls)
	}
}

func TestSynchronizerManualPrePushReusesFinalEvidence(t *testing.T) {
	now := time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC)
	name := mustBranch("feature/ABC-123-add-export")
	base := mustBase("origin", "develop")
	git := newFinalQualityGit(name, base)
	git.publication = domainbranch.PublicationUnpublished
	git.evidence = finalQualityEvidence(now, name, base)
	git.evidenceFound = true
	runner := newFinalQualityRunner()
	gate := newFinalQualityGate(git, runner, git, git, func() time.Time { return now }, time.Minute)
	synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), runner).
		WithFinalQualityGate(gate)

	result, err := synchronizer.ValidatePrePush(context.Background(), PrePushRequest{
		Repository: testRepository(),
		Name:       name,
		Base:       &base,
	})
	if err != nil || result.Quality.Status != port.QualityPassed || runner.runCalls != 0 || runner.fingerprintCalls != 1 {
		t.Fatalf(
			"ValidatePrePush() = (%#v, %v), runs=%d fingerprints=%d",
			result,
			err,
			runner.runCalls,
			runner.fingerprintCalls,
		)
	}
}

func TestFinalQualityGateHelperContracts(t *testing.T) {
	t.Parallel()

	left := []port.FinalQualityEvidenceUpdate{{
		LocalRef: "refs/heads/feature/ABC-123-add-export", LocalObjectID: "a", RemoteRef: "refs/heads/feature/ABC-123-add-export",
	}}
	right := append([]port.FinalQualityEvidenceUpdate(nil), left...)
	if !sameEvidenceUpdates(left, right) || sameEvidenceUpdates(left, nil) {
		t.Fatal("sameEvidenceUpdates did not preserve exact update equality")
	}
	if !sameStrings([]string{"one", "two"}, []string{"one", "two"}) ||
		sameStrings([]string{"one"}, []string{"two"}) ||
		sameStrings([]string{"one"}, []string{"one", "two"}) {
		t.Fatal("sameStrings did not preserve exact gate ordering")
	}
	if results := qualityGateResults([]string{"one", "two"}); len(results) != 2 || results[1].Name != "two" {
		t.Fatalf("qualityGateResults() = %#v", results)
	}
	if _, ok := problem.As(finalQualityCandidateInvalid()); !ok {
		t.Fatal("candidate problem is not typed")
	}
	if _, ok := problem.As(finalQualityEvidenceInvalid()); !ok {
		t.Fatal("evidence problem is not typed")
	}

	name := mustBranch("feature/ABC-123-add-export")
	base := mustBase("origin", "develop")
	git := newFinalQualityGit(name, base)
	runner := newFinalQualityRunner()
	constructed := NewFinalQualityGate(git, runner)
	if !constructed.Available() {
		t.Fatal("NewFinalQualityGate did not recognize complete dependencies")
	}
	defaulted := newFinalQualityGate(git, runner, git, git, nil, 0)
	if defaulted.now == nil || defaulted.ttl != defaultFinalQualityEvidenceTTL {
		t.Fatalf("defaulted final quality gate = %#v", defaulted)
	}

	_, err := defaulted.validateOrRun(context.Background(), testRepository(), []finalQualityCandidate{{}}, nil, false)
	assertProblemCode(t, err, problem.CodeInvalidInput)

	skipped, err := defaulted.ValidateOrRunForUpdates(context.Background(), testRepository(), []PrePushUpdateResult{{
		Update: PushUpdate{GovernedBranch: false},
	}})
	if err != nil || skipped.Status != port.QualitySkipped {
		t.Fatalf("empty governed batch = (%#v, %v)", skipped, err)
	}
}

func newFinalQualityGit(name domainbranch.BranchName, base domainbranch.TargetBase) *finalQualityGit {
	return &finalQualityGit{
		fakeGitRepository: &fakeGitRepository{clean: true},
		revisions: map[string]string{
			"refs/heads/" + name.String(): strings.Repeat("a", 40),
			base.String():                 strings.Repeat("b", 40),
		},
	}
}

func newFinalQualityRunner() *finalQualityRunner {
	return &finalQualityRunner{
		result: port.QualityResult{
			Status: port.QualityPassed,
			Detail: "quality passed",
			Gates:  []port.QualityGateResult{{Name: "full-local-build"}},
		},
		fingerprint: port.QualityFingerprint{
			ConfigurationDigest: "quality-config",
			Gates:               []string{"full-local-build"},
			Toolchain:           "go-test/windows-amd64",
		},
	}
}

func finalQualityEvidence(
	createdAt time.Time,
	name domainbranch.BranchName,
	base domainbranch.TargetBase,
) port.FinalQualityEvidence {
	return port.FinalQualityEvidence{
		SchemaVersion:       finalQualityEvidenceSchemaVersion,
		Remote:              "origin",
		ConfigurationDigest: "quality-config",
		Gates:               []string{"full-local-build"},
		Toolchain:           "go-test/windows-amd64",
		WorktreeClean:       true,
		CreatedAt:           createdAt,
		Updates: []port.FinalQualityEvidenceUpdate{{
			LocalRef:      "refs/heads/" + name.String(),
			LocalObjectID: strings.Repeat("a", 40),
			RemoteRef:     "refs/heads/" + name.String(),
			Branch:        name.String(),
			Base:          base.String(),
			BaseRevision:  strings.Repeat("b", 40),
		}},
	}
}

func finalQualityPushUpdate(
	name domainbranch.BranchName,
	objectID string,
	base *domainbranch.TargetBase,
) PrePushUpdateResult {
	return PrePushUpdateResult{
		Update: PushUpdate{
			LocalRef:       "refs/heads/" + name.String(),
			LocalObjectID:  objectID,
			RemoteRef:      "refs/heads/" + name.String(),
			RemoteObjectID: strings.Repeat("0", 40),
			Target:         name,
			Action:         PushActionCreate,
			GovernedBranch: true,
		},
		Base: base,
	}
}

func cloneFinalQualityRequest(request port.QualityRequest) port.QualityRequest {
	return port.QualityRequest{Families: append([]domainbranch.Family(nil), request.Families...)}
}

var _ port.GitRepository = (*finalQualityGit)(nil)
var _ port.FinalQualityEvidenceStore = (*finalQualityGit)(nil)
var _ port.RevisionResolver = (*finalQualityGit)(nil)
var _ port.QualityEvidenceRunner = (*finalQualityRunner)(nil)
