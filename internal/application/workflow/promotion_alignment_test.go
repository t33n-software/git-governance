package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

type promotionAlignmentGit struct {
	*releaseWhiteboxGit

	current        branch.BranchName
	currentErr     error
	targetExists   bool
	targetErr      error
	mergeErr       error
	validateErrors []error
	mergedBase     branch.TargetBase
	mergedMessage  commitmsg.Message

	resumeConflicts            bool
	resumeConflictsErr         error
	resumeTargetMatches        bool
	resumeTargetErr            error
	resumeContinueErr          error
	resumeMissingAfterContinue bool
	resumeContinued            bool
}

func (git *promotionAlignmentGit) CurrentBranch(context.Context, port.RepositoryIdentity) (branch.BranchName, error) {
	git.calls = append(git.calls, "current-branch")
	if git.currentErr != nil {
		return branch.BranchName{}, git.currentErr
	}
	return git.current, nil
}

func (git *promotionAlignmentGit) ValidateBranchRef(context.Context, port.RepositoryIdentity, branch.BranchName) error {
	git.calls = append(git.calls, "validate-ref")
	if len(git.validateErrors) > 0 {
		err := git.validateErrors[0]
		git.validateErrors = git.validateErrors[1:]
		return err
	}
	return git.validateErr
}

func (git *promotionAlignmentGit) TargetBaseExists(
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

func (git *promotionAlignmentGit) Merge(
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

func (git *promotionAlignmentGit) HasUnmergedConflicts(context.Context, port.RepositoryIdentity) (bool, error) {
	git.calls = append(git.calls, "unmerged-conflicts")
	if git.resumeConflictsErr != nil {
		return false, git.resumeConflictsErr
	}
	return git.resumeConflicts, nil
}

func (git *promotionAlignmentGit) ActiveMergeTargetMatches(
	context.Context,
	port.RepositoryIdentity,
	branch.TargetBase,
) (bool, error) {
	git.calls = append(git.calls, "active-merge-target")
	if git.resumeTargetErr != nil {
		return false, git.resumeTargetErr
	}
	return git.resumeTargetMatches, nil
}

func (git *promotionAlignmentGit) ContinueMerge(context.Context, port.RepositoryIdentity) error {
	git.calls = append(git.calls, "continue-merge")
	if git.resumeContinueErr != nil {
		return git.resumeContinueErr
	}
	git.active = false
	git.activeOperation = ""
	git.missing = git.resumeMissingAfterContinue
	git.resumeContinued = true
	return nil
}

type promotionAlignmentPublisher struct {
	releaseWhiteboxPublisher
	preflightErr   error
	preflightCalls int
}

type promotionAlignmentMinimalGit struct {
	*releaseWhiteboxGit

	current branch.BranchName
}

func (git *promotionAlignmentMinimalGit) CurrentBranch(context.Context, port.RepositoryIdentity) (branch.BranchName, error) {
	git.calls = append(git.calls, "current-branch")
	return git.current, nil
}

type promotionAlignmentMissingContinuatorGit struct {
	*promotionAlignmentMinimalGit
}

func (git *promotionAlignmentMissingContinuatorGit) ActiveMergeTargetMatches(
	context.Context,
	port.RepositoryIdentity,
	branch.TargetBase,
) (bool, error) {
	git.calls = append(git.calls, "active-merge-target")
	return true, nil
}

func (publisher *promotionAlignmentPublisher) Validate(context.Context, port.PullRequestPublication) error {
	publisher.preflightCalls++
	return publisher.preflightErr
}

func newPromotionAlignmentGit(t *testing.T) *promotionAlignmentGit {
	t.Helper()

	worker := mustBranch("chore/GOV-18-align-promotion-base")
	release := mustBranch("release/1.0.1")
	releaseBase := mustBase("origin", release.String())
	base := newReleaseWhiteboxGit()
	base.clean = true
	base.missing = true
	base.publication = branch.PublicationUnpublished
	base.workflowBases = map[string]branch.TargetBase{worker.String(): releaseBase}

	return &promotionAlignmentGit{
		releaseWhiteboxGit:  base,
		current:             worker,
		targetExists:        true,
		resumeTargetMatches: true,
	}
}

func newPromotionAlignmentService(
	git port.GitRepository,
	quality port.QualityRunner,
	publisher port.PullRequestPublisher,
) *ReleaseService {
	branches := branchapp.NewService(git, &fakeKeyPolicy{})
	tickets := NewTicketService(branches, branchapp.NewSynchronizer(git, branches, quality), git, quality, publisher)
	return NewReleaseService(branches, git, publisher).
		WithTicketService(tickets).
		WithQualityRunner(quality)
}

func newPromotionAlignmentMinimalGit(t *testing.T) *promotionAlignmentMinimalGit {
	t.Helper()

	worker := mustBranch("chore/GOV-18-align-promotion-base")
	release := mustBranch("release/1.0.1")
	releaseBase := mustBase("origin", release.String())
	base := newReleaseWhiteboxGit()
	base.active = true
	base.activeOperation = "merge"
	base.workflowBases = map[string]branch.TargetBase{worker.String(): releaseBase}
	return &promotionAlignmentMinimalGit{
		releaseWhiteboxGit: base,
		current:            worker,
	}
}

func promotionAlignmentRequest() AlignReleasePromotionBaseRequest {
	return AlignReleasePromotionBaseRequest{
		Repository: testRepository(),
		Release:    mustBranch("release/1.0.1"),
		Branch:     mustBranch("chore/GOV-18-align-promotion-base"),
	}
}

func TestReleasePromotionBaseAlignment(t *testing.T) {
	t.Run("plans a dry run without fetch merge quality or publication", func(t *testing.T) {
		git := newPromotionAlignmentGit(t)
		quality := &fakeQualityRunner{}

		request := promotionAlignmentRequest()
		request.DryRun = true
		result, err := newPromotionAlignmentService(git, quality, nil).AlignReleasePromotionBase(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if !result.DryRun || !result.MissingMainCommits || result.Merged || result.Pushed ||
			result.PullRequest.Source.String() != request.Branch.String() ||
			result.PullRequest.Target.String() != request.Release.String() {
			t.Fatalf("dry-run result = %#v", result)
		}
		if quality.calls != 0 || strings.Contains(strings.Join(git.calls, ","), "fetch") ||
			strings.Contains(strings.Join(git.calls, ","), "merge") ||
			strings.Contains(strings.Join(git.calls, ","), "push") {
			t.Fatalf("dry-run mutated workflow state: calls=%v quality=%d", git.calls, quality.calls)
		}
	})

	t.Run("merges the current main line, validates quality, and publishes its release PR", func(t *testing.T) {
		git := newPromotionAlignmentGit(t)
		quality := &fakeQualityRunner{}
		publisher := &promotionAlignmentPublisher{
			releaseWhiteboxPublisher: releaseWhiteboxPublisher{
				result: port.PublishedPullRequest{URL: "https://example.invalid/pr/alignment"},
			},
		}
		request := promotionAlignmentRequest()
		request.Push = true
		request.CreatePullRequest = true
		request.Draft = true

		result, err := newPromotionAlignmentService(git, quality, publisher).AlignReleasePromotionBase(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if !result.MissingMainCommits || !result.Merged || !result.Pushed ||
			result.PublishedURL != "https://example.invalid/pr/alignment" || result.Quality == nil ||
			result.Quality.Status != port.QualityPassed || !result.PullRequest.Draft {
			t.Fatalf("alignment result = %#v", result)
		}
		if git.mergedBase.String() != "origin/main" ||
			git.mergedMessage.String() != "chore(GOV-18): align release 1.0.1 with main for promotion" {
			t.Fatalf("merge = (%q, %q)", git.mergedBase, git.mergedMessage)
		}
		if quality.calls != 1 || len(git.pushed) != 1 || git.pushed[0] != request.Branch ||
			publisher.preflightCalls != 1 || len(publisher.requests) != 1 ||
			publisher.requests[0].Target != request.Release {
			t.Fatalf("quality=%d pushed=%v preflight=%d publications=%#v",
				quality.calls, git.pushed, publisher.preflightCalls, publisher.requests)
		}
	})

	t.Run("returns a safe no-op when main is already an ancestor", func(t *testing.T) {
		git := newPromotionAlignmentGit(t)
		git.missing = false
		quality := &fakeQualityRunner{}

		result, err := newPromotionAlignmentService(git, quality, nil).AlignReleasePromotionBase(
			context.Background(),
			promotionAlignmentRequest(),
		)
		if err != nil {
			t.Fatal(err)
		}
		if result.MissingMainCommits || result.Merged || result.Quality != nil || quality.calls != 0 {
			t.Fatalf("no-op result = %#v, quality=%d", result, quality.calls)
		}
	})

	t.Run("runs quality before an explicit push of an already aligned branch", func(t *testing.T) {
		git := newPromotionAlignmentGit(t)
		git.missing = false
		quality := &fakeQualityRunner{}
		request := promotionAlignmentRequest()
		request.Push = true

		result, err := newPromotionAlignmentService(git, quality, nil).AlignReleasePromotionBase(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if result.Merged || !result.Pushed || result.Quality == nil || quality.calls != 1 ||
			len(git.pushed) != 1 {
			t.Fatalf("already-aligned push = %#v, quality=%d, pushed=%v", result, quality.calls, git.pushed)
		}
	})
}

func TestReleasePromotionBaseAlignmentResume(t *testing.T) {
	t.Run("continues the exact resolved merge before quality and publication", func(t *testing.T) {
		git := newPromotionAlignmentGit(t)
		git.active = true
		git.activeOperation = "merge"
		quality := &fakeQualityRunner{}
		publisher := &promotionAlignmentPublisher{
			releaseWhiteboxPublisher: releaseWhiteboxPublisher{
				result: port.PublishedPullRequest{URL: "https://example.invalid/pr/resumed-alignment"},
			},
		}
		request := promotionAlignmentRequest()
		request.Resume = true
		request.Push = true
		request.CreatePullRequest = true

		result, err := newPromotionAlignmentService(git, quality, publisher).AlignReleasePromotionBase(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if !result.Merged || !result.Resumed || result.MissingMainCommits || !result.Pushed ||
			result.PublishedURL != "https://example.invalid/pr/resumed-alignment" || result.Quality == nil {
			t.Fatalf("resume result = %#v", result)
		}
		if !git.resumeContinued || quality.calls != 1 || len(git.pushed) != 1 ||
			publisher.preflightCalls != 1 || len(publisher.requests) != 1 {
			t.Fatalf("continued=%t quality=%d pushed=%v preflight=%d publications=%#v",
				git.resumeContinued, quality.calls, git.pushed, publisher.preflightCalls, publisher.requests)
		}
	})

	t.Run("rejects a dry-run resume", func(t *testing.T) {
		request := promotionAlignmentRequest()
		request.Resume = true
		request.DryRun = true

		_, err := newPromotionAlignmentService(newPromotionAlignmentGit(t), &fakeQualityRunner{}, nil).
			AlignReleasePromotionBase(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("rejects an inactive, wrong, or unresolved merge before continuation", func(t *testing.T) {
		testCases := []struct {
			name      string
			configure func(*promotionAlignmentGit)
		}{
			{name: "inactive", configure: func(_ *promotionAlignmentGit) {}},
			{name: "wrong operation", configure: func(git *promotionAlignmentGit) {
				git.active = true
				git.activeOperation = "rebase"
			}},
			{name: "unresolved conflicts", configure: func(git *promotionAlignmentGit) {
				git.active = true
				git.activeOperation = "merge"
				git.resumeConflicts = true
			}},
		}
		for _, testCase := range testCases {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				git := newPromotionAlignmentGit(t)
				testCase.configure(git)
				request := promotionAlignmentRequest()
				request.Resume = true

				_, err := newPromotionAlignmentService(git, &fakeQualityRunner{}, nil).
					AlignReleasePromotionBase(context.Background(), request)
				assertProblemCode(t, err, problem.CodeInvalidInput)
				if git.resumeContinued {
					t.Fatal("resume continued an unsafe merge")
				}
			})
		}
	})

	t.Run("fails closed when the merge target does not match current main", func(t *testing.T) {
		git := newPromotionAlignmentGit(t)
		git.active = true
		git.activeOperation = "merge"
		git.resumeTargetMatches = false
		request := promotionAlignmentRequest()
		request.Resume = true

		_, err := newPromotionAlignmentService(git, &fakeQualityRunner{}, nil).
			AlignReleasePromotionBase(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInvalidInput)
		if git.resumeContinued {
			t.Fatal("resume continued a stale main merge")
		}
	})

	t.Run("fails closed when main advances after continuation", func(t *testing.T) {
		git := newPromotionAlignmentGit(t)
		git.active = true
		git.activeOperation = "merge"
		git.resumeMissingAfterContinue = true
		quality := &fakeQualityRunner{}
		request := promotionAlignmentRequest()
		request.Resume = true

		_, err := newPromotionAlignmentService(git, quality, nil).
			AlignReleasePromotionBase(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInvalidInput)
		if !git.resumeContinued || quality.calls != 0 {
			t.Fatalf("continued=%t quality=%d", git.resumeContinued, quality.calls)
		}
	})

	t.Run("returns a validated resumed candidate before optional publication", func(t *testing.T) {
		git := newPromotionAlignmentGit(t)
		git.active = true
		git.activeOperation = "merge"
		quality := &fakeQualityRunner{}
		request := promotionAlignmentRequest()
		request.Resume = true

		result, err := newPromotionAlignmentService(git, quality, nil).
			AlignReleasePromotionBase(context.Background(), request)
		if err != nil || !result.Resumed || result.Pushed || quality.calls != 1 {
			t.Fatalf("unpublished resume = (%#v, %v), quality=%d", result, err, quality.calls)
		}

		git = newPromotionAlignmentGit(t)
		git.active = true
		git.activeOperation = "merge"
		request.Push = true
		result, err = newPromotionAlignmentService(git, &fakeQualityRunner{}, nil).
			AlignReleasePromotionBase(context.Background(), request)
		if err != nil || !result.Pushed || result.PublishedURL != "" {
			t.Fatalf("pushed resume = (%#v, %v)", result, err)
		}
	})

	t.Run("fails before continuation when dependencies or merge checks fail", func(t *testing.T) {
		resumeRequest := func() AlignReleasePromotionBaseRequest {
			request := promotionAlignmentRequest()
			request.Resume = true
			return request
		}

		t.Run("missing quality runner", func(t *testing.T) {
			git := newPromotionAlignmentGit(t)
			git.active = true
			git.activeOperation = "merge"
			_, err := newPromotionAlignmentService(git, nil, nil).
				AlignReleasePromotionBase(context.Background(), resumeRequest())
			assertProblemCode(t, err, problem.CodeInternal)
		})

		t.Run("missing publisher composition", func(t *testing.T) {
			git := newPromotionAlignmentGit(t)
			git.active = true
			git.activeOperation = "merge"
			branches := branchapp.NewService(git, &fakeKeyPolicy{})
			service := NewReleaseService(branches, git, nil).WithQualityRunner(&fakeQualityRunner{})
			request := resumeRequest()
			request.Push = true
			request.CreatePullRequest = true
			_, err := service.AlignReleasePromotionBase(context.Background(), request)
			assertProblemCode(t, err, problem.CodeInternal)
		})

		t.Run("publisher preflight", func(t *testing.T) {
			git := newPromotionAlignmentGit(t)
			git.active = true
			git.activeOperation = "merge"
			publisher := &promotionAlignmentPublisher{preflightErr: errors.New("preflight")}
			request := resumeRequest()
			request.Push = true
			request.CreatePullRequest = true
			_, err := newPromotionAlignmentService(git, &fakeQualityRunner{}, publisher).
				AlignReleasePromotionBase(context.Background(), request)
			if err == nil || git.resumeContinued {
				t.Fatalf("preflight err=%v continued=%t", err, git.resumeContinued)
			}
		})

		testCases := []struct {
			name      string
			configure func(*promotionAlignmentGit)
		}{
			{name: "active operation", configure: func(git *promotionAlignmentGit) {
				git.active = true
				git.activeOperation = "merge"
				git.activeErr = errors.New("active")
			}},
			{name: "conflict inspection", configure: func(git *promotionAlignmentGit) {
				git.active = true
				git.activeOperation = "merge"
				git.resumeConflictsErr = errors.New("conflicts")
			}},
			{name: "fetch", configure: func(git *promotionAlignmentGit) {
				git.active = true
				git.activeOperation = "merge"
				git.fetchErrors = []error{errors.New("fetch")}
			}},
			{name: "target lookup", configure: func(git *promotionAlignmentGit) {
				git.active = true
				git.activeOperation = "merge"
				git.targetErr = errors.New("target")
			}},
			{name: "target absent", configure: func(git *promotionAlignmentGit) {
				git.active = true
				git.activeOperation = "merge"
				git.targetExists = false
			}},
			{name: "target inspector", configure: func(git *promotionAlignmentGit) {
				git.active = true
				git.activeOperation = "merge"
				git.resumeTargetErr = errors.New("inspect target")
			}},
			{name: "continuation", configure: func(git *promotionAlignmentGit) {
				git.active = true
				git.activeOperation = "merge"
				git.resumeContinueErr = errors.New("continue")
			}},
			{name: "second fetch", configure: func(git *promotionAlignmentGit) {
				git.active = true
				git.activeOperation = "merge"
				git.fetchErrors = []error{nil, errors.New("fetch after continue")}
			}},
			{name: "post-merge main comparison", configure: func(git *promotionAlignmentGit) {
				git.active = true
				git.activeOperation = "merge"
				git.missingErr = errors.New("missing")
			}},
			{name: "post-merge validation", configure: func(git *promotionAlignmentGit) {
				git.active = true
				git.activeOperation = "merge"
				git.validateErrors = []error{nil, errors.New("validate")}
			}},
		}
		for _, testCase := range testCases {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				git := newPromotionAlignmentGit(t)
				testCase.configure(git)
				_, err := newPromotionAlignmentService(git, &fakeQualityRunner{}, nil).
					AlignReleasePromotionBase(context.Background(), resumeRequest())
				if err == nil {
					t.Fatal("expected resume failure")
				}
			})
		}
	})

	t.Run("fails closed when optional resume capabilities are absent", func(t *testing.T) {
		request := promotionAlignmentRequest()
		request.Resume = true

		missingInspector := newPromotionAlignmentMinimalGit(t)
		_, err := newPromotionAlignmentService(missingInspector, &fakeQualityRunner{}, nil).
			AlignReleasePromotionBase(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInternal)

		missingContinuator := &promotionAlignmentMissingContinuatorGit{
			promotionAlignmentMinimalGit: newPromotionAlignmentMinimalGit(t),
		}
		_, err = newPromotionAlignmentService(missingContinuator, &fakeQualityRunner{}, nil).
			AlignReleasePromotionBase(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("fails after continuation when quality or publication fails", func(t *testing.T) {
		testCases := []struct {
			name      string
			configure func(*promotionAlignmentGit)
			quality   *fakeQualityRunner
			publisher port.PullRequestPublisher
			request   func(*AlignReleasePromotionBaseRequest)
		}{
			{name: "quality", configure: func(git *promotionAlignmentGit) {
				git.active = true
				git.activeOperation = "merge"
			}, quality: &fakeQualityRunner{err: errors.New("quality")}},
			{name: "publication lookup", configure: func(git *promotionAlignmentGit) {
				git.active = true
				git.activeOperation = "merge"
				git.publicationErr = errors.New("publication")
			}, request: func(request *AlignReleasePromotionBaseRequest) { request.Push = true }},
			{name: "unknown publication", configure: func(git *promotionAlignmentGit) {
				git.active = true
				git.activeOperation = "merge"
				git.publication = branch.PublicationUnknown
			}, request: func(request *AlignReleasePromotionBaseRequest) { request.Push = true }},
			{name: "push", configure: func(git *promotionAlignmentGit) {
				git.active = true
				git.activeOperation = "merge"
				git.pushErr = errors.New("push")
			}, request: func(request *AlignReleasePromotionBaseRequest) { request.Push = true }},
			{name: "publisher", configure: func(git *promotionAlignmentGit) {
				git.active = true
				git.activeOperation = "merge"
			}, publisher: &promotionAlignmentPublisher{
				releaseWhiteboxPublisher: releaseWhiteboxPublisher{err: errors.New("publish")},
			}, request: func(request *AlignReleasePromotionBaseRequest) {
				request.Push = true
				request.CreatePullRequest = true
			}},
		}
		for _, testCase := range testCases {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				git := newPromotionAlignmentGit(t)
				testCase.configure(git)
				quality := testCase.quality
				if quality == nil {
					quality = &fakeQualityRunner{}
				}
				request := promotionAlignmentRequest()
				request.Resume = true
				if testCase.request != nil {
					testCase.request(&request)
				}
				_, err := newPromotionAlignmentService(git, quality, testCase.publisher).
					AlignReleasePromotionBase(context.Background(), request)
				if err == nil {
					t.Fatal("expected resume failure")
				}
			})
		}
	})
}

func TestReleasePromotionBaseAlignmentRejectsUnsafeInputsBeforeMerge(t *testing.T) {
	testCases := []struct {
		name      string
		configure func(*promotionAlignmentGit, *AlignReleasePromotionBaseRequest, *ReleaseService)
		code      problem.Code
	}{
		{
			name: "missing dependencies",
			configure: func(_ *promotionAlignmentGit, request *AlignReleasePromotionBaseRequest, service *ReleaseService) {
				*service = ReleaseService{}
				_ = request
			},
			code: problem.CodeInternal,
		},
		{
			name: "non-release line",
			configure: func(_ *promotionAlignmentGit, request *AlignReleasePromotionBaseRequest, _ *ReleaseService) {
				request.Release = mustBranch("main")
			},
			code: problem.CodeInvalidInput,
		},
		{
			name: "non-chore worker",
			configure: func(_ *promotionAlignmentGit, request *AlignReleasePromotionBaseRequest, _ *ReleaseService) {
				request.Branch = mustBranch("fix/GOV-18-align-promotion-base")
			},
			code: problem.CodeInvalidInput,
		},
		{
			name: "pull request without push",
			configure: func(_ *promotionAlignmentGit, request *AlignReleasePromotionBaseRequest, _ *ReleaseService) {
				request.CreatePullRequest = true
			},
			code: problem.CodeInvalidInput,
		},
		{
			name: "missing repository",
			configure: func(_ *promotionAlignmentGit, request *AlignReleasePromotionBaseRequest, _ *ReleaseService) {
				request.Repository = port.RepositoryIdentity{}
			},
			code: problem.CodeRepositoryNotFound,
		},
		{
			name: "invalid remote",
			configure: func(_ *promotionAlignmentGit, request *AlignReleasePromotionBaseRequest, _ *ReleaseService) {
				request.Repository.Remote = "invalid remote"
			},
			code: problem.CodeBranchBaseInvalid,
		},
		{
			name: "invalid worker reference",
			configure: func(git *promotionAlignmentGit, _ *AlignReleasePromotionBaseRequest, _ *ReleaseService) {
				git.validateErrors = []error{errors.New("validate worker")}
			},
			code: "",
		},
		{
			name: "wrong checked-out branch",
			configure: func(git *promotionAlignmentGit, _ *AlignReleasePromotionBaseRequest, _ *ReleaseService) {
				git.current = mustBranch("chore/GOV-18-another-release-prep")
			},
			code: problem.CodeInvalidInput,
		},
		{
			name: "current branch lookup failure",
			configure: func(git *promotionAlignmentGit, _ *AlignReleasePromotionBaseRequest, _ *ReleaseService) {
				git.currentErr = errors.New("read current branch")
			},
			code: "",
		},
		{
			name: "workflow base lookup failure",
			configure: func(git *promotionAlignmentGit, _ *AlignReleasePromotionBaseRequest, _ *ReleaseService) {
				git.workflowBaseErr = errors.New("read workflow base")
			},
			code: "",
		},
		{
			name: "missing workflow base",
			configure: func(git *promotionAlignmentGit, _ *AlignReleasePromotionBaseRequest, _ *ReleaseService) {
				git.workflowBases = nil
			},
			code: problem.CodeInvalidInput,
		},
		{
			name: "mismatched workflow base",
			configure: func(git *promotionAlignmentGit, request *AlignReleasePromotionBaseRequest, _ *ReleaseService) {
				git.workflowBases[request.Branch.String()] = mustBase("origin", "release/1.0.0")
			},
			code: problem.CodeInvalidInput,
		},
		{
			name: "missing main target",
			configure: func(git *promotionAlignmentGit, _ *AlignReleasePromotionBaseRequest, _ *ReleaseService) {
				git.targetExists = false
			},
			code: problem.CodeInvalidInput,
		},
		{
			name: "main target lookup failure",
			configure: func(git *promotionAlignmentGit, _ *AlignReleasePromotionBaseRequest, _ *ReleaseService) {
				git.targetErr = errors.New("inspect main")
			},
			code: "",
		},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			git := newPromotionAlignmentGit(t)
			quality := &fakeQualityRunner{}
			service := newPromotionAlignmentService(git, quality, nil)
			request := promotionAlignmentRequest()
			testCase.configure(git, &request, service)

			_, err := service.AlignReleasePromotionBase(context.Background(), request)
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

func TestReleasePromotionBaseAlignmentFailureAndCleanupPaths(t *testing.T) {
	t.Run("dry runs reject unavailable and unreadable main bases", func(t *testing.T) {
		testCases := []struct {
			name      string
			configure func(*promotionAlignmentGit)
			code      problem.Code
		}{
			{name: "target lookup", configure: func(git *promotionAlignmentGit) { git.targetErr = errors.New("target") }},
			{name: "target absent", configure: func(git *promotionAlignmentGit) { git.targetExists = false }, code: problem.CodeInvalidInput},
			{name: "missing-base lookup", configure: func(git *promotionAlignmentGit) { git.missingErr = errors.New("missing") }},
		}
		for _, testCase := range testCases {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				git := newPromotionAlignmentGit(t)
				testCase.configure(git)
				request := promotionAlignmentRequest()
				request.DryRun = true
				_, err := newPromotionAlignmentService(git, &fakeQualityRunner{}, nil).AlignReleasePromotionBase(context.Background(), request)
				if testCase.code != "" {
					assertProblemCode(t, err, testCase.code)
				} else if err == nil {
					t.Fatal("expected dry-run failure")
				}
			})
		}
	})

	t.Run("fails before mutation when pull request preflight is unavailable", func(t *testing.T) {
		git := newPromotionAlignmentGit(t)
		quality := &fakeQualityRunner{}
		request := promotionAlignmentRequest()
		request.Push = true
		request.CreatePullRequest = true

		_, err := newPromotionAlignmentService(git, quality, nil).AlignReleasePromotionBase(context.Background(), request)
		assertProblemCode(t, err, problem.CodeExternalCommandFailed)
		if strings.Contains(strings.Join(git.calls, ","), "merge") {
			t.Fatalf("missing publisher merged: %v", git.calls)
		}
	})

	t.Run("fails before mutation when ticket publication composition is absent", func(t *testing.T) {
		git := newPromotionAlignmentGit(t)
		branches := branchapp.NewService(git, &fakeKeyPolicy{})
		service := NewReleaseService(branches, git, nil).WithQualityRunner(&fakeQualityRunner{})
		request := promotionAlignmentRequest()
		request.Push = true
		request.CreatePullRequest = true

		_, err := service.AlignReleasePromotionBase(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInternal)
		if strings.Contains(strings.Join(git.calls, ","), "merge") {
			t.Fatalf("missing publication service merged: %v", git.calls)
		}
	})

	t.Run("fails before mutation when the publisher preflight rejects the PR", func(t *testing.T) {
		git := newPromotionAlignmentGit(t)
		quality := &fakeQualityRunner{}
		preflightErr := errors.New("publisher preflight")
		publisher := &promotionAlignmentPublisher{preflightErr: preflightErr}
		request := promotionAlignmentRequest()
		request.Push = true
		request.CreatePullRequest = true

		_, err := newPromotionAlignmentService(git, quality, publisher).AlignReleasePromotionBase(context.Background(), request)
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
			configure func(*promotionAlignmentGit)
		}{
			{name: "worktree lookup", configure: func(git *promotionAlignmentGit) { git.cleanErr = errors.New("status") }},
			{name: "dirty worktree", configure: func(git *promotionAlignmentGit) { git.clean = false }},
			{name: "fetch", configure: func(git *promotionAlignmentGit) { git.fetchErrors = []error{errors.New("fetch")} }},
			{name: "missing base", configure: func(git *promotionAlignmentGit) { git.missingErr = errors.New("missing base") }},
			{name: "merge", configure: func(git *promotionAlignmentGit) { git.mergeErr = errors.New("merge") }},
		}
		for _, testCase := range testCases {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				git := newPromotionAlignmentGit(t)
				testCase.configure(git)
				_, err := newPromotionAlignmentService(git, &fakeQualityRunner{}, nil).AlignReleasePromotionBase(
					context.Background(),
					promotionAlignmentRequest(),
				)
				if err == nil {
					t.Fatal("expected workflow failure")
				}
			})
		}
	})

	t.Run("preserves post-merge validation quality publication and provider failures", func(t *testing.T) {
		t.Run("merge without publication", func(t *testing.T) {
			git := newPromotionAlignmentGit(t)
			quality := &fakeQualityRunner{}
			result, err := newPromotionAlignmentService(git, quality, nil).AlignReleasePromotionBase(
				context.Background(),
				promotionAlignmentRequest(),
			)
			if err != nil || !result.Merged || result.Pushed || result.Quality == nil || quality.calls != 1 {
				t.Fatalf("unpublished merge = (%#v, %v), quality=%d", result, err, quality.calls)
			}
		})

		t.Run("post-merge validation", func(t *testing.T) {
			git := newPromotionAlignmentGit(t)
			git.validateErrors = []error{nil, errors.New("revalidate")}
			_, err := newPromotionAlignmentService(git, &fakeQualityRunner{}, nil).AlignReleasePromotionBase(
				context.Background(),
				promotionAlignmentRequest(),
			)
			if err == nil {
				t.Fatal("expected revalidation failure")
			}
		})

		t.Run("quality", func(t *testing.T) {
			qualityErr := errors.New("quality")
			_, err := newPromotionAlignmentService(newPromotionAlignmentGit(t), &fakeQualityRunner{err: qualityErr}, nil).
				AlignReleasePromotionBase(context.Background(), promotionAlignmentRequest())
			if !errors.Is(err, qualityErr) {
				t.Fatalf("quality error = %v, want %v", err, qualityErr)
			}
		})

		t.Run("missing quality runner", func(t *testing.T) {
			git := newPromotionAlignmentGit(t)
			service := newPromotionAlignmentService(git, nil, nil)
			_, err := service.AlignReleasePromotionBase(context.Background(), promotionAlignmentRequest())
			assertProblemCode(t, err, problem.CodeInternal)
		})

		t.Run("publication lookup", func(t *testing.T) {
			git := newPromotionAlignmentGit(t)
			git.publicationErr = errors.New("publication")
			request := promotionAlignmentRequest()
			request.Push = true
			_, err := newPromotionAlignmentService(git, &fakeQualityRunner{}, nil).AlignReleasePromotionBase(context.Background(), request)
			if err == nil {
				t.Fatal("expected publication failure")
			}
		})

		t.Run("unknown publication", func(t *testing.T) {
			git := newPromotionAlignmentGit(t)
			git.publication = branch.PublicationUnknown
			request := promotionAlignmentRequest()
			request.Push = true
			_, err := newPromotionAlignmentService(git, &fakeQualityRunner{}, nil).AlignReleasePromotionBase(context.Background(), request)
			assertProblemCode(t, err, problem.CodeInvalidInput)
		})

		t.Run("push", func(t *testing.T) {
			git := newPromotionAlignmentGit(t)
			git.pushErr = errors.New("push")
			request := promotionAlignmentRequest()
			request.Push = true
			_, err := newPromotionAlignmentService(git, &fakeQualityRunner{}, nil).AlignReleasePromotionBase(context.Background(), request)
			if err == nil {
				t.Fatal("expected push failure")
			}
		})

		t.Run("publisher", func(t *testing.T) {
			publishErr := errors.New("publish")
			publisher := &promotionAlignmentPublisher{
				releaseWhiteboxPublisher: releaseWhiteboxPublisher{err: publishErr},
			}
			request := promotionAlignmentRequest()
			request.Push = true
			request.CreatePullRequest = true
			_, err := newPromotionAlignmentService(newPromotionAlignmentGit(t), &fakeQualityRunner{}, publisher).
				AlignReleasePromotionBase(context.Background(), request)
			if !errors.Is(err, publishErr) {
				t.Fatalf("publisher error = %v, want %v", err, publishErr)
			}
		})
	})
}

func TestPromotionBaseAlignmentHelpers(t *testing.T) {
	worker := mustBranch("chore/GOV-18-align-promotion-base")
	release := mustBranch("release/1.0.1")
	message := promotionBaseAlignmentMergeMessage(worker, release)
	if message.String() != "chore(GOV-18): align release 1.0.1 with main for promotion" {
		t.Fatalf("merge message = %q", message)
	}
	pullRequest := promotionBaseAlignmentPullRequest(worker, release, true)
	if pullRequest.Source != worker || pullRequest.Target != release || pullRequest.Ticket.String() != "GOV-18" ||
		pullRequest.Title != "GOV-18: align-promotion-base" || !pullRequest.Draft {
		t.Fatalf("pull request = %#v", pullRequest)
	}
}
