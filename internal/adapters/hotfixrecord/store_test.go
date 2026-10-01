package hotfixrecord

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

func TestDefaultLocation(t *testing.T) {
	if got, want := DefaultLocation(mustTicket(t, "GOV-42")), ".git-governance/hotfix-release-records/GOV-42.json"; got != want {
		t.Fatalf("DefaultLocation() = %q, want %q", got, want)
	}
}

func TestNewReadsBoundedRecordFromRepository(t *testing.T) {
	root := t.TempDir()
	location := DefaultLocation(mustTicket(t, "GOV-42"))
	path := filepath.Join(root, filepath.FromSlash(location))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(validRecordContents()), 0o600); err != nil {
		t.Fatal(err)
	}

	record, err := New().LoadHotfixReleaseRecord(
		context.Background(),
		port.RepositoryIdentity{Root: root},
		mustTicket(t, "GOV-42"),
		location,
	)
	if err != nil || record.Ticket().String() != "GOV-42" {
		t.Fatalf("New().LoadHotfixReleaseRecord() = (%#v, %v)", record, err)
	}
}

func TestStoreLoadsValidatedRecord(t *testing.T) {
	contents := validRecordContents()
	filesystem := &testFilesystem{
		info: testFileInfo{size: int64(len(contents))},
		data: contents,
	}
	store := &Store{filesystem: filesystem}
	root := t.TempDir()

	record, err := store.LoadHotfixReleaseRecord(
		context.Background(),
		port.RepositoryIdentity{Root: root},
		mustTicket(t, "GOV-42"),
		".git-governance/hotfix-release-records/GOV-42.json",
	)
	if err != nil {
		t.Fatal(err)
	}
	if record.Ticket().String() != "GOV-42" || record.TargetVersion().String() != "1.0.2" {
		t.Fatalf("LoadHotfixReleaseRecord() = %#v", record)
	}
	want := filepath.Join(root, filepath.FromSlash(".git-governance/hotfix-release-records/GOV-42.json"))
	if filesystem.statPath != want || filesystem.readPath != want {
		t.Fatalf("record paths = stat %q, read %q, want %q", filesystem.statPath, filesystem.readPath, want)
	}

	t.Run("uses the ticket-bound default location", func(t *testing.T) {
		filesystem := &testFilesystem{
			info: testFileInfo{size: int64(len(contents))},
			data: contents,
		}
		store := &Store{filesystem: filesystem}
		if _, err := store.LoadHotfixReleaseRecord(context.Background(), port.RepositoryIdentity{Root: root}, mustTicket(t, "GOV-42"), ""); err != nil {
			t.Fatal(err)
		}
		if filesystem.statPath != want {
			t.Fatalf("default record path = %q, want %q", filesystem.statPath, want)
		}
	})

	t.Run("rejects a record for another ticket", func(t *testing.T) {
		wrong := strings.Replace(contents, `"ticket":"GOV-42"`, `"ticket":"GOV-43"`, 1)
		wrong = strings.Replace(wrong, "hotfix/GOV-42-", "hotfix/GOV-43-", 1)
		store := &Store{filesystem: &testFilesystem{
			info: testFileInfo{size: int64(len(wrong))},
			data: wrong,
		}}
		if _, err := store.LoadHotfixReleaseRecord(context.Background(), port.RepositoryIdentity{Root: root}, mustTicket(t, "GOV-42"), ""); err == nil {
			t.Fatal("LoadHotfixReleaseRecord accepted a mismatched ticket")
		}
	})
}

func TestStoreRejectsUnavailableAndInvalidRecords(t *testing.T) {
	root := t.TempDir()
	location := ".git-governance/hotfix-release-records/GOV-42.json"
	contents := validRecordContents()

	t.Run("honors cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		store := &Store{filesystem: &testFilesystem{}}
		if _, err := store.LoadHotfixReleaseRecord(ctx, port.RepositoryIdentity{Root: root}, mustTicket(t, "GOV-42"), location); !errors.Is(err, context.Canceled) {
			t.Fatalf("LoadHotfixReleaseRecord() error = %v, want context cancellation", err)
		}
	})

	t.Run("rejects unavailable stores", func(t *testing.T) {
		var nilStore *Store
		if _, err := nilStore.LoadHotfixReleaseRecord(context.Background(), port.RepositoryIdentity{Root: root}, mustTicket(t, "GOV-42"), location); err == nil {
			t.Fatal("nil Store unexpectedly loaded a record")
		}
		if _, err := (&Store{}).LoadHotfixReleaseRecord(context.Background(), port.RepositoryIdentity{Root: root}, mustTicket(t, "GOV-42"), location); err == nil {
			t.Fatal("Store without a filesystem unexpectedly loaded a record")
		}
	})

	t.Run("rejects an invalid record location before filesystem access", func(t *testing.T) {
		store := &Store{filesystem: &testFilesystem{}}
		if _, err := store.LoadHotfixReleaseRecord(context.Background(), port.RepositoryIdentity{Root: root}, mustTicket(t, "GOV-42"), "../record.json"); err == nil {
			t.Fatal("LoadHotfixReleaseRecord unexpectedly accepted an escaping location")
		}
	})

	for _, testCase := range []struct {
		name       string
		filesystem *testFilesystem
	}{
		{
			name: "stat error",
			filesystem: &testFilesystem{
				statErr: errors.New("missing"),
			},
		},
		{
			name: "directory",
			filesystem: &testFilesystem{
				info: testFileInfo{directory: true},
			},
		},
		{
			name: "oversized stat",
			filesystem: &testFilesystem{
				info: testFileInfo{size: maxRecordBytes + 1},
			},
		},
		{
			name: "read error",
			filesystem: &testFilesystem{
				info:    testFileInfo{size: int64(len(contents))},
				readErr: errors.New("read failure"),
			},
		},
		{
			name: "oversized data after stat",
			filesystem: &testFilesystem{
				info: testFileInfo{size: 1},
				data: strings.Repeat("x", maxRecordBytes+1),
			},
		},
		{
			name: "invalid json",
			filesystem: &testFilesystem{
				info: testFileInfo{size: 1},
				data: "{",
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			store := &Store{filesystem: testCase.filesystem}
			if _, err := store.LoadHotfixReleaseRecord(context.Background(), port.RepositoryIdentity{Root: root}, mustTicket(t, "GOV-42"), location); err == nil {
				t.Fatal("LoadHotfixReleaseRecord unexpectedly succeeded")
			}
		})
	}
}

func TestResolveLocationRestrictsRecordDirectory(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, filepath.FromSlash(".git-governance/hotfix-release-records/GOV-42.json"))
	path, err := resolveLocation(root, ".git-governance/hotfix-release-records/GOV-42.json")
	if err != nil || path != want {
		t.Fatalf("resolveLocation() = (%q, %v), want (%q, nil)", path, err, want)
	}

	for _, testCase := range []struct {
		name     string
		root     string
		location string
	}{
		{name: "empty root", root: "", location: ".git-governance/hotfix-release-records/GOV-42.json"},
		{name: "relative root", root: "repository", location: ".git-governance/hotfix-release-records/GOV-42.json"},
		{name: "empty location", root: root},
		{name: "whitespace location", root: root, location: " .git-governance/hotfix-release-records/GOV-42.json"},
		{name: "absolute location", root: root, location: filepath.Join(root, "record.json")},
		{name: "record directory", root: root, location: recordDirectory},
		{name: "parent traversal", root: root, location: "../GOV-42.json"},
		{name: "nested record", root: root, location: ".git-governance/hotfix-release-records/nested/GOV-42.json"},
		{name: "wrong extension", root: root, location: ".git-governance/hotfix-release-records/GOV-42.txt"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := resolveLocation(testCase.root, testCase.location); err == nil {
				t.Fatal("resolveLocation unexpectedly succeeded")
			}
		})
	}
}

type testFilesystem struct {
	info       os.FileInfo
	statErr    error
	data       string
	readErr    error
	statPath   string
	readPath   string
	readDirErr error
	entries    []os.DirEntry
}

func (filesystem *testFilesystem) ReadFile(path string) ([]byte, error) {
	filesystem.readPath = path
	if filesystem.readErr != nil {
		return nil, filesystem.readErr
	}
	return []byte(filesystem.data), nil
}

func (filesystem *testFilesystem) ReadDir(path string) ([]os.DirEntry, error) {
	if filesystem.readDirErr != nil {
		return nil, filesystem.readDirErr
	}
	if filesystem.entries != nil {
		return filesystem.entries, nil
	}
	return nil, os.ErrNotExist
}

func (filesystem *testFilesystem) Stat(path string) (os.FileInfo, error) {
	filesystem.statPath = path
	if filesystem.statErr != nil {
		return nil, filesystem.statErr
	}
	return filesystem.info, nil
}

type testFileInfo struct {
	size      int64
	directory bool
}

func (info testFileInfo) Name() string       { return "record.json" }
func (info testFileInfo) Size() int64        { return info.size }
func (info testFileInfo) Mode() os.FileMode  { return 0o600 }
func (info testFileInfo) ModTime() time.Time { return time.Time{} }
func (info testFileInfo) IsDir() bool        { return info.directory }
func (info testFileInfo) Sys() any           { return nil }

// testDirEntry is the configurable directory-entry fake for listing tests:
// it names one controlled record-directory entry and carries either a
// resolvable file info or an info read failure.
type testDirEntry struct {
	name    string
	dir     bool
	info    os.FileInfo
	infoErr error
}

func (entry *testDirEntry) Name() string               { return entry.name }
func (entry *testDirEntry) IsDir() bool                { return entry.dir }
func (entry *testDirEntry) Type() os.FileMode          { return 0o600 }
func (entry *testDirEntry) Info() (os.FileInfo, error) { return entry.info, entry.infoErr }

func validRecordContents() string {
	return fmt.Sprintf(
		`{"schemaVersion":1,"ticket":"GOV-42","incident":"INC-42","affectedLine":"main","targetVersion":"1.0.2","previousTag":"v1.0.1","expectedPullRequest":{"source":"hotfix/GOV-42-main-hotfix-patch-delivery","target":"main"},"manifest":["%s"],"commitBudgetException":"","propagationTargets":["develop"]}`,
		strings.Repeat("a", 40),
	)
}

func mustTicket(t *testing.T, raw string) ticket.ID {
	t.Helper()

	value, err := ticket.ParseID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestListHotfixReleaseRecords(t *testing.T) {
	t.Parallel()

	t.Run("lists every reviewed record below the controlled directory", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		directory := filepath.Join(root, filepath.FromSlash(recordDirectory))
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "GOV-42.json"), []byte(validRecordContents()), 0o600); err != nil {
			t.Fatal(err)
		}
		other := strings.Replace(validRecordContents(), `"ticket":"GOV-42"`, `"ticket":"GOV-43"`, 1)
		other = strings.Replace(other, "hotfix/GOV-42-", "hotfix/GOV-43-", 1)
		if err := os.WriteFile(filepath.Join(directory, "GOV-43.json"), []byte(other), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "notes.txt"), []byte("not a record"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(directory, "nested"), 0o700); err != nil {
			t.Fatal(err)
		}

		records, err := New().ListHotfixReleaseRecords(context.Background(), port.RepositoryIdentity{Root: root})
		if err != nil {
			t.Fatal(err)
		}
		if len(records) != 2 {
			t.Fatalf("ListHotfixReleaseRecords() = %#v", records)
		}
		if records[0].Record.Ticket().String() != "GOV-42" ||
			records[0].Location != ".git-governance/hotfix-release-records/GOV-42.json" {
			t.Fatalf("first record = %#v", records[0])
		}
		if records[1].Record.Ticket().String() != "GOV-43" {
			t.Fatalf("second record = %#v", records[1])
		}
	})

	t.Run("a repository without the record directory has no records", func(t *testing.T) {
		t.Parallel()
		records, err := New().ListHotfixReleaseRecords(context.Background(), port.RepositoryIdentity{Root: t.TempDir()})
		if err != nil || records != nil {
			t.Fatalf("ListHotfixReleaseRecords() = (%#v, %v)", records, err)
		}
	})

	t.Run("honors cancellation", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := New().ListHotfixReleaseRecords(ctx, port.RepositoryIdentity{Root: t.TempDir()}); !errors.Is(err, context.Canceled) {
			t.Fatalf("ListHotfixReleaseRecords() error = %v", err)
		}
	})

	t.Run("rejects unavailable stores and roots", func(t *testing.T) {
		t.Parallel()
		var nilStore *Store
		if _, err := nilStore.ListHotfixReleaseRecords(context.Background(), port.RepositoryIdentity{Root: t.TempDir()}); err == nil {
			t.Fatal("nil Store unexpectedly listed records")
		}
		if _, err := (&Store{}).ListHotfixReleaseRecords(context.Background(), port.RepositoryIdentity{Root: t.TempDir()}); err == nil {
			t.Fatal("Store without a filesystem unexpectedly listed records")
		}
		if _, err := New().ListHotfixReleaseRecords(context.Background(), port.RepositoryIdentity{}); err == nil {
			t.Fatal("empty repository root unexpectedly listed records")
		}
	})

	t.Run("a directory read failure fails the listing closed", func(t *testing.T) {
		t.Parallel()
		store := &Store{filesystem: &testFilesystem{readDirErr: errors.New("unreadable directory")}}
		if _, err := store.ListHotfixReleaseRecords(context.Background(), port.RepositoryIdentity{Root: t.TempDir()}); err == nil {
			t.Fatal("ListHotfixReleaseRecords unexpectedly succeeded")
		}
	})

	t.Run("a corrupt record fails the listing closed", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		directory := filepath.Join(root, filepath.FromSlash(recordDirectory))
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "GOV-42.json"), []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := New().ListHotfixReleaseRecords(context.Background(), port.RepositoryIdentity{Root: root}); err == nil {
			t.Fatal("ListHotfixReleaseRecords accepted a corrupt record")
		}
	})

	t.Run("an oversized record fails the listing closed", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		directory := filepath.Join(root, filepath.FromSlash(recordDirectory))
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "GOV-42.json"), []byte(strings.Repeat("x", maxRecordBytes+1)), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := New().ListHotfixReleaseRecords(context.Background(), port.RepositoryIdentity{Root: root}); err == nil {
			t.Fatal("ListHotfixReleaseRecords accepted an oversized record")
		}
	})

	t.Run("a record whose info cannot be read fails the listing closed", func(t *testing.T) {
		t.Parallel()
		store := &Store{filesystem: &testFilesystem{entries: []os.DirEntry{
			&testDirEntry{name: "GOV-42.json", infoErr: errors.New("unreadable info")},
		}}}
		if _, err := store.ListHotfixReleaseRecords(context.Background(), port.RepositoryIdentity{Root: t.TempDir()}); err == nil {
			t.Fatal("ListHotfixReleaseRecords unexpectedly succeeded")
		}
	})

	t.Run("a record that cannot be read fails the listing closed", func(t *testing.T) {
		t.Parallel()
		store := &Store{filesystem: &testFilesystem{
			entries: []os.DirEntry{&testDirEntry{name: "GOV-42.json", info: testFileInfo{size: 1}}},
			readErr: errors.New("unreadable record"),
		}}
		if _, err := store.ListHotfixReleaseRecords(context.Background(), port.RepositoryIdentity{Root: t.TempDir()}); err == nil {
			t.Fatal("ListHotfixReleaseRecords unexpectedly succeeded")
		}
	})

	t.Run("a record larger than its reported size fails the listing closed", func(t *testing.T) {
		t.Parallel()
		store := &Store{filesystem: &testFilesystem{
			entries: []os.DirEntry{&testDirEntry{name: "GOV-42.json", info: testFileInfo{size: 1}}},
			data:    strings.Repeat("x", maxRecordBytes+1),
		}}
		if _, err := store.ListHotfixReleaseRecords(context.Background(), port.RepositoryIdentity{Root: t.TempDir()}); err == nil {
			t.Fatal("ListHotfixReleaseRecords unexpectedly succeeded")
		}
	})
}
