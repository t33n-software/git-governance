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

// LoadActiveForHost resolves the active session through the host-level
// pointer record, which stores the active client ID under the bare host key.
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
	return store.store(ctx, session.Host, activeScopePointerAccount, []byte(session.ClientID))
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
