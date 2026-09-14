package bootstrap

import (
	"context"
	"testing"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/application/workflow"
	"github.com/t33n-software/git-governance/internal/domain/branch"
)

func TestAddIntegrationLineReturnFields(t *testing.T) {
	t.Parallel()

	t.Run("omits every field without a transition", func(t *testing.T) {
		t.Parallel()
		fields := map[string]string{}
		addIntegrationLineReturnFields(fields, nil)
		if len(fields) != 0 {
			t.Fatalf("fields = %v", fields)
		}
	})

	t.Run("renders the status and the integration line", func(t *testing.T) {
		t.Parallel()
		fields := map[string]string{}
		home, err := branch.ParseName("develop")
		if err != nil {
			t.Fatal(err)
		}
		addIntegrationLineReturnFields(fields, &workflow.IntegrationLineReturn{
			Status: workflow.IntegrationLineReturnSwitched,
			Branch: home,
		})
		if fields["integrationLine"] != "develop" || fields["integrationLineReturn"] != "switched" {
			t.Fatalf("fields = %v", fields)
		}
		if _, present := fields["integrationLineReturnDetail"]; present {
			t.Fatalf("empty detail must not render: %v", fields)
		}
		if _, present := fields["integrationLineRefresh"]; present {
			t.Fatalf("empty refresh must not render: %v", fields)
		}
		if _, present := fields["integrationLineRefreshDetail"]; present {
			t.Fatalf("empty refresh detail must not render: %v", fields)
		}
	})

	t.Run("renders the detail when present", func(t *testing.T) {
		t.Parallel()
		fields := map[string]string{}
		home, err := branch.ParseName("develop")
		if err != nil {
			t.Fatal(err)
		}
		addIntegrationLineReturnFields(fields, &workflow.IntegrationLineReturn{
			Status: workflow.IntegrationLineReturnSkippedDirtyWorktree,
			Branch: home,
			Detail: "uncommitted changes are preserved",
		})
		if fields["integrationLineReturn"] != "skipped-dirty-worktree" ||
			fields["integrationLineReturnDetail"] != "uncommitted changes are preserved" {
			t.Fatalf("fields = %v", fields)
		}
	})

	t.Run("renders the refresh outcome when present", func(t *testing.T) {
		t.Parallel()
		fields := map[string]string{}
		home, err := branch.ParseName("develop")
		if err != nil {
			t.Fatal(err)
		}
		addIntegrationLineReturnFields(fields, &workflow.IntegrationLineReturn{
			Status:        workflow.IntegrationLineReturnSwitched,
			Branch:        home,
			Refresh:       workflow.IntegrationLineRefreshDiverged,
			RefreshDetail: "left untouched",
		})
		if fields["integrationLineRefresh"] != "diverged" ||
			fields["integrationLineRefreshDetail"] != "left untouched" {
			t.Fatalf("fields = %v", fields)
		}
	})
}

// integrationLineReturnCommandGit records branch switches so the composition
// wiring of the post-publication transition is observable.
type integrationLineReturnCommandGit struct {
	*commandGit
	switchedTo []string
}

func (git *integrationLineReturnCommandGit) SwitchBranch(_ context.Context, _ port.RepositoryIdentity, name branch.BranchName) error {
	git.switchedTo = append(git.switchedTo, name.String())
	return nil
}

type integrationLineReturnPublisher struct{}

func (integrationLineReturnPublisher) Publish(context.Context, port.PullRequestPublication) (port.PublishedPullRequest, error) {
	return port.PublishedPullRequest{URL: "https://example.invalid/pr/1"}, nil
}

func TestServicesIntegrationLineReturnWiring(t *testing.T) {
	t.Parallel()

	request := port.PullRequest{
		Source: mustTicketPublishBranch(t, "feature/ABC-123-add-export"),
		Target: mustTicketPublishBranch(t, "develop"),
		Title:  "ABC-123: add-export",
		Body:   "Summary: Add the export button.",
	}
	repository := port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"}

	publish := func(t *testing.T, workflowTokenServer, propagationPublisherServer bool) (workflow.PublicationResult, *integrationLineReturnCommandGit) {
		t.Helper()
		git := &integrationLineReturnCommandGit{commandGit: newCommandGit(t, "feature/ABC-123-add-export", nil)}
		runtime := commandRuntime(git)
		runtime.Publisher = integrationLineReturnPublisher{}
		runtime.GitHubWorkflowTokenEnabled = func() bool { return workflowTokenServer }
		runtime.HotfixPropagationPublisherEnabled = func() bool { return propagationPublisherServer }
		application := newApplication(runtime, runtimeTestOptions())
		result, err := application.services().tickets.PublishPullRequest(context.Background(), repository, request)
		if err != nil {
			t.Fatal(err)
		}
		return result, git
	}

	t.Run("local operator context returns to the integration line", func(t *testing.T) {
		t.Parallel()
		result, git := publish(t, false, false)
		if result.IntegrationLineReturn == nil ||
			result.IntegrationLineReturn.Status != workflow.IntegrationLineReturnSwitched ||
			result.IntegrationLineReturn.Branch.String() != "develop" {
			t.Fatalf("PublishPullRequest() = %#v", result)
		}
		if len(git.switchedTo) != 1 || git.switchedTo[0] != "develop" {
			t.Fatalf("switches = %v", git.switchedTo)
		}
	})

	t.Run("protected workflow-token server context never switches", func(t *testing.T) {
		t.Parallel()
		result, git := publish(t, true, false)
		if result.IntegrationLineReturn != nil {
			t.Fatalf("server context must not transition: %#v", result.IntegrationLineReturn)
		}
		if len(git.switchedTo) != 0 {
			t.Fatalf("server context switched: %v", git.switchedTo)
		}
	})

	t.Run("hotfix propagation publisher server context never switches", func(t *testing.T) {
		t.Parallel()
		result, git := publish(t, false, true)
		if result.IntegrationLineReturn != nil {
			t.Fatalf("server context must not transition: %#v", result.IntegrationLineReturn)
		}
		if len(git.switchedTo) != 0 {
			t.Fatalf("server context switched: %v", git.switchedTo)
		}
	})
}

func TestCompleteTicketPublishInteractionIntegrationLineReturn(t *testing.T) {
	t.Parallel()

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
				Body:   "Summary: Add the export button.",
			},
		}
	}

	publish := func(t *testing.T, git *branchCommandGit, server bool) workflow.PublishTicketResult {
		t.Helper()
		prompt := &commandHelperPrompt{
			confirms: []commandHelperConfirmReply{{value: true}, {value: true}},
		}
		application := newBranchCommandApplication(git, nil, prompt, "human")
		application.runtime.Publisher = &workflowRecordingPublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/complete"}}
		if server {
			application.runtime.GitHubWorkflowTokenEnabled = func() bool { return true }
		}
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
		return result
	}

	t.Run("returns the workspace to the integration line after a confirmed publication", func(t *testing.T) {
		t.Parallel()
		git := newBranchCommandGit(t, name.String())
		result := publish(t, git, false)
		if result.PublishedURL != "https://example.invalid/pr/complete" ||
			result.IntegrationLineReturn == nil ||
			result.IntegrationLineReturn.Status != workflow.IntegrationLineReturnSwitched ||
			result.IntegrationLineReturn.Branch.String() != "develop" {
			t.Fatalf("completed publication = %#v", result)
		}
		if len(git.switchedBranches) != 1 || git.switchedBranches[0].String() != "develop" {
			t.Fatalf("switches = %v", git.switchedBranches)
		}
	})

	t.Run("never switches the checkout of a server-side controller", func(t *testing.T) {
		t.Parallel()
		git := newBranchCommandGit(t, name.String())
		result := publish(t, git, true)
		if result.PublishedURL != "https://example.invalid/pr/complete" || result.IntegrationLineReturn != nil {
			t.Fatalf("server publication = %#v", result)
		}
		if len(git.switchedBranches) != 0 {
			t.Fatalf("server context switched: %v", git.switchedBranches)
		}
	})
}

func TestTicketPublishReportIncludesIntegrationLineReturn(t *testing.T) {
	t.Parallel()

	name := ticketPublishTestBranch(t)
	git := newBranchCommandGit(t, name.String())
	prompt := &commandHelperPrompt{
		confirms: []commandHelperConfirmReply{{value: true}, {value: true}},
	}
	application := newBranchCommandApplication(git, nil, prompt, "human")
	application.runtime.Publisher = &workflowRecordingPublisher{result: port.PublishedPullRequest{URL: "https://example.invalid/pr/complete"}}
	result := workflow.PublishTicketResult{
		Branch: name,
		Sync: branchapp.SyncResult{
			Name: name,
			Base: ticketPublishTestBase(t),
		},
		PullRequest: port.PullRequest{
			Source: name,
			Target: mustTicketPublishBranch(t, "develop"),
			Body:   "Summary: Add the export button.",
		},
	}
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
	fields := map[string]string{}
	addIntegrationLineReturnFields(fields, result.IntegrationLineReturn)
	if fields["integrationLine"] != "develop" || fields["integrationLineReturn"] != "switched" {
		t.Fatalf("report fields = %v", fields)
	}
	if len(git.switchedBranches) != 1 || git.switchedBranches[0].String() != "develop" {
		t.Fatalf("switches = %v", git.switchedBranches)
	}
}
