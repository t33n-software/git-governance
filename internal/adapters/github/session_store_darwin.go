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
	return store.store(ctx, scope, macOSKeychainActiveAccount, []byte(session.Account))
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
	return store.delete(ctx, scope, macOSKeychainActiveAccount)
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
