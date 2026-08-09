# Changelog: Strict Whitebox Testing and Coverage Prompt
[INTENT: REFERENCE]

---

## 1. Scope Metadata
[INTENT: CONTEXT]

| Field | Value |
|-------|-------|
| Scope Root | `__prompts__/testing/coverage` |
| Versioning Standard | `Semantic Versioning 2.0.0` |
| Current Version | `1.0.0` |
| Semver Class | `patch` |
| Breaking Change | `no` |
| Commit Scope | `GOV-35` |
| Current HEAD Commit Hash | `b12a93a741e39ef53d3851113a16db93e2831dcf` |

---

## 2. Version Ledger
[INTENT: REFERENCE]

| Version | Date | Class | Breaking | Commit Type | HEAD Commit Hash | Summary | Commit Subject |
|---------|------|-------|----------|-------------|------------------|---------|----------------|
| `1.0.0` | `2026-08-07` | `patch` | `no` | `docs` | `b12a93a741e39ef53d3851113a16db93e2831dcf` | Canonicalized the strict whitebox testing and coverage prompt and its stable Cursor entrypoint. | `docs(GOV-35): centralize canonical rule prompts` |

---

## 3. Current Version Entry
[INTENT: SPECIFICATION]

### 3.1 Version `1.0.0`
[INTENT: SPECIFICATION]

**Classification**

| Field | Value |
|-------|-------|
| Semver Class | `patch` |
| Breaking Change | `no` |
| Rationale | The strict testing and coverage policy is preserved; only its canonical source location, relative Cursor entrypoint, and metadata pair are introduced. |

**Change Units**

| ID | Category | Breaking | Summary | Affected Files | Description Alignment |
|----|----------|----------|---------|----------------|----------------------|
| CHG-001 | policy | no | Added the complete canonical testing and coverage prompt and retained the Cursor rule through a relative symbolic link. | `prompt.md`, `.cursor/rules/whitebox-testing-and-coverage.mdc` | `REQ-001`, `REQ-002` |
| CHG-002 | docs | no | Added the canonical description and changelog metadata pair. | `DESCRIPTION.md`, `CHANGELOG.md` | `META-001` |

**Migration / Consumer Impact**

No migration required.

**Commit Alignment**

| Field | Value |
|-------|-------|
| Commit Subject | `docs(GOV-35): centralize canonical rule prompts` |
| Breaking Footer | `none` |
| Current HEAD Commit Hash | `b12a93a741e39ef53d3851113a16db93e2831dcf` |

---

## 4. Semver Rules
[INTENT: REFERENCE]

- Patch = non-breaking correction, clarification, or metadata synchronization.
- Minor = backward-compatible addition.
- Major = breaking contract or incompatible architectural change.

---

## 5. Path Index
[INTENT: REFERENCE]

| # | Path | Relevance |
|---|------|-----------|
| 1 | `prompt.md` | Canonical strict testing and coverage source |
| 2 | `DESCRIPTION.md` | Scope reference |
| 3 | `CHANGELOG.md` | Version ledger |
| 4 | `.cursor/rules/whitebox-testing-and-coverage.mdc` | Relative Cursor rule entrypoint |
