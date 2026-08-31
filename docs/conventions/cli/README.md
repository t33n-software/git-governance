# CLI Conventions
[INTENT: REFERENCE]

## Canonical source

This directory is the canonical source of truth for the command-line
interface conventions of this project: the rules that govern how a CLI
presents itself, how it is discovered, how values enter it, how results and
errors leave it, and how it is configured, secured, evolved, tested, and
distributed.

Every document in this area is standalone and self-contained: each one is
the single canonical definition of its subdomain and references no external
rule repository. The business code re-references these documents at the
implementation sites; the complete rationale lives exclusively here.

## Domain boundary

The CLI domain owns the interaction contract between the tool and its
consumers — humans and automation agents alike: identity, help, value
domains, output, interaction, errors, configuration, security,
compatibility, verification, and distribution.

It is distinct from the change-flow domain (`docs/conventions/commits/`,
`docs/conventions/pull-requests/`), which governs what a change must say
about itself, and from the hosting-platform domain
(`docs/conventions/hosting-platforms/`), which governs provider
infrastructure integration. A rule belongs to the CLI domain when it would
still apply if the tool managed an entirely different subject matter.

## Governing principle: discoverable-closed

All conventions in this area serve one architectural law: everything the
validation of a tool knows must be discoverable before the call — through
the help surface — not only after a failure. Fail-closed rejection of
invalid input is necessary but not sufficient; the target architecture is
discoverable-closed. The consumers of the help surface are humans and LLM
agents equally; both must be able to derive a valid call from the help
alone.

## Convention documents

| Document | Contract |
|---|---|
| [identity-and-discovery.md](identity-and-discovery.md) | one stable binary name, machine-readable version, consistent command-tree grouping, and the visibility law |
| [help-contract.md](help-contract.md) | a complete help area on every command node, the parent/child role split, the four help levels, and the generation mandate |
| [value-domain-model.md](value-domain-model.md) | the eight value classes and the binding help duty of each class, including the label law |
| [single-source-of-truth.md](single-source-of-truth.md) | one register per value domain, five rendering channels, the literal-duplication ban, and release-blocking drift |
| [output-contract.md](output-contract.md) | output modes, versioned machine schemas, structured error records, semantic exit codes, quiet mode, and the no-secrets law |
| [interaction-model.md](interaction-model.md) | non-interactive completeness, same-source prompts, mutation confirmation, dry-run, terminal detection, and accessibility |
| [error-philosophy.md](error-philosophy.md) | fail-closed at the earliest boundary, truthful remediation, no partial success, and missing-evidence-never-pass |
| [configuration.md](configuration.md) | one precedence order, configuration outside the code, secrets as references, bounded timeouts, and declared offline capability |
| [security-and-governance.md](security-and-governance.md) | read-only default, documented idempotency, audit records, least privilege, and login-flow authentication |
| [compatibility-and-lifecycle.md](compatibility-and-lifecycle.md) | semantic versioning on the CLI contract, the deprecation pipeline, stability levels, and additive schema evolution |
| [testing-and-verification.md](testing-and-verification.md) | help contract tests, property-based domain tests, the help-first consumer test, and CI-blocking drift |
| [distribution-and-operations.md](distribution-and-operations.md) | dependency-free artifacts, cross-platform parity, documented lifecycle, and telemetry discipline |

## Organization rules

- Directory and file names are lowercase kebab-case.
- Each convention document is written in English and is fully
  self-contained.
- Documents are programming-language-agnostic and subject-agnostic: they
  bind every CLI of the organization, present and future.
- Subdomain directories are created when the first convention for that
  subdomain exists; a child directory is created when a subdomain outgrows
  a single document.
