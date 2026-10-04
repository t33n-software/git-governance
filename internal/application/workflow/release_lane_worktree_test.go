package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// manifestLaneWorktreeGit layers the worktree capability onto the manifest
// propagation fake while recording the acquisition base the shared
// task-worktree guard measures. The detached classification drives the guard
// into its pre-start form; the linked flag drives the gate decision.
type manifestLaneWorktreeGit struct {
	*releaseManifestGit
	linked       bool
	measuredBase branch.TargetBase
}

func (git *manifestLaneWorktreeGit) CurrentBranch(context.Context, port.RepositoryIdentity) (branch.BranchName, error) {
	git.calls = append(git.calls, "current-branch")
	return branch.BranchName{}, detachedCurrentBranchProblem()
}

func (git *manifestLaneWorktreeGit) LinkedWorktree(context.Context, port.RepositoryIdentity) (bool, error) {
	git.calls = append(git.calls, "linked-worktree")
	return git.linked, nil
}

func (git *manifestLaneWorktreeGit) WorktreeAddDetached(context.Context, port.RepositoryIdentity, string, branch.TargetBase) error {
	return nil
}

func (git *manifestLaneWorktreeGit) WorktreeRemove(context.Context, port.RepositoryIdentity, string) error {
	return nil
}

func (git *manifestLaneWorktreeGit) WorktreeList(context.Context, port.RepositoryIdentity) ([]port.WorktreeEntry, error) {
	return nil, nil
}

func (git *manifestLaneWorktreeGit) WorktreeHeadMatchesBase(_ context.Context, _ port.RepositoryIdentity, base branch.TargetBase) (string, string, error) {
	git.calls = append(git.calls, "worktree-head-matches-base")
	git.measuredBase = base
	return base.String(), base.String(), nil
}

// promotionPreStartWorktreeGit models a detached pre-start worktree hosting
// promotion-base alignment while recording the acquisition base the shared
// guard measures.
type promotionPreStartWorktreeGit struct {
	*promotionAlignmentGit
	measuredBase branch.TargetBase
}

func (git *promotionPreStartWorktreeGit) CurrentBranch(context.Context, port.RepositoryIdentity) (branch.BranchName, error) {
	git.calls = append(git.calls, "current-branch")
	return branch.BranchName{}, detachedCurrentBranchProblem()
}

func (git *promotionPreStartWorktreeGit) LinkedWorktree(context.Context, port.RepositoryIdentity) (bool, error) {
	git.calls = append(git.calls, "linked-worktree")
	return true, nil
}

func (git *promotionPreStartWorktreeGit) WorktreeAddDetached(context.Context, port.RepositoryIdentity, string, branch.TargetBase) error {
	return nil
}

func (git *promotionPreStartWorktreeGit) WorktreeRemove(context.Context, port.RepositoryIdentity, string) error {
	return nil
}

func (git *promotionPreStartWorktreeGit) WorktreeList(context.Context, port.RepositoryIdentity) ([]port.WorktreeEntry, error) {
	return nil, nil
}

func (git *promotionPreStartWorktreeGit) WorktreeHeadMatchesBase(_ context.Context, _ port.RepositoryIdentity, base branch.TargetBase) (string, string, error) {
	git.calls = append(git.calls, "worktree-head-matches-base")
	git.measuredBase = base
	return base.String(), base.String(), nil
}

// reconciliationPreStartWorktreeGit models a detached pre-start worktree
// hosting reconciliation-base alignment while recording the acquisition base
// the shared guard measures.
type reconciliationPreStartWorktreeGit struct {
	*reconciliationAlignmentGit
	measuredBase branch.TargetBase
}

func (git *reconciliationPreStartWorktreeGit) CurrentBranch(context.Context, port.RepositoryIdentity) (branch.BranchName, error) {
	git.calls = append(git.calls, "current-branch")
	return branch.BranchName{}, detachedCurrentBranchProblem()
}

func (git *reconciliationPreStartWorktreeGit) LinkedWorktree(context.Context, port.RepositoryIdentity) (bool, error) {
	git.calls = append(git.calls, "linked-worktree")
	return true, nil
}

func (git *reconciliationPreStartWorktreeGit) WorktreeAddDetached(context.Context, port.RepositoryIdentity, string, branch.TargetBase) error {
	return nil
}

func (git *reconciliationPreStartWorktreeGit) WorktreeRemove(context.Context, port.RepositoryIdentity, string) error {
	return nil
}

func (git *reconciliationPreStartWorktreeGit) WorktreeList(context.Context, port.RepositoryIdentity) ([]port.WorktreeEntry, error) {
	return nil, nil
}

func (git *reconciliationPreStartWorktreeGit) WorktreeHeadMatchesBase(_ context.Context, _ port.RepositoryIdentity, base branch.TargetBase) (string, string, error) {
	git.calls = append(git.calls, "worktree-head-matches-base")
	git.measuredBase = base
	return base.String(), base.String(), nil
}

func TestReleaseLanesRequireTaskWorktrees(t *testing.T) {
	t.Parallel()

	source := mustBranch("hotfix/ABC-999-payment-timeout")
	target := mustBranch("develop")
	record := releaseRecordWithManifest(t, source.String(), "main", []string{strings.Repeat("a", 40)}, []string{"develop"})

	t.Run("hotfix start fails closed outside a task worktree", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{clean: true}}
		_, err := newReleaseWhiteboxService(git, nil).StartHotfix(context.Background(), releaseHotfixRequest())
		assertProblemCode(t, err, problem.CodeWorktreeRequired)
		assertReleaseNoCall(t, git.calls, "create-branch")
		assertReleaseNoCall(t, git.calls, "store-workflow-base")
	})

	t.Run("release stabilization fails closed outside a task worktree", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{clean: true}}
		_, err := newReleaseWhiteboxService(git, nil).CreateReleaseStabilization(context.Background(), releaseStabilizationRequest())
		assertProblemCode(t, err, problem.CodeWorktreeRequired)
		assertReleaseNoCall(t, git.calls, "create-branch")
		assertReleaseNoCall(t, git.calls, "store-workflow-base")
	})

	t.Run("release promotion fails closed outside a task worktree", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{clean: true}}
		_, err := newReleaseWhiteboxService(git, nil).PrepareReleasePromotion(context.Background(), PrepareReleasePromotionRequest{
			Repository: testRepository(),
			Release:    mustBranch("release/1.0.1"),
		})
		assertProblemCode(t, err, problem.CodeWorktreeRequired)
		assertReleaseNoCall(t, git.calls, "fetch")
	})

	t.Run("release backmerge fails closed outside a task worktree", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{clean: true}}
		_, err := newReleaseWhiteboxService(git, nil).PrepareReleaseBackmerge(context.Background(), PrepareReleaseBackmergeRequest{
			Repository: testRepository(),
			Release:    mustBranch("release/1.0.1"),
		})
		assertProblemCode(t, err, problem.CodeWorktreeRequired)
		assertReleaseNoCall(t, git.calls, "fetch")
	})

	t.Run("hotfix propagation fails closed outside a task worktree", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{clean: true}}
		_, err := newReleaseWhiteboxService(git, nil).PropagateHotfix(context.Background(), releasePropagationRequest())
		assertProblemCode(t, err, problem.CodeWorktreeRequired)
		assertReleaseNoCall(t, git.calls, "create-branch")
		assertReleaseNoCall(t, git.calls, "store-workflow-base")
		assertReleaseNoCall(t, git.calls, "cherry-pick")
	})

	t.Run("hotfix manifest propagation fails closed outside a task worktree", func(t *testing.T) {
		t.Parallel()
		git := &manifestLaneWorktreeGit{releaseManifestGit: &releaseManifestGit{releaseWhiteboxGit: newReleaseWhiteboxGit()}}
		_, err := newReleaseWhiteboxService(git, nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithQualityRunner(&releaseWhiteboxQuality{}).
			PropagateHotfixManifest(context.Background(), PropagateHotfixManifestRequest{
				Repository: testRepository(),
				Source:     source,
				TargetLine: target,
			})
		assertProblemCode(t, err, problem.CodeWorktreeRequired)
		assertReleaseNoCall(t, git.calls, "create-branch")
		if len(git.stored) != 0 {
			t.Fatalf("a gated manifest dispatch must not store progress: %#v", git.stored)
		}
	})

	t.Run("promotion-base alignment fails closed outside a task worktree", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{clean: true}}
		_, err := newReleaseWhiteboxService(git, nil).AlignReleasePromotionBase(context.Background(), promotionAlignmentRequest())
		assertProblemCode(t, err, problem.CodeWorktreeRequired)
		assertReleaseNoCall(t, git.calls, "merge")
		assertReleaseNoCall(t, git.calls, "push")
	})

	t.Run("reconciliation-base alignment fails closed outside a task worktree", func(t *testing.T) {
		t.Parallel()
		git := &fakeWorktreeGit{fakeGitRepository: &fakeGitRepository{clean: true}}
		_, err := newReleaseWhiteboxService(git, nil).AlignReleaseReconciliationBase(context.Background(), reconciliationAlignmentRequest())
		assertProblemCode(t, err, problem.CodeWorktreeRequired)
		assertReleaseNoCall(t, git.calls, "merge")
		assertReleaseNoCall(t, git.calls, "push")
	})

	t.Run("keeps the unenforced legacy behavior without the worktree capability", func(t *testing.T) {
		t.Parallel()
		git := newReleaseWhiteboxGit()
		result, err := newReleaseWhiteboxService(git, nil).StartHotfix(context.Background(), releaseHotfixRequest())
		if err != nil {
			t.Fatal(err)
		}
		if result.Name.String() != "hotfix/ABC-999-payment-timeout" {
			t.Fatalf("StartHotfix() = %#v", result)
		}
	})
}

func TestReleaseLanesMeasureTheirAcquisitionBase(t *testing.T) {
	t.Parallel()

	const baseRevision = "c46015869552bc0433fa2a5276713d74bfc73f87"
	detachedAtBase := func() *detachedLaneWorktreeGit {
		return &detachedLaneWorktreeGit{fakeWorktreeGit: &fakeWorktreeGit{
			fakeGitRepository: &fakeGitRepository{
				hasCommits: true,
				clean:      true,
				messages:   []string{"fix(ABC-999): resolve payment timeout"},
			},
			linked:       true,
			head:         baseRevision,
			baseRevision: baseRevision,
		}}
	}

	t.Run("the hotfix lane measures its affected protected line", func(t *testing.T) {
		t.Parallel()
		git := detachedAtBase()
		result, err := newReleaseWhiteboxService(git, nil).StartHotfix(context.Background(), releaseHotfixRequest())
		if err != nil {
			t.Fatal(err)
		}
		if result.Name.String() != "hotfix/ABC-999-payment-timeout" || result.Base.String() != "origin/main" {
			t.Fatalf("StartHotfix() = %#v", result)
		}
		if git.measuredBase.String() != "origin/main" {
			t.Fatalf("measured base = %q, want origin/main", git.measuredBase.String())
		}
	})

	t.Run("the stabilization lane measures its frozen release line", func(t *testing.T) {
		t.Parallel()
		git := detachedAtBase()
		result, err := newReleaseWhiteboxService(git, nil).CreateReleaseStabilization(context.Background(), releaseStabilizationRequest())
		if err != nil {
			t.Fatal(err)
		}
		if result.Name.String() != "fix/ABC-999-release-blocker" || result.Base.String() != "origin/release/2.8.0" {
			t.Fatalf("CreateReleaseStabilization() = %#v", result)
		}
		if git.measuredBase.String() != "origin/release/2.8.0" {
			t.Fatalf("measured base = %q, want origin/release/2.8.0", git.measuredBase.String())
		}
	})

	t.Run("the promotion dispatch measures its frozen release line", func(t *testing.T) {
		t.Parallel()
		git := detachedAtBase()
		result, err := newReleaseWhiteboxService(git, nil).PrepareReleasePromotion(context.Background(), PrepareReleasePromotionRequest{
			Repository: testRepository(),
			Release:    mustBranch("release/1.0.1"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.PullRequest.Target.String() != "main" {
			t.Fatalf("PrepareReleasePromotion() = %#v", result)
		}
		if git.measuredBase.String() != "origin/release/1.0.1" {
			t.Fatalf("measured base = %q, want origin/release/1.0.1", git.measuredBase.String())
		}
	})

	t.Run("the backmerge dispatch measures its frozen release line", func(t *testing.T) {
		t.Parallel()
		git := detachedAtBase()
		result, err := newReleaseWhiteboxService(git, nil).PrepareReleaseBackmerge(context.Background(), PrepareReleaseBackmergeRequest{
			Repository: testRepository(),
			Release:    mustBranch("release/1.0.1"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.PullRequest.Target.String() != "develop" {
			t.Fatalf("PrepareReleaseBackmerge() = %#v", result)
		}
		if git.measuredBase.String() != "origin/release/1.0.1" {
			t.Fatalf("measured base = %q, want origin/release/1.0.1", git.measuredBase.String())
		}
	})

	t.Run("the propagation lane measures its target line", func(t *testing.T) {
		t.Parallel()
		git := detachedAtBase()
		result, err := newReleaseWhiteboxService(git, nil).PropagateHotfix(context.Background(), releasePropagationRequest())
		if err != nil {
			t.Fatal(err)
		}
		if !result.CherryPicked || result.Branch.Name.String() != "fix/ABC-999-forward-port-payment-timeout" {
			t.Fatalf("PropagateHotfix() = %#v", result)
		}
		if git.measuredBase.String() != "origin/main" {
			t.Fatalf("measured base = %q, want origin/main", git.measuredBase.String())
		}
	})

	t.Run("the manifest propagation lane measures its target line", func(t *testing.T) {
		t.Parallel()
		source := mustBranch("hotfix/ABC-999-payment-timeout")
		target := mustBranch("develop")
		record := releaseRecordWithManifest(t, source.String(), "main", []string{strings.Repeat("a", 40)}, []string{"develop"})
		git := &manifestLaneWorktreeGit{releaseManifestGit: &releaseManifestGit{releaseWhiteboxGit: newReleaseWhiteboxGit()}, linked: true}
		result, err := newReleaseWhiteboxService(git, nil).
			WithHotfixReleaseRecordStore(&releaseWhiteboxRecordStore{record: record}).
			WithQualityRunner(&releaseWhiteboxQuality{}).
			PropagateHotfixManifest(context.Background(), PropagateHotfixManifestRequest{
				Repository: testRepository(),
				Source:     source,
				TargetLine: target,
				DryRun:     true,
			})
		if err != nil {
			t.Fatal(err)
		}
		if !result.DryRun || result.Branch.Name.String() != "fix/ABC-999-propagate-to-develop" {
			t.Fatalf("PropagateHotfixManifest() = %#v", result)
		}
		if git.measuredBase.String() != "origin/develop" {
			t.Fatalf("measured base = %q, want origin/develop", git.measuredBase.String())
		}
	})

	t.Run("promotion-base alignment measures its frozen release line before its branch context", func(t *testing.T) {
		t.Parallel()
		git := &promotionPreStartWorktreeGit{promotionAlignmentGit: &promotionAlignmentGit{
			releaseWhiteboxGit: newReleaseWhiteboxGit(),
			current:            mustBranch("chore/GOV-18-align-promotion-base"),
		}}
		_, err := newReleaseWhiteboxService(git, nil).AlignReleasePromotionBase(context.Background(), promotionAlignmentRequest())
		assertProblemCode(t, err, problem.CodeBranchNameInvalid)
		if git.measuredBase.String() != "origin/release/1.0.1" {
			t.Fatalf("measured base = %q, want origin/release/1.0.1", git.measuredBase.String())
		}
	})

	t.Run("reconciliation-base alignment measures its frozen release line before its branch context", func(t *testing.T) {
		t.Parallel()
		git := &reconciliationPreStartWorktreeGit{reconciliationAlignmentGit: &reconciliationAlignmentGit{
			releaseWhiteboxGit: newReleaseWhiteboxGit(),
			current:            mustBranch("chore/GOV-20-align-release-reconciliation-base"),
		}}
		_, err := newReleaseWhiteboxService(git, nil).AlignReleaseReconciliationBase(context.Background(), reconciliationAlignmentRequest())
		assertProblemCode(t, err, problem.CodeBranchNameInvalid)
		if git.measuredBase.String() != "origin/release/1.0.1" {
			t.Fatalf("measured base = %q, want origin/release/1.0.1", git.measuredBase.String())
		}
	})
}

// detachedLaneWorktreeGit reports the detached pre-start classification with
// the complete worktree capability while recording the acquisition base the
// shared task-worktree guard measures.
type detachedLaneWorktreeGit struct {
	*fakeWorktreeGit
	measuredBase branch.TargetBase
}

func (git *detachedLaneWorktreeGit) CurrentBranch(context.Context, port.RepositoryIdentity) (branch.BranchName, error) {
	git.calls = append(git.calls, "current-branch")
	return branch.BranchName{}, detachedCurrentBranchProblem()
}

func (git *detachedLaneWorktreeGit) WorktreeHeadMatchesBase(_ context.Context, _ port.RepositoryIdentity, base branch.TargetBase) (string, string, error) {
	git.calls = append(git.calls, "worktree-head-matches-base")
	git.measuredBase = base
	return git.head, git.baseRevision, nil
}

func TestReleaseLaneDispatchesRejectInvalidRemotesBeforeTheGuard(t *testing.T) {
	t.Parallel()

	t.Run("promotion preparation rejects an invalid remote before the worktree gate", func(t *testing.T) {
		t.Parallel()
		git := newReleaseWhiteboxGit()
		_, err := newReleaseWhiteboxService(git, nil).PrepareReleasePromotion(context.Background(), PrepareReleasePromotionRequest{
			Repository: port.RepositoryIdentity{Root: testRepository().Root, Remote: "invalid remote"},
			Release:    mustBranch("release/1.0.1"),
		})
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
		assertReleaseNoCall(t, git.calls, "linked-worktree")
	})

	t.Run("backmerge preparation rejects an invalid remote before the worktree gate", func(t *testing.T) {
		t.Parallel()
		git := newReleaseWhiteboxGit()
		_, err := newReleaseWhiteboxService(git, nil).PrepareReleaseBackmerge(context.Background(), PrepareReleaseBackmergeRequest{
			Repository: port.RepositoryIdentity{Root: testRepository().Root, Remote: "invalid remote"},
			Release:    mustBranch("release/1.0.1"),
		})
		assertProblemCode(t, err, problem.CodeBranchBaseInvalid)
		assertReleaseNoCall(t, git.calls, "linked-worktree")
	})
}

func TestRequireTaskWorktreeAtBase(t *testing.T) {
	t.Parallel()

	t.Run("keeps the legacy behavior without the worktree capability", func(t *testing.T) {
		t.Parallel()
		if err := requireTaskWorktreeAtBase(context.Background(), &fakeGitRepository{}, testRepository(), mustBase("origin", "release/1.0.1")); err != nil {
			t.Fatalf("requireTaskWorktreeAtBase() = %v, want nil without the capability", err)
		}
	})

	t.Run("binds the drift guard to the supplied lane base", func(t *testing.T) {
		t.Parallel()
		git := &detachedLaneWorktreeGit{fakeWorktreeGit: &fakeWorktreeGit{
			fakeGitRepository: &fakeGitRepository{clean: true},
			linked:            true,
			head:              "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			baseRevision:      "c46015869552bc0433fa2a5276713d74bfc73f87",
		}}
		err := requireTaskWorktreeAtBase(context.Background(), git, testRepository(), mustBase("origin", "release/1.0.1"))
		assertProblemCode(t, err, problem.CodeWorktreeBaseDrift)
		if git.measuredBase.String() != "origin/release/1.0.1" {
			t.Fatalf("measured base = %q, want origin/release/1.0.1", git.measuredBase.String())
		}
	})
}
