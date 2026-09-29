package integration_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/t33n-software/git-governance/internal/adapters/gitcli"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// TestGitCLIAdapterBootstrapOperations proves the governed repository birth
// against a real unborn temporary repository: the unborn proofs, the content
// set preview, the genesis commit, the develop birth from the same revision,
// the fail-closed signature verification, the hook boundary installation, and
// the remote birth of both shared lines.
func TestGitCLIAdapterBootstrapOperations(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	runGit(t, root, "init", "--bare", remote)

	local := filepath.Join(root, "local")
	if err := os.MkdirAll(local, 0o700); err != nil {
		t.Fatal(err)
	}
	runGit(t, local, "init", "--initial-branch=main")
	configureGitIdentity(t, local)
	// The genesis commit of this fixture is deliberately unsigned, so the
	// signature verification proof is deterministic on every host.
	runGit(t, local, "config", "commit.gpgsign", "false")

	adapter := gitcli.New(gitcli.Options{Timeout: integrationGitTimeout})
	ctx := context.Background()
	identity, err := adapter.Discover(ctx, local)
	if err != nil {
		t.Fatal(err)
	}
	identity.Remote = "origin"

	if hasCommits, err := adapter.HasCommits(ctx, identity); err != nil || hasCommits {
		t.Fatalf("HasCommits(unborn) = (%t, %v)", hasCommits, err)
	}
	if anyRef, err := adapter.HasAnyRef(ctx, identity); err != nil || anyRef {
		t.Fatalf("HasAnyRef(unborn) = (%t, %v)", anyRef, err)
	}
	current, err := adapter.CurrentBranch(ctx, identity)
	if err != nil || current.String() != "main" {
		t.Fatalf("CurrentBranch(unborn) = (%q, %v)", current.String(), err)
	}

	writeFile(t, filepath.Join(local, ".gitignore"), "secret.key\n")
	writeFile(t, filepath.Join(local, "secret.key"), "private\n")
	writeFile(t, filepath.Join(local, "README.md"), "initial\n")
	preview, err := adapter.PreviewStage(ctx, identity, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(preview, ",")
	if !strings.Contains(joined, "README.md") || !strings.Contains(joined, ".gitignore") {
		t.Fatalf("PreviewStage() = %v", preview)
	}
	if strings.Contains(joined, "secret.key") {
		t.Fatalf("PreviewStage() must respect the standard exclusions: %v", preview)
	}

	if err := adapter.Stage(ctx, identity, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if staged, err := adapter.HasStagedChanges(ctx, identity); err != nil || !staged {
		t.Fatalf("HasStagedChanges(after stage) = (%t, %v)", staged, err)
	}
	if err := adapter.Commit(ctx, identity, mustMessage(t, "chore(ABC-1): initialize repository")); err != nil {
		t.Fatal(err)
	}

	if hasCommits, err := adapter.HasCommits(ctx, identity); err != nil || !hasCommits {
		t.Fatalf("HasCommits(born) = (%t, %v)", hasCommits, err)
	}
	if anyRef, err := adapter.HasAnyRef(ctx, identity); err != nil || !anyRef {
		t.Fatalf("HasAnyRef(born) = (%t, %v)", anyRef, err)
	}
	revision, err := adapter.ResolveRevision(ctx, identity, "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	mainName := mustBranch(t, "main")
	develop := mustBranch(t, "develop")
	localBase, err := branch.NewLocalBase(mainName)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.CreateBranch(ctx, identity, develop, localBase, false); err != nil {
		t.Fatal(err)
	}
	developRevision, err := adapter.ResolveRevision(ctx, identity, "develop")
	if err != nil {
		t.Fatal(err)
	}
	if developRevision != revision {
		t.Fatalf("develop revision = %q, want the genesis revision %q", developRevision, revision)
	}

	// The unsigned integration commit proves the fail-closed signature gate.
	assertProblemCode(t, adapter.VerifyCommitSignature(ctx, identity, "HEAD"), problem.CodeCommitSignatureRequired)

	runGit(t, local, "remote", "add", "origin", remote)
	if err := adapter.Push(ctx, identity, mainName, true); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Push(ctx, identity, develop, true); err != nil {
		t.Fatal(err)
	}
	if remoteMain := strings.TrimSpace(runGit(t, remote, "rev-parse", "refs/heads/main")); remoteMain != revision {
		t.Fatalf("remote main = %q, want %q", remoteMain, revision)
	}
	if remoteDevelop := strings.TrimSpace(runGit(t, remote, "rev-parse", "refs/heads/develop")); remoteDevelop != revision {
		t.Fatalf("remote develop = %q, want %q", remoteDevelop, revision)
	}

	if _, err := exec.LookPath("lefthook"); err != nil {
		t.Skip("lefthook is not installed on this host")
	}
	writeFile(t, filepath.Join(local, "lefthook.yml"), "commit-msg:\n  commands:\n    validate:\n      run: echo ok\npre-push:\n  commands:\n    validate:\n      run: echo ok\n")
	installation, err := adapter.InstallHooks(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	if len(installation.Hooks) != 2 || installation.Hooks[0] != "commit-msg" || installation.Hooks[1] != "pre-push" {
		t.Fatalf("InstallHooks() = %#v", installation)
	}
	for _, hook := range installation.Hooks {
		if _, err := os.Stat(filepath.Join(installation.Directory, hook)); err != nil {
			t.Fatalf("installed hook %q: %v", hook, err)
		}
	}
}
