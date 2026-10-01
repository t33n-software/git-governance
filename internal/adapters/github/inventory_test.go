package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// testInventoryQuery builds the provider inventory query for the fake GitHub
// API server of one test.
func testInventoryQuery(server *httptest.Server) port.PullRequestInventoryQuery {
	return port.PullRequestInventoryQuery{
		Repository: port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"},
		RemoteURL:  server.URL + "/acme/governance.git",
	}
}

func TestListPullRequests(t *testing.T) {
	t.Parallel()

	t.Run("enumerates open and closed titles through pagination", func(t *testing.T) {
		t.Parallel()
		var paths []string
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			paths = append(paths, request.URL.Path+"?state="+request.URL.Query().Get("state")+
				"&per_page="+request.URL.Query().Get("per_page")+"&page="+request.URL.Query().Get("page"))
			if request.URL.Query().Get("page") == "1" {
				_, _ = writer.Write([]byte(`[
					{"number":55,"title":"ABC-37: allow-git-lfs-in-canonical-gitattributes","user":{"login":"CyberT33N"},"created_at":"2026-09-30T20:02:32Z"},
					{"number":12,"title":"fix(ABC-11): synchronize protected workflow contract","user":{"login":"CyberT33N"},"created_at":"2026-05-01T00:00:00Z"}
				]`))
				return
			}
			_, _ = writer.Write([]byte(`[]`))
		}))
		defer server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		summaries, err := publisher.ListPullRequests(context.Background(), testInventoryQuery(server))
		if err != nil {
			t.Fatal(err)
		}
		if len(summaries) != 2 {
			t.Fatalf("ListPullRequests() = %#v", summaries)
		}
		if summaries[0].Number != "55" || summaries[0].Title != "ABC-37: allow-git-lfs-in-canonical-gitattributes" ||
			summaries[0].Author != "CyberT33N" ||
			!summaries[0].CreatedAt.Equal(time.Date(2026, 9, 30, 20, 2, 32, 0, time.UTC)) {
			t.Fatalf("first summary = %#v", summaries[0])
		}
		if strings.Join(paths, "|") != "/repos/acme/governance/pulls?state=all&per_page=100&page=1" {
			t.Fatalf("pagination paths = %v", paths)
		}
	})

	t.Run("continues pagination until a short page", func(t *testing.T) {
		t.Parallel()
		calls := 0
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			calls++
			page := request.URL.Query().Get("page")
			if page == "1" {
				writer.WriteHeader(http.StatusOK)
				_, _ = writer.Write([]byte("[" + strings.TrimSuffix(strings.Repeat(pullRequestInventoryFixture()+",", pullRequestInventoryPageSize), ",") + "]"))
				return
			}
			_, _ = writer.Write([]byte("[" + pullRequestInventoryFixture() + "]"))
		}))
		defer server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		summaries, err := publisher.ListPullRequests(context.Background(), testInventoryQuery(server))
		if err != nil {
			t.Fatal(err)
		}
		if len(summaries) != pullRequestInventoryPageSize+1 || calls != 2 {
			t.Fatalf("ListPullRequests() = %d summaries in %d calls", len(summaries), calls)
		}
	})

	t.Run("exceeding the bounded pagination budget fails closed", func(t *testing.T) {
		t.Parallel()
		fullPage := "[" + strings.TrimSuffix(strings.Repeat(pullRequestInventoryFixture()+",", pullRequestInventoryPageSize), ",") + "]"
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(fullPage))
		}))
		defer server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		_, err := publisher.ListPullRequests(context.Background(), testInventoryQuery(server))
		assertProblem(t, err, problem.CodeExternalCommandFailed)
	})

	t.Run("an API error fails closed", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusForbidden)
			_, _ = writer.Write([]byte(`{"message":"resource not accessible by integration"}`))
		}))
		defer server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		_, err := publisher.ListPullRequests(context.Background(), testInventoryQuery(server))
		assertProblem(t, err, problem.CodeExternalCommandFailed)
	})

	t.Run("a non-HTTPS remote is rejected before any request", func(t *testing.T) {
		t.Parallel()
		publisher := New(Options{Resolver: testCredentialResolver()})
		query := port.PullRequestInventoryQuery{
			Repository: port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"},
			RemoteURL:  "file:///acme/governance.git",
		}
		_, err := publisher.ListPullRequests(context.Background(), query)
		assertProblem(t, err, problem.CodeConfigurationInvalid)
	})

	t.Run("malformed JSON fails closed with the decode failure", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte("{"))
		}))
		defer server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		if _, err := publisher.ListPullRequests(context.Background(), testInventoryQuery(server)); err == nil {
			t.Fatal("ListPullRequests unexpectedly succeeded on malformed JSON")
		}
	})

	t.Run("an unreachable API fails closed", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		endpoint, client := server.URL, server.Client()
		server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: endpoint, HTTPClient: client})
		query := port.PullRequestInventoryQuery{
			Repository: port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"},
			RemoteURL:  endpoint + "/acme/governance.git",
		}
		if _, err := publisher.ListPullRequests(context.Background(), query); err == nil {
			t.Fatal("ListPullRequests unexpectedly succeeded against a closed server")
		}
	})
}

func pullRequestInventoryFixture() string {
	return `{"number":1,"title":"ABC-1: fixture","user":{"login":"fixture"},"created_at":"2026-01-01T00:00:00Z"}`
}

func TestListProtectedLineRequests(t *testing.T) {
	t.Parallel()

	t.Run("enumerates durable request records with provider creation times", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch {
			case request.URL.Path == "/repos/acme/governance/deployments":
				_, _ = writer.Write([]byte("[" + protectedLineDeploymentFixture(6765149073, "ABC-37") + "]"))
			case request.URL.Path == "/repos/acme/governance/deployments/6765149073/statuses":
				_, _ = writer.Write([]byte(`[{"state":"success","description":"git-governance protected-line request state=request_authorized"}]`))
			default:
				t.Fatalf("unexpected path %q", request.URL.Path)
			}
		}))
		defer server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		query := port.ProtectedLineRequestInventoryQuery{
			Repository: testInventoryQuery(server).Repository,
			RemoteURL:  testInventoryQuery(server).RemoteURL,
		}
		records, err := publisher.ListProtectedLineRequests(context.Background(), query)
		if err != nil {
			t.Fatal(err)
		}
		if len(records) != 1 {
			t.Fatalf("ListProtectedLineRequests() = %#v", records)
		}
		if records[0].Request.Ticket().String() != "ABC-37" ||
			records[0].Request.Requester() != "d-demand-priv" ||
			!records[0].CreatedAt.Equal(time.Date(2026, 9, 30, 16, 53, 44, 0, time.UTC)) {
			t.Fatalf("record = %#v", records[0])
		}
	})

	t.Run("exceeding the bounded pagination budget fails closed", func(t *testing.T) {
		t.Parallel()
		fullPage := "[" + strings.TrimSuffix(strings.Repeat(protectedLineDeploymentFixture(1, "ABC-1")+",", protectedLineDeploymentPageSize), ",") + "]"
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/repos/acme/governance/deployments" {
				_, _ = writer.Write([]byte(fullPage))
				return
			}
			_, _ = writer.Write([]byte(`[{"state":"success","description":"git-governance protected-line request state=request_authorized"}]`))
		}))
		defer server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		query := port.ProtectedLineRequestInventoryQuery{
			Repository: testInventoryQuery(server).Repository,
			RemoteURL:  testInventoryQuery(server).RemoteURL,
		}
		_, err := publisher.ListProtectedLineRequests(context.Background(), query)
		assertProblem(t, err, problem.CodeConfigurationInvalid)
	})

	t.Run("an API error fails closed", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = writer.Write([]byte(`{"message":"boom"}`))
		}))
		defer server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		query := port.ProtectedLineRequestInventoryQuery{
			Repository: testInventoryQuery(server).Repository,
			RemoteURL:  testInventoryQuery(server).RemoteURL,
		}
		_, err := publisher.ListProtectedLineRequests(context.Background(), query)
		assertProblem(t, err, problem.CodeExternalCommandFailed)
	})

	t.Run("malformed JSON fails closed with the decode failure", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte("{"))
		}))
		defer server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		query := port.ProtectedLineRequestInventoryQuery{
			Repository: testInventoryQuery(server).Repository,
			RemoteURL:  testInventoryQuery(server).RemoteURL,
		}
		if _, err := publisher.ListProtectedLineRequests(context.Background(), query); err == nil {
			t.Fatal("ListProtectedLineRequests unexpectedly succeeded on malformed JSON")
		}
	})

	t.Run("an unreachable API fails closed", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		endpoint, client := server.URL, server.Client()
		server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: endpoint, HTTPClient: client})
		query := port.ProtectedLineRequestInventoryQuery{
			Repository: testInventoryQuery(server).Repository,
			RemoteURL:  testInventoryQuery(server).RemoteURL,
		}
		if _, err := publisher.ListProtectedLineRequests(context.Background(), query); err == nil {
			t.Fatal("ListProtectedLineRequests unexpectedly succeeded against a closed server")
		}
	})

	t.Run("a deployment outside the controller fails the listing closed", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/repos/acme/governance/deployments" {
				_, _ = writer.Write([]byte(`[{"id":1,"ref":"main","task":"other-task","environment":"release-request","payload":"{}","created_at":"2026-09-30T16:53:44Z"}]`))
				return
			}
			_, _ = writer.Write([]byte(`[{"state":"success","description":"git-governance protected-line request state=request_authorized"}]`))
		}))
		defer server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		query := port.ProtectedLineRequestInventoryQuery{
			Repository: testInventoryQuery(server).Repository,
			RemoteURL:  testInventoryQuery(server).RemoteURL,
		}
		if _, err := publisher.ListProtectedLineRequests(context.Background(), query); err == nil {
			t.Fatal("ListProtectedLineRequests unexpectedly accepted a foreign deployment")
		}
	})

	t.Run("a non-HTTPS remote is rejected before any request", func(t *testing.T) {
		t.Parallel()
		publisher := New(Options{Resolver: testCredentialResolver()})
		query := port.ProtectedLineRequestInventoryQuery{
			Repository: port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"},
			RemoteURL:  "file:///acme/governance.git",
		}
		_, err := publisher.ListProtectedLineRequests(context.Background(), query)
		assertProblem(t, err, problem.CodeConfigurationInvalid)
	})
}

func protectedLineDeploymentFixture(id int64, rawTicket string) string {
	return fmt.Sprintf(`{"id":%d,"ref":"main","task":"git-governance-protected-line-request",`+
		`"environment":"release-request","payload":{`+
		`"schemaVersion":1,"requestID":"req-%s","repository":"acme/governance","operation":"release",`+
		`"ticket":"%s","version":"1.1.1","targetRef":"release/1.1.1","sourceRef":"develop",`+
		`"sourceSHA":"%s","requester":"d-demand-priv","expectedExecutor":"execute-protected-line-request.yml",`+
		`"parentRunID":"6765149073","expiresAt":"2026-09-30T20:53:44Z","idempotencyKey":"%s"},`+
		`"created_at":"2026-09-30T16:53:44Z"}`,
		id, strings.ToLower(rawTicket), rawTicket, strings.Repeat("c", 40), strings.Repeat("d", 64))
}
