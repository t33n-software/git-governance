# ADR-0008: Task-Bound Worktree Isolation Model

- Status: accepted
- Date: 2026-10-03
- Scope: parallel ticket work on one development machine
- Deciders: Repository governance

## Context

Ticket work by multiple actors — developers and agent orchestration — shares
one development machine. A single shared working tree makes concurrent ticket
work collide: the tool's fail-closed gates (clean-worktree checks,
active-operation markers, the shared-line guard) correctly block these
collisions, but they block them on the wrong isolation unit. No isolation
boundary existed between "one machine" and "one branch"; unrelated tickets
serialize on one working tree, and parallel agent work has no sanctioned
physical separation.

## Decision

Ticket work is isolated in task-bound worktrees, and the convention binds
every actor executing ticket work — a developer working alone and an agent
orchestration alike:

- One detached worktree per active ticket, created from the current
  `origin/develop` revision.
- The worktree is the highest isolation boundary on a development machine;
  above it lie only machine and container boundaries (the CI class).
- The worktree acquisition runs through the governed
  `workflow worktree start` endpoint, which fetches the selected remote and
  creates the worktree detached from the current `origin/develop` revision.
- The sanctioned first mutation inside the worktree is the governed
  `workflow ticket start` dispatch, which creates the official working branch.
- Commits and publication run through the binary inside the worktree.
- Under agent orchestration, the orchestrating actor creates the worktree and
  contributing agents attach to it; the orchestration pattern is an
  application class of the convention, not its definition.

The working convention is canonical in
`docs/conventions/worktrees/worktree-isolation.md`.

## Invariants

- One worktree per active ticket per actor; a new, independent task receives
  its own worktree.
- Acquisition is detached; no branch is created outside the governed path.
- No commit on the detached worktree HEAD; the governed branch creation
  precedes every commit.
- A worktree is never shared across tickets.
- Cleanup removes the worktree only after the ticket's pull request is merged
  or the work is otherwise concluded.
- The fail-closed gates keep evaluating per worktree: cleanliness, active
  operations, and shared-line protection are worktree-scoped.

## Rejected alternatives

### Single shared working tree with gates only

Rejected: the gates block collisions correctly but on the wrong isolation
unit; unrelated tickets serialize on one tree and parallel agent work stays
unsanctioned.

### One full clone per task

Rejected: a clone duplicates the object store and remote configuration per
task; a worktree is Git's native mechanism for exactly this isolation and
shares the object store with the primary checkout.

### Long-lived worktree per developer

Rejected: it couples unrelated tickets to one long-lived tree, loses the
task-boundary freshness of a detached start from the current `origin/develop`
revision, and hides which task a tree belongs to.

## Consequences

- The workflow core treats a detached task worktree as a legitimate pre-start
  context with the governed `workflow ticket start` dispatch as the sanctioned
  first mutation.
- The binary provides the worktree lifecycle endpoints and publish-return
  awareness for the worktree checkout shape; the fail-closed gates remain per
  worktree.
- The derived task-worktree name of a ticket is reserved case-insensitively,
  and every acquisition collision is named fail-closed per the name
  reservation convention (`docs/conventions/worktrees/worktree-name-reservation.md`).
- Raw Git remains forbidden for the complete worktree lifecycle; acquisition,
  branch creation, commits, and publication stay governed.
- Cleanup discipline extends the governed branch cleanup with worktree
  removal.
