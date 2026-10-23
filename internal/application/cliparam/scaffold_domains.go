package cliparam

// This file holds the value-domain descriptor of the message scaffold
// endpoint, split from domains.go under the file-size governance modularization
// rule. The descriptor stays part of the canonical register through All().
// Canonical conventions: docs/conventions/cli/value-domain-model.md and
// docs/conventions/cli/message-file-transport.md.

// MessageScaffoldKind is the closed-enum domain of the message kinds the
// scaffold endpoint constructs. The bootstrap contract test pins this value
// set against the scaffold source register so the flag domain, the completion
// channel, and the construction surface can never drift apart.
func MessageScaffoldKind() Domain {
	return Domain{
		Concept: "scaffold message kind",
		Class:   ClassClosedEnum,
		Values:  []string{"commit-body", "pr-description"},
		Example: "commit-body",
	}
}
