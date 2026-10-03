package bootstrap

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// TestMessageFileEndpointsRejectRelativePathsFailClosed proves the endpoint
// inheritance of the message-file transport: every surface that resolves a
// message file rejects a relative path fail-closed at the earliest boundary,
// before any repository or provider work happens.
func TestMessageFileEndpointsRejectRelativePathsFailClosed(t *testing.T) {
	tests := []struct {
		name    string
		command func() *cobra.Command
		args    []string
	}{
		{
			name: "commit create",
			command: func() *cobra.Command {
				git := newCommitCommandGit(t, "feature/ABC-123-add-export")
				application := newCommitCommandApplication(git, nil)
				application.options.yes = true
				return newCommitCreateCommand(application)
			},
			args: []string{"--type", "feat", "--subject", "add export", "--body-file", "./message.txt"},
		},
		{
			name: "branch merge-scratch",
			command: func() *cobra.Command {
				git := newBranchCommandGit(t, "scratch/ABC-123-export-exploration")
				application := newBranchCommandApplication(git, nil, nil, "human")
				application.options.yes = true
				return newScratchMergeCommand(application)
			},
			args: []string{"--type", "feat", "--subject", "add export", "--body-file", "./message.txt"},
		},
		{
			name: "branch sync-base",
			command: func() *cobra.Command {
				git := newBranchCommandGit(t, "feature/ABC-123-add-export")
				application := newBranchCommandApplication(git, nil, nil, "human")
				application.options.yes = true
				return newBranchSyncBaseCommand(application)
			},
			args: []string{"--strategy", "merge", "--merge-type", "feat", "--merge-subject", "merge origin/develop", "--merge-body-file", "./message.txt"},
		},
		{
			name: "workflow ticket publish pull request description",
			command: func() *cobra.Command {
				git := newBranchCommandGit(t, "feature/ABC-123-add-export")
				application := newBranchCommandApplication(git, nil, nil, "human")
				application.options.yes = true
				return newTicketPublishCommand(application)
			},
			args: []string{"--body-file", "./message.txt"},
		},
		{
			name: "workflow ticket publish scratch squash transfer body",
			command: func() *cobra.Command {
				git := newBranchCommandGit(t, "feature/ABC-123-add-export")
				application := newBranchCommandApplication(git, nil, nil, "human")
				application.options.yes = true
				return newTicketPublishCommand(application)
			},
			args: []string{"--commit-body-file", "./message.txt"},
		},
		{
			name: "workflow hotfix publish",
			command: func() *cobra.Command {
				git := newBranchCommandGit(t, "hotfix/ABC-123-payment-timeout")
				application := newBranchCommandApplication(git, nil, nil, "human")
				application.options.yes = true
				return newHotfixPublishCommand(application)
			},
			args: []string{"--body-file", "./message.txt"},
		},
		{
			name: "workflow hotfix propagate",
			command: func() *cobra.Command {
				git := newBranchCommandGit(t, "hotfix/ABC-123-payment-timeout")
				application := newBranchCommandApplication(git, nil, nil, "human")
				application.options.yes = true
				return newHotfixPropagateCommand(application)
			},
			args: []string{"--body-file", "./message.txt"},
		},
		{
			name: "workflow release publish-stabilization",
			command: func() *cobra.Command {
				git := newBranchCommandGit(t, "fix/ABC-123-restore-session-renewal")
				application := newBranchCommandApplication(git, nil, nil, "human")
				application.options.yes = true
				return newReleasePublishStabilizationCommand(application)
			},
			args: []string{"--body-file", "./message.txt"},
		},
		{
			name: "workflow release align-promotion-base",
			command: func() *cobra.Command {
				git := newBranchCommandGit(t, "chore/ABC-123-release-preparation")
				application := newBranchCommandApplication(git, nil, nil, "human")
				application.options.yes = true
				return newReleaseAlignPromotionBaseCommand(application)
			},
			args: []string{"--body-file", "./message.txt"},
		},
		{
			name: "workflow release promote",
			command: func() *cobra.Command {
				git := newBranchCommandGit(t, "feature/ABC-123-add-export")
				application := newBranchCommandApplication(git, nil, nil, "human")
				application.options.yes = true
				return newReleasePromotionCommand(application)
			},
			args: []string{"--body-file", "./message.txt"},
		},
		{
			name: "workflow release backmerge",
			command: func() *cobra.Command {
				git := newBranchCommandGit(t, "feature/ABC-123-add-export")
				application := newBranchCommandApplication(git, nil, nil, "human")
				application.options.yes = true
				return newReleaseBackmergeCommand(application)
			},
			args: []string{"--body-file", "./message.txt"},
		},
		{
			name: "workflow release align-reconciliation-base",
			command: func() *cobra.Command {
				git := newBranchCommandGit(t, "chore/ABC-123-reconciliation-preparation")
				application := newBranchCommandApplication(git, nil, nil, "human")
				application.options.yes = true
				return newReleaseAlignReconciliationBaseCommand(application)
			},
			args: []string{"--body-file", "./message.txt"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := executeBranchCommand(t, test.command(), context.Background(), test.args...)
			typed, ok := problem.As(err)
			if !ok {
				t.Fatalf("%s error = %v, want a coded problem", test.name, err)
			}
			if typed.Code != problem.CodeInvalidInput {
				t.Fatalf("%s error code = %s, want %s", test.name, typed.Code, problem.CodeInvalidInput)
			}
			if typed.Expected != "an absolute path" {
				t.Fatalf("%s expectation = %q, want %q", test.name, typed.Expected, "an absolute path")
			}
		})
	}
}
