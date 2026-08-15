// Package commitmsg models the product's ticket-scoped Conventional Commit
// profile, including body, footers, and breaking-change declarations.
package commitmsg

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

const maxSubjectRunes = 200

var (
	headerPattern      = regexp.MustCompile(`^(build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test)\(([A-Z][A-Z0-9]*-[1-9][0-9]*)\)(!)?: ([^\r\n]+)$`)
	footerPattern      = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9-]*|BREAKING CHANGE|BREAKING-CHANGE)(: | #)(.+)$`)
	footerTokenPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)
	shaPattern         = regexp.MustCompile(`(?i)\b[0-9a-f]{7,64}\b`)
)

// Type classifies a Conventional Commit.
type Type string

const (
	TypeBuild    Type = "build"
	TypeChore    Type = "chore"
	TypeCI       Type = "ci"
	TypeDocs     Type = "docs"
	TypeFeat     Type = "feat"
	TypeFix      Type = "fix"
	TypePerf     Type = "perf"
	TypeRefactor Type = "refactor"
	TypeRevert   Type = "revert"
	TypeStyle    Type = "style"
	TypeTest     Type = "test"
)

var types = []Type{
	TypeBuild,
	TypeChore,
	TypeCI,
	TypeDocs,
	TypeFeat,
	TypeFix,
	TypePerf,
	TypeRefactor,
	TypeRevert,
	TypeStyle,
	TypeTest,
}

// Types returns all supported commit types in canonical display order.
func Types() []Type {
	result := make([]Type, len(types))
	copy(result, types)
	return result
}

// ParseType validates a commit type.
func ParseType(raw string) (Type, error) {
	kind := Type(raw)
	if !kind.IsKnown() {
		return "", invalidType(raw)
	}
	return kind, nil
}

// String returns the canonical commit type.
func (kind Type) String() string {
	return string(kind)
}

// IsKnown reports whether this is a supported commit type.
func (kind Type) IsKnown() bool {
	for _, candidate := range types {
		if kind == candidate {
			return true
		}
	}
	return false
}

// Header is the first line of a ticket-scoped Conventional Commit message.
type Header struct {
	kind     Type
	ticket   ticket.ID
	breaking bool
	subject  string
}

// NewHeader creates a validated commit header.
func NewHeader(kind Type, id ticket.ID, subject string, breaking bool) (Header, error) {
	if !kind.IsKnown() {
		return Header{}, invalidType(kind.String())
	}
	if id.IsZero() {
		return Header{}, invalidHeader("", "commit headers require a ticket scope")
	}
	if err := validateSubject(subject); err != nil {
		return Header{}, err
	}
	return Header{
		kind:     kind,
		ticket:   id,
		breaking: breaking,
		subject:  subject,
	}, nil
}

// ParseHeader validates a single-line commit header.
func ParseHeader(raw string) (Header, error) {
	match := headerPattern.FindStringSubmatch(raw)
	if match == nil {
		return Header{}, invalidHeader(raw, "commit headers must use type(TICKET)[!]: subject")
	}

	kind, err := ParseType(match[1])
	if err != nil {
		return Header{}, err
	}
	id, err := ticket.ParseID(match[2])
	if err != nil {
		return Header{}, err
	}
	return NewHeader(kind, id, match[4], match[3] == "!")
}

// Type returns the parsed commit type.
func (header Header) Type() Type {
	return header.kind
}

// Ticket returns the ticket scope.
func (header Header) Ticket() ticket.ID {
	return header.ticket
}

// Subject returns the short change summary.
func (header Header) Subject() string {
	return header.subject
}

// IsBreaking reports whether the header has the explicit breaking marker.
func (header Header) IsBreaking() bool {
	return header.breaking
}

// String returns the canonical header representation.
func (header Header) String() string {
	breaking := ""
	if header.breaking {
		breaking = "!"
	}
	return header.kind.String() + "(" + header.ticket.String() + ")" + breaking + ": " + header.subject
}

// Footer is a parsed commit footer. Continuation lines are folded into Value
// with newline separators.
type Footer struct {
	token     string
	separator string
	value     string
}

// NewFooter creates a footer with the standard colon separator.
func NewFooter(token, value string) (Footer, error) {
	return newFooter(token, ": ", value)
}

// Token returns the footer token.
func (footer Footer) Token() string {
	return footer.token
}

// Value returns the footer value.
func (footer Footer) Value() string {
	return footer.value
}

// IsBreaking reports whether this footer declares a breaking change.
func (footer Footer) IsBreaking() bool {
	return footer.token == "BREAKING CHANGE" || footer.token == "BREAKING-CHANGE"
}

// String returns the canonical footer representation, including continuation
// lines when the footer value spans multiple lines.
func (footer Footer) String() string {
	return footer.token + footer.separator + footer.value
}

// Message is a fully parsed Conventional Commit message.
type Message struct {
	header  Header
	body    string
	footers []Footer
}

// NewMessage constructs a validated message from already parsed components.
func NewMessage(header Header, body string, footers []Footer) (Message, error) {
	if !header.kind.IsKnown() || header.ticket.IsZero() {
		return Message{}, invalidHeader("", "commit messages require a valid header")
	}
	if err := validateText(body, true); err != nil {
		return Message{}, err
	}

	ownedFooters := make([]Footer, len(footers))
	copy(ownedFooters, footers)
	for _, footer := range ownedFooters {
		if err := validateFooter(footer); err != nil {
			return Message{}, err
		}
	}
	message := Message{
		header:  header,
		body:    body,
		footers: ownedFooters,
	}
	if header.Type() == TypeRevert && !message.hasCommitReference() {
		return Message{}, invalidDescription(
			"revert commits must reference the reverted commit",
			"include a commit SHA in the body or footer",
		)
	}
	return message, nil
}

// Parse validates a full multiline commit message.
func Parse(raw string) (Message, error) {
	normalized, err := normalizeInput(raw)
	if err != nil {
		return Message{}, err
	}

	lines := strings.Split(normalized, "\n")
	header, err := ParseHeader(lines[0])
	if err != nil {
		return Message{}, err
	}
	if len(lines) == 1 {
		return NewMessage(header, "", nil)
	}
	if lines[1] != "" {
		return Message{}, invalidDescription(
			"the header must be followed by a blank line before body or footers",
			"insert one blank line after the header",
		)
	}

	body, footers, err := parseSections(lines[2:])
	if err != nil {
		return Message{}, err
	}
	return NewMessage(header, body, footers)
}

// Header returns the parsed header.
func (message Message) Header() Header {
	return message.header
}

// Body returns the optional free-form body.
func (message Message) Body() string {
	return message.body
}

// Footers returns a defensive copy of the parsed footers.
func (message Message) Footers() []Footer {
	result := make([]Footer, len(message.footers))
	copy(result, message.footers)
	return result
}

// IsBreaking reports whether the header or any footer declares a breaking
// change.
func (message Message) IsBreaking() bool {
	if message.header.IsBreaking() {
		return true
	}
	for _, footer := range message.footers {
		if footer.IsBreaking() {
			return true
		}
	}
	return false
}

// String returns the canonical message representation without a trailing
// newline.
func (message Message) String() string {
	sections := []string{message.header.String()}
	if message.body != "" {
		sections = append(sections, message.body)
	}
	if len(message.footers) > 0 {
		lines := make([]string, 0, len(message.footers))
		for _, footer := range message.footers {
			lines = append(lines, footer.String())
		}
		sections = append(sections, strings.Join(lines, "\n"))
	}
	return strings.Join(sections, "\n\n")
}

func (message Message) hasCommitReference() bool {
	if shaPattern.MatchString(message.body) {
		return true
	}
	for _, footer := range message.footers {
		if shaPattern.MatchString(footer.value) {
			return true
		}
	}
	return false
}

func parseSections(lines []string) (string, []Footer, error) {
	if len(lines) == 0 {
		return "", nil, nil
	}

	footerStart := findFooterStart(lines)
	if footerStart < 0 {
		body := strings.TrimRight(strings.Join(lines, "\n"), "\n")
		if err := validateText(body, true); err != nil {
			return "", nil, err
		}
		return body, nil, nil
	}

	bodyLines := lines[:footerStart]
	if footerStart > 0 && bodyLines[len(bodyLines)-1] == "" {
		bodyLines = bodyLines[:len(bodyLines)-1]
	}
	body := strings.TrimRight(strings.Join(bodyLines, "\n"), "\n")
	if err := validateText(body, true); err != nil {
		return "", nil, err
	}

	footers, err := parseFooters(lines[footerStart:])
	if err != nil {
		return "", nil, err
	}
	return body, footers, nil
}

func findFooterStart(lines []string) int {
	if len(lines) == 0 {
		return -1
	}
	if isFooterLine(lines[0]) {
		return 0
	}
	for index := len(lines) - 1; index > 0; index-- {
		if lines[index-1] == "" && isFooterLine(lines[index]) {
			return index
		}
	}
	return -1
}

func parseFooters(lines []string) ([]Footer, error) {
	footers := make([]Footer, 0, len(lines))
	for _, line := range lines {
		match := footerPattern.FindStringSubmatch(line)
		if match != nil {
			footer, err := newFooter(match[1], match[2], match[3])
			if err != nil {
				return nil, err
			}
			footers = append(footers, footer)
			continue
		}
		if len(footers) > 0 && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			current := &footers[len(footers)-1]
			current.value += "\n" + line
			continue
		}
		return nil, invalidDescription(
			"footer blocks may only contain valid footer lines or indented continuations",
			"use TOKEN: value, TOKEN #value, BREAKING CHANGE: value, or an indented continuation line",
		)
	}
	return footers, nil
}

func isFooterLine(line string) bool {
	return footerPattern.MatchString(line)
}

func newFooter(token, separator, value string) (Footer, error) {
	footer := Footer{
		token:     token,
		separator: separator,
		value:     value,
	}
	if err := validateFooter(footer); err != nil {
		return Footer{}, err
	}
	return footer, nil
}

func validateFooter(footer Footer) error {
	if footer.token != "BREAKING CHANGE" && footer.token != "BREAKING-CHANGE" && !footerTokenPattern.MatchString(footer.token) {
		return invalidDescription(
			"footer tokens must use letters, digits, and hyphens",
			"use a token such as Refs, Reviewed-by, or BREAKING CHANGE",
		)
	}
	if footer.separator != ": " && footer.separator != " #" {
		return invalidDescription(
			"footer separators must be colon-space or space-hash",
			"use TOKEN: value or TOKEN #value",
		)
	}
	if footer.IsBreaking() && footer.separator != ": " {
		return problem.New(problem.Details{
			Code:        problem.CodeBreakingChangeInvalid,
			Category:    problem.CategoryGovernance,
			Field:       "breaking change footer",
			Expected:    "BREAKING CHANGE: description",
			Rule:        "breaking change footers use a colon-space separator",
			Example:     "BREAKING CHANGE: clients must migrate to the new contract",
			Remediation: "use BREAKING CHANGE: followed by a concrete migration impact",
		})
	}
	if footer.value == "" || strings.TrimSpace(footer.value) != footer.value {
		return invalidDescription(
			"footer values must be non-empty and unpadded",
			"provide a non-empty footer value without leading or trailing whitespace",
		)
	}
	return validateText(footer.value, true)
}

func normalizeInput(raw string) (string, error) {
	if raw == "" {
		return "", invalidHeader(raw, "commit messages must not be empty")
	}
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	if strings.ContainsRune(normalized, '\r') {
		return "", invalidDescription(
			"commit messages must use LF or CRLF line endings",
			"replace lone carriage returns with a supported line ending",
		)
	}
	normalized = strings.TrimSuffix(normalized, "\n")
	if normalized == "" {
		return "", invalidHeader(raw, "commit messages must contain a header")
	}
	if err := validateText(normalized, true); err != nil {
		return "", err
	}
	return normalized, nil
}

func validateSubject(subject string) error {
	if subject == "" || strings.TrimSpace(subject) != subject || utf8.RuneCountInString(subject) > maxSubjectRunes {
		return invalidDescription(
			"the commit subject must be non-empty, unpadded, and at most 200 characters",
			"use a concise subject such as add export button",
		)
	}
	return validateText(subject, false)
}

func validateText(value string, allowNewline bool) error {
	for _, runeValue := range value {
		if runeValue == '\n' && allowNewline {
			continue
		}
		if unicode.IsControl(runeValue) {
			return invalidDescription(
				"commit messages cannot contain control characters",
				"remove control characters from the commit message",
			)
		}
	}
	return nil
}

func invalidType(actual string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeCommitTypeInvalid,
		Category:    problem.CategoryGovernance,
		Field:       "commit type",
		Actual:      actual,
		Expected:    "build, chore, ci, docs, feat, fix, perf, refactor, revert, style, or test",
		Rule:        "commit types use the product's Conventional Commit profile",
		Example:     "feat(ABC-123): add export button",
		Remediation: "select a supported commit type",
	})
}

func invalidHeader(actual, rule string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeCommitHeaderInvalid,
		Category:    problem.CategoryGovernance,
		Field:       "commit header",
		Actual:      actual,
		Expected:    "type(TICKET)[!]: subject",
		Rule:        rule,
		Example:     "feat(ABC-123): add export button",
		Remediation: "supply a supported type, ticket scope, colon-space separator, and subject",
	})
}

func invalidDescription(rule, remediation string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeCommitDescriptionInvalid,
		Category:    problem.CategoryGovernance,
		Field:       "commit message",
		Expected:    "a valid Conventional Commit body and footer structure",
		Rule:        rule,
		Example:     "feat(ABC-123): add export button",
		Remediation: remediation,
	})
}
