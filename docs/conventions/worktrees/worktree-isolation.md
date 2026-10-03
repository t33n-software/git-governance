# Worktree isolation convention

## Purpose

This convention defines task-bound worktree isolation for every actor
executing ticket work in this repository — a developer working alone and an
orchestrating agent coordinating contributing agents alike. It gives parallel
ticket work a physical isolation boundary, so the tool's fail-closed gates
protect real task boundaries instead of serializing unrelated work.

The canonical decision record is
[ADR-0008](../architecture/ADR-0008-WORKTREE-ISOLATION-MODEL.md).

## Isolation model

The isolation chain is:

```text
ticket -> branch -> worktree
```

- One detached worktree exists per active ticket, created from the current
  `origin/develop` revision.
- The worktree is the highest isolation boundary on a development machine.
  Above it lie only machine and container boundaries (the CI class).
- Ticket-bound work is committed and published through the governed binary
  inside the worktree.

The convention binds every actor. Under agent orchestration, the orchestrating
actor creates the worktree and the contributing agents attach to it; this
orchestration pattern is an application class of the convention, not its
definition. A developer working alone creates and uses the worktree directly.

## Worktree acquisition

The worktree acquisition runs through the governed lifecycle endpoint:

```bash
git-governance workflow worktree start
```

The endpoint fetches the selected remote and creates the worktree detached
from the current `origin/develop` revision of that remote. No branch is
created in this step.

## Sanctioned first mutation

The first mutation inside a fresh task worktree is the governed branch
creation:

```bash
git-governance workflow ticket start
```

The branch-context rules treat a detached task worktree as a legitimate
pre-start context; the governed `workflow ticket start` dispatch creates the
official working branch inside the worktree. Every further mutation — commits
and publication — runs through the binary inside the worktree.

## Binding rules

1. One worktree per active ticket per actor; a new, independent task receives
   its own worktree.
2. Worktree acquisition runs through the governed
   `workflow worktree start` endpoint; branch creation is always governed
   (`workflow ticket start`).
3. Never commit on the detached worktree HEAD; the governed branch creation
   precedes every commit.
4. Never share one worktree across tickets.
5. Cleanup removes the worktree only after the ticket's pull request is
   merged or the work is otherwise concluded; official branch removal stays
   with the governed cleanup paths.

## Rejected forms

```bash
# Raw branch creation during acquisition — bypasses the governed path
git worktree add -b feature/ABC-123-slug ../wt origin/develop
```

A single shared working tree for unrelated parallel tickets is equally
rejected: the tool's gates then block collisions on the wrong isolation unit.

## Tool enforcement relationship

The binary's fail-closed gates already resolve worktree-correctly: worktree
aware repository discovery, per-worktree cleanliness checks, per-worktree
operation markers, and shared-line protection. The worktree lifecycle
endpoints and the publish-return worktree behavior are specified in
[ADR-0008](../architecture/ADR-0008-WORKTREE-ISOLATION-MODEL.md).
