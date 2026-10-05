# Task worktree lifecycle

Task-bound worktree isolation hosts every local working-branch lane inside a
linked task worktree. The lifecycle endpoints acquire, inventory, and retire
those worktrees; the isolation model and its decision record live in the
[worktree isolation convention](../../conventions/worktrees/worktree-isolation.md)
and [ADR-0008](../../architecture/ADR-0008-WORKTREE-ISOLATION-MODEL.md).

## Acquire a task worktree

Interactive:

```powershell
git governance --yes workflow worktree start `
  --key ABC `
  --ticket 123
```

The endpoint fetches the selected remote and creates a detached worktree at
`../<repository>-<KEY>-<NUMBER>` from the lane's acquisition base; no branch
is created in this step. The lane-to-base mapping binds the base: regular
ticket work acquires from `develop`, hotfix work from the protected base
line, and release stabilization work from the frozen release line. Pass
`--base main`, `--base release/1.2.0`, or `--base support/1.2` explicitly
when the lane requires it; working-family lines are rejected.

The sanctioned first mutation inside the fresh worktree is the governed
`workflow ticket start` dispatch (see [Ticket start](tickets/start.md)).

Non-interactive:

```powershell
git governance --interactive never --output json --yes workflow worktree start `
  --key ABC `
  --ticket 123
```

## Inventory task worktrees

```powershell
git governance --interactive never --output json workflow worktree list
```

The inventory reports the task worktrees of the repository with their
checkout state.

## Remove one task worktree

Interactive:

```powershell
git governance --yes workflow worktree remove `
  --key ABC `
  --ticket 123
```

The endpoint removes the worktree of one completed ticket and refuses
fail-closed when the target worktree is not clean or is not a registered task
worktree. Official branches and remote branches are never removed by this
endpoint.

## Prune proven-complete task worktrees

```powershell
git governance --dry-run workflow worktree prune
git governance --yes workflow worktree prune
```

`workflow worktree prune` discovers the task worktrees whose completion
evidence is proven and removes them in one planned pass. An entry is eligible
only when the newest pull request record of its branch is merged or closed,
the branch obligations on the remote surface are fulfilled, the worktree is
clean, and no operation marker is active. Inspect the plan with `--dry-run`
before the mutating dispatch.

## Removal discipline

Removal and prune are always actor-invoked endpoints; the binary runs no
background cleanup. `workflow worktree remove` is the point operation for one
ticket; `workflow worktree prune` is the evidence-based discovery and batch
pass that composes the same guards per entry instead of duplicating them. A
deviating side-channel worktree is never removed by the binary; the actor
resolves it per the
[name reservation convention](../../conventions/worktrees/worktree-name-reservation.md).

## Where a task worktree is required

The binary enforces the task-worktree requirement fail-closed for the local
working-branch dispatches of the ticket, hotfix, and release lanes:

| Endpoints | Requirement |
|---|---|
| `workflow ticket start`, `workflow hotfix start` | required — enforced |
| `workflow hotfix propagate`, `workflow hotfix propagate-manifest` | required — enforced |
| `workflow release stabilize`, `workflow release publish-stabilization`, `workflow release align-promotion-base`, `workflow release align-reconciliation-base`, `workflow release promote`, `workflow release backmerge` | required — enforced |

Excluded from the requirement:

| Endpoints | Reason |
|---|---|
| `workflow bootstrap` | repository birth runs in the primary checkout of an unborn repository |
| `workflow release request`, `workflow release cut`, `workflow release support` | protected-line dispatch operations without local working-branch work |
| `workflow ticket inventory`, `workflow worktree list`, `hotfix validate-record`, `hotfix verify-merge`, `hotfix verify-delivery` | read-only evidence endpoints |
| `workflow cleanup` | scratch branches are created and cleaned inside the worktree |
| `workflow ticket publish`, `workflow hotfix publish` | publication runs where the branch is checked out — normally the task worktree |
| compositions without the worktree capability | the server/CI class keeps the documented non-gated behavior |

The detached pre-start form is measured against the lane's own acquisition
base, so a worktree acquired from the wrong line is refused before any branch
mutation exists.