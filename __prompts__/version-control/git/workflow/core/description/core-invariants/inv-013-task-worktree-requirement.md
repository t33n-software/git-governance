# Description: INV-013 Task-bound worktree requirement across the local working-branch lanes
[INTENT: CONTEXT]

This leaf owns exactly one behavioral invariant of the portable core: INV-013.
The invariant register and the package map live in the parent TOC surface
`description/core-invariants.md`.

---

## INV-013: Task-bound worktree requirement across the local working-branch lanes
[INTENT: SPECIFICATION]

**Type:** CONSTRAINT

**Description:**

Every local working-branch lane hosts its work inside a linked task worktree.
The branch-context matrix binds the requirement for regular ticket work, for
hotfix starts, and for release stabilization, preparation, and propagation
dispatches — on shared lines through the governed `workflow worktree start`
acquisition and inside a fresh detached task worktree as the legitimate
pre-start context. The binary enforces the requirement fail-closed: lane
dispatches outside a linked task worktree are refused with the bound
worktree-required failure, and the remediation points to the governed
worktree acquisition. Read-only, remote-dispatch, and birth endpoints and
compositions without the worktree capability (the server/CI class) stay
outside the requirement.

The detached pre-start form is measured against the lane's own acquisition
base: regular ticket work from the current `origin/develop` revision, hotfix
work from the affected protected line, and release stabilization or
preparation work from the frozen release line. The concrete endpoint form of
the base selection stays binary-help ownership.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Worktree rule and branch-context matrix | Sections [3.1] and [4.3] |

**Positive Example(s):**

```text
a hotfix start inside a linked task worktree acquired from the affected line
-> the dispatch proceeds and creates the hotfix branch from the protected base
```

**Negative Example(s):**

```text
a hotfix start from the primary checkout
-> the dispatch fails closed with the worktree-required failure before any
   branch or worktree mutation
```

A primary-checkout dispatch would host lane work outside the isolation
boundary the convention binds, so the enforcement refuses it before any
mutation exists.

---

## Conventions and Constraints
[INTENT: CONSTRAINT]

- This invariant is binding for every consuming agent and adapter.
- An invariant change is a material core-contract change and belongs in
  `core/CHANGELOG.md` with a semantic version, a compatibility assessment,
  and the affected surfaces.