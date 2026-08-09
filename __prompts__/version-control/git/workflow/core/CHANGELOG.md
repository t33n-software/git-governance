# Changelog: Portable Git-Governance Agent Workflow Core
[INTENT: REFERENCE]

## 1. Scope Metadata

| Field | Value |
|---|---|
| Scope Root | `__prompts__/version-control/git/workflow/core` |
| Versioning Standard | Semantic Versioning 2.0.0 |
| Current Version | `1.0.0` |
| Semver Class | minor |
| Breaking Change | no |
| Ticket Scope | `GOV-42` |

## 2. Version Ledger

| Version | Date | Class | Breaking | Commit Type | Summary |
|---|---|---|---|---|---|
| `1.0.0` | `2026-08-08` | minor | no | `feat` | Introduced the portable binary-oriented core workflow, Help-first endpoint contract, state model, evidence gates, Scratch matrix, current hotfix delivery boundaries and adapter separation. |

## 3. Current Version Entry

### 3.1 Version `1.0.0`

**Architecture**

```text
portable core
-> current git-governance binary and --help
-> optional repository adapter
```

**Added contract surfaces**

| ID | Category | Breaking | Summary |
|---|---|---|---|
| CORE-001 | architecture | no | Separated portable workflow policy from repository-specific binary invocation. |
| CORE-002 | runtime | no | Required a fresh `--help` inspection before every actual endpoint invocation. |
| CORE-003 | governance | no | Added explicit state, proof, branch-continuation, intake and Scratch-decision surfaces. |
| CORE-004 | workflow | no | Defined endpoint-level topology for regular ticket, hotfix, release, support and cleanup paths. |
| CORE-005 | hotfix | no | Bound current hotfix record, manifest, delivery and propagation capability boundaries without inventing unavailable controllers. |
| CORE-006 | security | no | Centralized fail-closed, no-bypass, no-secret and evidence-completeness constraints. |

**Compatibility**

The core is backward-compatible with an adapter that resolves the same logical
`git-governance` endpoints. It deliberately does not preserve source-specific
Go invocation syntax; that responsibility belongs to the sibling adapter.

## 4. Versioning Rules

```text
patch  = clarification or non-behavioral metadata correction
minor  = backward-compatible workflow capability or endpoint-map addition
major  = incompatible workflow-state, authority or safety-contract change
```

## 5. File Index

| Path | Role |
|---|---|
| `prompt.md` | Portable core workflow |
| `DESCRIPTION.md` | Architecture reference |
| `CONVENTIONS.md` | Authoring and runtime constraints |
| `CHANGELOG.md` | This version ledger |
