# Description: Core Invariants
[INTENT: CONTEXT]

---

## 1. Scope Overview
[INTENT: CONTEXT]

This detail surface owns the behavioral invariants of the portable core:
help-first invocation, the deterministic workflow state machine, the
shared-line guard and mutation embargo, the task-bound worktree requirement
across the local working-branch lanes, task-pattern recognition with the
execution-level hierarchy, the area-symbol registry, safe Scratch usage,
proactive key and ticket discovery, the scoped one-time provider-session
verification, the completed-pull-request branch detection with the
post-publication workspace transition, the current hotfix capability
boundary, and the semantic commit-message and pull-request-description
content governance.

This surface is the TOC root of the core-invariants package: every INV unit
lives in exactly one leaf file under `description/core-invariants/`, and the
description index below is the navigation map into those leaves.

---

## 2. Information Register
[INTENT: REFERENCE]

|| ID | Type | Description | Change | Status |
||----|------|-------------|--------|--------|
|| INV-001 | CONSTRAINT | Help-first invocation | No | Active |
|| INV-002 | CONSTRAINT | Deterministic workflow state | No | Active |
|| INV-003 | CONSTRAINT | Shared-line guard and mutation embargo | No | Active |
|| INV-004 | CONSTRAINT | Task-pattern recognition and execution-level hierarchy | Yes | Active |
|| INV-005 | CONSTRAINT | Area-symbol registry | No | Active |
|| INV-006 | CONSTRAINT | Safe Scratch usage | No | Active |
|| INV-007 | WORKFLOW | Proactive key and ticket discovery | Yes | Active |
|| INV-008 | WORKFLOW | Scoped one-time provider-session verification | No | Active |
|| INV-009 | CONSTRAINT | Current hotfix capability boundary | No | Active |
|| INV-010 | CONSTRAINT | Commit-message and pull-request-description content governance | Yes | Active |
|| INV-011 | WORKFLOW | Completed-pull-request branch detection and post-publication workspace transition | Yes | Active |
|| INV-012 | CONSTRAINT | Governed repository birth and the unborn branch context | Yes | Active |
|| INV-013 | CONSTRAINT | Task-bound worktree requirement across the local working-branch lanes | Yes | Active |

---

## 3. Description Index
[INTENT: REFERENCE]

|| # | Path | Scope | Reason |
||---|------|-------|--------|
|| 1 | `description/core-invariants/inv-001-help-first-invocation.md` | INV-001 | Help-first runtime contract |
|| 2 | `description/core-invariants/inv-002-deterministic-workflow-state.md` | INV-002 | State model, proofs, transitions |
|| 3 | `description/core-invariants/inv-003-shared-line-guard-and-mutation-embargo.md` | INV-003 | Branch classes and embargo |
|| 4 | `description/core-invariants/inv-004-task-pattern-and-execution-levels.md` | INV-004 | Execution levels and conflict protocol |
|| 5 | `description/core-invariants/inv-005-area-symbol-registry.md` | INV-005 | Symbol registry and audit records |
|| 6 | `description/core-invariants/inv-006-safe-scratch-usage.md` | INV-006 | Scratch decision matrix |
|| 7 | `description/core-invariants/inv-007-proactive-key-and-ticket-discovery.md` | INV-007 | Discovery chain and proposal matrix |
|| 8 | `description/core-invariants/inv-008-provider-session-verification.md` | INV-008 | Provider-session prefetch |
|| 9 | `description/core-invariants/inv-009-hotfix-capability-boundary.md` | INV-009 | Hotfix endpoint family |
|| 10 | `description/core-invariants/inv-010-commit-and-pr-content-governance.md` | INV-010 | Commit content and pull-request description architecture |
|| 11 | `description/core-invariants/inv-011-completed-pr-branch-detection.md` | INV-011 | Branch-context probe, continuation, post-publication transition |
|| 12 | `description/core-invariants/inv-012-governed-repository-birth.md` | INV-012 | Unborn branch context and birth routing |
|| 13 | `description/core-invariants/inv-013-task-worktree-requirement.md` | INV-013 | Worktree rule and branch-context matrix |

---

## 4. Conventions and Constraints
[INTENT: CONSTRAINT]

- Every invariant is binding for every consuming agent and adapter.
- An invariant change is a material core-contract change and belongs in
  `core/CHANGELOG.md` with a semantic version, a compatibility assessment,
  and the affected surfaces.
- Each INV unit lives in exactly one leaf file under
  `description/core-invariants/`; a leaf that aggregates multiple invariants
  is forbidden.

---

## 5. Path Index
[INTENT: REFERENCE]

|| # | Path | Relevance | Unit IDs |
||---|------|-----------|----------|
|| 1 | `description/core-invariants/inv-001-help-first-invocation.md` | INV-001 leaf | INV-001 |
|| 2 | `description/core-invariants/inv-002-deterministic-workflow-state.md` | INV-002 leaf | INV-002 |
|| 3 | `description/core-invariants/inv-003-shared-line-guard-and-mutation-embargo.md` | INV-003 leaf | INV-003 |
|| 4 | `description/core-invariants/inv-004-task-pattern-and-execution-levels.md` | INV-004 leaf | INV-004 |
|| 5 | `description/core-invariants/inv-005-area-symbol-registry.md` | INV-005 leaf | INV-005 |
|| 6 | `description/core-invariants/inv-006-safe-scratch-usage.md` | INV-006 leaf | INV-006 |
|| 7 | `description/core-invariants/inv-007-proactive-key-and-ticket-discovery.md` | INV-007 leaf | INV-007 |
|| 8 | `description/core-invariants/inv-008-provider-session-verification.md` | INV-008 leaf | INV-008 |
|| 9 | `description/core-invariants/inv-009-hotfix-capability-boundary.md` | INV-009 leaf | INV-009 |
|| 10 | `description/core-invariants/inv-010-commit-and-pr-content-governance.md` | INV-010 leaf | INV-010 |
|| 11 | `description/core-invariants/inv-011-completed-pr-branch-detection.md` | INV-011 leaf | INV-011 |
|| 12 | `description/core-invariants/inv-012-governed-repository-birth.md` | INV-012 leaf | INV-012 |
|| 13 | `description/core-invariants/inv-013-task-worktree-requirement.md` | INV-013 leaf | INV-013 |
|| 14 | `core/prompt.md` | Portable core workflow | INV-001 to INV-013 |

---

## 6. Execution Context for LLM Agents
[INTENT: CONTEXT]

Read this surface after the root `DESCRIPTION.md` index and before relying on
any single invariant. The invariants apply together; the full detail of each
INV unit lives in its leaf file under `description/core-invariants/`. The
conflict-recovery binding in INV-004 is the authority for how a paused
synchronization is resumed. The completed-handoff rule in INV-011 is the
authority for continuation decisions on an official working branch with an
existing pull request.