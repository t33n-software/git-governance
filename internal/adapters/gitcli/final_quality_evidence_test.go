package gitcli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestRepositoryFinalQualityEvidenceContracts(t *testing.T) {
	t.Parallel()

	identity := testIdentity()
	revision := strings.Repeat("a", 40)
	evidence := port.FinalQualityEvidence{
		SchemaVersion:       1,
		Remote:              "origin",
		ConfigurationDigest: "configuration",
		Gates:               []string{"full-local-build"},
		Toolchain:           "go1.26/windows/amd64",
		WorktreeClean:       true,
		CreatedAt:           time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC),
		Updates: []port.FinalQualityEvidenceUpdate{{
			LocalRef:      "refs/heads/feature/ABC-123-add-export",
			LocalObjectID: revision,
			RemoteRef:     "refs/heads/feature/ABC-123-add-export",
			Branch:        "feature/ABC-123-add-export",
			Base:          "origin/develop",
			BaseRevision:  strings.Repeat("b", 40),
		}},
	}

	t.Run("resolves exact commit revisions", func(t *testing.T) {
		repository, runner := coverageRepository(processResult{stdout: strings.ToUpper(revision) + "\n"})
		actual, err := repository.ResolveRevision(context.Background(), identity, "refs/heads/feature/ABC-123-add-export")
		if err != nil || actual != revision {
			t.Fatalf("ResolveRevision() = (%q, %v)", actual, err)
		}
		assertCall(
			t,
			runner.calls[0],
			identity.Root,
			"",
			"rev-parse",
			"--verify",
			"--quiet",
			"refs/heads/feature/ABC-123-add-export^{commit}",
		)
	})

	t.Run("rejects failed and malformed revision resolution", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{err: errors.New("revision failed"), exitCode: 128})
		_, err := repository.ResolveRevision(context.Background(), identity, "HEAD")
		assertProblemCode(t, err, problem.CodeGitCommandFailed)

		repository, _ = coverageRepository(processResult{stdout: strings.Repeat("a", 41) + "\n"})
		_, err = repository.ResolveRevision(context.Background(), identity, "HEAD")
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("loads absence, rejects corruption, and decodes evidence", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{err: errors.New("missing"), exitCode: 1})
		actual, found, err := repository.LoadFinalQualityEvidence(context.Background(), identity)
		if err != nil || found || actual.SchemaVersion != 0 {
			t.Fatalf("absent evidence = (%#v, %t, %v)", actual, found, err)
		}

		repository, _ = coverageRepository(processResult{stdout: "not-json"})
		_, _, err = repository.LoadFinalQualityEvidence(context.Background(), identity)
		assertProblemCode(t, err, problem.CodeConfigurationInvalid)

		repository, _ = coverageRepository(processResult{err: errors.New("config failed"), exitCode: 128})
		_, _, err = repository.LoadFinalQualityEvidence(context.Background(), identity)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)

		encoded, err := json.Marshal(evidence)
		if err != nil {
			t.Fatal(err)
		}
		repository, runner := coverageRepository(processResult{stdout: string(encoded)})
		actual, found, err = repository.LoadFinalQualityEvidence(context.Background(), identity)
		if err != nil || !found || actual.Remote != evidence.Remote || actual.Updates[0] != evidence.Updates[0] {
			t.Fatalf("loaded evidence = (%#v, %t, %v)", actual, found, err)
		}
		assertCall(t, runner.calls[0], identity.Root, "", "config", "--local", "--get", finalQualityEvidenceConfigKey)
	})

	t.Run("stores one local Git metadata value and preserves write failures", func(t *testing.T) {
		repository, runner := coverageRepository(processResult{})
		if err := repository.StoreFinalQualityEvidence(context.Background(), identity, evidence); err != nil {
			t.Fatal(err)
		}
		if len(runner.calls) != 1 {
			t.Fatalf("store calls = %#v", runner.calls)
		}
		call := runner.calls[0]
		if call.directory != identity.Root ||
			strings.Join(call.arguments[:3], ",") != "config,--local,"+finalQualityEvidenceConfigKey {
			t.Fatalf("store call = %#v", call)
		}
		var stored port.FinalQualityEvidence
		if err := json.Unmarshal([]byte(call.arguments[3]), &stored); err != nil || stored.Remote != evidence.Remote {
			t.Fatalf("stored evidence = (%#v, %v)", stored, err)
		}

		storeErr := errors.New("write failed")
		repository, _ = coverageRepository(processResult{err: storeErr, exitCode: 128})
		err := repository.StoreFinalQualityEvidence(context.Background(), identity, evidence)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})
}
