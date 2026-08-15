package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/hotfix"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

func TestHotfixValidateRecordCommand(t *testing.T) {
	t.Run("reports a valid main hotfix record", func(t *testing.T) {
		git := newCommandGit(t, "hotfix/ABC-999-payment-timeout", nil)
		store := &hotfixRecordCommandStore{record: commandHotfixRecord(t, "hotfix/ABC-999-payment-timeout")}
		runtime := commandRuntime(git)
		runtime.HotfixRecords = store
		command := NewWithRuntime(BuildInfo{Version: "test"}, runtime)

		output, err := executeBootstrapCommand(
			t,
			command,
			"--interactive", "never",
			"--output", "json",
			"workflow", "hotfix", "validate-record",
		)
		if err != nil || !strings.Contains(output, `"operation":"workflow.hotfix.validate-record"`) ||
			!strings.Contains(output, `"targetVersion":"1.0.2"`) {
			t.Fatalf("validate record = (%q, %v)", output, err)
		}
		if store.calls != 1 || store.ticket.String() != "ABC-999" || store.location != "" {
			t.Fatalf("record store = %#v", store)
		}
	})

	t.Run("rejects a non hotfix branch before record loading", func(t *testing.T) {
		store := &hotfixRecordCommandStore{}
		runtime := commandRuntime(newCommandGit(t, "feature/ABC-999-payment-timeout", nil))
		runtime.HotfixRecords = store
		command := NewWithRuntime(BuildInfo{Version: "test"}, runtime)

		_, err := executeBootstrapCommand(
			t,
			command,
			"--interactive", "never",
			"--output", "json",
			"workflow", "hotfix", "validate-record",
		)
		if err == nil {
			t.Fatal("validate record unexpectedly accepted a feature branch")
		}
		if store.calls != 0 {
			t.Fatalf("record store calls = %d, want 0", store.calls)
		}
	})

	t.Run("preserves safe record load failures", func(t *testing.T) {
		store := &hotfixRecordCommandStore{err: errors.New("record not available")}
		runtime := commandRuntime(newCommandGit(t, "hotfix/ABC-999-payment-timeout", nil))
		runtime.HotfixRecords = store
		command := NewWithRuntime(BuildInfo{Version: "test"}, runtime)

		output, err := executeBootstrapCommand(
			t,
			command,
			"--interactive", "never",
			"--output", "json",
			"workflow", "hotfix", "validate-record",
			"--record", ".git-governance/hotfix-release-records/ABC-999.json",
		)
		if err == nil || strings.Contains(output, "record not available") {
			t.Fatalf("validate record failure = (%q, %v)", output, err)
		}
		if store.location != ".git-governance/hotfix-release-records/ABC-999.json" {
			t.Fatalf("record location = %q", store.location)
		}
	})

	t.Run("reports verified merge and delivery evidence", func(t *testing.T) {
		store := &hotfixRecordCommandStore{record: commandHotfixRecord(t, "hotfix/ABC-999-payment-timeout")}
		publisher := &workflowRecordingPublisher{}
		runtime := commandRuntime(newCommandGit(t, "hotfix/ABC-999-payment-timeout", nil))
		runtime.HotfixRecords = store
		runtime.Publisher = publisher
		command := NewWithRuntime(BuildInfo{Version: "test"}, runtime)

		mergeOutput, err := executeBootstrapCommand(
			t,
			command,
			"--interactive", "never",
			"--output", "json",
			"workflow", "hotfix", "verify-merge",
		)
		if err != nil || !strings.Contains(mergeOutput, `"operation":"workflow.hotfix.verify-merge"`) ||
			!strings.Contains(mergeOutput, `"tag":"v1.0.2"`) {
			t.Fatalf("verify merge = (%q, %v)", mergeOutput, err)
		}

		deliveryOutput, err := executeBootstrapCommand(
			t,
			command,
			"--interactive", "never",
			"--output", "json",
			"workflow", "hotfix", "verify-delivery",
		)
		if err != nil || !strings.Contains(deliveryOutput, `"operation":"workflow.hotfix.verify-delivery"`) ||
			!strings.Contains(deliveryOutput, `"workflowRunURL":"https://example.invalid/actions/hotfix-delivery"`) {
			t.Fatalf("verify delivery = (%q, %v)", deliveryOutput, err)
		}
		if publisher.hotfixMergeCalls != 1 || publisher.hotfixDeliveryCalls != 1 {
			t.Fatalf("publisher calls = %#v", publisher)
		}
	})

	t.Run("prepares a manifest candidate without a publication flag", func(t *testing.T) {
		git := &manifestCommandGit{commandGit: newCommandGit(t, "hotfix/ABC-999-payment-timeout", nil)}
		runtime := commandRuntime(git)
		runtime.HotfixRecords = &hotfixRecordCommandStore{record: commandHotfixRecord(t, "hotfix/ABC-999-payment-timeout")}
		runtime.Quality = commandHotfixQuality{}
		command := NewWithRuntime(BuildInfo{Version: "test"}, runtime)

		output, err := executeBootstrapCommand(
			t,
			command,
			"--interactive", "never",
			"--output", "json",
			"--yes",
			"workflow", "hotfix", "propagate-manifest",
			"--source", "hotfix/ABC-999-payment-timeout",
			"--target-line", "develop",
		)
		if err != nil ||
			!strings.Contains(output, `"operation":"workflow.hotfix.propagate-manifest"`) ||
			!strings.Contains(output, `"pushed":"false"`) ||
			!strings.Contains(output, `"branch":"fix/ABC-999-propagate-to-develop"`) {
			t.Fatalf("propagate manifest = (%q, %v)", output, err)
		}

		_, err = executeBootstrapCommand(
			t,
			command,
			"--interactive", "never",
			"--output", "json",
			"--yes",
			"workflow", "hotfix", "propagate-manifest",
			"--target-line", "develop",
			"--push",
		)
		if err == nil {
			t.Fatal("manifest propagation unexpectedly exposed a push flag")
		}

		_, err = executeBootstrapCommand(
			t,
			command,
			"--interactive", "never",
			"--output", "json",
			"--yes",
			"workflow", "hotfix", "propagate-manifest",
			"--source", "hotfix/ABC-999-payment-timeout",
			"--target-line", "develop",
			"--publish",
		)
		if err == nil {
			t.Fatal("manifest propagation unexpectedly published without the dedicated publisher boundary")
		}
	})

	t.Run("publishes only with the dedicated publisher runtime", func(t *testing.T) {
		source := "hotfix/ABC-999-payment-timeout"
		record := commandHotfixRecord(t, source)
		messages := []string{"fix(ABC-999): resolve payment timeout"}
		git := &manifestCommandGit{commandGit: newCommandGit(t, source, messages)}
		publisher := &workflowRecordingPublisher{
			result: port.PublishedPullRequest{URL: "https://example.invalid/pr/propagation"},
		}
		runtime := commandRuntime(git)
		runtime.HotfixRecords = &hotfixRecordCommandStore{record: record}
		runtime.Quality = commandHotfixQuality{}
		runtime.Publisher = publisher
		runtime.HotfixPropagationPublisherEnabled = func() bool { return true }
		command := NewWithRuntime(BuildInfo{Version: "test"}, runtime)

		output, err := executeBootstrapCommand(
			t,
			command,
			"--interactive", "never",
			"--output", "json",
			"--yes",
			"--pull-request-provider", "github",
			"workflow", "hotfix", "propagate-manifest",
			"--source", source,
			"--target-line", "develop",
			"--publish",
		)
		if err != nil ||
			!strings.Contains(output, `"pushed":"true"`) ||
			!strings.Contains(output, `"publishedPullRequest":"https://example.invalid/pr/propagation"`) {
			t.Fatalf("publish manifest = (%q, %v)", output, err)
		}

		candidate := "fix/ABC-999-propagate-to-develop"
		git = &manifestCommandGit{
			commandGit:    newCommandGit(t, source, messages),
			current:       mustCommandBranch(t, candidate),
			active:        true,
			operation:     "cherry-pick",
			progressFound: true,
			progress: port.HotfixManifestProgress{
				Branch:   mustCommandBranch(t, candidate),
				Source:   mustCommandBranch(t, source),
				Target:   mustCommandBranch(t, "develop"),
				Manifest: record.Manifest(),
			},
		}
		runtime = commandRuntime(git)
		runtime.HotfixRecords = &hotfixRecordCommandStore{record: record}
		runtime.Quality = commandHotfixQuality{}
		runtime.Publisher = publisher
		runtime.HotfixPropagationPublisherEnabled = func() bool { return true }
		command = NewWithRuntime(BuildInfo{Version: "test"}, runtime)
		output, err = executeBootstrapCommand(
			t,
			command,
			"--interactive", "never",
			"--output", "json",
			"--yes",
			"--pull-request-provider", "github",
			"workflow", "hotfix", "propagate-manifest",
			"--source", source,
			"--target-line", "develop",
			"--branch", candidate,
			"--resume",
			"--publish",
		)
		if err != nil ||
			!strings.Contains(output, `"resumed":"true"`) ||
			!strings.Contains(output, `"pushed":"true"`) {
			t.Fatalf("resume publish manifest = (%q, %v)", output, err)
		}
	})

	t.Run("accepts an explicit slug and resumes a resolved manifest candidate", func(t *testing.T) {
		source := "hotfix/ABC-999-payment-timeout"
		candidateRaw := "fix/ABC-999-propagate-to-develop"
		candidate, err := branch.ParseName(candidateRaw)
		if err != nil {
			t.Fatal(err)
		}
		record := commandHotfixRecord(t, source)
		git := &manifestCommandGit{
			commandGit:    newCommandGit(t, source, nil),
			current:       candidate,
			active:        true,
			operation:     "cherry-pick",
			progressFound: true,
			progress: port.HotfixManifestProgress{
				Branch:   candidate,
				Source:   mustCommandBranch(t, source),
				Target:   mustCommandBranch(t, "develop"),
				Manifest: record.Manifest(),
			},
		}
		runtime := commandRuntime(git)
		runtime.HotfixRecords = &hotfixRecordCommandStore{record: record}
		runtime.Quality = commandHotfixQuality{}
		command := NewWithRuntime(BuildInfo{Version: "test"}, runtime)

		output, err := executeBootstrapCommand(
			t,
			command,
			"--interactive", "never",
			"--output", "json",
			"--yes",
			"workflow", "hotfix", "propagate-manifest",
			"--source", source,
			"--target-line", "develop",
			"--branch", candidateRaw,
			"--resume",
		)
		if err != nil || !strings.Contains(output, `"resumed":"true"`) {
			t.Fatalf("resume manifest = (%q, %v)", output, err)
		}

		normalGit := &manifestCommandGit{commandGit: newCommandGit(t, source, nil)}
		runtime = commandRuntime(normalGit)
		runtime.HotfixRecords = &hotfixRecordCommandStore{record: record}
		runtime.Quality = commandHotfixQuality{}
		command = NewWithRuntime(BuildInfo{Version: "test"}, runtime)
		output, err = executeBootstrapCommand(
			t,
			command,
			"--interactive", "never",
			"--output", "json",
			"--yes",
			"workflow", "hotfix", "propagate-manifest",
			"--source", source,
			"--target-line", "develop",
			"--slug", "custom-propagation",
		)
		if err != nil || !strings.Contains(output, `"branch":"fix/ABC-999-custom-propagation"`) {
			t.Fatalf("custom manifest = (%q, %v)", output, err)
		}
	})

	t.Run("requires confirmation and preserves manifest service failures", func(t *testing.T) {
		source := "hotfix/ABC-999-payment-timeout"
		candidate := "fix/ABC-999-propagate-to-develop"
		record := commandHotfixRecord(t, source)

		for _, args := range [][]string{
			{"workflow", "hotfix", "propagate-manifest", "--source", source, "--target-line", "develop"},
			{"workflow", "hotfix", "propagate-manifest", "--resume", "--source", source, "--target-line", "develop", "--branch", candidate},
		} {
			git := &manifestCommandGit{commandGit: newCommandGit(t, source, nil)}
			runtime := commandRuntime(git)
			runtime.HotfixRecords = &hotfixRecordCommandStore{record: record}
			runtime.Quality = commandHotfixQuality{}
			command := NewWithRuntime(BuildInfo{Version: "test"}, runtime)
			_, err := executeBootstrapCommand(
				t,
				command,
				append([]string{"--interactive", "never", "--output", "json"}, args...)...,
			)
			if err == nil {
				t.Fatalf("%v unexpectedly skipped confirmation", args)
			}
		}

		for _, args := range [][]string{
			{"workflow", "hotfix", "propagate-manifest", "--source", source, "--target-line", "develop"},
			{"workflow", "hotfix", "propagate-manifest", "--resume", "--source", source, "--target-line", "develop", "--branch", candidate},
		} {
			git := &manifestCommandGit{
				commandGit:    newCommandGit(t, source, nil),
				current:       mustCommandBranch(t, candidate),
				active:        true,
				operation:     "cherry-pick",
				progressFound: true,
				progress: port.HotfixManifestProgress{
					Branch:   mustCommandBranch(t, candidate),
					Source:   mustCommandBranch(t, source),
					Target:   mustCommandBranch(t, "develop"),
					Manifest: record.Manifest(),
				},
			}
			runtime := commandRuntime(git)
			runtime.HotfixRecords = &hotfixRecordCommandStore{err: errors.New("record failure")}
			runtime.Quality = commandHotfixQuality{}
			command := NewWithRuntime(BuildInfo{Version: "test"}, runtime)
			_, err := executeBootstrapCommand(
				t,
				command,
				append([]string{"--interactive", "never", "--output", "json", "--yes"}, args...)...,
			)
			if err == nil {
				t.Fatalf("%v unexpectedly swallowed service failure", args)
			}
		}
	})
}

func TestHotfixRecordCommandsRejectInvalidInputsAndDiscoveryFailures(t *testing.T) {
	run := func(t *testing.T, runtime Runtime, args ...string) error {
		t.Helper()
		command := NewWithRuntime(BuildInfo{Version: "test"}, runtime)
		_, err := executeBootstrapCommand(
			t,
			command,
			append([]string{"--interactive", "never", "--output", "json", "--yes"}, args...)...,
		)
		return err
	}

	t.Run("propagates discovery and current branch failures", func(t *testing.T) {
		base := commandRuntime(newCommandGit(t, "hotfix/ABC-999-payment-timeout", nil))
		base.HotfixRecords = &hotfixRecordCommandStore{record: commandHotfixRecord(t, "hotfix/ABC-999-payment-timeout")}
		discoverFailure := errors.New("discover failure")
		base.GitFactory = func(time.Duration) port.GitRepository {
			return &hotfixCommandFailureGit{GitRepository: newCommandGit(t, "hotfix/ABC-999-payment-timeout", nil), discoverErr: discoverFailure}
		}
		for _, command := range [][]string{
			{"workflow", "hotfix", "validate-record"},
			{"workflow", "hotfix", "verify-merge"},
			{"workflow", "hotfix", "verify-delivery"},
			{"workflow", "hotfix", "propagate-manifest", "--target-line", "develop"},
		} {
			if err := run(t, base, command...); !errors.Is(err, discoverFailure) {
				t.Fatalf("%v discover error = %v, want %v", command, err, discoverFailure)
			}
		}

		currentFailure := errors.New("current branch failure")
		base = commandRuntime(newCommandGit(t, "hotfix/ABC-999-payment-timeout", nil))
		base.HotfixRecords = &hotfixRecordCommandStore{record: commandHotfixRecord(t, "hotfix/ABC-999-payment-timeout")}
		base.GitFactory = func(time.Duration) port.GitRepository {
			return &hotfixCommandFailureGit{GitRepository: newCommandGit(t, "hotfix/ABC-999-payment-timeout", nil), currentErr: currentFailure}
		}
		for _, command := range [][]string{
			{"workflow", "hotfix", "validate-record"},
			{"workflow", "hotfix", "verify-merge"},
			{"workflow", "hotfix", "verify-delivery"},
			{"workflow", "hotfix", "propagate-manifest", "--target-line", "develop"},
		} {
			if err := run(t, base, command...); !errors.Is(err, currentFailure) {
				t.Fatalf("%v current branch error = %v, want %v", command, err, currentFailure)
			}
		}
	})

	t.Run("rejects non hotfix record verification", func(t *testing.T) {
		runtime := commandRuntime(newCommandGit(t, "feature/ABC-999-wrong", nil))
		runtime.HotfixRecords = &hotfixRecordCommandStore{}
		for _, command := range [][]string{
			{"workflow", "hotfix", "validate-record"},
			{"workflow", "hotfix", "verify-merge"},
			{"workflow", "hotfix", "verify-delivery"},
		} {
			if err := run(t, runtime, command...); err == nil {
				t.Fatalf("%v unexpectedly accepted a feature branch", command)
			}
		}
	})

	t.Run("rejects manifest command bypasses and malformed input", func(t *testing.T) {
		runtime := commandRuntime(newCommandGit(t, "hotfix/ABC-999-payment-timeout", nil))
		runtime.HotfixRecords = &hotfixRecordCommandStore{record: commandHotfixRecord(t, "hotfix/ABC-999-payment-timeout")}
		runtime.Quality = commandHotfixQuality{}

		testCases := [][]string{
			{"--dry-run", "workflow", "hotfix", "propagate-manifest", "--resume", "--source", "hotfix/ABC-999-payment-timeout", "--target-line", "develop", "--branch", "fix/ABC-999-propagate-to-develop"},
			{"workflow", "hotfix", "propagate-manifest", "--resume", "--target-line", "develop"},
			{"workflow", "hotfix", "propagate-manifest", "--source", "feature/ABC-999-wrong", "--target-line", "develop"},
			{"workflow", "hotfix", "propagate-manifest", "--source", "hotfix/ABC-999-payment-timeout"},
			{"workflow", "hotfix", "propagate-manifest", "--resume", "--source", "hotfix/ABC-999-payment-timeout", "--target-line", "develop"},
			{"workflow", "hotfix", "propagate-manifest", "--resume", "--source", "hotfix/ABC-999-payment-timeout", "--target-line", "develop", "--branch", "not-a-branch"},
			{"workflow", "hotfix", "propagate-manifest", "--source", "hotfix/ABC-999-payment-timeout", "--target-line", "develop", "--slug", "Invalid Slug"},
		}
		for _, args := range testCases {
			if err := run(t, runtime, args...); err == nil {
				t.Fatalf("%v unexpectedly succeeded", args)
			}
		}
	})

	t.Run("preserves record failures in merge and delivery verification", func(t *testing.T) {
		recordFailure := errors.New("record failure")
		runtime := commandRuntime(newCommandGit(t, "hotfix/ABC-999-payment-timeout", nil))
		runtime.HotfixRecords = &hotfixRecordCommandStore{err: recordFailure}
		publisher := &workflowRecordingPublisher{}
		runtime.Publisher = publisher
		for _, command := range [][]string{
			{"workflow", "hotfix", "verify-merge"},
			{"workflow", "hotfix", "verify-delivery"},
		} {
			if err := run(t, runtime, command...); !errors.Is(err, recordFailure) {
				t.Fatalf("%v error = %v, want %v", command, err, recordFailure)
			}
		}
	})
}

type hotfixRecordCommandStore struct {
	record   hotfix.ReleaseRecord
	err      error
	calls    int
	ticket   ticket.ID
	location string
}

func (store *hotfixRecordCommandStore) LoadHotfixReleaseRecord(
	_ context.Context,
	_ port.RepositoryIdentity,
	id ticket.ID,
	location string,
) (hotfix.ReleaseRecord, error) {
	store.calls++
	store.ticket = id
	store.location = location
	if store.err != nil {
		return hotfix.ReleaseRecord{}, store.err
	}
	return store.record, nil
}

func commandHotfixRecord(t *testing.T, source string) hotfix.ReleaseRecord {
	t.Helper()

	contents := fmt.Sprintf(
		`{"schemaVersion":1,"ticket":"ABC-999","incident":"INC-999","affectedLine":"main","targetVersion":"1.0.2","previousTag":"v1.0.1","expectedPullRequest":{"source":%q,"target":"main"},"manifest":["%s"],"commitBudgetException":"","propagationTargets":["develop"]}`,
		source,
		strings.Repeat("a", 40),
	)
	record, err := hotfix.ParseRecord([]byte(contents))
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func mustCommandBranch(t *testing.T, raw string) branch.BranchName {
	t.Helper()

	value, err := branch.ParseName(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

type manifestCommandGit struct {
	*commandGit
	progress      port.HotfixManifestProgress
	progressFound bool
	current       branch.BranchName
	active        bool
	operation     string
	continueErr   error
}

func (git *manifestCommandGit) CurrentBranch(ctx context.Context, repository port.RepositoryIdentity) (branch.BranchName, error) {
	if !git.current.IsZero() {
		return git.current, nil
	}
	return git.commandGit.CurrentBranch(ctx, repository)
}

func (git *manifestCommandGit) ActiveOperation(context.Context, port.RepositoryIdentity) (string, bool, error) {
	return git.operation, git.active, nil
}

func (git *manifestCommandGit) ContinueCherryPick(context.Context, port.RepositoryIdentity) error {
	if git.continueErr != nil {
		return git.continueErr
	}
	git.active = false
	git.operation = ""
	return nil
}

func (git *manifestCommandGit) LoadHotfixManifestProgress(
	context.Context,
	port.RepositoryIdentity,
) (port.HotfixManifestProgress, bool, error) {
	return git.progress, git.progressFound, nil
}

func (git *manifestCommandGit) StoreHotfixManifestProgress(
	_ context.Context,
	_ port.RepositoryIdentity,
	progress port.HotfixManifestProgress,
) error {
	git.progress = progress
	git.progressFound = true
	return nil
}

func (git *manifestCommandGit) ClearHotfixManifestProgress(context.Context, port.RepositoryIdentity) error {
	git.progress = port.HotfixManifestProgress{}
	git.progressFound = false
	return nil
}

type commandHotfixQuality struct{}

func (commandHotfixQuality) Run(
	context.Context,
	port.RepositoryIdentity,
	port.QualityRequest,
) (port.QualityResult, error) {
	return port.QualityResult{Status: port.QualityPassed}, nil
}

var _ port.HotfixReleaseRecordStore = (*hotfixRecordCommandStore)(nil)
var _ port.HotfixManifestProgressStore = (*manifestCommandGit)(nil)
var _ port.QualityRunner = commandHotfixQuality{}

type hotfixCommandFailureGit struct {
	port.GitRepository
	discoverErr error
	currentErr  error
}

func (git *hotfixCommandFailureGit) Discover(ctx context.Context, path string) (port.RepositoryIdentity, error) {
	if git.discoverErr != nil {
		return port.RepositoryIdentity{}, git.discoverErr
	}
	return git.GitRepository.Discover(ctx, path)
}

func (git *hotfixCommandFailureGit) CurrentBranch(ctx context.Context, repository port.RepositoryIdentity) (branch.BranchName, error) {
	if git.currentErr != nil {
		return branch.BranchName{}, git.currentErr
	}
	return git.GitRepository.CurrentBranch(ctx, repository)
}
