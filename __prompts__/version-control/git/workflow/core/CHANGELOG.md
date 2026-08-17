# Changelog: Portable Git-Governance Agent Workflow Core
[INTENT: REFERENCE]

---

## 1. Scope Metadata
[INTENT: CONTEXT]

|| Field | Value |
||-------|-------|
|| Scope Root | `__prompts__/version-control/git/workflow/core` |
|| Versioning Standard | `Semantic Versioning 2.0.0` |
|| Current Version | `1.2.0` |
|| Semver Class | `minor` |
|| Breaking Change | `no` |
|| Commit Scope | `workflow-core` |
|| Ticket Scope | `GOV-78` |
|| Current HEAD Commit Hash | `ac1994626711670a2c36c85d3155ca21f9a1f28c` (pre-finalization evidence; the new version entry is intentionally not committed yet) |

---

## 2. Version Ledger Summary
[INTENT: REFERENCE]

|| Version Family | Path | Highest Version | Notes |
||----------------|------|-----------------|-------|
|| `v1` | `changelog/v1.md` | `1.2.0` | Major-family TOC; concrete version leaves descend only through semver-owned child paths |

---

## 3. Changelog Index
[INTENT: REFERENCE]

|| # | Path | Scope |
||---|------|-------|
|| 1 | `changelog/v1.md` | Major version family TOC |
|| 2 | `changelog/v1/v1-2-0.md` | Concrete version leaf for `1.2.0` |
|| 3 | `changelog/v1/v1-1-0.md` | Concrete version leaf for `1.1.0` |
|| 4 | `changelog/v1/v1-0-0.md` | Concrete version leaf for `1.0.0` |

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

|| # | Path | Relevance |
||---|------|-----------|
|| 1 | `CHANGELOG.md` | Root TOC |
|| 2 | `changelog/v1.md` | Major-family TOC |
|| 3 | `changelog/v1/v1-2-0.md` | Concrete version leaf |
|| 4 | `changelog/v1/v1-1-0.md` | Concrete version leaf |
|| 5 | `changelog/v1/v1-0-0.md` | Concrete version leaf |
|| 6 | `prompt.md` | Portable core workflow |
|| 7 | `DESCRIPTION.md` | Root description TOC |
|| 8 | `CONVENTIONS.md` | Authoring and runtime constraints |
