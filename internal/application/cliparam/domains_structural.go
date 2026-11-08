package cliparam

// This file holds the structural-reference and scalar-bounded value domains
// of the CLI: paths, branch references, and bounded durations whose contract
// resolves existence at runtime. Canonical conventions:
// docs/conventions/cli/value-domain-model.md and
// docs/conventions/cli/single-source-of-truth.md.

// MessageFile is the structural-reference domain of a message file: every
// multi-line message part crosses the CLI boundary exclusively through a
// file path. The endpoint role (commit body, pull request description) is
// rendered through WithLead at the call site; the transport rule exists
// exactly once.
func MessageFile() Domain {
	return Domain{
		Concept: "message file",
		Class:   ClassStructuralReference,
		Rule:    "absolute path to an existing plain UTF-8 text file of at most 1 MiB (relative paths are rejected); line endings are LF or CRLF and normalize to LF before validation; existence is resolved at runtime",
	}
}

// BaseBranch is the structural-reference domain of an explicit synchronization
// or validation base. Existence is resolved at runtime; the help must not
// promise full prevention.
func BaseBranch() Domain {
	return Domain{
		Concept: "explicit base",
		Class:   ClassStructuralReference,
		Rule: "canonical branch name or <remote>/<branch> on the selected remote; " +
			"existence is resolved at runtime",
		Example: "origin/develop",
	}
}

// BranchReference is the structural-reference domain of a canonical branch
// name argument.
func BranchReference() Domain {
	return Domain{
		Concept: "branch",
		Class:   ClassStructuralReference,
		Rule: "a canonical branch name for a supported family (main, develop, release/<semver>, " +
			"support/<major.minor>, or <family>/<KEY>-<NUMBER>-<slug>); existence is resolved at runtime",
		Example: "feature/ABC-123-add-export-button",
	}
}

// RecordPath is the structural-reference domain of the repository-relative
// hotfix release record path.
func RecordPath() Domain {
	return Domain{
		Concept: "repository-relative hotfix release record",
		Class:   ClassStructuralReference,
		Rule:    "repository-relative path to the hotfix release record; existence is resolved at runtime",
		Example: ".git-governance/hotfix-release-records/ABC-123.json",
	}
}

// CommitMessageFile is the structural-reference domain of a file carrying a
// complete commit message.
func CommitMessageFile() Domain {
	return Domain{
		Concept: "file containing the complete commit message",
		Class:   ClassStructuralReference,
		Rule:    "repository file carrying the complete commit message, at most 1 MiB; line endings are LF or CRLF and normalize to LF before validation; existence is resolved at runtime",
	}
}

// StagePath is the structural-reference domain of an explicit path to stage.
func StagePath() Domain {
	return Domain{
		Concept: "explicit path to stage",
		Class:   ClassStructuralReference,
		Rule:    "repository-relative path; existence is resolved at runtime; repeatable, one path per occurrence, or comma-separated values in one occurrence",
	}
}

// Timeout is the scalar-bounded domain of the external process timeout. The
// default is rendered by the command framework from the flag registration.
func Timeout() Domain {
	return Domain{
		Concept: "timeout for external Git processes",
		Class:   ClassScalarBounded,
		Rule:    "a positive duration (rejected otherwise)",
	}
}
