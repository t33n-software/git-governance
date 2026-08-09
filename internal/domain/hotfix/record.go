// Package hotfix defines immutable, reviewable facts required for a
// production hotfix delivery.
package hotfix

import (
	"bytes"
	"encoding/json"
	"io"
	"math/big"
	"regexp"
	"strings"

	"github.com/CyberT33N/git-governance/internal/domain/branch"
	"github.com/CyberT33N/git-governance/internal/domain/problem"
	"github.com/CyberT33N/git-governance/internal/domain/ticket"
)

const recordSchemaVersion = 1

var (
	fullCommitPattern  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	stableVersionParts = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
)

// ReleaseRecord is the reviewed input to a production hotfix delivery
// controller. Its fields are private so every instance has passed ParseRecord.
type ReleaseRecord struct {
	ticket                  ticket.ID
	incident                string
	affectedLine            branch.BranchName
	targetVersion           branch.SemanticVersion
	previousTag             string
	expectedSource          branch.BranchName
	expectedTarget          branch.BranchName
	manifest                []string
	commitBudgetException   string
	scopeEscalationApproval string
	propagationTargets      []branch.BranchName
}

// Ticket returns the ticket bound to the hotfix record.
func (record ReleaseRecord) Ticket() ticket.ID {
	return record.ticket
}

// Incident returns the non-secret incident reference bound to the record.
func (record ReleaseRecord) Incident() string {
	return record.incident
}

// AffectedLine returns the production line repaired by the hotfix.
func (record ReleaseRecord) AffectedLine() branch.BranchName {
	return record.affectedLine
}

// TargetVersion returns the immutable patch version to deliver.
func (record ReleaseRecord) TargetVersion() branch.SemanticVersion {
	return record.targetVersion
}

// PreviousTag returns the released tag immediately preceding TargetVersion.
func (record ReleaseRecord) PreviousTag() string {
	return record.previousTag
}

// ExpectedSource returns the reviewed hotfix branch expected in the merged PR.
func (record ReleaseRecord) ExpectedSource() branch.BranchName {
	return record.expectedSource
}

// ExpectedTarget returns the protected affected line expected in the merged PR.
func (record ReleaseRecord) ExpectedTarget() branch.BranchName {
	return record.expectedTarget
}

// Manifest returns a defensive copy of the ordered full-SHA semantic commits.
func (record ReleaseRecord) Manifest() []string {
	return append([]string(nil), record.manifest...)
}

// CommitBudgetException returns the required reason for a manifest exceeding
// the normal one-to-three semantic commit budget.
func (record ReleaseRecord) CommitBudgetException() string {
	return record.commitBudgetException
}

// ScopeEscalationApproval returns the explicit release approval reference
// required when the semantic manifest contains five or more commits.
func (record ReleaseRecord) ScopeEscalationApproval() string {
	return record.scopeEscalationApproval
}

// PropagationTargets returns a defensive copy of explicitly declared targets.
func (record ReleaseRecord) PropagationTargets() []branch.BranchName {
	return append([]branch.BranchName(nil), record.propagationTargets...)
}

// RequiresCommitBudgetException reports whether the semantic manifest exceeds
// the normal one-to-three commit budget.
func (record ReleaseRecord) RequiresCommitBudgetException() bool {
	return len(record.manifest) >= 4
}

// ValidateMainPatchDelivery verifies the main-specific patch-release
// invariants after ParseRecord has verified the generic record contract.
func (record ReleaseRecord) ValidateMainPatchDelivery() error {
	if record.affectedLine.Family() != branch.FamilyMain {
		return invalidRecord(
			"affected line",
			"a main hotfix record",
			"set affectedLine and expectedPullRequest.target to main for a main patch delivery",
		)
	}
	if record.expectedTarget.String() != record.affectedLine.String() {
		return invalidRecord(
			"expected pull request target",
			"the same protected line as affectedLine",
			"bind the expected pull request to the line repaired by this hotfix",
		)
	}
	previousVersion, err := branch.ParseSemanticVersion(strings.TrimPrefix(record.previousTag, "v"))
	if err != nil {
		return invalidRecord(
			"previous tag",
			"v<stable-semver>",
			"record the exact previous immutable main release tag",
		)
	}
	if !isPatchSuccessor(previousVersion, record.targetVersion) {
		return invalidRecord(
			"target version",
			"the next stable patch version after the previous tag",
			"increment only the patch component for a backward-compatible main hotfix",
		)
	}
	return nil
}

// ParseRecord decodes and validates the stable JSON representation of a
// reviewed hotfix release record. It rejects unknown and trailing JSON so a
// controller never silently ignores governance-relevant fields.
func ParseRecord(contents []byte) (ReleaseRecord, error) {
	var raw rawRecord
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return ReleaseRecord{}, invalidRecord(
			"hotfix release record",
			"a valid JSON record with the supported schema",
			"correct the JSON structure before publishing the hotfix",
		)
	}
	if err := ensureNoTrailingJSON(decoder); err != nil {
		return ReleaseRecord{}, err
	}
	if raw.SchemaVersion != recordSchemaVersion {
		return ReleaseRecord{}, invalidRecord(
			"record schema version",
			"1",
			"upgrade or recreate the record with the supported schema version",
		)
	}

	id, err := ticket.ParseID(raw.Ticket)
	if err != nil {
		return ReleaseRecord{}, err
	}
	incident := strings.TrimSpace(raw.Incident)
	if incident == "" || len(incident) > 200 || strings.ContainsAny(incident, "\r\n\t") {
		return ReleaseRecord{}, invalidRecord(
			"incident",
			"a non-empty one-line incident reference of at most 200 characters",
			"record the incident or operational reference without control characters",
		)
	}
	affected, err := branch.ParseName(raw.AffectedLine)
	if err != nil {
		return ReleaseRecord{}, err
	}
	switch affected.Family() {
	case branch.FamilyMain, branch.FamilyRelease, branch.FamilySupport:
	default:
		return ReleaseRecord{}, invalidRecord(
			"affected line",
			"main, release/<semver>, or support/<major.minor>",
			"bind the record to the active line that carries the defect",
		)
	}
	targetVersion, err := branch.ParseSemanticVersion(raw.TargetVersion)
	if err != nil {
		return ReleaseRecord{}, err
	}
	if !isStableVersion(targetVersion) {
		return ReleaseRecord{}, invalidRecord(
			"target version",
			"a stable major.minor.patch version",
			"do not use prerelease or build metadata for a production hotfix patch delivery",
		)
	}
	if !strings.HasPrefix(raw.PreviousTag, "v") {
		return ReleaseRecord{}, invalidRecord(
			"previous tag",
			"v<stable-semver>",
			"bind the record to the previous immutable release tag",
		)
	}
	previousVersion, err := branch.ParseSemanticVersion(strings.TrimPrefix(raw.PreviousTag, "v"))
	if err != nil || "v"+previousVersion.String() != raw.PreviousTag || !isStableVersion(previousVersion) {
		return ReleaseRecord{}, invalidRecord(
			"previous tag",
			"v<stable-semver>",
			"bind the record to one exact stable immutable release tag",
		)
	}

	source, err := branch.ParseName(raw.ExpectedPullRequest.Source)
	if err != nil {
		return ReleaseRecord{}, err
	}
	if source.Family() != branch.FamilyHotfix {
		return ReleaseRecord{}, invalidRecord(
			"expected pull request source",
			"a hotfix/<ticket>-<slug> branch",
			"bind the record to the ticket-scoped hotfix branch reviewed for delivery",
		)
	}
	sourceTicket, found := source.Ticket()
	if !found || sourceTicket.String() != id.String() {
		return ReleaseRecord{}, invalidRecord(
			"expected pull request source",
			"a hotfix branch carrying the record ticket",
			"use the same ticket in the hotfix branch and release record",
		)
	}
	target, err := branch.ParseName(raw.ExpectedPullRequest.Target)
	if err != nil {
		return ReleaseRecord{}, err
	}
	if target.String() != affected.String() {
		return ReleaseRecord{}, invalidRecord(
			"expected pull request target",
			"the affected active line",
			"bind the reviewed pull request to the production line repaired by the hotfix",
		)
	}

	manifest, err := parseManifest(raw.Manifest)
	if err != nil {
		return ReleaseRecord{}, err
	}
	exception := strings.TrimSpace(raw.CommitBudgetException)
	approval := strings.TrimSpace(raw.ScopeEscalationApproval)
	if len(manifest) >= 4 && (exception == "" || len(exception) > 400 || strings.ContainsAny(exception, "\r\n\t")) {
		return ReleaseRecord{}, invalidRecord(
			"commit budget exception",
			"a concise one-line reason for a manifest exceeding three semantic commits",
			"document why the ordered correction cannot be split safely",
		)
	}
	if len(manifest) < 4 && exception != "" {
		return ReleaseRecord{}, invalidRecord(
			"commit budget exception",
			"an empty value for a one-to-three commit manifest",
			"remove the exception unless the manifest contains at least four semantic commits",
		)
	}
	if len(manifest) < 5 && approval != "" {
		return ReleaseRecord{}, invalidRecord(
			"scope escalation approval",
			"an empty value for a manifest of four or fewer semantic commits",
			"remove the escalation approval unless the manifest contains five or more semantic commits",
		)
	}
	if len(manifest) >= 5 && (approval == "" || len(approval) > 400 || strings.ContainsAny(approval, "\r\n\t")) {
		return ReleaseRecord{}, invalidRecord(
			"scope escalation approval",
			"a concise one-line explicit release approval reference for five or more semantic commits",
			"record the approved scope escalation before the main hotfix merge",
		)
	}

	targets, err := parsePropagationTargets(raw.PropagationTargets, affected)
	if err != nil {
		return ReleaseRecord{}, err
	}
	return ReleaseRecord{
		ticket:                  id,
		incident:                incident,
		affectedLine:            affected,
		targetVersion:           targetVersion,
		previousTag:             raw.PreviousTag,
		expectedSource:          source,
		expectedTarget:          target,
		manifest:                manifest,
		commitBudgetException:   exception,
		scopeEscalationApproval: approval,
		propagationTargets:      targets,
	}, nil
}

type rawRecord struct {
	SchemaVersion           int                    `json:"schemaVersion"`
	Ticket                  string                 `json:"ticket"`
	Incident                string                 `json:"incident"`
	AffectedLine            string                 `json:"affectedLine"`
	TargetVersion           string                 `json:"targetVersion"`
	PreviousTag             string                 `json:"previousTag"`
	ExpectedPullRequest     rawExpectedPullRequest `json:"expectedPullRequest"`
	Manifest                []string               `json:"manifest"`
	CommitBudgetException   string                 `json:"commitBudgetException"`
	ScopeEscalationApproval string                 `json:"scopeEscalationApproval"`
	PropagationTargets      []string               `json:"propagationTargets"`
}

type rawExpectedPullRequest struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

func ensureNoTrailingJSON(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	}
	return invalidRecord(
		"hotfix release record",
		"exactly one JSON object",
		"remove trailing JSON values before publishing the hotfix",
	)
}

func parseManifest(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, invalidRecord(
			"hotfix commit manifest",
			"at least one ordered full 40-character commit SHA",
			"record every semantic hotfix commit in merge order",
		)
	}
	seen := make(map[string]struct{}, len(raw))
	manifest := make([]string, 0, len(raw))
	for _, commit := range raw {
		if !fullCommitPattern.MatchString(commit) {
			return nil, invalidRecord(
				"hotfix commit manifest",
				"lowercase full 40-character commit SHAs",
				"record immutable full commit IDs without abbreviated refs",
			)
		}
		if _, duplicate := seen[commit]; duplicate {
			return nil, invalidRecord(
				"hotfix commit manifest",
				"an ordered manifest without duplicate commits",
				"list each semantic hotfix commit exactly once",
			)
		}
		seen[commit] = struct{}{}
		manifest = append(manifest, commit)
	}
	return manifest, nil
}

func parsePropagationTargets(raw []string, affected branch.BranchName) ([]branch.BranchName, error) {
	if raw == nil {
		return nil, invalidRecord(
			"propagation targets",
			"an explicit JSON array, which may be empty",
			"declare every additional active line or explicitly provide an empty array",
		)
	}
	seen := make(map[string]struct{}, len(raw))
	targets := make([]branch.BranchName, 0, len(raw))
	for _, value := range raw {
		target, err := branch.ParseName(value)
		if err != nil {
			return nil, err
		}
		switch target.Family() {
		case branch.FamilyDevelop, branch.FamilyRelease, branch.FamilySupport:
		default:
			return nil, invalidRecord(
				"propagation target",
				"develop, release/<semver>, or support/<major.minor>",
				"declare only additional active lines that require an explicit reviewed propagation",
			)
		}
		if target.String() == affected.String() {
			return nil, invalidRecord(
				"propagation target",
				"a line other than the affected line",
				"remove the hotfix source line from the additional propagation targets",
			)
		}
		if _, duplicate := seen[target.String()]; duplicate {
			return nil, invalidRecord(
				"propagation targets",
				"unique target lines",
				"declare each additional active line once",
			)
		}
		seen[target.String()] = struct{}{}
		targets = append(targets, target)
	}
	return targets, nil
}

func isStableVersion(version branch.SemanticVersion) bool {
	return stableVersionParts.MatchString(version.String())
}

func isPatchSuccessor(previous, target branch.SemanticVersion) bool {
	previousParts := stableVersionParts.FindStringSubmatch(previous.String())
	targetParts := stableVersionParts.FindStringSubmatch(target.String())
	if len(previousParts) != 4 || len(targetParts) != 4 {
		return false
	}
	if previousParts[1] != targetParts[1] || previousParts[2] != targetParts[2] {
		return false
	}
	previousPatch := new(big.Int)
	previousPatch.SetString(previousParts[3], 10)
	targetPatch := new(big.Int)
	targetPatch.SetString(targetParts[3], 10)
	return targetPatch.Cmp(new(big.Int).Add(previousPatch, big.NewInt(1))) == 0
}

func invalidRecord(field, expected, remediation string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeInvalidInput,
		Category:    problem.CategoryGovernance,
		Field:       field,
		Expected:    expected,
		Rule:        "production hotfix delivery requires a complete reviewed release record",
		Remediation: remediation,
	})
}
