# Description: Strict Whitebox Testing and Coverage Prompt
[INTENT: CONTEXT]

This directory owns the canonical prompt for strict whitebox testing and
complete Go statement coverage. The existing Cursor rule path remains a
relative symlink, while `prompt.md` is the single editable source.

---

## 1. Scope Overview
[INTENT: CONTEXT]

`prompt.md` contains the repository's testing and coverage requirements:
direct same-package whitebox tests for relevant production paths, exact
`100.0%` statement coverage for executable Go packages, and the required Go
test and coverage verification commands for substantive Go changes.

The Cursor rule
`.cursor/rules/whitebox-testing-and-coverage.mdc` resolves to this canonical
prompt through a relative symlink. The policy text is preserved without
semantic changes.

---

## 2. Information Register
[INTENT: REFERENCE]

| ID | Type | Description | Change | Status |
|----|------|-------------|--------|--------|
| REQ-001 | REQUIREMENT | Canonicalize the strict whitebox testing and coverage prompt at `prompt.md`. | Yes | Active |
| REQ-002 | REQUIREMENT | Preserve the existing Cursor rule location through a relative symlink. | Yes | Active |
| CONV-001 | CONSTRAINT | Keep the testing, coverage, and verification requirements unchanged. | No | Active |
| META-001 | INFORMATION | Maintain canonical description and changelog metadata in this prompt directory. | Yes | Active |

---

## 3. Information Units
[INTENT: SPECIFICATION]

### 3.1 REQ-001: Canonical testing and coverage source
[INTENT: SPECIFICATION]

**Type:** REQUIREMENT

**Description:**
`prompt.md` is the canonical source for strict whitebox testing and coverage
governance.

**Current State:**
Before this change, the policy existed directly as a Cursor rule under
`.cursor/rules`.

**Target State:**
The complete policy is preserved in `prompt.md`, including direct whitebox
testing, exact coverage, and required verification constraints.

**Affected Files:**

| Path | Relevance | Elements |
|------|-----------|----------|
| `__prompts__/testing/coverage/prompt.md` | Canonical prompt source | Whitebox and coverage policy |

**Positive Example(s):**

```text
__prompts__/testing/coverage/prompt.md
```

is the single editable source for the testing and coverage policy.

**Negative Example(s):**

```text
Independent policy edits in prompt.md and the Cursor rule
```

This would create conflicting coverage requirements.

---

### 3.2 REQ-002: Stable Cursor coverage rule
[INTENT: SPECIFICATION]

**Type:** REQUIREMENT

**Description:**
The existing Cursor coverage rule path remains available as a relative symlink
to the canonical prompt.

**Current State:**
The testing and coverage policy was consumed directly from
`.cursor/rules/whitebox-testing-and-coverage.mdc`.

**Target State:**
The same path resolves to `prompt.md` through the relative target
`../../__prompts__/testing/coverage/prompt.md`.

**Affected Files:**

| Path | Relevance | Elements |
|------|-----------|----------|
| `.cursor/rules/whitebox-testing-and-coverage.mdc` | Stable Cursor entrypoint | Relative symbolic link |
| `__prompts__/testing/coverage/prompt.md` | Canonical target | Whitebox and coverage policy |

**Positive Example(s):**

```text
.cursor/rules/whitebox-testing-and-coverage.mdc
  -> ../../__prompts__/testing/coverage/prompt.md
```

**Negative Example(s):**

```text
.cursor/rules/whitebox-testing-and-coverage.mdc
  -> C:\absolute\machine-specific\path\prompt.md
```

An absolute target would not remain portable across repository checkouts.

---

### 3.3 CONV-001: Policy preservation
[INTENT: CONSTRAINT]

The canonical prompt keeps the strict whitebox testing, coverage, and
verification requirements intact. Canonicalization changes only source
ownership and rule lookup.

---

### 3.4 META-001: Local metadata pair
[INTENT: INFORMATION]

`DESCRIPTION.md` records the policy boundary, and `CHANGELOG.md` tracks
versioned metadata changes for this prompt directory.

---

## 4. Conventions and Constraints
[INTENT: CONSTRAINT]

- `prompt.md` is the canonical editable source.
- The Cursor rule path is retained as a relative symbolic link.
- The symbolic-link target stored by Git uses forward slashes and is relative
  to `.cursor/rules`.
- The strict testing and coverage contract remains unchanged.
- `DESCRIPTION.md` and `CHANGELOG.md` use their exact uppercase filenames.

---

## 5. Path Index
[INTENT: REFERENCE]

| # | Path | Relevance | Unit IDs |
|---|------|-----------|----------|
| 1 | `__prompts__/testing/coverage/prompt.md` | Canonical testing and coverage source | REQ-001, REQ-002, CONV-001 |
| 2 | `__prompts__/testing/coverage/DESCRIPTION.md` | Scope reference for the canonical prompt | META-001 |
| 3 | `__prompts__/testing/coverage/CHANGELOG.md` | Version ledger for metadata changes | META-001 |
| 4 | `.cursor/rules/whitebox-testing-and-coverage.mdc` | Relative Cursor rule entrypoint | REQ-002 |

---

## 6. Execution Context for LLM Agents
[INTENT: CONTEXT]

Read `prompt.md` as the complete whitebox testing and coverage contract. The
`.cursor/rules` path is a compatibility entrypoint and must not become an
independent policy source. When this prompt changes, retain the relative link
target and update this metadata pair in the same scoped change.
