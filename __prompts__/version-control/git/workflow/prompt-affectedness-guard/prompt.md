---
description: Forces a weighted affectedness evaluation against the governed workflow prompts (adapter and portable core) whenever CLI commands, subcommands, flags, or endpoint registrations change, and routes affected outcomes to an interactive user process instead of autonomous prompt edits
globs: cmd/**, internal/bootstrap/**, internal/application/cliparam/**, internal/application/workflow/**, internal/application/branch/**, internal/application/commit/**, internal/domain/**
alwaysApply: true
---

# Workflow-Prompt Affectedness Guard
[INTENT: INSTRUCTION]

## [0] Scope and Authority
[INTENT: INSTRUCTION]

This rule governs every creation, modification, rename, or deletion of a CLI
command, subcommand, flag, or endpoint registration in this repository. Its
protected surfaces are the governed workflow prompts:

- the source-repository adapter
  `__prompts__/version-control/git/workflow/prompt.md` (reachable through the
  `.cursor/rules` symlink `governed-task-to-pr-workflow.mdc`);
- the portable workflow core
  `__prompts__/version-control/git/workflow/core/prompt.md`.

Higher-priority system, safety, user, and repository rules remain binding.
This rule complements the adapter's activation contract; it never replaces it
and never authorizes autonomous edits of the protected surfaces.

## [1] Mandatory Conventions Read
[INTENT: INSTRUCTION]

Before every affectedness evaluation, read
`__prompts__/version-control/git/workflow/CONVENTIONS.md` fully. Its section 4
owns the authoritative ownership boundary used by the matrix in [3]:

- the core owns the generic workflow topology: endpoints, phases, gates, the
  state machine, and guard semantics;
- the binary owns CLI flags, validation formats, and endpoint options, which
  the core discovers at runtime through help-first re-anchoring;
- the adapter owns only the source-entrypoint binding.

Many cases are already excluded by this boundary: a flag-only delta on an
existing endpoint is covered by the help surface and never reaches the core.

## [2] Trigger
[INTENT: INSTRUCTION]

The affectedness evaluation is mandatory before committing any change that
creates, modifies, renames, or deletes:

- a command or subcommand registration in the cobra command tree;
- a flag or a value-domain entry that feeds the help surface;
- the externally visible behavior of a workflow application service;
- a branch, commit, or ticket grammar contract.

The evaluation runs even when the change looks purely internal: the register
drift axis in [3] exists precisely for changes that "only" add an endpoint.

## [3] Affectedness Multi-Decision Matrix
[INTENT: SPECIFICATION]

Compute the core-affectedness score from 0 to 100 using these weighted axes:

| Axis | Weight | A HIGH score means |
|---|---:|---|
| Topology delta | 30 | an endpoint or workflow family is added, removed, renamed, or its phase or gate topology changes |
| Register drift | 25 | the core's endpoint register would describe a command surface that does not exist, or miss one that does |
| Governance-semantics delta | 20 | the shared-line guard, the mutation embargo, the confirmation model, the evidence model, or the state machine changes |
| Failure-contract delta | 10 | error codes, exit classes, or the failure record shape change |
| Help-coverable surface only | -15 | the delta is entirely flag, option, validation, or help text of an existing endpoint (negative evidence: the core's help-first re-anchoring covers it at runtime) |

| Score | Binding | Action |
|---|---|---|
| 80-100 | core-affected | the interactive process in [4] is mandatory |
| 55-79 | uncertain | the interactive process in [4] with a focused question |
| 0-54 | not affected | no prompt edit; the help-first re-anchor is sufficient |

Adapter affectedness is evaluated separately and is true only when the
source-entrypoint binding changes (Go module layout, entrypoint path, adapter
location). Topology and flag deltas never affect the adapter.

## [4] Interactive Process Routing
[INTENT: CONSTRAINT]

The protected surfaces are never edited autonomously. When the matrix binds
core-affected or uncertain:

1. stop before any edit of the protected surface;
2. present the finding, the score, and the concrete proposed prompt delta to
   the user;
3. let the user confirm or correct every delta point;
4. edit only after explicit approval, exactly per delta point.

A one-time approval of a prompt delta is scope-locked to that delta and never
generalizes into a standing edit permission.

## [5] Prohibitions
[INTENT: CONSTRAINT]

Never:

- edit the core or the adapter autonomously, even when the change looks small;
- skip the CONVENTIONS.md read before the evaluation;
- treat register drift as harmless because the binary still works;
- bypass the matrix because a change "only adds a flag" without checking the
  help-coverable axis evidence;
- present a prompt edit as already decided while the interactive process is
  still pending.

## [6] Examples
[INTENT: REFERENCE]

Positive: adding the endpoint `workflow bootstrap` scores 85 (topology 30,
register 25, governance 20, failure 10) — core-affected; the interactive
process runs before any core edit.

Negative: adding a `--format` flag to an existing endpoint engages only the
help-coverable axis — not affected; no prompt edit, the runtime help
re-anchor covers it.
