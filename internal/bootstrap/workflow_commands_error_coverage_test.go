package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/application/workflow"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

type workflowCommandCoverageGit struct {
	*commandGit

	discoverErr error
	currentErr  error
	exists      bool
	identity    *port.RepositoryIdentity
	hasCommits  *bool
	releaseTags []string
}

type workflowExecutionFailureGit struct {
	*workflowCommandCoverageGit
	createErr error
}

func (git *workflowExecutionFailureGit) CreateBranch(
	ctx context.Context,
	repository port.RepositoryIdentity,
	name branch.BranchName,
	base branch.TargetBase,
	switchTo bool,
) error {
	if git.createErr != nil {
		return git.createErr
	}
	return git.workflowCommandCoverageGit.CreateBranch(ctx, repository, name, base, switchTo)
}

type workflowFailurePublisher struct {
	err error
}

func (publisher workflowFailurePublisher) Publish(context.Context, port.PullRequestPublication) (port.PublishedPullRequest, error) {
	return port.PublishedPullRequest{}, publisher.err
}

func (workflowFailurePublisher) DispatchSharedLine(
	context.Context,
	port.SharedLineDispatchRequest,
) (port.SharedLineDispatchResult, error) {
	return port.SharedLineDispatchResult{}, nil
}

func (workflowFailurePublisher) VerifyReleaseReconciliation(
	_ context.Context,
	request port.ReleaseReconciliationRequest,
) (port.ReleaseReconciliationEvidence, error) {
	version, _ := request.Release.ReleaseVersion()
	return port.ReleaseReconciliationEvidence{
		PromotionPullRequestURL: "https://example.invalid/pr/release",
		PromotionMergeCommit:    strings.Repeat("a", 40),
		Tag:                     "v" + version.String(),
		ReleaseURL:              "https://example.invalid/releases/" + version.String(),
		EffectiveDelta:          true,
	}, nil
}

type workflowPreflightFailurePublisher struct {
	err error
}

func (publisher workflowPreflightFailurePublisher) Publish(context.Context, port.PullRequestPublication) (port.PublishedPullRequest, error) {
	return port.PublishedPullRequest{}, nil
}

func (publisher workflowPreflightFailurePublisher) Validate(context.Context, port.PullRequestPublication) error {
	return publisher.err
}

func (workflowPreflightFailurePublisher) DispatchSharedLine(
	context.Context,
	port.SharedLineDispatchRequest,
) (port.SharedLineDispatchResult, error) {
	return port.SharedLineDispatchResult{}, nil
}

func (workflowPreflightFailurePublisher) VerifyReleaseReconciliation(
	_ context.Context,
	request port.ReleaseReconciliationRequest,
) (port.ReleaseReconciliationEvidence, error) {
	version, _ := request.Release.ReleaseVersion()
	return port.ReleaseReconciliationEvidence{
		PromotionPullRequestURL: "https://example.invalid/pr/release",
		PromotionMergeCommit:    strings.Repeat("a", 40),
		Tag:                     "v" + version.String(),
		ReleaseURL:              "https://example.invalid/releases/" + version.String(),
		EffectiveDelta:          true,
	}, nil
}

func (git *workflowCommandCoverageGit) Discover(context.Context, string) (port.RepositoryIdentity, error) {
	if git.discoverErr != nil {
		return port.RepositoryIdentity{}, git.discoverErr
	}
	if git.identity != nil {
		return *git.identity, nil
	}
	return git.commandGit.Discover(context.Background(), "")
}

func (git *workflowCommandCoverageGit) CurrentBranch(context.Context, port.RepositoryIdentity) (branch.BranchName, error) {
	if git.currentErr != nil {
		return branch.BranchName{}, git.currentErr
	}
	return git.commandGit.CurrentBranch(context.Background(), port.RepositoryIdentity{})
}

func (git *workflowCommandCoverageGit) BranchExists(context.Context, port.RepositoryIdentity, branch.BranchName) (bool, error) {
	return git.exists, nil
}

func (git *workflowCommandCoverageGit) HasCommits(context.Context, port.RepositoryIdentity) (bool, error) {
	if git.hasCommits != nil {
		return *git.hasCommits, nil
	}
	return git.commandGit.HasCommits(context.Background(), port.RepositoryIdentity{})
}

func (git *workflowCommandCoverageGit) ReleaseTagsAt(context.Context, port.RepositoryIdentity, string) ([]string, error) {
	if git.releaseTags != nil {
		return append([]string(nil), git.releaseTags...), nil
	}
	return git.commandGit.ReleaseTagsAt(context.Background(), port.RepositoryIdentity{}, "")
}

func newWorkflowCommandCoverageGit(t *testing.T, current string, messages []string) *workflowCommandCoverageGit {
	t.Helper()
	return &workflowCommandCoverageGit{
		commandGit: newCommandGit(t, current, messages),
	}
}

func executeWorkflowCoverageCommand(
	t *testing.T,
	git port.GitRepository,
	arguments ...string,
) (string, error) {
	t.Helper()
	command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
	root := []string{"--interactive", "never", "--output", "json"}
	return executeBootstrapCommand(t, command, append(root, arguments...)...)
}

func TestWorkflowCommandsPropagateRepositoryDiscoveryFailures(t *testing.T) {
	discoverErr := errors.New("repository discovery failed")
	commands := [][]string{
		{"workflow", "ticket", "start"},
		{"workflow", "ticket", "publish"},
		{"workflow", "hotfix", "start"},
		{"workflow", "hotfix", "publish"},
		{"workflow", "hotfix", "propagate"},
		{"workflow", "cleanup"},
		{"workflow", "release", "cut"},
		{"workflow", "release", "stabilize"},
		{"workflow", "release", "publish-stabilization"},
		{"workflow", "release", "promote"},
		{"workflow", "release", "backmerge"},
		{"workflow", "release", "support"},
	}

	for _, arguments := range commands {
		arguments := arguments
		t.Run(strings.Join(arguments, "-"), func(t *testing.T) {
			git := newWorkflowCommandCoverageGit(t, "feature/ABC-123-add-export", nil)
			git.discoverErr = discoverErr
			_, err := executeWorkflowCoverageCommand(t, git, arguments...)
			if !errors.Is(err, discoverErr) {
				t.Fatalf("%v error = %v, want %v", arguments, err, discoverErr)
			}
		})
	}
}

func TestWorkflowCommandsPropagateCurrentBranchFailures(t *testing.T) {
	currentErr := errors.New("current branch unavailable")
	testCases := [][]string{
		{"workflow", "ticket", "publish", "--yes", "--dry-run"},
		{"workflow", "hotfix", "publish", "--affected-line", "main", "--yes", "--dry-run"},
		{"workflow", "hotfix", "propagate", "--target-line", "develop", "--commit", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "--yes", "--dry-run"},
		{"workflow", "cleanup", "--yes", "--dry-run"},
		{"workflow", "release", "publish-stabilization", "--release", "release/2.8.0", "--yes", "--dry-run"},
	}

	for _, arguments := range testCases {
		arguments := arguments
		t.Run(strings.Join(arguments[:2], "-"), func(t *testing.T) {
			git := newWorkflowCommandCoverageGit(t, "feature/ABC-123-add-export", nil)
			git.currentErr = currentErr
			_, err := executeWorkflowCoverageCommand(t, git, arguments...)
			if !errors.Is(err, currentErr) {
				t.Fatalf("%v error = %v, want %v", arguments, err, currentErr)
			}
		})
	}
}

func TestWorkflowCommandsRejectInputAndServiceFailurePaths(t *testing.T) {
	testCases := []struct {
		name      string
		current   string
		messages  []string
		args      []string
		configure func(*workflowCommandCoverageGit)
	}{
		{
			name:    "ticket start missing family",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "ticket", "start", "--yes", "--dry-run"},
		},
		{
			name:    "ticket start missing key",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "ticket", "start", "--family", "feature", "--yes", "--dry-run"},
		},
		{
			name:    "ticket start missing number",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "ticket", "start", "--family", "feature", "--key", "ABC", "--yes", "--dry-run"},
		},
		{
			name:    "ticket start missing slug",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "ticket", "start", "--family", "feature", "--key", "ABC", "--ticket", "123", "--yes", "--dry-run"},
		},
		{
			name:    "ticket start invalid scratch slug",
			current: "feature/ABC-123-add-export",
			args: []string{
				"workflow", "ticket", "start", "--family", "feature", "--key", "ABC", "--ticket", "123", "--slug", "add-export",
				"--scratch", "--scratch-slug", "Invalid Scratch", "--yes", "--dry-run",
			},
		},
		{
			name:    "ticket start requires confirmation",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "ticket", "start", "--family", "feature", "--key", "ABC", "--ticket", "123", "--slug", "add-export"},
		},
		{
			name:    "ticket start rejects existing branch",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "ticket", "start", "--family", "feature", "--key", "ABC", "--ticket", "123", "--slug", "add-export", "--yes"},
			configure: func(git *workflowCommandCoverageGit) {
				git.exists = true
			},
		},
		{
			name:    "ticket publish rejects malformed branch",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "ticket", "publish", "--branch", "not-a-branch", "--yes", "--dry-run"},
		},
		{
			name:    "ticket publish rejects invalid base",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "ticket", "publish", "--branch", "feature/ABC-123-add-export", "--base", "invalid remote/develop", "--yes", "--dry-run"},
		},
		{
			name:    "ticket publish requires confirmation",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "ticket", "publish", "--branch", "feature/ABC-123-add-export"},
		},
		{
			name:    "ticket publish rejects shared line",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "ticket", "publish", "--branch", "main", "--yes", "--dry-run"},
		},
		{
			name:    "hotfix start missing key",
			current: "hotfix/ABC-999-payment-timeout",
			args:    []string{"workflow", "hotfix", "start", "--yes", "--dry-run"},
		},
		{
			name:    "hotfix start missing number",
			current: "hotfix/ABC-999-payment-timeout",
			args:    []string{"workflow", "hotfix", "start", "--key", "ABC", "--yes", "--dry-run"},
		},
		{
			name:    "hotfix start missing slug",
			current: "hotfix/ABC-999-payment-timeout",
			args:    []string{"workflow", "hotfix", "start", "--key", "ABC", "--ticket", "999", "--yes", "--dry-run"},
		},
		{
			name:    "hotfix start missing affected line",
			current: "hotfix/ABC-999-payment-timeout",
			args:    []string{"workflow", "hotfix", "start", "--key", "ABC", "--ticket", "999", "--slug", "payment-timeout", "--yes", "--dry-run"},
		},
		{
			name:    "hotfix start rejects malformed affected line",
			current: "hotfix/ABC-999-payment-timeout",
			args:    []string{"workflow", "hotfix", "start", "--key", "ABC", "--ticket", "999", "--slug", "payment-timeout", "--affected-line", "invalid", "--yes", "--dry-run"},
		},
		{
			name:    "hotfix start requires confirmation",
			current: "hotfix/ABC-999-payment-timeout",
			args:    []string{"workflow", "hotfix", "start", "--key", "ABC", "--ticket", "999", "--slug", "payment-timeout", "--affected-line", "main"},
		},
		{
			name:    "hotfix start rejects develop",
			current: "hotfix/ABC-999-payment-timeout",
			args:    []string{"workflow", "hotfix", "start", "--key", "ABC", "--ticket", "999", "--slug", "payment-timeout", "--affected-line", "develop", "--yes", "--dry-run"},
		},
		{
			name:    "hotfix publish rejects regular branch",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "hotfix", "publish", "--branch", "feature/ABC-123-add-export", "--affected-line", "main", "--yes", "--dry-run"},
		},
		{
			name:    "hotfix publish missing affected line",
			current: "hotfix/ABC-999-payment-timeout",
			args:    []string{"workflow", "hotfix", "publish", "--branch", "hotfix/ABC-999-payment-timeout", "--yes", "--dry-run"},
		},
		{
			name:    "hotfix publish rejects malformed affected line",
			current: "hotfix/ABC-999-payment-timeout",
			args:    []string{"workflow", "hotfix", "publish", "--branch", "hotfix/ABC-999-payment-timeout", "--affected-line", "invalid", "--yes", "--dry-run"},
		},
		{
			name:    "hotfix publish rejects develop affected line",
			current: "hotfix/ABC-999-payment-timeout",
			args:    []string{"workflow", "hotfix", "publish", "--branch", "hotfix/ABC-999-payment-timeout", "--affected-line", "develop", "--yes", "--dry-run"},
		},
		{
			name:    "hotfix publish requires confirmation",
			current: "hotfix/ABC-999-payment-timeout",
			args:    []string{"workflow", "hotfix", "publish", "--branch", "hotfix/ABC-999-payment-timeout", "--affected-line", "main"},
		},
		{
			name:    "hotfix publish rejects invalid remote base",
			current: "hotfix/ABC-999-payment-timeout",
			args:    []string{"--remote", "invalid remote", "workflow", "hotfix", "publish", "--branch", "hotfix/ABC-999-payment-timeout", "--affected-line", "main", "--yes", "--dry-run"},
		},
		{
			name:    "hotfix publish stops on invalid series",
			current: "hotfix/ABC-999-payment-timeout",
			args:    []string{"workflow", "hotfix", "publish", "--branch", "hotfix/ABC-999-payment-timeout", "--affected-line", "main", "--yes", "--dry-run"},
		},
		{
			name:    "hotfix propagation rejects regular source",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "hotfix", "propagate", "--source", "feature/ABC-123-add-export", "--target-line", "develop", "--commit", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "--yes", "--dry-run"},
		},
		{
			name:    "hotfix propagation missing target",
			current: "hotfix/ABC-999-payment-timeout",
			args:    []string{"workflow", "hotfix", "propagate", "--source", "hotfix/ABC-999-payment-timeout", "--yes", "--dry-run"},
		},
		{
			name:    "hotfix propagation rejects malformed target",
			current: "hotfix/ABC-999-payment-timeout",
			args:    []string{"workflow", "hotfix", "propagate", "--source", "hotfix/ABC-999-payment-timeout", "--target-line", "invalid", "--yes", "--dry-run"},
		},
		{
			name:    "hotfix propagation missing commit",
			current: "hotfix/ABC-999-payment-timeout",
			args:    []string{"workflow", "hotfix", "propagate", "--source", "hotfix/ABC-999-payment-timeout", "--target-line", "develop", "--yes", "--dry-run"},
		},
		{
			name:    "hotfix propagation rejects invalid slug",
			current: "hotfix/ABC-999-payment-timeout",
			args:    []string{"workflow", "hotfix", "propagate", "--source", "hotfix/ABC-999-payment-timeout", "--target-line", "develop", "--commit", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "--slug", "Invalid Slug", "--yes", "--dry-run"},
		},
		{
			name:    "hotfix propagation requires confirmation",
			current: "hotfix/ABC-999-payment-timeout",
			args:    []string{"workflow", "hotfix", "propagate", "--source", "hotfix/ABC-999-payment-timeout", "--target-line", "develop", "--commit", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		},
		{
			name:    "hotfix propagation rejects nonshared target",
			current: "hotfix/ABC-999-payment-timeout",
			args:    []string{"workflow", "hotfix", "propagate", "--source", "hotfix/ABC-999-payment-timeout", "--target-line", "feature/ABC-123-add-export", "--commit", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "--yes", "--dry-run"},
		},
		{
			name:    "cleanup rejects malformed branch",
			current: "scratch/ABC-123-export-exploration",
			args:    []string{"workflow", "cleanup", "--branch", "invalid", "--yes", "--dry-run"},
		},
		{
			name:    "cleanup requires confirmation",
			current: "scratch/ABC-123-export-exploration",
			args:    []string{"workflow", "cleanup", "--branch", "scratch/ABC-123-export-exploration"},
		},
		{
			name:    "cleanup rejects official branch",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "cleanup", "--branch", "feature/ABC-123-add-export", "--yes", "--dry-run"},
		},
		{
			name:    "release cut missing version",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "release", "cut", "--yes", "--dry-run"},
		},
		{
			name:    "release cut rejects malformed version",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "release", "cut", "--version", "invalid", "--yes", "--dry-run"},
		},
		{
			name:    "release cut requires confirmation",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "release", "cut", "--version", "2.8.0"},
		},
		{
			name:    "release cut rejects an empty repository",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "release", "cut", "--version", "2.8.0", "--yes"},
			configure: func(git *workflowCommandCoverageGit) {
				hasCommits := false
				git.hasCommits = &hasCommits
			},
		},
		{
			name:    "release stabilize missing release",
			current: "fix/ABC-999-release-blocker",
			args:    []string{"workflow", "release", "stabilize", "--yes", "--dry-run"},
		},
		{
			name:    "release stabilize rejects malformed release",
			current: "fix/ABC-999-release-blocker",
			args:    []string{"workflow", "release", "stabilize", "--release", "invalid", "--yes", "--dry-run"},
		},
		{
			name:    "release stabilize missing kind",
			current: "fix/ABC-999-release-blocker",
			args:    []string{"workflow", "release", "stabilize", "--release", "release/2.8.0", "--yes", "--dry-run"},
		},
		{
			name:    "release stabilize missing key",
			current: "fix/ABC-999-release-blocker",
			args:    []string{"workflow", "release", "stabilize", "--release", "release/2.8.0", "--kind", "blocker", "--yes", "--dry-run"},
		},
		{
			name:    "release stabilize missing ticket",
			current: "fix/ABC-999-release-blocker",
			args:    []string{"workflow", "release", "stabilize", "--release", "release/2.8.0", "--kind", "blocker", "--key", "ABC", "--yes", "--dry-run"},
		},
		{
			name:    "release stabilize missing slug",
			current: "fix/ABC-999-release-blocker",
			args:    []string{"workflow", "release", "stabilize", "--release", "release/2.8.0", "--kind", "blocker", "--key", "ABC", "--ticket", "999", "--yes", "--dry-run"},
		},
		{
			name:    "release stabilize requires confirmation",
			current: "fix/ABC-999-release-blocker",
			args:    []string{"workflow", "release", "stabilize", "--release", "release/2.8.0", "--kind", "blocker", "--key", "ABC", "--ticket", "999", "--slug", "release-blocker"},
		},
		{
			name:    "release stabilize rejects nonrelease source",
			current: "fix/ABC-999-release-blocker",
			args:    []string{"workflow", "release", "stabilize", "--release", "feature/ABC-123-add-export", "--kind", "blocker", "--key", "ABC", "--ticket", "999", "--slug", "release-blocker", "--yes", "--dry-run"},
		},
		{
			name:    "release publish rejects regular branch",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "release", "publish-stabilization", "--branch", "feature/ABC-123-add-export", "--release", "release/2.8.0", "--yes", "--dry-run"},
		},
		{
			name:    "release publish missing release",
			current: "fix/ABC-999-release-blocker",
			args:    []string{"workflow", "release", "publish-stabilization", "--branch", "fix/ABC-999-release-blocker", "--yes", "--dry-run"},
		},
		{
			name:    "release publish rejects malformed release",
			current: "fix/ABC-999-release-blocker",
			args:    []string{"workflow", "release", "publish-stabilization", "--branch", "fix/ABC-999-release-blocker", "--release", "invalid", "--yes", "--dry-run"},
		},
		{
			name:    "release publish rejects nonrelease target",
			current: "fix/ABC-999-release-blocker",
			args:    []string{"workflow", "release", "publish-stabilization", "--branch", "fix/ABC-999-release-blocker", "--release", "support/2.8", "--yes", "--dry-run"},
		},
		{
			name:    "release publish requires confirmation",
			current: "fix/ABC-999-release-blocker",
			args:    []string{"workflow", "release", "publish-stabilization", "--branch", "fix/ABC-999-release-blocker", "--release", "release/2.8.0"},
		},
		{
			name:    "release publish rejects invalid remote base",
			current: "fix/ABC-999-release-blocker",
			args:    []string{"--remote", "invalid remote", "workflow", "release", "publish-stabilization", "--branch", "fix/ABC-999-release-blocker", "--release", "release/2.8.0", "--yes", "--dry-run"},
		},
		{
			name:    "release publish stops on invalid series",
			current: "fix/ABC-999-release-blocker",
			args:    []string{"workflow", "release", "publish-stabilization", "--branch", "fix/ABC-999-release-blocker", "--release", "release/2.8.0", "--yes", "--dry-run"},
		},
		{
			name:    "release promotion missing release",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "release", "promote", "--yes", "--dry-run"},
		},
		{
			name:    "release promotion rejects malformed release",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "release", "promote", "--release", "invalid", "--yes", "--dry-run"},
		},
		{
			name:    "release promotion rejects nonrelease source",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "release", "promote", "--release", "feature/ABC-123-add-export", "--yes", "--dry-run"},
		},
		{
			name:    "release backmerge missing release",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "release", "backmerge", "--yes", "--dry-run"},
		},
		{
			name:    "release backmerge rejects malformed release",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "release", "backmerge", "--release", "invalid", "--yes", "--dry-run"},
		},
		{
			name:    "release backmerge rejects nonrelease source",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "release", "backmerge", "--release", "feature/ABC-123-add-export", "--yes", "--dry-run"},
		},
		{
			name:    "support preparation missing version",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "release", "support", "--yes", "--dry-run"},
		},
		{
			name:    "support preparation rejects malformed version",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "release", "support", "--version", "invalid", "--yes", "--dry-run"},
		},
		{
			name:    "support preparation requires confirmation",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "release", "support", "--version", "2.8"},
		},
		{
			name:    "support preparation rejects unavailable release tag",
			current: "feature/ABC-123-add-export",
			args:    []string{"workflow", "release", "support", "--version", "3.0", "--yes"},
			configure: func(git *workflowCommandCoverageGit) {
				git.releaseTags = []string{"v2.8.0"}
			},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			git := newWorkflowCommandCoverageGit(t, testCase.current, testCase.messages)
			if testCase.configure != nil {
				testCase.configure(git)
			}
			_, err := executeWorkflowCoverageCommand(t, git, testCase.args...)
			if err == nil {
				t.Fatalf("%s unexpectedly succeeded", testCase.name)
			}
		})
	}
}

func TestTicketStartPromptFailureAndQualityFieldCoverage(t *testing.T) {
	t.Run("propagates the optional scratch prompt failure", func(t *testing.T) {
		promptErr := errors.New("scratch prompt unavailable")
		git := newWorkflowCommandCoverageGit(t, "feature/ABC-123-add-export", nil)
		runtime := commandRuntime(git)
		runtime.PromptFactory = func(bool, string) port.Prompt {
			return &commandHelperPrompt{confirms: []commandHelperConfirmReply{{err: promptErr}}}
		}
		runtime.InputIsTerminal = func() bool { return true }
		runtime.OutputIsTerminal = func() bool { return true }
		command := NewWithRuntime(BuildInfo{Version: "test"}, runtime)

		_, err := executeBootstrapCommand(
			t,
			command,
			"--interactive", "auto",
			"--output", "human",
			"workflow", "ticket", "start",
			"--family", "feature",
			"--key", "ABC",
			"--ticket", "123",
			"--slug", "add-export",
		)
		if !errors.Is(err, promptErr) {
			t.Fatalf("ticket start error = %v, want %v", err, promptErr)
		}
	})

	t.Run("includes post-mutation quality details when present", func(t *testing.T) {
		fields := map[string]string{}
		status := port.QualityResult{Status: port.QualityPassed, Detail: "post-mutation checks passed"}
		addQualityFields(fields, workflowPublishResultWithPostMutationQuality(status))
		if fields["postMutationQualityStatus"] != "passed" || fields["postMutationQualityDetail"] != "post-mutation checks passed" {
			t.Fatalf("quality fields = %#v", fields)
		}
	})
}

func TestWorkflowCommandsAttachInputsToPostValidationFailures(t *testing.T) {
	createErr := errors.New("branch creation failed")
	testCases := []struct {
		name    string
		current string
		args    []string
	}{
		{
			name:    "ticket start includes scratch slug",
			current: "feature/ABC-123-add-export",
			args: []string{
				"workflow", "ticket", "start",
				"--family", "feature", "--key", "ABC", "--ticket", "123", "--slug", "add-export",
				"--scratch", "--scratch-slug", "exploration", "--yes",
			},
		},
		{
			name:    "hotfix start",
			current: "feature/ABC-123-add-export",
			args: []string{
				"workflow", "hotfix", "start",
				"--key", "ABC", "--ticket", "999", "--slug", "payment-timeout", "--affected-line", "main", "--yes",
			},
		},
		{
			name:    "hotfix propagation",
			current: "hotfix/ABC-999-payment-timeout",
			args: []string{
				"workflow", "hotfix", "propagate",
				"--target-line", "develop", "--commit", strings.Repeat("a", 40), "--slug", "forward-port-payment-timeout", "--yes",
			},
		},
		{
			name:    "release stabilization",
			current: "fix/ABC-999-release-blocker",
			args: []string{
				"workflow", "release", "stabilize",
				"--release", "release/2.8.0", "--kind", "blocker",
				"--key", "ABC", "--ticket", "999", "--slug", "release-blocker", "--yes",
			},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			git := &workflowExecutionFailureGit{
				workflowCommandCoverageGit: newWorkflowCommandCoverageGit(t, testCase.current, nil),
				createErr:                  createErr,
			}
			command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
			_, err := executeBootstrapCommand(
				t,
				command,
				append([]string{"--interactive", "never", "--output", "json"}, testCase.args...)...,
			)
			if !errors.Is(err, createErr) {
				t.Fatalf("workflow error = %v, want %v", err, createErr)
			}
			actual, ok := problem.As(err)
			if !ok || len(actual.WorkflowInputs) == 0 {
				t.Fatalf("workflow input summary = %#v", err)
			}
		})
	}

	t.Run("ticket publish records an explicit base", func(t *testing.T) {
		git := newWorkflowCommandCoverageGit(t, "feature/ABC-123-add-export", []string{"feat(ABC-123): add export"})
		output, err := executeWorkflowCoverageCommand(
			t,
			git,
			"workflow", "ticket", "publish",
			"--branch", "feature/ABC-123-add-export", "--base", "origin/develop", "--yes", "--dry-run",
		)
		if err != nil || !strings.Contains(output, `"ok":true`) {
			t.Fatalf("ticket publish = (%q, %v)", output, err)
		}
	})
}

func TestReleasePromotionAndBackmergeRequireTheMandatoryDescription(t *testing.T) {
	for _, arguments := range [][]string{
		{"workflow", "release", "promote", "--release", "release/2.8.0", "--create-pull-request", "--yes"},
		{"workflow", "release", "backmerge", "--release", "release/2.8.0", "--create-pull-request", "--yes"},
	} {
		arguments := arguments
		t.Run(strings.Join(arguments[2:3], "-"), func(t *testing.T) {
			runtime := commandRuntime(newWorkflowCommandCoverageGit(t, "feature/ABC-123-add-export", nil))
			runtime.Publisher = workflowFailurePublisher{err: errors.New("publisher unavailable")}
			command := NewWithRuntime(BuildInfo{Version: "test"}, runtime)
			_, err := executeBootstrapCommand(t, command, append([]string{"--interactive", "never", "--output", "json"}, arguments...)...)
			assertProblemCode(t, err, problem.CodeInvalidInput)
		})
	}
}

func TestReleasePromotionAndBackmergeAttachInputsToPublisherFailures(t *testing.T) {
	publishErr := errors.New("publisher unavailable")
	for _, arguments := range [][]string{
		{"workflow", "release", "promote", "--release", "release/2.8.0", "--create-pull-request", "--yes", "--body", "Summary: Promote release 2.8.0 into main."},
		{"workflow", "release", "backmerge", "--release", "release/2.8.0", "--create-pull-request", "--yes", "--body", "Summary: Backmerge release 2.8.0 into develop."},
	} {
		arguments := arguments
		t.Run(strings.Join(arguments[2:3], "-"), func(t *testing.T) {
			runtime := commandRuntime(newWorkflowCommandCoverageGit(t, "feature/ABC-123-add-export", nil))
			runtime.Publisher = workflowFailurePublisher{err: publishErr}
			command := NewWithRuntime(BuildInfo{Version: "test"}, runtime)
			_, err := executeBootstrapCommand(t, command, append([]string{"--interactive", "never", "--output", "json"}, arguments...)...)
			if !errors.Is(err, publishErr) {
				t.Fatalf("release workflow error = %v, want %v", err, publishErr)
			}
			actual, ok := problem.As(err)
			if !ok || len(actual.WorkflowInputs) != 1 || actual.WorkflowInputs[0].Field != "release branch" {
				t.Fatalf("release workflow input summary = %#v", err)
			}
		})
	}
}

func workflowPublishResultWithPostMutationQuality(status port.QualityResult) workflow.PublishTicketResult {
	return workflow.PublishTicketResult{
		Quality:             port.QualityResult{Status: port.QualityPassed, Detail: "initial checks passed"},
		PostMutationQuality: &status,
	}
}

var _ port.GitRepository = (*workflowCommandCoverageGit)(nil)
