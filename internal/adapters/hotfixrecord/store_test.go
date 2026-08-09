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

	"github.com/CyberT33N/git-governance/internal/application/port"
	"github.com/CyberT33N/git-governance/internal/domain/ticket"
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
	info     os.FileInfo
	statErr  error
	data     string
	readErr  error
	statPath string
	readPath string
}

func (filesystem *testFilesystem) ReadFile(path string) ([]byte, error) {
	filesystem.readPath = path
	if filesystem.readErr != nil {
		return nil, filesystem.readErr
	}
	return []byte(filesystem.data), nil
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
