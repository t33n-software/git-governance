//go:build windows

package configfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

const fsctlRequestOplockLevel1 = 0x00090000

func TestWindowsConfigurationReplacementRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	backup := path + ".bak"
	writeTestConfiguration(t, backup, "recovered")
	if err := recoverConfiguration(path); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, path, "recovered")
	assertNotExists(t, backup)

	writeTestConfiguration(t, path, "current")
	writeTestConfiguration(t, backup, "stale")
	if err := recoverConfiguration(path); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, path, "current")
	assertNotExists(t, backup)

	temporaryPath := createTemporaryConfiguration(t, filepath.Dir(path), "replacement")
	if err := replaceConfiguration(path, temporaryPath); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, path, "replacement")
	assertNotExists(t, backup)
}

func TestWindowsRecoverConfigurationMissingPathsIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	for attempt := 0; attempt < 2; attempt++ {
		if err := recoverConfiguration(path); err != nil {
			t.Fatalf("recovery attempt %d: %v", attempt+1, err)
		}
	}

	assertNotExists(t, path)
	assertNotExists(t, path+".bak")
}

func TestWindowsRecoverConfigurationReportsInspectionErrors(t *testing.T) {
	t.Run("target path", func(t *testing.T) {
		err := recoverConfiguration(filepath.Join(t.TempDir(), "config\x00.json"))
		assertNonNotExistError(t, err)
	})

	t.Run("backup path", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		backup := path + ".bak"
		if err := os.Symlink(filepath.Base(backup), backup); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = os.Remove(backup)
		})

		err := recoverConfiguration(path)
		assertNonNotExistError(t, err)

		if _, err := os.Lstat(backup); err != nil {
			t.Fatalf("backup link disappeared after failed recovery: %v", err)
		}
	})
}

func TestWindowsRecoverConfigurationLockedBackupsAreRecoverable(t *testing.T) {
	t.Run("stale backup cleanup", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		backup := path + ".bak"
		writeTestConfiguration(t, path, "current")
		writeTestConfiguration(t, backup, "stale")

		handle := lockFileWithoutDeleteShare(t, backup)
		locked := true
		defer func() {
			if locked {
				_ = windows.CloseHandle(handle)
			}
		}()

		err := recoverConfiguration(path)
		assertNonNotExistError(t, err)
		assertFileContent(t, path, "current")
		assertFileContent(t, backup, "stale")

		closeWindowsHandle(t, handle)
		locked = false
		if err := recoverConfiguration(path); err != nil {
			t.Fatal(err)
		}
		assertFileContent(t, path, "current")
		assertNotExists(t, backup)
	})

	t.Run("backup restore", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		backup := path + ".bak"
		writeTestConfiguration(t, backup, "recovered")

		handle := lockFileWithoutDeleteShare(t, backup)
		locked := true
		defer func() {
			if locked {
				_ = windows.CloseHandle(handle)
			}
		}()

		err := recoverConfiguration(path)
		assertNonNotExistError(t, err)
		assertNotExists(t, path)
		assertFileContent(t, backup, "recovered")

		closeWindowsHandle(t, handle)
		locked = false
		if err := recoverConfiguration(path); err != nil {
			t.Fatal(err)
		}
		if err := recoverConfiguration(path); err != nil {
			t.Fatal(err)
		}
		assertFileContent(t, path, "recovered")
		assertNotExists(t, backup)
	})
}

func TestWindowsReplaceConfigurationPropagatesRecoveryFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	backup := path + ".bak"
	writeTestConfiguration(t, path, "current")
	writeTestConfiguration(t, backup, "stale")
	temporaryPath := createTemporaryConfiguration(t, filepath.Dir(path), "replacement")

	handle := lockFileWithoutDeleteShare(t, backup)
	locked := true
	defer func() {
		if locked {
			_ = windows.CloseHandle(handle)
		}
	}()

	err := replaceConfiguration(path, temporaryPath)
	assertNonNotExistError(t, err)
	assertFileContent(t, path, "current")
	assertFileContent(t, backup, "stale")
	assertFileContent(t, temporaryPath, "replacement")

	closeWindowsHandle(t, handle)
	locked = false
	if err := replaceConfiguration(path, temporaryPath); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, path, "replacement")
	assertNotExists(t, backup)
	assertNotExists(t, temporaryPath)
}

func TestWindowsReplaceConfigurationRecoversAfterRenameFailures(t *testing.T) {
	t.Run("target rename", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		writeTestConfiguration(t, path, "current")
		temporaryPath := createTemporaryConfiguration(t, filepath.Dir(path), "replacement")

		handle := lockFileWithoutDeleteShare(t, path)
		locked := true
		defer func() {
			if locked {
				_ = windows.CloseHandle(handle)
			}
		}()

		err := replaceConfiguration(path, temporaryPath)
		assertNonNotExistError(t, err)
		assertFileContent(t, path, "current")
		assertNotExists(t, path+".bak")
		assertFileContent(t, temporaryPath, "replacement")

		closeWindowsHandle(t, handle)
		locked = false
		if err := replaceConfiguration(path, temporaryPath); err != nil {
			t.Fatal(err)
		}
		assertFileContent(t, path, "replacement")
		assertNotExists(t, path+".bak")
	})

	t.Run("temporary rename with target", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		writeTestConfiguration(t, path, "current")
		temporaryPath := createTemporaryConfiguration(t, filepath.Dir(path), "replacement")

		handle := lockFileWithoutDeleteShare(t, temporaryPath)
		locked := true
		defer func() {
			if locked {
				_ = windows.CloseHandle(handle)
			}
		}()

		err := replaceConfiguration(path, temporaryPath)
		assertNonNotExistError(t, err)
		assertFileContent(t, path, "current")
		assertNotExists(t, path+".bak")
		assertFileContent(t, temporaryPath, "replacement")

		closeWindowsHandle(t, handle)
		locked = false
		if err := replaceConfiguration(path, temporaryPath); err != nil {
			t.Fatal(err)
		}
		assertFileContent(t, path, "replacement")
		assertNotExists(t, path+".bak")
	})

	t.Run("temporary rename without target", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		temporaryPath := createTemporaryConfiguration(t, filepath.Dir(path), "replacement")

		handle := lockFileWithoutDeleteShare(t, temporaryPath)
		locked := true
		defer func() {
			if locked {
				_ = windows.CloseHandle(handle)
			}
		}()

		err := replaceConfiguration(path, temporaryPath)
		assertNonNotExistError(t, err)
		assertNotExists(t, path)
		assertNotExists(t, path+".bak")
		assertFileContent(t, temporaryPath, "replacement")

		closeWindowsHandle(t, handle)
		locked = false
		if err := replaceConfiguration(path, temporaryPath); err != nil {
			t.Fatal(err)
		}
		assertFileContent(t, path, "replacement")
		assertNotExists(t, path+".bak")
	})
}

func TestWindowsReplaceConfigurationReportsPostRecoveryTargetStatError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	backup := path + ".bak"
	writeTestConfiguration(t, path, "current")
	writeTestConfiguration(t, backup, "stale")
	temporaryPath := createTemporaryConfiguration(t, filepath.Dir(path), "replacement")

	handle, event := requestLevel1Oplock(t, backup)
	waiterDone := make(chan struct{})
	defer func() {
		<-waiterDone
		_ = windows.CloseHandle(event)
	}()

	replaced := make(chan error, 1)
	go func() {
		var replacementErr error
		defer func() {
			if err := windows.CloseHandle(handle); err != nil && replacementErr == nil {
				replacementErr = fmt.Errorf("close oplock handle: %w", err)
			}
			replaced <- replacementErr
			close(waiterDone)
		}()

		status, err := windows.WaitForSingleObject(event, 5_000)
		if err != nil {
			replacementErr = err
			return
		}
		if status != windows.WAIT_OBJECT_0 {
			replacementErr = fmt.Errorf("oplock wait status = %#x", status)
			return
		}
		if err := os.Remove(path); err != nil {
			replacementErr = err
			return
		}
		if err := os.Symlink(filepath.Base(path), path); err != nil {
			replacementErr = err
			return
		}
	}()

	err := replaceConfiguration(path, temporaryPath)
	if backgroundErr := <-replaced; backgroundErr != nil {
		t.Fatalf("replace target during oplock break: %v", backgroundErr)
	}
	<-waiterDone
	assertNonNotExistError(t, err)
	assertFileContent(t, temporaryPath, "replacement")

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	assertNotExists(t, backup)
}

func writeTestConfiguration(t *testing.T, path, contents string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(contents), defaultFileMode); err != nil {
		t.Fatal(err)
	}
}

func createTemporaryConfiguration(t *testing.T, directory, contents string) string {
	t.Helper()

	temporary, err := os.CreateTemp(directory, ".config-*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	path := temporary.Name()
	t.Cleanup(func() {
		_ = os.Remove(path)
	})
	if _, err := temporary.WriteString(contents); err != nil {
		_ = temporary.Close()
		t.Fatal(err)
	}
	if err := temporary.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func lockFileWithoutDeleteShare(t *testing.T, path string) windows.Handle {
	t.Helper()

	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(
		pointer,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	return handle
}

func closeWindowsHandle(t *testing.T, handle windows.Handle) {
	t.Helper()

	if err := windows.CloseHandle(handle); err != nil {
		t.Fatal(err)
	}
}

func requestLevel1Oplock(t *testing.T, path string) (windows.Handle, windows.Handle) {
	t.Helper()

	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(
		pointer,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OVERLAPPED,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	event, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		_ = windows.CloseHandle(handle)
		t.Fatal(err)
	}
	overlapped := windows.Overlapped{HEvent: event}
	err = windows.DeviceIoControl(handle, fsctlRequestOplockLevel1, nil, 0, nil, 0, nil, &overlapped)
	if !errors.Is(err, windows.ERROR_IO_PENDING) {
		_ = windows.CloseHandle(event)
		_ = windows.CloseHandle(handle)
		t.Fatalf("request oplock = %v, want ERROR_IO_PENDING", err)
	}
	return handle, event
}

func assertFileContent(t *testing.T, path, expected string) {
	t.Helper()

	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != expected {
		t.Fatalf("content at %s = %q, want %q", path, actual, expected)
	}
}

func assertNotExists(t *testing.T, path string) {
	t.Helper()

	_, err := os.Lstat(path)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s still exists or could not be inspected: %v", path, err)
	}
}

func assertNonNotExistError(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("expected a filesystem error, got nil")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, want a non-not-exist filesystem error", err)
	}
}
