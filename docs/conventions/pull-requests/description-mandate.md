# Pull requests — Description mandate
[INTENT: REFERENCE]

## Canonical source

This file is the canonical source of truth for the convention that every
governed pull request carries a mandatory, canonically structured
description. The CLI enforcement points and the portable workflow core
re-reference this file as the authority; the complete rationale lives
exclusively here.

## The architectural foundation

A pull request is the final review gate before a change crosses into a
protected shared line. In this topology every pull request is such a
crossing — scratch branches are never pull-request sources — so no
"trivial pull request" class exists. The gate's consumer is the reviewer
(human or agent), not the change itself: the description carries the
integration view that no single commit message holds.

The mandate is a supply-chain-fortress decision. Governance that depends on
author discretion alone is weaker than governance the tool enforces, so the
description is a validated, non-empty field at the CLI boundary — not a
convention that may be skipped. The mandate covers presence and contract
fidelity, never length: a narrow change answers every canonical section with
one sentence.

## Why the description MUST exist

1. **The gate needs deterministic information supply.** An optional
   description makes the evidence base of the last control gate before a
   protected mutation a matter of author mood. Mandatory structure makes it
   deterministic.
2. **The description is the aggregation point that prevents per-commit
   diff loads.** Without it, every reviewer and every later agent must
   aggregate N commit messages and M diffs to reconstruct intent, scope,
   risk, and verification — the exact context-cost explosion the commit
   content architecture exists to prevent, one abstraction level higher.
3. **Single source of truth is preserved, not violated.** The description
   never replicates commit content; it carries only what the commit series
   cannot hold: overall intent, scope and non-goals, the series map as
   navigation, aggregate risk and rollback reference, lane context, and the
   verification strategy and review focus.

## The canonical structure

The description is English and follows this fixed section order; all five
sections are present in every pull request, each headed by its Markdown
heading `## <section name>`:

```text
1. ## Summary — the single overall intent of the change set;
2. ## Scope and Non-Goals — the boundary and the explicit exclusions;
3. ## Commit Series — the subject list of the series as navigation,
   never as content repetition;
4. ## Risk and Rollback — aggregate risk, rollback reference, and lane
   context (target line, hotfix or release relation);
5. ## Verification and Review Focus — the verification strategy and where
   reviewers should concentrate.
```

The `##` heading form is binding: it renders as structure on the hosting
platform and keeps layout deterministic for composing agents. The commit
body is a different surface with its own grammar constraint — the
portable workflow core binds its layout separately and forbids
`Name: text` line starts there, because the commit footer grammar would
misread them; the pull-request description never passes the commit parser.

## Enforcement

The mandate is enforced fail-closed at three boundaries:

1. **CLI boundary:** every pull-request-creating workflow endpoint accepts
   the description input and rejects pull-request creation without it,
   before any Git mutation happens.
2. **Application boundary:** the provider-neutral publication funnel rejects
   an intent with an empty description, so programmatic callers cannot
   bypass the CLI boundary.
3. **Adapter boundary:** the hosting adapter rejects an empty description
   when creating the provider pull request, so the transport never emits an
   undocumented review gate.

The pull-request title remains derived by the binary from the governed
branch and ticket identity; only the description is composed by the caller.

## Boundary: the server-side propagation publisher

The dedicated server-side hotfix propagation publisher
(`workflow hotfix propagate-manifest --publish`) creates its pull requests
inside the protected controller, not in the local binary. There the reviewed
release record is the content source of truth, and the controller composes
the description from that record. The mandate applies unchanged — the
description is never absent — but its composition authority is the record,
not a local flag.
