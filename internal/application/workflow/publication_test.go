package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// resumeCommitsGit carries only the head-commit-counter capability.
type resumeCommitsGit struct{ fakeGitRepository }

func (fake *resumeCommitsGit) CountHeadCommits(context.Context, port.RepositoryIdentity) (int, error) {
	return 1, nil
}

// resumeRevisionsGit adds the revision-resolution capability.
type resumeRevisionsGit struct{ resumeCommitsGit }

func (fake *resumeRevisionsGit) ResolveRevision(context.Context, port.RepositoryIdentity, string) (string, error) {
	return bootstrapTestRevision, nil
}

// resumeVerifierGit adds the signature-verification capability.
type resumeVerifierGit struct{ resumeRevisionsGit }

func (fake *resumeVerifierGit) VerifyCommitSignature(context.Context, port.RepositoryIdentity, string) error {
	return nil
}

func TestPublishBornLines(t *testing.T) {
	t.Parallel()

	t.Run("guards dependencies and inputs", func(t *testing.T) {
		t.Parallel()
		if err := NewBootstrapService(nil, nil, nil, nil).PublishBornLines(context.Background(), testRepository()); err == nil {
			t.Fatal("expected the missing-dependency guard")
		}
		git := newBootstrapGit()
		if err := newBootstrapService(git, newBootstrapTools()).PublishBornLines(context.Background(), port.RepositoryIdentity{}); err == nil {
			t.Fatal("expected the repository guard")
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := newBootstrapService(git, newBootstrapTools()).PublishBornLines(ctx, testRepository()); err == nil {
			t.Fatal("expected the cancelled context guard")
		}
	})

	t.Run("unborn lines are refused", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.exists = map[string]bool{"main": true, "develop": false}
		err := newBootstrapService(git, newBootstrapTools()).PublishBornLines(context.Background(), testRepository())
		assertProblemCode(t, err, problem.CodeRepositoryNotFound)
	})

	t.Run("existence failures propagate", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.existsErr = errors.New("git failed")
		if err := newBootstrapService(git, newBootstrapTools()).PublishBornLines(context.Background(), testRepository()); err == nil {
			t.Fatal("expected the existence failure to propagate")
		}
	})

	t.Run("push failures propagate", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.pushErr = errors.New("push failed")
		if err := newBootstrapService(git, newBootstrapTools()).PublishBornLines(context.Background(), testRepository()); err == nil {
			t.Fatal("expected the push failure to propagate")
		}
	})

	t.Run("pushes main before develop with upstream configuration", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		if err := newBootstrapService(git, newBootstrapTools()).PublishBornLines(context.Background(), testRepository()); err != nil {
			t.Fatal(err)
		}
		if len(git.pushed) != 2 || git.pushed[0].String() != "main" || git.pushed[1].String() != "develop" {
			t.Fatalf("pushed = %v", git.pushed)
		}
	})
}

func TestPublishResumeCapabilityResolution(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		git  port.GitRepository
	}{
		{name: "head commit counting missing", git: &fakeGitRepository{}},
		{name: "revision resolution missing", git: &resumeCommitsGit{}},
		{name: "signature verification missing", git: &resumeRevisionsGit{}},
		{name: "hook boundary inspection missing", git: &resumeVerifierGit{}},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := newBootstrapService(testCase.git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{
				Repository: testRepository(),
			})
			assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
		})
	}
}

func TestPublishResumeBirthTopologyProof(t *testing.T) {
	t.Parallel()

	t.Run("guards dependencies and inputs", func(t *testing.T) {
		t.Parallel()
		if _, err := NewBootstrapService(nil, nil, nil, nil).PublishResume(context.Background(), PublishResumeRequest{Repository: testRepository()}); err == nil {
			t.Fatal("expected the missing-dependency guard")
		}
		if _, err := newBootstrapService(newBootstrapGit(), newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{}); err == nil {
			t.Fatal("expected the repository guard")
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := newBootstrapService(newBootstrapGit(), newBootstrapTools()).PublishResume(ctx, PublishResumeRequest{Repository: testRepository()}); err == nil {
			t.Fatal("expected the cancelled context guard")
		}
	})

	t.Run("an unborn repository is refused", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.hasCommits = false
		_, err := newBootstrapService(git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeBirthStateInvalid)
		if len(git.pushed) != 0 {
			t.Fatalf("the refused resume pushed: %v", git.pushed)
		}
	})

	t.Run("a multi-commit repository is refused", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.hasCommits = true
		git.headCommits = 3
		_, err := newBootstrapService(git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeBirthStateInvalid)
	})

	t.Run("a missing shared line is refused", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.hasCommits = true
		git.exists = map[string]bool{"main": true, "develop": false}
		_, err := newBootstrapService(git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeBirthStateInvalid)
	})

	t.Run("a diverged shared line is refused", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.hasCommits = true
		git.revisions = map[string]string{
			"HEAD":    bootstrapTestRevision,
			"main":    bootstrapTestRevision,
			"develop": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}
		_, err := newBootstrapService(git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeBirthStateInvalid)
	})

	t.Run("an unverifiable genesis signature is refused", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.hasCommits = true
		git.verifyErr = problem.New(problem.Details{
			Code:     problem.CodeCommitSignatureRequired,
			Category: problem.CategoryGovernance,
			Field:    "commit signature",
		})
		_, err := newBootstrapService(git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeCommitSignatureRequired)
	})

	t.Run("a missing hook boundary is refused", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.hasCommits = true
		git.hookBoundary = false
		_, err := newBootstrapService(git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeBirthStateInvalid)
	})

	t.Run("an unbound remote is refused before the push", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.hasCommits = true
		git.remoteURLErr = errors.New("no such remote")
		_, err := newBootstrapService(git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{Repository: testRepository()})
		assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
		if len(git.pushed) != 0 {
			t.Fatalf("the refused resume pushed: %v", git.pushed)
		}
	})

	t.Run("proof failures propagate without pushing", func(t *testing.T) {
		t.Parallel()
		testCases := []struct {
			name   string
			mutate func(git *bootstrapGit)
		}{
			{name: "has commits", mutate: func(git *bootstrapGit) { git.err = errors.New("git failed") }},
			{name: "head commit count", mutate: func(git *bootstrapGit) { git.countHeadErr = errors.New("git failed") }},
			{name: "head revision resolution", mutate: func(git *bootstrapGit) { git.resolveErr = errors.New("git failed") }},
			{name: "governed ref validation", mutate: func(git *bootstrapGit) { git.validateRefErr = errors.New("invalid ref") }},
			{name: "born ref existence check", mutate: func(git *bootstrapGit) { git.existsErr = errors.New("git failed") }},
			{name: "shared revision resolution", mutate: func(git *bootstrapGit) {
				git.resolveFailOn = map[string]error{"main": errors.New("git failed")}
			}},
			{name: "hook boundary inspection", mutate: func(git *bootstrapGit) { git.hookBndryErr = errors.New("git failed") }},
		}
		for _, testCase := range testCases {
			testCase := testCase
			t.Run(testCase.name, func(t *testing.T) {
				t.Parallel()
				git := newBootstrapGit()
				git.hasCommits = true
				testCase.mutate(git)
				if _, err := newBootstrapService(git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{Repository: testRepository()}); err == nil {
					t.Fatal("expected the proof failure to propagate")
				}
				if len(git.pushed) != 0 {
					t.Fatalf("the failed proof pushed: %v", git.pushed)
				}
			})
		}
	})
}

func TestPublishResumeExecution(t *testing.T) {
	t.Parallel()

	t.Run("the dry run proves the birth topology without pushing", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.hasCommits = true
		result, err := newBootstrapService(git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{
			Repository: testRepository(),
			DryRun:     true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !result.DryRun || result.Revision != bootstrapTestRevision || len(git.pushed) != 0 {
			t.Fatalf("the dry-run resume = %#v, pushed = %v", result, git.pushed)
		}
		actions := make([]string, 0, len(result.Plan))
		for _, step := range result.Plan {
			actions = append(actions, step.Action)
		}
		if strings.Join(actions, ",") != "prove-birth,publish" {
			t.Fatalf("dry-run plan = %q", strings.Join(actions, ","))
		}
	})

	t.Run("the proven birth publishes exactly the shared lines", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.hasCommits = true
		result, err := newBootstrapService(git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{Repository: testRepository()})
		if err != nil {
			t.Fatal(err)
		}
		if result.DryRun || result.Revision != bootstrapTestRevision {
			t.Fatalf("the resume result = %#v", result)
		}
		if len(git.pushed) != 2 || git.pushed[0].String() != "main" || git.pushed[1].String() != "develop" {
			t.Fatalf("pushed = %v", git.pushed)
		}
	})

	t.Run("a push failure propagates", func(t *testing.T) {
		t.Parallel()
		git := newBootstrapGit()
		git.hasCommits = true
		git.pushErr = errors.New("push failed")
		if _, err := newBootstrapService(git, newBootstrapTools()).PublishResume(context.Background(), PublishResumeRequest{Repository: testRepository()}); err == nil {
			t.Fatal("expected the push failure to propagate")
		}
	})
}
