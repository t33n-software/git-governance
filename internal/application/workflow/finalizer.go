package workflow

import (
	"context"
	"path/filepath"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// runGenesisFinalizer is the read-only closing proof of the birth: the born
// refs exist and satisfy the governed validation, both point at the genesis
// revision, the genesis commit signature verifies, and the hook boundary is
// materialized.
func (service *BootstrapService) runGenesisFinalizer(
	ctx context.Context,
	repository port.RepositoryIdentity,
	capabilities bootstrapCapabilities,
	revision string,
	installation port.HookInstallation,
) error {
	for _, line := range []branch.BranchName{mustMain(), mustDevelop()} {
		if _, err := service.branches.Validate(ctx, branchapp.ValidateRequest{
			Repository: repository,
			Name:       line,
		}); err != nil {
			return err
		}
		exists, err := service.git.BranchExists(ctx, repository, line)
		if err != nil {
			return err
		}
		if !exists {
			return genesisFinalizerProblem("the born ref " + line.String() + " does not exist")
		}
		lineRevision, err := capabilities.revisions.ResolveRevision(ctx, repository, line.String())
		if err != nil {
			return err
		}
		if lineRevision != revision {
			return genesisFinalizerProblem(
				"the born ref " + line.String() + " points at " + lineRevision + " instead of the genesis revision " + revision,
			)
		}
	}
	if err := capabilities.verifier.VerifyCommitSignature(ctx, repository, revision); err != nil {
		return err
	}
	for _, hook := range installation.Hooks {
		exists, err := service.tools.FileExists(filepath.Join(installation.Directory, hook))
		if err != nil {
			return problem.Wrap(problem.Details{
				Code:        problem.CodeExternalCommandFailed,
				Category:    problem.CategoryExternal,
				Field:       "hook boundary",
				Actual:      hook,
				Expected:    "a readable installed hook file",
				Rule:        "the genesis finalizer proves the installed hook boundary",
				Remediation: "repair the hook installation and retry the bootstrap",
			}, err)
		}
		if !exists {
			return genesisFinalizerProblem("the installed hook " + hook + " is missing in " + installation.Directory)
		}
	}
	return nil
}
