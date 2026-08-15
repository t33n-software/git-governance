// Package report renders application results as stable human or JSON output.
package report

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// Format controls the public reporting contract.
type Format string

const (
	FormatHuman Format = "human"
	FormatJSON  Format = "json"
)

// Options configures a Reporter.
type Options struct {
	Writer io.Writer
	Format Format
	Quiet  bool
	Color  bool
}

// Reporter implements the application output port.
type Reporter struct {
	writer io.Writer
	format Format
	quiet  bool
	color  bool
}

// New creates an output reporter with safe defaults.
func New(options Options) *Reporter {
	format := options.Format
	if format == "" {
		format = FormatHuman
	}
	writer := options.Writer
	if writer == nil {
		writer = io.Discard
	}
	return &Reporter{
		writer: writer,
		format: format,
		quiet:  options.Quiet,
		color:  options.Color,
	}
}

// Report writes a result according to the configured output contract.
func (reporter *Reporter) Report(ctx context.Context, result port.Report) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if reporter.format == FormatJSON {
		return reporter.writeJSON(result)
	}
	return reporter.writeHuman(result)
}

func (reporter *Reporter) writeJSON(result port.Report) error {
	output := jsonOutput{
		SchemaVersion: 1,
		OK:            result.Problem == nil,
		Operation:     result.Operation,
		Summary:       result.Summary,
		Fields:        result.Fields,
		Data:          result.Data,
	}
	if result.Problem != nil {
		output.Error = problemOutputFrom(result.Problem)
	}

	encoder := json.NewEncoder(reporter.writer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		return writeFailure(err)
	}
	return nil
}

func (reporter *Reporter) writeHuman(result port.Report) error {
	if result.Problem != nil {
		return reporter.writeHumanProblem(result.Problem)
	}
	if reporter.quiet {
		return nil
	}
	if result.Summary != "" {
		if _, err := fmt.Fprintln(reporter.writer, reporter.style("32", result.Summary)); err != nil {
			return writeFailure(err)
		}
	}
	keys := sortedKeys(result.Fields)
	for _, key := range keys {
		if _, err := fmt.Fprintf(reporter.writer, "%s: %s\n", reporter.style("36", key), result.Fields[key]); err != nil {
			return writeFailure(err)
		}
	}
	return nil
}

func (reporter *Reporter) writeHumanProblem(value *problem.Problem) error {
	if _, err := fmt.Fprintf(reporter.writer, "%s\n", reporter.style("31", "Error ["+string(value.Code)+"]")); err != nil {
		return writeFailure(err)
	}
	if value.Field != "" {
		if _, err := fmt.Fprintf(reporter.writer, "\n%s: %s\n", reporter.style("36", "Field"), value.Field); err != nil {
			return writeFailure(err)
		}
	}
	if value.Context != "" {
		if _, err := fmt.Fprintf(reporter.writer, "\n%s:\n  %s\n", reporter.style("36", "Context"), value.Context); err != nil {
			return writeFailure(err)
		}
	}
	if value.Actual != "" && !value.SensitiveActual {
		if _, err := fmt.Fprintf(reporter.writer, "\n%s:\n  %s\n", reporter.style("36", "Actual value"), value.Actual); err != nil {
			return writeFailure(err)
		}
	}
	if value.Rule != "" {
		if _, err := fmt.Fprintf(reporter.writer, "\n%s\n  %s\n", reporter.style("36", "What is wrong?"), value.Rule); err != nil {
			return writeFailure(err)
		}
	}
	if value.Expected != "" {
		if _, err := fmt.Fprintf(reporter.writer, "\n%s:\n  %s\n", reporter.style("36", "Expected"), value.Expected); err != nil {
			return writeFailure(err)
		}
	}
	if value.Example != "" {
		if _, err := fmt.Fprintf(reporter.writer, "\n%s:\n  %s\n", reporter.style("36", "Valid example"), value.Example); err != nil {
			return writeFailure(err)
		}
	}
	if value.Remediation != "" {
		if _, err := fmt.Fprintf(reporter.writer, "\n%s:\n  %s\n", reporter.style("36", "How to fix it"), value.Remediation); err != nil {
			return writeFailure(err)
		}
	}
	if value.Diagnostic != "" && !value.SensitiveDiagnostic {
		if _, err := fmt.Fprintf(reporter.writer, "\n%s:\n  %s\n", reporter.style("36", "Diagnostic"), value.Diagnostic); err != nil {
			return writeFailure(err)
		}
	}
	if len(value.WorkflowInputs) > 0 {
		if _, err := fmt.Fprintf(reporter.writer, "\n%s:\n", reporter.style("36", "Workflow inputs")); err != nil {
			return writeFailure(err)
		}
		for _, input := range value.WorkflowInputs {
			actual := input.Value
			if input.Sensitive {
				actual = "[redacted]"
			}
			if _, err := fmt.Fprintf(reporter.writer, "  %s: %s\n", input.Field, actual); err != nil {
				return writeFailure(err)
			}
		}
	}
	return nil
}

type jsonOutput struct {
	SchemaVersion int               `json:"schemaVersion"`
	OK            bool              `json:"ok"`
	Operation     string            `json:"operation,omitempty"`
	Summary       string            `json:"summary,omitempty"`
	Fields        map[string]string `json:"fields,omitempty"`
	Data          any               `json:"data,omitempty"`
	Error         *problemOutput    `json:"error,omitempty"`
}

type problemOutput struct {
	Code        problem.Code          `json:"code"`
	Category    problem.Category      `json:"category"`
	Field       string                `json:"field,omitempty"`
	Context     string                `json:"context,omitempty"`
	Actual      string                `json:"actual,omitempty"`
	Expected    string                `json:"expected,omitempty"`
	Rule        string                `json:"rule,omitempty"`
	Example     string                `json:"example,omitempty"`
	Remediation string                `json:"remediation,omitempty"`
	Diagnostic  string                `json:"diagnostic,omitempty"`
	Inputs      []workflowInputOutput `json:"inputs,omitempty"`
}

type workflowInputOutput struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

func problemOutputFrom(value *problem.Problem) *problemOutput {
	actual := value.Actual
	if value.SensitiveActual {
		actual = ""
	}
	diagnostic := value.Diagnostic
	if value.SensitiveDiagnostic {
		diagnostic = ""
	}
	inputs := make([]workflowInputOutput, 0, len(value.WorkflowInputs))
	for _, input := range value.WorkflowInputs {
		actual := input.Value
		if input.Sensitive {
			actual = ""
		}
		inputs = append(inputs, workflowInputOutput{
			Field: input.Field,
			Value: actual,
		})
	}
	return &problemOutput{
		Code:        value.Code,
		Category:    value.Category,
		Field:       value.Field,
		Context:     value.Context,
		Actual:      actual,
		Expected:    value.Expected,
		Rule:        value.Rule,
		Example:     value.Example,
		Remediation: value.Remediation,
		Diagnostic:  diagnostic,
		Inputs:      inputs,
	}
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (reporter *Reporter) style(code, value string) string {
	if !reporter.color || value == "" {
		return value
	}
	return "\x1b[" + code + "m" + value + "\x1b[0m"
}

func contextError(ctx context.Context) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	return problem.Wrap(problem.Details{
		Code:        problem.CodeOperationCancelled,
		Category:    problem.CategoryCancelled,
		Field:       "operation",
		Expected:    "an active context",
		Rule:        "reporting stops when the caller cancels its context",
		Remediation: "retry with an active context",
	}, ctx.Err())
}

func writeFailure(cause error) error {
	return problem.Wrap(problem.Details{
		Code:        problem.CodeExternalCommandFailed,
		Category:    problem.CategoryExternal,
		Field:       "output",
		Expected:    "a writable output stream",
		Rule:        "delivery output must be written completely",
		Remediation: "check the output destination and retry",
	}, cause)
}

var _ port.Reporter = (*Reporter)(nil)
