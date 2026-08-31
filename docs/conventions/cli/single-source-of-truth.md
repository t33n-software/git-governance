# CLI conventions — Single source of truth
[INTENT: REFERENCE]

## Canonical source

This document is the canonical source of truth for the
single-source-of-truth architecture of value domains: every value domain of
a command-line tool is defined exactly once, and every consumer-facing
surface renders from that one source. It binds every CLI of the
organization, independent of programming language and subject matter.

## 1. One register per value domain

Every value domain — value lists, grammars, rule sets, limits, examples —
is defined exactly once, in the domain or policy layer of the tool. Value
domains are domain knowledge; they do not live in the presentation layer.

## 2. The five consumption channels

Five consumption channels render from the one source — never from copies of
their own:

```text
K1  static --help texts
K2  interactive prompts and selects
K3  error contracts (expected / rule / example / remediation)
K4  shell completion on the value level
K5  machine-readable discovery endpoint (for example a policy describe with
    JSON output)
```

## 3. The literal-duplication ban

No value list, rule text, or grammar is maintained independently in two
places — not in help, errors, prompts, completion, discovery, or
documentation. This ban explicitly includes error texts: an error
constructor renders its expected-value list from the same source as the
help. A duplicated list drifts apart silently at the next domain extension
and creates parallel truths inside the same binary.

## 4. Endpoint subsets via declared filters

When an endpoint accepts a subset of a larger domain, the subset is produced
through a declared filter on the source (for example a "directly creatable"
predicate), never as a separate hand copy. The filter is part of the domain
definition, so the help of every endpoint provably shows the accepted set of
that endpoint (see [value-domain-model.md](value-domain-model.md)).

## 5. Contract tests pin every projection

Tests prove that help, errors, prompts, completion, and discovery output
match the source (see
[testing-and-verification.md](testing-and-verification.md)). The row rule of
the projection matrix: **no cell may contain a divergent answer — only a
compressed one** (same source, different depth of detail).

## 6. Drift is release-blocking

A deviation between a projection and the canonical source is a defect that
blocks the release — not a warning. Governance is enforced by the tool, not
merely documented as a convention.

## Positive example

```text
domain registry (one source) -> help renderer, prompt renderer, error renderer,
completion renderer, discovery endpoint; one contract test per channel pins the
projection against the registry.
```

## Negative example

The value list exists four times independently: as a help string, as an
error string, as a prompt text, and as a documentation table. At the next
domain extension at least two surfaces show stale values — a parallel truth
that loses its reason to exist.
