package policy

import (
	"context"
	"errors"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/cliparam"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

type memoryStore struct {
	preferences port.Preferences
	err         error
	loadErr     error
	saveErr     error
}

func (store *memoryStore) Load(context.Context) (port.Preferences, error) {
	if store.loadErr != nil {
		return port.Preferences{}, store.loadErr
	}
	return store.preferences, store.err
}

func (store *memoryStore) Save(_ context.Context, preferences port.Preferences) error {
	if store.saveErr != nil {
		return store.saveErr
	}
	if store.err != nil {
		return store.err
	}
	store.preferences = preferences
	return nil
}

func TestSyntaxOnlyKeyPolicy(t *testing.T) {
	t.Parallel()

	key := mustKey(t, "PLATFORM2")
	if err := (SyntaxOnlyKeyPolicy{}).ValidateKey(context.Background(), port.RepositoryIdentity{}, key); err != nil {
		t.Fatal(err)
	}
	if err := (SyntaxOnlyKeyPolicy{}).ValidateKey(context.Background(), port.RepositoryIdentity{}, ticket.Key{}); err == nil {
		t.Fatal("zero ticket key was accepted")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := (SyntaxOnlyKeyPolicy{}).ValidateKey(ctx, port.RepositoryIdentity{}, key)
	assertProblemCode(t, err, problem.CodeOperationCancelled)

	_, err = (SyntaxOnlyKeyPolicy{}).Status(ctx, port.RepositoryIdentity{})
	assertProblemCode(t, err, problem.CodeOperationCancelled)

	status, err := (SyntaxOnlyKeyPolicy{}).Status(context.Background(), port.RepositoryIdentity{})
	if err != nil || status.Mode != "syntax-only" || status.BundlePresent || status.BundleFresh {
		t.Fatalf("Status() = (%#v, %v)", status, err)
	}
}

func TestDescriptionIsSelfContained(t *testing.T) {
	t.Parallel()

	actual := Describe()
	if actual.SchemaVersion != schemaVersion || actual.KeyPolicy != "syntax-only" || actual.CommitSigning != CommitSigningRequired {
		t.Fatalf("Describe() = %#v", actual)
	}
	if len(actual.BranchFamilies) != 13 || len(actual.CommitTypes) != 11 {
		t.Fatalf("Describe() counts = %d families, %d types", len(actual.BranchFamilies), len(actual.CommitTypes))
	}
	if actual.Limits.TicketKeyMaximumLength != 32 || actual.Limits.CommitSubjectMaximumRunes != 200 {
		t.Fatalf("Describe() limits = %#v", actual.Limits)
	}

	registerTypes := cliparam.CommitType().Values
	if len(actual.CommitTypes) != len(registerTypes) {
		t.Fatalf("Describe() commit types = %v, want the register values %v", actual.CommitTypes, registerTypes)
	}
	for index, value := range registerTypes {
		if actual.CommitTypes[index] != value {
			t.Fatalf("Describe() commitTypes[%d] = %q, want %q", index, actual.CommitTypes[index], value)
		}
	}

	metadata := cliparam.CommitTypeMetadata()
	if len(actual.CommitFamilies) != len(metadata) {
		t.Fatalf("Describe() commitFamilies count = %d, want %d", len(actual.CommitFamilies), len(metadata))
	}
	for index, family := range metadata {
		actual_family := actual.CommitFamilies[index]
		if actual_family.Type != family.Type.String() || actual_family.Label != family.Label || actual_family.Description != family.Description {
			t.Fatalf("Describe() commitFamilies[%d] = %#v, want the canonical surface %+v", index, actual_family, family)
		}
	}
}

func TestPreferencesService(t *testing.T) {
	t.Parallel()

	store := &memoryStore{preferences: port.Preferences{SchemaVersion: 1}}
	service := NewPreferencesService(store)
	abc := mustKey(t, "ABC")
	platform := mustKey(t, "PLATFORM2")

	actual, err := service.AddKey(context.Background(), abc)
	if err != nil || len(actual.KnownKeys) != 1 {
		t.Fatalf("AddKey() = (%#v, %v)", actual, err)
	}
	actual, err = service.AddKey(context.Background(), abc)
	if err != nil || len(actual.KnownKeys) != 1 {
		t.Fatalf("duplicate AddKey() = (%#v, %v)", actual, err)
	}
	actual, err = service.SetDefaultKey(context.Background(), platform)
	if err != nil || actual.DefaultKey == nil || actual.DefaultKey.String() != "PLATFORM2" || len(actual.KnownKeys) != 2 {
		t.Fatalf("SetDefaultKey() = (%#v, %v)", actual, err)
	}
	actual, err = service.RemoveKey(context.Background(), platform)
	if err != nil || actual.DefaultKey != nil || len(actual.KnownKeys) != 1 {
		t.Fatalf("RemoveKey() = (%#v, %v)", actual, err)
	}
	_, err = service.RemoveKey(context.Background(), platform)
	assertProblemCode(t, err, problem.CodeInvalidInput)
}

func TestPreferencesServiceWhiteboxErrorPaths(t *testing.T) {
	t.Parallel()

	key := mustKey(t, "ABC")
	t.Run("missing store", func(t *testing.T) {
		_, err := NewPreferencesService(nil).List(context.Background())
		assertProblemCode(t, err, problem.CodeInternal)
	})

	t.Run("load errors propagate through every mutation", func(t *testing.T) {
		expected := errors.New("load failed")
		store := &memoryStore{loadErr: expected}
		service := NewPreferencesService(store)
		for _, operation := range []func() error{
			func() error {
				_, err := service.List(context.Background())
				return err
			},
			func() error {
				_, err := service.AddKey(context.Background(), key)
				return err
			},
			func() error {
				_, err := service.RemoveKey(context.Background(), key)
				return err
			},
			func() error {
				_, err := service.SetDefaultKey(context.Background(), key)
				return err
			},
		} {
			if err := operation(); !errors.Is(err, expected) {
				t.Fatalf("operation error = %v, want %v", err, expected)
			}
		}
	})

	t.Run("save errors propagate from every mutation", func(t *testing.T) {
		expected := errors.New("save failed")
		preferences := port.Preferences{SchemaVersion: schemaVersion, KnownKeys: []ticket.Key{key}}
		for _, operation := range []func(*PreferencesService) error{
			func(service *PreferencesService) error {
				_, err := service.AddKey(context.Background(), key)
				return err
			},
			func(service *PreferencesService) error {
				_, err := service.RemoveKey(context.Background(), key)
				return err
			},
			func(service *PreferencesService) error {
				_, err := service.SetDefaultKey(context.Background(), key)
				return err
			},
		} {
			store := &memoryStore{preferences: preferences, saveErr: expected}
			if err := operation(NewPreferencesService(store)); !errors.Is(err, expected) {
				t.Fatalf("operation error = %v, want %v", err, expected)
			}
		}
	})

	t.Run("contains and missing dependency helpers are deterministic", func(t *testing.T) {
		if contains(nil, key) {
			t.Fatal("contains reported a key in an empty list")
		}
		if !contains([]ticket.Key{key}, key) {
			t.Fatal("contains did not find key")
		}
		if _, ok := problem.As(missingDependency("store")); !ok {
			t.Fatal("missing dependency is not a problem")
		}
	})
}

func mustKey(t *testing.T, raw string) ticket.Key {
	t.Helper()
	actual, err := ticket.ParseKey(raw)
	if err != nil {
		t.Fatal(err)
	}
	return actual
}

func assertProblemCode(t *testing.T, err error, expected problem.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected problem code %q, got nil", expected)
	}
	actual, ok := problem.As(err)
	if !ok {
		t.Fatalf("error %T does not carry a problem: %v", err, err)
	}
	if actual.Code != expected {
		t.Fatalf("problem code = %q, want %q", actual.Code, expected)
	}
}

var _ port.PreferencesStore = (*memoryStore)(nil)
