# Conventions: Portable Git-Governance Agent Workflow Core
[INTENT: CONSTRAINT]

## 1. Scope

These conventions govern `core/prompt.md` and its metadata. They keep the
workflow portable across repositories that use an installed
`git-governance` binary.

The core owns:

```text
- agent workflow order;
- help-first endpoint discovery;
- state, proof and completion gates;
- branch-continuation and Scratch decisions;
- regular ticket, hotfix, release and support topology;
- conflict, security, evidence and publication boundaries.
```

The core does not own:

```text
- a repository's binary installation method;
- CLI flags, regexes, limits or option values;
- repository-specific quality command arrays;
- hosting credentials, tokens or private keys;
- project business policy;
- source-code implementation details.
```

## 2. Mandatory Help-first Rule

For every `git-governance` endpoint invocation:

```text
1. Print a concise public purpose statement.
2. Run: git-governance <endpoint> --help
3. Read the complete current help result.
4. Print a concise public purpose statement for the actual invocation.
5. Execute one invocation using only the newly exposed contract.
6. Wait for the real result.
```

The same endpoint requires a fresh help result for each actual invocation.
Never freeze a currently observed flag list or validation regex into this
portable prompt.

## 3. Runtime Authority

The current binary is the authority for:

```text
- option names and global modes;
- values, syntax and technical limits;
- available endpoint names;
- current policy output;
- validation and error results;
- dry-run, confirmation and provider behavior.
```

The core can name a logical endpoint only when that endpoint is required to
fulfill a workflow transition. If the endpoint disappears or its current help
does not expose the needed capability, report `BLOCKED`; do not synthesize
arguments or use a lower-level ungoverned replacement.

## 4. Prompt Authoring Rules

Changes to `core/prompt.md` must:

```text
- preserve the distinction between portable workflow invariant and runtime CLI detail;
- state why and when an endpoint is used;
- retain explicit state and evidence transitions;
- preserve no-secret and no-bypass constraints;
- keep ordinary, release and hotfix paths distinct;
- keep Scratch optional and evidence-based;
- avoid a duplicate external documentation dependency;
- avoid hardcoded project paths, Go commands, package names and provider-specific credentials.
```

Changes must not:

```text
- add a second source of truth for branch grammar or commit grammar;
- enumerate flags copied from a point-in-time help output;
- treat visible local contracts as architectural authority without checking scope;
- invent a future CLI endpoint or controller;
- make a local check equivalent to CI, review or Ruleset approval;
- permit raw Git, provider CLI or static-token workarounds for a missing governed capability.
```

## 5. Scratch Convention

Scratch is not a default phase. It requires the documented weighted decision
matrix from `core/prompt.md`. A clear, bounded implementation on an official
branch must work directly on that branch. Scratch is private, temporary and
never a PR source.

## 6. Hotfix Convention

Main- and Support-affecting hotfix work requires a reviewed record, commit
budget, ordered manifest, merge verification, immutable delivery evidence and
explicit propagation outcomes. The core must remain fail-closed when the
binary or protected controller has not implemented a required capability.

The core must never prescribe:

```text
- a broad main-to-develop merge;
- blind cherry-picking of a pull-request merge commit;
- a raw multi-commit cherry-pick loop;
- reuse of a delivery identity as an unrestricted publisher.
```

## 7. Metadata Convention

`DESCRIPTION.md` explains the current architecture and execution behavior.
`CHANGELOG.md` records every material core-contract change with a semantic
version, compatibility assessment and affected surfaces.

Metadata must describe only implemented prompt behavior. It must not portray
an unimplemented binary endpoint, controller or infrastructure component as
available.
