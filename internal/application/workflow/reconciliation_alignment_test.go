package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	branchapp "github.com/CyberT33N/git-governance/internal/application/branch"
	"github.com/CyberT33N/git-governance/internal/application/port"
	"github.com/CyberT33N/git-governance/internal/domain/branch"
	"github.com/CyberT33N/git-governance/internal/domain/commitmsg"
	"github.com/CyberT33N/git-governance/internal/domain/problem"
)

type reconciliationAlignmentGit struct {
	*releaseWhiteboxGit

	current         branch.BranchName
	currentErr      error
	targetExists    bool
	targetErr       error
	mergeErr        error
	validateErrors  []error
	mergedBase      branch.TargetBase
	mergedMessage   commitmsg.Message
	conflicts       bool
	conflictsErr    error
	continueErr     error
	continued       bool
	preserveMissing bool
	resolveErr      error
	headErr         error
	headMatches     bool
	releaseSHA      string
	developSHA      string
}

func (git *reconciliationAlignmentGit) CurrentBranch(context.Context, port.RepositoryIdentity) (branch.BranchName, error) {
	git.calls = append(git.calls, "current-branch")
	if git.currentErr != nil {
		return branch.BranchName{}, git.currentErr
	}
	return git.current, nil
}

func (git *reconciliationAlignmentGit) ValidateBranchRef(context.Context, port.RepositoryIdentity, branch.BranchName) error {
	git.calls = append(git.calls, "validate-ref")
	if len(git.validateErrors) > 0 {
		err := git.validateErrors[0]
		git.validateErrors = git.validateErrors[1:]
		return err
	}
	return git.validateErr
}

func (git *reconciliationAlignmentGit) TargetBaseExists(
	context.Context,
	port.RepositoryIdentity,
	branch.TargetBase,
) (bool, error) {
	git.calls = append(git.calls, "target-base-exists")
	if git.targetErr != nil {
		return false, git.targetErr
	}
	return git.targetExists, nil
}

func (git *reconciliationAlignmentGit) Merge(
	_ context.Context,
	_ port.RepositoryIdentity,
	base branch.TargetBase,
	message commitmsg.Message,
) error {
	git.calls = append(git.calls, "merge")
	if git.mergeErr != nil {
		return git.mergeErr
	}
	git.mergedBase = base
	git.mergedMessage = message
	return nil
}

func (git *reconciliationAlignmentGit) HasUnmergedConflicts(
	context.Context,
	port.RepositoryIdentity,
) (bool, error) {
	git.calls = append(git.calls, "unmerged-conflicts")
	return git.conflicts, git.conflictsErr
}

func (git *reconciliationAlignmentGit) ContinueMerge(
	context.Context,
	port.RepositoryIdentity,
) error {
	git.calls = append(git.calls, "continue-merge")
	if git.continueErr != nil {
		return git.continueErr
	}
	git.continued = true
	git.active = false
	git.activeOperation = ""
	if !git.preserveMissing {
		git.missing = false
	}
	return nil
}

func (git *reconciliationAlignmentGit) ResolveReconciliationBases(
	context.Context,
	port.RepositoryIdentity,
	branch.TargetBase,
	branch.TargetBase,
) (string, string, error) {
	git.calls = append(git.calls, "resolve-reconciliation-bases")
	if git.resolveErr != nil {
		return "", "", git.resolveErr
	}
	return git.releaseSHA, git.developSHA, nil
}

func (git *reconciliationAlignmentGit) HeadIsMergeOf(
	context.Context,
	port.RepositoryIdentity,
	branch.TargetBase,
	branch.TargetBase,
) (bool, error) {
	git.calls = append(git.calls, "head-is-reconciliation-merge")
	return git.headMatches, git.headErr
}

type reconciliationAlignmentNoInspectorGit struct {
	*releaseWhiteboxGit

	current   branch.BranchName
	conflicts bool
	mergeErr  error
}

func (git *reconciliationAlignmentNoInspectorGit) CurrentBranch(
	context.Context,
	port.RepositoryIdentity,
) (branch.BranchName, error) {
	return git.current, nil
}

func (git *reconciliationAlignmentNoInspectorGit) HasUnmergedConflicts(
	context.Context,
	port.RepositoryIdentity,
) (bool, error) {
	return git.conflicts, nil
}

func (git *reconciliationAlignmentNoInspectorGit) Merge(
	context.Context,
	port.RepositoryIdentity,
	branch.TargetBase,
	commitmsg.Message,
) error {
	return git.mergeErr
}

type reconciliationAlignmentPublisher struct {
	releaseWhiteboxPublisher
	preflightErr   error
	preflightCalls int
}

func (publisher *reconciliationAlignmentPublisher) Validate(context.Context, port.PullRequestPublication) error {
	publisher.preflightCalls++
	return publisher.preflightErr
}

func newReconciliationAlignmentGit(t *testing.T) *reconciliationAlignmentGit {
	t.Helper()

	worker := mustBranch("chore/GOV-20-align-release-reconciliation-base")
	release := mustBranch("release/1.0.1")
	releaseBase := mustBase("origin", release.String())
	base := newReleaseWhiteboxGit()
	base.clean = true
	base.missing = true
	base.publication = branch.PublicationUnpublished
	base.workflowBases = map[string]branch.TargetBase{worker.String(): releaseBase}

	return &reconciliationAlignmentGit{
		releaseWhiteboxGit: base,
		current:            worker,
		targetExists:       true,
		headMatches:        true,
		releaseSHA:         strings.Repeat("r", 40),
		developSHA:         strings.Repeat("d", 40),
	}
}

func newReconciliationAlignmentNoInspectorGit(t *testing.T) *reconciliationAlignmentNoInspectorGit {
	t.Helper()

	worker := mustBranch("chore/GOV-20-align-release-reconciliation-base")
	release := mustBranch("release/1.0.1")
	releaseBase := mustBase("origin", release.String())
	base := newReleaseWhiteboxGit()
	base.clean = true
	base.missing = true
	base.publication = branch.PublicationUnpublished
	base.workflowBases = map[string]branch.TargetBase{worker.String(): releaseBase}

	return &reconciliationAlignmentNoInspectorGit{
		releaseWhiteboxGit: base,
		current:            worker,
	}
}

func newReconciliationAlignmentService(
	git port.GitRepository,
	quality port.QualityRunner,
	publisher port.PullRequestPublisher,
	lifecycle port.ReleaseLifecycleProvider,
) *ReleaseService {
	branches := branchapp.NewService(git, &fakeKeyPolicy{})
	tickets := NewTicketService(branches, branchapp.NewSynchronizer(git, branches, quality), git, quality, publisher)
	return NewReleaseService(branches, git, publisher).
		WithTicketService(tickets).
		WithQualityRunner(quality).
		WithReleaseLifecycleProvider(lifecycle)
}

func reconciliationAlignmentRequest() AlignReleaseReconciliationBaseRequest {
	return AlignReleaseReconciliationBaseRequest{
		Repository: testRepository(),
		Release:    mustBranch("release/1.0.1"),
		Branch:     mustBranch("chore/GOV-20-align-release-reconciliation-base"),
	}
}

func requiredReconciliationEvidence() port.ReleaseReconciliationEvidence {
	return port.ReleaseReconciliationEvidence{
		PromotionPullRequestURL: "https://example.invalid/pr/30",
		PromotionMergeCommit:    strings.Repeat("a", 40),
		Tag:                     "v1.0.1",
		ReleaseURL:              "https://example.invalid/releases/v1.0.1",
		EffectiveDelta:          true,
	}
}

func TestReleaseReconciliationBaseAlignment(t *testing.T) {
	t.Run("plans a dry run without provider verification merge quality or publication", func(t *testing.T) {
		git := newReconciliationAlignmentGit(t)
		quality := &fakeQualityRunner{}
		lifecycle := &releaseWhiteboxLifecycle{evidence: requiredReconciliationEvidence()}

		request := reconciliationAlignmentRequest()
		request.DryRun = true
		result, err := newReconciliationAlignmentService(git, quality, nil, lifecycle).
			AlignReleaseReconciliationBase(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if !result.DryRun || !result.MissingDevelopCommits || result.Merged || result.Pushed ||
			result.PullRequest.Source.String() != request.Branch.String() ||
			result.PullRequest.Target.String() != "develop" {
			t.Fatalf("dry-run result = %#v", result)
		}
		if quality.calls != 0 || len(lifecycle.reconciles) != 0 ||
			strings.Contains(strings.Join(git.calls, ","), "fetch") ||
			strings.Contains(strings.Join(git.calls, ","), "merge") ||
			strings.Contains(strings.Join(git.calls, ","), "push") {
			t.Fatalf("dry-run mutated workflow state: calls=%v quality=%d reconciles=%d",
				git.calls, quality.calls, len(lifecycle.reconciles))
		}
	})

	t.Run("merges current develop, validates quality, and publishes the reconciliation PR", func(t *testing.T) {
		git := newReconciliationAlignmentGit(t)
		quality := &fakeQualityRunner{}
		publisher := &reconciliationAlignmentPublisher{
			releaseWhiteboxPublisher: releaseWhiteboxPublisher{
				result: port.PublishedPullRequest{URL: "https://example.invalid/pr/reconciliation"},
			},
		}
		lifecycle := &releaseWhiteboxLifecycle{evidence: requiredReconciliationEvidence()}
		request := reconciliationAlignmentRequest()
		request.Push = true
		request.CreatePullRequest = true
		request.Draft = true

		result, err := newReconciliationAlignmentService(git, quality, publisher, lifecycle).
			AlignReleaseReconciliationBase(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if !result.MissingDevelopCommits || !result.Merged || !result.Pushed ||
			result.PublishedURL != "https://example.invalid/pr/reconciliation" || result.Quality == nil ||
			result.Quality.Status != port.QualityPassed || !result.PullRequest.Draft ||
			!result.Evidence.EffectiveDelta {
			t.Fatalf("alignment result = %#v", result)
		}
		if git.mergedBase.String() != "origin/develop" ||
			git.mergedMessage.String() != "chore(GOV-20): align release 1.0.1 with develop for reconciliation" {
			t.Fatalf("merge = (%q, %q)", git.mergedBase, git.mergedMessage)
		}
		if quality.calls != 1 || len(git.pushed) != 1 || git.pushed[0] != request.Branch ||
			publisher.preflightCalls != 1 || len(publisher.requests) != 1 ||
			publisher.requests[0].Target.String() != "develop" || len(lifecycle.reconciles) != 1 {
			t.Fatalf("quality=%d pushed=%v preflight=%d publications=%#v reconciles=%#v",
				quality.calls, git.pushed, publisher.preflightCalls, publisher.requests, lifecycle.reconciles)
		}
	})

	t.Run("returns a validated unpublished alignment after merging develop", func(t *testing.T) {
		git := newReconciliationAlignmentGit(t)
		quality := &fakeQualityRunner{}
		lifecycle := &releaseWhiteboxLifecycle{evidence: requiredReconciliationEvidence()}

		result, err := newReconciliationAlignmentService(git, quality, nil, lifecycle).
			AlignReleaseReconciliationBase(context.Background(), reconciliationAlignmentRequest())
		if err != nil || !result.Merged || result.Pushed || result.Quality == nil || quality.calls != 1 {
			t.Fatalf("unpublished alignment = (%#v, %v), quality=%d", result, err, quality.calls)
		}
	})

	t.Run("returns a safe no-op when develop is already an ancestor", func(t *testing.T) {
		git := newReconciliationAlignmentGit(t)
		git.missing = false
		quality := &fakeQualityRunner{}
		lifecycle := &releaseWhiteboxLifecycle{evidence: requiredReconciliationEvidence()}

		result, err := newReconciliationAlignmentService(git, quality, nil, lifecycle).
			AlignReleaseReconciliationBase(context.Background(), reconciliationAlignmentRequest())
		if err != nil {
			t.Fatal(err)
		}
		if result.MissingDevelopCommits || result.Merged || result.Quality != nil || quality.calls != 0 ||
			len(lifecycle.reconciles) != 1 {
			t.Fatalf("no-op result = %#v quality=%d reconciles=%d", result, quality.calls, len(lifecycle.reconciles))
		}
	})

	t.Run("runs quality before an explicit push of an already aligned branch", func(t *testing.T) {
		git := newReconciliationAlignmentGit(t)
		git.missing = false
		quality := &fakeQualityRunner{}
		lifecycle := &releaseWhiteboxLifecycle{evidence: requiredReconciliationEvidence()}
		request := reconciliationAlignmentRequest()
		request.Push = true

		result, err := newReconciliationAlignmentService(git, quality, nil, lifecycle).
			AlignReleaseReconciliationBase(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if result.Merged || !result.Pushed || result.Quality == nil || quality.calls != 1 ||
			len(git.pushed) != 1 {
			t.Fatalf("already-aligned push = %#v quality=%d pushed=%v", result, quality.calls, git.pushed)
		}
	})
}

func TestReleaseReconciliationBaseAlignmentRecovery(t *testing.T) {
	newService := func(
		git port.GitRepository,
		quality port.QualityRunner,
	) *ReleaseService {
		return newReconciliationAlignmentService(
			git,
			quality,
			nil,
			&releaseWhiteboxLifecycle{evidence: requiredReconciliationEvidence()},
		)
	}

	t.Run("continues a staged merge and reruns quality on the resolved candidate", func(t *testing.T) {
		git := newReconciliationAlignmentGit(t)
		git.active = true
		git.activeOperation = "merge"
		request := reconciliationAlignmentRequest()
		request.Resume = true
		quality := &fakeQualityRunner{}

		result, err := newService(git, quality).AlignReleaseReconciliationBase(context.Background(), request)
		if err != nil || !result.Resumed || !result.Merged || !git.continued ||
			result.MissingDevelopCommits || result.Quality == nil || quality.calls != 1 {
			t.Fatalf("resume result = (%#v, %v), continued=%t quality=%d", result, err, git.continued, quality.calls)
		}
	})

	t.Run("accepts a provenance-validated prepared candidate without workflow metadata", func(t *testing.T) {
		git := newReconciliationAlignmentGit(t)
		git.missing = false
		git.workflowBases = nil
		request := reconciliationAlignmentRequest()
		request.Prepared = true
		quality := &fakeQualityRunner{}

		result, err := newService(git, quality).AlignReleaseReconciliationBase(context.Background(), request)
		if err != nil || !result.Prepared || !result.Merged || result.Quality == nil ||
			quality.calls != 1 || !strings.Contains(strings.Join(git.calls, ","), "head-is-reconciliation-merge") {
			t.Fatalf("prepared result = (%#v, %v), calls=%v quality=%d", result, err, git.calls, quality.calls)
		}
	})

	t.Run("rejects unsafe recovery states", func(t *testing.T) {
		testCases := []struct {
			name      string
			configure func(*reconciliationAlignmentGit, *AlignReleaseReconciliationBaseRequest)
			rawError  bool
		}{
			{
				name: "resume and prepared",
				configure: func(_ *reconciliationAlignmentGit, request *AlignReleaseReconciliationBaseRequest) {
					request.Resume = true
					request.Prepared = true
				},
			},
			{
				name: "inactive resume",
				configure: func(_ *reconciliationAlignmentGit, request *AlignReleaseReconciliationBaseRequest) {
					request.Resume = true
				},
			},
			{
				name: "resume operation lookup fails",
				configure: func(git *reconciliationAlignmentGit, request *AlignReleaseReconciliationBaseRequest) {
					request.Resume = true
					git.activeErr = errors.New("active operation")
				},
				rawError: true,
			},
			{
				name: "unresolved resume conflicts",
				configure: func(git *reconciliationAlignmentGit, request *AlignReleaseReconciliationBaseRequest) {
					request.Resume = true
					git.active = true
					git.activeOperation = "merge"
					git.conflicts = true
				},
			},
			{
				name: "resume conflict inspection fails",
				configure: func(git *reconciliationAlignmentGit, request *AlignReleaseReconciliationBaseRequest) {
					request.Resume = true
					git.active = true
					git.activeOperation = "merge"
					git.conflictsErr = errors.New("inspect conflicts")
				},
				rawError: true,
			},
			{
				name: "continue failure",
				configure: func(git *reconciliationAlignmentGit, request *AlignReleaseReconciliationBaseRequest) {
					request.Resume = true
					git.active = true
					git.activeOperation = "merge"
					git.continueErr = errors.New("continue")
				},
				rawError: true,
			},
			{
				name: "develop advanced during resume",
				configure: func(git *reconciliationAlignmentGit, request *AlignReleaseReconciliationBaseRequest) {
					request.Resume = true
					git.active = true
					git.activeOperation = "merge"
					git.preserveMissing = true
				},
			},
			{
				name: "prepared candidate omits current develop",
				configure: func(git *reconciliationAlignmentGit, request *AlignReleaseReconciliationBaseRequest) {
					request.Prepared = true
				},
			},
			{
				name: "prepared candidate provenance lookup fails",
				configure: func(git *reconciliationAlignmentGit, request *AlignReleaseReconciliationBaseRequest) {
					request.Prepared = true
					git.missing = false
					git.workflowBases = nil
					git.headErr = errors.New("provenance")
				},
				rawError: true,
			},
			{
				name: "prepared candidate provenance mismatches",
				configure: func(git *reconciliationAlignmentGit, request *AlignReleaseReconciliationBaseRequest) {
					request.Prepared = true
					git.missing = false
					git.workflowBases = nil
					git.headMatches = false
				},
			},
		}
		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				git := newReconciliationAlignmentGit(t)
				request := reconciliationAlignmentRequest()
				testCase.configure(git, &request)

				_, err := newService(git, &fakeQualityRunner{}).AlignReleaseReconciliationBase(context.Background(), request)
				if testCase.rawError {
					if err == nil {
						t.Fatal("expected recovery error")
					}
					return
				}
				assertProblemCode(t, err, problem.CodeInvalidInput)
			})
		}
	})

	t.Run("rejects recovery when required optional inspectors are unavailable", func(t *testing.T) {
		t.Run("resume continuator", func(t *testing.T) {
			git := newReconciliationAlignmentNoInspectorGit(t)
			git.active = true
			git.activeOperation = "merge"
			request := reconciliationAlignmentRequest()
			request.Resume = true

			_, err := newService(git, &fakeQualityRunner{}).AlignReleaseReconciliationBase(context.Background(), request)
			assertProblemCode(t, err, problem.CodeInternal)
		})

		t.Run("prepared provenance", func(t *testing.T) {
			git := newReconciliationAlignmentNoInspectorGit(t)
			git.missing = false
			git.workflowBases = nil
			request := reconciliationAlignmentRequest()
			request.Prepared = true

			_, err := newService(git, &fakeQualityRunner{}).AlignReleaseReconciliationBase(context.Background(), request)
			assertProblemCode(t, err, problem.CodeInternal)
		})

		t.Run("conflict provenance", func(t *testing.T) {
			git := newReconciliationAlignmentNoInspectorGit(t)
			git.conflicts = true
			git.mergeErr = errors.New("merge")

			_, err := newService(git, &fakeQualityRunner{}).AlignReleaseReconciliationBase(context.Background(), reconciliationAlignmentRequest())
			assertProblemCode(t, err, problem.CodeInternal)
		})
	})

	t.Run("reports a fail-closed conflict manifest", func(t *testing.T) {
		git := newReconciliationAlignmentGit(t)
		git.mergeErr = problem.New(problem.Details{
			Code:       problem.CodeGitCommandFailed,
			Category:   problem.CategoryGit,
			Context:    "merge-context",
			Diagnostic: "merge-diagnostic",
		})
		git.conflicts = true

		_, err := newService(git, &fakeQualityRunner{}).AlignReleaseReconciliationBase(context.Background(), reconciliationAlignmentRequest())
		assertProblemCode(t, err, problem.CodeMergeConflict)
		actual, ok := problem.As(err)
		if !ok {
			t.Fatalf("merge conflict problem = %v", err)
		}
		if actual.Context != "merge-context" || actual.Diagnostic != "merge-diagnostic" ||
			!strings.Contains(actual.Actual, "origin/develop@") {
			t.Fatalf("merge conflict problem = %#v", actual)
		}
	})

	t.Run("propagates conflict inspection and provenance failures", func(t *testing.T) {
		for _, configure := range []func(*reconciliationAlignmentGit){
			func(git *reconciliationAlignmentGit) {
				git.mergeErr = errors.New("merge")
				git.conflictsErr = errors.New("inspect conflicts")
			},
			func(git *reconciliationAlignmentGit) {
				git.mergeErr = errors.New("merge")
				git.conflicts = true
				git.resolveErr = errors.New("resolve bases")
			},
		} {
			git := newReconciliationAlignmentGit(t)
			configure(git)
			if _, err := newService(git, &fakeQualityRunner{}).
				AlignReleaseReconciliationBase(context.Background(), reconciliationAlignmentRequest()); err == nil {
				t.Fatal("expected conflict recovery failure")
			}
		}
	})
}

func TestReleaseReconciliationBaseAlignmentRejectsUnsafeInputsBeforeMerge(t *testing.T) {
	testCases := []struct {
		name      string
		configure func(*reconciliationAlignmentGit, *AlignReleaseReconciliationBaseRequest, *ReleaseService)
		code      problem.Code
	}{
		{
			name: "missing dependencies",
			configure: func(_ *reconciliationAlignmentGit, request *AlignReleaseReconciliationBaseRequest, service *ReleaseService) {
				*service = ReleaseService{}
				_ = request
			},
			code: problem.CodeInternal,
		},
		{
			name: "non-release line",
			configure: func(_ *reconciliationAlignmentGit, request *AlignReleaseReconciliationBaseRequest, _ *ReleaseService) {
				request.Release = mustBranch("main")
			},
			code: problem.CodeInvalidInput,
		},
		{
			name: "non-chore worker",
			configure: func(_ *reconciliationAlignmentGit, request *AlignReleaseReconciliationBaseRequest, _ *ReleaseService) {
				request.Branch = mustBranch("fix/GOV-20-align-release-reconciliation-base")
			},
			code: problem.CodeInvalidInput,
		},
		{
			name: "pull request without push",
			configure: func(_ *reconciliationAlignmentGit, request *AlignReleaseReconciliationBaseRequest, _ *ReleaseService) {
				request.CreatePullRequest = true
			},
			code: problem.CodeInvalidInput,
		},
		{
			name: "missing repository",
			configure: func(_ *reconciliationAlignmentGit, request *AlignReleaseReconciliationBaseRequest, _ *ReleaseService) {
				request.Repository = port.RepositoryIdentity{}
			},
			code: problem.CodeRepositoryNotFound,
		},
		{
			name: "invalid remote",
			configure: func(_ *reconciliationAlignmentGit, request *AlignReleaseReconciliationBaseRequest, _ *ReleaseService) {
				request.Repository.Remote = "invalid remote"
			},
			code: problem.CodeBranchBaseInvalid,
		},
		{
			name: "invalid worker reference",
			configure: func(git *reconciliationAlignmentGit, _ *AlignReleaseReconciliationBaseRequest, _ *ReleaseService) {
				git.validateErrors = []error{errors.New("validate worker")}
			},
		},
		{
			name: "wrong checked-out branch",
			configure: func(git *reconciliationAlignmentGit, _ *AlignReleaseReconciliationBaseRequest, _ *ReleaseService) {
				git.current = mustBranch("chore/GOV-20-another-release-prep")
			},
			code: problem.CodeInvalidInput,
		},
		{
			name: "current branch lookup failure",
			configure: func(git *reconciliationAlignmentGit, _ *AlignReleaseReconciliationBaseRequest, _ *ReleaseService) {
				git.currentErr = errors.New("read current branch")
			},
		},
		{
			name: "workflow base lookup failure",
			configure: func(git *reconciliationAlignmentGit, _ *AlignReleaseReconciliationBaseRequest, _ *ReleaseService) {
				git.workflowBaseErr = errors.New("read workflow base")
			},
		},
		{
			name: "missing workflow base",
			configure: func(git *reconciliationAlignmentGit, _ *AlignReleaseReconciliationBaseRequest, _ *ReleaseService) {
				git.workflowBases = nil
			},
			code: problem.CodeInvalidInput,
		},
		{
			name: "mismatched workflow base",
			configure: func(git *reconciliationAlignmentGit, request *AlignReleaseReconciliationBaseRequest, _ *ReleaseService) {
				git.workflowBases[request.Branch.String()] = mustBase("origin", "release/1.0.0")
			},
			code: problem.CodeInvalidInput,
		},
		{
			name: "missing develop target",
			configure: func(git *reconciliationAlignmentGit, _ *AlignReleaseReconciliationBaseRequest, _ *ReleaseService) {
				git.targetExists = false
			},
			code: problem.CodeInvalidInput,
		},
		{
			name: "develop target lookup failure",
			configure: func(git *reconciliationAlignmentGit, _ *AlignReleaseReconciliationBaseRequest, _ *ReleaseService) {
				git.targetErr = errors.New("inspect develop")
			},
		},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			git := newReconciliationAlignmentGit(t)
			quality := &fakeQualityRunner{}
			lifecycle := &releaseWhiteboxLifecycle{evidence: requiredReconciliationEvidence()}
			service := newReconciliationAlignmentService(git, quality, nil, lifecycle)
			request := reconciliationAlignmentRequest()
			testCase.configure(git, &request, service)

			_, err := service.AlignReleaseReconciliationBase(context.Background(), request)
			if testCase.code != "" {
				assertProblemCode(t, err, testCase.code)
			} else if err == nil {
				t.Fatal("expected dependency error")
			}
			if strings.Contains(strings.Join(git.calls, ","), "merge") {
				t.Fatalf("unsafe request merged: calls=%v", git.calls)
			}
		})
	}
}

func TestReleaseReconciliationBaseAlignmentFailureAndCleanupPaths(t *testing.T) {
	t.Run("dry runs reject unavailable and unreadable develop bases", func(t *testing.T) {
		testCases := []struct {
			name      string
			configure func(*reconciliationAlignmentGit)
			code      problem.Code
		}{
			{name: "target lookup", configure: func(git *reconciliationAlignmentGit) { git.targetErr = errors.New("target") }},
			{name: "target absent", configure: func(git *reconciliationAlignmentGit) { git.targetExists = false }, code: problem.CodeInvalidInput},
			{name: "missing-base lookup", configure: func(git *reconciliationAlignmentGit) { git.missingErr = errors.New("missing") }},
		}
		for _, testCase := range testCases {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				git := newReconciliationAlignmentGit(t)
				testCase.configure(git)
				request := reconciliationAlignmentRequest()
				request.DryRun = true
				_, err := newReconciliationAlignmentService(
					git,
					&fakeQualityRunner{},
					nil,
					&releaseWhiteboxLifecycle{evidence: requiredReconciliationEvidence()},
				).AlignReleaseReconciliationBase(context.Background(), request)
				if testCase.code != "" {
					assertProblemCode(t, err, testCase.code)
				} else if err == nil {
					t.Fatal("expected dry-run failure")
				}
			})
		}
	})

	t.Run("fails before mutation when pull request preflight is unavailable", func(t *testing.T) {
		git := newReconciliationAlignmentGit(t)
		request := reconciliationAlignmentRequest()
		request.Push = true
		request.CreatePullRequest = true

		_, err := newReconciliationAlignmentService(
			git,
			&fakeQualityRunner{},
			nil,
			&releaseWhiteboxLifecycle{evidence: requiredReconciliationEvidence()},
		).AlignReleaseReconciliationBase(context.Background(), request)
		assertProblemCode(t, err, problem.CodeExternalCommandFailed)
		if strings.Contains(strings.Join(git.calls, ","), "merge") {
			t.Fatalf("missing publisher merged: %v", git.calls)
		}
	})

	t.Run("fails before mutation when ticket publication composition is absent", func(t *testing.T) {
		git := newReconciliationAlignmentGit(t)
		branches := branchapp.NewService(git, &fakeKeyPolicy{})
		service := NewReleaseService(branches, git, nil).
			WithQualityRunner(&fakeQualityRunner{}).
			WithReleaseLifecycleProvider(&releaseWhiteboxLifecycle{evidence: requiredReconciliationEvidence()})
		request := reconciliationAlignmentRequest()
		request.Push = true
		request.CreatePullRequest = true

		_, err := service.AlignReleaseReconciliationBase(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInternal)
		if strings.Contains(strings.Join(git.calls, ","), "merge") {
			t.Fatalf("missing publication service merged: %v", git.calls)
		}
	})

	t.Run("fails before mutation when the publisher preflight rejects the PR", func(t *testing.T) {
		git := newReconciliationAlignmentGit(t)
		preflightErr := errors.New("publisher preflight")
		publisher := &reconciliationAlignmentPublisher{preflightErr: preflightErr}
		request := reconciliationAlignmentRequest()
		request.Push = true
		request.CreatePullRequest = true

		_, err := newReconciliationAlignmentService(
			git,
			&fakeQualityRunner{},
			publisher,
			&releaseWhiteboxLifecycle{evidence: requiredReconciliationEvidence()},
		).AlignReleaseReconciliationBase(context.Background(), request)
		if !errors.Is(err, preflightErr) {
			t.Fatalf("preflight error = %v, want %v", err, preflightErr)
		}
		if strings.Contains(strings.Join(git.calls, ","), "merge") {
			t.Fatalf("preflight failure merged: %v", git.calls)
		}
	})

	t.Run("propagates worktree fetch missing-base and merge failures", func(t *testing.T) {
		testCases := []struct {
			name      string
			configure func(*reconciliationAlignmentGit)
		}{
			{name: "worktree lookup", configure: func(git *reconciliationAlignmentGit) { git.cleanErr = errors.New("status") }},
			{name: "dirty worktree", configure: func(git *reconciliationAlignmentGit) { git.clean = false }},
			{name: "fetch", configure: func(git *reconciliationAlignmentGit) { git.fetchErrors = []error{errors.New("fetch")} }},
			{name: "missing base", configure: func(git *reconciliationAlignmentGit) { git.missingErr = errors.New("missing base") }},
			{name: "merge", configure: func(git *reconciliationAlignmentGit) { git.mergeErr = errors.New("merge") }},
		}
		for _, testCase := range testCases {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				git := newReconciliationAlignmentGit(t)
				testCase.configure(git)
				_, err := newReconciliationAlignmentService(
					git,
					&fakeQualityRunner{},
					nil,
					&releaseWhiteboxLifecycle{evidence: requiredReconciliationEvidence()},
				).AlignReleaseReconciliationBase(context.Background(), reconciliationAlignmentRequest())
				if err == nil {
					t.Fatal("expected workflow failure")
				}
			})
		}
	})

	t.Run("rejects unverified no-delta reconciliation before merge", func(t *testing.T) {
		git := newReconciliationAlignmentGit(t)
		evidence := requiredReconciliationEvidence()
		evidence.EffectiveDelta = false

		_, err := newReconciliationAlignmentService(
			git,
			&fakeQualityRunner{},
			nil,
			&releaseWhiteboxLifecycle{evidence: evidence},
		).AlignReleaseReconciliationBase(context.Background(), reconciliationAlignmentRequest())
		assertProblemCode(t, err, problem.CodeInvalidInput)
		if strings.Contains(strings.Join(git.calls, ","), "merge") {
			t.Fatalf("no-delta reconciliation merged: %v", git.calls)
		}
	})

	t.Run("preserves lifecycle validation quality publication and provider failures", func(t *testing.T) {
		t.Run("lifecycle", func(t *testing.T) {
			lifecycleErr := errors.New("verify release delivery")
			_, err := newReconciliationAlignmentService(
				newReconciliationAlignmentGit(t),
				&fakeQualityRunner{},
				nil,
				&releaseWhiteboxLifecycle{verifyErr: lifecycleErr},
			).AlignReleaseReconciliationBase(context.Background(), reconciliationAlignmentRequest())
			if !errors.Is(err, lifecycleErr) {
				t.Fatalf("lifecycle error = %v, want %v", err, lifecycleErr)
			}
		})

		t.Run("post-merge validation", func(t *testing.T) {
			git := newReconciliationAlignmentGit(t)
			git.validateErrors = []error{nil, errors.New("revalidate")}
			_, err := newReconciliationAlignmentService(
				git,
				&fakeQualityRunner{},
				nil,
				&releaseWhiteboxLifecycle{evidence: requiredReconciliationEvidence()},
			).AlignReleaseReconciliationBase(context.Background(), reconciliationAlignmentRequest())
			if err == nil {
				t.Fatal("expected revalidation failure")
			}
		})

		t.Run("quality", func(t *testing.T) {
			qualityErr := errors.New("quality")
			_, err := newReconciliationAlignmentService(
				newReconciliationAlignmentGit(t),
				&fakeQualityRunner{err: qualityErr},
				nil,
				&releaseWhiteboxLifecycle{evidence: requiredReconciliationEvidence()},
			).AlignReleaseReconciliationBase(context.Background(), reconciliationAlignmentRequest())
			if !errors.Is(err, qualityErr) {
				t.Fatalf("quality error = %v, want %v", err, qualityErr)
			}
		})

		t.Run("missing quality runner", func(t *testing.T) {
			git := newReconciliationAlignmentGit(t)
			_, err := newReconciliationAlignmentService(
				git,
				nil,
				nil,
				&releaseWhiteboxLifecycle{evidence: requiredReconciliationEvidence()},
			).AlignReleaseReconciliationBase(context.Background(), reconciliationAlignmentRequest())
			assertProblemCode(t, err, problem.CodeInternal)
		})

		t.Run("publication lookup", func(t *testing.T) {
			git := newReconciliationAlignmentGit(t)
			git.publicationErr = errors.New("publication")
			request := reconciliationAlignmentRequest()
			request.Push = true
			_, err := newReconciliationAlignmentService(
				git,
				&fakeQualityRunner{},
				nil,
				&releaseWhiteboxLifecycle{evidence: requiredReconciliationEvidence()},
			).AlignReleaseReconciliationBase(context.Background(), request)
			if err == nil {
				t.Fatal("expected publication failure")
			}
		})

		t.Run("unknown publication", func(t *testing.T) {
			git := newReconciliationAlignmentGit(t)
			git.publication = branch.PublicationUnknown
			request := reconciliationAlignmentRequest()
			request.Push = true
			_, err := newReconciliationAlignmentService(
				git,
				&fakeQualityRunner{},
				nil,
				&releaseWhiteboxLifecycle{evidence: requiredReconciliationEvidence()},
			).AlignReleaseReconciliationBase(context.Background(), request)
			assertProblemCode(t, err, problem.CodeInvalidInput)
		})

		t.Run("push", func(t *testing.T) {
			git := newReconciliationAlignmentGit(t)
			git.pushErr = errors.New("push")
			request := reconciliationAlignmentRequest()
			request.Push = true
			_, err := newReconciliationAlignmentService(
				git,
				&fakeQualityRunner{},
				nil,
				&releaseWhiteboxLifecycle{evidence: requiredReconciliationEvidence()},
			).AlignReleaseReconciliationBase(context.Background(), request)
			if err == nil {
				t.Fatal("expected push failure")
			}
		})

		t.Run("publisher", func(t *testing.T) {
			publishErr := errors.New("publish")
			publisher := &reconciliationAlignmentPublisher{
				releaseWhiteboxPublisher: releaseWhiteboxPublisher{err: publishErr},
			}
			request := reconciliationAlignmentRequest()
			request.Push = true
			request.CreatePullRequest = true
			_, err := newReconciliationAlignmentService(
				newReconciliationAlignmentGit(t),
				&fakeQualityRunner{},
				publisher,
				&releaseWhiteboxLifecycle{evidence: requiredReconciliationEvidence()},
			).AlignReleaseReconciliationBase(context.Background(), request)
			if !errors.Is(err, publishErr) {
				t.Fatalf("publisher error = %v, want %v", err, publishErr)
			}
		})
	})
}

func TestReconciliationBaseAlignmentHelpers(t *testing.T) {
	worker := mustBranch("chore/GOV-20-align-release-reconciliation-base")
	release := mustBranch("release/1.0.1")
	message := reconciliationBaseAlignmentMergeMessage(worker, release)
	if message.String() != "chore(GOV-20): align release 1.0.1 with develop for reconciliation" {
		t.Fatalf("merge message = %q", message)
	}
	pullRequest := reconciliationBaseAlignmentPullRequest(worker, mustBranch("develop"), true)
	if pullRequest.Source != worker || pullRequest.Target.String() != "develop" || pullRequest.Ticket.String() != "GOV-20" ||
		pullRequest.Title != "GOV-20: align-release-reconciliation-base" || !pullRequest.Draft {
		t.Fatalf("pull request = %#v", pullRequest)
	}
}
