package commitmsg

import (
	"strings"
	"testing"
)

func TestBodySectionsReturnsTheCanonicalOrderDefensively(t *testing.T) {
	t.Parallel()

	want := []string{
		"Motivation",
		"Behavioral Change",
		"Contracts and Invariants",
		"Verification",
		"Risks and Follow-ups",
	}
	sections := BodySections()
	if len(sections) != len(want) {
		t.Fatalf("BodySections() returned %d entries, want %d", len(sections), len(want))
	}
	for index, name := range want {
		if sections[index].Name != name {
			t.Fatalf("BodySections()[%d].Name = %q, want %q", index, sections[index].Name, name)
		}
		if sections[index].Slot == "" {
			t.Fatalf("BodySections()[%d] carries no scaffold slot", index)
		}
	}

	sections[0].Name = "mutated"
	if again := BodySections(); again[0].Name == "mutated" {
		t.Fatalf("BodySections() returned a shared backing array: %v", again[0].Name)
	}
}

func TestBodySkeletonRendersEveryCanonicalCategoryWithItsSlot(t *testing.T) {
	t.Parallel()

	skeleton := BodySkeleton()
	for _, section := range BodySections() {
		block := "## " + section.Name + "\n\n" + section.Slot
		if !strings.Contains(skeleton, block) {
			t.Fatalf("skeleton misses the category block %q:\n%s", block, skeleton)
		}
	}
	if !strings.HasSuffix(skeleton, "\n") || strings.HasSuffix(skeleton, "\n\n") {
		t.Fatalf("skeleton must end with exactly one trailing newline:\n%q", skeleton)
	}
	if strings.Contains(skeleton, "\n\n\n") {
		t.Fatalf("skeleton carries a doubled blank line between blocks:\n%s", skeleton)
	}
}
