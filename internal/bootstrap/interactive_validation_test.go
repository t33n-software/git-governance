package bootstrap

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/t33n-software/git-governance/internal/adapters/terminal"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestInteractiveBranchCreateRetriesInvalidValuesInPlace(t *testing.T) {
	t.Parallel()

	promptOutput := &bytes.Buffer{}
	prompt := terminal.New(terminal.Options{
		Input: strings.NewReader(
			"1\n" + // branch family: feature
				"abc\nABC\n" +
				"001\n1\n" +
				"Test\nadd-export\n",
		),
		Output: promptOutput,
	})
	git := newCommandGit(t, "feature/ABC-123-add-export", nil)
	runtime := commandRuntime(git)
	runtime.PromptFactory = func(bool, string) port.Prompt {
		return prompt
	}
	runtime.InputIsTerminal = func() bool { return true }
	runtime.OutputIsTerminal = func() bool { return true }
	command := NewWithRuntime(BuildInfo{Version: "test"}, runtime)

	output, err := executeBootstrapCommand(
		t,
		command,
		"--interactive",
		"always",
		"--dry-run",
		"branch",
		"create",
	)
	if err != nil {
		t.Fatalf("interactive branch create error = %v; prompt=%q; output=%q", err, promptOutput.String(), output)
	}
	if !strings.Contains(output, "Branch creation plan generated.") {
		t.Fatalf("branch create output = %q", output)
	}
	for _, expected := range []string{
		"Invalid value for Ticket key.",
		"Invalid value for Ticket number.",
		"Actual value:\n  001",
		"Expected:\n  1 to 18 decimal digits without a leading zero",
		"Invalid value for Branch description.",
		"Valid example:\n  add-export-button",
		"Enter a new value.",
	} {
		if !strings.Contains(promptOutput.String(), expected) {
			t.Fatalf("interactive diagnostic missing %q: %q", expected, promptOutput.String())
		}
	}
	if strings.Count(promptOutput.String(), "Ticket number") < 2 {
		t.Fatalf("ticket number prompt was not repeated: %q", promptOutput.String())
	}
}

func TestWorkflowInputSummaryKeepsAcceptedValuesAndPreservesFailure(t *testing.T) {
	t.Parallel()

	summary := &workflowInputSummary{}
	summary.add("", "ignored")
	summary.add("ticket number", "001")
	summary.add("ticket number", "1")
	summary.add("branch description", "add-export")

	cause := problem.New(problem.Details{
		Code:     problem.CodeBranchBaseInvalid,
		Category: problem.CategoryRepository,
		Field:    "target base",
	})
	err := summary.attach(cause)
	actual, ok := problem.As(err)
	if !ok || len(actual.WorkflowInputs) != 2 {
		t.Fatalf("summary attachment = %#v", err)
	}
	if actual.WorkflowInputs[0] != (problem.WorkflowInput{Field: "ticket number", Value: "1"}) {
		t.Fatalf("ticket summary = %#v", actual.WorkflowInputs)
	}

	wrapped := withWorkflowInputs(func(_ *cobra.Command, inputs *workflowInputSummary) error {
		inputs.add("affected line", "main")
		return cause
	})
	err = wrapped(&cobra.Command{}, nil)
	actual, ok = problem.As(err)
	if !ok || len(actual.WorkflowInputs) != 1 || actual.WorkflowInputs[0].Value != "main" {
		t.Fatalf("wrapped workflow summary = %#v", err)
	}
}
