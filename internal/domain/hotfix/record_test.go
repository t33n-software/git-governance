package hotfix

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/CyberT33N/git-governance/internal/domain/branch"
)

func TestParseRecordAcceptsCompleteMainPatchRecord(t *testing.T) {
	raw := validRawRecord()
	record, err := ParseRecord(mustMarshalRecord(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	if err := record.ValidateMainPatchDelivery(); err != nil {
		t.Fatal(err)
	}
	if record.Ticket().String() != "GOV-42" ||
		record.Incident() != "INC-42" ||
		record.AffectedLine().String() != "main" ||
		record.TargetVersion().String() != "1.0.2" ||
		record.PreviousTag() != "v1.0.1" ||
		record.ExpectedSource().String() != "hotfix/GOV-42-main-hotfix-patch-delivery" ||
		record.ExpectedTarget().String() != "main" {
		t.Fatalf("record accessors = %#v", record)
	}

	manifest := record.Manifest()
	manifest[0] = strings.Repeat("b", 40)
	if record.Manifest()[0] != strings.Repeat("a", 40) {
		t.Fatal("Manifest returned mutable internal state")
	}
	targets := record.PropagationTargets()
	targets[0], _ = branch.ParseName("support/1.0")
	if record.PropagationTargets()[0].String() != "develop" {
		t.Fatal("PropagationTargets returned mutable internal state")
	}
	if record.RequiresCommitBudgetException() {
		t.Fatal("one-commit record unexpectedly requires an exception")
	}
}

func TestParseRecordCommitBudget(t *testing.T) {
	t.Run("accepts a reviewed four-commit exception", func(t *testing.T) {
		raw := validRawRecord()
		raw.Manifest = []string{
			strings.Repeat("a", 40),
			strings.Repeat("b", 40),
			strings.Repeat("c", 40),
			strings.Repeat("d", 40),
		}
		raw.CommitBudgetException = "The ordered correction cannot be split into testable intermediate states."

		record, err := ParseRecord(mustMarshalRecord(t, raw))
		if err != nil {
			t.Fatal(err)
		}
		if !record.RequiresCommitBudgetException() || record.CommitBudgetException() != raw.CommitBudgetException {
			t.Fatalf("record budget = %#v", record)
		}
	})

	t.Run("accepts an explicitly approved scope escalation", func(t *testing.T) {
		raw := validRawRecord()
		raw.Manifest = []string{
			strings.Repeat("a", 40),
			strings.Repeat("b", 40),
			strings.Repeat("c", 40),
			strings.Repeat("d", 40),
			strings.Repeat("e", 40),
		}
		raw.CommitBudgetException = "The atomic incident correction spans five dependent semantic changes."
		raw.ScopeEscalationApproval = "REL-APPROVAL-42"

		record, err := ParseRecord(mustMarshalRecord(t, raw))
		if err != nil {
			t.Fatal(err)
		}
		if !record.RequiresCommitBudgetException() || record.ScopeEscalationApproval() != raw.ScopeEscalationApproval {
			t.Fatalf("record escalation = %#v", record)
		}
	})

	for _, testCase := range []struct {
		name   string
		mutate func(*rawRecord)
	}{
		{
			name: "requires an exception at four commits",
			mutate: func(raw *rawRecord) {
				raw.Manifest = []string{
					strings.Repeat("a", 40),
					strings.Repeat("b", 40),
					strings.Repeat("c", 40),
					strings.Repeat("d", 40),
				}
			},
		},
		{
			name: "rejects a multiline exception",
			mutate: func(raw *rawRecord) {
				raw.Manifest = []string{
					strings.Repeat("a", 40),
					strings.Repeat("b", 40),
					strings.Repeat("c", 40),
					strings.Repeat("d", 40),
				}
				raw.CommitBudgetException = "first line\nsecond line"
			},
		},
		{
			name: "rejects an exception below the threshold",
			mutate: func(raw *rawRecord) {
				raw.CommitBudgetException = "not needed"
			},
		},
		{
			name: "rejects scope escalation without a separate approval",
			mutate: func(raw *rawRecord) {
				raw.Manifest = []string{
					strings.Repeat("a", 40),
					strings.Repeat("b", 40),
					strings.Repeat("c", 40),
					strings.Repeat("d", 40),
					strings.Repeat("e", 40),
				}
				raw.CommitBudgetException = "too many"
			},
		},
		{
			name: "rejects escalation approval below five commits",
			mutate: func(raw *rawRecord) {
				raw.ScopeEscalationApproval = "REL-APPROVAL-42"
			},
		},
		{
			name: "rejects multiline escalation approval",
			mutate: func(raw *rawRecord) {
				raw.Manifest = []string{
					strings.Repeat("a", 40),
					strings.Repeat("b", 40),
					strings.Repeat("c", 40),
					strings.Repeat("d", 40),
					strings.Repeat("e", 40),
				}
				raw.CommitBudgetException = "too many"
				raw.ScopeEscalationApproval = "REL-APPROVAL\n42"
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			raw := validRawRecord()
			testCase.mutate(&raw)
			if _, err := ParseRecord(mustMarshalRecord(t, raw)); err == nil {
				t.Fatal("ParseRecord unexpectedly accepted an invalid budget")
			}
		})
	}
}

func TestParseRecordRejectsInvalidFields(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		mutate func(*rawRecord)
	}{
		{
			name: "unsupported schema version",
			mutate: func(raw *rawRecord) {
				raw.SchemaVersion = 2
			},
		},
		{
			name: "invalid ticket",
			mutate: func(raw *rawRecord) {
				raw.Ticket = "gov-42"
			},
		},
		{
			name: "empty incident",
			mutate: func(raw *rawRecord) {
				raw.Incident = " "
			},
		},
		{
			name: "incident with control characters",
			mutate: func(raw *rawRecord) {
				raw.Incident = "INC-42\tunsafe"
			},
		},
		{
			name: "incident over the limit",
			mutate: func(raw *rawRecord) {
				raw.Incident = strings.Repeat("x", 201)
			},
		},
		{
			name: "invalid affected branch grammar",
			mutate: func(raw *rawRecord) {
				raw.AffectedLine = "main/unsafe"
			},
		},
		{
			name: "non production affected line",
			mutate: func(raw *rawRecord) {
				raw.AffectedLine = "develop"
				raw.ExpectedPullRequest.Target = "develop"
			},
		},
		{
			name: "invalid target version",
			mutate: func(raw *rawRecord) {
				raw.TargetVersion = "v1.0.2"
			},
		},
		{
			name: "unstable target version",
			mutate: func(raw *rawRecord) {
				raw.TargetVersion = "1.0.2-rc.1"
			},
		},
		{
			name: "missing previous tag prefix",
			mutate: func(raw *rawRecord) {
				raw.PreviousTag = "1.0.1"
			},
		},
		{
			name: "invalid previous tag",
			mutate: func(raw *rawRecord) {
				raw.PreviousTag = "v01.0.1"
			},
		},
		{
			name: "unstable previous tag",
			mutate: func(raw *rawRecord) {
				raw.PreviousTag = "v1.0.1-rc.1"
			},
		},
		{
			name: "invalid expected source",
			mutate: func(raw *rawRecord) {
				raw.ExpectedPullRequest.Source = "hotfix/invalid"
			},
		},
		{
			name: "non hotfix expected source",
			mutate: func(raw *rawRecord) {
				raw.ExpectedPullRequest.Source = "fix/GOV-42-main-hotfix-patch-delivery"
			},
		},
		{
			name: "mismatched expected source ticket",
			mutate: func(raw *rawRecord) {
				raw.ExpectedPullRequest.Source = "hotfix/GOV-43-main-hotfix-patch-delivery"
			},
		},
		{
			name: "invalid expected target",
			mutate: func(raw *rawRecord) {
				raw.ExpectedPullRequest.Target = "not-a-branch"
			},
		},
		{
			name: "mismatched expected target",
			mutate: func(raw *rawRecord) {
				raw.ExpectedPullRequest.Target = "develop"
			},
		},
		{
			name: "missing manifest",
			mutate: func(raw *rawRecord) {
				raw.Manifest = nil
			},
		},
		{
			name: "abbreviated manifest commit",
			mutate: func(raw *rawRecord) {
				raw.Manifest = []string{"abc1234"}
			},
		},
		{
			name: "duplicate manifest commit",
			mutate: func(raw *rawRecord) {
				raw.Manifest = []string{strings.Repeat("a", 40), strings.Repeat("a", 40)}
			},
		},
		{
			name: "missing propagation target declaration",
			mutate: func(raw *rawRecord) {
				raw.PropagationTargets = nil
			},
		},
		{
			name: "invalid propagation target",
			mutate: func(raw *rawRecord) {
				raw.PropagationTargets = []string{"main"}
			},
		},
		{
			name: "malformed propagation target",
			mutate: func(raw *rawRecord) {
				raw.PropagationTargets = []string{"release/not-semver"}
			},
		},
		{
			name: "duplicate propagation target",
			mutate: func(raw *rawRecord) {
				raw.PropagationTargets = []string{"develop", "develop"}
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			raw := validRawRecord()
			testCase.mutate(&raw)
			if _, err := ParseRecord(mustMarshalRecord(t, raw)); err == nil {
				t.Fatal("ParseRecord unexpectedly accepted an invalid record")
			}
		})
	}
}

func TestParseRecordRejectsMalformedAndTrailingJSON(t *testing.T) {
	for _, contents := range [][]byte{
		[]byte("{"),
		[]byte(`{"schemaVersion":1,"unknown":true}`),
		append(mustMarshalRecord(t, validRawRecord()), []byte(` {}`)...),
	} {
		if _, err := ParseRecord(contents); err == nil {
			t.Fatalf("ParseRecord(%q) unexpectedly succeeded", contents)
		}
	}
}

func TestValidateMainPatchDeliveryRejectsNonMainOrNonSuccessor(t *testing.T) {
	record, err := ParseRecord(mustMarshalRecord(t, validRawRecord()))
	if err != nil {
		t.Fatal(err)
	}

	record.affectedLine, _ = branch.ParseName("release/1.0.2")
	if err := record.ValidateMainPatchDelivery(); err == nil {
		t.Fatal("ValidateMainPatchDelivery accepted a non-main record")
	}

	record = mustRecord(t, validRawRecord())
	record.expectedTarget, _ = branch.ParseName("develop")
	if err := record.ValidateMainPatchDelivery(); err == nil {
		t.Fatal("ValidateMainPatchDelivery accepted a mismatched expected target")
	}

	record = mustRecord(t, validRawRecord())
	record.previousTag = "vnot-semver"
	if err := record.ValidateMainPatchDelivery(); err == nil {
		t.Fatal("ValidateMainPatchDelivery accepted an invalid previous tag")
	}

	record = mustRecord(t, validRawRecord())
	record.previousTag = "v1.0.1 "
	if err := record.ValidateMainPatchDelivery(); err == nil {
		t.Fatal("ValidateMainPatchDelivery accepted a noncanonical previous tag")
	}

	record = mustRecord(t, validRawRecord())
	record.targetVersion, _ = branch.ParseSemanticVersion("1.1.2")
	if err := record.ValidateMainPatchDelivery(); err == nil {
		t.Fatal("ValidateMainPatchDelivery accepted a non-patch successor")
	}
}

func TestParsePropagationTargetsRejectsAffectedLine(t *testing.T) {
	affected, _ := branch.ParseName("release/1.0.2")
	if _, err := parsePropagationTargets([]string{"release/1.0.2"}, affected); err == nil {
		t.Fatal("parsePropagationTargets accepted the affected line")
	}
}

func TestIsPatchSuccessor(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		previous string
		target   string
		want     bool
	}{
		{name: "next patch", previous: "1.0.1", target: "1.0.2", want: true},
		{name: "large next patch", previous: "1.0.999999999999999999999999", target: "1.0.1000000000000000000000000", want: true},
		{name: "same patch", previous: "1.0.1", target: "1.0.1"},
		{name: "changed minor", previous: "1.0.1", target: "1.1.2"},
		{name: "changed major", previous: "1.0.1", target: "2.0.2"},
		{name: "pre release", previous: "1.0.1", target: "1.0.2-rc.1"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			previous, _ := branch.ParseSemanticVersion(testCase.previous)
			target, _ := branch.ParseSemanticVersion(testCase.target)
			if got := isPatchSuccessor(previous, target); got != testCase.want {
				t.Fatalf("isPatchSuccessor(%q, %q) = %t, want %t", testCase.previous, testCase.target, got, testCase.want)
			}
		})
	}
}

func validRawRecord() rawRecord {
	return rawRecord{
		SchemaVersion: 1,
		Ticket:        "GOV-42",
		Incident:      "INC-42",
		AffectedLine:  "main",
		TargetVersion: "1.0.2",
		PreviousTag:   "v1.0.1",
		ExpectedPullRequest: rawExpectedPullRequest{
			Source: "hotfix/GOV-42-main-hotfix-patch-delivery",
			Target: "main",
		},
		Manifest:           []string{strings.Repeat("a", 40)},
		PropagationTargets: []string{"develop"},
	}
}

func mustRecord(t *testing.T, raw rawRecord) ReleaseRecord {
	t.Helper()

	record, err := ParseRecord(mustMarshalRecord(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func mustMarshalRecord(t *testing.T, raw rawRecord) []byte {
	t.Helper()

	contents, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}
