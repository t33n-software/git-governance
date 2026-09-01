# Commit Subject Contract

## 1. Purpose

This document is the canonical project convention for the commit subject:
what the subject may contain, what it must never contain, and which
validation authority enforces the contract. It binds every creation path of
the CLI — non-interactive flags and the interactive prompt alike — and every
verification point.

## 2. The Two Surfaces: Envelope and Subject

A governed commit header is assembled by the CLI as:

```text
<type>(<TICKET>)[!]: <subject>
```

- The **envelope** (`<type>(<TICKET>)[!]:`) is assembly-owned by the CLI. The
  ticket identity is always derived from the current governed branch, and the
  family is always an explicit author decision (see
  [family-selection.md](family-selection.md)). The envelope is never authored
  as free text.
- The **subject** is the only free-content surface of the header: the precise
  one-sentence intent of the unit.

Because the envelope is always tool-owned, envelope-shaped content inside the
subject is a defect in 100 % of cases — it can only produce a duplicated or
contradictory header.

## 3. Allowed Content

The subject character inventory is free by convention:

- every printable Unicode character is legitimate subject content, including
  punctuation such as `(`, `)`, `:`, `#`, `!`, `?`, `,`, `.`, quotes,
  slashes, `+`, `-`, and `*`;
- one single line; non-empty; no leading or trailing whitespace;
- at most 200 Unicode characters — the numeric limit is owned by the active
  policy surface (`commitSubjectMaximumRunes`);
- no control characters;
- English, imperative mood, behavior perspective: the functional effect,
  never the technical touch;
- forbidden empty formulas: `update`, `fix stuff`, `changes`, `misc`, `wip`,
  and any form without a named behavior.

The CLI help and the interactive prompt carry the compact form of this
contract — the length, padding, control-character, and envelope rules as
validation rejections, the empty formulas as convention-violating — rendered
from the canonical value-domain register; the binding help duty is specified
in `docs/specification/cli-contract.md`, section 3 (Flag Help and
Value Domains). This document remains the content owner; the help projection
never duplicates or extends it.

## 4. Forbidden Content: The Metadata Envelope

The subject must never carry the header metadata envelope — the commit
family, the ticket scope, or the breaking marker in header form. The
prohibition is position-independent and case-insensitive, and it is defined
by two complementary match arms of one invariant:

| Arm | Position | Forbidden shape | Reason |
|---|---|---|---|
| R1 | subject start | a commit-family token directly followed by `(`, `!`, or `:` | after assembly the line reads as a doubled header prefix, even without a ticket scope (`feat(GQA-19): feat: …`) |
| R2 | any position | a commit-family token directly followed by a parenthesized ticket scope and the colon separator (`<family>(<KEY>-<NUMBER>)[!]:`) | the full envelope is unambiguous header syntax wherever it appears, including mid-subject placement by humans or agents |

The two arms are not redundant: R2 deliberately requires the ticket shape so
legitimate mid-subject prose such as `document fix(parser) behavior` stays
allowed, while R1 covers the start position, where even the bare family
prefix is indistinguishable from a header. Neither arm alone covers the
other's position with the same precision.

### 4.1 Canonical Family Set and Ticket Grammar

The family tokens are exactly the canonical commit families — `build`,
`chore`, `ci`, `docs`, `feat`, `fix`, `perf`, `refactor`, `revert`, `style`,
`test` — and the ticket scope shape is the ticket grammar (`<KEY>-<NUMBER>`:
uppercase key, number without leading zero). The validation derives both from
the domain's single sources of truth at runtime; no duplicated literal list
exists in the validation code.

## 5. Validation Authority and Enforcement Points

The contract is enforced fail-closed by exactly one canonical invariant: the
subject validation of the commit-message domain model
(`internal/domain/commitmsg`). Every surface that creates or verifies a
commit message passes through it:

- `commit create`, via flags or the interactive prompt;
- `branch merge-scratch` and the scratch transfer of
  `workflow ticket publish`;
- the merge-commit message of `branch sync-base --strategy merge`;
- `commit validate` and the `commit-msg` hook;
- the pre-push commit-series validation of every outgoing commit.

A historical commit that violates this contract fails validation at these
boundaries; because official branches are append-only after the first push,
such a commit must be corrected while the branch is still unpublished.

## 6. Rejected Alternatives (Decision Record)

| Alternative | Verdict | Reason |
|---|---|---|
| Exact duplicate check against the assembled prefix | rejected | blind to family mismatches (`--type feat` with a `fix(…)` subject) and to any reformatting |
| Generic any-word-before-parenthesis matching | rejected | false positives without taxonomy binding; the risk is the envelope, not parentheses in prose |
| Start-anchored check only | rejected | leaves mid-subject placement open |
| Character allowlists (letters and digits only, or banning `:`, `(`, `#`) | rejected | over-restriction destroys legitimate subjects without reducing the actual risk; governance precision beats blanket prohibition |
