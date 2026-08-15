// Package branchapp contains use cases for governed branch operations.
package branchapp

import "github.com/t33n-software/git-governance/internal/domain/branch"

// FamilyInfo explains a branch family to human and automation consumers.
type FamilyInfo struct {
	Family              branch.Family `json:"family"`
	Label               string        `json:"label"`
	Role                string        `json:"role"`
	Pattern             string        `json:"pattern"`
	DefaultBase         string        `json:"defaultBase,omitempty"`
	TypicalTarget       string        `json:"typicalTarget"`
	Description         string        `json:"description"`
	DirectlyCreatable   bool          `json:"directlyCreatable"`
	RequiresTicket      bool          `json:"requiresTicket"`
	RequiresSpecialFlow bool          `json:"requiresSpecialFlow"`
}

var catalog = []FamilyInfo{
	{
		Family:        branch.FamilyMain,
		Label:         "Main",
		Role:          "Published production truth",
		Pattern:       "main",
		TypicalTarget: "production",
		Description:   "A protected shared line. Developers do not create ordinary work branches from this command.",
	},
	{
		Family:        branch.FamilyDevelop,
		Label:         "Develop",
		Role:          "Integration line for the next release",
		Pattern:       "develop",
		TypicalTarget: "next release",
		Description:   "A protected shared line and the default base for regular ticket work.",
	},
	{
		Family:              branch.FamilyRelease,
		Label:               "Release",
		Role:                "Frozen release candidate",
		Pattern:             "release/<semver>",
		DefaultBase:         "origin/develop",
		TypicalTarget:       "main, then develop",
		Description:         "Created only by the release workflow; it permits limited stabilization rather than new feature work.",
		RequiresSpecialFlow: true,
	},
	{
		Family:              branch.FamilySupport,
		Label:               "Support",
		Role:                "Maintained older version line",
		Pattern:             "support/<major.minor>",
		TypicalTarget:       "support release",
		Description:         "Created only for explicit maintenance obligations; it is not a normal development branch.",
		RequiresSpecialFlow: true,
	},
	{
		Family:            branch.FamilyFeature,
		Label:             "Feature",
		Role:              "New product capability",
		Pattern:           "feature/<ticket>-<slug>",
		DefaultBase:       "origin/develop",
		TypicalTarget:     "develop",
		Description:       "The official branch for a reviewable feature ticket.",
		DirectlyCreatable: true,
		RequiresTicket:    true,
	},
	{
		Family:            branch.FamilyFix,
		Label:             "Fix",
		Role:              "Regular defect correction",
		Pattern:           "fix/<ticket>-<slug>",
		DefaultBase:       "origin/develop",
		TypicalTarget:     "develop",
		Description:       "The official branch for a non-hotfix bug correction.",
		DirectlyCreatable: true,
		RequiresTicket:    true,
	},
	{
		Family:            branch.FamilyDocs,
		Label:             "Documentation",
		Role:              "Documentation-only work",
		Pattern:           "docs/<ticket>-<slug>",
		DefaultBase:       "origin/develop",
		TypicalTarget:     "develop",
		Description:       "The official branch for documentation work tied to a ticket.",
		DirectlyCreatable: true,
		RequiresTicket:    true,
	},
	{
		Family:            branch.FamilyRefactor,
		Label:             "Refactor",
		Role:              "Internal design change",
		Pattern:           "refactor/<ticket>-<slug>",
		DefaultBase:       "origin/develop",
		TypicalTarget:     "develop",
		Description:       "The official branch for an intentional restructuring without a new feature.",
		DirectlyCreatable: true,
		RequiresTicket:    true,
	},
	{
		Family:            branch.FamilyChore,
		Label:             "Chore",
		Role:              "Maintenance or tooling",
		Pattern:           "chore/<ticket>-<slug>",
		DefaultBase:       "origin/develop",
		TypicalTarget:     "develop",
		Description:       "The official branch for maintenance and tooling changes.",
		DirectlyCreatable: true,
		RequiresTicket:    true,
	},
	{
		Family:            branch.FamilyTest,
		Label:             "Test",
		Role:              "Test-focused work",
		Pattern:           "test/<ticket>-<slug>",
		DefaultBase:       "origin/develop",
		TypicalTarget:     "develop",
		Description:       "The official branch for test additions or corrections.",
		DirectlyCreatable: true,
		RequiresTicket:    true,
	},
	{
		Family:            branch.FamilyPerf,
		Label:             "Performance",
		Role:              "Measured performance improvement",
		Pattern:           "perf/<ticket>-<slug>",
		DefaultBase:       "origin/develop",
		TypicalTarget:     "develop",
		Description:       "The official branch for performance-focused work.",
		DirectlyCreatable: true,
		RequiresTicket:    true,
	},
	{
		Family:              branch.FamilyHotfix,
		Label:               "Hotfix",
		Role:                "Correction on an affected active line",
		Pattern:             "hotfix/<ticket>-<slug>",
		TypicalTarget:       "the same affected line",
		Description:         "Created by the hotfix workflow from main, a release line, or a support line that actually carries the defect.",
		RequiresTicket:      true,
		RequiresSpecialFlow: true,
	},
	{
		Family:            branch.FamilyScratch,
		Label:             "Scratch",
		Role:              "Private exploration",
		Pattern:           "scratch/<ticket>-<slug>",
		TypicalTarget:     "no pull request",
		Description:       "Use only for uncertain experiments. Do not open a pull request from it; move stable work to the official ticket branch.",
		DirectlyCreatable: true,
		RequiresTicket:    true,
	},
}

// ListFamilies returns every branch family with an independent copy of the
// display data.
func ListFamilies() []FamilyInfo {
	result := make([]FamilyInfo, len(catalog))
	copy(result, catalog)
	return result
}
