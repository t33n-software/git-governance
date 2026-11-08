package cliparam

// This file holds the free-constrained value domains of the CLI: identifiers
// and descriptions bounded by a grammar rule rather than a closed value set.
// Canonical conventions: docs/conventions/cli/value-domain-model.md and
// docs/conventions/cli/single-source-of-truth.md.

// TicketKey is the free-constrained domain of a ticket namespace, mirroring
// the ticket domain validation rule.
func TicketKey() Domain {
	return Domain{
		Concept: "ticket `key`",
		Class:   ClassFreeConstrained,
		Rule: "1-32 uppercase ASCII letters or digits, beginning with a letter " +
			"(rejected unless matching `^[A-Z][A-Z0-9]*$`)",
		Example: "ABC",
	}
}

// TicketNumber is the free-constrained domain of a positive ticket number.
func TicketNumber() Domain {
	return Domain{
		Concept: "ticket `number`",
		Class:   ClassFreeConstrained,
		Rule:    "1-18 decimal digits without a leading zero (rejected unless matching `^[1-9][0-9]*$`)",
		Example: "123",
	}
}

// TicketID is the free-constrained domain of the canonical KEY-NUMBER ticket
// identity.
func TicketID() Domain {
	return Domain{
		Concept: "ticket",
		Class:   ClassFreeConstrained,
		Rule:    "KEY-NUMBER with exactly one key-number separator (rejected otherwise)",
		Example: "ABC-123",
	}
}

// BranchSlug is the free-constrained domain of the kebab-case branch
// description, mirroring the branch domain validation rule.
func BranchSlug() Domain {
	return Domain{
		Concept: "kebab-case branch `description`",
		Class:   ClassFreeConstrained,
		Rule: "1-100 lowercase ASCII letters or digits, words joined by single hyphens " +
			"(rejected unless matching `^[a-z0-9]+(?:-[a-z0-9]+)*$`)",
		Example: "add-export-button",
	}
}

// CommitSubject is the free-constrained domain of the envelope-free commit
// description. Length, padding, control characters, and the envelope ban are
// hard validation rejections; the filler formulas are a content convention
// that validation does not enforce — the label law keeps both distinguishable.
// Canonical convention: docs/conventions/commits/subject-contract.md.
func CommitSubject() Domain {
	return Domain{
		Concept: "commit description",
		Class:   ClassFreeConstrained,
		Rule: "1-200 runes, unpadded, free of control characters, and free of the " +
			"assembly-owned type(TICKET)[!]: envelope (rejected by validation)",
		Convention: "filler formulas without a named behavior such as update, fix stuff, changes, misc, or wip",
		Example:    "add export button",
	}
}

// CommitFooter is the composite-token domain of a commit footer: the CLI
// transport form TOKEN=VALUE renders as TOKEN: value in the message artifact.
func CommitFooter() Domain {
	return Domain{
		Concept: "commit footer",
		Class:   ClassCompositeToken,
		Rule: "footer as TOKEN=VALUE on the CLI, rendered as TOKEN: value in the message; " +
			"token: letters, digits, and hyphens, or BREAKING CHANGE; value non-empty and unpadded; " +
			"repeatable (rejected by validation)",
		Example:  "Refs=#123",
		Prefixes: []string{"BREAKING CHANGE="},
	}
}

// BreakingDescription is the free-constrained domain of the breaking change
// migration impact.
func BreakingDescription() Domain {
	return Domain{
		Concept: "breaking change migration impact",
		Class:   ClassFreeConstrained,
		Rule:    "non-empty and unpadded, without control characters (rejected by validation)",
		Example: "clients must migrate to the new contract",
	}
}

// CommitSHA is the free-constrained domain of the bounded hexadecimal commit
// identifier accepted by the controlled hotfix propagation workflow.
func CommitSHA() Domain {
	return Domain{
		Concept: "`commit`",
		Class:   ClassFreeConstrained,
		Rule:    "7-64 hexadecimal characters (rejected unless matching `^[0-9a-fA-F]{7,64}$`)",
		Example: "9fceb02",
	}
}

// RemoteName is the free-constrained domain of a Git remote name, mirroring
// the branch domain validation rule.
func RemoteName() Domain {
	return Domain{
		Concept: "Git remote `name`",
		Class:   ClassFreeConstrained,
		Rule: "letters, digits, dots, underscores, or hyphens, beginning with a letter or digit " +
			"(rejected unless matching `^[A-Za-z0-9][A-Za-z0-9._-]*$`)",
		Example: "origin",
	}
}

// RequestID is the free-constrained domain of the durable protected-line
// request identifier.
func RequestID() Domain {
	return Domain{
		Concept: "durable protected-line request `ID`",
		Class:   ClassFreeConstrained,
		Rule:    "1-64 letters, digits, underscores, or hyphens (rejected unless matching `^[A-Za-z0-9_-]{1,64}$`)",
	}
}

// RunID is the free-constrained domain of a provider workflow run identifier.
func RunID() Domain {
	return Domain{
		Concept: "workflow run `ID`",
		Class:   ClassFreeConstrained,
		Rule:    "a positive decimal number without a leading zero (rejected unless matching `^[1-9][0-9]*$`)",
	}
}

// Requester is the free-constrained domain of the request-authority actor.
func Requester() Domain {
	return Domain{
		Concept: "request-authority actor",
		Class:   ClassFreeConstrained,
		Rule:    "a non-empty one-line identifier of at most 200 characters (rejected by validation)",
	}
}
