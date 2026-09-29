# Description: Workflow-Prompt Affectedness Guard
[INTENT: CONTEXT]

## 1. Purpose
[INTENT: CONTEXT]

This directory provides the repository-local Cursor rule that guards the
consistency boundary between the `git-governance` CLI surface and the
governed workflow prompts (the source adapter and the portable core).

The rule forces a weighted affectedness evaluation whenever a command,
subcommand, flag, or endpoint registration is created, modified, renamed, or
deleted, and it routes every affected or uncertain outcome to an interactive
user process instead of autonomous prompt edits.

## 2. Why the Separation Exists
[INTENT: CONTEXT]

The guard is deliberately not part of the adapter and not part of the core:

```text
adapter  = activation contract and Go source-entrypoint binding
core     = portable workflow semantics
guard    = drift control for both, triggered by CLI-surface work
```

Folding the guard into the adapter would give the adapter a second concern
beyond binding; folding it into the core would make the portable core depend
on this repository's file layout. As its own artifact, the guard references
both surfaces without coupling them.

## 3. Architectural Decision Matrix
[INTENT: REFERENCE]

| Decision | Portability | Drift resistance | Boundary clarity | Isolation | Result |
|---|---:|---:|---:|---:|---|
| Guard logic inside the adapter | low | medium | low | low | Rejected |
| Guard logic inside the portable core | low | low | medium | low | Rejected |
| Guard as its own rule artifact beside the adapter | high | high | high | high | Selected |
| Autonomous core edits on detected drift | low | low | low | low | Rejected |
| Interactive user process on affected or uncertain outcomes | high | high | high | high | Selected |

## 4. How the Guard Works
[INTENT: SPECIFICATION]

1. The rule fires on CLI-surface work through its Cursor glob scope.
2. Before any evaluation, the agent reads the workflow area `CONVENTIONS.md`;
   its section 4 owns the ownership boundary (core owns workflow topology,
   the binary owns flags and options discovered through `--help`, the adapter
   owns only the source-entrypoint binding).
3. The weighted matrix scores the change: topology delta, register drift,
   governance-semantics delta, failure-contract delta, and the negative
   help-coverable axis.
4. The band decides: `80-100` core-affected, `55-79` uncertain, `0-54` not
   affected. Adapter affectedness is evaluated separately and binds only to
   source-entrypoint binding changes.
5. Affected or uncertain outcomes stop the workflow and route to the
   interactive user process; the protected surfaces are never edited
   autonomously.

## 5. Architectural Guarantees
[INTENT: SPECIFICATION]

```text
- every CLI-surface change passes the weighted evaluation before commit;
- flag-only deltas on existing endpoints are provably covered by the core's
  help-first re-anchoring and never trigger prompt edits;
- new endpoints always trigger the interactive process, because they are
  workflow topology and register content;
- the core and the adapter are never edited autonomously;
- a one-time delta approval never generalizes into a standing edit
  permission.
```

## 6. Cursor Entry Point and Portability
[INTENT: REFERENCE]

The stable Cursor entrypoint remains:

```text
.cursor/rules/workflow-prompt-affectedness-guard.mdc
```

It is a relative Git symlink to:

```text
../../__prompts__/version-control/git/workflow/prompt-affectedness-guard/prompt.md
```

That target remains portable across Windows, Linux and macOS checkouts. The
rule file carries the Cursor rule frontmatter block (`description`, `globs`,
`alwaysApply`) at its head; without it, a symlinked rule target resolves to
content that carries no activation metadata and is never injected into the
agent context.

## 7. File Index
[INTENT: REFERENCE]

| Path | Role |
|---|---|
| `prompt.md` | The affectedness guard rule |
| `DESCRIPTION.md` | This architecture explanation |
| `CHANGELOG.md` | Guard version ledger |
| `.cursor/rules/workflow-prompt-affectedness-guard.mdc` | Stable relative Cursor symlink to this rule |

## 8. Execution Context for LLM Agents
[INTENT: CONTEXT]

Treat `prompt.md` as a binding governance rule, not as reference material.
Run the CONVENTIONS.md read and the weighted evaluation before committing any
CLI-surface change. Never edit the protected workflow prompts autonomously;
route affected or uncertain outcomes to the interactive user process and wait
for explicit per-delta approval.
