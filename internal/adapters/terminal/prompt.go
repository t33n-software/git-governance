// Package terminal provides interactive terminal prompts. It uses Huh forms
// for a rich TUI and a deterministic line-oriented fallback for injected IO
// and accessibility-focused automation tests.
package terminal

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"charm.land/huh/v2"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// Options configures an interactive terminal adapter.
type Options struct {
	Input      io.Reader
	Output     io.Writer
	Accessible bool
	UseForms   bool
	Color      string
}

// Prompt implements the interactive application port.
type Prompt struct {
	input      io.Reader
	reader     *bufio.Reader
	output     io.Writer
	accessible bool
	useForms   bool
	color      string
}

// New constructs a prompt adapter. The default OS stdio path uses Huh forms;
// injected readers and writers use a deterministic line-oriented fallback
// unless UseForms is explicitly set.
func New(options Options) *Prompt {
	input := options.Input
	if input == nil {
		input = os.Stdin
	}
	output := options.Output
	if output == nil {
		output = os.Stdout
	}
	useForms := options.UseForms
	if options.Input == nil && options.Output == nil {
		useForms = true
	}
	color := options.Color
	if color == "" {
		color = "auto"
	}
	if color == "never" {
		useForms = false
	}

	return &Prompt{
		input:      input,
		reader:     bufio.NewReader(input),
		output:     output,
		accessible: options.Accessible,
		useForms:   useForms,
		color:      color,
	}
}

// Input prompts for one string value.
func (prompt *Prompt) Input(ctx context.Context, request port.InputRequest) (string, error) {
	if err := contextError(ctx); err != nil {
		return "", err
	}
	if prompt.useForms {
		return prompt.formInput(ctx, request)
	}
	return prompt.lineInput(ctx, request)
}

// Select prompts for one option value.
func (prompt *Prompt) Select(ctx context.Context, request port.SelectRequest) (string, error) {
	if err := contextError(ctx); err != nil {
		return "", err
	}
	if len(request.Options) == 0 {
		return "", invalidInput("selection", "at least one selectable option", "provide at least one option")
	}
	if prompt.useForms {
		return prompt.formSelect(ctx, request)
	}
	return prompt.lineSelect(ctx, request)
}

// Confirm prompts for an explicit yes/no decision.
func (prompt *Prompt) Confirm(ctx context.Context, request port.ConfirmRequest) (bool, error) {
	if err := contextError(ctx); err != nil {
		return false, err
	}
	if prompt.useForms {
		return prompt.formConfirm(ctx, request)
	}
	return prompt.lineConfirm(ctx, request)
}

func (prompt *Prompt) formInput(ctx context.Context, request port.InputRequest) (string, error) {
	value := request.Default
	field := huh.NewInput().
		Title(request.Label).
		Description(request.Description).
		Value(&value)
	if request.Required || request.Validate != nil {
		field.Validate(func(candidate string) error {
			return validateInput(request, candidate)
		})
	}
	form := huh.NewForm(huh.NewGroup(field)).
		WithAccessible(prompt.accessible).
		WithInput(prompt.input).
		WithOutput(prompt.output)
	if err := form.RunWithContext(ctx); err != nil {
		return "", formFailure(err)
	}
	return value, nil
}

func (prompt *Prompt) formSelect(ctx context.Context, request port.SelectRequest) (string, error) {
	value := request.Default
	if value == "" {
		value = request.Options[0].Value
	}
	options := make([]huh.Option[string], 0, len(request.Options))
	for _, option := range request.Options {
		label := option.Label
		if option.Description != "" {
			label += " — " + option.Description
		}
		options = append(options, huh.NewOption(label, option.Value))
	}
	field := huh.NewSelect[string]().
		Title(request.Label).
		Description(request.Description).
		Options(options...).
		Value(&value)
	form := huh.NewForm(huh.NewGroup(field)).
		WithAccessible(prompt.accessible).
		WithInput(prompt.input).
		WithOutput(prompt.output)
	if err := form.RunWithContext(ctx); err != nil {
		return "", formFailure(err)
	}
	return value, nil
}

func (prompt *Prompt) formConfirm(ctx context.Context, request port.ConfirmRequest) (bool, error) {
	value := request.Default
	field := huh.NewConfirm().
		Title(request.Label).
		Description(request.Description).
		Value(&value).
		Affirmative("Yes").
		Negative("No")
	form := huh.NewForm(huh.NewGroup(field)).
		WithAccessible(prompt.accessible).
		WithInput(prompt.input).
		WithOutput(prompt.output)
	if err := form.RunWithContext(ctx); err != nil {
		return false, formFailure(err)
	}
	return value, nil
}

func (prompt *Prompt) lineInput(ctx context.Context, request port.InputRequest) (string, error) {
	for {
		if err := prompt.writeLabel(request.Label, request.Description); err != nil {
			return "", err
		}
		if request.Default != "" {
			if _, err := fmt.Fprintf(prompt.output, " [%s]", request.Default); err != nil {
				return "", writeFailure(err)
			}
		}
		if _, err := fmt.Fprint(prompt.output, ": "); err != nil {
			return "", writeFailure(err)
		}

		value, err := prompt.readLine(ctx)
		if err != nil {
			return "", err
		}
		if value == "" {
			value = request.Default
		}
		if err := validateInput(request, value); err == nil {
			return value, nil
		} else if _, err := fmt.Fprintln(prompt.output, err); err != nil {
			return "", writeFailure(err)
		}
	}
}

func (prompt *Prompt) lineSelect(ctx context.Context, request port.SelectRequest) (string, error) {
	defaultIndex := 1
	for index, option := range request.Options {
		if request.Default != "" && option.Value == request.Default {
			defaultIndex = index + 1
		}
	}

	for {
		if err := prompt.writeLabel(request.Label, request.Description); err != nil {
			return "", err
		}
		if _, err := fmt.Fprintln(prompt.output); err != nil {
			return "", writeFailure(err)
		}
		for index, option := range request.Options {
			if _, err := fmt.Fprintf(prompt.output, "%d. %s\n", index+1, prompt.style("36", option.Label)); err != nil {
				return "", writeFailure(err)
			}
			if option.Description != "" {
				if _, err := fmt.Fprintf(prompt.output, "   %s\n", option.Description); err != nil {
					return "", writeFailure(err)
				}
			}
		}
		if _, err := fmt.Fprintf(prompt.output, "Select an option [default %d]: ", defaultIndex); err != nil {
			return "", writeFailure(err)
		}
		raw, err := prompt.readLine(ctx)
		if err != nil {
			return "", err
		}
		if raw == "" {
			return request.Options[defaultIndex-1].Value, nil
		}
		selection, err := strconv.Atoi(raw)
		if err == nil && selection >= 1 && selection <= len(request.Options) {
			return request.Options[selection-1].Value, nil
		}
		if _, err := fmt.Fprintln(prompt.output, "Choose a listed option number."); err != nil {
			return "", writeFailure(err)
		}
	}
}

func (prompt *Prompt) lineConfirm(ctx context.Context, request port.ConfirmRequest) (bool, error) {
	defaultLabel := "y/N"
	if request.Default {
		defaultLabel = "Y/n"
	}

	for {
		if err := prompt.writeLabel(request.Label, request.Description); err != nil {
			return false, err
		}
		if _, err := fmt.Fprintf(prompt.output, " [%s]: ", defaultLabel); err != nil {
			return false, writeFailure(err)
		}
		value, err := prompt.readLine(ctx)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(value) {
		case "":
			return request.Default, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			if _, err := fmt.Fprintln(prompt.output, "Enter yes or no."); err != nil {
				return false, writeFailure(err)
			}
		}
	}
}

func (prompt *Prompt) writeLabel(label, description string) error {
	if description != "" {
		if _, err := fmt.Fprintln(prompt.output, description); err != nil {
			return writeFailure(err)
		}
	}
	if label == "" {
		return invalidInput("prompt label", "a non-empty label", "supply a prompt label")
	}
	if _, err := fmt.Fprint(prompt.output, prompt.style("36", label)); err != nil {
		return writeFailure(err)
	}
	return nil
}

func (prompt *Prompt) readLine(ctx context.Context) (string, error) {
	if err := contextError(ctx); err != nil {
		return "", err
	}
	line, err := prompt.reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", writeFailure(err)
	}
	if errors.Is(err, io.EOF) && line == "" {
		return "", problem.New(problem.Details{
			Code:        problem.CodeOperationCancelled,
			Category:    problem.CategoryCancelled,
			Field:       "interactive input",
			Expected:    "a value from the terminal",
			Rule:        "interactive prompts stop when input closes",
			Remediation: "run the command from an interactive terminal or supply flags",
		})
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func formFailure(cause error) error {
	if errors.Is(cause, huh.ErrUserAborted) || errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return problem.Wrap(problem.Details{
			Code:        problem.CodeOperationCancelled,
			Category:    problem.CategoryCancelled,
			Field:       "interactive input",
			Expected:    "a completed form",
			Rule:        "interactive workflows require explicit completion",
			Remediation: "rerun the command and complete the form or use non-interactive flags",
		}, cause)
	}
	return problem.Wrap(problem.Details{
		Code:        problem.CodeExternalCommandFailed,
		Category:    problem.CategoryExternal,
		Field:       "interactive input",
		Expected:    "a working terminal form",
		Rule:        "the terminal adapter must render and receive input",
		Remediation: "use --accessible, run in a terminal, or use non-interactive flags",
	}, cause)
}

func invalidInput(field, expected, remediation string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeInvalidInput,
		Category:    problem.CategoryUsage,
		Field:       field,
		Expected:    expected,
		Rule:        "interactive input must satisfy the prompt contract",
		Remediation: remediation,
	})
}

func contextError(ctx context.Context) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	return problem.Wrap(problem.Details{
		Code:        problem.CodeOperationCancelled,
		Category:    problem.CategoryCancelled,
		Field:       "interactive input",
		Expected:    "an active context",
		Rule:        "interactive prompts stop when their context is cancelled",
		Remediation: "retry with an active context",
	}, ctx.Err())
}

func writeFailure(cause error) error {
	return problem.Wrap(problem.Details{
		Code:        problem.CodeExternalCommandFailed,
		Category:    problem.CategoryExternal,
		Field:       "terminal",
		Expected:    "readable input and writable output streams",
		Rule:        "terminal interaction must complete without an I/O failure",
		Remediation: "check the terminal stream or use non-interactive flags",
	}, cause)
}

func (prompt *Prompt) style(code, value string) string {
	if prompt.color != "always" || value == "" {
		return value
	}
	return "\x1b[" + code + "m" + value + "\x1b[0m"
}

func validateInput(request port.InputRequest, candidate string) error {
	if request.Required && strings.TrimSpace(candidate) == "" {
		return errors.New("a value is required")
	}
	if request.Validate == nil {
		return nil
	}
	if err := request.Validate(candidate); err != nil {
		return inputValidationFailure(request, candidate, err)
	}
	return nil
}

func inputValidationFailure(request port.InputRequest, candidate string, cause error) error {
	var message strings.Builder
	label := request.Label
	if label == "" {
		label = "this value"
	}
	fmt.Fprintf(&message, "Invalid value for %s.", label)

	typed, hasProblem := problem.As(cause)
	if !request.Sensitive && (!hasProblem || !typed.SensitiveActual) {
		actual := candidate
		if hasProblem && typed.Actual != "" {
			actual = typed.Actual
		}
		if actual != "" {
			fmt.Fprintf(&message, "\nActual value:\n  %s", displayValue(actual))
		}
	}

	if hasProblem {
		appendDiagnosticSection(&message, "What is wrong?", typed.Rule)
		appendDiagnosticSection(&message, "Expected", typed.Expected)
		appendDiagnosticSection(&message, "Valid example", typed.Example)
		appendDiagnosticSection(&message, "How to fix it", typed.Remediation)
	} else {
		appendDiagnosticSection(&message, "What is wrong?", cause.Error())
		appendDiagnosticSection(&message, "Expected", request.Description)
	}
	message.WriteString("\nEnter a new value.")
	return errors.New(message.String())
}

func appendDiagnosticSection(message *strings.Builder, label, value string) {
	if value == "" {
		return
	}
	separator := ":"
	if strings.HasSuffix(label, "?") {
		separator = ""
	}
	fmt.Fprintf(message, "\n%s%s\n  %s", label, separator, value)
}

func displayValue(value string) string {
	if strings.ContainsAny(value, "\r\n\t\x1b") {
		return strconv.Quote(value)
	}
	return value
}

var _ port.Prompt = (*Prompt)(nil)
