package branchapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestParsePrePushUpdates(t *testing.T) {
	t.Parallel()

	zero := strings.Repeat("0", 40)
	local := strings.Repeat("a", 40)
	remote := strings.Repeat("b", 40)
	input := strings.Join([]string{
		"refs/heads/feature/ABC-123-add-export " + local + " refs/heads/feature/ABC-123-add-export " + zero,
		"HEAD " + local + " refs/heads/main " + remote,
		"(delete) " + zero + " refs/heads/scratch/ABC-123-experiment " + remote,
		"refs/tags/v1.0.0 " + local + " refs/tags/v1.0.0 " + zero,
	}, "\n") + "\n"

	updates, err := ParsePrePushUpdates(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 4 {
		t.Fatalf("len(ParsePrePushUpdates()) = %d, want 4", len(updates))
	}

	if updates[0].Action != PushActionCreate || !updates[0].GovernedBranch || updates[0].Target.String() != "feature/ABC-123-add-export" {
		t.Fatalf("create update = %#v", updates[0])
	}
	if updates[1].Action != PushActionUpdate || updates[1].Target.String() != "main" || updates[1].LocalRef != "HEAD" {
		t.Fatalf("HEAD:main update = %#v", updates[1])
	}
	if updates[2].Action != PushActionDelete || updates[2].Target.String() != "scratch/ABC-123-experiment" {
		t.Fatalf("delete update = %#v", updates[2])
	}
	if updates[3].Action != PushActionOther || updates[3].GovernedBranch {
		t.Fatalf("tag update = %#v", updates[3])
	}
}

func TestParsePrePushUpdatesRejectsMalformedInput(t *testing.T) {
	t.Parallel()

	validOID := strings.Repeat("a", 40)
	for _, testCase := range []struct {
		raw  string
		code problem.Code
	}{
		{raw: "too few fields", code: problem.CodeInvalidInput},
		{raw: "refs/heads/main " + validOID + " refs/heads/main invalid", code: problem.CodeInvalidInput},
		{raw: "refs/heads/main " + validOID + " main " + validOID, code: problem.CodeInvalidInput},
		{raw: "refs/heads/main " + validOID + " refs/heads/feat/ABC-123-invalid " + validOID, code: problem.CodeBranchNameInvalid},
		{raw: "(delete) " + validOID + " refs/heads/feature/ABC-123-add-export " + validOID, code: problem.CodeInvalidInput},
		{raw: "refs/heads/feature/ABC-123-add-export " + strings.Repeat("0", 40) + " refs/heads/feature/ABC-123-add-export " + validOID + "\n\n", code: problem.CodeInvalidInput},
	} {
		testCase := testCase
		t.Run(testCase.raw, func(t *testing.T) {
			t.Parallel()
			_, err := ParsePrePushUpdates(strings.NewReader(testCase.raw))
			assertProblemCode(t, err, testCase.code)
		})
	}
}

func TestValidatePrePushUpdatesBlocksActualSharedLineTargets(t *testing.T) {
	t.Parallel()

	git := &fakeGitRepository{}
	synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
	update := pushUpdate(t, "HEAD", "main", PushActionUpdate)

	_, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), []PushUpdate{update}, nil)
	assertProblemCode(t, err, problem.CodeSharedLineMutationForbidden)
	if got := strings.Join(git.calls, ","); got != "fetch,validate-ref" {
		t.Fatalf("calls = %q, want fetch,validate-ref", got)
	}
}

func TestValidatePrePushUpdatesPropagatesSharedLineReferenceValidationFailure(t *testing.T) {
	t.Parallel()

	validateErr := errors.New("shared line ref is invalid")
	git := &fakeGitRepository{validateRefErr: validateErr}
	synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)

	_, err := synchronizer.ValidatePrePushUpdates(
		context.Background(),
		testRepository(),
		[]PushUpdate{pushUpdate(t, "HEAD", "main", PushActionUpdate)},
		nil,
	)
	if !errors.Is(err, validateErr) {
		t.Fatalf("ValidatePrePushUpdates() error = %v, want %v", err, validateErr)
	}
}

func TestValidatePrePushUpdatesValidatesEveryBranchUpdate(t *testing.T) {
	t.Parallel()

	git := &fakeGitRepository{
		inspections: []port.PushUpdateInspection{{
			FastForward:    true,
			CommitMessages: []string{"feat(ABC-123): add export"},
		}},
	}
	synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
	feature := pushUpdate(t, "refs/heads/feature/ABC-123-add-export", "feature/ABC-123-add-export", PushActionCreate)
	main := pushUpdate(t, "HEAD", "main", PushActionUpdate)

	_, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), []PushUpdate{feature, main}, nil)
	assertProblemCode(t, err, problem.CodeSharedLineMutationForbidden)
	if got := strings.Join(git.calls, ","); got != "fetch,validate-ref,inspect-push,validate-ref" {
		t.Fatalf("calls = %q", got)
	}
}

func TestValidatePrePushUpdatesRunsQualityOnceForAllGovernedFamilies(t *testing.T) {
	t.Parallel()

	git := &fakeGitRepository{
		inspections: []port.PushUpdateInspection{
			{FastForward: true, CommitMessages: []string{"feat(ABC-123): add export"}},
			{FastForward: true, CommitMessages: []string{"docs(ABC-124): update guide"}},
		},
	}
	quality := &fakeQualityRunner{}
	synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), quality)

	result, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), []PushUpdate{
		pushUpdate(t, "refs/heads/feature/ABC-123-add-export", "feature/ABC-123-add-export", PushActionCreate),
		pushUpdate(t, "refs/heads/docs/ABC-124-update-guide", "docs/ABC-124-update-guide", PushActionCreate),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if quality.calls != 1 || len(quality.requests) != 1 {
		t.Fatalf("quality calls = %d, requests = %#v", quality.calls, quality.requests)
	}
	families := quality.requests[0].Families
	if len(families) != 2 || families[0] != branch.FamilyFeature || families[1] != branch.FamilyDocs {
		t.Fatalf("quality families = %v", families)
	}
	if result.Quality.Status != port.QualityPassed {
		t.Fatalf("quality result = %#v", result.Quality)
	}
}

func TestValidatePrePushUpdatesEnforcesFirstPushAndForcePushPolicy(t *testing.T) {
	t.Parallel()

	t.Run("stale first push", func(t *testing.T) {
		git := &fakeGitRepository{
			inspections: []port.PushUpdateInspection{{
				MissingBaseCommits: true,
				FastForward:        true,
				CommitMessages:     []string{"feat(ABC-123): add export"},
			}},
		}
		synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		_, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), []PushUpdate{
			pushUpdate(t, "refs/heads/feature/ABC-123-add-export", "feature/ABC-123-add-export", PushActionCreate),
		}, nil)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})

	t.Run("non-fast-forward published update", func(t *testing.T) {
		git := &fakeGitRepository{
			inspections: []port.PushUpdateInspection{{
				FastForward:    false,
				CommitMessages: []string{"feat(ABC-123): add export"},
			}},
		}
		synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		_, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), []PushUpdate{
			pushUpdate(t, "refs/heads/feature/ABC-123-add-export", "feature/ABC-123-add-export", PushActionUpdate),
		}, nil)
		assertProblemCode(t, err, problem.CodeForcePushForbidden)
	})
}

func TestValidatePrePushUpdatesHandlesDeletionAndNonBranchRefs(t *testing.T) {
	t.Parallel()

	t.Run("working branch deletion is classified", func(t *testing.T) {
		git := &fakeGitRepository{}
		synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		result, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), []PushUpdate{
			pushUpdate(t, "(delete)", "feature/ABC-123-add-export", PushActionDelete),
		}, nil)
		if err != nil || len(result.Updates) != 1 || result.Updates[0].Update.Action != PushActionDelete {
			t.Fatalf("ValidatePrePushUpdates() = (%#v, %v)", result, err)
		}
		if result.Quality.Status != port.QualitySkipped {
			t.Fatalf("deletion quality = %#v", result.Quality)
		}
	})

	t.Run("shared line deletion is forbidden", func(t *testing.T) {
		git := &fakeGitRepository{}
		synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		_, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), []PushUpdate{
			pushUpdate(t, "(delete)", "main", PushActionDelete),
		}, nil)
		assertProblemCode(t, err, problem.CodeSharedLineMutationForbidden)
	})

	t.Run("tag ref is explicitly skipped", func(t *testing.T) {
		git := &fakeGitRepository{}
		synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		result, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), []PushUpdate{{
			LocalRef:       "refs/tags/v1.0.0",
			LocalObjectID:  strings.Repeat("a", 40),
			RemoteRef:      "refs/tags/v1.0.0",
			RemoteObjectID: strings.Repeat("0", 40),
			Action:         PushActionOther,
		}}, nil)
		if err != nil || len(result.Updates) != 1 || !result.Updates[0].Skipped {
			t.Fatalf("ValidatePrePushUpdates() = (%#v, %v)", result, err)
		}
		if result.Quality.Status != port.QualitySkipped {
			t.Fatalf("tag quality = %#v", result.Quality)
		}
	})
}

func pushUpdate(t *testing.T, localRef, target string, action PushAction) PushUpdate {
	t.Helper()
	name := mustBranch(target)
	localObjectID := strings.Repeat("a", 40)
	remoteObjectID := strings.Repeat("b", 40)
	if action == PushActionCreate {
		remoteObjectID = strings.Repeat("0", 40)
	}
	if action == PushActionDelete {
		localObjectID = strings.Repeat("0", 40)
	}
	return PushUpdate{
		LocalRef:       localRef,
		LocalObjectID:  localObjectID,
		RemoteRef:      "refs/heads/" + target,
		RemoteObjectID: remoteObjectID,
		Target:         name,
		Action:         action,
		GovernedBranch: true,
	}
}

func TestValidatePrePushUpdatesUsesExplicitHotfixBase(t *testing.T) {
	t.Parallel()

	base := mustBase("origin", "main")
	git := &fakeGitRepository{
		inspections: []port.PushUpdateInspection{{
			FastForward:    true,
			CommitMessages: []string{"fix(ABC-999): resolve timeout"},
		}},
	}
	synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
	result, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), []PushUpdate{
		pushUpdate(t, "refs/heads/hotfix/ABC-999-payment-timeout", "hotfix/ABC-999-payment-timeout", PushActionCreate),
	}, &base)
	if err != nil || len(result.Updates) != 1 || result.Updates[0].Base == nil || result.Updates[0].Base.String() != "origin/main" {
		t.Fatalf("ValidatePrePushUpdates() = (%#v, %v)", result, err)
	}
}

func TestValidatePrePushUpdatesUsesStoredWorkflowBase(t *testing.T) {
	t.Parallel()

	base := mustBase("origin", "main")
	git := &fakeGitRepository{
		workflowBase: &base,
		inspections: []port.PushUpdateInspection{{
			FastForward:    true,
			CommitMessages: []string{"fix(ABC-999): resolve timeout"},
		}},
	}
	synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
	result, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), []PushUpdate{
		pushUpdate(t, "refs/heads/hotfix/ABC-999-payment-timeout", "hotfix/ABC-999-payment-timeout", PushActionCreate),
	}, nil)
	if err != nil || len(result.Updates) != 1 || result.Updates[0].Base == nil || result.Updates[0].Base.String() != "origin/main" {
		t.Fatalf("ValidatePrePushUpdates() = (%#v, %v)", result, err)
	}
}

func TestValidateCommitSeries(t *testing.T) {
	t.Parallel()

	name := mustBranch("feature/ABC-123-add-export")
	if err := ValidateCommitSeries(name, []string{"feat(ABC-123): add export", "test(ABC-123): cover export"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCommitSeries(name, nil); err == nil {
		t.Fatal("ValidateCommitSeries accepted no commits")
	}
	err := ValidateCommitSeries(name, []string{"feat(ABC-124): unrelated"})
	assertProblemCode(t, err, problem.CodeCommitTicketMismatch)
}

func TestPushUpdateParserAcceptsSHA256(t *testing.T) {
	t.Parallel()

	oid := strings.Repeat("a", 64)
	input := "refs/heads/feature/ABC-123-add-export " + oid + " refs/heads/feature/ABC-123-add-export " + strings.Repeat("0", 64)
	updates, err := ParsePrePushUpdates(strings.NewReader(input))
	if err != nil || len(updates) != 1 || updates[0].LocalObjectID != oid {
		t.Fatalf("ParsePrePushUpdates() = (%#v, %v)", updates, err)
	}
}

func TestPrePushParserWhiteboxBoundaries(t *testing.T) {
	t.Parallel()

	t.Run("nil and empty input have no updates", func(t *testing.T) {
		updates, err := ParsePrePushUpdates(nil)
		if err != nil || updates != nil {
			t.Fatalf("ParsePrePushUpdates(nil) = (%#v, %v)", updates, err)
		}
		updates, err = ParsePrePushUpdates(strings.NewReader("\n"))
		if err != nil || updates != nil {
			t.Fatalf("ParsePrePushUpdates(empty) = (%#v, %v)", updates, err)
		}
	})

	t.Run("reader, size, and carriage return errors are classified", func(t *testing.T) {
		_, err := ParsePrePushUpdates(failingPushReader{err: errors.New("read failed")})
		assertProblemCode(t, err, problem.CodeExternalCommandFailed)

		_, err = ParsePrePushUpdates(strings.NewReader(strings.Repeat("a", maxPrePushInputBytes+1)))
		assertProblemCode(t, err, problem.CodeInvalidInput)

		oid := strings.Repeat("a", 40)
		_, err = ParsePrePushUpdates(strings.NewReader("HEAD " + oid + " refs/heads/main " + oid + "\r"))
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("deletion marker and non-head behavior are validated", func(t *testing.T) {
		oid := strings.Repeat("a", 40)
		zero := strings.Repeat("0", 40)
		for _, raw := range []string{
			"HEAD " + zero + " refs/heads/feature/ABC-123-add-export " + oid,
			"(delete) " + oid + " refs/heads/feature/ABC-123-add-export " + oid,
		} {
			_, err := ParsePrePushUpdates(strings.NewReader(raw))
			assertProblemCode(t, err, problem.CodeInvalidInput)
		}

		updates, err := ParsePrePushUpdates(strings.NewReader("HEAD " + strings.ToUpper(oid) + " refs/tags/v1.0.0 " + zero))
		if err != nil || len(updates) != 1 || updates[0].Action != PushActionOther || updates[0].LocalObjectID != oid {
			t.Fatalf("non-head update = (%#v, %v)", updates, err)
		}

		_, err = parsePrePushUpdate("")
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("all helper forms are covered", func(t *testing.T) {
		if anyEmpty([]string{"present", "also-present"}) {
			t.Fatal("anyEmpty reported populated values")
		}
		if !anyEmpty([]string{"present", ""}) {
			t.Fatal("anyEmpty did not find empty value")
		}
	})
}

func TestPrePushValidationWhiteboxFailuresAndOptionalQuality(t *testing.T) {
	t.Parallel()

	update := pushUpdate(t, "refs/heads/feature/ABC-123-add-export", "feature/ABC-123-add-export", PushActionCreate)
	t.Run("repository, dependency, and update requirements", func(t *testing.T) {
		_, err := NewSynchronizer(&fakeGitRepository{}, NewService(&fakeGitRepository{}, &fakeKeyPolicy{}), nil).
			ValidatePrePushUpdates(context.Background(), port.RepositoryIdentity{}, []PushUpdate{update}, nil)
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)

		_, err = NewSynchronizer(nil, nil, nil).ValidatePrePushUpdates(context.Background(), testRepository(), []PushUpdate{update}, nil)
		assertProblemCode(t, err, problem.CodeInternal)

		git := &fakeGitRepository{}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).
			ValidatePrePushUpdates(context.Background(), testRepository(), nil, nil)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("fetch, ref, inspection, and quality failures propagate", func(t *testing.T) {
		fetchErr := errors.New("fetch failed")
		git := &fakeGitRepository{fetchErr: fetchErr}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).
			ValidatePrePushUpdates(context.Background(), testRepository(), []PushUpdate{update}, nil)
		if !errors.Is(err, fetchErr) {
			t.Fatalf("fetch error = %v", err)
		}

		refErr := errors.New("ref failed")
		git = &fakeGitRepository{validateRefErr: refErr}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).
			ValidatePrePushUpdates(context.Background(), testRepository(), []PushUpdate{update}, nil)
		if !errors.Is(err, refErr) {
			t.Fatalf("ref error = %v", err)
		}

		inspectionErr := errors.New("inspection failed")
		git = &fakeGitRepository{inspectionErr: inspectionErr}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).
			ValidatePrePushUpdates(context.Background(), testRepository(), []PushUpdate{update}, nil)
		if !errors.Is(err, inspectionErr) {
			t.Fatalf("inspection error = %v", err)
		}

		quality := &fakeQualityRunner{err: errors.New("quality failed")}
		git = &fakeGitRepository{inspections: []port.PushUpdateInspection{{FastForward: true, CommitMessages: []string{"feat(ABC-123): add export"}}}}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), quality).
			ValidatePrePushUpdates(context.Background(), testRepository(), []PushUpdate{update}, nil)
		if err == nil {
			t.Fatal("quality failure was not returned")
		}

		workflowErr := errors.New("workflow metadata failed")
		hotfix := pushUpdate(t, "refs/heads/hotfix/ABC-123-payment-timeout", "hotfix/ABC-123-payment-timeout", PushActionCreate)
		git = &fakeGitRepository{workflowBaseErr: workflowErr}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).
			ValidatePrePushUpdates(context.Background(), testRepository(), []PushUpdate{hotfix}, nil)
		if !errors.Is(err, workflowErr) {
			t.Fatalf("workflow base error = %v", err)
		}

		git = &fakeGitRepository{}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).
			ValidatePrePushUpdates(context.Background(), testRepository(), []PushUpdate{hotfix}, nil)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)

		git = &fakeGitRepository{inspections: []port.PushUpdateInspection{{FastForward: true, CommitMessages: []string{"invalid commit"}}}}
		_, err = NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).
			ValidatePrePushUpdates(context.Background(), testRepository(), []PushUpdate{update}, nil)
		if err == nil {
			t.Fatal("invalid commit series was accepted")
		}
	})

	t.Run("scratch is optional and no runner reports unconfigured", func(t *testing.T) {
		git := &fakeGitRepository{}
		synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		result, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), []PushUpdate{
			pushUpdate(t, "refs/heads/scratch/ABC-123-experiment", "scratch/ABC-123-experiment", PushActionCreate),
		}, nil)
		if err != nil || result.Quality.Status != port.QualityUnconfigured {
			t.Fatalf("scratch validation = (%#v, %v)", result, err)
		}
	})
}

func TestPushHelperContracts(t *testing.T) {
	t.Parallel()

	t.Run("commit series handles non-official and malformed messages", func(t *testing.T) {
		if err := ValidateCommitSeries(mustBranch("scratch/ABC-123-experiment"), nil); err != nil {
			t.Fatalf("scratch commit series error = %v", err)
		}
		if err := ValidateCommitSeries(mustBranch("feature/ABC-123-add-export"), []string{"not a conventional commit"}); err == nil {
			t.Fatal("malformed commit message was accepted")
		}
	})

	t.Run("base resolution covers explicit workflow inputs", func(t *testing.T) {
		feature := mustBranch("feature/ABC-123-add-export")
		base := mustBase("origin", "develop")
		resolved, err := resolveSyncBase(feature, testRepository(), &base, false)
		if err != nil || resolved.String() != base.String() {
			t.Fatalf("resolveSyncBase explicit = (%q, %v)", resolved, err)
		}

		hotfix := mustBranch("hotfix/ABC-123-payment-timeout")
		if _, err := resolveSyncBase(hotfix, testRepository(), nil, false); err == nil {
			t.Fatal("hotfix base was inferred without explicit or stored workflow base")
		}

		wrong := mustBase("origin", "develop")
		if _, err := resolveSyncBase(hotfix, testRepository(), &wrong, false); err == nil {
			t.Fatal("hotfix accepted develop as its explicit base")
		}

		release := mustBase("origin", "release/2.8.0")
		resolved, err = resolveSyncBase(mustBranch("fix/ABC-123-release-blocker"), testRepository(), &release, true)
		if err != nil || resolved.String() != release.String() {
			t.Fatalf("workflow fix release base = (%q, %v)", resolved, err)
		}

		invalid := mustBase("origin", "feature/ABC-123-add-export")
		if _, err := resolveSyncBase(mustBranch("fix/ABC-123-release-blocker"), testRepository(), &invalid, true); err == nil {
			t.Fatal("workflow fix accepted an invalid special base")
		}
	})

	t.Run("base resolution rejects mismatched and missing bases", func(t *testing.T) {
		feature := mustBranch("feature/ABC-123-add-export")
		main := mustBase("origin", "main")
		if _, err := resolveSyncBase(feature, testRepository(), &main, false); err == nil {
			t.Fatal("feature accepted main as synchronization base")
		}
		if _, err := resolveSyncBase(mustBranch("hotfix/ABC-123-payment-timeout"), testRepository(), nil, false); err == nil {
			t.Fatal("hotfix accepted a missing explicit base")
		}
		if _, err := resolveSyncBase(feature, port.RepositoryIdentity{Root: "C:/repo", Remote: "bad/ref"}, nil, false); err == nil {
			t.Fatal("feature accepted an invalid remote")
		}
	})

	t.Run("unknown programmatic target is rejected before domain validation", func(t *testing.T) {
		git := &fakeGitRepository{}
		_, err := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil).ValidatePrePushUpdates(
			context.Background(),
			testRepository(),
			[]PushUpdate{{
				LocalRef:       "HEAD",
				LocalObjectID:  strings.Repeat("a", 40),
				RemoteRef:      "refs/heads/unknown",
				RemoteObjectID: strings.Repeat("0", 40),
				GovernedBranch: true,
				Action:         PushActionCreate,
			}},
			nil,
		)
		assertProblemCode(t, err, problem.CodeBranchFamilyInvalid)
	})
}

type failingPushReader struct {
	err error
}

func (reader failingPushReader) Read([]byte) (int, error) {
	return 0, reader.err
}

func TestPushUpdateResultRetainsTargetFamily(t *testing.T) {
	t.Parallel()

	update := pushUpdate(t, "HEAD", "feature/ABC-123-add-export", PushActionUpdate)
	if update.Target.Family() != branch.FamilyFeature {
		t.Fatalf("target family = %q", update.Target.Family())
	}
}
