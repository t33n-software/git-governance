# Description: Git-Governance Source-Repository Workflow Adapter
[INTENT: CONTEXT]

## 1. Purpose

This directory provides the repository-local entrypoint for a complete
`git-governance` agent workflow.

It deliberately separates:

```text
workflow/
├── prompt.md
│   -> thin adapter for this Go source repository
├── CONVENTIONS.md
├── DESCRIPTION.md
├── CHANGELOG.md
└── core/
    ├── prompt.md
    │   -> complete portable binary-oriented workflow
    ├── CONVENTIONS.md
    ├── DESCRIPTION.md
    └── CHANGELOG.md
```

The adapter is the stable target of the Cursor rule symlink. It fully loads
the relative core and maps the core's logical binary invocation to this
repository's source entrypoint.

## 2. Why the Separation Exists

The original workflow prompt contained two different concerns:

```text
portable git-governance workflow architecture
and
this repository's Go-source invocation
```

That coupling would force every downstream binary user to carry a `go run`
and `cmd/git-governance` assumption. It also makes a source checkout and an
installed release binary appear to be the same runtime environment.

The new architecture resolves that conflict:

| Layer | Responsibility | Deliberately excludes |
|---|---|---|
| `core/prompt.md` | Complete agent workflow, state, proof gates, Help-first endpoint discovery, branch, Scratch, release and hotfix decisions | Go source layout, fixed CLI flags, project documentation |
| `prompt.md` | Relative core loading and Go source-entrypoint binding | Generic workflow policy and CLI option duplication |
| Running CLI | Current flags, values, validators, errors and actual capabilities | Agent workflow architecture |

## 3. Architectural Decision Matrix

| Decision | Portability | Drift resistance | Workflow completeness | Isolation | Result |
|---|---:|---:|---:|---:|---|
| One source-repository prompt with Go commands | low | low | medium | low | Rejected |
| A core that copies current CLI flags and regexes | high | low | high | medium | Rejected |
| A binary-oriented core with per-endpoint Help-first discovery | high | high | high | high | Selected |
| A thin relative source adapter | high | high | high | high | Selected |
| Scratch for every non-trivial task | low | medium | low | low | Rejected |
| Scratch only after a weighted uncertainty threshold | high | high | high | high | Selected |

The selected design gives the current binary authority over evolving technical
details while retaining an explicit, complete agent workflow for every
branching, commit, release, hotfix and delivery transition.

## 4. How the Adapter Works

1. The agent begins at `prompt.md`.
2. It resolves `core/prompt.md` relative to this file.
3. It reads the core completely before any workflow action.
4. It applies the core's Help-first contract to every endpoint.
5. Whenever the core specifies:

```text
git-governance <endpoint> ...
```

the adapter runs:

```text
go run -mod=readonly ./cmd/git-governance <endpoint> ...
```

6. The adapter never adds hardcoded flags, values or argument shapes. Each
   invocation derives those details from the immediately preceding current
   `--help` output.

## 5. Architectural Guarantees

The combined adapter and core guarantee:

```text
- the portable workflow has no dependency on AI-Base-Rules, docs/ or business files;
- this source repository retains its source-based execution binding;
- the current CLI help remains the authority for command syntax;
- branch and commit conventions are obtained from the live policy and validators;
- regular ticket, hotfix, release, support and conflict paths are all explicit;
- Scratch is selected through a decision matrix instead of created by default;
- current GOV-42 main-hotfix delivery endpoints and controller boundaries are represented;
- unavailable binary or protected-controller capability fails closed;
- no raw Git, static-token or provider-CLI workaround replaces a governed path.
```

## 6. Current Endpoint Coverage

The portable core requires Help-first discovery for the current CLI's:

```text
branch, commit, policy, doctor, validation and authentication endpoints
ticket start and publication workflows
hotfix record, delivery, single-commit and manifest propagation workflows
release cut, stabilization, alignment, promotion, backmerge and support workflows
Scratch cleanup and controlled transfer paths
```

The prompt does not reproduce the current option list. The live binary reports
the current option, value and validation contract at the moment each endpoint
is needed.

## 7. Cursor Entry Point and Portability

The stable Cursor entrypoint remains:

```text
.cursor/rules/governed-task-to-pr-workflow.mdc
```

It is a relative Git symlink to:

```text
../../__prompts__/version-control/git/workflow/prompt.md
```

That target remains portable across Windows, Linux and macOS checkouts. The
adapter then resolves the core using its own relative path, so neither layer
depends on a machine-specific absolute path.

## 8. File Index

| Path | Role |
|---|---|
| `prompt.md` | This Go-source adapter |
| `CONVENTIONS.md` | Adapter-only constraints |
| `DESCRIPTION.md` | This architecture explanation |
| `CHANGELOG.md` | Adapter version ledger |
| `core/prompt.md` | Complete portable workflow |
| `core/CONVENTIONS.md` | Portable core conventions |
| `core/DESCRIPTION.md` | Portable core architecture |
| `core/CHANGELOG.md` | Portable core history |
| `.cursor/rules/governed-task-to-pr-workflow.mdc` | Stable relative Cursor symlink to this adapter |

## 9. Execution Context for LLM Agents

Treat `prompt.md` as a loader and source-execution adapter. Read
`core/prompt.md` completely before acting. Do not use the adapter's small
size as permission to omit core workflow gates. Do not consult external
documentation to reconstruct CLI syntax: use the current binary's `--help`.
