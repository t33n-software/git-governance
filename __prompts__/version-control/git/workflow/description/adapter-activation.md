# Description: Adapter Activation
[INTENT: CONTEXT]

---

## 1. Scope Overview
[INTENT: CONTEXT]

This detail surface owns the adapter's binding activation contract, the
gated state chain, the pre-action embargo, the invocation binding, and the
invalidation rules. It is a child of the modularized `DESCRIPTION.md` root
TOC.

---

## 2. Information Register
[INTENT: REFERENCE]

| ID | Type | Description | Change | Status |
|----|------|-------------|--------|--------|
| ACT-001 | INFORMATION | Activation contract, state chain and pre-action embargo | No | Active |
| ACT-002 | INFORMATION | Invocation binding and invalidation rules | No | Active |

---

## 3. Information Units
[INTENT: SPECIFICATION]

### 3.1 ACT-001: Activation contract, state chain and pre-action embargo
[INTENT: SPECIFICATION]

**Type:** INFORMATION

**Description:**

The adapter is an executable contract, not passive reference material. Its
presence in the agent's context (as an injected rule, an attached file, or
read content) activates it immediately and bindingly for the running
session.

1. The agent acknowledges the adapter activation as its first visible step.
2. It resolves `core/prompt.md` relative to the adapter file.
3. It reads the core completely before any workflow action and binds the
   content as `CORE_WORKFLOW_CONTRACT`.
4. It verifies the Go source entrypoint through the core's Help-first
   sequence.
5. Only then does it delegate to the core, which governs every further
   decision.

The exact gated state chain is:

```text
ADAPTER_ACTIVATED
-> CORE_PATH_RESOLVED
-> CORE_FULLY_LOADED
-> CORE_CONTRACT_BOUND
-> SOURCE_ENTRYPOINT_VERIFIED
-> CORE_WORKFLOW_EXECUTING
-> ADAPTER_COMPLETE
```

Each transition requires its bound proof surface. Before
`CORE_WORKFLOW_EXECUTING`, a pre-action embargo permits only the bootstrap
operations themselves: resolving the relative core path, reading the core
completely, verifying the source entrypoint, and emitting bootstrap status
lines. Task analysis, file search beyond the core path, edits, staging,
commits, branch operations, and raw Git are all embargoed until the core is
the active control plane.

---

### 3.2 ACT-002: Invocation binding and invalidation rules
[INTENT: SPECIFICATION]

**Type:** INFORMATION

**Description:**

Whenever the core specifies:

```text
git-governance <endpoint> ...
```

the adapter runs:

```text
go run -mod=readonly ./cmd/git-governance <endpoint> ...
```

The adapter never adds hardcoded flags, values or argument shapes. Each
invocation derives those details from the immediately preceding current
`--help` output.

A changed core file, a failed entrypoint after prior success, or a session
or repository switch invalidates the bound state and forces the earliest
affected state to be rebuilt; cached core content, help results, or
entrypoint verifications are never reused after invalidation.

---

## 4. Conventions and Constraints
[INTENT: CONSTRAINT]

- The adapter activation is binding on presence; a skipped initialization is a reportable process violation, never an alternative path.
- The core becomes the active control plane only after `SOURCE_ENTRYPOINT_VERIFIED`; every further decision is core-governed.

---

## 5. Path Index
[INTENT: REFERENCE]

| # | Path | Relevance | Unit IDs |
|---|------|-----------|----------|
| 1 | `../prompt.md` | Source-repository adapter carrying this contract | ACT-001, ACT-002 |
| 2 | `../core/prompt.md` | Complete portable workflow loaded by the adapter | ACT-001 |

---

## 6. Execution Context for LLM Agents
[INTENT: CONTEXT]

Use this surface to verify the adapter sequencing: acknowledge activation,
load the core completely, verify the entrypoint through help, and only then
delegate. Never reuse cached core content, help results, or entrypoint
verifications after an invalidation event.
