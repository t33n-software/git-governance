package policy

import (
	"context"
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
