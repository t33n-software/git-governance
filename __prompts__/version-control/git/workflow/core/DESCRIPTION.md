# Description: Portable Git-Governance Agent Workflow Core
[INTENT: CONTEXT]

## 1. Purpose
[INTENT: CONTEXT]

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

The only required runtime authority is the invoked `git-governance` binary and
its current `--help`, `policy describe`, diagnostic, validation, and workflow
surfaces.

## 2. Architecture
[INTENT: ARCHITECTURE]

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

The core deliberately separates three responsibilities:

| Responsibility | Owner | Reason |
|---|---|---|
| Workflow order, decision matrices, guard states, proof gates, security boundaries | `core/prompt.md` | Portable agent behavior |
| Current flags, values, errors, formats and available commands | Running `git-governance` binary | Prevents CLI-version drift |
| Repository-specific binary resolution | Optional adapter | Avoids coupling the portable core to a source checkout |

## 3. Core Invariants
[INTENT: SPECIFICATION]

### 3.1 Help-first invocation

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

### 3.2 Deterministic workflow state

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

### 3.3 Shared-line guard and mutation embargo

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

### 3.4 Task-pattern recognition and execution-level hierarchy

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
endpoint.

### 3.5 Area-symbol registry

All visible status messages and audit records carry a centralized
area-specific UTF-8 symbol defined in the core's symbol registry. Thirteen
architectural areas (context and guard, environment and policy, intake and
decision binding, branch provisioning, implementation, verification, commit,
publication and pull request, release/support lifecycle, hotfix lifecycle,
conflict recovery, waiting and blocked, completion and cleanup) each own one
distinct symbol. Symbols are derived from the architectural area of the
current step, never invented ad hoc, and never replace textual gate results.

### 3.6 Safe Scratch usage

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

### 3.7 Proactive key and ticket discovery

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

### 3.8 Current hotfix capability boundary

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

## 4. Endpoint Registry
[INTENT: REFERENCE]

The core contains an endpoint registry for:

```text
branch validation and synchronization
commit creation and validation
ticket start and publication
hotfix start, record validation, delivery verification and propagation
release request, cut, stabilization, alignment, promotion, backmerge and support
authentication, diagnostics, policy inspection and pre-push validation
```

The registry answers:

```text
- which endpoint is required;
- when it is allowed;
- which execution level it belongs to (workflow, bounded command, read-only);
- which workflow transition it supports;
- which evidence must exist before and after it;
- when an unavailable capability must block instead of being bypassed.
```

The registry intentionally does not duplicate the binary's option list.

## 5. Security and Evidence
[INTENT: CONSTRAINT]

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
- no completion claim without real PR, delivery, tag, artifact or
  propagation evidence where that evidence is required.
```

Local quality evidence is an optimization only. It cannot replace structural
pre-push checks, remote CI, required checks, review, Rulesets or protected
merge controls.

## 6. Relationship to the Source Adapter
[INTENT: CONTEXT]

The parent workflow directory contains an optional source-repository adapter:

```text
../prompt.md
```

That adapter fully reads this core through the relative path `core/prompt.md`
and maps the logical binary invocation to the repository's Go source
entrypoint. It contains no duplicate workflow policy.

Other projects use this core directly with an installed `git-governance`
binary or supply their own narrow adapter. The core remains the single
workflow authority in both cases.

## 7. File Index
[INTENT: REFERENCE]

| Path | Role |
|---|---|
| `core/prompt.md` | Complete portable agent workflow |
| `core/CONVENTIONS.md` | Core authoring, drift and runtime conventions |
| `core/DESCRIPTION.md` | This architecture and execution reference |
| `core/CHANGELOG.md` | Versioned core contract history |
| `../prompt.md` | Optional Go-source adapter for this repository |
| `../CONVENTIONS.md` | Adapter-specific source-binding conventions |

## 8. Execution Context for Agents
[INTENT: CONTEXT]

Read `core/prompt.md` completely before making a workflow decision. Use the
adapter only after the core is loaded. Classify the branch and bind the
shared-line guard before any mutation, and classify the task pattern before
any endpoint selection. When the user supplies neither key nor ticket, run
the proactive discovery and its confirmation gate before entering the ticket
stop sequence. Do not consult external documentation to fill a gap that the
binary's current help or a required protected controller must answer. If the
executable surface cannot provide a required capability, preserve all state
and report the precise blocker.
