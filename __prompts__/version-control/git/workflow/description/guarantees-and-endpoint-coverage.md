# Description: Guarantees and Endpoint Coverage
[INTENT: CONTEXT]

---

## 1. Scope Overview
[INTENT: CONTEXT]

This detail surface owns the architectural guarantees of the combined
adapter and core and the endpoint coverage classes the portable core
requires Help-first discovery for. It is a child of the modularized
`DESCRIPTION.md` root TOC.

---

## 2. Information Register
[INTENT: REFERENCE]

| ID | Type | Description | Change | Status |
|----|------|-------------|--------|--------|
| GUA-001 | INFORMATION | Architectural guarantees of the combined adapter and core | No | Active |
| COV-001 | INFORMATION | Current endpoint coverage classes of the portable core | No | Active |

---

## 3. Information Units
[INTENT: SPECIFICATION]

### 3.1 GUA-001: Architectural guarantees
[INTENT: SPECIFICATION]

**Type:** INFORMATION

**Description:**

The combined adapter and core guarantee:

```text
- the adapter activates on presence and funnels every git-affecting task
  through its gated state chain into the core;
- a skipped initialization is reported as a process violation and repeated,
  never silently continued;
- the portable workflow has no dependency on external knowledge bases, docs/ or business files;
- this source repository retains its source-based execution binding;
- the current CLI help remains the authority for command syntax;
- branch and commit conventions are obtained from the live policy and validators;
- regular ticket, hotfix, release, support and conflict paths are all explicit;
- shared lines are guarded by the core's mutation embargo before any edit;
- task-bound worktree isolation is actor-agnostic and lane-spanning: ticket,
  hotfix, and release lane work acquires its detached worktree through the
  governed `workflow worktree start` endpoint from the lane's own base, and
  the first mutation inside the worktree is the governed lane dispatch;
- the task-worktree requirement is programmatically enforced: every local
  working-branch lane dispatch fails closed outside a linked task worktree,
  while read-only, remote-dispatch, and birth endpoints and capability-less
  server compositions stay outside the requirement;
- the governed branch-hygiene endpoints delete completed local working
  branches only under proven hybrid evidence — merged pull request, deleted
  remote branch-cleanup obligation, no checkout, null-ahead against the
  recorded lane base — and refuse every loss-bearing class fail-closed;
  nothing runs in the background and the allocation inventory stays a pure
  read surface;
- Scratch is selected through a decision matrix instead of created by default;
- current GOV-42 main-hotfix delivery endpoints and controller boundaries are represented;
- unavailable binary or protected-controller capability fails closed;
- no raw Git, static-token or provider-CLI workaround replaces a governed path;
- the adapter adds no core policy and no core-area symbols of its own beyond
  the bootstrap symbol `🔌` used until the core becomes the control plane.
```

---

### 3.2 COV-001: Current endpoint coverage
[INTENT: SPECIFICATION]

**Type:** INFORMATION

**Description:**

The portable core requires Help-first discovery for the current CLI's:

```text
branch, commit, policy, doctor, validation and authentication endpoints
ticket start and publication workflows
worktree acquisition, inventory and removal lifecycle workflows
governed branch hygiene: controlled removal of one completed local working
branch and evidence-based prune of completed branches
hotfix record, delivery, single-commit and manifest propagation workflows
release request, cut, stabilization, alignment, promotion, backmerge and support workflows
Scratch cleanup and controlled transfer paths
```

The prompt does not reproduce the current option list. The live binary
reports the current option, value and validation contract at the moment each
endpoint is needed.

---

## 4. Conventions and Constraints
[INTENT: CONSTRAINT]

- The guarantees bind the combined adapter and core; neither layer may weaken one of them.
- The endpoint coverage is class-level; concrete flags and value domains are always derived from the live help.

---

## 5. Path Index
[INTENT: REFERENCE]

| # | Path | Relevance | Unit IDs |
|---|------|-----------|----------|
| 1 | `../prompt.md` | Source-repository adapter | GUA-001 |
| 2 | `../core/prompt.md` | Complete portable workflow | GUA-001, COV-001 |

---

## 6. Execution Context for LLM Agents
[INTENT: CONTEXT]

Use this surface to verify that a planned action stays inside the guaranteed
behavior and the covered endpoint classes. An action outside these classes
requires the core's own decision matrices before any mutation.
