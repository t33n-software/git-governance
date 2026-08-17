# Description: Portable Git-Governance Agent Workflow Core
[INTENT: CONTEXT]

---

## 1. Scope Overview
[INTENT: CONTEXT]

`core/prompt.md` is the portable, binary-oriented agent workflow for
`git-governance`. It defines how an agent safely carries a ticket from
intake through a governed branch, semantic commits, verification, publication,
release, hotfix delivery, propagation, and final evidence.

This surface is the modularized description root. It behaves as the
navigation hub for the metadata-owned detail package `description/`; every
semantic unit family lives in exactly one child file.

Active package topology:

```text
core/DESCRIPTION.md                           root TOC (this file)
core/description/purpose-and-architecture.md  purpose and responsibility split
core/description/core-invariants.md           behavioral invariants
core/description/endpoint-registry.md         endpoint registry role
core/description/security-and-evidence.md     security and evidence boundaries
core/description/adapter-relationship.md      source-adapter relationship
```

---

## 2. Information Register Summary
[INTENT: REFERENCE]

|| ID Family | Meaning | Detail Surface |
||-----------|---------|----------------|
|| PUR-* | Purpose and top-level architecture of the portable core | `description/purpose-and-architecture.md` |
|| INV-* | Behavioral invariants of the workflow | `description/core-invariants.md` |
|| REG-* | Endpoint registry purpose, coverage, and boundary | `description/endpoint-registry.md` |
|| SEC-* | Security and evidence requirements | `description/security-and-evidence.md` |
|| ADP-* | Source-adapter relationship and portability | `description/adapter-relationship.md` |

---

## 3. Description Index
[INTENT: REFERENCE]

|| # | Path | Scope | Reason |
||---|------|-------|--------|
|| 1 | `description/purpose-and-architecture.md` | Purpose and architecture | What the core is and what it never requires |
|| 2 | `description/core-invariants.md` | Core invariants | Help-first, state machine, guards, levels, symbols, scratch, discovery, provider prefetch, hotfix boundary |
|| 3 | `description/endpoint-registry.md` | Endpoint registry | Which endpoints are governed, including the governed synchronization resume |
|| 4 | `description/security-and-evidence.md` | Security and evidence | Prohibitions and completion evidence |
|| 5 | `description/adapter-relationship.md` | Adapter relationship | Optional Go-source adapter binding |

---

## 4. Conventions and Constraints
[INTENT: CONSTRAINT]

- The core stays portable across repositories that use an installed
  `git-governance` binary; no repository documentation, project source, or
  external knowledge source is a runtime dependency.
- The running `git-governance` binary is the only runtime authority for
  flags, values, errors, formats, and available commands.
- The prompt owns workflow order, decision matrices, guard states, proof
  gates, and security boundaries; it never freezes endpoint flags, value
  formats, regexes, technical limits, or runtime modes.
- Metadata describes only implemented prompt behavior and never portrays an
  unimplemented binary endpoint, controller, or infrastructure component as
  available.
- Every material core-contract change is recorded in `core/CHANGELOG.md` with
  a semantic version, a compatibility assessment, and the affected surfaces.

---

## 5. Path Index
[INTENT: REFERENCE]

|| # | Path | Relevance |
||---|------|-----------|
|| 1 | `DESCRIPTION.md` | Root TOC |
|| 2 | `description/purpose-and-architecture.md` | Detail surface |
|| 3 | `description/core-invariants.md` | Detail surface |
|| 4 | `description/endpoint-registry.md` | Detail surface |
|| 5 | `description/security-and-evidence.md` | Detail surface |
|| 6 | `description/adapter-relationship.md` | Detail surface |
|| 7 | `core/prompt.md` | Complete portable agent workflow |
|| 8 | `core/CONVENTIONS.md` | Core authoring, drift and runtime conventions |
|| 9 | `core/CHANGELOG.md` | Versioned core contract history |
|| 10 | `../prompt.md` | Optional Go-source adapter for this repository |
|| 11 | `../CONVENTIONS.md` | Adapter-specific source-binding conventions |

---

## 6. Execution Context for LLM Agents
[INTENT: CONTEXT]

Read `core/prompt.md` completely before making a workflow decision. Use this
root as the map into the `description/` detail surfaces and read the detail
surface that owns the semantic family being changed. Use the adapter only
after the core is loaded. Classify the branch and bind the shared-line guard
before any mutation, and classify the task pattern before any endpoint
selection. When the user supplies neither key nor ticket, run the proactive
discovery and its confirmation gate before entering the ticket stop sequence.
When the bound pattern implies provider publication, verify the provider
session exactly once after pattern binding and never re-probe within the same
scope. Do not consult external documentation to fill a gap that the binary's
current help or a required protected controller must answer. If the executable
surface cannot provide a required capability, preserve all state and report
the precise blocker.
