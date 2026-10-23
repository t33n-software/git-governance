package bootstrap

import (
	"os"
	"slices"

	"github.com/spf13/cobra"
	"github.com/t33n-software/git-governance/internal/application/cliparam"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

const (
	messageKindCommitBody  = "commit-body"
	messageKindPullRequest = "pr-description"
)

// messageScaffoldSources binds every scaffoldable message kind to the one
// canonical layout register it materializes from. The contract test pins the
// key set against the cliparam closed-enum domain so the two surfaces can
// never drift.
var messageScaffoldSources = map[string]func() string{
	messageKindCommitBody:  commitmsg.BodySkeleton,
	messageKindPullRequest: pullRequestBodySkeleton,
}

// createScaffoldFile and writeScaffoldBody are the file-system seams of the
// scaffold: production binds the platform primitives, tests inject failures.
var (
	createScaffoldFile = os.CreateTemp
	writeScaffoldBody  = writeScaffoldContents
)

func writeScaffoldContents(file *os.File, contents string) error {
	if _, err := file.WriteString(contents); err != nil {
		return err
	}
	return file.Close()
}

func newMessageCommand(application *application) *cobra.Command {
	command := &cobra.Command{
		Use:   "message",
		Short: "Scaffold governed message files",
	}
	command.AddCommand(newMessageScaffoldCommand(application))
	return command
}

// newMessageScaffoldCommand is the canonical message-file constructor: it
// creates the governed file in the operating-system temporary area through the
// atomic platform primitive (os.CreateTemp honors the environment-resolved
// temporary root on every OS family and guarantees a collision-safe unique
// name without any pre-existence probe), pre-fills the canonical layout from
// the same registers the help and the validator render from, and prints the
// absolute path for --body-file, --merge-body-file, or --commit-body-file. It
// is a construction aid, never a gate: the content contract stays owned by
// validation and the body-duty matrix stays a content contract. Canonical
// conventions: docs/conventions/cli/message-file-transport.md and
// docs/conventions/cli/help-contract.md.
func newMessageScaffoldCommand(application *application) *cobra.Command {
	var kind string
	command := &cobra.Command{
		Use:   "scaffold",
		Short: "Create a pre-filled message file in the OS temporary area",
		Long: "Create a pre-filled message file in the operating-system temporary area. " +
			"The scaffold is the canonical constructor of the governed message file: it places the file through the atomic platform primitive (environment-resolved temporary root, collision-safe unique name, no pre-existence probe), pre-fills the canonical layout for the selected kind from the same registers the help and the validators render from, and reports the created absolute path. " +
			"The reported file is an absolute path to a regular UTF-8 text file of at most 1 MiB, satisfying the message-file transport contract unchanged. " +
			"Pass it to --body-file, --merge-body-file, or --commit-body-file for a commit body, or to --body-file for a pull request description. " +
			"The scaffold is a construction aid, never a gate: editing or removing non-applicable sections stays the author's decision, and the content contract remains owned by validation. " +
			"Example: git-governance message scaffold --kind commit-body",
		RunE: func(command *cobra.Command, _ []string) error {
			source, ok := messageScaffoldSources[kind]
			if !ok {
				domain := cliparam.MessageScaffoldKind()
				return invalidOption("kind", kind, domain.ValueList())
			}
			file, err := createScaffoldFile("", kind+"-*.txt")
			if err != nil {
				return scaffoldFailure(kind, "the scaffolded message file must be created in the operating-system temporary area", err)
			}
			path := file.Name()
			if err := writeScaffoldBody(file, source()); err != nil {
				_ = os.Remove(path)
				return scaffoldFailure(kind, "the scaffolded message file must be written completely", err)
			}
			return application.report(command, port.Report{
				Operation: "message.scaffold",
				Summary:   "Message file scaffolded.",
				Fields: map[string]string{
					"kind": kind,
					"path": path,
				},
			})
		},
	}
	// The kind flag renders its closed-enum value list, completion, and error
	// contract from the canonical register like every value-carrying flag.
	registerValueDomainFlag(command, &kind, "kind", "", cliparam.MessageScaffoldKind(), "mandatory scaffold target")
	return command
}

func scaffoldFailure(kind, rule string, cause error) error {
	return problem.Wrap(problem.Details{
		Code:        problem.CodeMessageScaffoldFailed,
		Category:    problem.CategoryExternal,
		Field:       "kind",
		Actual:      kind,
		Expected:    "a writable operating-system temporary area",
		Rule:        rule,
		Remediation: "check that TMPDIR (Linux, macOS) or TEMP (Windows) points to a writable directory and retry",
	}, cause)
}

// messageScaffoldKindValues lists the scaffold source keys in the register
// order of the closed-enum domain for the drift contract test.
func messageScaffoldKindValues() []string {
	values := make([]string, 0, len(messageScaffoldSources))
	for _, value := range cliparam.MessageScaffoldKind().Values {
		if _, ok := messageScaffoldSources[value]; ok {
			values = append(values, value)
		}
	}
	if len(values) != len(messageScaffoldSources) {
		return nil
	}
	return slices.Clone(values)
}
