package bootstrap

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/t33n-software/git-governance/internal/adapters/github"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

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
				"The public client ID of the GitHub App used for the device-flow login. It is stored with the protected session and never required again for this host.",
			)
			if err != nil {
				return err
			}
			result, err := services.githubAuth.Login(command.Context(), github.LoginRequest{
				ClientID: clientID,
				OnDeviceAuthorization: func(device github.DeviceAuthorization) error {
					writeDeviceAuthorizationInstructions(command, device)
					if application.runtime.Browser != nil {
						_ = application.runtime.Browser.Open(command.Context(), device.VerificationURI)
					}
					return nil
				},
			})
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
			result, err := services.githubAuth.Status(command.Context())
			if err != nil {
				return err
			}
			return application.report(command, port.Report{
				Operation: "auth.status.github",
				Summary:   "GitHub App session status:",
				Fields:    githubSessionFields(result),
				Data:      result,
			})
		},
	}
}

func newGitHubLogoutCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "github",
		Short: "Remove the local GitHub App refresh session",
		RunE: func(command *cobra.Command, _ []string) error {
			services := application.services()
			if services.githubAuth == nil {
				return githubAuthenticationUnavailable()
			}
			result, err := services.githubAuth.Logout(command.Context())
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
	return map[string]string{
		"host":                  status.Host,
		"account":               status.Account,
		"source":                status.Source,
		"refreshState":          status.RefreshState,
		"refreshTokenExpiresAt": status.RefreshTokenExpiresAt.UTC().Format(time.RFC3339),
		"accessToken":           "not persisted; resolved on demand",
	}
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
