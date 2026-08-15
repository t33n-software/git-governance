package integration_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/t33n-software/git-governance/internal/adapters/gitcli"
	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	"github.com/t33n-software/git-governance/internal/application/policy"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

func TestGitCLIAdapterAgainstLocalRepositories(t *testing.T) {
	t.Parallel()

	local, remote := setupRepository(t)
	adapter := gitcli.New(gitcli.Options{Timeout: 10 * time.Second})
	ctx := context.Background()

	identity, err := adapter.Discover(ctx, local)
	if err != nil {
		t.Fatal(err)
	}
	identity.Remote = "origin"
	if canonicalRepositoryPath(t, identity.Root) != canonicalRepositoryPath(t, local) {
		t.Fatalf("Discover() root = %q, want %q", identity.Root, local)
	}
	if hasCommits, err := adapter.HasCommits(ctx, identity); err != nil || !hasCommits {
		t.Fatalf("HasCommits() = (%t, %v)", hasCommits, err)
	}
	if clean, err := adapter.IsWorktreeClean(ctx, identity); err != nil || !clean {
		t.Fatalf("IsWorktreeClean() = (%t, %v)", clean, err)
	}

	develop := mustBranch(t, "develop")
	base, err := branch.NewTargetBase("origin", develop)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Fetch(ctx, identity); err != nil {
		t.Fatal(err)
	}

	feature := mustBranch(t, "feature/ABC-123-add-export")
	if err := adapter.ValidateBranchRef(ctx, identity, feature); err != nil {
		t.Fatal(err)
	}
	if exists, err := adapter.BranchExists(ctx, identity, feature); err != nil || exists {
		t.Fatalf("BranchExists(before) = (%t, %v)", exists, err)
	}
	if err := adapter.CreateBranch(ctx, identity, feature, base, true); err != nil {
		t.Fatal(err)
	}
	duplicateTicket, err := ticket.ParseID("ABC-123")
	if err != nil {
		t.Fatal(err)
	}
	duplicateSlug, err := branch.ParseSlug("another-ticket-branch")
	if err != nil {
		t.Fatal(err)
	}
	_, err = branchapp.NewService(adapter, policy.SyntaxOnlyKeyPolicy{}).Create(ctx, branchapp.CreateRequest{
		Repository: identity,
		Family:     branch.FamilyFix,
		Ticket:     duplicateTicket,
		Slug:       duplicateSlug,
	})
	assertProblemCode(t, err, problem.CodeTicketBranchAlreadyExists)
	current, err := adapter.CurrentBranch(ctx, identity)
	if err != nil || current.String() != feature.String() {
		t.Fatalf("CurrentBranch() = (%q, %v)", current.String(), err)
	}
	if publication, err := adapter.PublicationState(ctx, identity, feature); err != nil || publication != branch.PublicationUnpublished {
		t.Fatalf("PublicationState(before push) = (%q, %v)", publication, err)
	}

	writeFile(t, filepath.Join(local, "feature.txt"), "feature implementation\n")
	if err := adapter.Stage(ctx, identity, []string{"feature.txt"}); err != nil {
		t.Fatal(err)
	}
	if staged, err := adapter.HasStagedChanges(ctx, identity); err != nil || !staged {
		t.Fatalf("HasStagedChanges() = (%t, %v)", staged, err)
	}
	featureMessage := mustMessage(t, "feat(ABC-123): add export")
	if err := adapter.Commit(ctx, identity, featureMessage); err != nil {
		t.Fatal(err)
	}

	localObjectID := strings.TrimSpace(runGit(t, local, "rev-parse", "HEAD"))
	updates, err := branchapp.ParsePrePushUpdates(strings.NewReader(
		"refs/heads/feature/ABC-123-add-export " + localObjectID +
			" refs/heads/feature/ABC-123-add-export " + strings.Repeat("0", 40),
	))
	if err != nil {
		t.Fatal(err)
	}
	synchronizer := branchapp.NewSynchronizer(
		adapter,
		branchapp.NewService(adapter, policy.SyntaxOnlyKeyPolicy{}),
		nil,
	)
	checked, err := synchronizer.ValidatePrePushUpdates(ctx, identity, updates, nil)
	if err != nil || len(checked.Updates) != 1 || checked.Updates[0].Update.Target.String() != feature.String() {
		t.Fatalf("ValidatePrePushUpdates(feature) = (%#v, %v)", checked, err)
	}

	mainObjectID := strings.TrimSpace(runGit(t, local, "rev-parse", "origin/main"))
	sharedUpdates, err := branchapp.ParsePrePushUpdates(strings.NewReader(
		"HEAD " + localObjectID + " refs/heads/main " + mainObjectID,
	))
	if err != nil {
		t.Fatal(err)
	}
	_, err = synchronizer.ValidatePrePushUpdates(ctx, identity, sharedUpdates, nil)
	assertProblemCode(t, err, problem.CodeSharedLineMutationForbidden)

	if err := adapter.Push(ctx, identity, feature, true); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Fetch(ctx, identity); err != nil {
		t.Fatal(err)
	}
	if publication, err := adapter.PublicationState(ctx, identity, feature); err != nil || publication != branch.PublicationPublished {
		t.Fatalf("PublicationState(after push) = (%q, %v)", publication, err)
	}
	messages, err := adapter.CommitMessagesSince(ctx, identity, base)
	if err != nil || len(messages) != 1 || !strings.Contains(messages[0], featureMessage.Header().String()) {
		t.Fatalf("CommitMessagesSince() = (%q, %v)", messages, err)
	}

	advanceRemoteDevelop(t, remote, "upstream-one.txt", "one")
	if err := adapter.Fetch(ctx, identity); err != nil {
		t.Fatal(err)
	}
	if missing, err := adapter.HasMissingBaseCommits(ctx, identity, base); err != nil || !missing {
		t.Fatalf("HasMissingBaseCommits(after advance) = (%t, %v)", missing, err)
	}
	mergeMessage := mustMessage(t, "chore(ABC-123): merge origin/develop")
	if err := adapter.Merge(ctx, identity, base, mergeMessage); err != nil {
		t.Fatal(err)
	}
	if missing, err := adapter.HasMissingBaseCommits(ctx, identity, base); err != nil || missing {
		t.Fatalf("HasMissingBaseCommits(after merge) = (%t, %v)", missing, err)
	}

	fix := mustBranch(t, "fix/ABC-124-rebase-check")
	if err := adapter.CreateBranch(ctx, identity, fix, base, true); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(local, "fix.txt"), "fix implementation\n")
	if err := adapter.Stage(ctx, identity, []string{"fix.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Commit(ctx, identity, mustMessage(t, "fix(ABC-124): add rebase coverage")); err != nil {
		t.Fatal(err)
	}
	advanceRemoteDevelop(t, remote, "upstream-two.txt", "two")
	if err := adapter.Fetch(ctx, identity); err != nil {
		t.Fatal(err)
	}
	if missing, err := adapter.HasMissingBaseCommits(ctx, identity, base); err != nil || !missing {
		t.Fatalf("HasMissingBaseCommits(before rebase) = (%t, %v)", missing, err)
	}
	if err := adapter.Rebase(ctx, identity, base); err != nil {
		t.Fatal(err)
	}
	if missing, err := adapter.HasMissingBaseCommits(ctx, identity, base); err != nil || missing {
		t.Fatalf("HasMissingBaseCommits(after rebase) = (%t, %v)", missing, err)
	}
}

func TestScratchSquashMergeAgainstLocalRepository(t *testing.T) {
	t.Parallel()

	local, _ := setupRepository(t)
	adapter := gitcli.New(gitcli.Options{Timeout: 10 * time.Second})
	ctx := context.Background()
	identity, err := adapter.Discover(ctx, local)
	if err != nil {
		t.Fatal(err)
	}
	identity.Remote = "origin"
	if err := adapter.Fetch(ctx, identity); err != nil {
		t.Fatal(err)
	}

	develop := mustBranch(t, "develop")
	base, err := branch.NewTargetBase("origin", develop)
	if err != nil {
		t.Fatal(err)
	}
	target := mustBranch(t, "feature/ABC-123-add-export")
	if err := adapter.CreateBranch(ctx, identity, target, base, true); err != nil {
		t.Fatal(err)
	}
	localTarget, err := branch.NewLocalBase(target)
	if err != nil {
		t.Fatal(err)
	}
	source := mustBranch(t, "scratch/ABC-123-export-exploration")
	if err := adapter.CreateBranch(ctx, identity, source, localTarget, true); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(local, "scratch.txt"), "explored implementation\n")
	if err := adapter.Stage(ctx, identity, []string{"scratch.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Commit(ctx, identity, mustMessage(t, "feat(ABC-123): explore export implementation")); err != nil {
		t.Fatal(err)
	}

	merger := branchapp.NewScratchMerger(
		adapter,
		branchapp.NewService(adapter, policy.SyntaxOnlyKeyPolicy{}),
	)
	result, err := merger.Merge(ctx, branchapp.ScratchMergeRequest{
		Repository: identity,
		Source:     source,
		Message:    mustMessage(t, "feat(ABC-123): add export"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Target != target || !result.Committed {
		t.Fatalf("scratch merge result = %#v", result)
	}
	current, err := adapter.CurrentBranch(ctx, identity)
	if err != nil || current != target {
		t.Fatalf("CurrentBranch() after squash = (%q, %v)", current.String(), err)
	}
	if contents, err := os.ReadFile(filepath.Join(local, "scratch.txt")); err != nil || strings.TrimSpace(string(contents)) != "explored implementation" {
		t.Fatalf("squashed file = (%q, %v)", contents, err)
	}
	if exists, err := adapter.BranchExists(ctx, identity, source); err != nil || !exists {
		t.Fatalf("scratch branch existence after transfer = (%t, %v)", exists, err)
	}
	history := runGit(t, local, "log", "--format=%s", target.String())
	if !strings.Contains(history, "feat(ABC-123): add export") {
		t.Fatalf("target history = %q", history)
	}
}

func TestGitCLIAdapterContinuesAResolvedRebase(t *testing.T) {
	t.Parallel()

	local, remote := setupRepository(t)
	adapter := gitcli.New(gitcli.Options{Timeout: 10 * time.Second})
	ctx := context.Background()
	identity, err := adapter.Discover(ctx, local)
	if err != nil {
		t.Fatal(err)
	}
	identity.Remote = "origin"
	if err := adapter.Fetch(ctx, identity); err != nil {
		t.Fatal(err)
	}

	develop := mustBranch(t, "develop")
	base, err := branch.NewTargetBase("origin", develop)
	if err != nil {
		t.Fatal(err)
	}
	working := mustBranch(t, "fix/ABC-125-resume-rebase")
	if err := adapter.CreateBranch(ctx, identity, working, base, true); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(local, "conflict.txt"), "local change\n")
	if err := adapter.Stage(ctx, identity, []string{"conflict.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Commit(ctx, identity, mustMessage(t, "fix(ABC-125): add conflicting change")); err != nil {
		t.Fatal(err)
	}

	advanceRemoteDevelop(t, remote, "conflict.txt", "remote change")
	if err := adapter.Fetch(ctx, identity); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Rebase(ctx, identity, base); err == nil {
		t.Fatal("Rebase() unexpectedly completed without a conflict")
	}
	operation, active, err := adapter.ActiveOperation(ctx, identity)
	if err != nil || !active || operation != "rebase" {
		t.Fatalf("ActiveOperation() after conflict = (%q, %t, %v)", operation, active, err)
	}
	if conflicts, err := adapter.HasUnmergedConflicts(ctx, identity); err != nil || !conflicts {
		t.Fatalf("HasUnmergedConflicts() after conflict = (%t, %v)", conflicts, err)
	}

	writeFile(t, filepath.Join(local, "conflict.txt"), "resolved change\n")
	if err := adapter.Stage(ctx, identity, []string{"conflict.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := adapter.ContinueRebase(ctx, identity); err != nil {
		t.Fatalf("ContinueRebase() error = %v", err)
	}
	operation, active, err = adapter.ActiveOperation(ctx, identity)
	if err != nil || active || operation != "" {
		t.Fatalf("ActiveOperation() after continuation = (%q, %t, %v)", operation, active, err)
	}
	if conflicts, err := adapter.HasUnmergedConflicts(ctx, identity); err != nil || conflicts {
		t.Fatalf("HasUnmergedConflicts() after continuation = (%t, %v)", conflicts, err)
	}
	current, err := adapter.CurrentBranch(ctx, identity)
	if err != nil || current != working {
		t.Fatalf("CurrentBranch() after continuation = (%q, %v)", current.String(), err)
	}
}

func setupRepository(t *testing.T) (string, string) {
	t.Helper()

	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	runGit(t, root, "init", "--bare", remote)

	local := filepath.Join(root, "local")
	if err := os.MkdirAll(local, 0o700); err != nil {
		t.Fatal(err)
	}
	runGit(t, local, "init", "--initial-branch=main")
	configureGitIdentity(t, local)
	writeFile(t, filepath.Join(local, "README.md"), "initial\n")
	runGit(t, local, "add", "--", "README.md")
	runGit(t, local, "commit", "-m", "chore(ABC-1): initialize repository")
	runGit(t, local, "remote", "add", "origin", remote)
	runGit(t, local, "push", "--set-upstream", "origin", "main")
	runGit(t, local, "switch", "-c", "develop")
	runGit(t, local, "push", "--set-upstream", "origin", "develop")
	return local, remote
}

func canonicalRepositoryPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(filepath.Clean(filepath.FromSlash(path)))
	if err != nil {
		t.Fatalf("resolve repository path %q: %v", path, err)
	}
	return filepath.Clean(resolved)
}

func advanceRemoteDevelop(t *testing.T, remote, fileName, contents string) {
	t.Helper()

	other := filepath.Join(t.TempDir(), "other")
	runGit(t, filepath.Dir(other), "clone", remote, other)
	configureGitIdentity(t, other)
	runGit(t, other, "switch", "develop")
	writeFile(t, filepath.Join(other, fileName), contents+"\n")
	runGit(t, other, "add", "--", fileName)
	runGit(t, other, "commit", "-m", "chore(ABC-1): advance develop")
	runGit(t, other, "push", "origin", "develop")
}

func configureGitIdentity(t *testing.T, directory string) {
	t.Helper()
	runGit(t, directory, "config", "user.name", "Integration Test")
	runGit(t, directory, "config", "user.email", "integration@example.invalid")
}

func runGit(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s failed: %v\n%s", strings.Join(arguments, " "), directory, err, output)
	}
	return string(output)
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustBranch(t *testing.T, raw string) branch.BranchName {
	t.Helper()
	value, err := branch.ParseName(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func mustMessage(t *testing.T, raw string) commitmsg.Message {
	t.Helper()
	value, err := commitmsg.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func assertProblemCode(t *testing.T, err error, expected problem.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected problem code %q, got nil", expected)
	}
	actual, ok := problem.As(err)
	if !ok {
		t.Fatalf("error %T does not carry a problem: %v", err, err)
	}
	if actual.Code != expected {
		t.Fatalf("problem code = %q, want %q", actual.Code, expected)
	}
}

var _ port.GitRepository = (*gitcli.Repository)(nil)
