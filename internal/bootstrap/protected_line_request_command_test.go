package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CyberT33N/git-governance/internal/application/port"
	"github.com/CyberT33N/git-governance/internal/domain/branch"
	"github.com/CyberT33N/git-governance/internal/domain/problem"
	"github.com/CyberT33N/git-governance/internal/domain/releaserequest"
	"github.com/CyberT33N/git-governance/internal/domain/ticket"
	"github.com/spf13/cobra"
)

type protectedLineCommandPublisher struct {
	authorizeCalls    []port.ProtectedLineRequestAuthorization
	executionCalls    []port.ProtectedLineExecutionAuthorization
	finalizationCalls []port.ProtectedLineFinalizationRequest
	authorizeErr      error
	executionErr      error
	finalizationErr   error
}

func (publisher *protectedLineCommandPublisher) Publish(context.Context, port.PullRequestPublication) (port.PublishedPullRequest, error) {
	return port.PublishedPullRequest{}, nil
}

func (publisher *protectedLineCommandPublisher) AuthorizeProtectedLineRequest(
	_ context.Context,
	request port.ProtectedLineRequestAuthorization,
) (port.ProtectedLineRequestResult, error) {
	publisher.authorizeCalls = append(publisher.authorizeCalls, request)
	return port.ProtectedLineRequestResult{Request: protectedLineCommandRecord()}, publisher.authorizeErr
}

func (publisher *protectedLineCommandPublisher) AuthorizeProtectedLineExecution(
	_ context.Context,
	request port.ProtectedLineExecutionAuthorization,
) (port.ProtectedLineExecutionPlan, error) {
	publisher.executionCalls = append(publisher.executionCalls, request)
	return port.ProtectedLineExecutionPlan{
		Request:       protectedLineCommandRecord(),
		NeedsMutation: true,
	}, publisher.executionErr
}

func (publisher *protectedLineCommandPublisher) FinalizeProtectedLineRequest(
	_ context.Context,
	request port.ProtectedLineFinalizationRequest,
) (port.ProtectedLineFinalizationResult, error) {
	publisher.finalizationCalls = append(publisher.finalizationCalls, request)
	return port.ProtectedLineFinalizationResult{Request: protectedLineCommandRecord()}, publisher.finalizationErr
}

func TestProtectedLineRequestCommandsRequireControllerBoundary(t *testing.T) {
	command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(newCommandGit(t, "feature/GOV-50-release-request-execution", nil)))
	_, err := executeBootstrapCommand(
		t,
		command,
		"--interactive", "never", "--output", "json", "--yes",
		"workflow", "release", "request",
		"--kind", "release",
		"--version", "1.2.0",
		"--key", "GOV",
		"--ticket", "50",
		"--requester", "requester",
		"--parent-run", "123",
	)
	assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
}

func TestProtectedLineRequestCommandsBindControllerInputs(t *testing.T) {
	publisher := &protectedLineCommandPublisher{}
	runtime := commandRuntime(newCommandGit(t, "feature/GOV-50-release-request-execution", nil))
	runtime.Publisher = publisher
	runtime.GitHubWorkflowTokenEnabled = func() bool { return true }
	runtime.GitHubWorkflowToken = func() string { return "ephemeral-token" }
	command := NewWithRuntime(BuildInfo{Version: "test"}, runtime)

	output, err := executeBootstrapCommand(
		t,
		command,
		"--interactive", "never", "--output", "json", "--yes",
		"--pull-request-provider", "github",
		"workflow", "release", "request",
		"--kind", "release",
		"--version", "1.2.0",
		"--key", "GOV",
		"--ticket", "50",
		"--requester", "requester",
		"--parent-run", "123",
	)
	if err != nil || len(publisher.authorizeCalls) != 1 || !strings.Contains(output, `"requestID":"request-50"`) {
		t.Fatalf("request command = (%q, %v), calls=%#v", output, err, publisher.authorizeCalls)
	}
	authorization := publisher.authorizeCalls[0]
	if authorization.Operation != releaserequest.OperationRelease || authorization.Version != "1.2.0" ||
		authorization.Ticket.String() != "GOV-50" || authorization.Requester != "requester" ||
		authorization.ParentRunID != "123" || authorization.Source.String() != "develop" ||
		authorization.Target.String() != "release/1.2.0" {
		t.Fatalf("request authorization = %#v", authorization)
	}

	output, err = executeBootstrapCommand(
		t,
		command,
		"--interactive", "never", "--output", "json", "--yes",
		"--pull-request-provider", "github",
		"workflow", "release", "execute-request",
		"--request-id", "request-50",
		"--executor-run", "456",
	)
	if err != nil || len(publisher.executionCalls) != 1 || !strings.Contains(output, `"needsMutation":"true"`) {
		t.Fatalf("execute command = (%q, %v), calls=%#v", output, err, publisher.executionCalls)
	}

	output, err = executeBootstrapCommand(
		t,
		command,
		"--interactive", "never", "--output", "json", "--yes",
		"--pull-request-provider", "github",
		"workflow", "release", "finalize-request",
		"--request-id", "request-50",
		"--executor-run", "456",
	)
	if err != nil || len(publisher.finalizationCalls) != 1 || !strings.Contains(output, `"state":"executing"`) {
		t.Fatalf("finalize command = (%q, %v), calls=%#v", output, err, publisher.finalizationCalls)
	}

	for _, value := range []string{"release", "support"} {
		operation, parseErr := parseProtectedLineOperation(value)
		if parseErr != nil || string(operation) != value {
			t.Fatalf("parse operation %q = (%q, %v)", value, operation, parseErr)
		}
	}
	if _, parseErr := parseProtectedLineOperation("invalid"); parseErr == nil {
		t.Fatal("invalid operation error = nil")
	}
}

func TestProtectedLineRequestCommandFailurePaths(t *testing.T) {
	newCommand := func(publisher *protectedLineCommandPublisher, enabled bool) *cobra.Command {
		runtime := commandRuntime(newCommandGit(t, "feature/GOV-50-release-request-execution", nil))
		runtime.Publisher = publisher
		runtime.GitHubWorkflowTokenEnabled = func() bool { return enabled }
		runtime.GitHubWorkflowToken = func() string { return "ephemeral-token" }
		return NewWithRuntime(BuildInfo{Version: "test"}, runtime)
	}
	requestArgs := []string{
		"--interactive", "never", "--output", "json", "--yes",
		"--pull-request-provider", "github",
		"workflow", "release", "request",
		"--kind", "release",
		"--version", "1.2.0",
		"--key", "GOV",
		"--ticket", "50",
		"--requester", "requester",
		"--parent-run", "123",
	}

	t.Run("request validates all controller inputs and propagates service errors", func(t *testing.T) {
		cases := []struct {
			name string
			args []string
		}{
			{name: "invalid operation", args: replaceCommandArgument(requestArgs, "--kind", "invalid")},
			{name: "invalid release version", args: replaceCommandArgument(requestArgs, "--version", "invalid")},
			{name: "invalid key", args: replaceCommandArgument(requestArgs, "--key", "invalid")},
			{name: "invalid ticket", args: replaceCommandArgument(requestArgs, "--ticket", "0")},
			{name: "missing requester", args: removeCommandArgument(requestArgs, "--requester")},
			{name: "missing parent run", args: removeCommandArgument(requestArgs, "--parent-run")},
			{name: "missing confirmation", args: removeCommandFlag(requestArgs, "--yes")},
		}
		for _, testCase := range cases {
			t.Run(testCase.name, func(t *testing.T) {
				_, err := executeBootstrapCommand(t, newCommand(&protectedLineCommandPublisher{}, true), testCase.args...)
				if err == nil {
					t.Fatal("request command error = nil")
				}
			})
		}
		failure := errors.New("authorize")
		_, err := executeBootstrapCommand(t, newCommand(&protectedLineCommandPublisher{authorizeErr: failure}, true), requestArgs...)
		if !errors.Is(err, failure) {
			t.Fatalf("request provider error = %v", err)
		}
		supportArgs := replaceCommandArgument(replaceCommandArgument(requestArgs, "--kind", "support"), "--version", "1.2")
		supportArgs = append([]string{"--dry-run"}, supportArgs...)
		if _, err := executeBootstrapCommand(t, newCommand(&protectedLineCommandPublisher{}, true), supportArgs...); err != nil {
			t.Fatalf("support dry request error = %v", err)
		}
		if _, err := newApplication(Runtime{}, runtimeTestOptions()).resolveProtectedLineVersion(context.Background(), releaserequest.OperationRelease, "invalid"); err == nil {
			t.Fatal("invalid release version resolver error = nil")
		}
		if _, err := newApplication(Runtime{}, runtimeTestOptions()).resolveProtectedLineVersion(context.Background(), releaserequest.OperationSupport, "invalid"); err == nil {
			t.Fatal("invalid support version resolver error = nil")
		}
		if _, err := newApplication(Runtime{}, runtimeTestOptions()).resolveProtectedLineVersion(context.Background(), "invalid", "1.2.0"); err == nil {
			t.Fatal("invalid operation resolver error = nil")
		}
	})

	t.Run("execution and finalization retain controller, input, confirmation, recovery, and provider failures", func(t *testing.T) {
		executeArgs := []string{
			"--interactive", "never", "--output", "json", "--yes",
			"--pull-request-provider", "github",
			"workflow", "release", "execute-request",
			"--request-id", "request-50",
			"--executor-run", "456",
		}
		finalizeArgs := []string{
			"--interactive", "never", "--output", "json", "--yes",
			"--pull-request-provider", "github",
			"workflow", "release", "finalize-request",
			"--request-id", "request-50",
			"--executor-run", "456",
		}
		_, err := executeBootstrapCommand(t, newCommand(&protectedLineCommandPublisher{}, false), executeArgs...)
		assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
		_, err = executeBootstrapCommand(t, newCommand(&protectedLineCommandPublisher{}, false), finalizeArgs...)
		assertProblemCode(t, err, problem.CodeConfigurationUnavailable)

		for _, args := range [][]string{
			removeCommandArgument(executeArgs, "--request-id"),
			removeCommandArgument(executeArgs, "--executor-run"),
			removeCommandFlag(executeArgs, "--yes"),
			removeCommandArgument(finalizeArgs, "--request-id"),
			removeCommandArgument(finalizeArgs, "--executor-run"),
			removeCommandFlag(finalizeArgs, "--yes"),
		} {
			_, err = executeBootstrapCommand(t, newCommand(&protectedLineCommandPublisher{}, true), args...)
			if err == nil {
				t.Fatal("controller command error = nil")
			}
		}
		executionFailure := errors.New("execute")
		_, err = executeBootstrapCommand(t, newCommand(&protectedLineCommandPublisher{executionErr: executionFailure}, true), executeArgs...)
		if !errors.Is(err, executionFailure) {
			t.Fatalf("execution provider error = %v", err)
		}
		finalizationFailure := errors.New("finalize")
		_, err = executeBootstrapCommand(t, newCommand(&protectedLineCommandPublisher{finalizationErr: finalizationFailure}, true), finalizeArgs...)
		if !errors.Is(err, finalizationFailure) {
			t.Fatalf("finalization provider error = %v", err)
		}
		recoveryArgs := removeCommandArgument(finalizeArgs, "--executor-run")
		recoveryArgs = append(recoveryArgs, "--recovery")
		if _, err := executeBootstrapCommand(t, newCommand(&protectedLineCommandPublisher{}, true), recoveryArgs...); err != nil {
			t.Fatalf("recovery finalization error = %v", err)
		}
	})

	t.Run("propagates repository discovery errors for every controller phase", func(t *testing.T) {
		discoveryFailure := errors.New("discover repository")
		newDiscoveryFailureCommand := func() *cobra.Command {
			runtime := commandRuntime(&runtimeTestGit{discoverErr: discoveryFailure})
			runtime.Publisher = &protectedLineCommandPublisher{}
			runtime.GitHubWorkflowTokenEnabled = func() bool { return true }
			runtime.GitHubWorkflowToken = func() string { return "ephemeral-token" }
			return NewWithRuntime(BuildInfo{Version: "test"}, runtime)
		}
		commands := [][]string{
			requestArgs,
			{
				"--interactive", "never", "--output", "json", "--yes",
				"--pull-request-provider", "github",
				"workflow", "release", "execute-request",
				"--request-id", "request-50",
				"--executor-run", "456",
			},
			{
				"--interactive", "never", "--output", "json", "--yes",
				"--pull-request-provider", "github",
				"workflow", "release", "finalize-request",
				"--request-id", "request-50",
				"--executor-run", "456",
			},
		}
		for _, args := range commands {
			_, err := executeBootstrapCommand(t, newDiscoveryFailureCommand(), args...)
			if !errors.Is(err, discoveryFailure) {
				t.Fatalf("discovery error = %v", err)
			}
		}
	})
}

func replaceCommandArgument(args []string, flag, value string) []string {
	result := append([]string(nil), args...)
	for index := range result {
		if result[index] == flag && index+1 < len(result) {
			result[index+1] = value
			return result
		}
	}
	panic("missing flag " + flag)
}

func removeCommandArgument(args []string, flag string) []string {
	result := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		if args[index] == flag {
			index++
			continue
		}
		result = append(result, args[index])
	}
	return result
}

func removeCommandFlag(args []string, flag string) []string {
	result := make([]string, 0, len(args))
	for _, value := range args {
		if value != flag {
			result = append(result, value)
		}
	}
	return result
}

func protectedLineCommandRecord() releaserequest.Request {
	id, _ := ticket.ParseID("GOV-50")
	target, _ := branch.ParseName("release/1.2.0")
	source, _ := branch.ParseName("develop")
	record, _ := releaserequest.New(releaserequest.Input{
		ID:               "request-50",
		Repository:       "acme/governance",
		Operation:        releaserequest.OperationRelease,
		Ticket:           id,
		Version:          "1.2.0",
		Target:           target,
		Source:           source,
		SourceSHA:        strings.Repeat("a", 40),
		Requester:        "requester",
		ExpectedExecutor: "create-protected-line.yml",
		ParentRunID:      "123",
		ExecutorRunID:    "456",
		ExpiresAt:        time.Now().Add(time.Hour),
		IdempotencyKey:   strings.Repeat("b", 64),
		DeploymentID:     71,
		State:            releaserequest.StateExecuting,
	}, time.Now())
	return record
}

var _ port.PullRequestPublisher = (*protectedLineCommandPublisher)(nil)
var _ port.ProtectedLineRequestProvider = (*protectedLineCommandPublisher)(nil)
