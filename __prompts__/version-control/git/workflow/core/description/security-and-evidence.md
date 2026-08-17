# Description: Security and Evidence
[INTENT: CONTEXT]

---

## 1. Scope Overview
[INTENT: CONTEXT]

This detail surface owns the core's security and evidence boundaries: the
prohibitions that protect shared lines, credentials, and history, and the
evidence rules that decide when a workflow may claim completion.

---

## 2. Information Register
[INTENT: REFERENCE]

|| ID | Type | Description | Change | Status |
||----|------|-------------|--------|--------|
|| SEC-001 | CONSTRAINT | Security and evidence requirements | No | Active |

---

## 3. Information Units
[INTENT: SPECIFICATION]

### 3.1 SEC-001: Security and evidence requirements
[INTENT: SPECIFICATION]

**Type:** CONSTRAINT

**Description:**

The core requires:

```text
- no mutation of protected shared lines, including file edits, staging,
  commits, branch creation, or branch switching before a governed workflow
  has created the official working branch;
- no raw-Git replacement where a governed CLI workflow exists;
- no implicit staging, amend, force push, reset or automatic stash;
- no static token, private-key, PEM or authorization-header exposure;
- no broad main-to-develop merge as hotfix propagation;
- no key or ticket binding from pull-request, branch, or history evidence
  without explicit user confirmation;
- no fabricated discovery evidence and no anonymous API access to
  non-public repositories;
- no repeated provider-session probing within a bound task scope and no
  prompt-level retry loop for a provider runtime failure;
- no completion claim without real PR, delivery, tag, artifact or
  propagation evidence where that evidence is required.
```

Local quality evidence is an optimization only. It cannot replace structural
pre-push checks, remote CI, required checks, review, Rulesets or protected
merge controls.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Conflict, security, evidence, and publication boundaries | Sections [7] and [8] |

**Positive Example(s):**

```text
stage only resolved conflict paths
-> resume through the governed endpoint
-> rerun quality
-> publish through the governed workflow with real provider evidence
```

**Negative Example(s):**

```text
claim completion from a successful local command without pull-request,
delivery, tag, artifact, or propagation evidence
```

A local success signal is not the external evidence the core requires.

---

## 4. Conventions and Constraints
[INTENT: CONSTRAINT]

- Secrets, tokens, private keys, PEM files, refresh values, and authorization
  headers never appear in prompts, commits, workflow output, configuration,
  or chat.
- Missing evidence is never a pass.

---

## 5. Path Index
[INTENT: REFERENCE]

|| # | Path | Relevance | Unit IDs |
||---|------|-----------|----------|
|| 1 | `core/prompt.md` | Security and evidence boundaries | SEC-001 |

---

## 6. Execution Context for LLM Agents
[INTENT: CONTEXT]

Treat every prohibition here as fail-closed. When a required capability or
evidence item is unavailable, preserve all state and report the precise
blocker instead of improvising a replacement.
