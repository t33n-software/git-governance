package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/releaserequest"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

func TestProtectedLineRequestAuthorization(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	originalNow := protectedLineRequestNow
	originalID := releaseRequestIDGenerator
	protectedLineRequestNow = func() time.Time { return now }
	releaseRequestIDGenerator = func() (string, error) { return "request-50", nil }
	t.Cleanup(func() {
		protectedLineRequestNow = originalNow
		releaseRequestIDGenerator = originalID
	})

	t.Run("persists, dispatches, and leaves execution approval pending", func(t *testing.T) {
		var (
			statuses   []createDeploymentStatusRequest
			payload    protectedLinePayload
			dispatched bool
		)
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/git/ref/heads/develop":
				writeProtectedLineJSON(t, writer, gitReferenceResponse{Object: gitObjectReference{SHA: strings.Repeat("a", 40), Type: "commit"}})
			case "/repos/acme/governance/deployments":
				if request.Method == http.MethodGet {
					writeProtectedLineJSON(t, writer, []deploymentResponse{})
					return
				}
				var body createDeploymentRequest
				decodeJSON(t, request, &body)
				payload = body.Payload
				if body.Ref != strings.Repeat("a", 40) || body.Task != protectedLineRequestTask ||
					body.Environment != protectedLineRequestEnvironment || body.AutoMerge ||
					body.TransientEnvironment || body.ProductionEnvironment {
					t.Fatalf("deployment request = %#v", body)
				}
				writer.WriteHeader(http.StatusCreated)
				writeProtectedLineJSON(t, writer, deploymentResponse{ID: 71})
			case "/repos/acme/governance/git/ref/heads/release/1.2.0":
				writer.WriteHeader(http.StatusNotFound)
			case "/repos/acme/governance/actions/runs/123":
				writeProtectedLineJSON(t, writer, protectedLineWorkflowRun(protectedLineRequestWorkflow, "release-requester"))
			case "/repos/acme/governance/deployments/71/statuses":
				var status createDeploymentStatusRequest
				decodeJSON(t, request, &status)
				statuses = append(statuses, status)
				writer.WriteHeader(http.StatusCreated)
			case "/repos/acme/governance/actions/workflows/execute-protected-line-request.yml/dispatches":
				var body workflowDispatchRequest
				decodeJSON(t, request, &body)
				if body.Ref != "main" || body.Inputs["request_id"] != "request-50" || len(body.Inputs) != 1 {
					t.Fatalf("executor dispatch = %#v", body)
				}
				if len(statuses) != 2 ||
					statuses[1].Description != protectedLineStatusDescription(releaserequest.StateAwaitingExecutionApproval, "") {
					t.Fatalf("executor dispatched before execution approval state persisted: %#v", statuses)
				}
				dispatched = true
				writer.WriteHeader(http.StatusNoContent)
			default:
				t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
			}
		}))
		defer server.Close()

		publisher := protectedLinePublisher(server)
		result, err := publisher.AuthorizeProtectedLineRequest(context.Background(), protectedLineAuthorization(t, server.URL))
		if err != nil {
			t.Fatal(err)
		}
		if !dispatched || result.Request.ID() != "request-50" || result.Request.DeploymentID() != 71 ||
			result.Request.State() != releaserequest.StateAwaitingExecutionApproval ||
			result.Request.SourceSHA() != strings.Repeat("a", 40) ||
			result.Request.Target().String() != "release/1.2.0" {
			t.Fatalf("authorization result = %#v", result)
		}
		if payload.SchemaVersion != releaserequest.SchemaVersion || payload.RequestID != "request-50" ||
			payload.Ticket != "GOV-50" || payload.ExpectedExecutor != protectedLineExecutorWorkflow ||
			payload.ParentRunID != "123" || payload.ExpiresAt != now.Add(protectedLineRequestTTL).Format(time.RFC3339Nano) {
			t.Fatalf("stored payload = %#v", payload)
		}
		if len(statuses) != 2 ||
			statuses[0].Description != protectedLineStatusDescription(releaserequest.StateRequestAuthorized, "") ||
			statuses[1].Description != protectedLineStatusDescription(releaserequest.StateAwaitingExecutionApproval, "") ||
			statuses[0].State != "queued" || statuses[1].State != "pending" {
			t.Fatalf("statuses = %#v", statuses)
		}
	})

	t.Run("returns an existing idempotent request without mutation", func(t *testing.T) {
		authorization := protectedLineAuthorization(t, "https://example.invalid")
		record := protectedLineRecordWithIdempotency(
			t,
			now,
			releaserequest.StateAwaitingExecutionApproval,
			"",
			protectedLineIdempotencyKey(repositoryRef{owner: "acme", name: "governance"}, authorization, strings.Repeat("a", 40)),
		)
		payload := protectedLinePayloadFromRequest(record)
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/git/ref/heads/develop":
				writeProtectedLineJSON(t, writer, gitReferenceResponse{Object: gitObjectReference{SHA: strings.Repeat("a", 40), Type: "commit"}})
			case "/repos/acme/governance/deployments":
				writeProtectedLineJSON(t, writer, []deploymentResponse{{
					ID:          71,
					Task:        protectedLineRequestTask,
					Environment: protectedLineRequestEnvironment,
					Payload:     mustProtectedLineJSON(t, payload),
				}})
			case "/repos/acme/governance/deployments/71/statuses":
				writeProtectedLineJSON(t, writer, []deploymentStatusResponse{{
					State:       "pending",
					Description: protectedLineStatusDescription(releaserequest.StateAwaitingExecutionApproval, ""),
				}})
			default:
				t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
			}
		}))
		defer server.Close()

		result, err := protectedLinePublisher(server).AuthorizeProtectedLineRequest(context.Background(), protectedLineAuthorization(t, server.URL))
		if err != nil || result.Request.DeploymentID() != 71 || result.Request.ID() != record.ID() {
			t.Fatalf("idempotent result = (%#v, %v)", result, err)
		}
	})

	t.Run("rejects invalid authorization, absent source, and existing target", func(t *testing.T) {
		if _, err := protectedLinePublisher(nil).AuthorizeProtectedLineRequest(context.Background(), port.ProtectedLineRequestAuthorization{}); err == nil {
			t.Fatal("invalid authorization error = nil")
		}

		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/git/ref/heads/develop":
				writer.WriteHeader(http.StatusNotFound)
			default:
				t.Fatalf("unexpected request %s", request.URL.Path)
			}
		}))
		defer server.Close()
		if _, err := protectedLinePublisher(server).AuthorizeProtectedLineRequest(context.Background(), protectedLineAuthorization(t, server.URL)); err == nil {
			t.Fatal("absent source error = nil")
		}

		targetServer := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/git/ref/heads/develop":
				writeProtectedLineJSON(t, writer, gitReferenceResponse{Object: gitObjectReference{SHA: strings.Repeat("a", 40), Type: "commit"}})
			case "/repos/acme/governance/deployments":
				writeProtectedLineJSON(t, writer, []deploymentResponse{})
			case "/repos/acme/governance/git/ref/heads/release/1.2.0":
				writeProtectedLineJSON(t, writer, gitReferenceResponse{Object: gitObjectReference{SHA: strings.Repeat("b", 40), Type: "commit"}})
			default:
				t.Fatalf("unexpected request %s", request.URL.Path)
			}
		}))
		defer targetServer.Close()
		if _, err := protectedLinePublisher(targetServer).AuthorizeProtectedLineRequest(context.Background(), protectedLineAuthorization(t, targetServer.URL)); err == nil {
			t.Fatal("existing target error = nil")
		}

		invalidServer := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			t.Fatalf("invalid authorization made provider request %s", request.URL.Path)
		}))
		defer invalidServer.Close()
		invalid := protectedLineAuthorization(t, invalidServer.URL)
		invalid.Ticket = ticket.ID{}
		if _, err := protectedLinePublisher(invalidServer).AuthorizeProtectedLineRequest(context.Background(), invalid); err == nil {
			t.Fatal("invalid authorization error = nil")
		}
	})
}

func TestProtectedLineExecutionAuthorization(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	originalNow := protectedLineRequestNow
	protectedLineRequestNow = func() time.Time { return now }
	t.Cleanup(func() { protectedLineRequestNow = originalNow })

	t.Run("binds execution and permits one absent target mutation", func(t *testing.T) {
		record := protectedLineRecord(t, now, releaserequest.StateAwaitingExecutionApproval, "")
		server := protectedLineRequestServer(t, record, []deploymentStatusResponse{{
			State:       "pending",
			Description: protectedLineStatusDescription(releaserequest.StateAwaitingExecutionApproval, ""),
		}}, nil, nil)
		defer server.Close()

		plan, err := protectedLinePublisher(server).AuthorizeProtectedLineExecution(context.Background(), protectedLineExecutionAuthorization(server.URL, record.ID(), "456"))
		if err != nil || !plan.NeedsMutation || plan.Request.State() != releaserequest.StateExecuting || plan.Request.ExecutorRunID() != "456" {
			t.Fatalf("execution plan = (%#v, %v)", plan, err)
		}
	})

	t.Run("returns verified requests idempotently and accepts matching existing targets", func(t *testing.T) {
		verified := protectedLineRecord(t, now, releaserequest.StateVerified, "456")
		server := protectedLineRequestServer(t, verified, []deploymentStatusResponse{{
			State:       "success",
			Description: protectedLineStatusDescription(releaserequest.StateVerified, "456"),
		}}, nil, nil)
		defer server.Close()
		plan, err := protectedLinePublisher(server).AuthorizeProtectedLineExecution(context.Background(), protectedLineExecutionAuthorization(server.URL, verified.ID(), "456"))
		if err != nil || plan.NeedsMutation || plan.Request.State() != releaserequest.StateVerified {
			t.Fatalf("verified plan = (%#v, %v)", plan, err)
		}

		awaiting := protectedLineRecord(t, now, releaserequest.StateAwaitingExecutionApproval, "")
		matchingServer := protectedLineRequestServer(t, awaiting, []deploymentStatusResponse{{
			State:       "pending",
			Description: protectedLineStatusDescription(releaserequest.StateAwaitingExecutionApproval, ""),
		}}, map[string]string{"release/1.2.0": strings.Repeat("a", 40)}, nil)
		defer matchingServer.Close()
		plan, err = protectedLinePublisher(matchingServer).AuthorizeProtectedLineExecution(context.Background(), protectedLineExecutionAuthorization(matchingServer.URL, awaiting.ID(), "456"))
		if err != nil || plan.NeedsMutation {
			t.Fatalf("matching target plan = (%#v, %v)", plan, err)
		}
	})

	t.Run("rejects missing, expired, consumed, and mismatched target requests", func(t *testing.T) {
		server := protectedLineRequestServer(t, releaserequest.Request{}, nil, nil, nil)
		defer server.Close()
		if _, err := protectedLinePublisher(server).AuthorizeProtectedLineExecution(context.Background(), protectedLineExecutionAuthorization(server.URL, "", "456")); err == nil {
			t.Fatal("empty request error = nil")
		}
		if _, err := protectedLinePublisher(server).AuthorizeProtectedLineExecution(context.Background(), protectedLineExecutionAuthorization(server.URL, "missing", "456")); err == nil {
			t.Fatal("missing request error = nil")
		}

		expired := protectedLineRecord(t, now.Add(-2*time.Hour), releaserequest.StateAwaitingExecutionApproval, "")
		expiredServer := protectedLineRequestServer(t, expired, []deploymentStatusResponse{{
			State:       "pending",
			Description: protectedLineStatusDescription(releaserequest.StateAwaitingExecutionApproval, ""),
		}}, nil, nil)
		defer expiredServer.Close()
		if _, err := protectedLinePublisher(expiredServer).AuthorizeProtectedLineExecution(context.Background(), protectedLineExecutionAuthorization(expiredServer.URL, expired.ID(), "456")); err == nil {
			t.Fatal("expired request error = nil")
		}

		failed := protectedLineRecord(t, now, releaserequest.StateFailed, "456")
		failedServer := protectedLineRequestServer(t, failed, []deploymentStatusResponse{{
			State:       "failure",
			Description: protectedLineStatusDescription(releaserequest.StateFailed, "456"),
		}}, nil, nil)
		defer failedServer.Close()
		if _, err := protectedLinePublisher(failedServer).AuthorizeProtectedLineExecution(context.Background(), protectedLineExecutionAuthorization(failedServer.URL, failed.ID(), "456")); err == nil {
			t.Fatal("failed request error = nil")
		}

		awaiting := protectedLineRecord(t, now, releaserequest.StateAwaitingExecutionApproval, "")
		mismatchServer := protectedLineRequestServer(t, awaiting, []deploymentStatusResponse{{
			State:       "pending",
			Description: protectedLineStatusDescription(releaserequest.StateAwaitingExecutionApproval, ""),
		}}, map[string]string{"release/1.2.0": strings.Repeat("b", 40)}, nil)
		defer mismatchServer.Close()
		if _, err := protectedLinePublisher(mismatchServer).AuthorizeProtectedLineExecution(context.Background(), protectedLineExecutionAuthorization(mismatchServer.URL, awaiting.ID(), "456")); err == nil {
			t.Fatal("mismatched target error = nil")
		}
	})

	t.Run("fails closed for malformed executor identity, lookup, target, and audit write failures", func(t *testing.T) {
		awaiting := protectedLineRecord(t, now, releaserequest.StateAwaitingExecutionApproval, "")
		server := protectedLineRequestServer(t, awaiting, []deploymentStatusResponse{{
			State:       "pending",
			Description: protectedLineStatusDescription(releaserequest.StateAwaitingExecutionApproval, ""),
		}}, nil, nil)
		defer server.Close()
		if _, err := protectedLinePublisher(server).AuthorizeProtectedLineExecution(context.Background(), protectedLineExecutionAuthorization(server.URL, awaiting.ID(), "not-a-run")); err == nil {
			t.Fatal("malformed executor run error = nil")
		}

		lookupServer := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/repos/acme/governance/deployments" {
				t.Fatalf("unexpected path %s", request.URL.Path)
			}
			writer.WriteHeader(http.StatusForbidden)
		}))
		defer lookupServer.Close()
		if _, err := protectedLinePublisher(lookupServer).AuthorizeProtectedLineExecution(context.Background(), protectedLineExecutionAuthorization(lookupServer.URL, awaiting.ID(), "456")); err == nil {
			t.Fatal("lookup failure error = nil")
		}

		targetFailureServer := protectedLineRequestServer(t, awaiting, []deploymentStatusResponse{{
			State:       "pending",
			Description: protectedLineStatusDescription(releaserequest.StateAwaitingExecutionApproval, ""),
		}}, nil, nil)
		defer targetFailureServer.Close()
		targetFailurePublisher := protectedLinePublisher(targetFailureServer)
		if _, err := targetFailurePublisher.AuthorizeProtectedLineExecution(context.Background(), port.ProtectedLineExecutionAuthorization{
			Repository:    port.RepositoryIdentity{Root: "C:/repository", Remote: "origin"},
			RemoteURL:     "not-a-remote",
			RequestID:     awaiting.ID(),
			ExecutorRunID: "456",
		}); err == nil {
			t.Fatal("invalid remote error = nil")
		}

		storeFailureServer := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/deployments":
				writeProtectedLineJSON(t, writer, []deploymentResponse{{
					ID:          71,
					Task:        protectedLineRequestTask,
					Environment: protectedLineRequestEnvironment,
					Payload:     mustProtectedLineJSON(t, protectedLinePayloadFromRequest(awaiting)),
				}})
			case "/repos/acme/governance/deployments/71/statuses":
				if request.Method == http.MethodPost {
					writer.WriteHeader(http.StatusForbidden)
					return
				}
				writeProtectedLineJSON(t, writer, []deploymentStatusResponse{{
					State:       "pending",
					Description: protectedLineStatusDescription(releaserequest.StateAwaitingExecutionApproval, ""),
				}})
			case "/repos/acme/governance/actions/runs/456":
				writeProtectedLineJSON(t, writer, protectedLineWorkflowRun(protectedLineExecutorWorkflow, "github-actions[bot]"))
			case "/repos/acme/governance/git/ref/heads/release/1.2.0":
				writer.WriteHeader(http.StatusNotFound)
			default:
				t.Fatalf("unexpected path %s", request.URL.Path)
			}
		}))
		defer storeFailureServer.Close()
		if _, err := protectedLinePublisher(storeFailureServer).AuthorizeProtectedLineExecution(context.Background(), protectedLineExecutionAuthorization(storeFailureServer.URL, awaiting.ID(), "456")); err == nil {
			t.Fatal("execution audit write error = nil")
		}

		targetErrorServer := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/deployments":
				writeProtectedLineJSON(t, writer, []deploymentResponse{{
					ID:          71,
					Task:        protectedLineRequestTask,
					Environment: protectedLineRequestEnvironment,
					Payload:     mustProtectedLineJSON(t, protectedLinePayloadFromRequest(awaiting)),
				}})
			case "/repos/acme/governance/deployments/71/statuses":
				writeProtectedLineJSON(t, writer, []deploymentStatusResponse{{
					State:       "pending",
					Description: protectedLineStatusDescription(releaserequest.StateAwaitingExecutionApproval, ""),
				}})
			case "/repos/acme/governance/actions/runs/456":
				writeProtectedLineJSON(t, writer, protectedLineWorkflowRun(protectedLineExecutorWorkflow, "github-actions[bot]"))
			case "/repos/acme/governance/git/ref/heads/release/1.2.0":
				writer.WriteHeader(http.StatusForbidden)
			default:
				t.Fatalf("unexpected path %s", request.URL.Path)
			}
		}))
		defer targetErrorServer.Close()
		if _, err := protectedLinePublisher(targetErrorServer).AuthorizeProtectedLineExecution(context.Background(), protectedLineExecutionAuthorization(targetErrorServer.URL, awaiting.ID(), "456")); err == nil {
			t.Fatal("target lookup error = nil")
		}
	})
}

func TestProtectedLineFinalization(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	originalNow := protectedLineRequestNow
	protectedLineRequestNow = func() time.Time { return now }
	t.Cleanup(func() { protectedLineRequestNow = originalNow })

	t.Run("marks an exact successful executor and ref as verified", func(t *testing.T) {
		record := protectedLineRecord(t, now, releaserequest.StateExecuting, "456")
		server := protectedLineRequestServer(t, record, []deploymentStatusResponse{{
			State:       "in_progress",
			Description: protectedLineStatusDescription(releaserequest.StateExecuting, "456"),
		}}, map[string]string{"release/1.2.0": strings.Repeat("a", 40)}, []workflowJobResponse{{
			Name:       "Execute bound protected-line request",
			Status:     "completed",
			Conclusion: "success",
		}})
		defer server.Close()

		result, err := protectedLinePublisher(server).FinalizeProtectedLineRequest(context.Background(), protectedLineFinalizationAuthorization(server.URL, record.ID(), "456", false))
		if err != nil || result.Request.State() != releaserequest.StateVerified {
			t.Fatalf("final result = (%#v, %v)", result, err)
		}
	})

	t.Run("marks a composite successful executor job as verified", func(t *testing.T) {
		record := protectedLineRecord(t, now, releaserequest.StateExecuting, "456")
		server := protectedLineRequestServer(t, record, []deploymentStatusResponse{{
			State:       "in_progress",
			Description: protectedLineStatusDescription(releaserequest.StateExecuting, "456"),
		}}, map[string]string{"release/1.2.0": strings.Repeat("a", 40)}, []workflowJobResponse{{
			Name:       "Release execution / Execute bound protected-line request",
			Status:     "completed",
			Conclusion: "success",
		}})
		defer server.Close()

		result, err := protectedLinePublisher(server).FinalizeProtectedLineRequest(context.Background(), protectedLineFinalizationAuthorization(server.URL, record.ID(), "456", false))
		if err != nil || result.Request.State() != releaserequest.StateVerified {
			t.Fatalf("composite final result = (%#v, %v)", result, err)
		}
	})

	t.Run("handles verified, failed, missing ref, pending, recovery, and invalid bindings", func(t *testing.T) {
		verified := protectedLineRecord(t, now, releaserequest.StateVerified, "456")
		verifiedServer := protectedLineRequestServer(t, verified, []deploymentStatusResponse{{
			State:       "success",
			Description: protectedLineStatusDescription(releaserequest.StateVerified, "456"),
		}}, nil, nil)
		defer verifiedServer.Close()
		result, err := protectedLinePublisher(verifiedServer).FinalizeProtectedLineRequest(context.Background(), protectedLineFinalizationAuthorization(verifiedServer.URL, verified.ID(), "wrong", false))
		if err != nil || result.Request.State() != releaserequest.StateVerified {
			t.Fatalf("verified finalization = (%#v, %v)", result, err)
		}

		executing := protectedLineRecord(t, now, releaserequest.StateExecuting, "456")
		failedServer := protectedLineRequestServer(t, executing, []deploymentStatusResponse{{
			State:       "in_progress",
			Description: protectedLineStatusDescription(releaserequest.StateExecuting, "456"),
		}}, nil, []workflowJobResponse{{
			Name:       "Execute bound protected-line request",
			Status:     "completed",
			Conclusion: "failure",
		}})
		defer failedServer.Close()
		result, err = protectedLinePublisher(failedServer).FinalizeProtectedLineRequest(context.Background(), protectedLineFinalizationAuthorization(failedServer.URL, executing.ID(), "456", false))
		if err != nil || result.Request.State() != releaserequest.StateFailed {
			t.Fatalf("failed finalization = (%#v, %v)", result, err)
		}

		missingServer := protectedLineRequestServer(t, executing, []deploymentStatusResponse{{
			State:       "in_progress",
			Description: protectedLineStatusDescription(releaserequest.StateExecuting, "456"),
		}}, nil, successfulExecutorJob())
		defer missingServer.Close()
		result, err = protectedLinePublisher(missingServer).FinalizeProtectedLineRequest(context.Background(), protectedLineFinalizationAuthorization(missingServer.URL, executing.ID(), "456", false))
		if err != nil || result.Request.State() != releaserequest.StateFailed {
			t.Fatalf("missing ref finalization = (%#v, %v)", result, err)
		}

		pending := protectedLineRecord(t, now, releaserequest.StateVerificationPending, "456")
		recoveryServer := protectedLineRequestServer(t, pending, []deploymentStatusResponse{{
			State:       "pending",
			Description: protectedLineStatusDescription(releaserequest.StateVerificationPending, "456"),
		}}, map[string]string{"release/1.2.0": strings.Repeat("a", 40)}, successfulExecutorJob())
		defer recoveryServer.Close()
		recoveryPublisher := protectedLinePublisher(recoveryServer)
		repository := repositoryRef{host: strings.TrimPrefix(recoveryServer.URL, "https://"), owner: "acme", name: "governance"}
		if succeeded, executionErr := recoveryPublisher.protectedLineExecutorSucceeded(context.Background(), mustAPIBase(t, recoveryServer.URL), repository, "456"); executionErr != nil || !succeeded {
			t.Fatalf("recovery executor result = (%t, %v)", succeeded, executionErr)
		}
		if sha, exists, referenceErr := recoveryPublisher.protectedLineRefIfPresent(context.Background(), mustAPIBase(t, recoveryServer.URL), repository, "release/1.2.0"); referenceErr != nil || !exists || sha != strings.Repeat("a", 40) {
			t.Fatalf("recovery ref result = (%q, %t, %v)", sha, exists, referenceErr)
		}
		result, err = recoveryPublisher.FinalizeProtectedLineRequest(context.Background(), protectedLineFinalizationAuthorization(recoveryServer.URL, pending.ID(), "", true))
		if err != nil || result.Request.State() != releaserequest.StateVerified {
			t.Fatalf("recovery finalization = (%#v, %v)", result, err)
		}

		if _, err := protectedLinePublisher(recoveryServer).FinalizeProtectedLineRequest(context.Background(), protectedLineFinalizationAuthorization(recoveryServer.URL, pending.ID(), "", false)); err == nil {
			t.Fatal("invalid normal finalization error = nil")
		}
	})

	t.Run("fails closed for empty, missing, mismatched, incomplete, and unknown finalization facts", func(t *testing.T) {
		record := protectedLineRecord(t, now, releaserequest.StateExecuting, "456")
		server := protectedLineRequestServer(t, record, []deploymentStatusResponse{{
			State:       "in_progress",
			Description: protectedLineStatusDescription(releaserequest.StateExecuting, "456"),
		}}, map[string]string{"release/1.2.0": strings.Repeat("a", 40)}, successfulExecutorJob())
		defer server.Close()
		publisher := protectedLinePublisher(server)
		if _, err := publisher.FinalizeProtectedLineRequest(context.Background(), protectedLineFinalizationAuthorization(server.URL, "", "456", false)); err == nil {
			t.Fatal("empty finalization request error = nil")
		}
		if _, err := publisher.FinalizeProtectedLineRequest(context.Background(), port.ProtectedLineFinalizationRequest{
			Repository:    port.RepositoryIdentity{Root: "C:/repository", Remote: "origin"},
			RemoteURL:     "invalid-remote",
			RequestID:     record.ID(),
			ExecutorRunID: "456",
		}); err == nil {
			t.Fatal("invalid finalization remote error = nil")
		}
		if _, err := publisher.FinalizeProtectedLineRequest(context.Background(), protectedLineFinalizationAuthorization(server.URL, record.ID(), "different", false)); err == nil {
			t.Fatal("mismatched executor run error = nil")
		}

		missingServer := protectedLineRequestServer(t, releaserequest.Request{}, nil, nil, nil)
		defer missingServer.Close()
		if _, err := protectedLinePublisher(missingServer).FinalizeProtectedLineRequest(context.Background(), protectedLineFinalizationAuthorization(missingServer.URL, "missing", "456", false)); err == nil {
			t.Fatal("missing request finalization error = nil")
		}

		lookupServer := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/repos/acme/governance/deployments" {
				t.Fatalf("unexpected path %s", request.URL.Path)
			}
			writer.WriteHeader(http.StatusForbidden)
		}))
		defer lookupServer.Close()
		if _, err := protectedLinePublisher(lookupServer).FinalizeProtectedLineRequest(context.Background(), protectedLineFinalizationAuthorization(lookupServer.URL, record.ID(), "456", false)); err == nil {
			t.Fatal("finalization lookup error = nil")
		}

		awaiting := protectedLineRecord(t, now, releaserequest.StateAwaitingExecutionApproval, "")
		awaitingServer := protectedLineRequestServer(t, awaiting, []deploymentStatusResponse{{
			State:       "pending",
			Description: protectedLineStatusDescription(releaserequest.StateAwaitingExecutionApproval, ""),
		}}, map[string]string{"release/1.2.0": strings.Repeat("a", 40)}, successfulExecutorJob())
		defer awaitingServer.Close()
		result, err := protectedLinePublisher(awaitingServer).FinalizeProtectedLineRequest(context.Background(), protectedLineFinalizationAuthorization(awaitingServer.URL, awaiting.ID(), "456", false))
		if err != nil || result.Request.State() != releaserequest.StateFailed {
			t.Fatalf("incomplete execution finalization = (%#v, %v)", result, err)
		}

		pending := protectedLineRecord(t, now, releaserequest.StateVerificationPending, "")
		pendingServer := protectedLineRequestServer(t, pending, []deploymentStatusResponse{{
			State:       "pending",
			Description: protectedLineStatusDescription(releaserequest.StateVerificationPending, ""),
		}}, nil, nil)
		defer pendingServer.Close()
		if _, err := protectedLinePublisher(pendingServer).FinalizeProtectedLineRequest(context.Background(), protectedLineFinalizationAuthorization(pendingServer.URL, pending.ID(), "", true)); err == nil {
			t.Fatal("recovery without recorded executor run error = nil")
		}

		refFailureServer := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/deployments":
				writeProtectedLineJSON(t, writer, []deploymentResponse{{
					ID:          71,
					Task:        protectedLineRequestTask,
					Environment: protectedLineRequestEnvironment,
					Payload:     mustProtectedLineJSON(t, protectedLinePayloadFromRequest(record)),
				}})
			case "/repos/acme/governance/deployments/71/statuses":
				if request.Method == http.MethodPost {
					writer.WriteHeader(http.StatusCreated)
					return
				}
				writeProtectedLineJSON(t, writer, []deploymentStatusResponse{{
					State:       "in_progress",
					Description: protectedLineStatusDescription(releaserequest.StateExecuting, "456"),
				}})
			case "/repos/acme/governance/actions/runs/456":
				writeProtectedLineJSON(t, writer, protectedLineWorkflowRun(protectedLineExecutorWorkflow, "github-actions[bot]"))
			case "/repos/acme/governance/actions/runs/456/jobs":
				writeProtectedLineJSON(t, writer, workflowJobsResponse{Jobs: successfulExecutorJob()})
			case "/repos/acme/governance/git/ref/heads/release/1.2.0":
				writer.WriteHeader(http.StatusForbidden)
			default:
				t.Fatalf("unexpected path %s", request.URL.Path)
			}
		}))
		defer refFailureServer.Close()
		result, err = protectedLinePublisher(refFailureServer).FinalizeProtectedLineRequest(context.Background(), protectedLineFinalizationAuthorization(refFailureServer.URL, record.ID(), "456", false))
		if err == nil || result.Request.State() != releaserequest.StateVerificationPending {
			t.Fatalf("unknown ref finalization = (%#v, %v)", result, err)
		}

		provenanceFailureServer := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/deployments":
				writeProtectedLineJSON(t, writer, []deploymentResponse{{
					ID:          71,
					Task:        protectedLineRequestTask,
					Environment: protectedLineRequestEnvironment,
					Payload:     mustProtectedLineJSON(t, protectedLinePayloadFromRequest(record)),
				}})
			case "/repos/acme/governance/deployments/71/statuses":
				if request.Method == http.MethodPost {
					writer.WriteHeader(http.StatusCreated)
					return
				}
				writeProtectedLineJSON(t, writer, []deploymentStatusResponse{{
					State:       "in_progress",
					Description: protectedLineStatusDescription(releaserequest.StateExecuting, "456"),
				}})
			case "/repos/acme/governance/actions/runs/456":
				writeProtectedLineJSON(t, writer, protectedLineWorkflowRun("other.yml", "github-actions[bot]"))
			default:
				t.Fatalf("unexpected path %s", request.URL.Path)
			}
		}))
		defer provenanceFailureServer.Close()
		result, err = protectedLinePublisher(provenanceFailureServer).FinalizeProtectedLineRequest(context.Background(), protectedLineFinalizationAuthorization(provenanceFailureServer.URL, record.ID(), "456", false))
		if err == nil || result.Request.State() != releaserequest.StateVerificationPending {
			t.Fatalf("unknown workflow finalization = (%#v, %v)", result, err)
		}
	})
}

func TestProtectedLineRequestHelpers(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	record := protectedLineRecord(t, now, releaserequest.StateRequested, "")
	payload := protectedLinePayloadFromRequest(record)

	parsed, err := parseProtectedLinePayload(mustProtectedLineJSON(t, payload))
	if err != nil || parsed != payload {
		t.Fatalf("parse payload = (%#v, %v)", parsed, err)
	}
	for _, raw := range []string{
		`{"schemaVersion":1,"unknown":true}`,
		`{"schemaVersion":1} {}`,
		`{"schemaVersion":2}`,
	} {
		if _, err := parseProtectedLinePayload(json.RawMessage(raw)); err == nil {
			t.Fatalf("parse payload %q error = nil", raw)
		}
	}

	withID := record.WithDeploymentID(71)
	if withID.DeploymentID() != 71 {
		t.Fatalf("with deployment ID = %#v", withID)
	}
	authorization := protectedLineAuthorization(t, "https://api.example")
	if protectedLineIdempotencyKey(repositoryRef{owner: "acme", name: "governance"}, authorization, strings.Repeat("a", 40)) ==
		protectedLineIdempotencyKey(repositoryRef{owner: "acme", name: "governance"}, authorization, strings.Repeat("b", 40)) {
		t.Fatal("idempotency keys match across source SHAs")
	}
	authorization.ParentRunID = "456"
	if protectedLineIdempotencyKey(repositoryRef{owner: "acme", name: "governance"}, protectedLineAuthorization(t, "https://api.example"), strings.Repeat("a", 40)) ==
		protectedLineIdempotencyKey(repositoryRef{owner: "acme", name: "governance"}, authorization, strings.Repeat("a", 40)) {
		t.Fatal("idempotency keys match across request-controller runs")
	}

	for _, state := range []releaserequest.State{
		releaserequest.StateRequested,
		releaserequest.StateRequestAuthorized,
		releaserequest.StateAwaitingExecutionApproval,
		releaserequest.StateExecuting,
		releaserequest.StateVerified,
		releaserequest.StateFailed,
		releaserequest.StateRejected,
		releaserequest.StateExpired,
		releaserequest.StateVerificationPending,
	} {
		if _, err := deploymentState(state); err != nil {
			t.Fatalf("deploymentState(%q) = %v", state, err)
		}
		description := protectedLineStatusDescription(state, "456")
		parsedState, runID, ok := parseProtectedLineStatusDescription(description)
		if !ok || parsedState != state || runID != "456" {
			t.Fatalf("parse description %q = (%q, %q, %t)", description, parsedState, runID, ok)
		}
	}
	for _, description := range []string{"", "other", "git-governance protected-line request state=", "git-governance protected-line request state=bad", "git-governance protected-line request state=verified executor_run=0", "git-governance protected-line request state=verified extra invalid"} {
		if _, _, ok := parseProtectedLineStatusDescription(description); ok {
			t.Fatalf("description %q unexpectedly parsed", description)
		}
	}
	if _, err := deploymentState("bad"); err == nil {
		t.Fatal("invalid deployment state error = nil")
	}
	if protectedLineRunURL(repositoryRef{owner: "acme", name: "governance"}, "456") == "" ||
		protectedLineRunURL(repositoryRef{owner: "acme", name: "governance"}, "0") != "" {
		t.Fatal("run URL validation mismatch")
	}
	if !validProtectedLineOperation(releaserequest.OperationRelease) ||
		validProtectedLineOperation("bad") ||
		!validSingleLineValue("actor", 5) ||
		validSingleLineValue(" actor", 5) ||
		!validRunID("123") ||
		validRunID("0") ||
		!isFullCommitID(strings.Repeat("a", 40)) ||
		isFullCommitID(strings.Repeat("A", 40)) {
		t.Fatal("helper validation mismatch")
	}
}

func TestWorkflowTokenResolver(t *testing.T) {
	resolver := NewWorkflowTokenResolver("token")
	token, err := resolver.Resolve(context.Background(), CredentialTarget{})
	if err != nil || token != "token" {
		t.Fatalf("Resolve() = (%q, %v)", token, err)
	}
	if _, err := NewWorkflowTokenResolver("").Resolve(context.Background(), CredentialTarget{}); err == nil {
		t.Fatal("empty token error = nil")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := resolver.Resolve(ctx, CredentialTarget{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Resolve() error = %v", err)
	}
}

func TestProtectedLineRequestAdapterFailsClosedForMalformedProviderData(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	record := protectedLineRecord(t, now, releaserequest.StateExecuting, "456")
	payload := protectedLinePayloadFromRequest(record)

	t.Run("validates every authorization field", func(t *testing.T) {
		valid := protectedLineAuthorization(t, "https://example.invalid")
		if err := validateProtectedLineAuthorization(valid); err != nil {
			t.Fatal(err)
		}
		cases := []func(*port.ProtectedLineRequestAuthorization){
			func(value *port.ProtectedLineRequestAuthorization) { value.Ticket = ticket.ID{} },
			func(value *port.ProtectedLineRequestAuthorization) { value.Operation = "bad" },
			func(value *port.ProtectedLineRequestAuthorization) { value.Source = branch.BranchName{} },
			func(value *port.ProtectedLineRequestAuthorization) { value.Target = branch.BranchName{} },
			func(value *port.ProtectedLineRequestAuthorization) { value.Version = "" },
			func(value *port.ProtectedLineRequestAuthorization) { value.Requester = "bad\nactor" },
			func(value *port.ProtectedLineRequestAuthorization) { value.ParentRunID = "0" },
		}
		for _, mutate := range cases {
			value := valid
			mutate(&value)
			if err := validateProtectedLineAuthorization(value); err == nil {
				t.Fatal("invalid authorization error = nil")
			}
		}
	})

	t.Run("rejects malformed durable payload fields", func(t *testing.T) {
		cases := []func(*protectedLinePayload){
			func(value *protectedLinePayload) { value.Ticket = "bad" },
			func(value *protectedLinePayload) { value.TargetRef = "bad" },
			func(value *protectedLinePayload) { value.SourceRef = "bad" },
			func(value *protectedLinePayload) { value.ExpiresAt = "invalid" },
			func(value *protectedLinePayload) { value.RequestID = "bad id" },
		}
		for _, mutate := range cases {
			value := payload
			mutate(&value)
			if _, err := protectedLineRequestFromPayload(value, 71, releaserequest.StateExecuting, "456"); err == nil {
				t.Fatal("malformed payload error = nil")
			}
		}
	})

	t.Run("rejects malformed deployment, reference, executor, and status data", func(t *testing.T) {
		publisher := protectedLinePublisher(nil)
		apiBase, err := parseAPIBaseURL("https://api.example")
		if err != nil {
			t.Fatal(err)
		}
		repository := repositoryRef{host: "api.example", owner: "acme", name: "governance"}
		if _, err := publisher.protectedLineRequestFromDeployment(context.Background(), apiBase, repository, deploymentResponse{}); err == nil {
			t.Fatal("invalid deployment error = nil")
		}
		if _, err := publisher.protectedLineRequestFromDeployment(context.Background(), apiBase, repository, deploymentResponse{
			ID:          71,
			Task:        protectedLineRequestTask,
			Environment: protectedLineRequestEnvironment,
			Payload:     json.RawMessage("invalid"),
		}); err == nil {
			t.Fatal("invalid deployment payload error = nil")
		}
	})
}

func TestProtectedLineRequestProviderHTTPFailurePaths(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	originalNow := protectedLineRequestNow
	protectedLineRequestNow = func() time.Time { return now }
	t.Cleanup(func() { protectedLineRequestNow = originalNow })

	t.Run("classifies request creation, dispatch, and state write failures", func(t *testing.T) {
		for _, failure := range []struct {
			name string
			path string
			code int
		}{
			{name: "deployment", path: "/repos/acme/governance/deployments", code: http.StatusInternalServerError},
			{name: "request state", path: "/repos/acme/governance/deployments/71/statuses", code: http.StatusInternalServerError},
			{name: "executor dispatch", path: "/repos/acme/governance/actions/workflows/execute-protected-line-request.yml/dispatches", code: http.StatusForbidden},
		} {
			t.Run(failure.name, func(t *testing.T) {
				server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					switch request.URL.Path {
					case "/repos/acme/governance/git/ref/heads/develop":
						writeProtectedLineJSON(t, writer, gitReferenceResponse{Object: gitObjectReference{SHA: strings.Repeat("a", 40), Type: "commit"}})
					case "/repos/acme/governance/deployments":
						if request.Method == http.MethodGet {
							writeProtectedLineJSON(t, writer, []deploymentResponse{})
							return
						}
						if failure.path == request.URL.Path {
							writer.WriteHeader(failure.code)
							return
						}
						writer.WriteHeader(http.StatusCreated)
						writeProtectedLineJSON(t, writer, deploymentResponse{ID: 71})
					case "/repos/acme/governance/git/ref/heads/release/1.2.0":
						writer.WriteHeader(http.StatusNotFound)
					case "/repos/acme/governance/actions/runs/123":
						writeProtectedLineJSON(t, writer, protectedLineWorkflowRun(protectedLineRequestWorkflow, "release-requester"))
					case "/repos/acme/governance/deployments/71/statuses", "/repos/acme/governance/actions/workflows/execute-protected-line-request.yml/dispatches":
						if failure.path == request.URL.Path {
							writer.WriteHeader(failure.code)
							return
						}
						if strings.HasSuffix(request.URL.Path, "/statuses") {
							writer.WriteHeader(http.StatusCreated)
							return
						}
						writer.WriteHeader(http.StatusNoContent)
					default:
						t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
					}
				}))
				defer server.Close()
				if _, err := protectedLinePublisher(server).AuthorizeProtectedLineRequest(context.Background(), protectedLineAuthorization(t, server.URL)); err == nil {
					t.Fatal("request failure error = nil")
				}
			})
		}
	})

	t.Run("fails closed before persistence when source, record lookup, target, or request identity is invalid", func(t *testing.T) {
		originalID := releaseRequestIDGenerator
		t.Cleanup(func() { releaseRequestIDGenerator = originalID })
		testCases := []struct {
			name    string
			handler func(http.ResponseWriter, *http.Request)
			prepare func()
		}{
			{
				name: "source ref error",
				handler: func(writer http.ResponseWriter, request *http.Request) {
					if request.URL.Path != "/repos/acme/governance/git/ref/heads/develop" {
						t.Fatalf("unexpected path %s", request.URL.Path)
					}
					writer.WriteHeader(http.StatusForbidden)
				},
			},
			{
				name: "request lookup error",
				handler: func(writer http.ResponseWriter, request *http.Request) {
					switch request.URL.Path {
					case "/repos/acme/governance/git/ref/heads/develop":
						writeProtectedLineJSON(t, writer, gitReferenceResponse{Object: gitObjectReference{SHA: strings.Repeat("a", 40), Type: "commit"}})
					case "/repos/acme/governance/deployments":
						writer.WriteHeader(http.StatusForbidden)
					default:
						t.Fatalf("unexpected path %s", request.URL.Path)
					}
				},
			},
			{
				name: "target lookup error",
				handler: func(writer http.ResponseWriter, request *http.Request) {
					switch request.URL.Path {
					case "/repos/acme/governance/git/ref/heads/develop":
						writeProtectedLineJSON(t, writer, gitReferenceResponse{Object: gitObjectReference{SHA: strings.Repeat("a", 40), Type: "commit"}})
					case "/repos/acme/governance/deployments":
						writeProtectedLineJSON(t, writer, []deploymentResponse{})
					case "/repos/acme/governance/git/ref/heads/release/1.2.0":
						writer.WriteHeader(http.StatusForbidden)
					default:
						t.Fatalf("unexpected path %s", request.URL.Path)
					}
				},
			},
			{
				name: "request id generation error",
				prepare: func() {
					releaseRequestIDGenerator = func() (string, error) { return "", errors.New("random unavailable") }
				},
				handler: func(writer http.ResponseWriter, request *http.Request) {
					switch request.URL.Path {
					case "/repos/acme/governance/git/ref/heads/develop":
						writeProtectedLineJSON(t, writer, gitReferenceResponse{Object: gitObjectReference{SHA: strings.Repeat("a", 40), Type: "commit"}})
					case "/repos/acme/governance/deployments":
						writeProtectedLineJSON(t, writer, []deploymentResponse{})
					case "/repos/acme/governance/git/ref/heads/release/1.2.0":
						writer.WriteHeader(http.StatusNotFound)
					default:
						t.Fatalf("unexpected path %s", request.URL.Path)
					}
				},
			},
		}
		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				releaseRequestIDGenerator = originalID
				if testCase.prepare != nil {
					testCase.prepare()
				}
				server := httptest.NewTLSServer(http.HandlerFunc(testCase.handler))
				defer server.Close()
				if _, err := protectedLinePublisher(server).AuthorizeProtectedLineRequest(context.Background(), protectedLineAuthorization(t, server.URL)); err == nil {
					t.Fatal("authorization error = nil")
				}
			})
		}
	})

	t.Run("fails closed if final awaiting state cannot be persisted", func(t *testing.T) {
		statusWrites := 0
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/git/ref/heads/develop":
				writeProtectedLineJSON(t, writer, gitReferenceResponse{Object: gitObjectReference{SHA: strings.Repeat("a", 40), Type: "commit"}})
			case "/repos/acme/governance/deployments":
				if request.Method == http.MethodGet {
					writeProtectedLineJSON(t, writer, []deploymentResponse{})
					return
				}
				writer.WriteHeader(http.StatusCreated)
				writeProtectedLineJSON(t, writer, deploymentResponse{ID: 71})
			case "/repos/acme/governance/git/ref/heads/release/1.2.0":
				writer.WriteHeader(http.StatusNotFound)
			case "/repos/acme/governance/actions/runs/123":
				writeProtectedLineJSON(t, writer, protectedLineWorkflowRun(protectedLineRequestWorkflow, "release-requester"))
			case "/repos/acme/governance/deployments/71/statuses":
				statusWrites++
				if statusWrites == 2 {
					writer.WriteHeader(http.StatusForbidden)
					return
				}
				writer.WriteHeader(http.StatusCreated)
			case "/repos/acme/governance/actions/workflows/execute-protected-line-request.yml/dispatches":
				writer.WriteHeader(http.StatusNoContent)
			default:
				t.Fatalf("unexpected path %s", request.URL.Path)
			}
		}))
		defer server.Close()
		if _, err := protectedLinePublisher(server).AuthorizeProtectedLineRequest(context.Background(), protectedLineAuthorization(t, server.URL)); err == nil {
			t.Fatal("awaiting state persistence error = nil")
		}
	})

	t.Run("requires the declared request-controller workflow provenance", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/git/ref/heads/develop":
				writeProtectedLineJSON(t, writer, gitReferenceResponse{Object: gitObjectReference{SHA: strings.Repeat("a", 40), Type: "commit"}})
			case "/repos/acme/governance/deployments":
				writeProtectedLineJSON(t, writer, []deploymentResponse{})
			case "/repos/acme/governance/git/ref/heads/release/1.2.0":
				writer.WriteHeader(http.StatusNotFound)
			case "/repos/acme/governance/actions/runs/123":
				writeProtectedLineJSON(t, writer, protectedLineWorkflowRun(protectedLineRequestWorkflow, "other-requester"))
			default:
				t.Fatalf("unexpected path %s", request.URL.Path)
			}
		}))
		defer server.Close()
		if _, err := protectedLinePublisher(server).AuthorizeProtectedLineRequest(context.Background(), protectedLineAuthorization(t, server.URL)); err == nil {
			t.Fatal("request controller provenance error = nil")
		}
	})

	t.Run("rejects invalid HTTP responses and payloads from record lookups", func(t *testing.T) {
		tests := []struct {
			name     string
			response func(http.ResponseWriter)
		}{
			{name: "list status", response: func(writer http.ResponseWriter) { writer.WriteHeader(http.StatusForbidden) }},
			{name: "list payload", response: func(writer http.ResponseWriter) { _, _ = writer.Write([]byte("{")) }},
		}
		for _, testCase := range tests {
			t.Run(testCase.name, func(t *testing.T) {
				server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					if request.URL.Path != "/repos/acme/governance/deployments" {
						t.Fatalf("unexpected request %s", request.URL.Path)
					}
					testCase.response(writer)
				}))
				defer server.Close()
				publisher := protectedLinePublisher(server)
				apiBase := mustAPIBase(t, server.URL)
				_, _, err := publisher.findProtectedLineRequest(context.Background(), apiBase, repositoryRef{host: strings.TrimPrefix(server.URL, "https://"), owner: "acme", name: "governance"}, "missing", "")
				if err == nil {
					t.Fatal("lookup error = nil")
				}
			})
		}
	})

	t.Run("marks verification pending only when provider evidence is unavailable", func(t *testing.T) {
		record := protectedLineRecord(t, now, releaserequest.StateExecuting, "456")
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/deployments":
				writeProtectedLineJSON(t, writer, []deploymentResponse{{
					ID:          71,
					Task:        protectedLineRequestTask,
					Environment: protectedLineRequestEnvironment,
					Payload:     mustProtectedLineJSON(t, protectedLinePayloadFromRequest(record)),
				}})
			case "/repos/acme/governance/deployments/71/statuses":
				if request.Method == http.MethodPost {
					writer.WriteHeader(http.StatusCreated)
					return
				}
				writeProtectedLineJSON(t, writer, []deploymentStatusResponse{{
					State:       "in_progress",
					Description: protectedLineStatusDescription(releaserequest.StateExecuting, "456"),
				}})
			case "/repos/acme/governance/actions/runs/456":
				writeProtectedLineJSON(t, writer, protectedLineWorkflowRun(protectedLineExecutorWorkflow, "github-actions[bot]"))
			case "/repos/acme/governance/actions/runs/456/jobs":
				writer.WriteHeader(http.StatusInternalServerError)
			default:
				t.Fatalf("unexpected request %s", request.URL.Path)
			}
		}))
		defer server.Close()
		result, err := protectedLinePublisher(server).FinalizeProtectedLineRequest(context.Background(), protectedLineFinalizationAuthorization(server.URL, record.ID(), "456", false))
		if err == nil || result.Request.State() != releaserequest.StateVerificationPending {
			t.Fatalf("verification pending = (%#v, %v)", result, err)
		}
	})
}

func TestProtectedLineRequestLowLevelProviderBoundaries(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	record := protectedLineRecord(t, now, releaserequest.StateAwaitingExecutionApproval, "")

	t.Run("rejects deployment create responses without a durable ID", func(t *testing.T) {
		for _, response := range []struct {
			name string
			body string
			code int
		}{
			{name: "status", code: http.StatusForbidden},
			{name: "invalid json", code: http.StatusCreated, body: "{"},
			{name: "zero id", code: http.StatusCreated, body: `{}`},
		} {
			t.Run(response.name, func(t *testing.T) {
				server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					if request.URL.Path != "/repos/acme/governance/deployments" {
						t.Fatalf("unexpected path %s", request.URL.Path)
					}
					writer.WriteHeader(response.code)
					if response.body != "" {
						_, _ = writer.Write([]byte(response.body))
					}
				}))
				defer server.Close()
				publisher := protectedLinePublisher(server)
				_, err := publisher.createProtectedLineDeployment(context.Background(), mustAPIBase(t, server.URL), repositoryRef{host: strings.TrimPrefix(server.URL, "https://"), owner: "acme", name: "governance"}, record)
				if err == nil {
					t.Fatal("deployment create error = nil")
				}
			})
		}
	})

	t.Run("rejects executor dispatch and audit writes outside their exact success response", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/actions/workflows/execute-protected-line-request.yml/dispatches":
				writer.WriteHeader(http.StatusForbidden)
			case "/repos/acme/governance/deployments/71/statuses":
				writer.WriteHeader(http.StatusForbidden)
			default:
				t.Fatalf("unexpected path %s", request.URL.Path)
			}
		}))
		defer server.Close()
		publisher := protectedLinePublisher(server)
		apiBase := mustAPIBase(t, server.URL)
		repository := repositoryRef{host: strings.TrimPrefix(server.URL, "https://"), owner: "acme", name: "governance"}
		if err := publisher.dispatchProtectedLineExecutor(context.Background(), apiBase, repository, record); err == nil {
			t.Fatal("dispatch error = nil")
		}
		if err := publisher.storeProtectedLineState(context.Background(), apiBase, repository, record, ""); err == nil {
			t.Fatal("status write error = nil")
		}
	})

	t.Run("rejects malformed references and executor job responses", func(t *testing.T) {
		for _, testCase := range []struct {
			name string
			path string
			body string
			code int
		}{
			{name: "reference status", path: "/repos/acme/governance/git/ref/heads/develop", code: http.StatusForbidden},
			{name: "reference object", path: "/repos/acme/governance/git/ref/heads/develop", body: `{"object":{"sha":"bad","type":"tag"}}`, code: http.StatusOK},
			{name: "reference json", path: "/repos/acme/governance/git/ref/heads/develop", body: "{", code: http.StatusOK},
			{name: "job status", path: "/repos/acme/governance/actions/runs/456/jobs", code: http.StatusForbidden},
			{name: "job json", path: "/repos/acme/governance/actions/runs/456/jobs", body: "{", code: http.StatusOK},
			{name: "missing executor job", path: "/repos/acme/governance/actions/runs/456/jobs", body: `{"jobs":[]}`, code: http.StatusOK},
		} {
			t.Run(testCase.name, func(t *testing.T) {
				server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					if request.URL.Path != testCase.path {
						t.Fatalf("unexpected path %s", request.URL.Path)
					}
					writer.WriteHeader(testCase.code)
					if testCase.body != "" {
						_, _ = writer.Write([]byte(testCase.body))
					}
				}))
				defer server.Close()
				publisher := protectedLinePublisher(server)
				apiBase := mustAPIBase(t, server.URL)
				repository := repositoryRef{host: strings.TrimPrefix(server.URL, "https://"), owner: "acme", name: "governance"}
				if strings.Contains(testCase.path, "/git/ref/") {
					if _, _, err := publisher.protectedLineRefIfPresent(context.Background(), apiBase, repository, "develop"); err == nil {
						t.Fatal("reference error = nil")
					}
					return
				}
				if _, err := publisher.protectedLineExecutorSucceeded(context.Background(), apiBase, repository, "456"); err == nil {
					t.Fatal("executor job error = nil")
				}
			})
		}
		if _, err := protectedLinePublisher(nil).protectedLineExecutorSucceeded(context.Background(), mustAPIBase(t, "https://api.example"), repositoryRef{host: "api.example", owner: "acme", name: "governance"}, "0"); err == nil {
			t.Fatal("invalid executor run error = nil")
		}
	})

	t.Run("interprets empty and malformed deployment status history fail closed", func(t *testing.T) {
		for _, testCase := range []struct {
			name   string
			status int
			body   string
			want   releaserequest.State
			ok     bool
		}{
			{name: "empty", status: http.StatusOK, body: "[]", want: releaserequest.StateRequested, ok: true},
			{name: "invalid description", status: http.StatusOK, body: `[{"state":"pending","description":"unknown"}]`},
			{name: "invalid json", status: http.StatusOK, body: "{"},
			{name: "status", status: http.StatusForbidden},
		} {
			t.Run(testCase.name, func(t *testing.T) {
				server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					if request.URL.Path != "/repos/acme/governance/deployments/71/statuses" {
						t.Fatalf("unexpected path %s", request.URL.Path)
					}
					writer.WriteHeader(testCase.status)
					if testCase.body != "" {
						_, _ = writer.Write([]byte(testCase.body))
					}
				}))
				defer server.Close()
				state, _, err := protectedLinePublisher(server).protectedLineDeploymentState(
					context.Background(),
					mustAPIBase(t, server.URL),
					repositoryRef{host: strings.TrimPrefix(server.URL, "https://"), owner: "acme", name: "governance"},
					71,
				)
				if testCase.ok {
					if err != nil || state != testCase.want {
						t.Fatalf("empty status = (%q, %v)", state, err)
					}
				} else if err == nil {
					t.Fatal("status history error = nil")
				}
			})
		}
	})

	if validRunID("") || isFullCommitID("short") {
		t.Fatal("empty run or short commit accepted")
	}
	if validRunID("12a") {
		t.Fatal("non-numeric run accepted")
	}
}

func TestProtectedLineRequestInternalFailureSeams(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	originalNew := protectedLineRequestNew
	originalTransition := protectedLineRequestTransition
	originalExecutionBind := protectedLineExecutionBind
	originalNow := protectedLineRequestNow
	protectedLineRequestNow = func() time.Time { return now }
	t.Cleanup(func() {
		protectedLineRequestNew = originalNew
		protectedLineRequestTransition = originalTransition
		protectedLineExecutionBind = originalExecutionBind
		protectedLineRequestNow = originalNow
	})

	newServer := func(t *testing.T, dispatchStatus int) *httptest.Server {
		t.Helper()
		return httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/git/ref/heads/develop":
				writeProtectedLineJSON(t, writer, gitReferenceResponse{Object: gitObjectReference{SHA: strings.Repeat("a", 40), Type: "commit"}})
			case "/repos/acme/governance/deployments":
				if request.Method == http.MethodGet {
					writeProtectedLineJSON(t, writer, []deploymentResponse{})
					return
				}
				writer.WriteHeader(http.StatusCreated)
				writeProtectedLineJSON(t, writer, deploymentResponse{ID: 71})
			case "/repos/acme/governance/git/ref/heads/release/1.2.0":
				writer.WriteHeader(http.StatusNotFound)
			case "/repos/acme/governance/actions/runs/123":
				writeProtectedLineJSON(t, writer, protectedLineWorkflowRun(protectedLineRequestWorkflow, "release-requester"))
			case "/repos/acme/governance/deployments/71/statuses":
				writer.WriteHeader(http.StatusCreated)
			case "/repos/acme/governance/actions/workflows/execute-protected-line-request.yml/dispatches":
				writer.WriteHeader(dispatchStatus)
			default:
				t.Fatalf("unexpected request %s", request.URL.Path)
			}
		}))
	}

	t.Run("propagates request construction and transition failures", func(t *testing.T) {
		server := newServer(t, http.StatusNoContent)
		defer server.Close()
		protectedLineRequestNew = func(releaserequest.Input, time.Time) (releaserequest.Request, error) {
			return releaserequest.Request{}, errors.New("record construction")
		}
		if _, err := protectedLinePublisher(server).AuthorizeProtectedLineRequest(context.Background(), protectedLineAuthorization(t, server.URL)); err == nil {
			t.Fatal("record construction error = nil")
		}

		protectedLineRequestNew = originalNew
		protectedLineRequestTransition = func(releaserequest.Request, releaserequest.State, time.Time) (releaserequest.Request, error) {
			return releaserequest.Request{}, errors.New("transition")
		}
		if _, err := protectedLinePublisher(server).AuthorizeProtectedLineRequest(context.Background(), protectedLineAuthorization(t, server.URL)); err == nil {
			t.Fatal("authorization transition error = nil")
		}
	})

	t.Run("records dispatch failure even when its terminal transition cannot be written", func(t *testing.T) {
		server := newServer(t, http.StatusForbidden)
		defer server.Close()
		protectedLineRequestNew = originalNew
		calls := 0
		protectedLineRequestTransition = func(request releaserequest.Request, state releaserequest.State, at time.Time) (releaserequest.Request, error) {
			calls++
			if calls == 2 {
				return releaserequest.Request{}, errors.New("failed transition")
			}
			return request.Transition(state, at)
		}
		if _, err := protectedLinePublisher(server).AuthorizeProtectedLineRequest(context.Background(), protectedLineAuthorization(t, server.URL)); err == nil {
			t.Fatal("dispatch error = nil")
		}
	})

	t.Run("fails before execution dispatch when awaiting transition cannot be written", func(t *testing.T) {
		server := newServer(t, http.StatusNoContent)
		defer server.Close()
		calls := 0
		protectedLineRequestTransition = func(request releaserequest.Request, state releaserequest.State, at time.Time) (releaserequest.Request, error) {
			calls++
			if calls == 2 {
				return releaserequest.Request{}, errors.New("awaiting transition")
			}
			return request.Transition(state, at)
		}
		if _, err := protectedLinePublisher(server).AuthorizeProtectedLineRequest(context.Background(), protectedLineAuthorization(t, server.URL)); err == nil {
			t.Fatal("awaiting transition error = nil")
		}
		protectedLineRequestTransition = originalTransition
	})

	t.Run("propagates internal finalizer transition failures", func(t *testing.T) {
		record := protectedLineRecord(t, now, releaserequest.StateExecuting, "456")
		server := protectedLineRequestServer(t, record, []deploymentStatusResponse{{
			State:       "in_progress",
			Description: protectedLineStatusDescription(releaserequest.StateExecuting, "456"),
		}}, map[string]string{"release/1.2.0": strings.Repeat("a", 40)}, successfulExecutorJob())
		defer server.Close()
		protectedLineRequestTransition = func(releaserequest.Request, releaserequest.State, time.Time) (releaserequest.Request, error) {
			return releaserequest.Request{}, errors.New("finalizer transition")
		}
		if _, err := protectedLinePublisher(server).FinalizeProtectedLineRequest(context.Background(), protectedLineFinalizationAuthorization(server.URL, record.ID(), "456", false)); err == nil {
			t.Fatal("finalizer transition error = nil")
		}
	})

	t.Run("fails closed when verification-pending transition cannot be written", func(t *testing.T) {
		record := protectedLineRecord(t, now, releaserequest.StateExecuting, "456")
		server := protectedLineRequestServer(t, record, []deploymentStatusResponse{{
			State:       "in_progress",
			Description: protectedLineStatusDescription(releaserequest.StateExecuting, "456"),
		}}, nil, nil)
		defer server.Close()
		protectedLineRequestTransition = func(releaserequest.Request, releaserequest.State, time.Time) (releaserequest.Request, error) {
			return releaserequest.Request{}, errors.New("pending transition")
		}
		if _, err := protectedLinePublisher(server).FinalizeProtectedLineRequest(context.Background(), protectedLineFinalizationAuthorization(server.URL, record.ID(), "456", false)); err == nil {
			t.Fatal("pending transition error = nil")
		}
	})

	t.Run("fails before target inspection when executor binding cannot be established", func(t *testing.T) {
		record := protectedLineRecord(t, now, releaserequest.StateAwaitingExecutionApproval, "")
		server := protectedLineRequestServer(t, record, []deploymentStatusResponse{{
			State:       "pending",
			Description: protectedLineStatusDescription(releaserequest.StateAwaitingExecutionApproval, ""),
		}}, nil, nil)
		defer server.Close()
		protectedLineExecutionBind = func(releaserequest.Request, string, time.Time) (releaserequest.Request, error) {
			return releaserequest.Request{}, errors.New("execution bind")
		}
		if _, err := protectedLinePublisher(server).AuthorizeProtectedLineExecution(context.Background(), protectedLineExecutionAuthorization(server.URL, record.ID(), "456")); err == nil {
			t.Fatal("execution bind error = nil")
		}
	})
}

func TestProtectedLineRequestProviderPropagatesCredentialAndLookupFailures(t *testing.T) {
	record := protectedLineRecord(t, time.Now(), releaserequest.StateAwaitingExecutionApproval, "")
	apiBase := mustAPIBase(t, "https://api.example")
	repository := repositoryRef{host: "api.example", owner: "acme", name: "governance"}
	credentialErr := errors.New("credential unavailable")
	publisher := New(Options{
		Resolver:   &fakeCredentialResolver{err: credentialErr},
		APIBaseURL: "https://api.example",
	})

	if _, err := publisher.createProtectedLineDeployment(context.Background(), apiBase, repository, record); !errors.Is(err, credentialErr) {
		t.Fatalf("deployment credential error = %v", err)
	}
	if err := publisher.dispatchProtectedLineExecutor(context.Background(), apiBase, repository, record); !errors.Is(err, credentialErr) {
		t.Fatalf("dispatch credential error = %v", err)
	}
	if err := publisher.storeProtectedLineState(context.Background(), apiBase, repository, record, ""); !errors.Is(err, credentialErr) {
		t.Fatalf("state credential error = %v", err)
	}
	if _, _, err := publisher.findProtectedLineRequest(context.Background(), apiBase, repository, "key", ""); !errors.Is(err, credentialErr) {
		t.Fatalf("lookup credential error = %v", err)
	}
	if _, _, err := publisher.protectedLineDeploymentState(context.Background(), apiBase, repository, 71); !errors.Is(err, credentialErr) {
		t.Fatalf("status credential error = %v", err)
	}
	if _, _, err := publisher.protectedLineRefIfPresent(context.Background(), apiBase, repository, "develop"); !errors.Is(err, credentialErr) {
		t.Fatalf("ref credential error = %v", err)
	}
	if _, err := publisher.protectedLineExecutorSucceeded(context.Background(), apiBase, repository, "456"); !errors.Is(err, credentialErr) {
		t.Fatalf("executor credential error = %v", err)
	}
	if err := publisher.storeProtectedLineState(context.Background(), apiBase, repository, releaserequest.Request{}, ""); err == nil {
		t.Fatal("zero-value request state write error = nil")
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/repos/acme/governance/deployments":
			writeProtectedLineJSON(t, writer, []deploymentResponse{{
				ID:          71,
				Task:        protectedLineRequestTask,
				Environment: protectedLineRequestEnvironment,
				Payload:     mustProtectedLineJSON(t, protectedLinePayloadFromRequest(record)),
			}})
		case "/repos/acme/governance/deployments/71/statuses":
			writer.WriteHeader(http.StatusForbidden)
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	_, _, err := protectedLinePublisher(server).findProtectedLineRequest(
		context.Background(),
		mustAPIBase(t, server.URL),
		repositoryRef{host: strings.TrimPrefix(server.URL, "https://"), owner: "acme", name: "governance"},
		"missing",
		"",
	)
	if err == nil {
		t.Fatal("invalid nested record state error = nil")
	}

	t.Run("bounds request-history pagination", func(t *testing.T) {
		deployments := make([]deploymentResponse, protectedLineDeploymentPageSize)
		for index := range deployments {
			deployments[index] = deploymentResponse{
				ID:          71,
				Task:        protectedLineRequestTask,
				Environment: protectedLineRequestEnvironment,
				Payload:     mustProtectedLineJSON(t, protectedLinePayloadFromRequest(record)),
			}
		}
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/repos/acme/governance/deployments":
				writeProtectedLineJSON(t, writer, deployments)
			case "/repos/acme/governance/deployments/71/statuses":
				writeProtectedLineJSON(t, writer, []deploymentStatusResponse{{
					State:       "pending",
					Description: protectedLineStatusDescription(releaserequest.StateAwaitingExecutionApproval, ""),
				}})
			default:
				t.Fatalf("unexpected path %s", request.URL.Path)
			}
		}))
		defer server.Close()
		_, _, err := protectedLinePublisher(server).findProtectedLineRequest(
			context.Background(),
			mustAPIBase(t, server.URL),
			repositoryRef{host: strings.TrimPrefix(server.URL, "https://"), owner: "acme", name: "governance"},
			"missing",
			"missing",
		)
		if err == nil {
			t.Fatal("unbounded history error = nil")
		}
	})
}

func TestProtectedLineWorkflowRunProvenanceValidation(t *testing.T) {
	repository := repositoryRef{host: "api.example", owner: "acme", name: "governance"}
	apiBase := mustAPIBase(t, "https://api.example")
	if err := protectedLinePublisher(nil).validateProtectedLineWorkflowRun(context.Background(), apiBase, repository, "0", protectedLineExecutorWorkflow, ""); err == nil {
		t.Fatal("invalid run ID error = nil")
	}
	if err := protectedLinePublisher(nil).validateProtectedLineWorkflowRun(context.Background(), apiBase, repository, "456", "../invalid.yml", ""); err == nil {
		t.Fatal("invalid workflow error = nil")
	}
	credentialErr := errors.New("credential unavailable")
	credentialPublisher := New(Options{Resolver: &fakeCredentialResolver{err: credentialErr}, APIBaseURL: "https://api.example"})
	if err := credentialPublisher.validateProtectedLineWorkflowRun(context.Background(), apiBase, repository, "456", protectedLineExecutorWorkflow, ""); !errors.Is(err, credentialErr) {
		t.Fatalf("credential error = %v", err)
	}

	tests := []struct {
		name      string
		status    int
		body      string
		requester string
	}{
		{name: "status", status: http.StatusForbidden},
		{name: "json", status: http.StatusOK, body: "{"},
		{name: "path", status: http.StatusOK, body: string(mustProtectedLineJSON(t, protectedLineWorkflowRun("other.yml", "requester")))},
		{name: "event", status: http.StatusOK, body: `{"path":".github/workflows/execute-protected-line-request.yml","event":"push","head_branch":"main","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`},
		{name: "branch", status: http.StatusOK, body: `{"path":".github/workflows/execute-protected-line-request.yml","event":"workflow_dispatch","head_branch":"develop","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`},
		{name: "sha", status: http.StatusOK, body: `{"path":".github/workflows/execute-protected-line-request.yml","event":"workflow_dispatch","head_branch":"main","head_sha":"bad"}`},
		{name: "actor", status: http.StatusOK, body: `{"path":".github/workflows/execute-protected-line-request.yml","event":"workflow_dispatch","head_branch":"main","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","actor":{"login":"other"}}`, requester: "requester"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/repos/acme/governance/actions/runs/456" {
					t.Fatalf("unexpected path %s", request.URL.Path)
				}
				writer.WriteHeader(testCase.status)
				if testCase.body != "" {
					_, _ = writer.Write([]byte(testCase.body))
				}
			}))
			defer server.Close()
			err := protectedLinePublisher(server).validateProtectedLineWorkflowRun(
				context.Background(),
				mustAPIBase(t, server.URL),
				repositoryRef{host: strings.TrimPrefix(server.URL, "https://"), owner: "acme", name: "governance"},
				"456",
				protectedLineExecutorWorkflow,
				testCase.requester,
			)
			if err == nil {
				t.Fatal("provenance validation error = nil")
			}
		})
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/repos/acme/governance/actions/runs/456" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		writeProtectedLineJSON(t, writer, protectedLineWorkflowRun(protectedLineExecutorWorkflow, "requester"))
	}))
	defer server.Close()
	if err := protectedLinePublisher(server).validateProtectedLineWorkflowRun(
		context.Background(),
		mustAPIBase(t, server.URL),
		repositoryRef{host: strings.TrimPrefix(server.URL, "https://"), owner: "acme", name: "governance"},
		"456",
		protectedLineExecutorWorkflow,
		"requester",
	); err != nil {
		t.Fatalf("valid provenance error = %v", err)
	}
}

func TestProtectedLineFinalizerFailureTransitionsAndAuditWrites(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	originalTransition := protectedLineRequestTransition
	originalNow := protectedLineRequestNow
	protectedLineRequestNow = func() time.Time { return now }
	t.Cleanup(func() {
		protectedLineRequestTransition = originalTransition
		protectedLineRequestNow = originalNow
	})

	t.Run("rejects recovery for a non-pending request", func(t *testing.T) {
		record := protectedLineRecord(t, now, releaserequest.StateExecuting, "456")
		server := protectedLineRequestServer(t, record, []deploymentStatusResponse{{
			State:       "in_progress",
			Description: protectedLineStatusDescription(releaserequest.StateExecuting, "456"),
		}}, nil, nil)
		defer server.Close()
		if _, err := protectedLinePublisher(server).FinalizeProtectedLineRequest(context.Background(), protectedLineFinalizationAuthorization(server.URL, record.ID(), "", true)); err == nil {
			t.Fatal("non-pending recovery error = nil")
		}
	})

	t.Run("surfaces audit write failures for each final terminal state", func(t *testing.T) {
		cases := []struct {
			name   string
			record releaserequest.State
			jobs   []workflowJobResponse
			refs   map[string]string
		}{
			{name: "failed executor", record: releaserequest.StateExecuting, jobs: []workflowJobResponse{{Name: "Execute bound protected-line request", Status: "completed", Conclusion: "failure"}}},
			{name: "awaiting success", record: releaserequest.StateAwaitingExecutionApproval, jobs: successfulExecutorJob(), refs: map[string]string{"release/1.2.0": strings.Repeat("a", 40)}},
			{name: "missing ref", record: releaserequest.StateExecuting, jobs: successfulExecutorJob()},
			{name: "verified", record: releaserequest.StateExecuting, jobs: successfulExecutorJob(), refs: map[string]string{"release/1.2.0": strings.Repeat("a", 40)}},
		}
		for _, testCase := range cases {
			t.Run(testCase.name, func(t *testing.T) {
				record := protectedLineRecord(t, now, testCase.record, "456")
				server := protectedLineRequestServerWithStatusWrite(t, record, []deploymentStatusResponse{{
					State:       deploymentStateForTest(t, testCase.record),
					Description: protectedLineStatusDescription(testCase.record, "456"),
				}}, testCase.refs, testCase.jobs, http.StatusForbidden)
				defer server.Close()
				if _, err := protectedLinePublisher(server).FinalizeProtectedLineRequest(context.Background(), protectedLineFinalizationAuthorization(server.URL, record.ID(), "456", false)); err == nil {
					t.Fatal("audit write error = nil")
				}
			})
		}
	})

	t.Run("surfaces each internal finalizer transition error", func(t *testing.T) {
		cases := []struct {
			name   string
			record releaserequest.State
			jobs   []workflowJobResponse
			refs   map[string]string
		}{
			{name: "failed executor", record: releaserequest.StateExecuting, jobs: []workflowJobResponse{{Name: "Execute bound protected-line request", Status: "completed", Conclusion: "failure"}}},
			{name: "awaiting success", record: releaserequest.StateAwaitingExecutionApproval, jobs: successfulExecutorJob(), refs: map[string]string{"release/1.2.0": strings.Repeat("a", 40)}},
			{name: "missing ref", record: releaserequest.StateExecuting, jobs: successfulExecutorJob()},
			{name: "verified", record: releaserequest.StateExecuting, jobs: successfulExecutorJob(), refs: map[string]string{"release/1.2.0": strings.Repeat("a", 40)}},
		}
		for _, testCase := range cases {
			t.Run(testCase.name, func(t *testing.T) {
				protectedLineRequestTransition = func(releaserequest.Request, releaserequest.State, time.Time) (releaserequest.Request, error) {
					return releaserequest.Request{}, errors.New("transition unavailable")
				}
				record := protectedLineRecord(t, now, testCase.record, "456")
				server := protectedLineRequestServer(t, record, []deploymentStatusResponse{{
					State:       deploymentStateForTest(t, testCase.record),
					Description: protectedLineStatusDescription(testCase.record, "456"),
				}}, testCase.refs, testCase.jobs)
				defer server.Close()
				if _, err := protectedLinePublisher(server).FinalizeProtectedLineRequest(context.Background(), protectedLineFinalizationAuthorization(server.URL, record.ID(), "456", false)); err == nil {
					t.Fatal("transition error = nil")
				}
				protectedLineRequestTransition = originalTransition
			})
		}
	})

	t.Run("keeps an unobservable execution verification pending", func(t *testing.T) {
		record := protectedLineRecord(t, now, releaserequest.StateExecuting, "456")
		server := protectedLineRequestServerWithStatusWrite(t, record, []deploymentStatusResponse{{
			State:       "in_progress",
			Description: protectedLineStatusDescription(releaserequest.StateExecuting, "456"),
		}}, nil, nil, http.StatusForbidden)
		defer server.Close()
		if _, err := protectedLinePublisher(server).FinalizeProtectedLineRequest(context.Background(), protectedLineFinalizationAuthorization(server.URL, record.ID(), "456", false)); err == nil {
			t.Fatal("pending audit write error = nil")
		}
	})
}

func protectedLinePublisher(server *httptest.Server) *Publisher {
	if server == nil {
		return New(Options{Resolver: testCredentialResolver()})
	}
	return New(Options{Resolver: testCredentialResolver(), APIBaseURL: server.URL, HTTPClient: server.Client()})
}

func protectedLineAuthorization(t *testing.T, serverURL string) port.ProtectedLineRequestAuthorization {
	t.Helper()

	id, err := ticket.ParseID("GOV-50")
	if err != nil {
		t.Fatal(err)
	}
	return port.ProtectedLineRequestAuthorization{
		Repository:  port.RepositoryIdentity{Root: "C:/repository", Remote: "origin"},
		RemoteURL:   remoteURLForServer(serverURL),
		Ticket:      id,
		Operation:   releaserequest.OperationRelease,
		Version:     "1.2.0",
		Source:      mustProtectedLineBranch(t, "develop"),
		Target:      mustProtectedLineBranch(t, "release/1.2.0"),
		Requester:   "release-requester",
		ParentRunID: "123",
	}
}

func protectedLineExecutionAuthorization(serverURL, requestID, runID string) port.ProtectedLineExecutionAuthorization {
	return port.ProtectedLineExecutionAuthorization{
		Repository:    port.RepositoryIdentity{Root: "C:/repository", Remote: "origin"},
		RemoteURL:     remoteURLForServer(serverURL),
		RequestID:     requestID,
		ExecutorRunID: runID,
	}
}

func protectedLineFinalizationAuthorization(serverURL, requestID, runID string, recovery bool) port.ProtectedLineFinalizationRequest {
	return port.ProtectedLineFinalizationRequest{
		Repository:    port.RepositoryIdentity{Root: "C:/repository", Remote: "origin"},
		RemoteURL:     remoteURLForServer(serverURL),
		RequestID:     requestID,
		ExecutorRunID: runID,
		Recovery:      recovery,
	}
}

func protectedLineRecord(t *testing.T, now time.Time, state releaserequest.State, executorRunID string) releaserequest.Request {
	return protectedLineRecordWithIdempotency(t, now, state, executorRunID, strings.Repeat("b", 64))
}

func protectedLineRecordWithIdempotency(
	t *testing.T,
	now time.Time,
	state releaserequest.State,
	executorRunID string,
	idempotencyKey string,
) releaserequest.Request {
	t.Helper()

	id, err := ticket.ParseID("GOV-50")
	if err != nil {
		t.Fatal(err)
	}
	record, err := releaserequest.New(releaserequest.Input{
		ID:               "request-50",
		Repository:       "acme/governance",
		Operation:        releaserequest.OperationRelease,
		Ticket:           id,
		Version:          "1.2.0",
		Target:           mustProtectedLineBranch(t, "release/1.2.0"),
		Source:           mustProtectedLineBranch(t, "develop"),
		SourceSHA:        strings.Repeat("a", 40),
		Requester:        "release-requester",
		ExpectedExecutor: protectedLineExecutorWorkflow,
		ParentRunID:      "123",
		ExecutorRunID:    executorRunID,
		ExpiresAt:        now.Add(time.Hour),
		IdempotencyKey:   idempotencyKey,
		DeploymentID:     71,
		State:            state,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func protectedLineRequestServer(
	t *testing.T,
	record releaserequest.Request,
	statuses []deploymentStatusResponse,
	refs map[string]string,
	jobs []workflowJobResponse,
) *httptest.Server {
	t.Helper()

	return httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/repos/acme/governance/deployments":
			if record.ID() == "" {
				writeProtectedLineJSON(t, writer, []deploymentResponse{})
				return
			}
			writeProtectedLineJSON(t, writer, []deploymentResponse{{
				ID:          record.DeploymentID(),
				Task:        protectedLineRequestTask,
				Environment: protectedLineRequestEnvironment,
				Payload:     mustProtectedLineJSON(t, protectedLinePayloadFromRequest(record)),
			}})
		case "/repos/acme/governance/deployments/71/statuses":
			if request.Method == http.MethodPost {
				writer.WriteHeader(http.StatusCreated)
				return
			}
			writeProtectedLineJSON(t, writer, statuses)
		case "/repos/acme/governance/actions/runs/456":
			writeProtectedLineJSON(t, writer, protectedLineWorkflowRun(protectedLineExecutorWorkflow, "github-actions[bot]"))
		case "/repos/acme/governance/actions/runs/456/jobs":
			writeProtectedLineJSON(t, writer, workflowJobsResponse{Jobs: jobs})
		default:
			const prefix = "/repos/acme/governance/git/ref/heads/"
			if strings.HasPrefix(request.URL.Path, prefix) {
				ref := strings.TrimPrefix(request.URL.Path, prefix)
				if sha, found := refs[ref]; found {
					writeProtectedLineJSON(t, writer, gitReferenceResponse{Object: gitObjectReference{SHA: sha, Type: "commit"}})
					return
				}
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
	}))
}

func protectedLineRequestServerWithStatusWrite(
	t *testing.T,
	record releaserequest.Request,
	statuses []deploymentStatusResponse,
	refs map[string]string,
	jobs []workflowJobResponse,
	statusWriteCode int,
) *httptest.Server {
	t.Helper()

	return httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/repos/acme/governance/deployments":
			writeProtectedLineJSON(t, writer, []deploymentResponse{{
				ID:          record.DeploymentID(),
				Task:        protectedLineRequestTask,
				Environment: protectedLineRequestEnvironment,
				Payload:     mustProtectedLineJSON(t, protectedLinePayloadFromRequest(record)),
			}})
		case "/repos/acme/governance/deployments/71/statuses":
			if request.Method == http.MethodPost {
				writer.WriteHeader(statusWriteCode)
				return
			}
			writeProtectedLineJSON(t, writer, statuses)
		case "/repos/acme/governance/actions/runs/456":
			writeProtectedLineJSON(t, writer, protectedLineWorkflowRun(protectedLineExecutorWorkflow, "github-actions[bot]"))
		case "/repos/acme/governance/actions/runs/456/jobs":
			writeProtectedLineJSON(t, writer, workflowJobsResponse{Jobs: jobs})
		default:
			const prefix = "/repos/acme/governance/git/ref/heads/"
			if strings.HasPrefix(request.URL.Path, prefix) {
				ref := strings.TrimPrefix(request.URL.Path, prefix)
				if sha, found := refs[ref]; found {
					writeProtectedLineJSON(t, writer, gitReferenceResponse{Object: gitObjectReference{SHA: sha, Type: "commit"}})
					return
				}
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
	}))
}

func deploymentStateForTest(t *testing.T, state releaserequest.State) string {
	t.Helper()

	value, err := deploymentState(state)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func protectedLineWorkflowRun(workflow, actor string) protectedLineWorkflowRunResponse {
	result := protectedLineWorkflowRunResponse{
		Path:       ".github/workflows/" + workflow,
		Event:      "workflow_dispatch",
		HeadBranch: "main",
		HeadSHA:    strings.Repeat("a", 40),
	}
	result.Actor.Login = actor
	return result
}

func TestIsProtectedLineExecutorJob(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name     string
		jobName  string
		expected bool
	}{
		{name: "bare payload job", jobName: "Execute bound protected-line request", expected: true},
		{name: "composite caller and payload job", jobName: "Release execution / Execute bound protected-line request", expected: true},
		{name: "other job", jobName: "Finalize bound protected-line request", expected: false},
		{name: "prefixed without composite separator", jobName: "Execute bound protected-line request extra", expected: false},
		{name: "empty", jobName: "", expected: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if result := isProtectedLineExecutorJob(testCase.jobName); result != testCase.expected {
				t.Fatalf("isProtectedLineExecutorJob(%q) = %t, want %t", testCase.jobName, result, testCase.expected)
			}
		})
	}
}

func successfulExecutorJob() []workflowJobResponse {
	return []workflowJobResponse{{
		Name:       "Execute bound protected-line request",
		Status:     "completed",
		Conclusion: "success",
	}}
}

func mustProtectedLineBranch(t *testing.T, value string) branch.BranchName {
	t.Helper()

	name, err := branch.ParseName(value)
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func remoteURLForServer(serverURL string) string {
	return "https://" + strings.TrimPrefix(serverURL, "https://") + "/acme/governance.git"
}

func mustAPIBase(t *testing.T, value string) *url.URL {
	t.Helper()

	base, err := parseAPIBaseURL(value)
	if err != nil {
		t.Fatal(err)
	}
	return base
}

func decodeJSON(t *testing.T, request *http.Request, value any) {
	t.Helper()
	if err := json.NewDecoder(request.Body).Decode(value); err != nil {
		t.Fatal(err)
	}
}

func writeProtectedLineJSON(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		t.Fatal(err)
	}
}

func mustProtectedLineJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	result, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
