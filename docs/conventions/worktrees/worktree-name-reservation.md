# Worktree name reservation and side-channel worktrees

## Purpose

This convention defines the name reservation of task worktrees and the
governed handling of worktrees created outside the governed lifecycle
endpoints (side-channel worktrees). It closes the gap between the physical
directory surface of a developer machine and the governed allocation
inventory: the derived task-worktree name of a ticket is reserved, every
collision is named fail-closed, and the allocation inventory remains the
only ticket-number truth.

The canonical decision record for the task-bound isolation model is
[ADR-0008](../../architecture/ADR-0008-WORKTREE-ISOLATION-MODEL.md).

## Name reservation

The task-worktree directory name of a ticket is derived by the governed
acquisition endpoint as a sibling directory named after the repository and
the ticket (`<repo>-<KEY>-<NUMBER>`, canonical key case). That derived
name is reserved per this convention:

- The governed `workflow worktree start` matches the derived name
  case-insensitively against the worktree inventory before any mutation.
- Every inventory entry whose path is case-insensitively equal to the
  derived name blocks the acquisition fail-closed (`WORKTREE_CONFLICT`),
  regardless of how the entry was created.
- The name is reserved by convention, not by filesystem case rules; the
  reserved-name semantics are identical on case-sensitive and
  case-insensitive filesystems.

## Side-channel worktrees

A side-channel worktree is a worktree directory created outside the
governed lifecycle endpoints — by a developer running raw Git commands or
by an agent bypassing the endpoints. Two classes exist:

1. **Convention-conforming side-channel worktree.** The directory name
   matches the `<repo>-<KEY>-<NUMBER>` grammar of the ticket. The
   allocation inventory reads it as the registry holder of that ticket
   number, exactly like a governed acquisition: the number is held, not
   free. The worktree state gates (detached, linked, clean, at the
   acquired base revision) decide whether the sanctioned first mutation
   may run inside it; the creation channel is not an enforcement surface
   and is deliberately not detected.

2. **Deviating side-channel worktree.** The directory name deviates from
   the grammar of its logical ticket name — for example a lowercase key
   spelling. The allocation inventory cannot parse it, so the number
   stays truthfully free, and the governed `workflow worktree start`
   fails closed on the case-insensitive name reservation with a record
   that names the deviating entry and states that it is not a governed
   task worktree of the ticket. A deviating worktree is never removed by
   the binary; the actor resolves it.

## Creation channel is not an enforcement surface

The architecture deliberately enforces the worktree state, not the
creation channel:

- The pre-start acceptance (detached, linked, clean, at the acquired base
  revision) and the continuing form (official branch checked out in the
  worktree) are the governance surfaces. A convention-conforming
  side-channel worktree that passes them is state-equivalent to a
  governed acquisition.
- No provenance marker is written or verified. Any marker would be
  forgeable by the same actor who can create the side-channel worktree,
  so it carries no evidence value.

## Ticket-number truth

The allocation inventory is the only ticket-number truth. Its surfaces
include the worktree registry, local and remote branch refs, commit
envelopes, hotfix release records, pull-request titles, and protected-line
request records:

- A convention-conforming side-channel worktree holds its ticket number;
  `nextFree` skips it; the number is consumed by the governed ticket
  start or released by the governed removal while no other holder exists.
- A deviating side-channel worktree holds nothing: the number stays
  truthfully free and may be legitimately allocated and worked.
- The remote surfaces carry the cross-machine allocation; a local corpse
  never blocks a number another machine legitimately works.

## Removal and residue

- `workflow worktree remove` matches the registered entry
  case-insensitively against the derived name and removes the actual
  registered path, so a case-variant spelling of the same logical name
  stays addressable through the governed endpoint.
- A deviating worktree whose name is genuinely foreign to its logical
  ticket name is actor-resolved hygiene; the binary builds no removal
  machinery for it.
- Removal never frees a ticket number held by other surfaces; commit
  envelopes and pull-request titles are permanent allocation holders.
