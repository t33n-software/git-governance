# Commit Family Selection Contract

## 1. Purpose

This document is the canonical project convention for how the commit family
of a governed commit is determined. It binds both input modes of the CLI and
every creation endpoint.

## 2. The Family Is Always an Explicit Decision

- **Non-interactive mode:** the family is always passed explicitly via the
  `--type` argument. A missing family fails closed; nothing is derived.
- **Interactive mode:** the family is chosen through an explicit selection.
  The branch family's default expectation may preselect the proposal, but the
  selection is confirmed by the author — a preselection is a proposal, never
  a decision.
- **No silent fallback:** the CLI never silently derives the commit family
  from the branch family. The silent derivation path is removed, not
  deprecated.

Rationale: the family classifies the functional outcome of the unit and
steers body duty, release semantics, and history readability. A value with
that governance weight is never set without an explicit, auditable decision.

## 3. No Branch Family Enforces a Single Commit Family

Branch families and commit families are orthogonal classification axes: the
branch family classifies the scope of a work unit; the commit family
classifies the nature of each semantic change within it. No branch family can
enforce a single commit family, for these complete reasons:

1. Review corrections on every working branch are commits of `fix` nature.
2. Semantically clean series separate implementation, tests, and
   documentation; `test` and `docs` commits are legitimate on every working
   branch.
3. Tooling-adjacent `chore` commits are possible on every branch — for
   example documentation tooling on a `docs/*` branch.
4. A machine-enforced single-family rule would be unsound: it would block
   legitimate companion commits or force misclassification, corrupting the
   history it exists to protect.
5. Shared lines (`main`, `develop`, `release/*`, `support/*`) carry no
   direct commits; there is nothing to enforce.

The legitimate commit families are, completely: `build`, `chore`, `ci`,
`docs`, `feat`, `fix`, `perf`, `refactor`, `revert`, `style`, `test`.

Consequently there is no branch-family × commit-family restriction matrix and
no per-family or per-mode special case ("derive here, require there"): such a
matrix would be a second, error-prone maintenance plane with regime ambiguity
at every call site. There is exactly one regime: always explicit.

The help surface of every family-carrying flag renders this complete set from
the canonical value-domain register — never from a hand-maintained copy — as
bound by `docs/specification/CLI-VERTRAG.md`, section 3 (Flag-Hilfe und
Wertedomänen).

## 4. Body Duty

The commit body is the default, not an option. The CLI enforces the body
fail-closed wherever the mandatory context is machine-known:

- commits on `hotfix/*` branches;
- commits on release-stabilization branches;
- commits marked breaking (`--breaking`);
- the scratch squash transfer (`branch merge-scratch`, and publishing from
  scratch via `workflow ticket publish`), whose body is the only record of
  the discarded experiment paths.

For all other units the body-duty matrix remains a content contract: a body
omission is only legitimate when the unit is provably trivial and
self-explanatory, and the omission is reported as justified.

## 5. Single Input Representation

Commit creation speaks structured values only: `--type`, `--subject`,
`--body`, `--footer`, `--breaking`, and `--breaking-description`. The project
deliberately defines no second input level on which a complete pre-assembled
message could be supplied; the envelope can therefore never be authored by
callers at all. Raw complete messages exist only at the verification boundary
— `commit validate` and the `commit-msg` hook — where an already-assembled
message is handed over for validation.
