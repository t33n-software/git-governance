# Changelog: Portable Git-Governance Agent Workflow Core
[INTENT: REFERENCE]

---

## 1. Scope Metadata
[INTENT: CONTEXT]

| Field | Value |
|-------|-------|
| Scope Root | `__prompts__/version-control/git/workflow/core` |
| Versioning Standard | `Semantic Versioning 2.0.0` |
| Current Version | `1.1.0` |
| Semver Class | `minor` |
| Breaking Change | `no` |
| Commit Scope | `workflow-core` |
| Ticket Scope | `GOV-42` |
| Current HEAD Commit Hash | `76324d8799b687854c854b0aad700beb85e004e1` (pre-finalization evidence; the new version entry is intentionally not committed yet) |

---

## 2. Version Ledger
[INTENT: REFERENCE]

| Version | Date | Class | Breaking | Commit Type | HEAD Commit Hash | Summary | Commit Subject |
|---------|------|-------|----------|-------------|------------------|---------|----------------|
| `1.1.0` | `2026-08-16` | minor | no | `feat` | pending (finalization deferred) | Hardened the core with a shared-line guard and mutation embargo, task-pattern recognition, an execution-level hierarchy, an extended state and proof model, the missing release-request registry entry, a centralized area-symbol registry for status and audit output, proactive evidence-based key/ticket discovery ahead of the ticket stop sequence, and a scope-bound one-time provider-session prefetch that replaces iterative authentication checks. | pending (finalization deferred) |
| `1.0.0` | `2026-08-08` | minor | no | `feat` | `562b705e8cbd15295010a0aa0ed64faa7118990e` | Introduced the portable binary-oriented core workflow, Help-first endpoint contract, state model, evidence gates, Scratch matrix, current hotfix delivery boundaries and adapter separation. | `feat(GOV-42): separate portable workflow core` |

---

## 3. Current Version Entry
[INTENT: SPECIFICATION]

### 3.1 Version `1.1.0`
[INTENT: SPECIFICATION]

**Classification**

| Field | Value |
|-------|-------|
| Semver Class | `minor` |
| Breaking Change | `no` |
| Rationale | All changes are additive hardening layers. Every pre-existing workflow rule, state, proof, decision table, endpoint row and prohibition is preserved in substance; no consumer contract was removed or inverted. |

**Change Units**

| ID | Category | Breaking | Summary | Affected Files | Description Alignment |
|----|----------|----------|---------|----------------|----------------------|
| CORE-101 | `contract` | no | Added the shared-line classification (`main`, `develop`, `release/*`, `support/*`) with a binding mutation embargo that blocks file edits, staging, commits, branch creation and raw Git mutations before any governed workflow creates the official working branch. | `prompt.md` | `DESCRIPTION.md` section 3 updated |
| CORE-102 | `contract` | no | Added task-pattern recognition and the branch-context x task-pattern multi-decision matrix that forces ticket work on shared lines into `workflow ticket start`, hotfixes into `workflow hotfix start` and release operations into the `workflow release` family. | `prompt.md` | `DESCRIPTION.md` section 3 updated |
| CORE-103 | `contract` | no | Added the three-level execution hierarchy (workflows first, bounded commands second, raw Git last) with named-gap proof duties and the prohibition of rebuilding a workflow from lower-level commands, including the `branch create` anti-pattern ban for ticket work and repair carry-over. | `prompt.md` | `DESCRIPTION.md` sections 3 and 5 updated |
| CORE-104 | `runtime` | no | Extended the state machine with `SHARED_LINE_GUARD` and `EXECUTION_LEVEL_BOUND`, added runtime surfaces (`current_branch_class`, `mutation_embargo`, `mutation_release_channel`, `active_task_pattern`, `execution_level`) and the matching proof ledger entries. | `prompt.md` | `DESCRIPTION.md` section 3 updated |
| CORE-105 | `registry` | no | Added the `workflow release request` endpoint row to close registry drift against the current binary and bound every registry endpoint to its execution level (`E1`, `E2`, `RO`). | `prompt.md` | `DESCRIPTION.md` section 4 updated |
| CORE-106 | `output` | no | Replaced the single uniform status and audit symbol with the centralized area-symbol registry (thirteen architectural areas, distinct UTF-8 symbols, binding derivation rules including lane precedence for the hotfix and release families). | `prompt.md` | `DESCRIPTION.md` section 3 updated |
| CORE-107 | `contract` | no | Added proactive key and ticket discovery before the ticket stop sequence: project binding from an explicitly passed project (for example `--repo`) or the current working directory, plus a prioritized capability chain (P1 `gh` integration, P2 available context tools, P3 anonymous GitHub API only for proven-public repositories, P4 discovery unavailable with a brief diagnostic status log). | `prompt.md` | `DESCRIPTION.md` section 3 updated |
| CORE-108 | `contract` | no | Added the discovery multi-decision matrix over open and closed pull requests (used keys, highest used ticket number, open ticket numbers, task pattern, execution level) producing a key/ticket proposal for the level-1 workflow or level-2 commands, plus the mandatory interactive confirmation gate before any ticket binding; the `WAITING_FOR_TICKET` stop sequence remains unchanged as the fallback. | `prompt.md` | `DESCRIPTION.md` sections 3 and 5 updated |
| CORE-109 | `runtime` | no | Added the discovery runtime surface (`ticket_binding`), proof entries (`key_ticket_discovery_evaluated`, `key_ticket_proposal_presented`, `ticket_binding_confirmed`), the `🎯 Discovery` audit record, and prohibitions against unconfirmed ticket binding, fabricated PR evidence, anonymous access to non-public repositories, and skipping the stop sequence. | `prompt.md` | `DESCRIPTION.md` section 3 updated |
| CORE-110 | `contract` | no | Added the scope-bound one-time provider-session prefetch: when the classified task pattern implies provider publication, the agent verifies `auth status github` exactly once after pattern binding, blocks early with a re-login remediation on failure, and never re-probes within the same scope; mid-flight provider failures remain the endpoint's fail-closed path rather than prompt-level retry logic. Includes the `provider_session_state` surface, the `provider_session_verified` proof, the publication-checklist binding, the registry row update, and the symbol-registry realignment of the auth endpoints into the environment area. | `prompt.md` | `DESCRIPTION.md` sections 3, 5 and 8 updated |

**Migration / Consumer Impact**

No migration required. Adapters that resolve the same logical `git-governance` endpoints remain compatible; the hardened states and proofs extend agent behavior without changing the adapter binding surface.

**Commit Alignment**

| Field | Value |
|-------|-------|
| Commit Subject | pending (finalization deferred by user instruction; metadata finalized without commit) |
| Breaking Footer | none |
| Current HEAD Commit Hash | `76324d8799b687854c854b0aad700beb85e004e1` |

---

## 4. Semver Rules
[INTENT: REFERENCE]

```text
patch  = clarification or non-behavioral metadata correction
minor  = backward-compatible workflow capability or endpoint-map addition
major  = incompatible workflow-state, authority or safety-contract change
```

---

## 5. Path Index
[INTENT: REFERENCE]

| # | Path | Relevance |
|---|------|-----------|
| 1 | `prompt.md` | Portable core workflow |
| 2 | `DESCRIPTION.md` | Architecture reference |
| 3 | `CONVENTIONS.md` | Authoring and runtime constraints |
| 4 | `CHANGELOG.md` | This version ledger |
