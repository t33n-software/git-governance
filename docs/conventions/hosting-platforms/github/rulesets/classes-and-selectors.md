# Classes and selectors
[INTENT: SPEZIFIKATION]

## The class model

The fleet is partitioned through the repository custom property
`quality-gates`:

| Class | Property value | Meaning |
|---|---|---|
| `full` | Quality gates for Linux, Windows, and macOS | Projects that ship binaries for all three operating systems (CLI tools) |
| `linux-only` | Quality gate exclusively for Linux | Projects that produce pure Linux/Docker/CI/CD artifacts and do not ship operating-system-specific binaries |

The architectural rationale of the classes: projects with real
operating-system deliveries need the full matrix, because every platform is
an independent delivery boundary. Projects without such artifacts would gain
no additional control value from Windows and macOS gates; their binding
boundary is the Linux CI from which the container and delivery artifacts are
built.

## Mutual exclusion (hard correctness rule)

Organization rulesets aggregate with each other and with repository
rulesets; the aggregate can only become more restrictive, never weaker. A
"general `~ALL` ruleset plus an additional weaker class ruleset" would
therefore fail closed on the class repositories, because the general variant
still binds them. Class variants of the same governance surface MUST
partition the fleet into mutually exclusive selector classes
(`quality-gates=full` versus `quality-gates=linux-only`), and no repository
may belong to both classes.

## Selector forms at the organization level

- `repository_name`: fnmatch patterns, `~ALL` for all repositories, an
  optional exclusion list, and a rename-protection flag.
- `repository_id`: explicit repository IDs (manual, stable, but unwieldy
  with fleet growth).
- `repository_property`: dynamic class or system properties; membership
  follows every property change on the repository automatically. This is the
  canonical mechanism for class-based differentiation.

## Global fleet addressing is an explicit option

An organization ruleset applies to all repositories only when the selector
says so explicitly: graphically the targeting choice **All repositories**, in
the REST schema `repository_name.include: ["~ALL"]`. A missing or empty
selector never implicitly means "all"; the organization schema requires
exactly one explicit selector per ruleset. The `~ALL` selection is dynamic:
it binds every current and every future repository from its creation, without
a re-import.

## The selector of the push protections

The push ruleset uses the `visibility` system property with the values
`private` and `internal`, because push rulesets exist only there. Before the
import, the selectability of this system property in the target account MUST
be verified; otherwise the documented fallback form is the explicit
`repository_name` selection of the private repositories.
