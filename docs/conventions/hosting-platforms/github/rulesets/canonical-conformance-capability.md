# Canonical-conformance capability partition
[INTENT: ANWEISUNG]

## The three orthogonal property axes

The fleet is governed through three independent repository custom properties:

| Axis | Property | Values | Consumer |
|---|---|---|---|
| Operating-system matrix | `quality-gates` | `full`, `linux-only`, `pending` | Class rulesets `02`–`04` |
| Security capability | `security-scanning` | `enabled`, `disabled` | Capability ruleset `09-security-scanning.json` |
| Canonical-conformance capability | `canonical-conformance` | `enabled`, `disabled` | Capability ruleset `10-canonical-conformance.json` |

Each axis carries exactly one value per repository, and the axes never
reference each other. The `quality-gates` axis decides which operating-system
quality gates bind; the `security-scanning` axis decides whether the GitHub
Advanced Security capability binds; the `canonical-conformance` axis decides
whether the canonical-conformance required context binds. A class ruleset
never binds a capability surface, and a capability ruleset never binds an
operating-system matrix surface.

## The capability ruleset

`10-canonical-conformance.json` is the single owner of the required
status-check context
`Canonical conformance / Canonical bindings verification`
on every shared line (`main`, `develop`, `release/*`, `support/*`). It selects
`repository_property canonical-conformance = enabled` and carries no
`do_not_enforce_on_create`: that exception stays the exclusive property of the
protected-line class rulesets, and a capability ruleset that skipped
enforcement on creation could silently admit new repositories without the
capability — a second owner of one invariant.

## Default posture (target architecture)

The canonical property definition declares `default_value: enabled`. The
target architecture binds the canonical-conformance proof to every governed
repository whose workflows carry the canonical conformance caller. `disabled`
is only ever a positive, organization-declared instance assignment through the
platform-instance bindings; it is never a canonical default and never
expressed by weakening a class ruleset.

## The canonical-home exception (structural, not cost-bound)

`disabled` is reserved for exactly two positive declarations:

1. **The verifier-owning canonical home** (`repository-governance`): a home
   never carries the conformance lane against itself. Self-attestation has no
   independent evidence value — producer and verifier would collapse onto one
   identity — and the required context would gate the repair path of the very
   artifact that emits it: a defect in the verifier could block the merge of
   its own fix. The home's proof of the lane content is its own contract test
   family inside `full-local-build` (the conformance vectors, the home
   manifest self-consistency, the schema-decoder lockstep).
2. **Emitter-less repositories** (the onboarding state of this axis): a
   repository whose workflows do not yet carry the canonical conformance
   caller. Binding the context before the caller exists blocks every merge on
   a context that can never be reported — the phantom-context class proven by
   the blocked pull requests of the class flip.

Unlike the security axis, this exception is never a per-repository cost or
entitlement choice: the target architecture is `enabled` for every repository
that carries the caller, and the instance guard rejects every `disabled`
assignment outside the verifier-owning home. The guard proves both directions
fail-closed: `enabled` requires the caller in the fleet record, and `disabled`
requires the verifier-owning home.

## The emitter-consistency law (second application)

A required status-check context may only bind on a repository whose workflows
provably emit it (the import-order contract of the ruleset README). The
capability axis is what keeps this law satisfiable for the conformance
surface: a repository with `canonical-conformance: disabled` is not bound by
`10-canonical-conformance.json` and therefore never carries the conformance
required context, because the capability ruleset does not bind it. Binding the
context inside a class ruleset — the form before this partition — blocked the
verifier-owning home's own merge path, because the class flip activates the
context regardless of the structural exception.

## Re-import behavior

The property definition, the per-repository values, and the capability ruleset
are re-projected through the reviewed instance lane in this order: property
schema, repository values, capability ruleset, class rulesets. A capability
ruleset whose selector property is missing or unassigned binds zero
repositories, exactly like a class ruleset.
