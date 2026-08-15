package branchapp

import (
	"context"
	"io"
	"regexp"
	"strings"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

const maxPrePushInputBytes = 1 << 20

var objectIDPattern = regexp.MustCompile(`^(?:[0-9a-fA-F]{40}|[0-9a-fA-F]{64})$`)

// PushAction identifies the Git operation represented by a pre-push update.
type PushAction string

const (
	PushActionCreate PushAction = "create"
	PushActionUpdate PushAction = "update"
	PushActionDelete PushAction = "delete"
	PushActionOther  PushAction = "other"
)

// PushUpdate is a validated line from Git's pre-push stdin protocol. Target is
// populated only for a canonical branch ref under refs/heads/.
type PushUpdate struct {
	LocalRef       string
	LocalObjectID  string
	RemoteRef      string
	RemoteObjectID string
	Target         branch.BranchName
	Action         PushAction
	GovernedBranch bool
}

// PrePushUpdateResult describes one checked outgoing ref. Non-branch refs are
// represented explicitly as skipped instead of being silently discarded.
type PrePushUpdateResult struct {
	Update             PushUpdate
	Publication        branch.PublicationState
	Base               *branch.TargetBase
	MissingBaseCommits bool
	FastForward        bool
	Skipped            bool
}

// PrePushBatchResult contains the per-ref governance decisions and the
// quality-gate outcome shared by the complete Git pre-push invocation.
type PrePushBatchResult struct {
	Updates []PrePushUpdateResult
	Quality port.QualityResult
}

// ParsePrePushUpdates parses Git's four-field, line-delimited pre-push input:
//
//	<local-ref> <local-object-id> <remote-ref> <remote-object-id>
//
// The function accepts both SHA-1 and SHA-256 object IDs. It intentionally
// parses every line before any policy decision so a multi-ref push cannot
// bypass validation through the currently checked-out branch.
func ParsePrePushUpdates(reader io.Reader) ([]PushUpdate, error) {
	if reader == nil {
		return nil, nil
	}
	contents, err := io.ReadAll(io.LimitReader(reader, maxPrePushInputBytes+1))
	if err != nil {
		return nil, problem.Wrap(problem.Details{
			Code:        problem.CodeExternalCommandFailed,
			Category:    problem.CategoryExternal,
			Field:       "pre-push input",
			Expected:    "readable Git hook input",
			Rule:        "pre-push validation must read every outgoing ref update",
			Remediation: "retry the push from a working Git hook environment",
		}, err)
	}
	if len(contents) > maxPrePushInputBytes {
		return nil, invalidPushInput(
			"pre-push input must not exceed 1 MiB",
			"split an unusually large push or investigate the invoking hook",
		)
	}

	raw := strings.ReplaceAll(string(contents), "\r\n", "\n")
	if strings.ContainsRune(raw, '\r') {
		return nil, invalidPushInput(
			"pre-push input must use LF or CRLF line endings",
			"retry with a Git-compatible hook input stream",
		)
	}
	raw = strings.TrimSuffix(raw, "\n")
	if raw == "" {
		return nil, nil
	}

	lines := strings.Split(raw, "\n")
	updates := make([]PushUpdate, 0, len(lines))
	for _, line := range lines {
		update, err := parsePrePushUpdate(line)
		if err != nil {
			return nil, err
		}
		updates = append(updates, update)
	}
	return updates, nil
}

func parsePrePushUpdate(line string) (PushUpdate, error) {
	if line == "" {
		return PushUpdate{}, invalidPushInput(
			"pre-push input must not contain empty update lines",
			"retry the push through Git rather than supplying a handcrafted update stream",
		)
	}
	fields := strings.Split(line, " ")
	if len(fields) != 4 || anyEmpty(fields) {
		return PushUpdate{}, invalidPushInput(
			"each pre-push update must contain exactly four space-delimited fields",
			"provide <local-ref> <local-object-id> <remote-ref> <remote-object-id>",
		)
	}
	if !objectIDPattern.MatchString(fields[1]) || !objectIDPattern.MatchString(fields[3]) {
		return PushUpdate{}, invalidPushInput(
			"pre-push object IDs must be complete SHA-1 or SHA-256 hexadecimal IDs",
			"retry through Git so it supplies complete object IDs",
		)
	}
	if !strings.HasPrefix(fields[2], "refs/") {
		return PushUpdate{}, invalidPushInput(
			"the remote ref must start with refs/",
			"retry through Git so it supplies a canonical remote ref",
		)
	}

	update := PushUpdate{
		LocalRef:       fields[0],
		LocalObjectID:  strings.ToLower(fields[1]),
		RemoteRef:      fields[2],
		RemoteObjectID: strings.ToLower(fields[3]),
		Action:         pushAction(fields[1], fields[3]),
	}
	if update.Action == PushActionDelete && update.LocalRef != "(delete)" {
		return PushUpdate{}, invalidPushInput(
			"a deletion update must use the local ref marker (delete)",
			"retry the deletion through Git rather than supplying a handcrafted update stream",
		)
	}
	if update.Action != PushActionDelete && update.LocalRef == "(delete)" {
		return PushUpdate{}, invalidPushInput(
			"the local ref marker (delete) is valid only for deletion updates",
			"retry the push through Git with a valid local source ref",
		)
	}

	const headsPrefix = "refs/heads/"
	if !strings.HasPrefix(update.RemoteRef, headsPrefix) {
		update.Action = PushActionOther
		return update, nil
	}
	target, err := branch.ParseName(strings.TrimPrefix(update.RemoteRef, headsPrefix))
	if err != nil {
		return PushUpdate{}, err
	}
	update.Target = target
	update.GovernedBranch = true
	return update, nil
}

// ValidatePrePushUpdates refreshes remote-tracking references once, then
// validates every actual outgoing branch update. It never rebases, merges, or
// pushes.
func (synchronizer *Synchronizer) ValidatePrePushUpdates(
	ctx context.Context,
	repository port.RepositoryIdentity,
	updates []PushUpdate,
	explicitBase *branch.TargetBase,
) (PrePushBatchResult, error) {
	repository, err := normalizeRepository(repository)
	if err != nil {
		return PrePushBatchResult{}, err
	}
	if synchronizer.validator == nil || synchronizer.git == nil {
		return PrePushBatchResult{}, internalDependencyError("pre-push validation dependencies")
	}
	if len(updates) == 0 {
		return PrePushBatchResult{}, invalidPushInput(
			"at least one Git pre-push update is required",
			"run this mode from a Git pre-push hook or provide --branch for manual validation",
		)
	}
	if err := synchronizer.git.Fetch(ctx, repository); err != nil {
		return PrePushBatchResult{}, err
	}

	results := make([]PrePushUpdateResult, 0, len(updates))
	for _, update := range updates {
		result, err := synchronizer.validatePrePushUpdate(ctx, repository, update, explicitBase)
		if err != nil {
			return PrePushBatchResult{}, err
		}
		results = append(results, result)
	}
	quality, err := synchronizer.runQualityForUpdates(ctx, repository, results)
	if err != nil {
		return PrePushBatchResult{}, err
	}
	return PrePushBatchResult{
		Updates: results,
		Quality: quality,
	}, nil
}

func (synchronizer *Synchronizer) validatePrePushUpdate(
	ctx context.Context,
	repository port.RepositoryIdentity,
	update PushUpdate,
	explicitBase *branch.TargetBase,
) (PrePushUpdateResult, error) {
	result := PrePushUpdateResult{
		Update:      update,
		FastForward: true,
	}
	if !update.GovernedBranch {
		result.Skipped = true
		return result, nil
	}
	if update.Target.Family().IsSharedLine() {
		if _, err := synchronizer.validator.Validate(ctx, ValidateRequest{
			Repository: repository,
			Name:       update.Target,
		}); err != nil {
			return PrePushUpdateResult{}, err
		}
		return PrePushUpdateResult{}, sharedLinePushForbidden(update)
	}
	if update.Target.Family() != branch.FamilyScratch && !update.Target.Family().IsOfficialWorkingBranch() {
		return PrePushUpdateResult{}, unsupportedSyncFamily(update.Target)
	}
	if _, err := synchronizer.validator.Validate(ctx, ValidateRequest{
		Repository: repository,
		Name:       update.Target,
	}); err != nil {
		return PrePushUpdateResult{}, err
	}
	if update.Action == PushActionDelete {
		result.Publication = branch.PublicationPublished
		return result, nil
	}
	if update.Target.Family() == branch.FamilyScratch {
		result.Publication = branch.PublicationUnknown
		return result, nil
	}
	baseInput, err := synchronizer.workflowBase(ctx, repository, update.Target, explicitBase)
	if err != nil {
		return PrePushUpdateResult{}, err
	}
	base, err := resolveSyncBase(update.Target, repository, baseInput, false)
	if err != nil {
		return PrePushUpdateResult{}, err
	}
	result.Base = &base
	result.Publication = publicationFromPushAction(update.Action)

	inspection, err := synchronizer.git.InspectPushUpdate(
		ctx,
		repository,
		base,
		update.LocalObjectID,
		update.RemoteObjectID,
	)
	if err != nil {
		return PrePushUpdateResult{}, err
	}
	result.MissingBaseCommits = inspection.MissingBaseCommits
	result.FastForward = inspection.FastForward
	if update.Action == PushActionUpdate && !inspection.FastForward {
		return PrePushUpdateResult{}, forcePushForbidden(update)
	}
	if result.Publication == branch.PublicationUnpublished && inspection.MissingBaseCommits {
		return PrePushUpdateResult{}, firstPushBaseStale(base)
	}
	if err := ValidateCommitSeries(update.Target, inspection.CommitMessages); err != nil {
		return PrePushUpdateResult{}, err
	}
	return result, nil
}

func (synchronizer *Synchronizer) runQualityForUpdates(
	ctx context.Context,
	repository port.RepositoryIdentity,
	results []PrePushUpdateResult,
) (port.QualityResult, error) {
	if synchronizer.finalQuality != nil {
		return synchronizer.finalQuality.ValidateOrRunForUpdates(ctx, repository, results)
	}
	families := make([]branch.Family, 0, len(results))
	for _, result := range results {
		if !result.Update.GovernedBranch || result.Update.Action == PushActionDelete {
			continue
		}
		families = append(families, result.Update.Target.Family())
	}
	if len(families) == 0 {
		return port.QualityResult{
			Status: port.QualitySkipped,
			Detail: "no outgoing governed branch update requires quality gates",
		}, nil
	}
	return synchronizer.runQuality(ctx, repository, families...)
}

// ValidateCommitSeries confirms that every outgoing commit belongs to the
// official ticket branch. It is shared by ticket publication and pre-push
// validation so the ticket rule has one implementation.
func ValidateCommitSeries(name branch.BranchName, messages []string) error {
	if !name.Family().IsOfficialWorkingBranch() {
		return nil
	}
	if len(messages) == 0 {
		return problem.New(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryGovernance,
			Field:       "commit series",
			Expected:    "at least one governed commit on the ticket branch",
			Rule:        "a pushable official branch requires reviewable ticket work",
			Example:     "feat(ABC-123): add export button",
			Remediation: "create and validate at least one commit before pushing the branch",
		})
	}
	branchTicket, _ := name.Ticket()
	for _, raw := range messages {
		message, err := commitmsg.Parse(raw)
		if err != nil {
			return err
		}
		if message.Header().Ticket().String() != branchTicket.String() {
			return problem.New(problem.Details{
				Code:        problem.CodeCommitTicketMismatch,
				Category:    problem.CategoryGovernance,
				Field:       "commit ticket",
				Actual:      message.Header().Ticket().String(),
				Expected:    branchTicket.String(),
				Rule:        "every commit in an official ticket branch uses the branch ticket",
				Example:     "feat(" + branchTicket.String() + "): add export button",
				Remediation: "split unrelated work into its own branch or correct the commit message before pushing",
			})
		}
	}
	return nil
}

func resolveSyncBase(
	name branch.BranchName,
	repository port.RepositoryIdentity,
	explicit *branch.TargetBase,
	workflowManaged bool,
) (branch.TargetBase, error) {
	if defaultBase, found, err := name.Family().DefaultTargetBase(repository.Remote); err != nil {
		return branch.TargetBase{}, err
	} else if found {
		if explicit != nil && explicit.String() != defaultBase.String() {
			if workflowManaged &&
				(name.Family() == branch.FamilyFix ||
					name.Family() == branch.FamilyDocs ||
					name.Family() == branch.FamilyChore) {
				if err := validateSpecialBase(name.Family(), *explicit); err != nil {
					return branch.TargetBase{}, err
				}
				return *explicit, nil
			}
			return branch.TargetBase{}, invalidBase(
				explicit.String(),
				"regular ticket work must synchronize against the configured remote develop branch",
				defaultBase.String(),
			)
		}
		return defaultBase, nil
	}
	if explicit == nil {
		return branch.TargetBase{}, invalidBase(
			"",
			"this branch family needs an explicit target base for synchronization",
			"origin/main, origin/release/<semver>, origin/support/<major.minor>, or origin/<official-ticket-branch>",
		)
	}
	if err := validateSpecialBase(name.Family(), *explicit); err != nil {
		return branch.TargetBase{}, err
	}
	return *explicit, nil
}

func publicationFromPushAction(action PushAction) branch.PublicationState {
	if action == PushActionCreate {
		return branch.PublicationUnpublished
	}
	return branch.PublicationPublished
}

func pushAction(localObjectID, remoteObjectID string) PushAction {
	if zeroObjectID(localObjectID) {
		return PushActionDelete
	}
	if zeroObjectID(remoteObjectID) {
		return PushActionCreate
	}
	return PushActionUpdate
}

func zeroObjectID(value string) bool {
	return value != "" && strings.Trim(value, "0") == ""
}

func anyEmpty(values []string) bool {
	for _, value := range values {
		if value == "" {
			return true
		}
	}
	return false
}

func sharedLinePushForbidden(update PushUpdate) error {
	return problem.New(problem.Details{
		Code:        problem.CodeSharedLineMutationForbidden,
		Category:    problem.CategoryGovernance,
		Field:       "push target",
		Actual:      update.RemoteRef,
		Expected:    "a pull request into a shared line",
		Rule:        "developers do not directly create, update, or delete main, develop, release, or support lines",
		Example:     "git push origin feature/ABC-123-add-export",
		Remediation: "push an official working branch and open a pull request for the shared line",
	})
}

func forcePushForbidden(update PushUpdate) error {
	return problem.New(problem.Details{
		Code:        problem.CodeForcePushForbidden,
		Category:    problem.CategoryGovernance,
		Field:       "push target",
		Actual:      update.RemoteRef,
		Expected:    "a fast-forward update for an official working branch",
		Rule:        "published official branches are append-only and cannot be force-pushed",
		Example:     "git push origin " + update.Target.String(),
		Remediation: "add a new commit or use a controlled merge instead of rewriting history",
	})
}

func firstPushBaseStale(base branch.TargetBase) error {
	return problem.New(problem.Details{
		Code:        problem.CodeBranchBaseInvalid,
		Category:    problem.CategoryGovernance,
		Field:       "target base",
		Actual:      base.String(),
		Expected:    "an unpublished branch based on the latest target base",
		Rule:        "before the first push, a branch with missing base commits must be rebased",
		Example:     "git rebase " + base.String(),
		Remediation: "run branch sync-base --strategy rebase, rerun validation, then push again",
	})
}

func invalidPushInput(rule, remediation string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeInvalidInput,
		Category:    problem.CategoryUsage,
		Field:       "pre-push input",
		Expected:    "Git's four-field pre-push update protocol",
		Rule:        rule,
		Example:     "refs/heads/feature/ABC-123-add-export <local-oid> refs/heads/feature/ABC-123-add-export <remote-oid>",
		Remediation: remediation,
	})
}
