package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CyberT33N/git-governance/internal/application/port"
	"github.com/CyberT33N/git-governance/internal/domain/releaserequest"
)

type protectedLineWhiteboxProvider struct {
	authorizeResult port.ProtectedLineRequestResult
	authorizeErr    error
	authorizeCalls  []port.ProtectedLineRequestAuthorization

	executionResult port.ProtectedLineExecutionPlan
	executionErr    error
	executionCalls  []port.ProtectedLineExecutionAuthorization

	finalizationResult port.ProtectedLineFinalizationResult
	finalizationErr    error
	finalizationCalls  []port.ProtectedLineFinalizationRequest
}

func (provider *protectedLineWhiteboxProvider) AuthorizeProtectedLineRequest(
	_ context.Context,
	request port.ProtectedLineRequestAuthorization,
) (port.ProtectedLineRequestResult, error) {
	provider.authorizeCalls = append(provider.authorizeCalls, request)
	return provider.authorizeResult, provider.authorizeErr
}

func (provider *protectedLineWhiteboxProvider) AuthorizeProtectedLineExecution(
	_ context.Context,
	request port.ProtectedLineExecutionAuthorization,
) (port.ProtectedLineExecutionPlan, error) {
	provider.executionCalls = append(provider.executionCalls, request)
	return provider.executionResult, provider.executionErr
}

func (provider *protectedLineWhiteboxProvider) FinalizeProtectedLineRequest(
	_ context.Context,
	request port.ProtectedLineFinalizationRequest,
) (port.ProtectedLineFinalizationResult, error) {
	provider.finalizationCalls = append(provider.finalizationCalls, request)
	return provider.finalizationResult, provider.finalizationErr
}

func TestReleaseServiceProtectedLineRequestLifecycle(t *testing.T) {
	version := mustReleaseVersion(t, "1.2.0")
	ticketID := mustTicket("GOV-50")

	t.Run("authorizes a release request only through the configured provider", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		service := newReleaseWhiteboxService(git, nil)
		request := RequestProtectedLineRequest{
			Repository:  testRepository(),
			Ticket:      ticketID,
			Operation:   releaserequest.OperationRelease,
			Version:     version.String(),
			Requester:   "release-requester",
			ParentRunID: "123",
		}
		if _, err := service.RequestProtectedLine(context.Background(), request); err == nil {
			t.Fatal("missing provider error = nil")
		}

		provider := &protectedLineWhiteboxProvider{authorizeResult: port.ProtectedLineRequestResult{
			Request: protectedLineWorkflowRecord(t, releaserequest.StateAwaitingExecutionApproval, ""),
		}}
		service.WithProtectedLineRequestProvider(provider)
		result, err := service.RequestProtectedLine(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if result.DryRun || result.Intent.Branch.String() != "release/1.2.0" ||
			result.Intent.Source.String() != "origin/develop" || result.Request.Request.ID() != "request-50" ||
			len(provider.authorizeCalls) != 1 {
			t.Fatalf("request result = %#v, calls=%#v", result, provider.authorizeCalls)
		}
		call := provider.authorizeCalls[0]
		if call.Ticket.String() != "GOV-50" || call.Operation != releaserequest.OperationRelease ||
			call.Version != "1.2.0" || call.Source.String() != "develop" || call.Target.String() != "release/1.2.0" ||
			call.Requester != "release-requester" || call.ParentRunID != "123" || call.RemoteURL == "" {
			t.Fatalf("authorization call = %#v", call)
		}
	})

	t.Run("requires Git and validates repository and remote dependencies", func(t *testing.T) {
		provider := &protectedLineWhiteboxProvider{}
		request := RequestProtectedLineRequest{
			Repository:  testRepository(),
			Ticket:      ticketID,
			Operation:   releaserequest.OperationRelease,
			Version:     version.String(),
			Requester:   "release-requester",
			ParentRunID: "123",
		}
		if _, err := (&ReleaseService{}).WithProtectedLineRequestProvider(provider).RequestProtectedLine(context.Background(), request); err == nil {
			t.Fatal("missing Git dependency error = nil")
		}

		service := newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil).WithProtectedLineRequestProvider(provider)
		request.Repository = port.RepositoryIdentity{}
		if _, err := service.RequestProtectedLine(context.Background(), request); err == nil {
			t.Fatal("invalid repository error = nil")
		}

		git := &releaseLifecycleGit{
			releaseWhiteboxGit: newReleaseWhiteboxGit(),
			remoteURLErr:       errors.New("remote unavailable"),
		}
		if _, err := newReleaseWhiteboxService(git, nil).WithProtectedLineRequestProvider(provider).RequestProtectedLine(context.Background(), RequestProtectedLineRequest{
			Repository:  testRepository(),
			Ticket:      ticketID,
			Operation:   releaserequest.OperationRelease,
			Version:     version.String(),
			Requester:   "release-requester",
			ParentRunID: "123",
		}); !strings.Contains(err.Error(), "remote unavailable") {
			t.Fatalf("remote error = %v", err)
		}
	})

	t.Run("plans dry runs and rejects incomplete bindings", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		provider := &protectedLineWhiteboxProvider{}
		service := newReleaseWhiteboxService(git, nil).WithProtectedLineRequestProvider(provider)
		planned, err := service.RequestProtectedLine(context.Background(), RequestProtectedLineRequest{
			Repository: testRepository(),
			Ticket:     ticketID,
			Operation:  releaserequest.OperationRelease,
			Version:    version.String(),
			DryRun:     true,
		})
		if err != nil || !planned.DryRun || planned.Intent.Branch.String() != "release/1.2.0" || len(provider.authorizeCalls) != 0 {
			t.Fatalf("dry request = (%#v, %v), calls=%d", planned, err, len(provider.authorizeCalls))
		}
		for _, request := range []RequestProtectedLineRequest{
			{Repository: testRepository(), Operation: releaserequest.OperationRelease, Version: version.String()},
			{Repository: testRepository(), Ticket: ticketID, Operation: "unknown", Version: version.String()},
			{Repository: testRepository(), Ticket: ticketID, Operation: releaserequest.OperationRelease, Version: "invalid"},
		} {
			if _, err := service.RequestProtectedLine(context.Background(), request); err == nil {
				t.Fatal("invalid request error = nil")
			}
		}
	})

	t.Run("supports main-derived support request planning", func(t *testing.T) {
		git := newReleaseWhiteboxGit()
		git.releaseTags = []string{"v1.2.3"}
		provider := &protectedLineWhiteboxProvider{}
		service := newReleaseWhiteboxService(git, nil).WithProtectedLineRequestProvider(provider)
		result, err := service.RequestProtectedLine(context.Background(), RequestProtectedLineRequest{
			Repository: testRepository(),
			Ticket:     ticketID,
			Operation:  releaserequest.OperationSupport,
			Version:    "1.2",
			DryRun:     true,
		})
		if err != nil || result.Intent.Source.String() != "origin/main" || result.Intent.Branch.String() != "support/1.2" {
			t.Fatalf("support request = (%#v, %v)", result, err)
		}
	})

	t.Run("propagates authorization errors", func(t *testing.T) {
		failure := errors.New("authorize request")
		git := newReleaseWhiteboxGit()
		provider := &protectedLineWhiteboxProvider{authorizeErr: failure}
		service := newReleaseWhiteboxService(git, nil).WithProtectedLineRequestProvider(provider)
		_, err := service.RequestProtectedLine(context.Background(), RequestProtectedLineRequest{
			Repository:  testRepository(),
			Ticket:      ticketID,
			Operation:   releaserequest.OperationRelease,
			Version:     version.String(),
			Requester:   "release-requester",
			ParentRunID: "123",
		})
		if !errors.Is(err, failure) {
			t.Fatalf("authorization error = %v", err)
		}
	})
}

func TestReleaseServiceProtectedLineExecutionAndFinalization(t *testing.T) {
	record := protectedLineWorkflowRecord(t, releaserequest.StateExecuting, "456")
	executionPlan := port.ProtectedLineExecutionPlan{Request: record, NeedsMutation: true}
	finalization := port.ProtectedLineFinalizationResult{Request: record}

	t.Run("requires providers and propagates exact execution binding", func(t *testing.T) {
		service := newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil)
		if _, err := service.AuthorizeProtectedLineExecution(context.Background(), testRepository(), "request-50", "456"); err == nil {
			t.Fatal("missing provider execution error = nil")
		}
		if _, err := service.FinalizeProtectedLineRequest(context.Background(), testRepository(), "request-50", "456", false); err == nil {
			t.Fatal("missing provider finalization error = nil")
		}

		provider := &protectedLineWhiteboxProvider{
			executionResult:    executionPlan,
			finalizationResult: finalization,
		}
		service.WithProtectedLineRequestProvider(provider)
		plan, err := service.AuthorizeProtectedLineExecution(context.Background(), testRepository(), "request-50", "456")
		if err != nil || plan != executionPlan || len(provider.executionCalls) != 1 {
			t.Fatalf("execution = (%#v, %v), calls=%#v", plan, err, provider.executionCalls)
		}
		if provider.executionCalls[0].RequestID != "request-50" || provider.executionCalls[0].ExecutorRunID != "456" ||
			provider.executionCalls[0].RemoteURL == "" {
			t.Fatalf("execution call = %#v", provider.executionCalls[0])
		}
		result, err := service.FinalizeProtectedLineRequest(context.Background(), testRepository(), "request-50", "456", true)
		if err != nil || result != finalization || len(provider.finalizationCalls) != 1 {
			t.Fatalf("finalization = (%#v, %v), calls=%#v", result, err, provider.finalizationCalls)
		}
		if !provider.finalizationCalls[0].Recovery || provider.finalizationCalls[0].ExecutorRunID != "456" ||
			provider.finalizationCalls[0].RemoteURL == "" {
			t.Fatalf("finalization call = %#v", provider.finalizationCalls[0])
		}
	})

	t.Run("requires Git and rejects invalid repository before provider use", func(t *testing.T) {
		provider := &protectedLineWhiteboxProvider{}
		if _, err := (&ReleaseService{}).WithProtectedLineRequestProvider(provider).AuthorizeProtectedLineExecution(context.Background(), testRepository(), "request-50", "456"); err == nil {
			t.Fatal("missing execution Git dependency error = nil")
		}
		if _, err := (&ReleaseService{}).WithProtectedLineRequestProvider(provider).FinalizeProtectedLineRequest(context.Background(), testRepository(), "request-50", "456", false); err == nil {
			t.Fatal("missing finalization Git dependency error = nil")
		}
		service := newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil).WithProtectedLineRequestProvider(provider)
		if _, err := service.AuthorizeProtectedLineExecution(context.Background(), port.RepositoryIdentity{}, "request-50", "456"); err == nil {
			t.Fatal("invalid execution repository error = nil")
		}
		if _, err := service.FinalizeProtectedLineRequest(context.Background(), port.RepositoryIdentity{}, "request-50", "456", false); err == nil {
			t.Fatal("invalid finalization repository error = nil")
		}

		remoteFailure := &releaseLifecycleGit{
			releaseWhiteboxGit: newReleaseWhiteboxGit(),
			remoteURLErr:       errors.New("remote unavailable"),
		}
		remoteService := newReleaseWhiteboxService(remoteFailure, nil).WithProtectedLineRequestProvider(provider)
		if _, err := remoteService.AuthorizeProtectedLineExecution(context.Background(), testRepository(), "request-50", "456"); !strings.Contains(err.Error(), "remote unavailable") {
			t.Fatalf("execution remote error = %v", err)
		}
		if _, err := remoteService.FinalizeProtectedLineRequest(context.Background(), testRepository(), "request-50", "456", false); !strings.Contains(err.Error(), "remote unavailable") {
			t.Fatalf("finalization remote error = %v", err)
		}
	})

	t.Run("propagates provider and repository errors", func(t *testing.T) {
		executionFailure := errors.New("execution")
		finalizationFailure := errors.New("finalization")
		provider := &protectedLineWhiteboxProvider{
			executionErr:    executionFailure,
			finalizationErr: finalizationFailure,
		}
		service := newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil).WithProtectedLineRequestProvider(provider)
		if _, err := service.AuthorizeProtectedLineExecution(context.Background(), testRepository(), "request-50", "456"); !errors.Is(err, executionFailure) {
			t.Fatalf("execution error = %v", err)
		}
		if _, err := service.FinalizeProtectedLineRequest(context.Background(), testRepository(), "request-50", "456", false); !errors.Is(err, finalizationFailure) {
			t.Fatalf("finalization error = %v", err)
		}
		if _, err := service.AuthorizeProtectedLineExecution(context.Background(), port.RepositoryIdentity{}, "request-50", "456"); err == nil {
			t.Fatal("invalid repository error = nil")
		}
	})
}

func TestProtectedLineIntentPropagatesSourcePreparationErrors(t *testing.T) {
	repository := testRepository()
	service := newReleaseWhiteboxService(newReleaseWhiteboxGit(), nil)
	if _, err := service.protectedLineIntent(context.Background(), repository, releaserequest.OperationSupport, "invalid", true); err == nil {
		t.Fatal("invalid support version error = nil")
	}

	releaseGit := newReleaseWhiteboxGit()
	releaseGit.fetchErrors = []error{errors.New("fetch release source")}
	if _, err := newReleaseWhiteboxService(releaseGit, nil).protectedLineIntent(context.Background(), repository, releaserequest.OperationRelease, "1.2.0", false); !strings.Contains(err.Error(), "fetch release source") {
		t.Fatalf("release source error = %v", err)
	}

	supportGit := newReleaseWhiteboxGit()
	supportGit.releaseTagsErr = errors.New("release tags unavailable")
	if _, err := newReleaseWhiteboxService(supportGit, nil).protectedLineIntent(context.Background(), repository, releaserequest.OperationSupport, "1.2", false); !strings.Contains(err.Error(), "release tags unavailable") {
		t.Fatalf("support source error = %v", err)
	}
}

func protectedLineWorkflowRecord(t *testing.T, state releaserequest.State, executorRun string) releaserequest.Request {
	t.Helper()

	request, err := releaserequest.New(releaserequest.Input{
		ID:               "request-50",
		Repository:       "acme/governance",
		Operation:        releaserequest.OperationRelease,
		Ticket:           mustTicket("GOV-50"),
		Version:          "1.2.0",
		Target:           mustBranch("release/1.2.0"),
		Source:           mustBranch("develop"),
		SourceSHA:        strings.Repeat("a", 40),
		Requester:        "release-requester",
		ExpectedExecutor: "create-protected-line.yml",
		ParentRunID:      "123",
		ExecutorRunID:    executorRun,
		ExpiresAt:        time.Now().Add(time.Hour),
		IdempotencyKey:   strings.Repeat("b", 64),
		DeploymentID:     71,
		State:            state,
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return request
}
