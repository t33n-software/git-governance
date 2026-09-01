# Hosting platform: GitHub — custom properties
[INTENT: SPEZIFIKATION]

## Canonical source

The organization custom properties for `t33n-software` are defined and
managed once, centrally, in this repository under
[`properties/github/`](../../../../../properties/github/README.md). Rulesets
**reference** properties through the `repository_property` selector; they
never define them. The definition is its own versioned artifact and is
distributed through the same signed and attested release channel as the
ruleset family.

A definition or value assignment outside this contract — in particular manual
maintenance in the Organization Settings — is an anti-pattern and forbidden
(the redundancy and drift prohibition).

## The three-layer contract
[INTENT: ARCHITEKTUR]

1. **Canonical definition (public, this repository).** Every property
   consumed by a ruleset selector exists exactly once as a versioned artifact
   `properties/github/<name>.json` in the REST schema of the property
   definition (`value_type`, `allowed_values`, `required`, `default_value`,
   `description`, `values_editable_by`). Contract tests prove that the
   `allowed_values` exactly match the class partition of the ruleset
   selectors and file names.
2. **Organization binding (private, platform instance).** The instance pins
   the definition and binds every repository-to-value assignment under
   `policy-overlays/hosting-platforms/github/properties/`
   (`definitions.yaml`, `assignments.yaml`, `enforcement.yaml`). The instance
   never authors a definition; it pins and binds — exactly like the ruleset
   bundle pin.
3. **Projection (reviewed IaC lane).** The definition and the assignments are
   projected through the OpenTofu standard lane: a value-free, generic module
   of the developer-platform-infrastructure core, executed with an
   organization owner identity from the instance lane. The graphical surface
   is exclusively a read and verification surface; a manual mutation is drift
   against the pinned bindings and is traced back to them.

## The positive-list convention
[INTENT: ANWEISUNG]

A custom property MAY only exist when all four preconditions are met:

1. a named GitHub-native consumer (a ruleset selector or a repository policy
   ruleset),
2. a canonical definition in `properties/github/`,
3. an instance binding,
4. a drift-bound verification.

Pure operational metadata without a ruleset consumer — for example
`databases`, `backup-required`, or `runtime-environment` — MUST NOT become
GitHub custom properties. Their canonical home is the responsible area of the
platform instance or the policy registry; CI/CD consumes them from there.
Defining metadata only in the Organization Settings creates an unversioned
drift surface and is forbidden.

## Value editability (hard boundary)
[INTENT: CONSTRAINT]

Every governed property sets `values_editable_by: org_actors`. With
`org_and_repo_actors`, a repository administrator could move their own
repository into a weaker ruleset class (for example `quality-gates` from
`full` to `linux-only`) and thereby weaken their own merge gates. Class
membership is a governance decision, never a repository-local one.

## The onboarding value `pending`
[INTENT: SPEZIFIKATION]

The `quality-gates` property carries the third allowed value `pending` as its
`default_value`. A repository whose workflows do not yet provably emit the
required check contexts of its class is assigned `pending` instead of
remaining property-less: the classification state is explicit, auditable, and
enforceable rather than implicitly absent. No shared-line ruleset targets
`pending`; such repositories remain bound only to the classless rulesets `00`
and `01` until their workflows are aligned.

## Activation sequence
[INTENT: ANWEISUNG]

The property schema and the initial assignments MUST be projected before the
class-partitioned shared-line rulesets `02`–`05` are switched to active. A
class ruleset whose selector references a missing or unassigned property
binds zero repositories — a silent fail-open of the entire shared-line
surface.

## Classification enforcement (deferred, entitlement-bound)
[INTENT: SPEZIFIKATION]

An organization ruleset with target `repository` (a repository policy
ruleset) CAN enforce the `quality-gates` classification so that no governed
repository remains unclassified. This capability is bound to the GitHub
Enterprise Cloud entitlement and is deferred until then. Until then, the
equivalent control is the drift and classification report of the instance
verifier and the policy validator. Upon activation, the ruleset MUST treat
`pending` as a compliant onboarding value so that the documented
workflow-alignment precondition is not broken.
