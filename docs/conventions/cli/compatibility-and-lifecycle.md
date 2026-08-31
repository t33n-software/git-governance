# CLI conventions — Compatibility and lifecycle
[INTENT: REFERENCE]

## Canonical source

This document is the canonical source of truth for the compatibility and
lifecycle conventions of a command-line tool: semantic versioning on the
CLI contract, the deprecation pipeline, stability levels in the help, and
additive schema evolution. It binds every CLI of the organization,
independent of programming language and subject matter.

## 1. Semantic versioning on the CLI contract

The command, flag, and output surface is a versioned contract. A breaking
change to commands, flags, value domains, or exit codes is a major change.
The version identifier is machine-readable (see
[identity-and-discovery.md](identity-and-discovery.md)).

## 2. The deprecation pipeline

Deprecation follows the fixed sequence "notice in the help → warning at
runtime → removal", each stage with a date or version horizon. Silent
removal is forbidden.

## 3. Stability levels in the help

Commands and flags carry their stability level (for example `stable`,
`experimental`, `internal`) visibly in the help.

## 4. Additive schema evolution

Machine-readable outputs evolve within a schema version exclusively
additively; removing or renaming changes require a new schema version (see
[output-contract.md](output-contract.md)).

## Positive example

```text
--old-flag string   DEPRECATED since 2.4.0, removal in 3.0.0; use --new-flag
```

## Negative example

A flag disappears in a minor release without warning; a JSON field is
renamed within the same schema version; a command meant as `experimental`
carries no stability marker at all.
