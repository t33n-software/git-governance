package commitmsg

import (
	"regexp"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

func TestTypesAreCompleteAndDefensivelyCopied(t *testing.T) {
	t.Parallel()

	actual := Types()
	if len(actual) != 11 {
		t.Fatalf("Types() length = %d, want 11", len(actual))
	}
	if actual[0] != TypeBuild || actual[len(actual)-1] != TypeTest {
		t.Fatalf("Types() = %v", actual)
	}
	actual[0] = "mutated"
	if Types()[0] != TypeBuild {
		t.Fatal("Types() returned mutable shared backing storage")
	}
}

func TestParseType(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		raw   string
		valid bool
	}{
		{raw: "feat", valid: true},
		{raw: "fix", valid: true},
		{raw: "revert", valid: true},
		{raw: "feature", valid: false},
		{raw: "FEAT", valid: false},
		{raw: "", valid: false},
	} {
		testCase := testCase
		t.Run(testCase.raw, func(t *testing.T) {
			t.Parallel()
			actual, err := ParseType(testCase.raw)
			if testCase.valid {
				if err != nil || actual.String() != testCase.raw {
					t.Fatalf("ParseType(%q) = (%q, %v)", testCase.raw, actual, err)
				}
				return
			}
			assertProblemCode(t, err, problem.CodeCommitTypeInvalid)
		})
	}
}

func TestParseHeader(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		raw      string
		valid    bool
		breaking bool
		code     problem.Code
	}{
		{raw: "feat(ABC-123): add export button", valid: true},
		{raw: "feat(ABC-123)!: replace export contract", valid: true, breaking: true},
		{raw: "fix(PLATFORM2-7): resolve timeout", valid: true},
		{raw: "feat: add export button", valid: false, code: problem.CodeCommitHeaderInvalid},
		{raw: "feat(abc-123): add export button", valid: false, code: problem.CodeCommitHeaderInvalid},
		{raw: "feat(ABC-001): add export button", valid: false, code: problem.CodeCommitHeaderInvalid},
		{raw: "feature(ABC-123): add export button", valid: false, code: problem.CodeCommitHeaderInvalid},
		{raw: "feat(ABC-123):  padded", valid: false, code: problem.CodeCommitDescriptionInvalid},
		{raw: "feat(ABC-123): ", valid: false, code: problem.CodeCommitHeaderInvalid},
		{raw: "feat(ABC-123): add\nexport", valid: false, code: problem.CodeCommitHeaderInvalid},
		{raw: "feat(ABC-123): " + strings.Repeat("a", maxSubjectRunes+1), valid: false, code: problem.CodeCommitDescriptionInvalid},
		{raw: "feat(ABC-123): feat(ABC-123): add export button", valid: false, code: problem.CodeCommitDescriptionInvalid},
	} {
		testCase := testCase
		t.Run(testCase.raw, func(t *testing.T) {
			t.Parallel()
			actual, err := ParseHeader(testCase.raw)
			if testCase.valid {
				if err != nil {
					t.Fatalf("ParseHeader(%q) error = %v", testCase.raw, err)
				}
				if actual.String() != testCase.raw || actual.IsBreaking() != testCase.breaking {
					t.Fatalf("ParseHeader(%q) = (%q, breaking=%t)", testCase.raw, actual.String(), actual.IsBreaking())
				}
				return
			}
			assertProblemCode(t, err, testCase.code)
		})
	}
}

func TestParseMessage(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		raw        string
		body       string
		footers    int
		breaking   bool
		serialized string
	}{
		{
			name:       "header only",
			raw:        "feat(ABC-123): add export button\n",
			serialized: "feat(ABC-123): add export button",
		},
		{
			name:       "crlf body",
			raw:        "fix(ABC-123): resolve timeout\r\n\r\nThe timeout now honors cancellation.\r\n",
			body:       "The timeout now honors cancellation.",
			serialized: "fix(ABC-123): resolve timeout\n\nThe timeout now honors cancellation.",
		},
		{
			name:       "footer",
			raw:        "docs(ABC-123): document export workflow\n\nRefs: #123",
			footers:    1,
			serialized: "docs(ABC-123): document export workflow\n\nRefs: #123",
		},
		{
			name:       "body and multiple footers",
			raw:        "feat(ABC-123)!: replace export contract\n\nThe endpoint now returns a resource envelope.\n\nBREAKING CHANGE: clients must read the resource field.\nRefs: #123",
			body:       "The endpoint now returns a resource envelope.",
			footers:    2,
			breaking:   true,
			serialized: "feat(ABC-123)!: replace export contract\n\nThe endpoint now returns a resource envelope.\n\nBREAKING CHANGE: clients must read the resource field.\nRefs: #123",
		},
		{
			name:       "multiline footer",
			raw:        "docs(ABC-123): document migration\n\nReviewed-by: Alice\n additional reviewer context",
			footers:    1,
			serialized: "docs(ABC-123): document migration\n\nReviewed-by: Alice\n additional reviewer context",
		},
		{
			name:       "revert with sha",
			raw:        "revert(ABC-123): revert export contract\n\nRefs: 0123456789abcdef",
			footers:    1,
			serialized: "revert(ABC-123): revert export contract\n\nRefs: 0123456789abcdef",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			actual, err := Parse(testCase.raw)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if actual.Body() != testCase.body {
				t.Fatalf("Body() = %q, want %q", actual.Body(), testCase.body)
			}
			if len(actual.Footers()) != testCase.footers {
				t.Fatalf("len(Footers()) = %d, want %d", len(actual.Footers()), testCase.footers)
			}
			if actual.IsBreaking() != testCase.breaking {
				t.Fatalf("IsBreaking() = %t, want %t", actual.IsBreaking(), testCase.breaking)
			}
			if actual.String() != testCase.serialized {
				t.Fatalf("String() = %q, want %q", actual.String(), testCase.serialized)
			}
		})
	}
}

func TestParseMessageRejectsInvalidStructure(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name string
		raw  string
		code problem.Code
	}{
		{name: "empty", raw: "", code: problem.CodeCommitHeaderInvalid},
		{name: "missing blank after header", raw: "feat(ABC-123): add export\nbody", code: problem.CodeCommitDescriptionInvalid},
		{name: "lone carriage return", raw: "feat(ABC-123): add export\r", code: problem.CodeCommitDescriptionInvalid},
		{name: "control character", raw: "feat(ABC-123): add export\x00", code: problem.CodeCommitDescriptionInvalid},
		{name: "invalid footer after valid footer", raw: "feat(ABC-123): add export\n\nRefs: #123\nnot a footer", code: problem.CodeCommitDescriptionInvalid},
		{name: "breaking hash footer", raw: "feat(ABC-123): add export\n\nBREAKING CHANGE #wrong", code: problem.CodeBreakingChangeInvalid},
		{name: "revert without sha", raw: "revert(ABC-123): revert export", code: problem.CodeCommitDescriptionInvalid},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse(testCase.raw)
			assertProblemCode(t, err, testCase.code)
		})
	}
}

func TestNewHeaderMessageAndFooter(t *testing.T) {
	t.Parallel()

	id, err := ticket.ParseID("ABC-123")
	if err != nil {
		t.Fatal(err)
	}
	header, err := NewHeader(TypeFix, id, "resolve timeout", false)
	if err != nil {
		t.Fatal(err)
	}
	footer, err := NewFooter("Refs", "#123")
	if err != nil {
		t.Fatal(err)
	}
	actual, err := NewMessage(header, "The timeout now honors cancellation.", []Footer{footer})
	if err != nil {
		t.Fatal(err)
	}
	if actual.String() != "fix(ABC-123): resolve timeout\n\nThe timeout now honors cancellation.\n\nRefs: #123" {
		t.Fatalf("NewMessage().String() = %q", actual.String())
	}

	footers := actual.Footers()
	footers[0].value = "mutated"
	if actual.Footers()[0].Value() != "#123" {
		t.Fatal("Footers() returned mutable shared backing storage")
	}
}

func TestMessageValueObjectErrorAndAccessorPaths(t *testing.T) {
	t.Parallel()

	if _, err := NewHeader(Type("unknown"), ticket.ID{}, "subject", false); err == nil {
		t.Fatal("NewHeader accepted an unknown type and zero ticket")
	}
	if _, err := NewHeader(TypeFeat, ticket.ID{}, "subject", false); err == nil {
		t.Fatal("NewHeader accepted a zero ticket")
	}

	id, err := ticket.ParseID("ABC-123")
	if err != nil {
		t.Fatal(err)
	}
	header, err := NewHeader(TypeFeat, id, "add export", false)
	if err != nil {
		t.Fatal(err)
	}
	if header.Ticket().String() != "ABC-123" || header.Subject() != "add export" {
		t.Fatalf("header accessors = (%q, %q)", header.Ticket(), header.Subject())
	}
	footer, err := NewFooter("Refs", "#123")
	if err != nil {
		t.Fatal(err)
	}
	if footer.Token() != "Refs" || footer.Value() != "#123" {
		t.Fatalf("footer accessors = (%q, %q)", footer.Token(), footer.Value())
	}
	if _, err := newFooter("invalid token!", ": ", "value"); err == nil {
		t.Fatal("newFooter accepted invalid token")
	}
	if _, err := newFooter("Refs", "=", "value"); err == nil {
		t.Fatal("newFooter accepted invalid separator")
	}
	if _, err := newFooter("BREAKING CHANGE", " #", "value"); err == nil {
		t.Fatal("newFooter accepted invalid breaking separator")
	}
	if _, err := newFooter("Refs", ": ", " padded "); err == nil {
		t.Fatal("newFooter accepted padded value")
	}
	if _, err := NewMessage(Header{}, "", nil); err == nil {
		t.Fatal("NewMessage accepted a zero header")
	}
	if _, err := NewMessage(header, "\x00", nil); err == nil {
		t.Fatal("NewMessage accepted a control character in the body")
	}
	if _, err := NewMessage(header, "", []Footer{{}}); err == nil {
		t.Fatal("NewMessage accepted an invalid footer value object")
	}
	message, err := NewMessage(header, "", []Footer{footer})
	if err != nil {
		t.Fatal(err)
	}
	if message.Header().String() != header.String() {
		t.Fatalf("Message.Header() = %q", message.Header())
	}
	breakingFooter, err := NewFooter("BREAKING CHANGE", "migration required")
	if err != nil {
		t.Fatal(err)
	}
	breakingMessage, err := NewMessage(header, "", []Footer{breakingFooter})
	if err != nil || !breakingMessage.IsBreaking() {
		t.Fatalf("footer breaking message = (%#v, %v)", breakingMessage, err)
	}
	revertHeader, err := NewHeader(TypeRevert, id, "revert export", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewMessage(revertHeader, "Refs: no commit here", nil); err == nil {
		t.Fatal("revert message without SHA reference was accepted")
	}
}

func TestParseHeaderDefensiveComponentFailures(t *testing.T) {
	original := headerPattern
	t.Cleanup(func() {
		headerPattern = original
	})

	testCases := []struct {
		name    string
		pattern string
		raw     string
		code    problem.Code
	}{
		{
			name:    "unknown type",
			pattern: `^(unknown)\((ABC-123)\)(!)?: (subject)$`,
			raw:     "unknown(ABC-123): subject",
			code:    problem.CodeCommitTypeInvalid,
		},
		{
			name:    "invalid ticket",
			pattern: `^(feat)\((abc-123)\)(!)?: (subject)$`,
			raw:     "feat(abc-123): subject",
			code:    problem.CodeTicketKeyInvalid,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			headerPattern = regexp.MustCompile(testCase.pattern)
			_, err := ParseHeader(testCase.raw)
			assertProblemCode(t, err, testCase.code)
		})
	}
}

func TestParseMessageDefensiveSectionPaths(t *testing.T) {
	original := headerPattern
	t.Cleanup(func() {
		headerPattern = original
	})

	headerPattern = regexp.MustCompile(`^(unknown)\((ABC-123)\)(!)?: (subject)$`)
	if _, err := Parse("unknown(ABC-123): subject"); err == nil {
		t.Fatal("Parse accepted a header with an injected unknown type")
	}
	headerPattern = original

	id, err := ticket.ParseID("ABC-123")
	if err != nil {
		t.Fatal(err)
	}
	revertHeader, err := NewHeader(TypeRevert, id, "revert export", false)
	if err != nil {
		t.Fatal(err)
	}
	revertMessage, err := NewMessage(revertHeader, "Restores 0123456789abcdef.", nil)
	if err != nil || !revertMessage.hasCommitReference() {
		t.Fatalf("revert body reference = (%#v, %v)", revertMessage, err)
	}
	if _, _, err := parseSections([]string{"body\x00"}); err == nil {
		t.Fatal("parseSections accepted a control character in body text")
	}
	if _, _, err := parseSections([]string{"body\x00", "", "Refs: #123"}); err == nil {
		t.Fatal("parseSections accepted an invalid body before a footer block")
	}
	if _, _, err := parseSections([]string{"body", "", "Refs: #123", "\x00"}); err == nil {
		t.Fatal("parseSections accepted an invalid footer continuation")
	}
}

func TestInternalSectionAndFooterHelpers(t *testing.T) {
	t.Parallel()

	if got := findFooterStart([]string{"body", "", "Refs: #123", "Reviewed-by: Alice"}); got != 2 {
		t.Fatalf("findFooterStart() = %d, want 2", got)
	}
	if got := findFooterStart([]string{"body", "still body"}); got != -1 {
		t.Fatalf("findFooterStart() = %d, want -1", got)
	}

	footers, err := parseFooters([]string{"Refs: #123", " continuation"})
	if err != nil {
		t.Fatal(err)
	}
	if len(footers) != 1 || footers[0].String() != "Refs: #123\n continuation" {
		t.Fatalf("parseFooters() = %#v", footers)
	}
	if _, err := parseFooters([]string{"invalid"}); err == nil {
		t.Fatal("parseFooters accepted invalid line")
	}
	if body, footers, err := parseSections(nil); err != nil || body != "" || len(footers) != 0 {
		t.Fatalf("parseSections(nil) = (%q, %#v, %v)", body, footers, err)
	}
	if body, footers, err := parseSections([]string{"body", "", "still body"}); err != nil || body != "body\n\nstill body" || len(footers) != 0 {
		t.Fatalf("parseSections(body only) = (%q, %#v, %v)", body, footers, err)
	}
	if got := findFooterStart([]string{}); got != -1 {
		t.Fatalf("findFooterStart(empty) = %d", got)
	}
	if !isFooterLine("Refs: #123") || isFooterLine("not a footer") {
		t.Fatal("isFooterLine classification is incorrect")
	}

	if _, err := normalizeInput("\n"); err == nil {
		t.Fatal("normalizeInput accepted an empty message")
	}
	if err := validateText("line one\nline two", true); err != nil {
		t.Fatalf("validateText allowed newline error = %v", err)
	}
	if err := validateText("line one\nline two", false); err == nil {
		t.Fatal("validateText allowed forbidden newline")
	}
}

func TestValidateSubjectRejectsMetadataEnvelope(t *testing.T) {
	t.Parallel()

	id, err := ticket.ParseID("GQA-19")
	if err != nil {
		t.Fatal(err)
	}

	for _, testCase := range []struct {
		name    string
		subject string
		valid   bool
	}{
		{name: "reported double prefix", subject: "feat(GQA-19): admit the governance CLI in the canonical tool catalog", valid: false},
		{name: "bare family prefix at start", subject: "feat: admit the governance CLI", valid: false},
		{name: "family with scope at start", subject: "fix(GQA-19): resolve timeout", valid: false},
		{name: "breaking prefix at start", subject: "feat!: replace export contract", valid: false},
		{name: "mixed case envelope at start", subject: "Feat(GQA-19): admit the CLI", valid: false},
		{name: "uppercase envelope at start", subject: "FEAT(GQA-19): admit the CLI", valid: false},
		{name: "mismatched family envelope", subject: "fix(GQA-19): admit the governance CLI", valid: false},
		{name: "envelope embedded mid subject", subject: "note see fix(GQA-19): for the embedded envelope", valid: false},
		{name: "envelope at the subject end", subject: "reference the documented feat(GQA-19):", valid: false},
		{name: "plain subject", subject: "admit the governance CLI in the canonical tool catalog", valid: true},
		{name: "family token as imperative verb", subject: "test the export button flow", valid: true},
		{name: "family token inside another word", subject: "defeat(GQA-19): is not a family prefix", valid: true},
		{name: "parenthesized prose mid subject", subject: "document fix(parser) behavior", valid: true},
		{name: "scope reference without colon", subject: "reference feat(GQA-19) for context", valid: true},
		{name: "slash after family word", subject: "ci/cd pipeline notes", valid: true},
		{name: "colon after non family word", subject: "note: explain the export flow", valid: true},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewHeader(TypeFeat, id, testCase.subject, false)
			if testCase.valid {
				if err != nil {
					t.Fatalf("NewHeader() error = %v for subject %q", err, testCase.subject)
				}
				return
			}
			assertProblemCode(t, err, problem.CodeCommitDescriptionInvalid)
		})
	}
}

func assertProblemCode(t *testing.T, err error, expected problem.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected problem code %q, got nil", expected)
	}
	actual, ok := problem.As(err)
	if !ok {
		t.Fatalf("error %T does not carry a problem: %v", err, err)
	}
	if actual.Code != expected {
		t.Fatalf("problem code = %q, want %q", actual.Code, expected)
	}
}

// TestInvalidTypeExpectedRendersTheTypeRegistry pins the error contract
// against the canonical type registry: when the registry gains a commit
// family, the error surface must show it without any literal edit.
func TestInvalidTypeExpectedRendersTheTypeRegistry(t *testing.T) {
	t.Parallel()

	typed, ok := problem.As(invalidType("feature"))
	if !ok {
		t.Fatal("invalidType must return a typed problem")
	}
	names := make([]string, 0, len(types))
	for _, kind := range Types() {
		names = append(names, kind.String())
	}
	want := strings.Join(names[:len(names)-1], ", ") + ", or " + names[len(names)-1]
	if typed.Expected != want {
		t.Fatalf("invalidType Expected = %q, want registry-derived %q", typed.Expected, want)
	}
}

func FuzzParseCommitMessage(f *testing.F) {
	for _, seed := range []string{
		"feat(ABC-123): add export",
		"feat(ABC-123)!: replace export\n\nBREAKING CHANGE: clients must migrate",
		"docs(ABC-123): document export\n\nRefs: #123",
		"revert(ABC-123): revert export\n\nRefs: 0123456789abcdef",
		"",
		"\x00",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		message, err := Parse(raw)
		if err != nil {
			return
		}
		roundTrip, err := Parse(message.String())
		if err != nil {
			t.Fatalf("valid message did not round-trip: %q: %v", message.String(), err)
		}
		if roundTrip.String() != message.String() {
			t.Fatalf("round-trip changed %q to %q", message.String(), roundTrip.String())
		}
	})
}
