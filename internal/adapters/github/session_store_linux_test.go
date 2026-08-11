//go:build linux

package github

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestLinuxSecretServiceStorePreservesRefreshSessionsWithoutSecretArguments(t *testing.T) {
	runner := &fakeLinuxSecretTool{values: make(map[string][]byte)}
	store := &linuxSecretServiceStore{runner: runner}
	session := testStoredSession("github.com", "octocat")
	if err := store.SaveActive(context.Background(), session); err != nil {
		t.Fatalf("SaveActive() error = %v", err)
	}
	if strings.Contains(strings.Join(runner.arguments, " "), session.RefreshToken) {
		t.Fatalf("Secret Service command arguments leaked a refresh token: %#v", runner.arguments)
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

func TestLinuxSecretServiceStoreIsolatesClientIDsAndRejectsLegacyStorage(t *testing.T) {
	store, runner := newFakeLinuxStore()
	tenantA := testStoredSession("github.com", "octocat")
	tenantA.ClientID = "tenant-a-client-id"
	tenantA.RefreshToken = "ghr-tenant-a"
	legacyEncoded, err := json.Marshal(tenantA)
	if err != nil {
		t.Fatal(err)
	}
	runner.values[runner.key(tenantA.Host, linuxSecretActiveAccount)] = []byte(tenantA.Account)
	runner.values[runner.key(tenantA.Host, tenantA.Account)] = legacyEncoded

	if _, err := store.LoadActive(context.Background(), tenantA.Host, tenantA.ClientID); !errors.Is(err, errSessionNotFound) {
		t.Fatalf("LoadActive() error = %v, want legacy storage to be ignored", err)
	}
	if _, found := runner.values[runner.key(tenantA.Host, linuxSecretActiveAccount)]; !found {
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
	if !strings.Contains(strings.Join(runner.arguments, "\n"), "service "+linuxSecretServiceName) {
		t.Fatalf("current session storage did not use the canonical Secret Service namespace: %#v", runner.arguments)
	}
}

func TestLinuxSecretServiceStoreRejectsFailureModes(t *testing.T) {
	runner := &fakeLinuxSecretTool{values: make(map[string][]byte)}
	store := &linuxSecretServiceStore{runner: runner}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.SaveActive(ctx, testStoredSession("github.com", "octocat")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled SaveActive() error = %v", err)
	}
	if err := store.SaveActive(context.Background(), Session{}); err == nil {
		t.Fatal("SaveActive accepted an incomplete session")
	}
	runner.err = errors.New("Secret Service unavailable")
	if _, err := store.LoadActive(context.Background(), "github.com", "public-client-id"); !errors.Is(err, errSessionNotFound) {
		t.Fatalf("lookup failure = %v", err)
	}
	runner.err = nil
	scope := nativeSessionScope("github.com", "public-client-id")
	runner.values[runner.key(scope, linuxSecretActiveAccount)] = []byte("octocat")
	runner.values[runner.key(scope, "octocat")] = []byte("{")
	if _, err := store.LoadActive(context.Background(), "github.com", "public-client-id"); err == nil {
		t.Fatal("LoadActive accepted malformed Secret Service JSON")
	}
	if linuxSecretHost(" GitHub.COM ") != "github.com" {
		t.Fatal("Secret Service host key was not normalized")
	}
}

func TestLinuxSecretServiceStoreWhiteboxErrorPaths(t *testing.T) {
	session := testStoredSession("github.com", "octocat")

	t.Run("uses the Linux-native store and executable contract", func(t *testing.T) {
		if _, ok := newPlatformSessionStore().(*linuxSecretServiceStore); !ok {
			t.Fatalf("platform session store = %T, want *linuxSecretServiceStore", newPlatformSessionStore())
		}
		if got := (linuxSecretTool{}).executable(); got != "secret-tool" {
			t.Fatalf("default secret-tool binary = %q", got)
		}
		if got := (linuxSecretTool{binary: "go"}).executable(); got != "go" {
			t.Fatalf("configured secret-tool binary = %q", got)
		}
		if _, err := (linuxSecretTool{binary: "go"}).run(context.Background(), nil, "version"); err != nil {
			t.Fatalf("native runner success error = %v", err)
		}
		if _, err := (linuxSecretTool{binary: "go"}).run(context.Background(), nil, "tool", "definitely-not-a-go-tool"); err == nil || errors.Is(err, errSessionStoreUnavailable) {
			t.Fatalf("controlled native runner failure = %v", err)
		}
		if _, err := (linuxSecretTool{binary: "git-governance-missing-secret-tool"}).run(context.Background(), nil, "version"); !errors.Is(err, errSessionStoreUnavailable) {
			t.Fatalf("missing native runner error = %v", err)
		}
	})

	t.Run("propagates cancellation through every store operation", func(t *testing.T) {
		store, _ := newFakeLinuxStore()
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
		store, runner := newFakeLinuxStore()
		runner.err = errSessionStoreUnavailable
		if _, err := store.LoadActive(context.Background(), session.Host, session.ClientID); !errors.Is(err, errSessionStoreUnavailable) {
			t.Fatalf("unavailable secret store error = %v", err)
		}

		store, runner = newFakeLinuxStore()
		runner.values[runner.key(session.Host, linuxSecretActiveAccount)] = []byte("octocat")
		runner.fail = func(command, _, account string, _ int) error {
			if command == "lookup" && account == session.Account {
				return errors.New("account lookup failed")
			}
			return nil
		}
		if _, err := store.LoadActive(context.Background(), session.Host, session.ClientID); !errors.Is(err, errSessionNotFound) {
			t.Fatalf("missing account session error = %v", err)
		}

		store, runner = newFakeLinuxStore()
		runner.values[runner.key(session.Host, linuxSecretActiveAccount)] = []byte("octocat")
		runner.values[runner.key(session.Host, session.Account)] = []byte("{}")
		if _, err := store.LoadActive(context.Background(), session.Host, session.ClientID); err == nil {
			t.Fatal("LoadActive accepted an incomplete decoded session")
		}

		store, runner = newFakeLinuxStore()
		runner.values[runner.key(session.Host, linuxSecretActiveAccount)] = []byte("octocat")
		runner.values[runner.key(session.Host, session.Account)] = []byte(" ")
		if _, err := store.LoadActive(context.Background(), session.Host, session.ClientID); !errors.Is(err, errSessionNotFound) {
			t.Fatalf("empty session error = %v", err)
		}
	})

	t.Run("classifies both save calls and both delete calls", func(t *testing.T) {
		store, runner := newFakeLinuxStore()
		runner.err = errSessionStoreUnavailable
		if err := store.DeleteActive(context.Background(), session.Host, session.ClientID); !errors.Is(err, errSessionStoreUnavailable) {
			t.Fatalf("DeleteActive unavailable secret store error = %v", err)
		}

		store, runner = newFakeLinuxStore()
		runner.fail = func(command, _, _ string, call int) error {
			if command == "store" && call == 1 {
				return errors.New("first store failed")
			}
			return nil
		}
		if err := store.SaveActive(context.Background(), session); err == nil {
			t.Fatal("SaveActive accepted a first-store failure")
		}

		store, runner = newFakeLinuxStore()
		runner.fail = func(command, _, _ string, call int) error {
			if command == "store" && call == 2 {
				return errors.New("active profile store failed")
			}
			return nil
		}
		if err := store.SaveActive(context.Background(), session); err == nil {
			t.Fatal("SaveActive accepted an active-profile store failure")
		}

		store, runner = newFakeLinuxStore()
		runner.values[runner.key(session.Host, linuxSecretActiveAccount)] = []byte(session.Account)
		runner.fail = func(command, _, _ string, call int) error {
			if command == "clear" && call == 1 {
				return errors.New("session clear failed")
			}
			return nil
		}
		if err := store.DeleteActive(context.Background(), session.Host, session.ClientID); err == nil {
			t.Fatal("DeleteActive accepted a session-clear failure")
		}

		store, runner = newFakeLinuxStore()
		runner.values[runner.key(session.Host, linuxSecretActiveAccount)] = []byte(session.Account)
		runner.fail = func(command, _, _ string, call int) error {
			if command == "clear" && call == 2 {
				return errors.New("active profile clear failed")
			}
			return nil
		}
		if err := store.DeleteActive(context.Background(), session.Host, session.ClientID); err == nil {
			t.Fatal("DeleteActive accepted an active-profile clear failure")
		}
	})

	t.Run("exercises direct lookup store and clear contracts", func(t *testing.T) {
		store, runner := newFakeLinuxStore()
		runner.err = errSessionStoreUnavailable
		if _, err := store.lookup(context.Background(), session.Host, session.Account); !errors.Is(err, errSessionStoreUnavailable) {
			t.Fatalf("lookup unavailable error = %v", err)
		}

		store, runner = newFakeLinuxStore()
		runner.fail = func(command, _, _ string, _ int) error {
			if command == "store" || command == "clear" {
				return errors.New(command + " failed")
			}
			return nil
		}
		if err := store.store(context.Background(), session.Host, session.Account, []byte("value")); err == nil {
			t.Fatal("store accepted a runner failure")
		}
		if err := store.clear(context.Background(), session.Host, session.Account); err == nil {
			t.Fatal("clear accepted a runner failure")
		}
		if got := linuxSecretArgument(nil, "missing"); got != "" {
			t.Fatalf("missing Linux secret argument = %q", got)
		}
	})
}

func newFakeLinuxStore() (*linuxSecretServiceStore, *fakeLinuxSecretTool) {
	runner := &fakeLinuxSecretTool{values: make(map[string][]byte)}
	return &linuxSecretServiceStore{runner: runner}, runner
}

type fakeLinuxSecretTool struct {
	values    map[string][]byte
	err       error
	arguments []string
	calls     map[string]int
	fail      func(command, host, account string, call int) error
}

func (runner *fakeLinuxSecretTool) run(_ context.Context, input []byte, arguments ...string) ([]byte, error) {
	runner.arguments = append(runner.arguments, strings.Join(arguments, " "))
	if runner.err != nil {
		return nil, runner.err
	}
	command := arguments[0]
	host := linuxSecretArgument(arguments, "host")
	account := linuxSecretArgument(arguments, "account")
	if runner.calls == nil {
		runner.calls = make(map[string]int)
	}
	runner.calls[command]++
	if runner.fail != nil {
		if err := runner.fail(command, host, account, runner.calls[command]); err != nil {
			return nil, err
		}
	}
	key := runner.key(host, account)
	switch command {
	case "lookup":
		value, found := runner.values[key]
		if !found {
			return nil, errors.New("not found")
		}
		return append([]byte(nil), value...), nil
	case "store":
		runner.values[key] = append([]byte(nil), input...)
		return nil, nil
	case "clear":
		delete(runner.values, key)
		return nil, nil
	default:
		return nil, errors.New("unexpected Secret Service command")
	}
}

func (runner *fakeLinuxSecretTool) key(host, account string) string {
	return linuxSecretHost(host) + "\x00" + account
}

func linuxSecretArgument(arguments []string, name string) string {
	for index := range arguments {
		if arguments[index] == name && index+1 < len(arguments) {
			return arguments[index+1]
		}
	}
	return ""
}

var _ linuxSecretToolRunner = (*fakeLinuxSecretTool)(nil)
