package cliparam

import (
	"strings"
	"testing"
)

func TestMessageScaffoldKindPinsTheScaffoldableMessageKinds(t *testing.T) {
	t.Parallel()

	domain := MessageScaffoldKind()
	if domain.Class != ClassClosedEnum {
		t.Fatalf("MessageScaffoldKind().Class = %q, want %q", domain.Class, ClassClosedEnum)
	}
	assertStringsEqual(t, "MessageScaffoldKind().Values", domain.Values, []string{"commit-body", "pr-description"})
	if got, wantList := domain.ValueList(), "commit-body or pr-description"; got != wantList {
		t.Fatalf("MessageScaffoldKind().ValueList() = %q, want %q", got, wantList)
	}
	help := domain.HelpText("")
	if !strings.Contains(help, "commit-body") || !strings.Contains(help, "pr-description") {
		t.Fatalf("MessageScaffoldKind().HelpText() = %q, want it to render both kind values", help)
	}
	if values := MessageScaffoldKind().Values; len(values) == 0 {
		t.Fatal("closed-enum descriptor carries no values")
	}
}
