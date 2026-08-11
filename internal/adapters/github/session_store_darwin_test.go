//go:build darwin

package github

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

const macOSKeychainRunnerHelperEnvironment = "GIT_GOVERNANCE_MACOS_KEYCHAIN_RUNNER_HELPER"

func TestMacOSKeychainStorePreservesRefreshSessionsWithoutSecretArguments(t *testing.T) {
	runner := &fakeMacOSKeychainRunner{values: make(map[string][]byte)}
	store := &macOSKeychainStore{runner: runner}
	session := testStoredSession("github.com", "octocat")
	if err := store.SaveActive(context.Background(), session); err != nil {
		t.Fatalf("SaveActive() error = %v", err)
	}
	if strings.Contains(strings.Join(runner.arguments, " "), session.RefreshToken) {
		t.Fatalf("Keychain command arguments leaked a refresh token: %#v", runner.arguments)
	}
	loaded, err := store.LoadActive(context.Background(), "github.com", session.ClientID)
	if err != nil || loaded != session {
		t.Fatalf("LoadActive() = (%#v, %v)", loaded, err)
	}
	if err := store.DeleteActive(context.Background(), "github.com", session.ClientID); err != nil {
		t.Fatalf("DeleteActive() error = %v", err)
	}
	if _, err := store.LoadActive(context.Background(), "github.com", session.ClientID); !errors.Is(err, errSessionNotFound) {
		t.Fatalf("deleted LoadActive() error = %v", err)
	}
}

func TestMacOSKeychainStoreIsolatesClientIDsAndRejectsLegacyStorage(t *testing.T) {
	store, runner := newFakeMacOSStore()
	tenantA := testStoredSession("github.com", "octocat")
	tenantA.ClientID = "tenant-a-client-id"
	tenantA.RefreshToken = "ghr-tenant-a"
	legacyEncoded, err := json.Marshal(tenantA)
	if err != nil {
		t.Fatal(err)
	}
	runner.values[runner.key(tenantA.Host, macOSKeychainActiveAccount)] = []byte(tenantA.Account)
	runner.values[runner.key(tenantA.Host, tenantA.Account)] = legacyEncoded

	if _, err := store.LoadActive(context.Background(), tenantA.Host, tenantA.ClientID); !errors.Is(err, errSessionNotFound) {
		t.Fatalf("LoadActive() error = %v, want legacy storage to be ignored", err)
	}
	if _, found := runner.values[runner.key(tenantA.Host, macOSKeychainActiveAccount)]; !found {
		t.Fatal("current storage read or removed the legacy session")
	}

	if err := store.SaveActive(context.Background(), tenantA); err != nil {
		t.Fatalf("SaveActive(tenant A) error = %v", err)
	}
	platform := testStoredSession("github.com", "octocat")
	platform.ClientID = "platform-client-id"
	platform.RefreshToken = "ghr-platform"
	if err := store.SaveActive(context.Background(), platform); err != nil {
		t.Fatalf("SaveActive(platform) error = %v", err)
	}
	if loaded, err := store.LoadActive(context.Background(), platform.Host, platform.ClientID); err != nil || loaded != platform {
		t.Fatalf("platform LoadActive() = (%#v, %v)", loaded, err)
	}
	if err := store.DeleteActive(context.Background(), platform.Host, platform.ClientID); err != nil {
		t.Fatalf("DeleteActive(platform) error = %v", err)
	}
	if _, err := store.LoadActive(context.Background(), tenantA.Host, tenantA.ClientID); err != nil {
		t.Fatalf("tenant session was removed by platform deletion: %v", err)
	}
	if !strings.Contains(strings.Join(runner.arguments, "\n"), "-s "+macOSKeychainService(nativeSessionScope(tenantA.Host, tenantA.ClientID))) {
		t.Fatalf("current session storage did not use the canonical Keychain namespace: %#v", runner.arguments)
	}
}

func TestMacOSKeychainStoreRejectsFailureModes(t *testing.T) {
	runner := &fakeMacOSKeychainRunner{values: make(map[string][]byte)}
	store := &macOSKeychainStore{runner: runner}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.SaveActive(ctx, testStoredSession("github.com", "octocat")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled SaveActive() error = %v", err)
	}
	if err := store.SaveActive(context.Background(), Session{}); err == nil {
		t.Fatal("SaveActive accepted an incomplete session")
	}
	runner.err = errors.New("keychain unavailable")
	if _, err := store.LoadActive(context.Background(), "github.com", "public-client-id"); !errors.Is(err, errSessionNotFound) {
		t.Fatalf("lookup failure = %v", err)
	}
	runner.err = nil
	scope := nativeSessionScope("github.com", "public-client-id")
	runner.values[runner.key(scope, macOSKeychainActiveAccount)] = []byte("octocat")
	runner.values[runner.key(scope, "octocat")] = []byte("{")
	if _, err := store.LoadActive(context.Background(), "github.com", "public-client-id"); err == nil {
		t.Fatal("LoadActive accepted malformed Keychain JSON")
	}
	if macOSKeychainService(" GitHub.COM ") != "git-governance.github-app.github.com" {
		t.Fatal("macOS Keychain service was not host-isolated")
	}
}

func TestMacOSKeychainStoreWhiteboxErrorPaths(t *testing.T) {
	session := testStoredSession("github.com", "octocat")

	t.Run("uses the macOS-native store and executable contract", func(t *testing.T) {
		if _, ok := newPlatformSessionStore().(*macOSKeychainStore); !ok {
			t.Fatalf("platform session store = %T, want *macOSKeychainStore", newPlatformSessionStore())
		}
		if got := (macOSSecurityRunner{}).executable(); got != "security" {
			t.Fatalf("default security binary = %q", got)
		}
		if got := (macOSSecurityRunner{binary: os.Args[0]}).executable(); got != os.Args[0] {
			t.Fatalf("configured security binary = %q", got)
		}
		t.Setenv(macOSKeychainRunnerHelperEnvironment, "success")
		if _, err := (macOSSecurityRunner{binary: os.Args[0]}).run(
			context.Background(),
			nil,
			"-test.run=^TestMacOSKeychainRunnerProcess$",
		); err != nil {
			t.Fatalf("native runner success error = %v", err)
		}
		if _, err := (macOSSecurityRunner{binary: "git-governance-missing-security"}).run(context.Background(), nil, "version"); !errors.Is(err, errSessionStoreUnavailable) {
			t.Fatalf("missing native runner error = %v", err)
		}
		t.Setenv(macOSKeychainRunnerHelperEnvironment, "failure")
		if _, err := (macOSSecurityRunner{binary: os.Args[0]}).run(
			context.Background(),
			nil,
			"-test.run=^TestMacOSKeychainRunnerProcess$",
		); err == nil {
			t.Fatal("native runner accepted a failing command")
		}
	})

	t.Run("propagates cancellation through every store operation", func(t *testing.T) {
		store, _ := newFakeMacOSStore()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := store.LoadActive(ctx, session.Host, session.ClientID); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled LoadActive() error = %v", err)
		}
		if err := store.SaveActive(ctx, session); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled SaveActive() error = %v", err)
		}
		if err := store.DeleteActive(ctx, session.Host, session.ClientID); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled DeleteActive() error = %v", err)
		}
		if sessionStoreContextError(testNilContext()) != nil || sessionStoreContextError(context.Background()) != nil {
			t.Fatal("active or nil test contexts must not report a store cancellation")
		}
		if !errors.Is(sessionStoreContextError(ctx), context.Canceled) {
			t.Fatal("cancelled context was not preserved")
		}
	})

	t.Run("classifies all load failures", func(t *testing.T) {
		store, runner := newFakeMacOSStore()
		runner.err = errSessionStoreUnavailable
		if _, err := store.LoadActive(context.Background(), session.Host, session.ClientID); !errors.Is(err, errSessionStoreUnavailable) {
			t.Fatalf("unavailable keychain error = %v", err)
		}

		store, runner = newFakeMacOSStore()
		runner.values[runner.key(session.Host, macOSKeychainActiveAccount)] = []byte(session.Account)
		runner.fail = func(command, _, account string, _ int) error {
			if command == "find-generic-password" && account == session.Account {
				return errors.New("account lookup failed")
			}
			return nil
		}
		if _, err := store.LoadActive(context.Background(), session.Host, session.ClientID); !errors.Is(err, errSessionNotFound) {
			t.Fatalf("missing account session error = %v", err)
		}

		store, runner = newFakeMacOSStore()
		runner.values[runner.key(session.Host, macOSKeychainActiveAccount)] = []byte(session.Account)
		runner.values[runner.key(session.Host, session.Account)] = []byte("{}")
		if _, err := store.LoadActive(context.Background(), session.Host, session.ClientID); err == nil {
			t.Fatal("LoadActive accepted an incomplete decoded session")
		}

		store, runner = newFakeMacOSStore()
		runner.values[runner.key(session.Host, macOSKeychainActiveAccount)] = []byte(session.Account)
		runner.values[runner.key(session.Host, session.Account)] = []byte(" ")
		if _, err := store.LoadActive(context.Background(), session.Host, session.ClientID); !errors.Is(err, errSessionNotFound) {
			t.Fatalf("empty session error = %v", err)
		}
	})

	t.Run("classifies both save calls and both delete calls", func(t *testing.T) {
		store, runner := newFakeMacOSStore()
		runner.err = errSessionStoreUnavailable
		if err := store.DeleteActive(context.Background(), session.Host, session.ClientID); !errors.Is(err, errSessionStoreUnavailable) {
			t.Fatalf("DeleteActive unavailable keychain error = %v", err)
		}

		store, runner = newFakeMacOSStore()
		runner.fail = func(command, _, _ string, call int) error {
			if command == "add-generic-password" && call == 1 {
				return errors.New("first store failed")
			}
			return nil
		}
		if err := store.SaveActive(context.Background(), session); err == nil {
			t.Fatal("SaveActive accepted a first-store failure")
		}

		store, runner = newFakeMacOSStore()
		runner.fail = func(command, _, _ string, call int) error {
			if command == "add-generic-password" && call == 2 {
				return errors.New("active profile store failed")
			}
			return nil
		}
		if err := store.SaveActive(context.Background(), session); err == nil {
			t.Fatal("SaveActive accepted an active-profile store failure")
		}

		store, runner = newFakeMacOSStore()
		runner.values[runner.key(session.Host, macOSKeychainActiveAccount)] = []byte(session.Account)
		runner.fail = func(command, _, _ string, call int) error {
			if command == "delete-generic-password" && call == 1 {
				return errors.New("session delete failed")
			}
			return nil
		}
		if err := store.DeleteActive(context.Background(), session.Host, session.ClientID); err == nil {
			t.Fatal("DeleteActive accepted a session-delete failure")
		}

		store, runner = newFakeMacOSStore()
		runner.values[runner.key(session.Host, macOSKeychainActiveAccount)] = []byte(session.Account)
		runner.fail = func(command, _, _ string, call int) error {
			if command == "delete-generic-password" && call == 2 {
				return errors.New("active profile delete failed")
			}
			return nil
		}
		if err := store.DeleteActive(context.Background(), session.Host, session.ClientID); err == nil {
			t.Fatal("DeleteActive accepted an active-profile delete failure")
		}
	})

	t.Run("exercises direct lookup store and delete contracts", func(t *testing.T) {
		store, runner := newFakeMacOSStore()
		runner.err = errSessionStoreUnavailable
		if _, err := store.lookup(context.Background(), session.Host, session.Account); !errors.Is(err, errSessionStoreUnavailable) {
			t.Fatalf("lookup unavailable error = %v", err)
		}

		store, runner = newFakeMacOSStore()
		runner.fail = func(command, _, _ string, _ int) error {
			if command == "add-generic-password" || command == "delete-generic-password" {
				return errors.New(command + " failed")
			}
			return nil
		}
		if err := store.store(context.Background(), session.Host, session.Account, []byte("value")); err == nil {
			t.Fatal("store accepted a runner failure")
		}
		if err := store.delete(context.Background(), session.Host, session.Account); err == nil {
			t.Fatal("delete accepted a runner failure")
		}
		if got := macOSArgument(nil, "missing"); got != "" {
			t.Fatalf("missing Keychain argument = %q", got)
		}
	})
}

func TestMacOSKeychainRunnerProcess(t *testing.T) {
	switch os.Getenv(macOSKeychainRunnerHelperEnvironment) {
	case "success":
		return
	case "failure":
		os.Exit(1)
	}
}

func newFakeMacOSStore() (*macOSKeychainStore, *fakeMacOSKeychainRunner) {
	runner := &fakeMacOSKeychainRunner{values: make(map[string][]byte)}
	return &macOSKeychainStore{runner: runner}, runner
}

type fakeMacOSKeychainRunner struct {
	values    map[string][]byte
	err       error
	arguments []string
	calls     map[string]int
	fail      func(command, service, account string, call int) error
}

func (runner *fakeMacOSKeychainRunner) run(_ context.Context, input []byte, arguments ...string) ([]byte, error) {
	runner.arguments = append(runner.arguments, strings.Join(arguments, " "))
	if runner.err != nil {
		return nil, runner.err
	}
	command := arguments[0]
	service := macOSArgument(arguments, "-s")
	account := macOSArgument(arguments, "-a")
	if runner.calls == nil {
		runner.calls = make(map[string]int)
	}
	runner.calls[command]++
	if runner.fail != nil {
		if err := runner.fail(command, service, account, runner.calls[command]); err != nil {
			return nil, err
		}
	}
	key := service + "\x00" + account
	switch command {
	case "find-generic-password":
		value, found := runner.values[key]
		if !found {
			return nil, errors.New("not found")
		}
		return append([]byte(nil), value...), nil
	case "add-generic-password":
		runner.values[key] = append([]byte(nil), input...)
		return nil, nil
	case "delete-generic-password":
		delete(runner.values, key)
		return nil, nil
	default:
		return nil, errors.New("unexpected Keychain command")
	}
}

func (runner *fakeMacOSKeychainRunner) key(host, account string) string {
	return macOSKeychainService(host) + "\x00" + account
}

func macOSArgument(arguments []string, flag string) string {
	for index := range arguments {
		if arguments[index] == flag && index+1 < len(arguments) {
			return arguments[index+1]
		}
	}
	return ""
}

var _ macOSKeychainRunner = (*fakeMacOSKeychainRunner)(nil)
