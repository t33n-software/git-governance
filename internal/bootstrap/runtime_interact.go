package bootstrap

import (
	"context"
	"os"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// promptAvailable resolves the interaction boundary: prompts exist only when
// the output is human, the interactive mode allows it, and both standard
// input and standard output are terminals.
func (application *application) promptAvailable() bool {
	if application.options.output == "json" || application.options.interactive == "never" {
		return false
	}
	if application.options.interactive == "always" {
		return application.inputIsTerminal() && application.outputIsTerminal()
	}
	return application.inputIsTerminal() && application.outputIsTerminal()
}

func (application *application) prompt() port.Prompt {
	return application.runtime.PromptFactory(application.options.accessible, application.options.color)
}

func (application *application) inputIsTerminal() bool {
	return application.runtime.InputIsTerminal != nil && application.runtime.InputIsTerminal()
}

func (application *application) outputIsTerminal() bool {
	return application.runtime.OutputIsTerminal != nil && application.runtime.OutputIsTerminal()
}

// colorEnabled resolves the color decision: color renders only in human output
// and only when explicitly forced or the output is a terminal.
func (application *application) colorEnabled() bool {
	if application.options.output != "human" {
		return false
	}
	switch application.options.color {
	case "always":
		return true
	case "never":
		return false
	default:
		return application.outputIsTerminal()
	}
}

// requireInput returns the supplied value or prompts for it; without a prompt
// surface the missing input fails closed.
func (application *application) requireInput(
	ctx context.Context,
	value, label, description string,
	validators ...port.InputValidator,
) (string, error) {
	if value != "" {
		return value, nil
	}
	if !application.promptAvailable() {
		return "", missingInput(label)
	}
	var validate port.InputValidator
	if len(validators) > 0 {
		validate = validators[0]
	}
	return application.prompt().Input(ctx, port.InputRequest{
		Label:       label,
		Description: description,
		Required:    true,
		Validate:    validate,
	})
}

// optionalConfirmation resolves a non-mutating confirmation: --yes answers it
// affirmatively, a missing prompt surface returns the default, otherwise the
// prompt decides.
func (application *application) optionalConfirmation(ctx context.Context, label, description string, defaultValue bool) (bool, error) {
	if application.options.yes {
		return true, nil
	}
	if !application.promptAvailable() {
		return defaultValue, nil
	}
	return application.prompt().Confirm(ctx, port.ConfirmRequest{
		Label:       label,
		Description: description,
		Default:     defaultValue,
	})
}

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func stderrIsTerminal() bool {
	info, err := os.Stderr.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func missingInput(field string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeInvalidInput,
		Category:    problem.CategoryUsage,
		Field:       field,
		Expected:    "a value supplied by a flag or interactive terminal",
		Rule:        "non-interactive execution requires all mandatory values",
		Remediation: "supply the required flag or run in an interactive terminal",
	})
}
