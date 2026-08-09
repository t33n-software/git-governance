// Package github implements the GitHub REST pull-request publishing adapter.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/CyberT33N/git-governance/internal/application/port"
	"github.com/CyberT33N/git-governance/internal/domain/problem"
)

const (
	defaultAPIBaseURL = "https://api.github.com"
	defaultAPIVersion = "2026-03-10"
	maxResponseBytes  = 1 << 20
)

// Options configures a GitHub REST publisher. Resolver obtains short-lived
// GitHub App user-access tokens immediately before API calls.
type Options struct {
	Resolver   CredentialResolver
	APIBaseURL string
	APIVersion string
	Timeout    time.Duration
	HTTPClient *http.Client
}

// Publisher creates or returns an existing GitHub pull request for a
// provider-neutral application intent.
type Publisher struct {
	resolver      CredentialResolver
	apiBaseURL    string
	deriveAPIBase bool
	apiVersion    string
	client        *http.Client
}

// New constructs a GitHub pull-request publisher. Configuration is validated
// on publication so unrelated CLI commands do not require GitHub credentials.
func New(options Options) *Publisher {
	apiBaseURL := options.APIBaseURL
	deriveAPIBase := apiBaseURL == ""
	if apiBaseURL == "" {
		apiBaseURL = defaultAPIBaseURL
	}
	apiVersion := options.APIVersion
	if apiVersion == "" {
		apiVersion = defaultAPIVersion
	}
	client := options.HTTPClient
	if client == nil {
		timeout := options.Timeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	return &Publisher{
		resolver:      options.Resolver,
		apiBaseURL:    apiBaseURL,
		deriveAPIBase: deriveAPIBase,
		apiVersion:    apiVersion,
		client:        client,
	}
}

// Publish creates a GitHub pull request when no equivalent open pull request
// exists. Returning an existing URL makes automation retries idempotent.
func (publisher *Publisher) Publish(
	ctx context.Context,
	publication port.PullRequestPublication,
) (port.PublishedPullRequest, error) {
	apiBase, repository, err := publisher.publicationTarget(publication)
	if err != nil {
		return port.PublishedPullRequest{}, err
	}
	if existingURL, found, err := publisher.findOpenPullRequest(ctx, apiBase, repository, publication.PullRequest); err != nil {
		return port.PublishedPullRequest{}, err
	} else if found {
		return port.PublishedPullRequest{URL: existingURL}, nil
	}
	return publisher.createPullRequest(ctx, apiBase, repository, publication.PullRequest)
}

// Validate verifies adapter routing, credentials, and exact repository
// authorization before a requested publication pushes Git state.
func (publisher *Publisher) Validate(ctx context.Context, publication port.PullRequestPublication) error {
	return publisher.validate(ctx, publication)
}

func (publisher *Publisher) validate(ctx context.Context, publication port.PullRequestPublication) error {
	_, repository, err := publisher.publicationTarget(publication)
	if err != nil {
		return err
	}
	_, err = publisher.resolveCredential(ctx, repository)
	return err
}

func (publisher *Publisher) publicationTarget(
	publication port.PullRequestPublication,
) (*url.URL, repositoryRef, error) {
	if publisher == nil {
		return nil, repositoryRef{}, configurationProblem(
			"GitHub publisher",
			"a configured GitHub publisher",
			"configure --pull-request-provider github before requesting pull-request creation",
		)
	}
	if publisher.resolver == nil {
		return nil, repositoryRef{}, configurationProblem(
			"GitHub App session",
			"a configured GitHub App credential resolver",
			"run auth login github in an interactive terminal before requesting pull-request creation",
		)
	}
	if publication.PullRequest.Source.IsZero() || publication.PullRequest.Target.IsZero() ||
		strings.TrimSpace(publication.PullRequest.Title) == "" {
		return nil, repositoryRef{}, configurationProblem(
			"pull request",
			"non-empty source, target, and title values",
			"publish only a fully prepared provider-neutral pull-request intent",
		)
	}
	repository, err := parseRepositoryRemote(publication.RemoteURL)
	if err != nil {
		return nil, repositoryRef{}, err
	}
	apiBaseURL := publisher.apiBaseURL
	if publisher.deriveAPIBase {
		apiBaseURL = apiBaseURLForGitHost(repository.host)
	}
	apiBase, err := parseAPIBaseURL(apiBaseURL)
	if err != nil {
		return nil, repositoryRef{}, err
	}
	if !sameHost(repository.host, expectedGitHost(apiBase.Hostname())) {
		return nil, repositoryRef{}, configurationProblem(
			"GitHub remote",
			"a remote hosted by "+expectedGitHost(apiBase.Hostname()),
			"select a remote hosted by the configured GitHub App host",
		)
	}
	return apiBase, repository, nil
}

type repositoryRef struct {
	host  string
	owner string
	name  string
}

type pullRequestResponse struct {
	HTMLURL string `json:"html_url"`
}

type createPullRequestRequest struct {
	Title string `json:"title"`
	Head  string `json:"head"`
	Base  string `json:"base"`
	Draft bool   `json:"draft"`
}

type githubAPIErrorResponse struct {
	Message string                     `json:"message"`
	Errors  []githubAPIValidationError `json:"errors"`
}

type githubAPIValidationError struct {
	Resource string `json:"resource"`
	Field    string `json:"field"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

func (publisher *Publisher) findOpenPullRequest(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	request port.PullRequest,
) (string, bool, error) {
	query := url.Values{}
	query.Set("state", "open")
	query.Set("head", repository.owner+":"+request.Source.String())
	query.Set("base", request.Target.String())
	endpoint := repositoryEndpoint(apiBase, repository, "pulls", query)

	response, err := publisher.request(ctx, repository, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", false, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", false, responseProblemWithBody(response, "find an existing GitHub pull request")
	}

	var pullRequests []pullRequestResponse
	if err := decodeResponse(response.Body, &pullRequests); err != nil {
		return "", false, err
	}
	if len(pullRequests) == 0 {
		return "", false, nil
	}
	if strings.TrimSpace(pullRequests[0].HTMLURL) == "" {
		return "", false, responseDecodeProblem("GitHub pull-request lookup response", nil)
	}
	return pullRequests[0].HTMLURL, true, nil
}

func (publisher *Publisher) createPullRequest(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	request port.PullRequest,
) (port.PublishedPullRequest, error) {
	body, _ := json.Marshal(createPullRequestRequest{
		Title: request.Title,
		Head:  request.Source.String(),
		Base:  request.Target.String(),
		Draft: request.Draft,
	})
	endpoint := repositoryEndpoint(apiBase, repository, "pulls", nil)
	response, err := publisher.request(ctx, repository, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return port.PublishedPullRequest{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return port.PublishedPullRequest{}, responseProblemWithBody(response, "create a GitHub pull request")
	}

	var created pullRequestResponse
	if err := decodeResponse(response.Body, &created); err != nil {
		return port.PublishedPullRequest{}, err
	}
	if strings.TrimSpace(created.HTMLURL) == "" {
		return port.PublishedPullRequest{}, responseDecodeProblem("GitHub pull-request creation response", nil)
	}
	return port.PublishedPullRequest{URL: created.HTMLURL}, nil
}

func (publisher *Publisher) request(
	ctx context.Context,
	repository repositoryRef,
	method string,
	endpoint *url.URL,
	body io.Reader,
) (*http.Response, error) {
	token, err := publisher.resolveCredential(ctx, repository)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return nil, problem.Wrap(problem.Details{
			Code:        problem.CodeConfigurationInvalid,
			Category:    problem.CategoryConfig,
			Field:       "GitHub API URL",
			Expected:    "a valid HTTPS API endpoint",
			Rule:        "GitHub API requests must use a valid configured endpoint",
			Remediation: "repair the GitHub App API endpoint configuration",
		}, err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-GitHub-Api-Version", publisher.apiVersion)
	request.Header.Set("User-Agent", "git-governance")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := publisher.client.Do(request)
	if err != nil {
		return nil, problem.Wrap(problem.Details{
			Code:        problem.CodeExternalCommandFailed,
			Category:    problem.CategoryExternal,
			Field:       "GitHub pull request",
			Expected:    "a reachable GitHub API endpoint",
			Rule:        "pull-request publication must complete through the configured hosting adapter",
			Remediation: "check network access, GitHub App authorization, and the selected repository",
		}, err)
	}
	return response, nil
}

func (publisher *Publisher) resolveCredential(ctx context.Context, repository repositoryRef) (string, error) {
	if publisher == nil || publisher.resolver == nil {
		return "", configurationProblem(
			"GitHub App session",
			"a configured GitHub App credential resolver",
			"run auth login github in an interactive terminal before requesting pull-request creation",
		)
	}
	return publisher.resolver.Resolve(ctx, CredentialTarget{
		Host:       repository.host,
		Owner:      repository.owner,
		Repository: repository.name,
	})
}

func parseAPIBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, configurationProblem(
			"GitHub API URL",
			"an HTTPS URL without credentials, query, or fragment",
			"use the GitHub App API endpoint for the selected host",
		)
	}
	return parsed, nil
}

func parseRepositoryRemote(raw string) (repositoryRef, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return repositoryRef{}, configurationProblem(
			"Git remote URL",
			"a configured GitHub remote URL",
			"configure the selected remote before requesting GitHub pull-request creation",
		)
	}
	if !strings.Contains(raw, "://") {
		accountHost, path, found := strings.Cut(raw, ":")
		account, host, hasAccount := strings.Cut(accountHost, "@")
		if found && hasAccount && account != "" && host != "" {
			return parseRepositoryPath(host, path)
		}
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" || parsed.Path == "" ||
		(parsed.Scheme != "https" && parsed.Scheme != "ssh" && parsed.Scheme != "git") {
		return repositoryRef{}, configurationProblem(
			"Git remote URL",
			"a GitHub HTTPS, SSH, or git remote URL",
			"select a canonical GitHub remote such as https://github.com/owner/repository.git",
		)
	}
	return parseRepositoryPath(parsed.Hostname(), parsed.Path)
}

func parseRepositoryPath(host, rawPath string) (repositoryRef, error) {
	segments := strings.Split(strings.Trim(rawPath, "/"), "/")
	if len(segments) != 2 {
		return repositoryRef{}, configurationProblem(
			"Git remote URL",
			"a repository URL with exactly owner and repository path segments",
			"select a remote such as git@github.com:owner/repository.git",
		)
	}
	owner := segments[0]
	name := strings.TrimSuffix(segments[1], ".git")
	if !validRepositorySegment(owner) || !validRepositorySegment(name) {
		return repositoryRef{}, configurationProblem(
			"Git remote URL",
			"a repository URL with non-empty owner and repository names",
			"select a canonical GitHub owner/repository remote URL",
		)
	}
	return repositoryRef{host: host, owner: owner, name: name}, nil
}

func validRepositorySegment(value string) bool {
	return value != "" && value != "." && value != ".." &&
		!strings.ContainsAny(value, " \t\r\n?#")
}

func repositoryEndpoint(base *url.URL, repository repositoryRef, suffix string, query url.Values) *url.URL {
	endpoint := *base
	endpoint.Path = strings.TrimRight(base.Path, "/") + "/repos/" +
		url.PathEscape(repository.owner) + "/" + url.PathEscape(repository.name) + "/" + suffix
	endpoint.RawQuery = ""
	if query != nil {
		endpoint.RawQuery = query.Encode()
	}
	return &endpoint
}

func expectedGitHost(apiHost string) string {
	if sameHost(apiHost, "api.github.com") {
		return "github.com"
	}
	return apiHost
}

func apiBaseURLForGitHost(host string) string {
	if sameHost(host, defaultGitHubHost) {
		return defaultAPIBaseURL
	}
	return "https://" + strings.TrimSpace(host) + "/api/v3"
}

func sameHost(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}

func decodeResponse(reader io.Reader, target any) error {
	payload, err := io.ReadAll(io.LimitReader(reader, maxResponseBytes+1))
	if err != nil {
		return responseDecodeProblem("GitHub API response", err)
	}
	if len(payload) > maxResponseBytes {
		return responseDecodeProblem("GitHub API response", nil)
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return responseDecodeProblem("GitHub API response", err)
	}
	return nil
}

func responseProblem(status int, operation string) error {
	return responseProblemForAPIError(status, operation, githubAPIErrorResponse{})
}

func responseProblemWithBody(response *http.Response, operation string) error {
	var apiError githubAPIErrorResponse
	if err := decodeResponse(response.Body, &apiError); err != nil {
		return responseProblem(response.StatusCode, operation)
	}
	return responseProblemForAPIError(response.StatusCode, operation, apiError)
}

func responseProblemForAPIError(status int, operation string, apiError githubAPIErrorResponse) error {
	if status == http.StatusUnauthorized || status == http.StatusForbidden || githubAPIErrorIndicatesPermission(apiError) {
		return problem.New(problem.Details{
			Code:        problem.CodeExternalCommandFailed,
			Category:    problem.CategoryExternal,
			Field:       "GitHub pull request authorization",
			Actual:      http.StatusText(status),
			Expected:    "a GitHub App session with Pull requests: write and Contents: write on the head repository",
			Rule:        "pull-request publication must use an app authorized for the selected repository and source branch",
			Remediation: "verify the GitHub App installation, repository selection, and Pull requests/Contents permissions before " + operation,
		})
	}
	if status == http.StatusNotFound {
		return problem.New(problem.Details{
			Code:        problem.CodeExternalCommandFailed,
			Category:    problem.CategoryExternal,
			Field:       "GitHub pull request repository visibility",
			Actual:      http.StatusText(status),
			Expected:    "a repository and source branch visible to the GitHub App",
			Rule:        "pull-request publication must address a repository visible to the selected app session",
			Remediation: "verify the repository installation boundary and source branch visibility before " + operation,
		})
	}
	if status == http.StatusUnprocessableEntity {
		return githubValidationProblem(operation, apiError)
	}
	return problem.New(problem.Details{
		Code:        problem.CodeExternalCommandFailed,
		Category:    problem.CategoryExternal,
		Field:       "GitHub pull request",
		Actual:      http.StatusText(status),
		Expected:    "a successful GitHub API response",
		Rule:        "GitHub pull-request publication must complete without an API error",
		Remediation: operation + " after checking GitHub permissions, branch visibility, and repository access",
	})
}

func githubValidationProblem(operation string, apiError githubAPIErrorResponse) error {
	if githubAPIErrorIndicatesDuplicatePullRequest(apiError) {
		return problem.New(problem.Details{
			Code:        problem.CodeExternalCommandFailed,
			Category:    problem.CategoryExternal,
			Field:       "GitHub pull request duplicate",
			Actual:      "an equivalent pull request already exists",
			Expected:    "at most one pull request for the selected source and target branches",
			Rule:        "pull-request publication must return an existing equivalent pull request instead of creating a duplicate",
			Remediation: "inspect the existing pull request or choose the intended source and target branches before " + operation,
		})
	}
	for _, validationError := range apiError.Errors {
		switch strings.ToLower(strings.TrimSpace(validationError.Field)) {
		case "head":
			return problem.New(problem.Details{
				Code:        problem.CodeExternalCommandFailed,
				Category:    problem.CategoryExternal,
				Field:       "GitHub pull request source branch",
				Actual:      "GitHub rejected the source branch",
				Expected:    "a pushed source branch visible to the GitHub App",
				Rule:        "pull-request publication must use an existing source branch in the selected repository",
				Remediation: "verify that the source branch was pushed and remains visible to the GitHub App before " + operation,
			})
		case "base":
			return problem.New(problem.Details{
				Code:        problem.CodeExternalCommandFailed,
				Category:    problem.CategoryExternal,
				Field:       "GitHub pull request target branch",
				Actual:      "GitHub rejected the target branch",
				Expected:    "an existing target branch in the selected repository",
				Rule:        "pull-request publication must target an existing repository branch",
				Remediation: "verify the governed target branch and repository visibility before " + operation,
			})
		case "title":
			return problem.New(problem.Details{
				Code:        problem.CodeExternalCommandFailed,
				Category:    problem.CategoryExternal,
				Field:       "GitHub pull request title",
				Actual:      "GitHub rejected the pull-request title",
				Expected:    "a GitHub-accepted pull-request title",
				Rule:        "pull-request publication must provide a valid title",
				Remediation: "repair the governed pull-request title before " + operation,
			})
		}
	}
	return problem.New(problem.Details{
		Code:        problem.CodeExternalCommandFailed,
		Category:    problem.CategoryExternal,
		Field:       "GitHub pull request validation",
		Actual:      "GitHub rejected the pull-request semantics",
		Expected:    "a GitHub-accepted source, target, and title combination",
		Rule:        "pull-request publication must satisfy GitHub validation without exposing provider response content",
		Remediation: "verify the governed pull-request intent and repository state before " + operation,
	})
}

func githubAPIErrorIndicatesPermission(apiError githubAPIErrorResponse) bool {
	return githubAPIErrorContains(apiError, "resource not accessible by integration") ||
		githubAPIErrorContains(apiError, "resource not accessible by personal access token")
}

func githubAPIErrorIndicatesDuplicatePullRequest(apiError githubAPIErrorResponse) bool {
	for _, message := range githubAPIErrorMessages(apiError) {
		if strings.Contains(message, "pull request") && strings.Contains(message, "already exists") {
			return true
		}
	}
	for _, validationError := range apiError.Errors {
		if strings.EqualFold(strings.TrimSpace(validationError.Code), "already_exists") {
			return true
		}
	}
	return false
}

func githubAPIErrorContains(apiError githubAPIErrorResponse, expected string) bool {
	for _, message := range githubAPIErrorMessages(apiError) {
		if strings.Contains(message, expected) {
			return true
		}
	}
	return false
}

func githubAPIErrorMessages(apiError githubAPIErrorResponse) []string {
	messages := make([]string, 0, len(apiError.Errors)+1)
	if message := strings.ToLower(strings.TrimSpace(apiError.Message)); message != "" {
		messages = append(messages, message)
	}
	for _, validationError := range apiError.Errors {
		if message := strings.ToLower(strings.TrimSpace(validationError.Message)); message != "" {
			messages = append(messages, message)
		}
	}
	return messages
}

func responseDecodeProblem(field string, cause error) error {
	return problem.Wrap(problem.Details{
		Code:        problem.CodeExternalCommandFailed,
		Category:    problem.CategoryExternal,
		Field:       field,
		Expected:    "a bounded valid JSON response containing html_url",
		Rule:        "GitHub responses must provide a usable pull-request URL",
		Remediation: "retry after checking the GitHub API endpoint and report malformed responses",
	}, cause)
}

func configurationProblem(field, expected, remediation string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeConfigurationInvalid,
		Category:    problem.CategoryConfig,
		Field:       field,
		Expected:    expected,
		Rule:        "GitHub pull-request publication requires explicit valid external configuration",
		Remediation: remediation,
	})
}

var _ port.PullRequestPublisher = (*Publisher)(nil)
var _ port.PullRequestPublisherPreflight = (*Publisher)(nil)
