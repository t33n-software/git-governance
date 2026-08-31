package policy

import (
	"context"
	"errors"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/cliparam"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
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

type doctorGit struct {
	discoverErr      error
	versionErr       error
	commits          bool
	commitErr        error
	remoteErr        error
	activeName       string
	active           bool
	activeErr        error
	authErr          error
	signingConfig    port.SigningConfiguration
	signingConfigErr error
	signingProofErr  error
}

type doctorTools struct {
	operatingSystem string
	architecture    string
	version         string
	exists          bool
	err             error
	versionErr      error
	fileErr         error
}

type doctorPolicy struct {
	status port.PolicyStatus
	err    error
}

func (policy doctorPolicy) Status(context.Context, port.RepositoryIdentity) (port.PolicyStatus, error) {
	return policy.status, policy.err
}

func (tools doctorTools) Platform() (string, string) {
	if tools.operatingSystem != "" || tools.architecture != "" {
		return tools.operatingSystem, tools.architecture
	}
	return "windows", "amd64"
}

func (tools doctorTools) Version(context.Context, string) (string, error) {
	if tools.versionErr != nil {
		return "", tools.versionErr
	}
	if tools.err != nil {
		return "", tools.err
	}
	return tools.version, nil
}

func (tools doctorTools) FileExists(string) (bool, error) {
	if tools.fileErr != nil {
		return false, tools.fileErr
	}
	if tools.err != nil {
		return false, tools.err
	}
	return tools.exists, nil
}

func (git doctorGit) Discover(context.Context, string) (port.RepositoryIdentity, error) {
	if git.discoverErr != nil {
		return port.RepositoryIdentity{}, git.discoverErr
	}
	return port.RepositoryIdentity{Root: "C:/repo", Remote: "origin"}, nil
}

func (git doctorGit) Version(context.Context) (string, error) {
	if git.versionErr != nil {
		return "", git.versionErr
	}
	return "git version test", nil
}

func (git doctorGit) RemoteURL(context.Context, port.RepositoryIdentity) (string, error) {
	if git.remoteErr != nil {
		return "", git.remoteErr
	}
	return "https://example.invalid/repo.git", nil
}

func (git doctorGit) ActiveOperation(context.Context, port.RepositoryIdentity) (string, bool, error) {
	return git.activeName, git.active, git.activeErr
}

func (git doctorGit) CheckTransportAuthentication(context.Context, port.RepositoryIdentity) error {
	return git.authErr
}

func (git doctorGit) SigningConfiguration(context.Context, port.RepositoryIdentity) (port.SigningConfiguration, error) {
	return git.signingConfig, git.signingConfigErr
}

func (git doctorGit) ProveSigningCapability(context.Context, port.RepositoryIdentity, port.SigningConfiguration) error {
	return git.signingProofErr
}

func validSigningConfiguration() port.SigningConfiguration {
	return port.SigningConfiguration{
		SigningEnabled:         true,
		Format:                 "ssh",
		SigningKey:             "C:/keys/signing.key",
		SigningKeyReadable:     true,
		UserEmail:              "lane@example.invalid",
		AllowedSignersFile:     "C:/keys/allowed_signers",
		AllowedSignersReadable: true,
	}
}

func (git doctorGit) HasCommits(context.Context, port.RepositoryIdentity) (bool, error) {
	return git.commits, git.commitErr
}

func (doctorGit) IsWorktreeClean(context.Context, port.RepositoryIdentity) (bool, error) {
	return true, nil
}

func (doctorGit) CurrentBranch(context.Context, port.RepositoryIdentity) (branch.BranchName, error) {
	return branch.ParseName("feature/ABC-123-add-export")
}

func (doctorGit) ValidateBranchRef(context.Context, port.RepositoryIdentity, branch.BranchName) error {
	return nil
}

func (doctorGit) BranchExists(context.Context, port.RepositoryIdentity, branch.BranchName) (bool, error) {
	return false, nil
}

func (doctorGit) OfficialBranchesForTicket(context.Context, port.RepositoryIdentity, ticket.ID) ([]branch.BranchName, error) {
	return nil, nil
}

func (doctorGit) Fetch(context.Context, port.RepositoryIdentity) error {
	return nil
}

func (doctorGit) TargetBaseExists(context.Context, port.RepositoryIdentity, branch.TargetBase) (bool, error) {
	return true, nil
}

func (doctorGit) CreateBranch(context.Context, port.RepositoryIdentity, branch.BranchName, branch.TargetBase, bool) error {
	return nil
}

func (doctorGit) StoreWorkflowBase(context.Context, port.RepositoryIdentity, branch.BranchName, branch.TargetBase) error {
	return nil
}

func (doctorGit) ClearWorkflowBase(context.Context, port.RepositoryIdentity, branch.BranchName) error {
	return nil
}

func (doctorGit) WorkflowBase(context.Context, port.RepositoryIdentity, branch.BranchName) (branch.TargetBase, bool, error) {
	return branch.TargetBase{}, false, nil
}

func (doctorGit) SwitchBranch(context.Context, port.RepositoryIdentity, branch.BranchName) error {
	return nil
}

func (doctorGit) PublicationState(context.Context, port.RepositoryIdentity, branch.BranchName) (branch.PublicationState, error) {
	return branch.PublicationUnpublished, nil
}

func (doctorGit) HasMissingBaseCommits(context.Context, port.RepositoryIdentity, branch.TargetBase) (bool, error) {
	return false, nil
}

func (doctorGit) CommitMessagesSince(context.Context, port.RepositoryIdentity, branch.TargetBase) ([]string, error) {
	return nil, nil
}

func (doctorGit) Rebase(context.Context, port.RepositoryIdentity, branch.TargetBase) error {
	return nil
}

func (doctorGit) ContinueRebase(context.Context, port.RepositoryIdentity) error {
	return nil
}

func (doctorGit) Merge(context.Context, port.RepositoryIdentity, branch.TargetBase, commitmsg.Message) error {
	return nil
}

func (doctorGit) SquashMerge(context.Context, port.RepositoryIdentity, branch.BranchName) error {
	return nil
}

func (doctorGit) CherryPick(context.Context, port.RepositoryIdentity, string) error {
	return nil
}

func (doctorGit) DeleteLocalBranch(context.Context, port.RepositoryIdentity, branch.BranchName, bool) error {
	return nil
}

func (doctorGit) ReleaseTagsAt(context.Context, port.RepositoryIdentity, string) ([]string, error) {
	return nil, nil
}

func (doctorGit) HasUnmergedConflicts(context.Context, port.RepositoryIdentity) (bool, error) {
	return false, nil
}

func (doctorGit) HasStagedChanges(context.Context, port.RepositoryIdentity) (bool, error) {
	return false, nil
}

func (doctorGit) Stage(context.Context, port.RepositoryIdentity, []string) error {
	return nil
}

func (doctorGit) Commit(context.Context, port.RepositoryIdentity, commitmsg.Message) error {
	return nil
}

func (doctorGit) Push(context.Context, port.RepositoryIdentity, branch.BranchName, bool) error {
	return nil
}

func (doctorGit) InspectPushUpdate(context.Context, port.RepositoryIdentity, branch.TargetBase, string, string) (port.PushUpdateInspection, error) {
	return port.PushUpdateInspection{}, nil
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

func TestDoctorIsReadOnly(t *testing.T) {
	t.Parallel()

	store := &memoryStore{preferences: port.Preferences{SchemaVersion: 1}}
	tools := doctorTools{version: "lefthook 2.1.8", exists: true}
	result, err := NewDoctorServiceWithDependencies(doctorGit{commits: true, signingConfig: validSigningConfiguration()}, store, SyntaxOnlyKeyPolicy{}, tools).Run(context.Background(), "C:/repo")
	if err != nil {
		t.Fatal(err)
	}
	if result.Repository.Root != "C:/repo" || len(result.Checks) != 13 {
		t.Fatalf("Doctor.Run() = %#v", result)
	}
	for _, check := range result.Checks {
		if !check.OK {
			t.Fatalf("unexpected failed doctor check: %#v", check)
		}
	}

	result, err = NewDoctorServiceWithDependencies(doctorGit{discoverErr: errors.New("not a repository")}, store, SyntaxOnlyKeyPolicy{}, tools).Run(context.Background(), "C:/missing")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Checks) == 0 || result.Checks[1].OK {
		t.Fatalf("failed discovery doctor result = %#v", result)
	}
}

func TestDoctorStopsBeforeInspectionWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewDoctorService(nil, nil).Run(ctx, "C:/repo")
	assertProblemCode(t, err, problem.CodeOperationCancelled)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Doctor.Run() error = %v, want context cancellation", err)
	}
}

func TestDoctorWhiteboxDiagnostics(t *testing.T) {
	t.Parallel()

	t.Run("minimal constructor reports absent optional dependencies", func(t *testing.T) {
		result, err := NewDoctorService(nil, nil).Run(context.Background(), "C:/repo")
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Checks) != 9 {
			t.Fatalf("minimal doctor checks = %#v", result.Checks)
		}
		for _, check := range result.Checks {
			if check.OK {
				t.Fatalf("unconfigured dependency unexpectedly passed: %#v", check)
			}
		}
	})

	t.Run("repository check failures are reported without aborting", func(t *testing.T) {
		store := &memoryStore{loadErr: errors.New("config unavailable")}
		tools := doctorTools{
			operatingSystem: "linux",
			versionErr:      errors.New("lefthook missing"),
			fileErr:         errors.New("stat failed"),
		}
		result, err := NewDoctorServiceWithDependencies(
			doctorGit{
				versionErr: errors.New("git unavailable"),
				commits:    false,
				remoteErr:  errors.New("remote missing"),
				activeErr:  errors.New("operation unknown"),
				authErr:    errors.New("authentication missing"),
			},
			store,
			doctorPolicy{err: errors.New("policy unavailable")},
			tools,
		).Run(context.Background(), "C:/repo")
		if err != nil {
			t.Fatal(err)
		}
		if result.Repository.Root == "" || len(result.Checks) != 13 {
			t.Fatalf("doctor failure result = %#v", result)
		}
		for _, name := range []string{
			"Git version",
			"repository history",
			"selected remote",
			"Git operation state",
			"Git authentication",
			"Commit signing configuration",
			"Commit signing proof",
			"runtime platform",
			"Lefthook executable",
			"Lefthook configuration",
			"local policy",
			"user configuration",
		} {
			if checkByName(t, result.Checks, name).OK {
				t.Fatalf("failure fixture unexpectedly passed %q: %#v", name, result.Checks)
			}
		}
	})

	t.Run("repository history error is distinct from an empty repository", func(t *testing.T) {
		historyErr := errors.New("history failed")
		result, err := NewDoctorServiceWithDependencies(
			doctorGit{commitErr: historyErr},
			&memoryStore{preferences: port.Preferences{SchemaVersion: schemaVersion}},
			nil,
			nil,
		).Run(context.Background(), "C:/repo")
		if err != nil {
			t.Fatal(err)
		}
		check := checkByName(t, result.Checks, "repository history")
		if check.OK || check.Detail != historyErr.Error() {
			t.Fatalf("history check = %#v", check)
		}
	})

	t.Run("active operation, missing config, and policy status are explicit", func(t *testing.T) {
		store := &memoryStore{preferences: port.Preferences{SchemaVersion: schemaVersion}}
		result, err := NewDoctorServiceWithDependencies(
			doctorGit{commits: true, active: true, activeName: "rebase"},
			store,
			doctorPolicy{status: port.PolicyStatus{Mode: "syntax-only", Detail: "syntax-only policy"}},
			doctorTools{operatingSystem: "linux", architecture: "", version: "lefthook 2", exists: false},
		).Run(context.Background(), "C:/repo")
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Checks) != 13 {
			t.Fatalf("doctor checks = %#v", result.Checks)
		}
		if checkByName(t, result.Checks, "Git operation state").OK ||
			checkByName(t, result.Checks, "runtime platform").OK ||
			checkByName(t, result.Checks, "Lefthook configuration").OK {
			t.Fatalf("expected active operation/platform/configuration failures: %#v", result.Checks)
		}
	})

	t.Run("Git authentication is required and fails closed", func(t *testing.T) {
		authErr := errors.New("Git credentials unavailable")
		result, err := NewDoctorServiceWithDependencies(
			doctorGit{commits: true, authErr: authErr},
			&memoryStore{preferences: port.Preferences{SchemaVersion: schemaVersion}},
			nil,
			nil,
		).Run(context.Background(), "C:/repo")
		if err != nil {
			t.Fatal(err)
		}
		check := checkByName(t, result.Checks, "Git authentication")
		if check.OK || !errors.Is(result.AuthenticationError(), authErr) {
			t.Fatalf("authentication check = %#v, error = %v", check, result.AuthenticationError())
		}

		withoutInspector := doctorGitWithoutAuth{GitRepository: doctorGit{commits: true}}
		result, err = NewDoctorServiceWithDependencies(
			withoutInspector,
			&memoryStore{preferences: port.Preferences{SchemaVersion: schemaVersion}},
			nil,
			nil,
		).Run(context.Background(), "C:/repo")
		if err != nil {
			t.Fatal(err)
		}
		check = checkByName(t, result.Checks, "Git authentication")
		if check.OK || result.AuthenticationError() == nil {
			t.Fatalf("missing-inspector authentication check = %#v, error = %v", check, result.AuthenticationError())
		}
	})

	t.Run("uses the explicitly selected remote for Git authentication", func(t *testing.T) {
		git := &remoteDoctorGit{doctorGit: doctorGit{commits: true}}
		result, err := NewDoctorServiceWithDependencies(
			git,
			&memoryStore{preferences: port.Preferences{SchemaVersion: schemaVersion}},
			nil,
			nil,
		).RunForRemote(context.Background(), "C:/repo", "upstream")
		if err != nil {
			t.Fatal(err)
		}
		if result.Repository.Remote != "upstream" || git.authenticatedRemote != "upstream" {
			t.Fatalf("selected remote = result %#v, Git remote %q", result.Repository, git.authenticatedRemote)
		}
	})

	t.Run("detail helpers cover both values", func(t *testing.T) {
		key := mustKey(t, "ABC")
		if got := configurationDetail(port.Preferences{}); got == "" {
			t.Fatal("empty preference detail")
		}
		if got := configurationDetail(port.Preferences{DefaultKey: &key}); got == "" {
			t.Fatal("default preference detail")
		}
		if got := lefthookConfigurationDetail(true); got != "lefthook.yml is present" {
			t.Fatalf("present detail = %q", got)
		}
		if got := lefthookConfigurationDetail(false); got != "lefthook.yml is not present" {
			t.Fatalf("absent detail = %q", got)
		}
	})
}

type doctorGitWithoutAuth struct {
	port.GitRepository
}

type remoteDoctorGit struct {
	doctorGit
	authenticatedRemote string
}

func (git *remoteDoctorGit) CheckTransportAuthentication(_ context.Context, repository port.RepositoryIdentity) error {
	git.authenticatedRemote = repository.Remote
	return git.authErr
}

func checkByName(t *testing.T, checks []Check, name string) Check {
	t.Helper()
	for _, check := range checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("missing doctor check %q in %#v", name, checks)
	return Check{}
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

var _ port.GitRepository = doctorGit{}
var _ port.GitTransportAuthenticator = doctorGit{}
var _ port.PreferencesStore = (*memoryStore)(nil)
var _ port.ToolInspector = doctorTools{}
