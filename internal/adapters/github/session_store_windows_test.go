//go:build windows

package github

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestDPAPISessionStoreRoundTripsAndRemovesEncryptedSessions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.dpapi")
	store := newDPAPISessionStore(path)
	ctx := context.Background()

	if _, err := store.LoadActive(ctx, "github.com", "public-client-id"); !errors.Is(err, errSessionNotFound) {
		t.Fatalf("missing LoadActive() error = %v", err)
	}
	if err := store.SaveActive(ctx, Session{}); err == nil {
		t.Fatal("SaveActive accepted an incomplete session")
	}

	octocat := testStoredSession("github.com", "octocat")
	if err := store.SaveActive(ctx, octocat); err != nil {
		t.Fatalf("SaveActive() error = %v", err)
	}
	encrypted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encrypted), octocat.RefreshToken) {
		t.Fatal("DPAPI session file contains the plaintext refresh token")
	}
	loaded, err := store.LoadActive(ctx, "GitHub.COM", octocat.ClientID)
	if err != nil || loaded != octocat {
		t.Fatalf("LoadActive() = (%#v, %v)", loaded, err)
	}

	enterprise := testStoredSession("github.enterprise.example", "hubot")
	if err := store.SaveActive(ctx, enterprise); err != nil {
		t.Fatalf("second SaveActive() error = %v", err)
	}
	if err := store.DeleteActive(ctx, enterprise.Host, enterprise.ClientID); err != nil {
		t.Fatalf("DeleteActive() with another stored profile error = %v", err)
	}
	if _, err := store.LoadActive(ctx, enterprise.Host, enterprise.ClientID); !errors.Is(err, errSessionNotFound) {
		t.Fatalf("deleted profile LoadActive() error = %v", err)
	}
	if _, err := store.LoadActive(ctx, octocat.Host, octocat.ClientID); err != nil {
		t.Fatalf("remaining profile LoadActive() error = %v", err)
	}

	if err := store.DeleteActive(ctx, octocat.Host, octocat.ClientID); err != nil {
		t.Fatalf("final DeleteActive() error = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("session file stat error = %v, want not exist", err)
	}
	if err := store.DeleteActive(ctx, octocat.Host, octocat.ClientID); !errors.Is(err, errSessionNotFound) {
		t.Fatalf("second DeleteActive() error = %v", err)
	}
}

func TestDPAPISessionStoreIsolatesClientIDsAndRejectsLegacySchema(t *testing.T) {
	ctx := context.Background()
	tenantA := testStoredSession("github.com", "octocat")
	tenantA.ClientID = "tenant-a-client-id"
	platform := testStoredSession("github.com", "octocat")
	platform.ClientID = "platform-client-id"
	platform.RefreshToken = "ghr-platform"

	legacyPath := filepath.Join(t.TempDir(), "github-app-sessions.dpapi")
	legacyDocument := struct {
		SchemaVersion int                `json:"schemaVersion"`
		ActiveByHost  map[string]string  `json:"activeByHost"`
		Sessions      map[string]Session `json:"sessions"`
	}{
		SchemaVersion: 1,
		ActiveByHost:  map[string]string{"github.com": tenantA.Account},
		Sessions: map[string]Session{
			"github.com\x00octocat": tenantA,
		},
	}
	encrypted, err := protectDPAPI(mustJSON(t, legacyDocument))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, encrypted, 0o600); err != nil {
		t.Fatal(err)
	}
	legacyStore := newDPAPISessionStore(legacyPath)
	if _, err := legacyStore.LoadActive(ctx, tenantA.Host, tenantA.ClientID); err == nil {
		t.Fatal("LoadActive accepted a legacy schema")
	}
	if err := legacyStore.SaveActive(ctx, tenantA); err == nil {
		t.Fatal("SaveActive accepted a legacy schema")
	}

	store := newDPAPISessionStore(filepath.Join(t.TempDir(), "github-app-sessions.dpapi"))
	if err := store.SaveActive(ctx, tenantA); err != nil {
		t.Fatalf("SaveActive(tenant A) error = %v", err)
	}
	if err := store.SaveActive(ctx, platform); err != nil {
		t.Fatalf("SaveActive(platform) error = %v", err)
	}
	if loaded, err := store.LoadActive(ctx, tenantA.Host, tenantA.ClientID); err != nil || loaded != tenantA {
		t.Fatalf("tenant LoadActive() = (%#v, %v)", loaded, err)
	}
	if err := store.DeleteActive(ctx, platform.Host, platform.ClientID); err != nil {
		t.Fatalf("DeleteActive(platform) error = %v", err)
	}
	if _, err := store.LoadActive(ctx, tenantA.Host, tenantA.ClientID); err != nil {
		t.Fatalf("tenant session was removed by platform deletion: %v", err)
	}
}

func TestDPAPISessionStoreResolvesActiveSessionByHost(t *testing.T) {
	ctx := context.Background()

	t.Run("round trips the host pointer through save load and delete", func(t *testing.T) {
		store := newDPAPISessionStore(filepath.Join(t.TempDir(), "sessions.dpapi"))
		if _, err := store.LoadActiveForHost(ctx, "github.com"); !errors.Is(err, errSessionNotFound) {
			t.Fatalf("empty LoadActiveForHost() error = %v", err)
		}

		tenantA := testStoredSession("github.com", "octocat")
		tenantA.ClientID = "tenant-a-client-id"
		if err := store.SaveActive(ctx, tenantA); err != nil {
			t.Fatalf("SaveActive(tenant A) error = %v", err)
		}
		loaded, err := store.LoadActiveForHost(ctx, "GitHub.COM")
		if err != nil || loaded != tenantA {
			t.Fatalf("LoadActiveForHost() = (%#v, %v)", loaded, err)
		}

		platform := testStoredSession("github.com", "octocat")
		platform.ClientID = "platform-client-id"
		platform.RefreshToken = "ghr-platform"
		if err := store.SaveActive(ctx, platform); err != nil {
			t.Fatalf("SaveActive(platform) error = %v", err)
		}
		loaded, err = store.LoadActiveForHost(ctx, "github.com")
		if err != nil || loaded != platform {
			t.Fatalf("host pointer did not follow the latest login: (%#v, %v)", loaded, err)
		}

		enterprise := testStoredSession("github.enterprise.example", "hubot")
		if err := store.SaveActive(ctx, enterprise); err != nil {
			t.Fatalf("SaveActive(enterprise) error = %v", err)
		}
		if err := store.DeleteActive(ctx, enterprise.Host, enterprise.ClientID); err != nil {
			t.Fatalf("DeleteActive(enterprise) error = %v", err)
		}
		if loaded, err := store.LoadActiveForHost(ctx, "github.com"); err != nil || loaded != platform {
			t.Fatalf("deleting another host moved the pointer: (%#v, %v)", loaded, err)
		}

		if err := store.DeleteActive(ctx, platform.Host, platform.ClientID); err != nil {
			t.Fatalf("DeleteActive(platform) error = %v", err)
		}
		if _, err := store.LoadActiveForHost(ctx, "github.com"); !errors.Is(err, errSessionNotFound) {
			t.Fatalf("pointer survived its session deletion: %v", err)
		}
	})

	t.Run("rejects inconsistent host pointers", func(t *testing.T) {
		for _, document := range []sessionDocument{
			{
				SchemaVersion:     sessionStoreSchemaVersion,
				ActiveScopeByHost: map[string]string{"github.com": "github.com"},
				ActiveByScope:     map[string]string{},
				Sessions:          map[string]Session{},
			},
			{
				SchemaVersion:     sessionStoreSchemaVersion,
				ActiveScopeByHost: map[string]string{"github.com": sessionScopeKey("github.com", "public-client-id")},
				ActiveByScope:     map[string]string{},
				Sessions:          map[string]Session{},
			},
			{
				SchemaVersion:     sessionStoreSchemaVersion,
				ActiveScopeByHost: map[string]string{"github.com": sessionScopeKey("github.com", "public-client-id")},
				ActiveByScope:     map[string]string{sessionScopeKey("github.com", "public-client-id"): "octocat"},
				Sessions:          map[string]Session{},
			},
		} {
			path := filepath.Join(t.TempDir(), "sessions.dpapi")
			encrypted, err := protectDPAPI(mustJSON(t, document))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, encrypted, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := newDPAPISessionStore(path).LoadActiveForHost(ctx, "github.com"); err == nil {
				t.Fatalf("LoadActiveForHost accepted an inconsistent pointer document: %#v", document)
			}
		}
	})

	t.Run("documents without the host pointer fail closed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "sessions.dpapi")
		encrypted, err := protectDPAPI([]byte(`{"schemaVersion":1}`))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, encrypted, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := newDPAPISessionStore(path).LoadActiveForHost(ctx, "github.com"); !errors.Is(err, errSessionNotFound) {
			t.Fatalf("legacy document LoadActiveForHost() error = %v", err)
		}
	})
}

func TestDPAPISessionStoreRepositoryBindings(t *testing.T) {
	ctx := context.Background()

	t.Run("round trips repository bindings and removes them with the session", func(t *testing.T) {
		store := newDPAPISessionStore(filepath.Join(t.TempDir(), "sessions.dpapi"))
		tenantA := testStoredSession("github.com", "octocat")
		tenantA.ClientID = "tenant-a-client-id"
		platform := testStoredSession("github.com", "octocat")
		platform.ClientID = "platform-client-id"
		if err := store.SaveActive(ctx, tenantA); err != nil {
			t.Fatal(err)
		}
		if err := store.SaveActive(ctx, platform); err != nil {
			t.Fatal(err)
		}

		if _, err := store.LoadActiveForRepository(ctx, "github.com", "acme", "governance"); !errors.Is(err, errSessionNotFound) {
			t.Fatalf("unbound LoadActiveForRepository() error = %v", err)
		}
		if err := store.BindRepository(ctx, "github.com", "acme", "governance", tenantA.ClientID); err != nil {
			t.Fatalf("BindRepository() error = %v", err)
		}
		if err := store.BindRepository(ctx, "GitHub.COM", "ACME", "Governance", platform.ClientID); err != nil {
			t.Fatalf("rebinding with different casing error = %v", err)
		}
		loaded, err := store.LoadActiveForRepository(ctx, "github.com", "acme", "governance")
		if err != nil || loaded != platform {
			t.Fatalf("rebound LoadActiveForRepository() = (%#v, %v)", loaded, err)
		}

		if err := store.BindRepository(ctx, "github.com", "acme", "license-hub", tenantA.ClientID); err != nil {
			t.Fatal(err)
		}
		if err := store.DeleteActive(ctx, platform.Host, platform.ClientID); err != nil {
			t.Fatal(err)
		}
		if _, err := store.LoadActiveForRepository(ctx, "github.com", "acme", "governance"); !errors.Is(err, errSessionNotFound) {
			t.Fatalf("binding of the deleted session survived: %v", err)
		}
		if loaded, err := store.LoadActiveForRepository(ctx, "github.com", "acme", "license-hub"); err != nil || loaded != tenantA {
			t.Fatalf("unrelated binding = (%#v, %v)", loaded, err)
		}
	})

	t.Run("rejects incomplete bindings and unknown scopes", func(t *testing.T) {
		store := newDPAPISessionStore(filepath.Join(t.TempDir(), "sessions.dpapi"))
		if err := store.SaveActive(ctx, testStoredSession("github.com", "octocat")); err != nil {
			t.Fatal(err)
		}
		for _, binding := range []struct{ host, owner, repository, clientID string }{
			{host: "github.com", owner: "", repository: "governance", clientID: "public-client-id"},
			{host: "github.com", owner: "acme", repository: "", clientID: "public-client-id"},
			{host: "github.com", owner: "acme", repository: "governance", clientID: " "},
		} {
			if err := store.BindRepository(ctx, binding.host, binding.owner, binding.repository, binding.clientID); err == nil {
				t.Fatalf("BindRepository(%#v) unexpectedly succeeded", binding)
			}
		}
		if err := store.BindRepository(ctx, "github.com", "acme", "governance", "unknown-client-id"); err == nil {
			t.Fatal("BindRepository accepted an unknown session scope")
		}
	})

	t.Run("fails closed when the bound session was removed", func(t *testing.T) {
		store := newDPAPISessionStore(filepath.Join(t.TempDir(), "sessions.dpapi"))
		session := testStoredSession("github.com", "octocat")
		if err := store.SaveActive(ctx, session); err != nil {
			t.Fatal(err)
		}
		if err := store.BindRepository(ctx, "github.com", "acme", "governance", session.ClientID); err != nil {
			t.Fatal(err)
		}
		document, err := store.load()
		if err != nil {
			t.Fatal(err)
		}
		delete(document.Sessions, sessionKey(session.Host, session.Account, session.ClientID))
		if err := store.save(document); err != nil {
			t.Fatal(err)
		}
		if _, err := store.LoadActiveForRepository(ctx, "github.com", "acme", "governance"); !errors.Is(err, errSessionNotFound) {
			t.Fatalf("dangling binding LoadActiveForRepository() error = %v", err)
		}
	})

	t.Run("lists host sessions in deterministic scope order", func(t *testing.T) {
		store := newDPAPISessionStore(filepath.Join(t.TempDir(), "sessions.dpapi"))
		platform := testStoredSession("github.com", "octocat")
		platform.ClientID = "platform-client-id"
		tenantA := testStoredSession("github.com", "octocat")
		tenantA.ClientID = "tenant-a-client-id"
		enterprise := testStoredSession("github.enterprise.example", "hubot")
		for _, session := range []Session{platform, tenantA, enterprise} {
			if err := store.SaveActive(ctx, session); err != nil {
				t.Fatal(err)
			}
		}
		sessions, err := store.ListForHost(ctx, "GitHub.COM")
		if err != nil {
			t.Fatal(err)
		}
		if len(sessions) != 2 || sessions[0].ClientID != "platform-client-id" || sessions[1].ClientID != "tenant-a-client-id" {
			t.Fatalf("ListForHost() = %#v", sessions)
		}
		enterpriseSessions, err := store.ListForHost(ctx, "github.enterprise.example")
		if err != nil || len(enterpriseSessions) != 1 || enterpriseSessions[0] != enterprise {
			t.Fatalf("enterprise ListForHost() = (%#v, %v)", enterpriseSessions, err)
		}
		empty, err := store.ListForHost(ctx, "unknown.example")
		if err != nil || len(empty) != 0 {
			t.Fatalf("unknown host ListForHost() = (%#v, %v)", empty, err)
		}

		document, err := store.load()
		if err != nil {
			t.Fatal(err)
		}
		document.ActiveByScope[sessionScopeKey("github.com", "missing-client-id")] = "octocat"
		if err := store.save(document); err != nil {
			t.Fatal(err)
		}
		if _, err := store.ListForHost(ctx, "github.com"); err == nil {
			t.Fatal("ListForHost accepted an inconsistent session index")
		}
	})

	t.Run("propagates cancellation and read failures through repository operations", func(t *testing.T) {
		store := newDPAPISessionStore(filepath.Join(t.TempDir(), "sessions.dpapi"))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := store.LoadActiveForRepository(ctx, "github.com", "acme", "governance"); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled LoadActiveForRepository() error = %v", err)
		}
		if _, err := store.ListForHost(ctx, "github.com"); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled ListForHost() error = %v", err)
		}
		if err := store.BindRepository(ctx, "github.com", "acme", "governance", "public-client-id"); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled BindRepository() error = %v", err)
		}

		preserveWindowsStoreHooks(t)
		readErr := errors.New("read failed")
		readSessionFile = func(string) ([]byte, error) {
			return nil, readErr
		}
		store = newDPAPISessionStore("session.dpapi")
		if _, err := store.LoadActiveForRepository(context.Background(), "github.com", "acme", "governance"); !errors.Is(err, readErr) {
			t.Fatalf("LoadActiveForRepository read error = %v", err)
		}
		if _, err := store.ListForHost(context.Background(), "github.com"); !errors.Is(err, readErr) {
			t.Fatalf("ListForHost read error = %v", err)
		}
		if err := store.BindRepository(context.Background(), "github.com", "acme", "governance", "public-client-id"); !errors.Is(err, readErr) {
			t.Fatalf("BindRepository read error = %v", err)
		}
	})
}

func TestDPAPISessionStoreScopedFailurePaths(t *testing.T) {
	t.Run("rejects incomplete and mismatched scoped sessions", func(t *testing.T) {
		for _, session := range []Session{
			{
				Host:     "github.com",
				Account:  "octocat",
				ClientID: "tenant-a-client-id",
			},
			{
				Host:                  "github.com",
				Account:               "octocat",
				ClientID:              "other-client-id",
				RefreshToken:          "ghr-other",
				RefreshTokenExpiresAt: time.Date(2026, time.December, 31, 23, 59, 59, 0, time.UTC),
			},
		} {
			session := session
			t.Run(session.ClientID, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "sessions.dpapi")
				document := emptySessionDocument()
				document.ActiveByScope[sessionScopeKey("github.com", "tenant-a-client-id")] = "octocat"
				document.Sessions[sessionKey("github.com", "octocat", "tenant-a-client-id")] = session
				encrypted, err := protectDPAPI(mustJSON(t, document))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, encrypted, 0o600); err != nil {
					t.Fatal(err)
				}
				if _, err := newDPAPISessionStore(path).LoadActive(context.Background(), "github.com", "tenant-a-client-id"); err == nil {
					t.Fatal("LoadActive accepted an invalid scoped session")
				}
			})
		}
	})

	t.Run("replaces only the active account for one client scope", func(t *testing.T) {
		store := newDPAPISessionStore(filepath.Join(t.TempDir(), "sessions.dpapi"))
		first := testStoredSession("github.com", "octocat")
		first.ClientID = "tenant-a-client-id"
		second := testStoredSession("github.com", "hubot")
		second.ClientID = first.ClientID
		if err := store.SaveActive(context.Background(), first); err != nil {
			t.Fatal(err)
		}
		if err := store.SaveActive(context.Background(), second); err != nil {
			t.Fatal(err)
		}
		loaded, err := store.LoadActive(context.Background(), first.Host, first.ClientID)
		if err != nil || loaded != second {
			t.Fatalf("active account after replacement = (%#v, %v)", loaded, err)
		}
		document, err := store.load()
		if err != nil {
			t.Fatal(err)
		}
		if _, found := document.Sessions[sessionKey(first.Host, first.Account, first.ClientID)]; found {
			t.Fatal("replaced account refresh session remained in the client scope")
		}
	})

	t.Run("initializes absent version-two maps and rejects every other schema", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "sessions.dpapi")
		store := newDPAPISessionStore(path)
		for _, document := range []sessionDocument{
			{SchemaVersion: sessionStoreSchemaVersion},
			{SchemaVersion: 3},
		} {
			encrypted, err := protectDPAPI(mustJSON(t, document))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, encrypted, 0o600); err != nil {
				t.Fatal(err)
			}
			loaded, err := store.load()
			if document.SchemaVersion == sessionStoreSchemaVersion {
				if err != nil || loaded.ActiveByScope == nil || loaded.Sessions == nil || loaded.ScopeByRepository == nil {
					t.Fatalf("load initialized version-two document = (%#v, %v)", loaded, err)
				}
				continue
			}
			if err == nil {
				t.Fatalf("load accepted unsupported schema %d", document.SchemaVersion)
			}
		}
	})
}

func TestDPAPISessionStoreHandlesCorruptionCancellationAndHelpers(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sessions.dpapi")
	store := newDPAPISessionStore(path)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.LoadActive(ctx, "github.com", "public-client-id"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled LoadActive() error = %v", err)
	}
	if _, err := store.LoadActiveForHost(ctx, "github.com"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled LoadActiveForHost() error = %v", err)
	}
	if err := store.SaveActive(ctx, testStoredSession("github.com", "octocat")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled SaveActive() error = %v", err)
	}
	if err := store.DeleteActive(ctx, "github.com", "public-client-id"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled DeleteActive() error = %v", err)
	}
	if contextFailure(testNilContext()) != nil || contextFailure(context.Background()) != nil {
		t.Fatal("context helper reported an active context as failed")
	}

	if err := os.WriteFile(path, []byte("not DPAPI data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.load(); err == nil {
		t.Fatal("load accepted an undecryptable session")
	}

	invalidJSON, err := protectDPAPI([]byte("{"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, invalidJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.load(); err == nil {
		t.Fatal("load accepted invalid JSON")
	}

	unsupported, err := protectDPAPI([]byte(`{"schemaVersion":3}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, unsupported, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.load(); err == nil {
		t.Fatal("load accepted an unsupported schema")
	}

	emptyMaps, err := protectDPAPI([]byte(`{"schemaVersion":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, emptyMaps, 0o600); err != nil {
		t.Fatal(err)
	}
	document, err := store.load()
	if err != nil || document.ActiveByScope == nil || document.Sessions == nil || document.ScopeByRepository == nil {
		t.Fatalf("load initialized nil maps = (%#v, %v)", document, err)
	}

	plain := []byte("refresh-session")
	protected, err := protectDPAPI(plain)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := unprotectDPAPI(protected)
	if err != nil || string(roundTrip) != string(plain) {
		t.Fatalf("DPAPI round trip = (%q, %v)", roundTrip, err)
	}
	if _, err := protectDPAPI(nil); err == nil {
		t.Fatal("protectDPAPI accepted empty data")
	}
	if _, err := unprotectDPAPI(nil); err == nil {
		t.Fatal("unprotectDPAPI accepted empty data")
	}
	freeDPAPIBlob(nil)
	if normalizeHost(" GitHub.COM ") != "github.com" {
		t.Fatal("normalizeHost did not canonicalize the host")
	}
	if document := emptySessionDocument(); document.SchemaVersion != sessionStoreSchemaVersion ||
		len(document.ActiveByScope) != 0 || len(document.Sessions) != 0 || len(document.ScopeByRepository) != 0 {
		t.Fatalf("empty session document = %#v", document)
	}
	if path, err := defaultDPAPISessionStorePath(); err != nil || !strings.HasSuffix(path, "github-app-sessions.dpapi") {
		t.Fatalf("default session path = (%q, %v)", path, err)
	}
}

func TestDPAPISessionStoreReportsFilesystemAndIndexFailures(t *testing.T) {
	pathErr := errors.New("path unavailable")
	store := &dpapiSessionStore{
		path: func() (string, error) {
			return "", pathErr
		},
	}
	if _, err := store.load(); !errors.Is(err, pathErr) {
		t.Fatalf("load path error = %v", err)
	}
	if err := store.save(emptySessionDocument()); !errors.Is(err, pathErr) {
		t.Fatalf("save path error = %v", err)
	}

	root := t.TempDir()
	path := filepath.Join(root, "sessions.dpapi")
	store = newDPAPISessionStore(path)
	document := emptySessionDocument()
	document.ActiveByScope[sessionScopeKey("github.com", "public-client-id")] = "missing"
	encrypted, err := protectDPAPI(mustJSON(t, document))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encrypted, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadActive(context.Background(), "github.com", "public-client-id"); err == nil {
		t.Fatal("LoadActive accepted an inconsistent active session index")
	}

	directoryPath := filepath.Join(root, "session-directory")
	if err := os.Mkdir(directoryPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directoryPath, "keep"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	directoryStore := newDPAPISessionStore(directoryPath)
	if err := directoryStore.save(emptySessionDocument()); err == nil {
		t.Fatal("save accepted a directory in place of the session file")
	}
	if err := directoryStore.save(sessionDocument{
		SchemaVersion: sessionStoreSchemaVersion,
		ActiveByScope: map[string]string{sessionScopeKey("github.com", "public-client-id"): "octocat"},
		Sessions:      map[string]Session{sessionKey("github.com", "octocat", "public-client-id"): testStoredSession("github.com", "octocat")},
	}); err == nil {
		t.Fatal("save replaced a directory as a session file")
	}
}

func TestDPAPISessionStoreInjectableFailurePaths(t *testing.T) {
	document := sessionDocument{
		SchemaVersion: sessionStoreSchemaVersion,
		ActiveByScope: map[string]string{sessionScopeKey("github.com", "public-client-id"): "octocat"},
		Sessions: map[string]Session{
			sessionKey("github.com", "octocat", "public-client-id"): testStoredSession("github.com", "octocat"),
		},
	}

	t.Run("propagates read failures through every operation", func(t *testing.T) {
		preserveWindowsStoreHooks(t)
		readErr := errors.New("read failed")
		readSessionFile = func(string) ([]byte, error) {
			return nil, readErr
		}
		store := newDPAPISessionStore("session.dpapi")
		if _, err := store.LoadActive(context.Background(), "github.com", "public-client-id"); !errors.Is(err, readErr) {
			t.Fatalf("LoadActive read error = %v", err)
		}
		if _, err := store.LoadActiveForHost(context.Background(), "github.com"); !errors.Is(err, readErr) {
			t.Fatalf("LoadActiveForHost read error = %v", err)
		}
		if err := store.SaveActive(context.Background(), testStoredSession("github.com", "octocat")); !errors.Is(err, readErr) {
			t.Fatalf("SaveActive read error = %v", err)
		}
		if err := store.DeleteActive(context.Background(), "github.com", "public-client-id"); !errors.Is(err, readErr) {
			t.Fatalf("DeleteActive read error = %v", err)
		}
	})

	t.Run("reports user configuration directory failures", func(t *testing.T) {
		preserveWindowsStoreHooks(t)
		expected := errors.New("user config unavailable")
		userConfigDirectory = func() (string, error) {
			return "", expected
		}
		if _, err := defaultDPAPISessionStorePath(); !errors.Is(err, expected) {
			t.Fatalf("defaultDPAPISessionStorePath() error = %v", err)
		}
	})

	t.Run("reports DPAPI protection failures", func(t *testing.T) {
		preserveWindowsStoreHooks(t)
		expected := errors.New("protect failed")
		cryptProtectData = func(
			*windows.DataBlob,
			*uint16,
			*windows.DataBlob,
			uintptr,
			*windows.CryptProtectPromptStruct,
			uint32,
			*windows.DataBlob,
		) error {
			return expected
		}
		if _, err := protectDPAPI([]byte("plaintext")); !errors.Is(err, expected) {
			t.Fatalf("protectDPAPI() error = %v", err)
		}
	})

	t.Run("reports DPAPI unprotection failures", func(t *testing.T) {
		preserveWindowsStoreHooks(t)
		expected := errors.New("unprotect failed")
		cryptUnprotectData = func(
			*windows.DataBlob,
			**uint16,
			*windows.DataBlob,
			uintptr,
			*windows.CryptProtectPromptStruct,
			uint32,
			*windows.DataBlob,
		) error {
			return expected
		}
		if _, err := unprotectDPAPI([]byte("protected")); !errors.Is(err, expected) {
			t.Fatalf("unprotectDPAPI() error = %v", err)
		}
	})

	t.Run("reports filesystem write failures", func(t *testing.T) {
		for _, testCase := range []struct {
			name      string
			configure func()
		}{
			{
				name: "remove",
				configure: func() {
					removeSessionFile = func(string) error {
						return errors.New("remove failed")
					}
				},
			},
			{
				name: "mkdir",
				configure: func() {
					makeSessionDir = func(string, os.FileMode) error {
						return errors.New("mkdir failed")
					}
				},
			},
			{
				name: "encrypt",
				configure: func() {
					cryptProtectData = func(
						*windows.DataBlob,
						*uint16,
						*windows.DataBlob,
						uintptr,
						*windows.CryptProtectPromptStruct,
						uint32,
						*windows.DataBlob,
					) error {
						return errors.New("encrypt failed")
					}
				},
			},
			{
				name: "create temporary file",
				configure: func() {
					createSessionTemp = func(string, string) (sessionTemporaryFile, error) {
						return nil, errors.New("create failed")
					}
				},
			},
			{
				name: "chmod",
				configure: func() {
					createSessionTemp = func(string, string) (sessionTemporaryFile, error) {
						return fakeSessionTemporaryFile{chmodErr: errors.New("chmod failed")}, nil
					}
				},
			},
			{
				name: "write",
				configure: func() {
					createSessionTemp = func(string, string) (sessionTemporaryFile, error) {
						return fakeSessionTemporaryFile{writeErr: errors.New("write failed")}, nil
					}
				},
			},
			{
				name: "close",
				configure: func() {
					createSessionTemp = func(string, string) (sessionTemporaryFile, error) {
						return fakeSessionTemporaryFile{closeErr: errors.New("close failed")}, nil
					}
				},
			},
			{
				name: "rename",
				configure: func() {
					renameSessionFile = func(string, string) error {
						return errors.New("rename failed")
					}
				},
			},
		} {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				preserveWindowsStoreHooks(t)
				makeSessionDir = func(string, os.FileMode) error { return nil }
				createSessionTemp = func(string, string) (sessionTemporaryFile, error) {
					return fakeSessionTemporaryFile{}, nil
				}
				renameSessionFile = func(string, string) error { return nil }
				removeSessionFile = func(string) error { return nil }
				testCase.configure()
				store := newDPAPISessionStore("session.dpapi")
				if testCase.name == "remove" {
					if err := store.save(emptySessionDocument()); err == nil {
						t.Fatal("save unexpectedly succeeded")
					}
					return
				}
				if err := store.save(document); err == nil {
					t.Fatal("save unexpectedly succeeded")
				}
			})
		}
	})
}

type fakeSessionTemporaryFile struct {
	chmodErr error
	writeErr error
	closeErr error
}

func (file fakeSessionTemporaryFile) Name() string {
	return "temporary.dpapi"
}

func (file fakeSessionTemporaryFile) Chmod(os.FileMode) error {
	return file.chmodErr
}

func (file fakeSessionTemporaryFile) Write(value []byte) (int, error) {
	if file.writeErr != nil {
		return 0, file.writeErr
	}
	return len(value), nil
}

func (file fakeSessionTemporaryFile) Close() error {
	return file.closeErr
}

func preserveWindowsStoreHooks(t *testing.T) {
	t.Helper()
	originalUserConfigDirectory := userConfigDirectory
	originalReadSessionFile := readSessionFile
	originalRemoveSessionFile := removeSessionFile
	originalMakeSessionDir := makeSessionDir
	originalCreateSessionTemp := createSessionTemp
	originalRenameSessionFile := renameSessionFile
	originalCryptProtectData := cryptProtectData
	originalCryptUnprotectData := cryptUnprotectData
	originalFreeLocalMemory := freeLocalMemory
	t.Cleanup(func() {
		userConfigDirectory = originalUserConfigDirectory
		readSessionFile = originalReadSessionFile
		removeSessionFile = originalRemoveSessionFile
		makeSessionDir = originalMakeSessionDir
		createSessionTemp = originalCreateSessionTemp
		renameSessionFile = originalRenameSessionFile
		cryptProtectData = originalCryptProtectData
		cryptUnprotectData = originalCryptUnprotectData
		freeLocalMemory = originalFreeLocalMemory
	})
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
