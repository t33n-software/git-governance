//go:build !windows

package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// TestResolveMessageFileRejectsUnreadableFiles injects a permission denial:
// the file exists for stat but denies read access through a zero permission
// mode, so the read branch of the message transport fails closed with a
// coded problem while the stat path succeeds.
func TestResolveMessageFileRejectsUnreadableFiles(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("the effective user is root and ignores file permission bits; the read branch is exercised on non-root runners")
	}
	locked := filepath.Join(t.TempDir(), "locked.txt")
	if err := os.WriteFile(locked, []byte("locked content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}

	_, err := resolveMessageFile(locked, "body-file")
	typed, ok := problem.As(err)
	if !ok {
		t.Fatalf("locked file error = %v, want a coded problem", err)
	}
	if typed.Code != problem.CodeInvalidInput || typed.Expected != "a readable message file" {
		t.Fatalf("locked file error = (%s, %q), want an invalid input for a readable message file", typed.Code, typed.Expected)
	}
	if typed.Field != "body-file" {
		t.Fatalf("locked file field = %q, want %q", typed.Field, "body-file")
	}
}
