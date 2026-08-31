# CLI conventions — Interaction model
[INTENT: REFERENCE]

## Canonical source

This document is the canonical source of truth for the interaction model of
a command-line tool: non-interactive completeness, prompts rendered from
the same source, mutation confirmation, dry-run, documented terminal
detection, and accessible output. It binds every CLI of the organization,
independent of programming language and subject matter.

## 1. Non-interactive completeness

Every capability of the tool is reachable without a terminal — via flags or
explicit configuration. The interactive mode is a guided comfort layer,
never the only path. When a required value is missing non-interactively,
the call fails closed instead of hanging in a prompt.

## 2. Prompts from the same source

Interactive prompts and selects carry the same rule texts and the same
options as the static help — rendered from the same source (see
[single-source-of-truth.md](single-source-of-truth.md)) — with live
validation. An interactive select that shows other values than the static
help is two truths and forbidden.

## 3. Mutation confirmation and dry-run

Mutating operations require an explicit confirmation or an explicit
confirmation flag (for example `--yes`). Every mutating command has a
`--dry-run` that shows the plan without mutating.

## 4. Documented terminal detection

The interaction mode (for example `auto|always|never`) is documented; in
non-terminal contexts (CI, agents) the tool never hangs in a prompt loop.

## 5. Accessible output

An accessible, line-oriented output form is available wherever form- or
color-based rendering is used.

## Positive example

```text
tool release cut --version 2.8.0 --dry-run     # shows the plan, mutates nothing
tool release cut --version 2.8.0 --yes         # non-interactive, complete
tool release cut                               # interactive: the select shows the same
                                               # values and rules as the static help
```

## Negative example

A required value is reachable only interactively (no flag exists); or the
tool opens a prompt loop in a CI environment and blocks the run; or the
interactive select shows different values than the static help.
