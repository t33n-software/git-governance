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
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestPublisherDispatchSharedLine(t *testing.T) {
	t.Run("dispatches correlated workflow and waits for success", func(t *testing.T) {
		var requestID string
		runs := 0
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/actions/workflows/execute-protected-line-request.yml/dispatches":
				if request.Method != http.MethodPost {
					t.Fatalf("dispatch method = %s", request.Method)
				}
				var payload workflowDispatchRequest
				if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if payload.Ref != "main" || payload.Inputs["kind"] != "release" || payload.Inputs["version"] != "2.8.0" {
					t.Fatalf("dispatch payload = %#v", payload)
				}
				requestID = payload.Inputs["request_id"]
				if requestID == "" {
					t.Fatal("dispatch did not provide a request ID")
				}
				writer.WriteHeader(http.StatusNoContent)
			case "/repos/acme/governance/actions/workflows/execute-protected-line-request.yml/runs":
				runs++
				if request.URL.Query().Get("event") != "workflow_dispatch" || request.URL.Query().Get("per_page") != "100" {
					t.Fatalf("workflow query = %q", request.URL.RawQuery)
				}
				response := workflowRunsResponse{
					WorkflowRuns: []workflowRunResponse{
						{Status: "completed", Conclusion: "success", DisplayTitle: "unrelated workflow"},
						{
							Status:       "completed",
							Conclusion:   "success",
							HTMLURL:      "https://github.example/actions/runs/42",
							DisplayTitle: "Create release line 2.8.0 (" + requestID + ")",
						},
					},
				}
				_ = json.NewEncoder(writer).Encode(response)
			default:
				t.Fatalf("unexpected path %q", request.URL.Path)
			}
		}))
		defer server.Close()

		release, _ := branch.ParseName("release/2.8.0")
		publisher := New(Options{
			Resolver:   testCredentialResolver(),
			APIBaseURL: server.URL,
			HTTPClient: server.Client(),
		})
		result, err := publisher.DispatchSharedLine(context.Background(), port.SharedLineDispatchRequest{
			Repository: port.RepositoryIdentity{Root: "C:/repository", Remote: "origin"},
			RemoteURL:  "https://" + server.URL[len("https://"):] + "/acme/governance.git",
			Workflow:   "execute-protected-line-request.yml",
			Ref:        "main",
			Inputs:     map[string]string{"kind": "release", "version": "2.8.0"},
			Branch:     release,
		})
		if err != nil || result.Branch != release || result.WorkflowRunURL != "https://github.example/actions/runs/42" || runs != 1 {
			t.Fatalf("DispatchSharedLine() = (%#v, %v), runs=%d", result, err, runs)
		}
	})

	t.Run("accepts an OK dispatch response when the correlated workflow succeeds", func(t *testing.T) {
		var requestID string
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/actions/workflows/execute-protected-line-request.yml/dispatches":
				var payload workflowDispatchRequest
				if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				requestID = payload.Inputs["request_id"]
				writer.WriteHeader(http.StatusOK)
			case "/repos/acme/governance/actions/workflows/execute-protected-line-request.yml/runs":
				_ = json.NewEncoder(writer).Encode(workflowRunsResponse{
					WorkflowRuns: []workflowRunResponse{{
						Status:       "completed",
						Conclusion:   "success",
						HTMLURL:      "https://github.example/actions/runs/44",
						DisplayTitle: requestID,
					}},
				})
			default:
				t.Fatalf("unexpected path %q", request.URL.Path)
			}
		}))
		defer server.Close()

		release, _ := branch.ParseName("release/2.8.0")
		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		result, err := publisher.DispatchSharedLine(context.Background(), port.SharedLineDispatchRequest{
			RemoteURL: "https://" + server.URL[len("https://"):] + "/acme/governance.git",
			Workflow:  "execute-protected-line-request.yml",
			Ref:       "main",
			Branch:    release,
		})
		if err != nil || result.WorkflowRunURL != "https://github.example/actions/runs/44" || requestID == "" {
			t.Fatalf("DispatchSharedLine() = (%#v, %v), requestID=%q", result, err, requestID)
		}
	})

	t.Run("waits through an in-progress workflow", func(t *testing.T) {
		var requestID string
		calls := 0
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if strings.HasSuffix(request.URL.Path, "/dispatches") {
				var payload workflowDispatchRequest
				if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				requestID = payload.Inputs["request_id"]
				writer.WriteHeader(http.StatusNoContent)
				return
			}
			calls++
			status := "in_progress"
			conclusion := ""
			if calls > 1 {
				status = "completed"
				conclusion = "success"
			}
			_ = json.NewEncoder(writer).Encode(workflowRunsResponse{
				WorkflowRuns: []workflowRunResponse{{
					Status:       status,
					Conclusion:   conclusion,
					HTMLURL:      "https://github.example/actions/runs/43",
					DisplayTitle: requestID,
				}},
			})
		}))
		defer server.Close()

		release, _ := branch.ParseName("release/2.8.0")
		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		_, err := publisher.DispatchSharedLine(context.Background(), port.SharedLineDispatchRequest{
			RemoteURL: "https://" + server.URL[len("https://"):] + "/acme/governance.git",
			Workflow:  "execute-protected-line-request.yml",
			Ref:       "main",
			Branch:    release,
		})
		if err != nil || calls != 2 {
			t.Fatalf("DispatchSharedLine() error = %v, calls=%d", err, calls)
		}
	})

	t.Run("rejects invalid requests and dispatch failures", func(t *testing.T) {
		release, _ := branch.ParseName("release/2.8.0")
		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: defaultAPIBaseURL})
		_, err := publisher.DispatchSharedLine(context.Background(), port.SharedLineDispatchRequest{
			RemoteURL: "https://github.com/acme/governance.git",
			Workflow:  "../workflow.yml",
			Ref:       "main",
			Branch:    release,
		})
		assertProblem(t, err, problem.CodeConfigurationInvalid)

		original := releaseRequestIDGenerator
		releaseRequestIDGenerator = func() (string, error) { return "", errors.New("random unavailable") }
		t.Cleanup(func() { releaseRequestIDGenerator = original })
		_, err = publisher.DispatchSharedLine(context.Background(), port.SharedLineDispatchRequest{
			RemoteURL: "https://github.com/acme/governance.git",
			Workflow:  "execute-protected-line-request.yml",
			Ref:       "main",
			Branch:    release,
		})
		assertProblem(t, err, problem.CodeExternalCommandFailed)

		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(http.StatusForbidden)
		}))
		defer server.Close()
		releaseRequestIDGenerator = original
		publisher = New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		_, err = publisher.DispatchSharedLine(context.Background(), port.SharedLineDispatchRequest{
			RemoteURL: "https://" + server.URL[len("https://"):] + "/acme/governance.git",
			Workflow:  "execute-protected-line-request.yml",
			Ref:       "main",
			Branch:    release,
		})
		assertProblem(t, err, problem.CodeExternalCommandFailed)
	})

	t.Run("rejects failed workflow results", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if strings.HasSuffix(request.URL.Path, "/dispatches") {
				writer.WriteHeader(http.StatusNoContent)
				return
			}
			_ = json.NewEncoder(writer).Encode(workflowRunsResponse{
				WorkflowRuns: []workflowRunResponse{{
					Status:       "completed",
					Conclusion:   "failure",
					DisplayTitle: strings.Repeat("a", 24),
				}},
			})
		}))
		defer server.Close()

		original := releaseRequestIDGenerator
		releaseRequestIDGenerator = func() (string, error) { return strings.Repeat("a", 24), nil }
		t.Cleanup(func() { releaseRequestIDGenerator = original })
		release, _ := branch.ParseName("release/2.8.0")
		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		_, err := publisher.DispatchSharedLine(context.Background(), port.SharedLineDispatchRequest{
			RemoteURL: "https://" + server.URL[len("https://"):] + "/acme/governance.git",
			Workflow:  "execute-protected-line-request.yml",
			Ref:       "main",
			Branch:    release,
		})
		assertProblem(t, err, problem.CodeConfigurationInvalid)
	})
}

func TestPublisherVerifyReleaseReconciliation(t *testing.T) {
	t.Run("proves delivery and an effective delta", func(t *testing.T) {
		server := lifecycleServer(t, true, true)
		defer server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		release, _ := branch.ParseName("release/2.8.0")
		result, err := publisher.VerifyReleaseReconciliation(context.Background(), port.ReleaseReconciliationRequest{
			RemoteURL: "https://" + server.URL[len("https://"):] + "/acme/governance.git",
			Release:   release,
		})
		if err != nil || !result.EffectiveDelta || result.PromotionMergeCommit != "merge-sha" ||
			result.Tag != "v2.8.0" || result.ReleaseURL != "https://github.example/releases/v2.8.0" {
			t.Fatalf("VerifyReleaseReconciliation() = (%#v, %v)", result, err)
		}
	})

	t.Run("reports no effective delta for content-equivalent release history", func(t *testing.T) {
		server := lifecycleServer(t, false, false)
		defer server.Close()

		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		release, _ := branch.ParseName("release/2.8.0")
		result, err := publisher.VerifyReleaseReconciliation(context.Background(), port.ReleaseReconciliationRequest{
			RemoteURL: "https://" + server.URL[len("https://"):] + "/acme/governance.git",
			Release:   release,
		})
		if err != nil || result.EffectiveDelta {
			t.Fatalf("VerifyReleaseReconciliation() = (%#v, %v)", result, err)
		}
	})

	t.Run("rejects incomplete delivery evidence", func(t *testing.T) {
		release, _ := branch.ParseName("release/2.8.0")
		publisher := New(Options{Resolver: testCredentialResolver()})
		main, _ := branch.ParseName("main")
		_, err := publisher.VerifyReleaseReconciliation(context.Background(), port.ReleaseReconciliationRequest{
			RemoteURL: "https://github.com/acme/governance.git",
			Release:   main,
		})
		assertProblem(t, err, problem.CodeConfigurationInvalid)

		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch {
			case request.URL.Path == "/repos/acme/governance/pulls":
				_ = json.NewEncoder(writer).Encode([]releasePullRequestResponse{})
			default:
				t.Fatalf("unexpected path %q", request.URL.Path)
			}
		}))
		defer server.Close()
		publisher = New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		_, err = publisher.VerifyReleaseReconciliation(context.Background(), port.ReleaseReconciliationRequest{
			RemoteURL: "https://" + server.URL[len("https://"):] + "/acme/governance.git",
			Release:   release,
		})
		assertProblem(t, err, problem.CodeConfigurationInvalid)
	})
}

func TestPublisherMergedPromotion(t *testing.T) {
	t.Run("finds the matching merged promotion across pages without a provider head filter", func(t *testing.T) {
		firstPage := make([]releasePullRequestResponse, releasePromotionPageSize)
		for index := range firstPage {
			firstPage[index] = releasePromotionResponseForTest(index+1, "release/other")
		}
		promotion := releasePromotionResponseForTest(30, "release/2.8.0")
		server := httptest.NewTLSServer(withGraphQLPromotion(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/repos/acme/governance/pulls" {
				t.Fatalf("unexpected path %q", request.URL.Path)
			}
			query := request.URL.Query()
			if query.Get("base") != "main" || query.Get("state") != "closed" ||
				query.Get("per_page") != "100" || query.Get("head") != "" {
				t.Fatalf("promotion query = %q", request.URL.RawQuery)
			}
			switch query.Get("page") {
			case "1":
				_ = json.NewEncoder(writer).Encode(firstPage)
			case "2":
				_ = json.NewEncoder(writer).Encode([]releasePullRequestResponse{promotion})
			default:
				t.Fatalf("unexpected promotion page %q", query.Get("page"))
			}
		}), promotion))
		defer server.Close()

		base, _ := url.Parse(server.URL)
		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		promotion, err := publisher.mergedPromotion(
			context.Background(),
			base,
			repositoryRef{host: "github.com", owner: "acme", name: "governance"},
			"release/2.8.0",
		)
		if err != nil || promotion.Number != 30 || promotion.MergeCommitSHA != "merge-sha" ||
			promotion.HTMLURL != "https://github.example/pull/30" {
			t.Fatalf("mergedPromotion() = (%#v, %v)", promotion, err)
		}
	})

	t.Run("rejects incomplete or mismatched provider evidence without credential details", func(t *testing.T) {
		candidates := []releasePullRequestResponse{
			releasePromotionResponseForTest(1, "release/2.8.0"),
			releasePromotionResponseForTest(2, "release/2.8.0"),
			releasePromotionResponseForTest(3, "release/other"),
			releasePromotionResponseForTest(4, "release/2.8.0"),
			releasePromotionResponseForTest(5, "release/2.8.0"),
			releasePromotionResponseForTest(6, "release/2.8.0"),
			releasePromotionResponseForTest(7, "release/2.8.0"),
		}
		candidates[0].Base.Ref = "develop"
		candidates[1].Base.Repository.FullName = "acme/other"
		candidates[3].Head.Repository.FullName = "acme/fork"
		candidates[4].MergedAt = nil
		candidates[5].Number = 0
		candidates[6].HTMLURL = ""

		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/repos/acme/governance/pulls" || request.URL.Query().Get("page") != "1" {
				t.Fatalf("unexpected promotion request %q", request.URL.String())
			}
			_ = json.NewEncoder(writer).Encode(candidates)
		}))
		defer server.Close()

		base, _ := url.Parse(server.URL)
		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		_, err := publisher.mergedPromotion(
			context.Background(),
			base,
			repositoryRef{host: "github.com", owner: "acme", name: "governance"},
			"release/2.8.0",
		)
		value, ok := problem.As(err)
		if !ok || value.Code != problem.CodeConfigurationInvalid ||
			value.Actual != "no merged release/2.8.0 -> main pull request with complete provider evidence" ||
			strings.Contains(strings.ToLower(value.Actual), "token") {
			t.Fatalf("mergedPromotion() problem = %#v", err)
		}
	})
}

func TestPublisherPromotionMergeCommit(t *testing.T) {
	repository := repositoryRef{host: "github.com", owner: "acme", name: "governance"}
	promotion := releasePromotionResponseForTest(30, "release/2.8.0")

	t.Run("queries GraphQL when the REST payload omits merge evidence", func(t *testing.T) {
		graphQLCalls := 0
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/pulls":
				_ = json.NewEncoder(writer).Encode([]releasePullRequestResponse{{
					Number:   promotion.Number,
					HTMLURL:  promotion.HTMLURL,
					MergedAt: promotion.MergedAt,
					Base:     promotion.Base,
					Head:     promotion.Head,
				}})
			case "/graphql":
				graphQLCalls++
				if request.Method != http.MethodPost {
					t.Fatalf("GraphQL method = %s", request.Method)
				}
				var payload graphQLPromotionRequest
				if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if payload.Query != graphQLPromotionQuery ||
					payload.Variables != (graphQLPromotionVariables{Owner: "acme", Name: "governance", Number: 30}) {
					t.Fatalf("GraphQL payload = %#v", payload)
				}
				writeGraphQLPromotion(writer, promotion)
			default:
				t.Fatalf("unexpected path %q", request.URL.Path)
			}
		}))
		defer server.Close()

		base, _ := url.Parse(server.URL)
		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		merged, err := publisher.mergedPromotion(context.Background(), base, repository, "release/2.8.0")
		if err != nil || merged.MergeCommitSHA != "merge-sha" || graphQLCalls != 1 {
			t.Fatalf("mergedPromotion() = (%#v, %v), GraphQL calls=%d", merged, err, graphQLCalls)
		}
	})

	t.Run("propagates GraphQL evidence failure from a matching REST promotion", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/pulls":
				_ = json.NewEncoder(writer).Encode([]releasePullRequestResponse{{
					Number:   promotion.Number,
					HTMLURL:  promotion.HTMLURL,
					MergedAt: promotion.MergedAt,
					Base:     promotion.Base,
					Head:     promotion.Head,
				}})
			case "/graphql":
				writer.WriteHeader(http.StatusForbidden)
			default:
				t.Fatalf("unexpected path %q", request.URL.Path)
			}
		}))
		defer server.Close()

		base, _ := url.Parse(server.URL)
		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		if _, err := publisher.mergedPromotion(context.Background(), base, repository, "release/2.8.0"); err == nil {
			t.Fatal("GraphQL merge-evidence failure was accepted")
		}
	})

	testCases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "status",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusForbidden)
			},
		},
		{
			name: "malformed response",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = writer.Write([]byte("{"))
			},
		},
		{
			name: "GraphQL errors",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(writer).Encode(graphQLPromotionResponse{
					Errors: []graphQLErrorResponse{{Message: "provider failure"}},
				})
			},
		},
		{
			name: "missing merge commit",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				response := graphQLPromotionResponseForTest(promotion)
				response.Data.Repository.PullRequest.MergeCommit = nil
				_ = json.NewEncoder(writer).Encode(response)
			},
		},
		{
			name: "mismatched promotion",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				response := graphQLPromotionResponseForTest(promotion)
				response.Data.Repository.PullRequest.HeadRefName = "release/other"
				_ = json.NewEncoder(writer).Encode(response)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewTLSServer(testCase.handler)
			defer server.Close()

			base, _ := url.Parse(server.URL)
			publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
			if _, err := publisher.promotionMergeCommit(context.Background(), base, repository, "release/2.8.0", promotion); err == nil {
				t.Fatal("incomplete GraphQL promotion evidence was accepted")
			}
		})
	}

	t.Run("propagates GraphQL transport failures", func(t *testing.T) {
		publisher := New(Options{
			Resolver: testCredentialResolver(),
			HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("GraphQL unavailable")
			})},
		})
		base, _ := url.Parse(defaultAPIBaseURL)
		if _, err := publisher.promotionMergeCommit(context.Background(), base, repository, "release/2.8.0", promotion); err == nil {
			t.Fatal("GraphQL transport failure was accepted")
		}
	})
}

func TestReleaseLifecycleHelpersAndFailures(t *testing.T) {
	t.Run("validates lifecycle targets and helper inputs", func(t *testing.T) {
		var nilPublisher *Publisher
		if _, _, err := nilPublisher.lifecycleTarget("https://github.com/acme/governance.git"); err == nil {
			t.Fatal("nil lifecycle publisher was accepted")
		}
		if _, _, err := New(Options{}).lifecycleTarget("https://github.com/acme/governance.git"); err == nil {
			t.Fatal("publisher without resolver was accepted")
		}
		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: defaultAPIBaseURL})
		if _, _, err := publisher.lifecycleTarget("https://gitlab.example/acme/governance.git"); err == nil {
			t.Fatal("mismatched lifecycle host was accepted")
		}
		if !validWorkflowFile("publish-release-artifacts.yml") || validWorkflowFile("") || validWorkflowFile("publish-release-artifacts.yaml") ||
			validWorkflowFile("../publish-release-artifacts.yml") || validWorkflowFile("publish-release-artifacts.yml\n") {
			t.Fatal("workflow file validation is incorrect")
		}
		for _, testCase := range []struct {
			status int
			want   bool
		}{
			{status: http.StatusContinue, want: false},
			{status: http.StatusOK, want: true},
			{status: http.StatusAccepted, want: true},
			{status: http.StatusNoContent, want: true},
			{status: http.StatusMultipleChoices - 1, want: true},
			{status: http.StatusMultipleChoices, want: false},
		} {
			if got := isSuccessfulHTTPStatus(testCase.status); got != testCase.want {
				t.Fatalf("isSuccessfulHTTPStatus(%d) = %t, want %t", testCase.status, got, testCase.want)
			}
		}
		requestID, err := newReleaseRequestID()
		if err != nil || len(requestID) != 24 {
			t.Fatalf("newReleaseRequestID() = (%q, %v)", requestID, err)
		}
		originalReader := releaseRandomReader
		releaseRandomReader = errReader{}
		t.Cleanup(func() { releaseRandomReader = originalReader })
		if _, err := newReleaseRequestID(); err == nil {
			t.Fatal("newReleaseRequestID accepted a failing random reader")
		}
		assertProblem(t, lifecycleResponseProblem(http.StatusForbidden, "retry"), problem.CodeExternalCommandFailed)
		assertProblem(t, lifecycleConfigurationProblem("field", "expected", "fix"), problem.CodeConfigurationInvalid)
		assertProblem(t, lifecycleExternalProblem("retry", errors.New("network")), problem.CodeExternalCommandFailed)
	})

	t.Run("classifies lifecycle HTTP, decoding, tag, release, and comparison failures", func(t *testing.T) {
		repository := repositoryRef{host: "github.com", owner: "acme", name: "governance"}
		release, _ := branch.ParseName("release/2.8.0")
		testCases := []struct {
			name string
			run  func(*Publisher, *url.URL) error
		}{
			{
				name: "promotion status",
				run: func(publisher *Publisher, base *url.URL) error {
					_, err := publisher.mergedPromotion(context.Background(), base, repository, release.String())
					return err
				},
			},
			{
				name: "tag status",
				run: func(publisher *Publisher, base *url.URL) error {
					_, err := publisher.tagCommit(context.Background(), base, repository, "v2.8.0")
					return err
				},
			},
			{
				name: "release status",
				run: func(publisher *Publisher, base *url.URL) error {
					_, err := publisher.publishedReleaseURL(context.Background(), base, repository, "v2.8.0")
					return err
				},
			},
			{
				name: "compare status",
				run: func(publisher *Publisher, base *url.URL) error {
					_, err := publisher.hasEffectiveReleaseDelta(context.Background(), base, repository, release.String())
					return err
				},
			},
		}
		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					writer.WriteHeader(http.StatusForbidden)
				}))
				defer server.Close()
				base, _ := url.Parse(server.URL)
				publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
				assertProblem(t, testCase.run(publisher, base), problem.CodeExternalCommandFailed)
			})
		}

		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/git/ref/tags/v2.8.0":
				_ = json.NewEncoder(writer).Encode(gitReferenceResponse{Object: gitObjectReference{SHA: "not-a-commit", Type: "tree"}})
			case "/repos/acme/governance/releases/tags/v2.8.0":
				_ = json.NewEncoder(writer).Encode(releaseResponse{Draft: true})
			case "/repos/acme/governance/compare/develop...release/2.8.0":
				_, _ = writer.Write([]byte("{"))
			default:
				t.Fatalf("unexpected path %q", request.URL.Path)
			}
		}))
		defer server.Close()
		base, _ := url.Parse(server.URL)
		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		if _, err := publisher.tagCommit(context.Background(), base, repository, "v2.8.0"); err == nil {
			t.Fatal("non-commit tag was accepted")
		}
		if _, err := publisher.publishedReleaseURL(context.Background(), base, repository, "v2.8.0"); err == nil {
			t.Fatal("draft release was accepted")
		}
		if _, err := publisher.hasEffectiveReleaseDelta(context.Background(), base, repository, release.String()); err == nil {
			t.Fatal("malformed comparison was accepted")
		}
	})
}

func TestReleaseLifecycleTransportAndResponseFailurePaths(t *testing.T) {
	release, _ := branch.ParseName("release/2.8.0")
	repository := repositoryRef{host: "github.com", owner: "acme", name: "governance"}

	t.Run("covers target and dispatch configuration failures", func(t *testing.T) {
		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: defaultAPIBaseURL})
		if _, _, err := publisher.lifecycleTarget(""); err == nil {
			t.Fatal("empty lifecycle remote was accepted")
		}
		invalidAPI := New(Options{Resolver: testCredentialResolver(), APIBaseURL: "http://github.com"})
		if _, _, err := invalidAPI.lifecycleTarget("https://github.com/acme/governance.git"); err == nil {
			t.Fatal("invalid lifecycle API URL was accepted")
		}
		_, err := publisher.DispatchSharedLine(context.Background(), port.SharedLineDispatchRequest{
			Workflow: "execute-protected-line-request.yml",
			Ref:      "main",
			Branch:   release,
		})
		assertProblem(t, err, problem.CodeConfigurationInvalid)
	})

	t.Run("covers dispatch and wait transport, response, decoding, and timeout failures", func(t *testing.T) {
		transportPublisher := New(Options{
			Resolver: testCredentialResolver(),
			HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("network unavailable")
			})},
		})
		_, err := transportPublisher.DispatchSharedLine(context.Background(), port.SharedLineDispatchRequest{
			RemoteURL: "https://github.com/acme/governance.git",
			Workflow:  "execute-protected-line-request.yml",
			Ref:       "main",
			Branch:    release,
		})
		assertProblem(t, err, problem.CodeExternalCommandFailed)

		testCases := []struct {
			name    string
			handler http.HandlerFunc
		}{
			{
				name: "workflow run status",
				handler: func(writer http.ResponseWriter, request *http.Request) {
					writer.WriteHeader(http.StatusForbidden)
				},
			},
			{
				name: "workflow run malformed response",
				handler: func(writer http.ResponseWriter, request *http.Request) {
					_, _ = writer.Write([]byte("{"))
				},
			},
		}
		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				server := httptest.NewTLSServer(testCase.handler)
				defer server.Close()
				base, _ := url.Parse(server.URL)
				publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
				_, err := publisher.waitForWorkflowRun(context.Background(), base, repository, "execute-protected-line-request.yml", "request")
				assertProblem(t, err, problem.CodeExternalCommandFailed)
			})
		}

		statusPublisher := New(Options{
			Resolver: testCredentialResolver(),
			HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusForbidden,
					Body:       io.NopCloser(strings.NewReader(`{}`)),
					Header:     make(http.Header),
				}, nil
			})},
		})
		statusBase, _ := url.Parse(defaultAPIBaseURL)
		_, err = statusPublisher.waitForWorkflowRun(context.Background(), statusBase, repository, "execute-protected-line-request.yml", "request")
		assertProblem(t, err, problem.CodeExternalCommandFailed)

		cancelContext, cancel := context.WithCancel(context.Background())
		cancelPublisher := New(Options{
			Resolver: testCredentialResolver(),
			HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				cancel()
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(
						`{"workflow_runs":[{"status":"completed","conclusion":"success","display_title":"other"}]}`,
					)),
					Header: make(http.Header),
				}, nil
			}), Timeout: time.Millisecond},
		})
		_, err = cancelPublisher.waitForWorkflowRun(cancelContext, statusBase, repository, "execute-protected-line-request.yml", "request")
		assertProblem(t, err, problem.CodeExternalCommandFailed)
	})

	t.Run("covers promotion, tag, release, and comparison decoding paths", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/pulls":
				_, _ = writer.Write([]byte("{"))
			case "/repos/acme/governance/git/ref/tags/v2.8.0":
				_, _ = writer.Write([]byte("{"))
			case "/repos/acme/governance/releases/tags/v2.8.0":
				_, _ = writer.Write([]byte("{"))
			case "/repos/acme/governance/compare/develop...release/2.8.0":
				_, _ = writer.Write([]byte("{"))
			default:
				t.Fatalf("unexpected path %q", request.URL.Path)
			}
		}))
		defer server.Close()
		base, _ := url.Parse(server.URL)
		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		if _, err := publisher.mergedPromotion(context.Background(), base, repository, release.String()); err == nil {
			t.Fatal("malformed promotion response was accepted")
		}
		if _, err := publisher.tagCommit(context.Background(), base, repository, "v2.8.0"); err == nil {
			t.Fatal("malformed tag response was accepted")
		}
		if _, err := publisher.publishedReleaseURL(context.Background(), base, repository, "v2.8.0"); err == nil {
			t.Fatal("malformed release response was accepted")
		}
		if _, err := publisher.hasEffectiveReleaseDelta(context.Background(), base, repository, release.String()); err == nil {
			t.Fatal("malformed comparison response was accepted")
		}
	})

	t.Run("covers annotated tag and published-release failure branches", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/git/ref/tags/v2.8.0":
				_ = json.NewEncoder(writer).Encode(gitReferenceResponse{
					Object: gitObjectReference{SHA: "annotated-tag", Type: "tag"},
				})
			case "/repos/acme/governance/git/tags/annotated-tag":
				writer.WriteHeader(http.StatusForbidden)
			case "/repos/acme/governance/releases/tags/v2.8.0":
				_ = json.NewEncoder(writer).Encode(releaseResponse{})
			default:
				t.Fatalf("unexpected path %q", request.URL.Path)
			}
		}))
		defer server.Close()
		base, _ := url.Parse(server.URL)
		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		if _, err := publisher.tagCommit(context.Background(), base, repository, "v2.8.0"); err == nil {
			t.Fatal("failed annotated tag resolution was accepted")
		}
		if _, err := publisher.publishedReleaseURL(context.Background(), base, repository, "v2.8.0"); err == nil {
			t.Fatal("release without URL was accepted")
		}
	})

	t.Run("propagates an annotated tag lookup transport failure", func(t *testing.T) {
		calls := 0
		publisher := New(Options{
			Resolver: testCredentialResolver(),
			HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body: io.NopCloser(strings.NewReader(
							`{"object":{"sha":"annotated-tag","type":"tag"}}`,
						)),
						Header: make(http.Header),
					}, nil
				}
				return nil, errors.New("annotated tag lookup unavailable")
			})},
		})
		base, _ := url.Parse(defaultAPIBaseURL)
		_, err := publisher.tagCommit(
			context.Background(),
			base,
			repositoryRef{host: "github.com", owner: "acme", name: "governance"},
			"v2.8.0",
		)
		assertProblem(t, err, problem.CodeExternalCommandFailed)
	})
}

func TestReleaseLifecycleVerificationFailurePropagation(t *testing.T) {
	release, _ := branch.ParseName("release/2.8.0")
	main, _ := branch.ParseName("main")

	t.Run("rejects lifecycle target and branch failures before provider calls", func(t *testing.T) {
		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: defaultAPIBaseURL})
		_, err := publisher.VerifyReleaseReconciliation(context.Background(), port.ReleaseReconciliationRequest{
			Release: release,
		})
		assertProblem(t, err, problem.CodeConfigurationInvalid)

		server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("invalid branch must not call the provider")
		}))
		defer server.Close()
		publisher = New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		_, err = publisher.VerifyReleaseReconciliation(context.Background(), port.ReleaseReconciliationRequest{
			RemoteURL: "https://" + server.URL[len("https://"):] + "/acme/governance.git",
			Release:   main,
		})
		assertProblem(t, err, problem.CodeConfigurationInvalid)
	})

	t.Run("propagates promotion, tag, tag-match, release, and comparison failures", func(t *testing.T) {
		testCases := []struct {
			name    string
			handler http.HandlerFunc
			code    problem.Code
		}{
			{
				name: "promotion",
				code: problem.CodeExternalCommandFailed,
				handler: func(writer http.ResponseWriter, request *http.Request) {
					writer.WriteHeader(http.StatusForbidden)
				},
			},
			{
				name: "tag",
				code: problem.CodeExternalCommandFailed,
				handler: func(writer http.ResponseWriter, request *http.Request) {
					switch request.URL.Path {
					case "/repos/acme/governance/pulls":
						writeMergedPromotion(writer)
					default:
						writer.WriteHeader(http.StatusForbidden)
					}
				},
			},
			{
				name: "tag does not match promotion",
				code: problem.CodeConfigurationInvalid,
				handler: func(writer http.ResponseWriter, request *http.Request) {
					switch request.URL.Path {
					case "/repos/acme/governance/pulls":
						writeMergedPromotion(writer)
					case "/repos/acme/governance/git/ref/tags/v2.8.0":
						_ = json.NewEncoder(writer).Encode(gitReferenceResponse{
							Object: gitObjectReference{SHA: "different-merge", Type: "commit"},
						})
					default:
						t.Fatalf("unexpected path %q", request.URL.Path)
					}
				},
			},
			{
				name: "published release",
				code: problem.CodeExternalCommandFailed,
				handler: func(writer http.ResponseWriter, request *http.Request) {
					switch request.URL.Path {
					case "/repos/acme/governance/pulls":
						writeMergedPromotion(writer)
					case "/repos/acme/governance/git/ref/tags/v2.8.0":
						writeReleaseTagCommit(writer)
					default:
						writer.WriteHeader(http.StatusForbidden)
					}
				},
			},
			{
				name: "comparison",
				code: problem.CodeExternalCommandFailed,
				handler: func(writer http.ResponseWriter, request *http.Request) {
					switch request.URL.Path {
					case "/repos/acme/governance/pulls":
						writeMergedPromotion(writer)
					case "/repos/acme/governance/git/ref/tags/v2.8.0":
						writeReleaseTagCommit(writer)
					case "/repos/acme/governance/releases/tags/v2.8.0":
						_ = json.NewEncoder(writer).Encode(releaseResponse{HTMLURL: "https://github.example/releases/v2.8.0"})
					default:
						writer.WriteHeader(http.StatusForbidden)
					}
				},
			},
		}
		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				server := httptest.NewTLSServer(withGraphQLPromotion(
					testCase.handler,
					releasePromotionResponseForTest(8, "release/2.8.0"),
				))
				defer server.Close()
				publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
				_, err := publisher.VerifyReleaseReconciliation(context.Background(), port.ReleaseReconciliationRequest{
					RemoteURL: "https://" + server.URL[len("https://"):] + "/acme/governance.git",
					Release:   release,
				})
				assertProblem(t, err, testCase.code)
			})
		}
	})

	t.Run("covers lifecycle request transport failures", func(t *testing.T) {
		publisher := New(Options{
			Resolver: testCredentialResolver(),
			HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("transport unavailable")
			})},
		})
		repository := repositoryRef{host: "github.com", owner: "acme", name: "governance"}
		base, _ := url.Parse(defaultAPIBaseURL)
		if _, err := publisher.mergedPromotion(context.Background(), base, repository, release.String()); err == nil {
			t.Fatal("promotion transport failure was accepted")
		}
		if _, err := publisher.tagCommit(context.Background(), base, repository, "v2.8.0"); err == nil {
			t.Fatal("tag transport failure was accepted")
		}
		if _, err := publisher.publishedReleaseURL(context.Background(), base, repository, "v2.8.0"); err == nil {
			t.Fatal("release transport failure was accepted")
		}
		if _, err := publisher.hasEffectiveReleaseDelta(context.Background(), base, repository, release.String()); err == nil {
			t.Fatal("comparison transport failure was accepted")
		}
	})

	t.Run("covers unmatched workflow cancellation and annotated tag failures", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/actions/workflows/execute-protected-line-request.yml/runs":
				_ = json.NewEncoder(writer).Encode(workflowRunsResponse{
					WorkflowRuns: []workflowRunResponse{{DisplayTitle: "unrelated", Status: "completed", Conclusion: "success"}},
				})
				cancel()
			case "/repos/acme/governance/git/ref/tags/v2.8.0":
				_ = json.NewEncoder(writer).Encode(gitReferenceResponse{
					Object: gitObjectReference{SHA: "tag-object", Type: "tag"},
				})
			case "/repos/acme/governance/git/tags/tag-object":
				_, _ = writer.Write([]byte("{"))
			default:
				t.Fatalf("unexpected path %q", request.URL.Path)
			}
		}))
		defer server.Close()
		base, _ := url.Parse(server.URL)
		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		if _, err := publisher.waitForWorkflowRun(ctx, base, repositoryRef{host: "github.com", owner: "acme", name: "governance"}, "execute-protected-line-request.yml", "request"); err == nil {
			t.Fatal("unmatched cancelled workflow was accepted")
		}
		if _, err := publisher.tagCommit(context.Background(), base, repositoryRef{host: "github.com", owner: "acme", name: "governance"}, "v2.8.0"); err == nil {
			t.Fatal("malformed annotated tag was accepted")
		}
	})
}

func lifecycleServer(t *testing.T, effectiveDelta, annotatedTag bool) *httptest.Server {
	t.Helper()
	promotion := releasePromotionResponseForTest(8, "release/2.8.0")
	return httptest.NewTLSServer(withGraphQLPromotion(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/repos/acme/governance/pulls":
			if request.URL.Query().Get("base") != "main" || request.URL.Query().Get("state") != "closed" ||
				request.URL.Query().Get("per_page") != "100" || request.URL.Query().Get("page") != "1" ||
				request.URL.Query().Get("head") != "" {
				t.Fatalf("promotion query = %q", request.URL.RawQuery)
			}
			_ = json.NewEncoder(writer).Encode([]releasePullRequestResponse{promotion})
		case "/repos/acme/governance/git/ref/tags/v2.8.0":
			objectType := "commit"
			objectSHA := "merge-sha"
			if annotatedTag {
				objectType = "tag"
				objectSHA = "tag-object"
			}
			_ = json.NewEncoder(writer).Encode(gitReferenceResponse{
				Object: gitObjectReference{SHA: objectSHA, Type: objectType},
			})
		case "/repos/acme/governance/git/tags/tag-object":
			_ = json.NewEncoder(writer).Encode(gitTagResponse{
				Object: gitObjectReference{SHA: "merge-sha", Type: "commit"},
			})
		case "/repos/acme/governance/releases/tags/v2.8.0":
			_ = json.NewEncoder(writer).Encode(releaseResponse{
				HTMLURL: "https://github.example/releases/v2.8.0",
			})
		case "/repos/acme/governance/compare/develop...release/2.8.0":
			comparison := compareResponse{}
			if effectiveDelta {
				comparison.AheadBy = 1
				comparison.Files = []json.RawMessage{json.RawMessage(`{}`)}
			}
			_ = json.NewEncoder(writer).Encode(comparison)
		default:
			t.Fatalf("unexpected lifecycle path %q", request.URL.Path)
		}
	}), promotion))
}

func writeMergedPromotion(writer http.ResponseWriter) {
	_ = json.NewEncoder(writer).Encode([]releasePullRequestResponse{
		releasePromotionResponseForTest(8, "release/2.8.0"),
	})
}

func releasePromotionResponseForTest(number int, release string) releasePullRequestResponse {
	return releasePullRequestResponse{
		Number:         number,
		HTMLURL:        "https://github.example/pull/" + strconv.Itoa(number),
		MergedAt:       timePtr(time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)),
		MergeCommitSHA: "merge-sha",
		Base: releasePullRequestBranchResponse{
			Ref:        "main",
			Repository: releasePullRequestRepositoryResponse{FullName: "acme/governance"},
		},
		Head: releasePullRequestBranchResponse{
			Ref:        release,
			Repository: releasePullRequestRepositoryResponse{FullName: "acme/governance"},
		},
	}
}

func writeGraphQLPromotion(writer http.ResponseWriter, promotion releasePullRequestResponse) {
	_ = json.NewEncoder(writer).Encode(graphQLPromotionResponseForTest(promotion))
}

func graphQLPromotionResponseForTest(promotion releasePullRequestResponse) graphQLPromotionResponse {
	var response graphQLPromotionResponse
	response.Data.Repository.PullRequest = &graphQLPromotionPullRequestResponse{
		Number:      promotion.Number,
		HTMLURL:     promotion.HTMLURL,
		MergedAt:    promotion.MergedAt,
		BaseRefName: promotion.Base.Ref,
		HeadRefName: promotion.Head.Ref,
		BaseRepository: graphQLPromotionRepositoryResponse{
			NameWithOwner: promotion.Base.Repository.FullName,
		},
		HeadRepository: graphQLPromotionRepositoryResponse{
			NameWithOwner: promotion.Head.Repository.FullName,
		},
		MergeCommit: &graphQLPromotionCommitResponse{OID: "merge-sha"},
	}
	return response
}

func withGraphQLPromotion(handler http.HandlerFunc, promotion releasePullRequestResponse) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/graphql" {
			writeGraphQLPromotion(writer, promotion)
			return
		}
		handler(writer, request)
	}
}

func writeReleaseTagCommit(writer http.ResponseWriter) {
	_ = json.NewEncoder(writer).Encode(gitReferenceResponse{
		Object: gitObjectReference{SHA: "merge-sha", Type: "commit"},
	})
}

func timePtr(value time.Time) *time.Time {
	return &value
}
