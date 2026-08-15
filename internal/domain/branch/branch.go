// Package branch models canonical Git branch names and lifecycle categories.
package branch

import (
	"regexp"
	"strings"

	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

const (
	maxSlugLength = 100
)

var (
	slugPattern         = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	semVerPattern       = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-((?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)
	supportPattern      = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	remoteNamePattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	ticketBranchPattern = regexp.MustCompile(`^(feature|fix|docs|refactor|chore|test|perf|hotfix|scratch)/([A-Z][A-Z0-9]*-[1-9][0-9]*)-([a-z0-9]+(?:-[a-z0-9]+)*)$`)
)

// Family classifies a branch according to the product's canonical taxonomy.
type Family string

const (
	FamilyMain     Family = "main"
	FamilyDevelop  Family = "develop"
	FamilyRelease  Family = "release"
	FamilySupport  Family = "support"
	FamilyFeature  Family = "feature"
	FamilyFix      Family = "fix"
	FamilyDocs     Family = "docs"
	FamilyRefactor Family = "refactor"
	FamilyChore    Family = "chore"
	FamilyTest     Family = "test"
	FamilyPerf     Family = "perf"
	FamilyHotfix   Family = "hotfix"
	FamilyScratch  Family = "scratch"
)

var families = []Family{
	FamilyMain,
	FamilyDevelop,
	FamilyRelease,
	FamilySupport,
	FamilyFeature,
	FamilyFix,
	FamilyDocs,
	FamilyRefactor,
	FamilyChore,
	FamilyTest,
	FamilyPerf,
	FamilyHotfix,
	FamilyScratch,
}

// Families returns every supported family in the canonical display order.
func Families() []Family {
	result := make([]Family, len(families))
	copy(result, families)
	return result
}

// ParseFamily validates a branch family.
func ParseFamily(raw string) (Family, error) {
	family := Family(raw)
	if !family.IsKnown() {
		return "", invalidFamily(raw)
	}
	return family, nil
}

// String returns the canonical family name.
func (f Family) String() string {
	return string(f)
}

// IsKnown reports whether a family belongs to the canonical taxonomy.
func (f Family) IsKnown() bool {
	for _, candidate := range families {
		if f == candidate {
			return true
		}
	}
	return false
}

// IsSharedLine reports whether developer mutations must be gated by a pull
// request and remote protection.
func (f Family) IsSharedLine() bool {
	switch f {
	case FamilyMain, FamilyDevelop, FamilyRelease, FamilySupport:
		return true
	default:
		return false
	}
}

// IsTicketScoped reports whether a branch name carries an explicit ticket.
func (f Family) IsTicketScoped() bool {
	switch f {
	case FamilyFeature, FamilyFix, FamilyDocs, FamilyRefactor, FamilyChore, FamilyTest, FamilyPerf, FamilyHotfix, FamilyScratch:
		return true
	default:
		return false
	}
}

// IsOfficialWorkingBranch reports whether a branch becomes append-only after
// its first push.
func (f Family) IsOfficialWorkingBranch() bool {
	return f.IsTicketScoped() && f != FamilyScratch
}

// AllowsPrivateRewrites reports whether the branch is intentionally private.
func (f Family) AllowsPrivateRewrites() bool {
	return f == FamilyScratch
}

// RequiresWorkflow reports whether creation requires a specialized workflow
// instead of the normal ticket branch creation path.
func (f Family) RequiresWorkflow() bool {
	switch f {
	case FamilyRelease, FamilySupport, FamilyHotfix:
		return true
	default:
		return false
	}
}

// MayUseWorkflowBase reports whether a specialized workflow may record a
// non-default remote base for later synchronization and hook validation.
func (f Family) MayUseWorkflowBase() bool {
	switch f {
	case FamilyHotfix, FamilyFix, FamilyDocs, FamilyChore:
		return true
	default:
		return false
	}
}

// DefaultTargetBase returns the standard remote base for regular ticket work.
func (f Family) DefaultTargetBase(remote string) (TargetBase, bool, error) {
	switch f {
	case FamilyFeature, FamilyFix, FamilyDocs, FamilyRefactor, FamilyChore, FamilyTest, FamilyPerf:
		develop, _ := ParseName("develop")
		base, err := NewTargetBase(remote, develop)
		return base, true, err
	default:
		return TargetBase{}, false, nil
	}
}

// Slug is a canonical ASCII kebab-case branch description.
type Slug struct {
	value string
}

// ParseSlug validates a branch description without silently normalizing it.
func ParseSlug(raw string) (Slug, error) {
	if len(raw) == 0 || len(raw) > maxSlugLength || !slugPattern.MatchString(raw) {
		return Slug{}, invalidSlug(raw)
	}
	return Slug{value: raw}, nil
}

// String returns the canonical slug.
func (s Slug) String() string {
	return s.value
}

// SemanticVersion is a valid SemVer 2.0.0 version without a leading v.
type SemanticVersion struct {
	value string
}

// ParseSemanticVersion validates a SemVer 2.0.0 version.
func ParseSemanticVersion(raw string) (SemanticVersion, error) {
	if !semVerPattern.MatchString(raw) {
		return SemanticVersion{}, invalidReleaseVersion(raw)
	}
	return SemanticVersion{value: raw}, nil
}

// String returns the canonical SemVer value.
func (v SemanticVersion) String() string {
	return v.value
}

// SupportVersion is a major.minor maintenance line version.
type SupportVersion struct {
	value string
}

// ParseSupportVersion validates a major.minor support version.
func ParseSupportVersion(raw string) (SupportVersion, error) {
	if !supportPattern.MatchString(raw) {
		return SupportVersion{}, invalidSupportVersion(raw)
	}
	return SupportVersion{value: raw}, nil
}

// String returns the canonical support version.
func (v SupportVersion) String() string {
	return v.value
}

// BranchName is a fully validated branch name with parsed components.
type BranchName struct {
	value          string
	family         Family
	ticket         ticket.ID
	hasTicket      bool
	slug           Slug
	hasSlug        bool
	releaseVersion SemanticVersion
	hasRelease     bool
	supportVersion SupportVersion
	hasSupport     bool
}

// NewTicketBranch creates a ticket-scoped branch for the specified family.
func NewTicketBranch(family Family, id ticket.ID, slug Slug) (BranchName, error) {
	if !family.IsTicketScoped() {
		return BranchName{}, invalidFamilyForTicketBranch(family)
	}
	if id.IsZero() {
		return BranchName{}, invalidBranchName("", "ticket-scoped branches require a ticket")
	}
	if slug.String() == "" {
		return BranchName{}, invalidBranchName("", "ticket-scoped branches require a slug")
	}

	value := family.String() + "/" + id.String() + "-" + slug.String()
	return BranchName{
		value:     value,
		family:    family,
		ticket:    id,
		hasTicket: true,
		slug:      slug,
		hasSlug:   true,
	}, nil
}

// NewReleaseBranch creates a release branch.
func NewReleaseBranch(version SemanticVersion) (BranchName, error) {
	if version.String() == "" {
		return BranchName{}, invalidBranchName("", "release branches require a semantic version")
	}
	return BranchName{
		value:          "release/" + version.String(),
		family:         FamilyRelease,
		releaseVersion: version,
		hasRelease:     true,
	}, nil
}

// NewSupportBranch creates a support branch.
func NewSupportBranch(version SupportVersion) (BranchName, error) {
	if version.String() == "" {
		return BranchName{}, invalidBranchName("", "support branches require a major.minor version")
	}
	return BranchName{
		value:          "support/" + version.String(),
		family:         FamilySupport,
		supportVersion: version,
		hasSupport:     true,
	}, nil
}

// ParseName validates every canonical branch family.
func ParseName(raw string) (BranchName, error) {
	switch raw {
	case "main":
		return BranchName{value: raw, family: FamilyMain}, nil
	case "develop":
		return BranchName{value: raw, family: FamilyDevelop}, nil
	}

	if version, found := strings.CutPrefix(raw, "release/"); found {
		if strings.Contains(version, "/") {
			return BranchName{}, invalidBranchName(raw, "release branches contain exactly one path separator")
		}
		parsed, err := ParseSemanticVersion(version)
		if err != nil {
			return BranchName{}, err
		}
		return NewReleaseBranch(parsed)
	}

	if version, found := strings.CutPrefix(raw, "support/"); found {
		if strings.Contains(version, "/") {
			return BranchName{}, invalidBranchName(raw, "support branches contain exactly one path separator")
		}
		parsed, err := ParseSupportVersion(version)
		if err != nil {
			return BranchName{}, err
		}
		return NewSupportBranch(parsed)
	}

	match := ticketBranchPattern.FindStringSubmatch(raw)
	if match == nil {
		return BranchName{}, invalidBranchName(raw, "branch name does not match a supported family grammar")
	}

	family, err := ParseFamily(match[1])
	if err != nil {
		return BranchName{}, err
	}
	id, err := ticket.ParseID(match[2])
	if err != nil {
		return BranchName{}, err
	}
	slug, err := ParseSlug(match[3])
	if err != nil {
		return BranchName{}, err
	}
	return NewTicketBranch(family, id, slug)
}

// String returns the canonical branch name.
func (name BranchName) String() string {
	return name.value
}

// Family returns the parsed branch family.
func (name BranchName) Family() Family {
	return name.family
}

// Ticket returns the branch ticket when the family is ticket-scoped.
func (name BranchName) Ticket() (ticket.ID, bool) {
	return name.ticket, name.hasTicket
}

// Slug returns the branch slug when present.
func (name BranchName) Slug() (Slug, bool) {
	return name.slug, name.hasSlug
}

// ReleaseVersion returns the release version when present.
func (name BranchName) ReleaseVersion() (SemanticVersion, bool) {
	return name.releaseVersion, name.hasRelease
}

// SupportVersion returns the support version when present.
func (name BranchName) SupportVersion() (SupportVersion, bool) {
	return name.supportVersion, name.hasSupport
}

// IsZero reports whether the branch name was not initialized.
func (name BranchName) IsZero() bool {
	return name.value == ""
}

// TargetBase identifies either a remote-tracking branch or an explicit local
// branch from which a working branch is created or synchronized.
type TargetBase struct {
	remote string
	branch BranchName
}

// NewTargetBase validates a remote name and a canonical branch target.
func NewTargetBase(remote string, branch BranchName) (TargetBase, error) {
	if !remoteNamePattern.MatchString(remote) {
		return TargetBase{}, problem.New(problem.Details{
			Code:        problem.CodeBranchBaseInvalid,
			Category:    problem.CategoryRepository,
			Field:       "remote",
			Actual:      remote,
			Expected:    "a Git remote name containing letters, digits, dots, underscores, or hyphens",
			Rule:        "remote names must match ^[A-Za-z0-9][A-Za-z0-9._-]*$",
			Example:     "origin",
			Remediation: "select an existing simple remote name such as origin",
		})
	}
	if branch.IsZero() {
		return TargetBase{}, invalidBranchName("", "a target base requires a canonical branch name")
	}
	return TargetBase{remote: remote, branch: branch}, nil
}

// ParseLocalBase parses a canonical branch name into a local target base.
func ParseLocalBase(raw string) (TargetBase, error) {
	name, err := ParseName(raw)
	if err != nil {
		return TargetBase{}, err
	}
	return localBase(name), nil
}

// NewLocalBase identifies an already available local branch. It is used for
// private scratch branches created from an official branch before first push.
func NewLocalBase(branch BranchName) (TargetBase, error) {
	if branch.IsZero() {
		return TargetBase{}, invalidBranchName("", "a local target base requires a canonical branch name")
	}
	return localBase(branch), nil
}

func localBase(branch BranchName) TargetBase {
	return TargetBase{branch: branch}
}

// Remote returns the remote name.
func (base TargetBase) Remote() string {
	return base.remote
}

// IsRemoteTracking reports whether the base resolves through a remote-tracking
// reference.
func (base TargetBase) IsRemoteTracking() bool {
	return base.remote != ""
}

// Branch returns the canonical target branch.
func (base TargetBase) Branch() BranchName {
	return base.branch
}

// String returns the Git ref accepted by branch, merge, and rebase commands.
func (base TargetBase) String() string {
	if base.branch.IsZero() {
		return ""
	}
	if base.remote == "" {
		return base.branch.String()
	}
	return base.remote + "/" + base.branch.String()
}

// PublicationState records whether an official working branch has been
// published to its configured remote.
type PublicationState string

const (
	PublicationUnknown     PublicationState = "unknown"
	PublicationUnpublished PublicationState = "unpublished"
	PublicationPublished   PublicationState = "published"
)

// CanRewriteHistory reports whether a rebase or amend remains policy-safe.
func (state PublicationState) CanRewriteHistory(family Family) bool {
	return state == PublicationUnpublished && family.IsOfficialWorkingBranch()
}

func invalidFamily(actual string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeBranchFamilyInvalid,
		Category:    problem.CategoryGovernance,
		Field:       "branch family",
		Actual:      actual,
		Expected:    "one of main, develop, release, support, feature, fix, docs, refactor, chore, test, perf, hotfix, scratch",
		Rule:        "branch families use the canonical taxonomy",
		Example:     "feature",
		Remediation: "select a supported branch family",
	})
}

func invalidFamilyForTicketBranch(family Family) error {
	return problem.New(problem.Details{
		Code:        problem.CodeBranchFamilyInvalid,
		Category:    problem.CategoryGovernance,
		Field:       "branch family",
		Actual:      family.String(),
		Expected:    "a ticket-scoped branch family",
		Rule:        "main, develop, release, and support do not use ticket-branch grammar",
		Example:     "feature/ABC-123-add-export-button",
		Remediation: "use a ticket-scoped family or the relevant release/support workflow",
	})
}

func invalidSlug(actual string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeBranchSlugInvalid,
		Category:    problem.CategoryGovernance,
		Field:       "branch slug",
		Actual:      actual,
		Expected:    "1 to 100 lowercase ASCII letters or digits separated by single hyphens",
		Rule:        "branch slugs must match ^[a-z0-9]+(?:-[a-z0-9]+)*$",
		Example:     "add-export-button",
		Remediation: "use lowercase words joined by single hyphens",
	})
}

func invalidReleaseVersion(actual string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeBranchNameInvalid,
		Category:    problem.CategoryGovernance,
		Field:       "release version",
		Actual:      actual,
		Expected:    "a Semantic Versioning 2.0.0 value without a leading v",
		Rule:        "release branches use release/<semver>",
		Example:     "release/2.8.0-rc.1",
		Remediation: "use major.minor.patch with optional pre-release and build metadata",
	})
}

func invalidSupportVersion(actual string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeBranchNameInvalid,
		Category:    problem.CategoryGovernance,
		Field:       "support version",
		Actual:      actual,
		Expected:    "a major.minor version without leading zeroes",
		Rule:        "support branches use support/<major.minor>",
		Example:     "support/2.7",
		Remediation: "use exactly two non-negative version components",
	})
}

func invalidBranchName(actual, rule string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeBranchNameInvalid,
		Category:    problem.CategoryGovernance,
		Field:       "branch",
		Actual:      actual,
		Expected:    "a canonical branch name for a supported family",
		Rule:        rule,
		Example:     "feature/ABC-123-add-export-button",
		Remediation: "use branch list to select a supported family and naming pattern",
	})
}
