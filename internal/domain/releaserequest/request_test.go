package releaserequest

import (
	"strings"
	"testing"
	"time"

	"github.com/CyberT33N/git-governance/internal/domain/branch"
	"github.com/CyberT33N/git-governance/internal/domain/ticket"
)

func TestNewValidatesImmutableRequestBinding(t *testing.T) {
	now := time.Date(2026, time.August, 10, 10, 0, 0, 0, time.UTC)

	t.Run("accepts release and support bindings", func(t *testing.T) {
		release, err := New(validInput(t, now), now)
		if err != nil {
			t.Fatal(err)
		}
		if release.ID() != "request-50" ||
			release.Repository() != "CyberT33N/git-governance" ||
			release.Operation() != OperationRelease ||
			release.Ticket().String() != "GOV-50" ||
			release.Version() != "1.2.0" ||
			release.Target().String() != "release/1.2.0" ||
			release.Source().String() != "develop" ||
			release.SourceSHA() != strings.Repeat("a", 40) ||
			release.Requester() != "release-requester" ||
			release.ExpectedExecutor() != "execute-protected-line-request.yml" ||
			release.ParentRunID() != "123" ||
			release.ExecutorRunID() != "" ||
			release.IdempotencyKey() != strings.Repeat("b", 64) ||
			release.DeploymentID() != 0 ||
			release.State() != StateAwaitingExecutionApproval ||
			!release.ExpiresAt().Equal(now.Add(time.Hour)) {
			t.Fatalf("release request = %#v", release)
		}
		if release.WithDeploymentID(71).DeploymentID() != 71 || release.DeploymentID() != 0 {
			t.Fatal("deployment ID binding mutated the original request")
		}

		support := validInput(t, now)
		support.Operation = OperationSupport
		support.Version = "1.2"
		support.Source = mustBranch(t, "main")
		support.Target = mustBranch(t, "support/1.2")
		support.DeploymentID = 99
		support.ExecutorRunID = "456"
		support.State = StateExecuting
		request, err := New(support, now)
		if err != nil {
			t.Fatal(err)
		}
		if request.Operation() != OperationSupport || request.Target().String() != "support/1.2" ||
			request.Source().String() != "main" || request.DeploymentID() != 99 || request.ExecutorRunID() != "456" {
			t.Fatalf("support request = %#v", request)
		}
	})

	cases := []struct {
		name   string
		mutate func(*Input)
	}{
		{"request id", func(input *Input) { input.ID = "bad id" }},
		{"repository", func(input *Input) { input.Repository = "owner/repo/extra" }},
		{"ticket", func(input *Input) { input.Ticket = ticket.ID{} }},
		{"source sha", func(input *Input) { input.SourceSHA = strings.Repeat("A", 40) }},
		{"requester", func(input *Input) { input.Requester = "requester\ninvalid" }},
		{"executor", func(input *Input) { input.ExpectedExecutor = "../executor.yml" }},
		{"parent run", func(input *Input) { input.ParentRunID = "0" }},
		{"executor run", func(input *Input) { input.ExecutorRunID = "run" }},
		{"expiry", func(input *Input) { input.ExpiresAt = time.Time{} }},
		{"idempotency key", func(input *Input) { input.IdempotencyKey = "key" }},
		{"deployment", func(input *Input) { input.DeploymentID = -1 }},
		{"release version", func(input *Input) { input.Version = "1.2" }},
		{"release source", func(input *Input) { input.Source = mustBranch(t, "main") }},
		{"release target", func(input *Input) { input.Target = mustBranch(t, "release/1.3.0") }},
		{"operation", func(input *Input) { input.Operation = "unknown" }},
		{"state", func(input *Input) { input.State = "unknown" }},
	}
	for _, testCase := range cases {
		t.Run("rejects "+testCase.name, func(t *testing.T) {
			input := validInput(t, now)
			testCase.mutate(&input)
			if _, err := New(input, now); err == nil {
				t.Fatal("New() error = nil")
			}
		})
	}

	t.Run("rejects malformed support binding", func(t *testing.T) {
		input := validInput(t, now)
		input.Operation = OperationSupport
		input.Version = "1.2"
		input.Source = mustBranch(t, "main")
		input.Target = mustBranch(t, "support/1.3")
		if _, err := New(input, now); err == nil {
			t.Fatal("New() error = nil")
		}
		input.Target = mustBranch(t, "support/1.2")
		input.Source = mustBranch(t, "develop")
		if _, err := New(input, now); err == nil {
			t.Fatal("New() accepted a support request from develop")
		}
		input.Source = mustBranch(t, "main")
		input.Version = "invalid"
		if _, err := New(input, now); err == nil {
			t.Fatal("New() accepted an invalid support version")
		}
	})
}

func TestRequestExecutionAndFinalizationStateTransitions(t *testing.T) {
	now := time.Date(2026, time.August, 10, 10, 0, 0, 0, time.UTC)
	request, err := New(validInput(t, now), now)
	if err != nil {
		t.Fatal(err)
	}
	if request.Expired(now) || !request.CanExecute(now) || request.CanRecover() {
		t.Fatalf("initial request state is invalid: %#v", request)
	}

	if _, err := request.WithExecutionRun("bad", now); err == nil {
		t.Fatal("WithExecutionRun() error = nil")
	}
	executing, err := request.WithExecutionRun("456", now)
	if err != nil {
		t.Fatal(err)
	}
	if executing.ExecutorRunID() != "456" || executing.State() != StateExecuting || executing.CanExecute(now) {
		t.Fatalf("executing request = %#v", executing)
	}

	verified, err := executing.Transition(StateVerified, now)
	if err != nil {
		t.Fatal(err)
	}
	if verified.State() != StateVerified {
		t.Fatalf("verified request = %#v", verified)
	}
	if _, err := verified.Transition(StateVerified, now); err != nil {
		t.Fatal(err)
	}
	if _, err := verified.Transition(StateExecuting, now); err == nil {
		t.Fatal("terminal transition error = nil")
	}
	if _, err := verified.WithExecutionRun("456", now); err == nil {
		t.Fatal("WithExecutionRun() accepted a non-executable request")
	}

	pending, err := executing.Transition(StateVerificationPending, now)
	if err != nil {
		t.Fatal(err)
	}
	if !pending.CanRecover() {
		t.Fatalf("pending request = %#v", pending)
	}
	if _, err := pending.Transition(StateFailed, now); err != nil {
		t.Fatal(err)
	}

	failed, err := executing.Transition(StateFailed, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := failed.Transition(StateFailed, now); err != nil {
		t.Fatal(err)
	}

	rejected, err := request.Transition(StateRejected, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rejected.Transition(StateRejected, now); err != nil {
		t.Fatal(err)
	}

	expiredInput := validInput(t, now)
	expiredInput.ExpiresAt = now.Add(time.Second)
	expiring, err := New(expiredInput, now)
	if err != nil {
		t.Fatal(err)
	}
	later := now.Add(time.Second)
	if !expiring.Expired(later) || expiring.CanExecute(later) {
		t.Fatalf("expired request = %#v", expiring)
	}
	expired, err := expiring.Transition(StateExpired, later)
	if err != nil {
		t.Fatal(err)
	}
	if expired.State() != StateExpired {
		t.Fatalf("expired transition = %#v", expired)
	}
	if _, err := expiring.Transition(StateFailed, later); err == nil {
		t.Fatal("expired request transitioned to failed")
	}

	requestedInput := validInput(t, now)
	requestedInput.State = StateRequested
	requested, err := New(requestedInput, now)
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := requested.Transition(StateRequestAuthorized, now)
	if err != nil {
		t.Fatal(err)
	}
	awaiting, err := authorized.Transition(StateAwaitingExecutionApproval, now)
	if err != nil {
		t.Fatal(err)
	}
	if !awaiting.CanExecute(now) {
		t.Fatalf("awaiting request = %#v", awaiting)
	}
}

func validInput(t *testing.T, now time.Time) Input {
	t.Helper()

	id, err := ticket.ParseID("GOV-50")
	if err != nil {
		t.Fatal(err)
	}
	return Input{
		ID:               "request-50",
		Repository:       "CyberT33N/git-governance",
		Operation:        OperationRelease,
		Ticket:           id,
		Version:          "1.2.0",
		Target:           mustBranch(t, "release/1.2.0"),
		Source:           mustBranch(t, "develop"),
		SourceSHA:        strings.Repeat("a", 40),
		Requester:        "release-requester",
		ExpectedExecutor: "execute-protected-line-request.yml",
		ParentRunID:      "123",
		ExpiresAt:        now.Add(time.Hour),
		IdempotencyKey:   strings.Repeat("b", 64),
		State:            StateAwaitingExecutionApproval,
	}
}

func mustBranch(t *testing.T, value string) branch.BranchName {
	t.Helper()

	name, err := branch.ParseName(value)
	if err != nil {
		t.Fatal(err)
	}
	return name
}
