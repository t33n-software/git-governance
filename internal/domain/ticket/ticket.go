// Package ticket models validated ticket identities without depending on a
// tracker or policy registry.
package ticket

import (
	"regexp"
	"strings"

	"github.com/t33n-software/git-governance/internal/domain/problem"
)

const (
	maxKeyLength    = 32
	maxNumberLength = 18
)

var (
	keyPattern    = regexp.MustCompile(`^[A-Z][A-Z0-9]*$`)
	numberPattern = regexp.MustCompile(`^[1-9][0-9]*$`)
)

// Key is a syntactically valid ticket namespace. Registry membership is
// deliberately outside this value object.
type Key struct {
	value string
}

// ParseKey validates a ticket key without normalizing user input.
func ParseKey(raw string) (Key, error) {
	if len(raw) == 0 || len(raw) > maxKeyLength || !keyPattern.MatchString(raw) {
		return Key{}, invalidKey(raw)
	}
	return Key{value: raw}, nil
}

// String returns the canonical key.
func (k Key) String() string {
	return k.value
}

// Number is a syntactically valid positive ticket number.
type Number struct {
	value string
}

// ParseNumber validates a ticket number without normalizing user input.
func ParseNumber(raw string) (Number, error) {
	if len(raw) == 0 || len(raw) > maxNumberLength || !numberPattern.MatchString(raw) {
		return Number{}, invalidNumber(raw)
	}
	return Number{value: raw}, nil
}

// String returns the canonical ticket number.
func (n Number) String() string {
	return n.value
}

// ID joins a key and positive number into one immutable ticket identity.
type ID struct {
	key    Key
	number Number
}

// NewID creates a ticket ID from separately validated values.
func NewID(key Key, number Number) ID {
	return ID{key: key, number: number}
}

// ParseID validates the canonical KEY-NUMBER form.
func ParseID(raw string) (ID, error) {
	keyRaw, numberRaw, found := strings.Cut(raw, "-")
	if !found || strings.Contains(numberRaw, "-") {
		return ID{}, invalidID(raw)
	}

	key, err := ParseKey(keyRaw)
	if err != nil {
		return ID{}, err
	}
	number, err := ParseNumber(numberRaw)
	if err != nil {
		return ID{}, err
	}
	return NewID(key, number), nil
}

// Key returns the namespace component.
func (id ID) Key() Key {
	return id.key
}

// Number returns the numeric component.
func (id ID) Number() Number {
	return id.number
}

// String returns the canonical KEY-NUMBER form.
func (id ID) String() string {
	return id.key.String() + "-" + id.number.String()
}

// IsZero reports whether the ID has not been initialized.
func (id ID) IsZero() bool {
	return id.key.value == "" || id.number.value == ""
}

func invalidKey(actual string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeTicketKeyInvalid,
		Category:    problem.CategoryGovernance,
		Field:       "ticket key",
		Actual:      actual,
		Expected:    "1 to 32 uppercase ASCII letters or digits, beginning with a letter",
		Rule:        "ticket keys must match ^[A-Z][A-Z0-9]*$",
		Example:     "ABC or PLATFORM2",
		Remediation: "use uppercase letters and digits only, starting with a letter",
	})
}

func invalidNumber(actual string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeTicketNumberInvalid,
		Category:    problem.CategoryGovernance,
		Field:       "ticket number",
		Actual:      actual,
		Expected:    "1 to 18 decimal digits without a leading zero",
		Rule:        "ticket numbers must match ^[1-9][0-9]*$",
		Example:     "123",
		Remediation: "use a positive decimal number without leading zeroes",
	})
}

func invalidID(actual string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeTicketIDInvalid,
		Category:    problem.CategoryGovernance,
		Field:       "ticket",
		Actual:      actual,
		Expected:    "KEY-NUMBER",
		Rule:        "a ticket ID contains exactly one key-number separator",
		Example:     "ABC-123",
		Remediation: "provide an uppercase ticket key followed by a positive number",
	})
}
