package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestBrokerResolverMintsAndCachesRepositoryBoundTokens(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		if request.Method != http.MethodPost || request.URL.Path != brokerInstallationTokenPath ||
			request.Header.Get("Authorization") != "Bearer workload-identity-secret" ||
			request.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("broker request = %s %s %#v", request.Method, request.URL.String(), request.Header)
		}
		var payload brokerRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload != (brokerRequest{Host: "github.com", Owner: "acme", Repository: "governance"}) {
			t.Fatalf("broker payload = %#v", payload)
		}
		writeJSON(t, writer, brokerResponse{
			AccessToken: "ghs-installation-secret",
			ExpiresAt:   now.Add(2 * time.Hour),
		})
	}))
	defer server.Close()

	resolver := NewBrokerResolver(BrokerOptions{
		Endpoint:         server.URL,
		WorkloadIdentity: func() string { return "workload-identity-secret" },
		HTTPClient:       server.Client(),
		Now:              func() time.Time { return now },
	})
	target := CredentialTarget{Host: "github.com", Owner: "acme", Repository: "governance"}
	token, err := resolver.Resolve(context.Background(), target)
	if err != nil || token != "ghs-installation-secret" {
		t.Fatalf("Resolve() = (%q, %v)", token, err)
	}
	again, err := resolver.Resolve(context.Background(), target)
	if err != nil || again != token || calls != 1 {
		t.Fatalf("cached Resolve() = (%q, %v), calls=%d", again, err, calls)
	}
}

func TestBrokerResolverRejectsUnsafeOrInvalidInputs(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	defaults := NewBrokerResolver(BrokerOptions{})
	if defaults.client == nil || defaults.now == nil || defaults.workloadIdentity == nil || defaults.cached == nil {
		t.Fatalf("default broker resolver = %#v", defaults)
	}
	target := CredentialTarget{Host: "github.com", Owner: "acme", Repository: "governance"}

	_, err := defaults.Resolve(context.Background(), target)
	assertBrokerProblem(t, err, problem.CodeConfigurationUnavailable)
	for _, invalid := range []CredentialTarget{{}, {Host: "github.com", Owner: "acme"}} {
		_, err := defaults.Resolve(context.Background(), invalid)
		assertBrokerProblem(t, err, problem.CodeConfigurationInvalid)
	}
	_, err = defaults.Resolve(testNilContext(), target)
	assertBrokerProblem(t, err, problem.CodeInvalidInput)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = defaults.Resolve(cancelled, target)
	assertBrokerProblem(t, err, problem.CodeOperationCancelled)

	badEndpoint := NewBrokerResolver(BrokerOptions{
		Endpoint:         "http://broker.example",
		WorkloadIdentity: func() string { return "identity" },
		Now:              func() time.Time { return now },
	})
	_, err = badEndpoint.Resolve(context.Background(), target)
	assertBrokerProblem(t, err, problem.CodeConfigurationInvalid)

	networkErr := errors.New("broker unavailable")
	network := NewBrokerResolver(BrokerOptions{
		Endpoint:         "https://broker.example",
		WorkloadIdentity: func() string { return "identity" },
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, networkErr
		})},
		Now: func() time.Time { return now },
	})
	_, err = network.Resolve(context.Background(), target)
	assertBrokerProblem(t, err, problem.CodeExternalCommandFailed)
	if !errors.Is(err, networkErr) {
		t.Fatalf("network broker error = %v, want %v", err, networkErr)
	}
}

func TestBrokerResolverClassifiesResponsesWithoutLeakingTokens(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	target := CredentialTarget{Host: "enterprise.example", Owner: "acme", Repository: "governance"}
	for _, testCase := range []struct {
		name   string
		status int
		body   string
		code   problem.Code
	}{
		{name: "forbidden", status: http.StatusForbidden, body: `{"access_token":"ghs-secret"}`, code: problem.CodeConfigurationInvalid},
		{name: "malformed", status: http.StatusOK, body: "{", code: problem.CodeConfigurationInvalid},
		{name: "missing token", status: http.StatusOK, body: `{"expires_at":"2026-07-16T14:00:00Z"}`, code: problem.CodeConfigurationInvalid},
		{name: "expired token", status: http.StatusOK, body: `{"access_token":"ghs-secret","expires_at":"2026-07-16T12:00:01Z"}`, code: problem.CodeConfigurationInvalid},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.WriteHeader(testCase.status)
				_, _ = writer.Write([]byte(testCase.body))
			}))
			defer server.Close()
			resolver := NewBrokerResolver(BrokerOptions{
				Endpoint:         server.URL + "/broker",
				WorkloadIdentity: func() string { return "identity" },
				HTTPClient:       server.Client(),
				Now:              func() time.Time { return now },
			})
			_, err := resolver.Resolve(context.Background(), target)
			assertBrokerProblem(t, err, testCase.code)
			if strings.Contains(err.Error(), "ghs-secret") || strings.Contains(err.Error(), "identity") {
				t.Fatalf("broker error leaked a secret: %v", err)
			}
		})
	}
}

func TestBrokerHelpers(t *testing.T) {
	target, err := validateBrokerTarget(CredentialTarget{
		Host:       " github.com ",
		Owner:      " acme ",
		Repository: " governance ",
	})
	if err != nil || target != (CredentialTarget{Host: "github.com", Owner: "acme", Repository: "governance"}) {
		t.Fatalf("validateBrokerTarget() = (%#v, %v)", target, err)
	}
	assertBrokerProblem(t, brokerEndpointProblem(errors.New("endpoint")), problem.CodeConfigurationInvalid)
	assertBrokerProblem(t, brokerNetworkProblem(errors.New("network")), problem.CodeExternalCommandFailed)
	assertBrokerProblem(t, brokerHTTPProblem(http.StatusUnauthorized), problem.CodeConfigurationInvalid)
}

func assertBrokerProblem(t *testing.T, err error, code problem.Code) {
	t.Helper()
	value, ok := problem.As(err)
	if !ok || value.Code != code {
		t.Fatalf("problem = %#v, want code %q", err, code)
	}
}
