# CLI conventions — Configuration
[INTENT: REFERENCE]

## Canonical source

This document is the canonical source of truth for the configuration
conventions of a command-line tool: the single precedence order,
configuration outside the code, secrets as references, bounded timeouts,
and declared offline capability. It binds every CLI of the organization,
independent of programming language and subject matter.

## 1. One precedence order, organization-wide

`flag > environment variable > configuration file > default` — documented
once, identical for every CLI of the organization. Every flag has an
environment-variable mapping by naming convention, or a documented reason
why not.

## 2. Configuration outside the code

Deployment-specific configuration lives outside the artifact and is
type-validated at startup. Invalid configuration fails closed at startup,
not midway through an operation.

## 3. Secrets are never CLI arguments

Secrets are passed exclusively as references (environment variable, file,
broker) — arguments land in process lists and shell histories (see the
`secret-reference` class in
[value-domain-model.md](value-domain-model.md)).

## 4. Bounded, configurable timeouts

External processes and network access have bounded, configurable timeouts
with documented defaults; no unbounded operations exist.

## 5. Declared offline capability

Per command it is declared whether it works offline or requires network.
Consumers can therefore plan offline usage without trial and error.

## Positive example

```text
TOOL_OUTPUT=json tool validate --context ci
# flag would beat env, env would beat file, file would beat default —
# one order, identical everywhere
```

## Negative example

Two tools of the organization apply different precedence orders; a token is
accepted as a `--token <value>` argument; a network call has no timeout and
hangs without bound.
