# ADR-0004: Governed Release Reconciliation Base Alignment

- Status: accepted
- Date: 2026-08-01
- Scope: `release/<semver> -> develop` under strict develop base freshness
- Deciders: Release governance

## Context

After a successful `release/<semver> -> main` promotion, an immutable tag, and
confirmed delivery, the release must be reconciled against `develop`. New
regular pull requests may have been merged into `develop` in the meantime.

A strict develop ruleset may require a current PR head for a backmerge. A
GitHub "Update branch", a rebase, or a direct merge of `develop` into
`release/<semver>`, however, would retroactively change the already delivered
shared release line.

The release ref is preserved until the documented reconciliation completion
but is no longer a working line after promotion and delivery.

## Decision

With an effective release-only delta and strict develop base freshness, the
reconciliation happens exclusively through a ticket-bound preparation branch:

```text
release/<semver>
  -> chore/<ticket>-<reconciliation-alignment>
  -> controlled merge of origin/develop
  -> full quality, security, and review gates
  -> merge-commit PR to develop
```

The workflow `workflow release align-reconciliation-base` only accepts a
currently checked-out `chore/*` preparation branch created from the given
release line. It verifies the delivery proof and the effective delta, merges
`origin/develop` exclusively into the working branch, runs the repository
quality gates, and optionally publishes the merge-commit PR to `develop`.

When develop does not require a current PR head, the direct, controlled PR
`release/<semver> -> develop` remains permissible. When no effective delta
exists, no preparation branch PR is created; the result reads `not-required`.

## Invariants

- `release/<semver>` remains unchanged after promotion, tag, and delivery.
- New develop work is never retroactively merged into `release/<semver>`.
- A preparation branch starts from exactly the release ref and carries a
  ticket reference.
- The current develop ref is merged into the preparation branch with a
  traceable merge commit, not rebased.
- A conflict remains fail-closed until only the concrete paths have been
  resolved and staged; the continuation happens through the governed resume
  contract.
- A server-side published resolution candidate proves exactly the release ref
  as the first and the verified develop ref as the second merge parent.
- The resulting PR targets `develop` and uses a merge commit.
- A new functional change after delivery requires a new patch release or
  hotfix on the actually affected published line; it is not retroactively
  written into the delivered release.

## Rejected alternatives

### GitHub Update branch on `release/<semver> -> develop`

Rejected, because the action changes the protected release ref directly and
bypasses the ticket-bound merge, the quality gates, and the separate review
event.

### `develop -> release/<semver>` pull request

Rejected, because new integration work would become part of an already
delivered release state. The backmerge serves exclusively the return of
missing release deltas to `develop`.

### Rebase of the release ref or the preparation branch onto develop

Rejected, because a rebase obscures the relevant merge provenance. The
preparation branch must absorb the current develop state through a visible,
ticket-bound merge.

### Weakening the strict develop base freshness

Rejected, because it would also weaken regular integration PRs. The
source-aware preparation workflow preserves the target protection rule and
only resolves the release reconciliation special case.

## Consequences

- The published release lineage remains unchanged and auditable.
- Develop can continue to integrate regular work during the release delivery.
- The concrete combined state is fully verified before the backmerge.
- The backmerge remains a visible, reviewable release event.
- The release branch is only cleaned up after a successfully merged backmerge
  or an auditable `not-required`.
