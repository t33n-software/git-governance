# Hosting platform: GitHub — workflows — Environment-gate trigger context
[INTENT: REFERENZ]

## Canonical source

This file is the canonical source of truth for the convention that governs on
which run context an environment-gated job of the release and hotfix lifecycle
family may execute. The caller workflows re-reference this file as the
authority and carry only a short pointer at the affected location; the
complete rationale lives exclusively here.

## The rule

A job that binds a protected GitHub environment MUST NEVER execute on a
`pull_request`-triggered run. A `pull_request` run executes on the ephemeral
merge ref `refs/pull/<n>/merge`, which is not a branch and can never satisfy
the protected environment's deployment branch policy (`main`, `v*`); GitHub
rejects the deployment admission before the first step, so the lane fails
without producing any effect.

## The bound pattern: detect then execute

An event-driven lane splits into exactly two jobs:

1. **Detection** runs on the `pull_request` (closed) event with no environment
   binding and only the `actions: write` grant. It validates the promotion
   semantics (merged, base line, source branch family, same repository) and
   dispatches the same caller as a `workflow_dispatch` run on the default
   branch with the lane inputs.
2. **Execution** runs exclusively on that main-bound dispatch run
   (`if: github.event_name == 'workflow_dispatch'`) and carries the
   environment-bound reusable call. The environment admission — the deployment
   branch policy plus the required reviewers — gates the mutation.

The payload is unchanged; the split lives entirely in the caller.

## Why this is the foundation

- The trigger automation stays server-side and the start remains a conscious
  admission act: the detection run is the evidence, the execution run is the
  gated mutation.
- The detection run is zero-privilege beyond the dispatch grant: it holds no
  content or identity permission and no environment, so a misfire cannot
  mutate anything; the payload validates fail-closed (the merge commit must be
  contained in the main line, the immutable tag must not already exist, the
  source branch must be canonical).
- The recovery of a missed or rejected execution is a manual dispatch of the
  same caller on the default branch with the same inputs — never a re-merge
  and never a history rewrite.

## Verification

The packaging contract tests prove the split on every change: the event-driven
callers carry both triggers, the detection job carries no environment and no
mutation or identity grant, the execution job is dispatch-only, and the
dispatch targets the default branch. A deviation is a fail-closed defect.
