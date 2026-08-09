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
  -> discovers each required git-governance endpoint through --help
  -> uses the current binary for syntax and validation
  -> enforces workflow states, proof gates and audit records
  -> performs only a governed workflow or reports an exact blocker
```

The core deliberately separates three responsibilities:

| Responsibility | Owner | Reason |
|---|---|---|
| Workflow order, decision matrices, proof gates, security boundaries | `core/prompt.md` | Portable agent behavior |
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
completion. It distinguishes:

```text
regular ticket work
hotfixes on active affected lines
release and support lifecycle work
release reconciliation
delivery waiting states
conflict recovery
```

### 3.3 Safe Scratch usage

Scratch is optional private exploration. The core uses a weighted decision
matrix:

```text
0–39   direct official work
40–59  read-only clarification, then reassess
60–100 scratch before speculative implementation
```

It prevents the former anti-pattern of creating a Scratch branch merely
because a workflow starts or a task is non-trivial.

### 3.4 Current hotfix capability boundary

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
release cut, stabilization, alignment, promotion, backmerge and support
authentication, diagnostics, policy inspection and pre-push validation
```

The registry answers:

```text
- which endpoint is required;
- when it is allowed;
- which workflow transition it supports;
- which evidence must exist before and after it;
- when an unavailable capability must block instead of being bypassed.
```

The registry intentionally does not duplicate the binary's option list.

## 5. Security and Evidence
[INTENT: CONSTRAINT]

The core requires:

```text
- no direct mutation of protected shared lines;
- no raw-Git replacement where a governed CLI workflow exists;
- no implicit staging, amend, force push, reset or automatic stash;
- no static token, private-key, PEM or authorization-header exposure;
- no broad main-to-develop merge as hotfix propagation;
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
adapter only after the core is loaded. Do not consult external documentation
to fill a gap that the binary's current help or a required protected
controller must answer. If the executable surface cannot provide a required
capability, preserve all state and report the precise blocker.
