# Description: INV-011 Completed-pull-request branch detection and post-publication workspace transition
[INTENT: CONTEXT]

This leaf owns exactly one behavioral invariant of the portable core: INV-011.
The invariant register and the package map live in the parent TOC surface
`description/core-invariants.md`.

---

## INV-011: Completed-pull-request branch detection and post-publication workspace transition
[INTENT: SPECIFICATION]

**Type:** WORKFLOW

**Description:**

The core probes the pull-request state of a checked-out official working
branch at intake, before any continuation decision:

```text
official working branch checked out
-> probe its pull-request state through the platform read paths of the
   discovery chain (P2/P3)
-> bind current_branch_pr_state = none | open | merged | unknown
```

A branch with an open or merged pull request is a completed handoff: its task
was already handed to review. For a new, independent task the agent never
interrogates that branch's status, never evaluates its commits, and never asks
a continuation question; it starts the new ticket intake directly and leaves
the existing branch untouched. A task that recognizably continues the open
pull request's scope — review feedback or a requested change — remains a
valid continuation under the unchanged existing rules. When the probe is
unavailable (`unknown`), the previous continuation decision applies unchanged.

The checked-out branch is evidence, never intent: a developer may have
switched branches manually between sessions, so the continuation decision
binds to probed states (worktree, active operations, pull-request state),
never to the mere presence of a branch.

After a successful publish run with a created pull request, the local
workspace is back on the `develop` integration line: the binary performs the
governed return switch and reports its outcome. The agent verifies that end
state through the branch context and treats it as the task's expected
completion signal, never as a new task source. If the workspace still sits on
the ticket branch — after a skipped or failed return, a manual switch, or an
older binary — the next start follows the continuation rules above.

**Current State:**

The core classified the checked-out branch and decided continuation from the
branch alone, so a branch whose pull request was already created still
triggered continuation analysis on the next task, and a completed publish left
the workspace on the ticket branch.

**Target State:**

The core probes the branch's pull-request state at intake, treats an open or
merged pull request as a completed handoff for any new independent task, and
expects the post-publication workspace to be back on the `develop`
integration line.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Branch-context probe and continuation decision | Sections [3.1] and [3.3] |
|| `core/prompt.md` | Intake decision matrix | Section [4.3] |
|| `core/prompt.md` | Post-publication workspace expectation | Section [8] |
|| `core/prompt.md` | State surface, proof, and audit record | Sections [2.2], [2.3], and [9] |

**Positive Example(s):**

```text
branch feature/ABC-123-add-export is checked out with an open pull request
-> a new independent task arrives
-> the branch is a completed handoff: start the new ticket intake directly
```

**Negative Example(s):**

```text
branch feature/ABC-123-add-export is checked out with an open pull request
-> the agent re-verifies the branch status, scans its commits, and asks
   whether to continue, although the new task is independent
```

Status archaeology on a completed handoff is the noise the probe eliminates.

---

## Conventions and Constraints
[INTENT: CONSTRAINT]

- This invariant is binding for every consuming agent and adapter.
- An invariant change is a material core-contract change and belongs in
  `core/CHANGELOG.md` with a semantic version, a compatibility assessment,
  and the affected surfaces.