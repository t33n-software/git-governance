# The organization as the management plane
[INTENT: ANWEISUNG]

## Convention

A GitHub **organization MUST** exist as the management plane for rulesets,
because only there can repository-spanning rulesets be defined. The rulesets
of this organization are managed once at the organization level:

- graphical: **Organization Settings → Repository → Rulesets**
- programmatic: `POST /orgs/{org}/rulesets`

Prerequisite: the organization plan MUST be at least **GitHub Team** (or
Enterprise), because the organization ruleset surface only exists from this
plan upward.

## Anti-pattern: individual repository rulesets

It is an **anti-pattern** to define the general rulesets individually per
repository. A definition copied per repository duplicates a governance
definition across the fleet, drifts apart unnoticed, and creates redundant
parallel truths.

Individual repository rulesets are valid **only** for explicit exception
scenarios. An exception scenario MUST be named and auditable, for example a
repository-specific required-check context or a stricter repository-local
layer above the organization baseline. Repository and organization rulesets
aggregate: the aggregate can only make a branch more restrictive, never
weaker. An exception ruleset MUST therefore never attempt to weaken,
duplicate, or override an organization ruleset; a perceived need for a weaker
rule is resolved through the targeting of the organization ruleset
(exclusion or narrower selection), never through a counteracting repository
ruleset.

## Governance properties of the organization level

- Only **organization owners** can create and edit organization rulesets;
  repository admins can only add additional, stricter repository rulesets.
- The organization level is therefore the binding governance baseline for all
  repositories.
- With the `~ALL` selector form, new repositories are bound automatically
  from their creation, without a re-import.
