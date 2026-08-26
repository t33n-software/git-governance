package gitcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func signingConfigResults(configuration map[string]string) []processResult {
	keys := []string{
		"commit.gpgsign",
		"gpg.format",
		"user.signingkey",
		"user.email",
		"gpg.ssh.program",
		"gpg.ssh.allowedSignersFile",
	}
	results := make([]processResult, 0, len(keys))
	for _, key := range keys {
		value, present := configuration[key]
		if !present {
			results = append(results, processResult{err: errors.New("unset"), exitCode: 1})
			continue
		}
		results = append(results, processResult{stdout: value + "\n"})
	}
	return results
}

func validSigningConfigValues() map[string]string {
	return map[string]string{
		"commit.gpgsign":             "true",
		"gpg.format":                 "ssh",
		"user.signingkey":            "unused",
		"user.email":                 "lane@example.invalid",
		"gpg.ssh.allowedSignersFile": "unused",
	}
}

func TestSigningConfigurationReadsTheEffectiveConfig(t *testing.T) {
	t.Parallel()

	t.Run("maps every fact and asserts the read order", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		key := filepath.Join(root, "signing.key")
		allowed := filepath.Join(root, "allowed_signers")
		if err := os.WriteFile(key, []byte("key"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(allowed, []byte("allowed"), 0o600); err != nil {
			t.Fatal(err)
		}
		values := validSigningConfigValues()
		values["user.signingkey"] = key
		values["gpg.ssh.allowedSignersFile"] = allowed
		values["gpg.ssh.program"] = "custom-signer"

		runner := &fakeRunner{results: signingConfigResults(values)}
		repository := &Repository{runner: runner, timeout: time.Second}
		configuration, err := repository.SigningConfiguration(context.Background(), port.RepositoryIdentity{Root: root, Remote: "origin"})
		if err != nil {
			t.Fatal(err)
		}
		if !configuration.SigningEnabled || configuration.Format != "ssh" ||
			configuration.SigningKey != key || !configuration.SigningKeyReadable ||
			configuration.UserEmail != "lane@example.invalid" ||
			configuration.SignProgram != "custom-signer" ||
			configuration.AllowedSignersFile != allowed || !configuration.AllowedSignersReadable ||
			configuration.LaneInjection || len(configuration.InjectedSigningKeys) != 0 {
			t.Fatalf("SigningConfiguration() = %#v", configuration)
		}

		expectedCalls := [][]string{
			{"config", "--type=bool", "--get", "commit.gpgsign"},
			{"config", "--get", "gpg.format"},
			{"config", "--get", "user.signingkey"},
			{"config", "--get", "user.email"},
			{"config", "--get", "gpg.ssh.program"},
			{"config", "--get", "gpg.ssh.allowedSignersFile"},
		}
		if len(runner.calls) != len(expectedCalls) {
			t.Fatalf("config reads = %#v", runner.calls)
		}
		for index, expected := range expectedCalls {
			actual := runner.calls[index]
			if actual.directory != root || strings.Join(actual.arguments, " ") != strings.Join(expected, " ") {
				t.Fatalf("config read %d = %#v, want %v", index, actual, expected)
			}
		}
	})

	t.Run("unset keys are facts and make files unreadable", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: signingConfigResults(map[string]string{})}
		repository := &Repository{runner: runner, timeout: time.Second}
		configuration, err := repository.SigningConfiguration(context.Background(), testIdentity())
		if err != nil {
			t.Fatal(err)
		}
		if configuration.SigningEnabled || configuration.Format != "" || configuration.SigningKey != "" ||
			configuration.SigningKeyReadable || configuration.UserEmail != "" || configuration.SignProgram != "" ||
			configuration.AllowedSignersFile != "" || configuration.AllowedSignersReadable {
			t.Fatalf("unset SigningConfiguration() = %#v", configuration)
		}
	})

	t.Run("a disabled boolean is a fact", func(t *testing.T) {
		t.Parallel()
		values := map[string]string{"commit.gpgsign": "false"}
		runner := &fakeRunner{results: signingConfigResults(values)}
		repository := &Repository{runner: runner, timeout: time.Second}
		configuration, err := repository.SigningConfiguration(context.Background(), testIdentity())
		if err != nil {
			t.Fatal(err)
		}
		if configuration.SigningEnabled {
			t.Fatalf("disabled SigningConfiguration() = %#v", configuration)
		}
	})

	t.Run("a non-normalized boolean fails closed", func(t *testing.T) {
		t.Parallel()
		results := signingConfigResults(map[string]string{})
		results[0] = processResult{stdout: "perhaps\n"}
		runner := &fakeRunner{results: results}
		repository := &Repository{runner: runner, timeout: time.Second}
		_, err := repository.SigningConfiguration(context.Background(), testIdentity())
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("config read failures stop the inspection", func(t *testing.T) {
		t.Parallel()
		for index := 0; index < 6; index++ {
			index := index
			t.Run("read "+strconv.Itoa(index), func(t *testing.T) {
				t.Parallel()
				results := signingConfigResults(validSigningConfigValues())
				results[index] = processResult{err: errors.New("config backend failed"), exitCode: 128}
				runner := &fakeRunner{results: results}
				repository := &Repository{runner: runner, timeout: time.Second}
				_, err := repository.SigningConfiguration(context.Background(), testIdentity())
				assertProblemCode(t, err, problem.CodeGitCommandFailed)
			})
		}
	})

	t.Run("relative and home paths resolve like the signing program receives them", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		relative := filepath.Join("keys", "signing.key")
		if err := os.MkdirAll(filepath.Join(root, "keys"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, relative), []byte("key"), 0o600); err != nil {
			t.Fatal(err)
		}
		values := validSigningConfigValues()
		values["user.signingkey"] = relative
		runner := &fakeRunner{results: signingConfigResults(values)}
		repository := &Repository{runner: runner, timeout: time.Second}
		configuration, err := repository.SigningConfiguration(context.Background(), port.RepositoryIdentity{Root: root, Remote: "origin"})
		if err != nil {
			t.Fatal(err)
		}
		if configuration.SigningKey != relative || !configuration.SigningKeyReadable {
			t.Fatalf("relative SigningConfiguration() = %#v", configuration)
		}
	})
}

func TestSigningConfigurationHomeResolution(t *testing.T) {
	t.Run("home-relative paths resolve against the user home", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, ".ssh", "signing.key"), []byte("key"), 0o600); err != nil {
			t.Fatal(err)
		}
		values := validSigningConfigValues()
		values["user.signingkey"] = "~/.ssh/signing.key"
		runner := &fakeRunner{results: signingConfigResults(values)}
		repository := &Repository{runner: runner, timeout: time.Second}
		configuration, err := repository.SigningConfiguration(context.Background(), testIdentity())
		if err != nil {
			t.Fatal(err)
		}
		if !configuration.SigningKeyReadable {
			t.Fatalf("home-relative SigningConfiguration() = %#v", configuration)
		}

		if err := os.WriteFile(filepath.Join(home, "signing-alt.key"), []byte("key"), 0o600); err != nil {
			t.Fatal(err)
		}
		alternative := validSigningConfigValues()
		alternative["user.signingkey"] = `~\signing-alt.key`
		alternativeRunner := &fakeRunner{results: signingConfigResults(alternative)}
		alternativeRepository := &Repository{runner: alternativeRunner, timeout: time.Second}
		alternativeConfiguration, err := alternativeRepository.SigningConfiguration(context.Background(), testIdentity())
		if err != nil {
			t.Fatal(err)
		}
		if !alternativeConfiguration.SigningKeyReadable {
			t.Fatalf("backslash home-relative SigningConfiguration() = %#v", alternativeConfiguration)
		}
	})

	t.Run("an unresolvable home keeps the raw value and reports unreadable", func(t *testing.T) {
		t.Setenv("HOME", "")
		t.Setenv("USERPROFILE", "")
		t.Setenv("HOMEDRIVE", "")
		t.Setenv("HOMEPATH", "")
		if _, err := os.UserHomeDir(); err == nil {
			t.Skip("the platform still resolves a home directory")
		}
		values := validSigningConfigValues()
		values["user.signingkey"] = "~/.ssh/signing.key"
		runner := &fakeRunner{results: signingConfigResults(values)}
		repository := &Repository{runner: runner, timeout: time.Second}
		configuration, err := repository.SigningConfiguration(context.Background(), testIdentity())
		if err != nil {
			t.Fatal(err)
		}
		if configuration.SigningKeyReadable {
			t.Fatalf("unresolvable-home SigningConfiguration() = %#v", configuration)
		}
	})
}

func TestSigningConfigurationLaneInjection(t *testing.T) {
	runConfiguration := func(t *testing.T) port.SigningConfiguration {
		t.Helper()
		runner := &fakeRunner{results: signingConfigResults(validSigningConfigValues())}
		repository := &Repository{runner: runner, timeout: time.Second}
		configuration, err := repository.SigningConfiguration(context.Background(), testIdentity())
		if err != nil {
			t.Fatal(err)
		}
		return configuration
	}

	t.Run("no injection is reported without the count variable", func(t *testing.T) {
		configuration := runConfiguration(t)
		if configuration.LaneInjection || len(configuration.InjectedSigningKeys) != 0 {
			t.Fatalf("no-injection SigningConfiguration() = %#v", configuration)
		}
	})

	t.Run("an empty count is no injection", func(t *testing.T) {
		t.Setenv(gitConfigCountEnv, "  ")
		configuration := runConfiguration(t)
		if configuration.LaneInjection {
			t.Fatalf("empty-count SigningConfiguration() = %#v", configuration)
		}
	})

	t.Run("a zero count is no injection", func(t *testing.T) {
		t.Setenv(gitConfigCountEnv, "0")
		configuration := runConfiguration(t)
		if configuration.LaneInjection {
			t.Fatalf("zero-count SigningConfiguration() = %#v", configuration)
		}
	})

	t.Run("a malformed count is an injection without keys", func(t *testing.T) {
		t.Setenv(gitConfigCountEnv, "not-a-number")
		configuration := runConfiguration(t)
		if !configuration.LaneInjection || len(configuration.InjectedSigningKeys) != 0 {
			t.Fatalf("malformed-count SigningConfiguration() = %#v", configuration)
		}
	})

	t.Run("a negative count is an injection without keys", func(t *testing.T) {
		t.Setenv(gitConfigCountEnv, "-1")
		configuration := runConfiguration(t)
		if !configuration.LaneInjection || len(configuration.InjectedSigningKeys) != 0 {
			t.Fatalf("negative-count SigningConfiguration() = %#v", configuration)
		}
	})

	t.Run("signing keys are collected case-insensitively and others are skipped", func(t *testing.T) {
		t.Setenv(gitConfigCountEnv, "4")
		t.Setenv(gitConfigKeyEnv+"0", "commit.gpgsign")
		t.Setenv(gitConfigKeyEnv+"1", "User.Email")
		t.Setenv(gitConfigKeyEnv+"2", "http.proxy")
		t.Setenv(gitConfigKeyEnv+"3", " user.signingkey ")
		configuration := runConfiguration(t)
		if !configuration.LaneInjection {
			t.Fatalf("lane SigningConfiguration() = %#v", configuration)
		}
		expected := []string{"commit.gpgsign", "user.email", "user.signingkey"}
		if strings.Join(configuration.InjectedSigningKeys, ",") != strings.Join(expected, ",") {
			t.Fatalf("injected keys = %#v, want %#v", configuration.InjectedSigningKeys, expected)
		}
	})
}

func TestProveSigningCapability(t *testing.T) {
	t.Parallel()

	configuration := port.SigningConfiguration{
		SigningEnabled:     true,
		Format:             "ssh",
		SigningKey:         "C:/keys/signing.key",
		UserEmail:          "lane@example.invalid",
		AllowedSignersFile: "C:/keys/allowed_signers",
	}

	t.Run("signs and verifies the canary with the configured identity", func(t *testing.T) {
		t.Parallel()
		program := &fakeRunner{results: []processResult{{}, {}}}
		repository := &Repository{
			runner:        &fakeRunner{},
			timeout:       time.Second,
			programRunner: func(string) processRunner { return program },
		}
		identity := port.RepositoryIdentity{Root: t.TempDir(), Remote: "origin"}
		if err := repository.ProveSigningCapability(context.Background(), identity, configuration); err != nil {
			t.Fatal(err)
		}
		if len(program.calls) != 2 {
			t.Fatalf("program calls = %#v", program.calls)
		}
		sign := program.calls[0]
		if strings.Join(sign.arguments, " ") != "-Y sign -f C:/keys/signing.key -n git "+filepath.Join(sign.directory, "payload") {
			t.Fatalf("sign call = %#v", sign)
		}
		verify := program.calls[1]
		if verify.stdin != signingCanaryPayload {
			t.Fatalf("verify stdin = %q, want the canary payload", verify.stdin)
		}
		expectedVerify := "-Y verify -f C:/keys/allowed_signers -I lane@example.invalid -n git -s " +
			filepath.Join(verify.directory, "payload") + ".sig"
		if strings.Join(verify.arguments, " ") != expectedVerify {
			t.Fatalf("verify call = %#v", verify)
		}
		if _, err := os.Stat(verify.directory); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("canary workspace was not cleaned up: %v", err)
		}
	})

	t.Run("uses the configured program and defaults to ssh-keygen", func(t *testing.T) {
		t.Parallel()
		var binaries []string
		repository := &Repository{
			runner:  &fakeRunner{},
			timeout: time.Second,
			programRunner: func(binary string) processRunner {
				binaries = append(binaries, binary)
				return &fakeRunner{results: []processResult{{}, {}}}
			},
		}
		identity := port.RepositoryIdentity{Root: t.TempDir(), Remote: "origin"}
		if err := repository.ProveSigningCapability(context.Background(), identity, configuration); err != nil {
			t.Fatal(err)
		}
		custom := configuration
		custom.SignProgram = "custom-signer"
		if err := repository.ProveSigningCapability(context.Background(), identity, custom); err != nil {
			t.Fatal(err)
		}
		if len(binaries) != 4 ||
			binaries[0] != "ssh-keygen" || binaries[1] != "ssh-keygen" ||
			binaries[2] != "custom-signer" || binaries[3] != "custom-signer" {
			t.Fatalf("program binaries = %#v", binaries)
		}
	})

	t.Run("a missing program runner fails closed", func(t *testing.T) {
		t.Parallel()
		repository := &Repository{runner: &fakeRunner{}, timeout: time.Second}
		err := repository.ProveSigningCapability(context.Background(), testIdentity(), configuration)
		assertProblemCode(t, err, problem.CodeExternalCommandFailed)
	})

	t.Run("a nil context is replaced with a timed context", func(t *testing.T) {
		t.Parallel()
		program := &contextCaptureRunner{}
		repository := &Repository{
			runner:        &fakeRunner{},
			timeout:       time.Second,
			programRunner: func(string) processRunner { return program },
		}
		if err := repository.ProveSigningCapability(testNilContext(), testIdentity(), configuration); err != nil {
			t.Fatal(err)
		}
		if program.received == nil {
			t.Fatal("program runner received a nil context")
		}
		if _, found := program.received.Deadline(); !found {
			t.Fatal("program runner context has no timeout deadline")
		}
	})

	// The seam-mutating tests stay sequential: parallel siblings resume only
	// after every non-parallel sibling has restored the shared seams.
	t.Run("a failing temporary workspace fails closed", func(t *testing.T) {
		expected := errors.New("temp unavailable")
		original := signingMkdirTemp
		signingMkdirTemp = func(string, string) (string, error) { return "", expected }
		defer func() { signingMkdirTemp = original }()
		repository := &Repository{
			runner:        &fakeRunner{},
			timeout:       time.Second,
			programRunner: func(string) processRunner { return &fakeRunner{} },
		}
		err := repository.ProveSigningCapability(context.Background(), testIdentity(), configuration)
		if !errors.Is(err, expected) {
			t.Fatalf("workspace error = %v, want %v", err, expected)
		}
		assertProblemCode(t, err, problem.CodeExternalCommandFailed)
	})

	t.Run("a failing payload write fails closed", func(t *testing.T) {
		expected := errors.New("payload unavailable")
		original := signingWriteFile
		signingWriteFile = func(string, []byte, os.FileMode) error { return expected }
		defer func() { signingWriteFile = original }()
		repository := &Repository{
			runner:        &fakeRunner{},
			timeout:       time.Second,
			programRunner: func(string) processRunner { return &fakeRunner{} },
		}
		err := repository.ProveSigningCapability(context.Background(), testIdentity(), configuration)
		if !errors.Is(err, expected) {
			t.Fatalf("payload error = %v, want %v", err, expected)
		}
		assertProblemCode(t, err, problem.CodeExternalCommandFailed)
	})

	t.Run("a failing signature stops before the verification", func(t *testing.T) {
		t.Parallel()
		program := &fakeRunner{results: []processResult{{err: errors.New("sign failed"), exitCode: 255}}}
		repository := &Repository{
			runner:        &fakeRunner{},
			timeout:       time.Second,
			programRunner: func(string) processRunner { return program },
		}
		err := repository.ProveSigningCapability(context.Background(), testIdentity(), configuration)
		assertProblemCode(t, err, problem.CodeExternalCommandFailed)
		if len(program.calls) != 1 {
			t.Fatalf("program calls = %#v, want exactly the sign call", program.calls)
		}
	})

	t.Run("a rejected verification fails closed", func(t *testing.T) {
		t.Parallel()
		program := &fakeRunner{results: []processResult{{}, {err: errors.New("unknown principal"), exitCode: 1}}}
		repository := &Repository{
			runner:        &fakeRunner{},
			timeout:       time.Second,
			programRunner: func(string) processRunner { return program },
		}
		err := repository.ProveSigningCapability(context.Background(), testIdentity(), configuration)
		assertProblemCode(t, err, problem.CodeConfigurationInvalid)
		if len(program.calls) != 2 {
			t.Fatalf("program calls = %#v, want sign and verify", program.calls)
		}
	})
}

func TestCommitSignatureVerification(t *testing.T) {
	t.Parallel()

	message := coverageMessage(t)

	t.Run("the verification is skipped when the policy does not require it", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{}}}
		repository := &Repository{runner: runner, timeout: time.Second}
		if err := repository.Commit(context.Background(), testIdentity(), message); err != nil {
			t.Fatal(err)
		}
		if len(runner.calls) != 1 || strings.Join(runner.calls[0].arguments, " ") != "commit --file=-" {
			t.Fatalf("commit calls = %#v", runner.calls)
		}
	})

	t.Run("a created commit is verified against the configured signing identity", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{}, {}}}
		repository := &Repository{runner: runner, timeout: time.Second, requireSignedCommits: true}
		if err := repository.Commit(context.Background(), testIdentity(), message); err != nil {
			t.Fatal(err)
		}
		if len(runner.calls) != 2 ||
			strings.Join(runner.calls[0].arguments, " ") != "commit --file=-" ||
			strings.Join(runner.calls[1].arguments, " ") != "verify-commit HEAD" {
			t.Fatalf("commit calls = %#v", runner.calls)
		}
	})

	t.Run("an unsigned or untrusted created commit fails closed", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{}, {err: errors.New("no signature found"), exitCode: 1}}}
		repository := &Repository{runner: runner, timeout: time.Second, requireSignedCommits: true}
		err := repository.Commit(context.Background(), testIdentity(), message)
		assertProblemCode(t, err, problem.CodeCommitSignatureRequired)
	})

	t.Run("a failing commit never reaches the verification", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{results: []processResult{{err: errors.New("commit failed"), exitCode: 1}}}
		repository := &Repository{runner: runner, timeout: time.Second, requireSignedCommits: true}
		err := repository.Commit(context.Background(), testIdentity(), message)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
		if len(runner.calls) != 1 {
			t.Fatalf("commit calls = %#v, want exactly the commit call", runner.calls)
		}
	})
}

func TestResolveSigningPathEmptyValue(t *testing.T) {
	t.Parallel()
	if got := resolveSigningPath(t.TempDir(), ""); got != "" {
		t.Fatalf("resolveSigningPath(empty) = %q, want empty", got)
	}
}

func TestNewBindsTheSigningOptions(t *testing.T) {
	t.Parallel()

	repository := New(Options{})
	if repository.requireSignedCommits {
		t.Fatal("New() requires signed commits by default")
	}
	if repository.programRunner == nil {
		t.Fatal("New() does not bind a program runner")
	}
	runner, ok := repository.programRunner("ssh-keygen").(execRunner)
	if !ok || runner.binary != "ssh-keygen" {
		t.Fatalf("New() program runner = %#v", runner)
	}

	custom := New(Options{RequireSignedCommits: true, MaxOutputBytes: 128})
	if !custom.requireSignedCommits {
		t.Fatal("New() did not bind the signing requirement")
	}
	customRunner, ok := custom.programRunner("custom-signer").(execRunner)
	if !ok || customRunner.binary != "custom-signer" || customRunner.maxOutputBytes != 128 {
		t.Fatalf("New() custom program runner = %#v", customRunner)
	}
}
