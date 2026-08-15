package terminal

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestPromptCoverageCancelledBoundaries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	prompt := New(Options{
		Input:  strings.NewReader("unused\n"),
		Output: &bytes.Buffer{},
	})

	_, err := prompt.Select(ctx, port.SelectRequest{
		Label:   "Family",
		Options: []port.SelectOption{{Label: "Feature", Value: "feature"}},
	})
	assertProblemCode(t, err, problem.CodeOperationCancelled)

	_, err = prompt.Confirm(ctx, port.ConfirmRequest{Label: "Continue"})
	assertProblemCode(t, err, problem.CodeOperationCancelled)

	_, err = prompt.readLine(ctx)
	assertProblemCode(t, err, problem.CodeOperationCancelled)
}

func TestPromptCoverageAccessibleFormRetries(t *testing.T) {
	t.Run("required input", func(t *testing.T) {
		output := &bytes.Buffer{}
		prompt := New(Options{
			Input:      strings.NewReader("\nABC\n"),
			Output:     output,
			Accessible: true,
			UseForms:   true,
		})

		actual, err := prompt.Input(context.Background(), port.InputRequest{
			Label:    "Ticket key",
			Required: true,
		})
		if err != nil || actual != "ABC" {
			t.Fatalf("Input() = (%q, %v); output=%q", actual, err, output.String())
		}
		if !strings.Contains(output.String(), "a value is required") {
			t.Fatalf("accessible retry output = %q", output.String())
		}
	})

	t.Run("select option descriptions", func(t *testing.T) {
		output := &bytes.Buffer{}
		prompt := New(Options{
			Input:      strings.NewReader("2\n"),
			Output:     output,
			Accessible: true,
			UseForms:   true,
		})

		actual, err := prompt.Select(context.Background(), port.SelectRequest{
			Label: "Family",
			Options: []port.SelectOption{
				{Label: "Feature", Value: "feature", Description: "New capability"},
				{Label: "Fix", Value: "fix"},
			},
		})
		if err != nil || actual != "fix" {
			t.Fatalf("Select() = (%q, %v); output=%q", actual, err, output.String())
		}
		if !strings.Contains(output.String(), "Feature — New capability") {
			t.Fatalf("accessible select output = %q", output.String())
		}
	})
}

func TestPromptCoverageFormFailuresUseInjectedReader(t *testing.T) {
	cases := []struct {
		name string
		run  func(*Prompt) error
	}{
		{
			name: "input",
			run: func(prompt *Prompt) error {
				_, err := prompt.Input(context.Background(), port.InputRequest{Label: "Ticket key"})
				return err
			},
		},
		{
			name: "select",
			run: func(prompt *Prompt) error {
				_, err := prompt.Select(context.Background(), port.SelectRequest{
					Label:   "Family",
					Options: []port.SelectOption{{Label: "Feature", Value: "feature"}},
				})
				return err
			},
		},
		{
			name: "confirm",
			run: func(prompt *Prompt) error {
				_, err := prompt.Confirm(context.Background(), port.ConfirmRequest{Label: "Continue"})
				return err
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			prompt := New(Options{
				Input:    coverageFailingReader{},
				Output:   &bytes.Buffer{},
				UseForms: true,
			})

			assertProblemCode(t, testCase.run(prompt), problem.CodeExternalCommandFailed)
		})
	}
}

func TestPromptCoverageLineInputWriteFailures(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		failAt  int
		request port.InputRequest
	}{
		{
			name:   "default",
			failAt: 2,
			request: port.InputRequest{
				Label:   "Ticket key",
				Default: "ABC",
			},
		},
		{
			name:   "separator",
			failAt: 2,
			request: port.InputRequest{
				Label: "Ticket key",
			},
		},
		{
			name:   "required feedback",
			input:  "\n",
			failAt: 3,
			request: port.InputRequest{
				Label:    "Ticket key",
				Required: true,
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			prompt := New(Options{
				Input:  strings.NewReader(testCase.input),
				Output: &coverageFailingWriter{failAt: testCase.failAt},
			})

			_, err := prompt.lineInput(context.Background(), testCase.request)
			assertProblemCode(t, err, problem.CodeExternalCommandFailed)
		})
	}
}

func TestPromptCoverageLineSelectFailures(t *testing.T) {
	oneOption := []port.SelectOption{{Label: "Feature", Value: "feature"}}
	describedOption := []port.SelectOption{{
		Label:       "Feature",
		Value:       "feature",
		Description: "New capability",
	}}

	cases := []struct {
		name    string
		input   io.Reader
		output  io.Writer
		request port.SelectRequest
	}{
		{
			name:   "label",
			input:  strings.NewReader(""),
			output: &coverageFailingWriter{failAt: 1},
			request: port.SelectRequest{
				Label:   "Family",
				Options: oneOption,
			},
		},
		{
			name:   "description",
			input:  strings.NewReader(""),
			output: &coverageFailingWriter{failAt: 1},
			request: port.SelectRequest{
				Label:       "Family",
				Description: "Choose a family",
				Options:     oneOption,
			},
		},
		{
			name:   "option separator",
			input:  strings.NewReader(""),
			output: &coverageFailingWriter{failAt: 2},
			request: port.SelectRequest{
				Label:   "Family",
				Options: oneOption,
			},
		},
		{
			name:   "option label",
			input:  strings.NewReader(""),
			output: &coverageFailingWriter{failAt: 3},
			request: port.SelectRequest{
				Label:   "Family",
				Options: oneOption,
			},
		},
		{
			name:   "option description",
			input:  strings.NewReader(""),
			output: &coverageFailingWriter{failAt: 4},
			request: port.SelectRequest{
				Label:   "Family",
				Options: describedOption,
			},
		},
		{
			name:   "selection prompt",
			input:  strings.NewReader(""),
			output: &coverageFailingWriter{failAt: 4},
			request: port.SelectRequest{
				Label:   "Family",
				Options: oneOption,
			},
		},
		{
			name:   "reader",
			input:  coverageFailingReader{},
			output: &bytes.Buffer{},
			request: port.SelectRequest{
				Label:   "Family",
				Options: oneOption,
			},
		},
		{
			name:   "invalid value feedback",
			input:  strings.NewReader("invalid\n"),
			output: &coverageFailingWriter{failAt: 5},
			request: port.SelectRequest{
				Label:   "Family",
				Options: oneOption,
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			prompt := New(Options{Input: testCase.input, Output: testCase.output})

			_, err := prompt.lineSelect(context.Background(), testCase.request)
			assertProblemCode(t, err, problem.CodeExternalCommandFailed)
		})
	}
}

func TestPromptCoverageLineConfirmValuesAndFailures(t *testing.T) {
	t.Run("default yes", func(t *testing.T) {
		prompt := New(Options{
			Input:  strings.NewReader("\n"),
			Output: &bytes.Buffer{},
		})

		actual, err := prompt.lineConfirm(context.Background(), port.ConfirmRequest{
			Label:   "Continue",
			Default: true,
		})
		if err != nil || !actual {
			t.Fatalf("lineConfirm() = (%t, %v)", actual, err)
		}
	})

	t.Run("negative value", func(t *testing.T) {
		prompt := New(Options{
			Input:  strings.NewReader("No\n"),
			Output: &bytes.Buffer{},
		})

		actual, err := prompt.lineConfirm(context.Background(), port.ConfirmRequest{Label: "Continue"})
		if err != nil || actual {
			t.Fatalf("lineConfirm() = (%t, %v)", actual, err)
		}
	})

	cases := []struct {
		name   string
		input  io.Reader
		output io.Writer
	}{
		{
			name:   "label",
			input:  strings.NewReader(""),
			output: &coverageFailingWriter{failAt: 1},
		},
		{
			name:   "prompt",
			input:  strings.NewReader(""),
			output: &coverageFailingWriter{failAt: 2},
		},
		{
			name:   "reader",
			input:  coverageFailingReader{},
			output: &bytes.Buffer{},
		},
		{
			name:   "invalid feedback",
			input:  strings.NewReader("maybe\n"),
			output: &coverageFailingWriter{failAt: 3},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			prompt := New(Options{Input: testCase.input, Output: testCase.output})

			_, err := prompt.lineConfirm(context.Background(), port.ConfirmRequest{Label: "Continue"})
			assertProblemCode(t, err, problem.CodeExternalCommandFailed)
		})
	}
}

func TestPromptCoverageStyleModes(t *testing.T) {
	if actual := (&Prompt{color: "auto"}).style("36", "Ticket key"); actual != "Ticket key" {
		t.Fatalf("auto style = %q", actual)
	}
	if actual := (&Prompt{color: "always"}).style("36", ""); actual != "" {
		t.Fatalf("empty style = %q", actual)
	}
}

type coverageFailingWriter struct {
	failAt int
	writes int
}

func (writer *coverageFailingWriter) Write(value []byte) (int, error) {
	writer.writes++
	if writer.writes == writer.failAt {
		return 0, errors.New("write failed")
	}
	return len(value), nil
}

type coverageFailingReader struct{}

func (coverageFailingReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}
