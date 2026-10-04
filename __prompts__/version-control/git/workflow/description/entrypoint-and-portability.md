# Description: Entrypoint and Portability
[INTENT: CONTEXT]

---

## 1. Scope Overview
[INTENT: CONTEXT]

This detail surface owns the stable Cursor entrypoint, the portability
guarantees of the symlink target, the frontmatter injection contract, and
the file index of the workflow package. It is a child of the modularized
`DESCRIPTION.md` root TOC.

---

## 2. Information Register
[INTENT: REFERENCE]

| ID | Type | Description | Change | Status |
|----|------|-------------|--------|--------|
| PRT-001 | INFORMATION | Cursor entrypoint, symlink target and frontmatter injection contract | No | Active |
| PRT-002 | REFERENCE | File index of the workflow package | No | Active |

---

## 3. Information Units
[INTENT: SPECIFICATION]

### 3.1 PRT-001: Cursor entrypoint and portability
[INTENT: SPECIFICATION]

**Type:** INFORMATION

**Description:**

The stable Cursor entrypoint remains:

```text
.cursor/rules/governed-task-to-pr-workflow.mdc
```

It is a relative Git symlink to:

```text
../../__prompts__/version-control/git/workflow/prompt.md
```

That target remains portable across Windows, Linux and macOS checkouts. The
adapter then resolves the core using its own relative path, so neither layer
depends on a machine-specific absolute path.

The adapter file carries the Cursor rule frontmatter block (`description` and
`alwaysApply`) at its head. This block is the repository-local injection
contract of the rule entrypoint: without it, a symlinked rule target resolves
to content that carries no activation metadata and is never injected into the
agent context. The frontmatter belongs to the adapter layer because the
adapter is the repository-specific entrypoint; the portable core deliberately
carries no tool-specific injection metadata.

---

### 3.2 PRT-002: File index
[INTENT: SPECIFICATION]

**Type:** REFERENCE

**Description:**

| Path | Role |
|---|---|
| `prompt.md` | Go-source adapter |
| `CONVENTIONS.md` | Adapter-only constraints |
| `DESCRIPTION.md` | Adapter and core architecture (root TOC) |
| `description/purpose-and-separation.md` | Detail surface: purpose, separation and decision matrix |
| `description/adapter-activation.md` | Detail surface: activation contract and runtime mechanics |
| `description/guarantees-and-endpoint-coverage.md` | Detail surface: guarantees and endpoint coverage |
| `description/entrypoint-and-portability.md` | Detail surface: entrypoint, portability and file index |
| `CHANGELOG.md` | Adapter version ledger (root TOC) |
| `changelog/v1.md` | Version family TOC for `v1` |
| `changelog/v1/v1-<minor>-<patch>.md` | Concrete version leaves, one per released version |
| `core/prompt.md` | Complete portable workflow |
| `core/CONVENTIONS.md` | Portable core conventions |
| `core/DESCRIPTION.md` | Portable core architecture (root TOC) |
| `core/CHANGELOG.md` | Portable core history (root TOC) |
| `prompt-affectedness-guard/prompt.md` | CLI-surface drift guard for the adapter and the core |
| `.cursor/rules/governed-task-to-pr-workflow.mdc` | Stable relative Cursor symlink to this adapter |
| `.cursor/rules/workflow-prompt-affectedness-guard.mdc` | Stable relative Cursor symlink to the drift guard |

---

## 4. Conventions and Constraints
[INTENT: CONSTRAINT]

- The symlink target stays portable across operating systems; no layer depends on a machine-specific absolute path.
- The frontmatter block stays on the adapter layer; the portable core carries no tool-specific injection metadata.

---

## 5. Path Index
[INTENT: REFERENCE]

| # | Path | Relevance | Unit IDs |
|---|------|-----------|----------|
| 1 | `../prompt.md` | Symlink target of the stable Cursor entrypoint | PRT-001 |
| 2 | `../DESCRIPTION.md` | Root TOC | PRT-002 |

---

## 6. Execution Context for LLM Agents
[INTENT: CONTEXT]

Use this surface to locate any workflow package file and to verify the
entrypoint chain. The root TOC (`../DESCRIPTION.md`) maps the semantic unit
families; this surface maps the physical files.
