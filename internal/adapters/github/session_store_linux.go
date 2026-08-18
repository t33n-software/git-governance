//go:build linux

package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
)

const (
	linuxSecretServiceName   = "git-governance"
	linuxSecretActiveAccount = "__git_governance_active__"
	linuxSecretSessionLabel  = "git-governance GitHub App session"
)

var errSessionStoreUnavailable = errors.New("native GitHub App secret store is unavailable")

type linuxSecretServiceStore struct {
	runner linuxSecretToolRunner
}

type linuxSecretToolRunner interface {
	run(context.Context, []byte, ...string) ([]byte, error)
}

type linuxSecretTool struct {
	binary string
}

func newPlatformSessionStore() SessionStore {
	return &linuxSecretServiceStore{runner: linuxSecretTool{}}
}

func (store *linuxSecretServiceStore) LoadActive(ctx context.Context, host, clientID string) (Session, error) {
	if err := sessionStoreContextError(ctx); err != nil {
		return Session{}, err
	}
	return store.loadScoped(ctx, nativeSessionScope(host, clientID), host, clientID)
}

// LoadActiveForHost resolves the most recently used session through the
// host-level pointer record, which stores the active client ID under the bare
// host key. It is a recency pointer for status, logout, and discovery
// ordering, never the publication selection path.
func (store *linuxSecretServiceStore) LoadActiveForHost(ctx context.Context, host string) (Session, error) {
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
func (store *linuxSecretServiceStore) LoadActiveForRepository(
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
func (store *linuxSecretServiceStore) ListForHost(ctx context.Context, host string) ([]Session, error) {
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
func (store *linuxSecretServiceStore) BindRepository(
	ctx context.Context,
	host, owner, repository, clientID string,
) error {
	if err := sessionStoreContextError(ctx); err != nil {
		return err
	}
	clientID = strings.TrimSpace(clientID)
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(repository) == "" || clientID == "" {
		return errors.New("secret service GitHub App repository binding is incomplete")
	}
	if _, err := store.lookup(ctx, nativeSessionScope(host, clientID), linuxSecretActiveAccount); err != nil {
		return err
	}
	return store.store(ctx, host, repositoryBindingAccount(host, owner, repository), []byte(clientID))
}

func (store *linuxSecretServiceStore) SaveActive(ctx context.Context, session Session) error {
	if err := sessionStoreContextError(ctx); err != nil {
		return err
	}
	if err := validateStoredSession(session); err != nil {
		return err
	}
	scope := nativeSessionScope(session.Host, session.ClientID)
	if previousAccount, err := store.lookup(ctx, scope, linuxSecretActiveAccount); err == nil &&
		!strings.EqualFold(strings.TrimSpace(string(previousAccount)), strings.TrimSpace(session.Account)) {
		if err := store.clear(ctx, scope, strings.TrimSpace(string(previousAccount))); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, errSessionNotFound) {
		return err
	}
	encoded, _ := json.Marshal(session)
	if err := store.store(ctx, scope, session.Account, encoded); err != nil {
		return err
	}
	if err := store.store(ctx, scope, linuxSecretActiveAccount, []byte(session.Account)); err != nil {
		return err
	}
	if err := store.store(ctx, session.Host, activeScopePointerAccount, []byte(session.ClientID)); err != nil {
		return err
	}
	return store.addToScopeIndex(ctx, session.Host, session.ClientID)
}

// addToScopeIndex records the client ID in the host scope index exactly once.
func (store *linuxSecretServiceStore) addToScopeIndex(ctx context.Context, host, clientID string) error {
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
func (store *linuxSecretServiceStore) removeFromScopeIndex(ctx context.Context, host, clientID string) error {
	index, err := store.scopeIndex(ctx, host)
	if errors.Is(err, errSessionNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	updated := removeClientIDFromScopeIndex(index, clientID)
	if len(updated) == 0 {
		return store.clear(ctx, host, scopeIndexAccount)
	}
	encoded, _ := json.Marshal(updated)
	return store.store(ctx, host, scopeIndexAccount, encoded)
}

func (store *linuxSecretServiceStore) scopeIndex(ctx context.Context, host string) ([]string, error) {
	raw, err := store.lookup(ctx, host, scopeIndexAccount)
	if err != nil {
		return nil, err
	}
	return parseScopeIndex(raw)
}

func (store *linuxSecretServiceStore) DeleteActive(ctx context.Context, host, clientID string) error {
	if err := sessionStoreContextError(ctx); err != nil {
		return err
	}
	scope := nativeSessionScope(host, clientID)
	account, err := store.lookup(ctx, scope, linuxSecretActiveAccount)
	if err != nil {
		return err
	}
	if err := store.clear(ctx, scope, strings.TrimSpace(string(account))); err != nil {
		return err
	}
	if err := store.clear(ctx, scope, linuxSecretActiveAccount); err != nil {
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
		return store.clear(ctx, host, activeScopePointerAccount)
	}
	return nil
}

func (store *linuxSecretServiceStore) loadScoped(ctx context.Context, scope, host, clientID string) (Session, error) {
	account, err := store.lookup(ctx, scope, linuxSecretActiveAccount)
	if err != nil {
		return Session{}, err
	}
	encoded, err := store.lookup(ctx, scope, strings.TrimSpace(string(account)))
	if err != nil {
		return Session{}, err
	}
	var session Session
	if err := json.Unmarshal(encoded, &session); err != nil {
		return Session{}, errors.New("secret service GitHub App session has an invalid format")
	}
	if err := validateStoredSession(session); err != nil {
		return Session{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(session.Host), strings.TrimSpace(host)) ||
		strings.TrimSpace(session.ClientID) != strings.TrimSpace(clientID) {
		return Session{}, errors.New("secret service GitHub App session scope is inconsistent")
	}
	return session, nil
}

func (store *linuxSecretServiceStore) lookup(ctx context.Context, host, account string) ([]byte, error) {
	value, err := store.runner.run(
		ctx,
		nil,
		"lookup",
		"service",
		linuxSecretServiceName,
		"host",
		linuxSecretHost(host),
		"account",
		account,
	)
	if errors.Is(err, errSessionStoreUnavailable) {
		return nil, err
	}
	if err != nil || strings.TrimSpace(string(value)) == "" {
		return nil, errSessionNotFound
	}
	return value, nil
}

func (store *linuxSecretServiceStore) store(ctx context.Context, host, account string, value []byte) error {
	_, err := store.runner.run(
		ctx,
		value,
		"store",
		"--label="+linuxSecretSessionLabel,
		"service",
		linuxSecretServiceName,
		"host",
		linuxSecretHost(host),
		"account",
		account,
	)
	if err != nil {
		return errors.New("secret service could not store the GitHub App session")
	}
	return nil
}

func (store *linuxSecretServiceStore) clear(ctx context.Context, host, account string) error {
	_, err := store.runner.run(
		ctx,
		nil,
		"clear",
		"service",
		linuxSecretServiceName,
		"host",
		linuxSecretHost(host),
		"account",
		account,
	)
	if err != nil {
		return errors.New("secret service could not delete the GitHub App session")
	}
	return nil
}

func (tool linuxSecretTool) run(ctx context.Context, input []byte, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, tool.executable(), arguments...)
	command.Stdin = bytes.NewReader(input)
	output, err := command.Output()
	if errors.Is(err, exec.ErrNotFound) {
		return nil, errSessionStoreUnavailable
	}
	return output, err
}

func (tool linuxSecretTool) executable() string {
	if tool.binary == "" {
		return "secret-tool"
	}
	return tool.binary
}

func linuxSecretHost(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}

func sessionStoreContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
