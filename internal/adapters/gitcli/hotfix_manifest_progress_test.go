package gitcli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/CyberT33N/git-governance/internal/application/port"
	"github.com/CyberT33N/git-governance/internal/domain/branch"
	"github.com/CyberT33N/git-governance/internal/domain/problem"
)

func TestRepositoryHotfixManifestProgressContracts(t *testing.T) {
	identity := testIdentity()
	progress := testHotfixManifestProgress(t)

	t.Run("loads absence corruption failures and valid progress", func(t *testing.T) {
		repository, _ := coverageRepository(processResult{err: errors.New("missing"), exitCode: 1})
		actual, found, err := repository.LoadHotfixManifestProgress(context.Background(), identity)
		if err != nil || found || actual.Next != 0 {
			t.Fatalf("absent progress = (%#v, %t, %v)", actual, found, err)
		}

		repository, _ = coverageRepository(processResult{stdout: "not-json"})
		_, _, err = repository.LoadHotfixManifestProgress(context.Background(), identity)
		assertProblemCode(t, err, problem.CodeConfigurationInvalid)

		invalidDocument, err := json.Marshal(hotfixManifestProgressDocument{
			Branch:   "invalid",
			Source:   "hotfix/ABC-999-payment-timeout",
			Target:   "develop",
			Manifest: []string{strings.Repeat("a", 40)},
		})
		if err != nil {
			t.Fatal(err)
		}
		repository, _ = coverageRepository(processResult{stdout: string(invalidDocument)})
		_, _, err = repository.LoadHotfixManifestProgress(context.Background(), identity)
		assertProblemCode(t, err, problem.CodeConfigurationInvalid)

		repository, _ = coverageRepository(processResult{err: errors.New("read failed"), exitCode: 128})
		_, _, err = repository.LoadHotfixManifestProgress(context.Background(), identity)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)

		document, err := newHotfixManifestProgressDocument(progress)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		repository, runner := coverageRepository(processResult{stdout: string(encoded)})
		actual, found, err = repository.LoadHotfixManifestProgress(context.Background(), identity)
		if err != nil || !found || actual.Branch.String() != progress.Branch.String() || actual.Next != progress.Next {
			t.Fatalf("loaded progress = (%#v, %t, %v)", actual, found, err)
		}
		assertCall(t, runner.calls[0], identity.Root, "", "config", "--local", "--get", hotfixManifestProgressConfigKey)
	})

	t.Run("stores valid progress and rejects invalid progress", func(t *testing.T) {
		repository, runner := coverageRepository(processResult{})
		if err := repository.StoreHotfixManifestProgress(context.Background(), identity, progress); err != nil {
			t.Fatal(err)
		}
		if len(runner.calls) != 1 {
			t.Fatalf("store calls = %#v", runner.calls)
		}
		call := runner.calls[0]
		if strings.Join(call.arguments[:3], ",") != "config,--local,"+hotfixManifestProgressConfigKey {
			t.Fatalf("store call = %#v", call)
		}
		var stored hotfixManifestProgressDocument
		if err := json.Unmarshal([]byte(call.arguments[3]), &stored); err != nil || stored.Next != progress.Next {
			t.Fatalf("stored progress = (%#v, %v)", stored, err)
		}

		invalid := progress
		invalid.Next = len(invalid.Manifest) + 1
		err := repository.StoreHotfixManifestProgress(context.Background(), identity, invalid)
		assertProblemCode(t, err, problem.CodeConfigurationInvalid)

		writeFailure := errors.New("write failed")
		repository, _ = coverageRepository(processResult{err: writeFailure, exitCode: 128})
		err = repository.StoreHotfixManifestProgress(context.Background(), identity, progress)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})

	t.Run("clears present absent and failed progress values", func(t *testing.T) {
		repository, runner := coverageRepository(processResult{})
		if err := repository.ClearHotfixManifestProgress(context.Background(), identity); err != nil {
			t.Fatal(err)
		}
		assertCall(t, runner.calls[0], identity.Root, "", "config", "--local", "--unset-all", hotfixManifestProgressConfigKey)

		repository, _ = coverageRepository(processResult{err: errors.New("absent"), exitCode: 1})
		if err := repository.ClearHotfixManifestProgress(context.Background(), identity); err != nil {
			t.Fatal(err)
		}

		repository, _ = coverageRepository(processResult{err: errors.New("clear failed"), exitCode: 128})
		err := repository.ClearHotfixManifestProgress(context.Background(), identity)
		assertProblemCode(t, err, problem.CodeGitCommandFailed)
	})
}

func TestHotfixManifestProgressDocumentRejectsInvalidState(t *testing.T) {
	valid := hotfixManifestProgressDocument{
		Branch:   "fix/ABC-999-propagate-to-develop",
		Source:   "hotfix/ABC-999-payment-timeout",
		Target:   "develop",
		Manifest: []string{strings.Repeat("a", 40)},
	}
	if _, err := valid.progress(); err != nil {
		t.Fatal(err)
	}

	for _, mutate := range []func(*hotfixManifestProgressDocument){
		func(document *hotfixManifestProgressDocument) { document.Branch = "invalid" },
		func(document *hotfixManifestProgressDocument) { document.Source = "invalid" },
		func(document *hotfixManifestProgressDocument) { document.Target = "invalid" },
		func(document *hotfixManifestProgressDocument) { document.Branch = "feature/ABC-999-wrong" },
		func(document *hotfixManifestProgressDocument) { document.Source = "fix/ABC-999-wrong" },
		func(document *hotfixManifestProgressDocument) { document.Target = "main" },
		func(document *hotfixManifestProgressDocument) { document.Manifest = nil },
		func(document *hotfixManifestProgressDocument) { document.Next = -1 },
		func(document *hotfixManifestProgressDocument) { document.Next = 2 },
		func(document *hotfixManifestProgressDocument) { document.Manifest = []string{"short"} },
		func(document *hotfixManifestProgressDocument) {
			document.Manifest = []string{strings.Repeat("a", 40), strings.Repeat("a", 40)}
		},
	} {
		document := valid
		document.Manifest = append([]string(nil), valid.Manifest...)
		mutate(&document)
		if _, err := document.progress(); err == nil {
			t.Fatalf("progress() accepted %#v", document)
		}
	}
}

func testHotfixManifestProgress(t *testing.T) port.HotfixManifestProgress {
	t.Helper()

	branchName, err := branch.ParseName("fix/ABC-999-propagate-to-develop")
	if err != nil {
		t.Fatal(err)
	}
	source, err := branch.ParseName("hotfix/ABC-999-payment-timeout")
	if err != nil {
		t.Fatal(err)
	}
	target, err := branch.ParseName("develop")
	if err != nil {
		t.Fatal(err)
	}
	return port.HotfixManifestProgress{
		Branch:   branchName,
		Source:   source,
		Target:   target,
		Manifest: []string{strings.Repeat("a", 40)},
	}
}
