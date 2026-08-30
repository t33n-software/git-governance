package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	commitapp "github.com/t33n-software/git-governance/internal/application/commit"
	"github.com/t33n-software/git-governance/internal/application/policy"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

func TestCommandHelpersValidateOptions(t *testing.T) {
	t.Run("accepts supported defaults and an interactive terminal", func(t *testing.T) {
		options := newCommandHelperOptions()
		options.output = ""
		if err := newCommandHelperApplication(options, nil, true, true).validateOptions(); err != nil {
			t.Fatalf("validateOptions() error = %v", err)
		}

		options = newCommandHelperOptions()
		options.interactive = "always"
		if err := newCommandHelperApplication(options, nil, true, true).validateOptions(); err != nil {
			t.Fatalf("validateOptions() with terminal error = %v", err)
		}
	})

	for _, testCase := range []struct {
		name           string
		configure      func(*appOptions)
		inputTerminal  bool
		outputTerminal bool
		field          string
	}{
		{
			name: "rejects an unknown interaction mode",
			configure: func(options *appOptions) {
				options.interactive = "sometimes"
			},
			inputTerminal:  true,
			outputTerminal: true,
			field:          "interactive",
		},
		{
			name: "rejects an unknown output format",
			configure: func(options *appOptions) {
				options.output = "yaml"
			},
			inputTerminal:  true,
			outputTerminal: true,
			field:          "output",
		},
		{
			name: "rejects an unknown color mode",
			configure: func(options *appOptions) {
				options.color = "sometimes"
			},
			inputTerminal:  true,
			outputTerminal: true,
			field:          "color",
		},
		{
			name: "rejects an unknown pull request provider",
			configure: func(options *appOptions) {
				options.pullRequestProvider = "gitlab"
			},
			inputTerminal:  true,
			outputTerminal: true,
			field:          "pull-request-provider",
		},
		{
			name: "rejects a zero timeout",
			configure: func(options *appOptions) {
				options.timeout = 0
			},
			inputTerminal:  true,
			outputTerminal: true,
			field:          "timeout",
		},
		{
			name: "rejects a negative timeout",
			configure: func(options *appOptions) {
				options.timeout = -time.Second
			},
			inputTerminal:  true,
			outputTerminal: true,
			field:          "timeout",
		},
		{
			name: "rejects interactive JSON output",
			configure: func(options *appOptions) {
				options.interactive = "always"
				options.output = "json"
			},
			inputTerminal:  true,
			outputTerminal: true,
			field:          "interactive",
		},
		{
			name: "requires an interactive input terminal",
			configure: func(options *appOptions) {
				options.interactive = "always"
			},
			inputTerminal:  false,
			outputTerminal: true,
			field:          "interactive",
		},
		{
			name: "requires an interactive output terminal",
			configure: func(options *appOptions) {
				options.interactive = "always"
			},
			inputTerminal:  true,
			outputTerminal: false,
			field:          "interactive",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			options := newCommandHelperOptions()
			testCase.configure(options)

			err := newCommandHelperApplication(
				options,
				nil,
				testCase.inputTerminal,
				testCase.outputTerminal,
			).validateOptions()
			assertCommandHelperProblem(t, err, problem.CodeInvalidInput, problem.CategoryUsage, testCase.field)
		})
	}
}

func TestCommandHelpersReportAndDiscover(t *testing.T) {
	t.Run("reports through the command output contract", func(t *testing.T) {
		options := newCommandHelperOptions()
		options.output = "json"
		application := newCommandHelperApplication(options, nil, false, false)
		output := &bytes.Buffer{}
		command := &cobra.Command{}
		command.SetOut(output)
		command.SetContext(context.Background())

		err := application.report(command, port.Report{
			Operation: "helpers.report",
			Summary:   "reported",
			Fields:    map[string]string{"state": "complete"},
		})
		if err != nil {
			t.Fatalf("report() error = %v", err)
		}
		for _, expected := range []string{
			`"operation":"helpers.report"`,
			`"summary":"reported"`,
			`"state":"complete"`,
		} {
			if !strings.Contains(output.String(), expected) {
				t.Fatalf("report output missing %q: %q", expected, output.String())
			}
		}
	})

	t.Run("preserves report write and cancellation failures", func(t *testing.T) {
		writeErr := errors.New("writer unavailable")
		application := newCommandHelperApplication(newCommandHelperOptions(), nil, false, false)
		command := &cobra.Command{}
		command.SetOut(commandHelperFailingWriter{err: writeErr})
		command.SetContext(context.Background())

		err := application.report(command, port.Report{Summary: "reported"})
		assertCommandHelperProblem(t, err, problem.CodeExternalCommandFailed, problem.CategoryExternal, "output")
		if !errors.Is(err, writeErr) {
			t.Fatalf("report write error = %v, want %v", err, writeErr)
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		command.SetOut(&bytes.Buffer{})
		command.SetContext(ctx)
		err = application.report(command, port.Report{Summary: "reported"})
		assertCommandHelperProblem(t, err, problem.CodeOperationCancelled, problem.CategoryCancelled, "operation")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("report cancellation = %v, want context cancellation", err)
		}
	})

	t.Run("sets the selected remote and preserves discovery failures", func(t *testing.T) {
		git := newCommandHelperGit(t, "feature/ABC-123-add-export")
		options := newCommandHelperOptions()
		options.repository = "C:/workspace"
		options.remote = "governance"
		application := newCommandHelperApplication(options, nil, false, false)
		ctx := context.Background()

		identity, err := application.discover(ctx, services{git: git})
		if err != nil {
			t.Fatalf("discover() error = %v", err)
		}
		if identity.Root != "C:/found" || identity.Remote != "governance" {
			t.Fatalf("discovered identity = %#v", identity)
		}
		if len(git.discoverContexts) != 1 || git.discoverContexts[0] != ctx {
			t.Fatalf("discover contexts = %#v, want supplied context", git.discoverContexts)
		}
		if got := git.discoverDirectories; len(got) != 1 || got[0] != options.repository {
			t.Fatalf("discover directories = %v, want %q", got, options.repository)
		}

		discoverErr := errors.New("repository discovery failed")
		git.discoverErr = discoverErr
		identity, err = application.discover(ctx, services{git: git})
		if identity != (port.RepositoryIdentity{}) {
			t.Fatalf("failed discovery identity = %#v, want zero", identity)
		}
		if !errors.Is(err, discoverErr) {
			t.Fatalf("discover failure = %v, want %v", err, discoverErr)
		}
	})
}

func TestCommandHelpersInteractiveFetchSummary(t *testing.T) {
	const summary = "Branch created."

	interactive := newCommandHelperApplication(newCommandHelperOptions(), nil, true, true)
	if got, want := interactive.withInteractiveFetchSummary(summary, "origin", true),
		"🟢 Remote references fetched and stale references pruned from origin before this operation.\n"+summary; got != want {
		t.Fatalf("interactive fetch summary = %q, want %q", got, want)
	}

	for _, testCase := range []struct {
		name        string
		application *application
		fetched     bool
	}{
		{
			name:        "fetch was not performed",
			application: interactive,
			fetched:     false,
		},
		{
			name:        "noninteractive execution",
			application: newCommandHelperApplication(newCommandHelperOptions(), nil, false, false),
			fetched:     true,
		},
		{
			name: "JSON output",
			application: newCommandHelperApplication(func() *appOptions {
				options := newCommandHelperOptions()
				options.output = "json"
				return options
			}(), nil, true, true),
			fetched: true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.application.withInteractiveFetchSummary(summary, "origin", testCase.fetched); got != summary {
				t.Fatalf("suppressed fetch summary = %q, want %q", got, summary)
			}
		})
	}

	for _, testCase := range []struct {
		name   string
		dryRun bool
		plan   []branchapp.PlanStep
		want   bool
	}{
		{name: "dry run", dryRun: true, plan: []branchapp.PlanStep{{Action: "fetch"}}, want: false},
		{name: "plan without fetch", plan: []branchapp.PlanStep{{Action: "create"}}, want: false},
		{name: "completed fetch", plan: []branchapp.PlanStep{{Action: "fetch"}}, want: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := fetchCompleted(testCase.dryRun, testCase.plan); got != testCase.want {
				t.Fatalf("fetchCompleted(%t, %#v) = %t, want %t", testCase.dryRun, testCase.plan, got, testCase.want)
			}
		})
	}
}

func TestCommandHelpersConfirmMutation(t *testing.T) {
	t.Run("skips confirmation for dry runs and explicit consent", func(t *testing.T) {
		for _, testCase := range []struct {
			name      string
			configure func(*appOptions)
		}{
			{
				name: "dry run",
				configure: func(options *appOptions) {
					options.dryRun = true
				},
			},
			{
				name: "yes flag",
				configure: func(options *appOptions) {
					options.yes = true
				},
			},
		} {
			t.Run(testCase.name, func(t *testing.T) {
				options := newCommandHelperOptions()
				testCase.configure(options)
				prompt := &commandHelperPrompt{}

				err := newCommandHelperApplication(options, prompt, true, true).confirmMutation(
					context.Background(),
					"Create branch",
					"Creates an official branch.",
				)
				if err != nil {
					t.Fatalf("confirmMutation() error = %v", err)
				}
				if len(prompt.confirmRequests) != 0 {
					t.Fatalf("confirmation prompts = %#v, want none", prompt.confirmRequests)
				}
			})
		}
	})

	t.Run("requires explicit consent when prompting is unavailable", func(t *testing.T) {
		options := newCommandHelperOptions()
		options.interactive = "never"
		err := newCommandHelperApplication(options, nil, false, false).confirmMutation(
			context.Background(),
			"Create branch",
			"Creates an official branch.",
		)
		assertCommandHelperProblem(t, err, problem.CodeInvalidInput, problem.CategoryUsage, "confirmation")
	})

	t.Run("passes the request to the prompt and accepts confirmation", func(t *testing.T) {
		prompt := &commandHelperPrompt{
			confirms: []commandHelperConfirmReply{{value: true}},
		}
		application := newCommandHelperApplication(newCommandHelperOptions(), prompt, true, true)
		ctx := context.Background()

		if err := application.confirmMutation(ctx, "Create branch", "Creates an official branch."); err != nil {
			t.Fatalf("confirmMutation() error = %v", err)
		}
		if len(prompt.confirmRequests) != 1 {
			t.Fatalf("confirmation requests = %d, want 1", len(prompt.confirmRequests))
		}
		request := prompt.confirmRequests[0]
		if request.Label != "Create branch" || request.Description != "Creates an official branch." || request.Default {
			t.Fatalf("confirmation request = %#v", request)
		}
		if len(prompt.confirmContexts) != 1 || prompt.confirmContexts[0] != ctx {
			t.Fatalf("confirmation contexts = %#v, want supplied context", prompt.confirmContexts)
		}
	})

	t.Run("preserves prompt failures and returns a typed decline", func(t *testing.T) {
		promptErr := errors.New("terminal failed")
		prompt := &commandHelperPrompt{
			confirms: []commandHelperConfirmReply{{err: promptErr}},
		}
		application := newCommandHelperApplication(newCommandHelperOptions(), prompt, true, true)
		err := application.confirmMutation(context.Background(), "Create branch", "Creates an official branch.")
		if !errors.Is(err, promptErr) {
			t.Fatalf("confirmation prompt failure = %v, want %v", err, promptErr)
		}

		cancelledPrompt := &commandHelperPrompt{
			confirms: []commandHelperConfirmReply{{err: context.Canceled}},
		}
		err = newCommandHelperApplication(newCommandHelperOptions(), cancelledPrompt, true, true).confirmMutation(
			context.Background(),
			"Create branch",
			"Creates an official branch.",
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("confirmation cancellation = %v, want context cancellation", err)
		}

		decliningPrompt := &commandHelperPrompt{
			confirms: []commandHelperConfirmReply{{value: false}},
		}
		err = newCommandHelperApplication(newCommandHelperOptions(), decliningPrompt, true, true).confirmMutation(
			context.Background(),
			"Create branch",
			"Creates an official branch.",
		)
		assertCommandHelperProblem(t, err, problem.CodeOperationCancelled, problem.CategoryCancelled, "confirmation")
	})
}

func TestCommandHelpersResolveKey(t *testing.T) {
	t.Run("parses explicit keys without consulting preferences", func(t *testing.T) {
		key, err := newCommandHelperApplication(newCommandHelperOptions(), nil, false, false).resolveKey(
			context.Background(),
			services{},
			"ABC",
		)
		if err != nil || key.String() != "ABC" {
			t.Fatalf("resolveKey() = (%q, %v)", key.String(), err)
		}

		_, err = newCommandHelperApplication(newCommandHelperOptions(), nil, false, false).resolveKey(
			context.Background(),
			services{},
			"abc",
		)
		assertCommandHelperProblem(t, err, problem.CodeTicketKeyInvalid, problem.CategoryGovernance, "ticket key")
	})

	t.Run("uses the configured default during interactive input", func(t *testing.T) {
		defaultKey := mustCommandHelperKey(t, "PLATFORM")
		store := &commandHelperPreferencesStore{
			preferences: port.Preferences{DefaultKey: &defaultKey},
		}
		prompt := &commandHelperPrompt{
			inputs: []commandHelperStringReply{{value: "API"}},
		}
		application := newCommandHelperApplication(newCommandHelperOptions(), prompt, true, true)
		ctx := context.Background()

		key, err := application.resolveKey(ctx, services{
			preferences: policy.NewPreferencesService(store),
		}, "")
		if err != nil || key.String() != "API" {
			t.Fatalf("resolveKey() = (%q, %v)", key.String(), err)
		}
		if len(prompt.inputRequests) != 1 {
			t.Fatalf("input requests = %d, want 1", len(prompt.inputRequests))
		}
		request := prompt.inputRequests[0]
		if request.Label != "Ticket key" || request.Default != "PLATFORM" || !request.Required {
			t.Fatalf("ticket key request = %#v", request)
		}
		if len(prompt.inputContexts) != 1 || prompt.inputContexts[0] != ctx {
			t.Fatalf("ticket key contexts = %#v, want supplied context", prompt.inputContexts)
		}
	})

	t.Run("uses safe noninteractive defaults and preserves input failures", func(t *testing.T) {
		defaultKey := mustCommandHelperKey(t, "ABC")
		store := &commandHelperPreferencesStore{
			preferences: port.Preferences{DefaultKey: &defaultKey},
		}
		options := newCommandHelperOptions()
		options.interactive = "never"
		_, err := newCommandHelperApplication(options, nil, false, false).resolveKey(
			context.Background(),
			services{preferences: policy.NewPreferencesService(store)},
			"",
		)
		assertCommandHelperProblem(t, err, problem.CodeInvalidInput, problem.CategoryUsage, "ticket key")

		loadErr := errors.New("preferences unavailable")
		failingStore := &commandHelperPreferencesStore{loadErr: loadErr}
		prompt := &commandHelperPrompt{
			inputs: []commandHelperStringReply{{value: "DEF"}},
		}
		key, err := newCommandHelperApplication(newCommandHelperOptions(), prompt, true, true).resolveKey(
			context.Background(),
			services{preferences: policy.NewPreferencesService(failingStore)},
			"",
		)
		if err != nil || key.String() != "DEF" {
			t.Fatalf("resolveKey() after preference failure = (%q, %v)", key.String(), err)
		}
		if prompt.inputRequests[0].Default != "" {
			t.Fatalf("key default after preference failure = %q, want empty", prompt.inputRequests[0].Default)
		}

		cancelledPrompt := &commandHelperPrompt{
			inputs: []commandHelperStringReply{{err: context.Canceled}},
		}
		_, err = newCommandHelperApplication(newCommandHelperOptions(), cancelledPrompt, true, true).resolveKey(
			context.Background(),
			services{preferences: policy.NewPreferencesService(&commandHelperPreferencesStore{})},
			"",
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ticket key cancellation = %v, want context cancellation", err)
		}

		invalidPrompt := &commandHelperPrompt{
			inputs: []commandHelperStringReply{{value: "invalid"}},
		}
		_, err = newCommandHelperApplication(newCommandHelperOptions(), invalidPrompt, true, true).resolveKey(
			context.Background(),
			services{preferences: policy.NewPreferencesService(&commandHelperPreferencesStore{})},
			"",
		)
		assertCommandHelperProblem(t, err, problem.CodeTicketKeyInvalid, problem.CategoryGovernance, "ticket key")
	})
}

func TestCommandHelpersResolveNumberAndSlug(t *testing.T) {
	t.Run("resolves ticket numbers across explicit interactive and missing input paths", func(t *testing.T) {
		application := newCommandHelperApplication(newCommandHelperOptions(), nil, false, false)
		number, err := application.resolveNumber(context.Background(), "123")
		if err != nil || number.String() != "123" {
			t.Fatalf("resolveNumber() = (%q, %v)", number.String(), err)
		}
		_, err = application.resolveNumber(context.Background(), "012")
		assertCommandHelperProblem(t, err, problem.CodeTicketNumberInvalid, problem.CategoryGovernance, "ticket number")

		options := newCommandHelperOptions()
		options.interactive = "never"
		_, err = newCommandHelperApplication(options, nil, false, false).resolveNumber(context.Background(), "")
		assertCommandHelperProblem(t, err, problem.CodeInvalidInput, problem.CategoryUsage, "Ticket number")

		prompt := &commandHelperPrompt{
			inputs: []commandHelperStringReply{{value: "456"}},
		}
		number, err = newCommandHelperApplication(newCommandHelperOptions(), prompt, true, true).resolveNumber(
			context.Background(),
			"",
		)
		if err != nil || number.String() != "456" {
			t.Fatalf("interactive resolveNumber() = (%q, %v)", number.String(), err)
		}

		cancelledPrompt := &commandHelperPrompt{
			inputs: []commandHelperStringReply{{err: context.Canceled}},
		}
		_, err = newCommandHelperApplication(newCommandHelperOptions(), cancelledPrompt, true, true).resolveNumber(
			context.Background(),
			"",
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ticket number cancellation = %v, want context cancellation", err)
		}
	})

	t.Run("resolves slugs without normalizing malformed input", func(t *testing.T) {
		application := newCommandHelperApplication(newCommandHelperOptions(), nil, false, false)
		slug, err := application.resolveSlug(context.Background(), "add-export", "Branch slug")
		if err != nil || slug.String() != "add-export" {
			t.Fatalf("resolveSlug() = (%q, %v)", slug.String(), err)
		}
		_, err = application.resolveSlug(context.Background(), "Add Export", "Branch slug")
		assertCommandHelperProblem(t, err, problem.CodeBranchSlugInvalid, problem.CategoryGovernance, "branch slug")

		options := newCommandHelperOptions()
		options.interactive = "never"
		_, err = newCommandHelperApplication(options, nil, false, false).resolveSlug(context.Background(), "", "Branch slug")
		assertCommandHelperProblem(t, err, problem.CodeInvalidInput, problem.CategoryUsage, "Branch slug")

		prompt := &commandHelperPrompt{
			inputs: []commandHelperStringReply{{value: "release-notes"}},
		}
		slug, err = newCommandHelperApplication(newCommandHelperOptions(), prompt, true, true).resolveSlug(
			context.Background(),
			"",
			"Branch slug",
		)
		if err != nil || slug.String() != "release-notes" {
			t.Fatalf("interactive resolveSlug() = (%q, %v)", slug.String(), err)
		}

		invalidPrompt := &commandHelperPrompt{
			inputs: []commandHelperStringReply{{value: "Release Notes"}},
		}
		_, err = newCommandHelperApplication(newCommandHelperOptions(), invalidPrompt, true, true).resolveSlug(
			context.Background(),
			"",
			"Branch slug",
		)
		assertCommandHelperProblem(t, err, problem.CodeBranchSlugInvalid, problem.CategoryGovernance, "branch slug")
	})
}

func TestCommandHelpersResolveScratchMergeMessage(t *testing.T) {
	target, err := branch.ParseName("feature/ABC-123-add-export")
	if err != nil {
		t.Fatal(err)
	}
	application := newCommandHelperApplication(newCommandHelperOptions(), nil, false, false)

	message, err := application.resolveScratchMergeMessage(
		context.Background(),
		port.RepositoryIdentity{},
		target,
		"feat",
		"add export",
		"## Motivation\n\nDocuments the discarded experiment paths.",
		false,
		"",
		nil,
	)
	if err != nil || message.Header().String() != "feat(ABC-123): add export" {
		t.Fatalf("resolveScratchMergeMessage() = (%q, %v)", message.String(), err)
	}
	if message.Body() == "" {
		t.Fatal("resolveScratchMergeMessage() accepted a missing body")
	}

	_, err = application.resolveScratchMergeMessage(
		context.Background(),
		port.RepositoryIdentity{},
		target,
		"feat",
		"add export",
		"",
		false,
		"",
		nil,
	)
	assertCommandHelperProblem(t, err, problem.CodeCommitBodyRequired, problem.CategoryGovernance, "commit body")

	_, err = application.resolveScratchMergeMessage(
		context.Background(),
		port.RepositoryIdentity{},
		target,
		"feat",
		"feat(ABC-123): add export",
		"## Motivation\n\nDocuments the discarded experiment paths.",
		false,
		"",
		nil,
	)
	assertCommandHelperProblem(t, err, problem.CodeCommitDescriptionInvalid, problem.CategoryGovernance, "commit subject")

	options := newCommandHelperOptions()
	options.interactive = "never"
	_, err = newCommandHelperApplication(options, nil, false, false).resolveScratchMergeMessage(
		context.Background(),
		port.RepositoryIdentity{},
		target,
		"",
		"",
		"",
		false,
		"",
		nil,
	)
	assertCommandHelperProblem(t, err, problem.CodeInvalidInput, problem.CategoryUsage, "commit family")

	prompt := &commandHelperPrompt{
		selects: []commandHelperStringReply{{value: "feat"}},
		inputs: []commandHelperStringReply{
			{value: "add export"},
			{value: "## Motivation\n\nDocuments the discarded experiment paths."},
		},
	}
	interactive := newCommandHelperApplication(newCommandHelperOptions(), prompt, true, true)
	message, err = interactive.resolveScratchMergeMessage(context.Background(), port.RepositoryIdentity{}, target, "", "", "", false, "", nil)
	if err != nil || message.Header().String() != "feat(ABC-123): add export" {
		t.Fatalf("interactive resolveScratchMergeMessage() = (%q, %v)", message.String(), err)
	}
	if len(prompt.selectRequests) != 1 ||
		prompt.selectRequests[0].Label != "Commit family" ||
		len(prompt.selectRequests[0].Options) != len(commitapp.Families()) ||
		!strings.Contains(prompt.selectRequests[0].Description, target.String()) ||
		!strings.Contains(prompt.selectRequests[0].Description, "ABC-123") ||
		len(prompt.inputRequests) != 2 ||
		prompt.inputRequests[0].Label != "Squash commit description" ||
		!strings.Contains(prompt.inputRequests[0].Description, target.String()) ||
		!strings.Contains(prompt.inputRequests[0].Description, "ABC-123") ||
		prompt.inputRequests[1].Label != "Commit body" {
		t.Fatalf("scratch message prompts = selects:%#v inputs:%#v", prompt.selectRequests, prompt.inputRequests)
	}
}

func TestCommandHelpersResolveFamily(t *testing.T) {
	t.Run("parses explicitly supplied families", func(t *testing.T) {
		application := newCommandHelperApplication(newCommandHelperOptions(), nil, false, false)
		family, err := application.resolveFamily(context.Background(), "feature", false)
		if err != nil || family != branch.FamilyFeature {
			t.Fatalf("resolveFamily() = (%q, %v)", family, err)
		}
		_, err = application.resolveFamily(context.Background(), "unknown", false)
		assertCommandHelperProblem(t, err, problem.CodeBranchFamilyInvalid, problem.CategoryGovernance, "branch family")
	})

	t.Run("requires input outside an interactive terminal", func(t *testing.T) {
		options := newCommandHelperOptions()
		options.interactive = "never"
		_, err := newCommandHelperApplication(options, nil, false, false).resolveFamily(context.Background(), "", false)
		assertCommandHelperProblem(t, err, problem.CodeInvalidInput, problem.CategoryUsage, "branch family")
	})

	t.Run("offers direct families by default", func(t *testing.T) {
		prompt := &commandHelperPrompt{
			selects: []commandHelperStringReply{{value: "feature"}},
		}
		application := newCommandHelperApplication(newCommandHelperOptions(), prompt, true, true)
		ctx := context.Background()

		family, err := application.resolveFamily(ctx, "", false)
		if err != nil || family != branch.FamilyFeature {
			t.Fatalf("resolveFamily() = (%q, %v)", family, err)
		}
		if len(prompt.selectRequests) != 1 {
			t.Fatalf("select requests = %d, want 1", len(prompt.selectRequests))
		}
		request := prompt.selectRequests[0]
		if request.Label != "Branch family" || request.Default != branch.FamilyFeature.String() {
			t.Fatalf("family select request = %#v", request)
		}
		if len(request.Options) != commandHelperDirectFamilyCount() {
			t.Fatalf("direct family options = %d, want %d", len(request.Options), commandHelperDirectFamilyCount())
		}
		if commandHelperHasSelectOption(request.Options, branch.FamilyHotfix.String()) {
			t.Fatalf("direct family options unexpectedly include hotfix: %#v", request.Options)
		}
		if len(prompt.selectContexts) != 1 || prompt.selectContexts[0] != ctx {
			t.Fatalf("family select contexts = %#v, want supplied context", prompt.selectContexts)
		}
	})

	t.Run("includes special workflow families when requested", func(t *testing.T) {
		prompt := &commandHelperPrompt{
			selects: []commandHelperStringReply{{value: "hotfix"}},
		}
		family, err := newCommandHelperApplication(newCommandHelperOptions(), prompt, true, true).resolveFamily(
			context.Background(),
			"",
			true,
		)
		if err != nil || family != branch.FamilyHotfix {
			t.Fatalf("resolveFamily() = (%q, %v)", family, err)
		}
		if got, want := len(prompt.selectRequests[0].Options), len(branchapp.ListFamilies()); got != want {
			t.Fatalf("all family options = %d, want %d", got, want)
		}
		if !commandHelperHasSelectOption(prompt.selectRequests[0].Options, branch.FamilyHotfix.String()) {
			t.Fatalf("all family options omit hotfix: %#v", prompt.selectRequests[0].Options)
		}
	})

	t.Run("preserves selection failures and validates selections", func(t *testing.T) {
		selectErr := errors.New("selection unavailable")
		failingPrompt := &commandHelperPrompt{
			selects: []commandHelperStringReply{{err: selectErr}},
		}
		_, err := newCommandHelperApplication(newCommandHelperOptions(), failingPrompt, true, true).resolveFamily(
			context.Background(),
			"",
			false,
		)
		if !errors.Is(err, selectErr) {
			t.Fatalf("family selection failure = %v, want %v", err, selectErr)
		}

		invalidPrompt := &commandHelperPrompt{
			selects: []commandHelperStringReply{{value: "unknown"}},
		}
		_, err = newCommandHelperApplication(newCommandHelperOptions(), invalidPrompt, true, true).resolveFamily(
			context.Background(),
			"",
			true,
		)
		assertCommandHelperProblem(t, err, problem.CodeBranchFamilyInvalid, problem.CategoryGovernance, "branch family")
	})
}

func TestCommandHelpersParseTicketPartsBaseAndFooterSpecs(t *testing.T) {
	t.Run("parses ticket parts and retains component failures", func(t *testing.T) {
		id, err := parseTicketParts("ABC", "123")
		if err != nil || id.String() != "ABC-123" {
			t.Fatalf("parseTicketParts() = (%q, %v)", id.String(), err)
		}

		_, err = parseTicketParts("abc", "123")
		assertCommandHelperProblem(t, err, problem.CodeTicketKeyInvalid, problem.CategoryGovernance, "ticket key")
		_, err = parseTicketParts("ABC", "012")
		assertCommandHelperProblem(t, err, problem.CodeTicketNumberInvalid, problem.CategoryGovernance, "ticket number")
	})

	t.Run("parses selected remote bases and rejects unsafe references", func(t *testing.T) {
		base, err := parseBase("", "origin")
		if err != nil || base != nil {
			t.Fatalf("empty parseBase() = (%#v, %v)", base, err)
		}

		for _, raw := range []string{"develop", "origin/develop"} {
			base, err = parseBase(raw, "origin")
			if err != nil || base == nil || base.String() != "origin/develop" {
				t.Fatalf("parseBase(%q) = (%#v, %v)", raw, base, err)
			}
		}

		_, err = parseBase("upstream/develop", "origin")
		assertCommandHelperProblem(t, err, problem.CodeBranchBaseInvalid, problem.CategoryUsage, "base")
		_, err = parseBase("not-a-canonical-branch", "origin")
		assertCommandHelperProblem(t, err, problem.CodeBranchNameInvalid, problem.CategoryGovernance, "branch")
		_, err = parseBase("develop", "invalid remote")
		assertCommandHelperProblem(t, err, problem.CodeBranchBaseInvalid, problem.CategoryRepository, "remote")
	})

	t.Run("parses footer separators without accepting malformed values", func(t *testing.T) {
		footer, err := parseFooterSpec("Refs=#123=related")
		if err != nil || footer.String() != "Refs: #123=related" {
			t.Fatalf("parseFooterSpec() = (%q, %v)", footer.String(), err)
		}

		_, err = parseFooterSpec("Refs")
		assertCommandHelperProblem(t, err, problem.CodeCommitDescriptionInvalid, problem.CategoryUsage, "footer")
		_, err = parseFooterSpec("Refs=")
		assertCommandHelperProblem(t, err, problem.CodeCommitDescriptionInvalid, problem.CategoryGovernance, "commit message")
	})
}

func TestCommandHelpersResolveScratchBase(t *testing.T) {
	id := mustCommandHelperTicket(t, "ABC-123")

	t.Run("uses an explicit local official branch", func(t *testing.T) {
		base, err := newCommandHelperApplication(newCommandHelperOptions(), nil, false, false).resolveScratchBase(
			context.Background(),
			"feature/ABC-123-add-export",
			"origin",
			id,
		)
		if err != nil || base == nil || base.String() != "feature/ABC-123-add-export" || base.IsRemoteTracking() {
			t.Fatalf("resolveScratchBase() = (%#v, %v)", base, err)
		}
	})

	t.Run("requests an official local branch when none is supplied", func(t *testing.T) {
		prompt := &commandHelperPrompt{
			inputs: []commandHelperStringReply{{value: "fix/ABC-123-correct-export"}},
		}
		application := newCommandHelperApplication(newCommandHelperOptions(), prompt, true, true)
		ctx := context.Background()

		base, err := application.resolveScratchBase(ctx, "", "origin", id)
		if err != nil || base == nil || base.String() != "fix/ABC-123-correct-export" {
			t.Fatalf("interactive resolveScratchBase() = (%#v, %v)", base, err)
		}
		if len(prompt.inputRequests) != 1 {
			t.Fatalf("scratch base requests = %d, want 1", len(prompt.inputRequests))
		}
		request := prompt.inputRequests[0]
		if request.Label != "Official ticket branch base" || !request.Required {
			t.Fatalf("scratch base request = %#v", request)
		}
		if len(prompt.inputContexts) != 1 || prompt.inputContexts[0] != ctx {
			t.Fatalf("scratch base contexts = %#v, want supplied context", prompt.inputContexts)
		}
	})

	t.Run("returns missing input and prompt cancellation", func(t *testing.T) {
		options := newCommandHelperOptions()
		options.interactive = "never"
		_, err := newCommandHelperApplication(options, nil, false, false).resolveScratchBase(
			context.Background(),
			"",
			"origin",
			id,
		)
		assertCommandHelperProblem(t, err, problem.CodeInvalidInput, problem.CategoryUsage, "Official ticket branch base")

		cancelledPrompt := &commandHelperPrompt{
			inputs: []commandHelperStringReply{{err: context.Canceled}},
		}
		_, err = newCommandHelperApplication(newCommandHelperOptions(), cancelledPrompt, true, true).resolveScratchBase(
			context.Background(),
			"",
			"origin",
			id,
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("scratch base cancellation = %v, want context cancellation", err)
		}
	})

	t.Run("rejects remote malformed and nonofficial scratch bases", func(t *testing.T) {
		application := newCommandHelperApplication(newCommandHelperOptions(), nil, false, false)

		_, err := application.resolveScratchBase(context.Background(), "origin/feature/ABC-123-add-export", "origin", id)
		assertCommandHelperProblem(t, err, problem.CodeBranchBaseInvalid, problem.CategoryGovernance, "scratch base")
		_, err = application.resolveScratchBase(context.Background(), "not-a-branch", "origin", id)
		assertCommandHelperProblem(t, err, problem.CodeBranchNameInvalid, problem.CategoryGovernance, "branch")
		_, err = application.resolveScratchBase(context.Background(), "main", "origin", id)
		assertCommandHelperProblem(t, err, problem.CodeBranchBaseInvalid, problem.CategoryGovernance, "scratch base")
	})
}

func TestCommandHelpersCurrentOrSpecifiedAndReadCommitMessage(t *testing.T) {
	t.Run("uses explicit branches before querying the repository", func(t *testing.T) {
		git := newCommandHelperGit(t, "feature/ABC-123-add-export")
		repository := port.RepositoryIdentity{Root: "C:/found", Remote: "origin"}

		name, err := currentOrSpecified(context.Background(), services{git: git}, "develop", repository)
		if err != nil || name.String() != "develop" {
			t.Fatalf("currentOrSpecified() = (%q, %v)", name.String(), err)
		}
		if len(git.currentContexts) != 0 {
			t.Fatalf("explicit branch unexpectedly queried repository: %#v", git.currentContexts)
		}

		_, err = currentOrSpecified(context.Background(), services{git: git}, "not-a-branch", repository)
		assertCommandHelperProblem(t, err, problem.CodeBranchNameInvalid, problem.CategoryGovernance, "branch")

		ctx := context.Background()
		name, err = currentOrSpecified(ctx, services{git: git}, "", repository)
		if err != nil || name.String() != "feature/ABC-123-add-export" {
			t.Fatalf("repository current branch = (%q, %v)", name.String(), err)
		}
		if len(git.currentContexts) != 1 || git.currentContexts[0] != ctx {
			t.Fatalf("current branch contexts = %#v, want supplied context", git.currentContexts)
		}

		currentErr := errors.New("current branch unavailable")
		git.currentErr = currentErr
		_, err = currentOrSpecified(ctx, services{git: git}, "", repository)
		if !errors.Is(err, currentErr) {
			t.Fatalf("current branch failure = %v, want %v", err, currentErr)
		}
	})

	t.Run("reads inline and bounded file message input safely", func(t *testing.T) {
		tempDir := t.TempDir()
		messagePath := filepath.Join(tempDir, "message.txt")
		message := "feat(ABC-123): add export\n"
		if err := os.WriteFile(messagePath, []byte(message), 0o600); err != nil {
			t.Fatal(err)
		}

		actual, err := readCommitMessage(filepath.Join(tempDir, "missing.txt"), "feat(ABC-123): inline")
		if err != nil || actual != "feat(ABC-123): inline" {
			t.Fatalf("inline readCommitMessage() = (%q, %v)", actual, err)
		}

		actual, err = readCommitMessage(messagePath, "")
		if err != nil || actual != message {
			t.Fatalf("file readCommitMessage() = (%q, %v)", actual, err)
		}

		_, err = readCommitMessage("", "")
		assertCommandHelperProblem(t, err, problem.CodeInvalidInput, problem.CategoryUsage, "commit message")

		missingPath := filepath.Join(tempDir, "missing.txt")
		_, err = readCommitMessage(missingPath, "")
		assertCommandHelperProblem(t, err, problem.CodeConfigurationUnavailable, problem.CategoryUsage, "message file")
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("missing message error = %v, want not exist", err)
		}

		oversizedPath := filepath.Join(tempDir, "oversized.txt")
		if err := os.WriteFile(oversizedPath, bytes.Repeat([]byte("x"), maxCommitMessageBytes+1), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err = readCommitMessage(oversizedPath, "")
		assertCommandHelperProblem(t, err, problem.CodeCommitHeaderInvalid, problem.CategoryUsage, "message file")

		directoryPath := filepath.Join(tempDir, "message-directory")
		if err := os.Mkdir(directoryPath, 0o700); err != nil {
			t.Fatal(err)
		}
		_, err = readCommitMessage(directoryPath, "")
		assertCommandHelperProblem(t, err, problem.CodeConfigurationUnavailable, problem.CategoryUsage, "message file")
	})
}

func newCommandHelperOptions() *appOptions {
	return &appOptions{
		interactive: "auto",
		output:      "human",
		color:       "auto",
		remote:      "origin",
		repository:  "C:/repository",
		timeout:     time.Second,
	}
}

func newCommandHelperApplication(
	options *appOptions,
	prompt port.Prompt,
	inputTerminal bool,
	outputTerminal bool,
) *application {
	return newApplication(Runtime{
		PromptFactory: func(bool, string) port.Prompt {
			return prompt
		},
		InputIsTerminal: func() bool {
			return inputTerminal
		},
		OutputIsTerminal: func() bool {
			return outputTerminal
		},
	}, options)
}

func assertCommandHelperProblem(
	t *testing.T,
	err error,
	code problem.Code,
	category problem.Category,
	field string,
) *problem.Problem {
	t.Helper()
	if err == nil {
		t.Fatalf("expected problem %q, got nil", code)
	}
	actual, ok := problem.As(err)
	if !ok {
		t.Fatalf("error %T does not carry a problem: %v", err, err)
	}
	if actual.Code != code || actual.Category != category || actual.Field != field {
		t.Fatalf(
			"problem = {Code:%q Category:%q Field:%q}, want {Code:%q Category:%q Field:%q}",
			actual.Code,
			actual.Category,
			actual.Field,
			code,
			category,
			field,
		)
	}
	return actual
}

func mustCommandHelperKey(t *testing.T, raw string) ticket.Key {
	t.Helper()
	key, err := ticket.ParseKey(raw)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func mustCommandHelperTicket(t *testing.T, raw string) ticket.ID {
	t.Helper()
	id, err := ticket.ParseID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func commandHelperDirectFamilyCount() int {
	count := 0
	for _, family := range branchapp.ListFamilies() {
		if family.DirectlyCreatable {
			count++
		}
	}
	return count
}

func commandHelperHasSelectOption(options []port.SelectOption, value string) bool {
	for _, option := range options {
		if option.Value == value {
			return true
		}
	}
	return false
}

type commandHelperFailingWriter struct {
	err error
}

func (writer commandHelperFailingWriter) Write([]byte) (int, error) {
	return 0, writer.err
}

type commandHelperStringReply struct {
	value string
	err   error
}

type commandHelperConfirmReply struct {
	value bool
	err   error
}

type commandHelperPrompt struct {
	inputs   []commandHelperStringReply
	selects  []commandHelperStringReply
	confirms []commandHelperConfirmReply

	inputRequests   []port.InputRequest
	selectRequests  []port.SelectRequest
	confirmRequests []port.ConfirmRequest

	inputContexts   []context.Context
	selectContexts  []context.Context
	confirmContexts []context.Context
}

func (prompt *commandHelperPrompt) Input(ctx context.Context, request port.InputRequest) (string, error) {
	prompt.inputContexts = append(prompt.inputContexts, ctx)
	prompt.inputRequests = append(prompt.inputRequests, request)
	if len(prompt.inputs) == 0 {
		return "", errors.New("unexpected input prompt")
	}
	reply := prompt.inputs[0]
	prompt.inputs = prompt.inputs[1:]
	return reply.value, reply.err
}

func (prompt *commandHelperPrompt) Select(ctx context.Context, request port.SelectRequest) (string, error) {
	prompt.selectContexts = append(prompt.selectContexts, ctx)
	prompt.selectRequests = append(prompt.selectRequests, request)
	if len(prompt.selects) == 0 {
		return "", errors.New("unexpected select prompt")
	}
	reply := prompt.selects[0]
	prompt.selects = prompt.selects[1:]
	return reply.value, reply.err
}

func (prompt *commandHelperPrompt) Confirm(ctx context.Context, request port.ConfirmRequest) (bool, error) {
	prompt.confirmContexts = append(prompt.confirmContexts, ctx)
	prompt.confirmRequests = append(prompt.confirmRequests, request)
	if len(prompt.confirms) == 0 {
		return false, errors.New("unexpected confirmation prompt")
	}
	reply := prompt.confirms[0]
	prompt.confirms = prompt.confirms[1:]
	return reply.value, reply.err
}

var _ port.Prompt = (*commandHelperPrompt)(nil)

type commandHelperPreferencesStore struct {
	preferences port.Preferences
	loadErr     error
}

func (store *commandHelperPreferencesStore) Load(context.Context) (port.Preferences, error) {
	if store.loadErr != nil {
		return port.Preferences{}, store.loadErr
	}
	return store.preferences, nil
}

func (store *commandHelperPreferencesStore) Save(_ context.Context, preferences port.Preferences) error {
	store.preferences = preferences
	return nil
}

var _ port.PreferencesStore = (*commandHelperPreferencesStore)(nil)

type commandHelperGit struct {
	*commandGit

	discoverErr         error
	discoverContexts    []context.Context
	discoverDirectories []string
	currentErr          error
	currentContexts     []context.Context
}

func newCommandHelperGit(t *testing.T, current string) *commandHelperGit {
	t.Helper()
	return &commandHelperGit{
		commandGit: newCommandGit(t, current, nil),
	}
}

func (git *commandHelperGit) Discover(ctx context.Context, directory string) (port.RepositoryIdentity, error) {
	git.discoverContexts = append(git.discoverContexts, ctx)
	git.discoverDirectories = append(git.discoverDirectories, directory)
	if git.discoverErr != nil {
		return port.RepositoryIdentity{}, git.discoverErr
	}
	return port.RepositoryIdentity{Root: "C:/found", Remote: "upstream"}, nil
}

func (git *commandHelperGit) CurrentBranch(
	ctx context.Context,
	repository port.RepositoryIdentity,
) (branch.BranchName, error) {
	git.currentContexts = append(git.currentContexts, ctx)
	if git.currentErr != nil {
		return branch.BranchName{}, git.currentErr
	}
	return git.commandGit.CurrentBranch(ctx, repository)
}

var _ port.GitRepository = (*commandHelperGit)(nil)
