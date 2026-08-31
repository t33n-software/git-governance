package bootstrap

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/t33n-software/git-governance/internal/application/workflow"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

// This file is the help-first consumer test: it simulates a consumer that
// reads only the help surface -- without the error path and without source
// code -- and proves that a valid call is derivable from it. Every value the
// help lists must pass the domain validation, every value the help excludes
// must stay out of the accepted set, and every documented example must
// validate. Canonical convention:
// docs/conventions/cli/testing-and-verification.md.

// TestHelpListedValuesPassDomainValidation extracts the accepted value lists
// from the rendered help texts and feeds them through the domain validators.
func TestHelpListedValuesPassDomainValidation(t *testing.T) {
	t.Parallel()

	root := New(BuildInfo{Version: "test"})

	t.Run("commit family", func(t *testing.T) {
		usage := flagUsageFor(t, root, []string{"commit", "create"}, "type")
		values := helpValueList(t, usage)
		if len(values) != 11 {
			t.Fatalf("help lists %d commit families, want 11: %v", len(values), values)
		}
		for _, value := range values {
			if _, err := commitmsg.ParseType(value); err != nil {
				t.Fatalf("help-listed commit family %q rejected by validation: %v", value, err)
			}
		}
		if _, err := commitmsg.ParseType("feature"); err == nil {
			t.Fatal("the branch-taxonomy value feature is not listed in the help but passes validation")
		}
	})

	t.Run("branch family subset and exclusions cover the taxonomy", func(t *testing.T) {
		usage := flagUsageFor(t, root, []string{"branch", "create"}, "family")
		accepted := helpValueList(t, usage)
		excluded := helpExclusionList(t, usage, "not directly creatable: ")

		seen := map[string]bool{}
		for _, value := range accepted {
			if _, err := branch.ParseFamily(value); err != nil {
				t.Fatalf("help-accepted branch family %q rejected by validation: %v", value, err)
			}
			seen[value] = true
		}
		for _, value := range excluded {
			if seen[value] {
				t.Fatalf("help lists branch family %q as accepted and excluded", value)
			}
			seen[value] = true
		}
		if len(seen) != len(branch.Families()) {
			t.Fatalf("help covers %d branch families, want the full taxonomy of %d", len(seen), len(branch.Families()))
		}
	})

	t.Run("global output mode", func(t *testing.T) {
		flag := root.PersistentFlags().Lookup("output")
		if flag == nil {
			t.Fatal("global flag output not registered")
		}
		for _, value := range helpValueList(t, flag.Usage) {
			application := &application{options: &appOptions{
				interactive: "auto",
				output:      value,
				color:       "auto",
				remote:      "origin",
				timeout:     time.Second,
			}}
			if err := application.validateOptions(); err != nil {
				t.Fatalf("help-listed output mode %q rejected by validation: %v", value, err)
			}
		}
	})
}

// TestHelpDocumentedExamplesValidate extracts the canonical example from the
// rendered help of every rule-carrying flag and proves it passes the domain
// validation -- consumers copy examples verbatim.
func TestHelpDocumentedExamplesValidate(t *testing.T) {
	t.Parallel()

	root := New(BuildInfo{Version: "test"})

	tests := []struct {
		name     string
		path     []string
		flagName string
		validate func(value string) error
	}{
		{name: "ticket key", path: []string{"workflow", "ticket", "start"}, flagName: "key", validate: func(value string) error {
			_, err := ticket.ParseKey(value)
			return err
		}},
		{name: "ticket number", path: []string{"workflow", "ticket", "start"}, flagName: "ticket", validate: func(value string) error {
			_, err := ticket.ParseNumber(value)
			return err
		}},
		{name: "branch slug", path: []string{"workflow", "ticket", "start"}, flagName: "slug", validate: func(value string) error {
			_, err := branch.ParseSlug(value)
			return err
		}},
		{name: "release version", path: []string{"workflow", "release", "cut"}, flagName: "version", validate: func(value string) error {
			_, err := branch.ParseSemanticVersion(value)
			return err
		}},
		{name: "release line", path: []string{"workflow", "release", "stabilize"}, flagName: "release", validate: func(value string) error {
			_, err := branch.ParseName(value)
			return err
		}},
		{name: "commit sha", path: []string{"workflow", "hotfix", "propagate"}, flagName: "commit", validate: func(value string) error {
			return workflow.ValidateCommitID(value)
		}},
		{name: "remote name", path: nil, flagName: "remote", validate: func(value string) error {
			develop, err := branch.ParseName("develop")
			if err != nil {
				return err
			}
			_, err = branch.NewTargetBase(value, develop)
			return err
		}},
		{name: "commit subject", path: []string{"commit", "create"}, flagName: "subject", validate: func(value string) error {
			id, err := ticket.ParseID("ABC-123")
			if err != nil {
				return err
			}
			_, err = commitmsg.NewHeader(commitmsg.TypeFeat, id, value, false)
			return err
		}},
		{name: "footer", path: []string{"commit", "create"}, flagName: "footer", validate: func(value string) error {
			_, err := parseFooterSpec(value)
			return err
		}},
		{name: "breaking description", path: []string{"commit", "create"}, flagName: "breaking-description", validate: func(value string) error {
			_, err := commitmsg.NewFooter("BREAKING CHANGE", value)
			return err
		}},
		{name: "base", path: []string{"commit", "create"}, flagName: "base", validate: func(value string) error {
			_, err := parseBase(value, "origin")
			return err
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var usage string
			if test.path == nil {
				flag := root.PersistentFlags().Lookup(test.flagName)
				if flag == nil {
					t.Fatalf("global flag %q not registered", test.flagName)
				}
				usage = flag.Usage
			} else {
				usage = flagUsageFor(t, root, test.path, test.flagName)
			}
			example := helpExample(t, usage)
			if err := test.validate(example); err != nil {
				t.Fatalf("documented example %q from --%s help fails validation: %v", example, test.flagName, err)
			}
		})
	}
}

// flagUsageFor navigates the production tree to one flag's rendered usage.
// Callers keep the navigation sequential because cobra's Find lazily
// initializes command state.
func flagUsageFor(t *testing.T, root *cobra.Command, path []string, name string) string {
	t.Helper()
	command, _, err := root.Find(path)
	if err != nil || command == nil {
		t.Fatalf("command path %v not found: %v", path, err)
	}
	flag := command.Flags().Lookup(name)
	if flag == nil {
		t.Fatalf("flag %q not registered on %v", name, path)
	}
	return flag.Usage
}

// helpValueList extracts the accepted value list rendered between the concept
// colon and the first section separator, simulating a consumer that reads
// only the help text.
func helpValueList(t *testing.T, usage string) []string {
	t.Helper()
	start := strings.Index(usage, ": ")
	if start < 0 {
		t.Fatalf("help text %q carries no value list", usage)
	}
	rest := usage[start+2:]
	if cut := strings.Index(rest, ";"); cut >= 0 {
		rest = rest[:cut]
	}
	var values []string
	if strings.Contains(rest, ", ") {
		values = strings.Split(rest, ", ")
		values[len(values)-1] = strings.TrimPrefix(values[len(values)-1], "or ")
	} else {
		values = strings.Split(rest, " or ")
	}
	return values
}

// helpExclusionList extracts the values named by an exclusion section of the
// help text, for example the workflow-bound families of a subset flag.
func helpExclusionList(t *testing.T, usage string, marker string) []string {
	t.Helper()
	start := strings.Index(usage, marker)
	if start < 0 {
		t.Fatalf("help text %q carries no exclusion section %q", usage, marker)
	}
	rest := usage[start+len(marker):]
	if cut := strings.Index(rest, " ("); cut >= 0 {
		rest = rest[:cut]
	}
	if cut := strings.Index(rest, ";"); cut >= 0 {
		rest = rest[:cut]
	}
	return strings.Split(rest, ", ")
}

// helpExample extracts the canonical example rendered as the final section of
// a rule-carrying help text.
func helpExample(t *testing.T, usage string) string {
	t.Helper()
	const marker = "; example: "
	index := strings.Index(usage, marker)
	if index < 0 {
		t.Fatalf("help text %q carries no example", usage)
	}
	return usage[index+len(marker):]
}
