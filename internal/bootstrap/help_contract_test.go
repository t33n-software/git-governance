package bootstrap

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestEveryValueCarryingFlagIsRegisterBound walks the production command tree
// and rejects every flag that carries a value domain without a register
// binding. This is the drift gate of the help contract: a new flag without a
// register-bound value domain fails CI. The declared exception sets cover
// boolean switches (their class duty is the effect text with the default),
// the global path and configuration flags, and the pull request description
// flags, which carry no machine-enforced rule set. Canonical convention:
// docs/conventions/cli/testing-and-verification.md.
func TestEveryValueCarryingFlagIsRegisterBound(t *testing.T) {
	t.Parallel()

	booleanSwitches := map[string]bool{
		"accessible": true, "quiet": true, "dry-run": true, "yes": true,
		"switch": true, "breaking": true, "merge-breaking": true, "commit-breaking": true,
		"resume": true, "push": true, "scratch": true, "create-pull-request": true,
		"draft": true, "publish": true, "recovery": true, "dispatch": true, "prepared": true,
	}
	globalPaths := map[string]bool{
		"repo": true, "config": true, "quality-config": true,
	}
	pullRequestDescriptions := map[string]bool{
		"workflow ticket publish --body":                    true,
		"workflow hotfix publish --body":                    true,
		"workflow hotfix propagate --body":                  true,
		"workflow release publish-stabilization --body":     true,
		"workflow release align-promotion-base --body":      true,
		"workflow release promote --body":                   true,
		"workflow release backmerge --body":                 true,
		"workflow release align-reconciliation-base --body": true,
	}

	var failures []string
	check := func(command *cobra.Command, flag *pflag.Flag) {
		if flag.Annotations[valueDomainFlagAnnotation] != nil {
			return
		}
		if flag.Name == "help" || booleanSwitches[flag.Name] {
			return
		}
		key := strings.TrimPrefix(command.CommandPath(), "git-governance ") + " --" + flag.Name
		key = strings.TrimPrefix(key, "git-governance --")
		if pullRequestDescriptions[key] {
			return
		}
		if command.Parent() == nil && globalPaths[flag.Name] {
			return
		}
		failures = append(failures, key)
	}

	var walk func(command *cobra.Command)
	walk = func(command *cobra.Command) {
		command.Flags().VisitAll(func(flag *pflag.Flag) {
			check(command, flag)
		})
		for _, child := range command.Commands() {
			walk(child)
		}
	}

	root := New(BuildInfo{Version: "test"})
	walk(root)
	root.PersistentFlags().VisitAll(func(flag *pflag.Flag) {
		check(root, flag)
	})

	if len(failures) != 0 {
		t.Fatalf("flags without a register-bound value domain: %v", failures)
	}
}
