package report

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestHumanReportSuccessAndQuiet(t *testing.T) {
	t.Parallel()

	buffer := &bytes.Buffer{}
	reporter := New(Options{Writer: buffer, Format: FormatHuman})
	err := reporter.Report(context.Background(), port.Report{
		Operation: "branch.create",
		Summary:   "Branch created.",
		Fields: map[string]string{
			"branch": "feature/ABC-123-add-export",
			"base":   "origin/develop",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := "Branch created.\nbase: origin/develop\nbranch: feature/ABC-123-add-export\n"
	if buffer.String() != expected {
		t.Fatalf("human output = %q, want %q", buffer.String(), expected)
	}

	quiet := &bytes.Buffer{}
	if err := New(Options{Writer: quiet, Quiet: true}).Report(context.Background(), port.Report{Summary: "hidden"}); err != nil {
		t.Fatal(err)
	}
	if quiet.Len() != 0 {
		t.Fatalf("quiet output = %q", quiet.String())
	}
}

func TestHumanReportAppliesExplicitColor(t *testing.T) {
	t.Parallel()

	buffer := &bytes.Buffer{}
	reporter := New(Options{Writer: buffer, Format: FormatHuman, Color: true})
	if err := reporter.Report(context.Background(), port.Report{
		Summary: "Branch created.",
		Fields:  map[string]string{"branch": "feature/ABC-123-add-export"},
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buffer.String(), "\x1b[32mBranch created.\x1b[0m") {
		t.Fatalf("colored summary = %q", buffer.String())
	}
	if !strings.Contains(buffer.String(), "\x1b[36mbranch\x1b[0m") {
		t.Fatalf("colored field = %q", buffer.String())
	}
}

func TestHumanProblemReport(t *testing.T) {
	t.Parallel()

	buffer := &bytes.Buffer{}
	value := problem.New(problem.Details{
		Code:        problem.CodeTicketKeyInvalid,
		Category:    problem.CategoryGovernance,
		Field:       "ticket key",
		Context:     "action=create branch",
		Actual:      "abc",
		Expected:    "uppercase letters",
		Rule:        "ticket keys must be uppercase",
		Example:     "ABC",
		Remediation: "use uppercase",
		Diagnostic:  "fatal: key is invalid",
		WorkflowInputs: []problem.WorkflowInput{
			{Field: "ticket key", Value: "abc"},
			{Field: "commit subject", Value: "private", Sensitive: true},
		},
	})
	if err := New(Options{Writer: buffer}).Report(context.Background(), port.Report{Problem: value}); err != nil {
		t.Fatal(err)
	}
	output := buffer.String()
	for _, expected := range []string{
		"Error [TICKET_KEY_INVALID]",
		"Field: ticket key",
		"Context:",
		"Actual value:",
		"abc",
		"What is wrong?",
		"Expected:",
		"Valid example:",
		"How to fix it:",
		"Diagnostic:",
		"Workflow inputs:",
		"ticket key: abc",
		"commit subject: [redacted]",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("human problem output missing %q: %q", expected, output)
		}
	}
}

func TestJSONReportContracts(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		buffer := &bytes.Buffer{}
		reporter := New(Options{Writer: buffer, Format: FormatJSON})
		if err := reporter.Report(context.Background(), port.Report{
			Operation: "branch.validate",
			Summary:   "valid",
			Fields:    map[string]string{"branch": "feature/ABC-123-add-export"},
		}); err != nil {
			t.Fatal(err)
		}
		expected := "{\"schemaVersion\":1,\"ok\":true,\"operation\":\"branch.validate\",\"summary\":\"valid\",\"fields\":{\"branch\":\"feature/ABC-123-add-export\"}}\n"
		if buffer.String() != expected {
			t.Fatalf("JSON output = %q, want %q", buffer.String(), expected)
		}
	})

	t.Run("sensitive problem omits actual", func(t *testing.T) {
		buffer := &bytes.Buffer{}
		value := problem.New(problem.Details{
			Code:                problem.CodeConfigurationInvalid,
			Category:            problem.CategoryConfig,
			Actual:              "secret",
			SensitiveActual:     true,
			Diagnostic:          "secret diagnostic",
			SensitiveDiagnostic: true,
			WorkflowInputs: []problem.WorkflowInput{
				{Field: "credential", Value: "secret", Sensitive: true},
			},
		})
		if err := New(Options{Writer: buffer, Format: FormatJSON}).Report(context.Background(), port.Report{Problem: value}); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(buffer.String(), "secret") || strings.Contains(buffer.String(), `"actual"`) || strings.Contains(buffer.String(), `"diagnostic"`) {
			t.Fatalf("sensitive JSON output = %q", buffer.String())
		}
	})

	t.Run("sensitive human problem omits actual", func(t *testing.T) {
		buffer := &bytes.Buffer{}
		value := problem.New(problem.Details{
			Code:                problem.CodeConfigurationInvalid,
			Category:            problem.CategoryConfig,
			Actual:              "secret",
			SensitiveActual:     true,
			Diagnostic:          "secret diagnostic",
			SensitiveDiagnostic: true,
			WorkflowInputs: []problem.WorkflowInput{
				{Field: "credential", Value: "secret", Sensitive: true},
			},
		})
		if err := New(Options{Writer: buffer, Format: FormatHuman}).Report(context.Background(), port.Report{Problem: value}); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(buffer.String(), "secret") || strings.Contains(buffer.String(), "Actual value:") || strings.Contains(buffer.String(), "Diagnostic:") {
			t.Fatalf("sensitive human output = %q", buffer.String())
		}
	})

	t.Run("JSON problem includes diagnostic context and non-sensitive inputs", func(t *testing.T) {
		buffer := &bytes.Buffer{}
		value := problem.New(problem.Details{
			Code:       problem.CodeGitCommandFailed,
			Category:   problem.CategoryGit,
			Context:    "action=create branch",
			Diagnostic: "fatal: invalid reference",
			WorkflowInputs: []problem.WorkflowInput{
				{Field: "ticket number", Value: "1"},
			},
		})
		if err := New(Options{Writer: buffer, Format: FormatJSON}).Report(context.Background(), port.Report{Problem: value}); err != nil {
			t.Fatal(err)
		}
		for _, expected := range []string{
			`"context":"action=create branch"`,
			`"diagnostic":"fatal: invalid reference"`,
			`"inputs":[{"field":"ticket number","value":"1"}]`,
		} {
			if !strings.Contains(buffer.String(), expected) {
				t.Fatalf("JSON problem output missing %q: %q", expected, buffer.String())
			}
		}
	})
}

func TestReportFailureAndCancellation(t *testing.T) {
	t.Parallel()

	t.Run("writer failure", func(t *testing.T) {
		err := New(Options{Writer: failingWriter{}}).Report(context.Background(), port.Report{Summary: "output"})
		assertProblemCode(t, err, problem.CodeExternalCommandFailed)
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := New(Options{Writer: &bytes.Buffer{}}).Report(ctx, port.Report{})
		assertProblemCode(t, err, problem.CodeOperationCancelled)
	})
}

func TestReporterDefaultAndOptionalFieldPaths(t *testing.T) {
	t.Parallel()

	if err := New(Options{}).Report(context.Background(), port.Report{}); err != nil {
		t.Fatalf("default reporter error = %v", err)
	}

	fieldsOnly := &bytes.Buffer{}
	if err := New(Options{Writer: fieldsOnly, Format: FormatHuman}).Report(context.Background(), port.Report{
		Fields: map[string]string{"branch": "feature/ABC-123-add-export"},
	}); err != nil {
		t.Fatal(err)
	}
	if fieldsOnly.String() != "branch: feature/ABC-123-add-export\n" {
		t.Fatalf("fields-only output = %q", fieldsOnly.String())
	}

	jsonFailure := New(Options{Writer: failingWriter{}, Format: FormatJSON})
	if err := jsonFailure.Report(context.Background(), port.Report{Summary: "result"}); err == nil {
		t.Fatal("JSON reporter suppressed writer failure")
	}

	minimalProblem := &bytes.Buffer{}
	if err := New(Options{Writer: minimalProblem}).Report(context.Background(), port.Report{
		Problem: problem.New(problem.Details{Code: problem.CodeInternal, Category: problem.CategoryInternal}),
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(minimalProblem.String(), "Error [INTERNAL]") {
		t.Fatalf("minimal problem output = %q", minimalProblem.String())
	}

	for failAt := 0; failAt < 11; failAt++ {
		failAt := failAt
		t.Run("problem writer failure "+string(rune('0'+failAt)), func(t *testing.T) {
			writer := &failAfterWriter{failAt: failAt}
			err := New(Options{Writer: writer}).Report(context.Background(), port.Report{
				Problem: problem.New(problem.Details{
					Code:        problem.CodeInvalidInput,
					Category:    problem.CategoryUsage,
					Field:       "field",
					Context:     "context",
					Actual:      "actual",
					Rule:        "rule",
					Expected:    "expected",
					Example:     "example",
					Remediation: "remediation",
					Diagnostic:  "diagnostic",
					WorkflowInputs: []problem.WorkflowInput{
						{Field: "input", Value: "value"},
					},
				}),
			})
			if err == nil {
				t.Fatalf("failure point %d was not propagated", failAt)
			}
		})
	}

	err := New(Options{Writer: &failAfterWriter{failAt: 1}}).Report(context.Background(), port.Report{
		Summary: "summary",
		Fields:  map[string]string{"field": "value"},
	})
	if err == nil {
		t.Fatal("human field writer failure was not propagated")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

type failAfterWriter struct {
	writes int
	failAt int
}

func (writer *failAfterWriter) Write(value []byte) (int, error) {
	if writer.writes == writer.failAt {
		return 0, errors.New("write failed")
	}
	writer.writes++
	return len(value), nil
}

func assertProblemCode(t *testing.T, err error, expected problem.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected problem code %q, got nil", expected)
	}
	actual, ok := problem.As(err)
	if !ok {
		t.Fatalf("error %T does not carry a problem: %v", err, err)
	}
	if actual.Code != expected {
		t.Fatalf("problem code = %q, want %q", actual.Code, expected)
	}
}
