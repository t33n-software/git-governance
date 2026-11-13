# Security-scanning capability partition
[INTENT: ANWEISUNG]

## The two orthogonal property axes

The fleet is governed through two independent repository custom properties:

| Axis | Property | Values | Consumer |
|---|---|---|---|
| Operating-system matrix | `quality-gates` | `full`, `linux-only`, `pending` | Class rulesets `02`–`04` |
| Security capability | `security-scanning` | `enabled`, `disabled` | Capability ruleset `09-security-scanning.json` |

Each axis carries exactly one value per repository, and the axes never
reference each other. The `quality-gates` axis decides which operating-system
quality gates bind; the `security-scanning` axis decides whether the GitHub
Advanced Security capability binds. A class ruleset never binds a security
capability surface, and the capability ruleset never binds an operating-system
matrix surface.

## The capability ruleset

`09-security-scanning.json` is the single owner of both security capability
surfaces on every shared line (`main`, `develop`, `release/*`,
`support/*`):

- the required status-check context
  `Dependency review / Dependency admission review`, and
- the `code_scanning` rule (CodeQL with `all` thresholds for alerts and
  security alerts).

It selects `repository_property security-scanning = enabled` and carries no
`do_not_enforce_on_create`: that exception stays the exclusive property of the
protected-line class rulesets, and a capability ruleset that skipped
enforcement on creation could silently admit new repositories without the
capability — a second owner of one invariant.

## Default posture (target architecture)

The canonical property definition declares `default_value: enabled`. The
target architecture binds the security capability to every governed
repository. `disabled` is a positive, organization-declared opt-out that the
platform instance assigns per repository through its pinned bindings; it is
never a canonical default, never a ruleset-class value, and never expressed by
weakening a class ruleset.

## The emitter-consistency law

A required status-check context may only bind on a repository whose workflows
provably emit it (the import-order contract of the ruleset README). The
capability axis is what keeps this law satisfiable under an opt-out: a
repository with `security-scanning: disabled` must not run the
dependency-review emitter and must not carry its required context, because the
capability ruleset does not bind it. Binding the context inside a class
ruleset instead would block every merge on an opted-out repository with a
context that can never be reported.

## Re-import behavior

The property definition, the per-repository values, and the capability ruleset
are re-projected through the reviewed instance lane in this order: property
schema, repository values, capability ruleset, class rulesets. A capability
ruleset whose selector property is missing or unassigned binds zero
repositories, exactly like a class ruleset.
