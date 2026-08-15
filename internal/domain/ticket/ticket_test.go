package ticket

import (
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestParseKey(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		raw   string
		valid bool
	}{
		{name: "single letter", raw: "A", valid: true},
		{name: "letters", raw: "ABC", valid: true},
		{name: "letters and digits", raw: "PLATFORM2", valid: true},
		{name: "maximum length", raw: "A" + strings.Repeat("1", maxKeyLength-1), valid: true},
		{name: "empty", raw: "", valid: false},
		{name: "lowercase", raw: "abc", valid: false},
		{name: "leading digit", raw: "2ABC", valid: false},
		{name: "hyphen", raw: "ABC-OPS", valid: false},
		{name: "underscore", raw: "ABC_OPS", valid: false},
		{name: "whitespace", raw: "ABC ", valid: false},
		{name: "too long", raw: "A" + strings.Repeat("1", maxKeyLength), valid: false},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			actual, err := ParseKey(testCase.raw)
			if testCase.valid {
				if err != nil {
					t.Fatalf("ParseKey(%q) error = %v", testCase.raw, err)
				}
				if actual.String() != testCase.raw {
					t.Fatalf("ParseKey(%q).String() = %q", testCase.raw, actual.String())
				}
				return
			}
			assertProblemCode(t, err, problem.CodeTicketKeyInvalid)
		})
	}
}

func TestParseNumber(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		raw   string
		valid bool
	}{
		{name: "one", raw: "1", valid: true},
		{name: "large", raw: "123456789012345678", valid: true},
		{name: "zero", raw: "0", valid: false},
		{name: "leading zero", raw: "001", valid: false},
		{name: "negative", raw: "-1", valid: false},
		{name: "decimal", raw: "1.5", valid: false},
		{name: "letter", raw: "A1", valid: false},
		{name: "too long", raw: strings.Repeat("1", maxNumberLength+1), valid: false},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			actual, err := ParseNumber(testCase.raw)
			if testCase.valid {
				if err != nil {
					t.Fatalf("ParseNumber(%q) error = %v", testCase.raw, err)
				}
				if actual.String() != testCase.raw {
					t.Fatalf("ParseNumber(%q).String() = %q", testCase.raw, actual.String())
				}
				return
			}
			assertProblemCode(t, err, problem.CodeTicketNumberInvalid)
		})
	}
}

func TestParseID(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		raw   string
		valid bool
		code  problem.Code
	}{
		{name: "valid", raw: "ABC-123", valid: true},
		{name: "key with digit", raw: "PLATFORM2-7", valid: true},
		{name: "missing separator", raw: "ABC123", code: problem.CodeTicketIDInvalid},
		{name: "multiple separators", raw: "ABC-1-2", code: problem.CodeTicketIDInvalid},
		{name: "invalid key", raw: "abc-123", code: problem.CodeTicketKeyInvalid},
		{name: "invalid number", raw: "ABC-001", code: problem.CodeTicketNumberInvalid},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			actual, err := ParseID(testCase.raw)
			if testCase.valid {
				if err != nil {
					t.Fatalf("ParseID(%q) error = %v", testCase.raw, err)
				}
				if actual.String() != testCase.raw {
					t.Fatalf("ParseID(%q).String() = %q", testCase.raw, actual.String())
				}
				if actual.Key().String() == "" || actual.Number().String() == "" || actual.IsZero() {
					t.Fatalf("ParseID(%q) returned incomplete ID", testCase.raw)
				}
				return
			}
			assertProblemCode(t, err, testCase.code)
		})
	}
}

func TestNewIDAndZeroValue(t *testing.T) {
	t.Parallel()

	var zero ID
	if !zero.IsZero() {
		t.Fatal("zero ID must report IsZero")
	}

	key, err := ParseKey("ABC")
	if err != nil {
		t.Fatal(err)
	}
	number, err := ParseNumber("123")
	if err != nil {
		t.Fatal(err)
	}
	actual := NewID(key, number)
	if actual.IsZero() || actual.String() != "ABC-123" {
		t.Fatalf("NewID() = %#v", actual)
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

func FuzzParseTicketValues(f *testing.F) {
	for _, seed := range []string{"ABC", "PLATFORM2", "ABC-123", "", "abc", "ABC-001", "\x00"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if key, err := ParseKey(raw); err == nil {
			if key.String() != raw {
				t.Fatalf("ParseKey(%q).String() = %q", raw, key.String())
			}
		}
		if number, err := ParseNumber(raw); err == nil {
			if number.String() != raw {
				t.Fatalf("ParseNumber(%q).String() = %q", raw, number.String())
			}
		}
		if id, err := ParseID(raw); err == nil {
			if id.String() != raw {
				t.Fatalf("ParseID(%q).String() = %q", raw, id.String())
			}
		}
	})
}
