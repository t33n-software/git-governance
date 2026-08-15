package terminal

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"charm.land/huh/v2"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

func TestLineInputUsesDefaultAndRequiredValidation(t *testing.T) {
	t.Parallel()

	t.Run("default", func(t *testing.T) {
		output := &bytes.Buffer{}
		prompt := New(Options{
			Input:  strings.NewReader("\n"),
			Output: output,
		})
		actual, err := prompt.Input(context.Background(), port.InputRequest{
			Label:       "Ticket key",
			Description: "Enter the ticket namespace.",
			Default:     "ABC",
			Required:    true,
		})
		if err != nil || actual != "ABC" {
			t.Fatalf("Input() = (%q, %v)", actual, err)
		}
		if !strings.Contains(output.String(), "Ticket key [ABC]:") {
			t.Fatalf("output = %q", output.String())
		}
	})

	t.Run("required retry", func(t *testing.T) {
		output := &bytes.Buffer{}
		prompt := New(Options{
			Input:  strings.NewReader("\nABC\n"),
			Output: output,
		})
		actual, err := prompt.Input(context.Background(), port.InputRequest{
			Label:    "Ticket key",
			Required: true,
		})
		if err != nil || actual != "ABC" {
			t.Fatalf("Input() = (%q, %v)", actual, err)
		}
		if !strings.Contains(output.String(), "a value is required") {
			t.Fatalf("output = %q", output.String())
		}
	})
}

func TestInputValidationRetriesAndExplainsDomainFailures(t *testing.T) {
	t.Parallel()

	t.Run("line input", func(t *testing.T) {
		output := &bytes.Buffer{}
		prompt := New(Options{
			Input:  strings.NewReader("001\n1\n"),
			Output: output,
		})

		actual, err := prompt.Input(context.Background(), port.InputRequest{
			Label:       "Ticket number",
			Description: "Enter a canonical ticket number.",
			Required:    true,
			Validate: func(value string) error {
				_, validationErr := ticket.ParseNumber(value)
				return validationErr
			},
		})
		if err != nil || actual != "1" {
			t.Fatalf("Input() = (%q, %v)", actual, err)
		}
		for _, expected := range []string{
			"Invalid value for Ticket number.",
			"Actual value:\n  001",
			"What is wrong?",
			"Expected:\n  1 to 18 decimal digits without a leading zero",
			"Valid example:\n  123",
			"How to fix it:",
			"Enter a new value.",
		} {
			if !strings.Contains(output.String(), expected) {
				t.Fatalf("validation output missing %q: %q", expected, output.String())
			}
		}
	})

	t.Run("accessible form", func(t *testing.T) {
		output := &bytes.Buffer{}
		prompt := New(Options{
			Input:      strings.NewReader("001\n1\n"),
			Output:     output,
			Accessible: true,
			UseForms:   true,
		})

		actual, err := prompt.Input(context.Background(), port.InputRequest{
			Label:    "Ticket number",
			Required: true,
			Validate: func(value string) error {
				_, validationErr := ticket.ParseNumber(value)
				return validationErr
			},
		})
		if err != nil || actual != "1" {
			t.Fatalf("form Input() = (%q, %v); output=%q", actual, err, output.String())
		}
		if !strings.Contains(output.String(), "ticket numbers must match ^[1-9][0-9]*$") {
			t.Fatalf("form validation output = %q", output.String())
		}
	})
}

func TestInputValidationFormattingProtectsSensitiveValues(t *testing.T) {
	t.Parallel()

	t.Run("generic validator error uses the prompt description", func(t *testing.T) {
		err := inputValidationFailure(
			port.InputRequest{Label: "", Description: "a canonical value"},
			"bad",
			errors.New("not accepted"),
		)
		for _, expected := range []string{
			"Invalid value for this value.",
			"Actual value:\n  bad",
			"What is wrong?\n  not accepted",
			"Expected:\n  a canonical value",
		} {
			if !strings.Contains(err.Error(), expected) {
				t.Fatalf("generic validation error missing %q: %q", expected, err)
			}
		}
	})

	t.Run("sensitive request omits the candidate", func(t *testing.T) {
		err := inputValidationFailure(
			port.InputRequest{Label: "Secret", Sensitive: true},
			"top-secret",
			problem.New(problem.Details{
				Actual:          "top-secret",
				Rule:            "the secret is invalid",
				SensitiveActual: true,
			}),
		)
		if strings.Contains(err.Error(), "top-secret") {
			t.Fatalf("sensitive validation error leaked value: %q", err)
		}
	})

	t.Run("typed problem without actual falls back to the candidate", func(t *testing.T) {
		err := inputValidationFailure(
			port.InputRequest{Label: "Value"},
			"candidate",
			problem.New(problem.Details{Rule: "the value is invalid"}),
		)
		if !strings.Contains(err.Error(), "Actual value:\n  candidate") {
			t.Fatalf("fallback actual = %q", err)
		}
	})

	if actual := displayValue("line\nbreak"); actual != `"line\nbreak"` {
		t.Fatalf("displayValue(control) = %q", actual)
	}
	if actual := displayValue("plain"); actual != "plain" {
		t.Fatalf("displayValue(plain) = %q", actual)
	}
	var message strings.Builder
	appendDiagnosticSection(&message, "Ignored", "")
	appendDiagnosticSection(&message, "Included", "value")
	if actual := message.String(); actual != "\nIncluded:\n  value" {
		t.Fatalf("diagnostic sections = %q", actual)
	}
}

func TestLineSelectAndConfirm(t *testing.T) {
	t.Parallel()

	options := []port.SelectOption{
		{Value: "feature", Label: "Feature", Description: "New product capability."},
		{Value: "fix", Label: "Fix", Description: "Correct a defect."},
	}

	t.Run("selection retries", func(t *testing.T) {
		output := &bytes.Buffer{}
		prompt := New(Options{
			Input:  strings.NewReader("9\n2\n"),
			Output: output,
		})
		actual, err := prompt.Select(context.Background(), port.SelectRequest{
			Label:       "Branch family",
			Description: "Choose a branch family.",
			Options:     options,
			Default:     "feature",
		})
		if err != nil || actual != "fix" {
			t.Fatalf("Select() = (%q, %v)", actual, err)
		}
		if !strings.Contains(output.String(), "Choose a listed option number.") {
			t.Fatalf("output = %q", output.String())
		}
	})

	t.Run("selection default", func(t *testing.T) {
		prompt := New(Options{
			Input:  strings.NewReader("\n"),
			Output: &bytes.Buffer{},
		})
		actual, err := prompt.Select(context.Background(), port.SelectRequest{
			Label:   "Branch family",
			Options: options,
			Default: "fix",
		})
		if err != nil || actual != "fix" {
			t.Fatalf("Select() = (%q, %v)", actual, err)
		}
	})

	t.Run("confirm values", func(t *testing.T) {
		prompt := New(Options{
			Input:  strings.NewReader("wrong\nyes\n"),
			Output: &bytes.Buffer{},
		})
		actual, err := prompt.Confirm(context.Background(), port.ConfirmRequest{Label: "Continue", Default: false})
		if err != nil || !actual {
			t.Fatalf("Confirm() = (%t, %v)", actual, err)
		}
	})
}

func TestColorModeControlsPlainFallbackAndLineStyling(t *testing.T) {
	t.Parallel()

	plain := New(Options{Color: "never"})
	if plain.useForms {
		t.Fatal("color=never must use the plain line-oriented prompt instead of a styled form")
	}

	output := &bytes.Buffer{}
	prompt := New(Options{
		Input:  strings.NewReader("ABC\n"),
		Output: output,
		Color:  "always",
	})
	if _, err := prompt.Input(context.Background(), port.InputRequest{Label: "Ticket key", Required: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "\x1b[36mTicket key\x1b[0m") {
		t.Fatalf("color=always prompt output = %q", output.String())
	}
}

func TestPromptInputFailures(t *testing.T) {
	t.Parallel()

	t.Run("empty options", func(t *testing.T) {
		_, err := New(Options{Input: strings.NewReader(""), Output: &bytes.Buffer{}}).Select(context.Background(), port.SelectRequest{Label: "Family"})
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("closed input", func(t *testing.T) {
		_, err := New(Options{Input: strings.NewReader(""), Output: &bytes.Buffer{}}).Input(context.Background(), port.InputRequest{Label: "Key"})
		assertProblemCode(t, err, problem.CodeOperationCancelled)
	})

	t.Run("missing label", func(t *testing.T) {
		_, err := New(Options{Input: strings.NewReader("value\n"), Output: &bytes.Buffer{}}).Input(context.Background(), port.InputRequest{})
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := New(Options{Input: strings.NewReader("value\n"), Output: &bytes.Buffer{}}).Input(ctx, port.InputRequest{Label: "Key"})
		assertProblemCode(t, err, problem.CodeOperationCancelled)
	})
}

func TestFormAdapterAccessibleMode(t *testing.T) {
	t.Parallel()

	output := &bytes.Buffer{}
	prompt := New(Options{
		Input:      strings.NewReader("1\n"),
		Output:     output,
		Accessible: true,
		UseForms:   true,
	})
	actual, err := prompt.Select(context.Background(), port.SelectRequest{
		Label: "Branch family",
		Options: []port.SelectOption{
			{Value: "feature", Label: "Feature"},
			{Value: "fix", Label: "Fix"},
		},
	})
	if err != nil || actual != "feature" {
		t.Fatalf("form Select() = (%q, %v); output=%q", actual, err, output.String())
	}
}

func TestFormInputAndConfirmAccessibleMode(t *testing.T) {
	t.Parallel()

	t.Run("input", func(t *testing.T) {
		output := &bytes.Buffer{}
		prompt := New(Options{
			Input:      strings.NewReader("ABC\n"),
			Output:     output,
			Accessible: true,
			UseForms:   true,
		})
		actual, err := prompt.Input(context.Background(), port.InputRequest{
			Label:    "Ticket key",
			Required: true,
		})
		if err != nil || actual != "ABC" {
			t.Fatalf("form Input() = (%q, %v); output=%q", actual, err, output.String())
		}
	})

	t.Run("confirm", func(t *testing.T) {
		output := &bytes.Buffer{}
		prompt := New(Options{
			Input:      strings.NewReader("y\n"),
			Output:     output,
			Accessible: true,
			UseForms:   true,
		})
		actual, err := prompt.Confirm(context.Background(), port.ConfirmRequest{
			Label:   "Continue",
			Default: false,
		})
		if err != nil || !actual {
			t.Fatalf("form Confirm() = (%t, %v); output=%q", actual, err, output.String())
		}
	})
}

func TestTerminalFailureHelpersAndOptionalPromptPaths(t *testing.T) {
	t.Parallel()

	assertProblemCode(t, formFailure(huh.ErrUserAborted), problem.CodeOperationCancelled)
	assertProblemCode(t, formFailure(errors.New("render failed")), problem.CodeExternalCommandFailed)
	assertProblemCode(t, writeFailure(errors.New("write failed")), problem.CodeExternalCommandFailed)

	output := &bytes.Buffer{}
	prompt := New(Options{Input: strings.NewReader("\n"), Output: output})
	confirmed, err := prompt.Confirm(context.Background(), port.ConfirmRequest{
		Label:   "Continue",
		Default: false,
	})
	if err != nil || confirmed {
		t.Fatalf("default confirmation = (%t, %v)", confirmed, err)
	}

	_, err = New(Options{
		Input:  strings.NewReader("value\n"),
		Output: failingTerminalWriter{},
	}).Input(context.Background(), port.InputRequest{Label: "Key"})
	assertProblemCode(t, err, problem.CodeExternalCommandFailed)

	prompt.reader = bufio.NewReader(failingTerminalReader{})
	_, err = prompt.readLine(context.Background())
	assertProblemCode(t, err, problem.CodeExternalCommandFailed)
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

type failingTerminalWriter struct{}

func (failingTerminalWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

type failingTerminalReader struct{}

func (failingTerminalReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}
