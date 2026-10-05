# Changelog: Git-Governance Source-Repository Workflow Adapter
[INTENT: REFERENCE]

---

## 1. Scope Metadata
[INTENT: CONTEXT]

| Field | Value |
|-------|-------|
| Scope Root | `__prompts__/version-control/git/workflow` |
| Versioning Standard | `Semantic Versioning 2.0.0` |
| Current Version | `1.6.0` |
| Semver Class | `minor` |
| Breaking Change | `no` for the complete workflow bundle |
| Commit Scope | `workflow-adapter` |
| Ticket Scope | `GOV-138` |
| Current HEAD Commit Hash | `a9d9ccc534bea1b33196aaec2c90d78f5a5172b7` (pre-finalization evidence; the new version entry is intentionally not committed yet) |

---

## 2. Version Ledger Summary
[INTENT: REFERENCE]

| Version Family | Path | Highest Version | Notes |
|----------------|------|-----------------|-------|
| `v1` | `changelog/v1.md` | `1.6.0` | Major-family TOC; concrete version leaves descend only through semver-owned child paths |

---

## 3. Changelog Index
[INTENT: REFERENCE]

| # | Path | Scope |
|---|------|-------|
| 1 | `changelog/v1.md` | Major version family TOC |
| 2 | `changelog/v1/v1-6-0.md` | Concrete version leaf for `1.6.0` |
| 3 | `changelog/v1/v1-5-0.md` | Concrete version leaf for `1.5.0` |
| 4 | `changelog/v1/v1-4-0.md` | Concrete version leaf for `1.4.0` |
| 5 | `changelog/v1/v1-3-0.md` | Concrete version leaf for `1.3.0` |
| 6 | `changelog/v1/v1-2-0.md` | Concrete version leaf for `1.2.0` |
| 7 | `changelog/v1/v1-1-0.md` | Concrete version leaf for `1.1.0` |
| 8 | `changelog/v1/v1-0-0.md` | Concrete version leaf for `1.0.0` |

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
| 1 | `CHANGELOG.md` | Root TOC (this file) |
| 2 | `changelog/v1.md` | Major-family TOC |
| 3 | `changelog/v1/v1-6-0.md` | Concrete version leaf |
| 4 | `changelog/v1/v1-5-0.md` | Concrete version leaf |
| 5 | `changelog/v1/v1-4-0.md` | Concrete version leaf |
| 6 | `changelog/v1/v1-3-0.md` | Concrete version leaf |
| 7 | `changelog/v1/v1-2-0.md` | Concrete version leaf |
| 8 | `changelog/v1/v1-1-0.md` | Concrete version leaf |
| 9 | `changelog/v1/v1-0-0.md` | Concrete version leaf |
| 10 | `prompt.md` | Source-repository adapter |
| 11 | `CONVENTIONS.md` | Adapter constraints |
| 12 | `DESCRIPTION.md` | Adapter and core architecture (root TOC) |
| 13 | `core/prompt.md` | Portable binary workflow |
| 14 | `core/CONVENTIONS.md` | Core conventions |
| 15 | `core/DESCRIPTION.md` | Core architecture (root TOC) |
| 16 | `core/CHANGELOG.md` | Core ledger (root TOC) |
