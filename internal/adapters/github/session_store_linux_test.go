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
	scope := nativeSessionScope(session.Host, session.ClientID)

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
		if _, err := store.LoadActiveForHost(ctx, session.Host); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled LoadActiveForHost() error = %v", err)
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
		runner.values[runner.key(scope, linuxSecretActiveAccount)] = []byte("octocat")
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
		runner.values[runner.key(scope, linuxSecretActiveAccount)] = []byte("octocat")
		runner.values[runner.key(scope, session.Account)] = []byte("{}")
		if _, err := store.LoadActive(context.Background(), session.Host, session.ClientID); err == nil {
			t.Fatal("LoadActive accepted an incomplete decoded session")
		}

		store, runner = newFakeLinuxStore()
		runner.values[runner.key(scope, linuxSecretActiveAccount)] = []byte("octocat")
		runner.values[runner.key(scope, session.Account)] = []byte(" ")
		if _, err := store.LoadActive(context.Background(), session.Host, session.ClientID); !errors.Is(err, errSessionNotFound) {
			t.Fatalf("empty session error = %v", err)
		}
	})

	t.Run("classifies all save calls and delete calls", func(t *testing.T) {
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
		runner.fail = func(command, _, _ string, call int) error {
			if command == "store" && call == 3 {
				return errors.New("host pointer store failed")
			}
			return nil
		}
		if err := store.SaveActive(context.Background(), session); err == nil {
			t.Fatal("SaveActive accepted a host-pointer store failure")
		}

		store, runner = newFakeLinuxStore()
		runner.values[runner.key(scope, linuxSecretActiveAccount)] = []byte(session.Account)
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
		runner.values[runner.key(scope, linuxSecretActiveAccount)] = []byte(session.Account)
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

	t.Run("resolves the active session through the host pointer", func(t *testing.T) {
		store, runner := newFakeLinuxStore()
		if _, err := store.LoadActiveForHost(context.Background(), "github.com"); !errors.Is(err, errSessionNotFound) {
			t.Fatalf("empty LoadActiveForHost() error = %v", err)
		}
		if err := store.SaveActive(context.Background(), session); err != nil {
			t.Fatalf("SaveActive() error = %v", err)
		}
		loaded, err := store.LoadActiveForHost(context.Background(), "GitHub.COM")
		if err != nil || loaded != session {
			t.Fatalf("LoadActiveForHost() = (%#v, %v)", loaded, err)
		}
		if got := string(runner.values[runner.key(session.Host, activeScopePointerAccount)]); got != session.ClientID {
			t.Fatalf("host pointer = %q, want %q", got, session.ClientID)
		}

		next := testStoredSession("github.com", "octocat")
		next.ClientID = "platform-client-id"
		next.RefreshToken = "ghr-platform"
		if err := store.SaveActive(context.Background(), next); err != nil {
			t.Fatalf("SaveActive(next) error = %v", err)
		}
		if loaded, err := store.LoadActiveForHost(context.Background(), "github.com"); err != nil || loaded != next {
			t.Fatalf("host pointer did not follow the latest login: (%#v, %v)", loaded, err)
		}

		if err := store.DeleteActive(context.Background(), session.Host, session.ClientID); err != nil {
			t.Fatalf("DeleteActive(previous scope) error = %v", err)
		}
		if loaded, err := store.LoadActiveForHost(context.Background(), "github.com"); err != nil || loaded != next {
			t.Fatalf("deleting a non-active scope moved the pointer: (%#v, %v)", loaded, err)
		}
		if err := store.DeleteActive(context.Background(), next.Host, next.ClientID); err != nil {
			t.Fatalf("DeleteActive(active scope) error = %v", err)
		}
		if _, err := store.LoadActiveForHost(context.Background(), "github.com"); !errors.Is(err, errSessionNotFound) {
			t.Fatalf("pointer survived its session deletion: %v", err)
		}
	})

	t.Run("propagates host pointer failures", func(t *testing.T) {
		store, runner := newFakeLinuxStore()
		runner.err = errSessionStoreUnavailable
		if _, err := store.LoadActiveForHost(context.Background(), "github.com"); !errors.Is(err, errSessionStoreUnavailable) {
			t.Fatalf("unavailable pointer lookup error = %v", err)
		}

		store, runner = newFakeLinuxStore()
		runner.values[runner.key(scope, linuxSecretActiveAccount)] = []byte(session.Account)
		if err := store.DeleteActive(context.Background(), session.Host, session.ClientID); err != nil {
			t.Fatalf("DeleteActive() without pointer error = %v", err)
		}

		store, runner = newFakeLinuxStore()
		runner.values[runner.key(scope, linuxSecretActiveAccount)] = []byte(session.Account)
		runner.fail = func(command, _, account string, _ int) error {
			if command == "lookup" && account == activeScopePointerAccount {
				return errSessionStoreUnavailable
			}
			return nil
		}
		if err := store.DeleteActive(context.Background(), session.Host, session.ClientID); !errors.Is(err, errSessionStoreUnavailable) {
			t.Fatalf("pointer lookup failure = %v", err)
		}
	})

	t.Run("replaces only the current client account and rejects scoped mismatches", func(t *testing.T) {
		store, runner := newFakeLinuxStore()
		next := testStoredSession(session.Host, "hubot")
		next.ClientID = session.ClientID
		runner.values[runner.key(scope, linuxSecretActiveAccount)] = []byte(session.Account)
		runner.values[runner.key(scope, session.Account)] = []byte("previous-refresh-session")
		if err := store.SaveActive(context.Background(), next); err != nil {
			t.Fatalf("SaveActive() replacement error = %v", err)
		}
		if _, found := runner.values[runner.key(scope, session.Account)]; found {
			t.Fatal("SaveActive retained the replaced account session")
		}
		if got := string(runner.values[runner.key(scope, linuxSecretActiveAccount)]); got != next.Account {
			t.Fatalf("active account = %q, want %q", got, next.Account)
		}

		store, runner = newFakeLinuxStore()
		runner.values[runner.key(scope, linuxSecretActiveAccount)] = []byte(session.Account)
		runner.fail = func(command, _, _ string, _ int) error {
			if command == "clear" {
				return errors.New("replacement clear failed")
			}
			return nil
		}
		if err := store.SaveActive(context.Background(), next); err == nil {
			t.Fatal("SaveActive accepted a replaced-account clear failure")
		}

		store, runner = newFakeLinuxStore()
		runner.err = errSessionStoreUnavailable
		if err := store.SaveActive(context.Background(), session); !errors.Is(err, errSessionStoreUnavailable) {
			t.Fatalf("SaveActive unavailable lookup error = %v", err)
		}

		store, runner = newFakeLinuxStore()
		mismatched := session
		mismatched.ClientID = "other-client-id"
		encoded, err := json.Marshal(mismatched)
		if err != nil {
			t.Fatal(err)
		}
		runner.values[runner.key(scope, linuxSecretActiveAccount)] = []byte(session.Account)
		runner.values[runner.key(scope, session.Account)] = encoded
		if _, err := store.LoadActive(context.Background(), session.Host, session.ClientID); err == nil {
			t.Fatal("LoadActive accepted a session for another client ID")
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

func TestLinuxSecretServiceStoreRepositoryBindings(t *testing.T) {
	session := testStoredSession("github.com", "octocat")

	t.Run("round trips repository bindings and self-heals dangling pointers", func(t *testing.T) {
		store, runner := newFakeLinuxStore()
		if _, err := store.LoadActiveForRepository(context.Background(), "github.com", "acme", "governance"); !errors.Is(err, errSessionNotFound) {
			t.Fatalf("unbound LoadActiveForRepository() error = %v", err)
		}
		if err := store.SaveActive(context.Background(), session); err != nil {
			t.Fatal(err)
		}
		if err := store.BindRepository(context.Background(), "github.com", "acme", "governance", session.ClientID); err != nil {
			t.Fatalf("BindRepository() error = %v", err)
		}
		loaded, err := store.LoadActiveForRepository(context.Background(), "GitHub.COM", "ACME", "Governance")
		if err != nil || loaded != session {
			t.Fatalf("LoadActiveForRepository() = (%#v, %v)", loaded, err)
		}
		bindingKey := runner.key("github.com", repositoryBindingAccount("github.com", "acme", "governance"))
		if got := string(runner.values[bindingKey]); got != session.ClientID {
			t.Fatalf("binding record = %q, want %q", got, session.ClientID)
		}
		if err := store.DeleteActive(context.Background(), session.Host, session.ClientID); err != nil {
			t.Fatal(err)
		}
		if _, err := store.LoadActiveForRepository(context.Background(), "github.com", "acme", "governance"); !errors.Is(err, errSessionNotFound) {
			t.Fatalf("dangling binding LoadActiveForRepository() error = %v", err)
		}
	})

	t.Run("rejects incomplete bindings and unknown scopes", func(t *testing.T) {
		store, _ := newFakeLinuxStore()
		if err := store.SaveActive(context.Background(), session); err != nil {
			t.Fatal(err)
		}
		for _, binding := range []struct{ owner, repository, clientID string }{
			{owner: "", repository: "governance", clientID: session.ClientID},
			{owner: "acme", repository: "", clientID: session.ClientID},
			{owner: "acme", repository: "governance", clientID: " "},
		} {
			if err := store.BindRepository(context.Background(), "github.com", binding.owner, binding.repository, binding.clientID); err == nil {
				t.Fatalf("BindRepository(%#v) unexpectedly succeeded", binding)
			}
		}
		if err := store.BindRepository(context.Background(), "github.com", "acme", "governance", "unknown-client-id"); !errors.Is(err, errSessionNotFound) {
			t.Fatalf("unknown scope BindRepository() error = %v", err)
		}
	})

	t.Run("lists host sessions through the scope index", func(t *testing.T) {
		store, runner := newFakeLinuxStore()
		if sessions, err := store.ListForHost(context.Background(), "github.com"); err != nil || len(sessions) != 0 {
			t.Fatalf("empty ListForHost() = (%#v, %v)", sessions, err)
		}
		platform := testStoredSession("github.com", "octocat")
		platform.ClientID = "platform-client-id"
		platform.RefreshToken = "ghr-platform"
		if err := store.SaveActive(context.Background(), session); err != nil {
			t.Fatal(err)
		}
		if err := store.SaveActive(context.Background(), platform); err != nil {
			t.Fatal(err)
		}
		indexKey := runner.key("github.com", scopeIndexAccount)
		var index []string
		if err := json.Unmarshal(runner.values[indexKey], &index); err != nil {
			t.Fatal(err)
		}
		if len(index) != 2 {
			t.Fatalf("scope index = %#v", index)
		}
		sessions, err := store.ListForHost(context.Background(), "github.com")
		if err != nil {
			t.Fatal(err)
		}
		if len(sessions) != 2 || sessions[0].ClientID != "platform-client-id" || sessions[1].ClientID != session.ClientID {
			t.Fatalf("ListForHost() = %#v", sessions)
		}

		// A stale index entry whose scope was removed is skipped.
		runner.values[indexKey] = []byte(`["ghost-client-id","platform-client-id","` + session.ClientID + `"]`)
		sessions, err = store.ListForHost(context.Background(), "github.com")
		if err != nil || len(sessions) != 2 {
			t.Fatalf("ListForHost() with stale entry = (%#v, %v)", sessions, err)
		}

		runner.values[indexKey] = []byte("{")
		if _, err := store.ListForHost(context.Background(), "github.com"); err == nil {
			t.Fatal("ListForHost accepted a malformed scope index")
		}
	})

	t.Run("maintains the scope index across save and delete", func(t *testing.T) {
		store, runner := newFakeLinuxStore()
		if err := store.SaveActive(context.Background(), session); err != nil {
			t.Fatal(err)
		}
		if err := store.SaveActive(context.Background(), session); err != nil {
			t.Fatal(err)
		}
		indexKey := runner.key("github.com", scopeIndexAccount)
		var index []string
		if err := json.Unmarshal(runner.values[indexKey], &index); err != nil {
			t.Fatal(err)
		}
		if len(index) != 1 || index[0] != session.ClientID {
			t.Fatalf("idempotent scope index = %#v", index)
		}
		if err := store.DeleteActive(context.Background(), session.Host, session.ClientID); err != nil {
			t.Fatal(err)
		}
		if _, found := runner.values[indexKey]; found {
			t.Fatal("scope index survived the deletion of its last scope")
		}
	})

	t.Run("propagates cancellation and store failures through repository operations", func(t *testing.T) {
		store, _ := newFakeLinuxStore()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := store.LoadActiveForRepository(ctx, "github.com", "acme", "governance"); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled LoadActiveForRepository() error = %v", err)
		}
		if _, err := store.ListForHost(ctx, "github.com"); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled ListForHost() error = %v", err)
		}
		if err := store.BindRepository(ctx, "github.com", "acme", "governance", session.ClientID); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled BindRepository() error = %v", err)
		}

		store, runner := newFakeLinuxStore()
		runner.err = errSessionStoreUnavailable
		if _, err := store.ListForHost(context.Background(), "github.com"); !errors.Is(err, errSessionStoreUnavailable) {
			t.Fatalf("unavailable ListForHost() error = %v", err)
		}
		if err := store.BindRepository(context.Background(), "github.com", "acme", "governance", session.ClientID); !errors.Is(err, errSessionStoreUnavailable) {
			t.Fatalf("unavailable BindRepository() error = %v", err)
		}

		store, runner = newFakeLinuxStore()
		runner.fail = func(command, _, _ string, call int) error {
			if command == "store" && call == 4 {
				return errors.New("scope index store failed")
			}
			return nil
		}
		if err := store.SaveActive(context.Background(), session); err == nil {
			t.Fatal("SaveActive accepted a scope-index store failure")
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
