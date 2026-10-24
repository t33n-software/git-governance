package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// TestAuthServiceStatusDiscoversCoveringHostSessionWithoutBinding proves the
// regression fix: a repository without any stored binding reports the host
// session whose GitHub App installation covers the target repository, while
// the read-only status query never creates a binding itself.
func TestAuthServiceStatusDiscoversCoveringHostSessionWithoutBinding(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	server := capabilityCardServer(t, []string{"acme/evidence"}, nil)
	defer server.Close()
	store := capabilityTestStore(t)
	service := newTestAuthService(t, AuthOptions{
		Store:        store,
		OAuthBaseURL: server.URL,
		APIBaseURL:   server.URL,
		HTTPClient:   server.Client(),
		Now:          func() time.Time { return now },
	})
	statuses, err := service.Status(context.Background(), CredentialTarget{
		Host: "github.com", Owner: "acme", Repository: "governance",
	})
	if err != nil {
		t.Fatalf("discovering Status() error = %v", err)
	}
	// Only the publisher installation covers acme/governance; the verify
	// session covers the unrelated evidence repository and is skipped.
	if len(statuses) != 1 || statuses[0].ClientID != "publisher-client-id" ||
		statuses[0].Repository != "acme/governance" || statuses[0].AppSlug != "acme-publisher" {
		t.Fatalf("discovered statuses = %#v", statuses)
	}
	if joined := strings.Join(statuses[0].Capabilities, ","); joined != "contents,metadata,pull_requests" {
		t.Fatalf("discovered capabilities = %q", joined)
	}
	if statuses[0].RefreshState != "active" {
		t.Fatalf("discovered refresh state = %q", statuses[0].RefreshState)
	}
	if store.bindCalls != 0 {
		t.Fatalf("the status query bound a session: calls=%d", store.bindCalls)
	}
}

// TestAuthServiceStatusFailsClosedWhenNoHostSessionCoversRepository proves the
// fail-closed boundary of the discovery fallback: without any covering session
// the repository target keeps the not-found verdict.
func TestAuthServiceStatusFailsClosedWhenNoHostSessionCoversRepository(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	server := capabilityCardServer(t, nil, nil)
	defer server.Close()
	service := newTestAuthService(t, AuthOptions{
		Store:        capabilityTestStore(t),
		OAuthBaseURL: server.URL,
		APIBaseURL:   server.URL,
		HTTPClient:   server.Client(),
		Now:          func() time.Time { return now },
	})
	_, err := service.Status(context.Background(), CredentialTarget{
		Host: "github.com", Owner: "acme", Repository: "uncovered",
	})
	assertAuthProblem(t, err, problem.CodeConfigurationUnavailable)
}

// TestAuthServiceStatusDiscoveryStoreFailures covers the store-boundary
// verdicts of the discovery fallback: a broken host index and an empty host
// store both fail closed without any network call.
func TestAuthServiceStatusDiscoveryStoreFailures(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	target := CredentialTarget{Host: "github.com", Owner: "acme", Repository: "governance"}

	t.Run("a host listing failure fails closed", func(t *testing.T) {
		store := capabilityTestStore(t)
		store.hostListErr = errors.New("host index broken")
		service := newTestAuthService(t, AuthOptions{
			Store: store,
			Now:   func() time.Time { return now },
		})
		_, err := service.Status(context.Background(), target)
		assertAuthProblem(t, err, problem.CodeConfigurationUnavailable)
	})

	t.Run("an empty host store fails closed", func(t *testing.T) {
		service := newTestAuthService(t, AuthOptions{
			Store: &memorySessionStore{},
			Now:   func() time.Time { return now },
		})
		_, err := service.Status(context.Background(), target)
		assertAuthProblem(t, err, problem.CodeConfigurationUnavailable)
	})
}

// TestAuthServiceStatusDiscoverySessionFailurePaths covers the per-candidate
// verdicts inside the discovery fallback: an expired refresh fails closed with
// the re-login remediation, and an unreadable app registration of a covering
// session fails the whole status closed.
func TestAuthServiceStatusDiscoverySessionFailurePaths(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	target := CredentialTarget{Host: "github.com", Owner: "acme", Repository: "governance"}

	t.Run("an expired discovered session fails closed", func(t *testing.T) {
		store := capabilityTestStore(t)
		store.mutex.Lock()
		for key, session := range store.sessions {
			session.RefreshTokenExpiresAt = now.Add(-time.Second)
			store.sessions[key] = session
		}
		store.mutex.Unlock()
		service := newTestAuthService(t, AuthOptions{
			Store: store,
			Now:   func() time.Time { return now },
		})
		_, err := service.Status(context.Background(), target)
		assertAuthProblem(t, err, problem.CodeConfigurationInvalid)
	})

	t.Run("an unreadable registration of a covering session fails closed", func(t *testing.T) {
		failingRegistration := true
		server := capabilityCardServer(t, []string{"acme/governance"}, &failingRegistration)
		defer server.Close()
		service := newTestAuthService(t, AuthOptions{
			Store:        capabilityTestStore(t),
			OAuthBaseURL: server.URL,
			APIBaseURL:   server.URL,
			HTTPClient:   server.Client(),
			Now:          func() time.Time { return now },
		})
		_, err := service.Status(context.Background(), target)
		assertAuthProblem(t, err, problem.CodeConfigurationInvalid)
	})
}

// TestAuthServiceStatusDiscoveryInfrastructureFailures covers the
// infrastructure verdicts of the discovery fallback: an external transport
// failure during a candidate refresh and an authorization endpoint failure of
// a candidate both surface the first hard error instead of the not-found
// remediation.
func TestAuthServiceStatusDiscoveryInfrastructureFailures(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	target := CredentialTarget{Host: "github.com", Owner: "acme", Repository: "governance"}

	t.Run("a transport failure during the probe is an infrastructure error", func(t *testing.T) {
		service := newTestAuthService(t, AuthOptions{
			Store: capabilityTestStore(t),
			HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("network unreachable")
			})},
			Now: func() time.Time { return now },
		})
		_, err := service.Status(context.Background(), target)
		assertAuthProblem(t, err, problem.CodeExternalCommandFailed)
	})

	t.Run("an authorization endpoint failure fails closed with its own verdict", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/login/oauth/access_token" {
				writeJSON(t, writer, validTokenResponse())
				return
			}
			writer.WriteHeader(http.StatusBadGateway)
		}))
		defer server.Close()
		service := newTestAuthService(t, AuthOptions{
			Store:        capabilityTestStore(t),
			OAuthBaseURL: server.URL,
			APIBaseURL:   server.URL,
			HTTPClient:   server.Client(),
			Now:          func() time.Time { return now },
		})
		_, err := service.Status(context.Background(), target)
		assertAuthProblem(t, err, problem.CodeConfigurationInvalid)
	})
}
