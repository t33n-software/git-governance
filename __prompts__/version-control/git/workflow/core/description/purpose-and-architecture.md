# Description: Core Purpose and Architecture
[INTENT: CONTEXT]

---

## 1. Scope Overview
[INTENT: CONTEXT]

This detail surface owns the purpose and the top-level architecture of the
portable Git-governance agent workflow core (`core/prompt.md`): what the core
is, what it deliberately does not require, and how it separates
responsibilities between prompt, running binary, and optional adapter.

---

## 2. Information Register
[INTENT: REFERENCE]

|| ID | Type | Description | Change | Status |
||----|------|-------------|--------|--------|
|| PUR-001 | INFORMATION | Portable self-contained core purpose | No | Active |
|| PUR-002 | CONSTRAINT | Responsibility separation between prompt, binary, and adapter | No | Active |
|| PUR-003 | CONSTRAINT | The running binary is the only runtime authority | No | Active |

---

## 3. Information Units
[INTENT: SPECIFICATION]

### 3.1 PUR-001: Portable self-contained core purpose
[INTENT: SPECIFICATION]

**Type:** INFORMATION

**Description:**

`core/prompt.md` is the portable, binary-oriented agent workflow for
`git-governance`. It defines how an agent safely carries a ticket from
intake through a governed branch, semantic commits, verification, publication,
release, hotfix delivery, propagation, and final evidence.

The core is self-contained. It does not require:

```text
- an AI-Base-Rules checkout;
- this repository's docs directory;
- project business files;
- a Go module or cmd directory;
- a prebuilt local policy copy;
- a particular hosting-provider CLI.
```

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Complete portable agent workflow | All sections |

---

### 3.2 PUR-002: Responsibility separation between prompt, binary, and adapter
[INTENT: SPECIFICATION]

**Type:** CONSTRAINT

**Description:**

The core deliberately separates three responsibilities:

|| Responsibility | Owner | Reason |
||---|---|---|
|| Workflow order, decision matrices, guard states, proof gates, security boundaries | `core/prompt.md` | Portable agent behavior |
|| Current flags, values, errors, formats and available commands | Running `git-governance` binary | Prevents CLI-version drift |
|| Repository-specific binary resolution | Optional adapter | Avoids coupling the portable core to a source checkout |

The runtime architecture is:

```text
Agent
  -> loads core/prompt.md completely
  -> classifies the checked-out branch and binds the shared-line guard
     before any mutation
  -> classifies the task pattern and binds the execution level
  -> discovers each required git-governance endpoint through --help
  -> uses the current binary for syntax and validation
  -> enforces workflow states, proof gates and audit records
  -> performs only a governed workflow or reports an exact blocker
```

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Portable workflow contract | Sections [0] to [10] |
|| `../prompt.md` | Optional source-repository adapter | Adapter binding |

---

### 3.3 PUR-003: The running binary is the only runtime authority
[INTENT: SPECIFICATION]

**Type:** CONSTRAINT

**Description:**

The only required runtime authority is the invoked `git-governance` binary and
its current `--help`, `policy describe`, diagnostic, validation, and workflow
surfaces. The core never freezes endpoint flags, value formats, regexes,
technical limits, or runtime modes in prompt text.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Help-first runtime contract | Section [0.2] |
|| `core/CONVENTIONS.md` | Authoring rules for this boundary | Sections 2 and 3 |

---

## 4. Conventions and Constraints
[INTENT: CONSTRAINT]

- The core stays portable across repositories that use an installed
  `git-governance` binary.
- No repository documentation, project source, or external knowledge source is
  a runtime dependency of the core.

---

## 5. Path Index
[INTENT: REFERENCE]

|| # | Path | Relevance | Unit IDs |
||---|------|-----------|----------|
|| 1 | `core/prompt.md` | Portable core workflow | PUR-001, PUR-002, PUR-003 |
|| 2 | `core/CONVENTIONS.md` | Authoring and runtime constraints | PUR-003 |
|| 3 | `../prompt.md` | Optional Go-source adapter | PUR-002 |

---

## 6. Execution Context for LLM Agents
[INTENT: CONTEXT]

Read this surface together with the root `DESCRIPTION.md` index. Treat the
responsibility matrix as the authority on what the prompt may own and what
must remain with the running binary.
