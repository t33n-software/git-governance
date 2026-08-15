package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
)

type promotionAlignmentCommandGit struct {
	*commandGit

	missing       bool
	mergeErr      error
	merged        bool
	mergedBase    branch.TargetBase
	mergedMessage commitmsg.Message
	pushErr       error
	pushed        bool
	active        bool
	operation     string
	conflicts     bool
	targetMatches bool
	continued     bool
}

func (git *promotionAlignmentCommandGit) HasMissingBaseCommits(
	context.Context,
	port.RepositoryIdentity,
	branch.TargetBase,
) (bool, error) {
	return git.missing, nil
}

func (git *promotionAlignmentCommandGit) Merge(
	_ context.Context,
	_ port.RepositoryIdentity,
	base branch.TargetBase,
	message commitmsg.Message,
) error {
	if git.mergeErr != nil {
		return git.mergeErr
	}
	git.merged = true
	git.mergedBase = base
	git.mergedMessage = message
	return nil
}

func (git *promotionAlignmentCommandGit) Push(
	_ context.Context,
	_ port.RepositoryIdentity,
	_ branch.BranchName,
	_ bool,
) error {
	if git.pushErr != nil {
		return git.pushErr
	}
	git.pushed = true
	return nil
}

func (git *promotionAlignmentCommandGit) ActiveOperation(context.Context, port.RepositoryIdentity) (string, bool, error) {
	return git.operation, git.active, nil
}

func (git *promotionAlignmentCommandGit) HasUnmergedConflicts(context.Context, port.RepositoryIdentity) (bool, error) {
	return git.conflicts, nil
}

func (git *promotionAlignmentCommandGit) ActiveMergeTargetMatches(
	context.Context,
	port.RepositoryIdentity,
	branch.TargetBase,
) (bool, error) {
	return git.targetMatches, nil
}

func (git *promotionAlignmentCommandGit) ContinueMerge(context.Context, port.RepositoryIdentity) error {
	git.continued = true
	git.active = false
	git.operation = ""
	git.missing = false
	return nil
}

type promotionAlignmentCommandQuality struct {
	calls int
	err   error
}

func (runner *promotionAlignmentCommandQuality) Run(
	context.Context,
	port.RepositoryIdentity,
	port.QualityRequest,
) (port.QualityResult, error) {
	runner.calls++
	return port.QualityResult{Status: port.QualityPassed, Detail: "alignment quality passed"}, runner.err
}

func newPromotionAlignmentCommandGit(t *testing.T) *promotionAlignmentCommandGit {
	t.Helper()

	worker, err := branch.ParseName("chore/GOV-18-align-promotion-base")
	if err != nil {
		t.Fatal(err)
	}
	release, err := branch.ParseName("release/1.0.1")
	if err != nil {
		t.Fatal(err)
	}
	releaseBase, err := branch.NewTargetBase("origin", release)
	if err != nil {
		t.Fatal(err)
	}
	base := newCommandGit(t, worker.String(), nil)
	base.workflowBases = map[string]branch.TargetBase{worker.String(): releaseBase}
	return &promotionAlignmentCommandGit{commandGit: base, missing: true, targetMatches: true}
}

func newPromotionAlignmentCommand(t *testing.T, git port.GitRepository, quality port.QualityRunner) *cobra.Command {
	t.Helper()

	runtime := commandRuntime(git)
	runtime.Quality = quality
	return NewWithRuntime(BuildInfo{Version: "test"}, runtime)
}

func TestReleaseAlignPromotionBaseCommand(t *testing.T) {
	t.Run("reports a read-only alignment plan", func(t *testing.T) {
		git := newPromotionAlignmentCommandGit(t)
		quality := &promotionAlignmentCommandQuality{}
		command := newPromotionAlignmentCommand(t, git, quality)

		output, err := executeBootstrapCommand(
			t,
			command,
			"--interactive", "never", "--output", "json", "--yes", "--dry-run",
			"workflow", "release", "align-promotion-base",
			"--release", "release/1.0.1",
		)
		if err != nil || !strings.Contains(output, `"missingMainCommits":"true"`) ||
			git.merged || quality.calls != 0 {
			t.Fatalf("dry-run = (%q, %v), merged=%t quality=%d", output, err, git.merged, quality.calls)
		}
	})

	t.Run("merges main and reports the quality result", func(t *testing.T) {
		git := newPromotionAlignmentCommandGit(t)
		quality := &promotionAlignmentCommandQuality{}
		command := newPromotionAlignmentCommand(t, git, quality)

		output, err := executeBootstrapCommand(
			t,
			command,
			"--interactive", "never", "--output", "json", "--yes",
			"workflow", "release", "align-promotion-base",
			"--release", "release/1.0.1",
		)
		if err != nil || !git.merged || git.mergedBase.String() != "origin/main" ||
			git.mergedMessage.String() != "chore(GOV-18): align release 1.0.1 with main for promotion" ||
			quality.calls != 1 || !strings.Contains(output, `"qualityStatus":"passed"`) {
			t.Fatalf("alignment = (%q, %v), merged=%t base=%q message=%q quality=%d",
				output, err, git.merged, git.mergedBase, git.mergedMessage, quality.calls)
		}
	})

	t.Run("continues a resolved promotion merge only with --resume", func(t *testing.T) {
		git := newPromotionAlignmentCommandGit(t)
		git.active = true
		git.operation = "merge"
		quality := &promotionAlignmentCommandQuality{}
		command := newPromotionAlignmentCommand(t, git, quality)

		output, err := executeBootstrapCommand(
			t,
			command,
			"--interactive", "never", "--output", "json", "--yes",
			"workflow", "release", "align-promotion-base",
			"--release", "release/1.0.1",
			"--resume",
		)
		if err != nil || !git.continued || quality.calls != 1 ||
			!strings.Contains(output, `"resumed":"true"`) {
			t.Fatalf("resume = (%q, %v), continued=%t quality=%d", output, err, git.continued, quality.calls)
		}
	})

	t.Run("rejects an unsupported branch, release, publication, confirmation, and service state", func(t *testing.T) {
		testCases := []struct {
			name      string
			arguments []string
			newGit    func() port.GitRepository
		}{
			{
				name:      "branch family",
				arguments: []string{"--release", "release/1.0.1"},
				newGit: func() port.GitRepository {
					return newCommandGit(t, "fix/GOV-18-not-release-prep", nil)
				},
			},
			{
				name:      "release line",
				arguments: []string{"--release", "main"},
				newGit: func() port.GitRepository {
					return newPromotionAlignmentCommandGit(t)
				},
			},
			{
				name:      "pull request without push",
				arguments: []string{"--release", "release/1.0.1", "--create-pull-request"},
				newGit: func() port.GitRepository {
					return newPromotionAlignmentCommandGit(t)
				},
			},
			{
				name:      "missing confirmation",
				arguments: []string{"--release", "release/1.0.1"},
				newGit: func() port.GitRepository {
					return newPromotionAlignmentCommandGit(t)
				},
			},
			{
				name:      "missing stabilization provenance",
				arguments: []string{"--release", "release/1.0.1"},
				newGit: func() port.GitRepository {
					git := newPromotionAlignmentCommandGit(t)
					git.workflowBases = nil
					return git
				},
			},
		}
		for _, testCase := range testCases {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				args := []string{"--interactive", "never", "--output", "json"}
				if testCase.name != "missing confirmation" {
					args = append(args, "--yes")
				}
				args = append(args, "workflow", "release", "align-promotion-base")
				args = append(args, testCase.arguments...)
				_, err := executeBootstrapCommand(
					t,
					newPromotionAlignmentCommand(t, testCase.newGit(), &promotionAlignmentCommandQuality{}),
					args...,
				)
				if err == nil {
					t.Fatal("expected command rejection")
				}
			})
		}
	})
}

func TestReleaseAlignPromotionBaseCommandPropagatesDiscoveryAndMergeFailures(t *testing.T) {
	t.Run("discovery", func(t *testing.T) {
		git := &workflowCommandCoverageGit{
			commandGit:  newCommandGit(t, "chore/GOV-18-align-promotion-base", nil),
			discoverErr: errors.New("discover"),
		}
		_, err := executeBootstrapCommand(
			t,
			newPromotionAlignmentCommand(t, git, &promotionAlignmentCommandQuality{}),
			"--interactive", "never", "--output", "json", "--yes",
			"workflow", "release", "align-promotion-base", "--release", "release/1.0.1",
		)
		if err == nil {
			t.Fatal("expected discovery failure")
		}
	})

	t.Run("current branch", func(t *testing.T) {
		git := &workflowCommandCoverageGit{
			commandGit: newCommandGit(t, "chore/GOV-18-align-promotion-base", nil),
			currentErr: errors.New("current"),
		}
		_, err := executeBootstrapCommand(
			t,
			newPromotionAlignmentCommand(t, git, &promotionAlignmentCommandQuality{}),
			"--interactive", "never", "--output", "json", "--yes",
			"workflow", "release", "align-promotion-base", "--release", "release/1.0.1",
		)
		if err == nil {
			t.Fatal("expected current branch failure")
		}
	})

	t.Run("alignment merge", func(t *testing.T) {
		git := newPromotionAlignmentCommandGit(t)
		mergeErr := errors.New("merge")
		git.mergeErr = mergeErr
		_, err := executeBootstrapCommand(
			t,
			newPromotionAlignmentCommand(t, git, &promotionAlignmentCommandQuality{}),
			"--interactive", "never", "--output", "json", "--yes",
			"workflow", "release", "align-promotion-base", "--release", "release/1.0.1",
		)
		if !errors.Is(err, mergeErr) {
			t.Fatalf("merge error = %v, want %v", err, mergeErr)
		}
	})
}

var _ port.GitRepository = (*promotionAlignmentCommandGit)(nil)
var _ port.QualityRunner = (*promotionAlignmentCommandQuality)(nil)
