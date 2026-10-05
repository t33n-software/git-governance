package gitcli

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// canonicalHookSet is the organization-fixed local hook boundary installed at
// the governed repository birth: the commit-message gate and the pre-push
// gate.
var canonicalHookSet = []string{"commit-msg", "pre-push"}

// PreviewStage resolves the effective initial content set of explicit stage
// paths without mutating the index. On an unborn repository every content file
// is untracked, so the untracked-file listing with standard exclusions matches
// exactly what a later stage operation would add.
func (repository *Repository) PreviewStage(
	ctx context.Context,
	identity port.RepositoryIdentity,
	paths []string,
) ([]string, error) {
	if len(paths) == 0 {
		return nil, problem.New(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryUsage,
			Field:       "stage paths",
			Expected:    "at least one explicit path",
			Rule:        "the CLI never stages all files implicitly",
			Example:     "--stage README.md",
			Remediation: "supply each path to stage explicitly",
		})
	}
	arguments := make([]string, 0, len(paths)+5)
	arguments = append(arguments, "ls-files", "--others", "--exclude-standard", "-z", "--")
	arguments = append(arguments, paths...)
	result := repository.invoke(ctx, identity.Root, nil, arguments...)
	if result.err != nil {
		return nil, repository.commandProblem(problem.CodeGitCommandFailed, identity, "preview the stage paths", result)
	}
	entries := strings.Split(result.stdout, "\x00")
	preview := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry != "" {
			preview = append(preview, entry)
		}
	}
	return preview, nil
}

// HasAnyRef proves whether the repository carries any Git reference at all.
// An unborn repository has no references; the check covers every reference
// namespace, not only branches.
func (repository *Repository) HasAnyRef(ctx context.Context, identity port.RepositoryIdentity) (bool, error) {
	result := repository.invoke(ctx, identity.Root, nil, "for-each-ref")
	if result.err != nil {
		return false, repository.commandProblem(problem.CodeGitCommandFailed, identity, "enumerate the repository references", result)
	}
	return strings.TrimSpace(result.stdout) != "", nil
}

// RestoreUnbornState restores the proven pre-birth state of an unborn
// repository after an aborted genesis step: the index returns to the empty
// tree and every existing reference is deleted, which leaves the symbolic
// unborn HEAD untouched. The working tree is never modified, so foreign
// working-tree changes survive the compensation unchanged.
func (repository *Repository) RestoreUnbornState(ctx context.Context, identity port.RepositoryIdentity) error {
	result := repository.invoke(ctx, identity.Root, nil, "read-tree", "--empty")
	if result.err != nil {
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "empty the index for the genesis compensation", result)
	}
	result = repository.invoke(ctx, identity.Root, nil, "for-each-ref", "--format=%(refname)")
	if result.err != nil {
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "enumerate the born references for the genesis compensation", result)
	}
	for _, raw := range strings.Split(strings.TrimSpace(result.stdout), "\n") {
		ref := strings.TrimSpace(raw)
		if ref == "" {
			continue
		}
		deletion := repository.invoke(ctx, identity.Root, nil, "update-ref", "-d", ref)
		if deletion.err != nil {
			return repository.commandProblem(problem.CodeGitCommandFailed, identity, "delete the born reference "+ref+" for the genesis compensation", deletion)
		}
	}
	return nil
}

// CountHeadCommits counts the commits reachable from HEAD. The caller proves
// the born state before counting: an unborn HEAD carries no revision for Git
// to walk, so the underlying failure stays a Git-command failure.
func (repository *Repository) CountHeadCommits(ctx context.Context, identity port.RepositoryIdentity) (int, error) {
	result := repository.invoke(ctx, identity.Root, nil, "rev-list", "--count", "HEAD")
	if result.err != nil {
		return 0, repository.commandProblem(problem.CodeGitCommandFailed, identity, "count the commits reachable from HEAD", result)
	}
	count, err := strconv.Atoi(strings.TrimSpace(result.stdout))
	if err != nil {
		return 0, problem.New(problem.Details{
			Code:        problem.CodeGitCommandFailed,
			Category:    problem.CategoryGit,
			Field:       "commit count",
			Actual:      strings.TrimSpace(result.stdout),
			Expected:    "a decimal commit count",
			Rule:        "the birth-topology proof requires the commit count of HEAD",
			Remediation: "repair the repository history and retry the publication resume",
		})
	}
	return count, nil
}

// HookBoundaryPresent proves that the canonical hook boundary is materialized
// in the repository's Git hooks directory. The birth finalizer installs the
// boundary; the publication resume re-proves it before the shared-line push
// without reinstalling it. Any unreadable hook file reports the boundary as
// absent, so the resume fails closed.
func (repository *Repository) HookBoundaryPresent(ctx context.Context, identity port.RepositoryIdentity) (bool, error) {
	gitDirectory, err := repository.gitDirectory(ctx, identity)
	if err != nil {
		return false, err
	}
	for _, hook := range canonicalHookSet {
		if _, statErr := os.Stat(filepath.Join(gitDirectory, "hooks", hook)); statErr != nil {
			return false, nil
		}
	}
	return true, nil
}

// VerifyCommitSignature proves that the referenced commit carries a valid,
// trusted signature. The check is fail-closed because the active policy
// requires signed commits.
func (repository *Repository) VerifyCommitSignature(
	ctx context.Context,
	identity port.RepositoryIdentity,
	revision string,
) error {
	if strings.TrimSpace(revision) == "" {
		return problem.New(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryUsage,
			Field:       "revision",
			Expected:    "a non-empty commit revision",
			Rule:        "signature verification requires the exact commit object to verify",
			Remediation: "supply the revision of the commit to verify",
		})
	}
	result := repository.invoke(ctx, identity.Root, nil, "verify-commit", revision)
	if result.err != nil {
		return problem.Wrap(problem.Details{
			Code:        problem.CodeCommitSignatureRequired,
			Category:    problem.CategoryGovernance,
			Field:       "commit signature",
			Context:     strings.Join(commandSummary(identity, "verify the commit signature of "+revision), " "),
			Diagnostic:  commandDiagnostic(result),
			Expected:    "a commit with a valid, trusted signature",
			Rule:        "the active policy requires signed commits; the referenced commit must verify against the configured signing identity",
			Remediation: "configure commit signing (commit.gpgsign, gpg.format, user.signingkey, gpg.ssh.allowedSignersFile), prove it with doctor, and recreate the commit",
		}, commandCause(result))
	}
	return nil
}

// InstallHooks installs the canonical local hook boundary through the
// repository's hook manager and proves the installed hook files. The proof is
// fail-closed: an installation that does not materialize the canonical hook
// set is a failed birth step.
func (repository *Repository) InstallHooks(
	ctx context.Context,
	identity port.RepositoryIdentity,
) (port.HookInstallation, error) {
	gitDirectory, err := repository.gitDirectory(ctx, identity)
	if err != nil {
		return port.HookInstallation{}, err
	}
	result := repository.invokeProgram(ctx, identity.Root, "lefthook", nil, "install")
	if result.err != nil {
		return port.HookInstallation{}, problem.Wrap(problem.Details{
			Code:        problem.CodeExternalCommandFailed,
			Category:    problem.CategoryExternal,
			Field:       "hook boundary",
			Context:     strings.Join(commandSummary(identity, "install the hook boundary"), " "),
			Diagnostic:  commandDiagnostic(result),
			Expected:    "a successful lefthook install in the repository",
			Rule:        "the governed repository birth installs the canonical hook boundary so every subsequent operation is governed",
			Remediation: "repair the lefthook installation and retry the bootstrap",
		}, commandCause(result))
	}
	hooksDirectory := filepath.Join(gitDirectory, "hooks")
	for _, hook := range canonicalHookSet {
		path := filepath.Join(hooksDirectory, hook)
		if _, statErr := os.Stat(path); statErr != nil {
			return port.HookInstallation{}, problem.Wrap(problem.Details{
				Code:        problem.CodeExternalCommandFailed,
				Category:    problem.CategoryExternal,
				Field:       "hook boundary",
				Actual:      hook,
				Expected:    "the canonical hook set materialized in " + hooksDirectory,
				Rule:        "the hook boundary proof requires the canonical hook set after installation",
				Remediation: "declare the canonical commit-msg and pre-push hooks in lefthook.yml and retry the bootstrap",
			}, commandCause(result))
		}
	}
	return port.HookInstallation{
		Directory: hooksDirectory,
		Hooks:     append([]string(nil), canonicalHookSet...),
	}, nil
}

// gitDirectory resolves the repository's Git directory, which is not always
// `<root>/.git` (worktrees and separated git directories relocate it).
func (repository *Repository) gitDirectory(
	ctx context.Context,
	identity port.RepositoryIdentity,
) (string, error) {
	result := repository.invoke(ctx, identity.Root, nil, "rev-parse", "--git-dir")
	if result.err != nil {
		return "", repository.commandProblem(problem.CodeGitCommandFailed, identity, "resolve the Git directory", result)
	}
	directory := strings.TrimSpace(result.stdout)
	if directory == "" {
		return "", problem.New(problem.Details{
			Code:        problem.CodeGitCommandFailed,
			Category:    problem.CategoryGit,
			Field:       "Git directory",
			Expected:    "a resolved Git directory path",
			Rule:        "the hook boundary proof requires the repository's Git directory",
			Remediation: "repair the repository metadata and retry the bootstrap",
		})
	}
	if filepath.IsAbs(directory) {
		return directory, nil
	}
	return filepath.Join(identity.Root, directory), nil
}

var _ port.StagePreviewer = (*Repository)(nil)
var _ port.RefExistenceInspector = (*Repository)(nil)
var _ port.CommitSignatureVerifier = (*Repository)(nil)
var _ port.HookInstaller = (*Repository)(nil)
var _ port.UnbornStateRestorer = (*Repository)(nil)
var _ port.HeadCommitCounter = (*Repository)(nil)
var _ port.HookBoundaryInspector = (*Repository)(nil)
