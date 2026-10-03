package github

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func diagnosticsResponse(status int, body io.Reader) *http.Response {
	response := &http.Response{StatusCode: status}
	if body != nil {
		response.Body = io.NopCloser(body)
	}
	return response
}

func TestOAuthErrorDiagnostic(t *testing.T) {
	t.Run("returns empty without a readable body", func(t *testing.T) {
		if got := oauthErrorDiagnostic(nil); got != "" {
			t.Fatalf("oauthErrorDiagnostic(nil) = %q", got)
		}
		if got := oauthErrorDiagnostic(diagnosticsResponse(http.StatusBadRequest, nil)); got != "" {
			t.Fatalf("oauthErrorDiagnostic(no body) = %q", got)
		}
		if got := oauthErrorDiagnostic(diagnosticsResponse(http.StatusBadRequest, io.NopCloser(errReader{}))); got != "" {
			t.Fatalf("oauthErrorDiagnostic(read error) = %q", got)
		}
		if got := oauthErrorDiagnostic(diagnosticsResponse(http.StatusBadRequest, strings.NewReader("  \n"))); got != "" {
			t.Fatalf("oauthErrorDiagnostic(blank) = %q", got)
		}
	})

	t.Run("projects the OAuth error body", func(t *testing.T) {
		for _, testCase := range []struct {
			name string
			body string
			want string
		}{
			{
				name: "error and description",
				body: `{"error":"device_flow_disabled","error_description":"Device Flow must be explicitly enabled for this App"}`,
				want: "device_flow_disabled — Device Flow must be explicitly enabled for this App",
			},
			{
				name: "error only",
				body: `{"error":"unauthorized_client"}`,
				want: "unauthorized_client",
			},
			{
				name: "description only",
				body: `{"error_description":"Device Flow must be explicitly enabled for this App"}`,
				want: "Device Flow must be explicitly enabled for this App",
			},
			{
				name: "whitespace-only reason",
				body: `{"error":"   "}`,
				want: "",
			},
			{
				name: "without a classified reason",
				body: `{"documentation_url":"https://docs.github.com"}`,
				want: "",
			},
		} {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				got := oauthErrorDiagnostic(diagnosticsResponse(http.StatusBadRequest, strings.NewReader(testCase.body)))
				if got != testCase.want {
					t.Fatalf("oauthErrorDiagnostic(%s) = %q, want %q", testCase.body, got, testCase.want)
				}
			})
		}
	})

	t.Run("projects the REST error envelope", func(t *testing.T) {
		for _, testCase := range []struct {
			name string
			body string
			want string
		}{
			{
				name: "message only",
				body: `{"message":"Not Found"}`,
				want: "Not Found",
			},
			{
				name: "message and validation entries",
				body: `{"message":"Validation Failed","errors":[{"message":"ref is invalid"}]}`,
				want: "Validation Failed; ref is invalid",
			},
			{
				name: "without a classified reason",
				body: `{"status":"404"}`,
				want: "",
			},
		} {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				got := oauthErrorDiagnostic(diagnosticsResponse(http.StatusNotFound, strings.NewReader(testCase.body)))
				if got != testCase.want {
					t.Fatalf("oauthErrorDiagnostic(%s) = %q, want %q", testCase.body, got, testCase.want)
				}
			})
		}
	})

	t.Run("surfaces non-JSON text unchanged", func(t *testing.T) {
		got := oauthErrorDiagnostic(diagnosticsResponse(
			http.StatusBadRequest,
			strings.NewReader(" token=ghp_secret is surfaced unchanged "),
		))
		if got != "token=ghp_secret is surfaced unchanged" {
			t.Fatalf("oauthErrorDiagnostic(non-JSON) = %q", got)
		}
	})

	t.Run("bounds oversized bodies", func(t *testing.T) {
		response := diagnosticsResponse(
			http.StatusBadRequest,
			strings.NewReader(strings.Repeat("a", lifecycleDiagnosticMaxBytes+100)),
		)
		got := oauthErrorDiagnostic(response)
		if len(got) <= lifecycleDiagnosticMaxBytes || !strings.HasSuffix(got, " …[truncated]") {
			t.Fatalf("oauthErrorDiagnostic(large) length = %d, value = %q", len(got), got)
		}
	})
}

func TestOAuthHTTPProblem(t *testing.T) {
	typed, ok := problem.As(oauthHTTPProblem(diagnosticsResponse(
		http.StatusBadRequest,
		strings.NewReader(`{"error":"device_flow_disabled","error_description":"Device Flow must be explicitly enabled for this App"}`),
	)))
	if !ok {
		t.Fatal("oauthHTTPProblem did not return a typed problem")
	}
	if typed.Code != problem.CodeConfigurationInvalid || typed.Category != problem.CategoryConfig {
		t.Fatalf("code/category = %s/%s, want configuration-invalid/config", typed.Code, typed.Category)
	}
	if typed.Field != "GitHub App authentication" {
		t.Fatalf("field = %q, want the auth surface", typed.Field)
	}
	if typed.Actual != "Bad Request" {
		t.Fatalf("actual = %q, want the HTTP status text", typed.Actual)
	}
	if typed.Diagnostic != "device_flow_disabled — Device Flow must be explicitly enabled for this App" {
		t.Fatalf("diagnostic = %q, want the projected provider reason", typed.Diagnostic)
	}

	typed, ok = problem.As(oauthHTTPProblem(nil))
	if !ok {
		t.Fatal("oauthHTTPProblem(nil) did not return a typed problem")
	}
	if typed.Actual != "" || typed.Diagnostic != "" {
		t.Fatalf("nil response projection = %q/%q, want empty actual and diagnostic", typed.Actual, typed.Diagnostic)
	}
}

func TestLoginSurfacesProviderDeviceCodeDiagnostic(t *testing.T) {
	server := newOAuthServer(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/login/device/code" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		if _, err := writer.Write([]byte(`{"error":"device_flow_disabled","error_description":"Device Flow must be explicitly enabled for this App"}`)); err != nil {
			t.Fatal(err)
		}
	})
	defer server.Close()
	service := newTestAuthService(t, AuthOptions{
		Store:        &memorySessionStore{},
		OAuthBaseURL: server.URL,
		HTTPClient:   server.Client(),
	})
	_, err := service.Login(context.Background(), LoginRequest{ClientID: "public-client-id"})
	assertAuthProblem(t, err, problem.CodeConfigurationInvalid)
	typed, _ := problem.As(err)
	if typed.Actual != "Bad Request" {
		t.Fatalf("actual = %q, want the HTTP status text", typed.Actual)
	}
	if typed.Diagnostic != "device_flow_disabled — Device Flow must be explicitly enabled for this App" {
		t.Fatalf("diagnostic = %q, want the provider reason", typed.Diagnostic)
	}
	if strings.Contains(typed.Diagnostic, "Authorization:") || strings.Contains(typed.Diagnostic, "Bearer") {
		t.Fatalf("diagnostic carries header material: %q", typed.Diagnostic)
	}
}

func TestAuthAPIRequestSurfacesProviderDiagnostic(t *testing.T) {
	server := newOAuthServer(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusForbidden)
		if _, err := writer.Write([]byte(`{"message":"Resource not accessible by integration"}`)); err != nil {
			t.Fatal(err)
		}
	})
	defer server.Close()
	service := newTestAuthService(t, AuthOptions{
		Store:      &memorySessionStore{},
		APIBaseURL: server.URL,
		HTTPClient: server.Client(),
	})
	err := service.repositoryIsInstalledAndAuthorized(
		context.Background(),
		"token",
		CredentialTarget{Host: "github.com", Owner: "acme", Repository: "governance"},
	)
	assertAuthProblem(t, err, problem.CodeConfigurationInvalid)
	typed, _ := problem.As(err)
	if typed.Actual != "Forbidden" {
		t.Fatalf("actual = %q, want the HTTP status text", typed.Actual)
	}
	if typed.Diagnostic != "Resource not accessible by integration" {
		t.Fatalf("diagnostic = %q, want the provider message", typed.Diagnostic)
	}
}
