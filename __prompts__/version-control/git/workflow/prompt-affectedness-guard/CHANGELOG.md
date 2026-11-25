# Changelog: Workflow-Prompt Affectedness Guard
[INTENT: REFERENCE]

---

## 1. Scope Metadata
[INTENT: CONTEXT]

| Field | Value |
|-------|-------|
| Scope Root | `__prompts__/version-control/git/workflow/prompt-affectedness-guard` |
| Versioning Standard | `Semantic Versioning 2.0.0` |
| Current Version | `1.1.0` |
| Semver Class | `minor` |
| Breaking Change | `no` |
| Commit Scope | `workflow-prompt-affectedness-guard` |
| Ticket Scope | `GOV-156` |
| Current HEAD Commit Hash | pending (finalization with the GOV-156 commit) |

---

## 2. Version Ledger
[INTENT: REFERENCE]

| Version | Date | Class | Breaking | Commit Type | HEAD Commit Hash | Summary | Commit Subject |
|---------|------|-------|----------|-------------|------------------|---------|----------------|
| `1.0.0` | `2026-09-29` | minor | no | `feat` | pending (finalization with the GOV-116 commit) | Initial creation: the weighted affectedness evaluation for CLI-surface changes against the governed workflow prompts, with the CONVENTIONS.md read mandate, the separate adapter-binding check, and the interactive process routing that forbids autonomous prompt edits. | `feat(GOV-116): add the governed repository bootstrap workflow` |
| `1.1.0` | `2026-11-25` | minor | no | `docs` | pending (finalization with the GOV-156 commit) | Bound the guard's frontmatter globs as the invalidation source of the core help-anchor contract epoch ([0.2.2]): an edit under the guarded CLI paths closes the epoch for every help, policy, or register unit anchored from the affected surface, and the next consuming invocation re-anchors before dispatch. | `feat(GOV-156): bind help anchoring to anchor epoch scenario classes` |

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
| Rationale | Backward-compatible addition: one new binding sentence in [2] names the existing glob set as the epoch invalidation source of the core anchor contract; the evaluation, matrix, bands, routing, and prohibitions are unchanged. |

**Change Units**

| ID | Category | Breaking | Summary | Affected Files | Description Alignment |
|----|----------|----------|---------|----------------|----------------------|
| CHG-004 | `contract` | no | Added the invalidation-source clause coupling the guard globs to the core [0.2.2] contract epoch closure and re-anchor duty. | `prompt.md` ([2]) | `DESCRIPTION.md` sections 4 and 5 |

**Migration / Consumer Impact**

No migration required. Consumers of the guard gain the epoch-closure consequence automatically: any CLI-surface edit under the globs invalidates scoped help anchors exactly through the already-listed trigger classes of core [0.2.2].

**Commit Alignment**

| Field | Value |
|-------|-------|
| Commit Subject | `feat(GOV-156): bind help anchoring to anchor epoch scenario classes` |
| Breaking Footer | none |
| Current HEAD Commit Hash | pending (finalization with the GOV-156 commit) |

---

### 3.2 Version `1.0.0`
[INTENT: SPECIFICATION]

**Classification**

| Field | Value |
|-------|-------|
| Semver Class | `minor` |
| Breaking Change | `no` |
| Rationale | Initial creation of the guard as its own artifact beside the workflow adapter, following the folder-per-artifact convention with its own metadata pair. |

**Change Units**

| ID | Category | Breaking | Summary | Affected Files | Description Alignment |
|----|----------|----------|---------|----------------|----------------------|
| CHG-001 | `governance` | no | Added the affectedness guard rule with the weighted multi-decision matrix (topology delta, register drift, governance-semantics delta, failure-contract delta, help-coverable negative axis) and the 80/55 band routing into the interactive user process. | `prompt.md` | `DESCRIPTION.md` sections 1, 4 and 5 |
| CHG-002 | `contract` | no | Bound the mandatory workflow-area CONVENTIONS.md read before every evaluation and the separate adapter affectedness check scoped to source-entrypoint binding changes. | `prompt.md` | `DESCRIPTION.md` section 4 |
| CHG-003 | `runtime` | no | Added the Cursor rule frontmatter and the relative symlink entrypoint `.cursor/rules/workflow-prompt-affectedness-guard.mdc`. | `prompt.md`, `.cursor/rules/workflow-prompt-affectedness-guard.mdc` | `DESCRIPTION.md` section 6 |

**Migration / Consumer Impact**

No migration required. The guard is additive; the existing adapter symlink and
the portable core remain unchanged.

**Commit Alignment**

| Field | Value |
|-------|-------|
| Commit Subject | `feat(GOV-116): add the governed repository bootstrap workflow` |
| Breaking Footer | none |
| Current HEAD Commit Hash | pending (finalization with the GOV-116 commit) |

---

## 4. Semver Rules
[INTENT: REFERENCE]

```text
patch  = clarification or metadata correction without guard behavior change
minor  = backward-compatible guard capability or matrix-surface addition
major  = incompatible removal of a required guard, gate or routing contract
```

---

## 5. Path Index
[INTENT: REFERENCE]

| # | Path | Relevance |
|---|------|-----------|
| 1 | `prompt.md` | The affectedness guard rule |
| 2 | `DESCRIPTION.md` | Guard architecture |
| 3 | `CHANGELOG.md` | This guard ledger |
