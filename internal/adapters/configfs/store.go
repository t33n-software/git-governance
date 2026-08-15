// Package configfs persists user-scoped git-governance preferences as a
// versioned JSON document under the operating system's configuration root.
package configfs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

const (
	applicationName      = "git-governance"
	configFileName       = "config.json"
	currentSchema        = 1
	defaultFileMode      = 0o600
	defaultDirectoryMode = 0o700
)

// ConfigDirectory resolves the OS-provided user configuration root. The
// production resolver uses os.UserConfigDir; tests inject deterministic roots.
type ConfigDirectory interface {
	UserConfigDir() (string, error)
}

type systemConfigDirectory struct{}

func (systemConfigDirectory) UserConfigDir() (string, error) {
	return os.UserConfigDir()
}

// configurationFilesystem isolates the complete durable-write boundary. The
// production implementation delegates to the OS and platform replacement
// helpers; the narrow interface keeps failure handling deterministic to test.
type configurationFilesystem interface {
	ReadFile(string) ([]byte, error)
	MkdirAll(string, os.FileMode) error
	CreateTemp(string, string) (configurationFile, error)
	Remove(string) error
	Recover(string) error
	Replace(string, string) error
}

type configurationFile interface {
	Name() string
	Chmod(os.FileMode) error
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

type systemConfigurationFilesystem struct{}

func (systemConfigurationFilesystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (systemConfigurationFilesystem) MkdirAll(path string, mode os.FileMode) error {
	return os.MkdirAll(path, mode)
}

func (systemConfigurationFilesystem) CreateTemp(directory, pattern string) (configurationFile, error) {
	return os.CreateTemp(directory, pattern)
}

func (systemConfigurationFilesystem) Remove(path string) error {
	return os.Remove(path)
}

func (systemConfigurationFilesystem) Recover(path string) error {
	return recoverConfiguration(path)
}

func (systemConfigurationFilesystem) Replace(path, temporaryPath string) error {
	return replaceConfiguration(path, temporaryPath)
}

type preferenceEncoder func(diskPreferences) ([]byte, error)

func encodePreferences(preferences diskPreferences) ([]byte, error) {
	return json.MarshalIndent(preferences, "", "  ")
}

// Options configures a preference store. Path overrides the OS default only
// when explicitly supplied by the caller.
type Options struct {
	Path      string
	Directory ConfigDirectory
}

// Store is a recoverable JSON implementation of port.PreferencesStore.
type Store struct {
	path       string
	directory  ConfigDirectory
	filesystem configurationFilesystem
	encode     preferenceEncoder
}

// New creates a preference store.
func New(options Options) *Store {
	directory := options.Directory
	if directory == nil {
		directory = systemConfigDirectory{}
	}
	return &Store{
		path:       options.Path,
		directory:  directory,
		filesystem: systemConfigurationFilesystem{},
		encode:     encodePreferences,
	}
}

// Load reads validated preferences. An absent configuration is equivalent to
// a new empty preferences document.
func (store *Store) Load(ctx context.Context) (port.Preferences, error) {
	if err := contextError(ctx); err != nil {
		return port.Preferences{}, err
	}
	path, err := store.Path()
	if err != nil {
		return port.Preferences{}, err
	}
	if err := store.filesystem.Recover(path); err != nil {
		return port.Preferences{}, unavailable(path, "recover an interrupted configuration replacement", err)
	}

	bytes, err := store.filesystem.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaultPreferences(), nil
	}
	if err != nil {
		return port.Preferences{}, unavailable(path, "read the configuration", err)
	}
	if err := contextError(ctx); err != nil {
		return port.Preferences{}, err
	}

	var disk diskPreferences
	if err := json.Unmarshal(bytes, &disk); err != nil {
		return port.Preferences{}, invalid(path, "configuration must contain valid JSON", err)
	}
	return fromDisk(path, disk)
}

// Save validates and atomically replaces user preferences.
func (store *Store) Save(ctx context.Context, preferences port.Preferences) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	path, err := store.Path()
	if err != nil {
		return err
	}
	if err := store.filesystem.Recover(path); err != nil {
		return unavailable(path, "recover an interrupted configuration replacement", err)
	}

	disk, err := toDisk(path, preferences)
	if err != nil {
		return err
	}
	encoded, err := store.encode(disk)
	if err != nil {
		return invalid(path, "configuration must be serializable", err)
	}
	encoded = append(encoded, '\n')

	if err := contextError(ctx); err != nil {
		return err
	}
	if err := store.filesystem.MkdirAll(filepath.Dir(path), defaultDirectoryMode); err != nil {
		return unavailable(path, "create the configuration directory", err)
	}
	return writeConfiguration(store.filesystem, path, encoded)
}

// Path returns the effective configuration file path.
func (store *Store) Path() (string, error) {
	if store.path != "" {
		return filepath.Clean(store.path), nil
	}
	root, err := store.directory.UserConfigDir()
	if err != nil {
		return "", unavailable("", "determine the user configuration directory", err)
	}
	if root == "" {
		return "", invalid("", "the user configuration directory must not be empty", nil)
	}
	return filepath.Join(root, applicationName, configFileName), nil
}

type diskPreferences struct {
	SchemaVersion int      `json:"schemaVersion"`
	KnownKeys     []string `json:"knownKeys"`
	DefaultKey    string   `json:"defaultKey,omitempty"`
	Accessible    bool     `json:"accessible"`
}

func defaultPreferences() port.Preferences {
	return port.Preferences{SchemaVersion: currentSchema}
}

func fromDisk(path string, disk diskPreferences) (port.Preferences, error) {
	if disk.SchemaVersion != currentSchema {
		return port.Preferences{}, invalid(
			path,
			fmt.Sprintf("schemaVersion must equal %d", currentSchema),
			nil,
		)
	}

	keys, err := parseAndDeduplicateKeys(path, disk.KnownKeys)
	if err != nil {
		return port.Preferences{}, err
	}

	preferences := port.Preferences{
		SchemaVersion: currentSchema,
		KnownKeys:     keys,
		Accessible:    disk.Accessible,
	}
	if disk.DefaultKey == "" {
		return preferences, nil
	}

	defaultKey, err := ticket.ParseKey(disk.DefaultKey)
	if err != nil {
		return port.Preferences{}, invalid(path, "defaultKey must be a valid ticket key", err)
	}
	if !containsKey(keys, defaultKey) {
		return port.Preferences{}, invalid(path, "defaultKey must also appear in knownKeys", nil)
	}
	preferences.DefaultKey = &defaultKey
	return preferences, nil
}

func toDisk(path string, preferences port.Preferences) (diskPreferences, error) {
	if preferences.SchemaVersion != 0 && preferences.SchemaVersion != currentSchema {
		return diskPreferences{}, invalid(
			path,
			fmt.Sprintf("schemaVersion must equal %d", currentSchema),
			nil,
		)
	}

	keys, err := normalizeKeys(path, preferences.KnownKeys)
	if err != nil {
		return diskPreferences{}, err
	}

	disk := diskPreferences{
		SchemaVersion: currentSchema,
		KnownKeys:     keyStrings(keys),
		Accessible:    preferences.Accessible,
	}
	if preferences.DefaultKey == nil {
		return disk, nil
	}

	defaultKey, err := ticket.ParseKey(preferences.DefaultKey.String())
	if err != nil {
		return diskPreferences{}, invalid(path, "defaultKey must be a valid ticket key", err)
	}
	if !containsKey(keys, defaultKey) {
		keys = append(keys, defaultKey)
		disk.KnownKeys = keyStrings(keys)
	}
	disk.DefaultKey = defaultKey.String()
	return disk, nil
}

func parseAndDeduplicateKeys(path string, values []string) ([]ticket.Key, error) {
	keys := make([]ticket.Key, 0, len(values))
	for _, value := range values {
		key, err := ticket.ParseKey(value)
		if err != nil {
			return nil, invalid(path, "knownKeys must contain valid ticket keys", err)
		}
		if !containsKey(keys, key) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func normalizeKeys(path string, values []ticket.Key) ([]ticket.Key, error) {
	keys := make([]ticket.Key, 0, len(values))
	for _, value := range values {
		key, err := ticket.ParseKey(value.String())
		if err != nil {
			return nil, invalid(path, "knownKeys must contain valid ticket keys", err)
		}
		if !containsKey(keys, key) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func containsKey(keys []ticket.Key, expected ticket.Key) bool {
	for _, key := range keys {
		if key.String() == expected.String() {
			return true
		}
	}
	return false
}

func keyStrings(keys []ticket.Key) []string {
	result := make([]string, len(keys))
	for index, key := range keys {
		result[index] = key.String()
	}
	return result
}

func writeConfiguration(filesystem configurationFilesystem, path string, contents []byte) (returnErr error) {
	directory := filepath.Dir(path)
	file, err := filesystem.CreateTemp(directory, ".config-*.tmp")
	if err != nil {
		return unavailable(path, "create a temporary configuration file", err)
	}
	temporaryPath := file.Name()
	defer func() {
		if returnErr != nil {
			_ = filesystem.Remove(temporaryPath)
		}
	}()

	if err := file.Chmod(defaultFileMode); err != nil {
		_ = file.Close()
		return unavailable(path, "set configuration file permissions", err)
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		return unavailable(path, "write the temporary configuration file", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return unavailable(path, "sync the temporary configuration file", err)
	}
	if err := file.Close(); err != nil {
		return unavailable(path, "close the temporary configuration file", err)
	}
	if err := filesystem.Replace(path, temporaryPath); err != nil {
		return unavailable(path, "replace the configuration file", err)
	}
	return nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return problem.Wrap(problem.Details{
			Code:        problem.CodeOperationCancelled,
			Category:    problem.CategoryCancelled,
			Field:       "operation",
			Expected:    "an active context",
			Rule:        "configuration operations stop when their context is cancelled",
			Remediation: "retry with an active context",
		}, err)
	}
	return nil
}

func unavailable(path, action string, cause error) error {
	return problem.Wrap(problem.Details{
		Code:        problem.CodeConfigurationUnavailable,
		Category:    problem.CategoryConfig,
		Field:       "configuration",
		Actual:      path,
		Expected:    "an accessible user configuration location",
		Rule:        action,
		Remediation: "check filesystem permissions and the configured path",
	}, cause)
}

func invalid(path, rule string, cause error) error {
	return problem.Wrap(problem.Details{
		Code:        problem.CodeConfigurationInvalid,
		Category:    problem.CategoryConfig,
		Field:       "configuration",
		Actual:      path,
		Expected:    "a valid git-governance configuration document",
		Rule:        rule,
		Example:     `{"schemaVersion":1,"knownKeys":["ABC"],"defaultKey":"ABC","accessible":false}`,
		Remediation: "correct the configuration or remove it to start with defaults",
	}, cause)
}

var _ port.PreferencesStore = (*Store)(nil)
