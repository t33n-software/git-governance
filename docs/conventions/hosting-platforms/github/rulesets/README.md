# Hosting platform: GitHub — rulesets
[INTENT: REFERENZ]

## Canonical source

The GitHub rulesets for the `t33n-software` organization are defined and
managed once, centrally, in this repository under
[`rulesets/github/`](../../../../../rulesets/github/README.md). This
repository is the canonical source of truth for the JSON definitions: it
explains the architecture, sets the definitions, and delivers the versioned,
importable artifacts.

A local copy, redefinition, or deviation in another repository is an
anti-pattern and forbidden (the redundancy and drift prohibition). Only
named, auditable repository exceptions are permitted, ones that are more
restrictive than the organization baseline, never weaker.

## Family in use

This project (`git-governance`) uses the **`quality-gates=full`** family:

- The quality gates run on **Linux**, **Windows**, and **macOS**.
- Architectural rationale: this project ships a CLI that is built, attested,
  and verified as a native binary for all three operating systems; delivering
  for all operating systems requires the full quality-gate matrix.

## Convention documentation

| Document | Content |
|---|---|
| [The organization as the management plane](organization.md) | Why an organization MUST exist; organization-level management; the anti-pattern of individual repository rulesets |
| [Branch governance](branch-governance.md) | Structure and rationale of every shared-line and working-branch ruleset; the naming triple |
| [Push protections](push-protections.md) | The boundary against secret-shaped artifacts; availability; template architecture |
| [Classes and selectors](classes-and-selectors.md) | The `quality-gates` class model, mutual exclusion, selector forms, `~ALL` |
| [Code quality and coverage](code-quality-and-coverage.md) | Organization-owned, language-agnostic gates; the exclusion of the hosting controls |
| [Merge strategy](merge-strategy.md) | The merge-method matrix, the update-branch boundary, the global repository settings |
| [Import and verification](import-and-verification.md) | Prerequisites, import order, Evaluate→Active, the verification checklist |
| [Tag governance](tag-governance.md) | The version-tag namespace bound to the release-automation identity, the namespace floor, the activation precondition |
| [Custom properties](../custom-properties/README.md) | The positive-list convention, the three-layer contract, the `org_actors` boundary, `pending` onboarding, the activation sequence |

## Management

- Management plane: the **organization** (`t33n-software`), never the
  individual repository level.
- Class membership of this repository: the `quality-gates=full` custom
  property.
- The custom property definition resides canonically in
  [`properties/github/`](../../../../../properties/github/README.md); its
  projection and assignment follow the
  [custom properties convention](../custom-properties/README.md).
- Changes to the rulesets happen exclusively in this canonical repository
  and are re-imported at the organization level afterwards.
