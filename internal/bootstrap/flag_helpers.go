package bootstrap

import (
	"github.com/spf13/cobra"
	"github.com/t33n-software/git-governance/internal/application/cliparam"
)

// This file holds the shared flag-registration helpers: one helper per value
// concept, each binding the flag name, the register-rendered help text with
// the endpoint context, and the shell-completion projection in a single call.
// Call sites pass endpoint context only; they never copy rule text. Canonical
// conventions: docs/conventions/cli/single-source-of-truth.md and
// docs/conventions/cli/help-contract.md.

// valueDomainCommandAnnotation marks commands whose flags render from the
// canonical value-domain register; the tree finalizer appends the discovery
// reference line to the help of every marked command.
const valueDomainCommandAnnotation = "git-governance.t33n-software/value-domain-bound"

// discoveryReferenceLine is the single level-3 deep reference carried by the
// help of every value-carrying endpoint: the canonical value domains and
// limits are machine-readable via policy describe, and the branch family
// patterns via branch list. Canonical convention:
// docs/conventions/cli/help-contract.md.
const discoveryReferenceLine = `Canonical value domains and limits: "policy describe"; branch family patterns: "branch list".`

// markValueDomainCommand records that the command binds at least one flag to
// the value-domain register.
func markValueDomainCommand(command *cobra.Command) {
	if command.Annotations == nil {
		command.Annotations = map[string]string{}
	}
	command.Annotations[valueDomainCommandAnnotation] = "true"
}

// registerValueDomainFlag binds one string flag to the canonical value-domain
// register: the help renders from the register descriptor and the completion
// channel K4 serves the same values.
func registerValueDomainFlag(command *cobra.Command, target *string, name, defaultValue string, domain cliparam.Domain, context string) {
	command.Flags().StringVar(target, name, defaultValue, domain.HelpText(context))
	registerValueCompletion(command, name, domain)
	markValueDomainCommand(command)
}

// registerValueDomainSliceFlag binds one repeatable string-slice flag to the
// canonical value-domain register.
func registerValueDomainSliceFlag(command *cobra.Command, target *[]string, name string, domain cliparam.Domain, context string) {
	command.Flags().StringSliceVar(target, name, nil, domain.HelpText(context))
	registerValueCompletion(command, name, domain)
	markValueDomainCommand(command)
}

// registerPathDomainFlag binds one structural-reference flag whose values are
// repository paths; completion stays with the framework's file completion.
func registerPathDomainFlag(command *cobra.Command, target *string, name string, domain cliparam.Domain, context string) {
	command.Flags().StringVar(target, name, "", domain.HelpText(context))
	markValueDomainCommand(command)
}

// registerPathDomainSliceFlag binds one repeatable structural-reference flag
// whose values are repository paths; completion stays with the framework's
// file completion.
func registerPathDomainSliceFlag(command *cobra.Command, target *[]string, name string, domain cliparam.Domain, context string) {
	command.Flags().StringSliceVar(target, name, nil, domain.HelpText(context))
	markValueDomainCommand(command)
}

// registerPersistentValueDomainFlag binds one global persistent string flag
// to the canonical value-domain register.
func registerPersistentValueDomainFlag(command *cobra.Command, target *string, name, defaultValue string, domain cliparam.Domain) {
	command.PersistentFlags().StringVar(target, name, defaultValue, domain.HelpText(""))
	registerValueCompletion(command, name, domain)
	markValueDomainCommand(command)
}

// registerValueCompletion projects the domain values and static prefix forms
// into the shell-completion channel K4. RegisterFlagCompletionFunc cannot
// fail here because the flag is registered immediately before this call.
func registerValueCompletion(command *cobra.Command, name string, domain cliparam.Domain) {
	_ = command.RegisterFlagCompletionFunc(name, func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return domain.Complete(toComplete), cobra.ShellCompDirectiveNoFileComp
	})
}

// registerCommitTypeFlag binds the commit family flag (closed-enum).
func registerCommitTypeFlag(command *cobra.Command, target *string, context string) {
	registerValueDomainFlag(command, target, "type", "", cliparam.CommitType(), context)
}

// registerMergeTypeFlag binds the commit family flag of a merge transfer.
func registerMergeTypeFlag(command *cobra.Command, target *string) {
	registerValueDomainFlag(command, target, "merge-type", "", cliparam.CommitType(), "for --strategy merge")
}

// registerBranchFamilyFlag binds the directly-creatable branch family subset
// (closed-enum endpoint subset via the declared DirectlyCreatable filter).
func registerBranchFamilyFlag(command *cobra.Command, target *string, context string) {
	registerValueDomainFlag(command, target, "family", "", cliparam.DirectlyCreatableBranchFamily(), context)
}

// registerSyncStrategyFlag binds the synchronization strategy flag with its
// check default (closed-enum).
func registerSyncStrategyFlag(command *cobra.Command, target *string) {
	registerValueDomainFlag(command, target, "strategy", cliparam.SyncStrategy().Values[0], cliparam.SyncStrategy(), "")
}

// registerReleaseRequestKindFlag binds the protected line operation flag
// (closed-enum).
func registerReleaseRequestKindFlag(command *cobra.Command, target *string) {
	registerValueDomainFlag(command, target, "kind", "", cliparam.ReleaseRequestKind(), "")
}

// registerStabilizationKindFlag binds the release stabilization category flag
// (closed-enum).
func registerStabilizationKindFlag(command *cobra.Command, target *string) {
	registerValueDomainFlag(command, target, "kind", "", cliparam.ReleaseStabilizationKind(), "")
}

// registerAffectedLineFlag binds the hotfix affected-line flag (shaped).
func registerAffectedLineFlag(command *cobra.Command, target *string) {
	registerValueDomainFlag(command, target, "affected-line", "", cliparam.AffectedLine(), "")
}

// registerTargetLineFlag binds the single-commit propagation target-line flag
// (shaped; includes main).
func registerTargetLineFlag(command *cobra.Command, target *string) {
	registerValueDomainFlag(command, target, "target-line", "", cliparam.PropagationTargetLine(), "")
}

// registerManifestTargetLineFlag binds the manifest propagation target-line
// flag (shaped; main is record-bound and excluded).
func registerManifestTargetLineFlag(command *cobra.Command, target *string) {
	registerValueDomainFlag(command, target, "target-line", "", cliparam.ManifestTargetLine(), "")
}

// registerTicketKeyFlag binds the ticket key flag (free-constrained).
func registerTicketKeyFlag(command *cobra.Command, target *string) {
	registerValueDomainFlag(command, target, "key", "", cliparam.TicketKey(), "")
}

// registerTicketNumberFlag binds the ticket number flag (free-constrained).
func registerTicketNumberFlag(command *cobra.Command, target *string) {
	registerValueDomainFlag(command, target, "ticket", "", cliparam.TicketNumber(), "")
}

// registerTicketIDFlag binds the ticket identity compatibility-check flag
// (free-constrained KEY-NUMBER).
func registerTicketIDFlag(command *cobra.Command, target *string, context string) {
	registerValueDomainFlag(command, target, "ticket", "", cliparam.TicketID(), context)
}

// registerSlugFlag binds a branch slug flag under the given name
// (free-constrained); the scratch variant uses the same domain.
func registerSlugFlag(command *cobra.Command, target *string, name, context string) {
	registerValueDomainFlag(command, target, name, "", cliparam.BranchSlug(), context)
}

// registerSubjectFlag binds a commit description flag under the given name
// (free-constrained with the convention label).
func registerSubjectFlag(command *cobra.Command, target *string, name, context string) {
	registerValueDomainFlag(command, target, name, "", cliparam.CommitSubject(), context)
}

// registerBodyFlag binds a commit body flag under the given name
// (free-constrained); mandatory-body contexts stay endpoint context.
func registerBodyFlag(command *cobra.Command, target *string, name, context string) {
	registerValueDomainFlag(command, target, name, "", cliparam.CommitBody(), context)
}

// registerFooterFlag binds a repeatable commit footer flag under the given
// name (composite-token TOKEN=VALUE).
func registerFooterFlag(command *cobra.Command, target *[]string, name, context string) {
	registerValueDomainSliceFlag(command, target, name, cliparam.CommitFooter(), context)
}

// registerBreakingDescriptionFlag binds a breaking change migration impact
// flag under the given name (free-constrained).
func registerBreakingDescriptionFlag(command *cobra.Command, target *string, name, context string) {
	registerValueDomainFlag(command, target, name, "", cliparam.BreakingDescription(), context)
}

// registerReleaseVersionFlag binds the release semantic version flag
// (shaped).
func registerReleaseVersionFlag(command *cobra.Command, target *string) {
	registerValueDomainFlag(command, target, "version", "", cliparam.ReleaseVersion(), "")
}

// registerSupportVersionFlag binds the support major.minor version flag
// (shaped).
func registerSupportVersionFlag(command *cobra.Command, target *string) {
	registerValueDomainFlag(command, target, "version", "", cliparam.SupportVersion(), "")
}

// registerProtectedLineVersionFlag binds the union version flag of the
// protected-line request endpoint (shaped).
func registerProtectedLineVersionFlag(command *cobra.Command, target *string) {
	registerValueDomainFlag(command, target, "version", "", cliparam.ProtectedLineVersion(), "")
}

// registerReleaseLineFlag binds a release line reference flag (shaped).
func registerReleaseLineFlag(command *cobra.Command, target *string, context string) {
	registerValueDomainFlag(command, target, "release", "", cliparam.ReleaseLine(), context)
}

// registerCommitSHAFlag binds the reviewed source commit flag of the hotfix
// propagation endpoint (free-constrained).
func registerCommitSHAFlag(command *cobra.Command, target *string) {
	registerValueDomainFlag(command, target, "commit", "", cliparam.CommitSHA(), "reviewed source commit SHA")
}

// registerBaseFlag binds an explicit base flag (structural-reference).
func registerBaseFlag(command *cobra.Command, target *string, context string) {
	registerValueDomainFlag(command, target, "base", "", cliparam.BaseBranch(), context)
}

// registerBranchReferenceFlag binds a canonical branch reference flag under
// the given name (structural-reference). The endpoint role leads the text;
// the register owns the grammar, the resolution rule, and the example.
func registerBranchReferenceFlag(command *cobra.Command, target *string, name, role string) {
	registerValueDomainFlag(command, target, name, "", cliparam.BranchReference().WithLead(role), "")
}

// registerRecordFlag binds the hotfix release record path flag
// (structural-reference with framework file completion).
func registerRecordFlag(command *cobra.Command, target *string) {
	registerPathDomainFlag(command, target, "record", cliparam.RecordPath(), "defaults to the ticket record path")
}

// registerCommitMessageFlag binds the complete commit message flag
// (free-constrained full grammar).
func registerCommitMessageFlag(command *cobra.Command, target *string) {
	registerValueDomainFlag(command, target, "message", "", cliparam.CommitMessage(), "")
}

// registerCommitMessageFileFlag binds the commit message file flag
// (structural-reference with framework file completion).
func registerCommitMessageFileFlag(command *cobra.Command, target *string) {
	registerPathDomainFlag(command, target, "message-file", cliparam.CommitMessageFile(), "")
}

// registerStageFlag binds the explicit stage path flag
// (structural-reference with framework file completion).
func registerStageFlag(command *cobra.Command, target *[]string) {
	registerPathDomainSliceFlag(command, target, "stage", cliparam.StagePath(), "")
}

// registerRequestIDFlag binds the durable protected-line request ID flag
// (free-constrained; controller-internal endpoints only).
func registerRequestIDFlag(command *cobra.Command, target *string) {
	registerValueDomainFlag(command, target, "request-id", "", cliparam.RequestID(), "")
}

// registerRunIDFlag binds a provider workflow run ID flag under the given
// name (free-constrained; controller-internal endpoints only).
func registerRunIDFlag(command *cobra.Command, target *string, name string) {
	registerValueDomainFlag(command, target, name, "", cliparam.RunID(), "")
}

// registerRequesterFlag binds the request-authority actor flag
// (free-constrained).
func registerRequesterFlag(command *cobra.Command, target *string) {
	registerValueDomainFlag(command, target, "requester", "", cliparam.Requester(), "")
}

// registerGlobalValueDomainFlags binds the global persistent flags that carry
// value domains to the canonical register: color, interactive, output, pull
// request provider, remote, and timeout.
func registerGlobalValueDomainFlags(command *cobra.Command, options *appOptions) {
	registerPersistentValueDomainFlag(command, &options.interactive, "interactive", options.interactive, cliparam.InteractiveMode())
	registerPersistentValueDomainFlag(command, &options.output, "output", options.output, cliparam.OutputMode())
	registerPersistentValueDomainFlag(command, &options.color, "color", options.color, cliparam.ColorMode())
	registerPersistentValueDomainFlag(command, &options.pullRequestProvider, "pull-request-provider", options.pullRequestProvider, cliparam.PullRequestProvider())
	registerPersistentValueDomainFlag(command, &options.remote, "remote", options.remote, cliparam.RemoteName())
	command.PersistentFlags().DurationVar(&options.timeout, "timeout", options.timeout, cliparam.Timeout().HelpText(""))
}
