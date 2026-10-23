package bootstrap

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/cliparam"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func executeMessageScaffold(t *testing.T, arguments ...string) (map[string]string, error) {
	t.Helper()

	root := New(BuildInfo{Version: "test"})
	stdout := &strings.Builder{}
	root.SetOut(stdout)
	root.SetErr(stdout)
	root.SetArgs(append([]string{"message", "scaffold", "--output", "json", "--interactive", "never"}, arguments...))
	err := root.Execute()
	if err != nil {
		return nil, err
	}
	var result struct {
		Fields map[string]string `json:"fields"`
	}
	if decodeErr := json.NewDecoder(strings.NewReader(stdout.String())).Decode(&result); decodeErr != nil {
		t.Fatalf("decode scaffold report: %v\n%s", decodeErr, stdout.String())
	}
	return result.Fields, nil
}

func TestMessageScaffoldCommitBodyCreatesTheCanonicalSkeleton(t *testing.T) {
	fields, err := executeMessageScaffold(t, "--kind", "commit-body")
	if err != nil {
		t.Fatalf("scaffold commit-body: %v", err)
	}
	defer func() { _ = os.Remove(fields["path"]) }()

	if fields["kind"] != "commit-body" {
		t.Fatalf("kind field = %q, want commit-body", fields["kind"])
	}
	if !strings.HasSuffix(fields["path"], ".txt") {
		t.Fatalf("scaffolded path %q does not carry the .txt suffix", fields["path"])
	}
	if !filepath.IsAbs(fields["path"]) {
		t.Fatalf("scaffolded path %q is not absolute", fields["path"])
	}
	contents, readErr := os.ReadFile(fields["path"])
	if readErr != nil {
		t.Fatalf("read scaffolded file: %v", readErr)
	}
	for _, section := range commitmsg.BodySections() {
		block := "## " + section.Name + "\n\n" + section.Slot
		if !strings.Contains(string(contents), block) {
			t.Fatalf("scaffolded commit body misses %q:\n%s", block, contents)
		}
	}
	resolved, resolveErr := resolveMessageFile(fields["path"], "body-file")
	if resolveErr != nil {
		t.Fatalf("the scaffolded file must satisfy the message-file transport contract: %v", resolveErr)
	}
	if resolved != commitmsg.BodySkeleton() {
		t.Fatalf("scaffolded contents drifted from the register skeleton:\n%s", resolved)
	}
}

func TestMessageScaffoldPullRequestCreatesTheFiveSectionSkeleton(t *testing.T) {
	fields, err := executeMessageScaffold(t, "--kind", "pr-description")
	if err != nil {
		t.Fatalf("scaffold pr-description: %v", err)
	}
	defer func() { _ = os.Remove(fields["path"]) }()

	contents, readErr := os.ReadFile(fields["path"])
	if readErr != nil {
		t.Fatalf("read scaffolded file: %v", readErr)
	}
	for _, section := range pullRequestDescriptionSections {
		if !strings.Contains(string(contents), "## "+section+"\n\n") {
			t.Fatalf("scaffolded description misses the %q heading:\n%s", section, contents)
		}
	}
}

func TestMessageScaffoldScaffoldedPathIsAbsolute(t *testing.T) {
	fields, err := executeMessageScaffold(t, "--kind", "commit-body")
	if err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	defer func() { _ = os.Remove(fields["path"]) }()
	if !filepath.IsAbs(fields["path"]) {
		t.Fatalf("scaffolded path %q is not absolute", fields["path"])
	}
}

func TestMessageScaffoldRejectsAnUnknownKind(t *testing.T) {
	_, err := executeMessageScaffold(t, "--kind", "nope")
	if err == nil {
		t.Fatal("unknown kind must fail closed")
	}
	typed, ok := problem.As(err)
	if !ok || typed.Code != problem.CodeInvalidInput || typed.Field != "kind" {
		t.Fatalf("unexpected failure: %+v", err)
	}
	if !strings.Contains(typed.Expected, "commit-body") || !strings.Contains(typed.Expected, "pr-description") {
		t.Fatalf("error must render the register value list: %q", typed.Expected)
	}
}

func TestMessageScaffoldFailsWhenTheTemporaryAreaIsUnwritable(t *testing.T) {
	original := createScaffoldFile
	createScaffoldFile = func(_, _ string) (*os.File, error) {
		return nil, os.ErrPermission
	}
	t.Cleanup(func() { createScaffoldFile = original })

	_, err := executeMessageScaffold(t, "--kind", "commit-body")
	typed, ok := problem.As(err)
	if !ok || typed.Code != problem.CodeMessageScaffoldFailed {
		t.Fatalf("create failure must map to the scaffold code: %+v", err)
	}
	if typed.Category != problem.CategoryExternal {
		t.Fatalf("category = %q, want external", typed.Category)
	}
}

func TestMessageScaffoldRemovesTheFileWhenTheWriteFails(t *testing.T) {
	original := writeScaffoldBody
	var created *os.File
	writeScaffoldBody = func(file *os.File, _ string) error {
		created = file
		return os.ErrClosed
	}
	t.Cleanup(func() { writeScaffoldBody = original })

	_, err := executeMessageScaffold(t, "--kind", "commit-body")
	typed, ok := problem.As(err)
	if !ok || typed.Code != problem.CodeMessageScaffoldFailed {
		t.Fatalf("write failure must map to the scaffold code: %+v", err)
	}
	// The handle must be released before the cleanup removal: on platforms
	// where an open file cannot be unlinked, a leaked handle silently keeps
	// the half-written file alive. A second Close proves the release.
	if closeErr := created.Close(); !errors.Is(closeErr, os.ErrClosed) {
		t.Fatalf("the scaffold must release the file handle before cleanup, close error = %v", closeErr)
	}
	if _, statErr := os.Stat(created.Name()); !os.IsNotExist(statErr) {
		t.Fatalf("a partially written scaffold file must be removed, stat: %v", statErr)
	}
}

func TestMessageScaffoldSourcesMatchTheClosedEnumDomain(t *testing.T) {
	t.Parallel()

	domain := cliparam.MessageScaffoldKind()
	if domain.Class != cliparam.ClassClosedEnum {
		t.Fatalf("class = %q, want closed-enum", domain.Class)
	}
	values := messageScaffoldKindValues()
	if strings.Join(values, "|") != strings.Join(domain.Values, "|") || len(values) != len(messageScaffoldSources) {
		t.Fatalf("scaffold sources %v drifted from the domain values %v", values, domain.Values)
	}
}

// TestMessageScaffoldKindValuesCollapsesOnDrift proves the drift detector
// itself: a source register that no longer matches the closed-enum domain
// values must collapse the check instead of reporting a false alignment.
func TestMessageScaffoldKindValuesCollapsesOnDrift(t *testing.T) {
	original := messageScaffoldSources
	messageScaffoldSources = map[string]func() string{
		messageKindCommitBody:  commitmsg.BodySkeleton,
		messageKindPullRequest: pullRequestBodySkeleton,
		"orphan-kind":          func() string { return "" },
	}
	t.Cleanup(func() { messageScaffoldSources = original })

	if values := messageScaffoldKindValues(); values != nil {
		t.Fatalf("a drifted source register must collapse the check, got %v", values)
	}
}

// TestCommitBodyEndpointsRenderTheDomainLayout pins the commit-body layout
// projection channel: every command that consumes a commit body message file
// renders the complete canonical layout block from the commit message domain
// register in its help, and no other command carries it. The tree navigation
// stays sequential because cobra's Find lazily initializes command state.
func TestCommitBodyEndpointsRenderTheDomainLayout(t *testing.T) {
	root := New(BuildInfo{Version: "test"})
	consumers := [][]string{
		{"commit", "create"},
		{"branch", "merge-scratch"},
		{"branch", "sync-base"},
		{"workflow", "ticket", "publish"},
	}

	layout := commitBodyLayoutHelp()
	for _, path := range consumers {
		command, _, err := root.Find(path)
		if err != nil || command == nil {
			t.Fatalf("command path %v not found: %v", path, err)
		}
		if !strings.Contains(command.Long, layout) {
			t.Fatalf("%v help misses the canonical commit body layout block:\n%s", path, command.Long)
		}
	}

	nonConsumer, _, err := root.Find([]string{"branch", "validate"})
	if err != nil || nonConsumer == nil {
		t.Fatalf("branch validate not found: %v", err)
	}
	if strings.Contains(nonConsumer.Long, "Commit body layout") {
		t.Fatalf("branch validate help must not carry the commit body layout block:\n%s", nonConsumer.Long)
	}
}

func TestMessageCommandTreeCarriesTheScaffoldNode(t *testing.T) {
	t.Parallel()

	root := New(BuildInfo{Version: "test"})
	command, _, err := root.Find([]string{"message", "scaffold"})
	if err != nil || command == nil {
		t.Fatalf("message scaffold not found: %v", err)
	}
	if command.Use != "scaffold" {
		t.Fatalf("Use = %q, want scaffold", command.Use)
	}
}

// TestWriteScaffoldContentsPropagatesWriteFailures pins the production seam
// itself: a write to an unusable file handle must surface the platform error
// so the scaffold maps it to the coded failure record and removes the file.
func TestWriteScaffoldContentsPropagatesWriteFailures(t *testing.T) {
	t.Parallel()

	file, err := os.CreateTemp(t.TempDir(), "scaffold-seam-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if writeErr := writeScaffoldContents(file, "body"); !errors.Is(writeErr, os.ErrClosed) {
		t.Fatalf("write to a closed file = %v, want os.ErrClosed", writeErr)
	}
}
