# CLI conventions — Testing and verification
[INTENT: REFERENCE]

## Canonical source

This document is the canonical source of truth for the testing and
verification law of a command-line tool: help contract tests,
property-based domain tests, the help-first consumer test, and CI-blocking
drift. It binds every CLI of the organization, independent of programming
language and subject matter.

## 1. Help contract tests

Every help text is pinned against the command registry and the value-domain
source by contract or golden tests; a deviation between help and source
turns CI red (drift blocks, see
[single-source-of-truth.md](single-source-of-truth.md)).

## 2. Property-based domain tests

For every `closed-enum` flag it is proven that every documented value is
accepted and every undocumented value is rejected with the correct error
code; for every documented rule of a `free-constrained` or `shaped` flag at
least one pass and one fail test exists.

## 3. The help-first consumer test

A consumer-simulating test proves that a valid call is derivable from the
help alone (without the error path, without source-code reading) — the test
passes only when help values and validation agree.

## 4. Drift blocks CI

Every deviation between a projection and the source — help, errors,
prompts, completion, discovery — is a CI-blocking defect, not a warning.

## Positive example

A test iterates over the canonical value list of the source and checks:
every value appears in the help text, is accepted, and a control value
outside the list is rejected with the correct error code.

## Negative example

The help is only manually "read and deemed fine"; a test claims coverage
without comparing help against the source; a line-coverage percentage is
produced as a substitute for the semantic agreement of help and validation.
