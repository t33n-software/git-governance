package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/CyberT33N/git-governance/internal/application/port"
	"github.com/CyberT33N/git-governance/internal/domain/hotfix"
	"github.com/CyberT33N/git-governance/internal/domain/problem"
)

func TestPublisherVerifyMainHotfixMerge(t *testing.T) {
	manifest := strings.Repeat("a", 40)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/repos/acme/governance/pulls":
			if request.URL.Query().Get("base") != "main" ||
				request.URL.Query().Get("state") != "closed" ||
				request.URL.Query().Get("page") != "1" {
				t.Fatalf("pull query = %q", request.URL.RawQuery)
			}
			_ = json.NewEncoder(writer).Encode([]hotfixPullRequestResponse{mergedHotfixPullRequest()})
		case "/graphql":
			assertGraphQLHotfixRequest(t, request, 42)
			_ = json.NewEncoder(writer).Encode(graphQLHotfixPayload("merge-sha"))
		case "/repos/acme/governance/pulls/42/commits":
			_ = json.NewEncoder(writer).Encode([]pullRequestCommitResponse{{SHA: manifest}})
		case "/repos/acme/governance/git/ref/tags/v1.0.1":
			_ = json.NewEncoder(writer).Encode(gitReferenceResponse{Object: gitObjectReference{SHA: "previous-merge", Type: "commit"}})
		case "/repos/acme/governance/git/ref/tags/v1.0.2":
			writer.WriteHeader(http.StatusNotFound)
		default:
			t.Fatalf("unexpected path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
	result, err := publisher.VerifyMainHotfixMerge(context.Background(), testMainHotfixRequest(t, server.URL, manifest))
	if err != nil || result.PullRequestURL != "https://github.example/pull/42" || result.MergeCommit != "merge-sha" || result.Tag != "v1.0.2" {
		t.Fatalf("VerifyMainHotfixMerge() = (%#v, %v)", result, err)
	}
}

func TestPublisherVerifyMainHotfixMergeRejectsExistingTag(t *testing.T) {
	manifest := strings.Repeat("a", 40)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/repos/acme/governance/pulls":
			_ = json.NewEncoder(writer).Encode([]hotfixPullRequestResponse{mergedHotfixPullRequest()})
		case "/graphql":
			_ = json.NewEncoder(writer).Encode(graphQLHotfixPayload("merge-sha"))
		case "/repos/acme/governance/pulls/42/commits":
			_ = json.NewEncoder(writer).Encode([]pullRequestCommitResponse{{SHA: manifest}})
		case "/repos/acme/governance/git/ref/tags/v1.0.1":
			_ = json.NewEncoder(writer).Encode(gitReferenceResponse{Object: gitObjectReference{SHA: "previous-merge", Type: "commit"}})
		case "/repos/acme/governance/git/ref/tags/v1.0.2":
			_ = json.NewEncoder(writer).Encode(gitReferenceResponse{})
		default:
			t.Fatalf("unexpected path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
	_, err := publisher.VerifyMainHotfixMerge(context.Background(), testMainHotfixRequest(t, server.URL, manifest))
	assertProblem(t, err, problem.CodeConfigurationInvalid)
}

func TestPublisherVerifyMainHotfixDelivery(t *testing.T) {
	manifest := strings.Repeat("a", 40)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/repos/acme/governance/pulls":
			_ = json.NewEncoder(writer).Encode([]hotfixPullRequestResponse{mergedHotfixPullRequest()})
		case "/graphql":
			_ = json.NewEncoder(writer).Encode(graphQLHotfixPayload("merge-sha"))
		case "/repos/acme/governance/pulls/42/commits":
			_ = json.NewEncoder(writer).Encode([]pullRequestCommitResponse{{SHA: manifest}})
		case "/repos/acme/governance/git/ref/tags/v1.0.1":
			_ = json.NewEncoder(writer).Encode(gitReferenceResponse{Object: gitObjectReference{SHA: "previous-merge", Type: "commit"}})
		case "/repos/acme/governance/git/ref/tags/v1.0.2":
			_ = json.NewEncoder(writer).Encode(gitReferenceResponse{Object: gitObjectReference{SHA: "merge-sha", Type: "commit"}})
		case "/repos/acme/governance/releases/tags/v1.0.2":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"html_url": "https://github.example/releases/v1.0.2",
				"draft":    false,
				"assets": []releaseAssetResponse{
					{Name: "git-governance_1.0.2_checksums.txt"},
					{Name: "git-governance_1.0.2_checksums.txt.sigstore.json"},
					{Name: "git-governance_1.0.2_linux_amd64.tar.gz"},
					{Name: "git-governance_1.0.2_linux_amd64.sbom.json"},
				},
			})
		case "/repos/acme/governance/actions/workflows/publish-release-artifacts.yml/runs":
			if request.URL.Query().Get("event") != "workflow_dispatch" {
				t.Fatalf("workflow query = %q", request.URL.RawQuery)
			}
			_ = json.NewEncoder(writer).Encode(workflowRunsResponse{
				WorkflowRuns: []workflowRunResponse{{
					Status:       "completed",
					Conclusion:   "success",
					HTMLURL:      "https://github.example/actions/runs/102",
					DisplayTitle: "Release v1.0.2",
				}},
			})
		default:
			t.Fatalf("unexpected path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
	result, err := publisher.VerifyMainHotfixDelivery(context.Background(), testMainHotfixRequest(t, server.URL, manifest))
	if err != nil ||
		result.PullRequestURL != "https://github.example/pull/42" ||
		result.MergeCommit != "merge-sha" ||
		result.Tag != "v1.0.2" ||
		result.ReleaseURL != "https://github.example/releases/v1.0.2" ||
		result.WorkflowRunURL != "https://github.example/actions/runs/102" {
		t.Fatalf("VerifyMainHotfixDelivery() = (%#v, %v)", result, err)
	}
}

func TestPublisherMainHotfixEvidenceBoundaries(t *testing.T) {
	t.Run("requires a main record", func(t *testing.T) {
		record := testHotfixRecord(t, "release/1.0.2", "hotfix/ABC-999-payment-timeout", "release/1.0.2", strings.Repeat("a", 40))
		publisher := New(Options{Resolver: testCredentialResolver()})
		_, err := publisher.verifiedMergedMainHotfix(context.Background(), nil, repositoryRef{}, port.MainHotfixDeliveryRequest{Record: record})
		assertProblem(t, err, problem.CodeConfigurationInvalid)
	})

	t.Run("rejects incomplete REST and GraphQL evidence", func(t *testing.T) {
		repository := repositoryRef{owner: "acme", name: "governance"}
		candidate := mergedHotfixPullRequest()
		if !isMergedMainHotfix(candidate, repository, "hotfix/ABC-999-payment-timeout") {
			t.Fatal("expected baseline hotfix evidence to match")
		}
		for _, mutate := range []func(*hotfixPullRequestResponse){
			func(value *hotfixPullRequestResponse) { value.Number = 0 },
			func(value *hotfixPullRequestResponse) { value.Base.Ref = "develop" },
			func(value *hotfixPullRequestResponse) { value.Base.Repository.FullName = "acme/other" },
			func(value *hotfixPullRequestResponse) { value.Head.Ref = "hotfix/ABC-999-other" },
			func(value *hotfixPullRequestResponse) { value.Head.Repository.FullName = "acme/fork" },
			func(value *hotfixPullRequestResponse) { value.MergedAt = nil },
			func(value *hotfixPullRequestResponse) { value.HTMLURL = "" },
		} {
			value := candidate
			mutate(&value)
			if isMergedMainHotfix(value, repository, "hotfix/ABC-999-payment-timeout") {
				t.Fatalf("isMergedMainHotfix accepted %#v", value)
			}
		}

		graphQL := graphQLHotfixPayload("merge-sha").Data.Repository.PullRequest
		if !isMergedGraphQLMainHotfix(graphQL, repository, "hotfix/ABC-999-payment-timeout", candidate) {
			t.Fatal("expected baseline GraphQL hotfix evidence to match")
		}
		graphQL.MergeCommit = nil
		if isMergedGraphQLMainHotfix(graphQL, repository, "hotfix/ABC-999-payment-timeout", candidate) {
			t.Fatal("isMergedGraphQLMainHotfix accepted a missing merge commit")
		}
	})

	t.Run("requires every manifest commit in order", func(t *testing.T) {
		manifest := []string{strings.Repeat("a", 40), strings.Repeat("b", 40)}
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			_ = json.NewEncoder(writer).Encode([]pullRequestCommitResponse{{SHA: manifest[1]}, {SHA: manifest[0]}})
		}))
		defer server.Close()
		base, _ := url.Parse(server.URL)
		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		err := publisher.verifyHotfixManifest(context.Background(), base, repositoryRef{owner: "acme", name: "governance"}, 42, manifest)
		assertProblem(t, err, problem.CodeConfigurationInvalid)
	})

	t.Run("requires complete release assets", func(t *testing.T) {
		if !hasHotfixArtifactEvidence([]releaseAssetResponse{
			{Name: "checksums.txt"},
			{Name: "bundle.sigstore.json"},
			{Name: "archive.tar.gz"},
			{Name: "archive.sbom.json"},
		}) {
			t.Fatal("complete artifact evidence was rejected")
		}
		for _, assets := range [][]releaseAssetResponse{
			nil,
			{{Name: "checksums.txt"}, {Name: "bundle.sigstore.json"}, {Name: "archive.tar.gz"}},
			{{Name: "checksums.txt"}, {Name: "bundle.sigstore.json"}, {Name: "archive.sbom.json"}},
			{{Name: "checksums.txt"}, {Name: "archive.tar.gz"}, {Name: "archive.sbom.json"}},
			{{Name: "bundle.sigstore.json"}, {Name: "archive.tar.gz"}, {Name: "archive.sbom.json"}},
		} {
			if hasHotfixArtifactEvidence(assets) {
				t.Fatalf("incomplete artifact evidence was accepted: %#v", assets)
			}
		}
	})
}

func TestPublisherWaitForHotfixArtifactWorkflowFailureAndTimeout(t *testing.T) {
	t.Run("rejects a failed delivery run", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			_ = json.NewEncoder(writer).Encode(workflowRunsResponse{
				WorkflowRuns: []workflowRunResponse{{
					Status:       "completed",
					Conclusion:   "failure",
					DisplayTitle: "Release v1.0.2",
				}},
			})
		}))
		defer server.Close()
		base, _ := url.Parse(server.URL)
		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		_, err := publisher.waitForHotfixArtifactWorkflow(context.Background(), base, repositoryRef{owner: "acme", name: "governance"}, "v1.0.2")
		assertProblem(t, err, problem.CodeConfigurationInvalid)
	})

	t.Run("times out when no matching delivery run exists", func(t *testing.T) {
		original := hotfixDeliveryWorkflowWaitLimit
		hotfixDeliveryWorkflowWaitLimit = time.Nanosecond
		t.Cleanup(func() { hotfixDeliveryWorkflowWaitLimit = original })

		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			_ = json.NewEncoder(writer).Encode(workflowRunsResponse{})
		}))
		defer server.Close()
		base, _ := url.Parse(server.URL)
		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		_, err := publisher.waitForHotfixArtifactWorkflow(context.Background(), base, repositoryRef{owner: "acme", name: "governance"}, "v1.0.2")
		assertProblem(t, err, problem.CodeExternalCommandFailed)
	})
}

func TestPublisherHotfixDeliveryProviderFailurePaths(t *testing.T) {
	t.Run("rejects unavailable lifecycle configuration", func(t *testing.T) {
		var publisher *Publisher
		_, err := publisher.VerifyMainHotfixMerge(context.Background(), port.MainHotfixDeliveryRequest{})
		assertProblem(t, err, problem.CodeConfigurationInvalid)
		_, err = publisher.VerifyMainHotfixDelivery(context.Background(), port.MainHotfixDeliveryRequest{})
		assertProblem(t, err, problem.CodeConfigurationInvalid)
	})

	t.Run("rejects non main record before provider reads", func(t *testing.T) {
		record := testHotfixRecord(t, "release/1.0.2", "hotfix/ABC-999-payment-timeout", "release/1.0.2", strings.Repeat("a", 40))
		publisher := New(Options{Resolver: testCredentialResolver()})
		_, err := publisher.VerifyMainHotfixMerge(context.Background(), port.MainHotfixDeliveryRequest{
			RemoteURL: "https://github.com/acme/governance.git",
			Record:    record,
		})
		assertProblem(t, err, problem.CodeConfigurationInvalid)
	})

	t.Run("propagates tag lookup and tag mismatch failures", func(t *testing.T) {
		manifest := strings.Repeat("a", 40)
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/pulls":
				_ = json.NewEncoder(writer).Encode([]hotfixPullRequestResponse{mergedHotfixPullRequest()})
			case "/graphql":
				_ = json.NewEncoder(writer).Encode(graphQLHotfixPayload("merge-sha"))
			case "/repos/acme/governance/pulls/42/commits":
				_ = json.NewEncoder(writer).Encode([]pullRequestCommitResponse{{SHA: manifest}})
			case "/repos/acme/governance/git/ref/tags/v1.0.1":
				_ = json.NewEncoder(writer).Encode(gitReferenceResponse{Object: gitObjectReference{SHA: "previous-merge", Type: "commit"}})
			case "/repos/acme/governance/git/ref/tags/v1.0.2":
				writer.WriteHeader(http.StatusInternalServerError)
			default:
				t.Fatalf("unexpected path %q", request.URL.Path)
			}
		}))
		defer server.Close()
		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		_, err := publisher.VerifyMainHotfixMerge(context.Background(), testMainHotfixRequest(t, server.URL, manifest))
		assertProblem(t, err, problem.CodeExternalCommandFailed)

		server.Config.Handler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/pulls":
				_ = json.NewEncoder(writer).Encode([]hotfixPullRequestResponse{mergedHotfixPullRequest()})
			case "/graphql":
				_ = json.NewEncoder(writer).Encode(graphQLHotfixPayload("merge-sha"))
			case "/repos/acme/governance/pulls/42/commits":
				_ = json.NewEncoder(writer).Encode([]pullRequestCommitResponse{{SHA: manifest}})
			case "/repos/acme/governance/git/ref/tags/v1.0.1":
				_ = json.NewEncoder(writer).Encode(gitReferenceResponse{Object: gitObjectReference{SHA: "previous-merge", Type: "commit"}})
			case "/repos/acme/governance/git/ref/tags/v1.0.2":
				_ = json.NewEncoder(writer).Encode(gitReferenceResponse{Object: gitObjectReference{SHA: "other-merge", Type: "commit"}})
			default:
				t.Fatalf("unexpected path %q", request.URL.Path)
			}
		})
		_, err = publisher.VerifyMainHotfixDelivery(context.Background(), testMainHotfixRequest(t, server.URL, manifest))
		assertProblem(t, err, problem.CodeConfigurationInvalid)
	})
}

func TestMainHotfixEvidenceHelpersCoverProviderFailures(t *testing.T) {
	repository := repositoryRef{owner: "acme", name: "governance"}

	t.Run("paginates matching hotfix pull requests", func(t *testing.T) {
		first := make([]hotfixPullRequestResponse, releasePromotionPageSize)
		for index := range first {
			first[index] = mergedHotfixPullRequest()
			first[index].Head.Ref = "hotfix/ABC-999-other"
		}
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Query().Get("page") {
			case "1":
				_ = json.NewEncoder(writer).Encode(first)
			case "2":
				_ = json.NewEncoder(writer).Encode([]hotfixPullRequestResponse{mergedHotfixPullRequest()})
			default:
				t.Fatalf("unexpected page %q", request.URL.Query().Get("page"))
			}
		}))
		defer server.Close()
		base, _ := url.Parse(server.URL)
		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		result, err := publisher.mergedMainHotfix(context.Background(), base, repository, "hotfix/ABC-999-payment-timeout")
		if err != nil || result.Number != 42 {
			t.Fatalf("mergedMainHotfix() = (%#v, %v)", result, err)
		}
	})

	t.Run("rejects failed, invalid, and missing pull request lists", func(t *testing.T) {
		for _, response := range []struct {
			status int
			body   string
		}{
			{status: http.StatusInternalServerError},
			{status: http.StatusOK, body: "not-json"},
			{status: http.StatusOK, body: "[]"},
		} {
			response := response
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.WriteHeader(response.status)
				_, _ = writer.Write([]byte(response.body))
			}))
			base, _ := url.Parse(server.URL)
			publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
			_, err := publisher.mergedMainHotfix(context.Background(), base, repository, "hotfix/ABC-999-payment-timeout")
			if err == nil {
				t.Fatal("mergedMainHotfix unexpectedly succeeded")
			}
			server.Close()
		}
	})

	t.Run("rejects GraphQL status decode and identity failures", func(t *testing.T) {
		rest := mergedHotfixPullRequest()
		for _, payload := range []struct {
			status int
			body   string
		}{
			{status: http.StatusInternalServerError},
			{status: http.StatusOK, body: "not-json"},
			{status: http.StatusOK, body: `{"errors":[{"message":"denied"}]}`},
			{status: http.StatusOK, body: `{"data":{"repository":{"pullRequest":null}}}`},
		} {
			payload := payload
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.WriteHeader(payload.status)
				_, _ = writer.Write([]byte(payload.body))
			}))
			base, _ := url.Parse(server.URL)
			publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
			_, err := publisher.mainHotfixMergeCommit(context.Background(), base, repository, "hotfix/ABC-999-payment-timeout", rest)
			if err == nil {
				t.Fatal("mainHotfixMergeCommit unexpectedly succeeded")
			}
			server.Close()
		}
	})

	t.Run("handles manifest request and decoding failures", func(t *testing.T) {
		for _, response := range []struct {
			status int
			body   string
		}{
			{status: http.StatusInternalServerError},
			{status: http.StatusOK, body: "not-json"},
		} {
			response := response
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.WriteHeader(response.status)
				_, _ = writer.Write([]byte(response.body))
			}))
			base, _ := url.Parse(server.URL)
			publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
			err := publisher.verifyHotfixManifest(context.Background(), base, repository, 42, []string{strings.Repeat("a", 40)})
			if err == nil {
				t.Fatal("verifyHotfixManifest unexpectedly succeeded")
			}
			server.Close()
		}
	})

	t.Run("handles tag and published release failures", func(t *testing.T) {
		for _, status := range []int{http.StatusInternalServerError} {
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.WriteHeader(status)
			}))
			base, _ := url.Parse(server.URL)
			publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
			if _, err := publisher.tagExists(context.Background(), base, repository, "v1.0.2"); err == nil {
				t.Fatal("tagExists unexpectedly succeeded")
			}
			if _, err := publisher.publishedHotfixReleaseURL(context.Background(), base, repository, "v1.0.2"); err == nil {
				t.Fatal("publishedHotfixReleaseURL unexpectedly succeeded")
			}
			server.Close()
		}
	})
}

func TestPublisherMainHotfixDeliveryTopLevelFailures(t *testing.T) {
	manifest := strings.Repeat("a", 40)
	for _, stage := range []string{"previous-tag", "merge", "graphql", "manifest", "tag", "release", "workflow"} {
		stage := stage
		t.Run(stage, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if stage == "merge" && request.URL.Path == "/repos/acme/governance/pulls" {
					writer.WriteHeader(http.StatusInternalServerError)
					return
				}
				switch request.URL.Path {
				case "/repos/acme/governance/pulls":
					_ = json.NewEncoder(writer).Encode([]hotfixPullRequestResponse{mergedHotfixPullRequest()})
				case "/graphql":
					if stage == "graphql" {
						writer.WriteHeader(http.StatusInternalServerError)
						return
					}
					_ = json.NewEncoder(writer).Encode(graphQLHotfixPayload("merge-sha"))
				case "/repos/acme/governance/pulls/42/commits":
					if stage == "manifest" {
						writer.WriteHeader(http.StatusInternalServerError)
						return
					}
					_ = json.NewEncoder(writer).Encode([]pullRequestCommitResponse{{SHA: manifest}})
				case "/repos/acme/governance/git/ref/tags/v1.0.1":
					if stage == "previous-tag" {
						writer.WriteHeader(http.StatusNotFound)
						return
					}
					_ = json.NewEncoder(writer).Encode(gitReferenceResponse{Object: gitObjectReference{SHA: "previous-merge", Type: "commit"}})
				case "/repos/acme/governance/git/ref/tags/v1.0.2":
					if stage == "tag" {
						writer.WriteHeader(http.StatusInternalServerError)
						return
					}
					_ = json.NewEncoder(writer).Encode(gitReferenceResponse{Object: gitObjectReference{SHA: "merge-sha", Type: "commit"}})
				case "/repos/acme/governance/releases/tags/v1.0.2":
					if stage == "release" {
						writer.WriteHeader(http.StatusInternalServerError)
						return
					}
					_ = json.NewEncoder(writer).Encode(map[string]any{
						"html_url": "https://github.example/releases/v1.0.2",
						"assets": []releaseAssetResponse{
							{Name: "checksums.txt"},
							{Name: "bundle.sigstore.json"},
							{Name: "archive.tar.gz"},
							{Name: "archive.sbom.json"},
						},
					})
				case "/repos/acme/governance/actions/workflows/publish-release-artifacts.yml/runs":
					if stage == "workflow" {
						writer.WriteHeader(http.StatusInternalServerError)
						return
					}
					t.Fatalf("unexpected successful workflow request for stage %q", stage)
				default:
					t.Fatalf("unexpected path %q", request.URL.Path)
				}
			}))
			defer server.Close()

			publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
			_, err := publisher.VerifyMainHotfixDelivery(context.Background(), testMainHotfixRequest(t, server.URL, manifest))
			if err == nil {
				t.Fatalf("VerifyMainHotfixDelivery unexpectedly succeeded at stage %q", stage)
			}
		})
	}
}

func TestMainHotfixDeliveryHelperEdgeCases(t *testing.T) {
	repository := repositoryRef{owner: "acme", name: "governance"}

	t.Run("propagates canceled request failures", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		publisher := New(Options{Resolver: testCredentialResolver()})
		apiBase, _ := url.Parse("https://github.example")
		rest := mergedHotfixPullRequest()
		for _, invoke := range []func() error{
			func() error {
				_, err := publisher.mergedMainHotfix(ctx, apiBase, repository, "hotfix/ABC-999-payment-timeout")
				return err
			},
			func() error {
				_, err := publisher.mainHotfixMergeCommit(ctx, apiBase, repository, "hotfix/ABC-999-payment-timeout", rest)
				return err
			},
			func() error {
				return publisher.verifyHotfixManifest(ctx, apiBase, repository, 42, []string{strings.Repeat("a", 40)})
			},
			func() error {
				_, err := publisher.tagExists(ctx, apiBase, repository, "v1.0.2")
				return err
			},
			func() error {
				_, err := publisher.publishedHotfixReleaseURL(ctx, apiBase, repository, "v1.0.2")
				return err
			},
			func() error {
				_, err := publisher.waitForHotfixArtifactWorkflow(ctx, apiBase, repository, "v1.0.2")
				return err
			},
		} {
			if err := invoke(); err == nil {
				t.Fatal("canceled helper unexpectedly succeeded")
			}
		}
	})

	t.Run("paginates ordered manifest commits", func(t *testing.T) {
		manifest := []string{strings.Repeat("a", 40)}
		firstPage := make([]pullRequestCommitResponse, releasePromotionPageSize)
		for index := range firstPage {
			firstPage[index] = pullRequestCommitResponse{SHA: strings.Repeat("b", 40)}
		}
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Query().Get("page") {
			case "1":
				_ = json.NewEncoder(writer).Encode(firstPage)
			case "2":
				_ = json.NewEncoder(writer).Encode([]pullRequestCommitResponse{{SHA: manifest[0]}})
			default:
				t.Fatalf("unexpected page %q", request.URL.Query().Get("page"))
			}
		}))
		defer server.Close()
		base, _ := url.Parse(server.URL)
		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		if err := publisher.verifyHotfixManifest(context.Background(), base, repository, 42, manifest); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("rejects malformed and incomplete release evidence", func(t *testing.T) {
		for _, payload := range []string{
			"not-json",
			`{"html_url":"","draft":false,"assets":[]}`,
			`{"html_url":"https://github.example/releases/v1.0.2","draft":true,"assets":[]}`,
		} {
			payload := payload
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				_, _ = writer.Write([]byte(payload))
			}))
			base, _ := url.Parse(server.URL)
			publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
			_, err := publisher.publishedHotfixReleaseURL(context.Background(), base, repository, "v1.0.2")
			if err == nil {
				t.Fatal("publishedHotfixReleaseURL unexpectedly accepted incomplete evidence")
			}
			server.Close()
		}
	})

	t.Run("waits through an in-progress delivery run", func(t *testing.T) {
		calls := 0
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			calls++
			status := "in_progress"
			conclusion := ""
			if calls == 2 {
				status = "completed"
				conclusion = "success"
			}
			_ = json.NewEncoder(writer).Encode(workflowRunsResponse{
				WorkflowRuns: []workflowRunResponse{{
					Status:       status,
					Conclusion:   conclusion,
					HTMLURL:      "https://github.example/actions/runs/103",
					DisplayTitle: "Release v1.0.2",
				}},
			})
		}))
		defer server.Close()
		base, _ := url.Parse(server.URL)
		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		result, err := publisher.waitForHotfixArtifactWorkflow(context.Background(), base, repository, "v1.0.2")
		if err != nil || result == "" || calls != 2 {
			t.Fatalf("waitForHotfixArtifactWorkflow() = (%q, %v), calls=%d", result, err, calls)
		}
	})

	t.Run("rejects invalid workflow responses and unrelated runs", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(writer).Encode(workflowRunsResponse{})
		}))
		base, _ := url.Parse(server.URL)
		publisher := New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		_, err := publisher.waitForHotfixArtifactWorkflow(context.Background(), base, repository, "v1.0.2")
		if err == nil {
			t.Fatal("waitForHotfixArtifactWorkflow accepted an invalid response")
		}
		server.Close()

		original := hotfixDeliveryWorkflowWaitLimit
		hotfixDeliveryWorkflowWaitLimit = 20 * time.Millisecond
		t.Cleanup(func() { hotfixDeliveryWorkflowWaitLimit = original })
		server = httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			_ = json.NewEncoder(writer).Encode(workflowRunsResponse{
				WorkflowRuns: []workflowRunResponse{{
					Status:       "completed",
					Conclusion:   "success",
					HTMLURL:      "https://github.example/actions/runs/unrelated",
					DisplayTitle: "Release v9.9.9",
				}},
			})
		}))
		defer server.Close()
		base, _ = url.Parse(server.URL)
		publisher = New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
		_, err = publisher.waitForHotfixArtifactWorkflow(context.Background(), base, repository, "v1.0.2")
		assertProblem(t, err, problem.CodeExternalCommandFailed)
	})
}

func testMainHotfixRequest(t *testing.T, apiBase, manifest string) port.MainHotfixDeliveryRequest {
	t.Helper()

	return port.MainHotfixDeliveryRequest{
		RemoteURL: "https://" + apiBase[len("https://"):] + "/acme/governance.git",
		Record:    testHotfixRecord(t, "main", "hotfix/ABC-999-payment-timeout", "main", manifest),
	}
}

func testHotfixRecord(t *testing.T, affected, source, target, manifest string) hotfix.ReleaseRecord {
	t.Helper()

	contents := fmt.Sprintf(
		`{"schemaVersion":1,"ticket":"ABC-999","incident":"INC-999","affectedLine":%q,"targetVersion":"1.0.2","previousTag":"v1.0.1","expectedPullRequest":{"source":%q,"target":%q},"manifest":["%s"],"commitBudgetException":"","propagationTargets":["develop"]}`,
		affected,
		source,
		target,
		manifest,
	)
	record, err := hotfix.ParseRecord([]byte(contents))
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func mergedHotfixPullRequest() hotfixPullRequestResponse {
	mergedAt := time.Date(2026, time.August, 8, 0, 0, 0, 0, time.UTC)
	return hotfixPullRequestResponse{
		Number:   42,
		HTMLURL:  "https://github.example/pull/42",
		MergedAt: &mergedAt,
		Base: releasePullRequestBranchResponse{
			Ref:        "main",
			Repository: releasePullRequestRepositoryResponse{FullName: "acme/governance"},
		},
		Head: releasePullRequestBranchResponse{
			Ref:        "hotfix/ABC-999-payment-timeout",
			Repository: releasePullRequestRepositoryResponse{FullName: "acme/governance"},
		},
	}
}

func graphQLHotfixPayload(merge string) graphQLHotfixResponse {
	mergedAt := time.Date(2026, time.August, 8, 0, 0, 0, 0, time.UTC)
	response := graphQLHotfixResponse{}
	response.Data.Repository.PullRequest = &graphQLHotfixPullRequestResponse{
		Number:         42,
		HTMLURL:        "https://github.example/pull/42",
		MergedAt:       &mergedAt,
		BaseRefName:    "main",
		HeadRefName:    "hotfix/ABC-999-payment-timeout",
		BaseRepository: graphQLHotfixRepositoryResponse{NameWithOwner: "acme/governance"},
		HeadRepository: graphQLHotfixRepositoryResponse{NameWithOwner: "acme/governance"},
		MergeCommit:    &graphQLHotfixCommitResponse{OID: merge},
	}
	return response
}

func assertGraphQLHotfixRequest(t *testing.T, request *http.Request, number int) {
	t.Helper()

	if request.Method != http.MethodPost {
		t.Fatalf("GraphQL method = %s", request.Method)
	}
	var payload graphQLHotfixRequest
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Variables.Owner != "acme" || payload.Variables.Name != "governance" || payload.Variables.Number != number {
		t.Fatalf("GraphQL variables = %#v", payload.Variables)
	}
}
