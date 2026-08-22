# GitHub release-lifecycle workflow family contract

This document is the binding contract of the centralized release and hotfix
lifecycle workflow family and its callers in this home. It owns the trigger
surface, the permission model, the delivery-variant seam, the binding seams,
the recovery fold, and the pin lifecycle. The payloads live at the
platform-enforced execution location `.github/workflows/`; the hash-pinned
canonical callers live at `workflows/github/callers/release-lifecycle/`.

## The payload family

| Payload | Capability | Typed inputs (beyond the lane parameters) |
|---|---|---|
| `.github/workflows/reusable-release-control.yml` | The release-request controller: authorizes scope, version, source SHA, and target ref; verifies the tenant credential broker | `delivery_variant`, `verification_environment`, `request_environment`, `governance_repository`, `governance_ref` |
| `.github/workflows/reusable-execute-protected-line-request.yml` | The generic protected-line executor: exactly one bound protected-line mutation, the automatic finalizer, and the read-only recovery mode | `repository`, `execution_environment`, `recovery`, `governance_repository`, `governance_ref` |
| `.github/workflows/reusable-release-reconciliation.yml` | The conditional, evidence-bound backmerge to the integration line | `delivery_variant`, `reconciliation_environment`, `governance_repository`, `governance_ref` |
| `.github/workflows/reusable-tag-promoted-release.yml` | Creates the immutable release tag after a controlled promotion | `delivery_environment` |
| `.github/workflows/reusable-publish-release-artifacts.yml` | Delivery: artifacts, checksums, SBOMs, signatures, attestations, release metadata | `delivery_variant`, `delivery_environment`, `repository`, `source_gate` |
| `.github/workflows/reusable-hotfix-delivery.yml` | The main-bound hotfix patch delivery | `delivery_variant`, `delivery_environment`, `governance_repository`, `governance_ref` |
| `.github/workflows/reusable-hotfix-propagation.yml` | The controlled propagation of provenance-validated `fix/*` candidates | `delivery_variant`, `delivery_environment`, `propagation_environment`, `governance_repository`, `governance_ref` |

Every payload carries exclusively `on: workflow_call` and never triggers
itself. Every action reference inside a payload is a full-length commit SHA
with a version comment; a bump is a home release event with evidence, followed
by a fleet pin bump through a reviewed pull request. No payload carries an
organization name, a tenant name, a hard-coded repository guard, an endpoint,
or a credential.

## The trigger surface

The callers own the trigger surface: `release-control.yml`,
`release-reconciliation.yml`, `execute-protected-line-request.yml`, and
`hotfix-propagation.yml` are dispatch-only; `tag-promoted-release.yml` and
`hotfix-delivery.yml` additionally bind the pull-request-closed event on the
main line; `publish-release-artifacts.yml` additionally binds the `v*` tag
push. The caller also owns the concurrency group and the event-gating `if`
conditions; the payload receives validated typed inputs.

## The permission model

The caller carries `permissions: {}` at workflow level (default deny) and
grants the callee's least-privilege set explicitly on the calling job. A called
workflow's token is capped by the calling job's permissions and can never be
elevated beyond it. The bound grants:

| Caller | Calling-job grant |
|---|---|
| `release-control.yml` | `actions: write`, `contents: read`, `deployments: write`, `id-token: write` |
| `release-reconciliation.yml` | `contents: write`, `id-token: write` |
| `execute-protected-line-request.yml` | `actions: read`, `contents: write`, `deployments: write` |
| `tag-promoted-release.yml` | `actions: write`, `contents: write` |
| `publish-release-artifacts.yml` | `contents: write`, `id-token: write`, `attestations: write` |
| `hotfix-delivery.yml` | `actions: write`, `contents: write`, `id-token: write` |
| `hotfix-propagation.yml` | `contents: read`, `id-token: write` |

## The delivery-variant seam

The lifecycle has exactly two named, mutually exclusive delivery variants,
bound as data through the `delivery_variant` input — never a forked payload:

- **cloud** — the tenant-bound credential broker mints the short-lived,
  repository- and operation-bound credential; delivery runs the full
  supply-chain artifact chain.
- **github-only** — a signed immutable tag plus a GitHub release with
  provenance attestation; no cloud credential issuer is invoked. Lanes whose
  execution is bound to the broker boundary today (hotfix delivery and hotfix
  propagation) fail closed with a named deferral in this variant until their
  evidence path is migrated.

## The binding seams

- **Protected environments** carry the lane's non-sensitive references (the
  WIF provider, the service account, the endpoints) as environment variables —
  never as repository secrets and never embedded in a payload. The environment
  names are canonical lane identities and bind through typed caller inputs.
- **Repository variables** carry the tenant bindings with a named consumer:
  `GIT_GOVERNANCE_DELIVERY_VARIANT` (the bound delivery variant),
  `GIT_GOVERNANCE_SOURCE_GATE` (the source-verification gate form), and
  `GIT_GOVERNANCE_SOURCE_REF` (the pinned governance-home commit for tenant
  lanes; empty in this home, whose lanes build the CLI from the running
  checkout).
- **The governance CLI source** binds through `governance_repository` and
  `governance_ref`: a tenant lane checks out the pinned home commit and builds
  the CLI from it; this home's lanes build from the running checkout.

## The recovery fold

The read-only recovery finalization is a bound mode of the executor payload,
never a separate workflow: the executor caller dispatches
`execute-protected-line-request.yml` with the `recovery` input set, and the
payload performs only the verification-pending finalization bound to the main
line. No separate recovery caller identity exists.

## The CLI coupling

The CLI dispatches and verifies the lanes by caller file name and by the
executor's callee job identity. A reusable-workflow call surfaces the callee
job under the composite `<caller job name> / <callee job name>` identity; the
CLI accepts the bare payload job name and the composite form. The caller file
names are the stable contract the CLI binds.

## The caller contract

A tenant adopts the lifecycle by carrying the byte-identical callers from
`workflows/github/callers/release-lifecycle/`; each caller references its
payload by full-length commit SHA. Any deviation from the canonical caller is
a hash mismatch against `caller-hashes.json` and blocks fail-closed in the
fleet's conformance proof. Every caller hash is computed over the
LF-normalized form (CRLF folds to LF before hashing), so the proof is stable
regardless of a platform's checkout line endings. A tenant carries only the
callers for the lanes it runs; a lane the tenant does not run has no caller —
the state is a named class, never a renamed or disabled file.

## Dogfooding

This home is a tenant of its own family: the seven files under
`.github/workflows/` are byte-identical to the canonical masters, and the
contract-test set in `internal/packaging/` proves the identity on every home
change.
