package workflow

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestBootstrapDryRun(t *testing.T) {
	t.Parallel()

	git := newBootstrapGit()
	request := bootstrapRequest()
	request.DryRun = true
	request.Push = true
	result, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun || result.Published {
		t.Fatalf("dry-run result = %#v", result)
	}
	for _, call := range git.calls {
		if call == "stage" || call == "commit" || call == "create-branch" || call == "install-hooks" || call == "push" {
			t.Fatalf("dry-run mutated the repository: calls = %v", git.calls)
		}
	}
	actions := make([]string, 0, len(result.Plan))
	for _, step := range result.Plan {
		actions = append(actions, step.Action)
	}
	joined := strings.Join(actions, ",")
	if joined != "preflight,stage,commit,create,install-hooks,finalize,publish" {
		t.Fatalf("dry-run plan = %q", joined)
	}
}

func TestBootstrapHappyPath(t *testing.T) {
	t.Parallel()

	git := newBootstrapGit()
	git.stagedQueue = []bool{false, true}
	result, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), bootstrapRequest())
	if err != nil {
		t.Fatal(err)
	}

	expectedOrder := []string{
		"has-commits", "has-any-ref", "current-branch", "active-operation", "staged",
		"version", "signing-configuration", "prove-signing", "preview-stage",
		"stage", "staged", "commit", "resolve-revision", "create-branch", "install-hooks",
		"validate-ref", "branch-exists", "resolve-revision", "validate-ref", "branch-exists", "resolve-revision",
		"verify-signature",
	}
	if strings.Join(git.calls, ",") != strings.Join(expectedOrder, ",") {
		t.Fatalf("call order = %v", git.calls)
	}
	if git.committed.Header().String() != "chore(GOV-116): initialize the governed repository" {
		t.Fatalf("genesis commit = %q", git.committed.Header().String())
	}
	if len(git.createdNames) != 1 || git.createdNames[0].String() != "develop" {
		t.Fatalf("created branches = %v", git.createdNames)
	}
	record := result.Record
	if record.Ticket != "GOV-116" || record.Revision != bootstrapTestRevision ||
		len(record.Refs) != 2 || record.Refs[0] != "main" || record.Refs[1] != "develop" ||
		!record.SignatureVerified || record.Actor != "lane@example.invalid" ||
		record.PolicySnapshot != "schemaVersion=1 keyPolicy=syntax-only commitSigning=required" ||
		!record.CreatedAt.Equal(time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("record = %#v", record)
	}
	if result.DryRun || result.Published {
		t.Fatalf("result = %#v", result)
	}
}

func TestBootstrapDefaultsTheRemote(t *testing.T) {
	t.Parallel()

	git := newBootstrapGit()
	git.stagedQueue = []bool{false, true}
	request := bootstrapRequest()
	request.Repository = port.RepositoryIdentity{Root: "C:/repo"}
	result, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Record.Remote != "origin" {
		t.Fatalf("record remote = %q", result.Record.Remote)
	}
}

func TestBootstrapMutationFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		mutate func(git *bootstrapGit)
		code   problem.Code
	}{
		{name: "stage", mutate: func(git *bootstrapGit) { git.stageErr = errors.New("stage failed") }},
		{name: "staged proof", mutate: func(git *bootstrapGit) {
			git.stagedQueue = []bool{false, true}
			git.stagedErrs = []error{nil, errors.New("staged proof failed")}
		}},
		{name: "empty index after staging", mutate: func(git *bootstrapGit) { git.stagedQueue = []bool{false, false} }},
		{name: "commit", mutate: func(git *bootstrapGit) { git.commitErr = errors.New("commit failed") }},
		{name: "revision resolution", mutate: func(git *bootstrapGit) { git.resolveErr = errors.New("resolve failed") }},
		{name: "develop creation", mutate: func(git *bootstrapGit) { git.createErr = errors.New("create failed") }},
		{name: "hook installation", mutate: func(git *bootstrapGit) { git.installErr = errors.New("install failed") }},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			git := newBootstrapGit()
			git.stagedQueue = []bool{false, true}
			testCase.mutate(git)
			_, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), bootstrapRequest())
			if err == nil {
				t.Fatal("expected the mutation failure to propagate")
			}
			if testCase.name == "empty index after staging" {
				assertProblemCode(t, err, problem.CodeGitCommandFailed)
			}
		})
	}
}

func TestBootstrapCompensatesAbortedGenesisSteps(t *testing.T) {
	t.Parallel()

	t.Run("an aborted mutation restores the proven pre-state", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.stagedQueue = []bool{false, true}
		git.stageErr = errors.New("stage failed")
		_, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), bootstrapRequest())
		if err == nil {
			t.Fatal("expected the mutation failure to propagate")
		}
		if !git.restored {
			t.Fatal("the aborted mutation did not restore the proven pre-state")
		}
		if err.Error() != "stage failed" {
			t.Fatalf("the compensation altered the untyped cause: %v", err)
		}
	})

	t.Run("a typed cause carries the compensation fact in its diagnostic", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.stagedQueue = []bool{false, true}
		git.stageErr = problem.New(problem.Details{
			Code:     problem.CodeGitCommandFailed,
			Category: problem.CategoryGit,
			Field:    "index",
			Rule:     "the staging step failed",
		})
		_, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), bootstrapRequest())
		if err == nil {
			t.Fatal("expected the mutation failure to propagate")
		}
		typed, ok := problem.As(err)
		if !ok {
			t.Fatalf("the typed cause lost its contract: %v", err)
		}
		if typed.Code != problem.CodeGitCommandFailed {
			t.Fatalf("the compensation changed the failure code: %v", typed.Code)
		}
		if !strings.Contains(typed.Details.Diagnostic, "compensated back to the proven unborn pre-state") {
			t.Fatalf("the compensation fact is missing: %q", typed.Details.Diagnostic)
		}
	})

	t.Run("an existing diagnostic is preserved and extended", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.stagedQueue = []bool{false, true}
		git.stageErr = problem.New(problem.Details{
			Code:       problem.CodeGitCommandFailed,
			Category:   problem.CategoryGit,
			Field:      "index",
			Diagnostic: "the staging process died",
			Rule:       "the staging step failed",
		})
		_, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), bootstrapRequest())
		if err == nil {
			t.Fatal("expected the mutation failure to propagate")
		}
		typed, _ := problem.As(err)
		if !strings.HasPrefix(typed.Details.Diagnostic, "the staging process died; ") {
			t.Fatalf("the existing diagnostic was not preserved: %q", typed.Details.Diagnostic)
		}
	})

	t.Run("a finalizer failure is compensated", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.stagedQueue = []bool{false, true}
		git.verifyErr = errors.New("unsigned")
		_, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), bootstrapRequest())
		if err == nil {
			t.Fatal("expected the finalizer failure to propagate")
		}
		if !git.restored {
			t.Fatal("the finalizer failure did not restore the proven pre-state")
		}
	})

	t.Run("a failed compensation blocks the retry", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.stagedQueue = []bool{false, true}
		git.stageErr = errors.New("stage failed")
		git.restoreErr = errors.New("index.lock persists")
		_, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), bootstrapRequest())
		if err == nil {
			t.Fatal("expected the compensation failure to block")
		}
		typed, ok := problem.As(err)
		if !ok || typed.Code != problem.CodeInternal {
			t.Fatalf("the failed compensation lost its blocking record: %v", err)
		}
		if typed.Field != "genesis compensation" {
			t.Fatalf("compensation field = %q", typed.Field)
		}
		if !strings.Contains(typed.Details.Diagnostic, "index.lock persists") {
			t.Fatalf("the restoration diagnostic is missing: %q", typed.Details.Diagnostic)
		}
		if !strings.Contains(typed.Details.Rule, "a failed compensation leaves the partial birth in place") {
			t.Fatalf("the blocking rule is missing: %q", typed.Details.Rule)
		}
		if !git.restored {
			t.Fatal("the compensation never attempted the restoration")
		}
	})

	t.Run("a successful birth never compensates", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.stagedQueue = []bool{false, true}
		if _, err := newBootstrapService(git, newBootstrapTools()).Bootstrap(context.Background(), bootstrapRequest()); err != nil {
			t.Fatal(err)
		}
		if git.restored {
			t.Fatal("the successful birth restored the pre-state")
		}
	})
}

func TestBootstrapFinalizerFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		mutate func(git *bootstrapGit, tools *bootstrapTools)
		code   problem.Code
	}{
		{
			name:   "governed validation",
			mutate: func(git *bootstrapGit, _ *bootstrapTools) { git.validateRefErr = errors.New("invalid ref") },
		},
		{
			name: "born ref existence",
			mutate: func(git *bootstrapGit, _ *bootstrapTools) {
				git.exists = map[string]bool{"main": true, "develop": false}
			},
			code: problem.CodeInternal,
		},
		{
			name: "born ref existence check failure",
			mutate: func(git *bootstrapGit, _ *bootstrapTools) {
				git.existsErr = errors.New("git failed")
			},
		},
		{
			name: "finalizer revision resolution failure",
			mutate: func(git *bootstrapGit, _ *bootstrapTools) {
				git.resolveFailOn = map[string]error{"main": errors.New("git failed")}
			},
		},
		{
			name: "shared revision equality",
			mutate: func(git *bootstrapGit, _ *bootstrapTools) {
				git.revisions = map[string]string{
					"HEAD":    bootstrapTestRevision,
					"main":    bootstrapTestRevision,
					"develop": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				}
			},
			code: problem.CodeInternal,
		},
		{
			name:   "signature verification",
			mutate: func(git *bootstrapGit, _ *bootstrapTools) { git.verifyErr = errors.New("unsigned") },
		},
		{
			name: "hook file proof",
			mutate: func(_ *bootstrapGit, tools *bootstrapTools) {
				tools.files[filepath.Join(testRepository().Root, ".git", "hooks", "pre-push")] = false
			},
			code: problem.CodeInternal,
		},
		{
			name: "hook file stat error",
			mutate: func(_ *bootstrapGit, tools *bootstrapTools) {
				tools.failOn = map[string]error{
					filepath.Join(testRepository().Root, ".git", "hooks", "commit-msg"): errors.New("stat failed"),
				}
			},
			code: problem.CodeExternalCommandFailed,
		},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			git := newBootstrapGit()
			git.stagedQueue = []bool{false, true}
			tools := newBootstrapTools()
			testCase.mutate(git, tools)
			_, err := newBootstrapService(git, tools).Bootstrap(context.Background(), bootstrapRequest())
			if err == nil {
				t.Fatal("expected the finalizer failure to propagate")
			}
			if testCase.code != "" {
				assertProblemCode(t, err, testCase.code)
			}
		})
	}
}

func TestBootstrapMessageCompositionFailure(t *testing.T) {
	t.Parallel()

	git := newBootstrapGit()
	service := newBootstrapService(git, newBootstrapTools()).
		WithPolicySnapshot(func() string { return "snapshot-with-\x01-control" })
	_, err := service.Bootstrap(context.Background(), bootstrapRequest())
	assertProblemCode(t, err, problem.CodeCommitDescriptionInvalid)
}
