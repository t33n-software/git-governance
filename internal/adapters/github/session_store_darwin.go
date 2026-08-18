//go:build darwin

package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
)

const macOSKeychainActiveAccount = "__git_governance_active__"

var errSessionStoreUnavailable = errors.New("native GitHub App secret store is unavailable")

type macOSKeychainStore struct {
	runner macOSKeychainRunner
}

type macOSKeychainRunner interface {
	run(context.Context, []byte, ...string) ([]byte, error)
}

type macOSSecurityRunner struct {
	binary string
}

func newPlatformSessionStore() SessionStore {
	return &macOSKeychainStore{runner: macOSSecurityRunner{}}
}

func (store *macOSKeychainStore) LoadActive(ctx context.Context, host, clientID string) (Session, error) {
	if err := sessionStoreContextError(ctx); err != nil {
		return Session{}, err
	}
	return store.loadScoped(ctx, nativeSessionScope(host, clientID), host, clientID)
}

// LoadActiveForHost resolves the most recently used session through the
// host-level pointer record, which stores the active client ID under the bare
// host service name. It is a recency pointer for status, logout, and
// discovery ordering, never the publication selection path.
func (store *macOSKeychainStore) LoadActiveForHost(ctx context.Context, host string) (Session, error) {
	if err := sessionStoreContextError(ctx); err != nil {
		return Session{}, err
	}
	clientID, err := store.lookup(ctx, host, activeScopePointerAccount)
	if err != nil {
		return Session{}, err
	}
	return store.loadScoped(ctx, nativeSessionScope(host, strings.TrimSpace(string(clientID))), host, strings.TrimSpace(string(clientID)))
}

// LoadActiveForRepository resolves the session bound to the canonical
// repository identity through its binding record. A missing binding or a
// binding whose session was removed fails closed with errSessionNotFound so
// discovery can rebind.
func (store *macOSKeychainStore) LoadActiveForRepository(
	ctx context.Context,
	host, owner, repository string,
) (Session, error) {
	if err := sessionStoreContextError(ctx); err != nil {
		return Session{}, err
	}
	clientID, err := store.lookup(ctx, host, repositoryBindingAccount(host, owner, repository))
	if err != nil {
		return Session{}, err
	}
	return store.loadScoped(ctx, nativeSessionScope(host, strings.TrimSpace(string(clientID))), host, strings.TrimSpace(string(clientID)))
}

// ListForHost returns every active session scope stored for the host in
// deterministic client-ID order. Index entries whose scope was removed are
// skipped so a partially written index cannot lock out discovery.
func (store *macOSKeychainStore) ListForHost(ctx context.Context, host string) ([]Session, error) {
	if err := sessionStoreContextError(ctx); err != nil {
		return nil, err
	}
	raw, err := store.lookup(ctx, host, scopeIndexAccount)
	if errors.Is(err, errSessionNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	clientIDs, err := parseScopeIndex(raw)
	if err != nil {
		return nil, err
	}
	sessions := make([]Session, 0, len(clientIDs))
	for _, clientID := range clientIDs {
		session, err := store.loadScoped(ctx, nativeSessionScope(host, clientID), host, clientID)
		if errors.Is(err, errSessionNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

// BindRepository binds the canonical repository identity to the session scope
// of the given client ID. Binding an unknown scope fails closed.
func (store *macOSKeychainStore) BindRepository(
	ctx context.Context,
	host, owner, repository, clientID string,
) error {
	if err := sessionStoreContextError(ctx); err != nil {
		return err
	}
	clientID = strings.TrimSpace(clientID)
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(repository) == "" || clientID == "" {
		return errors.New("macOS Keychain GitHub App repository binding is incomplete")
	}
	if _, err := store.lookup(ctx, nativeSessionScope(host, clientID), macOSKeychainActiveAccount); err != nil {
		return err
	}
	return store.store(ctx, host, repositoryBindingAccount(host, owner, repository), []byte(clientID))
}

func (store *macOSKeychainStore) SaveActive(ctx context.Context, session Session) error {
	if err := sessionStoreContextError(ctx); err != nil {
		return err
	}
	if err := validateStoredSession(session); err != nil {
		return err
	}
	scope := nativeSessionScope(session.Host, session.ClientID)
	if previousAccount, err := store.lookup(ctx, scope, macOSKeychainActiveAccount); err == nil &&
		!strings.EqualFold(strings.TrimSpace(string(previousAccount)), strings.TrimSpace(session.Account)) {
		if err := store.delete(ctx, scope, strings.TrimSpace(string(previousAccount))); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, errSessionNotFound) {
		return err
	}
	encoded, _ := json.Marshal(session)
	if err := store.store(ctx, scope, session.Account, encoded); err != nil {
		return err
	}
	if err := store.store(ctx, scope, macOSKeychainActiveAccount, []byte(session.Account)); err != nil {
		return err
	}
	if err := store.store(ctx, session.Host, activeScopePointerAccount, []byte(session.ClientID)); err != nil {
		return err
	}
	return store.addToScopeIndex(ctx, session.Host, session.ClientID)
}

// addToScopeIndex records the client ID in the host scope index exactly once.
func (store *macOSKeychainStore) addToScopeIndex(ctx context.Context, host, clientID string) error {
	index, err := store.scopeIndex(ctx, host)
	if errors.Is(err, errSessionNotFound) {
		index = nil
	} else if err != nil {
		return err
	}
	updated, changed := addClientIDToScopeIndex(index, clientID)
	if !changed {
		return nil
	}
	encoded, _ := json.Marshal(updated)
	return store.store(ctx, host, scopeIndexAccount, encoded)
}

// removeFromScopeIndex drops the client ID from the host scope index and
// clears the index record when it becomes empty.
func (store *macOSKeychainStore) removeFromScopeIndex(ctx context.Context, host, clientID string) error {
	index, err := store.scopeIndex(ctx, host)
	if errors.Is(err, errSessionNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	updated := removeClientIDFromScopeIndex(index, clientID)
	if len(updated) == 0 {
		return store.delete(ctx, host, scopeIndexAccount)
	}
	encoded, _ := json.Marshal(updated)
	return store.store(ctx, host, scopeIndexAccount, encoded)
}

func (store *macOSKeychainStore) scopeIndex(ctx context.Context, host string) ([]string, error) {
	raw, err := store.lookup(ctx, host, scopeIndexAccount)
	if err != nil {
		return nil, err
	}
	return parseScopeIndex(raw)
}

func (store *macOSKeychainStore) DeleteActive(ctx context.Context, host, clientID string) error {
	if err := sessionStoreContextError(ctx); err != nil {
		return err
	}
	scope := nativeSessionScope(host, clientID)
	account, err := store.lookup(ctx, scope, macOSKeychainActiveAccount)
	if err != nil {
		return err
	}
	if err := store.delete(ctx, scope, strings.TrimSpace(string(account))); err != nil {
		return err
	}
	if err := store.delete(ctx, scope, macOSKeychainActiveAccount); err != nil {
		return err
	}
	if err := store.removeFromScopeIndex(ctx, host, clientID); err != nil {
		return err
	}
	pointer, err := store.lookup(ctx, host, activeScopePointerAccount)
	if errors.Is(err, errSessionNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(pointer)) == strings.TrimSpace(clientID) {
		return store.delete(ctx, host, activeScopePointerAccount)
	}
	return nil
}

func (store *macOSKeychainStore) loadScoped(ctx context.Context, scope, host, clientID string) (Session, error) {
	account, err := store.lookup(ctx, scope, macOSKeychainActiveAccount)
	if err != nil {
		return Session{}, err
	}
	encoded, err := store.lookup(ctx, scope, strings.TrimSpace(string(account)))
	if err != nil {
		return Session{}, err
	}
	var session Session
	if err := json.Unmarshal(encoded, &session); err != nil {
		return Session{}, errors.New("macOS Keychain GitHub App session has an invalid format")
	}
	if err := validateStoredSession(session); err != nil {
		return Session{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(session.Host), strings.TrimSpace(host)) ||
		strings.TrimSpace(session.ClientID) != strings.TrimSpace(clientID) {
		return Session{}, errors.New("macOS Keychain GitHub App session scope is inconsistent")
	}
	return session, nil
}

func (store *macOSKeychainStore) lookup(ctx context.Context, host, account string) ([]byte, error) {
	value, err := store.runner.run(
		ctx,
		nil,
		"find-generic-password",
		"-s",
		macOSKeychainService(host),
		"-a",
		account,
		"-w",
	)
	if err != nil {
		if errors.Is(err, errSessionStoreUnavailable) {
			return nil, err
		}
		return nil, errSessionNotFound
	}
	if strings.TrimSpace(string(value)) == "" {
		return nil, errSessionNotFound
	}
	return value, nil
}

func (store *macOSKeychainStore) store(ctx context.Context, host, account string, value []byte) error {
	_, err := store.runner.run(
		ctx,
		value,
		"add-generic-password",
		"-s",
		macOSKeychainService(host),
		"-a",
		account,
		"-U",
		"-w",
	)
	if err != nil {
		return errors.New("macOS Keychain could not store the GitHub App session")
	}
	return nil
}

func (store *macOSKeychainStore) delete(ctx context.Context, host, account string) error {
	_, err := store.runner.run(
		ctx,
		nil,
		"delete-generic-password",
		"-s",
		macOSKeychainService(host),
		"-a",
		account,
	)
	if err != nil {
		return errors.New("macOS Keychain could not delete the GitHub App session")
	}
	return nil
}

func (runner macOSSecurityRunner) run(ctx context.Context, input []byte, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, runner.executable(), arguments...)
	command.Stdin = bytes.NewReader(input)
	output, err := command.Output()
	if errors.Is(err, exec.ErrNotFound) {
		return nil, errSessionStoreUnavailable
	}
	return output, err
}

func (runner macOSSecurityRunner) executable() string {
	if runner.binary == "" {
		return "security"
	}
	return runner.binary
}

func macOSKeychainService(host string) string {
	return "git-governance.github-app." + strings.ToLower(strings.TrimSpace(host))
}

func sessionStoreContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
