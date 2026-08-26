// Package policy contains local policy implementations and preference use
// cases. It deliberately does not perform network-based registry lookups.
package policy

import (
	"context"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

const schemaVersion = 1

// SyntaxOnlyKeyPolicy accepts every syntactically valid key. It is the
// deliberately limited v1 policy adapter until a verified local bundle is
// configured.
type SyntaxOnlyKeyPolicy struct{}

// ValidateKey confirms that a key remains syntactically valid.
func (SyntaxOnlyKeyPolicy) ValidateKey(ctx context.Context, _ port.RepositoryIdentity, key ticket.Key) error {
	if ctx != nil && ctx.Err() != nil {
		return problem.Wrap(problem.Details{
			Code:        problem.CodeOperationCancelled,
			Category:    problem.CategoryCancelled,
			Field:       "key policy",
			Expected:    "an active context",
			Rule:        "policy validation stops when the caller cancels its context",
			Remediation: "retry with an active context",
		}, ctx.Err())
	}
	_, err := ticket.ParseKey(key.String())
	return err
}

// Status reports the deliberate v1 limitation: validation is local syntax
// only, so no policy bundle is required or consulted.
func (SyntaxOnlyKeyPolicy) Status(ctx context.Context, _ port.RepositoryIdentity) (port.PolicyStatus, error) {
	if ctx != nil && ctx.Err() != nil {
		return port.PolicyStatus{}, problem.Wrap(problem.Details{
			Code:        problem.CodeOperationCancelled,
			Category:    problem.CategoryCancelled,
			Field:       "policy status",
			Expected:    "an active context",
			Rule:        "policy diagnostics stop when the caller cancels its context",
			Remediation: "retry with an active context",
		}, ctx.Err())
	}
	return port.PolicyStatus{
		Mode:          "syntax-only",
		BundlePresent: false,
		BundleFresh:   false,
		Detail:        "no policy bundle is configured; syntactically valid ticket keys are accepted",
	}, nil
}

// CommitSigningRequired is the local policy declaration for commit signing:
// every commit the CLI creates must carry a valid, trusted signature.
const CommitSigningRequired = "required"

// Description describes the active machine-readable local policy.
type Description struct {
	SchemaVersion  int                    `json:"schemaVersion"`
	KeyPolicy      string                 `json:"keyPolicy"`
	CommitSigning  string                 `json:"commitSigning"`
	BranchFamilies []branchapp.FamilyInfo `json:"branchFamilies"`
	CommitTypes    []string               `json:"commitTypes"`
	Limits         Limits                 `json:"limits"`
}

// Limits defines policy bounds for untrusted text inputs.
type Limits struct {
	TicketKeyMaximumLength    int `json:"ticketKeyMaximumLength"`
	TicketNumberMaximumLength int `json:"ticketNumberMaximumLength"`
	BranchSlugMaximumLength   int `json:"branchSlugMaximumLength"`
	CommitSubjectMaximumRunes int `json:"commitSubjectMaximumRunes"`
}

// Describe returns the stable local policy contract.
func Describe() Description {
	types := commitmsg.Types()
	commitTypes := make([]string, len(types))
	for index, kind := range types {
		commitTypes[index] = kind.String()
	}
	return Description{
		SchemaVersion:  schemaVersion,
		KeyPolicy:      "syntax-only",
		CommitSigning:  CommitSigningRequired,
		BranchFamilies: branchapp.ListFamilies(),
		CommitTypes:    commitTypes,
		Limits: Limits{
			TicketKeyMaximumLength:    32,
			TicketNumberMaximumLength: 18,
			BranchSlugMaximumLength:   100,
			CommitSubjectMaximumRunes: 200,
		},
	}
}

// PreferencesService manages user-scoped key preferences.
type PreferencesService struct {
	store port.PreferencesStore
}

// NewPreferencesService creates a preferences use case service.
func NewPreferencesService(store port.PreferencesStore) *PreferencesService {
	return &PreferencesService{store: store}
}

// List returns the current user preferences.
func (service *PreferencesService) List(ctx context.Context) (port.Preferences, error) {
	if service.store == nil {
		return port.Preferences{}, missingDependency("preferences store")
	}
	return service.store.Load(ctx)
}

// AddKey stores a known key once and preserves the existing default.
func (service *PreferencesService) AddKey(ctx context.Context, key ticket.Key) (port.Preferences, error) {
	preferences, err := service.List(ctx)
	if err != nil {
		return port.Preferences{}, err
	}
	if !contains(preferences.KnownKeys, key) {
		preferences.KnownKeys = append(preferences.KnownKeys, key)
	}
	if err := service.store.Save(ctx, preferences); err != nil {
		return port.Preferences{}, err
	}
	return service.store.Load(ctx)
}

// RemoveKey removes a known key and clears it if it was the default.
func (service *PreferencesService) RemoveKey(ctx context.Context, key ticket.Key) (port.Preferences, error) {
	preferences, err := service.List(ctx)
	if err != nil {
		return port.Preferences{}, err
	}
	if !contains(preferences.KnownKeys, key) {
		return port.Preferences{}, problem.New(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryUsage,
			Field:       "ticket key",
			Actual:      key.String(),
			Expected:    "a key stored in user preferences",
			Rule:        "only known keys can be removed",
			Example:     "ABC",
			Remediation: "list known keys before removing one",
		})
	}
	filtered := make([]ticket.Key, 0, len(preferences.KnownKeys)-1)
	for _, candidate := range preferences.KnownKeys {
		if candidate.String() != key.String() {
			filtered = append(filtered, candidate)
		}
	}
	preferences.KnownKeys = filtered
	if preferences.DefaultKey != nil && preferences.DefaultKey.String() == key.String() {
		preferences.DefaultKey = nil
	}
	if err := service.store.Save(ctx, preferences); err != nil {
		return port.Preferences{}, err
	}
	return service.store.Load(ctx)
}

// SetDefaultKey adds a key if necessary and marks it as the preferred
// interactive default.
func (service *PreferencesService) SetDefaultKey(ctx context.Context, key ticket.Key) (port.Preferences, error) {
	preferences, err := service.List(ctx)
	if err != nil {
		return port.Preferences{}, err
	}
	if !contains(preferences.KnownKeys, key) {
		preferences.KnownKeys = append(preferences.KnownKeys, key)
	}
	defaultKey := key
	preferences.DefaultKey = &defaultKey
	if err := service.store.Save(ctx, preferences); err != nil {
		return port.Preferences{}, err
	}
	return service.store.Load(ctx)
}

func contains(keys []ticket.Key, expected ticket.Key) bool {
	for _, key := range keys {
		if key.String() == expected.String() {
			return true
		}
	}
	return false
}

func missingDependency(name string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeInternal,
		Category:    problem.CategoryInternal,
		Field:       "dependency",
		Actual:      name,
		Expected:    "a configured application dependency",
		Rule:        "policy use cases require their configured ports",
		Remediation: "fix the composition root",
	})
}

var _ port.KeyPolicy = SyntaxOnlyKeyPolicy{}
var _ port.PolicyInspector = SyntaxOnlyKeyPolicy{}
