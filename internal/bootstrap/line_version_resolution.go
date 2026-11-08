package bootstrap

import (
	"context"

	"github.com/t33n-software/git-governance/internal/application/workflow"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// This file owns the line, version, and commit resolutions shared by the
// hotfix, release, and propagation endpoints: every prompt and every
// rejection carries the same governed line taxonomy.

func (application *application) resolveAffectedLine(ctx context.Context, raw string) (branch.BranchName, error) {
	return resolveAllowedBranchLine(
		application,
		ctx,
		raw,
		"Affected line",
		"Enter main, release/<semantic-version>, or support/<major.minor>. A hotfix must start from the active line that contains the defect. Examples: main, release/2.8.0, support/2.7.",
		"affected line",
		"main, release/<semver>, or support/<major.minor>",
		"a hotfix must start from the active line that contains the defect",
		"main",
		"select the main, release, or support line that contains the defect",
		branch.FamilyMain,
		branch.FamilyRelease,
		branch.FamilySupport,
	)
}

func (application *application) resolvePropagationTarget(ctx context.Context, raw string) (branch.BranchName, error) {
	return resolveAllowedBranchLine(
		application,
		ctx,
		raw,
		"Propagation target line",
		"Enter main, develop, release/<semantic-version>, or support/<major.minor>. The target must be the active line that also needs the reviewed hotfix. Examples: develop, release/2.8.0.",
		"propagation target line",
		"main, develop, release/<semver>, or support/<major.minor>",
		"hotfix propagation targets another active line",
		"develop",
		"select the active line that also needs the reviewed hotfix",
		branch.FamilyMain,
		branch.FamilyDevelop,
		branch.FamilyRelease,
		branch.FamilySupport,
	)
}

func (application *application) resolveReleaseLine(
	ctx context.Context,
	raw, label, description string,
) (branch.BranchName, error) {
	return resolveAllowedBranchLine(
		application,
		ctx,
		raw,
		label,
		description,
		"release line",
		"release/<semver>",
		"the selected workflow requires a canonical release line",
		"release/2.8.0",
		"enter the release/<semver> line required by this workflow",
		branch.FamilyRelease,
	)
}

func (application *application) resolveReviewedCommit(ctx context.Context, raw string) (string, error) {
	return resolveValidatedInput(
		application,
		ctx,
		raw,
		"Reviewed source commit",
		"Enter the 7 to 64 character hexadecimal SHA of the reviewed hotfix commit. Do not enter a branch name, tag, ref, or spaces. Example: 0123456789abcdef0123456789abcdef01234567.",
		func(value string) (string, error) {
			return value, workflow.ValidateCommitID(value)
		},
	)
}

func (application *application) resolveReleaseVersion(ctx context.Context, raw string) (branch.SemanticVersion, error) {
	return resolveValidatedInput(
		application,
		ctx,
		raw,
		"Release version",
		"Enter Semantic Versioning 2.0.0 without a leading v: major.minor.patch, optionally with pre-release or build metadata. Examples: 2.8.0, 2.8.0-rc.1.",
		branch.ParseSemanticVersion,
	)
}

func (application *application) resolveSupportVersion(ctx context.Context, raw string) (branch.SupportVersion, error) {
	return resolveValidatedInput(
		application,
		ctx,
		raw,
		"Support version",
		"Enter exactly major.minor without a leading v or leading zeroes. Examples: 2.7, 0.9. Patch versions are not allowed.",
		branch.ParseSupportVersion,
	)
}

func (application *application) resolveStabilizationKind(ctx context.Context, raw string) (workflow.ReleaseStabilizationKind, error) {
	return resolveValidatedInput(
		application,
		ctx,
		raw,
		"Stabilization kind",
		"Enter blocker, docs, or release-prep. Frozen release lines do not allow new features or general refactors. Example: blocker.",
		workflow.ParseReleaseStabilizationKind,
	)
}

func resolveAllowedBranchLine(
	application *application,
	ctx context.Context,
	raw, label, description, field, expected, rule, example, remediation string,
	allowed ...branch.Family,
) (branch.BranchName, error) {
	return resolveValidatedInput(
		application,
		ctx,
		raw,
		label,
		description,
		func(value string) (branch.BranchName, error) {
			name, err := branch.ParseName(value)
			if err != nil {
				return branch.BranchName{}, err
			}
			for _, family := range allowed {
				if name.Family() == family {
					return name, nil
				}
			}
			return branch.BranchName{}, problem.New(problem.Details{
				Code:        problem.CodeInvalidInput,
				Category:    problem.CategoryGovernance,
				Field:       field,
				Actual:      value,
				Expected:    expected,
				Rule:        rule,
				Example:     example,
				Remediation: remediation,
			})
		},
	)
}
