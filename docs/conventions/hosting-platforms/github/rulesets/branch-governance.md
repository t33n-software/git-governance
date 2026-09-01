# Branch governance: structure of the shared-line and working-branch rulesets
[INTENT: SPEZIFIKATION]

This document describes independently what each ruleset of the family
enforces and why. The importable JSON definitions reside under
[`rulesets/github/`](../../../../../rulesets/github/README.md).

## Overview

| Ruleset | Target refs | Class |
|---|---|---|
| `push-protections: secret artifact boundary` | every push (no branch binding) | classless, private/internal repositories only |
| `branch-governance: ticket working branches` | `feature/*`, `fix/*`, `docs/*`, `refactor/*`, `chore/*`, `test/*`, `perf/*`, `hotfix/*` | classless, `~ALL` |
| `branch-governance: develop shared line (quality-gates=<class>)` | `develop` | full / linux-only |
| `branch-governance: main shared line (quality-gates=<class>)` | `main` | full / linux-only |
| `branch-governance: release shared lines (quality-gates=<class>)` | `release/*` | full / linux-only |
| `branch-governance: support shared lines (quality-gates=<class>)` | `support/*` | full / linux-only |

## Working-branch protection

Official working branches remain directly committable but become append-only
after the first push: `non_fast_forward` blocks the rewriting of published
history. No merge-time gate and no deletion prohibition are set there, so
that the normal development flow and the automatic deletion of merged head
branches keep working.

## Shared-line protection

Every shared line receives the same protection core:

- **deletion**: The line is never deletable; it carries published history,
  promotion, and evidence lineage.
- **non_fast_forward**: No rewriting of the published line.
- **pull_request**: Mutation only through reviewed pull requests with at
  least one approval, dismissed stale reviews, enforced resolution of all
  review threads, and **code owner review** (`require_code_owner_review`
  against `.github/CODEOWNERS`). Without the versioned ownership file, the
  line blocks fail-closed — the contract MUST be merged before activation.
- **required_status_checks** (strict): The binding contexts follow the naming
  law of the composed era (caller job = lane identity, callee job =
  gate/variant identity): the `linux-only` class binds the composite contexts
  `Quality gates / linux-amd64` and
  `Dependency review / Dependency admission review`; the `full` class carries
  the inline-era contexts `Quality gates (<os>)` plus
  `Dependency admission review` until the caller adoption of its
  repositories. The strict mode binds the merge to a current PR state.
- **code_scanning**: CodeQL with `all` thresholds for alerts and security
  alerts; available on public repositories at no additional cost.

Merge methods per line:

| Line | Allowed methods | Reason |
|---|---|---|
| `develop` | merge, rebase, squash | Choice of context for regular tickets; the semantic commit series may be preserved or cleaned up |
| `main`, `release/*`, `support/*` | merge only | Release, hotfix, and maintenance lineage remains visible as an explicit merge event |

## The creation exception `do_not_enforce_on_create`

Only the `release/*` and `support/*` rulesets carry
`do_not_enforce_on_create: true`: a newly created protected line cannot have
branch-bound check results before its existence; without the exception, the
governed creation would be impossible by definition. `develop` and `main`
explicitly carry `false`: they are standing lines without a controlled
recurring creation path; a re-creation is an anomaly and MUST hit the full
gate. The exception applies only to the one-time creation event and does not
loosen any other rule.

## The naming triple

Title, selector, and file name form one machine-verifiable triple:
`<context>: <aggregate> [(quality-gates=<class>)]` ↔
`repository_property` value ↔ `<nn>-<line>[.quality-gates-<class>].json`.
Classless rulesets deliberately carry no class.

## Deliberately not created

- **`scratch/*`**: private exploration with its own rewrite boundary; never a
  PR source — a ruleset would be ineffective or harmful.
- **`staging`**: an environment made of release artifacts, not a branch.
- **A separate `hotfix/*` ruleset**: redundant, because `hotfix/*` receives
  the same working-branch protection and its PR target carries the stricter
  shared-line ruleset.
