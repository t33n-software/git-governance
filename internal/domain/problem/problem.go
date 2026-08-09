// Package problem defines the stable failure contract shared by the CLI,
// application services, adapters, hooks, and automation consumers.
package problem

import (
	"errors"
	"fmt"
	"strings"
)

// Category classifies a failure for exit-code selection and automation.
type Category string

const (
	CategoryUsage      Category = "usage"
	CategoryGovernance Category = "governance"
	CategoryRepository Category = "repository"
	CategoryGit        Category = "git"
	CategoryConfig     Category = "config"
	CategoryPolicy     Category = "policy"
	CategoryExternal   Category = "external"
	CategoryCancelled  Category = "cancelled"
	CategoryInternal   Category = "internal"
)

// Code is a stable machine-readable failure identifier.
type Code string

const (
	CodeInvalidInput                 Code = "INVALID_INPUT"
	CodeTicketKeyInvalid             Code = "TICKET_KEY_INVALID"
	CodeTicketNumberInvalid          Code = "TICKET_NUMBER_INVALID"
	CodeTicketIDInvalid              Code = "TICKET_ID_INVALID"
	CodeBranchFamilyInvalid          Code = "BRANCH_FAMILY_INVALID"
	CodeBranchSlugInvalid            Code = "BRANCH_SLUG_INVALID"
	CodeBranchNameInvalid            Code = "BRANCH_NAME_INVALID"
	CodeBranchRefInvalid             Code = "BRANCH_REF_INVALID"
	CodeBranchBaseInvalid            Code = "BRANCH_BASE_INVALID"
	CodeBranchAlreadyExists          Code = "BRANCH_ALREADY_EXISTS"
	CodeTicketBranchAlreadyExists    Code = "TICKET_BRANCH_ALREADY_EXISTS"
	CodeBranchPublicationUnknown     Code = "BRANCH_PUBLICATION_UNKNOWN"
	CodeScratchSourceBranchMissing   Code = "SCRATCH_SOURCE_BRANCH_MISSING"
	CodeScratchTargetBranchMissing   Code = "SCRATCH_TARGET_BRANCH_MISSING"
	CodeScratchTargetBranchAmbiguous Code = "SCRATCH_TARGET_BRANCH_AMBIGUOUS"
	CodeScratchMergeEmpty            Code = "SCRATCH_MERGE_EMPTY"
	CodeScratchMergeConflict         Code = "SCRATCH_MERGE_CONFLICT"
	CodeSharedLineMutationForbidden  Code = "SHARED_LINE_MUTATION_FORBIDDEN"
	CodeRebaseNotRequired            Code = "REBASE_NOT_REQUIRED"
	CodeRebaseConflict               Code = "REBASE_CONFLICT"
	CodeMergeConflict                Code = "MERGE_CONFLICT"
	CodeCherryPickConflict           Code = "CHERRY_PICK_CONFLICT"
	CodeRebaseAfterPublishForbidden  Code = "REBASE_AFTER_PUBLISH_FORBIDDEN"
	CodeForcePushForbidden           Code = "FORCE_PUSH_FORBIDDEN"
	CodeCommitTypeInvalid            Code = "COMMIT_TYPE_INVALID"
	CodeCommitHeaderInvalid          Code = "COMMIT_HEADER_INVALID"
	CodeCommitDescriptionInvalid     Code = "COMMIT_DESCRIPTION_INVALID"
	CodeCommitTicketMismatch         Code = "COMMIT_TICKET_MISMATCH"
	CodeBreakingChangeInvalid        Code = "BREAKING_CHANGE_INVALID"
	CodeWorktreeNotClean             Code = "WORKTREE_NOT_CLEAN"
	CodeRepositoryNotFound           Code = "REPOSITORY_NOT_FOUND"
	CodeRepositoryHasNoCommits       Code = "REPOSITORY_HAS_NO_COMMITS"
	CodeGitCommandFailed             Code = "GIT_COMMAND_FAILED"
	CodePolicyBundleMissing          Code = "POLICY_BUNDLE_MISSING"
	CodePolicyBundleStale            Code = "POLICY_BUNDLE_STALE"
	CodeConfigurationInvalid         Code = "CONFIGURATION_INVALID"
	CodeConfigurationUnavailable     Code = "CONFIGURATION_UNAVAILABLE"
	CodeExternalCommandFailed        Code = "EXTERNAL_COMMAND_FAILED"
	CodeOperationCancelled           Code = "OPERATION_CANCELLED"
	CodeInternal                     Code = "INTERNAL"
)

const (
	ExitSuccess    = 0
	ExitInternal   = 1
	ExitUsage      = 2
	ExitGovernance = 3
	ExitRepository = 4
	ExitGit        = 5
	ExitConfig     = 6
	ExitExternal   = 7
	ExitCancelled  = 130
)

// Details describes an actionable product failure. Actual is omitted from
// delivery output when SensitiveActual is true.
type Details struct {
	Code                Code
	Category            Category
	Field               string
	Actual              string
	Context             string
	Diagnostic          string
	Expected            string
	Rule                string
	Example             string
	Remediation         string
	SensitiveActual     bool
	SensitiveDiagnostic bool
	WorkflowInputs      []WorkflowInput
}

// WorkflowInput records one accepted value used by an interactive workflow
// failure report. Sensitive values are retained only as a redaction marker.
type WorkflowInput struct {
	Field     string
	Value     string
	Sensitive bool
}

// Problem is a typed error that preserves a causal error without exposing it
// as part of the stable product contract.
type Problem struct {
	Details
	cause error
}

// New creates an actionable problem without an underlying cause.
func New(details Details) *Problem {
	return &Problem{Details: normalize(details)}
}

// Wrap creates an actionable problem that retains an underlying cause for
// diagnostics and errors.Is/errors.As callers.
func Wrap(details Details, cause error) *Problem {
	return &Problem{
		Details: normalize(details),
		cause:   cause,
	}
}

// Error implements error without leaking sensitive values or opaque adapter
// diagnostics. Callers can still inspect the preserved cause through
// errors.Is, errors.As, or Unwrap.
func (p *Problem) Error() string {
	if p == nil {
		return ""
	}

	parts := []string{string(p.Code)}
	if p.Field != "" {
		parts = append(parts, fmt.Sprintf("field %q", p.Field))
	}
	if p.Rule != "" {
		parts = append(parts, p.Rule)
	} else if p.Expected != "" {
		parts = append(parts, fmt.Sprintf("expected %s", p.Expected))
	}
	return strings.Join(parts, ": ")
}

// Unwrap returns the causal error, if there is one.
func (p *Problem) Unwrap() error {
	if p == nil {
		return nil
	}
	return p.cause
}

// ExitCode maps an error to the stable CLI process contract.
func ExitCode(err error) int {
	if err == nil {
		return ExitSuccess
	}

	var typed *Problem
	if errors.As(err, &typed) {
		switch typed.Category {
		case CategoryUsage:
			return ExitUsage
		case CategoryGovernance, CategoryPolicy:
			return ExitGovernance
		case CategoryRepository:
			return ExitRepository
		case CategoryGit:
			return ExitGit
		case CategoryConfig:
			return ExitConfig
		case CategoryExternal:
			return ExitExternal
		case CategoryCancelled:
			return ExitCancelled
		default:
			return ExitInternal
		}
	}
	return ExitInternal
}

// As returns the typed problem carried by err, if any.
func As(err error) (*Problem, bool) {
	var typed *Problem
	if !errors.As(err, &typed) {
		return nil, false
	}
	return typed, true
}

// WithWorkflowInputs adds an ordered, defensive copy of accepted workflow
// inputs to a classified failure while preserving its causal error chain.
func WithWorkflowInputs(err error, inputs []WorkflowInput) error {
	if err == nil || len(inputs) == 0 {
		return err
	}
	typed, ok := As(err)
	if !ok {
		return Wrap(Details{
			Code:           CodeInternal,
			Category:       CategoryInternal,
			Field:          "workflow operation",
			Expected:       "a classified workflow failure",
			Rule:           "workflow failures must retain their accepted input summary",
			Remediation:    "review the diagnostic, correct the workflow state, and retry",
			WorkflowInputs: append([]WorkflowInput(nil), inputs...),
		}, err)
	}
	details := typed.Details
	details.WorkflowInputs = append([]WorkflowInput(nil), inputs...)
	return Wrap(details, err)
}

func normalize(details Details) Details {
	if details.Code == "" {
		details.Code = CodeInternal
	}
	if details.Category == "" {
		details.Category = CategoryInternal
	}
	return details
}
