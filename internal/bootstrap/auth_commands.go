package bootstrap

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/t33n-software/git-governance/internal/adapters/github"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// newAuthCommand exposes explicit login-flow authentication; credentials are
// never accepted as command arguments, and status output carries only
// non-sensitive metadata. Canonical convention:
// docs/conventions/cli/security-and-governance.md.
func newAuthCommand(application *application) *cobra.Command {
	command := &cobra.Command{
		Use:   "auth",
		Short: "Manage explicit hosting-provider authentication sessions",
	}

	login := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with a hosting provider",
	}
	login.AddCommand(newGitHubLoginCommand(application))

	status := &cobra.Command{
		Use:   "status",
		Short: "Show non-sensitive hosting-provider authentication metadata",
	}
	status.AddCommand(newGitHubStatusCommand(application))

	logout := &cobra.Command{
		Use:   "logout",
		Short: "Remove a local hosting-provider authentication session",
	}
	logout.AddCommand(newGitHubLogoutCommand(application))

	command.AddCommand(login, status, logout)
	return command
}

func newGitHubLoginCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "github",
		Short: "Start an explicit GitHub App browser Device Flow login",
		RunE: func(command *cobra.Command, _ []string) error {
			if err := application.requireInteractiveAuthentication(); err != nil {
				return err
			}
			services := application.services()
			if services.githubAuth == nil {
				return githubAuthenticationUnavailable()
			}
			clientID, err := application.requireInput(
				command.Context(),
				"",
				"GitHub App client ID",
				"The public client ID of the GitHub App used for the device-flow login. It is stored with the protected session and bound to the repository selected by the working directory or --repo.",
			)
			if err != nil {
				return err
			}
			request := github.LoginRequest{
				ClientID: clientID,
				OnDeviceAuthorization: func(device github.DeviceAuthorization) error {
					writeDeviceAuthorizationInstructions(command, device)
					if application.runtime.Browser != nil {
						_ = application.runtime.Browser.Open(command.Context(), device.VerificationURI)
					}
					return nil
				},
			}
			if target, ok := application.repositoryCredentialTarget(command.Context(), services.git); ok {
				request.Repository = target
			}
			result, err := services.githubAuth.Login(command.Context(), request)
			if err != nil {
				return err
			}
			return application.report(command, port.Report{
				Operation: "auth.login.github",
				Summary:   "GitHub App login completed.",
				Fields:    githubSessionFields(result),
				Data:      result,
			})
		},
	}
}

func newGitHubStatusCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "github",
		Short: "Show non-sensitive GitHub App session metadata",
		RunE: func(command *cobra.Command, _ []string) error {
			services := application.services()
			if services.githubAuth == nil {
				return githubAuthenticationUnavailable()
			}
			target, _ := application.repositoryCredentialTarget(command.Context(), services.git)
			statuses, err := services.githubAuth.Status(command.Context(), target)
			if err != nil {
				return err
			}
			return application.report(command, port.Report{
				Operation: "auth.status.github",
				Summary:   "GitHub App session status:",
				Fields:    githubSessionListFields(statuses),
				Data:      statuses,
			})
		},
	}
}

func newGitHubLogoutCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "github",
		Short: "Remove a local GitHub App refresh session",
		RunE: func(command *cobra.Command, _ []string) error {
			services := application.services()
			if services.githubAuth == nil {
				return githubAuthenticationUnavailable()
			}
			target, _ := application.repositoryCredentialTarget(command.Context(), services.git)
			request := github.LogoutRequest{Repository: target}
			if application.promptAvailable() {
				request.OnSessionSelection = func(candidates []github.SessionStatus) (github.SessionStatus, error) {
					return selectGitHubSession(command.Context(), application, candidates)
				}
			}
			result, err := services.githubAuth.Logout(command.Context(), request)
			if err != nil {
				return err
			}
			fields := githubSessionFields(result)
			fields["remoteRevocation"] = "not supported by the local Device Flow client"
			return application.report(command, port.Report{
				Operation: "auth.logout.github",
				Summary:   "GitHub App local session removed.",
				Fields:    fields,
				Data:      result,
			})
		},
	}
}

// selectGitHubSession resolves the logout ambiguity interactively over the
// bound sessions of the repository. The offered candidates carry stored
// identity metadata only; no token is resolved and no measurement runs.
func selectGitHubSession(ctx context.Context, application *application, candidates []github.SessionStatus) (github.SessionStatus, error) {
	options := make([]port.SelectOption, 0, len(candidates))
	for _, candidate := range candidates {
		options = append(options, port.SelectOption{
			Value:       candidate.ClientID,
			Label:       candidate.Account + " (" + candidate.ClientID + ")",
			Description: "bound to " + candidate.Repository + "; refresh state " + candidate.RefreshState,
		})
	}
	selected, err := application.prompt().Select(ctx, port.SelectRequest{
		Label:       "Several GitHub App sessions are bound to this repository",
		Description: "Select the session to remove. The remaining sessions keep their bindings.",
		Options:     options,
	})
	if err != nil {
		return github.SessionStatus{}, err
	}
	for _, candidate := range candidates {
		if candidate.ClientID == selected {
			return candidate, nil
		}
	}
	return github.SessionStatus{}, problem.New(problem.Details{
		Code:        problem.CodeInvalidInput,
		Category:    problem.CategoryUsage,
		Field:       "GitHub App session selection",
		Actual:      selected,
		Expected:    "one of the offered bound sessions",
		Rule:        "logout removes exactly the session the selection returned",
		Remediation: "select one of the offered bound sessions",
	})
}

// repositoryCredentialTarget derives the canonical repository identity of the
// selected working context (--repo or the working directory) from its remote.
// The zero target and false are returned when the context has no resolvable
// GitHub remote; callers then use the host-level recency session.
func (application *application) repositoryCredentialTarget(
	ctx context.Context,
	git port.GitRepository,
) (github.CredentialTarget, bool) {
	identity, err := git.Discover(ctx, application.options.repository)
	if err != nil {
		return github.CredentialTarget{}, false
	}
	identity.Remote = application.options.remote
	remoteURL, err := git.RemoteURL(ctx, identity)
	if err != nil {
		return github.CredentialTarget{}, false
	}
	target, err := github.ParseCredentialTarget(remoteURL)
	if err != nil {
		return github.CredentialTarget{}, false
	}
	return target, true
}

func (application *application) requireInteractiveAuthentication() error {
	if application.options.interactive == "never" || application.options.output != "human" ||
		!application.inputIsTerminal() || !application.outputIsTerminal() {
		return problem.New(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryUsage,
			Field:       "GitHub App login",
			Expected:    "an interactive human terminal with --interactive auto or always",
			Rule:        "GitHub App login never starts in non-interactive or JSON execution",
			Remediation: "run auth login github from an interactive terminal with human output",
		})
	}
	return nil
}

func writeDeviceAuthorizationInstructions(command *cobra.Command, device github.DeviceAuthorization) {
	writer := command.OutOrStdout()
	_, _ = fmt.Fprintln(writer, "Open the GitHub verification page in your browser and enter the displayed code.")
	_, _ = fmt.Fprintln(writer, "Verification URL:", device.VerificationURI)
	_, _ = fmt.Fprintln(writer, "User code:", device.UserCode)
}

func githubSessionFields(status github.SessionStatus) map[string]string {
	fields := map[string]string{
		"host":                  status.Host,
		"account":               status.Account,
		"source":                status.Source,
		"refreshState":          status.RefreshState,
		"refreshTokenExpiresAt": status.RefreshTokenExpiresAt.UTC().Format(time.RFC3339),
		"accessToken":           "not persisted; resolved on demand",
	}
	if status.Repository != "" {
		fields["repository"] = status.Repository
	}
	if status.AppSlug != "" {
		fields["appSlug"] = status.AppSlug
	}
	if len(status.Capabilities) > 0 {
		fields["capabilities"] = strings.Join(status.Capabilities, ", ")
	}
	return fields
}

// githubSessionListFields renders one field block per bound session. A
// single-entry list renders unprefixed; several entries carry indexed keys so
// every bound GitHub App class stays individually visible.
func githubSessionListFields(statuses []github.SessionStatus) map[string]string {
	fields := make(map[string]string, len(statuses)*7)
	for index, status := range statuses {
		prefix := ""
		if len(statuses) > 1 {
			prefix = fmt.Sprintf("session[%d].", index+1)
		}
		for key, value := range githubSessionFields(status) {
			fields[prefix+key] = value
		}
	}
	return fields
}

func githubAuthenticationUnavailable() error {
	return problem.New(problem.Details{
		Code:        problem.CodeConfigurationUnavailable,
		Category:    problem.CategoryConfig,
		Field:       "GitHub App authentication",
		Expected:    "a configured GitHub App authentication provider",
		Rule:        "GitHub authentication commands require the platform authentication adapter",
		Remediation: "repair the runtime composition and retry",
	})
}
