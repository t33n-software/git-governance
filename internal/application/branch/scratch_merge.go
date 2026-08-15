package branchapp

import (
	"context"
	"sort"
	"strings"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

// ScratchMerger transfers stable private exploration to its official ticket
// branch as one governed squash commit.
type ScratchMerger struct {
	git       port.GitRepository
	validator *Service
}

// NewScratchMerger creates the application service for scratch transfers.
func NewScratchMerger(git port.GitRepository, validator *Service) *ScratchMerger {
	return &ScratchMerger{
		git:       git,
		validator: validator,
	}
}

// ScratchMergeRequest describes a transfer from a local scratch branch to its
// local official ticket branch. Target is optional only when exactly one local
// official branch carries the scratch ticket.
type ScratchMergeRequest struct {
	Repository port.RepositoryIdentity
	Source     branch.BranchName
	Target     *branch.BranchName
	Message    commitmsg.Message
	DryRun     bool
}

// ScratchMergeResult describes the executed or planned squash transfer.
type ScratchMergeResult struct {
	Source    branch.BranchName
	Target    branch.BranchName
	Message   commitmsg.Message
	Committed bool
	DryRun    bool
	Plan      []PlanStep
}

// ResolveTarget resolves a scratch branch to one local official ticket branch.
// It deliberately uses the ticket ID rather than the slug because scratch and
// official branch descriptions are allowed to differ.
func (merger *ScratchMerger) ResolveTarget(
	ctx context.Context,
	repository port.RepositoryIdentity,
	source branch.BranchName,
	explicit *branch.BranchName,
) (branch.BranchName, error) {
	repository, err := normalizeRepository(repository)
	if err != nil {
		return branch.BranchName{}, err
	}
	if err := contextError(ctx); err != nil {
		return branch.BranchName{}, err
	}
	if merger == nil || merger.git == nil {
		return branch.BranchName{}, internalDependencyError("Git repository")
	}

	sourceTicket, err := scratchTicket(source)
	if err != nil {
		return branch.BranchName{}, err
	}
	sourceExists, err := merger.git.BranchExists(ctx, repository, source)
	if err != nil {
		return branch.BranchName{}, err
	}
	if !sourceExists {
		return branch.BranchName{}, scratchSourceMissing(source)
	}

	if explicit != nil {
		if err := validateScratchTarget(sourceTicket.String(), *explicit); err != nil {
			return branch.BranchName{}, err
		}
		exists, err := merger.git.BranchExists(ctx, repository, *explicit)
		if err != nil {
			return branch.BranchName{}, err
		}
		if !exists {
			return branch.BranchName{}, scratchTargetMissing(sourceTicket.String(), explicit.String())
		}
		return *explicit, nil
	}

	candidates, err := merger.git.OfficialBranchesForTicket(ctx, repository, sourceTicket)
	if err != nil {
		return branch.BranchName{}, err
	}
	local := make([]branch.BranchName, 0, len(candidates))
	for _, candidate := range candidates {
		if err := validateScratchTarget(sourceTicket.String(), candidate); err != nil {
			return branch.BranchName{}, err
		}
		exists, err := merger.git.BranchExists(ctx, repository, candidate)
		if err != nil {
			return branch.BranchName{}, err
		}
		if exists {
			local = append(local, candidate)
		}
	}
	if len(local) == 0 {
		return branch.BranchName{}, scratchTargetMissing(sourceTicket.String(), "")
	}
	if len(local) == 1 {
		return local[0], nil
	}

	names := make([]string, 0, len(local))
	for _, candidate := range local {
		names = append(names, candidate.String())
	}
	sort.Strings(names)
	return branch.BranchName{}, scratchTargetAmbiguous(sourceTicket.String(), names)
}

// Merge validates the source, target, and commit before switching to the
// target, applying Git's squash merge, and creating exactly one commit.
func (merger *ScratchMerger) Merge(ctx context.Context, request ScratchMergeRequest) (ScratchMergeResult, error) {
	repository, err := normalizeRepository(request.Repository)
	if err != nil {
		return ScratchMergeResult{}, err
	}
	if err := contextError(ctx); err != nil {
		return ScratchMergeResult{}, err
	}
	if merger == nil || merger.git == nil {
		return ScratchMergeResult{}, internalDependencyError("Git repository")
	}
	if merger.validator == nil {
		return ScratchMergeResult{}, internalDependencyError("branch validator")
	}

	target, err := merger.ResolveTarget(ctx, repository, request.Source, request.Target)
	if err != nil {
		return ScratchMergeResult{}, err
	}
	if err := ValidateScratchMergeMessage(target, request.Message); err != nil {
		return ScratchMergeResult{}, err
	}
	if _, err := merger.validator.Validate(ctx, ValidateRequest{
		Repository: repository,
		Name:       request.Source,
	}); err != nil {
		return ScratchMergeResult{}, err
	}
	if _, err := merger.validator.Validate(ctx, ValidateRequest{
		Repository: repository,
		Name:       target,
	}); err != nil {
		return ScratchMergeResult{}, err
	}

	result := ScratchMergeResult{
		Source:  request.Source,
		Target:  target,
		Message: request.Message,
		DryRun:  request.DryRun,
		Plan: []PlanStep{
			{Action: "switch", Detail: "switch to " + target.String()},
			{Action: "squash-merge", Detail: request.Source.String() + " into " + target.String()},
			{Action: "commit", Detail: request.Message.Header().String()},
		},
	}
	if request.DryRun {
		return result, nil
	}

	clean, err := merger.git.IsWorktreeClean(ctx, repository)
	if err != nil {
		return ScratchMergeResult{}, err
	}
	if !clean {
		return ScratchMergeResult{}, worktreeNotCleanForSync()
	}
	if err := merger.git.SwitchBranch(ctx, repository, target); err != nil {
		return ScratchMergeResult{}, err
	}
	if err := merger.git.SquashMerge(ctx, repository, request.Source); err != nil {
		conflicted, conflictErr := merger.git.HasUnmergedConflicts(ctx, repository)
		if conflictErr != nil {
			return ScratchMergeResult{}, conflictErr
		}
		if conflicted {
			return ScratchMergeResult{}, scratchMergeConflict(request.Source, target, err)
		}
		return ScratchMergeResult{}, err
	}
	staged, err := merger.git.HasStagedChanges(ctx, repository)
	if err != nil {
		return ScratchMergeResult{}, err
	}
	if !staged {
		return ScratchMergeResult{}, scratchMergeEmpty(request.Source, target)
	}
	if err := merger.git.Commit(ctx, repository, request.Message); err != nil {
		return ScratchMergeResult{}, err
	}
	result.Committed = true
	return result, nil
}

// Resume finishes a previously conflicted scratch squash transfer after the
// user resolves and stages every conflicting file. It never runs a second
// squash merge and therefore preserves the original workflow context.
func (merger *ScratchMerger) Resume(ctx context.Context, request ScratchMergeRequest) (ScratchMergeResult, error) {
	repository, err := normalizeRepository(request.Repository)
	if err != nil {
		return ScratchMergeResult{}, err
	}
	if err := contextError(ctx); err != nil {
		return ScratchMergeResult{}, err
	}
	if merger == nil || merger.git == nil {
		return ScratchMergeResult{}, internalDependencyError("Git repository")
	}
	if merger.validator == nil {
		return ScratchMergeResult{}, internalDependencyError("branch validator")
	}

	target, err := merger.ResolveTarget(ctx, repository, request.Source, request.Target)
	if err != nil {
		return ScratchMergeResult{}, err
	}
	if err := ValidateScratchMergeMessage(target, request.Message); err != nil {
		return ScratchMergeResult{}, err
	}
	if _, err := merger.validator.Validate(ctx, ValidateRequest{Repository: repository, Name: request.Source}); err != nil {
		return ScratchMergeResult{}, err
	}
	if _, err := merger.validator.Validate(ctx, ValidateRequest{Repository: repository, Name: target}); err != nil {
		return ScratchMergeResult{}, err
	}

	result := ScratchMergeResult{
		Source:  request.Source,
		Target:  target,
		Message: request.Message,
		DryRun:  request.DryRun,
		Plan: []PlanStep{
			{Action: "resolve-conflicts", Detail: "resolve and stage conflicts from " + request.Source.String()},
			{Action: "commit", Detail: request.Message.Header().String()},
		},
	}
	if request.DryRun {
		return result, nil
	}
	conflicted, err := merger.git.HasUnmergedConflicts(ctx, repository)
	if err != nil {
		return ScratchMergeResult{}, err
	}
	if conflicted {
		return ScratchMergeResult{}, scratchMergeConflict(request.Source, target, nil)
	}
	staged, err := merger.git.HasStagedChanges(ctx, repository)
	if err != nil {
		return ScratchMergeResult{}, err
	}
	if !staged {
		return ScratchMergeResult{}, scratchMergeEmpty(request.Source, target)
	}
	if err := merger.git.Commit(ctx, repository, request.Message); err != nil {
		return ScratchMergeResult{}, err
	}
	result.Committed = true
	return result, nil
}

// ValidateScratchMergeMessage ensures the generated squash commit belongs to
// the target ticket branch before Git state is changed.
func ValidateScratchMergeMessage(target branch.BranchName, message commitmsg.Message) error {
	if message.Header().Type() == "" {
		return problem.New(problem.Details{
			Code:        problem.CodeCommitHeaderInvalid,
			Category:    problem.CategoryGovernance,
			Field:       "squash commit message",
			Expected:    "a validated ticket-scoped Conventional Commit message",
			Rule:        "a scratch transfer creates one governed squash commit",
			Example:     "feat(ABC-123): add export button",
			Remediation: "provide a complete Conventional Commit message for the squashed change",
		})
	}
	targetTicket, hasTargetTicket := target.Ticket()
	if !hasTargetTicket || !target.Family().IsOfficialWorkingBranch() {
		return problem.New(problem.Details{
			Code:        problem.CodeBranchBaseInvalid,
			Category:    problem.CategoryGovernance,
			Field:       "scratch target",
			Actual:      target.String(),
			Expected:    "an official ticket branch",
			Rule:        "scratch work is transferred only to its official ticket branch",
			Example:     "feature/ABC-123-add-export",
			Remediation: "select the existing official branch for the scratch ticket",
		})
	}
	if message.Header().Ticket().String() != targetTicket.String() {
		return problem.New(problem.Details{
			Code:        problem.CodeCommitTicketMismatch,
			Category:    problem.CategoryGovernance,
			Field:       "squash commit ticket",
			Actual:      message.Header().Ticket().String(),
			Expected:    targetTicket.String(),
			Rule:        "the squashed commit uses the ticket of its official target branch",
			Example:     "feat(" + targetTicket.String() + "): add export button",
			Remediation: "use the target branch ticket in the squash commit message",
		})
	}
	return nil
}

func scratchTicket(source branch.BranchName) (ticket.ID, error) {
	if source.Family() != branch.FamilyScratch {
		return ticket.ID{}, problem.New(problem.Details{
			Code:        problem.CodeBranchFamilyInvalid,
			Category:    problem.CategoryGovernance,
			Field:       "scratch branch",
			Actual:      source.String(),
			Expected:    "a scratch/<ticket>-<slug> branch",
			Rule:        "only private scratch branches can be squashed into an official ticket branch",
			Example:     "scratch/ABC-123-export-exploration",
			Remediation: "switch to a scratch branch or use ticket publish directly from the official branch",
		})
	}
	ticketID, _ := source.Ticket()
	return ticketID, nil
}

func validateScratchTarget(sourceTicket string, target branch.BranchName) error {
	if !target.Family().IsOfficialWorkingBranch() {
		return problem.New(problem.Details{
			Code:        problem.CodeBranchBaseInvalid,
			Category:    problem.CategoryGovernance,
			Field:       "scratch target",
			Actual:      target.String(),
			Expected:    "a feature, fix, docs, refactor, chore, test, perf, or hotfix branch",
			Rule:        "scratch work is transferred only to an official ticket branch",
			Example:     "feature/" + sourceTicket + "-add-export",
			Remediation: "select the official branch for the scratch ticket",
		})
	}
	targetTicket, _ := target.Ticket()
	if targetTicket.String() != sourceTicket {
		return problem.New(problem.Details{
			Code:        problem.CodeBranchBaseInvalid,
			Category:    problem.CategoryGovernance,
			Field:       "scratch target",
			Actual:      target.String(),
			Expected:    "an official branch for ticket " + sourceTicket,
			Rule:        "scratch work cannot cross ticket boundaries",
			Example:     "feature/" + sourceTicket + "-add-export",
			Remediation: "select the official branch carrying the same ticket as the scratch branch",
		})
	}
	return nil
}

func scratchSourceMissing(source branch.BranchName) error {
	return problem.New(problem.Details{
		Code:        problem.CodeScratchSourceBranchMissing,
		Category:    problem.CategoryRepository,
		Field:       "scratch branch",
		Actual:      source.String(),
		Expected:    "an existing local scratch branch",
		Rule:        "a scratch transfer requires its local source branch",
		Example:     "scratch/ABC-123-export-exploration",
		Remediation: "switch to the existing scratch branch or supply it with --branch",
	})
}

func scratchTargetMissing(ticketID, target string) error {
	actual := ticketID
	if target != "" {
		actual = target
	}
	return problem.New(problem.Details{
		Code:        problem.CodeScratchTargetBranchMissing,
		Category:    problem.CategoryRepository,
		Field:       "official ticket branch",
		Actual:      actual,
		Expected:    "one existing local official branch for ticket " + ticketID,
		Rule:        "a scratch branch can be squashed only into an existing local official ticket branch",
		Example:     "feature/" + ticketID + "-add-export",
		Remediation: "create or fetch and check out the official branch first, then retry the scratch transfer",
	})
}

func scratchTargetAmbiguous(ticketID string, candidates []string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeScratchTargetBranchAmbiguous,
		Category:    problem.CategoryGovernance,
		Field:       "official ticket branch",
		Actual:      strings.Join(candidates, ", "),
		Expected:    "exactly one local official branch for ticket " + ticketID,
		Rule:        "scratch transfer must not guess between multiple official branches",
		Example:     "--target feature/" + ticketID + "-add-export",
		Remediation: "supply --target with the intended local official branch",
	})
}

func scratchMergeConflict(source, target branch.BranchName, cause error) error {
	return problem.Wrap(problem.Details{
		Code:        problem.CodeScratchMergeConflict,
		Category:    problem.CategoryGit,
		Field:       "scratch merge",
		Actual:      source.String() + " into " + target.String(),
		Expected:    "a squash merge without unresolved conflicts",
		Rule:        "scratch publication pauses while Git requires manual conflict resolution",
		Example:     "resolve conflicts, stage the resolutions, then select Retry",
		Remediation: "resolve and stage every conflicting file, then select Retry to finish the existing squash transfer",
	}, cause)
}

func scratchMergeEmpty(source, target branch.BranchName) error {
	return problem.New(problem.Details{
		Code:        problem.CodeScratchMergeEmpty,
		Category:    problem.CategoryGovernance,
		Field:       "scratch merge",
		Actual:      source.String(),
		Expected:    "at least one staged change after squashing into " + target.String(),
		Rule:        "a scratch transfer creates one commit only when it changes the official branch",
		Remediation: "add committed scratch changes or remove work already present on the official branch",
	})
}
