package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

func TestPrePushCommandValidatesHookInputAndManualFallback(t *testing.T) {
	t.Run("validates supplied hook updates and emits one JSON result", func(t *testing.T) {
		git := newUtilityCommandGit(t, "feature/ABC-123-add-export", []string{
			"feat(ABC-123): add export",
		})
		quality := &utilityQualityRunner{
			result: port.QualityResult{
				Status: port.QualityPassed,
				Detail: "quality gates passed",
			},
		}
		application := newUtilityCommandApplication(git, &utilityPreferencesStore{}, quality, nil)

		stdout, stderr, err := executeUtilityCommand(
			t,
			newValidateCommand(application),
			context.Background(),
			strings.NewReader(utilityPrePushUpdate("feature/ABC-123-add-export")),
			"pre-push",
		)
		if err != nil {
			t.Fatalf("pre-push hook validation error = %v", err)
		}
		if stderr != "" {
			t.Fatalf("pre-push stderr = %q", stderr)
		}
		fields := utilityJSONFields(t, assertSingleUtilityJSONResult(t, stdout, "validate.pre-push"))
		if fields["updates"] != "1" || fields["qualityStatus"] != string(port.QualityPassed) || fields["qualityDetail"] != "quality gates passed" {
			t.Fatalf("hook report fields = %#v", fields)
		}
		if git.fetchCalls != 1 {
			t.Fatalf("hook validation fetch calls = %d, want 1", git.fetchCalls)
		}
		if quality.calls != 1 {
			t.Fatalf("hook validation quality calls = %d, want 1", quality.calls)
		}
	})

	t.Run("falls back to the current branch without hook updates", func(t *testing.T) {
		git := newUtilityCommandGit(t, "feature/ABC-123-add-export", nil)
		quality := &utilityQualityRunner{
			result: port.QualityResult{
				Status: port.QualityPassed,
				Detail: "manual quality passed",
			},
		}
		application := newUtilityCommandApplication(git, &utilityPreferencesStore{}, quality, nil)

		stdout, stderr, err := executeUtilityCommand(
			t,
			newValidateCommand(application),
			context.Background(),
			strings.NewReader(""),
			"pre-push",
			"--base", "origin/develop",
		)
		if err != nil {
			t.Fatalf("manual pre-push validation error = %v", err)
		}
		if stderr != "" {
			t.Fatalf("manual pre-push stderr = %q", stderr)
		}
		fields := utilityJSONFields(t, assertSingleUtilityJSONResult(t, stdout, "validate.pre-push"))
		if fields["branch"] != "feature/ABC-123-add-export" ||
			fields["base"] != "origin/develop" ||
			fields["missingBaseCommits"] != "false" ||
			fields["publication"] != string(branch.PublicationUnpublished) ||
			fields["qualityStatus"] != string(port.QualityPassed) ||
			fields["qualityDetail"] != "manual quality passed" {
			t.Fatalf("manual report fields = %#v", fields)
		}
		if git.currentCalls != 1 {
			t.Fatalf("manual fallback current branch calls = %d, want 1", git.currentCalls)
		}
	})
}

func TestPrePushCommandFailureAndCancellationContracts(t *testing.T) {
	discoverErr := errors.New("repository discovery failed")
	fetchErr := errors.New("fetch failed")
	currentErr := errors.New("current branch failed")

	testCases := []struct {
		name      string
		input     string
		args      []string
		configure func(*utilityCommandGit)
		wantErr   error
		wantCode  problem.Code
	}{
		{
			name:  "preserves discovery failures",
			input: utilityPrePushUpdate("feature/ABC-123-add-export"),
			configure: func(git *utilityCommandGit) {
				git.discoverErr = discoverErr
			},
			wantErr: discoverErr,
		},
		{
			name:     "rejects a base on another remote",
			input:    utilityPrePushUpdate("feature/ABC-123-add-export"),
			args:     []string{"--base", "upstream/develop"},
			wantCode: problem.CodeBranchBaseInvalid,
		},
		{
			name:     "rejects malformed hook input",
			input:    "not-a-pre-push-update\n",
			wantCode: problem.CodeInvalidInput,
		},
		{
			name:     "rejects manual branch selection with hook updates",
			input:    utilityPrePushUpdate("feature/ABC-123-add-export"),
			args:     []string{"--branch", "feature/ABC-123-add-export"},
			wantCode: problem.CodeInvalidInput,
		},
		{
			name:  "preserves hook validation dependency failures",
			input: utilityPrePushUpdate("feature/ABC-123-add-export"),
			configure: func(git *utilityCommandGit) {
				git.fetchErr = fetchErr
			},
			wantErr: fetchErr,
		},
		{
			name:  "preserves manual current branch failures",
			input: "",
			configure: func(git *utilityCommandGit) {
				git.currentErr = currentErr
			},
			wantErr: currentErr,
		},
		{
			name:  "preserves manual validation dependency failures",
			input: "",
			configure: func(git *utilityCommandGit) {
				git.fetchErr = fetchErr
			},
			wantErr: fetchErr,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			git := newUtilityCommandGit(t, "feature/ABC-123-add-export", []string{
				"feat(ABC-123): add export",
			})
			if testCase.configure != nil {
				testCase.configure(git)
			}
			application := newUtilityCommandApplication(git, &utilityPreferencesStore{}, &utilityQualityRunner{}, nil)

			_, _, err := executeUtilityCommand(
				t,
				newPrePushCommand(application),
				context.Background(),
				strings.NewReader(testCase.input),
				testCase.args...,
			)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("pre-push error = %v, want %v", err, testCase.wantErr)
				}
				return
			}
			assertProblemCode(t, err, testCase.wantCode)
		})
	}

	t.Run("passes cancellation to validation dependencies", func(t *testing.T) {
		git := newUtilityCommandGit(t, "feature/ABC-123-add-export", []string{
			"feat(ABC-123): add export",
		})
		application := newUtilityCommandApplication(git, &utilityPreferencesStore{}, &utilityQualityRunner{}, nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, _, err := executeUtilityCommand(
			t,
			newPrePushCommand(application),
			ctx,
			strings.NewReader(utilityPrePushUpdate("feature/ABC-123-add-export")),
		)
		assertProblemCode(t, err, problem.CodeOperationCancelled)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled pre-push error = %v, want context cancellation", err)
		}
		if len(git.fetchContexts) != 1 || git.fetchContexts[0] != ctx {
			t.Fatalf("fetch contexts = %#v, want cancelled command context", git.fetchContexts)
		}
	})
}

func TestReadPrePushUpdatesContracts(t *testing.T) {
	t.Run("parses a supplied update stream", func(t *testing.T) {
		command := &cobra.Command{}
		command.SetIn(strings.NewReader(utilityPrePushUpdate("feature/ABC-123-add-export")))

		updates, err := readPrePushUpdates(command)
		if err != nil {
			t.Fatalf("read pre-push updates error = %v", err)
		}
		if len(updates) != 1 ||
			updates[0].Target.String() != "feature/ABC-123-add-export" ||
			updates[0].Action != branchapp.PushActionCreate {
			t.Fatalf("parsed updates = %#v", updates)
		}
	})

	t.Run("preserves parser failures", func(t *testing.T) {
		command := &cobra.Command{}
		command.SetIn(strings.NewReader("invalid\n"))

		_, err := readPrePushUpdates(command)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("treats a terminal standard input as manual mode", func(t *testing.T) {
		terminal, err := os.Open(os.DevNull)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = terminal.Close()
		})
		info, err := terminal.Stat()
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&os.ModeCharDevice == 0 {
			t.Fatalf("%q is not a character device: mode=%v", os.DevNull, info.Mode())
		}

		originalStdin := os.Stdin
		os.Stdin = terminal
		t.Cleanup(func() {
			os.Stdin = originalStdin
		})

		updates, err := readPrePushUpdates(&cobra.Command{})
		if err != nil {
			t.Fatalf("terminal fallback error = %v", err)
		}
		if updates != nil {
			t.Fatalf("terminal fallback updates = %#v, want nil", updates)
		}
	})
}

func TestConfigKeyCommandsManagePreferences(t *testing.T) {
	abc := mustUtilityKey(t, "ABC")
	xyz := mustUtilityKey(t, "XYZ")
	loadErr := errors.New("preferences unavailable")

	t.Run("lists populated and empty preferences", func(t *testing.T) {
		store := &utilityPreferencesStore{
			preferences: port.Preferences{
				SchemaVersion: 1,
				KnownKeys:     []ticket.Key{abc, xyz},
				DefaultKey:    &abc,
			},
		}
		application := newUtilityCommandApplication(newUtilityCommandGit(t, "main", nil), store, nil, nil)

		stdout, stderr, err := executeUtilityCommand(
			t,
			newConfigCommand(application),
			context.Background(),
			nil,
			"key", "list",
		)
		if err != nil {
			t.Fatalf("key list error = %v", err)
		}
		if stderr != "" {
			t.Fatalf("key list stderr = %q", stderr)
		}
		fields := utilityJSONFields(t, assertSingleUtilityJSONResult(t, stdout, "config.key.list"))
		if fields["keys"] != "ABC,XYZ" || fields["defaultKey"] != "ABC" {
			t.Fatalf("populated key list fields = %#v", fields)
		}

		emptyStore := &utilityPreferencesStore{preferences: port.Preferences{SchemaVersion: 1}}
		emptyApplication := newUtilityCommandApplication(newUtilityCommandGit(t, "main", nil), emptyStore, nil, nil)
		stdout, _, err = executeUtilityCommand(
			t,
			newConfigCommand(emptyApplication),
			context.Background(),
			nil,
			"key", "list",
		)
		if err != nil {
			t.Fatalf("empty key list error = %v", err)
		}
		fields = utilityJSONFields(t, assertSingleUtilityJSONResult(t, stdout, "config.key.list"))
		if fields["keys"] != "" {
			t.Fatalf("empty key list fields = %#v", fields)
		}
		if _, found := fields["defaultKey"]; found {
			t.Fatalf("empty key list unexpectedly reported a default: %#v", fields)
		}
	})

	t.Run("propagates list dependency failures", func(t *testing.T) {
		store := &utilityPreferencesStore{loadErr: loadErr}
		application := newUtilityCommandApplication(newUtilityCommandGit(t, "main", nil), store, nil, nil)

		_, _, err := executeUtilityCommand(
			t,
			newConfigCommand(application),
			context.Background(),
			nil,
			"key", "list",
		)
		if !errors.Is(err, loadErr) {
			t.Fatalf("key list error = %v, want %v", err, loadErr)
		}
	})

	t.Run("adds valid keys and rejects invalid or missing input", func(t *testing.T) {
		store := &utilityPreferencesStore{preferences: port.Preferences{SchemaVersion: 1}}
		application := newUtilityCommandApplication(newUtilityCommandGit(t, "main", nil), store, nil, nil)

		stdout, _, err := executeUtilityCommand(
			t,
			newConfigCommand(application),
			context.Background(),
			nil,
			"key", "add", "--key", "ABC",
		)
		if err != nil {
			t.Fatalf("key add error = %v", err)
		}
		fields := utilityJSONFields(t, assertSingleUtilityJSONResult(t, stdout, "config.key.add"))
		if fields["key"] != "ABC" || fields["knownKeyCount"] != "1" {
			t.Fatalf("key add fields = %#v", fields)
		}
		if got := store.preferences.KnownKeys; len(got) != 1 || got[0].String() != "ABC" {
			t.Fatalf("saved known keys = %#v", got)
		}

		_, _, err = executeUtilityCommand(
			t,
			newConfigCommand(application),
			context.Background(),
			nil,
			"key", "add", "--key", "invalid",
		)
		assertProblemCode(t, err, problem.CodeTicketKeyInvalid)

		_, _, err = executeUtilityCommand(
			t,
			newConfigCommand(application),
			context.Background(),
			nil,
			"key", "add",
		)
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("propagates add dependency failures", func(t *testing.T) {
		store := &utilityPreferencesStore{loadErr: loadErr}
		application := newUtilityCommandApplication(newUtilityCommandGit(t, "main", nil), store, nil, nil)

		_, _, err := executeUtilityCommand(
			t,
			newConfigCommand(application),
			context.Background(),
			nil,
			"key", "add", "--key", "ABC",
		)
		if !errors.Is(err, loadErr) {
			t.Fatalf("key add error = %v, want %v", err, loadErr)
		}
	})

	t.Run("removes a default key and preserves removal failures", func(t *testing.T) {
		store := &utilityPreferencesStore{
			preferences: port.Preferences{
				SchemaVersion: 1,
				KnownKeys:     []ticket.Key{abc},
				DefaultKey:    &abc,
			},
		}
		application := newUtilityCommandApplication(newUtilityCommandGit(t, "main", nil), store, nil, nil)

		stdout, _, err := executeUtilityCommand(
			t,
			newConfigCommand(application),
			context.Background(),
			nil,
			"key", "remove", "--key", "ABC",
		)
		if err != nil {
			t.Fatalf("key remove error = %v", err)
		}
		fields := utilityJSONFields(t, assertSingleUtilityJSONResult(t, stdout, "config.key.remove"))
		if fields["key"] != "ABC" || fields["knownKeyCount"] != "0" {
			t.Fatalf("key remove fields = %#v", fields)
		}
		if len(store.preferences.KnownKeys) != 0 || store.preferences.DefaultKey != nil {
			t.Fatalf("preferences after removal = %#v", store.preferences)
		}

		_, _, err = executeUtilityCommand(
			t,
			newConfigCommand(application),
			context.Background(),
			nil,
			"key", "remove", "--key", "XYZ",
		)
		assertProblemCode(t, err, problem.CodeInvalidInput)

		_, _, err = executeUtilityCommand(
			t,
			newConfigCommand(application),
			context.Background(),
			nil,
			"key", "remove", "--key", "invalid",
		)
		assertProblemCode(t, err, problem.CodeTicketKeyInvalid)
	})

	t.Run("propagates remove dependency failures", func(t *testing.T) {
		store := &utilityPreferencesStore{
			preferences: port.Preferences{
				SchemaVersion: 1,
				KnownKeys:     []ticket.Key{abc},
			},
			loadErr: loadErr,
		}
		application := newUtilityCommandApplication(newUtilityCommandGit(t, "main", nil), store, nil, nil)

		_, _, err := executeUtilityCommand(
			t,
			newConfigCommand(application),
			context.Background(),
			nil,
			"key", "remove", "--key", "ABC",
		)
		if !errors.Is(err, loadErr) {
			t.Fatalf("key remove error = %v, want %v", err, loadErr)
		}
	})

	t.Run("sets a default key and propagates dependency failures", func(t *testing.T) {
		store := &utilityPreferencesStore{preferences: port.Preferences{SchemaVersion: 1}}
		application := newUtilityCommandApplication(newUtilityCommandGit(t, "main", nil), store, nil, nil)

		stdout, _, err := executeUtilityCommand(
			t,
			newConfigCommand(application),
			context.Background(),
			nil,
			"key", "set-default", "--key", "XYZ",
		)
		if err != nil {
			t.Fatalf("set default error = %v", err)
		}
		fields := utilityJSONFields(t, assertSingleUtilityJSONResult(t, stdout, "config.key.set-default"))
		if fields["defaultKey"] != "XYZ" {
			t.Fatalf("set default fields = %#v", fields)
		}
		if store.preferences.DefaultKey == nil || store.preferences.DefaultKey.String() != "XYZ" {
			t.Fatalf("preferences default = %#v", store.preferences.DefaultKey)
		}

		_, _, err = executeUtilityCommand(
			t,
			newConfigCommand(application),
			context.Background(),
			nil,
			"key", "set-default",
		)
		assertProblemCode(t, err, problem.CodeInvalidInput)

		failingStore := &utilityPreferencesStore{loadErr: loadErr}
		failingApplication := newUtilityCommandApplication(newUtilityCommandGit(t, "main", nil), failingStore, nil, nil)
		_, _, err = executeUtilityCommand(
			t,
			newConfigCommand(failingApplication),
			context.Background(),
			nil,
			"key", "set-default", "--key", "XYZ",
		)
		if !errors.Is(err, loadErr) {
			t.Fatalf("set default error = %v, want %v", err, loadErr)
		}
	})
}

func TestPolicyDoctorAndReportContracts(t *testing.T) {
	t.Run("describes the active policy as one JSON result", func(t *testing.T) {
		application := newUtilityCommandApplication(newUtilityCommandGit(t, "main", nil), &utilityPreferencesStore{}, nil, nil)

		stdout, stderr, err := executeUtilityCommand(
			t,
			newPolicyCommand(application),
			context.Background(),
			nil,
			"describe",
		)
		if err != nil {
			t.Fatalf("policy describe error = %v", err)
		}
		if stderr != "" {
			t.Fatalf("policy describe stderr = %q", stderr)
		}
		fields := utilityJSONFields(t, assertSingleUtilityJSONResult(t, stdout, "policy.describe"))
		if fields["schemaVersion"] != "1" || fields["keyPolicy"] != "syntax-only" {
			t.Fatalf("policy description fields = %#v", fields)
		}
	})

	t.Run("renders successful and failed doctor checks", func(t *testing.T) {
		tools := &utilityToolInspector{
			operatingSystem: "windows",
			architecture:    "",
			versionErr:      errors.New("lefthook unavailable"),
			exists:          false,
		}
		application := newUtilityCommandApplication(
			newUtilityCommandGit(t, "main", nil),
			&utilityPreferencesStore{preferences: port.Preferences{SchemaVersion: 1}},
			nil,
			tools,
		)

		stdout, stderr, err := executeUtilityCommand(
			t,
			newDoctorCommand(application),
			context.Background(),
			nil,
		)
		if err != nil {
			t.Fatalf("doctor error = %v", err)
		}
		if stderr != "" {
			t.Fatalf("doctor stderr = %q", stderr)
		}
		fields := utilityJSONFields(t, assertSingleUtilityJSONResult(t, stdout, "doctor"))
		if fields["Git version"] != "ok: git version test" ||
			fields["runtime platform"] != "failed: windows/" ||
			fields["Lefthook executable"] != "failed: lefthook unavailable" ||
			fields["Lefthook configuration"] != "failed: lefthook.yml is not present" {
			t.Fatalf("doctor fields = %#v", fields)
		}
	})

	t.Run("fails the command when Git transport authentication is unavailable", func(t *testing.T) {
		expected := errors.New("Git transport credentials unavailable")
		git := &utilityAuthFailureGit{
			utilityCommandGit: newUtilityCommandGit(t, "feature/ABC-123-add-export", nil),
			authErr:           expected,
		}
		application := newUtilityCommandApplication(git, &utilityPreferencesStore{}, nil, nil)
		_, _, err := executeUtilityCommand(t, newDoctorCommand(application), context.Background(), nil)
		if !errors.Is(err, expected) {
			t.Fatalf("doctor authentication error = %v, want %v", err, expected)
		}
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("fails the command when the commit-signing proof is unavailable", func(t *testing.T) {
		expected := errors.New("canary verification failed")
		git := newUtilityCommandGit(t, "feature/ABC-123-add-export", nil)
		git.signingProofErr = expected
		application := newUtilityCommandApplication(git, &utilityPreferencesStore{}, nil, nil)
		_, _, err := executeUtilityCommand(t, newDoctorCommand(application), context.Background(), nil)
		if !errors.Is(err, expected) {
			t.Fatalf("doctor signing error = %v, want %v", err, expected)
		}
	})

	t.Run("stops diagnostics when the command context is cancelled", func(t *testing.T) {
		application := newUtilityCommandApplication(
			newUtilityCommandGit(t, "main", nil),
			&utilityPreferencesStore{preferences: port.Preferences{SchemaVersion: 1}},
			nil,
			nil,
		)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, _, err := executeUtilityCommand(
			t,
			newDoctorCommand(application),
			ctx,
			nil,
		)
		assertProblemCode(t, err, problem.CodeOperationCancelled)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled doctor error = %v, want context cancellation", err)
		}
	})

	t.Run("propagates output writer failures", func(t *testing.T) {
		writeErr := errors.New("output unavailable")
		application := newUtilityCommandApplication(newUtilityCommandGit(t, "main", nil), &utilityPreferencesStore{}, nil, nil)
		command := newPolicyCommand(application)
		command.SetOut(utilityFailingWriter{err: writeErr})
		command.SetErr(io.Discard)
		command.SetArgs([]string{"describe"})

		err := command.ExecuteContext(context.Background())
		assertProblemCode(t, err, problem.CodeExternalCommandFailed)
		if !errors.Is(err, writeErr) {
			t.Fatalf("report error = %v, want %v", err, writeErr)
		}
	})

	t.Run("propagates report cancellation", func(t *testing.T) {
		application := newUtilityCommandApplication(newUtilityCommandGit(t, "main", nil), &utilityPreferencesStore{}, nil, nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, _, err := executeUtilityCommand(
			t,
			newPolicyCommand(application),
			ctx,
			nil,
			"describe",
		)
		assertProblemCode(t, err, problem.CodeOperationCancelled)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled report error = %v, want context cancellation", err)
		}
	})
}

func TestUtilityCommandHelpers(t *testing.T) {
	if got := itoa(-42); got != "-42" {
		t.Fatalf("itoa(-42) = %q, want -42", got)
	}
}

func newUtilityCommandApplication(
	git port.GitRepository,
	store port.PreferencesStore,
	quality port.QualityRunner,
	tools port.ToolInspector,
) *application {
	if quality == nil {
		quality = &utilityQualityRunner{
			result: port.QualityResult{
				Status: port.QualityUnconfigured,
				Detail: "no configured quality gates",
			},
		}
	}
	if tools == nil {
		tools = &utilityToolInspector{
			operatingSystem: "windows",
			architecture:    "amd64",
			version:         "lefthook version test",
			exists:          true,
		}
	}
	return newApplication(Runtime{
		GitFactory: func(time.Duration) port.GitRepository {
			return git
		},
		StoreFactory: func(string) port.PreferencesStore {
			return store
		},
		Quality: quality,
		Tools:   tools,
		InputIsTerminal: func() bool {
			return false
		},
		OutputIsTerminal: func() bool {
			return false
		},
	}, &appOptions{
		interactive: "never",
		output:      "json",
		color:       "never",
		remote:      "origin",
		repository:  "C:/repo",
		timeout:     time.Second,
	})
}

func executeUtilityCommand(
	t *testing.T,
	command *cobra.Command,
	ctx context.Context,
	input io.Reader,
	arguments ...string,
) (string, string, error) {
	t.Helper()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	command.SetOut(stdout)
	command.SetErr(stderr)
	if input != nil {
		command.SetIn(input)
	}
	command.SetArgs(arguments)
	err := command.ExecuteContext(ctx)
	return stdout.String(), stderr.String(), err
}

func assertSingleUtilityJSONResult(t *testing.T, output, operation string) map[string]any {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(output))
	var result map[string]any
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("decode JSON result %q: %v", output, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("JSON output contains more than one result: %q", output)
	}
	if result["schemaVersion"] != float64(1) {
		t.Fatalf("JSON schema version = %#v, want 1", result["schemaVersion"])
	}
	if result["ok"] != true {
		t.Fatalf("JSON success = %#v, want true: %#v", result["ok"], result)
	}
	if result["operation"] != operation {
		t.Fatalf("JSON operation = %#v, want %q", result["operation"], operation)
	}
	return result
}

func utilityJSONFields(t *testing.T, result map[string]any) map[string]string {
	t.Helper()

	rawFields, ok := result["fields"].(map[string]any)
	if !ok {
		t.Fatalf("JSON fields = %#v, want object", result["fields"])
	}
	fields := make(map[string]string, len(rawFields))
	for key, value := range rawFields {
		text, ok := value.(string)
		if !ok {
			t.Fatalf("JSON field %q = %#v, want string", key, value)
		}
		fields[key] = text
	}
	return fields
}

func utilityPrePushUpdate(name string) string {
	return "refs/heads/" + name + " " + strings.Repeat("a", 40) +
		" refs/heads/" + name + " " + strings.Repeat("0", 40) + "\n"
}

func mustUtilityKey(t *testing.T, raw string) ticket.Key {
	t.Helper()
	key, err := ticket.ParseKey(raw)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

type utilityCommandGit struct {
	*commandGit

	discoverErr error
	currentErr  error
	validateErr error
	fetchErr    error

	currentCalls     int
	fetchCalls       int
	fetchContexts    []context.Context
	discoverContexts []context.Context
}

type utilityAuthFailureGit struct {
	*utilityCommandGit
	authErr error
}

func (git *utilityAuthFailureGit) CheckTransportAuthentication(context.Context, port.RepositoryIdentity) error {
	return git.authErr
}

func newUtilityCommandGit(t *testing.T, current string, messages []string) *utilityCommandGit {
	t.Helper()
	return &utilityCommandGit{commandGit: newCommandGit(t, current, messages)}
}

func (git *utilityCommandGit) Discover(ctx context.Context, directory string) (port.RepositoryIdentity, error) {
	git.discoverContexts = append(git.discoverContexts, ctx)
	if git.discoverErr != nil {
		return port.RepositoryIdentity{}, git.discoverErr
	}
	return git.commandGit.Discover(ctx, directory)
}

func (git *utilityCommandGit) CurrentBranch(ctx context.Context, repository port.RepositoryIdentity) (branch.BranchName, error) {
	git.currentCalls++
	if git.currentErr != nil {
		return branch.BranchName{}, git.currentErr
	}
	return git.commandGit.CurrentBranch(ctx, repository)
}

func (git *utilityCommandGit) ValidateBranchRef(ctx context.Context, repository port.RepositoryIdentity, name branch.BranchName) error {
	if git.validateErr != nil {
		return git.validateErr
	}
	return git.commandGit.ValidateBranchRef(ctx, repository, name)
}

func (git *utilityCommandGit) Fetch(ctx context.Context, repository port.RepositoryIdentity) error {
	git.fetchCalls++
	git.fetchContexts = append(git.fetchContexts, ctx)
	if git.fetchErr != nil {
		return git.fetchErr
	}
	return git.commandGit.Fetch(ctx, repository)
}

var _ port.GitRepository = (*utilityCommandGit)(nil)

type utilityPreferencesStore struct {
	preferences port.Preferences
	loadErr     error
	saveErr     error
}

func (store *utilityPreferencesStore) Load(context.Context) (port.Preferences, error) {
	if store.loadErr != nil {
		return port.Preferences{}, store.loadErr
	}
	return cloneUtilityPreferences(store.preferences), nil
}

func (store *utilityPreferencesStore) Save(_ context.Context, preferences port.Preferences) error {
	if store.saveErr != nil {
		return store.saveErr
	}
	store.preferences = cloneUtilityPreferences(preferences)
	return nil
}

func cloneUtilityPreferences(preferences port.Preferences) port.Preferences {
	cloned := preferences
	cloned.KnownKeys = append([]ticket.Key(nil), preferences.KnownKeys...)
	if preferences.DefaultKey != nil {
		defaultKey := *preferences.DefaultKey
		cloned.DefaultKey = &defaultKey
	}
	return cloned
}

var _ port.PreferencesStore = (*utilityPreferencesStore)(nil)

type utilityQualityRunner struct {
	result port.QualityResult
	err    error
	calls  int
}

func (runner *utilityQualityRunner) Run(
	_ context.Context,
	_ port.RepositoryIdentity,
	_ port.QualityRequest,
) (port.QualityResult, error) {
	runner.calls++
	if runner.err != nil {
		return port.QualityResult{}, runner.err
	}
	return runner.result, nil
}

var _ port.QualityRunner = (*utilityQualityRunner)(nil)

type utilityToolInspector struct {
	operatingSystem string
	architecture    string
	version         string
	versionErr      error
	exists          bool
	fileErr         error
}

func (tools *utilityToolInspector) Platform() (string, string) {
	return tools.operatingSystem, tools.architecture
}

func (tools *utilityToolInspector) Version(context.Context, string) (string, error) {
	if tools.versionErr != nil {
		return "", tools.versionErr
	}
	return tools.version, nil
}

func (tools *utilityToolInspector) FileExists(string) (bool, error) {
	if tools.fileErr != nil {
		return false, tools.fileErr
	}
	return tools.exists, nil
}

var _ port.ToolInspector = (*utilityToolInspector)(nil)

type utilityFailingWriter struct {
	err error
}

func (writer utilityFailingWriter) Write([]byte) (int, error) {
	return 0, writer.err
}
