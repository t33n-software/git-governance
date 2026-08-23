package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestPublisherCreatesAndReusesPullRequests(t *testing.T) {
	t.Run("creates a pull request with the GitHub REST contract", func(t *testing.T) {
		var requests []*http.Request
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			requests = append(requests, request)
			if request.URL.Path != "/repos/acme/governance/pulls" {
				t.Fatalf("path = %q", request.URL.Path)
			}
			switch request.Method {
			case http.MethodGet:
				if got := request.URL.Query().Get("state"); got != "open" {
					t.Fatalf("state query = %q", got)
				}
				if got := request.URL.Query().Get("head"); got != "acme:feature/ABC-123-add-export" {
					t.Fatalf("head query = %q", got)
				}
				if got := request.URL.Query().Get("base"); got != "develop" {
					t.Fatalf("base query = %q", got)
				}
				_, _ = writer.Write([]byte("[]"))
			case http.MethodPost:
				var body createPullRequestRequest
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body != (createPullRequestRequest{
					Title: "ABC-123: add export",
					Body:  "Summary: Add the export button.\n\nScope and Non-Goals: The export button only.\n\nCommit Series:\n- feat(ABC-123): add export button\n\nRisk and Rollback: Low; revert the commit.\n\nVerification and Review Focus: Unit tests; review the button wiring.",
					Head:  "feature/ABC-123-add-export",
					Base:  "develop",
					Draft: true,
				}) {
					t.Fatalf("body = %#v", body)
				}
				writer.WriteHeader(http.StatusCreated)
				_, _ = writer.Write([]byte(`{"html_url":"https://github.example/pr/42"}`))
			default:
				t.Fatalf("method = %s", request.Method)
			}
		}))
		defer server.Close()

		publisher := New(Options{
			Resolver:   testCredentialResolver(),
			APIBaseURL: server.URL,
			APIVersion: "test-version",
			HTTPClient: server.Client(),
		})
		publication := testPublication(server.URL, true)
		if err := publisher.Validate(context.Background(), publication); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
		result, err := publisher.Publish(context.Background(), publication)
		if err != nil || result.URL != "https://github.example/pr/42" {
			t.Fatalf("Publish() = (%#v, %v)", result, err)
		}
		if len(requests) != 2 {
			t.Fatalf("request count = %d", len(requests))
		}
		for _, request := range requests {
			if request.Header.Get("Authorization") != "Bearer token" ||
				request.Header.Get("Accept") != "application/vnd.github+json" ||
				request.Header.Get("X-GitHub-Api-Version") != "test-version" ||
				request.Header.Get("User-Agent") != "git-governance" {
				t.Fatalf("headers = %#v", request.Header)
			}
		}
		if requests[1].Header.Get("Content-Type") != "application/json" {
			t.Fatalf("content type = %q", requests[1].Header.Get("Content-Type"))
		}
	})

	t.Run("returns an existing matching open pull request", func(t *testing.T) {
		calls := 0
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			calls++
			if request.Method != http.MethodGet {
				t.Fatalf("method = %s, want GET", request.Method)
			}
			_, _ = writer.Write([]byte(`[{"html_url":"https://github.example/pr/existing"}]`))
		}))
		defer server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		result, err := publisher.Publish(context.Background(), testPublication(server.URL, false))
		if err != nil || result.URL != "https://github.example/pr/existing" || calls != 1 {
			t.Fatalf("Publish() = (%#v, %v), calls=%d", result, err, calls)
		}
	})
}

func TestPublisherRejectsInvalidConfigurationAndIntent(t *testing.T) {
	publication := testPublication("https://github.com", false)

	t.Run("nil publisher", func(t *testing.T) {
		var publisher *Publisher
		_, err := publisher.Publish(context.Background(), publication)
		assertProblem(t, err, problem.CodeConfigurationInvalid)
	})

	t.Run("missing credential resolver", func(t *testing.T) {
		_, err := New(Options{}).Publish(context.Background(), publication)
		assertProblem(t, err, problem.CodeConfigurationInvalid)
	})

	t.Run("incomplete intent", func(t *testing.T) {
		invalid := publication
		invalid.PullRequest.Title = ""
		_, err := New(Options{
			Resolver:   testCredentialResolver(),
			APIBaseURL: defaultAPIBaseURL,
		}).Publish(context.Background(), invalid)
		assertProblem(t, err, problem.CodeConfigurationInvalid)
	})

	t.Run("missing mandatory description", func(t *testing.T) {
		invalid := publication
		invalid.PullRequest.Body = "  "
		_, err := New(Options{
			Resolver:   testCredentialResolver(),
			APIBaseURL: defaultAPIBaseURL,
		}).Publish(context.Background(), invalid)
		assertProblem(t, err, problem.CodeConfigurationInvalid)
	})

	t.Run("invalid API URL", func(t *testing.T) {
		_, err := New(Options{Resolver: testCredentialResolver(), APIBaseURL: "http://api.github.com"}).Publish(context.Background(), publication)
		assertProblem(t, err, problem.CodeConfigurationInvalid)
	})

	t.Run("invalid remote", func(t *testing.T) {
		invalid := publication
		invalid.RemoteURL = ""
		_, err := New(Options{Resolver: testCredentialResolver()}).Publish(context.Background(), invalid)
		assertProblem(t, err, problem.CodeConfigurationInvalid)
	})

	t.Run("remote host does not match API host", func(t *testing.T) {
		invalid := publication
		invalid.RemoteURL = "https://gitlab.example/acme/governance.git"
		_, err := New(Options{
			Resolver:   testCredentialResolver(),
			APIBaseURL: defaultAPIBaseURL,
		}).Publish(context.Background(), invalid)
		assertProblem(t, err, problem.CodeConfigurationInvalid)
	})

	t.Run("derives a GitHub Enterprise API base from a broker-routed remote", func(t *testing.T) {
		enterprise := publication
		enterprise.RemoteURL = "https://github.enterprise.example/acme/governance.git"
		apiBase, repository, err := New(Options{Resolver: testCredentialResolver()}).publicationTarget(enterprise)
		if err != nil || apiBase.String() != "https://github.enterprise.example/api/v3" ||
			repository.host != "github.enterprise.example" {
			t.Fatalf("enterprise publication target = (%#v, %#v, %v)", apiBase, repository, err)
		}
	})
}

func TestPublisherClassifiesHTTPAndResponseFailures(t *testing.T) {
	testCases := []struct {
		name       string
		getBody    string
		getStatus  int
		postBody   string
		postStatus int
	}{
		{name: "lookup status", getStatus: http.StatusForbidden},
		{name: "lookup malformed JSON", getStatus: http.StatusOK, getBody: "{"},
		{name: "lookup missing URL", getStatus: http.StatusOK, getBody: `[{}]`},
		{name: "create status", getStatus: http.StatusOK, getBody: "[]", postStatus: http.StatusUnprocessableEntity},
		{name: "create malformed JSON", getStatus: http.StatusOK, getBody: "[]", postStatus: http.StatusCreated, postBody: "{"},
		{name: "create missing URL", getStatus: http.StatusOK, getBody: "[]", postStatus: http.StatusCreated, postBody: `{}`},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.Method {
				case http.MethodGet:
					status := testCase.getStatus
					if status == 0 {
						status = http.StatusOK
					}
					writer.WriteHeader(status)
					_, _ = writer.Write([]byte(testCase.getBody))
				case http.MethodPost:
					status := testCase.postStatus
					if status == 0 {
						status = http.StatusCreated
					}
					writer.WriteHeader(status)
					_, _ = writer.Write([]byte(testCase.postBody))
				default:
					t.Fatalf("unexpected method %s", request.Method)
				}
			}))
			defer server.Close()

			publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
			_, err := publisher.Publish(context.Background(), testPublication(server.URL, false))
			assertProblem(t, err, problem.CodeExternalCommandFailed)
		})
	}

	t.Run("transport error", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("network unavailable")
		})}
		publisher := New(Options{Resolver: testCredentialResolver(), HTTPClient: client})
		_, err := publisher.Publish(context.Background(), testPublication("https://github.com", false))
		assertProblem(t, err, problem.CodeExternalCommandFailed)
	})
}

func TestPublisherClassifiesSafeGitHubResponseDetails(t *testing.T) {
	testCases := []struct {
		name           string
		status         int
		body           string
		expectedField  string
		expectedActual string
	}{
		{
			name:           "authorization",
			status:         http.StatusUnprocessableEntity,
			body:           `{"message":"Resource not accessible by integration secret-token"}`,
			expectedField:  "GitHub pull request authorization",
			expectedActual: http.StatusText(http.StatusUnprocessableEntity),
		},
		{
			name:           "repository visibility",
			status:         http.StatusNotFound,
			body:           `{"message":"repository secret-token was not found"}`,
			expectedField:  "GitHub pull request repository visibility",
			expectedActual: http.StatusText(http.StatusNotFound),
		},
		{
			name:           "source branch",
			status:         http.StatusUnprocessableEntity,
			body:           `{"message":"Validation Failed","errors":[{"resource":"PullRequest","field":"head","code":"invalid","message":"secret-token"}]}`,
			expectedField:  "GitHub pull request source branch",
			expectedActual: "GitHub rejected the source branch",
		},
		{
			name:           "target branch",
			status:         http.StatusUnprocessableEntity,
			body:           `{"message":"Validation Failed","errors":[{"resource":"PullRequest","field":"base","code":"invalid","message":"secret-token"}]}`,
			expectedField:  "GitHub pull request target branch",
			expectedActual: "GitHub rejected the target branch",
		},
		{
			name:           "title",
			status:         http.StatusUnprocessableEntity,
			body:           `{"message":"Validation Failed","errors":[{"resource":"PullRequest","field":"title","code":"invalid","message":"secret-token"}]}`,
			expectedField:  "GitHub pull request title",
			expectedActual: "GitHub rejected the pull-request title",
		},
		{
			name:           "duplicate message",
			status:         http.StatusUnprocessableEntity,
			body:           `{"message":"Validation Failed","errors":[{"resource":"PullRequest","code":"custom","message":"A pull request already exists for secret-token"}]}`,
			expectedField:  "GitHub pull request duplicate",
			expectedActual: "an equivalent pull request already exists",
		},
		{
			name:           "duplicate code",
			status:         http.StatusUnprocessableEntity,
			body:           `{"errors":[{"resource":"PullRequest","code":"already_exists"}]}`,
			expectedField:  "GitHub pull request duplicate",
			expectedActual: "an equivalent pull request already exists",
		},
		{
			name:           "unknown validation",
			status:         http.StatusUnprocessableEntity,
			body:           `{"message":"Validation Failed secret-token"}`,
			expectedField:  "GitHub pull request validation",
			expectedActual: "GitHub rejected the pull-request semantics",
		},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			err := responseProblemWithBody(&http.Response{
				StatusCode: testCase.status,
				Body:       io.NopCloser(strings.NewReader(testCase.body)),
			}, "create a GitHub pull request")
			value, ok := problem.As(err)
			if !ok {
				t.Fatalf("problem = %T, want classified problem", err)
			}
			if value.Field != testCase.expectedField || value.Actual != testCase.expectedActual {
				t.Fatalf("problem = %#v, want field=%q actual=%q", value.Details, testCase.expectedField, testCase.expectedActual)
			}
			assertNoPublisherResponseLeak(t, err, value)
		})
	}
}

func TestPublisherCredentialResolutionFailures(t *testing.T) {
	publication := testPublication("https://github.com", false)
	resolverErr := errors.New("credential unavailable")
	publisher := New(Options{Resolver: &fakeCredentialResolver{err: resolverErr}})
	if err := publisher.Validate(context.Background(), publication); !errors.Is(err, resolverErr) {
		t.Fatalf("Validate() error = %v, want %v", err, resolverErr)
	}
	invalid := publication
	invalid.PullRequest.Title = ""
	if err := New(Options{Resolver: testCredentialResolver()}).Validate(context.Background(), invalid); err == nil {
		t.Fatal("Validate() accepted an incomplete pull-request intent")
	}
	base, err := url.Parse("https://github.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.request(
		context.Background(),
		repositoryRef{host: "github.com", owner: "acme", name: "governance"},
		http.MethodGet,
		base,
		nil,
	); !errors.Is(err, resolverErr) {
		t.Fatalf("request() error = %v, want %v", err, resolverErr)
	}
	var nilPublisher *Publisher
	if _, err := nilPublisher.resolveCredential(context.Background(), repositoryRef{}); err == nil {
		t.Fatal("nil publisher resolved a credential")
	}
}

func TestPublisherHelpersAndBoundaries(t *testing.T) {
	t.Run("new applies defaults and preserves options", func(t *testing.T) {
		defaults := New(Options{})
		if defaults.apiBaseURL != defaultAPIBaseURL || defaults.apiVersion != defaultAPIVersion ||
			!defaults.deriveAPIBase || defaults.client == nil || defaults.client.Timeout != 30*time.Second {
			t.Fatalf("default publisher = %#v", defaults)
		}
		client := &http.Client{}
		configured := New(Options{
			Resolver:   testCredentialResolver(),
			APIBaseURL: "https://github.example/api/v3",
			APIVersion: "configured",
			Timeout:    time.Second,
			HTTPClient: client,
		})
		if configured.client != client || configured.apiBaseURL != "https://github.example/api/v3" ||
			configured.deriveAPIBase || configured.apiVersion != "configured" {
			t.Fatalf("configured publisher = %#v", configured)
		}
	})

	t.Run("parses API URLs and repository remotes", func(t *testing.T) {
		api, err := parseAPIBaseURL("https://github.example/api/v3")
		if err != nil || api.Path != "/api/v3" {
			t.Fatalf("parseAPIBaseURL() = (%#v, %v)", api, err)
		}
		for _, invalid := range []string{"", "http://github.example", "https://user@github.example", "https://github.example?x=y", "https://github.example#fragment"} {
			if _, err := parseAPIBaseURL(invalid); err == nil {
				t.Fatalf("parseAPIBaseURL(%q) unexpectedly succeeded", invalid)
			}
		}
		for _, raw := range []string{
			"git@github.com:acme/governance.git",
			"https://github.com/acme/governance.git",
			"ssh://git@github.com/acme/governance.git",
			"git://github.com/acme/governance.git",
		} {
			repository, err := parseRepositoryRemote(raw)
			if err != nil || repository != (repositoryRef{host: "github.com", owner: "acme", name: "governance"}) {
				t.Fatalf("parseRepositoryRemote(%q) = (%#v, %v)", raw, repository, err)
			}
		}
		for _, invalid := range []string{"git@github.com", "https://github.com/acme", "file:///repository", "https://github.com/acme/.git"} {
			if _, err := parseRepositoryRemote(invalid); err == nil {
				t.Fatalf("parseRepositoryRemote(%q) unexpectedly succeeded", invalid)
			}
		}
	})

	t.Run("constructs escaped endpoints and compares hosts", func(t *testing.T) {
		base, err := url.Parse("https://github.example/api/v3/")
		if err != nil {
			t.Fatal(err)
		}
		query := url.Values{"base": {"develop"}, "head": {"acme:feature/ABC-123-add-export"}}
		endpoint := repositoryEndpoint(base, repositoryRef{owner: "acme", name: "governance"}, "pulls", query)
		if endpoint.Path != "/api/v3/repos/acme/governance/pulls" || endpoint.Query().Get("head") == "" {
			t.Fatalf("endpoint = %s", endpoint)
		}
		if expectedGitHost("api.github.com") != "github.com" ||
			expectedGitHost("github.example") != "github.example" ||
			apiBaseURLForGitHost("github.com") != defaultAPIBaseURL ||
			apiBaseURLForGitHost("github.enterprise.example") != "https://github.enterprise.example/api/v3" ||
			!sameHost("GitHub.COM", "github.com") || sameHost("github.com", "gitlab.com") {
			t.Fatal("host helpers returned unexpected results")
		}
		if !validRepositorySegment("repository-name") || validRepositorySegment(" ") || validRepositorySegment("..") {
			t.Fatal("repository segment validation is incorrect")
		}
	})

	t.Run("bounds and classifies response decoding", func(t *testing.T) {
		var value map[string]string
		if err := decodeResponse(bytes.NewBufferString(`{"ok":"true"}`), &value); err != nil || value["ok"] != "true" {
			t.Fatalf("decodeResponse() = (%#v, %v)", value, err)
		}
		if err := decodeResponse(bytes.NewBufferString(strings.Repeat("x", maxResponseBytes+1)), &value); err == nil {
			t.Fatal("oversized response was accepted")
		}
		if err := decodeResponse(errReader{}, &value); err == nil {
			t.Fatal("reader failure was accepted")
		}
		cause := errors.New("decode failure")
		if err := responseDecodeProblem("response", cause); !errors.Is(err, cause) {
			t.Fatalf("responseDecodeProblem() = %v, want wrapped cause", err)
		}
		assertProblem(t, responseProblem(http.StatusTooManyRequests, "retry"), problem.CodeExternalCommandFailed)
		assertProblem(t, configurationProblem("field", "expected", "fix it"), problem.CodeConfigurationInvalid)
	})

	t.Run("covers request construction and create transport failures", func(t *testing.T) {
		base, err := url.Parse("https://github.com")
		if err != nil {
			t.Fatal(err)
		}
		publisher := New(Options{
			Resolver: testCredentialResolver(),
			HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("post unavailable")
			})},
		})
		_, err = publisher.createPullRequest(context.Background(), base, repositoryRef{
			host:  "github.com",
			owner: "acme",
			name:  "governance",
		}, testPublication("https://github.com", false).PullRequest)
		assertProblem(t, err, problem.CodeExternalCommandFailed)

		_, err = publisher.request(context.Background(), repositoryRef{}, "\n", base, nil)
		assertProblem(t, err, problem.CodeConfigurationInvalid)
	})
}

func testPublication(apiURL string, draft bool) port.PullRequestPublication {
	source, _ := branch.ParseName("feature/ABC-123-add-export")
	target, _ := branch.ParseName("develop")
	parsed, _ := url.Parse(apiURL)
	return port.PullRequestPublication{
		Repository: port.RepositoryIdentity{Root: "C:/repository", Remote: "origin"},
		RemoteURL:  "https://" + parsed.Hostname() + "/acme/governance.git",
		PullRequest: port.PullRequest{
			Source: source,
			Target: target,
			Title:  "ABC-123: add export",
			Body:   "Summary: Add the export button.\n\nScope and Non-Goals: The export button only.\n\nCommit Series:\n- feat(ABC-123): add export button\n\nRisk and Rollback: Low; revert the commit.\n\nVerification and Review Focus: Unit tests; review the button wiring.",
			Draft:  draft,
		},
	}
}

func assertProblem(t *testing.T, err error, code problem.Code) {
	t.Helper()
	value, ok := problem.As(err)
	if !ok || value.Code != code {
		t.Fatalf("problem = %#v, want code %q", err, code)
	}
}

func assertNoPublisherResponseLeak(t *testing.T, err error, value *problem.Problem) {
	t.Helper()

	for _, output := range []string{
		err.Error(),
		value.Actual,
		value.Context,
		value.Diagnostic,
		value.Expected,
		value.Rule,
		value.Remediation,
	} {
		if strings.Contains(output, "secret-token") {
			t.Fatalf("publisher response leaked sensitive content in %q", output)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

type fakeCredentialResolver struct {
	token   string
	err     error
	targets []CredentialTarget
}

func testCredentialResolver() *fakeCredentialResolver {
	return &fakeCredentialResolver{token: "token"}
}

func (resolver *fakeCredentialResolver) Resolve(_ context.Context, target CredentialTarget) (string, error) {
	resolver.targets = append(resolver.targets, target)
	if resolver.err != nil {
		return "", resolver.err
	}
	return resolver.token, nil
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("read failure")
}
