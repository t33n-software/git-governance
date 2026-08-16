package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/t33n-software/git-governance/internal/adapters/browser"
	"github.com/t33n-software/git-governance/internal/adapters/github"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestGitHubAuthCommandsUseExplicitInteractiveDeviceFlow(t *testing.T) {
	expiresAt := time.Date(2026, time.December, 31, 12, 0, 0, 0, time.UTC)
	status := github.SessionStatus{
		Host:                  "github.com",
		Account:               "octocat",
		Source:                "native-secret-store",
		RefreshTokenExpiresAt: expiresAt,
		RefreshState:          "active",
	}
	provider := &bootstrapAuthProvider{loginStatus: status, statusValue: status, logoutStatus: status}
	opener := &bootstrapBrowserOpener{}
	prompt := &runtimeTestPrompt{inputValue: "public-client-id"}
	application := newAuthCommandApplication(provider, opener, prompt)
	command := newAuthCommand(application)
	stdout, stderr, err := executeAuthCommand(t, command, context.Background(), "login", "github")
	if err != nil {
		t.Fatalf("auth login error = %v", err)
	}
	if stderr != "" || provider.loginCalls != 1 || provider.loginClientID != "public-client-id" {
		t.Fatalf("login output=%q stderr=%q provider=%#v", stdout, stderr, provider)
	}
	if len(prompt.inputRequests) != 1 || prompt.inputRequests[0].Label != "GitHub App client ID" ||
		!prompt.inputRequests[0].Required {
		t.Fatalf("login did not prompt once for the client ID: %#v", prompt.inputRequests)
	}
	if len(opener.urls) != 1 || opener.urls[0] != "https://github.com/login/device" {
		t.Fatalf("opened browser URLs = %#v", opener.urls)
	}
	for _, expected := range []string{
		"Verification URL: https://github.com/login/device",
		"User code: CODE-CODE",
		"GitHub App login completed.",
		"account: octocat",
		"accessToken: not persisted; resolved on demand",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("login output missing %q: %q", expected, stdout)
		}
	}
	if strings.Contains(stdout, "ghu-") || strings.Contains(stdout, "ghr-") {
		t.Fatalf("login output leaked a token: %q", stdout)
	}
}

func TestGitHubAuthStatusAndLogoutContracts(t *testing.T) {
	expiresAt := time.Date(2026, time.December, 31, 12, 0, 0, 0, time.UTC)
	status := github.SessionStatus{
		Host:                  "github.com",
		Account:               "octocat",
		Source:                "native-secret-store",
		RefreshTokenExpiresAt: expiresAt,
		RefreshState:          "active",
	}
	provider := &bootstrapAuthProvider{statusValue: status, logoutStatus: status}
	application := newAuthCommandApplication(provider, &bootstrapBrowserOpener{}, &runtimeTestPrompt{})

	statusCommand := newAuthCommand(application)
	application.options.output = "json"
	statusOutput, _, err := executeAuthCommand(t, statusCommand, context.Background(), "status", "github")
	if err != nil {
		t.Fatalf("auth status error = %v", err)
	}
	result := map[string]any{}
	if err := json.Unmarshal([]byte(statusOutput), &result); err != nil {
		t.Fatalf("status JSON = %q: %v", statusOutput, err)
	}
	if result["operation"] != "auth.status.github" || result["ok"] != true ||
		strings.Contains(statusOutput, "ghu-") || strings.Contains(statusOutput, "ghr-") {
		t.Fatalf("status result = %#v", result)
	}
	if provider.statusCalls != 1 {
		t.Fatalf("status calls = %d", provider.statusCalls)
	}

	application.options.output = "human"
	logoutCommand := newAuthCommand(application)
	logoutOutput, _, err := executeAuthCommand(t, logoutCommand, context.Background(), "logout", "github")
	if err != nil {
		t.Fatalf("auth logout error = %v", err)
	}
	if provider.logoutCalls != 1 || !strings.Contains(logoutOutput, "remoteRevocation: not supported by the local Device Flow client") {
		t.Fatalf("logout output=%q provider=%#v", logoutOutput, provider)
	}
}

func TestGitHubAuthCommandsRejectUnsafeExecutionAndProviderFailures(t *testing.T) {
	t.Run("requires a human interactive terminal for login", func(t *testing.T) {
		provider := &bootstrapAuthProvider{}
		application := newAuthCommandApplication(provider, &bootstrapBrowserOpener{}, &runtimeTestPrompt{})
		for _, configure := range []func(){
			func() { application.options.interactive = "never" },
			func() { application.options.output = "json" },
			func() { application.runtime.InputIsTerminal = func() bool { return false } },
			func() { application.runtime.OutputIsTerminal = func() bool { return false } },
		} {
			application.options.interactive = "auto"
			application.options.output = "human"
			application.runtime.InputIsTerminal = func() bool { return true }
			application.runtime.OutputIsTerminal = func() bool { return true }
			configure()
			_, _, err := executeAuthCommand(t, newAuthCommand(application), context.Background(), "login", "github")
			assertBootstrapAuthProblem(t, err, problem.CodeInvalidInput)
		}
		if provider.loginCalls != 0 {
			t.Fatalf("non-interactive login called provider %d times", provider.loginCalls)
		}
	})

	t.Run("surfaces provider errors and unavailable composition", func(t *testing.T) {
		expected := errors.New("session unavailable")
		provider := &bootstrapAuthProvider{loginErr: expected, statusErr: expected, logoutErr: expected}
		application := newAuthCommandApplication(provider, &bootstrapBrowserOpener{}, &runtimeTestPrompt{inputValue: "client"})
		for _, args := range [][]string{{"login", "github"}, {"status", "github"}, {"logout", "github"}} {
			_, _, err := executeAuthCommand(t, newAuthCommand(application), context.Background(), args...)
			if !errors.Is(err, expected) {
				t.Fatalf("auth %v error = %v, want %v", args, err, expected)
			}
		}
		application.runtime.GitHubAuthFactory = func(time.Duration) github.AuthProvider { return nil }
		_, _, err := executeAuthCommand(t, newAuthCommand(application), context.Background(), "login", "github")
		assertBootstrapAuthProblem(t, err, problem.CodeConfigurationUnavailable)
		_, _, err = executeAuthCommand(t, newAuthCommand(application), context.Background(), "status", "github")
		assertBootstrapAuthProblem(t, err, problem.CodeConfigurationUnavailable)
		_, _, err = executeAuthCommand(t, newAuthCommand(application), context.Background(), "logout", "github")
		assertBootstrapAuthProblem(t, err, problem.CodeConfigurationUnavailable)
	})

	t.Run("surfaces a client ID prompt failure before the device flow", func(t *testing.T) {
		expected := errors.New("input unavailable")
		provider := &bootstrapAuthProvider{}
		application := newAuthCommandApplication(provider, &bootstrapBrowserOpener{}, &runtimeTestPrompt{inputErr: expected})
		_, _, err := executeAuthCommand(t, newAuthCommand(application), context.Background(), "login", "github")
		if !errors.Is(err, expected) {
			t.Fatalf("login prompt error = %v, want %v", err, expected)
		}
		if provider.loginCalls != 0 {
			t.Fatalf("prompt failure called provider %d times", provider.loginCalls)
		}
	})

	t.Run("browser opening failure does not block manual verification", func(t *testing.T) {
		opener := &bootstrapBrowserOpener{err: errors.New("browser unavailable")}
		provider := &bootstrapAuthProvider{loginStatus: github.SessionStatus{Host: "github.com", Account: "octocat"}}
		application := newAuthCommandApplication(provider, opener, &runtimeTestPrompt{inputValue: "client"})
		output, _, err := executeAuthCommand(t, newAuthCommand(application), context.Background(), "login", "github")
		if err != nil || !strings.Contains(output, "Verification URL:") || len(opener.urls) != 1 {
			t.Fatalf("manual login fallback = (%q, %v), urls=%#v", output, err, opener.urls)
		}
		application.runtime.Browser = nil
		output, _, err = executeAuthCommand(t, newAuthCommand(application), context.Background(), "login", "github")
		if err != nil || !strings.Contains(output, "Verification URL:") || len(opener.urls) != 1 {
			t.Fatalf("missing browser fallback = (%q, %v), urls=%#v", output, err, opener.urls)
		}
	})
}

func TestAuthCommandHelpersAreRedactedAndStable(t *testing.T) {
	status := github.SessionStatus{
		Host:                  "github.com",
		Account:               "octocat",
		Source:                "native-secret-store",
		RefreshTokenExpiresAt: time.Date(2026, time.December, 31, 12, 0, 0, 0, time.UTC),
		RefreshState:          "active",
	}
	fields := githubSessionFields(status)
	if fields["host"] != "github.com" || fields["account"] != "octocat" ||
		fields["accessToken"] != "not persisted; resolved on demand" ||
		strings.Contains(strings.Join(mapValues(fields), ","), "ghr-") {
		t.Fatalf("session fields = %#v", fields)
	}
	output := &bytes.Buffer{}
	command := newAuthCommand(newAuthCommandApplication(&bootstrapAuthProvider{}, &bootstrapBrowserOpener{}, &runtimeTestPrompt{}))
	command.SetOut(output)
	writeDeviceAuthorizationInstructions(command, github.DeviceAuthorization{
		VerificationURI: "https://github.com/login/device",
		UserCode:        "CODE-CODE",
	})
	if !strings.Contains(output.String(), "CODE-CODE") {
		t.Fatalf("device instructions = %q", output.String())
	}
	assertBootstrapAuthProblem(t, githubAuthenticationUnavailable(), problem.CodeConfigurationUnavailable)
}

func newAuthCommandApplication(
	provider github.AuthProvider,
	opener browser.Opener,
	prompt port.Prompt,
) *application {
	return newApplication(Runtime{
		GitFactory: func(time.Duration) port.GitRepository {
			return newCommandGitForAuth()
		},
		StoreFactory: func(string) port.PreferencesStore {
			return &commandStore{}
		},
		GitHubAuthFactory: func(time.Duration) github.AuthProvider {
			return provider
		},
		Browser: opener,
		PromptFactory: func(bool, string) port.Prompt {
			return prompt
		},
		InputIsTerminal:  func() bool { return true },
		OutputIsTerminal: func() bool { return true },
	}, &appOptions{
		interactive: "auto",
		output:      "human",
		color:       "never",
		remote:      "origin",
		repository:  "C:/repo",
		timeout:     time.Second,
	})
}

func newCommandGitForAuth() port.GitRepository {
	return &commandGit{}
}

func executeAuthCommand(
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

func assertBootstrapAuthProblem(t *testing.T, err error, code problem.Code) {
	t.Helper()
	value, ok := problem.As(err)
	if !ok || value.Code != code {
		t.Fatalf("problem = %#v, want code %q", err, code)
	}
}

func mapValues(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

type bootstrapAuthProvider struct {
	loginStatus   github.SessionStatus
	statusValue   github.SessionStatus
	logoutStatus  github.SessionStatus
	loginErr      error
	statusErr     error
	logoutErr     error
	resolveErr    error
	loginCalls    int
	statusCalls   int
	logoutCalls   int
	loginClientID string
}

func (provider *bootstrapAuthProvider) Resolve(context.Context, github.CredentialTarget) (string, error) {
	if provider.resolveErr != nil {
		return "", provider.resolveErr
	}
	return "ghu-test-access-token", nil
}

func (provider *bootstrapAuthProvider) Login(_ context.Context, request github.LoginRequest) (github.SessionStatus, error) {
	provider.loginCalls++
	provider.loginClientID = request.ClientID
	if provider.loginErr != nil {
		return github.SessionStatus{}, provider.loginErr
	}
	if request.OnDeviceAuthorization != nil {
		if err := request.OnDeviceAuthorization(github.DeviceAuthorization{
			VerificationURI: "https://github.com/login/device",
			UserCode:        "CODE-CODE",
		}); err != nil {
			return github.SessionStatus{}, err
		}
	}
	return provider.loginStatus, nil
}

func (provider *bootstrapAuthProvider) Status(context.Context) (github.SessionStatus, error) {
	provider.statusCalls++
	return provider.statusValue, provider.statusErr
}

func (provider *bootstrapAuthProvider) Logout(context.Context) (github.SessionStatus, error) {
	provider.logoutCalls++
	return provider.logoutStatus, provider.logoutErr
}

type bootstrapBrowserOpener struct {
	urls []string
	err  error
}

func (opener *bootstrapBrowserOpener) Open(_ context.Context, rawURL string) error {
	opener.urls = append(opener.urls, rawURL)
	return opener.err
}

var _ github.AuthProvider = (*bootstrapAuthProvider)(nil)
var _ browser.Opener = (*bootstrapBrowserOpener)(nil)
