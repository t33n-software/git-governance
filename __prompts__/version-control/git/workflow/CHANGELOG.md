# Changelog: Git-Governance Source-Repository Workflow Adapter
[INTENT: REFERENCE]

## 1. Scope Metadata

| Field | Value |
|---|---|
| Scope Root | `__prompts__/version-control/git/workflow` |
| Versioning Standard | Semantic Versioning 2.0.0 |
| Current Version | `1.1.0` |
| Semver Class | minor |
| Breaking Change | no for the complete workflow bundle |
| Ticket Scope | `GOV-42` |

## 2. Version Ledger

| Version | Date | Class | Breaking | Commit Type | Summary |
|---|---|---|---|---|---|
| `1.1.0` | `2026-08-08` | minor | no | `feat` | Split the portable binary workflow core from this repository's Go-source adapter; added complete adapter and core metadata plus Help-first drift controls. |
| `1.0.0` | `2026-08-07` | patch | no | `docs` | Canonicalized the governed workflow prompt and retained the stable Cursor rule entrypoint. |

## 3. Current Version Entry

### 3.1 Version `1.1.0`

**Classification**

| Field | Value |
|---|---|
| Semver Class | `minor` |
| Breaking Change | `no` for consumers that retain the complete workflow directory |
| Compatibility | The stable Cursor symlink continues to resolve to `prompt.md`; the adapter loads the co-located relative core. |

**Change Units**

| ID | Category | Breaking | Summary | Affected Surfaces |
|---|---|---|---|---|
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

The final governed commit identifier is intentionally not fabricated before
the commit exists. Git history is the authoritative commit record after the
governed commit is created.

### 3.2 Version `1.0.0`

The initial version placed the complete workflow directly in `prompt.md` and
retained the Cursor entrypoint as a relative symbolic link. That stable
entrypoint remains intact in version `1.1.0`.

## 4. Versioning Rules

```text
patch  = clarification or metadata correction without workflow behavior change
minor  = backward-compatible workflow capability or bundle-architecture addition
major  = incompatible removal of a required workflow, authority or safety contract
```

## 5. File Index

| Path | Role |
|---|---|
| `prompt.md` | Source-repository adapter |
| `CONVENTIONS.md` | Adapter constraints |
| `DESCRIPTION.md` | Adapter and core architecture |
| `CHANGELOG.md` | This adapter ledger |
| `core/prompt.md` | Portable binary workflow |
| `core/CONVENTIONS.md` | Core conventions |
| `core/DESCRIPTION.md` | Core architecture |
| `core/CHANGELOG.md` | Core ledger |
| `.cursor/rules/governed-task-to-pr-workflow.mdc` | Stable relative symlink to `prompt.md` |
