package bootstrap

import (
	"errors"
	"strings"
	"testing"
)

func TestTicketInventoryCommand(t *testing.T) {
	t.Parallel()

	t.Run("reports the derived allocation inventory as JSON", func(t *testing.T) {
		t.Parallel()
		git := newCommandGit(t, "develop", nil)
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		args := []string{"--interactive", "never", "--output", "json", "workflow", "ticket", "inventory", "--key", "ABC"}

		output, err := executeBootstrapCommand(t, command, args...)
		if err != nil {
			t.Fatalf("inventory command error = %v; output=%q", err, output)
		}
		for _, expected := range []string{
			`"ok":true`,
			`"key":"ABC"`,
			`"nextFree":"1"`,
			`"surface":"branch refs","state":"scanned"`,
			`"surface":"commit envelopes","state":"scanned"`,
			`"surface":"hotfix release records","state":"scanned"`,
			`"surface":"pull request titles","state":"absent"`,
			`"surface":"protected-line request records","state":"absent"`,
		} {
			if !strings.Contains(output, expected) {
				t.Fatalf("inventory output missing %q: %q", expected, output)
			}
		}
	})

	t.Run("requires a ticket key in non-interactive runs", func(t *testing.T) {
		t.Parallel()
		git := newCommandGit(t, "develop", nil)
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		args := []string{"--interactive", "never", "--output", "json", "workflow", "ticket", "inventory"}

		output, err := executeBootstrapCommand(t, command, args...)
		if err == nil {
			t.Fatalf("inventory without a key unexpectedly succeeded: %q", output)
		}
		if !strings.Contains(err.Error(), "ticket key") {
			t.Fatalf("missing-key failure must name the ticket key input: %v", err)
		}
	})

	t.Run("reports held numbers with holder evidence", func(t *testing.T) {
		t.Parallel()
		git := newCommandGit(t, "feature/ABC-123-add-export", []string{"feat(ABC-123): add export"})
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		args := []string{"--interactive", "never", "--output", "json", "--yes", "workflow", "ticket", "inventory", "--key", "ABC"}

		output, err := executeBootstrapCommand(t, command, args...)
		if err != nil {
			t.Fatalf("inventory command error = %v; output=%q", err, output)
		}
		for _, expected := range []string{
			`"nextFree":"124"`,
			`"123"`,
			`local branch ref feature/ABC-123-add-export`,
			`feat(ABC-123): add export`,
		} {
			if !strings.Contains(output, expected) {
				t.Fatalf("holder evidence missing %q: %q", expected, output)
			}
		}
	})

	t.Run("skips the fetch in dry-run and still reports the inventory", func(t *testing.T) {
		t.Parallel()
		git := newCommandGit(t, "develop", nil)
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		args := []string{"--interactive", "never", "--output", "json", "--dry-run", "workflow", "ticket", "inventory", "--key", "ABC"}

		output, err := executeBootstrapCommand(t, command, args...)
		if err != nil {
			t.Fatalf("inventory command error = %v; output=%q", err, output)
		}
		for _, expected := range []string{
			`"ok":true`,
			`"key":"ABC"`,
			`"nextFree":"1"`,
		} {
			if !strings.Contains(output, expected) {
				t.Fatalf("dry-run inventory output missing %q: %q", expected, output)
			}
		}
	})

	t.Run("fails when the repository cannot be discovered", func(t *testing.T) {
		t.Parallel()
		git := newCommandGit(t, "develop", nil)
		git.discoverErr = errors.New("discover failed")
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		args := []string{"--interactive", "never", "--output", "json", "workflow", "ticket", "inventory", "--key", "ABC"}

		if _, err := executeBootstrapCommand(t, command, args...); err == nil {
			t.Fatal("inventory unexpectedly succeeded with a discovery failure")
		}
	})

	t.Run("fails when the freshness fetch fails", func(t *testing.T) {
		t.Parallel()
		git := newCommandGit(t, "develop", nil)
		git.fetchErr = errors.New("fetch failed")
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		args := []string{"--interactive", "never", "--output", "json", "workflow", "ticket", "inventory", "--key", "ABC"}

		if _, err := executeBootstrapCommand(t, command, args...); err == nil {
			t.Fatal("inventory unexpectedly succeeded with a fetch failure")
		}
	})

	t.Run("fails when the allocation inventory fails closed", func(t *testing.T) {
		t.Parallel()
		git := newCommandGit(t, "develop", nil)
		git.subjectsErr = errors.New("commit subject history unavailable")
		command := NewWithRuntime(BuildInfo{Version: "test"}, commandRuntime(git))
		args := []string{"--interactive", "never", "--output", "json", "workflow", "ticket", "inventory", "--key", "ABC"}

		if _, err := executeBootstrapCommand(t, command, args...); err == nil {
			t.Fatal("inventory unexpectedly succeeded with a closed inventory")
		}
	})
}
