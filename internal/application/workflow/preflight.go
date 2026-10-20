package workflow

import (
	"context"
	"path/filepath"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/genesis"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// runBootstrapPreflight proves every birth precondition without mutating
// anything: the unborn state, the unborn HEAD targeting main, the idle Git
// state, the empty index, the environment, the signing gate with its canary,
// the hook-manager resolution, the policy snapshot, the ticket key policy, and
// the content boundary of the explicit content set.
func (service *BootstrapService) runBootstrapPreflight(
	ctx context.Context,
	repository port.RepositoryIdentity,
	request BootstrapRequest,
	capabilities bootstrapCapabilities,
) (BootstrapPreflight, error) {
	hasCommits, err := service.git.HasCommits(ctx, repository)
	if err != nil {
		return BootstrapPreflight{}, err
	}
	if hasCommits {
		return BootstrapPreflight{}, repositoryAlreadyBorn("the repository HEAD already carries a commit")
	}
	anyRef, err := capabilities.refs.HasAnyRef(ctx, repository)
	if err != nil {
		return BootstrapPreflight{}, err
	}
	if anyRef {
		return BootstrapPreflight{}, repositoryAlreadyBorn("the repository already carries references")
	}
	current, err := service.git.CurrentBranch(ctx, repository)
	if err != nil {
		return BootstrapPreflight{}, err
	}
	if current.Family() != branch.FamilyMain {
		return BootstrapPreflight{}, unbornHeadMismatch(current)
	}
	operation, active, err := service.git.ActiveOperation(ctx, repository)
	if err != nil {
		return BootstrapPreflight{}, err
	}
	if active {
		return BootstrapPreflight{}, problem.New(problem.Details{
			Code:        problem.CodeOperationInProgress,
			Category:    problem.CategoryRepository,
			Field:       "git operation",
			Actual:      operation,
			Expected:    "no merge, rebase, or cherry-pick in progress",
			Rule:        "a running Git operation blocks the governed repository birth",
			Remediation: "complete or abort the active operation and retry the bootstrap",
		})
	}
	staged, err := service.git.HasStagedChanges(ctx, repository)
	if err != nil {
		return BootstrapPreflight{}, err
	}
	if staged {
		return BootstrapPreflight{}, problem.New(problem.Details{
			Code:        problem.CodeWorktreeNotClean,
			Category:    problem.CategoryRepository,
			Field:       "index",
			Expected:    "an empty index before the governed birth",
			Rule:        "the governed genesis stages the content set explicitly; a pre-staged index would bypass the content boundary scan",
			Remediation: "unstage the index and pass the content set through --stage",
		})
	}
	version, err := service.git.Version(ctx)
	if err != nil {
		return BootstrapPreflight{}, err
	}
	configuration, err := capabilities.signer.SigningConfiguration(ctx, repository)
	if err != nil {
		return BootstrapPreflight{}, err
	}
	if err := service.signingReadiness(configuration); err != nil {
		return BootstrapPreflight{}, err
	}
	if configuration.UserName == "" {
		return BootstrapPreflight{}, problem.New(problem.Details{
			Code:        problem.CodeConfigurationUnavailable,
			Category:    problem.CategoryConfig,
			Field:       "user.name",
			Expected:    "a configured user.name",
			Rule:        "the governed genesis requires the committer identity before the mutation",
			Remediation: "configure user.name for the committer identity and retry the bootstrap",
		})
	}
	if err := capabilities.signer.ProveSigningCapability(ctx, repository, configuration); err != nil {
		return BootstrapPreflight{}, err
	}
	hookManager, projectResolved, err := port.ResolveToolVersion(ctx, service.tools, repository.Root, port.HookManagerToolName)
	if err != nil {
		return BootstrapPreflight{}, problem.Wrap(problem.Details{
			Code:        problem.CodeExternalCommandFailed,
			Category:    problem.CategoryExternal,
			Field:       "hook manager",
			Expected:    "the repository-declared lefthook tool or an approved host installation",
			Rule:        "the governed repository birth installs the canonical hook boundary through the resolved lefthook invocation",
			Remediation: "declare lefthook as a Go tool in the project module or install the approved binary, then retry the bootstrap",
		}, err)
	}
	hookInvocation := port.HookManagerInvocation{Executable: port.HookManagerToolName}
	if projectResolved {
		hookInvocation = port.HookManagerInvocation{Executable: "go", Arguments: []string{"tool", port.HookManagerToolName}}
	}
	hookConfig, err := service.tools.FileExists(filepath.Join(repository.Root, "lefthook.yml"))
	if err != nil {
		return BootstrapPreflight{}, problem.Wrap(problem.Details{
			Code:        problem.CodeExternalCommandFailed,
			Category:    problem.CategoryExternal,
			Field:       "lefthook.yml",
			Expected:    "a readable hook configuration path",
			Rule:        "the governed repository birth must resolve the hook configuration state",
			Remediation: "repair the repository filesystem and retry the bootstrap",
		}, err)
	}
	if !hookConfig {
		return BootstrapPreflight{}, problem.New(problem.Details{
			Code:        problem.CodeConfigurationUnavailable,
			Category:    problem.CategoryConfig,
			Field:       "lefthook.yml",
			Expected:    "the repository's lefthook.yml hook configuration",
			Rule:        "the governed repository birth requires the canonical hook configuration before the mutation",
			Remediation: "add the canonical commit-msg and pre-push hooks through lefthook.yml and retry the bootstrap",
		})
	}
	snapshot := service.policySnapshot()
	if service.keyPolicy != nil {
		if err := service.keyPolicy.ValidateKey(ctx, repository, request.Ticket.Key()); err != nil {
			return BootstrapPreflight{}, err
		}
	}
	if len(request.StagePaths) == 0 {
		return BootstrapPreflight{}, emptyContentSet()
	}
	preview, err := capabilities.previewer.PreviewStage(ctx, repository, request.StagePaths)
	if err != nil {
		return BootstrapPreflight{}, err
	}
	if len(preview) == 0 {
		return BootstrapPreflight{}, emptyContentSet()
	}
	if err := genesis.ScanContentSet(preview); err != nil {
		return BootstrapPreflight{}, err
	}
	if request.Push {
		if _, err := service.git.RemoteURL(ctx, repository); err != nil {
			return BootstrapPreflight{}, problem.New(problem.Details{
				Code:        problem.CodeConfigurationUnavailable,
				Category:    problem.CategoryConfig,
				Field:       "remote",
				Actual:      repository.Remote,
				Expected:    "a bound remote for the publication of the born shared lines",
				Rule:        "publication requires a bound remote; the local genesis completes without it only when --push is not requested",
				Remediation: "bind the remote first (git remote add " + repository.Remote + " <url>) and retry with --push",
			})
		}
	}
	return BootstrapPreflight{
		ContentFiles:   len(preview),
		PolicySnapshot: snapshot,
		Actor:          configuration.UserEmail,
		GitVersion:     version,
		HookManager:    hookManager,
		HookInvocation: hookInvocation,
	}, nil
}
