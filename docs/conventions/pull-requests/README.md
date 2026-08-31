# Conventions: pull-requests
[INTENT: REFERENCE]

## Canonical source

This directory is the canonical source of truth for conventions of the
pull-request subdomain: the review gate that every governed change crosses
before it reaches a protected shared line (`main`, `develop`, `release/*`,
`support/*`).

## Domain boundary

The pull request is a domain-level review gate of the change-flow core
domain, not hosting infrastructure. The product proves this through the
provider-neutral `port.PullRequest` intent: the domain owns source, target,
ticket, title, and description; the hosting platform is only the transport
adapter beneath it.

`docs/conventions/hosting-platforms/` remains the infrastructure-integration
subdomain (provider workflow files, rulesets, environment gates, execution
materialization). A content mandate for the review gate does not belong
there, because it governs what a change must say about itself — independent
of which provider transports it.

## Area map

Conventions are grouped by subdomain, never by tooling:

```text
docs/conventions/
  cli/                 command-line interface interaction contracts
  branching/           branch topology, families, and naming
  commits/             commit message content contracts
  pull-requests/       the review gate (this area)
  release-lifecycle/   release, hotfix, and support line governance
  hosting-platforms/   provider infrastructure integration
```

Subdomain directories are created when the first convention for that
subdomain exists; this area is the first member of `pull-requests/`.

## Convention documents

- [Pull-request description mandate](description-mandate.md) — why every
  governed pull request carries a mandatory, canonically structured
  description, and how the CLI enforces it fail-closed.
