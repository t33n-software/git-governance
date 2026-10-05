# Description: INV-006 Safe Scratch usage
[INTENT: CONTEXT]

This leaf owns exactly one behavioral invariant of the portable core: INV-006.
The invariant register and the package map live in the parent TOC surface
`description/core-invariants.md`.

---

## INV-006: Safe Scratch usage
[INTENT: SPECIFICATION]

**Type:** CONSTRAINT

**Description:**

Scratch is optional private exploration. The core uses a weighted decision
matrix:

```text
0–39   direct official work
40–59  read-only clarification, then reassess
60–100 scratch before speculative implementation
```

It prevents the former anti-pattern of creating a Scratch branch merely
because a workflow starts or a task is non-trivial. Scratch is created only
through the governed ticket workflow path, never directly from a shared line
through `branch create` or raw Git.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Scratch decision matrix | Section [4.8] |

---

## Conventions and Constraints
[INTENT: CONSTRAINT]

- This invariant is binding for every consuming agent and adapter.
- An invariant change is a material core-contract change and belongs in
  `core/CHANGELOG.md` with a semantic version, a compatibility assessment,
  and the affected surfaces.