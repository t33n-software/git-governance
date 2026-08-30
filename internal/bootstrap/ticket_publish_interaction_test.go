package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/application/workflow"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestTicketPublishSynchronizationReportPaths(t *testing.T) {
	t.Parallel()

	name := ticketPublishTestBranch(t)
	base := ticketPublishTestBase(t)
	prompt := &commandHelperPrompt{}
	application := newBranchCommandApplication(newBranchCommandGit(t, name.String()), nil, prompt, "human")
	command := &cobra.Command{}
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetContext(context.Background())

	for _, testCase := range []struct {
		action string
		want   string
	}{
		{action: "rebased", want: "Rebase completed successfully"},
		{action: "none", want: "No rebase was performed because the target base has no commits"},
		{action: "merge", want: "No rebase was performed because the branch is already published"},
		{action: "other", want: "Target-base synchronization completed without a rebase"},
	} {
		if err := application.reportTicketSynchronization(command, branchapp.SyncResult{
			Name:              name,
			Base:              base,
			RecommendedAction: testCase.action,
		}, false); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), testCase.want) {
			t.Fatalf("synchronization report missing %q: %q", testCase.want, output.String())
		}
	}

	before := output.Len()
	if err := application.reportTicketSynchronization(command, branchapp.SyncResult{}, true); err != nil {
		t.Fatal(err)
	}
	if output.Len() != before {
		t.Fatalf("dry-run synchronization report wrote output: %q", output.String())
	}
}

func TestTicketPublishRebaseRetryInteractionPaths(t *testing.T) {
	name := ticketPublishTestBranch(t)
	base := ticketPublishTestBase(t)

	t.Run("returns prompt failures and cancellation", func(t *testing.T) {
		cancelPrompt := &commandHelperPrompt{
			selects: []commandHelperStringReply{{value: "cancel"}},
		}
		application := newBranchCommandApplication(newBranchCommandGit(t, name.String()), nil, cancelPrompt, "human")
		_, err := application.resumeTicketPublishAfterRebaseConflict(
			context.Background(),
			services{},
			port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"},
			name,
			&base,
			false,
		)
		assertProblemCode(t, err, problem.CodeOperationCancelled)

		promptErr := errors.New("selection unavailable")
		failingPrompt := &commandHelperPrompt{
			selects: []commandHelperStringReply{{err: promptErr}},
		}
		application = newBranchCommandApplication(newBranchCommandGit(t, name.String()), nil, failingPrompt, "human")
		_, err = application.resumeTicketPublishAfterRebaseConflict(
			context.Background(),
			services{},
			port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"},
			name,
			&base,
			false,
		)
		if !errors.Is(err, promptErr) {
			t.Fatalf("retry prompt error = %v, want %v", err, promptErr)
		}
	})

	t.Run("repeats retry while Git remains conflicted", func(t *testing.T) {
		git := newBranchCommandGit(t, name.String())
		git.messages = []string{"feat(ABC-123): add export"}
		git.publication = branch.PublicationUnpublished
		git.active = true
		git.activeOperation = "rebase"
		git.continueRebaseErrors = []error{errors.New("still conflicted"), nil}
		prompt := &commandHelperPrompt{
			selects: []commandHelperStringReply{{value: "retry"}, {value: "retry"}},
		}
		application := newBranchCommandApplication(git, nil, prompt, "human")
		result, err := application.resumeTicketPublishAfterRebaseConflict(
			context.Background(),
			application.services(),
			port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"},
			name,
			&base,
			false,
		)
		if err != nil || result.Sync.RecommendedAction != "rebased" || len(prompt.selectRequests) != 2 {
			t.Fatalf("rebase retry = (%#v, %v), prompts=%#v", result, err, prompt.selectRequests)
		}
	})

	t.Run("stops on a non-retryable resume failure", func(t *testing.T) {
		git := newBranchCommandGit(t, name.String())
		prompt := &commandHelperPrompt{
			selects: []commandHelperStringReply{{value: "retry"}},
		}
		application := newBranchCommandApplication(git, nil, prompt, "human")
		_, err := application.resumeTicketPublishAfterRebaseConflict(
			context.Background(),
			application.services(),
			port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"},
			mustTicketPublishBranch(t, "scratch/ABC-123-experiment"),
			&base,
			false,
		)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})
}

func TestTicketPublishScratchMergeRetryInteractionPaths(t *testing.T) {
	source := mustTicketPublishBranch(t, "scratch/ABC-123-export-exploration")
	target := ticketPublishTestBranch(t)
	message, err := commitmsg.Parse("feat(ABC-123): add export")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("returns prompt failures and cancellation", func(t *testing.T) {
		cancelPrompt := &commandHelperPrompt{
			selects: []commandHelperStringReply{{value: "cancel"}},
		}
		application := newBranchCommandApplication(newBranchCommandGit(t, source.String()), nil, cancelPrompt, "human")
		_, err := application.resumeScratchMergeAfterConflict(
			context.Background(),
			services{},
			port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"},
			source,
			target,
			message,
		)
		assertProblemCode(t, err, problem.CodeOperationCancelled)

		promptErr := errors.New("selection unavailable")
		failingPrompt := &commandHelperPrompt{
			selects: []commandHelperStringReply{{err: promptErr}},
		}
		application = newBranchCommandApplication(newBranchCommandGit(t, source.String()), nil, failingPrompt, "human")
		_, err = application.resumeScratchMergeAfterConflict(
			context.Background(),
			services{},
			port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"},
			source,
			target,
			message,
		)
		if !errors.Is(err, promptErr) {
			t.Fatalf("retry prompt error = %v, want %v", err, promptErr)
		}
	})

	t.Run("retries until every scratch conflict is resolved", func(t *testing.T) {
		git := newBranchCommandGit(t, source.String())
		git.officialBranches = []branch.BranchName{target}
		git.localBranches = map[string]bool{
			source.String(): true,
			target.String(): true,
		}
		git.unmergedStates = []bool{true, false}
		prompt := &commandHelperPrompt{
			selects: []commandHelperStringReply{{value: "retry"}, {value: "retry"}},
		}
		application := newBranchCommandApplication(git, nil, prompt, "human")
		result, err := application.resumeScratchMergeAfterConflict(
			context.Background(),
			application.services(),
			port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"},
			source,
			target,
			message,
		)
		if err != nil || !result.Committed || len(prompt.selectRequests) != 2 {
			t.Fatalf("scratch retry = (%#v, %v), prompts=%#v", result, err, prompt.selectRequests)
		}
		if len(git.committedMessages) != 1 || git.committedMessages[0].String() != message.String() {
			t.Fatalf("scratch retry commits = %#v", git.committedMessages)
		}
	})

	t.Run("stops on a non-retryable scratch failure", func(t *testing.T) {
		git := newBranchCommandGit(t, source.String())
		git.officialBranches = []branch.BranchName{target}
		git.localBranches = map[string]bool{
			source.String(): true,
			target.String(): true,
		}
		conflictInspectionErr := errors.New("conflict inspection failed")
		git.unmergedConflictsErr = conflictInspectionErr
		prompt := &commandHelperPrompt{
			selects: []commandHelperStringReply{{value: "retry"}},
		}
		application := newBranchCommandApplication(git, nil, prompt, "human")
		_, err := application.resumeScratchMergeAfterConflict(
			context.Background(),
			application.services(),
			port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"},
			source,
			target,
			message,
		)
		if !errors.Is(err, conflictInspectionErr) {
			t.Fatalf("scratch retry failure = %v, want %v", err, conflictInspectionErr)
		}
	})
}

func TestTicketPublishCompletionInteractionPaths(t *testing.T) {
	name := ticketPublishTestBranch(t)
	base := ticketPublishTestBase(t)

	newResult := func() workflow.PublishTicketResult {
		return workflow.PublishTicketResult{
			Branch: name,
			Sync: branchapp.SyncResult{
				Name: name,
				Base: base,
			},
			PullRequest: port.PullRequest{
				Source: name,
				Target: mustTicketPublishBranch(t, "develop"),
				Body:   "Summary: Add the export button.\n\nScope and Non-Goals: The export button only.\n\nCommit Series:\n- feat(ABC-123): add export button\n\nRisk and Rollback: Low; revert the commit.\n\nVerification and Review Focus: Unit tests; review the button wiring.",
			},
		}
	}

	t.Run("ignores nil and dry-run results", func(t *testing.T) {
		application := newBranchCommandApplication(newBranchCommandGit(t, name.String()), nil, nil, "human")
		if err := application.completeTicketPublishInteraction(context.Background(), services{}, port.RepositoryIdentity{}, nil, true, false, false); err != nil {
			t.Fatal(err)
		}
		dry := newResult()
		dry.DryRun = true
		if err := application.completeTicketPublishInteraction(context.Background(), services{}, port.RepositoryIdentity{}, &dry, true, false, false); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("honors a declined push and prompt failures", func(t *testing.T) {
		prompt := &commandHelperPrompt{
			confirms: []commandHelperConfirmReply{{value: false}},
		}
		application := newBranchCommandApplication(newBranchCommandGit(t, name.String()), nil, prompt, "human")
		result := newResult()
		if err := application.completeTicketPublishInteraction(context.Background(), services{}, port.RepositoryIdentity{}, &result, false, false, false); err != nil {
			t.Fatal(err)
		}
		if result.Pushed {
			t.Fatal("declined push was performed")
		}

		promptErr := errors.New("confirmation unavailable")
		prompt = &commandHelperPrompt{
			confirms: []commandHelperConfirmReply{{err: promptErr}},
		}
		application = newBranchCommandApplication(newBranchCommandGit(t, name.String()), nil, prompt, "human")
		result = newResult()
		err := application.completeTicketPublishInteraction(context.Background(), services{}, port.RepositoryIdentity{}, &result, false, false, false)
		if !errors.Is(err, promptErr) {
			t.Fatalf("push confirmation error = %v, want %v", err, promptErr)
		}
	})

	t.Run("pushes without a provider and can decline or create a provider pull request", func(t *testing.T) {
		git := newBranchCommandGit(t, name.String())
		prompt := &commandHelperPrompt{
			confirms: []commandHelperConfirmReply{{value: true}},
		}
		application := newBranchCommandApplication(git, nil, prompt, "human")
		result := newResult()
		if err := application.completeTicketPublishInteraction(
			context.Background(),
			application.services(),
			port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"},
			&result,
			false,
			false,
			false,
		); err != nil {
			t.Fatal(err)
		}
		if !result.Pushed || result.PublishedURL != "" {
			t.Fatalf("intent-only completion = %#v", result)
		}

		publisher := &workflowRecordingPublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/complete"}}
		git = newBranchCommandGit(t, name.String())
		prompt = &commandHelperPrompt{
			confirms: []commandHelperConfirmReply{{value: true}, {value: false}},
		}
		application = newBranchCommandApplication(git, nil, prompt, "human")
		application.runtime.Publisher = publisher
		result = newResult()
		if err := application.completeTicketPublishInteraction(
			context.Background(),
			application.services(),
			port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"},
			&result,
			false,
			false,
			false,
		); err != nil {
			t.Fatal(err)
		}
		if !result.Pushed || result.PublishedURL != "" || publisher.calls != 0 {
			t.Fatalf("declined pull request = %#v, publisher=%#v", result, publisher)
		}

		publisher = &workflowRecordingPublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/complete"}}
		git = newBranchCommandGit(t, name.String())
		prompt = &commandHelperPrompt{
			confirms: []commandHelperConfirmReply{{value: true}, {value: true}},
		}
		application = newBranchCommandApplication(git, nil, prompt, "human")
		application.runtime.Publisher = publisher
		result = newResult()
		if err := application.completeTicketPublishInteraction(
			context.Background(),
			application.services(),
			port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"},
			&result,
			false,
			false,
			false,
		); err != nil {
			t.Fatal(err)
		}
		if result.PublishedURL == "" || publisher.calls != 1 {
			t.Fatalf("created pull request = %#v, publisher=%#v", result, publisher)
		}
	})

	t.Run("returns provider failures", func(t *testing.T) {
		publishErr := errors.New("publisher failed")
		git := newBranchCommandGit(t, name.String())
		prompt := &commandHelperPrompt{
			confirms: []commandHelperConfirmReply{{value: true}, {value: true}},
		}
		application := newBranchCommandApplication(git, nil, prompt, "human")
		application.runtime.Publisher = &workflowRecordingPublisher{err: publishErr}
		result := newResult()
		err := application.completeTicketPublishInteraction(
			context.Background(),
			application.services(),
			port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"},
			&result,
			false,
			false,
			false,
		)
		if !errors.Is(err, publishErr) {
			t.Fatalf("publisher failure = %v, want %v", err, publishErr)
		}
	})

	t.Run("returns push and pull-request confirmation failures", func(t *testing.T) {
		pushErr := errors.New("push failed")
		git := newBranchCommandGit(t, name.String())
		git.pushErr = pushErr
		prompt := &commandHelperPrompt{
			confirms: []commandHelperConfirmReply{{value: true}},
		}
		application := newBranchCommandApplication(git, nil, prompt, "human")
		result := newResult()
		err := application.completeTicketPublishInteraction(
			context.Background(),
			application.services(),
			port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"},
			&result,
			false,
			false,
			false,
		)
		if !errors.Is(err, pushErr) {
			t.Fatalf("push failure = %v, want %v", err, pushErr)
		}

		confirmationErr := errors.New("pull request confirmation failed")
		git = newBranchCommandGit(t, name.String())
		prompt = &commandHelperPrompt{
			confirms: []commandHelperConfirmReply{{value: true}, {err: confirmationErr}},
		}
		application = newBranchCommandApplication(git, nil, prompt, "human")
		application.runtime.Publisher = &workflowRecordingPublisher{}
		result = newResult()
		err = application.completeTicketPublishInteraction(
			context.Background(),
			application.services(),
			port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"},
			&result,
			false,
			false,
			false,
		)
		if !errors.Is(err, confirmationErr) {
			t.Fatalf("pull request confirmation failure = %v, want %v", err, confirmationErr)
		}
	})
}

func TestPullRequestPublicationSilentContracts(t *testing.T) {
	name := ticketPublishTestBranch(t)
	base := ticketPublishTestBase(t)
	request := port.PullRequest{
		Source: name,
		Target: mustTicketPublishBranch(t, "develop"),
		Title:  "ABC-123: add export",
		Body:   "Summary: Add the export button.\n\nScope and Non-Goals: The export button only.\n\nCommit Series:\n- feat(ABC-123): add export button\n\nRisk and Rollback: Low; revert the commit.\n\nVerification and Review Focus: Unit tests; review the button wiring.",
	}
	newResult := func() workflow.PublishTicketResult {
		return workflow.PublishTicketResult{
			Branch: name,
			Sync: branchapp.SyncResult{
				Name: name,
				Base: base,
			},
			PullRequest: request,
		}
	}

	t.Run("preflights explicit provider publication", func(t *testing.T) {
		application := newBranchCommandApplication(newBranchCommandGit(t, name.String()), nil, nil, "human")
		if err := application.validatePullRequestPublication(application.services(), false, true, "Summary: add export."); err == nil {
			t.Fatal("provider publication without a push unexpectedly succeeded")
		}
		if err := application.validatePullRequestPublication(application.services(), true, true, "Summary: add export."); err == nil {
			t.Fatal("provider publication without an adapter unexpectedly succeeded")
		}
		application.options.dryRun = true
		if err := application.validatePullRequestPublication(application.services(), true, true, "Summary: add export."); err != nil {
			t.Fatalf("dry-run provider plan failed: %v", err)
		}
	})

	t.Run("requires the mandatory pull-request description", func(t *testing.T) {
		application := newBranchCommandApplication(newBranchCommandGit(t, name.String()), nil, nil, "human")
		application.options.dryRun = true
		if err := application.validatePullRequestPublication(application.services(), true, true, "   "); err == nil {
			t.Fatal("provider publication with an empty description unexpectedly succeeded")
		}
		if err := validatePullRequestBody(true, ""); err == nil {
			t.Fatal("pull-request body validation accepted an empty description")
		}
		if err := validatePullRequestBody(false, ""); err != nil {
			t.Fatalf("pull-request body validation blocked an intent-only plan: %v", err)
		}
	})

	t.Run("never selects provider publication during a dry run", func(t *testing.T) {
		application := newBranchCommandApplication(newBranchCommandGit(t, name.String()), nil, nil, "human")
		application.options.dryRun = true
		application.options.yes = true
		application.runtime.Publisher = &workflowRecordingPublisher{}

		_, published, err := application.resolvePullRequestPublication(
			context.Background(),
			application.services(),
			request,
			true,
		)
		if err != nil || published {
			t.Fatalf("dry-run publication selection = (%t, %v)", published, err)
		}
	})

	t.Run("requires explicit confirmation outside an interactive terminal", func(t *testing.T) {
		application := newBranchCommandApplication(newBranchCommandGit(t, name.String()), nil, nil, "human")
		application.runtime.Publisher = &workflowRecordingPublisher{}
		_, _, err := application.resolvePullRequestPublication(context.Background(), application.services(), request, true)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("publishes silently only with --yes and explicit request", func(t *testing.T) {
		git := newBranchCommandGit(t, name.String())
		publisher := &workflowRecordingPublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/silent"}}
		application := newBranchCommandApplication(git, nil, nil, "human")
		application.options.yes = true
		application.runtime.Publisher = publisher
		result := newResult()

		if err := application.completeTicketPublishInteraction(
			context.Background(),
			application.services(),
			port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"},
			&result,
			true,
			true,
			false,
		); err != nil {
			t.Fatal(err)
		}
		if !result.Pushed || result.PublishedURL != "https://example.invalid/pr/silent" || publisher.calls != 1 {
			t.Fatalf("silent publication = %#v, publisher=%#v", result, publisher)
		}
	})

	t.Run("reports an unavailable adapter only for an explicit request", func(t *testing.T) {
		application := newBranchCommandApplication(newBranchCommandGit(t, name.String()), nil, nil, "human")
		_, published, err := application.resolvePullRequestPublication(context.Background(), application.services(), request, false)
		if err != nil || published {
			t.Fatalf("intent-only publication = (%t, %v)", published, err)
		}
		_, _, err = application.resolvePullRequestPublication(context.Background(), application.services(), request, true)
		assertProblemCode(t, err, problem.CodeExternalCommandFailed)
		assertProblemCode(t, pullRequestPublisherUnavailable(), problem.CodeExternalCommandFailed)
		assertProblemCode(t, pullRequestConfirmationRequired(), problem.CodeInvalidInput)
	})

	t.Run("resolves a missing description interactively and validates it", func(t *testing.T) {
		prompt := &commandHelperPrompt{
			confirms: []commandHelperConfirmReply{{value: true}},
			inputs:   []commandHelperStringReply{{value: "Summary: Added interactively."}},
		}
		application := newBranchCommandApplication(newBranchCommandGit(t, name.String()), nil, prompt, "human")
		application.runtime.Publisher = &workflowRecordingPublisher{}
		emptyBody := request
		emptyBody.Body = "  "
		updated, create, err := application.resolvePullRequestPublication(context.Background(), application.services(), emptyBody, true)
		if err != nil || !create || updated.Body != "Summary: Added interactively." {
			t.Fatalf("interactive body resolution = (%#v, %t, %v)", updated, create, err)
		}
		if len(prompt.inputRequests) != 1 || prompt.inputRequests[0].Validate == nil {
			t.Fatalf("body prompt requests = %#v", prompt.inputRequests)
		}
		if err := prompt.inputRequests[0].Validate(" "); err == nil {
			t.Fatal("body validator accepted an empty description")
		}
		if err := prompt.inputRequests[0].Validate("Summary: ok."); err != nil {
			t.Fatalf("body validator rejected a description: %v", err)
		}
	})

	t.Run("fails closed for a missing description outside an interactive terminal", func(t *testing.T) {
		application := newBranchCommandApplication(newBranchCommandGit(t, name.String()), nil, nil, "human")
		application.options.yes = true
		application.runtime.Publisher = &workflowRecordingPublisher{}
		emptyBody := request
		emptyBody.Body = ""
		_, _, err := application.resolvePullRequestPublication(context.Background(), application.services(), emptyBody, true)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("propagates interactive description prompt failures", func(t *testing.T) {
		promptErr := errors.New("input unavailable")
		prompt := &commandHelperPrompt{
			confirms: []commandHelperConfirmReply{{value: true}},
			inputs:   []commandHelperStringReply{{err: promptErr}},
		}
		application := newBranchCommandApplication(newBranchCommandGit(t, name.String()), nil, prompt, "human")
		application.runtime.Publisher = &workflowRecordingPublisher{}
		emptyBody := request
		emptyBody.Body = ""
		_, _, err := application.resolvePullRequestPublication(context.Background(), application.services(), emptyBody, true)
		if !errors.Is(err, promptErr) {
			t.Fatalf("body prompt error = %v, want %v", err, promptErr)
		}
	})

	t.Run("preflights an interactively selected pull request before publication", func(t *testing.T) {
		preflightErr := errors.New("provider configuration is invalid")
		prompt := &commandHelperPrompt{
			confirms: []commandHelperConfirmReply{{value: true}, {value: true}},
		}
		application := newBranchCommandApplication(newBranchCommandGit(t, name.String()), nil, prompt, "human")
		application.runtime.Publisher = workflowPreflightFailurePublisher{err: preflightErr}
		result := newResult()
		err := application.completeTicketPublishInteraction(
			context.Background(),
			application.services(),
			port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"},
			&result,
			false,
			false,
			false,
		)
		if !errors.Is(err, preflightErr) {
			t.Fatalf("interactive preflight error = %v, want %v", err, preflightErr)
		}
	})
}

func TestTicketPublishResumesScratchTransferWithoutPrompts(t *testing.T) {
	source := "scratch/ABC-123-export-exploration"
	target := ticketPublishTestBranch(t)
	git := newBranchCommandGit(t, source)
	git.messages = []string{"feat(ABC-123): add export"}
	git.officialBranches = []branch.BranchName{target}
	git.localBranches = map[string]bool{
		source:          true,
		target.String(): true,
	}
	application := newBranchCommandApplication(git, nil, nil, "json")
	application.options.yes = true

	stdout, stderr, err := executeBranchCommand(
		t,
		newTicketPublishCommand(application),
		context.Background(),
		"--type", "feat",
		"--subject", "add export",
		"--commit-body", "## Motivation\n\nDocuments the discarded experiment paths.",
		"--resume",
	)
	if err != nil || stderr != "" {
		t.Fatalf("silent scratch resume = (%q, %q, %v)", stdout, stderr, err)
	}
	if !strings.Contains(stdout, "squashMerged") || len(git.committedMessages) != 1 {
		t.Fatalf("silent scratch resume output=%q commits=%#v", stdout, git.committedMessages)
	}

	git = newBranchCommandGit(t, source)
	git.officialBranches = []branch.BranchName{target}
	git.localBranches = map[string]bool{
		source:          true,
		target.String(): true,
	}
	git.unmergedConflicts = true
	application = newBranchCommandApplication(git, nil, nil, "json")
	application.options.yes = true
	_, _, err = executeBranchCommand(
		t,
		newTicketPublishCommand(application),
		context.Background(),
		"--type", "feat",
		"--subject", "add export",
		"--commit-body", "## Motivation\n\nDocuments the discarded experiment paths.",
		"--resume",
	)
	assertProblemCode(t, err, problem.CodeScratchMergeConflict)
}

func ticketPublishTestBranch(t *testing.T) branch.BranchName {
	t.Helper()
	return mustTicketPublishBranch(t, "feature/ABC-123-add-export")
}

func ticketPublishTestBase(t *testing.T) branch.TargetBase {
	t.Helper()
	develop := mustTicketPublishBranch(t, "develop")
	base, err := branch.NewTargetBase("origin", develop)
	if err != nil {
		t.Fatal(err)
	}
	return base
}

func mustTicketPublishBranch(t *testing.T, raw string) branch.BranchName {
	t.Helper()
	name, err := branch.ParseName(raw)
	if err != nil {
		t.Fatal(err)
	}
	return name
}
