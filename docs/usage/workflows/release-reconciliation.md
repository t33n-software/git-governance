# Release reconciliation

Release reconciliation is the mandatory final assessment of a delivered
`release/<semver>` line against `develop`. It is not an unconditional request
to create an empty pull request.

## Preconditions

Before reconciliation, all of these facts must be authoritative:

1. The `release/<semver> -> main` pull request merged.
2. The immutable `v<semver>` tag points exactly to that promotion merge commit.
3. The configured release artifact workflow completed and published the
   non-draft GitHub Release, or the repository explicitly marks that delivery
   component as not applicable.

The order is causal, not time-based. A release branch is not eligible for
backmerge merely because a main pull request was opened or because a tag name
exists.

## Conditional result

Run the governed reconciliation:

```powershell
git governance --interactive never --output json --yes `
  --pull-request-provider github workflow release backmerge `
  --release release/2.8.0 `
  --create-pull-request
```

The GitHub lifecycle adapter verifies the promotion, tag, and published release
before comparing `release/2.8.0` with `develop`.

| Result | Meaning | Required action |
|---|---|---|
| `required` | A release-only effective delta remains outside `develop`. | Create or use the `release/2.8.0 -> develop` PR and merge it with a merge commit. |
| `not-required` | No effective delta remains. For example, a stabilization change was already independently carried to `develop`. | Do not create an empty PR; retain the returned evidence in the release audit. |

A commit-count difference alone is not final authority. The provider comparison
also checks whether an effective content delta remains.

## Strict Develop base freshness

New regular pull requests may merge into `develop` between Main-promotion and
reconciliation. They belong to the next integration phase and must never be
merged, rebased, or otherwise written back into the delivered
`release/<semver>` reference.

When the Develop protection policy accepts the release head, the controlled
direct pull request `release/<semver> -> develop` remains valid. When the
policy requires a current pull-request head, the protected
`reconciliation-align` operation in `release-reconciliation.yml` builds a
trusted binary from `main`, creates a ticket-bound release-preparation branch,
and executes `workflow release align-reconciliation-base` there. The operation
merges current Develop only into the preparation branch and opens the reviewed
merge-commit PR to Develop. It never updates the release ref.

In the target lifecycle, successful release delivery automatically dispatches
this controller. It revalidates promotion, tag, published release, artifacts,
attestations, ticket metadata, and idempotency before it creates a PR. Manual
dispatch exists only for incident, retry, or recovery and follows the same
checks.

## Conflict recovery

A conflict while merging the current Develop ref into the ticket-bound
Preparation-Branch is an expected fail-closed outcome. It does not authorize
GitHub **Update branch**, a rebase, a direct update of `release/<semver>`, or
an automatic `ours`/`theirs` decision.

The failed controller run is the conflict manifest. It records the delivered
release, current Develop context, ticket-bound branch, conflict paths, and
provider-delivery evidence without exposing credentials. It creates neither a
reconciliation PR nor a remote unresolved branch.

Resolve semantic conflicts only in a non-shared, ticket-bound resolution
workspace. Start the Preparation-Branch from the delivered release through the
governed CLI, let the controlled alignment start the merge, resolve and stage
only the exact conflicted paths, then resume and push the completed merge:

```powershell
git governance --interactive never --output json --yes `
  workflow release stabilize `
  --release release/2.8.0 `
  --kind release-prep `
  --key ABC `
  --ticket 999 `
  --slug reconcile-release-2-8-0

git governance --interactive never --output json --yes `
  workflow release align-reconciliation-base `
  --release release/2.8.0

# Resolve exact conflicts and stage only those paths.

git governance --interactive never --output json --yes `
  workflow release align-reconciliation-base `
  --release release/2.8.0 `
  --resume `
  --push
```

The local workspace may prepare and push the non-shared candidate branch, but
it must not create the Develop PR or receive release-automation credentials.
Dispatch `reconciliation-resume` from `release-reconciliation.yml` on `main`
with the same release, ticket, slug, and the exact candidate branch. This dispatch is a
deliberate admission act: whoever triggers it — a human in the GitHub UI, a
human or agent through `gh`, or a separate explicit command — is
trust-equivalent, because the protected environment, the server-side
provenance revalidation, and the publisher identity carry the control. The
local CLI therefore never triggers this workflow as an automatic side effect
of the candidate push; the trigger-boundary rationale is recorded in
[ADR-0004](../../architecture/ADR-0004-TRUSTED-RELEASE-RECONCILIATION-CONTROL.md).
The trusted controller accepts the branch only when it proves all of the
following:

- its ticket-bound `chore/*` name matches the supplied ticket and slug;
- its merge commit has the immutable release ref as first parent and the
  current Develop ref as second parent;
- no Develop commit advanced after that merge;
- delivery, delta, quality, security, and review gates succeed again.

Only then does CI use the `release-reconciliation` environment and its
dedicated publisher broker to publish the reviewed merge-commit candidate and
PR to `develop`. The publisher App has only repository contents and pull
request permissions required for this path; it has no Ruleset bypass or
shared-line mutation role.

## Conflict recovery and privileged publication

If that controlled merge conflicts, it remains fail-closed on the non-shared
preparation branch. Resolve only the reported paths and stage only those exact
paths. Continue an active merge exclusively through:

```text
workflow release align-reconciliation-base --resume
```

The server-side recovery route uses the protected `reconciliation-resume`
operation in `release-reconciliation.yml`. It validates the ticket-bound
`resolution_branch`, exact two-parent release/develop merge provenance,
delivery, quality, and the current Develop revision before publishing a
preparation-branch PR. The local resolution workspace never receives the
publisher credential, and neither route may mutate `release/<semver>` or
`develop` directly.

## Cleanup

The release line remains protected until one reconciliation outcome is proven:

- a required backmerge PR merged successfully; or
- a `not-required` result was recorded with the promotion, tag, release, and
  comparison evidence.

The local CLI never deletes an official release branch. GitHub or controlled CI
performs that lifecycle action after the release record is complete.

## End-to-end verification after a feature release

Run this sequence only after the change under review is merged into `develop`
and the release owner approves a concrete SemVer version:

1. Run `workflow release cut --dispatch` and confirm the returned workflow URL
   succeeded and `origin/release/<semver>` exists.
2. Complete only permitted stabilization work, if any, through its own PRs to
   the frozen release line.
3. Create and merge the reviewed `release/<semver> -> main` promotion PR.
4. Confirm the immutable tag, successful artifact workflow, and published
   GitHub Release.
5. Run `workflow release backmerge --create-pull-request`.
6. Verify either the required `release/<semver> -> develop` PR or the
   `not-required` evidence.
7. Let controlled hosting automation clean the release branch only after that
   outcome.

Do not substitute a local Git push, a manually invented tag, or a no-op pull
request for any of these provider-verified lifecycle steps.
