package bootstrap

import (
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/policy"
	"github.com/t33n-software/git-governance/internal/application/port"
)

func newValidateCommand(application *application) *cobra.Command {
	command := &cobra.Command{
		Use:   "validate",
		Short: "Run local governance validation",
	}
	command.AddCommand(newPrePushCommand(application))
	return command
}

func newPrePushCommand(application *application) *cobra.Command {
	var (
		branchRaw string
		baseRaw   string
	)
	command := &cobra.Command{
		Use:   "pre-push",
		Short: "Validate every outgoing branch update before a push",
		RunE: func(command *cobra.Command, _ []string) error {
			services := application.services()
			repository, err := application.discover(command.Context(), services)
			if err != nil {
				return err
			}
			base, err := parseBase(baseRaw, repository.Remote)
			if err != nil {
				return err
			}
			updates, err := readPrePushUpdates(command)
			if err != nil {
				return err
			}
			if len(updates) > 0 {
				if branchRaw != "" {
					return invalidOption("branch", branchRaw, "--branch is only supported when no Git pre-push updates are supplied")
				}
				result, err := services.sync.ValidatePrePushUpdates(command.Context(), repository, updates, base)
				if err != nil {
					return err
				}
				return application.report(command, port.Report{
					Operation: "validate.pre-push",
					Summary: application.withInteractiveFetchSummary(
						"Pre-push validation passed for every supplied update.",
						repository.Remote,
						true,
					),
					Fields: map[string]string{
						"updates":       strconv.Itoa(len(result.Updates)),
						"qualityStatus": string(result.Quality.Status),
						"qualityDetail": result.Quality.Detail,
					},
					Data: result,
				})
			}

			name, err := currentOrSpecified(command.Context(), services, branchRaw, repository)
			if err != nil {
				return err
			}
			result, err := services.sync.ValidatePrePush(command.Context(), branchapp.PrePushRequest{
				Repository: repository,
				Name:       name,
				Base:       base,
			})
			if err != nil {
				return err
			}
			fields := map[string]string{
				"branch":      result.Name.String(),
				"publication": string(result.Publication),
			}
			if result.Base != nil {
				fields["base"] = result.Base.String()
				fields["missingBaseCommits"] = boolString(result.MissingBaseCommits)
			}
			fields["qualityStatus"] = string(result.Quality.Status)
			fields["qualityDetail"] = result.Quality.Detail
			return application.report(command, port.Report{
				Operation: "validate.pre-push",
				Summary: application.withInteractiveFetchSummary(
					"Pre-push validation passed.",
					repository.Remote,
					result.Base != nil,
				),
				Fields: fields,
			})
		},
	}
	command.Flags().StringVar(&branchRaw, "branch", "", "branch name; defaults to the current branch")
	command.Flags().StringVar(&baseRaw, "base", "", "explicit target base")
	return command
}

func readPrePushUpdates(command *cobra.Command) ([]branchapp.PushUpdate, error) {
	reader := command.InOrStdin()
	if reader == os.Stdin && stdinIsTerminal() {
		return nil, nil
	}
	return branchapp.ParsePrePushUpdates(reader)
}

func newConfigCommand(application *application) *cobra.Command {
	command := &cobra.Command{
		Use:   "config",
		Short: "Manage local user preferences",
	}
	keyCommand := &cobra.Command{
		Use:   "key",
		Short: "Manage known ticket keys",
	}
	keyCommand.AddCommand(
		newConfigKeyListCommand(application),
		newConfigKeyAddCommand(application),
		newConfigKeyRemoveCommand(application),
		newConfigKeyDefaultCommand(application),
	)
	command.AddCommand(keyCommand)
	return command
}

func newConfigKeyListCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List known local ticket keys",
		RunE: func(command *cobra.Command, _ []string) error {
			preferences, err := application.services().preferences.List(command.Context())
			if err != nil {
				return err
			}
			keys := make([]string, 0, len(preferences.KnownKeys))
			for _, key := range preferences.KnownKeys {
				keys = append(keys, key.String())
			}
			fields := map[string]string{
				"keys": strings.Join(keys, ","),
			}
			if preferences.DefaultKey != nil {
				fields["defaultKey"] = preferences.DefaultKey.String()
			}
			return application.report(command, port.Report{
				Operation: "config.key.list",
				Summary:   "Known ticket keys:",
				Fields:    fields,
				Data:      preferences,
			})
		},
	}
}

func newConfigKeyAddCommand(application *application) *cobra.Command {
	var keyRaw string
	command := &cobra.Command{
		Use:   "add",
		Short: "Add a known ticket key",
		RunE: func(command *cobra.Command, _ []string) error {
			services := application.services()
			key, err := application.resolveKey(command.Context(), services, keyRaw)
			if err != nil {
				return err
			}
			preferences, err := services.preferences.AddKey(command.Context(), key)
			if err != nil {
				return err
			}
			return application.report(command, port.Report{
				Operation: "config.key.add",
				Summary:   "Ticket key saved.",
				Fields: map[string]string{
					"key":           key.String(),
					"knownKeyCount": itoa(len(preferences.KnownKeys)),
				},
			})
		},
	}
	command.Flags().StringVar(&keyRaw, "key", "", "ticket key")
	return command
}

func newConfigKeyRemoveCommand(application *application) *cobra.Command {
	var keyRaw string
	command := &cobra.Command{
		Use:   "remove",
		Short: "Remove a known ticket key",
		RunE: func(command *cobra.Command, _ []string) error {
			services := application.services()
			key, err := application.resolveKey(command.Context(), services, keyRaw)
			if err != nil {
				return err
			}
			preferences, err := services.preferences.RemoveKey(command.Context(), key)
			if err != nil {
				return err
			}
			return application.report(command, port.Report{
				Operation: "config.key.remove",
				Summary:   "Ticket key removed.",
				Fields: map[string]string{
					"key":           key.String(),
					"knownKeyCount": itoa(len(preferences.KnownKeys)),
				},
			})
		},
	}
	command.Flags().StringVar(&keyRaw, "key", "", "ticket key")
	return command
}

func newConfigKeyDefaultCommand(application *application) *cobra.Command {
	var keyRaw string
	command := &cobra.Command{
		Use:   "set-default",
		Short: "Set the default ticket key for interactive prompts",
		RunE: func(command *cobra.Command, _ []string) error {
			services := application.services()
			key, err := application.resolveKey(command.Context(), services, keyRaw)
			if err != nil {
				return err
			}
			_, err = services.preferences.SetDefaultKey(command.Context(), key)
			if err != nil {
				return err
			}
			return application.report(command, port.Report{
				Operation: "config.key.set-default",
				Summary:   "Default ticket key saved.",
				Fields:    map[string]string{"defaultKey": key.String()},
			})
		},
	}
	command.Flags().StringVar(&keyRaw, "key", "", "ticket key")
	return command
}

func newPolicyCommand(application *application) *cobra.Command {
	command := &cobra.Command{
		Use:   "policy",
		Short: "Inspect the active local policy",
	}
	command.AddCommand(&cobra.Command{
		Use:   "describe",
		Short: "Describe the complete local policy contract",
		RunE: func(command *cobra.Command, _ []string) error {
			description := policy.Describe()
			fields := map[string]string{
				"schemaVersion": itoa(description.SchemaVersion),
				"keyPolicy":     description.KeyPolicy,
			}
			return application.report(command, port.Report{
				Operation: "policy.describe",
				Summary:   "Active local policy:",
				Fields:    fields,
				Data:      description,
			})
		},
	})
	return command
}

func newDoctorCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Run read-only local diagnostics",
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := application.services().doctor.RunForRemote(
				command.Context(),
				application.options.repository,
				application.options.remote,
			)
			if err != nil {
				return err
			}
			if err := result.AuthenticationError(); err != nil {
				return err
			}
			if err := result.SigningError(); err != nil {
				return err
			}
			fields := make(map[string]string, len(result.Checks))
			for _, check := range result.Checks {
				status := "ok"
				if !check.OK {
					status = "failed"
				}
				fields[check.Name] = status + ": " + check.Detail
			}
			return application.report(command, port.Report{
				Operation: "doctor",
				Summary:   "Local diagnostics completed.",
				Fields:    fields,
				Data:      result,
			})
		},
	}
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
