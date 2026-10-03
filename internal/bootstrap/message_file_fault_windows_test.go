//go:build windows

package bootstrap

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"unsafe"

	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// TestResolveMessageFileRejectsUnreadableFiles injects a Windows sharing
// violation: the file exists for stat but denies read access through an
// exclusive share-mode lock, so the read branch of the message transport
// fails closed with a coded problem while the stat path succeeds.
func TestResolveMessageFileRejectsUnreadableFiles(t *testing.T) {
	locked := filepath.Join(t.TempDir(), "locked.txt")
	if err := os.WriteFile(locked, []byte("locked content"), 0o600); err != nil {
		t.Fatal(err)
	}
	pathPtr, err := syscall.UTF16PtrFromString(locked)
	if err != nil {
		t.Fatal(err)
	}
	createFile := syscall.NewLazyDLL("kernel32.dll").NewProc("CreateFileW")
	handle, _, callErr := createFile.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(0x80000000), // GENERIC_READ
		uintptr(0),          // exclusive share mode: no other read may open the file
		uintptr(0),          // default security descriptor
		uintptr(3),          // OPEN_EXISTING
		uintptr(0x80),       // FILE_ATTRIBUTE_NORMAL
		uintptr(0),          // no template file
	)
	if handle == uintptr(syscall.InvalidHandle) {
		t.Fatalf("lock injection failed: %v", callErr)
	}
	defer syscall.CloseHandle(syscall.Handle(handle))

	_, err = resolveMessageFile(locked, "body-file")
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
