# Merge strategy and repository settings
[INTENT: SPEZIFIKATION]

## Merge methods per target line

The merge strategy is a deliberate, target-dependent governance decision, not
a generic platform default. Rulesets act on the target branch, not on the
source family; the choice is made by the merge-authorized person at merge
time according to this matrix:

| Scenario | Target | Method | Reason |
|---|---|---|---|
| Regular ticket with a meaningful semantic commit series | `develop` | Rebase merge | Preserves the reviewed series without a merge commit |
| Regular ticket with a visible integration topology | `develop` | Merge commit | Explicit integration event |
| Regular ticket with internal commit noise | `develop` | Selective squash | One clean integration commit |
| Release promotion | `main` | Merge commit | Release lineage and visible approval |
| Hotfix onto the affected shared line | `main`, `release/*`, `support/*` | Merge commit | Explicit incident lineage |
| Stabilization or maintenance PR | `release/*`, `support/*` | Merge commit | Controlled shared-line history |
| Release backmerge | `develop` | Merge commit allowed | Transfers only the effective release delta after the confirmed delivery |

A local rebase before the first push of an unpublished working branch is a
different operation from the GitHub rebase merge and is allowed only there;
after the first push, the official branch remains append-only.

## The update-branch boundary

GitHub's **Update branch** action mutates the PR head ref; it is not a
metadata refresh. For a PR with a shared line as its head, that would be a
direct mutation of a protected line — the rulesets reject the underlying
update. A required base alignment of a stale promotion or reconciliation
happens exclusively through a ticket-bound preparation working branch with
full gates and its own merge-commit PR; the frozen shared line is never
updated through the platform surface.

## Repository settings (global, per repository)

| Setting | Required state | Reason |
|---|---|---|
| Allow merge commits | enabled | Required for `main`, `release/*`, `support/*`, and integration paths |
| Allow rebase merging | enabled | For the allowed rebase-merge path into `develop` |
| Allow squash merging | enabled | For the allowed selective-squash path into `develop` |
| Automatically delete head branches | enabled | Cleans up merged, deletable head branches automatically; shared lines are unaffected because of the deletion protection |
| Allow auto-merge | disabled | Auto-merge would undermine the deliberate merge-method decision and the approval sequence |
| Always suggest updating pull request branches | disabled | The visible update suggestion is not an authorization to mutate a protected PR head |
| Release immutability | enabled before the first production release is published | Published releases and tags remain immutable evidence |

The effective merge method is the intersection of the globally enabled
capability and the `allowed_merge_methods` of the target ruleset.
