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

// This file holds the closed-enum value domains of the CLI. Closed value sets
// are derived from the domain registries at call time; endpoint subsets are
// produced through declared filters on those sources. Canonical conventions:
// docs/conventions/cli/value-domain-model.md and
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

// CompletionShell is the closed-enum domain of the shells supported by the
// completion script endpoint.
func CompletionShell() Domain {
	return Domain{
		Concept: "shell",
		Class:   ClassClosedEnum,
		Values:  []string{"bash", "zsh", "fish", "powershell"},
	}
}
