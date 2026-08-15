package configfs

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

var errUnexpectedCoverageFilesystemCall = errors.New("unexpected filesystem call")

type coverageFilesystem struct {
	readFile   func(string) ([]byte, error)
	mkdirAll   func(string, os.FileMode) error
	createTemp func(string, string) (configurationFile, error)
	remove     func(string) error
	recover    func(string) error
	replace    func(string, string) error
}

func (filesystem coverageFilesystem) ReadFile(path string) ([]byte, error) {
	if filesystem.readFile == nil {
		return nil, errUnexpectedCoverageFilesystemCall
	}
	return filesystem.readFile(path)
}

func (filesystem coverageFilesystem) MkdirAll(path string, mode os.FileMode) error {
	if filesystem.mkdirAll == nil {
		return errUnexpectedCoverageFilesystemCall
	}
	return filesystem.mkdirAll(path, mode)
}

func (filesystem coverageFilesystem) CreateTemp(directory, pattern string) (configurationFile, error) {
	if filesystem.createTemp == nil {
		return nil, errUnexpectedCoverageFilesystemCall
	}
	return filesystem.createTemp(directory, pattern)
}

func (filesystem coverageFilesystem) Remove(path string) error {
	if filesystem.remove == nil {
		return errUnexpectedCoverageFilesystemCall
	}
	return filesystem.remove(path)
}

func (filesystem coverageFilesystem) Recover(path string) error {
	if filesystem.recover == nil {
		return errUnexpectedCoverageFilesystemCall
	}
	return filesystem.recover(path)
}

func (filesystem coverageFilesystem) Replace(path, temporaryPath string) error {
	if filesystem.replace == nil {
		return errUnexpectedCoverageFilesystemCall
	}
	return filesystem.replace(path, temporaryPath)
}

type coverageFile struct {
	name       string
	contents   []byte
	chmodModes []os.FileMode
	closeCalls int
	chmodErr   error
	writeErr   error
	syncErr    error
	closeErr   error
}

func (file *coverageFile) Name() string {
	return file.name
}

func (file *coverageFile) Chmod(mode os.FileMode) error {
	file.chmodModes = append(file.chmodModes, mode)
	return file.chmodErr
}

func (file *coverageFile) Write(contents []byte) (int, error) {
	if file.writeErr != nil {
		return 0, file.writeErr
	}
	file.contents = append(file.contents, contents...)
	return len(contents), nil
}

func (file *coverageFile) Sync() error {
	return file.syncErr
}

func (file *coverageFile) Close() error {
	file.closeCalls++
	return file.closeErr
}

type stagedCancellationContext struct {
	remainingActiveChecks int
	errChecks             int
	done                  chan struct{}
}

func newStagedCancellationContext(remainingActiveChecks int) *stagedCancellationContext {
	return &stagedCancellationContext{
		remainingActiveChecks: remainingActiveChecks,
		done:                  make(chan struct{}),
	}
}

func (ctx *stagedCancellationContext) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (ctx *stagedCancellationContext) Done() <-chan struct{} {
	return ctx.done
}

func (ctx *stagedCancellationContext) Err() error {
	ctx.errChecks++
	if ctx.errChecks <= ctx.remainingActiveChecks {
		return nil
	}
	select {
	case <-ctx.done:
	default:
		close(ctx.done)
	}
	return context.Canceled
}

func (*stagedCancellationContext) Value(any) any {
	return nil
}

func newCoverageStore(path string, filesystem configurationFilesystem) *Store {
	store := New(Options{Path: path})
	store.filesystem = filesystem
	return store
}

func TestStoreCoverageSystemConfigDirectoryDelegates(t *testing.T) {
	expected, expectedErr := os.UserConfigDir()
	actual, err := (systemConfigDirectory{}).UserConfigDir()
	if (err != nil) != (expectedErr != nil) || actual != expected {
		t.Fatalf("UserConfigDir() = (%q, %v), want (%q, error present %t)", actual, err, expected, expectedErr != nil)
	}
}

func TestStoreCoverageContextErrorAcceptsNilContext(t *testing.T) {
	if err := contextError(testNilContext()); err != nil {
		t.Fatalf("contextError(nil) = %v, want nil", err)
	}
}

func TestStoreCoverageSystemFilesystemRemovesTemporaryFiles(t *testing.T) {
	temporaryPath := filepath.Join(t.TempDir(), "temporary.json")
	if err := os.WriteFile(temporaryPath, []byte("temporary"), defaultFileMode); err != nil {
		t.Fatal(err)
	}

	if err := (systemConfigurationFilesystem{}).Remove(temporaryPath); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := os.Lstat(temporaryPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary file remains after Remove(): %v", err)
	}
}

func TestStoreCoverageLoadStorageBoundaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), configFileName)

	t.Run("resolver failure", func(t *testing.T) {
		cause := errors.New("directory resolver failed")
		_, err := New(Options{Directory: fakeDirectory{err: cause}}).Load(context.Background())
		assertUnavailableWithCause(t, err, "determine the user configuration directory", cause)
	})

	t.Run("recovery failure", func(t *testing.T) {
		cause := errors.New("recovery failed")
		store := newCoverageStore(path, coverageFilesystem{
			recover: func(string) error { return cause },
		})

		_, err := store.Load(context.Background())
		assertUnavailableWithCause(t, err, "recover an interrupted configuration replacement", cause)
	})

	t.Run("read failure", func(t *testing.T) {
		cause := errors.New("read failed")
		store := newCoverageStore(path, coverageFilesystem{
			recover:  func(string) error { return nil },
			readFile: func(string) ([]byte, error) { return nil, cause },
		})

		_, err := store.Load(context.Background())
		assertUnavailableWithCause(t, err, "read the configuration", cause)
	})

	t.Run("missing configuration uses defaults", func(t *testing.T) {
		store := newCoverageStore(path, coverageFilesystem{
			recover:  func(string) error { return nil },
			readFile: func(string) ([]byte, error) { return nil, os.ErrNotExist },
		})

		preferences, err := store.Load(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if preferences.SchemaVersion != currentSchema || len(preferences.KnownKeys) != 0 || preferences.DefaultKey != nil || preferences.Accessible {
			t.Fatalf("Load() = %#v, want %#v", preferences, defaultPreferences())
		}
	})

	t.Run("corrupt document", func(t *testing.T) {
		store := newCoverageStore(path, coverageFilesystem{
			recover:  func(string) error { return nil },
			readFile: func(string) ([]byte, error) { return []byte("{"), nil },
		})

		_, err := store.Load(context.Background())
		assertConfigProblemRule(t, err, problem.CodeConfigurationInvalid, "configuration must contain valid JSON")
	})

	t.Run("cancellation after read", func(t *testing.T) {
		store := newCoverageStore(path, coverageFilesystem{
			recover: func(string) error { return nil },
			readFile: func(string) ([]byte, error) {
				return []byte(`{"schemaVersion":1,"knownKeys":[],"accessible":false}`), nil
			},
		})

		_, err := store.Load(newStagedCancellationContext(1))
		assertConfigProblemRule(t, err, problem.CodeOperationCancelled, "configuration operations stop when their context is cancelled")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Load() error = %v, want context cancellation", err)
		}
	})
}

func TestStoreCoverageSaveStorageBoundaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), configFileName)

	t.Run("resolver failure", func(t *testing.T) {
		cause := errors.New("directory resolver failed")
		err := New(Options{Directory: fakeDirectory{err: cause}}).Save(context.Background(), port.Preferences{})
		assertUnavailableWithCause(t, err, "determine the user configuration directory", cause)
	})

	t.Run("recovery failure", func(t *testing.T) {
		cause := errors.New("recovery failed")
		store := newCoverageStore(path, coverageFilesystem{
			recover: func(string) error { return cause },
		})

		err := store.Save(context.Background(), port.Preferences{})
		assertUnavailableWithCause(t, err, "recover an interrupted configuration replacement", cause)
	})

	t.Run("schema validation failure", func(t *testing.T) {
		store := newCoverageStore(path, coverageFilesystem{
			recover: func(string) error { return nil },
		})

		err := store.Save(context.Background(), port.Preferences{SchemaVersion: currentSchema + 1})
		assertConfigProblemRule(t, err, problem.CodeConfigurationInvalid, "schemaVersion must equal 1")
	})

	t.Run("serialization failure", func(t *testing.T) {
		cause := errors.New("encode failed")
		store := newCoverageStore(path, coverageFilesystem{
			recover: func(string) error { return nil },
		})
		store.encode = func(diskPreferences) ([]byte, error) {
			return nil, cause
		}

		err := store.Save(context.Background(), port.Preferences{})
		assertConfigProblemRule(t, err, problem.CodeConfigurationInvalid, "configuration must be serializable")
		if !errors.Is(err, cause) {
			t.Fatalf("Save() error = %v, want serialization cause", err)
		}
	})

	t.Run("cancellation after serialization", func(t *testing.T) {
		store := newCoverageStore(path, coverageFilesystem{
			recover: func(string) error { return nil },
		})

		err := store.Save(newStagedCancellationContext(1), port.Preferences{})
		assertConfigProblemRule(t, err, problem.CodeOperationCancelled, "configuration operations stop when their context is cancelled")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Save() error = %v, want context cancellation", err)
		}
	})

	t.Run("directory creation failure", func(t *testing.T) {
		cause := errors.New("mkdir failed")
		var actualPath string
		var actualMode os.FileMode
		store := newCoverageStore(path, coverageFilesystem{
			recover: func(string) error { return nil },
			mkdirAll: func(directory string, mode os.FileMode) error {
				actualPath = directory
				actualMode = mode
				return cause
			},
		})

		err := store.Save(context.Background(), port.Preferences{})
		assertUnavailableWithCause(t, err, "create the configuration directory", cause)
		if actualPath != filepath.Dir(path) || actualMode != defaultDirectoryMode {
			t.Fatalf("MkdirAll() = (%q, %o), want (%q, %o)", actualPath, actualMode, filepath.Dir(path), defaultDirectoryMode)
		}
	})
}

func TestStoreCoverageSchemaNormalizationAndSerialization(t *testing.T) {
	path := filepath.Join(t.TempDir(), configFileName)
	abc := mustKey(t, "ABC")
	platform := mustKey(t, "PLATFORM2")

	t.Run("deduplicates and restores defaults", func(t *testing.T) {
		preferences, err := fromDisk(path, diskPreferences{
			SchemaVersion: currentSchema,
			KnownKeys:     []string{abc.String(), abc.String(), platform.String()},
			DefaultKey:    platform.String(),
			Accessible:    true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := keyStrings(preferences.KnownKeys); len(got) != 2 || got[0] != abc.String() || got[1] != platform.String() {
			t.Fatalf("KnownKeys = %v, want [%s %s]", got, abc, platform)
		}
		if preferences.DefaultKey == nil || preferences.DefaultKey.String() != platform.String() || !preferences.Accessible {
			t.Fatalf("fromDisk() = %#v", preferences)
		}
	})

	t.Run("default key remains optional", func(t *testing.T) {
		preferences, err := fromDisk(path, diskPreferences{
			SchemaVersion: currentSchema,
			KnownKeys:     []string{abc.String()},
		})
		if err != nil {
			t.Fatal(err)
		}
		if preferences.DefaultKey != nil || len(preferences.KnownKeys) != 1 {
			t.Fatalf("fromDisk() = %#v, want no default key", preferences)
		}
	})

	t.Run("rejects invalid disk default key", func(t *testing.T) {
		_, err := fromDisk(path, diskPreferences{
			SchemaVersion: currentSchema,
			KnownKeys:     []string{abc.String()},
			DefaultKey:    "invalid",
		})
		assertConfigProblemRule(t, err, problem.CodeConfigurationInvalid, "defaultKey must be a valid ticket key")
	})

	t.Run("rejects invalid in-memory preferences", func(t *testing.T) {
		var zero ticket.Key
		testCases := []struct {
			name        string
			preferences port.Preferences
			rule        string
		}{
			{
				name:        "unsupported schema",
				preferences: port.Preferences{SchemaVersion: currentSchema + 1},
				rule:        "schemaVersion must equal 1",
			},
			{
				name:        "invalid known key",
				preferences: port.Preferences{KnownKeys: []ticket.Key{zero}},
				rule:        "knownKeys must contain valid ticket keys",
			},
			{
				name:        "invalid default key",
				preferences: port.Preferences{DefaultKey: &zero},
				rule:        "defaultKey must be a valid ticket key",
			},
		}

		for _, testCase := range testCases {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				_, err := toDisk(path, testCase.preferences)
				assertConfigProblemRule(t, err, problem.CodeConfigurationInvalid, testCase.rule)
			})
		}
	})

	t.Run("serializes and deserializes canonical preferences", func(t *testing.T) {
		disk, err := toDisk(path, port.Preferences{
			KnownKeys:  []ticket.Key{abc, platform, abc},
			DefaultKey: &platform,
			Accessible: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := encodePreferences(disk)
		if err != nil {
			t.Fatal(err)
		}
		var decoded diskPreferences
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		preferences, err := fromDisk(path, decoded)
		if err != nil {
			t.Fatal(err)
		}
		if got := keyStrings(preferences.KnownKeys); len(got) != 2 || got[0] != abc.String() || got[1] != platform.String() {
			t.Fatalf("round-trip keys = %v", got)
		}
		if preferences.DefaultKey == nil || preferences.DefaultKey.String() != platform.String() || !preferences.Accessible {
			t.Fatalf("round-trip preferences = %#v", preferences)
		}
	})
}

func TestStoreCoverageWritesAndCleansUpDeterministically(t *testing.T) {
	path := filepath.Join(t.TempDir(), configFileName)
	contents := []byte("configuration")
	testCases := []struct {
		name       string
		createErr  error
		chmodErr   error
		writeErr   error
		syncErr    error
		closeErr   error
		replaceErr error
		rule       string
	}{
		{name: "create temporary file", createErr: errors.New("create failed"), rule: "create a temporary configuration file"},
		{name: "set permissions", chmodErr: errors.New("chmod failed"), rule: "set configuration file permissions"},
		{name: "write temporary file", writeErr: errors.New("write failed"), rule: "write the temporary configuration file"},
		{name: "sync temporary file", syncErr: errors.New("sync failed"), rule: "sync the temporary configuration file"},
		{name: "close temporary file", closeErr: errors.New("close failed"), rule: "close the temporary configuration file"},
		{name: "replace configuration", replaceErr: errors.New("replace failed"), rule: "replace the configuration file"},
		{name: "successful replacement"},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			file := &coverageFile{
				name:     filepath.Join(filepath.Dir(path), ".config-coverage.tmp"),
				chmodErr: testCase.chmodErr,
				writeErr: testCase.writeErr,
				syncErr:  testCase.syncErr,
				closeErr: testCase.closeErr,
			}
			var removed []string
			replaceCalls := 0
			filesystem := coverageFilesystem{
				createTemp: func(string, string) (configurationFile, error) {
					if testCase.createErr != nil {
						return nil, testCase.createErr
					}
					return file, nil
				},
				remove: func(temporaryPath string) error {
					removed = append(removed, temporaryPath)
					return errors.New("cleanup failed")
				},
				replace: func(string, string) error {
					replaceCalls++
					return testCase.replaceErr
				},
			}

			err := writeConfiguration(filesystem, path, contents)
			wantFailure := testCase.rule != ""
			if wantFailure {
				cause := firstError(testCase.createErr, testCase.chmodErr, testCase.writeErr, testCase.syncErr, testCase.closeErr, testCase.replaceErr)
				assertUnavailableWithCause(t, err, testCase.rule, cause)
			} else if err != nil {
				t.Fatalf("writeConfiguration() error = %v", err)
			}

			wantCloseCalls := 1
			if testCase.createErr != nil {
				wantCloseCalls = 0
			}
			if file.closeCalls != wantCloseCalls {
				t.Fatalf("Close() calls = %d, want %d", file.closeCalls, wantCloseCalls)
			}

			if testCase.createErr == nil {
				if len(file.chmodModes) != 1 || file.chmodModes[0] != defaultFileMode {
					t.Fatalf("Chmod() modes = %v, want [%o]", file.chmodModes, defaultFileMode)
				}
			}

			wantReplaceCalls := 0
			if !wantFailure || testCase.replaceErr != nil {
				wantReplaceCalls = 1
			}
			if replaceCalls != wantReplaceCalls {
				t.Fatalf("Replace() calls = %d, want %d", replaceCalls, wantReplaceCalls)
			}

			wantRemoved := 0
			if wantFailure && testCase.createErr == nil {
				wantRemoved = 1
			}
			if len(removed) != wantRemoved {
				t.Fatalf("Remove() calls = %v, want %d cleanup calls", removed, wantRemoved)
			}
			if !wantFailure && string(file.contents) != string(contents) {
				t.Fatalf("written contents = %q, want %q", file.contents, contents)
			}
		})
	}
}

func TestStoreCoverageRecoversInterruptedWindowsReplacement(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows stores interrupted replacements as .bak files")
	}

	path := filepath.Join(t.TempDir(), configFileName)
	backup := path + ".bak"
	if err := os.WriteFile(backup, []byte(`{"schemaVersion":1,"knownKeys":["ABC"],"defaultKey":"ABC","accessible":true}`), defaultFileMode); err != nil {
		t.Fatal(err)
	}

	preferences, err := New(Options{Path: path}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if preferences.DefaultKey == nil || preferences.DefaultKey.String() != "ABC" || !preferences.Accessible {
		t.Fatalf("Load() after recovery = %#v", preferences)
	}
	if _, err := os.Lstat(backup); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup remains after recovery: %v", err)
	}
}

func assertConfigProblemRule(t *testing.T, err error, code problem.Code, rule string) {
	t.Helper()
	assertProblemCode(t, err, code)
	actual, ok := problem.As(err)
	if !ok || actual.Rule != rule {
		t.Fatalf("problem = %#v, want rule %q", actual, rule)
	}
}

func assertUnavailableWithCause(t *testing.T, err error, rule string, cause error) {
	t.Helper()
	assertConfigProblemRule(t, err, problem.CodeConfigurationUnavailable, rule)
	if !errors.Is(err, cause) {
		t.Fatalf("error = %v, want wrapped cause %v", err, cause)
	}
}

func firstError(errorsToCheck ...error) error {
	for _, err := range errorsToCheck {
		if err != nil {
			return err
		}
	}
	return nil
}

func testNilContext() context.Context {
	return nil
}
