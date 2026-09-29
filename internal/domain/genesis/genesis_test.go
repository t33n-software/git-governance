package genesis

import (
	"strings"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

func TestNewRecordBindsTheBirthFacts(t *testing.T) {
	t.Parallel()

	id := mustTicket(t, "ABC-1")
	createdAt := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	record := NewRecord(
		"C:/work/repository",
		"origin",
		id,
		"0123456789abcdef0123456789abcdef01234567",
		[]string{"main", "develop"},
		"schemaVersion=1 keyPolicy=syntax-only commitSigning=required",
		"lane@example.invalid",
		createdAt,
	)

	if record.SchemaVersion != 1 ||
		record.Repository != "C:/work/repository" ||
		record.Remote != "origin" ||
		record.Ticket != "ABC-1" ||
		record.Revision != "0123456789abcdef0123456789abcdef01234567" ||
		len(record.Refs) != 2 || record.Refs[0] != "main" || record.Refs[1] != "develop" ||
		!record.SignatureVerified ||
		record.PolicySnapshot != "schemaVersion=1 keyPolicy=syntax-only commitSigning=required" ||
		record.Actor != "lane@example.invalid" ||
		!record.CreatedAt.Equal(createdAt) {
		t.Fatalf("NewRecord() = %#v", record)
	}
}

func TestScanContentSetAcceptsCleanContent(t *testing.T) {
	t.Parallel()

	for _, paths := range [][]string{
		nil,
		{},
		{"README.md", "cmd/tool/main.go", "docs/usage/index.md"},
		{"config/.env.example", ".env.sample", "deploy/.env.template"},
		{"docs/private-key-notes.md", "scripts/deploy.sh", "internal/keeper/store.go"},
	} {
		if err := ScanContentSet(paths); err != nil {
			t.Fatalf("ScanContentSet(%v) = %v", paths, err)
		}
	}
}

func TestScanContentSetRejectsBoundaryArtifacts(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		paths []string
		want  []string
	}{
		{name: "environment file", paths: []string{".env"}, want: []string{".env"}},
		{name: "nested environment file", paths: []string{"config/.env"}, want: []string{"config/.env"}},
		{name: "environment variant", paths: []string{".env.production"}, want: []string{".env.production"}},
		{name: "uppercase environment variant", paths: []string{".ENV.LOCAL"}, want: []string{".ENV.LOCAL"}},
		{name: "netrc", paths: []string{".netrc"}, want: []string{".netrc"}},
		{name: "git credentials", paths: []string{".git-credentials"}, want: []string{".git-credentials"}},
		{name: "rsa key", paths: []string{".ssh/id_rsa"}, want: []string{".ssh/id_rsa"}},
		{name: "ed25519 key", paths: []string{"id_ed25519"}, want: []string{"id_ed25519"}},
		{name: "dsa key", paths: []string{"id_dsa"}, want: []string{"id_dsa"}},
		{name: "ecdsa key", paths: []string{"id_ecdsa"}, want: []string{"id_ecdsa"}},
		{name: "pem", paths: []string{"tls/server.pem"}, want: []string{"tls/server.pem"}},
		{name: "key extension", paths: []string{"signing.key"}, want: []string{"signing.key"}},
		{name: "p12", paths: []string{"store.p12"}, want: []string{"store.p12"}},
		{name: "pfx", paths: []string{"store.pfx"}, want: []string{"store.pfx"}},
		{name: "ppk", paths: []string{"store.ppk"}, want: []string{"store.ppk"}},
		{name: "credentials extension", paths: []string{"aws.credentials"}, want: []string{"aws.credentials"}},
		{
			name:  "every violation is reported",
			paths: []string{"README.md", ".env", "keys/id_rsa", "docs/guide.md"},
			want:  []string{".env", "keys/id_rsa"},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := ScanContentSet(testCase.paths)
			if err == nil {
				t.Fatalf("ScanContentSet(%v) unexpectedly passed", testCase.paths)
			}
			typed, ok := problem.As(err)
			if !ok {
				t.Fatalf("ScanContentSet(%v) error does not carry a problem: %v", testCase.paths, err)
			}
			if typed.Code != problem.CodeContentBoundaryViolation || typed.Category != problem.CategoryGovernance {
				t.Fatalf("ScanContentSet(%v) problem = %#v", testCase.paths, typed.Details)
			}
			for _, wanted := range testCase.want {
				if !strings.Contains(typed.Actual, wanted) {
					t.Fatalf("ScanContentSet(%v) actual %q misses %q", testCase.paths, typed.Actual, wanted)
				}
			}
		})
	}
}

func TestCommitBodyCarriesTheCanonicalCategories(t *testing.T) {
	t.Parallel()

	body := CommitBody(BodyFacts{
		Ticket:         mustTicket(t, "ABC-1"),
		ContentFiles:   3,
		PolicySnapshot: "schemaVersion=1 keyPolicy=syntax-only commitSigning=required",
	})

	for _, category := range []string{
		"## Motivation",
		"## Behavioral Change",
		"## Contracts and Invariants",
		"## Verification",
		"## Risks and Follow-ups",
	} {
		if !strings.Contains(body, category) {
			t.Fatalf("CommitBody() misses %q:\n%s", category, body)
		}
	}
	if !strings.Contains(body, "ABC-1") || !strings.Contains(body, "3 files") ||
		!strings.Contains(body, "schemaVersion=1 keyPolicy=syntax-only commitSigning=required") {
		t.Fatalf("CommitBody() misses the bound facts:\n%s", body)
	}
	if !strings.Contains(body, "Local-only until the separately confirmed publication step") {
		t.Fatalf("CommitBody() without publication misses the local-only risk note:\n%s", body)
	}

	// The body must survive the creation-time round-trip parse: no line may
	// collide with the footer grammar.
	if _, err := commitmsg.Parse("chore(ABC-1): " + CommitSubject + "\n\n" + body); err != nil {
		t.Fatalf("genesis message does not round-trip: %v", err)
	}
}

func TestCommitBodyWithPublicationNamesTheRemoteBirth(t *testing.T) {
	t.Parallel()

	body := CommitBody(BodyFacts{
		Ticket:         mustTicket(t, "ABC-1"),
		ContentFiles:   1,
		PolicySnapshot: "schemaVersion=1",
		Publish:        true,
	})
	if !strings.Contains(body, "separately confirmed in the same invocation") {
		t.Fatalf("CommitBody() with publication misses the publication risk note:\n%s", body)
	}
	if _, err := commitmsg.Parse("chore(ABC-1): " + CommitSubject + "\n\n" + body); err != nil {
		t.Fatalf("genesis message with publication does not round-trip: %v", err)
	}
}

func mustTicket(t *testing.T, raw string) ticket.ID {
	t.Helper()
	value, err := ticket.ParseID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
