package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/adapters/github"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestDefaultRuntimeConstructsAllAdapters(t *testing.T) {
	runtime := defaultRuntime()

	if runtime.GitFactory == nil || runtime.StoreFactory == nil || runtime.KeyPolicy == nil ||
		runtime.QualityFactory == nil || runtime.GitHubAuthFactory == nil || runtime.Browser == nil ||
		runtime.GitHubCredentialBrokerURL == nil ||
		runtime.GitHubWorkloadIdentity == nil || runtime.GitHubWorkflowToken == nil ||
		runtime.GitHubWorkflowTokenEnabled == nil || runtime.HotfixPropagationPublisherEnabled == nil ||
		runtime.Tools == nil || runtime.PromptFactory == nil ||
		runtime.InputIsTerminal == nil || runtime.OutputIsTerminal == nil {
		t.Fatal("default runtime left a required dependency unset")
	}
	if runtime.GitFactory(time.Second) == nil {
		t.Fatal("default Git factory returned nil")
	}
	if runtime.StoreFactory("preferences.yaml") == nil {
		t.Fatal("default preferences store factory returned nil")
	}
	if runtime.QualityFactory("quality.yaml", time.Second) == nil {
		t.Fatal("default quality factory returned nil")
	}
	if runtime.GitHubAuthFactory(time.Second) == nil {
		t.Fatal("default GitHub authentication runtime dependencies are unavailable")
	}
	_ = runtime.GitHubCredentialBrokerURL()
	_ = runtime.GitHubWorkloadIdentity()
	_ = runtime.GitHubWorkflowToken()
	_ = runtime.GitHubWorkflowTokenEnabled()
	_ = runtime.HotfixPropagationPublisherEnabled()
	if runtime.PromptFactory(true, "never") == nil {
		t.Fatal("default prompt factory returned nil")
	}
}

func TestDefaultRuntimeHotfixPropagationPublisherRequiresManagedBoundary(t *testing.T) {
	t.Setenv("GIT_GOVERNANCE_HOTFIX_PROPAGATION_PUBLISHER", "server")
	t.Setenv("GIT_GOVERNANCE_GITHUB_CREDENTIAL_BROKER_URL", "https://broker.example")
	t.Setenv("GIT_GOVERNANCE_WORKLOAD_IDENTITY_TOKEN", "workload-identity")
	if !defaultRuntime().HotfixPropagationPublisherEnabled() {
		t.Fatal("default runtime did not enable the managed hotfix propagation publisher")
	}

	t.Setenv("GIT_GOVERNANCE_WORKLOAD_IDENTITY_TOKEN", "")
	if defaultRuntime().HotfixPropagationPublisherEnabled() {
		t.Fatal("default runtime enabled the publisher without workload identity")
	}

	t.Setenv("GIT_GOVERNANCE_WORKLOAD_IDENTITY_TOKEN", "workload-identity")
	t.Setenv("GIT_GOVERNANCE_HOTFIX_PROPAGATION_PUBLISHER", "local")
	if defaultRuntime().HotfixPropagationPublisherEnabled() {
		t.Fatal("default runtime enabled the publisher without the server marker")
	}
}

func TestDefaultRuntimeWorkflowTokenRequiresExplicitControllerMarker(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_TOKEN", "ephemeral-token")
	t.Setenv("GIT_GOVERNANCE_WORKFLOW_TOKEN", "server")
	runtime := defaultRuntime()
	if !runtime.GitHubWorkflowTokenEnabled() || runtime.GitHubWorkflowToken() != "ephemeral-token" {
		t.Fatal("default runtime did not enable the explicit protected-line controller token")
	}

	t.Setenv("GIT_GOVERNANCE_WORKFLOW_TOKEN", "local")
	if runtime.GitHubWorkflowTokenEnabled() {
		t.Fatal("default runtime accepted an unmarked workflow token")
	}
	t.Setenv("GITHUB_ACTIONS", "false")
	t.Setenv("GIT_GOVERNANCE_WORKFLOW_TOKEN", "server")
	if runtime.GitHubWorkflowTokenEnabled() {
		t.Fatal("default runtime accepted a controller marker outside GitHub Actions")
	}
}

func TestNewApplicationSuppliesAndPreservesRuntimeSeams(t *testing.T) {
	options := runtimeTestOptions()
	withFallbacks := newApplication(Runtime{}, options)
	if withFallbacks.options != options {
		t.Fatal("newApplication did not retain the options instance")
	}
	if withFallbacks.runtime.GitFactory == nil || withFallbacks.runtime.StoreFactory == nil ||
		withFallbacks.runtime.KeyPolicy == nil || withFallbacks.runtime.QualityFactory == nil ||
		withFallbacks.runtime.GitHubAuthFactory == nil || withFallbacks.runtime.Browser == nil ||
		withFallbacks.runtime.GitHubCredentialBrokerURL == nil ||
		withFallbacks.runtime.GitHubWorkloadIdentity == nil || withFallbacks.runtime.HotfixPropagationPublisherEnabled == nil ||
		withFallbacks.runtime.Tools == nil || withFallbacks.runtime.PromptFactory == nil ||
		withFallbacks.runtime.InputIsTerminal == nil || withFallbacks.runtime.OutputIsTerminal == nil {
		t.Fatal("newApplication did not supply all runtime fallbacks")
	}
	if withFallbacks.runtime.Quality != nil {
		t.Fatal("newApplication unexpectedly supplied a direct quality runner")
	}

	git := &runtimeTestGit{}
	store := &runtimeTestStore{}
	policy := &runtimeTestPolicy{}
	quality := &runtimeTestQuality{}
	factoryQuality := &runtimeTestQuality{}
	tools := &runtimeTestTools{}
	prompt := &runtimeTestPrompt{}
	githubAuth := &bootstrapAuthProvider{}
	browser := &bootstrapBrowserOpener{}
	inputTerminal := func() bool { return true }
	outputTerminal := func() bool { return false }
	application := newApplication(Runtime{
		GitFactory: func(time.Duration) port.GitRepository {
			return git
		},
		StoreFactory: func(string) port.PreferencesStore {
			return store
		},
		KeyPolicy: policy,
		Quality:   quality,
		QualityFactory: func(string, time.Duration) port.QualityRunner {
			return factoryQuality
		},
		GitHubAuthFactory: func(time.Duration) github.AuthProvider {
			return githubAuth
		},
		Browser: browser,
		GitHubCredentialBrokerURL: func() string {
			return "https://broker.example"
		},
		GitHubWorkloadIdentity: func() string {
			return "workload-identity"
		},
		GitHubWorkflowToken: func() string {
			return "ephemeral-token"
		},
		GitHubWorkflowTokenEnabled: func() bool {
			return true
		},
		HotfixPropagationPublisherEnabled: func() bool {
			return true
		},
		Tools: tools,
		PromptFactory: func(bool, string) port.Prompt {
			return prompt
		},
		InputIsTerminal:  inputTerminal,
		OutputIsTerminal: outputTerminal,
	}, options)

	if application.runtime.GitFactory(time.Second) != git ||
		application.runtime.StoreFactory("preferences.yaml") != store ||
		application.runtime.KeyPolicy != policy ||
		application.runtime.Quality != quality ||
		application.runtime.QualityFactory("quality.yaml", time.Second) != factoryQuality ||
		application.runtime.GitHubAuthFactory(time.Second) != githubAuth ||
		application.runtime.Browser != browser ||
		application.runtime.GitHubCredentialBrokerURL() != "https://broker.example" ||
		application.runtime.GitHubWorkloadIdentity() != "workload-identity" ||
		application.runtime.GitHubWorkflowToken() != "ephemeral-token" ||
		!application.runtime.GitHubWorkflowTokenEnabled() ||
		!application.runtime.HotfixPropagationPublisherEnabled() ||
		application.runtime.Tools != tools ||
		application.runtime.PromptFactory(false, "auto") != prompt {
		t.Fatal("newApplication replaced an injected runtime dependency")
	}
	if !application.inputIsTerminal() || application.outputIsTerminal() {
		t.Fatal("newApplication replaced injected terminal seams")
	}
}

func TestServicesWireDependenciesAndQualityFallback(t *testing.T) {
	options := runtimeTestOptions()
	git := &runtimeTestGit{}
	store := &runtimeTestStore{}
	quality := &runtimeTestQuality{}
	var gitTimeout, qualityTimeout time.Duration
	var storePath, qualityPath string
	qualityFactoryCalls := 0
	application := newApplication(Runtime{
		GitFactory: func(timeout time.Duration) port.GitRepository {
			gitTimeout = timeout
			return git
		},
		StoreFactory: func(path string) port.PreferencesStore {
			storePath = path
			return store
		},
		Quality: quality,
		QualityFactory: func(path string, timeout time.Duration) port.QualityRunner {
			qualityFactoryCalls++
			qualityPath = path
			qualityTimeout = timeout
			return &runtimeTestQuality{}
		},
		Tools: &runtimeTestTools{},
	}, options)

	services := application.services()
	if services.git != git || services.branches == nil || services.sync == nil || services.commits == nil ||
		services.tickets == nil || services.releases == nil || services.preferences == nil || services.doctor == nil {
		t.Fatal("services did not construct the complete application graph")
	}
	if gitTimeout != options.timeout || storePath != options.config {
		t.Fatalf("service factory arguments = timeout %s, store path %q", gitTimeout, storePath)
	}
	if qualityFactoryCalls != 0 {
		t.Fatalf("quality factory called %d times despite direct quality runner", qualityFactoryCalls)
	}

	factoryQuality := &runtimeTestQuality{}
	fallbackCalls := 0
	fallbackApplication := newApplication(Runtime{
		GitFactory: func(time.Duration) port.GitRepository {
			return git
		},
		StoreFactory: func(string) port.PreferencesStore {
			return store
		},
		QualityFactory: func(path string, timeout time.Duration) port.QualityRunner {
			fallbackCalls++
			qualityPath = path
			qualityTimeout = timeout
			return factoryQuality
		},
		Tools: &runtimeTestTools{},
	}, options)
	if fallbackApplication.services().sync == nil {
		t.Fatal("quality fallback did not produce a synchronizer")
	}
	if fallbackCalls != 1 || qualityPath != options.qualityConfig || qualityTimeout != options.timeout {
		t.Fatalf("quality fallback = calls %d, path %q, timeout %s", fallbackCalls, qualityPath, qualityTimeout)
	}

	githubOptions := runtimeTestOptions()
	githubOptions.pullRequestProvider = "github"
	githubApplication := newApplication(Runtime{
		GitFactory: func(time.Duration) port.GitRepository {
			return git
		},
		StoreFactory: func(string) port.PreferencesStore {
			return store
		},
		Quality: quality,
		Tools:   &runtimeTestTools{},
		GitHubAuthFactory: func(time.Duration) github.AuthProvider {
			return &bootstrapAuthProvider{}
		},
		GitHubCredentialBrokerURL: func() string {
			return "https://broker.example"
		},
		GitHubWorkloadIdentity: func() string {
			return "workload-identity"
		},
	}, githubOptions)
	if !githubApplication.services().tickets.HasPullRequestPublisher() {
		t.Fatal("GitHub provider selection did not construct a pull-request publisher")
	}
}

func TestRepositoryOverwritesRemoteAndPropagatesCancellation(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		git := &runtimeTestGit{
			identity: port.RepositoryIdentity{Root: "C:/repository", Remote: "upstream"},
		}
		options := runtimeTestOptions()
		options.repository = "C:/selected"
		options.remote = "origin"
		application := newApplication(Runtime{
			GitFactory: func(time.Duration) port.GitRepository {
				return git
			},
		}, options)
		ctx := context.WithValue(context.Background(), runtimeTestContextKey{}, "request")

		identity, err := application.repository(ctx)
		if err != nil {
			t.Fatalf("repository() error = %v", err)
		}
		if identity.Root != "C:/repository" || identity.Remote != "origin" {
			t.Fatalf("repository() identity = %#v", identity)
		}
		if git.receivedContext != ctx || git.directory != options.repository {
			t.Fatalf("Discover received context %v and directory %q", git.receivedContext, git.directory)
		}
	})

	t.Run("cancelled discovery", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		git := &runtimeTestGit{discoverErr: ctx.Err()}
		application := newApplication(Runtime{
			GitFactory: func(time.Duration) port.GitRepository {
				return git
			},
		}, runtimeTestOptions())

		identity, err := application.repository(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("repository() error = %v, want context cancellation", err)
		}
		if identity != (port.RepositoryIdentity{}) {
			t.Fatalf("repository() identity on error = %#v", identity)
		}
		if git.receivedContext != ctx {
			t.Fatal("repository() did not forward the cancelled context")
		}
	})
}

func TestReporterPreservesOutputContracts(t *testing.T) {
	t.Run("human output is ordered and uncolored when disabled", func(t *testing.T) {
		options := runtimeTestOptions()
		options.color = "never"
		application := newApplication(Runtime{
			OutputIsTerminal: func() bool { return true },
		}, options)
		output := &bytes.Buffer{}

		if err := application.reporter(output).Report(context.Background(), port.Report{
			Summary: "Branch created.",
			Fields: map[string]string{
				"branch": "feature/ABC-123-add-export",
				"base":   "origin/develop",
			},
		}); err != nil {
			t.Fatalf("human reporter error = %v", err)
		}
		const expected = "Branch created.\nbase: origin/develop\nbranch: feature/ABC-123-add-export\n"
		if output.String() != expected {
			t.Fatalf("human reporter output = %q, want %q", output.String(), expected)
		}
	})

	t.Run("json remains machine readable without color", func(t *testing.T) {
		options := runtimeTestOptions()
		options.output = "json"
		options.color = "always"
		application := newApplication(Runtime{
			OutputIsTerminal: func() bool { return true },
		}, options)
		output := &bytes.Buffer{}

		if err := application.reporter(output).Report(context.Background(), port.Report{
			Operation: "branch.create",
			Summary:   "created",
		}); err != nil {
			t.Fatalf("JSON reporter error = %v", err)
		}
		if strings.Contains(output.String(), "\x1b[") {
			t.Fatalf("JSON reporter emitted color: %q", output.String())
		}
		var result map[string]any
		if err := json.Unmarshal(output.Bytes(), &result); err != nil {
			t.Fatalf("JSON reporter output is invalid: %v", err)
		}
		if result["schemaVersion"] != float64(1) || result["ok"] != true ||
			result["operation"] != "branch.create" || result["summary"] != "created" {
			t.Fatalf("JSON reporter result = %#v", result)
		}
	})

	t.Run("quiet suppresses successful human output", func(t *testing.T) {
		options := runtimeTestOptions()
		options.quiet = true
		application := newApplication(Runtime{
			OutputIsTerminal: func() bool { return false },
		}, options)
		output := &bytes.Buffer{}
		if err := application.reporter(output).Report(context.Background(), port.Report{Summary: "hidden"}); err != nil {
			t.Fatalf("quiet reporter error = %v", err)
		}
		if output.Len() != 0 {
			t.Fatalf("quiet reporter output = %q", output.String())
		}
	})

	t.Run("nil writer constructs a reporter without writing", func(t *testing.T) {
		application := newApplication(Runtime{
			OutputIsTerminal: func() bool { return false },
		}, runtimeTestOptions())
		if application.reporter(nil) == nil {
			t.Fatal("reporter(nil) returned nil")
		}
	})
}

func TestOutputFormatAcceptsOnlyStableValues(t *testing.T) {
	testCases := []struct {
		name   string
		output string
		want   string
	}{
		{name: "default", output: "", want: "human"},
		{name: "human", output: "human", want: "human"},
		{name: "json", output: "json", want: "json"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			options := runtimeTestOptions()
			options.output = testCase.output
			format, err := newApplication(Runtime{}, options).outputFormat()
			if err != nil || string(format) != testCase.want {
				t.Fatalf("outputFormat() = (%q, %v), want (%q, nil)", format, err, testCase.want)
			}
		})
	}

	options := runtimeTestOptions()
	options.output = "yaml"
	_, err := newApplication(Runtime{}, options).outputFormat()
	runtimeTestProblemCode(t, err, problem.CodeInvalidInput)
	value, _ := problem.As(err)
	if value.Field != "output" || value.Actual != "yaml" || value.Expected != "human or json" {
		t.Fatalf("invalid output problem = %#v", value)
	}
}

func TestPromptAvailabilityHonorsInteractionAndTTYRules(t *testing.T) {
	testCases := []struct {
		name        string
		interactive string
		output      string
		inputTTY    bool
		outputTTY   bool
		want        bool
	}{
		{name: "json never prompts", interactive: "auto", output: "json", inputTTY: true, outputTTY: true, want: false},
		{name: "never never prompts", interactive: "never", output: "human", inputTTY: true, outputTTY: true, want: false},
		{name: "always requires both terminals", interactive: "always", output: "human", inputTTY: true, outputTTY: true, want: true},
		{name: "always rejects a missing input terminal", interactive: "always", output: "human", inputTTY: false, outputTTY: true, want: false},
		{name: "auto permits both terminals", interactive: "auto", output: "human", inputTTY: true, outputTTY: true, want: true},
		{name: "auto rejects a missing output terminal", interactive: "auto", output: "human", inputTTY: true, outputTTY: false, want: false},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			options := runtimeTestOptions()
			options.interactive = testCase.interactive
			options.output = testCase.output
			application := newApplication(Runtime{
				InputIsTerminal:  func() bool { return testCase.inputTTY },
				OutputIsTerminal: func() bool { return testCase.outputTTY },
			}, options)
			if got := application.promptAvailable(); got != testCase.want {
				t.Fatalf("promptAvailable() = %t, want %t", got, testCase.want)
			}
		})
	}

	withoutSeams := &application{}
	if withoutSeams.inputIsTerminal() || withoutSeams.outputIsTerminal() {
		t.Fatal("nil terminal seams must be treated as unavailable")
	}
}

func TestPromptConstructionAndRequiredInputContracts(t *testing.T) {
	t.Run("uses the injected prompt with requested presentation settings", func(t *testing.T) {
		prompt := &runtimeTestPrompt{inputValue: "ABC"}
		var factoryCalls int
		var accessible bool
		var color string
		options := runtimeTestOptions()
		options.accessible = true
		options.color = "never"
		application := newApplication(Runtime{
			PromptFactory: func(gotAccessible bool, gotColor string) port.Prompt {
				factoryCalls++
				accessible = gotAccessible
				color = gotColor
				return prompt
			},
			InputIsTerminal:  func() bool { return true },
			OutputIsTerminal: func() bool { return true },
		}, options)
		ctx := context.WithValue(context.Background(), runtimeTestContextKey{}, "input")

		value, err := application.requireInput(ctx, "", "Ticket key", "The work item key.")
		if err != nil || value != "ABC" {
			t.Fatalf("requireInput() = (%q, %v)", value, err)
		}
		if factoryCalls != 1 || !accessible || color != "never" {
			t.Fatalf("prompt factory calls = %d, accessible = %t, color = %q", factoryCalls, accessible, color)
		}
		if len(prompt.inputRequests) != 1 {
			t.Fatalf("prompt input request = %#v", prompt.inputRequests)
		}
		request := prompt.inputRequests[0]
		if request.Label != "Ticket key" ||
			request.Description != "The work item key." ||
			!request.Required ||
			request.Validate != nil ||
			request.Sensitive {
			t.Fatalf("prompt input request = %#v", request)
		}
		if prompt.inputContexts[0] != ctx {
			t.Fatal("required input did not forward its context")
		}
	})

	t.Run("uses supplied input without prompting", func(t *testing.T) {
		application := newApplication(Runtime{
			PromptFactory: func(bool, string) port.Prompt {
				t.Fatal("prompt factory was called for supplied input")
				return nil
			},
		}, runtimeTestOptions())
		value, err := application.requireInput(context.Background(), "provided", "Ticket key", "ignored")
		if err != nil || value != "provided" {
			t.Fatalf("requireInput() = (%q, %v)", value, err)
		}
	})

	t.Run("returns typed missing input without an interactive terminal", func(t *testing.T) {
		application := newApplication(Runtime{
			InputIsTerminal:  func() bool { return false },
			OutputIsTerminal: func() bool { return true },
		}, runtimeTestOptions())
		_, err := application.requireInput(context.Background(), "", "Ticket key", "The work item key.")
		runtimeTestProblemCode(t, err, problem.CodeInvalidInput)
		value, _ := problem.As(err)
		if value.Field != "Ticket key" {
			t.Fatalf("missing input field = %q", value.Field)
		}
	})

	t.Run("propagates a cancelled prompt", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		prompt := &runtimeTestPrompt{inputErr: ctx.Err()}
		application := newApplication(Runtime{
			PromptFactory:    func(bool, string) port.Prompt { return prompt },
			InputIsTerminal:  func() bool { return true },
			OutputIsTerminal: func() bool { return true },
		}, runtimeTestOptions())

		_, err := application.requireInput(ctx, "", "Ticket key", "The work item key.")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("requireInput() error = %v, want context cancellation", err)
		}
		if len(prompt.inputContexts) != 1 || prompt.inputContexts[0] != ctx {
			t.Fatal("required input did not forward the cancelled context")
		}
	})
}

func TestOptionalConfirmationHonorsYesTerminalAndCancellationRules(t *testing.T) {
	t.Run("yes bypasses prompting", func(t *testing.T) {
		prompt := &runtimeTestPrompt{}
		options := runtimeTestOptions()
		options.yes = true
		application := newApplication(Runtime{
			PromptFactory:    func(bool, string) port.Prompt { return prompt },
			InputIsTerminal:  func() bool { return true },
			OutputIsTerminal: func() bool { return true },
		}, options)
		confirmed, err := application.optionalConfirmation(context.Background(), "Publish", "Push changes.", false)
		if err != nil || !confirmed {
			t.Fatalf("optionalConfirmation() = (%t, %v)", confirmed, err)
		}
		if len(prompt.confirmRequests) != 0 {
			t.Fatal("optionalConfirmation prompted despite --yes")
		}
	})

	t.Run("returns its default when prompting is unavailable", func(t *testing.T) {
		application := newApplication(Runtime{
			InputIsTerminal:  func() bool { return false },
			OutputIsTerminal: func() bool { return true },
		}, runtimeTestOptions())
		confirmed, err := application.optionalConfirmation(context.Background(), "Publish", "Push changes.", true)
		if err != nil || !confirmed {
			t.Fatalf("optionalConfirmation() = (%t, %v)", confirmed, err)
		}
	})

	t.Run("returns the prompt response", func(t *testing.T) {
		prompt := &runtimeTestPrompt{confirmValue: false}
		application := newApplication(Runtime{
			PromptFactory:    func(bool, string) port.Prompt { return prompt },
			InputIsTerminal:  func() bool { return true },
			OutputIsTerminal: func() bool { return true },
		}, runtimeTestOptions())
		confirmed, err := application.optionalConfirmation(context.Background(), "Publish", "Push changes.", true)
		if err != nil || confirmed {
			t.Fatalf("optionalConfirmation() = (%t, %v), want (false, nil)", confirmed, err)
		}
		if len(prompt.confirmRequests) != 1 || prompt.confirmRequests[0] != (port.ConfirmRequest{
			Label:       "Publish",
			Description: "Push changes.",
			Default:     true,
		}) {
			t.Fatalf("confirmation request = %#v", prompt.confirmRequests)
		}
	})

	t.Run("forwards request and propagates cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		prompt := &runtimeTestPrompt{confirmErr: ctx.Err()}
		application := newApplication(Runtime{
			PromptFactory:    func(bool, string) port.Prompt { return prompt },
			InputIsTerminal:  func() bool { return true },
			OutputIsTerminal: func() bool { return true },
		}, runtimeTestOptions())
		confirmed, err := application.optionalConfirmation(ctx, "Publish", "Push changes.", false)
		if confirmed || !errors.Is(err, context.Canceled) {
			t.Fatalf("optionalConfirmation() = (%t, %v), want (false, context cancellation)", confirmed, err)
		}
		if len(prompt.confirmRequests) != 1 || prompt.confirmRequests[0] != (port.ConfirmRequest{
			Label:       "Publish",
			Description: "Push changes.",
			Default:     false,
		}) || prompt.confirmContexts[0] != ctx {
			t.Fatalf("confirmation request = %#v, context = %v", prompt.confirmRequests, prompt.confirmContexts)
		}
	})
}

func TestColorEnabledFollowsOutputAndColorModes(t *testing.T) {
	testCases := []struct {
		name      string
		output    string
		color     string
		outputTTY bool
		want      bool
	}{
		{name: "non-human never colors", output: "json", color: "always", outputTTY: true, want: false},
		{name: "always colors human output", output: "human", color: "always", outputTTY: false, want: true},
		{name: "never does not color", output: "human", color: "never", outputTTY: true, want: false},
		{name: "auto colors a terminal", output: "human", color: "auto", outputTTY: true, want: true},
		{name: "auto does not color a pipe", output: "human", color: "auto", outputTTY: false, want: false},
		{name: "unknown mode follows terminal default", output: "human", color: "unexpected", outputTTY: true, want: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			options := runtimeTestOptions()
			options.output = testCase.output
			options.color = testCase.color
			application := newApplication(Runtime{
				OutputIsTerminal: func() bool { return testCase.outputTTY },
			}, options)
			if got := application.colorEnabled(); got != testCase.want {
				t.Fatalf("colorEnabled() = %t, want %t", got, testCase.want)
			}
		})
	}
}

func TestTerminalHelpersRejectPipesAndClosedStreams(t *testing.T) {
	testCases := []struct {
		name          string
		current       func() *os.File
		replace       func(*os.File)
		helper        func() bool
		defaultHelper func(Runtime) func() bool
	}{
		{
			name:          "stdin",
			current:       func() *os.File { return os.Stdin },
			replace:       func(file *os.File) { os.Stdin = file },
			helper:        stdinIsTerminal,
			defaultHelper: func(runtime Runtime) func() bool { return runtime.InputIsTerminal },
		},
		{
			name:          "stdout",
			current:       func() *os.File { return os.Stdout },
			replace:       func(file *os.File) { os.Stdout = file },
			helper:        stdoutIsTerminal,
			defaultHelper: func(runtime Runtime) func() bool { return runtime.OutputIsTerminal },
		},
		{
			name:    "stderr",
			current: func() *os.File { return os.Stderr },
			replace: func(file *os.File) { os.Stderr = file },
			helper:  stderrIsTerminal,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			if runtimeTestTerminalResult(testCase.current, testCase.replace, reader, testCase.helper) {
				reader.Close()
				writer.Close()
				t.Fatal("pipe was incorrectly detected as a terminal")
			}
			if testCase.defaultHelper != nil &&
				runtimeTestTerminalResult(testCase.current, testCase.replace, reader, testCase.defaultHelper(defaultRuntime())) {
				reader.Close()
				writer.Close()
				t.Fatal("default runtime incorrectly detected a pipe as a terminal")
			}
			if err := reader.Close(); err != nil {
				writer.Close()
				t.Fatal(err)
			}
			if runtimeTestTerminalResult(testCase.current, testCase.replace, reader, testCase.helper) {
				writer.Close()
				t.Fatal("closed stream was incorrectly detected as a terminal")
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMissingInputDescribesNonInteractiveRequirement(t *testing.T) {
	err := missingInput("ticket number")
	runtimeTestProblemCode(t, err, problem.CodeInvalidInput)
	value, _ := problem.As(err)
	if value.Field != "ticket number" ||
		value.Expected != "a value supplied by a flag or interactive terminal" ||
		value.Rule != "non-interactive execution requires all mandatory values" ||
		value.Remediation != "supply the required flag or run in an interactive terminal" {
		t.Fatalf("missingInput() problem = %#v", value)
	}
}

func runtimeTestOptions() *appOptions {
	return &appOptions{
		interactive:   "auto",
		output:        "human",
		color:         "auto",
		remote:        "origin",
		repository:    "C:/repository",
		config:        "preferences.yaml",
		qualityConfig: "quality.yaml",
		timeout:       3 * time.Second,
	}
}

func runtimeTestProblemCode(t *testing.T, err error, want problem.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected problem code %q, got nil", want)
	}
	value, ok := problem.As(err)
	if !ok {
		t.Fatalf("error %T is not a problem: %v", err, err)
	}
	if value.Code != want {
		t.Fatalf("problem code = %q, want %q", value.Code, want)
	}
}

func runtimeTestTerminalResult(
	current func() *os.File,
	replace func(*os.File),
	candidate *os.File,
	helper func() bool,
) bool {
	original := current()
	replace(candidate)
	defer replace(original)
	return helper()
}

type runtimeTestContextKey struct{}

type runtimeTestGit struct {
	port.GitRepository
	identity        port.RepositoryIdentity
	discoverErr     error
	receivedContext context.Context
	directory       string
}

func (git *runtimeTestGit) Discover(ctx context.Context, directory string) (port.RepositoryIdentity, error) {
	git.receivedContext = ctx
	git.directory = directory
	if git.discoverErr != nil {
		return port.RepositoryIdentity{}, git.discoverErr
	}
	return git.identity, nil
}

type runtimeTestStore struct {
	port.PreferencesStore
}

type runtimeTestPolicy struct {
	port.KeyPolicy
}

type runtimeTestQuality struct {
	port.QualityRunner
}

type runtimeTestTools struct {
	port.ToolInspector
}

type runtimeTestPrompt struct {
	inputValue      string
	inputErr        error
	inputRequests   []port.InputRequest
	inputContexts   []context.Context
	confirmValue    bool
	confirmErr      error
	confirmRequests []port.ConfirmRequest
	confirmContexts []context.Context
}

func (prompt *runtimeTestPrompt) Input(ctx context.Context, request port.InputRequest) (string, error) {
	prompt.inputContexts = append(prompt.inputContexts, ctx)
	prompt.inputRequests = append(prompt.inputRequests, request)
	return prompt.inputValue, prompt.inputErr
}

func (*runtimeTestPrompt) Select(context.Context, port.SelectRequest) (string, error) {
	return "", errors.New("unexpected select prompt")
}

func (prompt *runtimeTestPrompt) Confirm(ctx context.Context, request port.ConfirmRequest) (bool, error) {
	prompt.confirmContexts = append(prompt.confirmContexts, ctx)
	prompt.confirmRequests = append(prompt.confirmRequests, request)
	return prompt.confirmValue, prompt.confirmErr
}

var _ port.GitRepository = (*runtimeTestGit)(nil)
var _ port.PreferencesStore = (*runtimeTestStore)(nil)
var _ port.KeyPolicy = (*runtimeTestPolicy)(nil)
var _ port.QualityRunner = (*runtimeTestQuality)(nil)
var _ port.ToolInspector = (*runtimeTestTools)(nil)
var _ port.Prompt = (*runtimeTestPrompt)(nil)
