package branchapp

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// BirthTopologyCapabilities binds the optional adapter capabilities the
// birth-topology proof consumes: the head commit counter, the revision
// resolver, the commit signature verifier, and the hook boundary inspector.
type BirthTopologyCapabilities struct {
	Commits   port.HeadCommitCounter
	Revisions port.RevisionResolver
	Verifier  port.CommitSignatureVerifier
	Hooks     port.HookBoundaryInspector
}

// ResolveBirthTopologyCapabilities resolves the optional adapter capabilities
// the birth-topology proof consumes. The governed birth finalizer, the
// publication resume, and the pre-push recognition all require the composed
// adapter to carry this exact set.
func ResolveBirthTopologyCapabilities(git port.GitRepository) (BirthTopologyCapabilities, error) {
	commits, ok := git.(port.HeadCommitCounter)
	if !ok {
		return BirthTopologyCapabilities{}, birthTopologyCapabilityRequired("head commit counting")
	}
	revisions, ok := git.(port.RevisionResolver)
	if !ok {
		return BirthTopologyCapabilities{}, birthTopologyCapabilityRequired("revision resolution")
	}
	verifier, ok := git.(port.CommitSignatureVerifier)
	if !ok {
		return BirthTopologyCapabilities{}, birthTopologyCapabilityRequired("commit signature verification")
	}
	hooks, ok := git.(port.HookBoundaryInspector)
	if !ok {
		return BirthTopologyCapabilities{}, birthTopologyCapabilityRequired("hook boundary inspection")
	}
	return BirthTopologyCapabilities{
		Commits:   commits,
		Revisions: revisions,
		Verifier:  verifier,
		Hooks:     hooks,
	}, nil
}

// ProveBirthTopology re-proves the complete birth topology at finalizer
// grade: exactly one commit reachable from HEAD, both shared lines existing
// and pointing at that shared genesis revision, the verified genesis
// signature, and the materialized hook boundary. It is the single
// implementation of the birth predicate; the governed birth finalizer, the
// publication resume, and the pre-push birth recognition consume it.
func ProveBirthTopology(
	ctx context.Context,
	git port.GitRepository,
	validator *Service,
	repository port.RepositoryIdentity,
	capabilities BirthTopologyCapabilities,
) (string, error) {
	hasCommits, err := git.HasCommits(ctx, repository)
	if err != nil {
		return "", err
	}
	if !hasCommits {
		return "", birthStateInvalid("the repository HEAD carries no commit")
	}
	commits, err := capabilities.Commits.CountHeadCommits(ctx, repository)
	if err != nil {
		return "", err
	}
	if commits != 1 {
		return "", birthStateInvalid(strconv.Itoa(commits) + " commits are reachable from HEAD")
	}
	revision, err := capabilities.Revisions.ResolveRevision(ctx, repository, "HEAD")
	if err != nil {
		return "", err
	}
	for _, line := range []branch.BranchName{mustBirthLine("main"), mustBirthLine("develop")} {
		if _, err := validator.Validate(ctx, ValidateRequest{
			Repository: repository,
			Name:       line,
		}); err != nil {
			return "", err
		}
		exists, err := git.BranchExists(ctx, repository, line)
		if err != nil {
			return "", err
		}
		if !exists {
			return "", birthStateInvalid("the born ref " + line.String() + " does not exist")
		}
		lineRevision, err := capabilities.Revisions.ResolveRevision(ctx, repository, line.String())
		if err != nil {
			return "", err
		}
		if lineRevision != revision {
			return "", birthStateInvalid("the born ref " + line.String() + " points at " + lineRevision + " instead of the shared genesis revision " + revision)
		}
	}
	if err := capabilities.Verifier.VerifyCommitSignature(ctx, repository, revision); err != nil {
		return "", err
	}
	present, err := capabilities.Hooks.HookBoundaryPresent(ctx, repository)
	if err != nil {
		return "", err
	}
	if !present {
		return "", birthStateInvalid("the installed hook boundary is not materialized")
	}
	return revision, nil
}

// recognizeBirthPublication attempts the read-only birth recognition of the
// pre-push boundary. It returns recognized=true only when the update batch
// exclusively creates the born shared lines, the adapter carries the proof
// capabilities, and the shared birth predicate proves the exact genesis
// state with the pushed object IDs. Every other outcome leaves the push
// under the unchanged shared-line guard: a proof-state mismatch fails closed
// to the forbidden verdict, a capability gap fails closed the same way, and
// a proof read failure propagates so the hook never fails open.
func (synchronizer *Synchronizer) recognizeBirthPublication(
	ctx context.Context,
	repository port.RepositoryIdentity,
	updates []PushUpdate,
) (PrePushBatchResult, bool, error) {
	if !birthPublicationShape(updates) {
		return PrePushBatchResult{}, false, nil
	}
	capabilities, err := ResolveBirthTopologyCapabilities(synchronizer.git)
	if err != nil {
		return PrePushBatchResult{}, false, nil
	}
	revision, err := ProveBirthTopology(ctx, synchronizer.git, synchronizer.validator, repository, capabilities)
	if err != nil {
		if birthStateMismatch(err) {
			return PrePushBatchResult{}, false, nil
		}
		return PrePushBatchResult{}, false, err
	}
	for _, update := range updates {
		if !strings.EqualFold(update.LocalObjectID, revision) {
			return PrePushBatchResult{}, false, nil
		}
	}
	results := make([]PrePushUpdateResult, 0, len(updates))
	for _, update := range updates {
		results = append(results, PrePushUpdateResult{
			Update:      update,
			Publication: branch.PublicationUnpublished,
			FastForward: true,
		})
	}
	return PrePushBatchResult{
		Updates: results,
		Quality: port.QualityResult{
			Status: port.QualitySkipped,
			Detail: "the governed remote birth is validated by its birth evidence: born refs main and develop on the shared genesis revision " + revision + ", verified genesis signature, materialized hook boundary",
		},
	}, true, nil
}

// birthPublicationShape reports whether the update batch exclusively creates
// the born shared lines: every governed update targets main or develop as a
// creation and no other ref is part of the batch. Anything else stays under
// the unchanged shared-line guard.
func birthPublicationShape(updates []PushUpdate) bool {
	if len(updates) == 0 {
		return false
	}
	for _, update := range updates {
		if !update.GovernedBranch {
			return false
		}
		family := update.Target.Family()
		if family != branch.FamilyMain && family != branch.FamilyDevelop {
			return false
		}
		if update.Action != PushActionCreate {
			return false
		}
	}
	return true
}

// birthStateMismatch classifies the controlled refusal of the birth
// predicate: a proven state outside the exact genesis topology keeps the
// push under the unchanged shared-line guard instead of surfacing the
// refusal as a new failure class.
func birthStateMismatch(err error) bool {
	var governed *problem.Problem
	if errors.As(err, &governed) {
		return governed.Code == problem.CodeBirthStateInvalid
	}
	return false
}

// birthStateInvalid refuses a birth publication outside the proven birth
// topology: the shared lines publish only from the exact genesis state.
func birthStateInvalid(detail string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeBirthStateInvalid,
		Category:    problem.CategoryRepository,
		Field:       "repository",
		Actual:      detail,
		Expected:    "the proven birth topology: exactly one commit, main and develop on the shared genesis revision, verified signature, materialized hook boundary",
		Rule:        "the shared lines publish only from the proven genesis state",
		Remediation: "review the repository state; the shared lines publish only from the proven genesis state",
	})
}

// birthTopologyCapabilityRequired refuses the birth-topology proof on an
// adapter that lacks one of the composed capabilities.
func birthTopologyCapabilityRequired(capability string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeConfigurationUnavailable,
		Category:    problem.CategoryConfig,
		Field:       "Git adapter",
		Expected:    "a Git adapter with " + capability + " capability",
		Rule:        "the birth-topology proof requires the composed adapter capabilities",
		Remediation: "use the composed Git adapter of the CLI",
	})
}

// mustBirthLine parses one of the product's fixed shared-line literals; the
// branch domain tests independently validate the taxonomy.
func mustBirthLine(name string) branch.BranchName {
	parsed, _ := branch.ParseName(name)
	return parsed
}
