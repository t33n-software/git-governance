package configfs

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

type fakeDirectory struct {
	root string
	err  error
}

func (directory fakeDirectory) UserConfigDir() (string, error) {
	return directory.root, directory.err
}

func TestPathUsesConfiguredRoots(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		root string
	}{
		{name: "Windows AppData root", root: `C:\Users\developer\AppData\Roaming`},
		{name: "Darwin Application Support root", root: `/Users/developer/Library/Application Support`},
		{name: "XDG config root", root: `/home/developer/.config`},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			store := New(Options{Directory: fakeDirectory{root: testCase.root}})
			actual, err := store.Path()
			if err != nil {
				t.Fatal(err)
			}
			expected := filepath.Join(testCase.root, applicationName, configFileName)
			if actual != expected {
				t.Fatalf("Path() = %q, want %q", actual, expected)
			}
		})
	}
}

func TestPathErrorCases(t *testing.T) {
	t.Parallel()

	t.Run("explicit override", func(t *testing.T) {
		store := New(Options{Path: `.\custom\config.json`})
		actual, err := store.Path()
		if err != nil {
			t.Fatal(err)
		}
		if actual != filepath.Clean(`.\custom\config.json`) {
			t.Fatalf("Path() = %q", actual)
		}
	})

	t.Run("missing home or config root", func(t *testing.T) {
		store := New(Options{Directory: fakeDirectory{root: ""}})
		_, err := store.Path()
		assertProblemCode(t, err, problem.CodeConfigurationInvalid)
	})

	t.Run("relative XDG configuration is rejected by resolver", func(t *testing.T) {
		store := New(Options{Directory: fakeDirectory{err: errors.New("XDG_CONFIG_HOME is relative")}})
		_, err := store.Path()
		assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
	})
}

func TestLoadMissingConfigurationReturnsDefaults(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), configFileName)
	store := New(Options{Path: path})
	actual, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if actual.SchemaVersion != currentSchema || len(actual.KnownKeys) != 0 || actual.DefaultKey != nil || actual.Accessible {
		t.Fatalf("Load() = %#v", actual)
	}
}

func TestSaveAndLoadRoundTripNormalizesKeys(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", configFileName)
	store := New(Options{Path: path})
	abc := mustKey(t, "ABC")
	platform := mustKey(t, "PLATFORM2")

	err := store.Save(context.Background(), port.Preferences{
		KnownKeys:  []ticket.Key{abc, platform, abc},
		DefaultKey: &platform,
		Accessible: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	actual, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if actual.SchemaVersion != currentSchema || !actual.Accessible {
		t.Fatalf("Load() = %#v", actual)
	}
	if got := keyStrings(actual.KnownKeys); strings.Join(got, ",") != "ABC,PLATFORM2" {
		t.Fatalf("KnownKeys = %v", got)
	}
	if actual.DefaultKey == nil || actual.DefaultKey.String() != "PLATFORM2" {
		t.Fatalf("DefaultKey = %#v", actual.DefaultKey)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(contents), "\n") || !strings.Contains(string(contents), `"schemaVersion": 1`) {
		t.Fatalf("config contents = %q", contents)
	}
	temporaryFiles, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".config-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temporaryFiles) != 0 {
		t.Fatalf("temporary files remain: %v", temporaryFiles)
	}
}

func TestSaveAddsDefaultKeyToKnownKeys(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), configFileName)
	store := New(Options{Path: path})
	defaultKey := mustKey(t, "ABC")
	if err := store.Save(context.Background(), port.Preferences{DefaultKey: &defaultKey}); err != nil {
		t.Fatal(err)
	}

	actual, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := keyStrings(actual.KnownKeys); strings.Join(got, ",") != "ABC" {
		t.Fatalf("KnownKeys = %v", got)
	}
}

func TestLoadRejectsCorruptAndInvalidConfiguration(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		contents string
	}{
		{name: "malformed JSON", contents: `{`},
		{name: "invalid key", contents: `{"schemaVersion":1,"knownKeys":["abc"],"accessible":false}`},
		{name: "unsupported schema", contents: `{"schemaVersion":2,"knownKeys":[],"accessible":false}`},
		{name: "default outside known keys", contents: `{"schemaVersion":1,"knownKeys":["ABC"],"defaultKey":"PLATFORM2","accessible":false}`},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), configFileName)
			if err := os.WriteFile(path, []byte(testCase.contents), defaultFileMode); err != nil {
				t.Fatal(err)
			}
			_, err := New(Options{Path: path}).Load(context.Background())
			assertProblemCode(t, err, problem.CodeConfigurationInvalid)
		})
	}
}

func TestSaveRejectsUnavailableParentAndCancelledContext(t *testing.T) {
	t.Parallel()

	t.Run("parent is a file", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "not-a-directory")
		if err := os.WriteFile(parent, []byte("blocker"), defaultFileMode); err != nil {
			t.Fatal(err)
		}
		store := New(Options{Path: filepath.Join(parent, configFileName)})
		err := store.Save(context.Background(), port.Preferences{})
		assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		store := New(Options{Path: filepath.Join(t.TempDir(), configFileName)})
		_, err := store.Load(ctx)
		assertProblemCode(t, err, problem.CodeOperationCancelled)
	})
}

func TestSaveHonorsCancelledContextWithoutMutation(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), configFileName)
	if err := os.WriteFile(path, []byte("current"), defaultFileMode); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := New(Options{Path: path}).Save(ctx, port.Preferences{})
	assertProblemCode(t, err, problem.CodeOperationCancelled)

	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != "current" {
		t.Fatalf("configuration after cancelled save = %q, want %q", actual, "current")
	}
	if _, err := os.Lstat(path + ".bak"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup after cancelled save = %v, want not exist", err)
	}
	temporaryFiles, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".config-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temporaryFiles) != 0 {
		t.Fatalf("temporary files remain after cancelled save: %v", temporaryFiles)
	}
}

func TestInternalNormalizationAndConfigurationWrite(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), configFileName)
	abc := mustKey(t, "ABC")
	platform := mustKey(t, "PLATFORM2")

	disk, err := toDisk(path, port.Preferences{
		SchemaVersion: currentSchema,
		KnownKeys:     []ticket.Key{abc, platform, abc},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(disk.KnownKeys, ","); got != "ABC,PLATFORM2" {
		t.Fatalf("toDisk keys = %q", got)
	}

	if err := os.MkdirAll(filepath.Dir(path), defaultDirectoryMode); err != nil {
		t.Fatal(err)
	}
	filesystem := systemConfigurationFilesystem{}
	if err := writeConfiguration(filesystem, path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := writeConfiguration(filesystem, path, []byte("second")); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != "second" {
		t.Fatalf("writeConfiguration content = %q", actual)
	}
}

func mustKey(t *testing.T, raw string) ticket.Key {
	t.Helper()
	actual, err := ticket.ParseKey(raw)
	if err != nil {
		t.Fatal(err)
	}
	return actual
}

func assertProblemCode(t *testing.T, err error, expected problem.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected problem code %q, got nil", expected)
	}
	actual, ok := problem.As(err)
	if !ok {
		t.Fatalf("error %T does not carry a problem: %v", err, err)
	}
	if actual.Code != expected {
		t.Fatalf("problem code = %q, want %q", actual.Code, expected)
	}
}

func FuzzDecodePreferences(f *testing.F) {
	for _, seed := range []string{
		`{"schemaVersion":1,"knownKeys":["ABC"],"defaultKey":"ABC","accessible":false}`,
		`{"schemaVersion":1,"knownKeys":[],"accessible":true}`,
		`{`,
		"",
		"\x00",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		var disk diskPreferences
		if err := json.Unmarshal([]byte(raw), &disk); err != nil {
			return
		}
		_, _ = fromDisk("fuzz-config.json", disk)
	})
}
