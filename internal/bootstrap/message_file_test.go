package bootstrap

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestResolveMessageFileResolvesEmptyPathsOptionally(t *testing.T) {
	body, err := resolveMessageFile("", "body-file")
	if err != nil || body != "" {
		t.Fatalf("resolveMessageFile(\"\") = (%q, %v), want an empty optional body", body, err)
	}
}

func TestResolveMessageFileRejectsRelativePathsFailClosed(t *testing.T) {
	for _, rawPath := range []string{"./message.txt", "message.txt", "../message.txt"} {
		_, err := resolveMessageFile(rawPath, "body-file")
		typed, ok := problem.As(err)
		if !ok {
			t.Fatalf("resolveMessageFile(%q) error = %v, want a coded problem", rawPath, err)
		}
		if typed.Code != problem.CodeInvalidInput || typed.Category != problem.CategoryUsage {
			t.Fatalf("relative path error = (%s, %s), want an invalid usage input", typed.Code, typed.Category)
		}
		if typed.Field != "body-file" || typed.Actual != rawPath {
			t.Fatalf("relative path error fields = (%q, %q), want the flag field and the actual value", typed.Field, typed.Actual)
		}
		if typed.Expected != "an absolute path" {
			t.Fatalf("relative path expectation = %q, want \"an absolute path\"", typed.Expected)
		}
		if typed.Rule == "" || typed.Remediation == "" {
			t.Fatalf("relative path error carries incomplete guidance: %#v", typed.Details)
		}
	}
}

func TestResolveMessageFileRejectsMissingFiles(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.txt")
	_, err := resolveMessageFile(missing, "body-file")
	typed, ok := problem.As(err)
	if !ok {
		t.Fatalf("missing file error = %v, want a coded problem", err)
	}
	if typed.Code != problem.CodeInvalidInput || typed.Expected != "an existing regular file" {
		t.Fatalf("missing file error = (%s, %q), want an invalid input for an existing regular file", typed.Code, typed.Expected)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file error = %v, want the wrapped not-exist cause", err)
	}
}

func TestResolveMessageFileRejectsDirectoriesAndSpecialFiles(t *testing.T) {
	directory := t.TempDir()
	_, err := resolveMessageFile(directory, "body-file")
	typed, ok := problem.As(err)
	if !ok {
		t.Fatalf("directory error = %v, want a coded problem", err)
	}
	if typed.Code != problem.CodeInvalidInput || typed.Expected != "a regular file" {
		t.Fatalf("directory error = (%s, %q), want an invalid input for a regular file", typed.Code, typed.Expected)
	}
}

func TestResolveMessageFileRejectsOversizedFiles(t *testing.T) {
	oversized := filepath.Join(t.TempDir(), "oversized.txt")
	if err := os.WriteFile(oversized, bytes.Repeat([]byte("x"), maxMessageFileBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := resolveMessageFile(oversized, "body-file")
	typed, ok := problem.As(err)
	if !ok {
		t.Fatalf("oversized error = %v, want a coded problem", err)
	}
	if typed.Code != problem.CodeInvalidInput {
		t.Fatalf("oversized error code = %s, want %s", typed.Code, problem.CodeInvalidInput)
	}
	if typed.Expected != "at most 1048576 bytes" {
		t.Fatalf("oversized expectation = %q, want the transport ceiling", typed.Expected)
	}
}

func TestResolveMessageFileAcceptsFilesWithinTheCeiling(t *testing.T) {
	boundary := filepath.Join(t.TempDir(), "boundary.txt")
	if err := os.WriteFile(boundary, bytes.Repeat([]byte("x"), maxMessageFileBytes), 0o600); err != nil {
		t.Fatal(err)
	}
	body, err := resolveMessageFile(boundary, "body-file")
	if err != nil || len(body) != maxMessageFileBytes {
		t.Fatalf("ceiling-boundary file = (%d bytes, %v), want a full read", len(body), err)
	}
}

func TestResolveMessageFileRejectsNonUTF8Content(t *testing.T) {
	invalid := filepath.Join(t.TempDir(), "invalid.txt")
	if err := os.WriteFile(invalid, []byte{0x68, 0x61, 0x6C, 0x6C, 0x6F, 0xFF}, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := resolveMessageFile(invalid, "body-file")
	typed, ok := problem.As(err)
	if !ok {
		t.Fatalf("non-UTF8 error = %v, want a coded problem", err)
	}
	if typed.Code != problem.CodeInvalidInput || typed.Expected != "a plain UTF-8 text file" {
		t.Fatalf("non-UTF8 error = (%s, %q), want an invalid input for plain UTF-8 text", typed.Code, typed.Expected)
	}
}

func TestResolveMessageFileCarriesTheContentVerbatim(t *testing.T) {
	content := "## Motivation\r\n\r\nExpired tokens were accepted.\n\nLine with trailing spaces.   \n"
	message := filepath.Join(t.TempDir(), "body.txt")
	if err := os.WriteFile(message, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	body, err := resolveMessageFile(message, "body-file")
	if err != nil {
		t.Fatalf("verbatim read error = %v", err)
	}
	if body != content {
		t.Fatalf("verbatim read = %q, want the exact file content %q", body, content)
	}
	if strings.ContainsRune(body, 0xFFFD) {
		t.Fatalf("verbatim read replaced bytes: %q", body)
	}
}

func TestResolveMessageFileCarriesTheRequestedFieldLabel(t *testing.T) {
	_, err := resolveMessageFile("relative.txt", "merge-body-file")
	typed, ok := problem.As(err)
	if !ok {
		t.Fatalf("field label error = %v, want a coded problem", err)
	}
	if typed.Field != "merge-body-file" {
		t.Fatalf("field label = %q, want %q", typed.Field, "merge-body-file")
	}
}
