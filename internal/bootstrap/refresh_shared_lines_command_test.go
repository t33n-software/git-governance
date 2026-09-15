package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestBranchRefreshSharedLinesCommandContracts(t *testing.T) {
	t.Run("refreshes every local shared line and reports JSON", func(t *testing.T) {
		git := newBranchCommandGit(t, "develop")
		git.localBranchList = []branch.BranchName{mustTicketPublishBranch(t, "develop"), mustTicketPublishBranch(t, "main")}
		application := newBranchCommandApplication(git, nil, nil, "json")
		application.options.yes = true

		stdout, stderr, err := executeBranchCommand(
			t,
			newBranchRefreshSharedLinesCommand(application),
			context.Background(),
		)
		if err != nil {
			t.Fatalf("shared-line refresh error = %v", err)
		}
		if stderr != "" {
			t.Fatalf("shared-line refresh stderr = %q", stderr)
		}
		result := assertSingleUtilityJSONResult(t, stdout, "branch.refresh-shared-lines")
		fields := utilityJSONFields(t, result)
		if fields["lines"] != "2" || fields["dryRun"] != "false" ||
			fields["line develop"] != "updated" || fields["line main"] != "updated" {
			t.Fatalf("refresh fields = %#v", fields)
		}
		data, ok := result["data"].([]any)
		if !ok || len(data) != 2 {
			t.Fatalf("refresh data = %#v", result["data"])
		}
		if git.fetchCalls != 1 {
			t.Fatalf("fetch calls = %d, want 1", git.fetchCalls)
		}
		if len(git.ffCalls) != 1 || git.ffCalls[0] != "develop" {
			t.Fatalf("checked-out refresh calls = %v", git.ffCalls)
		}
		if len(git.ffReferenceCalls) != 1 || git.ffReferenceCalls[0] != "main" {
			t.Fatalf("reference refresh calls = %v", git.ffReferenceCalls)
		}
	})

	t.Run("renders the human report with the fetch summary", func(t *testing.T) {
		git := newBranchCommandGit(t, "develop")
		application := newBranchCommandApplication(git, nil, &commandHelperPrompt{}, "human")
		application.options.yes = true

		stdout, _, err := executeBranchCommand(
			t,
			newBranchRefreshSharedLinesCommand(application),
			context.Background(),
		)
		if err != nil {
			t.Fatalf("human refresh error = %v", err)
		}
		for _, expected := range []string{
			"🟢 Remote references fetched and stale references pruned from origin before this operation.",
			"Local shared-line checkouts refreshed.",
			"line develop: updated",
		} {
			if !strings.Contains(stdout, expected) {
				t.Fatalf("human refresh output missing %q: %q", expected, stdout)
			}
		}
	})

	t.Run("plans a dry run without fetching or mutating", func(t *testing.T) {
		git := newBranchCommandGit(t, "develop")
		application := newBranchCommandApplication(git, nil, nil, "json")
		application.options.dryRun = true

		stdout, _, err := executeBranchCommand(
			t,
			newBranchRefreshSharedLinesCommand(application),
			context.Background(),
		)
		if err != nil {
			t.Fatalf("dry-run refresh error = %v", err)
		}
		fields := utilityJSONFields(t, assertSingleUtilityJSONResult(t, stdout, "branch.refresh-shared-lines"))
		if fields["dryRun"] != "true" || fields["lines"] != "0" ||
			!strings.Contains(fields["plan"], "refresh: develop to origin/develop") {
			t.Fatalf("dry-run fields = %#v", fields)
		}
		if git.fetchCalls != 0 || len(git.ffCalls) != 0 || len(git.ffReferenceCalls) != 0 {
			t.Fatalf("dry-run mutated Git: fetch=%d ff=%v ref=%v", git.fetchCalls, git.ffCalls, git.ffReferenceCalls)
		}
		if strings.Contains(stdout, `"data"`) {
			t.Fatalf("an empty dry-run result must omit the data field: %q", stdout)
		}
	})

	t.Run("honors the explicit repeatable line selection", func(t *testing.T) {
		git := newBranchCommandGit(t, "feature/ABC-123-add-export")
		git.localBranches = map[string]bool{"develop": true, "main": true}
		application := newBranchCommandApplication(git, nil, nil, "json")
		application.options.yes = true

		_, _, err := executeBranchCommand(
			t,
			newBranchRefreshSharedLinesCommand(application),
			context.Background(),
			"--line", "develop",
			"--line", "main",
		)
		if err != nil {
			t.Fatalf("explicit selection error = %v", err)
		}
		if len(git.ffCalls) != 0 {
			t.Fatalf("the feature checkout must never be refreshed here: %v", git.ffCalls)
		}
		if strings.Join(git.ffReferenceCalls, ",") != "develop,main" {
			t.Fatalf("reference refresh calls = %v", git.ffReferenceCalls)
		}
	})

	t.Run("rejects a non-shared line selection", func(t *testing.T) {
		git := newBranchCommandGit(t, "feature/ABC-123-add-export")
		application := newBranchCommandApplication(git, nil, nil, "human")
		application.options.yes = true

		_, _, err := executeBranchCommand(
			t,
			newBranchRefreshSharedLinesCommand(application),
			context.Background(),
			"--line", "feature/ABC-123-add-export",
		)
		assertProblemCode(t, err, problem.CodeBranchFamilyInvalid)
		if git.fetchCalls != 0 || len(git.ffReferenceCalls) != 0 {
			t.Fatalf("a non-shared selection must not mutate: fetch=%d ref=%v", git.fetchCalls, git.ffReferenceCalls)
		}
	})

	t.Run("rejects a malformed line value", func(t *testing.T) {
		git := newBranchCommandGit(t, "develop")
		application := newBranchCommandApplication(git, nil, nil, "human")
		application.options.yes = true

		_, _, err := executeBranchCommand(
			t,
			newBranchRefreshSharedLinesCommand(application),
			context.Background(),
			"--line", "not a branch",
		)
		assertProblemCode(t, err, problem.CodeBranchNameInvalid)
	})

	t.Run("fails closed on a diverged line", func(t *testing.T) {
		git := newBranchCommandGit(t, "develop")
		git.ffOutcome = port.FastForwardDiverged
		application := newBranchCommandApplication(git, nil, nil, "human")
		application.options.yes = true

		_, _, err := executeBranchCommand(
			t,
			newBranchRefreshSharedLinesCommand(application),
			context.Background(),
		)
		assertProblemCode(t, err, problem.CodeSharedLineDiverged)
	})

	t.Run("requires noninteractive consent and honors a declined prompt", func(t *testing.T) {
		noninteractiveGit := newBranchCommandGit(t, "develop")
		_, _, err := executeBranchCommand(
			t,
			newBranchRefreshSharedLinesCommand(newBranchCommandApplication(noninteractiveGit, nil, nil, "human")),
			context.Background(),
		)
		assertProblemCode(t, err, problem.CodeInvalidInput)
		if noninteractiveGit.fetchCalls != 0 {
			t.Fatalf("a missing consent must not fetch: %d", noninteractiveGit.fetchCalls)
		}

		prompt := &commandHelperPrompt{
			confirms: []commandHelperConfirmReply{{value: false}},
		}
		declinedGit := newBranchCommandGit(t, "develop")
		_, _, err = executeBranchCommand(
			t,
			newBranchRefreshSharedLinesCommand(newBranchCommandApplication(declinedGit, nil, prompt, "human")),
			context.Background(),
		)
		assertProblemCode(t, err, problem.CodeOperationCancelled)
		if len(prompt.confirmRequests) != 1 || prompt.confirmRequests[0].Label != "Refresh shared lines" {
			t.Fatalf("refresh confirmation request = %#v", prompt.confirmRequests)
		}
		if declinedGit.fetchCalls != 0 {
			t.Fatalf("a declined refresh must not fetch: %d", declinedGit.fetchCalls)
		}
	})

	t.Run("preserves discovery and fetch failures", func(t *testing.T) {
		discoverErr := errors.New("repository discovery failed")
		git := newBranchCommandGit(t, "develop")
		git.discoverErr = discoverErr
		application := newBranchCommandApplication(git, nil, nil, "human")
		application.options.yes = true
		_, _, err := executeBranchCommand(t, newBranchRefreshSharedLinesCommand(application), context.Background())
		if !errors.Is(err, discoverErr) {
			t.Fatalf("discovery error = %v, want %v", err, discoverErr)
		}

		fetchErr := errors.New("fetch failed")
		git = newBranchCommandGit(t, "develop")
		git.fetchErr = fetchErr
		application = newBranchCommandApplication(git, nil, nil, "human")
		application.options.yes = true
		_, _, err = executeBranchCommand(t, newBranchRefreshSharedLinesCommand(application), context.Background())
		if !errors.Is(err, fetchErr) {
			t.Fatalf("fetch error = %v, want %v", err, fetchErr)
		}
	})

	t.Run("stops the refresh when the command context is cancelled", func(t *testing.T) {
		git := newBranchCommandGit(t, "develop")
		application := newBranchCommandApplication(git, nil, nil, "human")
		application.options.yes = true
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, _, err := executeBranchCommand(
			t,
			newBranchRefreshSharedLinesCommand(application),
			ctx,
		)
		assertProblemCode(t, err, problem.CodeOperationCancelled)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled refresh error = %v, want context cancellation", err)
		}
	})
}

func TestBranchRefreshSharedLinesHelpContract(t *testing.T) {
	git := newBranchCommandGit(t, "develop")
	root := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
	command, _, err := root.Find([]string{"branch", "refresh-shared-lines"})
	if err != nil || command == nil {
		t.Fatalf("refresh-shared-lines not found: %v", err)
	}
	if !strings.Contains(command.Long, discoveryReferenceLine) {
		t.Fatalf("refresh-shared-lines help misses the discovery reference: %q", command.Long)
	}
	flag := command.Flags().Lookup("line")
	if flag == nil {
		t.Fatal("the --line flag is not registered")
	}
	for _, expected := range []string{
		"main, develop, release/<semver>, or support/<major.minor>",
		"example:",
		"repeatable",
	} {
		if !strings.Contains(flag.Usage, expected) {
			t.Fatalf("--line help missing %q: %q", expected, flag.Usage)
		}
	}
	if flag.Annotations[valueDomainFlagAnnotation] == nil {
		t.Fatal("the --line flag is not bound to the value-domain register")
	}
}
