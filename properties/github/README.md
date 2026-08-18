# GitHub organization custom properties (canonical definition artifacts)

This directory is the canonical source of truth for the organization-wide
GitHub custom properties of the `t33n-software` organization. Each file is the
schema body of one organization custom property for:

- API: `PUT /orgs/{org}/properties/schema/{custom_property_name}`
- IaC: the generic, value-free projection module of the developer-platform
  infrastructure core, consumed by the platform-instance bindings

Custom properties are **referenced** by organization rulesets through the
`repository_property` selector; a ruleset never defines a property. The
definition surface is therefore governed as its own artifact family next to
[`../../rulesets/github/`](../../rulesets/github/README.md).

## Governed properties

| Property | Type | Allowed values | Consumer |
|---|---|---|---|
| `quality-gates` | `single_select` | `full`, `linux-only`, `pending` | Class partition of the shared-line rulesets `02`–`05` |

`full` binds the complete operating-system quality matrix (Linux, macOS,
Windows); `linux-only` binds the Linux quality gate only. `pending` is the
explicit onboarding state: a repository whose workflows cannot yet provably
emit the required check contexts of its class carries `pending` and remains
bound only by the classless rulesets `00` and `01` until its workflows are
aligned. No shared-line ruleset targets `pending`.

## Hard boundaries

- `values_editable_by` is always `org_actors`. With `org_and_repo_actors`, a
  repository administrator could reclassify its own repository into a weaker
  ruleset class and thereby weaken its own merge gates. Class membership is a
  governance decision, never a repository-local one.
- `required` is `true` and `default_value` is `pending`, so every repository —
  including every future repository — carries an explicit, auditable
  classification instead of an implicit absence.
- A property is created only with a named GitHub-native consumer (a ruleset
  selector or a repository policy ruleset), this canonical definition, a
  platform-instance binding, and drift-bound verification. Pure operational
  metadata without a ruleset consumer is never defined as a custom property
  here.

## Activation sequencing

The property schema and the initial repository assignments MUST be projected
before the class-partitioned shared-line rulesets `02`–`05` are switched to
active. A class ruleset whose selector references a missing or unassigned
property binds zero repositories — a silent fail-open for the shared-line
surface.

## Application and drift

The definition is applied through the reviewed platform-instance lane
(OpenTofu projection with an organization-owner identity), never through the
graphical surface. The organization settings UI is a read and verification
surface only; a manual mutation is drift against the pinned instance bindings
and is reconciled back to them. After a merged change to these sources,
re-project the definition and re-verify the live state against the bindings.
