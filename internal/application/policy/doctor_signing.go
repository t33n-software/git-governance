package policy

import (
	"context"
	"strings"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// appendSigningChecks proves the effective commit-signing configuration, the
// canary signature against the allowed-signers surface, and the lane
// machine-identity injection when a lane context is detected. Every failure is
// fail-closed: a missing precondition must surface locally, not at the remote
// boundary.
func (result *DoctorResult) appendSigningChecks(
	ctx context.Context,
	git port.GitRepository,
	repository port.RepositoryIdentity,
) {
	inspector, ok := git.(port.GitSigningInspector)
	if !ok {
		result.appendSigningFailure(problem.New(problem.Details{
			Code:        problem.CodeConfigurationUnavailable,
			Category:    problem.CategoryConfig,
			Field:       "Commit signing configuration",
			Expected:    "a commit-signing inspector",
			Rule:        "doctor proves commit-signing readiness before governed work",
			Remediation: "repair the Git runtime composition and retry doctor",
		}))
		result.Checks = append(result.Checks, Check{
			Name:   "Commit signing proof",
			OK:     false,
			Detail: "commit-signing inspector is not configured",
		})
		return
	}

	configuration, err := inspector.SigningConfiguration(ctx, repository)
	if err != nil {
		result.appendSigningFailure(err)
		result.Checks = append(result.Checks, Check{
			Name:   "Commit signing proof",
			OK:     false,
			Detail: "the signing proof requires a readable signing configuration",
		})
		return
	}

	if err := SigningConfigurationProblem(configuration); err != nil {
		result.Checks = append(result.Checks, Check{
			Name:   "Commit signing configuration",
			OK:     false,
			Detail: err.Error(),
		})
		result.signingError = err
		result.Checks = append(result.Checks, Check{
			Name:   "Commit signing proof",
			OK:     false,
			Detail: "the signing proof requires a complete signing configuration",
		})
	} else {
		result.Checks = append(result.Checks, Check{
			Name:   "Commit signing configuration",
			OK:     true,
			Detail: "commit.gpgsign is enabled with the ssh format; the signing key and the allowed-signers file are readable",
		})
		if err := inspector.ProveSigningCapability(ctx, repository, configuration); err != nil {
			result.Checks = append(result.Checks, Check{
				Name:   "Commit signing proof",
				OK:     false,
				Detail: err.Error(),
			})
			result.signingError = err
		} else {
			result.Checks = append(result.Checks, Check{
				Name:   "Commit signing proof",
				OK:     true,
				Detail: "a canary payload was signed and verified against the allowed-signers file",
			})
		}
	}

	if configuration.LaneInjection {
		result.appendLaneSigningCheck(configuration)
	}
}

func (result *DoctorResult) appendLaneSigningCheck(configuration port.SigningConfiguration) {
	missing := make([]string, 0, 5)
	for _, required := range []string{"commit.gpgsign", "gpg.format", "user.signingkey", "user.name", "user.email"} {
		if !containsString(configuration.InjectedSigningKeys, required) {
			missing = append(missing, required)
		}
	}
	if len(missing) > 0 {
		err := problem.New(problem.Details{
			Code:        problem.CodeConfigurationUnavailable,
			Category:    problem.CategoryConfig,
			Field:       "Lane signing identity",
			Actual:      "missing " + strings.Join(missing, ", "),
			Expected:    "the machine-identity signing keys in the lane configuration injection",
			Rule:        "a commit-producing lane injects commit.gpgsign, gpg.format, user.signingkey, user.name, and user.email for its machine identity",
			Remediation: "inject the complete machine-identity signing configuration into the lane and retry doctor",
		})
		result.Checks = append(result.Checks, Check{
			Name:   "Lane signing identity",
			OK:     false,
			Detail: err.Error(),
		})
		result.signingError = err
		return
	}
	result.Checks = append(result.Checks, Check{
		Name:   "Lane signing identity",
		OK:     true,
		Detail: "the lane injection carries the machine-identity signing configuration",
	})
}

func (result *DoctorResult) appendSigningFailure(err error) {
	if _, classified := problem.As(err); !classified {
		err = problem.Wrap(problem.Details{
			Code:        problem.CodeConfigurationUnavailable,
			Category:    problem.CategoryConfig,
			Field:       "Commit signing configuration",
			Expected:    "a readable commit-signing configuration",
			Rule:        "doctor proves commit-signing readiness before governed work",
			Remediation: "repair the Git runtime composition and retry doctor",
		}, err)
	}
	result.Checks = append(result.Checks, Check{
		Name:   "Commit signing configuration",
		OK:     false,
		Detail: err.Error(),
	})
	result.signingError = err
}

// SigningConfigurationProblem evaluates the effective signing facts against
// the governed baseline in a fixed order and reports the first violation. It
// is the single source of truth for signing readiness: doctor diagnostics and
// the governed repository birth consume the same evaluation.
func SigningConfigurationProblem(configuration port.SigningConfiguration) error {
	if !configuration.SigningEnabled {
		return problem.New(problem.Details{
			Code:        problem.CodeConfigurationUnavailable,
			Category:    problem.CategoryConfig,
			Field:       "commit.gpgsign",
			Expected:    "commit.gpgsign enabled",
			Rule:        "the governed baseline requires commit.gpgsign=true so every commit carries a signature",
			Remediation: "enable commit signing (git config --global commit.gpgsign true) and retry doctor",
		})
	}
	if configuration.Format != "ssh" {
		return problem.New(problem.Details{
			Code:        problem.CodeConfigurationInvalid,
			Category:    problem.CategoryConfig,
			Field:       "gpg.format",
			Actual:      configuration.Format,
			Expected:    "ssh",
			Rule:        "the governed baseline signs commits with SSH keys",
			Remediation: "set gpg.format to ssh and retry doctor",
		})
	}
	if configuration.SigningKey == "" {
		return problem.New(problem.Details{
			Code:        problem.CodeConfigurationUnavailable,
			Category:    problem.CategoryConfig,
			Field:       "user.signingkey",
			Expected:    "a configured signing key",
			Rule:        "commit signing requires a dedicated configured signing key",
			Remediation: "configure user.signingkey to the dedicated signing key and retry doctor",
		})
	}
	if !configuration.SigningKeyReadable {
		return problem.New(problem.Details{
			Code:        problem.CodeConfigurationInvalid,
			Category:    problem.CategoryConfig,
			Field:       "user.signingkey",
			Actual:      configuration.SigningKey,
			Expected:    "a readable signing key file",
			Rule:        "the configured signing key must be a readable file",
			Remediation: "point user.signingkey at the readable private key file and retry doctor",
		})
	}
	if configuration.UserEmail == "" {
		return problem.New(problem.Details{
			Code:        problem.CodeConfigurationUnavailable,
			Category:    problem.CategoryConfig,
			Field:       "user.email",
			Expected:    "a configured user.email",
			Rule:        "the commit-signing identity requires user.email as the verification principal",
			Remediation: "configure user.email to the signing identity address and retry doctor",
		})
	}
	if configuration.AllowedSignersFile == "" {
		return problem.New(problem.Details{
			Code:        problem.CodeConfigurationUnavailable,
			Category:    problem.CategoryConfig,
			Field:       "gpg.ssh.allowedSignersFile",
			Expected:    "a configured allowed-signers file",
			Rule:        "signature verification requires an allowed-signers file",
			Remediation: "configure gpg.ssh.allowedSignersFile and retry doctor",
		})
	}
	if !configuration.AllowedSignersReadable {
		return problem.New(problem.Details{
			Code:        problem.CodeConfigurationInvalid,
			Category:    problem.CategoryConfig,
			Field:       "gpg.ssh.allowedSignersFile",
			Actual:      configuration.AllowedSignersFile,
			Expected:    "a readable allowed-signers file",
			Rule:        "the configured allowed-signers file must be a readable file",
			Remediation: "point gpg.ssh.allowedSignersFile at the readable allowed-signers file and retry doctor",
		})
	}
	return nil
}
