package policy

import (
	"context"
	"errors"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

type doctorGitNoInspector struct {
	port.GitRepository
}

func (doctorGitNoInspector) CheckTransportAuthentication(context.Context, port.RepositoryIdentity) error {
	return nil
}

func runSigningDoctor(t *testing.T, git port.GitRepository) DoctorResult {
	t.Helper()
	result, err := NewDoctorServiceWithDependencies(
		git,
		&memoryStore{preferences: port.Preferences{SchemaVersion: schemaVersion}},
		SyntaxOnlyKeyPolicy{},
		doctorTools{version: "lefthook 2.1.8", exists: true},
	).Run(context.Background(), "C:/repo")
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestDoctorSigningGate(t *testing.T) {
	t.Parallel()

	t.Run("a complete signing configuration passes all signing checks", func(t *testing.T) {
		t.Parallel()
		result := runSigningDoctor(t, doctorGit{commits: true, signingConfig: validSigningConfiguration()})
		if len(result.Checks) != 13 {
			t.Fatalf("doctor checks = %#v", result.Checks)
		}
		if !checkByName(t, result.Checks, "Commit signing configuration").OK ||
			!checkByName(t, result.Checks, "Commit signing proof").OK {
			t.Fatalf("signing checks = %#v", result.Checks)
		}
		if result.SigningError() != nil {
			t.Fatalf("SigningError() = %v", result.SigningError())
		}
	})

	t.Run("every configuration violation fails closed with its own problem", func(t *testing.T) {
		t.Parallel()
		testCases := []struct {
			name   string
			mutate func(*port.SigningConfiguration)
			code   problem.Code
		}{
			{
				name:   "signing disabled",
				mutate: func(configuration *port.SigningConfiguration) { configuration.SigningEnabled = false },
				code:   problem.CodeConfigurationUnavailable,
			},
			{
				name:   "wrong format",
				mutate: func(configuration *port.SigningConfiguration) { configuration.Format = "openpgp" },
				code:   problem.CodeConfigurationInvalid,
			},
			{
				name:   "missing signing key",
				mutate: func(configuration *port.SigningConfiguration) { configuration.SigningKey = "" },
				code:   problem.CodeConfigurationUnavailable,
			},
			{
				name:   "unreadable signing key",
				mutate: func(configuration *port.SigningConfiguration) { configuration.SigningKeyReadable = false },
				code:   problem.CodeConfigurationInvalid,
			},
			{
				name:   "missing identity",
				mutate: func(configuration *port.SigningConfiguration) { configuration.UserEmail = "" },
				code:   problem.CodeConfigurationUnavailable,
			},
			{
				name:   "missing allowed signers",
				mutate: func(configuration *port.SigningConfiguration) { configuration.AllowedSignersFile = "" },
				code:   problem.CodeConfigurationUnavailable,
			},
			{
				name:   "unreadable allowed signers",
				mutate: func(configuration *port.SigningConfiguration) { configuration.AllowedSignersReadable = false },
				code:   problem.CodeConfigurationInvalid,
			},
		}
		for _, testCase := range testCases {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				t.Parallel()
				configuration := validSigningConfiguration()
				testCase.mutate(&configuration)
				result := runSigningDoctor(t, doctorGit{commits: true, signingConfig: configuration})
				configurationCheck := checkByName(t, result.Checks, "Commit signing configuration")
				proofCheck := checkByName(t, result.Checks, "Commit signing proof")
				if configurationCheck.OK || proofCheck.OK {
					t.Fatalf("signing checks unexpectedly passed: %#v", result.Checks)
				}
				if proofCheck.Detail != "the signing proof requires a complete signing configuration" {
					t.Fatalf("proof check detail = %q", proofCheck.Detail)
				}
				assertProblemCode(t, result.SigningError(), testCase.code)
			})
		}
	})

	t.Run("a configuration read failure skips the proof", func(t *testing.T) {
		t.Parallel()
		expected := errors.New("config backend unavailable")
		result := runSigningDoctor(t, doctorGit{commits: true, signingConfigErr: expected})
		if checkByName(t, result.Checks, "Commit signing configuration").OK {
			t.Fatalf("signing checks = %#v", result.Checks)
		}
		proofCheck := checkByName(t, result.Checks, "Commit signing proof")
		if proofCheck.OK || proofCheck.Detail != "the signing proof requires a readable signing configuration" {
			t.Fatalf("proof check = %#v", proofCheck)
		}
		if !errors.Is(result.SigningError(), expected) {
			t.Fatalf("SigningError() = %v, want %v", result.SigningError(), expected)
		}
	})

	t.Run("a failing canary proof fails closed", func(t *testing.T) {
		t.Parallel()
		expected := errors.New("canary verification failed")
		result := runSigningDoctor(t, doctorGit{
			commits:         true,
			signingConfig:   validSigningConfiguration(),
			signingProofErr: expected,
		})
		if !checkByName(t, result.Checks, "Commit signing configuration").OK {
			t.Fatalf("configuration check = %#v", result.Checks)
		}
		if checkByName(t, result.Checks, "Commit signing proof").OK {
			t.Fatalf("proof check unexpectedly passed: %#v", result.Checks)
		}
		if !errors.Is(result.SigningError(), expected) {
			t.Fatalf("SigningError() = %v, want %v", result.SigningError(), expected)
		}
	})

	t.Run("a missing inspector capability fails closed", func(t *testing.T) {
		t.Parallel()
		withoutInspector := doctorGitNoInspector{GitRepository: doctorGit{commits: true}}
		result := runSigningDoctor(t, withoutInspector)
		configurationCheck := checkByName(t, result.Checks, "Commit signing configuration")
		proofCheck := checkByName(t, result.Checks, "Commit signing proof")
		if configurationCheck.OK || proofCheck.OK {
			t.Fatalf("signing checks unexpectedly passed: %#v", result.Checks)
		}
		if proofCheck.Detail != "commit-signing inspector is not configured" {
			t.Fatalf("proof check detail = %q", proofCheck.Detail)
		}
		assertProblemCode(t, result.SigningError(), problem.CodeConfigurationUnavailable)
	})

	t.Run("a complete lane injection passes the lane check", func(t *testing.T) {
		t.Parallel()
		configuration := validSigningConfiguration()
		configuration.LaneInjection = true
		configuration.InjectedSigningKeys = []string{
			"commit.gpgsign", "gpg.format", "user.signingkey", "user.name", "user.email",
		}
		result := runSigningDoctor(t, doctorGit{commits: true, signingConfig: configuration})
		if len(result.Checks) != 14 {
			t.Fatalf("lane doctor checks = %#v", result.Checks)
		}
		if !checkByName(t, result.Checks, "Lane signing identity").OK {
			t.Fatalf("lane check = %#v", result.Checks)
		}
		if result.SigningError() != nil {
			t.Fatalf("SigningError() = %v", result.SigningError())
		}
	})

	t.Run("an incomplete lane injection fails closed", func(t *testing.T) {
		t.Parallel()
		configuration := validSigningConfiguration()
		configuration.LaneInjection = true
		configuration.InjectedSigningKeys = []string{"commit.gpgsign", "gpg.format"}
		result := runSigningDoctor(t, doctorGit{commits: true, signingConfig: configuration})
		laneCheck := checkByName(t, result.Checks, "Lane signing identity")
		if laneCheck.OK {
			t.Fatalf("lane check unexpectedly passed: %#v", result.Checks)
		}
		assertProblemCode(t, result.SigningError(), problem.CodeConfigurationUnavailable)
	})

	t.Run("a missing Git adapter fails every signing check", func(t *testing.T) {
		t.Parallel()
		result, err := NewDoctorService(nil, nil).Run(context.Background(), "C:/repo")
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Checks) != 9 {
			t.Fatalf("minimal doctor checks = %#v", result.Checks)
		}
		if checkByName(t, result.Checks, "Commit signing configuration").OK ||
			checkByName(t, result.Checks, "Commit signing proof").OK {
			t.Fatalf("signing checks unexpectedly passed: %#v", result.Checks)
		}
		assertProblemCode(t, result.SigningError(), problem.CodeConfigurationUnavailable)
	})
}

var _ port.GitTransportAuthenticator = doctorGitNoInspector{}
