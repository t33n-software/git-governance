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
-> DISCOVERY_AND_TASK_RESPONSE
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

The `DISCOVERY_AND_TASK_RESPONSE` phase runs the read-only discovery of the
active task and releases its report, answer, or question obligation before any
worktree acquisition; the blocking side state
`WAITING_FOR_TASK_RESPONSE_DECISION` holds the workflow until the user decides,
and the transition to `ENVIRONMENT_READY` requires the proofs
`task_response_released` and `shared_line_refresh_checked`.

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