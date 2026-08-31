# CLI conventions — Output contract
[INTENT: REFERENCE]

## Canonical source

This document is the canonical source of truth for the output and error
contract of a command-line tool: the stable output-mode switch, the
versioned machine-readable schema, the structured error record, semantic
exit codes, quiet mode, and the no-secrets law. It binds every CLI of the
organization, independent of programming language and subject matter.

## 1. The stable output-mode contract

The tool has a documented output-mode switch (for example
`--output human|json`). The machine-readable form carries a versioned
schema that evolves only additively within a schema version (see
[compatibility-and-lifecycle.md](compatibility-and-lifecycle.md)).

## 2. The structured error record

Every error is a coded record carrying at least:

- the error code;
- the affected field;
- the actual value;
- the expected value or set;
- the violated rule;
- a valid example;
- the remediation.

The human and the machine form carry **informationally identical** content;
a JSON form with less information than the human form (or vice versa) is a
contract breach.

## 3. Semantic exit codes

Exit codes are stable, documented, and carry semantic classes (for example:
success, usage error, governance rejection, external failure, internal
failure). Scripts and automation must be able to branch on the class
without parsing message text.

## 4. Quiet mode

Successful human output is suppressible (for example `--quiet`); machine
output is never chatty. Error output is not suppressed by quiet mode.

## 5. The no-secrets law

Neither errors nor logs nor audit records contain credentials, tokens,
keys, or headers (see
[security-and-governance.md](security-and-governance.md)).

## Positive example

```text
Error [VALUE_INVALID]
Field: strategy
Actual value: rebase-and-merge
Expected: check, auto, rebase, or merge
Valid example: --strategy rebase
How to fix it: select a supported strategy
(exit code: usage class)
```

## Negative example

```text
Error: invalid input, see --help
```

— without field, actual value, expected set, and remediation; doubly
problematic when the referenced help does not actually carry the values (an
untrue remediation, see [error-philosophy.md](error-philosophy.md)).
Equally forbidden: errors as prose without a stable code, or JSON and human
forms with diverging information content.
