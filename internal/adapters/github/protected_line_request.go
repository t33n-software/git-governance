package github

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/releaserequest"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

const (
	protectedLineRequestTask        = "git-governance-protected-line-request"
	protectedLineRequestEnvironment = "release-request"
	protectedLineRequestWorkflow    = "release-control.yml"
	protectedLineExecutorWorkflow   = "execute-protected-line-request.yml"
	protectedLineRequestTTL         = 4 * time.Hour
	protectedLineDeploymentPageSize = 100
	protectedLineDeploymentMaxPages = 10
)

var protectedLineRequestNow = time.Now
var protectedLineRequestNew = releaserequest.New
var protectedLineRequestTransition = func(
	request releaserequest.Request,
	next releaserequest.State,
	now time.Time,
) (releaserequest.Request, error) {
	return request.Transition(next, now)
}
var protectedLineExecutionBind = func(
	request releaserequest.Request,
	runID string,
	now time.Time,
) (releaserequest.Request, error) {
	return request.WithExecutionRun(runID, now)
}

type deploymentResponse struct {
	ID          int64           `json:"id"`
	Ref         string          `json:"ref"`
	Task        string          `json:"task"`
	Environment string          `json:"environment"`
	Payload     json.RawMessage `json:"payload"`
}

type deploymentStatusResponse struct {
	State       string `json:"state"`
	Description string `json:"description"`
}

type createDeploymentRequest struct {
	Ref                   string               `json:"ref"`
	Task                  string               `json:"task"`
	Environment           string               `json:"environment"`
	AutoMerge             bool                 `json:"auto_merge"`
	Payload               protectedLinePayload `json:"payload"`
	TransientEnvironment  bool                 `json:"transient_environment"`
	ProductionEnvironment bool                 `json:"production_environment"`
}

type createDeploymentStatusRequest struct {
	State        string `json:"state"`
	Description  string `json:"description"`
	AutoInactive bool   `json:"auto_inactive"`
	LogURL       string `json:"log_url,omitempty"`
}

type protectedLinePayload struct {
	SchemaVersion    int    `json:"schemaVersion"`
	RequestID        string `json:"requestID"`
	Repository       string `json:"repository"`
	Operation        string `json:"operation"`
	Ticket           string `json:"ticket"`
	Version          string `json:"version"`
	TargetRef        string `json:"targetRef"`
	SourceRef        string `json:"sourceRef"`
	SourceSHA        string `json:"sourceSHA"`
	Requester        string `json:"requester"`
	ExpectedExecutor string `json:"expectedExecutor"`
	ParentRunID      string `json:"parentRunID"`
	ExpiresAt        string `json:"expiresAt"`
	IdempotencyKey   string `json:"idempotencyKey"`
}

type workflowJobsResponse struct {
	Jobs []workflowJobResponse `json:"jobs"`
}

type workflowJobResponse struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

type protectedLineWorkflowRunResponse struct {
	Path       string `json:"path"`
	Event      string `json:"event"`
	HeadBranch string `json:"head_branch"`
	HeadSHA    string `json:"head_sha"`
	Actor      struct {
		Login string `json:"login"`
	} `json:"actor"`
}

// AuthorizeProtectedLineRequest creates the durable audit record only after
// the request controller's environment authorization, then dispatches the
// bound executor without waiting for its separate execution approval.
func (publisher *Publisher) AuthorizeProtectedLineRequest(
	ctx context.Context,
	request port.ProtectedLineRequestAuthorization,
) (port.ProtectedLineRequestResult, error) {
	apiBase, repository, err := publisher.lifecycleTarget(request.RemoteURL)
	if err != nil {
		return port.ProtectedLineRequestResult{}, err
	}
	if err := validateProtectedLineAuthorization(request); err != nil {
		return port.ProtectedLineRequestResult{}, err
	}
	sourceSHA, err := publisher.protectedLineRef(ctx, apiBase, repository, request.Source.String())
	if err != nil {
		return port.ProtectedLineRequestResult{}, err
	}
	idempotencyKey := protectedLineIdempotencyKey(repository, request, sourceSHA)
	if existing, found, err := publisher.findProtectedLineRequest(ctx, apiBase, repository, idempotencyKey, ""); err != nil {
		return port.ProtectedLineRequestResult{}, err
	} else if found {
		return port.ProtectedLineRequestResult{Request: existing}, nil
	}
	if exists, err := publisher.protectedLineRefExists(ctx, apiBase, repository, request.Target.String()); err != nil {
		return port.ProtectedLineRequestResult{}, err
	} else if exists {
		return port.ProtectedLineRequestResult{}, protectedLineProblem(
			"protected line target",
			"an absent release or support target ref before execution",
			"select a new version or recover the matching existing release request instead of overwriting a protected line",
		)
	}
	requestID, err := releaseRequestIDGenerator()
	if err != nil {
		return port.ProtectedLineRequestResult{}, lifecycleExternalProblem(
			"generate a protected-line request identifier",
			err,
		)
	}
	now := protectedLineRequestNow().UTC()
	record, err := protectedLineRequestNew(releaserequest.Input{
		ID:               requestID,
		Repository:       repository.owner + "/" + repository.name,
		Operation:        request.Operation,
		Ticket:           request.Ticket,
		Version:          request.Version,
		Target:           request.Target,
		Source:           request.Source,
		SourceSHA:        sourceSHA,
		Requester:        request.Requester,
		ExpectedExecutor: protectedLineExecutorWorkflow,
		ParentRunID:      request.ParentRunID,
		ExpiresAt:        now.Add(protectedLineRequestTTL),
		IdempotencyKey:   idempotencyKey,
		State:            releaserequest.StateRequested,
	}, now)
	if err != nil {
		return port.ProtectedLineRequestResult{}, err
	}
	if err := publisher.validateProtectedLineWorkflowRun(
		ctx,
		apiBase,
		repository,
		request.ParentRunID,
		protectedLineRequestWorkflow,
		request.Requester,
	); err != nil {
		return port.ProtectedLineRequestResult{}, err
	}
	deployment, err := publisher.createProtectedLineDeployment(ctx, apiBase, repository, record)
	if err != nil {
		return port.ProtectedLineRequestResult{}, err
	}
	record = record.WithDeploymentID(deployment.ID)
	authorized, err := protectedLineRequestTransition(record, releaserequest.StateRequestAuthorized, now)
	if err != nil {
		return port.ProtectedLineRequestResult{}, err
	}
	if err := publisher.storeProtectedLineState(ctx, apiBase, repository, authorized, ""); err != nil {
		return port.ProtectedLineRequestResult{}, err
	}
	awaiting, err := protectedLineRequestTransition(authorized, releaserequest.StateAwaitingExecutionApproval, now)
	if err != nil {
		return port.ProtectedLineRequestResult{}, err
	}
	if err := publisher.storeProtectedLineState(ctx, apiBase, repository, awaiting, ""); err != nil {
		return port.ProtectedLineRequestResult{}, err
	}
	if err := publisher.dispatchProtectedLineExecutor(ctx, apiBase, repository, authorized); err != nil {
		failed, transitionErr := protectedLineRequestTransition(awaiting, releaserequest.StateFailed, now)
		if transitionErr == nil {
			_ = publisher.storeProtectedLineState(ctx, apiBase, repository, failed, "")
		}
		return port.ProtectedLineRequestResult{}, err
	}
	return port.ProtectedLineRequestResult{Request: awaiting}, nil
}

// AuthorizeProtectedLineExecution validates the immutable request immediately
// before the separately approved executor performs its single mutation.
func (publisher *Publisher) AuthorizeProtectedLineExecution(
	ctx context.Context,
	request port.ProtectedLineExecutionAuthorization,
) (port.ProtectedLineExecutionPlan, error) {
	apiBase, repository, err := publisher.lifecycleTarget(request.RemoteURL)
	if err != nil {
		return port.ProtectedLineExecutionPlan{}, err
	}
	if strings.TrimSpace(request.RequestID) == "" || strings.TrimSpace(request.ExecutorRunID) == "" {
		return port.ProtectedLineExecutionPlan{}, protectedLineProblem(
			"protected-line execution request",
			"a request ID and executor workflow run ID",
			"invoke the executor only from the correlated protected-line workflow",
		)
	}
	record, found, err := publisher.findProtectedLineRequest(ctx, apiBase, repository, "", request.RequestID)
	if err != nil {
		return port.ProtectedLineExecutionPlan{}, err
	}
	if !found {
		return port.ProtectedLineExecutionPlan{}, protectedLineProblem(
			"protected-line request",
			"a durable authorized request record",
			"request the protected line through the request controller before execution",
		)
	}
	now := protectedLineRequestNow().UTC()
	if record.Expired(now) {
		expired, transitionErr := protectedLineRequestTransition(record, releaserequest.StateExpired, now)
		if transitionErr == nil {
			_ = publisher.storeProtectedLineState(ctx, apiBase, repository, expired, record.ExecutorRunID())
		}
		return port.ProtectedLineExecutionPlan{}, protectedLineProblem(
			"protected-line request expiry",
			"an unexpired request awaiting execution approval",
			"submit and authorize a new release request before execution",
		)
	}
	if record.State() == releaserequest.StateVerified {
		return port.ProtectedLineExecutionPlan{Request: record, NeedsMutation: false}, nil
	}
	if record.ExpectedExecutor() != protectedLineExecutorWorkflow || !record.CanExecute(now) {
		return port.ProtectedLineExecutionPlan{}, protectedLineProblem(
			"protected-line request state",
			"an unexpired request bound to the protected-line executor and awaiting execution approval",
			"do not invoke the executor directly or reuse a consumed request",
		)
	}
	if err := publisher.validateProtectedLineWorkflowRun(
		ctx,
		apiBase,
		repository,
		request.ExecutorRunID,
		record.ExpectedExecutor(),
		"",
	); err != nil {
		return port.ProtectedLineExecutionPlan{}, err
	}
	executing, err := protectedLineExecutionBind(record, request.ExecutorRunID, now)
	if err != nil {
		return port.ProtectedLineExecutionPlan{}, err
	}
	targetSHA, exists, err := publisher.protectedLineRefIfPresent(ctx, apiBase, repository, executing.Target().String())
	if err != nil {
		return port.ProtectedLineExecutionPlan{}, err
	}
	if exists && targetSHA != executing.SourceSHA() {
		failed, transitionErr := protectedLineRequestTransition(executing, releaserequest.StateFailed, now)
		if transitionErr == nil {
			_ = publisher.storeProtectedLineState(ctx, apiBase, repository, failed, executing.ExecutorRunID())
		}
		return port.ProtectedLineExecutionPlan{}, protectedLineProblem(
			"protected-line target",
			"an absent target or an existing ref at the authorized source SHA",
			"do not overwrite a protected line whose revision differs from the authorized request",
		)
	}
	if err := publisher.storeProtectedLineState(ctx, apiBase, repository, executing, executing.ExecutorRunID()); err != nil {
		return port.ProtectedLineExecutionPlan{}, err
	}
	return port.ProtectedLineExecutionPlan{Request: executing, NeedsMutation: !exists}, nil
}

// FinalizeProtectedLineRequest independently proves one executor result and
// the resulting remote ref. Recovery is read-only with respect to Git refs.
func (publisher *Publisher) FinalizeProtectedLineRequest(
	ctx context.Context,
	request port.ProtectedLineFinalizationRequest,
) (port.ProtectedLineFinalizationResult, error) {
	apiBase, repository, err := publisher.lifecycleTarget(request.RemoteURL)
	if err != nil {
		return port.ProtectedLineFinalizationResult{}, err
	}
	if strings.TrimSpace(request.RequestID) == "" {
		return port.ProtectedLineFinalizationResult{}, protectedLineProblem(
			"protected-line finalization request",
			"a durable request ID",
			"finalize only the correlated protected-line execution request",
		)
	}
	record, found, err := publisher.findProtectedLineRequest(ctx, apiBase, repository, "", request.RequestID)
	if err != nil {
		return port.ProtectedLineFinalizationResult{}, err
	}
	if !found {
		return port.ProtectedLineFinalizationResult{}, protectedLineProblem(
			"protected-line request",
			"an existing durable request record",
			"finalize only a request created by the authorized request controller",
		)
	}
	if record.State() == releaserequest.StateVerified {
		return port.ProtectedLineFinalizationResult{Request: record}, nil
	}
	if request.Recovery {
		if !record.CanRecover() {
			return port.ProtectedLineFinalizationResult{}, protectedLineProblem(
				"protected-line recovery state",
				"a request in verification_pending",
				"use recovery only after the normal finalizer could not determine the result",
			)
		}
		request.ExecutorRunID = record.ExecutorRunID()
	} else {
		switch record.State() {
		case releaserequest.StateExecuting:
			if request.ExecutorRunID != record.ExecutorRunID() {
				return port.ProtectedLineFinalizationResult{}, protectedLineProblem(
					"protected-line finalization binding",
					"the exact executor run bound to an executing request",
					"finalize only from the correlated automatic finalizer job",
				)
			}
		case releaserequest.StateAwaitingExecutionApproval:
			// A failed executor can terminate before it persists its own
			// execution binding. The automatic finalizer still records that
			// failure without granting a retrying mutation path.
		default:
			return port.ProtectedLineFinalizationResult{}, protectedLineProblem(
				"protected-line finalization binding",
				"the exact executor run bound to an executing request",
				"finalize only from the correlated automatic finalizer job",
			)
		}
	}
	if strings.TrimSpace(request.ExecutorRunID) == "" {
		return port.ProtectedLineFinalizationResult{}, protectedLineProblem(
			"protected-line executor run",
			"a recorded executor workflow run",
			"wait for the executor to bind its run before recovery finalization",
		)
	}
	if err := publisher.validateProtectedLineWorkflowRun(
		ctx,
		apiBase,
		repository,
		request.ExecutorRunID,
		record.ExpectedExecutor(),
		"",
	); err != nil {
		return publisher.protectedLineVerificationPending(ctx, apiBase, repository, record, request.ExecutorRunID, err)
	}
	executed, err := publisher.protectedLineExecutorSucceeded(ctx, apiBase, repository, request.ExecutorRunID)
	if err != nil {
		return publisher.protectedLineVerificationPending(ctx, apiBase, repository, record, request.ExecutorRunID, err)
	}
	if !executed {
		failed, transitionErr := protectedLineRequestTransition(record, releaserequest.StateFailed, protectedLineRequestNow().UTC())
		if transitionErr != nil {
			return port.ProtectedLineFinalizationResult{}, transitionErr
		}
		if err := publisher.storeProtectedLineState(ctx, apiBase, repository, failed, request.ExecutorRunID); err != nil {
			return port.ProtectedLineFinalizationResult{}, err
		}
		return port.ProtectedLineFinalizationResult{Request: failed}, nil
	}
	if !request.Recovery && record.State() != releaserequest.StateExecuting {
		failed, transitionErr := protectedLineRequestTransition(record, releaserequest.StateFailed, protectedLineRequestNow().UTC())
		if transitionErr != nil {
			return port.ProtectedLineFinalizationResult{}, transitionErr
		}
		if err := publisher.storeProtectedLineState(ctx, apiBase, repository, failed, request.ExecutorRunID); err != nil {
			return port.ProtectedLineFinalizationResult{}, err
		}
		return port.ProtectedLineFinalizationResult{Request: failed}, nil
	}
	targetSHA, exists, err := publisher.protectedLineRefIfPresent(ctx, apiBase, repository, record.Target().String())
	if err != nil {
		return publisher.protectedLineVerificationPending(ctx, apiBase, repository, record, request.ExecutorRunID, err)
	}
	if !exists || targetSHA != record.SourceSHA() {
		failed, transitionErr := protectedLineRequestTransition(record, releaserequest.StateFailed, protectedLineRequestNow().UTC())
		if transitionErr != nil {
			return port.ProtectedLineFinalizationResult{}, transitionErr
		}
		if err := publisher.storeProtectedLineState(ctx, apiBase, repository, failed, request.ExecutorRunID); err != nil {
			return port.ProtectedLineFinalizationResult{}, err
		}
		return port.ProtectedLineFinalizationResult{Request: failed}, nil
	}
	verified, err := protectedLineRequestTransition(record, releaserequest.StateVerified, protectedLineRequestNow().UTC())
	if err != nil {
		return port.ProtectedLineFinalizationResult{}, err
	}
	if err := publisher.storeProtectedLineState(ctx, apiBase, repository, verified, request.ExecutorRunID); err != nil {
		return port.ProtectedLineFinalizationResult{}, err
	}
	return port.ProtectedLineFinalizationResult{Request: verified}, nil
}

func (publisher *Publisher) protectedLineVerificationPending(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	record releaserequest.Request,
	executorRunID string,
	cause error,
) (port.ProtectedLineFinalizationResult, error) {
	pending, transitionErr := protectedLineRequestTransition(record, releaserequest.StateVerificationPending, protectedLineRequestNow().UTC())
	if transitionErr != nil {
		return port.ProtectedLineFinalizationResult{}, transitionErr
	}
	if err := publisher.storeProtectedLineState(ctx, apiBase, repository, pending, executorRunID); err != nil {
		return port.ProtectedLineFinalizationResult{}, err
	}
	return port.ProtectedLineFinalizationResult{Request: pending}, lifecycleExternalProblem(
		"verify the protected-line execution result",
		cause,
	)
}

func validateProtectedLineAuthorization(request port.ProtectedLineRequestAuthorization) error {
	if request.Ticket.Key().String() == "" || request.Ticket.Number().String() == "" ||
		!validProtectedLineOperation(request.Operation) ||
		request.Source.IsZero() ||
		request.Target.IsZero() ||
		strings.TrimSpace(request.Version) == "" ||
		!validSingleLineValue(request.Requester, 200) ||
		!validRunID(request.ParentRunID) {
		return protectedLineProblem(
			"protected-line request authorization",
			"a ticket, operation, version, source, target, requester, and request-controller run",
			"authorize the complete immutable release request before dispatching execution",
		)
	}
	return nil
}

func (publisher *Publisher) createProtectedLineDeployment(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	record releaserequest.Request,
) (deploymentResponse, error) {
	body, _ := json.Marshal(createDeploymentRequest{
		Ref:                   record.SourceSHA(),
		Task:                  protectedLineRequestTask,
		Environment:           protectedLineRequestEnvironment,
		AutoMerge:             false,
		Payload:               protectedLinePayloadFromRequest(record),
		TransientEnvironment:  false,
		ProductionEnvironment: false,
	})
	response, err := publisher.request(ctx, repository, http.MethodPost, repositoryEndpoint(apiBase, repository, "deployments", nil), bytes.NewReader(body))
	if err != nil {
		return deploymentResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return deploymentResponse{}, lifecycleResponseProblem(response.StatusCode, "persist the protected-line request record")
	}
	var deployment deploymentResponse
	if err := decodeResponse(response.Body, &deployment); err != nil {
		return deploymentResponse{}, err
	}
	if deployment.ID <= 0 {
		return deploymentResponse{}, protectedLineProblem(
			"protected-line request deployment",
			"a durable provider deployment identifier",
			"retry only after the request controller can persist the release request record",
		)
	}
	return deployment, nil
}

func (publisher *Publisher) dispatchProtectedLineExecutor(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	record releaserequest.Request,
) error {
	body, _ := json.Marshal(workflowDispatchRequest{
		Ref: "main",
		Inputs: map[string]string{
			"request_id": record.ID(),
		},
	})
	response, err := publisher.request(
		ctx,
		repository,
		http.MethodPost,
		workflowEndpoint(apiBase, repository, record.ExpectedExecutor(), "/dispatches", nil),
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if !isSuccessfulHTTPStatus(response.StatusCode) {
		return lifecycleResponseProblem(response.StatusCode, "dispatch the bound protected-line executor")
	}
	return nil
}

func (publisher *Publisher) storeProtectedLineState(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	record releaserequest.Request,
	executorRunID string,
) error {
	state, err := deploymentState(record.State())
	if err != nil {
		return err
	}
	body, _ := json.Marshal(createDeploymentStatusRequest{
		State:        state,
		Description:  protectedLineStatusDescription(record.State(), executorRunID),
		AutoInactive: false,
		LogURL:       protectedLineRunURL(repository, executorRunID),
	})
	response, err := publisher.request(
		ctx,
		repository,
		http.MethodPost,
		repositoryEndpoint(apiBase, repository, "deployments/"+strconv.FormatInt(record.DeploymentID(), 10)+"/statuses", nil),
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return lifecycleResponseProblem(response.StatusCode, "write the protected-line request audit state")
	}
	return nil
}

func (publisher *Publisher) findProtectedLineRequest(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	idempotencyKey string,
	requestID string,
) (releaserequest.Request, bool, error) {
	for page := 1; page <= protectedLineDeploymentMaxPages; page++ {
		query := url.Values{
			"environment": {protectedLineRequestEnvironment},
			"task":        {protectedLineRequestTask},
			"per_page":    {strconv.Itoa(protectedLineDeploymentPageSize)},
			"page":        {strconv.Itoa(page)},
		}
		response, err := publisher.request(ctx, repository, http.MethodGet, repositoryEndpoint(apiBase, repository, "deployments", query), nil)
		if err != nil {
			return releaserequest.Request{}, false, err
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			return releaserequest.Request{}, false, lifecycleResponseProblem(response.StatusCode, "inspect protected-line request records")
		}
		var deployments []deploymentResponse
		decodeErr := decodeResponse(response.Body, &deployments)
		_ = response.Body.Close()
		if decodeErr != nil {
			return releaserequest.Request{}, false, decodeErr
		}
		for _, deployment := range deployments {
			record, err := publisher.protectedLineRequestFromDeployment(ctx, apiBase, repository, deployment)
			if err != nil {
				return releaserequest.Request{}, false, err
			}
			if (idempotencyKey != "" && record.IdempotencyKey() == idempotencyKey) ||
				(requestID != "" && record.ID() == requestID) {
				return record, true, nil
			}
		}
		if len(deployments) < protectedLineDeploymentPageSize {
			return releaserequest.Request{}, false, nil
		}
	}
	return releaserequest.Request{}, false, protectedLineProblem(
		"protected-line request lookup",
		"a bounded request-record history",
		"archive or narrow stale request records before creating a new protected-line request",
	)
}

func (publisher *Publisher) protectedLineRequestFromDeployment(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	deployment deploymentResponse,
) (releaserequest.Request, error) {
	if deployment.ID <= 0 || deployment.Task != protectedLineRequestTask || deployment.Environment != protectedLineRequestEnvironment {
		return releaserequest.Request{}, protectedLineProblem(
			"protected-line request deployment",
			"a matching durable release-request deployment",
			"select only records created by the protected-line request controller",
		)
	}
	payload, err := parseProtectedLinePayload(deployment.Payload)
	if err != nil {
		return releaserequest.Request{}, err
	}
	state, executorRunID, err := publisher.protectedLineDeploymentState(ctx, apiBase, repository, deployment.ID)
	if err != nil {
		return releaserequest.Request{}, err
	}
	return protectedLineRequestFromPayload(payload, deployment.ID, state, executorRunID)
}

func (publisher *Publisher) protectedLineDeploymentState(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	deploymentID int64,
) (releaserequest.State, string, error) {
	query := url.Values{"per_page": {"1"}}
	response, err := publisher.request(
		ctx,
		repository,
		http.MethodGet,
		repositoryEndpoint(apiBase, repository, "deployments/"+strconv.FormatInt(deploymentID, 10)+"/statuses", query),
		nil,
	)
	if err != nil {
		return "", "", err
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return "", "", lifecycleResponseProblem(response.StatusCode, "inspect the protected-line request audit state")
	}
	var statuses []deploymentStatusResponse
	decodeErr := decodeResponse(response.Body, &statuses)
	_ = response.Body.Close()
	if decodeErr != nil {
		return "", "", decodeErr
	}
	if len(statuses) == 0 {
		return releaserequest.StateRequested, "", nil
	}
	state, runID, ok := parseProtectedLineStatusDescription(statuses[0].Description)
	if !ok {
		return "", "", protectedLineProblem(
			"protected-line request audit state",
			"a controller-written request state description",
			"do not use a deployment record whose latest status cannot be correlated to the protected-line controller",
		)
	}
	return state, runID, nil
}

func (publisher *Publisher) protectedLineRef(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	ref string,
) (string, error) {
	sha, exists, err := publisher.protectedLineRefIfPresent(ctx, apiBase, repository, ref)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", protectedLineProblem(
			"protected-line source ref",
			"an existing authorized source reference",
			"resolve the release source before authorizing its protected target",
		)
	}
	return sha, nil
}

func (publisher *Publisher) protectedLineRefExists(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	ref string,
) (bool, error) {
	_, exists, err := publisher.protectedLineRefIfPresent(ctx, apiBase, repository, ref)
	return exists, err
}

func (publisher *Publisher) protectedLineRefIfPresent(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	ref string,
) (string, bool, error) {
	response, err := publisher.request(
		ctx,
		repository,
		http.MethodGet,
		repositoryEndpoint(apiBase, repository, "git/ref/heads/"+ref, nil),
		nil,
	)
	if err != nil {
		return "", false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return "", false, nil
	}
	if response.StatusCode != http.StatusOK {
		return "", false, lifecycleResponseProblem(response.StatusCode, "inspect the protected-line source or target ref")
	}
	var reference gitReferenceResponse
	if err := decodeResponse(response.Body, &reference); err != nil {
		return "", false, err
	}
	if reference.Object.Type != "commit" || !isFullCommitID(reference.Object.SHA) {
		return "", false, protectedLineProblem(
			"protected-line ref",
			"a commit-backed branch reference with a full SHA",
			"repair the provider ref before authorizing or finalizing the protected-line request",
		)
	}
	return reference.Object.SHA, true, nil
}

func (publisher *Publisher) protectedLineExecutorSucceeded(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	runID string,
) (bool, error) {
	if !validRunID(runID) {
		return false, protectedLineProblem(
			"protected-line executor run",
			"a positive GitHub Actions run identifier",
			"finalize only the executor run correlated to the durable request",
		)
	}
	response, err := publisher.request(
		ctx,
		repository,
		http.MethodGet,
		repositoryEndpoint(apiBase, repository, "actions/runs/"+runID+"/jobs", nil),
		nil,
	)
	if err != nil {
		return false, err
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return false, lifecycleResponseProblem(response.StatusCode, "inspect the protected-line executor job")
	}
	var jobs workflowJobsResponse
	decodeErr := decodeResponse(response.Body, &jobs)
	_ = response.Body.Close()
	if decodeErr != nil {
		return false, decodeErr
	}
	for _, job := range jobs.Jobs {
		if job.Name == "Execute bound protected-line request" {
			return job.Status == "completed" && job.Conclusion == "success", nil
		}
	}
	return false, protectedLineProblem(
		"protected-line executor job",
		"a completed correlated executor job",
		"wait for the bound execution job to complete before finalization",
	)
}

func (publisher *Publisher) validateProtectedLineWorkflowRun(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	runID string,
	workflow string,
	requester string,
) error {
	if !validRunID(runID) || !validWorkflowFile(workflow) {
		return protectedLineProblem(
			"protected-line workflow run",
			"a positive run identifier and the expected controller workflow",
			"bind the request only to the designated request or execution workflow",
		)
	}
	response, err := publisher.request(
		ctx,
		repository,
		http.MethodGet,
		repositoryEndpoint(apiBase, repository, "actions/runs/"+runID, nil),
		nil,
	)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return lifecycleResponseProblem(response.StatusCode, "inspect the bound protected-line workflow run")
	}
	var run protectedLineWorkflowRunResponse
	decodeErr := decodeResponse(response.Body, &run)
	_ = response.Body.Close()
	if decodeErr != nil {
		return decodeErr
	}
	if run.Path != ".github/workflows/"+workflow ||
		run.Event != "workflow_dispatch" ||
		run.HeadBranch != "main" ||
		!isFullCommitID(run.HeadSHA) ||
		(requester != "" && run.Actor.Login != requester) {
		return protectedLineProblem(
			"protected-line workflow provenance",
			"the exact main-bound workflow-dispatch run and, for requests, the authorized requester",
			"do not bind authorization or execution to an unrelated workflow run",
		)
	}
	return nil
}

func protectedLinePayloadFromRequest(request releaserequest.Request) protectedLinePayload {
	return protectedLinePayload{
		SchemaVersion:    releaserequest.SchemaVersion,
		RequestID:        request.ID(),
		Repository:       request.Repository(),
		Operation:        string(request.Operation()),
		Ticket:           request.Ticket().String(),
		Version:          request.Version(),
		TargetRef:        request.Target().String(),
		SourceRef:        request.Source().String(),
		SourceSHA:        request.SourceSHA(),
		Requester:        request.Requester(),
		ExpectedExecutor: request.ExpectedExecutor(),
		ParentRunID:      request.ParentRunID(),
		ExpiresAt:        request.ExpiresAt().Format(time.RFC3339Nano),
		IdempotencyKey:   request.IdempotencyKey(),
	}
}

func parseProtectedLinePayload(raw json.RawMessage) (protectedLinePayload, error) {
	var payload protectedLinePayload
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return protectedLinePayload{}, protectedLineProblem(
			"protected-line request record",
			"a valid immutable controller payload",
			"recover only a request created by the current protected-line request controller",
		)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return protectedLinePayload{}, protectedLineProblem(
			"protected-line request record",
			"exactly one request payload object",
			"remove trailing payload values before authorizing execution",
		)
	}
	if payload.SchemaVersion != releaserequest.SchemaVersion {
		return protectedLinePayload{}, protectedLineProblem(
			"protected-line request schema",
			fmt.Sprintf("schema version %d", releaserequest.SchemaVersion),
			"create a new request through the current request controller",
		)
	}
	return payload, nil
}

func protectedLineRequestFromPayload(
	payload protectedLinePayload,
	deploymentID int64,
	state releaserequest.State,
	executorRunID string,
) (releaserequest.Request, error) {
	id, err := ticket.ParseID(payload.Ticket)
	if err != nil {
		return releaserequest.Request{}, err
	}
	target, err := branch.ParseName(payload.TargetRef)
	if err != nil {
		return releaserequest.Request{}, err
	}
	source, err := branch.ParseName(payload.SourceRef)
	if err != nil {
		return releaserequest.Request{}, err
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, payload.ExpiresAt)
	if err != nil {
		return releaserequest.Request{}, protectedLineProblem(
			"protected-line request expiry",
			"an RFC3339 authorization expiry",
			"create a new request through the authorized request controller",
		)
	}
	return protectedLineRequestNew(releaserequest.Input{
		ID:               payload.RequestID,
		Repository:       payload.Repository,
		Operation:        releaserequest.Operation(payload.Operation),
		Ticket:           id,
		Version:          payload.Version,
		Target:           target,
		Source:           source,
		SourceSHA:        payload.SourceSHA,
		Requester:        payload.Requester,
		ExpectedExecutor: payload.ExpectedExecutor,
		ParentRunID:      payload.ParentRunID,
		ExecutorRunID:    executorRunID,
		ExpiresAt:        expiresAt,
		IdempotencyKey:   payload.IdempotencyKey,
		DeploymentID:     deploymentID,
		State:            state,
	}, protectedLineRequestNow().UTC())
}

func protectedLineIdempotencyKey(
	repository repositoryRef,
	request port.ProtectedLineRequestAuthorization,
	sourceSHA string,
) string {
	value := strings.Join([]string{
		repository.owner + "/" + repository.name,
		string(request.Operation),
		request.Ticket.String(),
		request.Version,
		request.Source.String(),
		request.Target.String(),
		sourceSHA,
		request.ParentRunID,
	}, "\x00")
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func deploymentState(state releaserequest.State) (string, error) {
	switch state {
	case releaserequest.StateRequested, releaserequest.StateRequestAuthorized:
		return "queued", nil
	case releaserequest.StateAwaitingExecutionApproval, releaserequest.StateVerificationPending:
		return "pending", nil
	case releaserequest.StateExecuting:
		return "in_progress", nil
	case releaserequest.StateVerified:
		return "success", nil
	case releaserequest.StateFailed, releaserequest.StateRejected:
		return "failure", nil
	case releaserequest.StateExpired:
		return "inactive", nil
	default:
		return "", protectedLineProblem(
			"protected-line request state",
			"a known durable request lifecycle state",
			"do not write an unrecognized release request state",
		)
	}
}

func protectedLineStatusDescription(state releaserequest.State, executorRunID string) string {
	if executorRunID == "" {
		return "git-governance protected-line request state=" + string(state)
	}
	return "git-governance protected-line request state=" + string(state) + " executor_run=" + executorRunID
}

func parseProtectedLineStatusDescription(description string) (releaserequest.State, string, bool) {
	const prefix = "git-governance protected-line request state="
	if !strings.HasPrefix(description, prefix) {
		return "", "", false
	}
	parts := strings.Fields(strings.TrimPrefix(description, prefix))
	if len(parts) == 0 {
		return "", "", false
	}
	state := releaserequest.State(parts[0])
	if _, err := deploymentState(state); err != nil {
		return "", "", false
	}
	if len(parts) == 1 {
		return state, "", true
	}
	if len(parts) != 2 || !strings.HasPrefix(parts[1], "executor_run=") {
		return "", "", false
	}
	runID := strings.TrimPrefix(parts[1], "executor_run=")
	if !validRunID(runID) {
		return "", "", false
	}
	return state, runID, true
}

func protectedLineRunURL(repository repositoryRef, runID string) string {
	if !validRunID(runID) {
		return ""
	}
	return "https://github.com/" + repository.owner + "/" + repository.name + "/actions/runs/" + runID
}

func validProtectedLineOperation(operation releaserequest.Operation) bool {
	return operation == releaserequest.OperationRelease || operation == releaserequest.OperationSupport
}

func validSingleLineValue(value string, limit int) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && trimmed == value && len(value) <= limit && !strings.ContainsAny(value, "\r\n\t")
}

func validRunID(value string) bool {
	if value == "" {
		return false
	}
	for _, runeValue := range value {
		if runeValue < '0' || runeValue > '9' {
			return false
		}
	}
	return value[0] != '0'
}

func isFullCommitID(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, runeValue := range value {
		if (runeValue < '0' || runeValue > '9') && (runeValue < 'a' || runeValue > 'f') {
			return false
		}
	}
	return true
}

func protectedLineProblem(field, expected, remediation string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeConfigurationInvalid,
		Category:    problem.CategoryGovernance,
		Field:       field,
		Expected:    expected,
		Rule:        "protected-line execution requires one durable authorized release request and independent finalization evidence",
		Remediation: remediation,
	})
}

var _ port.ProtectedLineRequestProvider = (*Publisher)(nil)
