package bootstrap

import (
	"strings"
	"testing"
)

// TestWorktreeStartHelpStatesTheCallerCdDuty pins the worktree-start help
// contract: the generated leaf help states the caller duty to change into the
// reported worktreePath, because a child process cannot move the invoking
// shell's working directory. Canonical convention:
// docs/conventions/cli/help-contract.md.
func TestWorktreeStartHelpStatesTheCallerCdDuty(t *testing.T) {
	t.Parallel()

	command, _, err := New(BuildInfo{Version: "test"}).Find([]string{"workflow", "worktree", "start"})
	if err != nil || command == nil {
		t.Fatalf("workflow worktree start not found: %v", err)
	}
	const duty = "change into the reported worktreePath before the next in-worktree step"
	if !strings.Contains(command.Long, duty) {
		t.Fatalf("worktree start help misses the caller cd duty:\n%s", command.Long)
	}
}
