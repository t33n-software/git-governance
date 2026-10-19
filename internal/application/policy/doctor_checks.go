package policy

import (
	"context"
	"path/filepath"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

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
	version, _, err := port.ResolveToolVersion(ctx, tools, result.Repository.Root, port.HookManagerToolName)
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

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
