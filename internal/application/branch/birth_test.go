package branchapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// birthCapableGit extends the shared fake with the optional capabilities the
// birth-topology proof consumes. The base fake stays capability-less so the
// fail-closed recognition path stays testable.
type birthCapableGit struct {
	*fakeGitRepository
	headCommits       int
	headCommitsErr    error
	resolvedRevision  string
	resolvedRevisions map[string]string
	resolveErr        error
	resolveErrsByRef  map[string]error
	signatureErr      error
	hookBoundary      bool
	hookBoundaryErr   error
}

func (fake *birthCapableGit) CountHeadCommits(ctx context.Context, repository port.RepositoryIdentity) (int, error) {
	fake.calls = append(fake.calls, "count-head-commits")
	return fake.headCommits, fake.methodError(fake.headCommitsErr)
}

func (fake *birthCapableGit) ResolveRevision(_ context.Context, _ port.RepositoryIdentity, revision string) (string, error) {
	fake.calls = append(fake.calls, "resolve-revision")
	if fake.resolveErrsByRef != nil {
		if resolveErr, has := fake.resolveErrsByRef[revision]; has {
			return "", resolveErr
		}
	}
	if fake.resolveErr != nil {
		return "", fake.resolveErr
	}
	if fake.resolvedRevisions != nil {
		if resolved, found := fake.resolvedRevisions[revision]; found {
			return resolved, nil
		}
	}
	return fake.resolvedRevision, nil
}

func (fake *birthCapableGit) VerifyCommitSignature(ctx context.Context, repository port.RepositoryIdentity, revision string) error {
	fake.calls = append(fake.calls, "verify-signature")
	return fake.methodError(fake.signatureErr)
}

func (fake *birthCapableGit) HookBoundaryPresent(ctx context.Context, repository port.RepositoryIdentity) (bool, error) {
	fake.calls = append(fake.calls, "hook-boundary")
	return fake.hookBoundary, fake.methodError(fake.hookBoundaryErr)
}

// birthCountingGit carries only the head commit counter capability.
type birthCountingGit struct {
	*fakeGitRepository
}

func (fake *birthCountingGit) CountHeadCommits(ctx context.Context, repository port.RepositoryIdentity) (int, error) {
	return 1, nil
}

// birthResolvingGit carries the head commit counter and the revision resolver.
type birthResolvingGit struct {
	*birthCountingGit
}

func (fake *birthResolvingGit) ResolveRevision(context.Context, port.RepositoryIdentity, string) (string, error) {
	return strings.Repeat("c", 40), nil
}

// birthSigningGit carries counter, resolver, and signature verifier.
type birthSigningGit struct {
	*birthResolvingGit
}

func (fake *birthSigningGit) VerifyCommitSignature(context.Context, port.RepositoryIdentity, string) error {
	return nil
}

func birthCapableGitFixture() *birthCapableGit {
	return &birthCapableGit{
		fakeGitRepository: &fakeGitRepository{
			hasCommits: true,
			exists:     true,
		},
		headCommits:      1,
		resolvedRevision: strings.Repeat("c", 40),
		hookBoundary:     true,
	}
}

func birthUpdates(t *testing.T, genesis string) []PushUpdate {
	t.Helper()
	main := pushUpdate(t, "refs/heads/main", "main", PushActionCreate)
	main.LocalObjectID = genesis
	develop := pushUpdate(t, "refs/heads/develop", "develop", PushActionCreate)
	develop.LocalObjectID = genesis
	return []PushUpdate{main, develop}
}

func TestResolveBirthTopologyCapabilities(t *testing.T) {
	t.Parallel()

	t.Run("the head commit counter is missing", func(t *testing.T) {
		t.Parallel()
		_, err := ResolveBirthTopologyCapabilities(&fakeGitRepository{})
		assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
	})

	t.Run("the revision resolver is missing", func(t *testing.T) {
		t.Parallel()
		_, err := ResolveBirthTopologyCapabilities(&birthCountingGit{fakeGitRepository: &fakeGitRepository{}})
		assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
	})

	t.Run("the signature verifier is missing", func(t *testing.T) {
		t.Parallel()
		_, err := ResolveBirthTopologyCapabilities(&birthResolvingGit{birthCountingGit: &birthCountingGit{fakeGitRepository: &fakeGitRepository{}}})
		assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
	})

	t.Run("the hook boundary inspector is missing", func(t *testing.T) {
		t.Parallel()
		_, err := ResolveBirthTopologyCapabilities(&birthSigningGit{birthResolvingGit: &birthResolvingGit{birthCountingGit: &birthCountingGit{fakeGitRepository: &fakeGitRepository{}}}})
		assertProblemCode(t, err, problem.CodeConfigurationUnavailable)
	})

	t.Run("the complete set resolves", func(t *testing.T) {
		t.Parallel()
		capabilities, err := ResolveBirthTopologyCapabilities(birthCapableGitFixture())
		if err != nil {
			t.Fatal(err)
		}
		if capabilities.Commits == nil || capabilities.Revisions == nil || capabilities.Verifier == nil || capabilities.Hooks == nil {
			t.Fatalf("capabilities = %#v", capabilities)
		}
	})
}

func TestBirthPublicationShape(t *testing.T) {
	t.Parallel()

	genesis := strings.Repeat("c", 40)
	mainCreate := pushUpdate(t, "refs/heads/main", "main", PushActionCreate)
	mainCreate.LocalObjectID = genesis
	developCreate := pushUpdate(t, "refs/heads/develop", "develop", PushActionCreate)
	developCreate.LocalObjectID = genesis
	featureCreate := pushUpdate(t, "refs/heads/feature/ABC-123-add-export", "feature/ABC-123-add-export", PushActionCreate)
	mainUpdate := pushUpdate(t, "HEAD", "main", PushActionUpdate)
	mainDelete := pushUpdate(t, "(delete)", "main", PushActionDelete)
	tagUpdate := PushUpdate{
		LocalRef:       "refs/tags/v1.0.0",
		LocalObjectID:  genesis,
		RemoteRef:      "refs/tags/v1.0.0",
		RemoteObjectID: strings.Repeat("0", 40),
		Action:         PushActionOther,
	}

	for name, testCase := range map[string]struct {
		updates []PushUpdate
		want    bool
	}{
		"empty batch":                {updates: nil, want: false},
		"tag ref":                    {updates: []PushUpdate{tagUpdate}, want: false},
		"shared line update":         {updates: []PushUpdate{mainUpdate}, want: false},
		"shared line deletion":       {updates: []PushUpdate{mainDelete}, want: false},
		"working branch rides along": {updates: []PushUpdate{mainCreate, featureCreate}, want: false},
		"single born line":           {updates: []PushUpdate{mainCreate}, want: true},
		"both born lines":            {updates: []PushUpdate{mainCreate, developCreate}, want: true},
	} {
		testCase := testCase
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := birthPublicationShape(testCase.updates); got != testCase.want {
				t.Fatalf("birthPublicationShape() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestValidatePrePushUpdatesRecognizesTheGovernedRemoteBirth(t *testing.T) {
	t.Parallel()

	git := birthCapableGitFixture()
	synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)

	result, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), birthUpdates(t, git.resolvedRevision), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Updates) != 2 {
		t.Fatalf("updates = %d, want 2", len(result.Updates))
	}
	for _, update := range result.Updates {
		if update.Publication != branch.PublicationUnpublished || !update.FastForward {
			t.Fatalf("birth update = %#v", update)
		}
	}
	if result.Quality.Status != port.QualitySkipped {
		t.Fatalf("quality = %#v", result.Quality)
	}
	if !strings.Contains(result.Quality.Detail, git.resolvedRevision) {
		t.Fatalf("quality detail = %q", result.Quality.Detail)
	}
	if got := strings.Join(git.calls, ","); got != "fetch,has-commits,count-head-commits,resolve-revision,validate-ref,branch-exists,resolve-revision,validate-ref,branch-exists,resolve-revision,verify-signature,hook-boundary" {
		t.Fatalf("calls = %q", got)
	}
}

func TestValidatePrePushUpdatesKeepsTheSharedLineGuardOutsideTheBirthPredicate(t *testing.T) {
	t.Parallel()

	t.Run("the repository is unborn", func(t *testing.T) {
		t.Parallel()
		git := birthCapableGitFixture()
		git.hasCommits = false
		synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		_, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), birthUpdates(t, git.resolvedRevision), nil)
		assertProblemCode(t, err, problem.CodeSharedLineMutationForbidden)
		if got := strings.Join(git.calls, ","); got != "fetch,has-commits,validate-ref" {
			t.Fatalf("calls = %q", got)
		}
	})

	t.Run("the head carries more than one commit", func(t *testing.T) {
		t.Parallel()
		git := birthCapableGitFixture()
		git.headCommits = 2
		synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		_, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), birthUpdates(t, git.resolvedRevision), nil)
		assertProblemCode(t, err, problem.CodeSharedLineMutationForbidden)
	})

	t.Run("a born ref does not exist", func(t *testing.T) {
		t.Parallel()
		git := birthCapableGitFixture()
		git.exists = false
		synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		_, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), birthUpdates(t, git.resolvedRevision), nil)
		assertProblemCode(t, err, problem.CodeSharedLineMutationForbidden)
	})

	t.Run("a born ref diverges from the genesis revision", func(t *testing.T) {
		t.Parallel()
		git := birthCapableGitFixture()
		git.resolvedRevisions = map[string]string{"develop": strings.Repeat("d", 40)}
		synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		_, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), birthUpdates(t, git.resolvedRevision), nil)
		assertProblemCode(t, err, problem.CodeSharedLineMutationForbidden)
	})

	t.Run("the hook boundary is missing", func(t *testing.T) {
		t.Parallel()
		git := birthCapableGitFixture()
		git.hookBoundary = false
		synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		_, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), birthUpdates(t, git.resolvedRevision), nil)
		assertProblemCode(t, err, problem.CodeSharedLineMutationForbidden)
	})

	t.Run("the pushed object id does not match the genesis revision", func(t *testing.T) {
		t.Parallel()
		git := birthCapableGitFixture()
		synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		updates := birthUpdates(t, strings.Repeat("e", 40))
		_, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), updates, nil)
		assertProblemCode(t, err, problem.CodeSharedLineMutationForbidden)
	})

	t.Run("a working branch rides along", func(t *testing.T) {
		t.Parallel()
		git := birthCapableGitFixture()
		synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		updates := append(birthUpdates(t, git.resolvedRevision), pushUpdate(t, "refs/heads/feature/ABC-123-add-export", "feature/ABC-123-add-export", PushActionCreate))
		_, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), updates, nil)
		assertProblemCode(t, err, problem.CodeSharedLineMutationForbidden)
		if got := strings.Join(git.calls, ","); got != "fetch,validate-ref" {
			t.Fatalf("calls = %q", got)
		}
	})

	t.Run("a shared line update rides along", func(t *testing.T) {
		t.Parallel()
		git := birthCapableGitFixture()
		synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		updates := append(birthUpdates(t, git.resolvedRevision), pushUpdate(t, "HEAD", "develop", PushActionUpdate))
		_, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), updates, nil)
		assertProblemCode(t, err, problem.CodeSharedLineMutationForbidden)
	})

	t.Run("the adapter cannot prove the birth state", func(t *testing.T) {
		t.Parallel()
		git := &fakeGitRepository{}
		synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		_, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), birthUpdates(t, strings.Repeat("c", 40)), nil)
		assertProblemCode(t, err, problem.CodeSharedLineMutationForbidden)
		if got := strings.Join(git.calls, ","); got != "fetch,validate-ref" {
			t.Fatalf("calls = %q", got)
		}
	})
}

func TestValidatePrePushUpdatesPropagatesBirthProofReadFailures(t *testing.T) {
	t.Parallel()

	t.Run("head commit counting fails", func(t *testing.T) {
		t.Parallel()
		git := birthCapableGitFixture()
		git.headCommitsErr = errors.New("count failed")
		synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		_, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), birthUpdates(t, git.resolvedRevision), nil)
		if !errors.Is(err, git.headCommitsErr) {
			t.Fatalf("error = %v, want %v", err, git.headCommitsErr)
		}
	})

	t.Run("the commit-state read fails", func(t *testing.T) {
		t.Parallel()
		git := birthCapableGitFixture()
		git.hasCommitsErr = errors.New("commit state read failed")
		synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		_, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), birthUpdates(t, git.resolvedRevision), nil)
		if !errors.Is(err, git.hasCommitsErr) {
			t.Fatalf("error = %v, want %v", err, git.hasCommitsErr)
		}
	})

	t.Run("the born-ref revision read fails", func(t *testing.T) {
		t.Parallel()
		git := birthCapableGitFixture()
		git.resolveErrsByRef = map[string]error{"develop": errors.New("develop resolve failed")}
		synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		_, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), birthUpdates(t, git.resolvedRevision), nil)
		if !errors.Is(err, git.resolveErrsByRef["develop"]) {
			t.Fatalf("error = %v, want %v", err, git.resolveErrsByRef["develop"])
		}
	})

	t.Run("revision resolution fails", func(t *testing.T) {
		t.Parallel()
		git := birthCapableGitFixture()
		git.resolveErr = errors.New("resolve failed")
		synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		_, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), birthUpdates(t, git.resolvedRevision), nil)
		if !errors.Is(err, git.resolveErr) {
			t.Fatalf("error = %v, want %v", err, git.resolveErr)
		}
	})

	t.Run("born ref existence fails", func(t *testing.T) {
		t.Parallel()
		git := birthCapableGitFixture()
		git.branchExistsErr = errors.New("existence read failed")
		synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		_, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), birthUpdates(t, git.resolvedRevision), nil)
		if !errors.Is(err, git.branchExistsErr) {
			t.Fatalf("error = %v, want %v", err, git.branchExistsErr)
		}
	})

	t.Run("shared line validation fails inside the proof", func(t *testing.T) {
		t.Parallel()
		git := birthCapableGitFixture()
		git.validateRefErr = errors.New("shared line ref is invalid")
		synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		_, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), birthUpdates(t, git.resolvedRevision), nil)
		if !errors.Is(err, git.validateRefErr) {
			t.Fatalf("error = %v, want %v", err, git.validateRefErr)
		}
	})

	t.Run("signature verification fails", func(t *testing.T) {
		t.Parallel()
		git := birthCapableGitFixture()
		git.signatureErr = errors.New("signature read failed")
		synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		_, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), birthUpdates(t, git.resolvedRevision), nil)
		if !errors.Is(err, git.signatureErr) {
			t.Fatalf("error = %v, want %v", err, git.signatureErr)
		}
	})

	t.Run("hook boundary inspection fails", func(t *testing.T) {
		t.Parallel()
		git := birthCapableGitFixture()
		git.hookBoundaryErr = errors.New("hook read failed")
		synchronizer := NewSynchronizer(git, NewService(git, &fakeKeyPolicy{}), nil)
		_, err := synchronizer.ValidatePrePushUpdates(context.Background(), testRepository(), birthUpdates(t, git.resolvedRevision), nil)
		if !errors.Is(err, git.hookBoundaryErr) {
			t.Fatalf("error = %v, want %v", err, git.hookBoundaryErr)
		}
	})
}
