# Description: Git-Governance Source-Repository Workflow Adapter
[INTENT: CONTEXT]

---

## 1. Scope Overview
[INTENT: CONTEXT]

This directory provides the repository-local entrypoint for a complete
`git-governance` agent workflow. It separates the portable workflow core
from this repository's Go-source adapter and keeps the metadata surfaces as
modular navigation hubs.

This surface is the modularized description root. It behaves as the
navigation hub for the metadata-owned detail package `description/`; every
semantic unit family lives in exactly one child file.

Active package topology:

```text
workflow/
├── prompt.md
│   -> thin adapter for this Go source repository
├── CONVENTIONS.md
├── DESCRIPTION.md
│   -> root TOC (this file)
├── description/
│   ├── purpose-and-separation.md
│   ├── adapter-activation.md
│   ├── guarantees-and-endpoint-coverage.md
│   └── entrypoint-and-portability.md
├── CHANGELOG.md
│   -> root TOC of the version ledger
├── changelog/
│   ├── v1.md
│   └── v1/ (concrete version leaves)
├── core/
│   ├── prompt.md
│   │   -> complete portable binary-oriented workflow
│   ├── CONVENTIONS.md
│   ├── DESCRIPTION.md
│   └── CHANGELOG.md
└── prompt-affectedness-guard/
    ├── prompt.md
    │   -> CLI-surface drift guard for the adapter and the core
    ├── DESCRIPTION.md
    └── CHANGELOG.md
```

The adapter is the stable target of the Cursor rule symlink. It fully loads
the relative core and maps the core's logical binary invocation to this
repository's source entrypoint.

---

## 2. Information Register Summary
[INTENT: REFERENCE]

| ID Family | Meaning | Detail Surface |
|-----------|---------|----------------|
| PUR-* | Purpose and package topology of the workflow entrypoint | `description/purpose-and-separation.md` |
| SEP-* | Why the core/adapter separation exists | `description/purpose-and-separation.md` |
| DEC-* | Architectural decision matrix of the workflow design | `description/purpose-and-separation.md` |
| ACT-* | Adapter activation and runtime mechanics | `description/adapter-activation.md` |
| GUA-* | Architectural guarantees of the combined adapter and core | `description/guarantees-and-endpoint-coverage.md` |
| COV-* | Current endpoint coverage classes | `description/guarantees-and-endpoint-coverage.md` |
| PRT-* | Cursor entrypoint, portability and file index | `description/entrypoint-and-portability.md` |

---

## 3. Description Index
[INTENT: REFERENCE]

| # | Path | Scope | Reason |
|---|------|-------|--------|
| 1 | `description/purpose-and-separation.md` | Purpose and separation | What the entrypoint provides, why the core/adapter separation exists, and which architecture the decision matrix selected |
| 2 | `description/adapter-activation.md` | Adapter activation | The binding activation contract, the gated state chain, the pre-action embargo, the invocation binding and the invalidation rules |
| 3 | `description/guarantees-and-endpoint-coverage.md` | Guarantees and coverage | The combined adapter/core guarantees and the endpoint coverage classes of the portable core |
| 4 | `description/entrypoint-and-portability.md` | Entrypoint and portability | The stable Cursor symlink, the frontmatter injection contract and the file index |

---

## 4. Conventions and Constraints
[INTENT: CONSTRAINT]

- The adapter never duplicates core policy; the core never freezes binary flags, value formats, or technical limits.
- Metadata describes only implemented prompt behavior and never portrays an unimplemented binary endpoint, controller, or infrastructure component as available.
- Every material adapter or core contract change is recorded in `CHANGELOG.md` or `core/CHANGELOG.md` with a semantic version, a compatibility assessment, and the affected surfaces.
- The root metadata surfaces are navigation hubs; semantic detail lives in exactly one child file of `description/` or `changelog/`.

---

## 5. Path Index
[INTENT: REFERENCE]

| # | Path | Relevance |
|---|------|-----------|
| 1 | `DESCRIPTION.md` | Root TOC (this file) |
| 2 | `description/purpose-and-separation.md` | Detail surface |
| 3 | `description/adapter-activation.md` | Detail surface |
| 4 | `description/guarantees-and-endpoint-coverage.md` | Detail surface |
| 5 | `description/entrypoint-and-portability.md` | Detail surface |
| 6 | `CHANGELOG.md` | Version ledger root TOC |
| 7 | `prompt.md` | Source-repository adapter |
| 8 | `core/prompt.md` | Complete portable workflow |
| 9 | `core/DESCRIPTION.md` | Core description root TOC |
| 10 | `core/CHANGELOG.md` | Core version ledger root TOC |

---

## 6. Execution Context for LLM Agents
[INTENT: CONTEXT]

Treat `prompt.md` as a loader and source-execution adapter with a binding
activation contract. Read `core/prompt.md` completely before acting. Use
this root as the map into the `description/` detail surfaces and read the
detail surface that owns the semantic family being changed. Do not use the
adapter's small size as permission to omit core workflow gates. Do not
consult external documentation to reconstruct CLI syntax: use the current
binary's `--help`. If the adapter was present but its initialization was
skipped, stop, report the process violation, and run the full state chain
before any further mutation.
