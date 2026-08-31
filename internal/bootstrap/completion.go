package bootstrap

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newCompletionCommand exposes the shell-completion surface (channel K4 of the
// value-domain single source of truth). Canonical convention:
// docs/conventions/cli/single-source-of-truth.md.
func newCompletionCommand(root *cobra.Command) *cobra.Command {
	command := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Long:  "Generate a completion script for Bash, Zsh, Fish, or PowerShell.",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return root.GenBashCompletion(command.OutOrStdout())
			case "zsh":
				return root.GenZshCompletion(command.OutOrStdout())
			case "fish":
				return root.GenFishCompletion(command.OutOrStdout(), true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(command.OutOrStdout())
			default:
				return fmt.Errorf("unsupported shell %q; expected bash, zsh, fish, or powershell", args[0])
			}
		},
	}
	return command
}
