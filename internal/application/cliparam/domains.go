package cliparam

import (
	"strings"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	commitapp "github.com/t33n-software/git-governance/internal/application/commit"
	"github.com/t33n-software/git-governance/internal/application/workflow"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/releaserequest"
)

// This file holds the canonical descriptor of every CLI value domain. Closed
// value sets are derived from the domain registries at call time; endpoint
// subsets are produced through declared filters on those sources. Canonical
// conventions: docs/conventions/cli/value-domain-model.md and
// docs/conventions/cli/single-source-of-truth.md.

// CommitType is the closed-enum domain of the Conventional Commit family,
// derived from the commit message domain registry.
func CommitType() Domain {
	types := commitmsg.Types()
	values := make([]string, 0, len(types))
	for _, kind := range types {
		values = append(values, kind.String())
	}
	return Domain{
		Concept: "commit family",
		Class:   ClassClosedEnum,
		Values:  values,
		Example: "feat",
	}
}

// CommitTypeMetadata returns the commit families with their selection labels
// and descriptions for the discovery channel, rendered from the canonical
// commit-family surface.
func CommitTypeMetadata() []commitapp.Family {
	return commitapp.Families()
}

// BranchFamily is the closed-enum domain of the canonical branch taxonomy,
// derived from the branch domain registry.
func BranchFamily() Domain {
	families := branch.Families()
	values := make([]string, 0, len(families))
	for _, family := range families {
		values = append(values, family.String())
	}
	return Domain{
		Concept: "branch family",
		Class:   ClassClosedEnum,
		Values:  values,
		Example: "feature",
	}
}

// BranchFamilyMetadata returns every branch family with its catalog metadata
// for the discovery channel, rendered from the canonical branch catalog.
func BranchFamilyMetadata() []branchapp.FamilyInfo {
	return branchapp.ListFamilies()
}

// DirectlyCreatableBranchFamily is the endpoint subset of branch families
// that branch create and the ticket workflow accept, produced through the
// declared DirectlyCreatable filter on the canonical catalog.
func DirectlyCreatableBranchFamily() Domain {
	creatable := make([]string, 0, len(branchapp.ListFamilies()))
	bound := make([]string, 0, len(branchapp.ListFamilies()))
	for _, info := range branchapp.ListFamilies() {
		if info.DirectlyCreatable {
			creatable = append(creatable, info.Family.String())
		} else {
			bound = append(bound, info.Family.String())
		}
	}
	return Domain{
		Concept: "branch family",
		Class:   ClassClosedEnum,
		Values:  creatable,
		Rule: "not directly creatable: " + strings.Join(bound, ", ") +
			" (shared lines and special-flow families are created by their governed workflows)",
		Example: "feature",
	}
}

// SyncStrategy is the closed-enum domain of the base synchronization
// strategies, derived from the branch application service contract.
func SyncStrategy() Domain {
	return Domain{
		Concept: "sync strategy",
		Class:   ClassClosedEnum,
		Values: []string{
			string(branchapp.SyncCheck),
			string(branchapp.SyncAuto),
			string(branchapp.SyncRebase),
			string(branchapp.SyncMerge),
		},
	}
}

// ReleaseRequestKind is the closed-enum domain of protected line operations,
// derived from the release request domain.
func ReleaseRequestKind() Domain {
	return Domain{
		Concept: "protected line operation",
		Class:   ClassClosedEnum,
		Values: []string{
			string(releaserequest.OperationRelease),
			string(releaserequest.OperationSupport),
		},
	}
}

// ReleaseStabilizationKind is the closed-enum domain of change categories
// allowed on a frozen release line, derived from the workflow service.
func ReleaseStabilizationKind() Domain {
	return Domain{
		Concept: "stabilization kind",
		Class:   ClassClosedEnum,
		Values: []string{
			string(workflow.ReleaseStabilizationBlocker),
			string(workflow.ReleaseStabilizationDocs),
			string(workflow.ReleaseStabilizationPrep),
		},
	}
}

// ColorMode is the closed-enum domain of the global color mode.
func ColorMode() Domain {
	return Domain{
		Concept: "color mode",
		Class:   ClassClosedEnum,
		Values:  []string{"auto", "always", "never"},
	}
}

// InteractiveMode is the closed-enum domain of the global interaction mode,
// including the fail-closed combination rule with the JSON output mode.
func InteractiveMode() Domain {
	return Domain{
		Concept: "interactive mode",
		Class:   ClassClosedEnum,
		Values:  []string{"auto", "always", "never"},
		Rule:    "always is rejected when --output json is selected or no terminal is available",
	}
}

// OutputMode is the closed-enum domain of the global output mode, including
// the fail-closed combination rule with the forced interactive mode.
func OutputMode() Domain {
	return Domain{
		Concept: "output mode",
		Class:   ClassClosedEnum,
		Values:  []string{"human", "json"},
		Rule:    "json rejects --interactive always",
	}
}

// PullRequestProvider is the closed-enum domain of the hosting provider used
// for pull request publication.
func PullRequestProvider() Domain {
	return Domain{
		Concept: "pull request provider",
		Class:   ClassClosedEnum,
		Values:  []string{"none", "github"},
	}
}

// CompletionShell is the closed-enum domain of the shells supported by the
// completion script endpoint.
func CompletionShell() Domain {
	return Domain{
		Concept: "shell",
		Class:   ClassClosedEnum,
		Values:  []string{"bash", "zsh", "fish", "powershell"},
	}
}

// ReleaseVersion is the shaped domain of a release semantic version.
func ReleaseVersion() Domain {
	return Domain{
		Concept: "release semantic version",
		Class:   ClassShaped,
		Rule: "SemVer 2.0.0 without a leading v: major.minor.patch with optional " +
			"pre-release and build metadata (rejected otherwise)",
		Example: "2.8.0-rc.1",
	}
}

// SupportVersion is the shaped domain of a support line major.minor version.
func SupportVersion() Domain {
	return Domain{
		Concept: "support major.minor version",
		Class:   ClassShaped,
		Rule:    "major.minor without leading zeroes (rejected otherwise)",
		Example: "2.7",
	}
}

// ReleaseLine is the shaped domain of a release line reference.
func ReleaseLine() Domain {
	return Domain{
		Concept:  "release line",
		Class:    ClassShaped,
		Rule:     "release/<semver> with a SemVer 2.0.0 version without a leading v (rejected otherwise)",
		Example:  "release/2.8.0-rc.1",
		Prefixes: []string{"release/"},
	}
}

// AffectedLine is the shaped domain of the line affected by a hotfix.
func AffectedLine() Domain {
	return Domain{
		Concept:  "affected line",
		Class:    ClassShaped,
		Rule:     "main, release/<semver>, or support/<major.minor> (other lines are rejected)",
		Example:  "release/2.8.0",
		Prefixes: []string{"main", "release/", "support/"},
	}
}

// PropagationTargetLine is the shaped domain of the target line accepted by
// the single-commit hotfix propagation endpoint, which includes main.
func PropagationTargetLine() Domain {
	return Domain{
		Concept:  "target line",
		Class:    ClassShaped,
		Rule:     "main, develop, release/<semver>, or support/<major.minor> (other lines are rejected)",
		Example:  "support/2.7",
		Prefixes: []string{"main", "develop", "release/", "support/"},
	}
}

// ManifestTargetLine is the shaped domain of the target line accepted by the
// manifest-based hotfix propagation endpoint; main is record-bound and
// therefore excluded.
func ManifestTargetLine() Domain {
	return Domain{
		Concept:  "declared target line",
		Class:    ClassShaped,
		Rule:     "develop, release/<semver>, or support/<major.minor>; main is record-bound and rejected",
		Example:  "release/2.8.0",
		Prefixes: []string{"develop", "release/", "support/"},
	}
}

// TicketKey is the free-constrained domain of a ticket namespace, mirroring
// the ticket domain validation rule.
func TicketKey() Domain {
	return Domain{
		Concept: "ticket key",
		Class:   ClassFreeConstrained,
		Rule: "1-32 uppercase ASCII letters or digits, beginning with a letter " +
			"(rejected unless matching ^[A-Z][A-Z0-9]*$)",
		Example: "ABC",
	}
}

// TicketNumber is the free-constrained domain of a positive ticket number.
func TicketNumber() Domain {
	return Domain{
		Concept: "ticket number",
		Class:   ClassFreeConstrained,
		Rule:    "1-18 decimal digits without a leading zero (rejected unless matching ^[1-9][0-9]*$)",
		Example: "123",
	}
}

// TicketID is the free-constrained domain of the canonical KEY-NUMBER ticket
// identity.
func TicketID() Domain {
	return Domain{
		Concept: "ticket",
		Class:   ClassFreeConstrained,
		Rule:    "KEY-NUMBER with exactly one key-number separator (rejected otherwise)",
		Example: "ABC-123",
	}
}

// BranchSlug is the free-constrained domain of the kebab-case branch
// description, mirroring the branch domain validation rule.
func BranchSlug() Domain {
	return Domain{
		Concept: "kebab-case branch description",
		Class:   ClassFreeConstrained,
		Rule: "1-100 lowercase ASCII letters or digits, words joined by single hyphens " +
			"(rejected unless matching ^[a-z0-9]+(?:-[a-z0-9]+)*$)",
		Example: "add-export-button",
	}
}

// CommitSubject is the free-constrained domain of the envelope-free commit
// description. Length, padding, control characters, and the envelope ban are
// hard validation rejections; the filler formulas are a content convention
// that validation does not enforce — the label law keeps both distinguishable.
// Canonical convention: docs/conventions/commits/subject-contract.md.
func CommitSubject() Domain {
	return Domain{
		Concept: "commit description",
		Class:   ClassFreeConstrained,
		Rule: "1-200 runes, unpadded, free of control characters, and free of the " +
			"assembly-owned type(TICKET)[!]: envelope (rejected by validation)",
		Convention: "filler formulas without a named behavior such as update, fix stuff, changes, misc, or wip",
		Example:    "add export button",
	}
}

// CommitBody is the free-constrained domain of the commit body. The mandatory
// body contexts (breaking changes, hotfix lane, release stabilization,
// scratch squash transfer) are endpoint context added by the call site.
func CommitBody() Domain {
	return Domain{
		Concept: "commit body",
		Class:   ClassFreeConstrained,
		Rule:    "free-form text without control characters other than LF (rejected by validation)",
	}
}

// CommitFooter is the composite-token domain of a commit footer: the CLI
// transport form TOKEN=VALUE renders as TOKEN: value in the message artifact.
func CommitFooter() Domain {
	return Domain{
		Concept: "commit footer",
		Class:   ClassCompositeToken,
		Rule: "footer as TOKEN=VALUE on the CLI, rendered as TOKEN: value in the message; " +
			"token: letters, digits, and hyphens, or BREAKING CHANGE; value non-empty and unpadded; " +
			"repeatable (rejected by validation)",
		Example:  "Refs=#123",
		Prefixes: []string{"BREAKING CHANGE="},
	}
}

// BreakingDescription is the free-constrained domain of the breaking change
// migration impact.
func BreakingDescription() Domain {
	return Domain{
		Concept: "breaking change migration impact",
		Class:   ClassFreeConstrained,
		Rule:    "non-empty and unpadded, without control characters (rejected by validation)",
		Example: "clients must migrate to the new contract",
	}
}

// CommitSHA is the free-constrained domain of the bounded hexadecimal commit
// identifier accepted by the controlled hotfix propagation workflow.
func CommitSHA() Domain {
	return Domain{
		Concept: "commit",
		Class:   ClassFreeConstrained,
		Rule:    "7-64 hexadecimal characters (rejected unless matching ^[0-9a-fA-F]{7,64}$)",
		Example: "9fceb02",
	}
}

// RemoteName is the free-constrained domain of a Git remote name, mirroring
// the branch domain validation rule.
func RemoteName() Domain {
	return Domain{
		Concept: "Git remote name",
		Class:   ClassFreeConstrained,
		Rule: "letters, digits, dots, underscores, or hyphens, beginning with a letter or digit " +
			"(rejected unless matching ^[A-Za-z0-9][A-Za-z0-9._-]*$)",
		Example: "origin",
	}
}

// RequestID is the free-constrained domain of the durable protected-line
// request identifier.
func RequestID() Domain {
	return Domain{
		Concept: "durable protected-line request ID",
		Class:   ClassFreeConstrained,
		Rule:    "1-64 letters, digits, underscores, or hyphens (rejected unless matching ^[A-Za-z0-9_-]{1,64}$)",
	}
}

// RunID is the free-constrained domain of a provider workflow run identifier.
func RunID() Domain {
	return Domain{
		Concept: "workflow run ID",
		Class:   ClassFreeConstrained,
		Rule:    "a positive decimal number without a leading zero (rejected unless matching ^[1-9][0-9]*$)",
	}
}

// Requester is the free-constrained domain of the request-authority actor.
func Requester() Domain {
	return Domain{
		Concept: "request-authority actor",
		Class:   ClassFreeConstrained,
		Rule:    "a non-empty one-line identifier of at most 200 characters (rejected by validation)",
	}
}

// CommitMessage is the free-constrained domain of a complete commit message
// supplied out of band.
func CommitMessage() Domain {
	return Domain{
		Concept: "complete commit message",
		Class:   ClassFreeConstrained,
		Rule: "the full type(TICKET)[!]: subject grammar with optional body and footers " +
			"(rejected by validation)",
		Example: "feat(ABC-123): add export button",
	}
}

// BaseBranch is the structural-reference domain of an explicit synchronization
// or validation base. Existence is resolved at runtime; the help must not
// promise full prevention.
func BaseBranch() Domain {
	return Domain{
		Concept: "explicit base",
		Class:   ClassStructuralReference,
		Rule: "canonical branch name or <remote>/<branch> on the selected remote; " +
			"existence is resolved at runtime",
		Example: "origin/develop",
	}
}

// BranchReference is the structural-reference domain of a canonical branch
// name argument.
func BranchReference() Domain {
	return Domain{
		Concept: "branch",
		Class:   ClassStructuralReference,
		Rule: "a canonical branch name for a supported family: main, develop, release/<semver>, " +
			"support/<major.minor>, or <family>/<KEY>-<NUMBER>-<slug>; existence is resolved at runtime",
		Example: "feature/ABC-123-add-export-button",
	}
}

// RecordPath is the structural-reference domain of the repository-relative
// hotfix release record path.
func RecordPath() Domain {
	return Domain{
		Concept: "repository-relative hotfix release record",
		Class:   ClassStructuralReference,
		Rule:    "repository-relative path to the hotfix release record; existence is resolved at runtime",
		Example: ".git-governance/hotfix-release-records/ABC-123.json",
	}
}

// CommitMessageFile is the structural-reference domain of a file carrying a
// complete commit message.
func CommitMessageFile() Domain {
	return Domain{
		Concept: "file containing the complete commit message",
		Class:   ClassStructuralReference,
		Rule:    "repository file carrying the complete commit message, at most 1 MiB; existence is resolved at runtime",
	}
}

// StagePath is the structural-reference domain of an explicit path to stage.
func StagePath() Domain {
	return Domain{
		Concept: "explicit path to stage",
		Class:   ClassStructuralReference,
		Rule:    "repository-relative path; existence is resolved at runtime; repeatable",
	}
}

// Timeout is the scalar-bounded domain of the external process timeout. The
// default is rendered by the command framework from the flag registration.
func Timeout() Domain {
	return Domain{
		Concept: "timeout for external Git processes",
		Class:   ClassScalarBounded,
		Rule:    "a positive duration (rejected otherwise)",
	}
}

// All returns every canonical value-domain descriptor of the CLI. Contract
// tests iterate this set to pin every projection against its source.
func All() []Domain {
	return []Domain{
		CommitType(),
		BranchFamily(),
		DirectlyCreatableBranchFamily(),
		SyncStrategy(),
		ReleaseRequestKind(),
		ReleaseStabilizationKind(),
		ColorMode(),
		InteractiveMode(),
		OutputMode(),
		PullRequestProvider(),
		CompletionShell(),
		ReleaseVersion(),
		SupportVersion(),
		ReleaseLine(),
		AffectedLine(),
		PropagationTargetLine(),
		ManifestTargetLine(),
		TicketKey(),
		TicketNumber(),
		TicketID(),
		BranchSlug(),
		CommitSubject(),
		CommitBody(),
		CommitFooter(),
		BreakingDescription(),
		CommitSHA(),
		RemoteName(),
		RequestID(),
		RunID(),
		Requester(),
		CommitMessage(),
		BaseBranch(),
		BranchReference(),
		RecordPath(),
		CommitMessageFile(),
		StagePath(),
		Timeout(),
	}
}
