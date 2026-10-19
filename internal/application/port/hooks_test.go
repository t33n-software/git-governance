package port

import (
	"strings"
	"testing"
)

func TestHookManagerToolNameIsCanonical(t *testing.T) {
	t.Parallel()

	if HookManagerToolName != "lefthook" {
		t.Fatalf("HookManagerToolName = %q, want lefthook", HookManagerToolName)
	}
}

func TestHookManagerInvocationForms(t *testing.T) {
	t.Parallel()

	t.Run("the zero invocation keeps the canonical host form", func(t *testing.T) {
		t.Parallel()

		var invocation HookManagerInvocation
		if invocation.ExecutableName() != HookManagerToolName {
			t.Fatalf("ExecutableName() = %q, want %q", invocation.ExecutableName(), HookManagerToolName)
		}
		command := invocation.InstallCommand()
		if len(command) != 1 || command[0] != "install" {
			t.Fatalf("InstallCommand() = %v, want [install]", command)
		}
	})

	t.Run("the project form prefixes the install command", func(t *testing.T) {
		t.Parallel()

		invocation := HookManagerInvocation{Executable: "go", Arguments: []string{"tool", HookManagerToolName}}
		if invocation.ExecutableName() != "go" {
			t.Fatalf("ExecutableName() = %q, want go", invocation.ExecutableName())
		}
		command := invocation.InstallCommand()
		if strings.Join(command, " ") != "tool lefthook install" {
			t.Fatalf("InstallCommand() = %v", command)
		}
		if len(invocation.Arguments) != 2 {
			t.Fatalf("InstallCommand() mutated the prefix: %v", invocation.Arguments)
		}
	})
}
