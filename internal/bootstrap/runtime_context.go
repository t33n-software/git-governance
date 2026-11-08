package bootstrap

import (
	"context"
	"io"
	"os"

	"github.com/t33n-software/git-governance/internal/adapters/report"
	"github.com/t33n-software/git-governance/internal/application/cliparam"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// repository resolves the repository identity of the selected working context
// and binds the selected remote name into it.
func (application *application) repository(ctx context.Context) (port.RepositoryIdentity, error) {
	identity, err := application.services().git.Discover(ctx, application.options.repository)
	if err != nil {
		return port.RepositoryIdentity{}, err
	}
	identity.Remote = application.options.remote
	return identity, nil
}

// reporter constructs the report surface for the resolved output format,
// quiet mode, and color decision.
func (application *application) reporter(writer io.Writer) port.Reporter {
	if writer == nil {
		writer = os.Stdout
	}
	format, _ := application.outputFormat()
	return report.New(report.Options{
		Writer: writer,
		Format: format,
		Quiet:  application.options.quiet,
		Color:  application.colorEnabled(),
	})
}

// outputFormat resolves the global output mode fail-closed from the canonical
// value-domain register.
func (application *application) outputFormat() (report.Format, error) {
	switch application.options.output {
	case "", "human":
		return report.FormatHuman, nil
	case "json":
		return report.FormatJSON, nil
	default:
		return "", problem.New(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryUsage,
			Field:       "output",
			Actual:      application.options.output,
			Expected:    cliparam.OutputMode().ValueList(),
			Rule:        "output format must be explicit and stable",
			Example:     "--output json",
			Remediation: "select human or json output",
		})
	}
}
