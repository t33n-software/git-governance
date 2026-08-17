package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

func TestBranchCommandTreeAndListReportContracts(t *testing.T) {
	application := newBranchCommandApplication(
		newBranchCommandGit(t, "feature/ABC-123-add-export"),
		nil,
		nil,
		"json",
	)
	command := newBranchCommand(application)
	children := make(map[string]bool)
	for _, child := range command.Commands() {
		children[child.Name()] = true
	}
	for _, expected := range []string{"list", "validate", "create", "merge-scratch", "sync-base"} {
		if !children[expected] {
			t.Fatalf("branch command children = %#v, missing %q", children, expected)
		}
	}

	stdout, stderr, err := executeBranchCommand(t, newBranchListCommand(application), context.Background())
	if err != nil {
		t.Fatalf("branch list error = %v", err)
	}
	if stderr != "" {
		t.Fatalf("branch list stderr = %q", stderr)
	}
	result := assertSingleUtilityJSONResult(t, stdout, "branch.list")
	fields := utilityJSONFields(t, result)
	if !strings.Contains(fields["feature"], "feature/<ticket>-<slug>") {
		t.Fatalf("feature family report = %q", fields["feature"])
	}
	families, ok := result["data"].([]any)
	if !ok || len(families) != len(branchapp.ListFamilies()) {
		t.Fatalf("branch list data = %#v", result["data"])
	}

	application.options.output = "human"
	humanOutput, _, err := executeBranchCommand(t, newBranchListCommand(application), context.Background())
	if err != nil {
		t.Fatalf("human branch list error = %v", err)
	}
	if !strings.Contains(humanOutput, "Supported branch families:") ||
		!strings.Contains(humanOutput, "feature") {
		t.Fatalf("human branch list output = %q", humanOutput)
	}
}

func TestBranchValidateCommandContracts(t *testing.T) {
	t.Run("validates explicit and current branches with a JSON report", func(t *testing.T) {
		git := newBranchCommandGit(t, "feature/ABC-123-add-export")
		application := newBranchCommandApplication(git, nil, nil, "json")

		stdout, stderr, err := executeBranchCommand(
			t,
			newBranchValidateCommand(application),
			context.Background(),
			"--branch", "feature/ABC-123-add-export",
		)
		if err != nil {
			t.Fatalf("explicit branch validation error = %v", err)
		}
		if stderr != "" {
			t.Fatalf("explicit branch validation stderr = %q", stderr)
		}
		fields := utilityJSONFields(t, assertSingleUtilityJSONResult(t, stdout, "branch.validate"))
		if fields["branch"] != "feature/ABC-123-add-export" || fields["family"] != "feature" {
			t.Fatalf("explicit validation fields = %#v", fields)
		}
		if git.currentCalls != 0 {
			t.Fatalf("explicit branch validation called CurrentBranch %d times", git.currentCalls)
		}
		if got := branchNames(git.validatedBranches); strings.Join(got, ",") != "feature/ABC-123-add-export" {
			t.Fatalf("validated branches = %v", got)
		}

		manualGit := newBranchCommandGit(t, "feature/ABC-123-add-export")
		manualApplication := newBranchCommandApplication(manualGit, nil, nil, "human")
		manualOutput, _, err := executeBranchCommand(
			t,
			newBranchValidateCommand(manualApplication),
			context.Background(),
		)
		if err != nil {
			t.Fatalf("manual branch validation error = %v", err)
		}
		if manualGit.currentCalls != 1 {
			t.Fatalf("manual validation CurrentBranch calls = %d, want 1", manualGit.currentCalls)
		}
		if !strings.Contains(manualOutput, "Branch is valid.") {
			t.Fatalf("manual validation output = %q", manualOutput)
		}
	})

	discoverErr := errors.New("repository discovery failed")
	currentErr := errors.New("current branch failed")
	validateErr := errors.New("branch reference validation failed")
	for _, testCase := range []struct {
		name      string
		arguments []string
		configure func(*branchCommandGit)
		wantErr   error
	}{
		{
			name: "preserves discovery failures",
			configure: func(git *branchCommandGit) {
				git.discoverErr = discoverErr
			},
			wantErr: discoverErr,
		},
		{
			name: "preserves current branch failures",
			configure: func(git *branchCommandGit) {
				git.currentErr = currentErr
			},
			wantErr: currentErr,
		},
		{
			name:      "preserves validation service failures",
			arguments: []string{"--branch", "feature/ABC-123-add-export"},
			configure: func(git *branchCommandGit) {
				git.validateErr = validateErr
			},
			wantErr: validateErr,
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			git := newBranchCommandGit(t, "feature/ABC-123-add-export")
			testCase.configure(git)
			_, _, err := executeBranchCommand(
				t,
				newBranchValidateCommand(newBranchCommandApplication(git, nil, nil, "human")),
				context.Background(),
				testCase.arguments...,
			)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("branch validation error = %v, want %v", err, testCase.wantErr)
			}
		})
	}

	t.Run("stops validation when the command context is cancelled", func(t *testing.T) {
		git := newBranchCommandGit(t, "feature/ABC-123-add-export")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, _, err := executeBranchCommand(
			t,
			newBranchValidateCommand(newBranchCommandApplication(git, nil, nil, "human")),
			ctx,
			"--branch", "feature/ABC-123-add-export",
		)
		assertProblemCode(t, err, problem.CodeOperationCancelled)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled validation error = %v, want context cancellation", err)
		}
		if len(git.discoverContexts) != 1 || git.discoverContexts[0] != ctx {
			t.Fatalf("discover contexts = %#v, want cancelled command context", git.discoverContexts)
		}
	})
}

func TestBranchCreateCommandContracts(t *testing.T) {
	t.Run("parses root and creation flags then creates a regular branch", func(t *testing.T) {
		git := newBranchCommandGit(t, "feature/ABC-123-add-export")
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))

		output, err := executeBootstrapCommand(
			t,
			command,
			"--interactive", "never",
			"--output", "json",
			"--yes",
			"branch", "create",
			"--family", "feature",
			"--key", "ABC",
			"--ticket", "123",
			"--slug", "add-export",
			"--switch=false",
		)
		if err != nil {
			t.Fatalf("regular branch creation error = %v", err)
		}
		if len(git.createdBranches) != 1 {
			t.Fatalf("created branches = %#v", git.createdBranches)
		}
		created := git.createdBranches[0]
		if created.name.String() != "feature/ABC-123-add-export" ||
			created.base.String() != "origin/develop" ||
			created.switchTo {
			t.Fatalf("created branch = %#v", created)
		}
		fields := utilityJSONFields(t, assertSingleUtilityJSONResult(t, output, "branch.create"))
		if fields["branch"] != created.name.String() ||
			fields["base"] != "origin/develop" ||
			fields["switched"] != "false" ||
			fields["dryRun"] != "false" ||
			!strings.Contains(fields["plan"], "create: feature/ABC-123-add-export from origin/develop") {
			t.Fatalf("regular creation fields = %#v", fields)
		}
		if strings.Contains(output, "Remote references fetched and stale references pruned") {
			t.Fatalf("noninteractive JSON output unexpectedly contains an interactive fetch summary: %q", output)
		}
	})

	t.Run("shows the completed remote refresh in the interactive summary", func(t *testing.T) {
		git := newBranchCommandGit(t, "feature/ABC-123-add-export")
		application := newBranchCommandApplication(git, nil, &commandHelperPrompt{}, "human")
		application.options.yes = true

		stdout, stderr, err := executeBranchCommand(
			t,
			newBranchCreateCommand(application),
			context.Background(),
			"--family", "feature",
			"--key", "ABC",
			"--ticket", "123",
			"--slug", "add-export",
			"--switch=false",
		)
		if err != nil {
			t.Fatalf("interactive branch creation error = %v", err)
		}
		if stderr != "" {
			t.Fatalf("interactive branch creation stderr = %q", stderr)
		}
		for _, expected := range []string{
			"🟢 Remote references fetched and stale references pruned from origin before this operation.",
			"Branch created.",
			"branch: feature/ABC-123-add-export",
		} {
			if !strings.Contains(stdout, expected) {
				t.Fatalf("interactive branch creation output missing %q: %q", expected, stdout)
			}
		}
		if git.fetchCalls != 1 {
			t.Fatalf("interactive branch creation fetch calls = %d, want 1", git.fetchCalls)
		}
	})

	t.Run("generates scratch dry-run plans without mutating Git", func(t *testing.T) {
		git := newBranchCommandGit(t, "feature/ABC-123-add-export")
		application := newBranchCommandApplication(git, nil, nil, "json")
		application.options.dryRun = true

		stdout, stderr, err := executeBranchCommand(
			t,
			newBranchCreateCommand(application),
			context.Background(),
			"--family", "scratch",
			"--key", "ABC",
			"--ticket", "123",
			"--slug", "exploration",
			"--base", "feature/ABC-123-add-export",
			"--switch=false",
		)
		if err != nil {
			t.Fatalf("scratch dry-run error = %v", err)
		}
		if stderr != "" {
			t.Fatalf("scratch dry-run stderr = %q", stderr)
		}
		if git.fetchCalls != 0 || len(git.createdBranches) != 0 {
			t.Fatalf("scratch dry-run mutated Git: fetches=%d creates=%#v", git.fetchCalls, git.createdBranches)
		}
		fields := utilityJSONFields(t, assertSingleUtilityJSONResult(t, stdout, "branch.create"))
		if fields["branch"] != "scratch/ABC-123-exploration" ||
			fields["base"] != "feature/ABC-123-add-export" ||
			fields["switched"] != "false" ||
			fields["dryRun"] != "true" ||
			fields["plan"] != "fetch: git fetch --prune origin; create: scratch/ABC-123-exploration from feature/ABC-123-add-export" {
			t.Fatalf("scratch dry-run fields = %#v", fields)
		}
	})

	t.Run("requires noninteractive consent and honors a declined prompt", func(t *testing.T) {
		arguments := []string{
			"--family", "feature",
			"--key", "ABC",
			"--ticket", "123",
			"--slug", "add-export",
		}
		noninteractiveGit := newBranchCommandGit(t, "feature/ABC-123-add-export")
		_, _, err := executeBranchCommand(
			t,
			newBranchCreateCommand(newBranchCommandApplication(noninteractiveGit, nil, nil, "human")),
			context.Background(),
			arguments...,
		)
		assertProblemCode(t, err, problem.CodeInvalidInput)
		if len(noninteractiveGit.createdBranches) != 0 {
			t.Fatalf("noninteractive creation mutated Git: %#v", noninteractiveGit.createdBranches)
		}

		prompt := &commandHelperPrompt{
			confirms: []commandHelperConfirmReply{{value: false}},
		}
		declinedGit := newBranchCommandGit(t, "feature/ABC-123-add-export")
		_, _, err = executeBranchCommand(
			t,
			newBranchCreateCommand(newBranchCommandApplication(declinedGit, nil, prompt, "human")),
			context.Background(),
			arguments...,
		)
		assertProblemCode(t, err, problem.CodeOperationCancelled)
		if len(prompt.confirmRequests) != 1 ||
			prompt.confirmRequests[0].Label != "Create branch" ||
			!strings.Contains(prompt.confirmRequests[0].Description, "feature/ABC-123-add-export") {
			t.Fatalf("creation confirmation request = %#v", prompt.confirmRequests)
		}
		if len(declinedGit.createdBranches) != 0 {
			t.Fatalf("declined creation mutated Git: %#v", declinedGit.createdBranches)
		}
	})

	t.Run("preserves report writer failures after a successful creation", func(t *testing.T) {
		writeErr := errors.New("output unavailable")
		git := newBranchCommandGit(t, "feature/ABC-123-add-export")
		application := newBranchCommandApplication(git, nil, nil, "human")
		application.options.yes = true
		command := newBranchCreateCommand(application)
		command.SetOut(commandHelperFailingWriter{err: writeErr})
		command.SetErr(io.Discard)
		command.SetArgs([]string{
			"--family", "feature",
			"--key", "ABC",
			"--ticket", "123",
			"--slug", "add-export",
		})

		err := command.ExecuteContext(context.Background())
		assertProblemCode(t, err, problem.CodeExternalCommandFailed)
		if !errors.Is(err, writeErr) {
			t.Fatalf("creation report error = %v, want %v", err, writeErr)
		}
		if len(git.createdBranches) != 1 {
			t.Fatalf("creation before report failure = %#v", git.createdBranches)
		}
	})

	discoverErr := errors.New("repository discovery failed")
	fetchErr := errors.New("fetch failed")
	for _, testCase := range []struct {
		name      string
		arguments []string
		configure func(*branchCommandGit, *application)
		wantCode  problem.Code
		wantErr   error
	}{
		{
			name: "preserves discovery failures",
			configure: func(git *branchCommandGit, _ *application) {
				git.discoverErr = discoverErr
			},
			wantErr: discoverErr,
		},
		{
			name:      "rejects unsupported branch families",
			arguments: []string{"--family", "unknown"},
			wantCode:  problem.CodeBranchFamilyInvalid,
		},
		{
			name:      "rejects invalid ticket keys",
			arguments: []string{"--family", "feature", "--key", "invalid"},
			wantCode:  problem.CodeTicketKeyInvalid,
		},
		{
			name:      "rejects invalid ticket numbers",
			arguments: []string{"--family", "feature", "--key", "ABC", "--ticket", "012"},
			wantCode:  problem.CodeTicketNumberInvalid,
		},
		{
			name:      "rejects invalid branch slugs",
			arguments: []string{"--family", "feature", "--key", "ABC", "--ticket", "123", "--slug", "Add Export"},
			wantCode:  problem.CodeBranchSlugInvalid,
		},
		{
			name:      "rejects bases from another remote",
			arguments: []string{"--family", "feature", "--key", "ABC", "--ticket", "123", "--slug", "add-export", "--base", "upstream/develop"},
			wantCode:  problem.CodeBranchBaseInvalid,
		},
		{
			name:      "rejects non-ticket families after parsing inputs",
			arguments: []string{"--family", "main", "--key", "ABC", "--ticket", "123", "--slug", "add-export"},
			wantCode:  problem.CodeBranchFamilyInvalid,
		},
		{
			name:      "rejects remote scratch bases",
			arguments: []string{"--family", "scratch", "--key", "ABC", "--ticket", "123", "--slug", "exploration", "--base", "origin/feature/ABC-123-add-export"},
			wantCode:  problem.CodeBranchBaseInvalid,
		},
		{
			name:      "enforces matching scratch ticket bases",
			arguments: []string{"--family", "scratch", "--key", "ABC", "--ticket", "123", "--slug", "exploration", "--base", "feature/XYZ-999-other-ticket"},
			wantCode:  problem.CodeBranchBaseInvalid,
		},
		{
			name: "preserves branch creation service failures",
			arguments: []string{
				"--family", "feature",
				"--key", "ABC",
				"--ticket", "123",
				"--slug", "add-export",
			},
			configure: func(git *branchCommandGit, _ *application) {
				git.fetchErr = fetchErr
			},
			wantErr: fetchErr,
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			git := newBranchCommandGit(t, "feature/ABC-123-add-export")
			application := newBranchCommandApplication(git, nil, nil, "human")
			application.options.yes = true
			if testCase.configure != nil {
				testCase.configure(git, application)
			}

			_, _, err := executeBranchCommand(
				t,
				newBranchCreateCommand(application),
				context.Background(),
				testCase.arguments...,
			)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("branch creation error = %v, want %v", err, testCase.wantErr)
				}
			} else {
				assertProblemCode(t, err, testCase.wantCode)
			}
			if len(git.createdBranches) != 0 {
				t.Fatalf("failed creation mutated Git: %#v", git.createdBranches)
			}
		})
	}

	t.Run("stops creation when the command context is cancelled", func(t *testing.T) {
		git := newBranchCommandGit(t, "feature/ABC-123-add-export")
		application := newBranchCommandApplication(git, nil, nil, "human")
		application.options.yes = true
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, _, err := executeBranchCommand(
			t,
			newBranchCreateCommand(application),
			ctx,
			"--family", "feature",
			"--key", "ABC",
			"--ticket", "123",
			"--slug", "add-export",
		)
		assertProblemCode(t, err, problem.CodeOperationCancelled)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled creation error = %v, want context cancellation", err)
		}
		if len(git.createdBranches) != 0 {
			t.Fatalf("cancelled creation mutated Git: %#v", git.createdBranches)
		}
	})
}

func TestBranchSyncBaseCommandContracts(t *testing.T) {
	t.Run("checks the current branch and reports JSON without quality fields", func(t *testing.T) {
		git := newBranchCommandGit(t, "feature/ABC-123-add-export")
		application := newBranchCommandApplication(git, nil, nil, "json")

		stdout, stderr, err := executeBranchCommand(
			t,
			newBranchSyncBaseCommand(application),
			context.Background(),
			"--base", "develop",
		)
		if err != nil {
			t.Fatalf("manual sync check error = %v", err)
		}
		if stderr != "" {
			t.Fatalf("manual sync check stderr = %q", stderr)
		}
		if git.currentCalls != 1 || git.fetchCalls != 1 || len(git.rebasedBases) != 0 || len(git.mergedBranches) != 0 {
			t.Fatalf(
				"sync check calls = current:%d fetch:%d rebase:%v merge:%v",
				git.currentCalls,
				git.fetchCalls,
				git.rebasedBases,
				git.mergedBranches,
			)
		}
		fields := utilityJSONFields(t, assertSingleUtilityJSONResult(t, stdout, "branch.sync-base"))
		if fields["branch"] != "feature/ABC-123-add-export" ||
			fields["base"] != "origin/develop" ||
			fields["publication"] != string(branch.PublicationUnpublished) ||
			fields["missingBaseCommits"] != "false" ||
			fields["mutated"] != "false" ||
			fields["recommendedAction"] != "none" {
			t.Fatalf("sync check fields = %#v", fields)
		}
		if _, found := fields["qualityStatus"]; found {
			t.Fatalf("non-mutating sync unexpectedly reported quality: %#v", fields)
		}
	})

	t.Run("rejects an explicitly named branch that is not checked out", func(t *testing.T) {
		git := newBranchCommandGit(t, "feature/ABC-123-add-export")
		application := newBranchCommandApplication(git, nil, nil, "human")

		_, _, err := executeBranchCommand(
			t,
			newBranchSyncBaseCommand(application),
			context.Background(),
			"--branch", "fix/ABC-123-other-work",
			"--strategy", "merge",
			"--merge-message", "fix(ABC-123): synchronize develop",
		)
		assertProblemCode(t, err, problem.CodeInvalidInput)
		if git.fetchCalls != 0 || len(git.mergedBranches) != 0 {
			t.Fatalf("noncurrent branch synchronization mutated Git: fetch=%d merge=%v", git.fetchCalls, git.mergedBranches)
		}
	})

	t.Run("preserves current branch lookup failure for an explicit branch", func(t *testing.T) {
		currentErr := errors.New("current branch unavailable")
		git := newBranchCommandGit(t, "feature/ABC-123-add-export")
		git.currentErr = currentErr
		application := newBranchCommandApplication(git, nil, nil, "human")

		_, _, err := executeBranchCommand(
			t,
			newBranchSyncBaseCommand(application),
			context.Background(),
			"--branch", "feature/ABC-123-add-export",
		)
		if !errors.Is(err, currentErr) {
			t.Fatalf("explicit branch current lookup error = %v, want %v", err, currentErr)
		}
		if git.fetchCalls != 0 || len(git.mergedBranches) != 0 {
			t.Fatalf("current branch lookup failure mutated Git: fetch=%d merge=%v", git.fetchCalls, git.mergedBranches)
		}
	})

	t.Run("merges a published branch and reports post-mutation quality", func(t *testing.T) {
		git := newBranchCommandGit(t, "feature/ABC-123-add-export")
		git.publication = branch.PublicationPublished
		git.missingBaseCommits = true
		quality := &branchCommandQuality{
			result: port.QualityResult{
				Status: port.QualityPassed,
				Detail: "all branch gates passed",
			},
		}
		application := newBranchCommandApplication(git, quality, nil, "json")
		application.options.yes = true

		stdout, _, err := executeBranchCommand(
			t,
			newBranchSyncBaseCommand(application),
			context.Background(),
			"--branch", "feature/ABC-123-add-export",
			"--base", "origin/develop",
			"--strategy", "merge",
			"--merge-message", "chore(ABC-123): merge origin/develop",
		)
		if err != nil {
			t.Fatalf("merge sync error = %v", err)
		}
		if len(git.mergedBranches) != 1 || git.mergedBranches[0].base.String() != "origin/develop" {
			t.Fatalf("merge calls = %#v", git.mergedBranches)
		}
		if quality.calls != 1 {
			t.Fatalf("quality calls = %d, want 1", quality.calls)
		}
		fields := utilityJSONFields(t, assertSingleUtilityJSONResult(t, stdout, "branch.sync-base"))
		if fields["mutated"] != "true" ||
			fields["recommendedAction"] != "merged" ||
			fields["qualityStatus"] != string(port.QualityPassed) ||
			fields["qualityDetail"] != "all branch gates passed" {
			t.Fatalf("merge sync fields = %#v", fields)
		}
	})

	t.Run("accepts structured merge input and rejects it for other strategies", func(t *testing.T) {
		git := newBranchCommandGit(t, "feature/ABC-123-add-export")
		git.publication = branch.PublicationPublished
		git.missingBaseCommits = true
		application := newBranchCommandApplication(git, nil, nil, "human")
		application.options.yes = true
		_, _, err := executeBranchCommand(
			t,
			newBranchSyncBaseCommand(application),
			context.Background(),
			"--strategy", "merge",
			"--merge-type", "chore",
			"--merge-subject", "merge origin/develop",
		)
		if err != nil || len(git.mergedBranches) != 1 ||
			git.mergedBranches[0].message.Header().String() != "chore(ABC-123): merge origin/develop" {
			t.Fatalf("structured merge = (%#v, %v)", git.mergedBranches, err)
		}

		_, _, err = executeBranchCommand(
			t,
			newBranchSyncBaseCommand(newBranchCommandApplication(newBranchCommandGit(t, "feature/ABC-123-add-export"), nil, nil, "human")),
			context.Background(),
			"--merge-type", "chore",
		)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("honors a declined rebase confirmation", func(t *testing.T) {
		prompt := &commandHelperPrompt{
			confirms: []commandHelperConfirmReply{{value: false}},
		}
		git := newBranchCommandGit(t, "feature/ABC-123-add-export")
		application := newBranchCommandApplication(git, nil, prompt, "human")

		_, _, err := executeBranchCommand(
			t,
			newBranchSyncBaseCommand(application),
			context.Background(),
			"--branch", "feature/ABC-123-add-export",
			"--strategy", "rebase",
		)
		assertProblemCode(t, err, problem.CodeOperationCancelled)
		if len(prompt.confirmRequests) != 1 ||
			prompt.confirmRequests[0].Label != "Synchronize branch base" ||
			!strings.Contains(prompt.confirmRequests[0].Description, "rebase") {
			t.Fatalf("sync confirmation request = %#v", prompt.confirmRequests)
		}
		if len(git.rebasedBases) != 0 {
			t.Fatalf("declined sync rebased %v", git.rebasedBases)
		}
	})

	discoverErr := errors.New("repository discovery failed")
	currentErr := errors.New("current branch failed")
	fetchErr := errors.New("fetch failed")
	for _, testCase := range []struct {
		name      string
		arguments []string
		configure func(*branchCommandGit)
		wantCode  problem.Code
		wantErr   error
	}{
		{
			name: "preserves discovery failures",
			configure: func(git *branchCommandGit) {
				git.discoverErr = discoverErr
			},
			wantErr: discoverErr,
		},
		{
			name: "preserves current branch failures",
			configure: func(git *branchCommandGit) {
				git.currentErr = currentErr
			},
			wantErr: currentErr,
		},
		{
			name:      "rejects bases from another remote",
			arguments: []string{"--branch", "feature/ABC-123-add-export", "--base", "upstream/develop"},
			wantCode:  problem.CodeBranchBaseInvalid,
		},
		{
			name:      "rejects malformed merge messages before confirmation",
			arguments: []string{"--branch", "feature/ABC-123-add-export", "--strategy", "merge", "--merge-message", "not a commit message"},
			wantCode:  problem.CodeCommitHeaderInvalid,
		},
		{
			name:      "preserves synchronization service failures",
			arguments: []string{"--branch", "feature/ABC-123-add-export"},
			configure: func(git *branchCommandGit) {
				git.fetchErr = fetchErr
			},
			wantErr: fetchErr,
		},
		{
			name:      "returns invalid strategy errors from the synchronizer",
			arguments: []string{"--branch", "feature/ABC-123-add-export", "--strategy", "rewrite"},
			wantCode:  problem.CodeInvalidInput,
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			git := newBranchCommandGit(t, "feature/ABC-123-add-export")
			if testCase.configure != nil {
				testCase.configure(git)
			}
			_, _, err := executeBranchCommand(
				t,
				newBranchSyncBaseCommand(newBranchCommandApplication(git, nil, nil, "human")),
				context.Background(),
				testCase.arguments...,
			)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("sync error = %v, want %v", err, testCase.wantErr)
				}
			} else {
				assertProblemCode(t, err, testCase.wantCode)
			}
		})
	}

	t.Run("stops synchronization when the command context is cancelled", func(t *testing.T) {
		git := newBranchCommandGit(t, "feature/ABC-123-add-export")
		application := newBranchCommandApplication(git, nil, nil, "human")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, _, err := executeBranchCommand(
			t,
			newBranchSyncBaseCommand(application),
			ctx,
			"--branch", "feature/ABC-123-add-export",
		)
		assertProblemCode(t, err, problem.CodeOperationCancelled)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled sync error = %v, want context cancellation", err)
		}
	})
}

func TestBranchSyncBaseResumeContracts(t *testing.T) {
	t.Run("continues a resolved rebase and reports JSON", func(t *testing.T) {
		git := newBranchCommandGit(t, "feature/ABC-123-add-export")
		git.active = true
		git.activeOperation = "rebase"
		git.missingBaseCommits = true
		quality := &branchCommandQuality{
			result: port.QualityResult{
				Status: port.QualityPassed,
				Detail: "all branch gates passed",
			},
		}
		application := newBranchCommandApplication(git, quality, nil, "json")
		application.options.yes = true

		stdout, _, err := executeBranchCommand(
			t,
			newBranchSyncBaseCommand(application),
			context.Background(),
			"--resume",
		)
		if err != nil {
			t.Fatalf("resume error = %v", err)
		}
		if git.active {
			t.Fatal("resolved rebase left the operation active")
		}
		fields := utilityJSONFields(t, assertSingleUtilityJSONResult(t, stdout, "branch.sync-base"))
		if fields["branch"] != "feature/ABC-123-add-export" ||
			fields["base"] != "origin/develop" ||
			fields["publication"] != string(branch.PublicationUnpublished) ||
			fields["mutated"] != "true" ||
			fields["recommendedAction"] != "rebased" ||
			fields["qualityStatus"] != string(port.QualityPassed) {
			t.Fatalf("resume fields = %#v", fields)
		}
		if quality.calls != 1 {
			t.Fatalf("quality calls = %d, want 1", quality.calls)
		}
	})

	t.Run("continues a resolved merge with target provenance", func(t *testing.T) {
		git := newBranchCommandGit(t, "feature/ABC-123-add-export")
		git.publication = branch.PublicationPublished
		git.active = true
		git.activeOperation = "merge"
		git.mergeTargetMatches = true
		git.missingBaseCommits = true
		application := newBranchCommandApplication(git, nil, nil, "json")
		application.options.yes = true

		stdout, _, err := executeBranchCommand(
			t,
			newBranchSyncBaseCommand(application),
			context.Background(),
			"--resume",
		)
		if err != nil {
			t.Fatalf("merge resume error = %v", err)
		}
		if git.continueMergeCalls != 1 {
			t.Fatalf("continue merge calls = %d, want 1", git.continueMergeCalls)
		}
		fields := utilityJSONFields(t, assertSingleUtilityJSONResult(t, stdout, "branch.sync-base"))
		if fields["mutated"] != "true" ||
			fields["recommendedAction"] != "merged" ||
			fields["publication"] != string(branch.PublicationPublished) {
			t.Fatalf("merge resume fields = %#v", fields)
		}
	})

	t.Run("verifies an externally completed synchronization", func(t *testing.T) {
		git := newBranchCommandGit(t, "feature/ABC-123-add-export")
		application := newBranchCommandApplication(git, nil, nil, "json")
		application.options.yes = true

		stdout, _, err := executeBranchCommand(
			t,
			newBranchSyncBaseCommand(application),
			context.Background(),
			"--resume",
		)
		if err != nil {
			t.Fatalf("external resume error = %v", err)
		}
		fields := utilityJSONFields(t, assertSingleUtilityJSONResult(t, stdout, "branch.sync-base"))
		if fields["mutated"] != "true" || fields["recommendedAction"] != "rebased" {
			t.Fatalf("external resume fields = %#v", fields)
		}
	})

	t.Run("rejects resume with dry-run, a strategy, or merge inputs", func(t *testing.T) {
		for _, testCase := range []struct {
			name      string
			arguments []string
			dryRun    bool
		}{
			{name: "dry-run", arguments: []string{"--resume"}, dryRun: true},
			{name: "strategy", arguments: []string{"--resume", "--strategy", "rebase"}},
			{name: "merge message", arguments: []string{"--resume", "--merge-message", "chore(ABC-123): merge origin/develop"}},
			{name: "merge type", arguments: []string{"--resume", "--merge-type", "chore"}},
			{name: "merge subject", arguments: []string{"--resume", "--merge-subject", "merge origin/develop"}},
		} {
			t.Run(testCase.name, func(t *testing.T) {
				git := newBranchCommandGit(t, "feature/ABC-123-add-export")
				git.active = true
				git.activeOperation = "rebase"
				application := newBranchCommandApplication(git, nil, nil, "human")
				application.options.yes = true
				application.options.dryRun = testCase.dryRun

				_, _, err := executeBranchCommand(
					t,
					newBranchSyncBaseCommand(application),
					context.Background(),
					testCase.arguments...,
				)
				assertProblemCode(t, err, problem.CodeInvalidInput)
				if !git.active {
					t.Fatalf("rejected resume mutated the paused operation: %v", testCase.arguments)
				}
			})
		}
	})

	t.Run("honors a declined resume confirmation", func(t *testing.T) {
		prompt := &commandHelperPrompt{
			confirms: []commandHelperConfirmReply{{value: false}},
		}
		git := newBranchCommandGit(t, "feature/ABC-123-add-export")
		git.active = true
		git.activeOperation = "rebase"
		application := newBranchCommandApplication(git, nil, prompt, "human")

		_, _, err := executeBranchCommand(
			t,
			newBranchSyncBaseCommand(application),
			context.Background(),
			"--resume",
		)
		assertProblemCode(t, err, problem.CodeOperationCancelled)
		if len(prompt.confirmRequests) != 1 ||
			prompt.confirmRequests[0].Label != "Resume branch base synchronization" ||
			!strings.Contains(prompt.confirmRequests[0].Description, "rebase or merge") {
			t.Fatalf("resume confirmation request = %#v", prompt.confirmRequests)
		}
		if !git.active {
			t.Fatal("declined resume continued the paused rebase")
		}
	})

	t.Run("propagates a still conflicted rebase as a governed conflict problem", func(t *testing.T) {
		continueErr := errors.New("unresolved conflicts")
		git := newBranchCommandGit(t, "feature/ABC-123-add-export")
		git.active = true
		git.activeOperation = "rebase"
		git.continueRebaseErr = continueErr
		application := newBranchCommandApplication(git, nil, nil, "human")
		application.options.yes = true

		_, _, err := executeBranchCommand(
			t,
			newBranchSyncBaseCommand(application),
			context.Background(),
			"--resume",
		)
		assertProblemCode(t, err, problem.CodeRebaseConflict)
		if !errors.Is(err, continueErr) {
			t.Fatalf("resume conflict error = %v, want %v", err, continueErr)
		}
	})

	t.Run("rejects a paused rebase on a published branch", func(t *testing.T) {
		git := newBranchCommandGit(t, "feature/ABC-123-add-export")
		git.publication = branch.PublicationPublished
		git.active = true
		git.activeOperation = "rebase"
		application := newBranchCommandApplication(git, nil, nil, "human")
		application.options.yes = true

		_, _, err := executeBranchCommand(
			t,
			newBranchSyncBaseCommand(application),
			context.Background(),
			"--resume",
		)
		assertProblemCode(t, err, problem.CodeRebaseAfterPublishForbidden)
	})
}

func TestScratchMergeCommandFailureContracts(t *testing.T) {
	const source = "scratch/ABC-123-export-exploration"
	target, err := branch.ParseName("feature/ABC-123-add-export")
	if err != nil {
		t.Fatal(err)
	}
	message := "feat(ABC-123): add export"
	newGit := func() *branchCommandGit {
		git := newBranchCommandGit(t, source)
		git.officialBranches = []branch.BranchName{target}
		git.localBranches = map[string]bool{
			source:          true,
			target.String(): true,
		}
		return git
	}

	t.Run("propagates discovery and current branch failures", func(t *testing.T) {
		discoverErr := errors.New("repository discovery failed")
		git := newGit()
		git.discoverErr = discoverErr
		_, _, err := executeBranchCommand(
			t,
			newScratchMergeCommand(newBranchCommandApplication(git, nil, nil, "human")),
			context.Background(),
		)
		if !errors.Is(err, discoverErr) {
			t.Fatalf("discovery error = %v, want %v", err, discoverErr)
		}

		currentErr := errors.New("current branch failed")
		git = newGit()
		git.currentErr = currentErr
		_, _, err = executeBranchCommand(
			t,
			newScratchMergeCommand(newBranchCommandApplication(git, nil, nil, "human")),
			context.Background(),
		)
		if !errors.Is(err, currentErr) {
			t.Fatalf("current branch error = %v, want %v", err, currentErr)
		}
	})

	t.Run("rejects invalid targets missing sources and invalid messages", func(t *testing.T) {
		git := newGit()
		_, _, err := executeBranchCommand(
			t,
			newScratchMergeCommand(newBranchCommandApplication(git, nil, nil, "human")),
			context.Background(),
			"--target", "not-a-branch",
		)
		assertProblemCode(t, err, problem.CodeBranchNameInvalid)

		git = newGit()
		git.localBranches[source] = false
		_, _, err = executeBranchCommand(
			t,
			newScratchMergeCommand(newBranchCommandApplication(git, nil, nil, "human")),
			context.Background(),
			"--message", message,
		)
		assertProblemCode(t, err, problem.CodeScratchSourceBranchMissing)

		git = newGit()
		_, _, err = executeBranchCommand(
			t,
			newScratchMergeCommand(newBranchCommandApplication(git, nil, nil, "human")),
			context.Background(),
			"--message", "not a Conventional Commit",
		)
		assertProblemCode(t, err, problem.CodeCommitHeaderInvalid)
	})

	t.Run("propagates squash merge failures after confirmation", func(t *testing.T) {
		squashErr := errors.New("squash conflict")
		git := newGit()
		git.squashErr = squashErr
		application := newBranchCommandApplication(git, nil, nil, "human")
		application.options.yes = true
		_, _, err := executeBranchCommand(
			t,
			newScratchMergeCommand(application),
			context.Background(),
			"--message", message,
		)
		if !errors.Is(err, squashErr) {
			t.Fatalf("squash merge error = %v, want %v", err, squashErr)
		}
	})
}

func TestScratchMergeCommandContracts(t *testing.T) {
	source := "scratch/ABC-123-export-exploration"
	target, err := branch.ParseName("feature/ABC-123-add-export")
	if err != nil {
		t.Fatal(err)
	}
	message := "feat(ABC-123): add export"

	t.Run("squashes the current scratch branch in noninteractive automation", func(t *testing.T) {
		git := newBranchCommandGit(t, source)
		git.officialBranches = []branch.BranchName{target}
		git.localBranches = map[string]bool{
			source:          true,
			target.String(): true,
		}
		application := newBranchCommandApplication(git, nil, nil, "json")
		application.options.yes = true

		stdout, stderr, err := executeBranchCommand(
			t,
			newScratchMergeCommand(application),
			context.Background(),
			"--target", target.String(),
			"--message", message,
		)
		if err != nil {
			t.Fatalf("scratch merge error = %v", err)
		}
		if stderr != "" {
			t.Fatalf("scratch merge stderr = %q", stderr)
		}
		fields := utilityJSONFields(t, assertSingleUtilityJSONResult(t, stdout, "branch.merge-scratch"))
		if fields["scratchBranch"] != source ||
			fields["officialBranch"] != target.String() ||
			fields["commit"] != message ||
			fields["committed"] != "true" ||
			fields["dryRun"] != "false" ||
			!strings.Contains(fields["plan"], "squash-merge: "+source+" into "+target.String()) {
			t.Fatalf("scratch merge fields = %#v", fields)
		}
		if len(git.switchedBranches) != 1 || git.switchedBranches[0] != target ||
			len(git.squashedBranches) != 1 || git.squashedBranches[0].String() != source ||
			len(git.committedMessages) != 1 || git.committedMessages[0].String() != message {
			t.Fatalf(
				"scratch merge calls = switched:%#v squashed:%#v committed:%#v",
				git.switchedBranches,
				git.squashedBranches,
				git.committedMessages,
			)
		}
	})

	t.Run("shows source and target before confirmation", func(t *testing.T) {
		git := newBranchCommandGit(t, source)
		git.officialBranches = []branch.BranchName{target}
		git.localBranches = map[string]bool{
			source:          true,
			target.String(): true,
		}
		prompt := &commandHelperPrompt{
			confirms: []commandHelperConfirmReply{{value: false}},
		}
		application := newBranchCommandApplication(git, nil, prompt, "human")

		_, _, err := executeBranchCommand(
			t,
			newScratchMergeCommand(application),
			context.Background(),
			"--message", message,
		)
		assertProblemCode(t, err, problem.CodeOperationCancelled)
		if len(prompt.confirmRequests) != 1 ||
			prompt.confirmRequests[0].Label != "Squash merge scratch branch" ||
			!strings.Contains(prompt.confirmRequests[0].Description, source) ||
			!strings.Contains(prompt.confirmRequests[0].Description, target.String()) {
			t.Fatalf("scratch merge confirmation = %#v", prompt.confirmRequests)
		}
		if len(git.squashedBranches) != 0 || len(git.committedMessages) != 0 {
			t.Fatalf(
				"declined scratch merge mutated Git: squashed=%#v committed=%#v",
				git.squashedBranches,
				git.committedMessages,
			)
		}
	})
}

func TestBranchCommandReportHelpers(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		result branchapp.CreateResult
		want   string
	}{
		{
			name:   "dry run",
			result: branchapp.CreateResult{DryRun: true},
			want:   "Branch creation plan generated.",
		},
		{
			name: "created branch",
			want: "Branch created.",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := branchCreationSummary(testCase.result); got != testCase.want {
				t.Fatalf("branchCreationSummary(%#v) = %q, want %q", testCase.result, got, testCase.want)
			}
		})
	}

	plan := []branchapp.PlanStep{
		{Action: "fetch", Detail: "git fetch --prune origin"},
		{Action: "create", Detail: "feature/ABC-123-add-export from origin/develop"},
	}
	if got := planText(plan); got != "fetch: git fetch --prune origin; create: feature/ABC-123-add-export from origin/develop" {
		t.Fatalf("planText() = %q", got)
	}
	if got := planText(nil); got != "" {
		t.Fatalf("empty planText() = %q", got)
	}
	if got := boolString(true); got != "true" {
		t.Fatalf("boolString(true) = %q", got)
	}
	if got := boolString(false); got != "false" {
		t.Fatalf("boolString(false) = %q", got)
	}
	for _, testCase := range []struct {
		result branchapp.ScratchMergeResult
		want   string
	}{
		{result: branchapp.ScratchMergeResult{DryRun: true}, want: "Scratch squash-merge plan generated."},
		{result: branchapp.ScratchMergeResult{}, want: "Scratch branch squashed into the official ticket branch."},
	} {
		if got := scratchMergeSummary(testCase.result); got != testCase.want {
			t.Fatalf("scratchMergeSummary(%#v) = %q, want %q", testCase.result, got, testCase.want)
		}
	}
}

func TestBranchCommandFlagParsingFailure(t *testing.T) {
	command := NewWithRuntime(
		BuildInfo{Version: "test"},
		commandRuntime(newBranchCommandGit(t, "feature/ABC-123-add-export")),
	)
	_, err := executeBootstrapCommand(
		t,
		command,
		"branch", "create",
		"--switch=not-a-boolean",
	)
	assertProblemCode(t, err, problem.CodeInvalidInput)
}

func executeBranchCommand(
	t *testing.T,
	command *cobra.Command,
	ctx context.Context,
	arguments ...string,
) (string, string, error) {
	t.Helper()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	command.SetOut(stdout)
	command.SetErr(stderr)
	command.SetArgs(arguments)
	err := command.ExecuteContext(ctx)
	return stdout.String(), stderr.String(), err
}

func newBranchCommandApplication(
	git port.GitRepository,
	quality port.QualityRunner,
	prompt port.Prompt,
	output string,
) *application {
	if quality == nil {
		quality = &branchCommandQuality{
			result: port.QualityResult{
				Status: port.QualityUnconfigured,
				Detail: "quality is not configured",
			},
		}
	}
	runtime := commandRuntime(git)
	runtime.Quality = quality
	runtime.PromptFactory = func(bool, string) port.Prompt {
		return prompt
	}
	runtime.InputIsTerminal = func() bool {
		return prompt != nil
	}
	runtime.OutputIsTerminal = func() bool {
		return prompt != nil
	}
	interactive := "never"
	if prompt != nil {
		interactive = "auto"
	}
	return newApplication(runtime, &appOptions{
		interactive: interactive,
		output:      output,
		color:       "never",
		remote:      "origin",
		repository:  "C:/repo",
		timeout:     time.Second,
	})
}

func branchNames(names []branch.BranchName) []string {
	result := make([]string, 0, len(names))
	for _, name := range names {
		result = append(result, name.String())
	}
	return result
}

type branchCreateCall struct {
	name     branch.BranchName
	base     branch.TargetBase
	switchTo bool
}

type branchMergeCall struct {
	base    branch.TargetBase
	message commitmsg.Message
}

type branchCommandGit struct {
	*commandGit

	discoverErr           error
	currentErr            error
	validateErr           error
	hasCommitsErr         error
	worktreeCleanErr      error
	branchExistsErr       error
	officialBranchesErr   error
	fetchErr              error
	createErr             error
	publicationErr        error
	missingBaseCommitsErr error
	rebaseErr             error
	continueRebaseErr     error
	continueRebaseErrors  []error
	continueMergeErr      error
	continueMergeCalls    int
	mergeTargetMatches    bool
	mergeTargetMatchesErr error
	activeOperationErr    error
	unmergedConflictsErr  error
	mergeErr              error
	pushErr               error
	switchErr             error
	squashErr             error
	commitErr             error

	hasCommits         bool
	worktreeClean      bool
	branchExists       bool
	officialBranches   []branch.BranchName
	publication        branch.PublicationState
	missingBaseCommits bool
	localBranches      map[string]bool
	activeOperation    string
	active             bool
	unmergedConflicts  bool
	unmergedStates     []bool

	discoverContexts  []context.Context
	currentContexts   []context.Context
	currentCalls      int
	validatedBranches []branch.BranchName
	fetchCalls        int
	createdBranches   []branchCreateCall
	rebasedBases      []branch.TargetBase
	pushes            []branch.BranchName
	mergedBranches    []branchMergeCall
	switchedBranches  []branch.BranchName
	squashedBranches  []branch.BranchName
	committedMessages []commitmsg.Message
}

func newBranchCommandGit(t *testing.T, current string) *branchCommandGit {
	t.Helper()
	return &branchCommandGit{
		commandGit:    newCommandGit(t, current, nil),
		hasCommits:    true,
		worktreeClean: true,
		publication:   branch.PublicationUnpublished,
	}
}

func (git *branchCommandGit) Discover(ctx context.Context, directory string) (port.RepositoryIdentity, error) {
	git.discoverContexts = append(git.discoverContexts, ctx)
	if git.discoverErr != nil {
		return port.RepositoryIdentity{}, git.discoverErr
	}
	return git.commandGit.Discover(ctx, directory)
}

func (git *branchCommandGit) CurrentBranch(
	ctx context.Context,
	repository port.RepositoryIdentity,
) (branch.BranchName, error) {
	git.currentCalls++
	git.currentContexts = append(git.currentContexts, ctx)
	if git.currentErr != nil {
		return branch.BranchName{}, git.currentErr
	}
	return git.commandGit.CurrentBranch(ctx, repository)
}

func (git *branchCommandGit) ActiveOperation(context.Context, port.RepositoryIdentity) (string, bool, error) {
	if git.activeOperationErr != nil {
		return "", false, git.activeOperationErr
	}
	return git.activeOperation, git.active, nil
}

func (git *branchCommandGit) ValidateBranchRef(
	ctx context.Context,
	repository port.RepositoryIdentity,
	name branch.BranchName,
) error {
	git.validatedBranches = append(git.validatedBranches, name)
	if git.validateErr != nil {
		return git.validateErr
	}
	return git.commandGit.ValidateBranchRef(ctx, repository, name)
}

func (git *branchCommandGit) HasCommits(
	ctx context.Context,
	repository port.RepositoryIdentity,
) (bool, error) {
	if git.hasCommitsErr != nil {
		return false, git.hasCommitsErr
	}
	return git.hasCommits, nil
}

func (git *branchCommandGit) IsWorktreeClean(
	ctx context.Context,
	repository port.RepositoryIdentity,
) (bool, error) {
	if git.worktreeCleanErr != nil {
		return false, git.worktreeCleanErr
	}
	return git.worktreeClean, nil
}

func (git *branchCommandGit) BranchExists(
	ctx context.Context,
	repository port.RepositoryIdentity,
	name branch.BranchName,
) (bool, error) {
	if git.branchExistsErr != nil {
		return false, git.branchExistsErr
	}
	if git.localBranches != nil {
		return git.localBranches[name.String()], nil
	}
	return git.branchExists, nil
}

func (git *branchCommandGit) OfficialBranchesForTicket(
	ctx context.Context,
	repository port.RepositoryIdentity,
	id ticket.ID,
) ([]branch.BranchName, error) {
	if git.officialBranchesErr != nil {
		return nil, git.officialBranchesErr
	}
	return append([]branch.BranchName(nil), git.officialBranches...), nil
}

func (git *branchCommandGit) Fetch(ctx context.Context, repository port.RepositoryIdentity) error {
	git.fetchCalls++
	if git.fetchErr != nil {
		return git.fetchErr
	}
	return git.commandGit.Fetch(ctx, repository)
}

func (git *branchCommandGit) CreateBranch(
	ctx context.Context,
	repository port.RepositoryIdentity,
	name branch.BranchName,
	base branch.TargetBase,
	switchTo bool,
) error {
	if git.createErr != nil {
		return git.createErr
	}
	git.createdBranches = append(git.createdBranches, branchCreateCall{
		name:     name,
		base:     base,
		switchTo: switchTo,
	})
	return nil
}

func (git *branchCommandGit) PublicationState(
	ctx context.Context,
	repository port.RepositoryIdentity,
	name branch.BranchName,
) (branch.PublicationState, error) {
	if git.publicationErr != nil {
		return branch.PublicationUnknown, git.publicationErr
	}
	return git.publication, nil
}

func (git *branchCommandGit) HasMissingBaseCommits(
	ctx context.Context,
	repository port.RepositoryIdentity,
	base branch.TargetBase,
) (bool, error) {
	if git.missingBaseCommitsErr != nil {
		return false, git.missingBaseCommitsErr
	}
	return git.missingBaseCommits, nil
}

func (git *branchCommandGit) HasUnmergedConflicts(context.Context, port.RepositoryIdentity) (bool, error) {
	if git.unmergedConflictsErr != nil {
		return false, git.unmergedConflictsErr
	}
	if len(git.unmergedStates) > 0 {
		value := git.unmergedStates[0]
		git.unmergedStates = git.unmergedStates[1:]
		return value, nil
	}
	return git.unmergedConflicts, nil
}

func (git *branchCommandGit) Rebase(
	ctx context.Context,
	repository port.RepositoryIdentity,
	base branch.TargetBase,
) error {
	if git.rebaseErr != nil {
		return git.rebaseErr
	}
	git.rebasedBases = append(git.rebasedBases, base)
	return nil
}

func (git *branchCommandGit) ContinueRebase(context.Context, port.RepositoryIdentity) error {
	if len(git.continueRebaseErrors) > 0 {
		err := git.continueRebaseErrors[0]
		git.continueRebaseErrors = git.continueRebaseErrors[1:]
		if err != nil {
			return err
		}
	}
	if git.continueRebaseErr != nil {
		return git.continueRebaseErr
	}
	git.active = false
	git.activeOperation = ""
	git.missingBaseCommits = false
	return nil
}

func (git *branchCommandGit) ContinueMerge(context.Context, port.RepositoryIdentity) error {
	git.continueMergeCalls++
	if git.continueMergeErr != nil {
		return git.continueMergeErr
	}
	git.active = false
	git.activeOperation = ""
	git.missingBaseCommits = false
	return nil
}

func (git *branchCommandGit) ActiveMergeTargetMatches(context.Context, port.RepositoryIdentity, branch.TargetBase) (bool, error) {
	if git.mergeTargetMatchesErr != nil {
		return false, git.mergeTargetMatchesErr
	}
	return git.mergeTargetMatches, nil
}

func (git *branchCommandGit) Merge(
	ctx context.Context,
	repository port.RepositoryIdentity,
	base branch.TargetBase,
	message commitmsg.Message,
) error {
	if git.mergeErr != nil {
		return git.mergeErr
	}
	git.mergedBranches = append(git.mergedBranches, branchMergeCall{
		base:    base,
		message: message,
	})
	return nil
}

func (git *branchCommandGit) SwitchBranch(
	_ context.Context,
	_ port.RepositoryIdentity,
	name branch.BranchName,
) error {
	if git.switchErr != nil {
		return git.switchErr
	}
	git.switchedBranches = append(git.switchedBranches, name)
	return nil
}

func (git *branchCommandGit) SquashMerge(
	_ context.Context,
	_ port.RepositoryIdentity,
	source branch.BranchName,
) error {
	if git.squashErr != nil {
		return git.squashErr
	}
	git.squashedBranches = append(git.squashedBranches, source)
	return nil
}

func (git *branchCommandGit) Commit(
	_ context.Context,
	_ port.RepositoryIdentity,
	message commitmsg.Message,
) error {
	if git.commitErr != nil {
		return git.commitErr
	}
	git.committedMessages = append(git.committedMessages, message)
	return nil
}

func (git *branchCommandGit) Push(
	_ context.Context,
	_ port.RepositoryIdentity,
	name branch.BranchName,
	_ bool,
) error {
	if git.pushErr != nil {
		return git.pushErr
	}
	git.pushes = append(git.pushes, name)
	return nil
}

var _ port.GitRepository = (*branchCommandGit)(nil)

type branchCommandQuality struct {
	result   port.QualityResult
	err      error
	calls    int
	contexts []context.Context
	requests []port.QualityRequest
}

func (runner *branchCommandQuality) Run(
	ctx context.Context,
	repository port.RepositoryIdentity,
	request port.QualityRequest,
) (port.QualityResult, error) {
	runner.calls++
	runner.contexts = append(runner.contexts, ctx)
	cloned := request
	cloned.Families = append([]branch.Family(nil), request.Families...)
	runner.requests = append(runner.requests, cloned)
	if runner.err != nil {
		return port.QualityResult{}, runner.err
	}
	return runner.result, nil
}

var _ port.QualityRunner = (*branchCommandQuality)(nil)
