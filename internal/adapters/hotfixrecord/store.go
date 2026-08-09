// Package hotfixrecord loads reviewed hotfix release records from the
// repository-controlled record directory.
package hotfixrecord

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/CyberT33N/git-governance/internal/application/port"
	"github.com/CyberT33N/git-governance/internal/domain/hotfix"
	"github.com/CyberT33N/git-governance/internal/domain/problem"
	"github.com/CyberT33N/git-governance/internal/domain/ticket"
)

const (
	recordDirectory = ".git-governance/hotfix-release-records"
	maxRecordBytes  = 64 * 1024
)

type filesystem interface {
	ReadFile(string) ([]byte, error)
	Stat(string) (os.FileInfo, error)
}

type systemFilesystem struct{}

func (systemFilesystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (systemFilesystem) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

// Store reads bounded JSON records from the controlled repository directory.
type Store struct {
	filesystem filesystem
}

// New creates a record store.
func New() *Store {
	return &Store{filesystem: systemFilesystem{}}
}

// DefaultLocation returns the repository-relative record location for one
// ticket. The target version remains inside the reviewed JSON record so the
// controller has exactly one record lookup key per hotfix ticket.
func DefaultLocation(id ticket.ID) string {
	return filepath.ToSlash(filepath.Join(recordDirectory, id.String()+".json"))
}

// LoadHotfixReleaseRecord loads and validates a record without allowing an
// absolute, parent-traversing, or arbitrary repository path.
func (store *Store) LoadHotfixReleaseRecord(
	ctx context.Context,
	repository port.RepositoryIdentity,
	id ticket.ID,
	location string,
) (hotfix.ReleaseRecord, error) {
	if err := ctx.Err(); err != nil {
		return hotfix.ReleaseRecord{}, err
	}
	if store == nil || store.filesystem == nil {
		return hotfix.ReleaseRecord{}, unavailableRecordProblem("hotfix release record store")
	}
	if location == "" {
		location = DefaultLocation(id)
	}
	path, err := resolveLocation(repository.Root, location)
	if err != nil {
		return hotfix.ReleaseRecord{}, err
	}
	info, err := store.filesystem.Stat(path)
	if err != nil {
		return hotfix.ReleaseRecord{}, unavailableRecordProblem("reviewed hotfix release record")
	}
	if info.IsDir() || info.Size() > maxRecordBytes {
		return hotfix.ReleaseRecord{}, invalidRecordLocation(
			"a JSON release record no larger than 65536 bytes",
			"store one bounded JSON release record for the hotfix ticket",
		)
	}
	contents, err := store.filesystem.ReadFile(path)
	if err != nil {
		return hotfix.ReleaseRecord{}, unavailableRecordProblem("reviewed hotfix release record")
	}
	if len(contents) > maxRecordBytes {
		return hotfix.ReleaseRecord{}, invalidRecordLocation(
			"a JSON release record no larger than 65536 bytes",
			"reduce the record to its bounded governance fields",
		)
	}
	record, err := hotfix.ParseRecord(contents)
	if err != nil {
		return hotfix.ReleaseRecord{}, err
	}
	if record.Ticket().String() != id.String() {
		return hotfix.ReleaseRecord{}, invalidRecordLocation(
			"a release record whose ticket matches the hotfix branch",
			"store the release record under and inside the ticket-bound record path",
		)
	}
	return record, nil
}

func resolveLocation(root, location string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", unavailableRecordProblem("repository root")
	}
	if location == "" || strings.TrimSpace(location) != location {
		return "", invalidRecordLocation(
			"a relative JSON path below .git-governance/hotfix-release-records",
			"use the canonical ticket record location without whitespace",
		)
	}
	clean := filepath.Clean(filepath.FromSlash(location))
	if filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" {
		return "", invalidRecordLocation(
			"a repository-relative hotfix release record path",
			"do not supply an absolute record path",
		)
	}
	relative, err := filepath.Rel(recordDirectory, clean)
	if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || relative == ".." ||
		filepath.Dir(relative) != "." || filepath.Ext(relative) != ".json" {
		return "", invalidRecordLocation(
			"a direct JSON file below .git-governance/hotfix-release-records",
			"use the canonical .git-governance/hotfix-release-records/<KEY-NUMBER>.json path",
		)
	}
	if !filepath.IsAbs(root) {
		return "", unavailableRecordProblem("repository root")
	}
	rootPath := filepath.Clean(root)
	path := filepath.Join(rootPath, clean)
	return path, nil
}

func unavailableRecordProblem(field string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeConfigurationUnavailable,
		Category:    problem.CategoryConfig,
		Field:       field,
		Expected:    "an available repository-local hotfix release record",
		Rule:        "production hotfix delivery reads only a reviewed repository-local record",
		Remediation: "restore the controlled release record and retry from the repository root",
	})
}

func invalidRecordLocation(expected, remediation string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeInvalidInput,
		Category:    problem.CategoryGovernance,
		Field:       "hotfix release record",
		Expected:    expected,
		Rule:        "hotfix release records stay in the controlled repository record directory",
		Remediation: remediation,
	})
}

var _ port.HotfixReleaseRecordStore = (*Store)(nil)
