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

After Main-promotion and delivery, new regular pull requests may merge into
`develop`. Those commits belong to the next integration phase and must not be
merged, rebased, or otherwise written back into the delivered
`release/<semver>` reference.

When the Develop protection policy accepts the release head, the controlled
direct pull request `release/<semver> -> develop` remains valid. When the
policy requires a current pull-request head, create a ticket-bound
release-preparation branch from the unchanged release line and run:

```text
release/<semver>
  -> chore/<ticket>-<reconciliation-alignment>
  -> merge origin/develop into the preparation branch
  -> quality and review gates
  -> merge-commit pull request to develop
```

Use `workflow release align-reconciliation-base` only from that checked-out
preparation branch. It verifies delivery and an effective release-only delta,
then merges `develop` only into the preparation branch. It never updates the
release ref. A rebase, a GitHub **Update branch** action, or a
`develop -> release/<semver>` pull request is not a valid reconciliation
substitute.

## Conflict recovery and privileged publication

If that controlled merge conflicts, it remains fail-closed on the non-shared
preparation branch. Resolve only the reported paths and stage only those exact
paths. Continue an active merge exclusively through:

```text
workflow release align-reconciliation-base --resume
```

The server-side recovery route uses the protected `reconciliation-resume`
operation in `release-control.yml`. It validates the ticket-bound
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
