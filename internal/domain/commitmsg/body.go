package commitmsg

import "strings"

// BodySection is one canonical commit-body category with its scaffold slot.
// The five categories are the immutable content-contract order; the slots are
// descriptive placeholders the author replaces, never fabricated content.
// Canonical convention: docs/conventions/commits/family-selection.md.
type BodySection struct {
	Name string
	Slot string
}

// bodySections is the canonical commit-body section register: the scaffold
// skeleton, the help layout block, and the machine-readable discovery channel
// all render from this one source, never from duplicated literals. Canonical
// convention: docs/conventions/cli/single-source-of-truth.md.
var bodySections = []BodySection{
	{Name: "Motivation", Slot: "<functional reason and goal of the unit>"},
	{Name: "Behavioral Change", Slot: "<scope and boundaries at behavior level, never line level>"},
	{Name: "Contracts and Invariants", Slot: "<touched public contracts, invariants, or integration points>"},
	{Name: "Verification", Slot: "<which check proves the behavior>"},
	{Name: "Risks and Follow-ups", Slot: "<side effects and named follow-up work>"},
}

// BodySections returns a defensive copy of the canonical commit-body
// categories in their immutable order.
func BodySections() []BodySection {
	result := make([]BodySection, len(bodySections))
	copy(result, bodySections)
	return result
}

// BodySkeleton renders the copy-ready commit body skeleton: every canonical
// category as a "## <Category Name>" heading followed by its slot. The
// skeleton is a construction aid, not a mechanical gate: only applicable
// categories must survive into the final body, and shipping an unfilled slot
// violates the filler-text and invented-content anti-patterns.
func BodySkeleton() string {
	blocks := make([]string, 0, len(bodySections))
	for _, section := range bodySections {
		blocks = append(blocks, "## "+section.Name+"\n\n"+section.Slot)
	}
	return strings.Join(blocks, "\n\n") + "\n"
}
