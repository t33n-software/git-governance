# Changelog: Git-Governance Source-Repository Workflow Adapter
[INTENT: REFERENCE]

---

## 1. Scope Metadata
[INTENT: CONTEXT]

| Field | Value |
|-------|-------|
| Scope Root | `__prompts__/version-control/git/workflow` |
| Versioning Standard | `Semantic Versioning 2.0.0` |
| Current Version | `1.2.0` |
| Semver Class | `minor` |
| Breaking Change | `no` for the complete workflow bundle |
| Commit Scope | `workflow-adapter` |
| Ticket Scope | `GOV-42` |
| Current HEAD Commit Hash | `76324d8799b687854c854b0aad700beb85e004e1` (pre-finalization evidence; the new version entry is intentionally not committed yet) |

---

## 2. Version Ledger
[INTENT: REFERENCE]

| Version | Date | Class | Breaking | Commit Type | HEAD Commit Hash | Summary | Commit Subject |
|---------|------|-------|----------|-------------|------------------|---------|----------------|
| `1.2.0` | `2026-08-16` | minor | no | `feat` | pending (finalization deferred) | Hardened the adapter into a fully gated DWCEA bootstrap: binding activation contract, fine-grained state machine, explicit proof ledger, pre-action embargo, invalidation rules, bootstrap symbol discipline and an extended prohibition and completion contract. | pending (finalization deferred) |
| `1.1.0` | `2026-08-08` | minor | no | `feat` | `562b705e8cbd15295010a0aa0ed64faa7118990e` | Split the portable binary workflow core from this repository's Go-source adapter; added complete adapter and core metadata plus Help-first drift controls. | `feat(GOV-42): separate portable workflow core` |
| `1.0.0` | `2026-08-07` | patch | no | `docs` | `9ee1a308355ccf8f14ae2467b15faba33947ca67` | Canonicalized the governed workflow prompt and retained the stable Cursor rule entrypoint. | `docs(GOV-35): centralize canonical rule prompts` |

---

## 3. Current Version Entry
[INTENT: SPECIFICATION]

### 3.1 Version `1.2.0`
[INTENT: SPECIFICATION]

**Classification**

| Field | Value |
|-------|-------|
| Semver Class | `minor` |
| Breaking Change | `no` |
| Rationale | The adapter keeps its entire prior business logic: relative core loading, Go source-entrypoint binding, core delegation, non-duplication and the fail-closed entrypoint check are preserved in substance. The version adds the missing state-management and enforcement layers around them. |

**Change Units**

| ID | Category | Breaking | Summary | Affected Files | Description Alignment |
|----|----------|----------|---------|----------------|----------------------|
| CHG-201 | `contract` | no | Added the binding activation contract: presence of the adapter in the agent context activates it immediately, every git-affecting task must funnel through the adapter state chain into the core, and a skipped initialization is a reportable process violation instead of an alternative path. | `prompt.md` | `DESCRIPTION.md` sections 4 and 5 updated |
| CHG-202 | `runtime` | no | Replaced the four-state sequence with the gated state machine `ADAPTER_ACTIVATED -> CORE_PATH_RESOLVED -> CORE_FULLY_LOADED -> CORE_CONTRACT_BOUND -> SOURCE_ENTRYPOINT_VERIFIED -> CORE_WORKFLOW_EXECUTING -> ADAPTER_COMPLETE` plus explicit runtime surfaces. | `prompt.md` | `DESCRIPTION.md` section 4 updated |
| CHG-203 | `runtime` | no | Added the minimum proof ledger (`adapter_presence_acknowledged` through `adapter_completion_reported`) with the rule that no transition may occur without its bound proof and no early proof substitutes for a later one. | `prompt.md` | `DESCRIPTION.md` section 5 updated |
| CHG-204 | `contract` | no | Added the pre-action embargo: before `CORE_WORKFLOW_EXECUTING`, only resolving the relative core path, reading the core completely, running the entrypoint help check and emitting bootstrap status lines are permitted. | `prompt.md` | `DESCRIPTION.md` section 5 updated |
| CHG-205 | `runtime` | no | Added invalidation and re-anchoring rules: a changed core file, a failed entrypoint after prior success, or a session/repository switch resets the affected state and forbids reuse of cached core content, help results, or entrypoint verifications. | `prompt.md` | `DESCRIPTION.md` section 4 updated |
| CHG-206 | `output` | no | Added the adapter-local bootstrap symbol discipline (`🔌` until `CORE_WORKFLOW_EXECUTING`, afterwards the core symbol registry governs) and the adapter gate audit record. | `prompt.md` | `DESCRIPTION.md` section 5 updated |
| CHG-207 | `contract` | no | Extended the prohibition list and the completion verification ledger, including the mandatory open report when an initialization had to be repeated after being skipped. | `prompt.md` | `DESCRIPTION.md` section 5 updated |
| CHG-208 | `runtime` | no | Added the Cursor rule frontmatter block (`description`, `alwaysApply: true`) to the adapter head, repairing the missing rule-injection metadata of the symlinked entrypoint `.cursor/rules/governed-task-to-pr-workflow.mdc`; the target of a rule symlink carries no activation metadata without this block and is never injected into the agent context. | `prompt.md` | `DESCRIPTION.md` section 7 updated |

**Migration / Consumer Impact**

No migration required. The stable Cursor symlink continues to resolve to `prompt.md`; the adapter still loads the co-located relative core and binds only the Go source entrypoint. Agents experience stricter sequencing and proof discipline, not a changed workflow contract.

**Commit Alignment**

| Field | Value |
|-------|-------|
| Commit Subject | pending (finalization deferred by user instruction; metadata finalized without commit) |
| Breaking Footer | none |
| Current HEAD Commit Hash | `76324d8799b687854c854b0aad700beb85e004e1` |

### 3.2 Version `1.1.0`
[INTENT: SPECIFICATION]

**Classification**

| Field | Value |
|-------|-------|
| Semver Class | `minor` |
| Breaking Change | `no` for consumers that retain the complete workflow directory |
| Compatibility | The stable Cursor symlink continues to resolve to `prompt.md`; the adapter loads the co-located relative core. |

**Change Units**

| ID | Category | Breaking | Summary | Affected Surfaces |
|----|----------|----------|---------|-------------------|
| CHG-101 | architecture | no | Added a self-contained portable binary-oriented workflow core. | `core/prompt.md` |
| CHG-102 | adapter | no | Converted `prompt.md` into a relative-core loader and Go source-entrypoint binder. | `prompt.md` |
| CHG-103 | runtime | no | Replaced duplicated CLI option knowledge with per-endpoint Help-first discovery. | adapter and core prompts |
| CHG-104 | governance | no | Added explicit state, proof, Scratch and current hotfix delivery capability boundaries to the portable core. | `core/prompt.md` |
| CHG-105 | docs | no | Added adapter/core conventions, descriptions and version ledgers. | workflow metadata pair and `core/` metadata |

**Consumer Impact**

The complete `workflow/` directory is now the distributable prompt bundle.
Consumers that previously copied only `prompt.md` must copy the co-located
`core/` directory as well, because the source adapter intentionally avoids a
second full workflow copy. The repository's Cursor entrypoint remains stable.

**Commit Alignment**

| Field | Value |
|-------|-------|
| Commit Subject | `feat(GOV-42): separate portable workflow core` |
| Breaking Footer | none |
| HEAD Commit Hash | `562b705e8cbd15295010a0aa0ed64faa7118990e` |

### 3.3 Version `1.0.0`
[INTENT: SPECIFICATION]

The initial version placed the complete workflow directly in `prompt.md` and
retained the Cursor entrypoint as a relative symbolic link. That stable
entrypoint remains intact across all later versions.

**Commit Alignment**

| Field | Value |
|-------|-------|
| Commit Subject | `docs(GOV-35): centralize canonical rule prompts` |
| Breaking Footer | none |
| HEAD Commit Hash | `9ee1a308355ccf8f14ae2467b15faba33947ca67` |

---

## 4. Semver Rules
[INTENT: REFERENCE]

```text
patch  = clarification or metadata correction without workflow behavior change
minor  = backward-compatible workflow capability or bundle-architecture addition
major  = incompatible removal of a required workflow, authority or safety contract
```

---

## 5. Path Index
[INTENT: REFERENCE]

| # | Path | Relevance |
|---|------|-----------|
| 1 | `prompt.md` | Source-repository adapter |
| 2 | `CONVENTIONS.md` | Adapter constraints |
| 3 | `DESCRIPTION.md` | Adapter and core architecture |
| 4 | `CHANGELOG.md` | This adapter ledger |
| 5 | `core/prompt.md` | Portable binary workflow |
| 6 | `core/CONVENTIONS.md` | Core conventions |
| 7 | `core/DESCRIPTION.md` | Core architecture |
| 8 | `core/CHANGELOG.md` | Core ledger |
