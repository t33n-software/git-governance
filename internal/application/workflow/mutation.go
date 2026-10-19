package workflow

import (
	"context"
	"strconv"
	"time"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	commitapp "github.com/t33n-software/git-governance/internal/application/commit"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/genesis"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

// runGenesisMutation is the single bound transaction of the birth: stage the
// explicit content set, create the signed genesis commit on main, create
// develop from the same revision, and install the hook boundary. The staged
// surface carries the derived mutation budget; the hook installation stays on
// the preflight surface because it is corpus-independent.
func (service *BootstrapService) runGenesisMutation(
	ctx context.Context,
	git port.GitRepository,
	capabilities bootstrapCapabilities,
	repository port.RepositoryIdentity,
	request BootstrapRequest,
	message commitmsg.Message,
) (string, port.HookInstallation, error) {
	if err := git.Stage(ctx, repository, request.StagePaths); err != nil {
		return "", port.HookInstallation{}, err
	}
	staged, err := git.HasStagedChanges(ctx, repository)
	if err != nil {
		return "", port.HookInstallation{}, err
	}
	if !staged {
		return "", port.HookInstallation{}, problem.New(problem.Details{
			Code:        problem.CodeGitCommandFailed,
			Category:    problem.CategoryGit,
			Field:       "index",
			Expected:    "the staged content set after staging the explicit paths",
			Rule:        "the genesis mutation requires the staged content set the preflight proved",
			Remediation: "review the repository state and retry the bootstrap",
		})
	}
	if err := git.Commit(ctx, repository, message); err != nil {
		return "", port.HookInstallation{}, err
	}
	revision, err := capabilities.revisions.ResolveRevision(ctx, repository, "HEAD")
	if err != nil {
		return "", port.HookInstallation{}, err
	}
	// mustMain is the product's fixed production-line taxonomy and therefore a
	// canonical local branch name. NewLocalBase cannot reject that invariant.
	developBase, _ := branch.NewLocalBase(mustMain())
	if err := git.CreateBranch(ctx, repository, mustDevelop(), developBase, false); err != nil {
		return "", port.HookInstallation{}, err
	}
	installation, err := capabilities.installer.InstallHooks(ctx, repository)
	if err != nil {
		return "", port.HookInstallation{}, err
	}
	return revision, installation, nil
}

// composeGenesisMessage assembles the canonical genesis commit: the chore
// family, the explicit ticket scope, the fixed subject, and the canonical
// body. Composition is pure and round-trip validated before any mutation.
func composeGenesisMessage(
	id ticket.ID,
	preflight BootstrapPreflight,
	publish bool,
) (commitmsg.Message, error) {
	return commitapp.Compose(commitapp.Draft{
		Family:  commitmsg.TypeChore,
		Ticket:  id,
		Subject: genesis.CommitSubject,
		Body: genesis.CommitBody(genesis.BodyFacts{
			Ticket:         id,
			ContentFiles:   preflight.ContentFiles,
			PolicySnapshot: preflight.PolicySnapshot,
			Publish:        publish,
		}),
	})
}

// bootstrapPlan renders the full genesis plan: the executed preflight plus
// the planned mutation, finalizer, and — when requested — the separately
// confirmed publication.
func bootstrapPlan(
	repository port.RepositoryIdentity,
	request BootstrapRequest,
	preflight BootstrapPreflight,
	message commitmsg.Message,
) []branchapp.PlanStep {
	plan := []branchapp.PlanStep{
		{Action: "preflight", Detail: "unborn-state proof, signing gate with canary, hook-manager resolution, policy snapshot, content boundary scan"},
		{Action: "stage", Detail: "stage " + strconv.Itoa(preflight.ContentFiles) + " content files from the explicit stage paths"},
		{Action: "commit", Detail: message.Header().String()},
		{Action: "create", Detail: "create develop from the genesis revision"},
		{Action: "install-hooks", Detail: "install the canonical hook boundary (commit-msg, pre-push)"},
		{Action: "finalize", Detail: "prove the born refs, the shared revision, the signature, and the hook boundary"},
	}
	if request.Push {
		plan = append(plan, branchapp.PlanStep{
			Action: "publish",
			Detail: "push main and develop to " + repository.Remote + " after the separate publication confirmation",
		})
	}
	return plan
}

// The genesis mutation budget derives from the proven preflight corpus size
// instead of a flat caller default: the base ceiling covers the commit, the
// branch creation, and the hook installation; the per-file allowance covers
// the linear staging cost of the explicit content set. The ceiling keeps the
// budget a bounded-execution guarantee for every corpus size.
const (
	genesisBaseBudget       = 60 * time.Second
	genesisPerFileAllowance = 10 * time.Millisecond
	genesisMaxBudget        = 30 * time.Minute
)

// deriveGenesisMutationBudget derives the per-process budget of the genesis
// mutation from the proven content-file count of the preflight.
func deriveGenesisMutationBudget(contentFiles int) time.Duration {
	if contentFiles < 0 {
		contentFiles = 0
	}
	budget := genesisBaseBudget + time.Duration(contentFiles)*genesisPerFileAllowance
	if budget > genesisMaxBudget {
		budget = genesisMaxBudget
	}
	return budget
}

// resolveGenesisMutationBudget binds the effective mutation budget: the
// derived budget is the primary protection, and an explicitly supplied caller
// timeout caps it as an upper bound. Zero keeps the derived budget.
func resolveGenesisMutationBudget(contentFiles int, explicitTimeout time.Duration) time.Duration {
	budget := deriveGenesisMutationBudget(contentFiles)
	if explicitTimeout > 0 && explicitTimeout < budget {
		return explicitTimeout
	}
	return budget
}

// scopeMutationGit binds the mutation-phase Git surface: when the adapter
// carries the operation-timeout capability, the derived budget applies to a
// scoped surface; otherwise the configured surface runs unchanged.
func scopeMutationGit(git port.GitRepository, budget time.Duration) port.GitRepository {
	if scoper, ok := git.(port.OperationTimeoutScoper); ok {
		return scoper.WithOperationTimeout(budget)
	}
	return git
}

// compensateGenesisAbortion restores the proven unborn pre-state after an
// aborted genesis step. A successful restoration returns the original cause
// with the compensation fact added to its diagnostic; a failed restoration
// returns a blocking record that names the partial state and its recovery,
// wrapping the original cause. The restoration runs on a detached context, so
// a cancelled caller cannot skip the restoration of the proven pre-state. A
// cause without the typed problem contract carries no diagnostic surface and
// is returned unchanged.
func compensateGenesisAbortion(ctx context.Context, restorer port.UnbornStateRestorer, repository port.RepositoryIdentity, cause error) error {
	if err := restorer.RestoreUnbornState(context.WithoutCancel(ctx), repository); err != nil {
		return problem.Wrap(problem.Details{
			Code:        problem.CodeInternal,
			Category:    problem.CategoryInternal,
			Field:       "genesis compensation",
			Diagnostic:  err.Error(),
			Expected:    "the proven unborn pre-state: empty index, no references, unborn HEAD",
			Rule:        "a failed compensation leaves the partial birth in place and blocks the retry",
			Remediation: "empty the index, delete the born refs, and retry the bootstrap",
		}, cause)
	}
	if typed, ok := problem.As(cause); ok {
		details := typed.Details
		if details.Diagnostic != "" {
			details.Diagnostic += "; "
		}
		details.Diagnostic += "the aborted birth was compensated back to the proven unborn pre-state (empty index, no references, unborn HEAD); the retry is idempotent"
		return problem.Wrap(details, cause)
	}
	return cause
}
