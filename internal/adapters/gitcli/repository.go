// Package gitcli adapts the installed Git executable to application-owned
// repository contracts. It never builds shell command strings.
package gitcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CyberT33N/git-governance/internal/application/port"
	"github.com/CyberT33N/git-governance/internal/domain/branch"
	"github.com/CyberT33N/git-governance/internal/domain/commitmsg"
	"github.com/CyberT33N/git-governance/internal/domain/problem"
	"github.com/CyberT33N/git-governance/internal/domain/ticket"
)

const (
	defaultTimeout                  = 30 * time.Second
	workflowBasesConfigKey          = "git-governance.workflow-bases"
	finalQualityEvidenceConfigKey   = "git-governance.final-quality-evidence"
	hotfixManifestProgressConfigKey = "git-governance.hotfix-manifest-progress"
	maxDiagnosticBytes              = 4096
)

var noPromptGitEnvironment = []string{
	"GIT_TERMINAL_PROMPT=0",
	"GCM_INTERACTIVE=Never",
	"SSH_ASKPASS_REQUIRE=never",
}

var (
	urlCredentialsPattern   = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^/\s@]+@`)
	secretAssignmentPattern = regexp.MustCompile(`(?i)\b(token|password|secret|authorization)=\S+`)
	commitObjectIDPattern   = regexp.MustCompile(`^(?:[0-9a-fA-F]{40}|[0-9a-fA-F]{64})$`)
)

// Options configures the Git process adapter.
type Options struct {
	Binary         string
	Timeout        time.Duration
	MaxOutputBytes int
}

// Repository executes the Git commands needed by the application layer.
type Repository struct {
	runner  processRunner
	timeout time.Duration
}

// New constructs a Git CLI adapter with safe defaults.
func New(options Options) *Repository {
	binary := options.Binary
	if binary == "" {
		binary = "git"
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Repository{
		runner: execRunner{
			binary:         binary,
			maxOutputBytes: options.MaxOutputBytes,
		},
		timeout: timeout,
	}
}

// Discover resolves a working directory to its Git top-level directory.
func (repository *Repository) Discover(ctx context.Context, directory string) (port.RepositoryIdentity, error) {
	result := repository.invoke(ctx, directory, nil, "rev-parse", "--show-toplevel")
	if result.err != nil {
		return port.RepositoryIdentity{}, problem.Wrap(problem.Details{
			Code:        problem.CodeRepositoryNotFound,
			Category:    problem.CategoryRepository,
			Field:       "repository",
			Actual:      directory,
			Expected:    "a directory inside a Git worktree",
			Rule:        "Git operations require a local repository",
			Example:     "C:\\work\\repository",
			Remediation: "run the command inside a Git repository or pass --repo",
		}, commandCause(result))
	}

	root := strings.TrimSpace(result.stdout)
	if root == "" {
		return port.RepositoryIdentity{}, problem.New(problem.Details{
			Code:        problem.CodeRepositoryNotFound,
			Category:    problem.CategoryRepository,
			Field:       "repository",
			Actual:      directory,
			Expected:    "a Git worktree with a top-level directory",
			Rule:        "Git must resolve the repository root",
			Example:     "C:\\work\\repository",
			Remediation: "run the command inside an initialized repository",
		})
	}

	return port.RepositoryIdentity{Root: root, Remote: "origin"}, nil
}

// Version returns the installed Git version without requiring a repository.
func (repository *Repository) Version(ctx context.Context) (string, error) {
	result := repository.invoke(ctx, "", nil, "--version")
	if result.err != nil {
		return "", repository.commandProblem(problem.CodeGitCommandFailed, port.RepositoryIdentity{}, "read the Git version", result)
	}
	version := strings.TrimSpace(result.stdout)
	if version == "" {
		return "", problem.New(problem.Details{
			Code:        problem.CodeGitCommandFailed,
			Category:    problem.CategoryGit,
			Field:       "Git version",
			Expected:    "a non-empty version string from git --version",
			Rule:        "doctor must identify the installed Git executable",
			Remediation: "install or repair Git and retry doctor",
		})
	}
	return version, nil
}

// RemoteURL returns the configured URL for the selected remote.
func (repository *Repository) RemoteURL(ctx context.Context, identity port.RepositoryIdentity) (string, error) {
	result := repository.invoke(ctx, identity.Root, nil, "remote", "get-url", identity.Remote)
	if result.err != nil {
		return "", repository.commandProblem(problem.CodeGitCommandFailed, identity, "read the selected remote URL", result)
	}
	url := strings.TrimSpace(result.stdout)
	if url == "" {
		return "", problem.New(problem.Details{
			Code:        problem.CodeGitCommandFailed,
			Category:    problem.CategoryRepository,
			Field:       "remote",
			Actual:      identity.Remote,
			Expected:    "a configured remote URL",
			Rule:        "the selected remote must resolve to a configured URL",
			Remediation: "configure the remote or select an existing --remote",
		})
	}
	return url, nil
}

// CheckTransportAuthentication verifies that the selected Git transport can
// authenticate and authorize a dry-run update of the currently checked-out
// branch. Git does not mutate refs for --dry-run, and the command disables
// terminal prompts so doctor remains non-interactive.
func (repository *Repository) CheckTransportAuthentication(
	ctx context.Context,
	identity port.RepositoryIdentity,
) error {
	current := repository.invokeNoPrompt(ctx, identity.Root, nil, "branch", "--show-current")
	if current.err != nil {
		return repository.gitAuthenticationProblem(identity, "identify the checked-out branch", current)
	}
	branchName := strings.TrimSpace(current.stdout)
	if branchName == "" {
		return problem.New(problem.Details{
			Code:        problem.CodeGitCommandFailed,
			Category:    problem.CategoryGit,
			Field:       "Git authentication",
			Expected:    "a checked-out branch for a dry-run authenticated push",
			Rule:        "doctor verifies Git transport authentication without mutating remote references",
			Remediation: "check out a branch, authenticate Git transport, and retry doctor",
		})
	}
	result := repository.invokeNoPrompt(
		ctx,
		identity.Root,
		nil,
		"push",
		"--dry-run",
		"--no-verify",
		"--porcelain",
		identity.Remote,
		"HEAD:refs/heads/"+branchName,
	)
	if result.err != nil {
		return repository.gitAuthenticationProblem(identity, "perform an authenticated dry-run push", result)
	}
	return nil
}

// ActiveOperation reports an in-progress rebase, merge, or cherry-pick without
// mutating Git state.
func (repository *Repository) ActiveOperation(ctx context.Context, identity port.RepositoryIdentity) (string, bool, error) {
	for _, candidate := range []struct {
		name string
		path string
	}{
		{name: "rebase", path: "rebase-merge"},
		{name: "rebase", path: "rebase-apply"},
		{name: "merge", path: "MERGE_HEAD"},
		{name: "cherry-pick", path: "CHERRY_PICK_HEAD"},
	} {
		result := repository.invoke(ctx, identity.Root, nil, "rev-parse", "--git-path", candidate.path)
		if result.err != nil {
			return "", false, repository.commandProblem(problem.CodeGitCommandFailed, identity, "inspect active Git operations", result)
		}
		path := strings.TrimSpace(result.stdout)
		if path == "" {
			return "", false, problem.New(problem.Details{
				Code:        problem.CodeGitCommandFailed,
				Category:    problem.CategoryRepository,
				Field:       "Git operation state",
				Expected:    "a path returned by git rev-parse --git-path",
				Rule:        "doctor must locate Git operation state through Git",
				Remediation: "repair the repository metadata and retry doctor",
			})
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(identity.Root, path)
		}
		if _, err := os.Stat(path); err == nil {
			return candidate.name, true, nil
		} else if !os.IsNotExist(err) {
			return "", false, problem.Wrap(problem.Details{
				Code:        problem.CodeGitCommandFailed,
				Category:    problem.CategoryRepository,
				Field:       "Git operation state",
				Actual:      path,
				Expected:    "an accessible Git operation marker",
				Rule:        "doctor must inspect Git operation markers safely",
				Remediation: "check repository permissions and retry doctor",
			}, err)
		}
	}
	return "", false, nil
}

// HasCommits reports whether HEAD resolves to at least one commit.
func (repository *Repository) HasCommits(ctx context.Context, identity port.RepositoryIdentity) (bool, error) {
	result := repository.invoke(ctx, identity.Root, nil, "rev-parse", "--verify", "--quiet", "HEAD")
	switch {
	case result.err == nil:
		return true, nil
	case result.exitCode == 1:
		return false, nil
	default:
		return false, repository.commandProblem(problem.CodeGitCommandFailed, identity, "determine whether HEAD exists", result)
	}
}

// IsWorktreeClean reports whether tracked and untracked files are unchanged.
func (repository *Repository) IsWorktreeClean(ctx context.Context, identity port.RepositoryIdentity) (bool, error) {
	result := repository.invoke(ctx, identity.Root, nil, "status", "--porcelain=v1", "--untracked-files=normal")
	if result.err != nil {
		return false, repository.commandProblem(problem.CodeGitCommandFailed, identity, "inspect the worktree", result)
	}
	return strings.TrimSpace(result.stdout) == "", nil
}

// CurrentBranch resolves the current branch and parses the product's canonical
// branch taxonomy.
func (repository *Repository) CurrentBranch(ctx context.Context, identity port.RepositoryIdentity) (branch.BranchName, error) {
	result := repository.invoke(ctx, identity.Root, nil, "branch", "--show-current")
	if result.err != nil {
		return branch.BranchName{}, repository.commandProblem(problem.CodeGitCommandFailed, identity, "read the current branch", result)
	}

	raw := strings.TrimSpace(result.stdout)
	if raw == "" {
		return branch.BranchName{}, problem.New(problem.Details{
			Code:        problem.CodeBranchNameInvalid,
			Category:    problem.CategoryRepository,
			Field:       "current branch",
			Expected:    "a checked-out canonical branch",
			Rule:        "detached HEAD cannot satisfy governed branch workflows",
			Example:     "feature/ABC-123-add-export-button",
			Remediation: "switch to a canonical branch before continuing",
		})
	}
	return branch.ParseName(raw)
}

// ValidateBranchRef delegates Git's ref-format validation after domain parsing.
func (repository *Repository) ValidateBranchRef(ctx context.Context, identity port.RepositoryIdentity, name branch.BranchName) error {
	result := repository.invoke(ctx, identity.Root, nil, "check-ref-format", "--branch", name.String())
	if result.err == nil {
		return nil
	}
	return problem.Wrap(problem.Details{
		Code:        problem.CodeBranchRefInvalid,
		Category:    problem.CategoryGovernance,
		Field:       "branch",
		Actual:      name.String(),
		Expected:    "a Git-valid branch reference",
		Rule:        "branch names must pass git check-ref-format --branch",
		Example:     "feature/ABC-123-add-export-button",
		Remediation: "use a canonical name without Git-reserved ref syntax",
	}, commandCause(result))
}

// BranchExists reports whether a local branch exists.
func (repository *Repository) BranchExists(ctx context.Context, identity port.RepositoryIdentity, name branch.BranchName) (bool, error) {
	result := repository.invoke(ctx, identity.Root, nil, "show-ref", "--verify", "--quiet", "refs/heads/"+name.String())
	switch {
	case result.err == nil:
		return true, nil
	case result.exitCode == 1:
		return false, nil
	default:
		return false, repository.commandProblem(problem.CodeGitCommandFailed, identity, "check whether the branch exists", result)
	}
}

// OfficialBranchesForTicket finds canonical official working branches carrying
// the requested ticket in either local or selected remote-tracking refs. It
// intentionally ignores noncanonical user branches because they are outside the
// product's governed namespace.
func (repository *Repository) OfficialBranchesForTicket(
	ctx context.Context,
	identity port.RepositoryIdentity,
	id ticket.ID,
) ([]branch.BranchName, error) {
	result := repository.invoke(
		ctx,
		identity.Root,
		nil,
		"for-each-ref",
		"--format=%(refname)",
		"refs/heads",
		"refs/remotes/"+identity.Remote,
	)
	if result.err != nil {
		return nil, repository.commandProblem(problem.CodeGitCommandFailed, identity, "find official branches for the ticket", result)
	}

	localPrefix := "refs/heads/"
	remotePrefix := "refs/remotes/" + identity.Remote + "/"
	found := make(map[string]branch.BranchName)
	for _, raw := range strings.Split(strings.TrimSpace(result.stdout), "\n") {
		ref := strings.TrimSpace(raw)
		if ref == "" {
			continue
		}

		var candidate string
		switch {
		case strings.HasPrefix(ref, localPrefix):
			candidate = strings.TrimPrefix(ref, localPrefix)
		case strings.HasPrefix(ref, remotePrefix):
			candidate = strings.TrimPrefix(ref, remotePrefix)
		default:
			continue
		}
		if candidate == "HEAD" {
			continue
		}

		name, err := branch.ParseName(candidate)
		if err != nil {
			continue
		}
		branchTicket, ticketScoped := name.Ticket()
		if !ticketScoped || !name.Family().IsOfficialWorkingBranch() || branchTicket.String() != id.String() {
			continue
		}
		found[name.String()] = name
	}

	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)

	resultBranches := make([]branch.BranchName, 0, len(names))
	for _, name := range names {
		resultBranches = append(resultBranches, found[name])
	}
	return resultBranches, nil
}

// Fetch updates remote-tracking references while pruning deleted remote refs.
func (repository *Repository) Fetch(ctx context.Context, identity port.RepositoryIdentity) error {
	result := repository.invoke(ctx, identity.Root, nil, "fetch", "--prune", identity.Remote)
	if result.err != nil {
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "fetch the selected remote", result)
	}
	return nil
}

// TargetBaseExists checks the fetched remote-tracking reference used to create
// or synchronize a branch. A missing base is a normal repository state, not a
// failed Git operation.
func (repository *Repository) TargetBaseExists(
	ctx context.Context,
	identity port.RepositoryIdentity,
	base branch.TargetBase,
) (bool, error) {
	if !base.IsRemoteTracking() {
		return false, problem.New(problem.Details{
			Code:        problem.CodeBranchBaseInvalid,
			Category:    problem.CategoryRepository,
			Field:       "target base",
			Actual:      base.String(),
			Expected:    "a remote-tracking target base",
			Rule:        "remote branch creation checks the fetched selected remote",
			Example:     identity.Remote + "/develop",
			Remediation: "select a canonical remote target base",
		})
	}
	result := repository.invoke(
		ctx,
		identity.Root,
		nil,
		"show-ref",
		"--verify",
		"--quiet",
		"refs/remotes/"+base.String(),
	)
	switch {
	case result.err == nil:
		return true, nil
	case result.exitCode == 1:
		return false, nil
	default:
		return false, repository.commandProblem(problem.CodeGitCommandFailed, identity, "verify the fetched target base", result)
	}
}

// CreateBranch creates a branch directly from its remote-tracking target base.
// It switches to the branch only when switchTo is true.
func (repository *Repository) CreateBranch(ctx context.Context, identity port.RepositoryIdentity, name branch.BranchName, base branch.TargetBase, switchTo bool) error {
	arguments := []string{"branch", name.String(), base.String()}
	action := "create the branch"
	if switchTo {
		arguments = []string{"switch", "-c", name.String(), base.String()}
		action = "create and switch to the branch"
	}
	result := repository.invoke(ctx, identity.Root, nil, arguments...)
	if result.err != nil {
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, action, result)
	}
	return nil
}

// StoreWorkflowBase records the actual remote base selected by a specialized
// workflow in local Git configuration. Later hook and sync validation can use
// this value without guessing from the branch name.
func (repository *Repository) StoreWorkflowBase(ctx context.Context, identity port.RepositoryIdentity, name branch.BranchName, base branch.TargetBase) error {
	if !base.IsRemoteTracking() {
		return problem.New(problem.Details{
			Code:        problem.CodeBranchBaseInvalid,
			Category:    problem.CategoryRepository,
			Field:       "workflow base",
			Actual:      base.String(),
			Expected:    "a remote-tracking workflow base",
			Rule:        "specialized workflow bases must be reproducible by local hook validation",
			Remediation: "use a remote main, develop, release, or support base",
		})
	}
	bases, err := repository.workflowBases(ctx, identity)
	if err != nil {
		return err
	}
	bases[name.String()] = base.String()
	encoded := encodeWorkflowBases(bases)
	result := repository.invoke(ctx, identity.Root, nil, "config", "--local", workflowBasesConfigKey, string(encoded))
	if result.err != nil {
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "store workflow base metadata", result)
	}
	return nil
}

// ClearWorkflowBase removes the local base metadata for a branch after local
// cleanup. An absent entry is already clear and therefore succeeds.
func (repository *Repository) ClearWorkflowBase(
	ctx context.Context,
	identity port.RepositoryIdentity,
	name branch.BranchName,
) error {
	bases, err := repository.workflowBases(ctx, identity)
	if err != nil {
		return err
	}
	if _, found := bases[name.String()]; !found {
		return nil
	}
	delete(bases, name.String())
	encoded := encodeWorkflowBases(bases)
	result := repository.invoke(ctx, identity.Root, nil, "config", "--local", workflowBasesConfigKey, string(encoded))
	if result.err != nil {
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "clear workflow base metadata", result)
	}
	return nil
}

// WorkflowBase loads a recorded specialized workflow base for a branch.
func (repository *Repository) WorkflowBase(ctx context.Context, identity port.RepositoryIdentity, name branch.BranchName) (branch.TargetBase, bool, error) {
	bases, err := repository.workflowBases(ctx, identity)
	if err != nil {
		return branch.TargetBase{}, false, err
	}
	raw, found := bases[name.String()]
	if !found {
		return branch.TargetBase{}, false, nil
	}
	base, err := parseWorkflowBase(identity.Remote, raw)
	if err != nil {
		return branch.TargetBase{}, false, err
	}
	return base, true, nil
}

// ResolveRevision resolves a ref to the exact commit object used by local
// quality evidence. It never reads a revision from untrusted metadata.
func (repository *Repository) ResolveRevision(
	ctx context.Context,
	identity port.RepositoryIdentity,
	revision string,
) (string, error) {
	result := repository.invoke(ctx, identity.Root, nil, "rev-parse", "--verify", "--quiet", revision+"^{commit}")
	if result.err != nil {
		return "", repository.commandProblem(problem.CodeGitCommandFailed, identity, "resolve a commit revision", result)
	}
	resolved := strings.ToLower(strings.TrimSpace(result.stdout))
	if !commitObjectIDPattern.MatchString(resolved) {
		return "", problem.New(problem.Details{
			Code:        problem.CodeGitCommandFailed,
			Category:    problem.CategoryGit,
			Field:       "Git revision",
			Actual:      resolved,
			Expected:    "a complete hexadecimal commit object ID",
			Rule:        "revision-bound quality evidence uses an exact commit object",
			Remediation: "repair the repository metadata and retry the publish validation",
		})
	}
	return resolved, nil
}

// LoadFinalQualityEvidence reads the single short-lived quality record from
// repository-local Git metadata. An absent record is a normal cache miss.
func (repository *Repository) LoadFinalQualityEvidence(
	ctx context.Context,
	identity port.RepositoryIdentity,
) (port.FinalQualityEvidence, bool, error) {
	result := repository.invoke(ctx, identity.Root, nil, "config", "--local", "--get", finalQualityEvidenceConfigKey)
	switch {
	case result.err == nil:
		var evidence port.FinalQualityEvidence
		if err := json.Unmarshal([]byte(strings.TrimSpace(result.stdout)), &evidence); err != nil {
			return port.FinalQualityEvidence{}, false, problem.Wrap(problem.Details{
				Code:        problem.CodeConfigurationInvalid,
				Category:    problem.CategoryConfig,
				Field:       "final quality evidence",
				Expected:    "a valid repository-local final-quality JSON record",
				Rule:        "local publish quality evidence must remain machine-readable and fail closed when corrupted",
				Remediation: "run the final local quality suite again to replace the local evidence record",
			}, err)
		}
		return evidence, true, nil
	case result.exitCode == 1:
		return port.FinalQualityEvidence{}, false, nil
	default:
		return port.FinalQualityEvidence{}, false, repository.commandProblem(
			problem.CodeGitCommandFailed,
			identity,
			"read final quality evidence",
			result,
		)
	}
}

// StoreFinalQualityEvidence writes one JSON value through Git's local config
// lockfile path, keeping the optimization outside versioned worktree files.
func (repository *Repository) StoreFinalQualityEvidence(
	ctx context.Context,
	identity port.RepositoryIdentity,
	evidence port.FinalQualityEvidence,
) error {
	// FinalQualityEvidence contains only JSON-serializable scalar, slice, and
	// time values. Keeping this invariant in the port avoids an unreachable
	// error path at the Git metadata boundary.
	encoded, _ := json.Marshal(evidence)
	result := repository.invoke(
		ctx,
		identity.Root,
		nil,
		"config",
		"--local",
		finalQualityEvidenceConfigKey,
		string(encoded),
	)
	if result.err != nil {
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "store final quality evidence", result)
	}
	return nil
}

// LoadHotfixManifestProgress reads the one active ordered propagation state.
// An absent value is a normal no-progress result; malformed state fails
// closed instead of guessing which commit to cherry-pick next.
func (repository *Repository) LoadHotfixManifestProgress(
	ctx context.Context,
	identity port.RepositoryIdentity,
) (port.HotfixManifestProgress, bool, error) {
	result := repository.invoke(ctx, identity.Root, nil, "config", "--local", "--get", hotfixManifestProgressConfigKey)
	switch {
	case result.err == nil:
		var document hotfixManifestProgressDocument
		if err := json.Unmarshal([]byte(strings.TrimSpace(result.stdout)), &document); err != nil {
			return port.HotfixManifestProgress{}, false, invalidHotfixManifestProgress(err)
		}
		progress, err := document.progress()
		if err != nil {
			return port.HotfixManifestProgress{}, false, err
		}
		return progress, true, nil
	case result.exitCode == 1:
		return port.HotfixManifestProgress{}, false, nil
	default:
		return port.HotfixManifestProgress{}, false, repository.commandProblem(
			problem.CodeGitCommandFailed,
			identity,
			"read hotfix manifest propagation progress",
			result,
		)
	}
}

// StoreHotfixManifestProgress writes one bounded propagation cursor through
// Git's local config lockfile path.
func (repository *Repository) StoreHotfixManifestProgress(
	ctx context.Context,
	identity port.RepositoryIdentity,
	progress port.HotfixManifestProgress,
) error {
	document, err := newHotfixManifestProgressDocument(progress)
	if err != nil {
		return err
	}
	encoded, _ := json.Marshal(document)
	result := repository.invoke(
		ctx,
		identity.Root,
		nil,
		"config",
		"--local",
		hotfixManifestProgressConfigKey,
		string(encoded),
	)
	if result.err != nil {
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "store hotfix manifest propagation progress", result)
	}
	return nil
}

// ClearHotfixManifestProgress removes the completed or abandoned local cursor.
func (repository *Repository) ClearHotfixManifestProgress(
	ctx context.Context,
	identity port.RepositoryIdentity,
) error {
	result := repository.invoke(ctx, identity.Root, nil, "config", "--local", "--unset-all", hotfixManifestProgressConfigKey)
	switch {
	case result.err == nil, result.exitCode == 1:
		return nil
	default:
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "clear hotfix manifest propagation progress", result)
	}
}

type hotfixManifestProgressDocument struct {
	Branch   string   `json:"branch"`
	Source   string   `json:"source"`
	Target   string   `json:"target"`
	Manifest []string `json:"manifest"`
	Next     int      `json:"next"`
}

func newHotfixManifestProgressDocument(progress port.HotfixManifestProgress) (hotfixManifestProgressDocument, error) {
	document := hotfixManifestProgressDocument{
		Branch:   progress.Branch.String(),
		Source:   progress.Source.String(),
		Target:   progress.Target.String(),
		Manifest: append([]string(nil), progress.Manifest...),
		Next:     progress.Next,
	}
	if _, err := document.progress(); err != nil {
		return hotfixManifestProgressDocument{}, err
	}
	return document, nil
}

func (document hotfixManifestProgressDocument) progress() (port.HotfixManifestProgress, error) {
	branchName, err := branch.ParseName(document.Branch)
	if err != nil {
		return port.HotfixManifestProgress{}, invalidHotfixManifestProgress(err)
	}
	source, err := branch.ParseName(document.Source)
	if err != nil {
		return port.HotfixManifestProgress{}, invalidHotfixManifestProgress(err)
	}
	target, err := branch.ParseName(document.Target)
	if err != nil {
		return port.HotfixManifestProgress{}, invalidHotfixManifestProgress(err)
	}
	if branchName.Family() != branch.FamilyFix || source.Family() != branch.FamilyHotfix {
		return port.HotfixManifestProgress{}, invalidHotfixManifestProgress(nil)
	}
	switch target.Family() {
	case branch.FamilyDevelop, branch.FamilyRelease, branch.FamilySupport:
	default:
		return port.HotfixManifestProgress{}, invalidHotfixManifestProgress(nil)
	}
	if len(document.Manifest) == 0 || document.Next < 0 || document.Next > len(document.Manifest) {
		return port.HotfixManifestProgress{}, invalidHotfixManifestProgress(nil)
	}
	seen := make(map[string]struct{}, len(document.Manifest))
	for _, commit := range document.Manifest {
		if !commitObjectIDPattern.MatchString(commit) {
			return port.HotfixManifestProgress{}, invalidHotfixManifestProgress(nil)
		}
		if _, found := seen[commit]; found {
			return port.HotfixManifestProgress{}, invalidHotfixManifestProgress(nil)
		}
		seen[commit] = struct{}{}
	}
	return port.HotfixManifestProgress{
		Branch:   branchName,
		Source:   source,
		Target:   target,
		Manifest: append([]string(nil), document.Manifest...),
		Next:     document.Next,
	}, nil
}

func invalidHotfixManifestProgress(cause error) error {
	return problem.Wrap(problem.Details{
		Code:        problem.CodeConfigurationInvalid,
		Category:    problem.CategoryConfig,
		Field:       "hotfix manifest propagation progress",
		Expected:    "a valid repository-local ordered hotfix propagation record",
		Rule:        "multi-commit hotfix propagation resumes only from exact local metadata",
		Remediation: "recreate the controlled propagation branch instead of guessing the next cherry-pick",
	}, cause)
}

// encodeWorkflowBases cannot fail because the workflow metadata is restricted
// to a map with string keys and string values. Keeping this invariant local
// avoids an unreachable runtime error path while preserving JSON encoding.
func encodeWorkflowBases(bases map[string]string) []byte {
	encoded, _ := json.Marshal(bases)
	return encoded
}

// SwitchBranch switches to an existing canonical branch.
func (repository *Repository) SwitchBranch(ctx context.Context, identity port.RepositoryIdentity, name branch.BranchName) error {
	result := repository.invoke(ctx, identity.Root, nil, "switch", name.String())
	if result.err != nil {
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "switch branches", result)
	}
	return nil
}

// PublicationState uses the fetched remote-tracking ref as the local,
// network-free publication signal. Callers must fetch before relying on it.
func (repository *Repository) PublicationState(ctx context.Context, identity port.RepositoryIdentity, name branch.BranchName) (branch.PublicationState, error) {
	result := repository.invoke(ctx, identity.Root, nil, "show-ref", "--verify", "--quiet", "refs/remotes/"+identity.Remote+"/"+name.String())
	switch {
	case result.err == nil:
		return branch.PublicationPublished, nil
	case result.exitCode == 1:
		return branch.PublicationUnpublished, nil
	default:
		return branch.PublicationUnknown, repository.commandProblem(problem.CodeBranchPublicationUnknown, identity, "determine branch publication state", result)
	}
}

// HasMissingBaseCommits reports whether the current HEAD lacks commits from
// the selected remote-tracking base.
func (repository *Repository) HasMissingBaseCommits(ctx context.Context, identity port.RepositoryIdentity, base branch.TargetBase) (bool, error) {
	result := repository.invoke(ctx, identity.Root, nil, "rev-list", "--count", "HEAD.."+base.String())
	if result.err != nil {
		return false, repository.commandProblem(problem.CodeBranchBaseInvalid, identity, "compare the current branch with its target base", result)
	}

	count, err := strconv.ParseInt(strings.TrimSpace(result.stdout), 10, 64)
	if err != nil || count < 0 {
		return false, problem.Wrap(problem.Details{
			Code:        problem.CodeBranchBaseInvalid,
			Category:    problem.CategoryRepository,
			Field:       "target base",
			Actual:      base.String(),
			Expected:    "a comparable remote-tracking base",
			Rule:        "Git must return a non-negative missing-commit count",
			Example:     "origin/develop",
			Remediation: "fetch the remote and verify the selected target base exists",
		}, err)
	}
	return count > 0, nil
}

// CommitMessagesSince returns complete commit messages in the range from base
// (exclusive) to HEAD. Control separators are safe because commit validation
// forbids them in governed messages.
func (repository *Repository) CommitMessagesSince(ctx context.Context, identity port.RepositoryIdentity, base branch.TargetBase) ([]string, error) {
	result := repository.invoke(ctx, identity.Root, nil, "log", "--format=%x1e%B%x00", base.String()+"..HEAD")
	if result.err != nil {
		return nil, repository.commandProblem(problem.CodeGitCommandFailed, identity, "read commit messages for the branch", result)
	}
	return parseCommitMessages(result.stdout)
}

// Rebase reapplies local commits onto the target base. The application layer
// decides whether this mutation is policy-safe before calling the adapter.
func (repository *Repository) Rebase(ctx context.Context, identity port.RepositoryIdentity, base branch.TargetBase) error {
	result := repository.invoke(ctx, identity.Root, nil, "rebase", base.String())
	if result.err != nil {
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "rebase onto the target base", result)
	}
	return nil
}

// ContinueRebase advances an already started rebase after the user resolves
// and stages its conflicts. A non-interactive editor avoids blocking the retry
// interaction while preserving Git's existing commit message.
func (repository *Repository) ContinueRebase(ctx context.Context, identity port.RepositoryIdentity) error {
	result := repository.invoke(ctx, identity.Root, nil, "-c", "core.editor=true", "rebase", "--continue")
	if result.err != nil {
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "continue the resolved rebase", result)
	}
	return nil
}

// Merge merges the target base with an explicit governed merge message.
func (repository *Repository) Merge(ctx context.Context, identity port.RepositoryIdentity, base branch.TargetBase, message commitmsg.Message) error {
	result := repository.invoke(ctx, identity.Root, nil, "merge", "--no-ff", "--no-edit", "-m", message.String(), base.String())
	if result.err != nil {
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "merge the target base", result)
	}
	return nil
}

// ContinueMerge advances an already started merge after the resolver stages
// every conflicted path. A non-interactive editor preserves the governed merge
// message recorded when the merge began.
func (repository *Repository) ContinueMerge(ctx context.Context, identity port.RepositoryIdentity) error {
	result := repository.invoke(ctx, identity.Root, nil, "-c", "core.editor=true", "merge", "--continue")
	if result.err != nil {
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "continue the resolved merge", result)
	}
	return nil
}

// ActiveMergeTargetMatches verifies that the in-progress merge still targets
// the selected fetched remote-tracking base before it is continued.
func (repository *Repository) ActiveMergeTargetMatches(
	ctx context.Context,
	identity port.RepositoryIdentity,
	target branch.TargetBase,
) (bool, error) {
	mergeHead, err := repository.resolveRevision(ctx, identity, "MERGE_HEAD", "resolve the active merge target")
	if err != nil {
		return false, err
	}
	targetRevision, err := repository.resolveCommit(ctx, identity, target, "resolve the selected merge target")
	if err != nil {
		return false, err
	}
	return mergeHead == targetRevision, nil
}

// resolveCommit preserves the GOV-25 commit-resolution contract for any
// remote-tracking target base that participates in governed merge recovery.
func (repository *Repository) resolveCommit(
	ctx context.Context,
	identity port.RepositoryIdentity,
	base branch.TargetBase,
	operation string,
) (string, error) {
	return repository.resolveRevision(ctx, identity, base.String(), operation)
}

func (repository *Repository) resolveRevision(
	ctx context.Context,
	identity port.RepositoryIdentity,
	reference string,
	operation string,
) (string, error) {
	result := repository.invoke(ctx, identity.Root, nil, "rev-parse", "--verify", reference+"^{commit}")
	if result.err != nil {
		return "", repository.commandProblem(problem.CodeGitCommandFailed, identity, operation, result)
	}
	revision := strings.TrimSpace(result.stdout)
	if revision == "" {
		return "", problem.New(problem.Details{
			Code:        problem.CodeGitCommandFailed,
			Category:    problem.CategoryGit,
			Field:       "Git revision",
			Expected:    "a non-empty commit revision",
			Rule:        "governed merge recovery requires immutable target commit identities",
			Remediation: "fetch the selected remote and verify the recovery input refs before retrying",
		})
	}
	return revision, nil
}

// SquashMerge stages the net changes from a private branch without preserving
// its individual commits. The application validates and creates the resulting
// governed commit separately.
func (repository *Repository) SquashMerge(ctx context.Context, identity port.RepositoryIdentity, source branch.BranchName) error {
	result := repository.invoke(ctx, identity.Root, nil, "merge", "--squash", "--no-commit", source.String())
	if result.err != nil {
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "squash merge the scratch branch", result)
	}
	return nil
}

// CherryPick applies one explicitly supplied commit and preserves its source
// commit ID in the generated message through Git's -x trailer.
func (repository *Repository) CherryPick(ctx context.Context, identity port.RepositoryIdentity, commitID string) error {
	result := repository.invoke(ctx, identity.Root, nil, "cherry-pick", "-x", commitID)
	if result.err != nil {
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "cherry-pick the requested source commit", result)
	}
	return nil
}

// ContinueCherryPick advances an already started cherry-pick after the user
// resolves and stages its conflicts. A non-interactive editor preserves the
// original Git message while keeping automated continuation non-blocking.
func (repository *Repository) ContinueCherryPick(ctx context.Context, identity port.RepositoryIdentity) error {
	result := repository.invoke(ctx, identity.Root, nil, "-c", "core.editor=true", "cherry-pick", "--continue")
	if result.err != nil {
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "continue the resolved cherry-pick", result)
	}
	return nil
}

// DeleteLocalBranch removes a completed local branch. Scratch branches may use
// force deletion because they are explicitly private and disposable.
func (repository *Repository) DeleteLocalBranch(ctx context.Context, identity port.RepositoryIdentity, name branch.BranchName, force bool) error {
	option := "-d"
	if force {
		option = "-D"
	}
	result := repository.invoke(ctx, identity.Root, nil, "branch", option, name.String())
	if result.err != nil {
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "delete the local branch", result)
	}
	return nil
}

// ReleaseTagsAt lists local release tags pointing at a fetched revision.
func (repository *Repository) ReleaseTagsAt(ctx context.Context, identity port.RepositoryIdentity, revision string) ([]string, error) {
	result := repository.invoke(ctx, identity.Root, nil, "tag", "--points-at", revision)
	if result.err != nil {
		return nil, repository.commandProblem(problem.CodeGitCommandFailed, identity, "inspect release tags for a revision", result)
	}
	lines := strings.Split(strings.TrimSpace(result.stdout), "\n")
	tags := make([]string, 0, len(lines))
	for _, line := range lines {
		tag := strings.TrimSpace(line)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags, nil
}

// HasUnmergedConflicts reports whether the index contains unresolved conflict
// entries left by an interrupted merge, squash merge, rebase, or cherry-pick.
func (repository *Repository) HasUnmergedConflicts(ctx context.Context, identity port.RepositoryIdentity) (bool, error) {
	result := repository.invoke(ctx, identity.Root, nil, "diff", "--name-only", "--diff-filter=U")
	if result.err != nil {
		return false, repository.commandProblem(problem.CodeGitCommandFailed, identity, "inspect unresolved Git conflicts", result)
	}
	return strings.TrimSpace(result.stdout) != "", nil
}

// HasStagedChanges reports whether the Git index contains changes.
func (repository *Repository) HasStagedChanges(ctx context.Context, identity port.RepositoryIdentity) (bool, error) {
	result := repository.invoke(ctx, identity.Root, nil, "diff", "--cached", "--quiet")
	switch {
	case result.err == nil:
		return false, nil
	case result.exitCode == 1:
		return true, nil
	default:
		return false, repository.commandProblem(problem.CodeGitCommandFailed, identity, "inspect staged changes", result)
	}
}

// Stage explicitly stages only the supplied paths.
func (repository *Repository) Stage(ctx context.Context, identity port.RepositoryIdentity, paths []string) error {
	if len(paths) == 0 {
		return problem.New(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryUsage,
			Field:       "stage paths",
			Expected:    "at least one explicit path",
			Rule:        "the CLI never stages all files implicitly",
			Example:     "--stage cmd/git-governance/main.go",
			Remediation: "supply each path to stage explicitly or stage files with Git before invoking the command",
		})
	}

	arguments := make([]string, 0, len(paths)+2)
	arguments = append(arguments, "add", "--")
	arguments = append(arguments, paths...)
	result := repository.invoke(ctx, identity.Root, nil, arguments...)
	if result.err != nil {
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "stage the requested paths", result)
	}
	return nil
}

// Commit creates a commit by streaming its validated message to Git.
func (repository *Repository) Commit(ctx context.Context, identity port.RepositoryIdentity, message commitmsg.Message) error {
	stdin := strings.NewReader(message.String() + "\n")
	result := repository.invoke(ctx, identity.Root, stdin, "commit", "--file=-")
	if result.err != nil {
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "create the commit", result)
	}
	return nil
}

// Push pushes the named branch, optionally configuring its upstream.
func (repository *Repository) Push(ctx context.Context, identity port.RepositoryIdentity, name branch.BranchName, setUpstream bool) error {
	arguments := []string{"push"}
	if setUpstream {
		arguments = append(arguments, "--set-upstream")
	}
	arguments = append(arguments, identity.Remote, name.String())
	result := repository.invoke(ctx, identity.Root, nil, arguments...)
	if result.err != nil {
		return repository.commandProblem(problem.CodeGitCommandFailed, identity, "push the branch", result)
	}
	return nil
}

// InspectPushUpdate evaluates the exact object IDs supplied by Git's pre-push
// hook. It never substitutes HEAD for the update's local object ID, so pushes
// such as HEAD:main and multi-ref pushes remain correctly attributable.
func (repository *Repository) InspectPushUpdate(
	ctx context.Context,
	identity port.RepositoryIdentity,
	base branch.TargetBase,
	localObjectID string,
	remoteObjectID string,
) (port.PushUpdateInspection, error) {
	missingBaseCommits, err := repository.hasMissingBaseCommitsForRevision(ctx, identity, base, localObjectID)
	if err != nil {
		return port.PushUpdateInspection{}, err
	}
	commitMessages, err := repository.commitMessagesSinceRevision(ctx, identity, base, localObjectID)
	if err != nil {
		return port.PushUpdateInspection{}, err
	}

	fastForward := true
	if !zeroObjectID(remoteObjectID) {
		fastForward, err = repository.isAncestor(ctx, identity, remoteObjectID, localObjectID)
		if err != nil {
			return port.PushUpdateInspection{}, err
		}
	}
	return port.PushUpdateInspection{
		MissingBaseCommits: missingBaseCommits,
		FastForward:        fastForward,
		CommitMessages:     commitMessages,
	}, nil
}

func (repository *Repository) invoke(ctx context.Context, directory string, stdin io.Reader, arguments ...string) processResult {
	if ctx == nil {
		ctx = context.Background()
	}
	contextWithTimeout, cancel := context.WithTimeout(ctx, repository.timeout)
	defer cancel()
	return repository.runner.run(contextWithTimeout, directory, stdin, arguments...)
}

func (repository *Repository) invokeNoPrompt(ctx context.Context, directory string, stdin io.Reader, arguments ...string) processResult {
	if ctx == nil {
		ctx = context.Background()
	}
	contextWithTimeout, cancel := context.WithTimeout(ctx, repository.timeout)
	defer cancel()
	if runner, ok := repository.runner.(environmentProcessRunner); ok {
		return runner.runWithEnvironment(contextWithTimeout, directory, stdin, noPromptGitEnvironment, arguments...)
	}
	return repository.runner.run(contextWithTimeout, directory, stdin, arguments...)
}

func (repository *Repository) gitAuthenticationProblem(
	identity port.RepositoryIdentity,
	action string,
	result processResult,
) error {
	code := problem.CodeGitCommandFailed
	category := problem.CategoryGit
	if result.err != nil && isContextError(result.err) {
		code = problem.CodeExternalCommandFailed
		category = problem.CategoryExternal
	}
	return problem.Wrap(problem.Details{
		Code:        code,
		Category:    category,
		Field:       "Git authentication",
		Context:     strings.Join(commandSummary(identity, action), " "),
		Expected:    "a non-interactive credential capable of an authenticated dry-run push",
		Rule:        "doctor verifies Git transport authentication without mutating remote references",
		Remediation: "authenticate Git transport with SSH or Git Credential Manager and retry doctor",
	}, commandCause(result))
}

func (repository *Repository) commandProblem(code problem.Code, identity port.RepositoryIdentity, action string, result processResult) error {
	category := problem.CategoryGit
	if code == problem.CodeBranchPublicationUnknown || code == problem.CodeBranchBaseInvalid {
		category = problem.CategoryRepository
	}
	if result.err != nil && isContextError(result.err) {
		category = problem.CategoryExternal
		code = problem.CodeExternalCommandFailed
	}

	return problem.Wrap(problem.Details{
		Code:        code,
		Category:    category,
		Field:       "git operation",
		Context:     strings.Join(commandSummary(identity, action), " "),
		Diagnostic:  commandDiagnostic(result),
		Expected:    "a successful Git operation",
		Rule:        "Git operations must complete successfully before the workflow can continue",
		Example:     "git fetch --prune origin",
		Remediation: "review the Git diagnostic, correct the repository state, and retry",
	}, commandCause(result))
}

func commandDiagnostic(result processResult) string {
	diagnostic := strings.TrimSpace(result.stderr)
	if diagnostic == "" {
		diagnostic = strings.TrimSpace(result.stdout)
	}
	if diagnostic == "" && result.truncated {
		return "[Git diagnostic output was truncated]"
	}
	diagnostic = urlCredentialsPattern.ReplaceAllString(diagnostic, "${1}[redacted]@")
	diagnostic = secretAssignmentPattern.ReplaceAllString(diagnostic, "$1=[redacted]")
	if len(diagnostic) > maxDiagnosticBytes {
		diagnostic = diagnostic[:maxDiagnosticBytes] + "\n[Git diagnostic output truncated]"
	} else if result.truncated {
		diagnostic += "\n[Git diagnostic output truncated]"
	}
	return diagnostic
}

func commandSummary(identity port.RepositoryIdentity, action string) []string {
	parts := []string{"action=" + action}
	if identity.Root != "" {
		parts = append(parts, "repository="+identity.Root)
	}
	if identity.Remote != "" {
		parts = append(parts, "remote="+identity.Remote)
	}
	return parts
}

func commandCause(result processResult) error {
	if result.err == nil {
		return nil
	}
	if result.exitCode >= 0 {
		return fmt.Errorf("git exited with status %d", result.exitCode)
	}
	return fmt.Errorf("git process could not be started")
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func parseCommitMessages(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	records := strings.Split(raw, "\x00")
	messages := make([]string, 0, len(records))
	for _, record := range records {
		record = strings.TrimLeft(record, "\n")
		if record == "" {
			continue
		}
		if !strings.HasPrefix(record, "\x1e") {
			return nil, problem.New(problem.Details{
				Code:        problem.CodeGitCommandFailed,
				Category:    problem.CategoryGit,
				Field:       "git log output",
				Expected:    "record-separated complete commit messages",
				Rule:        "commit history parsing uses explicit control separators",
				Remediation: "rerun the command with a supported Git version",
			})
		}
		message := strings.TrimPrefix(record, "\x1e")
		message = strings.TrimSuffix(message, "\n")
		messages = append(messages, message)
	}
	return messages, nil
}

func (repository *Repository) hasMissingBaseCommitsForRevision(
	ctx context.Context,
	identity port.RepositoryIdentity,
	base branch.TargetBase,
	revision string,
) (bool, error) {
	result := repository.invoke(ctx, identity.Root, nil, "rev-list", "--count", revision+".."+base.String())
	if result.err != nil {
		return false, repository.commandProblem(problem.CodeBranchBaseInvalid, identity, "compare the pushed revision with its target base", result)
	}
	count, err := strconv.ParseInt(strings.TrimSpace(result.stdout), 10, 64)
	if err != nil || count < 0 {
		return false, problem.Wrap(problem.Details{
			Code:        problem.CodeBranchBaseInvalid,
			Category:    problem.CategoryRepository,
			Field:       "target base",
			Actual:      base.String(),
			Expected:    "a comparable remote-tracking base",
			Rule:        "Git must return a non-negative missing-commit count",
			Example:     "origin/develop",
			Remediation: "fetch the remote and verify the selected target base exists",
		}, err)
	}
	return count > 0, nil
}

func (repository *Repository) commitMessagesSinceRevision(
	ctx context.Context,
	identity port.RepositoryIdentity,
	base branch.TargetBase,
	revision string,
) ([]string, error) {
	result := repository.invoke(ctx, identity.Root, nil, "log", "--format=%x1e%B%x00", base.String()+".."+revision)
	if result.err != nil {
		return nil, repository.commandProblem(problem.CodeGitCommandFailed, identity, "read commit messages for the pushed revision", result)
	}
	return parseCommitMessages(result.stdout)
}

func (repository *Repository) isAncestor(
	ctx context.Context,
	identity port.RepositoryIdentity,
	ancestor string,
	descendant string,
) (bool, error) {
	result := repository.invoke(ctx, identity.Root, nil, "merge-base", "--is-ancestor", ancestor, descendant)
	switch {
	case result.err == nil:
		return true, nil
	case result.exitCode == 1:
		return false, nil
	default:
		return false, repository.commandProblem(problem.CodeGitCommandFailed, identity, "check whether the pushed update is fast-forward", result)
	}
}

func zeroObjectID(value string) bool {
	return value != "" && strings.Trim(value, "0") == ""
}

func (repository *Repository) workflowBases(ctx context.Context, identity port.RepositoryIdentity) (map[string]string, error) {
	result := repository.invoke(ctx, identity.Root, nil, "config", "--local", "--get", workflowBasesConfigKey)
	switch {
	case result.err == nil:
		var bases map[string]string
		if err := json.Unmarshal([]byte(strings.TrimSpace(result.stdout)), &bases); err != nil {
			return nil, problem.Wrap(problem.Details{
				Code:        problem.CodeConfigurationInvalid,
				Category:    problem.CategoryConfig,
				Field:       "workflow base metadata",
				Expected:    "a valid local JSON workflow-base map",
				Rule:        "specialized workflow bases must be stored in canonical local Git metadata",
				Remediation: "remove or correct git-governance.workflow-bases in local Git configuration",
			}, err)
		}
		if bases == nil {
			bases = make(map[string]string)
		}
		return bases, nil
	case result.exitCode == 1:
		return make(map[string]string), nil
	default:
		return nil, repository.commandProblem(problem.CodeGitCommandFailed, identity, "read workflow base metadata", result)
	}
}

func parseWorkflowBase(remote, raw string) (branch.TargetBase, error) {
	prefix := remote + "/"
	if !strings.HasPrefix(raw, prefix) {
		return branch.TargetBase{}, problem.New(problem.Details{
			Code:        problem.CodeBranchBaseInvalid,
			Category:    problem.CategoryRepository,
			Field:       "workflow base metadata",
			Actual:      raw,
			Expected:    prefix + "<canonical-branch>",
			Rule:        "stored workflow bases must use the selected remote",
			Remediation: "recreate the specialized branch through git-governance",
		})
	}
	name, err := branch.ParseName(strings.TrimPrefix(raw, prefix))
	if err != nil {
		return branch.TargetBase{}, err
	}
	return branch.NewTargetBase(remote, name)
}

var _ port.GitRepository = (*Repository)(nil)
var _ port.GitTransportAuthenticator = (*Repository)(nil)
var _ port.RevisionResolver = (*Repository)(nil)
var _ port.FinalQualityEvidenceStore = (*Repository)(nil)
var _ port.MergeContinuator = (*Repository)(nil)
var _ port.ActiveMergeTargetInspector = (*Repository)(nil)
