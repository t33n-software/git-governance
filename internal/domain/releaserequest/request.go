// Package releaserequest defines the immutable, provider-independent facts
// that bind a protected release or support-line mutation to its authorization.
package releaserequest

import (
	"regexp"
	"strings"
	"time"

	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

const SchemaVersion = 1

var (
	requestIDPattern      = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	commitPattern         = regexp.MustCompile(`^[0-9a-f]{40}$`)
	workflowPattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*\.yml$`)
	runIDPattern          = regexp.MustCompile(`^[1-9][0-9]*$`)
	idempotencyKeyPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Operation identifies the protected-line lifecycle requested by one record.
type Operation string

const (
	OperationRelease Operation = "release"
	OperationSupport Operation = "support"
)

// State is the durable lifecycle state derived from the provider's audit
// record. Only Verified represents a completed protected-line cut.
type State string

const (
	StateRequested                 State = "requested"
	StateRequestAuthorized         State = "request_authorized"
	StateAwaitingExecutionApproval State = "awaiting_execution_approval"
	StateExecuting                 State = "executing"
	StateVerified                  State = "verified"
	StateFailed                    State = "failed"
	StateRejected                  State = "rejected"
	StateExpired                   State = "expired"
	StateVerificationPending       State = "verification_pending"
)

// Request is the immutable authorization binding for one protected-line
// mutation. Its fields are private so every instance has passed New or Parse.
type Request struct {
	id               string
	repository       string
	operation        Operation
	ticket           ticket.ID
	version          string
	target           branch.BranchName
	source           branch.BranchName
	sourceSHA        string
	requester        string
	expectedExecutor string
	parentRunID      string
	executorRunID    string
	expiresAt        time.Time
	idempotencyKey   string
	deploymentID     int64
	state            State
}

// Input contains every immutable authorization fact for one request.
type Input struct {
	ID               string
	Repository       string
	Operation        Operation
	Ticket           ticket.ID
	Version          string
	Target           branch.BranchName
	Source           branch.BranchName
	SourceSHA        string
	Requester        string
	ExpectedExecutor string
	ParentRunID      string
	ExecutorRunID    string
	ExpiresAt        time.Time
	IdempotencyKey   string
	DeploymentID     int64
	State            State
}

// New validates and constructs an immutable protected-line request.
func New(input Input, _ time.Time) (Request, error) {
	if !requestIDPattern.MatchString(input.ID) {
		return Request{}, invalid(
			"release request ID",
			"1 to 64 ASCII letters, digits, underscores, or hyphens",
			"create or select a bounded request identifier before authorization",
		)
	}
	if !validRepository(input.Repository) {
		return Request{}, invalid(
			"release request repository",
			"an owner/repository identifier without whitespace",
			"bind the request to its exact GitHub repository",
		)
	}
	if input.Ticket.Key().String() == "" || input.Ticket.Number().String() == "" {
		return Request{}, invalid(
			"release request ticket",
			"a non-empty governed ticket ID",
			"bind the request to the ticket or milestone authorizing the release scope",
		)
	}
	if !commitPattern.MatchString(input.SourceSHA) {
		return Request{}, invalid(
			"release request source SHA",
			"a lowercase full 40-character commit SHA",
			"resolve and bind the exact authorized source revision",
		)
	}
	if !validSingleLine(input.Requester, 200) {
		return Request{}, invalid(
			"release request requester",
			"a non-empty one-line requester identifier",
			"record the actor who requested the protected-line cut",
		)
	}
	if !workflowPattern.MatchString(input.ExpectedExecutor) {
		return Request{}, invalid(
			"release request executor",
			"a bounded .yml workflow filename",
			"bind the request to the expected protected-line executor workflow",
		)
	}
	if !runIDPattern.MatchString(input.ParentRunID) {
		return Request{}, invalid(
			"release request parent run",
			"a positive GitHub Actions run identifier",
			"record the request-controller workflow run before dispatching execution",
		)
	}
	if input.ExecutorRunID != "" && !runIDPattern.MatchString(input.ExecutorRunID) {
		return Request{}, invalid(
			"release request executor run",
			"an empty value or a positive GitHub Actions run identifier",
			"record only the correlated protected-line executor run",
		)
	}
	if input.ExpiresAt.IsZero() {
		return Request{}, invalid(
			"release request expiry",
			"a non-zero authorization expiry",
			"bind the request to an explicit authorization expiry",
		)
	}
	if !idempotencyKeyPattern.MatchString(input.IdempotencyKey) {
		return Request{}, invalid(
			"release request idempotency key",
			"a lowercase 64-character SHA-256 value",
			"derive one deterministic idempotency key from the bound release request",
		)
	}
	if input.DeploymentID < 0 {
		return Request{}, invalid(
			"release request deployment ID",
			"a non-negative provider deployment identifier",
			"record the durable provider identifier after request persistence",
		)
	}
	if err := validateOperationBinding(input); err != nil {
		return Request{}, err
	}
	if !validState(input.State) {
		return Request{}, invalid(
			"release request state",
			"an authorized protected-line request lifecycle state",
			"persist only a recognized request, execution, or finalization state",
		)
	}
	return Request{
		id:               input.ID,
		repository:       input.Repository,
		operation:        input.Operation,
		ticket:           input.Ticket,
		version:          input.Version,
		target:           input.Target,
		source:           input.Source,
		sourceSHA:        input.SourceSHA,
		requester:        input.Requester,
		expectedExecutor: input.ExpectedExecutor,
		parentRunID:      input.ParentRunID,
		executorRunID:    input.ExecutorRunID,
		expiresAt:        input.ExpiresAt.UTC(),
		idempotencyKey:   input.IdempotencyKey,
		deploymentID:     input.DeploymentID,
		state:            input.State,
	}, nil
}

// ID returns the durable request identifier.
func (request Request) ID() string {
	return request.id
}

// Repository returns the exact owner/repository binding.
func (request Request) Repository() string {
	return request.repository
}

// Operation returns the authorized protected-line operation.
func (request Request) Operation() Operation {
	return request.operation
}

// Ticket returns the scope-authorizing ticket.
func (request Request) Ticket() ticket.ID {
	return request.ticket
}

// Version returns the authorized release or support version.
func (request Request) Version() string {
	return request.version
}

// Target returns the one protected line that execution may create.
func (request Request) Target() branch.BranchName {
	return request.target
}

// Source returns the authorized source line.
func (request Request) Source() branch.BranchName {
	return request.source
}

// SourceSHA returns the immutable source revision.
func (request Request) SourceSHA() string {
	return request.sourceSHA
}

// Requester returns the actor that requested the release scope.
func (request Request) Requester() string {
	return request.requester
}

// ExpectedExecutor returns the only workflow allowed to execute the request.
func (request Request) ExpectedExecutor() string {
	return request.expectedExecutor
}

// ParentRunID returns the request-controller run identifier.
func (request Request) ParentRunID() string {
	return request.parentRunID
}

// ExecutorRunID returns the correlated executor run identifier, if known.
func (request Request) ExecutorRunID() string {
	return request.executorRunID
}

// ExpiresAt returns the authorization expiry in UTC.
func (request Request) ExpiresAt() time.Time {
	return request.expiresAt
}

// IdempotencyKey returns the deterministic key for equivalent requests.
func (request Request) IdempotencyKey() string {
	return request.idempotencyKey
}

// DeploymentID returns the durable provider record identifier.
func (request Request) DeploymentID() int64 {
	return request.deploymentID
}

// WithDeploymentID binds the provider-assigned durable record identifier after
// the provider has already validated that the identifier is positive.
func (request Request) WithDeploymentID(deploymentID int64) Request {
	request.deploymentID = deploymentID
	return request
}

// State returns the latest durable request lifecycle state.
func (request Request) State() State {
	return request.state
}

// Expired reports whether the request may no longer be executed.
func (request Request) Expired(now time.Time) bool {
	return !request.expiresAt.After(now)
}

// CanExecute reports whether the request is still authorized for its one
// protected-line mutation.
func (request Request) CanExecute(now time.Time) bool {
	return request.state == StateAwaitingExecutionApproval && !request.Expired(now)
}

// CanRecover reports whether a read-only finalizer may revisit the request.
func (request Request) CanRecover() bool {
	return request.state == StateVerificationPending
}

// WithExecutionRun binds a request to one executor run before it transitions
// into execution.
func (request Request) WithExecutionRun(runID string, now time.Time) (Request, error) {
	if !request.CanExecute(now) {
		return Request{}, invalid(
			"release request state",
			"an unexpired request awaiting execution approval",
			"create a new request or use only the read-only recovery finalizer",
		)
	}
	if !runIDPattern.MatchString(runID) {
		return Request{}, invalid(
			"release request executor run",
			"a positive GitHub Actions run identifier",
			"bind execution to the correlated protected-line workflow run",
		)
	}
	request.executorRunID = runID
	request.state = StateExecuting
	return request, nil
}

// Transition applies a permitted durable state transition. It never changes
// the request's scope, source, target, or identity bindings.
func (request Request) Transition(next State, now time.Time) (Request, error) {
	if !validState(next) || !allowedTransition(request.state, next, request.Expired(now)) {
		return Request{}, invalid(
			"release request transition",
			"a permitted protected-line request state transition",
			"preserve the immutable request binding and use the matching controller phase",
		)
	}
	request.state = next
	return request, nil
}

func validateOperationBinding(input Input) error {
	switch input.Operation {
	case OperationRelease:
		version, err := branch.ParseSemanticVersion(input.Version)
		if err != nil {
			return err
		}
		// ParseSemanticVersion has already established the only precondition
		// NewReleaseBranch can reject, so this construction is total here.
		expected, _ := branch.NewReleaseBranch(version)
		if input.Source.Family() != branch.FamilyDevelop || input.Target.String() != expected.String() {
			return invalid(
				"release request binding",
				"develop -> release/<semantic-version>",
				"bind a release request to the exact develop source and version-derived release target",
			)
		}
	case OperationSupport:
		version, err := branch.ParseSupportVersion(input.Version)
		if err != nil {
			return err
		}
		// ParseSupportVersion has already established the only precondition
		// NewSupportBranch can reject, so this construction is total here.
		expected, _ := branch.NewSupportBranch(version)
		if input.Source.Family() != branch.FamilyMain || input.Target.String() != expected.String() {
			return invalid(
				"support request binding",
				"main -> support/<major.minor>",
				"bind a support request to the exact main source and version-derived support target",
			)
		}
	default:
		return invalid(
			"release request operation",
			"release or support",
			"select the protected-line lifecycle operation before authorization",
		)
	}
	return nil
}

func validRepository(value string) bool {
	if !validSingleLine(value, 200) || strings.Count(value, "/") != 1 {
		return false
	}
	owner, name, _ := strings.Cut(value, "/")
	return owner != "" && name != ""
}

func validSingleLine(value string, limit int) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && trimmed == value && len(value) <= limit && !strings.ContainsAny(value, "\r\n\t")
}

func validState(state State) bool {
	switch state {
	case StateRequested, StateRequestAuthorized, StateAwaitingExecutionApproval, StateExecuting, StateVerified, StateFailed, StateRejected, StateExpired, StateVerificationPending:
		return true
	default:
		return false
	}
}

func allowedTransition(current, next State, expired bool) bool {
	if current == next {
		return next == StateVerified || next == StateFailed || next == StateRejected || next == StateExpired
	}
	if expired {
		return next == StateExpired
	}
	switch current {
	case StateRequested:
		return next == StateRequestAuthorized || next == StateRejected || next == StateFailed || next == StateExpired
	case StateRequestAuthorized:
		return next == StateAwaitingExecutionApproval || next == StateRejected || next == StateFailed || next == StateExpired
	case StateAwaitingExecutionApproval:
		return next == StateExecuting || next == StateRejected || next == StateExpired || next == StateFailed || next == StateVerificationPending
	case StateExecuting:
		return next == StateVerified || next == StateFailed || next == StateVerificationPending
	case StateVerificationPending:
		return next == StateVerified || next == StateFailed || next == StateExpired
	default:
		return false
	}
}

func invalid(field, expected, remediation string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeInvalidInput,
		Category:    problem.CategoryGovernance,
		Field:       field,
		Expected:    expected,
		Rule:        "protected-line execution requires an immutable authorized release request",
		Remediation: remediation,
	})
}
