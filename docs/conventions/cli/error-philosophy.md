# CLI conventions — Error philosophy
[INTENT: REFERENCE]

## Canonical source

This document is the canonical source of truth for the error philosophy of
a command-line tool: where errors are rejected, what the help owes before
the call, what a remediation owes after the failure, and how partial
results and missing evidence are reported. It binds every CLI of the
organization, independent of programming language and subject matter.

## 1. Fail-closed at the earliest boundary

Invalid input is rejected hard at the earliest possible point — with a
named remediation. A precondition that fails only at a remote boundary is a
gate at the wrong place.

## 2. Discoverable-closed

Fail-closed alone is not sufficient: everything the validation knows must
additionally be discoverable before the call — through the help and the
discovery endpoint (see [help-contract.md](help-contract.md) and
[value-domain-model.md](value-domain-model.md)). A tool that reveals its
value domains only in error messages forces every consumer — human or
agent — into a costly guess-fail cycle.

## 3. Truthful remediation

A remediation reference may only point to surfaces that actually carry the
answer. "See `--help`" is forbidden when the help does not contain the
referenced information. The remediation is part of the error contract (see
[output-contract.md](output-contract.md)) and is held to the same truth
standard as the error itself.

## 4. No partial success as success

A partially executed operation is never reported as overall success; the
reported state names the exact stand and the next step.

## 5. Missing evidence is never PASS

A check that was not executed, or whose proof is missing, does not count as
passed. This law binds every verification surface of the tool and of its
own development process alike.

## Positive example

The input violation is rejected before any mutation; the error record names
field, actual, expected, example, and a remediation whose reference target
carries the answer.

## Negative example

The invalid input is noticed only after a half-finished execution; the
error points to a help that does not list the value set; the completion
reports "done" although a sub-step failed. All three behaviors violate the
error philosophy independently of one another.
