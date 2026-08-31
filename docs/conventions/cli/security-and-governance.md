# CLI conventions — Security and governance
[INTENT: REFERENCE]

## Canonical source

This document is the canonical source of truth for the security and
governance conventions of a command-line tool: read-only default,
documented idempotency, audit records, least privilege, login-flow
authentication, and fortified distribution. It binds every CLI of the
organization, independent of programming language and subject matter.

## 1. Read-only default

Reading commands are the normal state; every mutation is explicit (its own
switch, a confirmation, a dry-run path — see
[interaction-model.md](interaction-model.md)).

## 2. Documented idempotency

For every mutating command it is documented whether a repetition is safe
and how the repetition case is handled.

## 3. Audit records for governed mutations

Governed mutations produce an auditable record (actor, action, time,
inputs, result) — without secrets (the no-secrets law of
[output-contract.md](output-contract.md)).

## 4. Least privilege

The tool requests only the permissions the invoked operation needs.
Authentication happens through explicit login flows, never through token
arguments (see [configuration.md](configuration.md)).

## 5. Fortified distribution

The artifact is signed and distributed with checksums and attestations;
dependencies are pinned; a bill of materials is attached. Governance is
enforced by the tool, not only documented as convention (see
[distribution-and-operations.md](distribution-and-operations.md)).

## Positive example

A mutating command without a confirmation flag asks interactively, shows
the plan first (dry-run equivalent), and writes an audit record without
confidential values; the release artifact carries signature, checksum, and
SBOM.

## Negative example

A command mutates silently within the same call that also reads; an access
token is accepted as a positional argument; the artifact is distributed
without checksum and signature.
