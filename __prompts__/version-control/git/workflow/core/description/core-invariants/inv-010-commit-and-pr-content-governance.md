# Description: INV-010 Commit-message and pull-request-description content governance
[INTENT: CONTEXT]

This leaf owns exactly one behavioral invariant of the portable core: INV-010.
The invariant register and the package map live in the parent TOC surface
`description/core-invariants.md`.

---

## INV-010: Commit-message and pull-request-description content governance
[INTENT: SPECIFICATION]

**Type:** CONSTRAINT

**Description:**

The binary and the active policy own commit grammar, families, and technical
limits. The core owns content completeness: every commit message carries a
precise one-sentence subject plus a body that holds what the diff cannot
express — intent, behavioral scope, touched invariants or contracts,
verification evidence, and risks. The body is the relevance filter of the
commit history: a later agent must be able to judge a unit's relevance from
subject and body without loading the diff (filter before fetch).

The agent makes only generic decisions (the ticket's actual content); every
architectural decision is bound by the contract. The subject formulation
contract fixes language (English), imperative mood, behavior-over-file
naming, and forbidden filler forms. The subject never carries the metadata
envelope (family, ticket scope, breaking marker) in header form at any
position: the envelope is assembly-owned by the binary, which rejects
envelope content in the subject fail-closed. The commit family is always an
explicit decision from the unit's functional outcome, never a silent
derivation from the branch family. The canonical body layout fixes the
category order (Motivation, Behavioral Change, Contracts and Invariants,
Verification, Risks and Follow-ups) with a short-form rule for narrow
scopes. An executable acceptance gate makes `commit_content_verified`
falsifiable: five canonical questions must be answerable from subject and
body alone, without the diff, or the omission must be matrix-justified.

The body is the default, not an option. A binding decision matrix scopes
every exception: hotfix lanes, release and support lanes, breaking markers,
and scratch squash transfers always require a body; behavior and structure
families require it except for provably trivial, self-explanatory units;
process families require it once behavior, contracts, or processes are
touched; only provably trivial `style` or `chore` units may omit it, because
the filler-text prohibition outranks the mandate. An omission must be
matrix-justified and is audited as `omitted-justified`; an unjustified
omission is never `PASS`.

Content anti-patterns are forbidden: diff narration, filler text, content
not evidenced by the actually staged paths (reality anchoring), and secrets.

The pull request is a separate abstraction layer above the commit series.
Its description carries the integration view that no single commit message
holds and never replicates commit content (single source of truth: detail
stays in the commits). The description is mandatory, never optional: every
pull request crosses a protected shared line, so the review gate needs its
information carriers deterministically; the mandate covers presence and
contract fidelity, not length. The canonical section order is fixed:
Summary, Scope and Non-Goals, Commit Series (navigation only), Risk and
Rollback, Verification and Review Focus.

Transport is file-based and stays help-first: multi-line message parts (the
commit body and the pull-request description) cross the CLI boundary
exclusively through a message file passed by absolute path — an existing
plain UTF-8 text file of at most 1 MiB whose content is carried verbatim;
scalar parts (subject, footers, breaking impact) stay flag arguments. The
composing agent derives the carrying arguments from the immediately
preceding `commit create` or publish-endpoint help and creates the message
file in the OS temp area under the [6.4.6] discipline. If the current binary
exposes no body transport for a commit or no description transport for a
pull request, the agent blocks with the named gap instead of falling back to
raw Git, an external PR CLI, raw provider calls, or manual web edits.

**Current State:**

The core bound semantic unit boundaries and commit validation but left the
content completeness of commit messages and pull-request descriptions
unowned; the binary currently exposes commit subject, body, and footer
transport, while no publish endpoint exposes a pull-request description
transport.

**Target State:**

The core binds commit-message content through the body-mandate decision
matrix and the `commit_content_verified` proof, and pull-request-description
content through the integration-layer contract and the
`pr_description_verified` proof. A missing description transport on the
binary is reported as a named blocker, never bypassed.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Commit content architecture | Section [6.4] |
|| `core/prompt.md` | Pull-request description architecture | Section [8.1] |
|| `core/prompt.md` | Proof surfaces and audit records | Sections [2.3] and [9] |
|| `core/prompt.md` | Content prohibitions | Section [10] |

**Positive Example(s):**

```text
compose subject and body from the frozen acceptance ledger and the actually
staged paths
-> bind the body-mandate matrix decision, including any justified omission
-> set commit_content_verified
-> transport through the freshly read commit create help
```

**Negative Example(s):**

```text
narrate the diff line by line in the body, or omit the body on a hotfix
lane commit, or replicate commit bodies into the pull-request description
```

Diff narration duplicates what the diff carries more efficiently; an
unjustified omission removes the relevance filter; a replicated pull-request
body breaks the single source of truth.

---

## Conventions and Constraints
[INTENT: CONSTRAINT]

- This invariant is binding for every consuming agent and adapter.
- An invariant change is a material core-contract change and belongs in
  `core/CHANGELOG.md` with a semantic version, a compatibility assessment,
  and the affected surfaces.