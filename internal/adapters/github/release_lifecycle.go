package github

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

const (
	releaseWorkflowWaitLimit    = 30 * time.Second
	releasePromotionPageSize    = 100
	lifecycleDiagnosticMaxBytes = 4096
)

var releaseRequestIDGenerator = newReleaseRequestID
var releaseRandomReader io.Reader = rand.Reader

type workflowDispatchRequest struct {
	Ref    string            `json:"ref"`
	Inputs map[string]string `json:"inputs"`
}

type workflowRunsResponse struct {
	WorkflowRuns []workflowRunResponse `json:"workflow_runs"`
}

type workflowRunResponse struct {
	Status       string `json:"status"`
	Conclusion   string `json:"conclusion"`
	HTMLURL      string `json:"html_url"`
	DisplayTitle string `json:"display_title"`
}

type releasePullRequestResponse struct {
	Number   int        `json:"number"`
	HTMLURL  string     `json:"html_url"`
	MergedAt *time.Time `json:"merged_at"`
	// MergeCommitSHA is resolved from GraphQL because current REST pull-request
	// payloads no longer expose merge_commit_sha.
	MergeCommitSHA string                           `json:"-"`
	Base           releasePullRequestBranchResponse `json:"base"`
	Head           releasePullRequestBranchResponse `json:"head"`
}

type releasePullRequestBranchResponse struct {
	Ref        string                               `json:"ref"`
	Repository releasePullRequestRepositoryResponse `json:"repo"`
}

type releasePullRequestRepositoryResponse struct {
	FullName string `json:"full_name"`
}

type graphQLPromotionRequest struct {
	Query     string                    `json:"query"`
	Variables graphQLPromotionVariables `json:"variables"`
}

type graphQLPromotionVariables struct {
	Owner  string `json:"owner"`
	Name   string `json:"name"`
	Number int    `json:"number"`
}

type graphQLPromotionResponse struct {
	Data struct {
		Repository struct {
			PullRequest *graphQLPromotionPullRequestResponse `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
	Errors []graphQLErrorResponse `json:"errors"`
}

type graphQLErrorResponse struct {
	Message string `json:"message"`
}

type graphQLPromotionPullRequestResponse struct {
	Number         int                                `json:"number"`
	HTMLURL        string                             `json:"url"`
	MergedAt       *time.Time                         `json:"mergedAt"`
	BaseRefName    string                             `json:"baseRefName"`
	HeadRefName    string                             `json:"headRefName"`
	BaseRepository graphQLPromotionRepositoryResponse `json:"baseRepository"`
	HeadRepository graphQLPromotionRepositoryResponse `json:"headRepository"`
	MergeCommit    *graphQLPromotionCommitResponse    `json:"mergeCommit"`
}

type graphQLPromotionRepositoryResponse struct {
	NameWithOwner string `json:"nameWithOwner"`
}

type graphQLPromotionCommitResponse struct {
	OID string `json:"oid"`
}

type gitReferenceResponse struct {
	Object gitObjectReference `json:"object"`
}

type gitTagResponse struct {
	Object gitObjectReference `json:"object"`
}

type gitObjectReference struct {
	SHA  string `json:"sha"`
	Type string `json:"type"`
}

type releaseResponse struct {
	HTMLURL string `json:"html_url"`
	Draft   bool   `json:"draft"`
}

type compareResponse struct {
	AheadBy int               `json:"ahead_by"`
	Files   []json.RawMessage `json:"files"`
}

// DispatchSharedLine starts the named protected-line workflow, waits for its
// correlated result, and leaves Git verification of the created reference to
// the application service.
func (publisher *Publisher) DispatchSharedLine(
	ctx context.Context,
	request port.SharedLineDispatchRequest,
) (port.SharedLineDispatchResult, error) {
	apiBase, repository, err := publisher.lifecycleTarget(request.RemoteURL)
	if err != nil {
		return port.SharedLineDispatchResult{}, err
	}
	if request.Branch.IsZero() || !validWorkflowFile(request.Workflow) || strings.TrimSpace(request.Ref) == "" {
		return port.SharedLineDispatchResult{}, lifecycleConfigurationProblem(
			"protected-line dispatch",
			"a branch, workflow file, and workflow reference",
			"prepare a complete protected-line intent before dispatching it",
		)
	}
	requestID, err := releaseRequestIDGenerator()
	if err != nil {
		return port.SharedLineDispatchResult{}, lifecycleExternalProblem(
			"generate a protected-line workflow request identifier",
			err,
		)
	}
	inputs := make(map[string]string, len(request.Inputs)+1)
	for key, value := range request.Inputs {
		inputs[key] = value
	}
	inputs["request_id"] = requestID
	body, _ := json.Marshal(workflowDispatchRequest{
		Ref:    request.Ref,
		Inputs: inputs,
	})
	endpoint := workflowEndpoint(apiBase, repository, request.Workflow, "/dispatches", nil)
	response, err := publisher.request(ctx, repository, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return port.SharedLineDispatchResult{}, err
	}
	defer response.Body.Close()
	if !isSuccessfulHTTPStatus(response.StatusCode) {
		return port.SharedLineDispatchResult{}, lifecycleResponseProblem(
			response,
			"dispatch the protected-line creation workflow",
		)
	}
	runURL, err := publisher.waitForWorkflowRun(ctx, apiBase, repository, request.Workflow, requestID)
	if err != nil {
		return port.SharedLineDispatchResult{}, err
	}
	return port.SharedLineDispatchResult{
		WorkflowRunURL: runURL,
		Branch:         request.Branch,
	}, nil
}

// VerifyReleaseReconciliation proves the promotion, immutable tag, published
// release, and effective release-to-develop delta before a backmerge can be
// prepared.
func (publisher *Publisher) VerifyReleaseReconciliation(
	ctx context.Context,
	request port.ReleaseReconciliationRequest,
) (port.ReleaseReconciliationEvidence, error) {
	apiBase, repository, err := publisher.lifecycleTarget(request.RemoteURL)
	if err != nil {
		return port.ReleaseReconciliationEvidence{}, err
	}
	if request.Release.Family().String() != "release" {
		return port.ReleaseReconciliationEvidence{}, lifecycleConfigurationProblem(
			"release reconciliation",
			"a release/<semver> branch",
			"select the completed release branch to reconcile with develop",
		)
	}
	version, _ := request.Release.ReleaseVersion()
	promotion, err := publisher.mergedPromotion(ctx, apiBase, repository, request.Release.String())
	if err != nil {
		return port.ReleaseReconciliationEvidence{}, err
	}
	tag := "v" + version.String()
	tagCommit, err := publisher.tagCommit(ctx, apiBase, repository, tag)
	if err != nil {
		return port.ReleaseReconciliationEvidence{}, err
	}
	if tagCommit != promotion.MergeCommitSHA {
		return port.ReleaseReconciliationEvidence{}, lifecycleConfigurationProblem(
			"release tag",
			"an immutable tag pointing at the release promotion merge commit",
			"wait for the release tag workflow to tag the exact main promotion merge",
		)
	}
	releaseURL, err := publisher.publishedReleaseURL(ctx, apiBase, repository, tag)
	if err != nil {
		return port.ReleaseReconciliationEvidence{}, err
	}
	effectiveDelta, err := publisher.hasEffectiveReleaseDelta(ctx, apiBase, repository, request.Release.String())
	if err != nil {
		return port.ReleaseReconciliationEvidence{}, err
	}
	return port.ReleaseReconciliationEvidence{
		PromotionPullRequestURL: promotion.HTMLURL,
		PromotionMergeCommit:    promotion.MergeCommitSHA,
		Tag:                     tag,
		ReleaseURL:              releaseURL,
		EffectiveDelta:          effectiveDelta,
	}, nil
}

func (publisher *Publisher) lifecycleTarget(remoteURL string) (*url.URL, repositoryRef, error) {
	if publisher == nil {
		return nil, repositoryRef{}, lifecycleConfigurationProblem(
			"GitHub release lifecycle",
			"a configured GitHub lifecycle provider",
			"configure --pull-request-provider github before requesting release dispatch or reconciliation",
		)
	}
	if publisher.resolver == nil {
		return nil, repositoryRef{}, lifecycleConfigurationProblem(
			"GitHub App session",
			"a configured GitHub App credential resolver",
			"run auth login github or configure the managed credential broker before requesting release lifecycle operations",
		)
	}
	repository, err := parseRepositoryRemote(remoteURL)
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
		return nil, repositoryRef{}, lifecycleConfigurationProblem(
			"GitHub remote",
			"a remote hosted by "+expectedGitHost(apiBase.Hostname()),
			"select a remote hosted by the configured GitHub App host",
		)
	}
	return apiBase, repository, nil
}

func (publisher *Publisher) waitForWorkflowRun(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	workflow string,
	requestID string,
) (string, error) {
	waitLimit := releaseWorkflowWaitLimit
	if publisher.client != nil && publisher.client.Timeout > 0 && publisher.client.Timeout < waitLimit {
		waitLimit = publisher.client.Timeout
	}
	waitContext, cancel := context.WithTimeout(ctx, waitLimit)
	defer cancel()

	for {
		endpoint := workflowEndpoint(apiBase, repository, workflow, "/runs", url.Values{
			"event":    {"workflow_dispatch"},
			"per_page": {"100"},
		})
		response, err := publisher.request(waitContext, repository, http.MethodGet, endpoint, nil)
		if err != nil {
			return "", err
		}
		var runs workflowRunsResponse
		decodeErr := decodeResponse(response.Body, &runs)
		status := response.StatusCode
		_ = response.Body.Close()
		if decodeErr != nil {
			return "", decodeErr
		}
		if status != http.StatusOK {
			return "", lifecycleResponseProblem(response, "inspect the protected-line workflow run")
		}
		for _, run := range runs.WorkflowRuns {
			if !strings.Contains(run.DisplayTitle, requestID) {
				continue
			}
			if run.Status != "completed" {
				break
			}
			if run.Conclusion != "success" || strings.TrimSpace(run.HTMLURL) == "" {
				return "", lifecycleConfigurationProblem(
					"protected-line workflow",
					"a successful provider workflow run with a usable URL",
					"inspect and correct the protected-line workflow before retrying the release request",
				)
			}
			return run.HTMLURL, nil
		}
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-waitContext.Done():
			timer.Stop()
			return "", lifecycleExternalProblem(
				"wait for the protected-line workflow result",
				waitContext.Err(),
			)
		case <-timer.C:
		}
	}
}

func (publisher *Publisher) mergedPromotion(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	release string,
) (releasePullRequestResponse, error) {
	for page := 1; ; page++ {
		query := url.Values{
			"base":     {"main"},
			"page":     {strconv.Itoa(page)},
			"per_page": {strconv.Itoa(releasePromotionPageSize)},
			"state":    {"closed"},
		}
		endpoint := repositoryEndpoint(apiBase, repository, "pulls", query)
		response, err := publisher.request(ctx, repository, http.MethodGet, endpoint, nil)
		if err != nil {
			return releasePullRequestResponse{}, err
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			return releasePullRequestResponse{}, lifecycleResponseProblem(response, "inspect the release promotion pull request")
		}
		var pullRequests []releasePullRequestResponse
		decodeErr := decodeResponse(response.Body, &pullRequests)
		_ = response.Body.Close()
		if decodeErr != nil {
			return releasePullRequestResponse{}, decodeErr
		}
		for _, pullRequest := range pullRequests {
			if isMergedReleasePromotion(pullRequest, repository, release) {
				if strings.TrimSpace(pullRequest.MergeCommitSHA) == "" {
					mergeCommitSHA, err := publisher.promotionMergeCommit(ctx, apiBase, repository, release, pullRequest)
					if err != nil {
						return releasePullRequestResponse{}, err
					}
					pullRequest.MergeCommitSHA = mergeCommitSHA
				}
				return pullRequest, nil
			}
		}
		if len(pullRequests) < releasePromotionPageSize {
			break
		}
	}
	return releasePullRequestResponse{}, lifecyclePromotionNotFoundProblem(release)
}

func isMergedReleasePromotion(
	pullRequest releasePullRequestResponse,
	repository repositoryRef,
	release string,
) bool {
	fullName := repository.owner + "/" + repository.name
	return pullRequest.Base.Ref == "main" &&
		strings.EqualFold(strings.TrimSpace(pullRequest.Base.Repository.FullName), fullName) &&
		pullRequest.Head.Ref == release &&
		strings.EqualFold(strings.TrimSpace(pullRequest.Head.Repository.FullName), fullName) &&
		pullRequest.Number > 0 &&
		pullRequest.MergedAt != nil &&
		strings.TrimSpace(pullRequest.HTMLURL) != ""
}

const graphQLPromotionQuery = `query ReleasePromotion($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      number
      url
      mergedAt
      baseRefName
      headRefName
      baseRepository {
        nameWithOwner
      }
      headRepository {
        nameWithOwner
      }
      mergeCommit {
        oid
      }
    }
  }
}`

func (publisher *Publisher) promotionMergeCommit(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	release string,
	pullRequest releasePullRequestResponse,
) (string, error) {
	body, _ := json.Marshal(graphQLPromotionRequest{
		Query: graphQLPromotionQuery,
		Variables: graphQLPromotionVariables{
			Owner:  repository.owner,
			Name:   repository.name,
			Number: pullRequest.Number,
		},
	})
	response, err := publisher.request(
		ctx,
		repository,
		http.MethodPost,
		graphQLPromotionEndpoint(apiBase),
		bytes.NewReader(body),
	)
	if err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return "", lifecycleResponseProblem(response, "resolve the release promotion merge commit")
	}
	var graphQLResponse graphQLPromotionResponse
	decodeErr := decodeResponse(response.Body, &graphQLResponse)
	_ = response.Body.Close()
	if decodeErr != nil {
		return "", decodeErr
	}
	if len(graphQLResponse.Errors) != 0 ||
		!isMergedGraphQLPromotion(graphQLResponse.Data.Repository.PullRequest, repository, release, pullRequest) {
		return "", lifecyclePromotionNotFoundProblem(release)
	}
	return graphQLResponse.Data.Repository.PullRequest.MergeCommit.OID, nil
}

func isMergedGraphQLPromotion(
	pullRequest *graphQLPromotionPullRequestResponse,
	repository repositoryRef,
	release string,
	restPullRequest releasePullRequestResponse,
) bool {
	if pullRequest == nil || pullRequest.MergeCommit == nil {
		return false
	}
	fullName := repository.owner + "/" + repository.name
	return pullRequest.Number == restPullRequest.Number &&
		pullRequest.BaseRefName == "main" &&
		strings.EqualFold(strings.TrimSpace(pullRequest.BaseRepository.NameWithOwner), fullName) &&
		pullRequest.HeadRefName == release &&
		strings.EqualFold(strings.TrimSpace(pullRequest.HeadRepository.NameWithOwner), fullName) &&
		pullRequest.MergedAt != nil &&
		strings.TrimSpace(pullRequest.HTMLURL) == strings.TrimSpace(restPullRequest.HTMLURL) &&
		strings.TrimSpace(pullRequest.MergeCommit.OID) != ""
}

func graphQLPromotionEndpoint(apiBase *url.URL) *url.URL {
	endpoint := *apiBase
	endpoint.Path = strings.TrimSuffix(strings.TrimRight(apiBase.Path, "/"), "/v3") + "/graphql"
	endpoint.RawQuery = ""
	return &endpoint
}

func lifecyclePromotionNotFoundProblem(release string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeConfigurationInvalid,
		Category:    problem.CategoryConfig,
		Field:       "release promotion",
		Actual:      "no merged " + release + " -> main pull request with complete provider evidence",
		Expected:    "a merged release/<semver> -> main pull request with an exact source, target, merge SHA, and URL",
		Rule:        "release lifecycle automation requires verified provider evidence",
		Remediation: "verify the provider-visible promotion source, target, merge status, merge SHA, and pull request URL before requesting backmerge reconciliation",
	})
}

func (publisher *Publisher) tagCommit(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	tag string,
) (string, error) {
	endpoint := repositoryEndpoint(apiBase, repository, "git/ref/tags/"+tag, nil)
	response, err := publisher.request(ctx, repository, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", lifecycleResponseProblem(response, "inspect the immutable release tag")
	}
	var reference gitReferenceResponse
	if err := decodeResponse(response.Body, &reference); err != nil {
		return "", err
	}
	object := reference.Object
	if object.Type == "tag" {
		tagEndpoint := repositoryEndpoint(apiBase, repository, "git/tags/"+object.SHA, nil)
		tagResponse, err := publisher.request(ctx, repository, http.MethodGet, tagEndpoint, nil)
		if err != nil {
			return "", err
		}
		defer tagResponse.Body.Close()
		if tagResponse.StatusCode != http.StatusOK {
			return "", lifecycleResponseProblem(tagResponse, "resolve the annotated release tag")
		}
		var annotated gitTagResponse
		if err := decodeResponse(tagResponse.Body, &annotated); err != nil {
			return "", err
		}
		object = annotated.Object
	}
	if object.Type != "commit" || strings.TrimSpace(object.SHA) == "" {
		return "", lifecycleConfigurationProblem(
			"release tag",
			"an annotated or lightweight tag resolving to a commit",
			"repair the release tag before requesting backmerge reconciliation",
		)
	}
	return object.SHA, nil
}

func (publisher *Publisher) publishedReleaseURL(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	tag string,
) (string, error) {
	endpoint := repositoryEndpoint(apiBase, repository, "releases/tags/"+tag, nil)
	response, err := publisher.request(ctx, repository, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", lifecycleResponseProblem(response, "inspect the published GitHub release")
	}
	var release releaseResponse
	if err := decodeResponse(response.Body, &release); err != nil {
		return "", err
	}
	if release.Draft || strings.TrimSpace(release.HTMLURL) == "" {
		return "", lifecycleConfigurationProblem(
			"published release",
			"a non-draft GitHub release with a usable URL",
			"complete the artifact publication before requesting backmerge reconciliation",
		)
	}
	return release.HTMLURL, nil
}

func (publisher *Publisher) hasEffectiveReleaseDelta(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	release string,
) (bool, error) {
	endpoint := repositoryEndpoint(apiBase, repository, "compare/develop..."+release, nil)
	response, err := publisher.request(ctx, repository, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false, lifecycleResponseProblem(response, "compare the released line with develop")
	}
	var comparison compareResponse
	if err := decodeResponse(response.Body, &comparison); err != nil {
		return false, err
	}
	return comparison.AheadBy > 0 && len(comparison.Files) > 0, nil
}

func workflowEndpoint(
	base *url.URL,
	repository repositoryRef,
	workflow string,
	suffix string,
	query url.Values,
) *url.URL {
	return repositoryEndpoint(base, repository, "actions/workflows/"+workflow+suffix, query)
}

func validWorkflowFile(value string) bool {
	return strings.HasSuffix(value, ".yml") &&
		!strings.ContainsAny(value, "/\\\r\n\t") &&
		strings.TrimSpace(value) == value &&
		value != ""
}

// isSuccessfulHTTPStatus accepts any successful dispatch response. The
// correlated workflow run remains the authoritative proof that the requested
// protected-line operation completed.
func isSuccessfulHTTPStatus(status int) bool {
	return status >= http.StatusOK && status < http.StatusMultipleChoices
}

func newReleaseRequestID() (string, error) {
	var value [12]byte
	if _, err := io.ReadFull(releaseRandomReader, value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func lifecycleResponseProblem(response *http.Response, operation string) error {
	status := 0
	if response != nil {
		status = response.StatusCode
	}
	return problem.New(problem.Details{
		Code:        problem.CodeExternalCommandFailed,
		Category:    problem.CategoryExternal,
		Field:       "GitHub release lifecycle",
		Actual:      http.StatusText(status),
		Diagnostic:  lifecycleErrorDiagnostic(response),
		Expected:    "a successful GitHub API response",
		Rule:        "release lifecycle operations must complete through the configured GitHub adapter",
		Remediation: operation + " after checking GitHub App permissions, workflow availability, and repository access",
	})
}

// lifecycleErrorDiagnostic surfaces the provider's own failure message for
// diagnosability. No redaction occurs: the request is secret-free by
// construction and the credential is header-isolated, so the provider
// diagnostic message cannot carry secrets. Canonical rationale:
// docs/conventions/hosting-platforms/github/workflows/provider-fehlerdiagnostik.md
func lifecycleErrorDiagnostic(response *http.Response) string {
	if response == nil || response.Body == nil {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, lifecycleDiagnosticMaxBytes+1))
	if err != nil {
		return ""
	}
	truncated := len(body) > lifecycleDiagnosticMaxBytes
	if truncated {
		body = body[:lifecycleDiagnosticMaxBytes]
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		return ""
	}
	var envelope struct {
		Message string `json:"message"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(text), &envelope); err == nil {
		messages := make([]string, 0, len(envelope.Errors)+1)
		if message := strings.TrimSpace(envelope.Message); message != "" {
			messages = append(messages, message)
		}
		for _, entry := range envelope.Errors {
			if message := strings.TrimSpace(entry.Message); message != "" {
				messages = append(messages, message)
			}
		}
		if len(messages) == 0 {
			return ""
		}
		text = strings.Join(messages, "; ")
	}
	if truncated {
		text += " …[truncated]"
	}
	return text
}

func lifecycleConfigurationProblem(field, expected, remediation string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeConfigurationInvalid,
		Category:    problem.CategoryConfig,
		Field:       field,
		Expected:    expected,
		Rule:        "release lifecycle automation requires verified provider evidence",
		Remediation: remediation,
	})
}

func lifecycleExternalProblem(operation string, cause error) error {
	return problem.Wrap(problem.Details{
		Code:        problem.CodeExternalCommandFailed,
		Category:    problem.CategoryExternal,
		Field:       "GitHub release lifecycle",
		Expected:    "a completed provider operation",
		Rule:        "release lifecycle automation must observe the provider result",
		Remediation: operation + " after checking GitHub workflow status and network access",
	}, cause)
}

var _ port.ReleaseLifecycleProvider = (*Publisher)(nil)
