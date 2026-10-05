# Description: INV-002 Deterministic workflow state
[INTENT: CONTEXT]

This leaf owns exactly one behavioral invariant of the portable core: INV-002.
The invariant register and the package map live in the parent TOC surface
`description/core-invariants.md`.

---

## INV-002: Deterministic workflow state
[INTENT: SPECIFICATION]

**Type:** CONSTRAINT

**Description:**

The core uses explicit states and proof gates rather than treating an
uninspected branch, a successful shell command, or an intended PR as proof of
completion. The state machine is:

```text
BRANCH_CONTEXT_CHECK
-> SHARED_LINE_GUARD
-> ENVIRONMENT_READY
-> INTAKE_READY
-> EXECUTION_LEVEL_BOUND
-> BRANCH_READY
-> EXECUTING
-> VERIFIED
-> COMMIT_READY
-> PUBLICATION_READY
-> PR_CREATED
-> COMPLETE
```

It distinguishes:

```text
regular ticket work
hotfixes on active affected lines
release and support lifecycle work
release reconciliation
delivery waiting states
conflict recovery
```

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | State model, proofs, transitions | Section [2] |

---

## Conventions and Constraints
[INTENT: CONSTRAINT]

- This invariant is binding for every consuming agent and adapter.
- An invariant change is a material core-contract change and belongs in
  `core/CHANGELOG.md` with a semantic version, a compatibility assessment,
  and the affected surfaces.