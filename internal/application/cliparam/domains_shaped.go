package cliparam

// This file holds the shaped value domains of the CLI: version, line, and
// base references whose grammar is defined by a rule rather than a closed
// value set. Canonical conventions: docs/conventions/cli/value-domain-model.md
// and docs/conventions/cli/single-source-of-truth.md.

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

// ProtectedLineVersion is the shaped union domain of the version accepted by
// the protected-line request endpoint: a release semantic version or a
// support major.minor version, matching the requested operation.
func ProtectedLineVersion() Domain {
	return Domain{
		Concept: "protected line version",
		Class:   ClassShaped,
		Rule: "release semantic version without a leading v, or support major.minor " +
			"without leading zeroes, matching the requested operation (rejected otherwise)",
		Example: "2.8.0-rc.1 or 2.7",
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

// ManifestTargetLine is the shaped domain of the target line accepted by
// the manifest-based hotfix propagation endpoint; main is record-bound and
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

// WorktreeAcquisitionBase is the shaped domain of the base line whose current
// revision a task worktree is acquired from. The empty default binds the
// develop integration line of regular ticket work; hotfix and stabilization
// lanes declare their protected or frozen base line.
func WorktreeAcquisitionBase() Domain {
	return Domain{
		Concept:  "acquisition base line",
		Class:    ClassShaped,
		Rule:     "develop, main, release/<semver>, or support/<major.minor>; the lane-to-base mapping binds ticket work to develop, hotfix work to the protected base line, and stabilization work to the frozen release line; working-family lines are rejected",
		Example:  "main",
		Prefixes: []string{"develop", "main", "release/", "support/"},
	}
}
