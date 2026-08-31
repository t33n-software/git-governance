package bootstrap

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/t33n-software/git-governance/internal/application/cliparam"
	"github.com/t33n-software/git-governance/internal/application/policy"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

// TestConceptHelpersBindNameHelpAndDefault pins every shared registration
// helper against the canonical register: the flag name, the rendered help
// text with the endpoint context, and the default value must match the
// register projection exactly.
func TestConceptHelpersBindNameHelpAndDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		register    func(command *cobra.Command, target *string)
		flagName    string
		wantUsage   string
		wantDefault string
	}{
		{
			name:      "commit type",
			register:  func(command *cobra.Command, target *string) { registerCommitTypeFlag(command, target, "") },
			flagName:  "type",
			wantUsage: cliparam.CommitType().HelpText(""),
		},
		{
			name: "commit type with context",
			register: func(command *cobra.Command, target *string) {
				registerCommitTypeFlag(command, target, "for the squashed change")
			},
			flagName:  "type",
			wantUsage: cliparam.CommitType().HelpText("for the squashed change"),
		},
		{
			name:      "merge type",
			register:  func(command *cobra.Command, target *string) { registerMergeTypeFlag(command, target) },
			flagName:  "merge-type",
			wantUsage: cliparam.CommitType().HelpText("for --strategy merge"),
		},
		{
			name:      "branch family",
			register:  func(command *cobra.Command, target *string) { registerBranchFamilyFlag(command, target, "") },
			flagName:  "family",
			wantUsage: cliparam.DirectlyCreatableBranchFamily().HelpText(""),
		},
		{
			name:        "sync strategy keeps the check default",
			register:    func(command *cobra.Command, target *string) { registerSyncStrategyFlag(command, target) },
			flagName:    "strategy",
			wantUsage:   cliparam.SyncStrategy().HelpText(""),
			wantDefault: "check",
		},
		{
			name:      "release request kind",
			register:  func(command *cobra.Command, target *string) { registerReleaseRequestKindFlag(command, target) },
			flagName:  "kind",
			wantUsage: cliparam.ReleaseRequestKind().HelpText(""),
		},
		{
			name:      "stabilization kind",
			register:  func(command *cobra.Command, target *string) { registerStabilizationKindFlag(command, target) },
			flagName:  "kind",
			wantUsage: cliparam.ReleaseStabilizationKind().HelpText(""),
		},
		{
			name:      "affected line",
			register:  func(command *cobra.Command, target *string) { registerAffectedLineFlag(command, target) },
			flagName:  "affected-line",
			wantUsage: cliparam.AffectedLine().HelpText(""),
		},
		{
			name:      "propagation target line",
			register:  func(command *cobra.Command, target *string) { registerTargetLineFlag(command, target) },
			flagName:  "target-line",
			wantUsage: cliparam.PropagationTargetLine().HelpText(""),
		},
		{
			name:      "manifest target line",
			register:  func(command *cobra.Command, target *string) { registerManifestTargetLineFlag(command, target) },
			flagName:  "target-line",
			wantUsage: cliparam.ManifestTargetLine().HelpText(""),
		},
		{
			name:      "ticket key",
			register:  func(command *cobra.Command, target *string) { registerTicketKeyFlag(command, target) },
			flagName:  "key",
			wantUsage: cliparam.TicketKey().HelpText(""),
		},
		{
			name:      "ticket number",
			register:  func(command *cobra.Command, target *string) { registerTicketNumberFlag(command, target) },
			flagName:  "ticket",
			wantUsage: cliparam.TicketNumber().HelpText(""),
		},
		{
			name: "ticket identity",
			register: func(command *cobra.Command, target *string) {
				registerTicketIDFlag(command, target, "compatibility check")
			},
			flagName:  "ticket",
			wantUsage: cliparam.TicketID().HelpText("compatibility check"),
		},
		{
			name:      "slug",
			register:  func(command *cobra.Command, target *string) { registerSlugFlag(command, target, "slug", "") },
			flagName:  "slug",
			wantUsage: cliparam.BranchSlug().HelpText(""),
		},
		{
			name: "scratch slug",
			register: func(command *cobra.Command, target *string) {
				registerSlugFlag(command, target, "scratch-slug", "for the scratch branch")
			},
			flagName:  "scratch-slug",
			wantUsage: cliparam.BranchSlug().HelpText("for the scratch branch"),
		},
		{
			name:      "subject",
			register:  func(command *cobra.Command, target *string) { registerSubjectFlag(command, target, "subject", "") },
			flagName:  "subject",
			wantUsage: cliparam.CommitSubject().HelpText(""),
		},
		{
			name: "merge subject",
			register: func(command *cobra.Command, target *string) {
				registerSubjectFlag(command, target, "merge-subject", "for --strategy merge")
			},
			flagName:  "merge-subject",
			wantUsage: cliparam.CommitSubject().HelpText("for --strategy merge"),
		},
		{
			name:      "body",
			register:  func(command *cobra.Command, target *string) { registerBodyFlag(command, target, "body", "") },
			flagName:  "body",
			wantUsage: cliparam.CommitBody().HelpText(""),
		},
		{
			name: "breaking description",
			register: func(command *cobra.Command, target *string) {
				registerBreakingDescriptionFlag(command, target, "breaking-description", "")
			},
			flagName:  "breaking-description",
			wantUsage: cliparam.BreakingDescription().HelpText(""),
		},
		{
			name:      "release version",
			register:  func(command *cobra.Command, target *string) { registerReleaseVersionFlag(command, target) },
			flagName:  "version",
			wantUsage: cliparam.ReleaseVersion().HelpText(""),
		},
		{
			name:      "support version",
			register:  func(command *cobra.Command, target *string) { registerSupportVersionFlag(command, target) },
			flagName:  "version",
			wantUsage: cliparam.SupportVersion().HelpText(""),
		},
		{
			name:      "protected line version",
			register:  func(command *cobra.Command, target *string) { registerProtectedLineVersionFlag(command, target) },
			flagName:  "version",
			wantUsage: cliparam.ProtectedLineVersion().HelpText(""),
		},
		{
			name:      "release line",
			register:  func(command *cobra.Command, target *string) { registerReleaseLineFlag(command, target, "") },
			flagName:  "release",
			wantUsage: cliparam.ReleaseLine().HelpText(""),
		},
		{
			name:      "commit sha",
			register:  func(command *cobra.Command, target *string) { registerCommitSHAFlag(command, target) },
			flagName:  "commit",
			wantUsage: cliparam.CommitSHA().HelpText("reviewed source commit SHA"),
		},
		{
			name: "base",
			register: func(command *cobra.Command, target *string) {
				registerBaseFlag(command, target, "for pre-push validation")
			},
			flagName:  "base",
			wantUsage: cliparam.BaseBranch().HelpText("for pre-push validation"),
		},
		{
			name: "branch reference",
			register: func(command *cobra.Command, target *string) {
				registerBranchReferenceFlag(command, target, "source", "to sync from")
			},
			flagName:  "source",
			wantUsage: cliparam.BranchReference().WithLead("to sync from").HelpText(""),
		},
		{
			name:      "record",
			register:  func(command *cobra.Command, target *string) { registerRecordFlag(command, target) },
			flagName:  "record",
			wantUsage: cliparam.RecordPath().HelpText("defaults to the ticket record path"),
		},
		{
			name:      "commit message",
			register:  func(command *cobra.Command, target *string) { registerCommitMessageFlag(command, target) },
			flagName:  "message",
			wantUsage: cliparam.CommitMessage().HelpText(""),
		},
		{
			name:      "commit message file",
			register:  func(command *cobra.Command, target *string) { registerCommitMessageFileFlag(command, target) },
			flagName:  "message-file",
			wantUsage: cliparam.CommitMessageFile().HelpText(""),
		},
		{
			name:      "request id",
			register:  func(command *cobra.Command, target *string) { registerRequestIDFlag(command, target) },
			flagName:  "request-id",
			wantUsage: cliparam.RequestID().HelpText(""),
		},
		{
			name:      "run id",
			register:  func(command *cobra.Command, target *string) { registerRunIDFlag(command, target, "executor-run") },
			flagName:  "executor-run",
			wantUsage: cliparam.RunID().HelpText(""),
		},
		{
			name:      "requester",
			register:  func(command *cobra.Command, target *string) { registerRequesterFlag(command, target) },
			flagName:  "requester",
			wantUsage: cliparam.Requester().HelpText(""),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			command := &cobra.Command{Use: "probe"}
			var target string
			test.register(command, &target)

			flag := command.Flags().Lookup(test.flagName)
			if flag == nil {
				t.Fatalf("flag %q was not registered", test.flagName)
			}
			if flag.Usage != test.wantUsage {
				t.Fatalf("flag %q usage = %q, want %q", test.flagName, flag.Usage, test.wantUsage)
			}
			if flag.DefValue != test.wantDefault {
				t.Fatalf("flag %q default = %q, want %q", test.flagName, flag.DefValue, test.wantDefault)
			}
		})
	}
}

// TestSliceAndPathHelpersBindRegisterHelp pins the slice-valued and
// path-valued registration helpers against the register projection.
func TestSliceAndPathHelpersBindRegisterHelp(t *testing.T) {
	t.Parallel()

	t.Run("footer slice", func(t *testing.T) {
		t.Parallel()
		command := &cobra.Command{Use: "probe"}
		var target []string
		registerFooterFlag(command, &target, "footer", "")
		flag := command.Flags().Lookup("footer")
		if flag == nil {
			t.Fatal("flag footer was not registered")
		}
		if flag.Usage != cliparam.CommitFooter().HelpText("") {
			t.Fatalf("footer usage = %q, want %q", flag.Usage, cliparam.CommitFooter().HelpText(""))
		}
	})

	t.Run("stage slice keeps framework file completion", func(t *testing.T) {
		t.Parallel()
		command := &cobra.Command{Use: "probe"}
		var target []string
		registerStageFlag(command, &target)
		flag := command.Flags().Lookup("stage")
		if flag == nil {
			t.Fatal("flag stage was not registered")
		}
		if flag.Usage != cliparam.StagePath().HelpText("") {
			t.Fatalf("stage usage = %q, want %q", flag.Usage, cliparam.StagePath().HelpText(""))
		}
	})
}

// TestValueCompletionServesRegisterValues executes the completion projection
// through the framework's __complete machinery and proves that the served
// candidates come from the register, filtered by the typed prefix.
func TestValueCompletionServesRegisterValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		register     func(command *cobra.Command, target *string)
		flagName     string
		toComplete   string
		wantContains []string
		wantExcludes []string
	}{
		{
			name:         "commit type values",
			register:     func(command *cobra.Command, target *string) { registerCommitTypeFlag(command, target, "") },
			flagName:     "type",
			toComplete:   "f",
			wantContains: []string{"feat", "fix"},
			wantExcludes: []string{"build", "chore"},
		},
		{
			name:         "affected line prefix forms",
			register:     func(command *cobra.Command, target *string) { registerAffectedLineFlag(command, target) },
			flagName:     "affected-line",
			toComplete:   "re",
			wantContains: []string{"release/"},
			wantExcludes: []string{"main", "support/"},
		},
		{
			name:         "empty prefix serves every value",
			register:     func(command *cobra.Command, target *string) { registerSyncStrategyFlag(command, target) },
			flagName:     "strategy",
			toComplete:   "",
			wantContains: []string{"check", "auto", "rebase", "merge"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := &cobra.Command{Use: "root"}
			var target string
			probe := &cobra.Command{
				Use:  "probe",
				RunE: func(_ *cobra.Command, _ []string) error { return nil },
			}
			test.register(probe, &target)
			root.AddCommand(probe)

			var output strings.Builder
			root.SetOut(&output)
			root.SetErr(&output)
			root.SetArgs([]string{"__complete", "probe", "--" + test.flagName, test.toComplete})
			if err := root.Execute(); err != nil {
				t.Fatalf("__complete failed: %v", err)
			}

			for _, want := range test.wantContains {
				if !strings.Contains(output.String(), want) {
					t.Fatalf("completion output %q misses %q", output.String(), want)
				}
			}
			for _, unwanted := range test.wantExcludes {
				for _, line := range strings.Split(output.String(), "\n") {
					if line == unwanted {
						t.Fatalf("completion output contains excluded candidate %q", unwanted)
					}
				}
			}
		})
	}
}

// TestMigratedEndpointsRenderRegisterHelp pins the help text of every
// migrated endpoint flag in the production command tree against the register
// projection: the closed-enum and line-form flags must render the
// endpoint-accepted value domain, never a literal.
func TestMigratedEndpointsRenderRegisterHelp(t *testing.T) {
	t.Parallel()

	root := New(BuildInfo{Version: "test"})

	tests := []struct {
		path     []string
		flagName string
		want     string
	}{
		{path: []string{"commit", "create"}, flagName: "type", want: cliparam.CommitType().HelpText("")},
		{path: []string{"branch", "merge-scratch"}, flagName: "type", want: cliparam.CommitType().HelpText("for the squashed change")},
		{path: []string{"workflow", "ticket", "publish"}, flagName: "type", want: cliparam.CommitType().HelpText("for a scratch squash transfer")},
		{path: []string{"branch", "sync-base"}, flagName: "merge-type", want: cliparam.CommitType().HelpText("for --strategy merge")},
		{path: []string{"branch", "sync-base"}, flagName: "strategy", want: cliparam.SyncStrategy().HelpText("")},
		{path: []string{"branch", "create"}, flagName: "family", want: cliparam.DirectlyCreatableBranchFamily().HelpText("")},
		{path: []string{"workflow", "ticket", "start"}, flagName: "family", want: cliparam.DirectlyCreatableBranchFamily().HelpText("for a regular ticket branch")},
		{path: []string{"workflow", "release", "request"}, flagName: "kind", want: cliparam.ReleaseRequestKind().HelpText("")},
		{path: []string{"workflow", "release", "stabilize"}, flagName: "kind", want: cliparam.ReleaseStabilizationKind().HelpText("")},
		{path: []string{"workflow", "hotfix", "start"}, flagName: "affected-line", want: cliparam.AffectedLine().HelpText("")},
		{path: []string{"workflow", "hotfix", "publish"}, flagName: "affected-line", want: cliparam.AffectedLine().HelpText("")},
		{path: []string{"workflow", "hotfix", "propagate"}, flagName: "target-line", want: cliparam.PropagationTargetLine().HelpText("")},
		{path: []string{"workflow", "hotfix", "propagate-manifest"}, flagName: "target-line", want: cliparam.ManifestTargetLine().HelpText("")},
		{path: []string{"commit", "create"}, flagName: "ticket", want: cliparam.TicketID().HelpText("compatibility check; the current branch is authoritative")},
		{path: []string{"commit", "create"}, flagName: "subject", want: cliparam.CommitSubject().HelpText("")},
		{path: []string{"commit", "create"}, flagName: "body", want: cliparam.CommitBody().HelpText("mandatory for breaking changes, hotfix-lane commits, release-stabilization branches, and the scratch squash transfer")},
		{path: []string{"commit", "create"}, flagName: "breaking-description", want: cliparam.BreakingDescription().HelpText("")},
		{path: []string{"commit", "create"}, flagName: "footer", want: cliparam.CommitFooter().HelpText("")},
		{path: []string{"commit", "create"}, flagName: "stage", want: cliparam.StagePath().HelpText("")},
		{path: []string{"branch", "create"}, flagName: "key", want: cliparam.TicketKey().HelpText("")},
		{path: []string{"branch", "create"}, flagName: "ticket", want: cliparam.TicketNumber().HelpText("")},
		{path: []string{"branch", "create"}, flagName: "slug", want: cliparam.BranchSlug().HelpText("")},
		{path: []string{"branch", "merge-scratch"}, flagName: "subject", want: cliparam.CommitSubject().HelpText("for the squashed change")},
		{path: []string{"branch", "merge-scratch"}, flagName: "body", want: cliparam.CommitBody().HelpText("documenting the discarded experiment paths (mandatory)")},
		{path: []string{"branch", "merge-scratch"}, flagName: "footer", want: cliparam.CommitFooter().HelpText("")},
		{path: []string{"branch", "merge-scratch"}, flagName: "breaking-description", want: cliparam.BreakingDescription().HelpText("")},
		{path: []string{"branch", "sync-base"}, flagName: "merge-subject", want: cliparam.CommitSubject().HelpText("for --strategy merge")},
		{path: []string{"branch", "sync-base"}, flagName: "merge-body", want: cliparam.CommitBody().HelpText("for the --strategy merge commit")},
		{path: []string{"branch", "sync-base"}, flagName: "merge-footer", want: cliparam.CommitFooter().HelpText("for the merge commit")},
		{path: []string{"branch", "sync-base"}, flagName: "merge-breaking-description", want: cliparam.BreakingDescription().HelpText("for the merge commit")},
		{path: []string{"workflow", "ticket", "start"}, flagName: "key", want: cliparam.TicketKey().HelpText("")},
		{path: []string{"workflow", "ticket", "start"}, flagName: "ticket", want: cliparam.TicketNumber().HelpText("")},
		{path: []string{"workflow", "ticket", "start"}, flagName: "slug", want: cliparam.BranchSlug().HelpText("")},
		{path: []string{"workflow", "ticket", "start"}, flagName: "scratch-slug", want: cliparam.BranchSlug().HelpText("for the private scratch branch (optional)")},
		{path: []string{"workflow", "ticket", "publish"}, flagName: "subject", want: cliparam.CommitSubject().HelpText("for a scratch squash transfer")},
		{path: []string{"workflow", "ticket", "publish"}, flagName: "commit-body", want: cliparam.CommitBody().HelpText("for a scratch squash transfer (mandatory, documents the discarded experiment paths)")},
		{path: []string{"workflow", "ticket", "publish"}, flagName: "commit-footer", want: cliparam.CommitFooter().HelpText("for a scratch squash transfer")},
		{path: []string{"workflow", "ticket", "publish"}, flagName: "commit-breaking-description", want: cliparam.BreakingDescription().HelpText("for the scratch squash transfer commit")},
		{path: []string{"workflow", "hotfix", "start"}, flagName: "key", want: cliparam.TicketKey().HelpText("")},
		{path: []string{"workflow", "hotfix", "start"}, flagName: "ticket", want: cliparam.TicketNumber().HelpText("")},
		{path: []string{"workflow", "hotfix", "start"}, flagName: "slug", want: cliparam.BranchSlug().HelpText("for the hotfix branch")},
		{path: []string{"workflow", "hotfix", "propagate"}, flagName: "commit", want: cliparam.CommitSHA().HelpText("reviewed source commit SHA")},
		{path: []string{"workflow", "hotfix", "propagate"}, flagName: "slug", want: cliparam.BranchSlug().HelpText("for the propagation branch (optional)")},
		{path: []string{"workflow", "hotfix", "propagate-manifest"}, flagName: "slug", want: cliparam.BranchSlug().HelpText("for the propagation branch (optional)")},
		{path: []string{"workflow", "release", "request"}, flagName: "key", want: cliparam.TicketKey().HelpText("")},
		{path: []string{"workflow", "release", "request"}, flagName: "ticket", want: cliparam.TicketNumber().HelpText("")},
		{path: []string{"workflow", "release", "request"}, flagName: "requester", want: cliparam.Requester().HelpText("")},
		{path: []string{"workflow", "release", "request"}, flagName: "parent-run", want: cliparam.RunID().HelpText("")},
		{path: []string{"workflow", "release", "execute-request"}, flagName: "request-id", want: cliparam.RequestID().HelpText("")},
		{path: []string{"workflow", "release", "execute-request"}, flagName: "executor-run", want: cliparam.RunID().HelpText("")},
		{path: []string{"workflow", "release", "finalize-request"}, flagName: "request-id", want: cliparam.RequestID().HelpText("")},
		{path: []string{"workflow", "release", "finalize-request"}, flagName: "executor-run", want: cliparam.RunID().HelpText("")},
		{path: []string{"workflow", "release", "stabilize"}, flagName: "key", want: cliparam.TicketKey().HelpText("")},
		{path: []string{"workflow", "release", "stabilize"}, flagName: "ticket", want: cliparam.TicketNumber().HelpText("")},
		{path: []string{"workflow", "release", "stabilize"}, flagName: "slug", want: cliparam.BranchSlug().HelpText("for the stabilization branch")},
		{path: []string{"config", "key", "add"}, flagName: "key", want: cliparam.TicketKey().HelpText("")},
		{path: []string{"config", "key", "remove"}, flagName: "key", want: cliparam.TicketKey().HelpText("")},
		{path: []string{"config", "key", "set-default"}, flagName: "key", want: cliparam.TicketKey().HelpText("")},
		{path: []string{"workflow", "release", "request"}, flagName: "version", want: cliparam.ProtectedLineVersion().HelpText("")},
		{path: []string{"workflow", "release", "cut"}, flagName: "version", want: cliparam.ReleaseVersion().HelpText("")},
		{path: []string{"workflow", "release", "support"}, flagName: "version", want: cliparam.SupportVersion().HelpText("")},
		{path: []string{"workflow", "release", "stabilize"}, flagName: "release", want: cliparam.ReleaseLine().HelpText("")},
		{path: []string{"workflow", "release", "publish-stabilization"}, flagName: "release", want: cliparam.ReleaseLine().HelpText("target")},
		{path: []string{"workflow", "release", "align-promotion-base"}, flagName: "release", want: cliparam.ReleaseLine().HelpText("target")},
		{path: []string{"workflow", "release", "promote"}, flagName: "release", want: cliparam.ReleaseLine().HelpText("")},
		{path: []string{"workflow", "release", "backmerge"}, flagName: "release", want: cliparam.ReleaseLine().HelpText("delivered")},
		{path: []string{"workflow", "release", "align-reconciliation-base"}, flagName: "release", want: cliparam.ReleaseLine().HelpText("delivered")},
		{path: []string{"commit", "create"}, flagName: "base", want: cliparam.BaseBranch().HelpText("for pre-push validation")},
		{path: []string{"branch", "create"}, flagName: "base", want: cliparam.BaseBranch().HelpText("for eligible branch families")},
		{path: []string{"branch", "sync-base"}, flagName: "base", want: cliparam.BaseBranch().HelpText("for synchronization")},
		{path: []string{"validate", "pre-push"}, flagName: "base", want: cliparam.BaseBranch().HelpText("for pre-push validation")},
		{path: []string{"commit", "validate"}, flagName: "message", want: cliparam.CommitMessage().HelpText("")},
		{path: []string{"commit", "validate"}, flagName: "message-file", want: cliparam.CommitMessageFile().HelpText("")},
		{path: []string{"workflow", "hotfix", "validate-record"}, flagName: "record", want: cliparam.RecordPath().HelpText("defaults to the ticket record path")},
		{path: []string{"workflow", "hotfix", "verify-merge"}, flagName: "record", want: cliparam.RecordPath().HelpText("defaults to the ticket record path")},
		{path: []string{"workflow", "hotfix", "verify-delivery"}, flagName: "record", want: cliparam.RecordPath().HelpText("defaults to the ticket record path")},
		{path: []string{"workflow", "hotfix", "propagate-manifest"}, flagName: "record", want: cliparam.RecordPath().HelpText("defaults to the ticket record path")},
		{path: []string{"branch", "validate"}, flagName: "branch", want: cliparam.BranchReference().WithLead("branch name; defaults to the current branch").HelpText("")},
		{path: []string{"validate", "pre-push"}, flagName: "branch", want: cliparam.BranchReference().WithLead("branch name; defaults to the current branch").HelpText("")},
		{path: []string{"commit", "validate"}, flagName: "branch", want: cliparam.BranchReference().WithLead("branch name; defaults to the current branch").HelpText("")},
		{path: []string{"workflow", "ticket", "publish"}, flagName: "branch", want: cliparam.BranchReference().WithLead("ticket branch; defaults to the current branch").HelpText("")},
		{path: []string{"workflow", "ticket", "publish"}, flagName: "target", want: cliparam.BranchReference().WithLead("optional local official target when publishing from scratch").HelpText("")},
		{path: []string{"workflow", "hotfix", "publish"}, flagName: "branch", want: cliparam.BranchReference().WithLead("hotfix branch; defaults to the current branch").HelpText("")},
		{path: []string{"workflow", "hotfix", "verify-merge"}, flagName: "branch", want: cliparam.BranchReference().WithLead("merged hotfix branch; defaults to the current branch").HelpText("")},
		{path: []string{"workflow", "hotfix", "verify-delivery"}, flagName: "branch", want: cliparam.BranchReference().WithLead("merged hotfix branch; defaults to the current branch").HelpText("")},
		{path: []string{"workflow", "hotfix", "propagate"}, flagName: "source", want: cliparam.BranchReference().WithLead("hotfix source branch; defaults to the current branch").HelpText("")},
		{path: []string{"workflow", "hotfix", "propagate"}, flagName: "branch", want: cliparam.BranchReference().WithLead("generated propagation branch; required with --resume").HelpText("")},
		{path: []string{"workflow", "hotfix", "propagate-manifest"}, flagName: "source", want: cliparam.BranchReference().WithLead("hotfix source branch; defaults to the current branch").HelpText("")},
		{path: []string{"workflow", "hotfix", "propagate-manifest"}, flagName: "branch", want: cliparam.BranchReference().WithLead("generated fix branch; required with --resume").HelpText("")},
		{path: []string{"workflow", "cleanup"}, flagName: "branch", want: cliparam.BranchReference().WithLead("local scratch branch; defaults to the current branch").HelpText("")},
		{path: []string{"workflow", "release", "publish-stabilization"}, flagName: "branch", want: cliparam.BranchReference().WithLead("stabilization branch; defaults to the current branch").HelpText("")},
		{path: []string{"workflow", "release", "align-promotion-base"}, flagName: "branch", want: cliparam.BranchReference().WithLead("release-preparation branch; defaults to the current branch").HelpText("")},
		{path: []string{"workflow", "release", "align-reconciliation-base"}, flagName: "branch", want: cliparam.BranchReference().WithLead("reconciliation-preparation branch; defaults to the current branch").HelpText("")},
		{path: []string{"branch", "merge-scratch"}, flagName: "branch", want: cliparam.BranchReference().WithLead("scratch branch; defaults to the current branch").HelpText("")},
		{path: []string{"branch", "merge-scratch"}, flagName: "target", want: cliparam.BranchReference().WithLead("optional local official ticket branch target").HelpText("")},
		{path: []string{"branch", "sync-base"}, flagName: "branch", want: cliparam.BranchReference().WithLead("branch name; must match the current branch when supplied").HelpText("")},
	}

	// The subtests navigate the shared tree sequentially on purpose: cobra's
	// Find lazily initializes command state, so concurrent lookups on one
	// command tree are not safe.
	for _, test := range tests {
		t.Run(strings.Join(append(test.path, "--"+test.flagName), " "), func(t *testing.T) {
			command, _, err := root.Find(test.path)
			if err != nil || command == nil {
				t.Fatalf("command path %v not found: %v", test.path, err)
			}
			flag := command.Flags().Lookup(test.flagName)
			if flag == nil {
				t.Fatalf("flag %q not registered on %v", test.flagName, test.path)
			}
			if flag.Usage != test.want {
				t.Fatalf("%v --%s usage = %q, want %q", test.path, test.flagName, flag.Usage, test.want)
			}
		})
	}
}

// TestPromptRuleTextsRenderFromTheRegister pins the interactive prompt
// channel K2 against the register: the prompt descriptions must render the
// same rule set as the static help, never an independent prose copy.
func TestPromptRuleTextsRenderFromTheRegister(t *testing.T) {
	t.Parallel()

	t.Run("ticket key prompt", func(t *testing.T) {
		t.Parallel()
		prompt := &commandHelperPrompt{inputs: []commandHelperStringReply{{value: "API"}}}
		application := newCommandHelperApplication(newCommandHelperOptions(), prompt, true, true)
		_, err := application.resolveKey(context.Background(), services{
			preferences: policy.NewPreferencesService(&commandHelperPreferencesStore{}),
		}, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(prompt.inputRequests) != 1 {
			t.Fatalf("input requests = %d, want 1", len(prompt.inputRequests))
		}
		if got := prompt.inputRequests[0].Description; got != cliparam.TicketKey().PromptText() {
			t.Fatalf("key prompt description = %q, want %q", got, cliparam.TicketKey().PromptText())
		}
	})

	t.Run("ticket number prompt", func(t *testing.T) {
		t.Parallel()
		prompt := &commandHelperPrompt{inputs: []commandHelperStringReply{{value: "123"}}}
		application := newCommandHelperApplication(newCommandHelperOptions(), prompt, true, true)
		if _, err := application.resolveNumber(context.Background(), ""); err != nil {
			t.Fatal(err)
		}
		if len(prompt.inputRequests) != 1 {
			t.Fatalf("input requests = %d, want 1", len(prompt.inputRequests))
		}
		if got := prompt.inputRequests[0].Description; got != cliparam.TicketNumber().PromptText() {
			t.Fatalf("number prompt description = %q, want %q", got, cliparam.TicketNumber().PromptText())
		}
	})

	t.Run("branch slug prompt", func(t *testing.T) {
		t.Parallel()
		prompt := &commandHelperPrompt{inputs: []commandHelperStringReply{{value: "add-export-button"}}}
		application := newCommandHelperApplication(newCommandHelperOptions(), prompt, true, true)
		if _, err := application.resolveSlug(context.Background(), "", "Branch description"); err != nil {
			t.Fatal(err)
		}
		if len(prompt.inputRequests) != 1 {
			t.Fatalf("input requests = %d, want 1", len(prompt.inputRequests))
		}
		if got := prompt.inputRequests[0].Description; got != cliparam.BranchSlug().PromptText() {
			t.Fatalf("slug prompt description = %q, want %q", got, cliparam.BranchSlug().PromptText())
		}
	})

	t.Run("commit subject prompt carries the register rule set", func(t *testing.T) {
		t.Parallel()
		prompt := &commandHelperPrompt{inputs: []commandHelperStringReply{{value: "add export button"}}}
		application := newCommandHelperApplication(newCommandHelperOptions(), prompt, true, true)
		id, err := ticket.ParseID("ABC-123")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := application.resolveCommitDescription(context.Background(), commitMessageInput{}, commitmsg.TypeFeat, id); err != nil {
			t.Fatal(err)
		}
		if len(prompt.inputRequests) != 1 {
			t.Fatalf("input requests = %d, want 1", len(prompt.inputRequests))
		}
		if got := prompt.inputRequests[0].Description; !strings.HasSuffix(got, cliparam.CommitSubject().PromptText()) {
			t.Fatalf("subject prompt description = %q, want suffix %q", got, cliparam.CommitSubject().PromptText())
		}
	})
}

// TestCompletionProjectionOnTheProductionTree pins the completion channel K4
// on the production command tree: closed-enum flags serve every register
// value, shaped flags serve the static prefix forms, free-constrained flags
// suppress file completion, and path flags keep the framework's file
// completion. Every subtest builds its own tree because command execution
// mutates flag state.
func TestCompletionProjectionOnTheProductionTree(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		args          []string
		wantContains  []string
		wantExcludes  []string
		wantDirective string
	}{
		{
			name:          "commit type serves every commit family",
			args:          []string{"__complete", "commit", "create", "--type", ""},
			wantContains:  cliparam.CommitType().Values,
			wantDirective: ":4",
		},
		{
			name:          "branch family serves only the directly creatable subset",
			args:          []string{"__complete", "branch", "create", "--family", ""},
			wantContains:  cliparam.DirectlyCreatableBranchFamily().Values,
			wantExcludes:  []string{"main", "develop", "release", "support", "hotfix"},
			wantDirective: ":4",
		},
		{
			name:          "sync strategy serves the strategy values",
			args:          []string{"__complete", "branch", "sync-base", "--strategy", ""},
			wantContains:  cliparam.SyncStrategy().Values,
			wantDirective: ":4",
		},
		{
			name:          "affected line serves the prefix forms",
			args:          []string{"__complete", "workflow", "hotfix", "start", "--affected-line", "re"},
			wantContains:  []string{"release/"},
			wantExcludes:  []string{"main", "support/"},
			wantDirective: ":4",
		},
		{
			name:          "free-constrained key suppresses file completion without values",
			args:          []string{"__complete", "workflow", "ticket", "start", "--key", ""},
			wantDirective: ":4",
		},
		{
			name:          "completion positional serves the shells",
			args:          []string{"__complete", "completion", ""},
			wantContains:  cliparam.CompletionShell().Values,
			wantDirective: ":4",
		},
		{
			name:          "record path keeps the framework file completion",
			args:          []string{"__complete", "workflow", "hotfix", "validate-record", "--record", ""},
			wantDirective: ":0",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := New(BuildInfo{Version: "test"})
			var output strings.Builder
			root.SetOut(&output)
			root.SetErr(&output)
			root.SetArgs(test.args)
			if err := root.Execute(); err != nil {
				t.Fatalf("__complete %v failed: %v", test.args, err)
			}

			lines := strings.Split(output.String(), "\n")
			for _, want := range test.wantContains {
				found := false
				for _, line := range lines {
					if line == want {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("completion %v misses candidate %q in %q", test.args, want, output.String())
				}
			}
			for _, unwanted := range test.wantExcludes {
				for _, line := range lines {
					if line == unwanted {
						t.Fatalf("completion %v serves excluded candidate %q", test.args, unwanted)
					}
				}
			}
			directiveFound := false
			for _, line := range lines {
				if line == test.wantDirective {
					directiveFound = true
				}
			}
			if !directiveFound {
				t.Fatalf("completion %v directive line %q missing in %q", test.args, test.wantDirective, output.String())
			}
		})
	}
}

// TestDiscoveryReferenceAppearsOnRegisterBoundEndpointsOnly pins the level-3
// deep reference: every endpoint whose flags render from the register carries
// exactly one discovery reference line in its help, and endpoints without
// register-bound value domains carry none. The tree navigation stays
// sequential because cobra's Find lazily initializes command state.
func TestDiscoveryReferenceAppearsOnRegisterBoundEndpointsOnly(t *testing.T) {
	t.Parallel()

	root := New(BuildInfo{Version: "test"})

	withReference := [][]string{
		{},
		{"commit", "create"},
		{"branch", "create"},
		{"workflow", "ticket", "start"},
		{"workflow", "hotfix", "start"},
		{"workflow", "release", "cut"},
		{"validate", "pre-push"},
		{"config", "key", "add"},
	}
	for _, path := range withReference {
		command := root
		if len(path) > 0 {
			found, _, err := root.Find(path)
			if err != nil || found == nil {
				t.Fatalf("command path %v not found: %v", path, err)
			}
			command = found
		}
		if !strings.Contains(command.Long, discoveryReferenceLine) {
			t.Fatalf("%q help misses the discovery reference line", command.CommandPath())
		}
		if strings.Count(command.Long, discoveryReferenceLine) != 1 {
			t.Fatalf("%q help carries the discovery reference line more than once", command.CommandPath())
		}
	}

	withoutReference := [][]string{
		{"doctor"},
		{"policy", "describe"},
		{"branch", "list"},
		{"workflow"},
		{"branch"},
	}
	for _, path := range withoutReference {
		found, _, err := root.Find(path)
		if err != nil || found == nil {
			t.Fatalf("command path %v not found: %v", path, err)
		}
		if strings.Contains(found.Long, discoveryReferenceLine) {
			t.Fatalf("%q help must not carry the discovery reference line", found.CommandPath())
		}
	}
}

// TestRegisterGlobalValueDomainFlags pins the global persistent flags onto the
// register projection.
func TestRegisterGlobalValueDomainFlags(t *testing.T) {
	t.Parallel()

	options := &appOptions{
		interactive:         "auto",
		output:              "human",
		color:               "auto",
		remote:              "origin",
		pullRequestProvider: "none",
		timeout:             30 * time.Second,
	}
	command := &cobra.Command{Use: "root"}
	registerGlobalValueDomainFlags(command, options)

	tests := []struct {
		flagName    string
		wantUsage   string
		wantDefault string
	}{
		{flagName: "interactive", wantUsage: cliparam.InteractiveMode().HelpText(""), wantDefault: "auto"},
		{flagName: "output", wantUsage: cliparam.OutputMode().HelpText(""), wantDefault: "human"},
		{flagName: "color", wantUsage: cliparam.ColorMode().HelpText(""), wantDefault: "auto"},
		{flagName: "pull-request-provider", wantUsage: cliparam.PullRequestProvider().HelpText(""), wantDefault: "none"},
		{flagName: "remote", wantUsage: cliparam.RemoteName().HelpText(""), wantDefault: "origin"},
		{flagName: "timeout", wantUsage: cliparam.Timeout().HelpText(""), wantDefault: "30s"},
	}

	for _, test := range tests {
		flag := command.PersistentFlags().Lookup(test.flagName)
		if flag == nil {
			t.Fatalf("persistent flag %q was not registered", test.flagName)
		}
		if flag.Usage != test.wantUsage {
			t.Fatalf("flag %q usage = %q, want %q", test.flagName, flag.Usage, test.wantUsage)
		}
		if flag.DefValue != test.wantDefault {
			t.Fatalf("flag %q default = %q, want %q", test.flagName, flag.DefValue, test.wantDefault)
		}
	}
}

// TestValidateOptionsDerivesGlobalValueDomains proves the global option
// validation consumes the register value sets instead of local literals.
func TestValidateOptionsDerivesGlobalValueDomains(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(options *appOptions)
		wantField string
	}{
		{name: "interactive", mutate: func(options *appOptions) { options.interactive = "sometimes" }, wantField: "interactive"},
		{name: "color", mutate: func(options *appOptions) { options.color = "neon" }, wantField: "color"},
		{name: "pull request provider", mutate: func(options *appOptions) { options.pullRequestProvider = "gitlab" }, wantField: "pull-request-provider"},
		{name: "timeout", mutate: func(options *appOptions) { options.timeout = 0 }, wantField: "timeout"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			application := &application{
				options: &appOptions{
					interactive: "auto",
					output:      "human",
					color:       "auto",
					remote:      "origin",
					timeout:     30 * time.Second,
				},
			}
			test.mutate(application.options)
			err := application.validateOptions()
			assertCommandHelperProblem(t, err, problem.CodeInvalidInput, problem.CategoryUsage, test.wantField)
		})
	}
}
