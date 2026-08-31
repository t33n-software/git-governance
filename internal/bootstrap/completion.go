package bootstrap

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/t33n-software/git-governance/internal/application/cliparam"
)

// newCompletionCommand exposes the shell-completion surface (channel K4 of the
// value-domain single source of truth). Canonical convention:
// docs/conventions/cli/single-source-of-truth.md.
func newCompletionCommand(root *cobra.Command) *cobra.Command {
	shells := cliparam.CompletionShell()
	command := &cobra.Command{
		Use:   "completion [" + strings.Join(shells.Values, "|") + "]",
		Short: "Generate shell completion scripts",
		Long:  "Generate a completion script for Bash, Zsh, Fish, or PowerShell.",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return shells.Complete(toComplete), cobra.ShellCompDirectiveNoFileComp
		},
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
				return fmt.Errorf("unsupported shell %q; expected %s", args[0], shells.ValueList())
			}
		},
	}
	return command
}
