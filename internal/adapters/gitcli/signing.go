package gitcli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

const (
	signingCanaryPayload = "git-governance commit-signing canary\n"
	gitConfigCountEnv    = "GIT_CONFIG_COUNT"
	gitConfigKeyEnv      = "GIT_CONFIG_KEY_"
	defaultSignProgram   = "ssh-keygen"
)

// laneSigningConfigKeys are the machine-identity signing keys a
// commit-producing lane must inject through its environment configuration.
var laneSigningConfigKeys = []string{
	"commit.gpgsign",
	"gpg.format",
	"user.signingkey",
	"user.name",
	"user.email",
}

// Test seams for the OS primitives of the canary workspace.
var (
	signingMkdirTemp = os.MkdirTemp
	signingWriteFile = os.WriteFile
)

// SigningConfiguration reads the effective commit-signing configuration of the
// repository context and reports readiness facts without exposing key material.
func (repository *Repository) SigningConfiguration(
	ctx context.Context,
	identity port.RepositoryIdentity,
) (port.SigningConfiguration, error) {
	enabled, err := repository.gitConfigBool(ctx, identity, "commit.gpgsign")
	if err != nil {
		return port.SigningConfiguration{}, err
	}
	format, err := repository.gitConfigValue(ctx, identity, "gpg.format")
	if err != nil {
		return port.SigningConfiguration{}, err
	}
	signingKey, err := repository.gitConfigValue(ctx, identity, "user.signingkey")
	if err != nil {
		return port.SigningConfiguration{}, err
	}
	email, err := repository.gitConfigValue(ctx, identity, "user.email")
	if err != nil {
		return port.SigningConfiguration{}, err
	}
	program, err := repository.gitConfigValue(ctx, identity, "gpg.ssh.program")
	if err != nil {
		return port.SigningConfiguration{}, err
	}
	allowedSigners, err := repository.gitConfigValue(ctx, identity, "gpg.ssh.allowedSignersFile")
	if err != nil {
		return port.SigningConfiguration{}, err
	}

	laneInjection, injectedKeys := laneSigningInjection()
	return port.SigningConfiguration{
		SigningEnabled:         enabled,
		Format:                 format,
		SigningKey:             signingKey,
		SigningKeyReadable:     signingFileReadable(identity.Root, signingKey),
		UserEmail:              email,
		SignProgram:            program,
		AllowedSignersFile:     allowedSigners,
		AllowedSignersReadable: signingFileReadable(identity.Root, allowedSigners),
		LaneInjection:          laneInjection,
		InjectedSigningKeys:    injectedKeys,
	}, nil
}

// ProveSigningCapability signs a bounded canary payload with the configured
// key and verifies that signature against the configured allowed-signers
// surface. The canary never mutates the repository, its refs, or its objects.
func (repository *Repository) ProveSigningCapability(
	ctx context.Context,
	identity port.RepositoryIdentity,
	configuration port.SigningConfiguration,
) error {
	program := configuration.SignProgram
	if program == "" {
		program = defaultSignProgram
	}
	directory, err := signingMkdirTemp("", "git-governance-signing-canary-")
	if err != nil {
		return problem.Wrap(problem.Details{
			Code:        problem.CodeExternalCommandFailed,
			Category:    problem.CategoryExternal,
			Field:       "Commit signing proof",
			Expected:    "a temporary workspace for the signing canary",
			Rule:        "doctor proves commit signing without mutating the repository",
			Remediation: "repair the temporary directory configuration and retry doctor",
		}, err)
	}
	defer func() { _ = os.RemoveAll(directory) }()

	payload := filepath.Join(directory, "payload")
	if err := signingWriteFile(payload, []byte(signingCanaryPayload), 0o600); err != nil {
		return problem.Wrap(problem.Details{
			Code:        problem.CodeExternalCommandFailed,
			Category:    problem.CategoryExternal,
			Field:       "Commit signing proof",
			Expected:    "a writable canary payload in the temporary workspace",
			Rule:        "doctor proves commit signing without mutating the repository",
			Remediation: "repair the temporary directory configuration and retry doctor",
		}, err)
	}

	sign := repository.invokeProgram(ctx, directory, program, nil,
		"-Y", "sign",
		"-f", resolveSigningPath(identity.Root, configuration.SigningKey),
		"-n", "git",
		payload,
	)
	if sign.err != nil {
		return problem.Wrap(problem.Details{
			Code:        problem.CodeExternalCommandFailed,
			Category:    problem.CategoryExternal,
			Field:       "Commit signing proof",
			Context:     "action=sign the canary payload",
			Diagnostic:  commandDiagnostic(sign),
			Expected:    "a successful canary signature with the configured signing key",
			Rule:        "doctor proves the configured signing key can produce a signature",
			Remediation: "repair the signing key or the configured signing program and retry doctor",
		}, commandCause(sign))
	}

	verify := repository.invokeProgram(ctx, directory, program, strings.NewReader(signingCanaryPayload),
		"-Y", "verify",
		"-f", resolveSigningPath(identity.Root, configuration.AllowedSignersFile),
		"-I", configuration.UserEmail,
		"-n", "git",
		"-s", payload+".sig",
	)
	if verify.err != nil {
		return problem.Wrap(problem.Details{
			Code:        problem.CodeConfigurationInvalid,
			Category:    problem.CategoryConfig,
			Field:       "Commit signing proof",
			Context:     "action=verify the canary signature",
			Diagnostic:  commandDiagnostic(verify),
			Expected:    "a canary signature that verifies against the allowed-signers file",
			Rule:        "doctor verifies the canary signature with the configured identity against the allowed-signers surface",
			Remediation: "register the signing key in the allowed-signers file for the configured user.email and retry doctor",
		}, commandCause(verify))
	}
	return nil
}

// verifyCreatedCommitSignature proves that the commit the CLI just created
// carries a valid, trusted signature. The check is fail-closed because the
// active policy requires signed commits.
func (repository *Repository) verifyCreatedCommitSignature(ctx context.Context, identity port.RepositoryIdentity) error {
	result := repository.invoke(ctx, identity.Root, nil, "verify-commit", "HEAD")
	if result.err != nil {
		return problem.Wrap(problem.Details{
			Code:        problem.CodeCommitSignatureRequired,
			Category:    problem.CategoryGovernance,
			Field:       "commit signature",
			Context:     strings.Join(commandSummary(identity, "verify the created commit signature"), " "),
			Diagnostic:  commandDiagnostic(result),
			Expected:    "a newly created commit with a valid, trusted signature",
			Rule:        "the active policy requires signed commits; every commit the CLI creates must verify against the configured signing identity",
			Remediation: "configure commit signing (commit.gpgsign, gpg.format, user.signingkey, gpg.ssh.allowedSignersFile), prove it with doctor, and recreate the commit",
		}, commandCause(result))
	}
	return nil
}

// gitConfigValue reads one effective Git configuration key. An unset key is a
// fact, not a failure.
func (repository *Repository) gitConfigValue(
	ctx context.Context,
	identity port.RepositoryIdentity,
	key string,
) (string, error) {
	result := repository.invoke(ctx, identity.Root, nil, "config", "--get", key)
	if result.exitCode == 1 {
		return "", nil
	}
	if result.err != nil {
		return "", repository.commandProblem(problem.CodeGitCommandFailed, identity, "read the Git configuration key "+key, result)
	}
	return strings.TrimSpace(result.stdout), nil
}

// gitConfigBool reads one effective Git configuration key normalized through
// Git's own boolean parser.
func (repository *Repository) gitConfigBool(
	ctx context.Context,
	identity port.RepositoryIdentity,
	key string,
) (bool, error) {
	result := repository.invoke(ctx, identity.Root, nil, "config", "--type=bool", "--get", key)
	if result.exitCode == 1 {
		return false, nil
	}
	if result.err != nil {
		return false, repository.commandProblem(problem.CodeGitCommandFailed, identity, "read the Git configuration key "+key, result)
	}
	switch strings.TrimSpace(result.stdout) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, problem.New(problem.Details{
			Code:        problem.CodeGitCommandFailed,
			Category:    problem.CategoryGit,
			Field:       "Git configuration",
			Actual:      key,
			Expected:    "a normalized boolean value from git config --type=bool",
			Rule:        "doctor reads the effective signing configuration through Git's own parser",
			Remediation: "review the Git configuration and retry doctor",
		})
	}
}

// invokeProgram runs a non-Git signing program with the same timeout and
// bounded-output contract as Git invocations.
func (repository *Repository) invokeProgram(
	ctx context.Context,
	directory string,
	binary string,
	stdin io.Reader,
	arguments ...string,
) processResult {
	if ctx == nil {
		ctx = context.Background()
	}
	contextWithTimeout, cancel := context.WithTimeout(ctx, repository.timeout)
	defer cancel()
	runnerFactory := repository.programRunner
	if runnerFactory == nil {
		runnerFactory = func(binary string) processRunner {
			return execRunner{binary: binary}
		}
	}
	return runnerFactory(binary).run(contextWithTimeout, directory, stdin, arguments...)
}

// signingFileReadable reports whether the configured path references a
// readable file.
func signingFileReadable(root, value string) bool {
	if value == "" {
		return false
	}
	file, err := os.Open(resolveSigningPath(root, value))
	if err != nil {
		return false
	}
	_ = file.Close()
	return true
}

// resolveSigningPath resolves a configured key or allowed-signers path the way
// the signing program receives it: home-relative, absolute, or relative to the
// repository root the CLI operates on.
func resolveSigningPath(root, value string) string {
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "~/") || strings.HasPrefix(value, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return value
		}
		return filepath.Join(home, value[2:])
	}
	if filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(root, value)
}

// laneSigningInjection reports whether the process environment carries a
// Git configuration injection and which machine-identity signing keys it
// supplies. A malformed injection is reported as present without keys so the
// lane check fails closed.
func laneSigningInjection() (bool, []string) {
	raw, present := os.LookupEnv(gitConfigCountEnv)
	if !present || strings.TrimSpace(raw) == "" {
		return false, nil
	}
	count, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || count < 0 {
		return true, nil
	}
	if count == 0 {
		return false, nil
	}
	keys := make([]string, 0, len(laneSigningConfigKeys))
	for index := 0; index < count; index++ {
		key := strings.ToLower(strings.TrimSpace(os.Getenv(gitConfigKeyEnv + strconv.Itoa(index))))
		if isLaneSigningKey(key) {
			keys = append(keys, key)
		}
	}
	return true, keys
}

func isLaneSigningKey(key string) bool {
	for _, candidate := range laneSigningConfigKeys {
		if key == candidate {
			return true
		}
	}
	return false
}
