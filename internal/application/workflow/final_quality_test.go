package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
)

type finalTicketGit struct {
	*fakeGitRepository

	revisions     map[string]string
	evidence      port.FinalQualityEvidence
	evidenceFound bool
}

func (git *finalTicketGit) ResolveRevision(_ context.Context, _ port.RepositoryIdentity, revision string) (string, error) {
	value, found := git.revisions[revision]
	if !found {
		return "", errors.New("missing revision " + revision)
	}
	return value, nil
}

func (git *finalTicketGit) LoadFinalQualityEvidence(
	context.Context,
	port.RepositoryIdentity,
) (port.FinalQualityEvidence, bool, error) {
	return git.evidence, git.evidenceFound, nil
}

func (git *finalTicketGit) StoreFinalQualityEvidence(
	_ context.Context,
	_ port.RepositoryIdentity,
	evidence port.FinalQualityEvidence,
) error {
	git.evidence = evidence
	git.evidenceFound = true
	return nil
}

type finalTicketQualityRunner struct {
	calls            int
	fingerprintCalls int
	err              error
	fingerprintErr   error
}

func (runner *finalTicketQualityRunner) Run(
	ctx context.Context,
	repository port.RepositoryIdentity,
	request port.QualityRequest,
) (port.QualityResult, error) {
	result, _, err := runner.RunWithFingerprint(ctx, repository, request)
	return result, err
}

func (runner *finalTicketQualityRunner) RunWithFingerprint(
	_ context.Context,
	_ port.RepositoryIdentity,
	_ port.QualityRequest,
) (port.QualityResult, port.QualityFingerprint, error) {
	runner.calls++
	if runner.err != nil {
		return port.QualityResult{}, port.QualityFingerprint{}, runner.err
	}
	return port.QualityResult{
			Status: port.QualityPassed,
			Gates:  []port.QualityGateResult{{Name: "full-local-build"}},
		},
		port.QualityFingerprint{
			ConfigurationDigest: "config",
			Gates:               []string{"full-local-build"},
			Toolchain:           "go-test/windows-amd64",
		},
		nil
}

func (runner *finalTicketQualityRunner) Fingerprint(
	_ context.Context,
	_ port.RepositoryIdentity,
	_ port.QualityRequest,
) (port.QualityFingerprint, error) {
	runner.fingerprintCalls++
	if runner.fingerprintErr != nil {
		return port.QualityFingerprint{}, runner.fingerprintErr
	}
	return port.QualityFingerprint{
		ConfigurationDigest: "config",
		Gates:               []string{"full-local-build"},
		Toolchain:           "go-test/windows-amd64",
	}, nil
}

func TestTicketServiceFinalQualityRunsAfterSynchronizationAndBeforePush(t *testing.T) {
	name := mustBranch("feature/ABC-123-add-export")
	base := mustBase("origin", "develop")
	git := newFinalTicketGit(name, base)
	runner := &finalTicketQualityRunner{}
	branches := branchapp.NewService(git, &fakeKeyPolicy{})
	finalQuality := branchapp.NewFinalQualityGate(git, runner)
	sync := branchapp.NewSynchronizer(git, branches, runner).WithFinalQualityGate(finalQuality)
	service := NewTicketService(branches, sync, git, runner, nil).WithFinalQualityGate(finalQuality)

	result, err := service.PublishTicket(context.Background(), PublishTicketRequest{
		Repository: testRepository(),
		Branch:     name,
	})
	if err != nil || result.Quality.Status != port.QualityPassed || result.Sync.Mutated || runner.calls != 1 || !git.evidenceFound {
		t.Fatalf("PublishTicket() = (%#v, %v), runs=%d evidence=%t", result, err, runner.calls, git.evidenceFound)
	}
	if err := service.PushPreparedTicket(context.Background(), testRepository(), name, &base, false); err != nil {
		t.Fatalf("PushPreparedTicket() error = %v", err)
	}
	if runner.calls != 1 || runner.fingerprintCalls != 1 {
		t.Fatalf("pre-push duplicated final quality: runs=%d fingerprints=%d", runner.calls, runner.fingerprintCalls)
	}
}

func TestTicketServiceFinalQualityCoversRebaseAndResumeCandidates(t *testing.T) {
	name := mustBranch("feature/ABC-123-add-export")
	base := mustBase("origin", "develop")

	t.Run("rebase candidate", func(t *testing.T) {
		git := newFinalTicketGit(name, base)
		git.missing = true
		runner := &finalTicketQualityRunner{}
		service := newFinalTicketService(git, runner)

		result, err := service.PublishTicket(context.Background(), PublishTicketRequest{
			Repository: testRepository(),
			Branch:     name,
		})
		if err != nil || !result.Sync.Mutated || result.PostMutationQuality == nil || runner.calls != 1 {
			t.Fatalf("rebased PublishTicket() = (%#v, %v), runs=%d", result, err, runner.calls)
		}
	})

	t.Run("resume candidate", func(t *testing.T) {
		git := newFinalTicketGit(name, base)
		runner := &finalTicketQualityRunner{}
		service := newFinalTicketService(git, runner)

		result, err := service.ResumeTicketPublish(context.Background(), ResumeTicketPublishRequest{
			Repository: testRepository(),
			Branch:     name,
			Base:       &base,
		})
		if err != nil || result.PostMutationQuality == nil || result.Quality.Status != port.QualityPassed || runner.calls != 1 {
			t.Fatalf("ResumeTicketPublish() = (%#v, %v), runs=%d", result, err, runner.calls)
		}
	})

	t.Run("final quality failure blocks publication", func(t *testing.T) {
		git := newFinalTicketGit(name, base)
		runner := &finalTicketQualityRunner{err: errors.New("quality failed")}
		service := newFinalTicketService(git, runner)

		_, err := service.PublishTicket(context.Background(), PublishTicketRequest{
			Repository: testRepository(),
			Branch:     name,
		})
		if !errors.Is(err, runner.err) {
			t.Fatalf("quality error = %v, want %v", err, runner.err)
		}
		if strings.Contains(strings.Join(git.calls, ","), "push") {
			t.Fatalf("failed final quality reached push: %v", git.calls)
		}
	})

	t.Run("resume final quality failure is preserved", func(t *testing.T) {
		git := newFinalTicketGit(name, base)
		runner := &finalTicketQualityRunner{err: errors.New("quality failed")}
		service := newFinalTicketService(git, runner)

		_, err := service.ResumeTicketPublish(context.Background(), ResumeTicketPublishRequest{
			Repository: testRepository(),
			Branch:     name,
			Base:       &base,
		})
		if !errors.Is(err, runner.err) {
			t.Fatalf("resume quality error = %v, want %v", err, runner.err)
		}
	})
}

func newFinalTicketGit(name branch.BranchName, base branch.TargetBase) *finalTicketGit {
	return &finalTicketGit{
		fakeGitRepository: &fakeGitRepository{
			clean:       true,
			publication: branch.PublicationUnpublished,
			messages:    []string{"feat(ABC-123): add export"},
		},
		revisions: map[string]string{
			"refs/heads/" + name.String(): strings.Repeat("a", 40),
			base.String():                 strings.Repeat("b", 40),
		},
	}
}

func newFinalTicketService(git *finalTicketGit, runner *finalTicketQualityRunner) *TicketService {
	branches := branchapp.NewService(git, &fakeKeyPolicy{})
	finalQuality := branchapp.NewFinalQualityGate(git, runner)
	sync := branchapp.NewSynchronizer(git, branches, runner).WithFinalQualityGate(finalQuality)
	return NewTicketService(branches, sync, git, runner, nil).WithFinalQualityGate(finalQuality)
}

var _ port.FinalQualityEvidenceStore = (*finalTicketGit)(nil)
var _ port.RevisionResolver = (*finalTicketGit)(nil)
var _ port.QualityEvidenceRunner = (*finalTicketQualityRunner)(nil)
