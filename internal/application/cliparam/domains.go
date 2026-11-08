package cliparam

// This file holds the canonical registry of every CLI value domain. The
// descriptors themselves live in the class files: domains_closed_enum.go,
// domains_shaped.go, domains_free_constrained.go, and domains_structural.go;
// the scaffold registry entry lives in scaffold_domains.go. Contract tests
// iterate the registry below to pin every projection against its source.
// Canonical conventions: docs/conventions/cli/value-domain-model.md and
// docs/conventions/cli/single-source-of-truth.md.

// All returns every canonical value-domain descriptor of the CLI. Contract
// tests iterate this set to pin every projection against its source.
func All() []Domain {
	return []Domain{
		CommitType(),
		BranchFamily(),
		DirectlyCreatableBranchFamily(),
		SyncStrategy(),
		ReleaseRequestKind(),
		ReleaseStabilizationKind(),
		ColorMode(),
		InteractiveMode(),
		OutputMode(),
		PullRequestProvider(),
		CompletionShell(),
		ReleaseVersion(),
		SupportVersion(),
		ProtectedLineVersion(),
		ReleaseLine(),
		AffectedLine(),
		PropagationTargetLine(),
		ManifestTargetLine(),
		WorktreeAcquisitionBase(),
		TicketKey(),
		TicketNumber(),
		TicketID(),
		BranchSlug(),
		CommitSubject(),
		MessageFile(),
		MessageScaffoldKind(),
		CommitFooter(),
		BreakingDescription(),
		CommitSHA(),
		RemoteName(),
		RequestID(),
		RunID(),
		Requester(),
		BaseBranch(),
		BranchReference(),
		RecordPath(),
		CommitMessageFile(),
		StagePath(),
		Timeout(),
	}
}
