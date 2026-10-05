# Description: INV-004 Task-pattern recognition and execution-level hierarchy
[INTENT: CONTEXT]

This leaf owns exactly one behavioral invariant of the portable core: INV-004.
The invariant register and the package map live in the parent TOC surface
`description/core-invariants.md`.

---

## INV-004: Task-pattern recognition and execution-level hierarchy
[INTENT: SPECIFICATION]

**Type:** CONSTRAINT

**Description:**

Before any mutation planning, the core classifies the request into exactly one
task pattern (`ticket`, `hotfix`, `release`, `support`, `exploration`,
`diagnostic`) and binds it with the branch class through the multi-decision
matrix to the mandatory entry path. Every Git effect runs through exactly one
execution level:

```text
Level 1: governed workflows (workflow ticket|hotfix|release|cleanup)
         mandatory whenever the task pattern is covered
Level 2: bounded CLI commands and subcommands (branch, commit, validate, ...)
         only for a bounded action no current workflow covers
Level 3: raw Git
         only for read-only orientation and effects neither level covers;
         never a replacement for a governed capability and never a mutation
         on a shared line
```

A workflow is never rebuilt from manually chained level-2 commands. The only
sanctioned raw-Git mutation is the explicit staging of resolved conflict paths
inside the conflict protocol, immediately followed by the governed resume
endpoint: for a rebase or merge paused by `branch sync-base`, the resume mode
of the same endpoint; for an operation paused by a workflow, the resume entry
of that owning workflow.

**Current State:**

The conflict protocol named the governed resume endpoint only generically as
whatever the current help offers, without binding the concrete entry point for
a synchronization paused by the standalone `branch sync-base` command.

**Target State:**

The conflict protocol binds the governed resume step to the concrete governed
entry points: the resume mode of `branch sync-base` for rebase or merge
operations paused by that endpoint, and the owning workflow's resume entry for
workflow-paused operations. The endpoint registry row for `branch sync-base`
binds the same capability. The prompt still names no flag; the current help
exposes the actual resume invocation form.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Execution levels and conflict protocol | Sections [4.2] and [7] |

**Positive Example(s):**

```text
resolve the exact conflicted paths
-> stage only the resolved paths
-> continue through the governed resume entry the current help exposes
   for the endpoint that paused the operation
-> re-verify base, provenance, and quality
```

**Negative Example(s):**

```text
resolve conflicts and run a raw rebase or merge continuation
```

A raw continuation bypasses the governed resume endpoint and skips branch
re-validation, publication guards, and the quality rerun.

---

## Conventions and Constraints
[INTENT: CONSTRAINT]

- This invariant is binding for every consuming agent and adapter.
- An invariant change is a material core-contract change and belongs in
  `core/CHANGELOG.md` with a semantic version, a compatibility assessment,
  and the affected surfaces.