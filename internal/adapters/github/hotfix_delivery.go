package github

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/CyberT33N/git-governance/internal/application/port"
	"github.com/CyberT33N/git-governance/internal/domain/problem"
)

var hotfixDeliveryWorkflowWaitLimit = 15 * time.Minute

type hotfixPullRequestResponse struct {
	Number   int                              `json:"number"`
	HTMLURL  string                           `json:"html_url"`
	MergedAt *time.Time                       `json:"merged_at"`
	Base     releasePullRequestBranchResponse `json:"base"`
	Head     releasePullRequestBranchResponse `json:"head"`
}

type graphQLHotfixRequest struct {
	Query     string                 `json:"query"`
	Variables graphQLHotfixVariables `json:"variables"`
}

type graphQLHotfixVariables struct {
	Owner  string `json:"owner"`
	Name   string `json:"name"`
	Number int    `json:"number"`
}

type graphQLHotfixResponse struct {
	Data struct {
		Repository struct {
			PullRequest *graphQLHotfixPullRequestResponse `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
	Errors []graphQLHotfixErrorResponse `json:"errors"`
}

type graphQLHotfixErrorResponse struct {
	Message string `json:"message"`
}

type graphQLHotfixPullRequestResponse struct {
	Number         int                             `json:"number"`
	HTMLURL        string                          `json:"url"`
	MergedAt       *time.Time                      `json:"mergedAt"`
	BaseRefName    string                          `json:"baseRefName"`
	HeadRefName    string                          `json:"headRefName"`
	BaseRepository graphQLHotfixRepositoryResponse `json:"baseRepository"`
	HeadRepository graphQLHotfixRepositoryResponse `json:"headRepository"`
	MergeCommit    *graphQLHotfixCommitResponse    `json:"mergeCommit"`
}

type graphQLHotfixRepositoryResponse struct {
	NameWithOwner string `json:"nameWithOwner"`
}

type graphQLHotfixCommitResponse struct {
	OID string `json:"oid"`
}

type pullRequestCommitResponse struct {
	SHA string `json:"sha"`
}

type releaseAssetResponse struct {
	Name string `json:"name"`
}

// VerifyMainHotfixMerge proves the reviewed same-repository main hotfix pull
// request, its GraphQL merge commit, ordered manifest, and tag idempotency
// before a trusted workflow creates an immutable tag.
func (publisher *Publisher) VerifyMainHotfixMerge(
	ctx context.Context,
	request port.MainHotfixDeliveryRequest,
) (port.MainHotfixMergeEvidence, error) {
	apiBase, repository, err := publisher.lifecycleTarget(request.RemoteURL)
	if err != nil {
		return port.MainHotfixMergeEvidence{}, err
	}
	hotfix, err := publisher.verifiedMergedMainHotfix(ctx, apiBase, repository, request)
	if err != nil {
		return port.MainHotfixMergeEvidence{}, err
	}
	tag := "v" + request.Record.TargetVersion().String()
	exists, err := publisher.tagExists(ctx, apiBase, repository, tag)
	if err != nil {
		return port.MainHotfixMergeEvidence{}, err
	}
	if exists {
		return port.MainHotfixMergeEvidence{}, hotfixLifecycleProblem(
			"hotfix patch tag",
			"an absent immutable tag for the reviewed patch version",
			"select a new approved patch version; never replace an existing release tag",
		)
	}
	return port.MainHotfixMergeEvidence{
		PullRequestURL: hotfix.HTMLURL,
		MergeCommit:    hotfix.MergeCommit,
		Tag:            tag,
	}, nil
}

// VerifyMainHotfixDelivery proves the immutable tag, published artifact set,
// and successful artifact workflow after the trusted controller dispatches it.
func (publisher *Publisher) VerifyMainHotfixDelivery(
	ctx context.Context,
	request port.MainHotfixDeliveryRequest,
) (port.MainHotfixDeliveryEvidence, error) {
	apiBase, repository, err := publisher.lifecycleTarget(request.RemoteURL)
	if err != nil {
		return port.MainHotfixDeliveryEvidence{}, err
	}
	hotfix, err := publisher.verifiedMergedMainHotfix(ctx, apiBase, repository, request)
	if err != nil {
		return port.MainHotfixDeliveryEvidence{}, err
	}
	tag := "v" + request.Record.TargetVersion().String()
	tagCommit, err := publisher.tagCommit(ctx, apiBase, repository, tag)
	if err != nil {
		return port.MainHotfixDeliveryEvidence{}, err
	}
	if tagCommit != hotfix.MergeCommit {
		return port.MainHotfixDeliveryEvidence{}, hotfixLifecycleProblem(
			"hotfix patch tag",
			"an immutable tag pointing exactly at the verified main hotfix merge",
			"repair the delivery controller input; do not replace or move an existing tag",
		)
	}
	releaseURL, err := publisher.publishedHotfixReleaseURL(ctx, apiBase, repository, tag)
	if err != nil {
		return port.MainHotfixDeliveryEvidence{}, err
	}
	workflowURL, err := publisher.waitForHotfixArtifactWorkflow(ctx, apiBase, repository, tag)
	if err != nil {
		return port.MainHotfixDeliveryEvidence{}, err
	}
	return port.MainHotfixDeliveryEvidence{
		MainHotfixMergeEvidence: port.MainHotfixMergeEvidence{
			PullRequestURL: hotfix.HTMLURL,
			MergeCommit:    hotfix.MergeCommit,
			Tag:            tag,
		},
		ReleaseURL:     releaseURL,
		WorkflowRunURL: workflowURL,
	}, nil
}

type mergedMainHotfix struct {
	HTMLURL      string
	MergeCommit  string
	PullRequest  int
	SourceBranch string
}

func (publisher *Publisher) verifiedMergedMainHotfix(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	request port.MainHotfixDeliveryRequest,
) (mergedMainHotfix, error) {
	if request.Record.AffectedLine().String() != "main" {
		return mergedMainHotfix{}, hotfixLifecycleProblem(
			"main hotfix delivery",
			"a reviewed main hotfix release record",
			"use the release-line or support-line workflow for a non-main hotfix",
		)
	}
	if _, err := publisher.tagCommit(ctx, apiBase, repository, request.Record.PreviousTag()); err != nil {
		return mergedMainHotfix{}, err
	}
	pullRequest, err := publisher.mergedMainHotfix(ctx, apiBase, repository, request.Record.ExpectedSource().String())
	if err != nil {
		return mergedMainHotfix{}, err
	}
	mergeCommit, err := publisher.mainHotfixMergeCommit(ctx, apiBase, repository, request.Record.ExpectedSource().String(), pullRequest)
	if err != nil {
		return mergedMainHotfix{}, err
	}
	if err := publisher.verifyHotfixManifest(ctx, apiBase, repository, pullRequest.Number, request.Record.Manifest()); err != nil {
		return mergedMainHotfix{}, err
	}
	return mergedMainHotfix{
		HTMLURL:      pullRequest.HTMLURL,
		MergeCommit:  mergeCommit,
		PullRequest:  pullRequest.Number,
		SourceBranch: request.Record.ExpectedSource().String(),
	}, nil
}

func (publisher *Publisher) mergedMainHotfix(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	source string,
) (hotfixPullRequestResponse, error) {
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
			return hotfixPullRequestResponse{}, err
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			return hotfixPullRequestResponse{}, lifecycleResponseProblem(response.StatusCode, "inspect the main hotfix pull request")
		}
		var pullRequests []hotfixPullRequestResponse
		decodeErr := decodeResponse(response.Body, &pullRequests)
		_ = response.Body.Close()
		if decodeErr != nil {
			return hotfixPullRequestResponse{}, decodeErr
		}
		for _, pullRequest := range pullRequests {
			if isMergedMainHotfix(pullRequest, repository, source) {
				return pullRequest, nil
			}
		}
		if len(pullRequests) < releasePromotionPageSize {
			break
		}
	}
	return hotfixPullRequestResponse{}, hotfixLifecycleProblem(
		"main hotfix merge",
		"a merged same-repository hotfix pull request with complete provider evidence",
		"verify the hotfix source, main target, merge status, and repository identity before delivery",
	)
}

func isMergedMainHotfix(
	pullRequest hotfixPullRequestResponse,
	repository repositoryRef,
	source string,
) bool {
	fullName := repository.owner + "/" + repository.name
	return pullRequest.Number > 0 &&
		pullRequest.Base.Ref == "main" &&
		strings.EqualFold(strings.TrimSpace(pullRequest.Base.Repository.FullName), fullName) &&
		pullRequest.Head.Ref == source &&
		strings.EqualFold(strings.TrimSpace(pullRequest.Head.Repository.FullName), fullName) &&
		pullRequest.MergedAt != nil &&
		strings.TrimSpace(pullRequest.HTMLURL) != ""
}

const graphQLMainHotfixQuery = `query MainHotfix($owner: String!, $name: String!, $number: Int!) {
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

func (publisher *Publisher) mainHotfixMergeCommit(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	source string,
	pullRequest hotfixPullRequestResponse,
) (string, error) {
	body, _ := json.Marshal(graphQLHotfixRequest{
		Query: graphQLMainHotfixQuery,
		Variables: graphQLHotfixVariables{
			Owner:  repository.owner,
			Name:   repository.name,
			Number: pullRequest.Number,
		},
	})
	response, err := publisher.request(
		ctx,
		repository,
		http.MethodPost,
		hotfixGraphQLEndpoint(apiBase),
		bytes.NewReader(body),
	)
	if err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return "", lifecycleResponseProblem(response.StatusCode, "resolve the main hotfix merge commit")
	}
	var graphQLResponse graphQLHotfixResponse
	decodeErr := decodeResponse(response.Body, &graphQLResponse)
	_ = response.Body.Close()
	if decodeErr != nil {
		return "", decodeErr
	}
	if len(graphQLResponse.Errors) != 0 ||
		!isMergedGraphQLMainHotfix(graphQLResponse.Data.Repository.PullRequest, repository, source, pullRequest) {
		return "", hotfixLifecycleProblem(
			"main hotfix merge",
			"matching GraphQL merge evidence for the reviewed hotfix pull request",
			"verify the provider-visible pull request identity and merge commit before delivery",
		)
	}
	return graphQLResponse.Data.Repository.PullRequest.MergeCommit.OID, nil
}

func isMergedGraphQLMainHotfix(
	pullRequest *graphQLHotfixPullRequestResponse,
	repository repositoryRef,
	source string,
	restPullRequest hotfixPullRequestResponse,
) bool {
	if pullRequest == nil || pullRequest.MergeCommit == nil {
		return false
	}
	fullName := repository.owner + "/" + repository.name
	return pullRequest.Number == restPullRequest.Number &&
		pullRequest.BaseRefName == "main" &&
		strings.EqualFold(strings.TrimSpace(pullRequest.BaseRepository.NameWithOwner), fullName) &&
		pullRequest.HeadRefName == source &&
		strings.EqualFold(strings.TrimSpace(pullRequest.HeadRepository.NameWithOwner), fullName) &&
		pullRequest.MergedAt != nil &&
		strings.TrimSpace(pullRequest.HTMLURL) == strings.TrimSpace(restPullRequest.HTMLURL) &&
		strings.TrimSpace(pullRequest.MergeCommit.OID) != ""
}

func (publisher *Publisher) verifyHotfixManifest(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	pullRequestNumber int,
	manifest []string,
) error {
	index := 0
	for page := 1; index < len(manifest); page++ {
		endpoint := repositoryEndpoint(
			apiBase,
			repository,
			"pulls/"+strconv.Itoa(pullRequestNumber)+"/commits",
			url.Values{
				"page":     {strconv.Itoa(page)},
				"per_page": {strconv.Itoa(releasePromotionPageSize)},
			},
		)
		response, err := publisher.request(ctx, repository, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			return lifecycleResponseProblem(response.StatusCode, "inspect the main hotfix commit manifest")
		}
		var commits []pullRequestCommitResponse
		decodeErr := decodeResponse(response.Body, &commits)
		_ = response.Body.Close()
		if decodeErr != nil {
			return decodeErr
		}
		for _, commit := range commits {
			if index < len(manifest) && commit.SHA == manifest[index] {
				index++
			}
		}
		if len(commits) < releasePromotionPageSize {
			break
		}
	}
	if index != len(manifest) {
		return hotfixLifecycleProblem(
			"hotfix commit manifest",
			"the exact ordered full-SHA manifest in the merged hotfix pull request",
			"repair the reviewed release record or hotfix commit series before delivery",
		)
	}
	return nil
}

func (publisher *Publisher) tagExists(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	tag string,
) (bool, error) {
	endpoint := repositoryEndpoint(apiBase, repository, "git/ref/tags/"+tag, nil)
	response, err := publisher.request(ctx, repository, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, lifecycleResponseProblem(response.StatusCode, "inspect the proposed immutable hotfix patch tag")
	}
}

func (publisher *Publisher) publishedHotfixReleaseURL(
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
		return "", lifecycleResponseProblem(response.StatusCode, "inspect the published main hotfix release")
	}
	var release struct {
		HTMLURL string                 `json:"html_url"`
		Draft   bool                   `json:"draft"`
		Assets  []releaseAssetResponse `json:"assets"`
	}
	if err := decodeResponse(response.Body, &release); err != nil {
		return "", err
	}
	if release.Draft || strings.TrimSpace(release.HTMLURL) == "" || !hasHotfixArtifactEvidence(release.Assets) {
		return "", hotfixLifecycleProblem(
			"main hotfix delivery evidence",
			"a non-draft release with checksums, payloads, SBOMs, and a Sigstore bundle",
			"wait for the artifact workflow and verify its complete published evidence set",
		)
	}
	return release.HTMLURL, nil
}

func hasHotfixArtifactEvidence(assets []releaseAssetResponse) bool {
	var checksums, payload, sbom, signature bool
	for _, asset := range assets {
		name := strings.ToLower(strings.TrimSpace(asset.Name))
		switch {
		case strings.HasSuffix(name, ".sigstore.json"):
			signature = true
		case strings.HasSuffix(name, ".sbom.json") || strings.HasSuffix(name, ".spdx.json"):
			sbom = true
		case strings.Contains(name, "checksums"):
			checksums = true
		case strings.HasSuffix(name, ".zip") ||
			strings.HasSuffix(name, ".tar.gz") ||
			strings.HasSuffix(name, ".deb") ||
			strings.HasSuffix(name, ".rpm") ||
			strings.HasSuffix(name, ".apk"):
			payload = true
		}
	}
	return checksums && payload && sbom && signature
}

func (publisher *Publisher) waitForHotfixArtifactWorkflow(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	tag string,
) (string, error) {
	waitContext, cancel := context.WithTimeout(ctx, hotfixDeliveryWorkflowWaitLimit)
	defer cancel()

	for {
		endpoint := workflowEndpoint(apiBase, repository, "release.yml", "/runs", url.Values{
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
			return "", lifecycleResponseProblem(status, "inspect the main hotfix artifact workflow")
		}
		for _, run := range runs.WorkflowRuns {
			if run.DisplayTitle != "Release "+tag {
				continue
			}
			if run.Status != "completed" {
				break
			}
			if run.Conclusion != "success" || strings.TrimSpace(run.HTMLURL) == "" {
				return "", hotfixLifecycleProblem(
					"main hotfix artifact workflow",
					"a successful artifact workflow for the immutable patch tag",
					"inspect the artifact workflow and retry delivery verification after its evidence is complete",
				)
			}
			return run.HTMLURL, nil
		}
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-waitContext.Done():
			timer.Stop()
			return "", hotfixLifecycleExternalProblem("wait for the main hotfix artifact workflow", waitContext.Err())
		case <-timer.C:
		}
	}
}

func hotfixGraphQLEndpoint(apiBase *url.URL) *url.URL {
	endpoint := *apiBase
	endpoint.Path = strings.TrimSuffix(strings.TrimRight(apiBase.Path, "/"), "/v3") + "/graphql"
	endpoint.RawQuery = ""
	return &endpoint
}

func hotfixLifecycleProblem(field, expected, remediation string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeConfigurationInvalid,
		Category:    problem.CategoryConfig,
		Field:       field,
		Expected:    expected,
		Rule:        "main hotfix delivery requires verified provider evidence",
		Remediation: remediation,
	})
}

func hotfixLifecycleExternalProblem(operation string, cause error) error {
	return problem.Wrap(problem.Details{
		Code:        problem.CodeExternalCommandFailed,
		Category:    problem.CategoryExternal,
		Field:       "main hotfix delivery",
		Expected:    "a completed provider operation",
		Rule:        "main hotfix delivery must observe the provider result",
		Remediation: operation + " after checking the protected controller and GitHub Actions result",
	}, cause)
}

var _ port.MainHotfixLifecycleProvider = (*Publisher)(nil)
