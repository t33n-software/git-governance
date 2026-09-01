# ADR-0003: Governed Release Promotion Base Alignment

- Status: accepted
- Date: 2026-08-01
- Scope: `release/<semver> -> main` under strict main base freshness
- Deciders: Release governance

## Context

`main` and a frozen `release/<semver>` line can diverge on both sides. A mere
note that the promotion PR is "out of date" proves neither a merge conflict
nor that all missing main commits belong in the release.

The main ruleset requires up-to-date required status checks. GitHub may show
an action like **Update branch** for a PR. That action, however, updates the
head ref of the PR. For a promotion PR with `release/<semver>` as its head,
this would be a direct mutation of a protected shared line.

A rebase of the release line is equally ruled out: it rewrites published
release history and requires a non-fast-forward update. Weakening the main
ruleset, on the other hand, would leave the combined main/release state
unchecked.

## Decision

A required promotion base alignment happens exclusively through a
ticket-bound `release-prep` working branch:

```text
release/<semver>
  -> chore/<ticket>-<promotion-alignment>
  -> controlled merge of origin/main
  -> quality gates and review
  -> merge-commit PR to release/<semver>
  -> re-verification of the existing release/<semver> -> main PR
```

The existing `release/*` ref is never aligned through GitHub **Update
branch**, rebase, force push, or a direct developer or CI code commit.

The alignment is only permissible when the following are proven before the
merge:

1. The main ruleset actually requires a current promotion head.
2. The missing main commits are already represented in content on the release
   line or are explicitly approved for this release version.
3. The working branch was created from exactly the affected release line.
4. The combined state passes all quality, security, and review gates applying
   to the release line.

The local product adapter is `workflow release align-promotion-base`. It is
not a new canonical Git policy: a protected workflow or a narrowly scoped
GitHub App / hosting integration is equivalent when it enforces the same
invariants.

## Enforcement

The decision is enforced on several levels:

- `04-release.json` protects `release/*` with the PR requirement,
  `non_fast_forward`, review, status checks, and exclusively merge commits.
- The workflow only accepts the checked-out `chore/*` release preparation
  branch with the stored base `origin/release/<semver>`.
- The merge of `origin/main` is created on the working branch with a
  ticket-bound message, not on the shared line.
- A conflict remains fail-closed on the working branch. The controlled resume
  path requires explicitly staged resolution paths, binds `MERGE_HEAD` to the
  fetched main revision, and discards a candidate when main has moved on
  before publication.
- The repository quality gates run before push and PR.
- The PR back onto `release/*` and the later promotion PR to `main` remain
  independent review events.

GitHub rulesets can protect the underlying release ref but cannot separately
hide a UI button based on the PR head family. Therefore the release ref
mutation must fail server-side; the source-aware decision remains the task of
the controlled workflow.

## Rejected alternatives

### GitHub Update branch on a promotion PR

Rejected, because the action updates the `release/*` head directly and thereby
bypasses the ticket, the provenance, the separate release review, and the
explicit quality gates.

### Rebase of `release/*` onto `main`

Rejected, because the shared-line history is rewritten and a non-fast-forward
update is required.

### Disabling the strict main base freshness

Rejected, because the result set of main and release would then not
necessarily be checked by the required checks.

### Merge queue as the sole solution

Only permissible when it checks the resulting merge against the current main
and additionally models the release-specific approval and compatibility
decision. It does not replace release preparation provenance.

## Consequences

- An "out of date" note first triggers a semantic release compatibility
  check, not an automatic Git operation.
- A reusable, auditable alignment is possible without allowing release-line
  rewrites.
- Release and main PRs remain separate governance boundaries.
- After a successful promotion, the tag, the delivery proof, and the
  delta-conditioned reconciliation to `develop` follow as before.

## References

- `docs/specification/policy-and-validation.md`
- `docs/usage/workflows/release.md`
- `docs/specification/cli-contract.md`
- `rulesets/github/04-release.quality-gates-full.json`
- `rulesets/github/README.md`
