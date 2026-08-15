package branchapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/port"
	domainbranch "github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

type scratchMergeGit struct {
	*fakeGitRepository

	localBranches      map[string]bool
	branchExistsErrors []error
	switchErr          error
	squashErr          error
	staged             bool
	stagedErr          error
	commitErr          error

	switched  []domainbranch.BranchName
	squashed  []domainbranch.BranchName
	committed []commitmsg.Message
}

func newScratchMergeGit(source, target domainbranch.BranchName) *scratchMergeGit {
	return &scratchMergeGit{
		fakeGitRepository: &fakeGitRepository{
			clean:    true,
			official: []domainbranch.BranchName{target},
		},
		localBranches: map[string]bool{
			source.String(): true,
			target.String(): true,
		},
		staged: true,
	}
}

func (git *scratchMergeGit) BranchExists(
	_ context.Context,
	_ port.RepositoryIdentity,
	name domainbranch.BranchName,
) (bool, error) {
	git.calls = append(git.calls, "branch-exists")
	if len(git.branchExistsErrors) > 0 {
		err := git.branchExistsErrors[0]
		git.branchExistsErrors = git.branchExistsErrors[1:]
		if err != nil {
			return false, err
		}
	}
	return git.localBranches[name.String()], nil
}

func (git *scratchMergeGit) SwitchBranch(
	_ context.Context,
	_ port.RepositoryIdentity,
	name domainbranch.BranchName,
) error {
	git.calls = append(git.calls, "switch")
	if git.switchErr != nil {
		return git.switchErr
	}
	git.switched = append(git.switched, name)
	return nil
}

func (git *scratchMergeGit) SquashMerge(
	_ context.Context,
	_ port.RepositoryIdentity,
	source domainbranch.BranchName,
) error {
	git.calls = append(git.calls, "squash-merge")
	if git.squashErr != nil {
		return git.squashErr
	}
	git.squashed = append(git.squashed, source)
	return nil
}

func (git *scratchMergeGit) HasStagedChanges(context.Context, port.RepositoryIdentity) (bool, error) {
	git.calls = append(git.calls, "has-staged")
	if git.stagedErr != nil {
		return false, git.stagedErr
	}
	return git.staged, nil
}

func (git *scratchMergeGit) Commit(
	_ context.Context,
	_ port.RepositoryIdentity,
	message commitmsg.Message,
) error {
	git.calls = append(git.calls, "commit")
	if git.commitErr != nil {
		return git.commitErr
	}
	git.committed = append(git.committed, message)
	return nil
}

func TestScratchMergerTransfersOneGovernedCommit(t *testing.T) {
	source := mustBranch("scratch/ABC-123-export-exploration")
	target := mustBranch("feature/ABC-123-add-export")
	message := mustMessage(t, "feat(ABC-123): add export")
	git := newScratchMergeGit(source, target)
	keys := &fakeKeyPolicy{}
	merger := NewScratchMerger(git, NewService(git, keys))

	result, err := merger.Merge(context.Background(), ScratchMergeRequest{
		Repository: testRepository(),
		Source:     source,
		Message:    message,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != source || result.Target != target || !result.Committed || result.DryRun {
		t.Fatalf("Merge() = %#v", result)
	}
	if len(result.Plan) != 3 ||
		result.Plan[0].Action != "switch" ||
		result.Plan[1].Action != "squash-merge" ||
		result.Plan[2].Action != "commit" {
		t.Fatalf("scratch merge plan = %#v", result.Plan)
	}
	if len(git.switched) != 1 || git.switched[0] != target {
		t.Fatalf("switched branches = %#v", git.switched)
	}
	if len(git.squashed) != 1 || git.squashed[0] != source {
		t.Fatalf("squashed branches = %#v", git.squashed)
	}
	if len(git.committed) != 1 || git.committed[0].String() != message.String() {
		t.Fatalf("committed messages = %#v", git.committed)
	}
	if got := strings.Join(keys.keys, ","); got != "ABC,ABC" {
		t.Fatalf("validated keys = %q", got)
	}
}

func TestScratchMergerResolvesLocalOfficialTargets(t *testing.T) {
	source := mustBranch("scratch/ABC-123-export-exploration")
	target := mustBranch("feature/ABC-123-add-export")

	t.Run("uses an explicit matching local target", func(t *testing.T) {
		git := newScratchMergeGit(source, target)
		merger := NewScratchMerger(git, NewService(git, &fakeKeyPolicy{}))

		actual, err := merger.ResolveTarget(context.Background(), testRepository(), source, &target)
		if err != nil || actual != target {
			t.Fatalf("ResolveTarget() = (%q, %v)", actual.String(), err)
		}
		if strings.Contains(strings.Join(git.calls, ","), "official-branches-for-ticket") {
			t.Fatalf("explicit target unexpectedly queried candidates: %v", git.calls)
		}
	})

	t.Run("uses the sole local official branch for the ticket", func(t *testing.T) {
		git := newScratchMergeGit(source, target)
		merger := NewScratchMerger(git, NewService(git, &fakeKeyPolicy{}))

		actual, err := merger.ResolveTarget(context.Background(), testRepository(), source, nil)
		if err != nil || actual != target {
			t.Fatalf("ResolveTarget() = (%q, %v)", actual.String(), err)
		}
	})

	t.Run("rejects missing sources and targets", func(t *testing.T) {
		git := newScratchMergeGit(source, target)
		git.localBranches[source.String()] = false
		merger := NewScratchMerger(git, NewService(git, &fakeKeyPolicy{}))
		_, err := merger.ResolveTarget(context.Background(), testRepository(), source, nil)
		assertProblemCode(t, err, problem.CodeScratchSourceBranchMissing)

		git = newScratchMergeGit(source, target)
		git.localBranches[target.String()] = false
		merger = NewScratchMerger(git, NewService(git, &fakeKeyPolicy{}))
		_, err = merger.ResolveTarget(context.Background(), testRepository(), source, &target)
		assertProblemCode(t, err, problem.CodeScratchTargetBranchMissing)

		git = newScratchMergeGit(source, target)
		git.official = nil
		merger = NewScratchMerger(git, NewService(git, &fakeKeyPolicy{}))
		_, err = merger.ResolveTarget(context.Background(), testRepository(), source, nil)
		assertProblemCode(t, err, problem.CodeScratchTargetBranchMissing)
	})

	t.Run("rejects ambiguous or invalid candidates", func(t *testing.T) {
		second := mustBranch("docs/ABC-123-update-export-guide")
		git := newScratchMergeGit(source, target)
		git.official = []domainbranch.BranchName{second, target}
		git.localBranches[second.String()] = true
		merger := NewScratchMerger(git, NewService(git, &fakeKeyPolicy{}))
		_, err := merger.ResolveTarget(context.Background(), testRepository(), source, nil)
		actual, ok := problem.As(err)
		if !ok || actual.Code != problem.CodeScratchTargetBranchAmbiguous ||
			actual.Actual != "docs/ABC-123-update-export-guide, feature/ABC-123-add-export" {
			t.Fatalf("ambiguous target error = %#v", err)
		}

		git = newScratchMergeGit(source, target)
		git.official = []domainbranch.BranchName{mustBranch("main")}
		merger = NewScratchMerger(git, NewService(git, &fakeKeyPolicy{}))
		_, err = merger.ResolveTarget(context.Background(), testRepository(), source, nil)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)

		otherTicket := mustBranch("feature/ABC-124-other-ticket")
		git = newScratchMergeGit(source, target)
		git.localBranches[otherTicket.String()] = true
		merger = NewScratchMerger(git, NewService(git, &fakeKeyPolicy{}))
		_, err = merger.ResolveTarget(context.Background(), testRepository(), source, &otherTicket)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})

	t.Run("propagates repository lookup failures", func(t *testing.T) {
		branchExistsErr := errors.New("branch lookup failed")
		git := newScratchMergeGit(source, target)
		git.branchExistsErrors = []error{branchExistsErr}
		merger := NewScratchMerger(git, NewService(git, &fakeKeyPolicy{}))
		_, err := merger.ResolveTarget(context.Background(), testRepository(), source, nil)
		if !errors.Is(err, branchExistsErr) {
			t.Fatalf("source lookup error = %v, want %v", err, branchExistsErr)
		}

		officialErr := errors.New("official branch lookup failed")
		git = newScratchMergeGit(source, target)
		git.officialBranchesErr = officialErr
		merger = NewScratchMerger(git, NewService(git, &fakeKeyPolicy{}))
		_, err = merger.ResolveTarget(context.Background(), testRepository(), source, nil)
		if !errors.Is(err, officialErr) {
			t.Fatalf("official lookup error = %v, want %v", err, officialErr)
		}

		git = newScratchMergeGit(source, target)
		git.branchExistsErrors = []error{nil, branchExistsErr}
		merger = NewScratchMerger(git, NewService(git, &fakeKeyPolicy{}))
		_, err = merger.ResolveTarget(context.Background(), testRepository(), source, nil)
		if !errors.Is(err, branchExistsErr) {
			t.Fatalf("candidate lookup error = %v, want %v", err, branchExistsErr)
		}

		git = newScratchMergeGit(source, target)
		git.branchExistsErrors = []error{nil, branchExistsErr}
		merger = NewScratchMerger(git, NewService(git, &fakeKeyPolicy{}))
		_, err = merger.ResolveTarget(context.Background(), testRepository(), source, &target)
		if !errors.Is(err, branchExistsErr) {
			t.Fatalf("explicit target lookup error = %v, want %v", err, branchExistsErr)
		}
	})

	t.Run("rejects non-scratch sources and unavailable dependencies", func(t *testing.T) {
		merger := NewScratchMerger(newScratchMergeGit(source, target), NewService(newScratchMergeGit(source, target), &fakeKeyPolicy{}))
		_, err := merger.ResolveTarget(context.Background(), testRepository(), target, nil)
		assertProblemCode(t, err, problem.CodeBranchFamilyInvalid)

		_, err = (*ScratchMerger)(nil).ResolveTarget(context.Background(), testRepository(), source, nil)
		assertProblemCode(t, err, problem.CodeInternal)

		_, err = merger.ResolveTarget(context.Background(), port.RepositoryIdentity{}, source, nil)
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = merger.ResolveTarget(ctx, testRepository(), source, nil)
		assertProblemCode(t, err, problem.CodeOperationCancelled)
	})
}

func TestScratchMergerValidatesAndProtectsMutationPaths(t *testing.T) {
	source := mustBranch("scratch/ABC-123-export-exploration")
	target := mustBranch("feature/ABC-123-add-export")
	message := mustMessage(t, "feat(ABC-123): add export")
	request := func() ScratchMergeRequest {
		return ScratchMergeRequest{
			Repository: testRepository(),
			Source:     source,
			Message:    message,
		}
	}

	t.Run("validates message contracts before mutation", func(t *testing.T) {
		git := newScratchMergeGit(source, target)
		merger := NewScratchMerger(git, NewService(git, &fakeKeyPolicy{}))

		invalid := request()
		invalid.Message = commitmsg.Message{}
		_, err := merger.Merge(context.Background(), invalid)
		assertProblemCode(t, err, problem.CodeCommitHeaderInvalid)

		invalid = request()
		invalid.Message = mustMessage(t, "feat(ABC-124): wrong ticket")
		_, err = merger.Merge(context.Background(), invalid)
		assertProblemCode(t, err, problem.CodeCommitTicketMismatch)

		err = ValidateScratchMergeMessage(mustBranch("develop"), message)
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
	})

	t.Run("handles service and context preconditions", func(t *testing.T) {
		git := newScratchMergeGit(source, target)
		_, err := NewScratchMerger(git, nil).Merge(context.Background(), request())
		assertProblemCode(t, err, problem.CodeInternal)

		invalidRepository := request()
		invalidRepository.Repository = port.RepositoryIdentity{}
		_, err = NewScratchMerger(git, NewService(git, &fakeKeyPolicy{})).Merge(context.Background(), invalidRepository)
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = NewScratchMerger(git, NewService(git, &fakeKeyPolicy{})).Merge(ctx, request())
		assertProblemCode(t, err, problem.CodeOperationCancelled)

		_, err = (*ScratchMerger)(nil).Merge(context.Background(), request())
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("propagates source and target validation failures", func(t *testing.T) {
		sourceValidationErr := errors.New("source validation failed")
		git := newScratchMergeGit(source, target)
		git.validateRefErr = sourceValidationErr
		_, err := NewScratchMerger(git, NewService(git, &fakeKeyPolicy{})).Merge(context.Background(), request())
		if !errors.Is(err, sourceValidationErr) {
			t.Fatalf("source validation error = %v, want %v", err, sourceValidationErr)
		}

		targetValidationErr := errors.New("target validation failed")
		git = newScratchMergeGit(source, target)
		git.validateRefErrors = []error{nil, targetValidationErr}
		_, err = NewScratchMerger(git, NewService(git, &fakeKeyPolicy{})).Merge(context.Background(), request())
		if !errors.Is(err, targetValidationErr) {
			t.Fatalf("target validation error = %v, want %v", err, targetValidationErr)
		}
	})

	t.Run("propagates target resolution failures", func(t *testing.T) {
		git := newScratchMergeGit(source, target)
		git.localBranches[source.String()] = false
		_, err := NewScratchMerger(git, NewService(git, &fakeKeyPolicy{})).Merge(context.Background(), request())
		assertProblemCode(t, err, problem.CodeScratchSourceBranchMissing)
	})

	t.Run("returns a dry-run plan without Git mutation", func(t *testing.T) {
		git := newScratchMergeGit(source, target)
		dryRun := request()
		dryRun.DryRun = true
		result, err := NewScratchMerger(git, NewService(git, &fakeKeyPolicy{})).Merge(context.Background(), dryRun)
		if err != nil || !result.DryRun || result.Committed {
			t.Fatalf("dry Merge() = (%#v, %v)", result, err)
		}
		for _, prohibited := range []string{"worktree-clean", "switch", "squash-merge", "has-staged", "commit"} {
			if strings.Contains(strings.Join(git.calls, ","), prohibited) {
				t.Fatalf("dry run called %q: %v", prohibited, git.calls)
			}
		}
	})

	for _, testCase := range []struct {
		name      string
		configure func(*scratchMergeGit)
		wantCode  problem.Code
		wantErr   func(*scratchMergeGit) error
	}{
		{
			name: "dirty worktree",
			configure: func(git *scratchMergeGit) {
				git.clean = false
			},
			wantCode: problem.CodeWorktreeNotClean,
		},
		{
			name: "worktree inspection",
			configure: func(git *scratchMergeGit) {
				git.worktreeCleanErr = errors.New("status failed")
			},
			wantErr: func(git *scratchMergeGit) error {
				return git.worktreeCleanErr
			},
		},
		{
			name: "switch",
			configure: func(git *scratchMergeGit) {
				git.switchErr = errors.New("switch failed")
			},
			wantErr: func(git *scratchMergeGit) error {
				return git.switchErr
			},
		},
		{
			name: "squash merge",
			configure: func(git *scratchMergeGit) {
				git.squashErr = errors.New("squash failed")
			},
			wantErr: func(git *scratchMergeGit) error {
				return git.squashErr
			},
		},
		{
			name: "staged changes",
			configure: func(git *scratchMergeGit) {
				git.stagedErr = errors.New("index failed")
			},
			wantErr: func(git *scratchMergeGit) error {
				return git.stagedErr
			},
		},
		{
			name: "empty squash",
			configure: func(git *scratchMergeGit) {
				git.staged = false
			},
			wantCode: problem.CodeScratchMergeEmpty,
		},
		{
			name: "commit",
			configure: func(git *scratchMergeGit) {
				git.commitErr = errors.New("commit failed")
			},
			wantErr: func(git *scratchMergeGit) error {
				return git.commitErr
			},
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			git := newScratchMergeGit(source, target)
			testCase.configure(git)
			_, err := NewScratchMerger(git, NewService(git, &fakeKeyPolicy{})).Merge(context.Background(), request())
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr(git)) {
					t.Fatalf("Merge() error = %v, want %v", err, testCase.wantErr(git))
				}
				return
			}
			assertProblemCode(t, err, testCase.wantCode)
		})
	}
}

func TestScratchMergerClassifiesAndResumesConflictedSquashes(t *testing.T) {
	source := mustBranch("scratch/ABC-123-export-exploration")
	target := mustBranch("feature/ABC-123-add-export")
	message := mustMessage(t, "feat(ABC-123): add export")
	request := ScratchMergeRequest{
		Repository: testRepository(),
		Source:     source,
		Message:    message,
	}

	t.Run("classifies only unresolved squash conflicts as resumable", func(t *testing.T) {
		squashErr := errors.New("merge conflict")
		git := newScratchMergeGit(source, target)
		git.squashErr = squashErr
		git.unmergedConflicts = true
		_, err := NewScratchMerger(git, NewService(git, &fakeKeyPolicy{})).Merge(context.Background(), request)
		assertProblemCode(t, err, problem.CodeScratchMergeConflict)
		if !errors.Is(err, squashErr) {
			t.Fatalf("conflict error = %v, want %v", err, squashErr)
		}

		git = newScratchMergeGit(source, target)
		git.squashErr = squashErr
		_, err = NewScratchMerger(git, NewService(git, &fakeKeyPolicy{})).Merge(context.Background(), request)
		if !errors.Is(err, squashErr) {
			t.Fatalf("non-conflict squash error = %v, want %v", err, squashErr)
		}

		conflictInspectionErr := errors.New("conflict inspection failed")
		git = newScratchMergeGit(source, target)
		git.squashErr = squashErr
		git.unmergedConflictsErr = conflictInspectionErr
		_, err = NewScratchMerger(git, NewService(git, &fakeKeyPolicy{})).Merge(context.Background(), request)
		if !errors.Is(err, conflictInspectionErr) {
			t.Fatalf("conflict inspection error = %v, want %v", err, conflictInspectionErr)
		}
	})

	t.Run("finishes a resolved squash without merging again", func(t *testing.T) {
		git := newScratchMergeGit(source, target)
		merger := NewScratchMerger(git, NewService(git, &fakeKeyPolicy{}))
		result, err := merger.Resume(context.Background(), request)
		if err != nil || !result.Committed || len(result.Plan) != 2 {
			t.Fatalf("Resume() = (%#v, %v)", result, err)
		}
		if len(git.squashed) != 0 || len(git.committed) != 1 || git.committed[0].String() != message.String() {
			t.Fatalf("resume mutations = squashed:%#v committed:%#v", git.squashed, git.committed)
		}
	})

	t.Run("keeps retryable conflicts and error paths explicit", func(t *testing.T) {
		git := newScratchMergeGit(source, target)
		git.unmergedConflicts = true
		_, err := NewScratchMerger(git, NewService(git, &fakeKeyPolicy{})).Resume(context.Background(), request)
		assertProblemCode(t, err, problem.CodeScratchMergeConflict)

		git = newScratchMergeGit(source, target)
		git.unmergedConflictsErr = errors.New("inspect conflicts")
		_, err = NewScratchMerger(git, NewService(git, &fakeKeyPolicy{})).Resume(context.Background(), request)
		if !errors.Is(err, git.unmergedConflictsErr) {
			t.Fatalf("resume conflict inspection error = %v", err)
		}

		git = newScratchMergeGit(source, target)
		git.staged = false
		_, err = NewScratchMerger(git, NewService(git, &fakeKeyPolicy{})).Resume(context.Background(), request)
		assertProblemCode(t, err, problem.CodeScratchMergeEmpty)

		git = newScratchMergeGit(source, target)
		git.stagedErr = errors.New("index failed")
		_, err = NewScratchMerger(git, NewService(git, &fakeKeyPolicy{})).Resume(context.Background(), request)
		if !errors.Is(err, git.stagedErr) {
			t.Fatalf("resume staged error = %v", err)
		}

		git = newScratchMergeGit(source, target)
		git.commitErr = errors.New("commit failed")
		_, err = NewScratchMerger(git, NewService(git, &fakeKeyPolicy{})).Resume(context.Background(), request)
		if !errors.Is(err, git.commitErr) {
			t.Fatalf("resume commit error = %v", err)
		}
	})

	t.Run("preserves validation guards and dry-run behavior", func(t *testing.T) {
		_, err := (&ScratchMerger{}).Resume(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInternal)

		git := newScratchMergeGit(source, target)
		_, err = NewScratchMerger(git, nil).Resume(context.Background(), request)
		assertProblemCode(t, err, problem.CodeInternal)

		invalidRepository := request
		invalidRepository.Repository = port.RepositoryIdentity{}
		_, err = NewScratchMerger(git, NewService(git, &fakeKeyPolicy{})).Resume(context.Background(), invalidRepository)
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = NewScratchMerger(git, NewService(git, &fakeKeyPolicy{})).Resume(ctx, request)
		assertProblemCode(t, err, problem.CodeOperationCancelled)

		dry := request
		dry.DryRun = true
		result, err := NewScratchMerger(git, NewService(git, &fakeKeyPolicy{})).Resume(context.Background(), dry)
		if err != nil || !result.DryRun || result.Committed {
			t.Fatalf("dry Resume() = (%#v, %v)", result, err)
		}

		git = newScratchMergeGit(source, target)
		git.localBranches[source.String()] = false
		_, err = NewScratchMerger(git, NewService(git, &fakeKeyPolicy{})).Resume(context.Background(), request)
		assertProblemCode(t, err, problem.CodeScratchSourceBranchMissing)

		invalidMessage := request
		invalidMessage.Message = commitmsg.Message{}
		git = newScratchMergeGit(source, target)
		_, err = NewScratchMerger(git, NewService(git, &fakeKeyPolicy{})).Resume(context.Background(), invalidMessage)
		assertProblemCode(t, err, problem.CodeCommitHeaderInvalid)

		sourceValidationErr := errors.New("source validation failed")
		git = newScratchMergeGit(source, target)
		git.validateRefErr = sourceValidationErr
		_, err = NewScratchMerger(git, NewService(git, &fakeKeyPolicy{})).Resume(context.Background(), request)
		if !errors.Is(err, sourceValidationErr) {
			t.Fatalf("resume source validation error = %v", err)
		}

		targetValidationErr := errors.New("target validation failed")
		git = newScratchMergeGit(source, target)
		git.validateRefErrors = []error{nil, targetValidationErr}
		_, err = NewScratchMerger(git, NewService(git, &fakeKeyPolicy{})).Resume(context.Background(), request)
		if !errors.Is(err, targetValidationErr) {
			t.Fatalf("resume target validation error = %v", err)
		}
	})
}

var _ port.GitRepository = (*scratchMergeGit)(nil)
