//go:build windows

package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	sessionStoreSchemaVersion = 1
	dpapiEntropy              = "git-governance/github-app-session-store/v1"
)

type sessionTemporaryFile interface {
	Name() string
	Chmod(os.FileMode) error
	Write([]byte) (int, error)
	Close() error
}

var (
	userConfigDirectory = os.UserConfigDir
	readSessionFile     = os.ReadFile
	removeSessionFile   = os.Remove
	makeSessionDir      = os.MkdirAll
	createSessionTemp   = func(directory, pattern string) (sessionTemporaryFile, error) {
		return os.CreateTemp(directory, pattern)
	}
	renameSessionFile  = os.Rename
	cryptProtectData   = windows.CryptProtectData
	cryptUnprotectData = windows.CryptUnprotectData
	freeLocalMemory    = windows.LocalFree
)

type sessionDocument struct {
	SchemaVersion int `json:"schemaVersion"`
	// ActiveScopeByHost maps a normalized host to the scope key of the
	// currently active session. It lets the store resolve the active session
	// without an ambient client-ID value.
	ActiveScopeByHost map[string]string  `json:"activeScopeByHost,omitempty"`
	ActiveByScope     map[string]string  `json:"activeByScope,omitempty"`
	Sessions          map[string]Session `json:"sessions"`
}

type dpapiSessionStore struct {
	path func() (string, error)
}

func newPlatformSessionStore() SessionStore {
	return &dpapiSessionStore{path: defaultDPAPISessionStorePath}
}

func newDPAPISessionStore(path string) *dpapiSessionStore {
	return &dpapiSessionStore{
		path: func() (string, error) {
			return path, nil
		},
	}
}

func (store *dpapiSessionStore) LoadActive(ctx context.Context, host, clientID string) (Session, error) {
	if err := contextFailure(ctx); err != nil {
		return Session{}, err
	}
	document, err := store.load()
	if err != nil {
		return Session{}, err
	}
	return loadScopedSession(document, host, clientID)
}

func (store *dpapiSessionStore) LoadActiveForHost(ctx context.Context, host string) (Session, error) {
	if err := contextFailure(ctx); err != nil {
		return Session{}, err
	}
	document, err := store.load()
	if err != nil {
		return Session{}, err
	}
	scope, found := document.ActiveScopeByHost[normalizeHost(host)]
	if !found || strings.TrimSpace(scope) == "" {
		return Session{}, errSessionNotFound
	}
	clientID, found := strings.CutPrefix(scope, normalizeHost(host)+"\x00")
	if !found || strings.TrimSpace(clientID) == "" {
		return Session{}, errors.New("protected GitHub App session host pointer is inconsistent")
	}
	return loadScopedSession(document, host, clientID)
}

func loadScopedSession(document sessionDocument, host, clientID string) (Session, error) {
	scope := sessionScopeKey(host, clientID)
	account, found := document.ActiveByScope[scope]
	if !found || strings.TrimSpace(account) == "" {
		return Session{}, errSessionNotFound
	}
	session, found := document.Sessions[sessionKey(host, account, clientID)]
	if !found {
		return Session{}, errors.New("protected GitHub App session index is inconsistent")
	}
	if err := validateStoredSession(session); err != nil {
		return Session{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(session.Host), strings.TrimSpace(host)) ||
		strings.TrimSpace(session.ClientID) != strings.TrimSpace(clientID) {
		return Session{}, errors.New("protected GitHub App session scope is inconsistent")
	}
	return session, nil
}

func (store *dpapiSessionStore) SaveActive(ctx context.Context, session Session) error {
	if err := contextFailure(ctx); err != nil {
		return err
	}
	if err := validateStoredSession(session); err != nil {
		return err
	}
	document, err := store.load()
	if err != nil {
		return err
	}
	scope := sessionScopeKey(session.Host, session.ClientID)
	if previousAccount, found := document.ActiveByScope[scope]; found &&
		!strings.EqualFold(strings.TrimSpace(previousAccount), strings.TrimSpace(session.Account)) {
		delete(document.Sessions, sessionKey(session.Host, previousAccount, session.ClientID))
	}
	document.Sessions[sessionKey(session.Host, session.Account, session.ClientID)] = session
	document.ActiveByScope[scope] = session.Account
	document.ActiveScopeByHost[normalizeHost(session.Host)] = scope
	return store.save(document)
}

func (store *dpapiSessionStore) DeleteActive(ctx context.Context, host, clientID string) error {
	if err := contextFailure(ctx); err != nil {
		return err
	}
	document, err := store.load()
	if err != nil {
		return err
	}
	scope := sessionScopeKey(host, clientID)
	account, found := document.ActiveByScope[scope]
	if !found || strings.TrimSpace(account) == "" {
		return errSessionNotFound
	}
	delete(document.ActiveByScope, scope)
	delete(document.Sessions, sessionKey(host, account, clientID))
	if document.ActiveScopeByHost[normalizeHost(host)] == scope {
		delete(document.ActiveScopeByHost, normalizeHost(host))
	}
	return store.save(document)
}

func (store *dpapiSessionStore) load() (sessionDocument, error) {
	path, err := store.path()
	if err != nil {
		return sessionDocument{}, err
	}
	encrypted, err := readSessionFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return emptySessionDocument(), nil
	}
	if err != nil {
		return sessionDocument{}, fmt.Errorf("read protected GitHub App session: %w", err)
	}
	plain, err := unprotectDPAPI(encrypted)
	if err != nil {
		return sessionDocument{}, fmt.Errorf("decrypt protected GitHub App session: %w", err)
	}
	var document sessionDocument
	if err := json.Unmarshal(plain, &document); err != nil {
		return sessionDocument{}, errors.New("protected GitHub App session has an invalid format")
	}
	if document.SchemaVersion != sessionStoreSchemaVersion {
		return sessionDocument{}, errors.New("protected GitHub App session has an unsupported schema version")
	}
	if document.ActiveByScope == nil {
		if len(document.Sessions) != 0 {
			return sessionDocument{}, errors.New("protected GitHub App session does not use the client-ID-scoped layout")
		}
		document.ActiveByScope = make(map[string]string)
	}
	if document.Sessions == nil {
		document.Sessions = make(map[string]Session)
	}
	if document.ActiveScopeByHost == nil {
		document.ActiveScopeByHost = make(map[string]string)
	}
	return document, nil
}

func (store *dpapiSessionStore) save(document sessionDocument) error {
	path, err := store.path()
	if err != nil {
		return err
	}
	if len(document.Sessions) == 0 {
		if err := removeSessionFile(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove protected GitHub App session: %w", err)
		}
		return nil
	}
	document.SchemaVersion = sessionStoreSchemaVersion
	// sessionDocument contains only JSON-encodable map, string, and time values.
	encoded, _ := json.Marshal(document)
	encrypted, err := protectDPAPI(encoded)
	if err != nil {
		return fmt.Errorf("encrypt protected GitHub App session: %w", err)
	}
	directory := filepath.Dir(path)
	if err := makeSessionDir(directory, 0o700); err != nil {
		return fmt.Errorf("create protected GitHub App session directory: %w", err)
	}
	temporary, err := createSessionTemp(directory, ".github-app-session-*")
	if err != nil {
		return fmt.Errorf("create protected GitHub App session file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer removeSessionFile(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect GitHub App session file: %w", err)
	}
	if _, err := temporary.Write(encrypted); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write protected GitHub App session: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close protected GitHub App session: %w", err)
	}
	if err := renameSessionFile(temporaryPath, path); err != nil {
		return fmt.Errorf("replace protected GitHub App session: %w", err)
	}
	return nil
}

func defaultDPAPISessionStorePath() (string, error) {
	directory, err := userConfigDirectory()
	if err != nil {
		return "", fmt.Errorf("resolve user configuration directory: %w", err)
	}
	return filepath.Join(directory, "git-governance", "github-app-sessions.dpapi"), nil
}

func emptySessionDocument() sessionDocument {
	return sessionDocument{
		SchemaVersion:     sessionStoreSchemaVersion,
		ActiveScopeByHost: make(map[string]string),
		ActiveByScope:     make(map[string]string),
		Sessions:          make(map[string]Session),
	}
}

func contextFailure(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func protectDPAPI(plain []byte) ([]byte, error) {
	if len(plain) == 0 {
		return nil, errors.New("cannot protect an empty session")
	}
	entropy := []byte(dpapiEntropy)
	input := windows.DataBlob{Size: uint32(len(plain)), Data: &plain[0]}
	optionalEntropy := windows.DataBlob{Size: uint32(len(entropy)), Data: &entropy[0]}
	var output windows.DataBlob
	if err := cryptProtectData(
		&input,
		nil,
		&optionalEntropy,
		0,
		nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN,
		&output,
	); err != nil {
		return nil, err
	}
	defer freeDPAPIBlob(&output)
	protected := append([]byte(nil), unsafe.Slice(output.Data, output.Size)...)
	runtime.KeepAlive(plain)
	runtime.KeepAlive(entropy)
	return protected, nil
}

func unprotectDPAPI(encrypted []byte) ([]byte, error) {
	if len(encrypted) == 0 {
		return nil, errors.New("protected GitHub App session is empty")
	}
	entropy := []byte(dpapiEntropy)
	input := windows.DataBlob{Size: uint32(len(encrypted)), Data: &encrypted[0]}
	optionalEntropy := windows.DataBlob{Size: uint32(len(entropy)), Data: &entropy[0]}
	var output windows.DataBlob
	if err := cryptUnprotectData(
		&input,
		nil,
		&optionalEntropy,
		0,
		nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN,
		&output,
	); err != nil {
		return nil, err
	}
	defer freeDPAPIBlob(&output)
	plain := append([]byte(nil), unsafe.Slice(output.Data, output.Size)...)
	runtime.KeepAlive(encrypted)
	runtime.KeepAlive(entropy)
	return plain, nil
}

func freeDPAPIBlob(blob *windows.DataBlob) {
	if blob == nil || blob.Data == nil {
		return
	}
	_, _ = freeLocalMemory(windows.Handle(uintptr(unsafe.Pointer(blob.Data))))
	blob.Data = nil
	blob.Size = 0
}
