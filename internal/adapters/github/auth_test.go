package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestAuthServiceLoginPersistsOnlyRefreshSession(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	store := &memorySessionStore{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/login/device/code":
			assertFormValue(t, request, "client_id", "public-client-id")
			writeJSON(t, writer, deviceCodeResponse{
				DeviceCode:      "device-code-secret",
				UserCode:        "ABCD-EFGH",
				VerificationURI: "https://github.com/login/device",
				ExpiresIn:       900,
				Interval:        1,
			})
		case "/login/oauth/access_token":
			assertFormValue(t, request, "client_id", "public-client-id")
			assertFormValue(t, request, "device_code", "device-code-secret")
			assertFormValue(t, request, "grant_type", "urn:ietf:params:oauth:grant-type:device_code")
			writeJSON(t, writer, tokenResponse{
				AccessToken:           "ghu-access-secret",
				ExpiresIn:             28800,
				RefreshToken:          "ghr-refresh-secret",
				RefreshTokenExpiresIn: 15897600,
				TokenType:             "bearer",
			})
		case "/user":
			if got := request.Header.Get("Authorization"); got != "Bearer ghu-access-secret" {
				t.Fatalf("user authorization = %q", got)
			}
			writeJSON(t, writer, userResponse{Login: "octocat"})
		default:
			t.Fatalf("unexpected request path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	var instructions []DeviceAuthorization
	service := newTestAuthService(t, AuthOptions{
		Store:        store,
		OAuthBaseURL: server.URL,
		APIBaseURL:   server.URL,
		HTTPClient:   server.Client(),
		Now:          func() time.Time { return now },
	})
	status, err := service.Login(context.Background(), LoginRequest{
		ClientID: "public-client-id",
		OnDeviceAuthorization: func(device DeviceAuthorization) error {
			instructions = append(instructions, device)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if status.Host != defaultGitHubHost || status.Account != "octocat" ||
		status.Source != secretStoreSourceLabel || status.RefreshState != "active" {
		t.Fatalf("Login() status = %#v", status)
	}
	if len(instructions) != 1 || instructions[0].UserCode != "ABCD-EFGH" ||
		instructions[0].VerificationURI != "https://github.com/login/device" ||
		!instructions[0].ExpiresAt.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("device instructions = %#v", instructions)
	}
	if store.session.Host != defaultGitHubHost || store.session.Account != "octocat" ||
		store.session.ClientID != "public-client-id" || store.session.RefreshToken != "ghr-refresh-secret" {
		t.Fatalf("stored session = %#v", store.session)
	}
	if !store.session.RefreshTokenExpiresAt.Equal(now.Add(15897600 * time.Second)) {
		t.Fatalf("refresh expiration = %s", store.session.RefreshTokenExpiresAt)
	}
	if got := statusJSON(t, status); strings.Contains(got, "ghu-access-secret") ||
		strings.Contains(got, "ghr-refresh-secret") || strings.Contains(got, "device-code-secret") {
		t.Fatalf("session status contains secret data: %s", got)
	}
}

func TestAuthServiceLoginHandlesDevicePollingAndFailures(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	t.Run("waits for pending authorization and slows down", func(t *testing.T) {
		store := &memorySessionStore{}
		var tokenCalls int
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/login/device/code":
				writeJSON(t, writer, deviceCodeResponse{
					DeviceCode:      "device",
					UserCode:        "CODE-CODE",
					VerificationURI: "https://github.com/login/device",
					ExpiresIn:       900,
					Interval:        2,
				})
			case "/login/oauth/access_token":
				tokenCalls++
				switch tokenCalls {
				case 1:
					writeJSON(t, writer, tokenResponse{Error: "authorization_pending"})
				case 2:
					writeJSON(t, writer, tokenResponse{Error: "slow_down"})
				default:
					writeJSON(t, writer, tokenResponse{
						AccessToken:           "ghu-access",
						ExpiresIn:             10,
						RefreshToken:          "ghr-refresh",
						RefreshTokenExpiresIn: 20,
						TokenType:             "bearer",
					})
				}
			case "/user":
				writeJSON(t, writer, userResponse{Login: "octocat"})
			default:
				t.Fatalf("unexpected path %q", request.URL.Path)
			}
		}))
		defer server.Close()

		var waits []time.Duration
		service := newTestAuthService(t, AuthOptions{
			Store:        store,
			OAuthBaseURL: server.URL,
			APIBaseURL:   server.URL,
			HTTPClient:   server.Client(),
			Now:          func() time.Time { return now },
			Wait: func(context.Context, time.Duration) error {
				waits = append(waits, time.Duration(0))
				return nil
			},
		})
		// Preserve the requested delay values without allowing the test to sleep.
		service.wait = func(_ context.Context, duration time.Duration) error {
			waits = append(waits, duration)
			return nil
		}
		if _, err := service.Login(context.Background(), LoginRequest{ClientID: "public-client-id"}); err != nil {
			t.Fatalf("Login() error = %v", err)
		}
		if tokenCalls != 3 || len(waits) != 2 || waits[0] != 2*time.Second || waits[1] != 7*time.Second {
			t.Fatalf("polling = calls %d, waits %#v", tokenCalls, waits)
		}
	})

	for _, testCase := range []struct {
		name      string
		response  tokenResponse
		now       func() time.Time
		wait      func(context.Context, time.Duration) error
		wantCode  problem.Code
		wantCause error
	}{
		{
			name:     "denied authorization",
			response: tokenResponse{Error: "access_denied"},
			wantCode: problem.CodeConfigurationInvalid,
		},
		{
			name:     "invalid token payload",
			response: tokenResponse{AccessToken: "ghu-only"},
			wantCode: problem.CodeConfigurationInvalid,
		},
		{
			name: "expired device code before first poll",
			now: func() time.Time {
				return now.Add(time.Hour)
			},
			wantCode: problem.CodeConfigurationInvalid,
		},
		{
			name:     "cancelled wait",
			response: tokenResponse{Error: "authorization_pending"},
			wait: func(context.Context, time.Duration) error {
				return context.Canceled
			},
			wantCode:  problem.CodeOperationCancelled,
			wantCause: context.Canceled,
		},
		{
			name:     "wait transport error",
			response: tokenResponse{Error: "authorization_pending"},
			wait: func(context.Context, time.Duration) error {
				return errors.New("timer unavailable")
			},
			wantCode: problem.CodeExternalCommandFailed,
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/login/oauth/access_token" {
					t.Fatalf("path = %q", request.URL.Path)
				}
				writeJSON(t, writer, testCase.response)
			}))
			defer server.Close()
			current := now
			if testCase.now != nil {
				current = testCase.now()
			}
			service := newTestAuthService(t, AuthOptions{
				Store:        &memorySessionStore{},
				OAuthBaseURL: server.URL,
				HTTPClient:   server.Client(),
				Now:          func() time.Time { return current },
				Wait:         testCase.wait,
			})
			if service.wait == nil {
				service.wait = waitForContext
			}
			_, err := service.pollForTokens(context.Background(), "client", DeviceAuthorization{
				ExpiresAt: now.Add(time.Minute),
				Interval:  time.Second,
			}, "device")
			assertAuthProblem(t, err, testCase.wantCode)
			if testCase.wantCause != nil && !errors.Is(err, testCase.wantCause) {
				t.Fatalf("error = %v, want cause %v", err, testCase.wantCause)
			}
		})
	}
}

func TestAuthServiceResolverRefreshesAndAuthorizesExactRepository(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	store := &memorySessionStore{session: Session{
		Host:                  "github.com",
		Account:               "octocat",
		ClientID:              "public-client-id",
		RefreshToken:          "ghr-old-secret",
		RefreshTokenExpiresAt: now.Add(time.Hour),
	}}
	var authorizationTargets []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "" {
			authorizationTargets = append(authorizationTargets, request.Header.Get("Authorization"))
		}
		switch request.URL.Path {
		case "/login/oauth/access_token":
			assertFormValue(t, request, "client_id", "public-client-id")
			assertFormValue(t, request, "grant_type", "refresh_token")
			assertFormValue(t, request, "refresh_token", "ghr-old-secret")
			writeJSON(t, writer, tokenResponse{
				AccessToken:           "ghu-new-secret",
				ExpiresIn:             3600,
				RefreshToken:          "ghr-new-secret",
				RefreshTokenExpiresIn: 7200,
				TokenType:             "bearer",
			})
		case "/user/installations":
			writeJSON(t, writer, installationsResponse{Installations: []installationResponse{{ID: 41}}})
		case "/user/installations/41/repositories":
			writeJSON(t, writer, installationRepositoriesResponse{
				TotalCount: 1,
				Repositories: []repositoryResponse{
					{FullName: "acme/governance"},
				},
			})
		default:
			t.Fatalf("unexpected request %s", request.URL.String())
		}
	}))
	defer server.Close()

	service := newTestAuthService(t, AuthOptions{
		Store:        store,
		OAuthBaseURL: server.URL,
		APIBaseURL:   server.URL,
		HTTPClient:   server.Client(),
		Now:          func() time.Time { return now },
	})
	target := CredentialTarget{Host: "github.com", Owner: "acme", Repository: "governance"}
	token, err := service.Resolve(context.Background(), target)
	if err != nil || token != "ghu-new-secret" {
		t.Fatalf("Resolve() = (%q, %v)", token, err)
	}
	if store.saveCalls != 1 || store.session.RefreshToken != "ghr-new-secret" ||
		!store.session.RefreshTokenExpiresAt.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("rotated store session = %#v, saves=%d", store.session, store.saveCalls)
	}
	if len(authorizationTargets) != 2 {
		t.Fatalf("GitHub authorization requests = %#v", authorizationTargets)
	}
	for _, header := range authorizationTargets {
		if header != "Bearer ghu-new-secret" {
			t.Fatalf("authorization header = %q", header)
		}
	}

	again, err := service.Resolve(context.Background(), target)
	if err != nil || again != token {
		t.Fatalf("cached Resolve() = (%q, %v)", again, err)
	}
	if store.saveCalls != 1 || len(authorizationTargets) != 2 {
		t.Fatalf("cached resolver refreshed or reauthorized: saves=%d headers=%d", store.saveCalls, len(authorizationTargets))
	}
}

func TestAuthServiceResolverFailsClosedAndRedactsSecrets(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	validSession := Session{
		Host:                  "github.com",
		Account:               "octocat",
		ClientID:              "public-client-id",
		RefreshToken:          "ghr-secret",
		RefreshTokenExpiresAt: now.Add(time.Hour),
	}
	t.Run("rejects host and incomplete target before network calls", func(t *testing.T) {
		service := newTestAuthService(t, AuthOptions{
			Store: &memorySessionStore{session: validSession},
			Now:   func() time.Time { return now },
		})
		for _, target := range []CredentialTarget{
			{Host: "enterprise.example", Owner: "acme", Repository: "governance"},
			{Host: "github.com", Owner: "", Repository: "governance"},
		} {
			_, err := service.Resolve(context.Background(), target)
			assertAuthProblem(t, err, problem.CodeConfigurationInvalid)
		}
	})

	t.Run("requires a stored session", func(t *testing.T) {
		service := newTestAuthService(t, AuthOptions{
			Store: &memorySessionStore{loadErr: errSessionNotFound},
			Now:   func() time.Time { return now },
		})
		_, err := service.Resolve(context.Background(), CredentialTarget{Host: "github.com", Owner: "acme", Repository: "governance"})
		assertAuthProblem(t, err, problem.CodeConfigurationUnavailable)
	})

	t.Run("rejects expired and malformed sessions", func(t *testing.T) {
		expired := validSession
		expired.RefreshTokenExpiresAt = now.Add(-time.Second)
		for _, session := range []Session{expired, {
			Host:     "github.com",
			Account:  "octocat",
			ClientID: "public-client-id",
		}} {
			service := newTestAuthService(t, AuthOptions{
				Store: &memorySessionStore{session: session},
				Now:   func() time.Time { return now },
			})
			_, err := service.Resolve(context.Background(), CredentialTarget{Host: "github.com", Owner: "acme", Repository: "governance"})
			assertAuthProblem(t, err, problem.CodeConfigurationInvalid)
		}
	})

	t.Run("rejects repository not installed for the active session", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/login/oauth/access_token":
				writeJSON(t, writer, tokenResponse{
					AccessToken:           "ghu-secret",
					ExpiresIn:             3600,
					RefreshToken:          "ghr-rotated-secret",
					RefreshTokenExpiresIn: 7200,
					TokenType:             "bearer",
				})
			case "/user/installations":
				writeJSON(t, writer, installationsResponse{Installations: []installationResponse{{ID: 1}}})
			case "/user/installations/1/repositories":
				writeJSON(t, writer, installationRepositoriesResponse{
					TotalCount:   1,
					Repositories: []repositoryResponse{{FullName: "other/repository"}},
				})
			default:
				t.Fatalf("path = %q", request.URL.Path)
			}
		}))
		defer server.Close()
		service := newTestAuthService(t, AuthOptions{
			Store:        &memorySessionStore{session: validSession},
			OAuthBaseURL: server.URL,
			APIBaseURL:   server.URL,
			HTTPClient:   server.Client(),
			Now:          func() time.Time { return now },
		})
		_, err := service.Resolve(context.Background(), CredentialTarget{Host: "github.com", Owner: "acme", Repository: "governance"})
		assertAuthProblem(t, err, problem.CodeConfigurationInvalid)
		if rendered := err.Error(); strings.Contains(rendered, "ghu-secret") || strings.Contains(rendered, "ghr-secret") {
			t.Fatalf("resolver error leaked a token: %q", rendered)
		}
	})
}

func TestAuthServiceRefreshesExactlyOncePerProfile(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	store := &memorySessionStore{session: Session{
		Host:                  "github.com",
		Account:               "octocat",
		ClientID:              "public-client-id",
		RefreshToken:          "ghr-refresh",
		RefreshTokenExpiresAt: now.Add(time.Hour),
	}}
	var refreshCalls atomic.Int32
	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/login/oauth/access_token":
			if refreshCalls.Add(1) == 1 {
				close(refreshStarted)
				<-releaseRefresh
			}
			writeJSON(t, writer, tokenResponse{
				AccessToken:           "ghu-access",
				ExpiresIn:             3600,
				RefreshToken:          "ghr-next",
				RefreshTokenExpiresIn: 7200,
				TokenType:             "bearer",
			})
		case "/user/installations":
			writeJSON(t, writer, installationsResponse{Installations: []installationResponse{{ID: 1}}})
		case "/user/installations/1/repositories":
			writeJSON(t, writer, installationRepositoriesResponse{
				TotalCount:   1,
				Repositories: []repositoryResponse{{FullName: "acme/governance"}},
			})
		default:
			t.Fatalf("path = %q", request.URL.Path)
		}
	}))
	defer server.Close()
	service := newTestAuthService(t, AuthOptions{
		Store:        store,
		OAuthBaseURL: server.URL,
		APIBaseURL:   server.URL,
		HTTPClient:   server.Client(),
		Now:          func() time.Time { return now },
	})
	target := CredentialTarget{Host: "github.com", Owner: "acme", Repository: "governance"}
	errorsChannel := make(chan error, 2)
	go func() {
		_, err := service.Resolve(context.Background(), target)
		errorsChannel <- err
	}()
	<-refreshStarted
	go func() {
		_, err := service.Resolve(context.Background(), target)
		errorsChannel <- err
	}()
	close(releaseRefresh)
	for range 2 {
		if err := <-errorsChannel; err != nil {
			t.Fatalf("concurrent Resolve() error = %v", err)
		}
	}
	if refreshCalls.Load() != 1 {
		t.Fatalf("refresh calls = %d, want 1", refreshCalls.Load())
	}
}

func TestAuthStatusLogoutAndUtilityContracts(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	session := Session{
		Host:                  "github.com",
		Account:               "octocat",
		ClientID:              "public-client-id",
		RefreshToken:          "ghr-secret",
		RefreshTokenExpiresAt: now.Add(time.Hour),
	}
	store := &memorySessionStore{session: session}
	service := newTestAuthService(t, AuthOptions{
		Store: store,
		Now:   func() time.Time { return now },
	})
	status, err := service.Status(context.Background())
	if err != nil || status.RefreshState != "active" || status.Account != "octocat" {
		t.Fatalf("Status() = (%#v, %v)", status, err)
	}
	removed, err := service.Logout(context.Background())
	if err != nil || removed.Account != "octocat" || store.deleteCalls != 1 {
		t.Fatalf("Logout() = (%#v, %v), deletes=%d", removed, err, store.deleteCalls)
	}
	_, err = service.Status(context.Background())
	assertAuthProblem(t, err, problem.CodeConfigurationUnavailable)

	expired := sessionStatus(Session{RefreshTokenExpiresAt: now.Add(-time.Second)}, now)
	if expired.RefreshState != "expired" {
		t.Fatalf("expired session status = %#v", expired)
	}
	if !tokenUsable(cachedToken{value: "token", expiresAt: now.Add(2 * time.Minute)}, now) ||
		tokenUsable(cachedToken{value: "token", expiresAt: now.Add(credentialExpirySkew)}, now) ||
		tokenUsable(cachedToken{}, now) {
		t.Fatal("token usability boundaries are wrong")
	}
	if sessionKey("GitHub.COM", "OctoCat", "public-client-id") != "github.com\x00octocat\x00public-client-id" {
		t.Fatal("session key was not normalized")
	}
	for _, raw := range []string{"", "http://github.com", "https://user@github.com", "https://github.com"} {
		if raw == "https://github.com" {
			continue
		}
		if _, err := joinHTTPSURL(raw, "/path"); err == nil {
			t.Fatalf("joinHTTPSURL(%q) unexpectedly succeeded", raw)
		}
	}
	if joined, err := joinHTTPSURL("https://github.com/base/", "/path?x=y"); err != nil ||
		joined != "https://github.com/base/path?x=y" {
		t.Fatalf("joinHTTPSURL() = (%q, %v)", joined, err)
	}
	for _, path := range []string{"https://attacker.example/path", "://bad"} {
		if _, err := joinHTTPSURL("https://github.com", path); err == nil {
			t.Fatalf("joinHTTPSURL accepted invalid path %q", path)
		}
	}
}

func TestAuthServiceResolvesTheHostScopedActiveSession(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	tenantA := testStoredSession("github.com", "octocat")
	tenantA.ClientID = "tenant-a-client-id"
	tenantA.RefreshToken = "ghr-tenant-a-refresh"
	platform := testStoredSession("github.com", "octocat")
	platform.ClientID = "platform-client-id"
	platform.RefreshToken = "ghr-platform-refresh"

	store := &memorySessionStore{}
	if err := store.SaveActive(context.Background(), tenantA); err != nil {
		t.Fatalf("SaveActive(tenant A) error = %v", err)
	}
	if err := store.SaveActive(context.Background(), platform); err != nil {
		t.Fatalf("SaveActive(platform) error = %v", err)
	}

	service := newTestAuthService(t, AuthOptions{
		Store: store,
		Now:   func() time.Time { return now },
	})

	// The latest login becomes the active session for the host; no ambient
	// client ID is consulted.
	status, err := service.Status(context.Background())
	if err != nil || status.Account != platform.Account {
		t.Fatalf("host-scoped Status() = (%#v, %v)", status, err)
	}
	if _, err := service.Logout(context.Background()); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	// Logging out the active session clears the host pointer even though the
	// replaced scope record remains stored; resolution fails closed until the
	// next explicit login.
	_, err = service.Status(context.Background())
	assertAuthProblem(t, err, problem.CodeConfigurationUnavailable)
}

func TestAuthServiceFailsClosedWithoutActiveSession(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	store := &memorySessionStore{}
	service := newTestAuthService(t, AuthOptions{
		Store: store,
		Now:   func() time.Time { return now },
	})
	_, err := service.Status(context.Background())
	assertAuthProblem(t, err, problem.CodeConfigurationUnavailable)
	_, err = service.Logout(context.Background())
	assertAuthProblem(t, err, problem.CodeConfigurationUnavailable)
	_, err = service.Resolve(context.Background(), CredentialTarget{
		Host:       "github.com",
		Owner:      "acme",
		Repository: "governance",
	})
	assertAuthProblem(t, err, problem.CodeConfigurationUnavailable)
	if store.saveCalls != 0 {
		t.Fatalf("failed resolution persisted a session: saves=%d", store.saveCalls)
	}

	_, err = service.Login(context.Background(), LoginRequest{})
	assertAuthProblem(t, err, problem.CodeConfigurationInvalid)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = service.Logout(cancelled)
	assertAuthProblem(t, err, problem.CodeOperationCancelled)

	if first, second := nativeSessionScope("github.com", "tenant-a-client-id"), nativeSessionScope("github.com", "platform-client-id"); first == second ||
		!strings.HasPrefix(first, "github.com.") || strings.Contains(first, "tenant-a-client-id") {
		t.Fatalf("native session scopes = (%q, %q)", first, second)
	}
}

func TestAuthServiceClassifiesOAuthAndContextFailures(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	service := newTestAuthService(t, AuthOptions{Store: &memorySessionStore{}})
	for _, call := range []func() error{
		func() error {
			_, err := service.Login(cancelled, LoginRequest{})
			return err
		},
		func() error {
			_, err := service.Status(cancelled)
			return err
		},
		func() error {
			_, err := service.Resolve(cancelled, CredentialTarget{})
			return err
		},
	} {
		assertAuthProblem(t, call(), problem.CodeOperationCancelled)
	}
	_, err := service.Status(testNilContext())
	assertAuthProblem(t, err, problem.CodeInvalidInput)

	t.Run("callback error is preserved", func(t *testing.T) {
		callbackErr := errors.New("browser unavailable")
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writeJSON(t, writer, deviceCodeResponse{
				DeviceCode:      "device",
				UserCode:        "CODE-CODE",
				VerificationURI: "https://github.com/login/device",
				ExpiresIn:       900,
			})
		}))
		defer server.Close()
		service := newTestAuthService(t, AuthOptions{
			Store:        &memorySessionStore{},
			OAuthBaseURL: server.URL,
			HTTPClient:   server.Client(),
		})
		_, err := service.Login(context.Background(), LoginRequest{
			ClientID: "public-client-id",
			OnDeviceAuthorization: func(DeviceAuthorization) error {
				return callbackErr
			},
		})
		if !errors.Is(err, callbackErr) {
			t.Fatalf("Login callback error = %v, want %v", err, callbackErr)
		}
	})

	t.Run("malformed and unsuccessful HTTP responses are typed and redacted", func(t *testing.T) {
		for _, response := range []struct {
			name   string
			status int
			body   string
		}{
			{name: "HTTP failure", status: http.StatusUnauthorized, body: `{"error":"token ghu-secret"}`},
			{name: "malformed JSON", status: http.StatusOK, body: "{"},
			{name: "oversized JSON", status: http.StatusOK, body: strings.Repeat("x", maxOAuthResponseBytes+1)},
		} {
			response := response
			t.Run(response.name, func(t *testing.T) {
				server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					writer.WriteHeader(response.status)
					_, _ = io.WriteString(writer, response.body)
				}))
				defer server.Close()
				service := newTestAuthService(t, AuthOptions{
					Store:        &memorySessionStore{},
					OAuthBaseURL: server.URL,
					HTTPClient:   server.Client(),
				})
				_, _, err := service.requestDeviceAuthorization(context.Background(), "client")
				assertAuthProblem(t, err, problem.CodeConfigurationInvalid)
				if strings.Contains(err.Error(), "ghu-secret") {
					t.Fatalf("OAuth error leaked a secret: %v", err)
				}
			})
		}
	})

	t.Run("network failure preserves its cause", func(t *testing.T) {
		networkErr := errors.New("network unreachable")
		service := newTestAuthService(t, AuthOptions{
			Store: &memorySessionStore{},
			HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, networkErr
			})},
		})
		_, _, err := service.requestDeviceAuthorization(context.Background(), "client")
		assertAuthProblem(t, err, problem.CodeExternalCommandFailed)
		if !errors.Is(err, networkErr) {
			t.Fatalf("network error = %v, want %v", err, networkErr)
		}
	})
}

func TestAuthServiceWhiteboxFailurePaths(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	defaults := NewAuthService(AuthOptions{})
	if defaults.store == nil || defaults.host != defaultGitHubHost ||
		defaults.oauthBaseURL != defaultOAuthBaseURL || defaults.apiBaseURL != defaultAPIBaseURL ||
		defaults.client == nil || defaults.now == nil || defaults.wait == nil {
		t.Fatalf("default AuthService = %#v", defaults)
	}
	custom := NewAuthService(AuthOptions{
		Store:        &memorySessionStore{},
		Host:         "github.example",
		OAuthBaseURL: "https://oauth.example/",
		APIBaseURL:   "https://api.example/",
		HTTPClient:   &http.Client{},
		Now:          func() time.Time { return now },
		Wait:         func(context.Context, time.Duration) error { return nil },
	})
	if custom.host != "github.example" || custom.oauthBaseURL != "https://oauth.example" ||
		custom.apiBaseURL != "https://api.example" {
		t.Fatalf("custom AuthService = %#v", custom)
	}
	if clientID, err := validateClientID(" client "); err != nil || clientID != "client" {
		t.Fatalf("validateClientID() = (%q, %v)", clientID, err)
	}
	_, err := validateClientID(" ")
	assertAuthProblem(t, err, problem.CodeConfigurationInvalid)
	assertAuthProblem(t, oauthEndpointProblem(errors.New("bad endpoint")), problem.CodeConfigurationInvalid)
	assertAuthProblem(t, sessionStoreProblem("load", errors.New("vault broken")), problem.CodeConfigurationUnavailable)
	assertAuthProblem(t, sessionStoreProblem("load", errSessionNotFound), problem.CodeConfigurationUnavailable)
	if err := waitForContext(context.Background(), 0); err != nil {
		t.Fatalf("waitForContext() error = %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitForContext(cancelled, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled waitForContext() error = %v", err)
	}

	t.Run("login returns every dependency failure", func(t *testing.T) {
		_, err := custom.Login(context.Background(), LoginRequest{})
		assertAuthProblem(t, err, problem.CodeConfigurationInvalid)

		badEndpoint := NewAuthService(AuthOptions{
			Store:        &memorySessionStore{},
			OAuthBaseURL: "http://not-https",
			Now:          func() time.Time { return now },
		})
		_, err = badEndpoint.Login(context.Background(), LoginRequest{ClientID: "public-client-id"})
		assertAuthProblem(t, err, problem.CodeConfigurationInvalid)

		store := &memorySessionStore{saveErr: errors.New("vault unavailable")}
		server := newOAuthServer(t, func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/login/device/code":
				writeJSON(t, writer, validDeviceCodeResponse())
			case "/login/oauth/access_token":
				writeJSON(t, writer, validTokenResponse())
			case "/user":
				writeJSON(t, writer, userResponse{Login: "octocat"})
			default:
				t.Fatalf("path = %q", request.URL.Path)
			}
		})
		defer server.Close()
		service := newTestAuthService(t, AuthOptions{
			Store:        store,
			OAuthBaseURL: server.URL,
			APIBaseURL:   server.URL,
			HTTPClient:   server.Client(),
			Now:          func() time.Time { return now },
		})
		_, err = service.Login(context.Background(), LoginRequest{ClientID: "public-client-id"})
		assertAuthProblem(t, err, problem.CodeConfigurationUnavailable)

		for _, testCase := range []struct {
			name    string
			token   tokenResponse
			account userResponse
		}{
			{name: "polling failure", token: tokenResponse{Error: "access_denied"}},
			{name: "account lookup failure", token: validTokenResponse(), account: userResponse{}},
		} {
			testCase := testCase
			server := newOAuthServer(t, func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/login/device/code":
					writeJSON(t, writer, validDeviceCodeResponse())
				case "/login/oauth/access_token":
					writeJSON(t, writer, testCase.token)
				case "/user":
					writeJSON(t, writer, testCase.account)
				default:
					t.Fatalf("path = %q", request.URL.Path)
				}
			})
			service := newTestAuthService(t, AuthOptions{
				Store:        &memorySessionStore{},
				OAuthBaseURL: server.URL,
				APIBaseURL:   server.URL,
				HTTPClient:   server.Client(),
				Now:          func() time.Time { return now },
			})
			_, err := service.Login(context.Background(), LoginRequest{ClientID: "public-client-id"})
			assertAuthProblem(t, err, problem.CodeConfigurationInvalid)
			server.Close()
		}
	})

	t.Run("status and logout classify store and session errors", func(t *testing.T) {
		for _, testCase := range []struct {
			store *memorySessionStore
			code  problem.Code
		}{
			{store: &memorySessionStore{loadErr: errors.New("load unavailable")}, code: problem.CodeConfigurationUnavailable},
			{store: &memorySessionStore{session: Session{
				Host:     "github.com",
				Account:  "octocat",
				ClientID: "public-client-id",
			}}, code: problem.CodeConfigurationInvalid},
		} {
			service := newTestAuthService(t, AuthOptions{Store: testCase.store, Now: func() time.Time { return now }})
			_, err := service.Status(context.Background())
			assertAuthProblem(t, err, testCase.code)
		}
		deleteStore := &memorySessionStore{
			session:   testStoredSession("github.com", "octocat"),
			deleteErr: errors.New("delete unavailable"),
		}
		service := newTestAuthService(t, AuthOptions{Store: deleteStore, Now: func() time.Time { return now }})
		_, err := service.Logout(context.Background())
		assertAuthProblem(t, err, problem.CodeConfigurationUnavailable)
		_, err = newTestAuthService(t, AuthOptions{
			Store: &memorySessionStore{loadErr: errors.New("load unavailable")},
			Now:   func() time.Time { return now },
		}).Logout(context.Background())
		assertAuthProblem(t, err, problem.CodeConfigurationUnavailable)
	})

	t.Run("validates every malformed device authorization response", func(t *testing.T) {
		for _, response := range []deviceCodeResponse{
			{Error: "device_flow_disabled"},
			{DeviceCode: "", UserCode: "CODE-CODE", VerificationURI: "https://github.com/login/device", ExpiresIn: 1},
			{DeviceCode: "device", UserCode: "CODE-CODE", VerificationURI: "http://github.com/login/device", ExpiresIn: 1},
			{DeviceCode: "device", UserCode: "CODE-CODE", VerificationURI: "https://github.com/login/device", ExpiresIn: 1},
		} {
			response := response
			server := newOAuthServer(t, func(writer http.ResponseWriter, request *http.Request) {
				writeJSON(t, writer, response)
			})
			service := newTestAuthService(t, AuthOptions{
				Store:        &memorySessionStore{},
				OAuthBaseURL: server.URL,
				HTTPClient:   server.Client(),
				Now:          func() time.Time { return now },
			})
			device, _, err := service.requestDeviceAuthorization(context.Background(), "client")
			if response.Interval == 0 && response.DeviceCode != "" && response.Error == "" &&
				strings.HasPrefix(response.VerificationURI, "https://") {
				if err != nil || device.Interval != defaultDeviceInterval {
					t.Fatalf("default interval result = (%#v, %v)", device, err)
				}
			} else {
				assertAuthProblem(t, err, problem.CodeConfigurationInvalid)
			}
			server.Close()
		}
	})

	t.Run("covers pending refresh errors", func(t *testing.T) {
		session := testStoredSession("github.com", "octocat")
		service := newTestAuthService(t, AuthOptions{Store: &memorySessionStore{}, Now: func() time.Time { return now }})
		key := sessionKey(session.Host, session.Account, session.ClientID)
		pending := &refreshCall{done: make(chan struct{}), err: errors.New("refresh failed")}
		service.refreshing[key] = pending
		close(pending.done)
		_, err := service.accessToken(context.Background(), session)
		if !errors.Is(err, pending.err) {
			t.Fatalf("pending refresh error = %v", err)
		}
		pending = &refreshCall{done: make(chan struct{})}
		service.refreshing[key] = pending
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = service.accessToken(ctx, session)
		assertAuthProblem(t, err, problem.CodeOperationCancelled)
	})

	t.Run("covers slow-down wait failures", func(t *testing.T) {
		server := newOAuthServer(t, func(writer http.ResponseWriter, request *http.Request) {
			writeJSON(t, writer, tokenResponse{Error: "slow_down"})
		})
		defer server.Close()
		service := newTestAuthService(t, AuthOptions{
			Store:        &memorySessionStore{},
			OAuthBaseURL: server.URL,
			HTTPClient:   server.Client(),
			Now:          func() time.Time { return now },
			Wait: func(context.Context, time.Duration) error {
				return context.DeadlineExceeded
			},
		})
		_, err := service.pollForTokens(context.Background(), "client", DeviceAuthorization{
			ExpiresAt: now.Add(time.Minute),
			Interval:  time.Second,
		}, "device")
		assertAuthProblem(t, err, problem.CodeOperationCancelled)
	})

	t.Run("covers refresh rejection and persistence errors", func(t *testing.T) {
		expired := testStoredSession("github.com", "octocat")
		expired.RefreshTokenExpiresAt = now.Add(-time.Second)
		service := newTestAuthService(t, AuthOptions{Store: &memorySessionStore{}, Now: func() time.Time { return now }})
		_, err := service.refresh(context.Background(), expired)
		assertAuthProblem(t, err, problem.CodeConfigurationInvalid)

		for _, response := range []tokenResponse{
			{Error: "bad_refresh_token"},
			{AccessToken: "ghu", ExpiresIn: 1, TokenType: "bearer"},
		} {
			response := response
			server := newOAuthServer(t, func(writer http.ResponseWriter, request *http.Request) {
				writeJSON(t, writer, response)
			})
			service := newTestAuthService(t, AuthOptions{
				Store:        &memorySessionStore{},
				OAuthBaseURL: server.URL,
				HTTPClient:   server.Client(),
				Now:          func() time.Time { return now },
			})
			_, err := service.refresh(context.Background(), testStoredSession("github.com", "octocat"))
			assertAuthProblem(t, err, problem.CodeConfigurationInvalid)
			server.Close()
		}
		server := newOAuthServer(t, func(writer http.ResponseWriter, request *http.Request) {
			writeJSON(t, writer, validTokenResponse())
		})
		defer server.Close()
		service = newTestAuthService(t, AuthOptions{
			Store:        &memorySessionStore{saveErr: errors.New("save unavailable")},
			OAuthBaseURL: server.URL,
			HTTPClient:   server.Client(),
			Now:          func() time.Time { return now },
		})
		_, err = service.refresh(context.Background(), testStoredSession("github.com", "octocat"))
		assertAuthProblem(t, err, problem.CodeConfigurationUnavailable)
		service.oauthBaseURL = "http://invalid"
		_, err = service.refresh(context.Background(), testStoredSession("github.com", "octocat"))
		assertAuthProblem(t, err, problem.CodeConfigurationInvalid)
	})

	t.Run("covers repository pagination, skipped installations, and API failures", func(t *testing.T) {
		var pages []string
		server := newOAuthServer(t, func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/user/installations":
				writeJSON(t, writer, installationsResponse{Installations: []installationResponse{{ID: 0}, {ID: 7}}})
			case "/user/installations/7/repositories":
				pages = append(pages, request.URL.Query().Get("page"))
				if request.URL.Query().Get("page") == "1" {
					repositories := make([]repositoryResponse, 100)
					for index := range repositories {
						repositories[index] = repositoryResponse{FullName: "other/" + strconv.Itoa(index)}
					}
					writeJSON(t, writer, installationRepositoriesResponse{TotalCount: 101, Repositories: repositories})
					return
				}
				writeJSON(t, writer, installationRepositoriesResponse{
					TotalCount:   101,
					Repositories: []repositoryResponse{{FullName: "acme/governance"}},
				})
			default:
				t.Fatalf("path = %q", request.URL.Path)
			}
		})
		defer server.Close()
		service := newTestAuthService(t, AuthOptions{
			Store:      &memorySessionStore{},
			APIBaseURL: server.URL,
			HTTPClient: server.Client(),
		})
		if err := service.repositoryIsInstalledAndAuthorized(
			context.Background(),
			"token",
			CredentialTarget{Host: "github.com", Owner: "acme", Repository: "governance"},
		); err != nil {
			t.Fatalf("repository authorization error = %v", err)
		}
		if strings.Join(pages, ",") != "1,2" {
			t.Fatalf("repository pages = %#v", pages)
		}

		failing := newTestAuthService(t, AuthOptions{
			Store: &memorySessionStore{},
			HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("API unavailable")
			})},
		})
		assertAuthProblem(t, failing.repositoryIsInstalledAndAuthorized(
			context.Background(),
			"token",
			CredentialTarget{Host: "github.com", Owner: "acme", Repository: "governance"},
		), problem.CodeExternalCommandFailed)
		repositoryFailure := newOAuthServer(t, func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/user/installations":
				writeJSON(t, writer, installationsResponse{Installations: []installationResponse{{ID: 1}}})
			default:
				writer.WriteHeader(http.StatusForbidden)
			}
		})
		defer repositoryFailure.Close()
		failing = newTestAuthService(t, AuthOptions{
			Store:      &memorySessionStore{},
			APIBaseURL: repositoryFailure.URL,
			HTTPClient: repositoryFailure.Client(),
		})
		assertAuthProblem(t, failing.repositoryIsInstalledAndAuthorized(
			context.Background(),
			"token",
			CredentialTarget{Host: "github.com", Owner: "acme", Repository: "governance"},
		), problem.CodeConfigurationInvalid)
	})

	t.Run("covers account lookup and low-level API failures", func(t *testing.T) {
		blankAccount := newOAuthServer(t, func(writer http.ResponseWriter, request *http.Request) {
			writeJSON(t, writer, userResponse{})
		})
		service := newTestAuthService(t, AuthOptions{
			Store:      &memorySessionStore{},
			APIBaseURL: blankAccount.URL,
			HTTPClient: blankAccount.Client(),
		})
		_, err := service.lookupAccount(context.Background(), "token")
		assertAuthProblem(t, err, problem.CodeConfigurationInvalid)
		blankAccount.Close()

		failedAccount := newOAuthServer(t, func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(http.StatusUnauthorized)
		})
		service = newTestAuthService(t, AuthOptions{
			Store:      &memorySessionStore{},
			APIBaseURL: failedAccount.URL,
			HTTPClient: failedAccount.Client(),
		})
		_, err = service.lookupAccount(context.Background(), "token")
		assertAuthProblem(t, err, problem.CodeConfigurationInvalid)
		failedAccount.Close()

		service.apiBaseURL = "http://invalid"
		err = service.githubAPIRequest(context.Background(), http.MethodGet, "/user", "token", &userResponse{})
		assertAuthProblem(t, err, problem.CodeConfigurationInvalid)
		service.apiBaseURL = defaultAPIBaseURL
		err = service.githubAPIRequest(context.Background(), "\n", "/user", "token", &userResponse{})
		assertAuthProblem(t, err, problem.CodeConfigurationInvalid)
		err = service.oauthFormRequest(testNilContext(), "/login/device/code", url.Values{}, &deviceCodeResponse{})
		assertAuthProblem(t, err, problem.CodeConfigurationInvalid)
		service.oauthBaseURL = "http://invalid"
		_, err = service.pollForTokens(context.Background(), "client", DeviceAuthorization{
			ExpiresAt: now.Add(time.Minute),
			Interval:  time.Second,
		}, "device")
		assertAuthProblem(t, err, problem.CodeConfigurationInvalid)
	})

	service := newTestAuthService(t, AuthOptions{Store: &memorySessionStore{}, Now: func() time.Time { return now }})
	prefix := sessionKey("github.com", "octocat", "public-client-id")
	service.authorized[prefix+"\x00acme\x00governance"] = now
	service.authorized["other"] = now
	service.dropAuthorizationsForSession(prefix)
	if _, found := service.authorized[prefix+"\x00acme\x00governance"]; found {
		t.Fatal("session authorization cache was not cleared")
	}
	if _, found := service.authorized["other"]; !found {
		t.Fatal("unrelated authorization cache entry was removed")
	}
}

func newOAuthServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(handler)
}

func validDeviceCodeResponse() deviceCodeResponse {
	return deviceCodeResponse{
		DeviceCode:      "device",
		UserCode:        "CODE-CODE",
		VerificationURI: "https://github.com/login/device",
		ExpiresIn:       900,
		Interval:        1,
	}
}

func validTokenResponse() tokenResponse {
	return tokenResponse{
		AccessToken:           "ghu-access",
		ExpiresIn:             3600,
		RefreshToken:          "ghr-refresh",
		RefreshTokenExpiresIn: 7200,
		TokenType:             "bearer",
	}
}

func testStoredSession(host, account string) Session {
	return Session{
		Host:                  host,
		Account:               account,
		ClientID:              "public-client-id",
		RefreshToken:          "ghr-test-refresh-token-" + account,
		RefreshTokenExpiresAt: time.Date(2026, time.December, 31, 23, 59, 59, 0, time.UTC),
	}
}

func testNilContext() context.Context {
	return nil
}

func newTestAuthService(t *testing.T, options AuthOptions) *AuthService {
	t.Helper()
	if options.Now == nil {
		options.Now = func() time.Time {
			return time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
		}
	}
	return NewAuthService(options)
}

func writeJSON(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		t.Fatal(err)
	}
}

func assertFormValue(t *testing.T, request *http.Request, key, want string) {
	t.Helper()
	if err := request.ParseForm(); err != nil {
		t.Fatal(err)
	}
	if got := request.Form.Get(key); got != want {
		t.Fatalf("form[%q] = %q, want %q", key, got, want)
	}
}

func assertAuthProblem(t *testing.T, err error, code problem.Code) {
	t.Helper()
	value, ok := problem.As(err)
	if !ok || value.Code != code {
		t.Fatalf("problem = %#v, want code %q", err, code)
	}
}

func statusJSON(t *testing.T, status SessionStatus) string {
	t.Helper()
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

type memorySessionStore struct {
	mutex             sync.Mutex
	session           Session
	sessions          map[string]Session
	activeByScope     map[string]string
	activeScopeByHost map[string]string
	initialized       bool
	loadErr           error
	saveErr           error
	deleteErr         error
	saveCalls         int
	deleteCalls       int
}

func (store *memorySessionStore) LoadActive(_ context.Context, host, clientID string) (Session, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if store.loadErr != nil {
		return Session{}, store.loadErr
	}
	store.initialize()
	account, found := store.activeByScope[sessionScopeKey(host, clientID)]
	if !found {
		return Session{}, errSessionNotFound
	}
	session, found := store.sessions[sessionKey(host, account, clientID)]
	if !found {
		return Session{}, errSessionNotFound
	}
	return session, nil
}

func (store *memorySessionStore) LoadActiveForHost(_ context.Context, host string) (Session, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if store.loadErr != nil {
		return Session{}, store.loadErr
	}
	store.initialize()
	scope, found := store.activeScopeByHost[normalizeHost(host)]
	if !found {
		return Session{}, errSessionNotFound
	}
	clientID, found := strings.CutPrefix(scope, normalizeHost(host)+"\x00")
	if !found || strings.TrimSpace(clientID) == "" {
		return Session{}, errSessionNotFound
	}
	account, found := store.activeByScope[scope]
	if !found {
		return Session{}, errSessionNotFound
	}
	session, found := store.sessions[sessionKey(host, account, clientID)]
	if !found {
		return Session{}, errSessionNotFound
	}
	return session, nil
}

func (store *memorySessionStore) SaveActive(_ context.Context, session Session) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if store.saveErr != nil {
		return store.saveErr
	}
	store.initialize()
	scope := sessionScopeKey(session.Host, session.ClientID)
	if previousAccount, found := store.activeByScope[scope]; found &&
		!strings.EqualFold(strings.TrimSpace(previousAccount), strings.TrimSpace(session.Account)) {
		delete(store.sessions, sessionKey(session.Host, previousAccount, session.ClientID))
	}
	store.sessions[sessionKey(session.Host, session.Account, session.ClientID)] = session
	store.activeByScope[scope] = session.Account
	store.activeScopeByHost[normalizeHost(session.Host)] = scope
	store.session = session
	store.saveCalls++
	return nil
}

func (store *memorySessionStore) DeleteActive(_ context.Context, host, clientID string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if store.deleteErr != nil {
		return store.deleteErr
	}
	store.initialize()
	scope := sessionScopeKey(host, clientID)
	account, found := store.activeByScope[scope]
	if !found {
		return errSessionNotFound
	}
	delete(store.activeByScope, scope)
	deleted := store.sessions[sessionKey(host, account, clientID)]
	delete(store.sessions, sessionKey(host, account, clientID))
	if store.activeScopeByHost[normalizeHost(host)] == scope {
		delete(store.activeScopeByHost, normalizeHost(host))
	}
	if store.session == deleted {
		store.session = Session{}
	}
	store.deleteCalls++
	return nil
}

func (store *memorySessionStore) initialize() {
	if store.initialized {
		return
	}
	store.sessions = make(map[string]Session)
	store.activeByScope = make(map[string]string)
	store.activeScopeByHost = make(map[string]string)
	if strings.TrimSpace(store.session.Account) != "" {
		store.sessions[sessionKey(store.session.Host, store.session.Account, store.session.ClientID)] = store.session
		scope := sessionScopeKey(store.session.Host, store.session.ClientID)
		store.activeByScope[scope] = store.session.Account
		store.activeScopeByHost[normalizeHost(store.session.Host)] = scope
	}
	store.initialized = true
}

var _ SessionStore = (*memorySessionStore)(nil)
