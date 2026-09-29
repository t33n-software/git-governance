// Package genesis models the governed birth of a repository: the boundary
// rules for the initial content set, the canonical genesis commit content,
// and the evidence record of the birth. The birth is the single governed
// creation path of the shared lines main and develop.
package genesis

import (
	"strconv"
	"strings"
	"time"

	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

// schemaVersion is the additive-only machine schema of the evidence record.
const schemaVersion = 1

// CommitSubject is the fixed behavior-focused subject of the genesis commit.
const CommitSubject = "initialize the governed repository"

// Record is the evidence artifact of a completed repository genesis. It is
// the root subject of the repository's evidence chain and carries no secrets.
type Record struct {
	SchemaVersion     int       `json:"schemaVersion"`
	Repository        string    `json:"repository"`
	Remote            string    `json:"remote"`
	Ticket            string    `json:"ticket"`
	Revision          string    `json:"revision"`
	Refs              []string  `json:"refs"`
	SignatureVerified bool      `json:"signatureVerified"`
	PolicySnapshot    string    `json:"policySnapshot"`
	Actor             string    `json:"actor"`
	CreatedAt         time.Time `json:"createdAt"`
}

// NewRecord binds the birth facts proven by the finalizer into the evidence
// record. CreatedAt is supplied by the caller so the record stays
// clock-agnostic and testable.
func NewRecord(
	repository string,
	remote string,
	id ticket.ID,
	revision string,
	refs []string,
	policySnapshot string,
	actor string,
	createdAt time.Time,
) Record {
	ownedRefs := make([]string, len(refs))
	copy(ownedRefs, refs)
	return Record{
		SchemaVersion:     schemaVersion,
		Repository:        repository,
		Remote:            remote,
		Ticket:            id.String(),
		Revision:          revision,
		Refs:              ownedRefs,
		SignatureVerified: true,
		PolicySnapshot:    policySnapshot,
		Actor:             actor,
		CreatedAt:         createdAt,
	}
}

// boundaryExactNames are base names that always carry secret or boundary
// content and therefore never belong to a governed initial content set.
var boundaryExactNames = map[string]bool{
	".env":             true,
	".netrc":           true,
	".git-credentials": true,
	"id_rsa":           true,
	"id_dsa":           true,
	"id_ecdsa":         true,
	"id_ed25519":       true,
}

// boundaryExtensions are file extensions of private-key or credential-store
// artifacts that never belong to a governed initial content set.
var boundaryExtensions = []string{
	".pem",
	".key",
	".p12",
	".pfx",
	".ppk",
	".credentials",
}

// envAllowedSuffixes are the documentation-only suffixes of the environment
// file family that carry placeholders instead of secrets.
var envAllowedSuffixes = []string{
	".example",
	".sample",
	".template",
}

// ScanContentSet enforces the content boundary of the governed genesis: the
// initial content set must not carry secret-bearing boundary artifacts. The
// scan is path-based and deterministic; content-level secret detection remains
// with the dedicated scanning boundaries outside this endpoint. The scan is
// fail-closed and reports every violating path.
func ScanContentSet(paths []string) error {
	violations := make([]string, 0, len(paths))
	for _, path := range paths {
		if isBoundaryArtifact(path) {
			violations = append(violations, path)
		}
	}
	if len(violations) == 0 {
		return nil
	}
	return problem.New(problem.Details{
		Code:     problem.CodeContentBoundaryViolation,
		Category: problem.CategoryGovernance,
		Field:    "content set",
		Actual:   strings.Join(violations, ", "),
		Expected: "an initial content set without secret or boundary artifacts",
		Rule: "the governed genesis refuses secret-bearing boundary artifacts (environment files, private keys, " +
			"credential stores) in the initial content set",
		Example: ".env.example instead of .env",
		Remediation: "remove the listed artifacts from the content set, ignore them through .gitignore, and provide " +
			"secrets through the organization's secret boundary",
	})
}

// isBoundaryArtifact classifies one slash-separated repository-relative path
// against the boundary-artifact rules. Matching is case-insensitive on the
// base name.
func isBoundaryArtifact(path string) bool {
	base := path
	if index := strings.LastIndex(path, "/"); index >= 0 {
		base = path[index+1:]
	}
	base = strings.ToLower(base)
	if boundaryExactNames[base] {
		return true
	}
	if strings.HasPrefix(base, ".env.") {
		for _, suffix := range envAllowedSuffixes {
			if strings.HasSuffix(base, suffix) {
				return false
			}
		}
		return true
	}
	for _, extension := range boundaryExtensions {
		if strings.HasSuffix(base, extension) {
			return true
		}
	}
	return false
}

// BodyFacts carries the verified preflight facts into the genesis commit body.
type BodyFacts struct {
	Ticket         ticket.ID
	ContentFiles   int
	PolicySnapshot string
	Publish        bool
}

// CommitBody renders the canonical genesis commit body in the binding
// category order: Motivation, Behavioral Change, Contracts and Invariants,
// Verification, Risks and Follow-ups. The genesis is never a trivial chore,
// so the body duty always applies. No line begins with the footer grammar
// form, so the message survives the creation-time round-trip parse.
func CommitBody(facts BodyFacts) string {
	risks := "Local-only until the separately confirmed publication step; rollback before any push is the " +
		"documented local teardown."
	if facts.Publish {
		risks = "Publication of main and develop to the selected remote is separately confirmed in the same " +
			"invocation; after publication the genesis is governed, non-rewriteable history."
	}
	lines := []string{
		"## Motivation",
		"",
		"Repository genesis under the governed lifecycle for " + facts.Ticket.String() + ": the audit and authorship",
		"evidence chain starts inside the governed surface from the first commit.",
		"",
		"## Behavioral Change",
		"",
		"Initial governed content set of " + strconv.Itoa(facts.ContentFiles) + " files; main carries the signed genesis commit; develop is",
		"born from the same revision; the hook boundary (commit-msg, pre-push) is installed.",
		"",
		"## Contracts and Invariants",
		"",
		"Policy snapshot bound at birth: " + facts.PolicySnapshot + ". No public contract changes.",
		"",
		"## Verification",
		"",
		"Preflight evidence: unborn-state proof, signing configuration and canary, hook-manager resolution,",
		"content boundary scan. Finalizer proof of refs, revision, signature, and hooks follows the mutation.",
		"",
		"## Risks and Follow-ups",
		"",
		risks,
	}
	return strings.Join(lines, "\n")
}
