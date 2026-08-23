# Description: Core Invariants
[INTENT: CONTEXT]

---

## 1. Scope Overview
[INTENT: CONTEXT]

This detail surface owns the behavioral invariants of the portable core:
help-first invocation, the deterministic workflow state machine, the
shared-line guard and mutation embargo, task-pattern recognition with the
execution-level hierarchy, the area-symbol registry, safe Scratch usage,
proactive key and ticket discovery, the scoped one-time provider-session
verification, the current hotfix capability boundary, and the semantic
commit-message and pull-request-description content governance.

---

## 2. Information Register
[INTENT: REFERENCE]

|| ID | Type | Description | Change | Status |
||----|------|-------------|--------|--------|
|| INV-001 | CONSTRAINT | Help-first invocation | No | Active |
|| INV-002 | CONSTRAINT | Deterministic workflow state | No | Active |
|| INV-003 | CONSTRAINT | Shared-line guard and mutation embargo | No | Active |
|| INV-004 | CONSTRAINT | Task-pattern recognition and execution-level hierarchy | Yes | Active |
|| INV-005 | CONSTRAINT | Area-symbol registry | No | Active |
|| INV-006 | CONSTRAINT | Safe Scratch usage | No | Active |
|| INV-007 | WORKFLOW | Proactive key and ticket discovery | No | Active |
|| INV-008 | WORKFLOW | Scoped one-time provider-session verification | No | Active |
|| INV-009 | CONSTRAINT | Current hotfix capability boundary | No | Active |
| INV-010 | CONSTRAINT | Commit-message and pull-request-description content governance | Yes | Active |

---

## 3. Information Units
[INTENT: SPECIFICATION]

### 3.1 INV-001: Help-first invocation
[INTENT: SPECIFICATION]

**Type:** CONSTRAINT

**Description:**

Every endpoint is discovered immediately before use:

```text
git-governance <endpoint> --help
-> inspect the actual contract
-> execute one matching invocation
-> wait for the real result
```

The core names the necessary endpoint and the reason for calling it. It does
not freeze endpoint flags, value formats, regexes, technical limits or runtime
modes in prompt text.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Help-first runtime contract | Section [0.2] |

---

### 3.2 INV-002: Deterministic workflow state
[INTENT: SPECIFICATION]

**Type:** CONSTRAINT

**Description:**

The core uses explicit states and proof gates rather than treating an
uninspected branch, a successful shell command, or an intended PR as proof of
completion. The state machine is:

```text
BRANCH_CONTEXT_CHECK
-> SHARED_LINE_GUARD
-> ENVIRONMENT_READY
-> INTAKE_READY
-> EXECUTION_LEVEL_BOUND
-> BRANCH_READY
-> EXECUTING
-> VERIFIED
-> COMMIT_READY
-> PUBLICATION_READY
-> PR_CREATED
-> COMPLETE
```

It distinguishes:

```text
regular ticket work
hotfixes on active affected lines
release and support lifecycle work
release reconciliation
delivery waiting states
conflict recovery
```

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | State model, proofs, transitions | Section [2] |

---

### 3.3 INV-003: Shared-line guard and mutation embargo
[INTENT: SPECIFICATION]

**Type:** CONSTRAINT

**Description:**

Immediately after branch detection, the core classifies the checked-out branch
as `shared_line`, `official_working`, `scratch`, `detached`, or `unknown`.
On `main`, `develop`, `release/*`, and `support/*`, a binding mutation embargo
activates before any other work:

```text
- no file creation, edit, rename, or deletion;
- no staging of any kind;
- no `commit create` or any other commit creation;
- no `branch create` or self-directed branch switching;
- no raw Git mutation.
```

The embargo lifts only after a governed level-1 workflow has created or
confirmed the official working branch and the branch context has been
re-verified. Pre-existing uncommitted damage found on a shared line escalates
to a user decision instead of an autonomous repair through `branch create`
carry-over.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Branch classes and embargo | Sections [3.1] and [3.2] |

---

### 3.4 INV-004: Task-pattern recognition and execution-level hierarchy
[INTENT: SPECIFICATION]

**Type:** CONSTRAINT

**Description:**

Before any mutation planning, the core classifies the request into exactly one
task pattern (`ticket`, `hotfix`, `release`, `support`, `exploration`,
`diagnostic`) and binds it with the branch class through the multi-decision
matrix to the mandatory entry path. Every Git effect runs through exactly one
execution level:

```text
Level 1: governed workflows (workflow ticket|hotfix|release|cleanup)
         mandatory whenever the task pattern is covered
Level 2: bounded CLI commands and subcommands (branch, commit, validate, ...)
         only for a bounded action no current workflow covers
Level 3: raw Git
         only for read-only orientation and effects neither level covers;
         never a replacement for a governed capability and never a mutation
         on a shared line
```

A workflow is never rebuilt from manually chained level-2 commands. The only
sanctioned raw-Git mutation is the explicit staging of resolved conflict paths
inside the conflict protocol, immediately followed by the governed resume
endpoint: for a rebase or merge paused by `branch sync-base`, the resume mode
of the same endpoint; for an operation paused by a workflow, the resume entry
of that owning workflow.

**Current State:**

The conflict protocol named the governed resume endpoint only generically as
whatever the current help offers, without binding the concrete entry point for
a synchronization paused by the standalone `branch sync-base` command.

**Target State:**

The conflict protocol binds the governed resume step to the concrete governed
entry points: the resume mode of `branch sync-base` for rebase or merge
operations paused by that endpoint, and the owning workflow's resume entry for
workflow-paused operations. The endpoint registry row for `branch sync-base`
binds the same capability. The prompt still names no flag; the current help
exposes the actual resume invocation form.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Execution levels and conflict protocol | Sections [4.2] and [7] |

**Positive Example(s):**

```text
resolve the exact conflicted paths
-> stage only the resolved paths
-> continue through the governed resume entry the current help exposes
   for the endpoint that paused the operation
-> re-verify base, provenance, and quality
```

**Negative Example(s):**

```text
resolve conflicts and run a raw rebase or merge continuation
```

A raw continuation bypasses the governed resume endpoint and skips branch
re-validation, publication guards, and the quality rerun.

---

### 3.5 INV-005: Area-symbol registry
[INTENT: SPECIFICATION]

**Type:** CONSTRAINT

**Description:**

All visible status messages and audit records carry a centralized
area-specific UTF-8 symbol defined in the core's symbol registry. Thirteen
architectural areas (context and guard, environment and policy, intake and
decision binding, branch provisioning, implementation, verification, commit,
publication and pull request, release/support lifecycle, hotfix lifecycle,
conflict recovery, waiting and blocked, completion and cleanup) each own one
distinct symbol. Symbols are derived from the architectural area of the
current step, never invented ad hoc, and never replace textual gate results.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Symbol registry and audit records | Sections [0.3] and [9] |

---

### 3.6 INV-006: Safe Scratch usage
[INTENT: SPECIFICATION]

**Type:** CONSTRAINT

**Description:**

Scratch is optional private exploration. The core uses a weighted decision
matrix:

```text
0–39   direct official work
40–59  read-only clarification, then reassess
60–100 scratch before speculative implementation
```

It prevents the former anti-pattern of creating a Scratch branch merely
because a workflow starts or a task is non-trivial. Scratch is created only
through the governed ticket workflow path, never directly from a shared line
through `branch create` or raw Git.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Scratch decision matrix | Section [4.8] |

---

### 3.7 INV-007: Proactive key and ticket discovery
[INTENT: SPECIFICATION]

**Type:** WORKFLOW

**Description:**

When the active task requires a ticket and the user supplied neither key nor
ticket, the core runs an evidence-based discovery before the ticket stop
sequence. The discovery binds exactly one project — the explicitly passed
project (for example via `--repo`) or the current working directory — and
selects exactly one capability level in priority order:

```text
P1  gh integration (available, authenticated, can read the project's PRs)
P2  available context tools that actually cover listing open and closed PRs
P3  anonymous GitHub API, only for proven-public repositories
P4  discovery unavailable (no capable tool, or non-public repository
    without authenticated access) -> brief diagnostic status log, then the
    unchanged stop sequence
```

Open and closed pull requests are analyzed for used and still-open keys and
ticket numbers. A multi-decision matrix over task pattern, key distribution,
highest used ticket number, open tickets, and execution level produces a
proposal for the level-1 workflow or level-2 commands. The proposal binds
nothing: only an explicit user confirmation or user-supplied replacement
values set `ticket_binding` to `user_provided` or `confirmed_proposal`. If
discovery fails or the proposal is declined without replacement values, the
`WAITING_FOR_TICKET` stop sequence applies unchanged.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Discovery chain and proposal matrix | Section [4.5] |

---

### 3.8 INV-008: Scoped one-time provider-session verification
[INTENT: SPECIFICATION]

**Type:** WORKFLOW

**Description:**

When the classified task pattern implies provider publication (pull-request
creation in ticket work, hotfix or release provider steps), the core verifies
the provider session exactly once, immediately after pattern binding:

```text
task pattern bound and publication in scope
-> one auth status check (Help-first)
-> provider_session_verified bound to this scope
-> never re-probed within the same scope
```

A failed prefetch blocks early with the re-login remediation, before branch
or implementation steps begin. A mid-flight provider failure — for example a
session revoked or expired after the prefetch — is the affected endpoint's
fail-closed runtime path and is reported as `BLOCKED` with the re-login
remediation; the prompt deliberately carries no iterative re-check logic for
it. Patterns without provider effects (`diagnostic`, local-only `exploration`)
mark the surface `not_required` and skip the check entirely.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Provider-session prefetch | Section [4.1] |

---

### 3.9 INV-009: Current hotfix capability boundary
[INTENT: SPECIFICATION]

**Type:** CONSTRAINT

**Description:**

The core recognizes the current bounded hotfix endpoints:

```text
start
validate-record
publish
verify-merge
verify-delivery
single-commit propagation
manifest-candidate preparation
```

It requires a reviewed release record, a semantic commit budget, an ordered
manifest, immutable patch delivery evidence and target-local propagation
outcomes. It does not claim that an unavailable protected controller exists.
If a required manifest-publishing capability is absent, the core blocks
instead of replacing it with raw Git or a privileged local token.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Hotfix endpoint family | Section [5.3] |

---

### 3.10 INV-010: Commit-message and pull-request-description content governance
[INTENT: SPECIFICATION]

**Type:** CONSTRAINT

**Description:**

The binary and the active policy own commit grammar, families, and technical
limits. The core owns content completeness: every commit message carries a
precise one-sentence subject plus a body that holds what the diff cannot
express — intent, behavioral scope, touched invariants or contracts,
verification evidence, and risks. The body is the relevance filter of the
commit history: a later agent must be able to judge a unit's relevance from
subject and body without loading the diff (filter before fetch).

The agent makes only generic decisions (the ticket's actual content); every
architectural decision is bound by the contract. The subject formulation
contract fixes language (English), imperative mood, behavior-over-file
naming, and forbidden filler forms. The canonical body layout fixes the
category order (Motivation, Behavioral Change, Contracts and Invariants,
Verification, Risks and Follow-ups) with a short-form rule for narrow
scopes. An executable acceptance gate makes `commit_content_verified`
falsifiable: five canonical questions must be answerable from subject and
body alone, without the diff, or the omission must be matrix-justified.

The body is the default, not an option. A binding decision matrix scopes
every exception: hotfix lanes, release and support lanes, breaking markers,
and scratch squash transfers always require a body; behavior and structure
families require it except for provably trivial, self-explanatory units;
process families require it once behavior, contracts, or processes are
touched; only provably trivial `style` or `chore` units may omit it, because
the filler-text prohibition outranks the mandate. An omission must be
matrix-justified and is audited as `omitted-justified`; an unjustified
omission is never `PASS`.

Content anti-patterns are forbidden: diff narration, filler text, content
not evidenced by the actually staged paths (reality anchoring), and secrets.

The pull request is a separate abstraction layer above the commit series.
Its description carries the integration view that no single commit message
holds and never replicates commit content (single source of truth: detail
stays in the commits). The description is mandatory, never optional: every
pull request crosses a protected shared line, so the review gate needs its
information carriers deterministically; the mandate covers presence and
contract fidelity, not length. The canonical section order is fixed:
Summary, Scope and Non-Goals, Commit Series (navigation only), Risk and
Rollback, Verification and Review Focus.

Transport stays help-first: the composing agent derives the carrying
arguments from the immediately preceding `commit create` or publish-endpoint
help. If the current binary exposes no body transport for a commit or no
description transport for a pull request, the agent blocks with the named
gap instead of falling back to raw Git, an external PR CLI, raw provider
calls, or manual web edits.

**Current State:**

The core bound semantic unit boundaries and commit validation but left the
content completeness of commit messages and pull-request descriptions
unowned; the binary currently exposes commit subject, body, and footer
transport, while no publish endpoint exposes a pull-request description
transport.

**Target State:**

The core binds commit-message content through the body-mandate decision
matrix and the `commit_content_verified` proof, and pull-request-description
content through the integration-layer contract and the
`pr_description_verified` proof. A missing description transport on the
binary is reported as a named blocker, never bypassed.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Commit content architecture | Section [6.4] |
|| `core/prompt.md` | Pull-request description architecture | Section [8.1] |
|| `core/prompt.md` | Proof surfaces and audit records | Sections [2.3] and [9] |
|| `core/prompt.md` | Content prohibitions | Section [10] |

**Positive Example(s):**

```text
compose subject and body from the frozen acceptance ledger and the actually
staged paths
-> bind the body-mandate matrix decision, including any justified omission
-> set commit_content_verified
-> transport through the freshly read commit create help
```

**Negative Example(s):**

```text
narrate the diff line by line in the body, or omit the body on a hotfix
lane commit, or replicate commit bodies into the pull-request description
```

Diff narration duplicates what the diff carries more efficiently; an
unjustified omission removes the relevance filter; a replicated pull-request
body breaks the single source of truth.

---

## 4. Conventions and Constraints
[INTENT: CONSTRAINT]

- Every invariant is binding for every consuming agent and adapter.
- An invariant change is a material core-contract change and belongs in
  `core/CHANGELOG.md` with a semantic version, a compatibility assessment,
  and the affected surfaces.

---

## 5. Path Index
[INTENT: REFERENCE]

|| # | Path | Relevance | Unit IDs |
||---|------|-----------|----------|
|| 1 | `core/prompt.md` | Portable core workflow | INV-001 to INV-010 |

---

## 6. Execution Context for LLM Agents
[INTENT: CONTEXT]

Read this surface after the root `DESCRIPTION.md` index and before relying on
any single invariant. The invariants apply together; the conflict-recovery
binding in INV-004 is the authority for how a paused synchronization is
resumed.
