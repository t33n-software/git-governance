package policy

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// DoctorService performs read-only local diagnostics.
type DoctorService struct {
	git    port.GitRepository
	store  port.PreferencesStore
	policy port.PolicyInspector
	tools  port.ToolInspector
}

// NewDoctorService creates a read-only diagnostics service.
func NewDoctorService(git port.GitRepository, store port.PreferencesStore) *DoctorService {
	return NewDoctorServiceWithDependencies(git, store, nil, nil)
}

// NewDoctorServiceWithDependencies creates a diagnostics service with optional
// policy and host-tool inspectors.
func NewDoctorServiceWithDependencies(
	git port.GitRepository,
	store port.PreferencesStore,
	policy port.PolicyInspector,
	tools port.ToolInspector,
) *DoctorService {
	return &DoctorService{
		git:    git,
		store:  store,
		policy: policy,
		tools:  tools,
	}
}

// Check is one diagnostic outcome.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// DoctorResult is a non-mutating environment snapshot.
type DoctorResult struct {
	Repository          port.RepositoryIdentity `json:"repository"`
	Checks              []Check                 `json:"checks"`
	authenticationError error
	signingError        error
}

// Run checks the repository and configuration without installing, repairing,
// fetching, or otherwise mutating anything.
func (service *DoctorService) Run(ctx context.Context, directory string) (DoctorResult, error) {
	return service.RunForRemote(ctx, directory, "")
}

// RunForRemote checks the repository using an explicit selected Git remote
// when one was provided through the CLI's global remote option.
func (service *DoctorService) RunForRemote(
	ctx context.Context,
	directory string,
	remote string,
) (DoctorResult, error) {
	if ctx != nil && ctx.Err() != nil {
		return DoctorResult{}, problem.Wrap(problem.Details{
			Code:        problem.CodeOperationCancelled,
			Category:    problem.CategoryCancelled,
			Field:       "doctor",
			Expected:    "an active context",
			Rule:        "doctor diagnostics stop when the caller cancels the command",
			Remediation: "retry with an active context",
		}, ctx.Err())
	}
	result := DoctorResult{Checks: make([]Check, 0, 14)}
	if service.git == nil {
		result.Checks = append(result.Checks, Check{
			Name:   "git repository",
			OK:     false,
			Detail: "Git adapter is not configured",
		})
		result.appendGitAuthenticationFailure(problem.New(problem.Details{
			Code:        problem.CodeConfigurationUnavailable,
			Category:    problem.CategoryConfig,
			Field:       "Git authentication",
			Expected:    "a configured Git transport authentication inspector",
			Rule:        "doctor requires Git transport authentication diagnostics",
			Remediation: "repair the Git runtime composition and retry doctor",
		}))
		result.appendSigningFailure(problem.New(problem.Details{
			Code:        problem.CodeConfigurationUnavailable,
			Category:    problem.CategoryConfig,
			Field:       "Commit signing configuration",
			Expected:    "a configured commit-signing inspector",
			Rule:        "doctor requires commit-signing diagnostics",
			Remediation: "repair the Git runtime composition and retry doctor",
		}))
		result.Checks = append(result.Checks, Check{
			Name:   "Commit signing proof",
			OK:     false,
			Detail: "commit-signing inspector is not configured",
		})
	} else {
		version, err := service.git.Version(ctx)
		if err != nil {
			result.Checks = append(result.Checks, Check{
				Name:   "Git version",
				OK:     false,
				Detail: err.Error(),
			})
		} else {
			result.Checks = append(result.Checks, Check{
				Name:   "Git version",
				OK:     true,
				Detail: version,
			})
		}

		repository, err := service.git.Discover(ctx, directory)
		if err != nil {
			result.Checks = append(result.Checks, Check{
				Name:   "git repository",
				OK:     false,
				Detail: err.Error(),
			})
		} else {
			if strings.TrimSpace(remote) != "" {
				repository.Remote = remote
			}
			result.Repository = repository
			result.appendRepositoryChecks(ctx, service.git, repository)
		}
	}

	result.appendToolChecks(ctx, service.tools)
	result.appendPolicyCheck(ctx, service.policy)
	result.appendConfigurationCheck(ctx, service.store)
	return result, nil
}

// AuthenticationError returns the fail-closed Git transport readiness error,
// if doctor could not verify an authenticated dry-run push.
func (result DoctorResult) AuthenticationError() error {
	return result.authenticationError
}

// SigningError returns the fail-closed commit-signing readiness error, if
// doctor could not prove the signing configuration, the canary signature, or
// the lane machine-identity injection.
func (result DoctorResult) SigningError() error {
	return result.signingError
}

func (result *DoctorResult) appendRepositoryChecks(ctx context.Context, git port.GitRepository, repository port.RepositoryIdentity) {
	result.Checks = append(result.Checks, Check{
		Name:   "git repository",
		OK:     true,
		Detail: repository.Root,
	})

	hasCommits, commitErr := git.HasCommits(ctx, repository)
	if commitErr != nil {
		result.Checks = append(result.Checks, Check{
			Name:   "repository history",
			OK:     false,
			Detail: commitErr.Error(),
		})
	} else {
		detail := "repository has at least one commit"
		if !hasCommits {
			detail = "repository has no commits; branch creation is unavailable"
		}
		result.Checks = append(result.Checks, Check{
			Name:   "repository history",
			OK:     hasCommits,
			Detail: detail,
		})
	}

	if _, err := git.RemoteURL(ctx, repository); err != nil {
		result.Checks = append(result.Checks, Check{
			Name:   "selected remote",
			OK:     false,
			Detail: err.Error(),
		})
	} else {
		result.Checks = append(result.Checks, Check{
			Name:   "selected remote",
			OK:     true,
			Detail: repository.Remote + " is configured",
		})
	}

	operation, active, err := git.ActiveOperation(ctx, repository)
	if err != nil {
		result.Checks = append(result.Checks, Check{
			Name:   "Git operation state",
			OK:     false,
			Detail: err.Error(),
		})
	} else if active {
		result.Checks = append(result.Checks, Check{
			Name:   "Git operation state",
			OK:     false,
			Detail: operation + " is in progress; complete or abort it before governed mutations",
		})
	} else {
		result.Checks = append(result.Checks, Check{
			Name:   "Git operation state",
			OK:     true,
			Detail: "no merge, rebase, or cherry-pick is in progress",
		})
	}

	result.appendGitAuthenticationCheck(ctx, git, repository)
	result.appendSigningChecks(ctx, git, repository)
}

func (result *DoctorResult) appendGitAuthenticationCheck(
	ctx context.Context,
	git port.GitRepository,
	repository port.RepositoryIdentity,
) {
	authenticator, ok := git.(port.GitTransportAuthenticator)
	if !ok {
		result.appendGitAuthenticationFailure(problem.New(problem.Details{
			Code:        problem.CodeConfigurationUnavailable,
			Category:    problem.CategoryConfig,
			Field:       "Git authentication",
			Expected:    "a Git transport authentication inspector",
			Rule:        "doctor verifies Git transport authentication before governed work",
			Remediation: "repair the Git adapter and retry doctor",
		}))
		return
	}
	if err := authenticator.CheckTransportAuthentication(ctx, repository); err != nil {
		result.appendGitAuthenticationFailure(err)
		return
	}
	result.Checks = append(result.Checks, Check{
		Name:   "Git authentication",
		OK:     true,
		Detail: "authenticated dry-run push succeeded without an interactive prompt",
	})
}

func (result *DoctorResult) appendGitAuthenticationFailure(err error) {
	if _, classified := problem.As(err); !classified {
		err = problem.Wrap(problem.Details{
			Code:        problem.CodeGitCommandFailed,
			Category:    problem.CategoryGit,
			Field:       "Git authentication",
			Expected:    "a non-interactive credential capable of an authenticated dry-run push",
			Rule:        "doctor verifies Git transport authentication without mutating remote references",
			Remediation: "authenticate Git transport with SSH or Git Credential Manager and retry doctor",
		}, err)
	}
	detail := "Git transport authentication could not be verified"
	if err != nil {
		detail = err.Error()
	}
	result.Checks = append(result.Checks, Check{
		Name:   "Git authentication",
		OK:     false,
		Detail: detail,
	})
	result.authenticationError = err
}

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

	if err := signingConfigurationProblem(configuration); err != nil {
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

// signingConfigurationProblem evaluates the effective signing facts against
// the governed baseline in a fixed order and reports the first violation.
func signingConfigurationProblem(configuration port.SigningConfiguration) error {
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

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func (result *DoctorResult) appendToolChecks(ctx context.Context, tools port.ToolInspector) {
	if tools == nil {
		result.Checks = append(result.Checks, Check{
			Name:   "runtime platform",
			OK:     false,
			Detail: "tool inspector is not configured",
		})
		result.Checks = append(result.Checks, Check{
			Name:   "Lefthook executable",
			OK:     false,
			Detail: "tool inspector is not configured",
		})
		result.Checks = append(result.Checks, Check{
			Name:   "Lefthook configuration",
			OK:     false,
			Detail: "tool inspector is not configured",
		})
		return
	}
	operatingSystem, architecture := tools.Platform()
	result.Checks = append(result.Checks, Check{
		Name:   "runtime platform",
		OK:     operatingSystem != "" && architecture != "",
		Detail: operatingSystem + "/" + architecture,
	})
	version, err := tools.Version(ctx, "lefthook")
	if err != nil {
		result.Checks = append(result.Checks, Check{
			Name:   "Lefthook executable",
			OK:     false,
			Detail: err.Error(),
		})
	} else {
		result.Checks = append(result.Checks, Check{
			Name:   "Lefthook executable",
			OK:     true,
			Detail: version,
		})
	}
	if result.Repository.Root == "" {
		result.Checks = append(result.Checks, Check{
			Name:   "Lefthook configuration",
			OK:     false,
			Detail: "repository root is unavailable",
		})
		return
	}
	exists, err := tools.FileExists(filepath.Join(result.Repository.Root, "lefthook.yml"))
	if err != nil {
		result.Checks = append(result.Checks, Check{
			Name:   "Lefthook configuration",
			OK:     false,
			Detail: err.Error(),
		})
		return
	}
	result.Checks = append(result.Checks, Check{
		Name:   "Lefthook configuration",
		OK:     exists,
		Detail: lefthookConfigurationDetail(exists),
	})
}

func (result *DoctorResult) appendPolicyCheck(ctx context.Context, policy port.PolicyInspector) {
	if policy == nil {
		result.Checks = append(result.Checks, Check{
			Name:   "local policy",
			OK:     false,
			Detail: "policy inspector is not configured",
		})
		return
	}
	status, err := policy.Status(ctx, result.Repository)
	if err != nil {
		result.Checks = append(result.Checks, Check{
			Name:   "local policy",
			OK:     false,
			Detail: err.Error(),
		})
		return
	}
	result.Checks = append(result.Checks, Check{
		Name:   "local policy",
		OK:     true,
		Detail: status.Detail,
	})
}

func (result *DoctorResult) appendConfigurationCheck(ctx context.Context, store port.PreferencesStore) {
	if store == nil {
		result.Checks = append(result.Checks, Check{
			Name:   "user configuration",
			OK:     false,
			Detail: "preferences store is not configured",
		})
		return
	}
	preferences, err := store.Load(ctx)
	if err != nil {
		result.Checks = append(result.Checks, Check{
			Name:   "user configuration",
			OK:     false,
			Detail: err.Error(),
		})
		return
	}
	result.Checks = append(result.Checks, Check{
		Name:   "user configuration",
		OK:     true,
		Detail: configurationDetail(preferences),
	})
}

func configurationDetail(preferences port.Preferences) string {
	if preferences.DefaultKey == nil {
		return "configuration is readable; no default ticket key is set"
	}
	return "configuration is readable; default ticket key is " + preferences.DefaultKey.String()
}

func lefthookConfigurationDetail(exists bool) string {
	if exists {
		return "lefthook.yml is present"
	}
	return "lefthook.yml is not present"
}
