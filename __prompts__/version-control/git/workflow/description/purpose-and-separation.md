# Description: Purpose and Separation
[INTENT: CONTEXT]

---

## 1. Scope Overview
[INTENT: CONTEXT]

This detail surface owns the purpose of the workflow entrypoint directory,
the reason the core/adapter separation exists, and the architectural
decision matrix that selected the current design. It is a child of the
modularized `DESCRIPTION.md` root TOC.

---

## 2. Information Register
[INTENT: REFERENCE]

| ID | Type | Description | Change | Status |
|----|------|-------------|--------|--------|
| PUR-001 | INFORMATION | Purpose and package topology of the workflow entrypoint | No | Active |
| SEP-001 | INFORMATION | Why the core/adapter separation exists | No | Active |
| DEC-001 | INFORMATION | Architectural decision matrix of the workflow design | No | Active |

---

## 3. Information Units
[INTENT: SPECIFICATION]

### 3.1 PUR-001: Purpose and package topology
[INTENT: SPECIFICATION]

**Type:** INFORMATION

**Description:**

The `workflow/` directory is the repository-local entrypoint for a complete
`git-governance` agent workflow. It deliberately separates:

```text
workflow/
├── prompt.md
│   -> thin adapter for this Go source repository
├── CONVENTIONS.md
├── DESCRIPTION.md
│   -> root TOC
├── description/
│   -> detail package of this description surface
├── CHANGELOG.md
│   -> root TOC of the version ledger
├── changelog/
│   -> semver-owned changelog detail package
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

### 3.2 SEP-001: Why the separation exists
[INTENT: SPECIFICATION]

**Type:** INFORMATION

**Description:**

The original workflow prompt contained two different concerns:

```text
portable git-governance workflow architecture
and
this repository's Go-source invocation
```

That coupling would force every downstream binary user to carry a `go run`
and `cmd/git-governance` assumption. It also makes a source checkout and an
installed release binary appear to be the same runtime environment.

The architecture resolves that conflict:

| Layer | Responsibility | Deliberately excludes |
|---|---|---|
| `core/prompt.md` | Complete agent workflow, state, proof gates, Help-first endpoint discovery, branch, Scratch, release and hotfix decisions | Go source layout, fixed CLI flags, project documentation |
| `prompt.md` | Activation contract, relative core loading and Go source-entrypoint binding | Generic workflow policy and CLI option duplication |
| Running CLI | Current flags, values, validators, errors and actual capabilities | Agent workflow architecture |

---

### 3.3 DEC-001: Architectural decision matrix
[INTENT: SPECIFICATION]

**Type:** INFORMATION

**Description:**

| Decision | Portability | Drift resistance | Workflow completeness | Isolation | Result |
|---|---:|---:|---:|---:|---|
| One source-repository prompt with Go commands | low | low | medium | low | Rejected |
| A core that copies current CLI flags and regexes | high | low | high | medium | Rejected |
| A binary-oriented core with per-endpoint Help-first discovery | high | high | high | high | Selected |
| A thin relative source adapter | high | high | high | high | Selected |
| Scratch for every non-trivial task | low | medium | low | low | Rejected |
| Scratch only after a weighted uncertainty threshold | high | high | high | high | Selected |

The selected design gives the current binary authority over evolving
technical details while retaining an explicit, complete agent workflow for
every branching, commit, release, hotfix and delivery transition.

---

## 4. Conventions and Constraints
[INTENT: CONSTRAINT]

- The separation is architectural, not cosmetic: the core excludes Go source layout, fixed CLI flags, and project documentation; the adapter excludes generic workflow policy and CLI option duplication.
- The running CLI stays the authority for current flags, values, validators, errors, and actual capabilities.

---

## 5. Path Index
[INTENT: REFERENCE]

| # | Path | Relevance | Unit IDs |
|---|------|-----------|----------|
| 1 | `prompt.md` | Source-repository adapter | SEP-001 |
| 2 | `core/prompt.md` | Complete portable workflow | SEP-001, DEC-001 |
| 3 | `../DESCRIPTION.md` | Root TOC | PUR-001 |

---

## 6. Execution Context for LLM Agents
[INTENT: CONTEXT]

Read this surface to understand what the entrypoint provides and why the
layers are separated. The activation and runtime mechanics live in
`adapter-activation.md`; read that surface before acting as the adapter.
