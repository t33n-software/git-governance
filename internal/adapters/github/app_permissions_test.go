package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func testAppPermissionQuery(server *httptest.Server) port.AppPermissionQuery {
	return port.AppPermissionQuery{
		Repository: port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"},
		RemoteURL:  server.URL + "/acme/governance.git",
	}
}

func TestInspectAppPermissions(t *testing.T) {
	t.Parallel()

	t.Run("resolves the session app slug and reads the public registration", func(t *testing.T) {
		t.Parallel()
		var paths []string
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			paths = append(paths, request.URL.Path)
			switch request.URL.Path {
			case "/user/installations":
				_, _ = writer.Write([]byte(`{"installations":[{"id":1,"app_slug":"acme-publisher"},{"id":2,"app_slug":"acme-publisher"}]}`))
			case "/apps/acme-publisher":
				_, _ = writer.Write([]byte(`{"slug":"acme-publisher","permissions":{"contents":"write","metadata":"read","pull_requests":"write"}}`))
			default:
				t.Fatalf("unexpected path %q", request.URL.Path)
			}
		}))
		defer server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		snapshot, err := publisher.InspectAppPermissions(context.Background(), testAppPermissionQuery(server))
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.Slug != "acme-publisher" {
			t.Fatalf("slug = %q", snapshot.Slug)
		}
		if snapshot.Permissions["pull_requests"] != "write" || snapshot.Permissions["contents"] != "write" {
			t.Fatalf("permissions = %#v", snapshot.Permissions)
		}
		if len(paths) != 2 {
			t.Fatalf("discovery paths = %v", paths)
		}
	})

	t.Run("a failed installation discovery fails closed", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusForbidden)
			_, _ = writer.Write([]byte(`{"message":"resource not accessible by integration"}`))
		}))
		defer server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		_, err := publisher.InspectAppPermissions(context.Background(), testAppPermissionQuery(server))
		assertProblem(t, err, problem.CodeExternalCommandFailed)
		typed, ok := problem.As(err)
		if !ok || typed.Field != "GitHub App installation discovery" {
			t.Fatalf("failure must name the installation discovery field: %#v", typed)
		}
	})

	t.Run("a session without installations fails closed", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(`{"installations":[]}`))
		}))
		defer server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		_, err := publisher.InspectAppPermissions(context.Background(), testAppPermissionQuery(server))
		assertProblem(t, err, problem.CodeExternalCommandFailed)
	})

	t.Run("installations without an app slug fail closed", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(`{"installations":[{"id":1}]}`))
		}))
		defer server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		_, err := publisher.InspectAppPermissions(context.Background(), testAppPermissionQuery(server))
		assertProblem(t, err, problem.CodeExternalCommandFailed)
	})

	t.Run("an unreadable app registration fails closed", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/user/installations" {
				_, _ = writer.Write([]byte(`{"installations":[{"id":1,"app_slug":"acme-publisher"}]}`))
				return
			}
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"message":"Not Found"}`))
		}))
		defer server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		_, err := publisher.InspectAppPermissions(context.Background(), testAppPermissionQuery(server))
		assertProblem(t, err, problem.CodeExternalCommandFailed)
		typed, ok := problem.As(err)
		if !ok || typed.Field != "GitHub App permission registration" {
			t.Fatalf("failure must name the registration field: %#v", typed)
		}
	})

	t.Run("a registration without a slug fails closed", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/user/installations" {
				_, _ = writer.Write([]byte(`{"installations":[{"id":1,"app_slug":"acme-publisher"}]}`))
				return
			}
			_, _ = writer.Write([]byte(`{"slug":"","permissions":{"contents":"read"}}`))
		}))
		defer server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		_, err := publisher.InspectAppPermissions(context.Background(), testAppPermissionQuery(server))
		assertProblem(t, err, problem.CodeExternalCommandFailed)
	})

	t.Run("an empty permission map is carried as an empty classification", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/user/installations" {
				_, _ = writer.Write([]byte(`{"installations":[{"id":1,"app_slug":"acme-publisher"}]}`))
				return
			}
			_, _ = writer.Write([]byte(`{"slug":"acme-publisher"}`))
		}))
		defer server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		snapshot, err := publisher.InspectAppPermissions(context.Background(), testAppPermissionQuery(server))
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.Permissions == nil || len(snapshot.Permissions) != 0 {
			t.Fatalf("permissions = %#v; want an empty non-nil map", snapshot.Permissions)
		}
	})

	t.Run("an unreachable API fails closed", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		endpoint, client := server.URL, server.Client()
		server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: endpoint, HTTPClient: client})
		if _, err := publisher.InspectAppPermissions(context.Background(), testAppPermissionQuery(server)); err == nil {
			t.Fatal("InspectAppPermissions unexpectedly succeeded against a closed server")
		}
	})

	t.Run("malformed installation JSON fails closed", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte("{"))
		}))
		defer server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		if _, err := publisher.InspectAppPermissions(context.Background(), testAppPermissionQuery(server)); err == nil {
			t.Fatal("InspectAppPermissions unexpectedly succeeded on malformed installation JSON")
		}
	})

	t.Run("a network failure during registration fails closed", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/user/installations" {
				_, _ = writer.Write([]byte(`{"installations":[{"id":1,"app_slug":"acme-publisher"}]}`))
				return
			}
			hijacker, ok := writer.(http.Hijacker)
			if !ok {
				t.Fatalf("the test server does not support hijacking")
			}
			connection, _, err := hijacker.Hijack()
			if err != nil {
				t.Fatalf("hijack failed: %v", err)
			}
			_ = connection.Close()
		}))
		defer server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		if _, err := publisher.InspectAppPermissions(context.Background(), testAppPermissionQuery(server)); err == nil {
			t.Fatal("InspectAppPermissions unexpectedly succeeded on a dropped registration connection")
		}
	})

	t.Run("malformed registration JSON fails closed", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/user/installations" {
				_, _ = writer.Write([]byte(`{"installations":[{"id":1,"app_slug":"acme-publisher"}]}`))
				return
			}
			_, _ = writer.Write([]byte("{"))
		}))
		defer server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		if _, err := publisher.InspectAppPermissions(context.Background(), testAppPermissionQuery(server)); err == nil {
			t.Fatal("InspectAppPermissions unexpectedly succeeded on malformed registration JSON")
		}
	})

	t.Run("a non-HTTPS remote is rejected before any request", func(t *testing.T) {
		t.Parallel()
		publisher := New(Options{Resolver: testCredentialResolver()})
		query := port.AppPermissionQuery{
			Repository: port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"},
			RemoteURL:  "file:///acme/governance.git",
		}
		_, err := publisher.InspectAppPermissions(context.Background(), query)
		assertProblem(t, err, problem.CodeConfigurationInvalid)
	})
}
