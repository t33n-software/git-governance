package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/CyberT33N/git-governance/internal/application/port"
	"github.com/CyberT33N/git-governance/internal/domain/branch"
	"github.com/CyberT33N/git-governance/internal/domain/commitmsg"
	"github.com/spf13/cobra"
)

type reconciliationAlignmentCommandGit struct {
	*commandGit

	missing       bool
	mergeErr      error
	merged        bool
	mergedBase    branch.TargetBase
	mergedMessage commitmsg.Message
	operation     string
	active        bool
	conflicts     bool
	continueErr   error
	continued     bool
	pushErr       error
	pushed        bool
}

func (git *reconciliationAlignmentCommandGit) HasMissingBaseCommits(
	context.Context,
	port.RepositoryIdentity,
	branch.TargetBase,
) (bool, error) {
	return git.missing, nil
}

func (git *reconciliationAlignmentCommandGit) Merge(
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

func (git *reconciliationAlignmentCommandGit) ActiveOperation(
	context.Context,
	port.RepositoryIdentity,
) (string, bool, error) {
	return git.operation, git.active, nil
}

func (git *reconciliationAlignmentCommandGit) HasUnmergedConflicts(
	context.Context,
	port.RepositoryIdentity,
) (bool, error) {
	return git.conflicts, nil
}

func (git *reconciliationAlignmentCommandGit) ContinueMerge(
	context.Context,
	port.RepositoryIdentity,
) error {
	if git.continueErr != nil {
		return git.continueErr
	}
	git.continued = true
	return nil
}

func (git *reconciliationAlignmentCommandGit) Push(
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

type reconciliationAlignmentCommandQuality struct {
	calls int
	err   error
}

func (runner *reconciliationAlignmentCommandQuality) Run(
	context.Context,
	port.RepositoryIdentity,
	port.QualityRequest,
) (port.QualityResult, error) {
	runner.calls++
	return port.QualityResult{Status: port.QualityPassed, Detail: "alignment quality passed"}, runner.err
}

func newReconciliationAlignmentCommandGit(t *testing.T) *reconciliationAlignmentCommandGit {
	t.Helper()

	worker, err := branch.ParseName("chore/GOV-20-align-release-reconciliation-base")
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
	return &reconciliationAlignmentCommandGit{commandGit: base, missing: true}
}

func newReconciliationAlignmentCommand(
	t *testing.T,
	git port.GitRepository,
	quality port.QualityRunner,
) *cobra.Command {
	t.Helper()

	runtime := commandRuntime(git)
	runtime.Quality = quality
	runtime.Publisher = &workflowRecordingPublisher{}
	return NewWithRuntime(BuildInfo{Version: "test"}, runtime)
}

func TestReleaseAlignReconciliationBaseCommand(t *testing.T) {
	t.Run("reports a read-only alignment plan", func(t *testing.T) {
		git := newReconciliationAlignmentCommandGit(t)
		quality := &reconciliationAlignmentCommandQuality{}
		command := newReconciliationAlignmentCommand(t, git, quality)

		output, err := executeBootstrapCommand(
			t,
			command,
			"--interactive", "never", "--output", "json", "--yes", "--dry-run",
			"workflow", "release", "align-reconciliation-base",
			"--release", "release/1.0.1",
		)
		if err != nil || !strings.Contains(output, `"missingDevelopCommits":"true"`) ||
			git.merged || quality.calls != 0 {
			t.Fatalf("dry-run = (%q, %v), merged=%t quality=%d", output, err, git.merged, quality.calls)
		}
	})

	t.Run("merges develop and reports the quality result", func(t *testing.T) {
		git := newReconciliationAlignmentCommandGit(t)
		quality := &reconciliationAlignmentCommandQuality{}
		command := newReconciliationAlignmentCommand(t, git, quality)

		output, err := executeBootstrapCommand(
			t,
			command,
			"--interactive", "never", "--output", "json", "--yes",
			"workflow", "release", "align-reconciliation-base",
			"--release", "release/1.0.1",
		)
		if err != nil || !git.merged || git.mergedBase.String() != "origin/develop" ||
			git.mergedMessage.String() != "chore(GOV-20): align release 1.0.1 with develop for reconciliation" ||
			quality.calls != 1 || !strings.Contains(output, `"qualityStatus":"passed"`) {
			t.Fatalf("alignment = (%q, %v), merged=%t base=%q message=%q quality=%d",
				output, err, git.merged, git.mergedBase, git.mergedMessage, quality.calls)
		}
	})

	t.Run("continues a resolved reconciliation merge", func(t *testing.T) {
		git := newReconciliationAlignmentCommandGit(t)
		git.missing = false
		git.operation = "merge"
		git.active = true
		quality := &reconciliationAlignmentCommandQuality{}
		command := newReconciliationAlignmentCommand(t, git, quality)

		output, err := executeBootstrapCommand(
			t,
			command,
			"--interactive", "never", "--output", "json", "--yes",
			"workflow", "release", "align-reconciliation-base",
			"--release", "release/1.0.1", "--resume",
		)
		if err != nil || !git.continued || quality.calls != 1 ||
			!strings.Contains(output, `"resumed":"true"`) {
			t.Fatalf("resume = (%q, %v), continued=%t quality=%d", output, err, git.continued, quality.calls)
		}
	})

	t.Run("publishes only with an explicit push", func(t *testing.T) {
		git := newReconciliationAlignmentCommandGit(t)
		quality := &reconciliationAlignmentCommandQuality{}
		command := newReconciliationAlignmentCommand(t, git, quality)

		output, err := executeBootstrapCommand(
			t,
			command,
			"--interactive", "never", "--output", "json", "--yes", "--pull-request-provider", "github",
			"workflow", "release", "align-reconciliation-base",
			"--release", "release/1.0.1", "--push", "--create-pull-request",
		)
		if err != nil || !git.pushed || !strings.Contains(output, `"pushed":"true"`) {
			t.Fatalf("publication = (%q, %v), pushed=%t", output, err, git.pushed)
		}
	})

	t.Run("rejects unsafe command inputs", func(t *testing.T) {
		testCases := []struct {
			name      string
			arguments []string
			newGit    func() port.GitRepository
		}{
			{
				name:      "branch family",
				arguments: []string{"--release", "release/1.0.1"},
				newGit: func() port.GitRepository {
					return newCommandGit(t, "fix/GOV-20-not-release-prep", nil)
				},
			},
			{
				name:      "release line",
				arguments: []string{"--release", "main"},
				newGit: func() port.GitRepository {
					return newReconciliationAlignmentCommandGit(t)
				},
			},
			{
				name:      "pull request without push",
				arguments: []string{"--release", "release/1.0.1", "--create-pull-request"},
				newGit: func() port.GitRepository {
					return newReconciliationAlignmentCommandGit(t)
				},
			},
			{
				name:      "missing confirmation",
				arguments: []string{"--release", "release/1.0.1"},
				newGit: func() port.GitRepository {
					return newReconciliationAlignmentCommandGit(t)
				},
			},
			{
				name:      "missing preparation provenance",
				arguments: []string{"--release", "release/1.0.1"},
				newGit: func() port.GitRepository {
					git := newReconciliationAlignmentCommandGit(t)
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
				args = append(args, "workflow", "release", "align-reconciliation-base")
				args = append(args, testCase.arguments...)
				_, err := executeBootstrapCommand(
					t,
					newReconciliationAlignmentCommand(t, testCase.newGit(), &reconciliationAlignmentCommandQuality{}),
					args...,
				)
				if err == nil {
					t.Fatal("expected command rejection")
				}
			})
		}
	})
}

func TestReleaseAlignReconciliationBaseCommandPropagatesFailures(t *testing.T) {
	t.Run("discovery", func(t *testing.T) {
		git := &workflowCommandCoverageGit{
			commandGit:  newCommandGit(t, "chore/GOV-20-align-release-reconciliation-base", nil),
			discoverErr: errors.New("discover"),
		}
		_, err := executeBootstrapCommand(
			t,
			newReconciliationAlignmentCommand(t, git, &reconciliationAlignmentCommandQuality{}),
			"--interactive", "never", "--output", "json", "--yes",
			"workflow", "release", "align-reconciliation-base", "--release", "release/1.0.1",
		)
		if err == nil {
			t.Fatal("expected discovery failure")
		}
	})

	t.Run("current branch", func(t *testing.T) {
		git := &workflowCommandCoverageGit{
			commandGit: newCommandGit(t, "chore/GOV-20-align-release-reconciliation-base", nil),
			currentErr: errors.New("current"),
		}
		release, _ := branch.ParseName("release/1.0.1")
		base, _ := branch.NewTargetBase("origin", release)
		git.workflowBases = map[string]branch.TargetBase{git.current.String(): base}
		_, err := executeBootstrapCommand(
			t,
			newReconciliationAlignmentCommand(t, git, &reconciliationAlignmentCommandQuality{}),
			"--interactive", "never", "--output", "json", "--yes",
			"workflow", "release", "align-reconciliation-base", "--release", "release/1.0.1",
		)
		if err == nil {
			t.Fatal("expected current branch failure")
		}
	})

	t.Run("alignment merge", func(t *testing.T) {
		git := newReconciliationAlignmentCommandGit(t)
		mergeErr := errors.New("merge")
		git.mergeErr = mergeErr
		_, err := executeBootstrapCommand(
			t,
			newReconciliationAlignmentCommand(t, git, &reconciliationAlignmentCommandQuality{}),
			"--interactive", "never", "--output", "json", "--yes",
			"workflow", "release", "align-reconciliation-base", "--release", "release/1.0.1",
		)
		if !errors.Is(err, mergeErr) {
			t.Fatalf("merge error = %v, want %v", err, mergeErr)
		}
	})
}

var _ port.GitRepository = (*reconciliationAlignmentCommandGit)(nil)
var _ port.QualityRunner = (*reconciliationAlignmentCommandQuality)(nil)
