# ADR-0004: Trusted Release Reconciliation Control

- Status: Accepted
- Date: 2026-08-01
- Scope: execution of a strict `release/<semver> -> develop` reconciliation
- Deciders: Release governance

## Context

A delivered `release/<semver>` ref remains unchanged after promotion, tag, and
delivery. When `develop` contains new commits since the promotion and enforces
a current pull request base, a ticket-bound preparation branch from the
release ref is needed.

The preparation branch deliberately does not automatically contain the latest
CLI implementation. A call of `go run ./cmd/git-governance` from that worktree
would therefore not reliably contain the controlled reconciliation workflow. A
manual merge would not be a permissible alternative.

## Decision

The protected release control workflow file on `main` builds an immutable CLI
binary from the trusted main control-plane commit before every reconciliation
branch switch.

```text
trusted main control-plane source
  -> build immutable release-control binary
  -> create release-derived preparation branch
  -> execute binary against that branch
  -> merge current develop only into preparation branch
  -> quality gates and reviewed PR to develop
  -> or fail-closed conflict manifest and controlled recovery
```

The workflow operation `reconciliation-align` requires the release line, the
ticket key, the ticket number, and the slug. It receives a short-lived broker
workload identity, configures the Git transport only in the ephemeral runner
with a masked installation token, and removes that configuration at the end of
the job.

After successful tag, artifact, attestation, and release delivery, the
reconciliation is started automatically in the regular target path from the
same delivery lifecycle. The controller re-verifies all delivery facts and
only creates the preparation branch PR to `develop` when an effective delta
exists. A manual `workflow_dispatch` start is exclusively the incident, retry,
and recovery fallback.

The binary first runs `workflow release stabilize --kind release-prep` and
then `workflow release align-reconciliation-base`. The broker remains
exclusively the token issuer; it creates neither branches nor pull requests.

The release-automation identity and the reconciliation-publisher identity are
separate. The publisher identity is used exclusively through the protected
`release-reconciliation` environment and its own broker, app key, and
short-lived installation token. It publishes only the provenance-validated
candidate and its pull request; it has no ruleset bypass, no release-line
dispatch, and no shared-line mutation permission. ADR-0005 defines this
identity boundary.

On a merge conflict, no unresolved branch is pushed and no PR is created. The
conflict proof binds the release SHA, the develop SHA, the ticket, the
preparation branch, the conflict paths, and the controller run. A human- or
agent-resolved candidate branch remains non-shared and receives no
release-automation permission.

The protected `reconciliation-resume` path does not accept a candidate based
on its name. It verifies the branch and ticket binding, the unchanged release
origin, and an exact two-parent merge with the pinned develop revision. Only
then may CI with a short-lived broker token run quality, publish the candidate
branch, and create the PR to `develop`.

## Trigger boundary and the admission act

Triggering the reconciliation controller is its own governance act and is
strictly separated from the privileged execution chain. The trigger is an
admission — the deliberate registration of a matter for privileged
verification — not the privilege itself. The privileged effect arises
exclusively behind the server-side chain of environment approval, OIDC/WIF
workload identity, publisher broker, and dedicated publisher token (ADR-0005).

Four stipulations apply bindingly:

1. **Trigger identity equivalence.** Whether a human triggers the dispatch
   through the GitHub surface, a human or agent through `gh`, or an explicit,
   separate command, is trust-equivalent as long as the act happens
   separately, deliberately, and with a recorded actor identity. The control
   is carried by the server-side approval and validation chain, not by the
   triggering identity. A dispatch requires neither publisher nor shared-line
   permission, only a dispatch-authorized operator identity.

2. **No automatic trigger as a side effect.** The local CLI never triggers
   the reconciliation controller as an automatic consequence of a candidate
   push or resume. Preparation (non-shared, local) and admission (a deliberate
   act) do not collapse into an automated act; the local CLI receives no
   standing dispatch credential for the reconciliation lane.

3. **The automation location is server-side.** The regular trigger automation
   belongs exclusively in the server-side delivery lifecycle, which starts
   event-driven and idempotently after confirmed delivery. Local tooling is
   never the automation location of the reconciliation lane.

4. **Manual dispatch as the protected recovery entry.** The manual dispatch
   remains the designated incident, retry, and recovery path. It is a
   deliberate admission decision after a fail-closed state and passes the same
   input, delivery, idempotency, and audit checks as the automatic path.

This boundary is not justified by a missing capability of the local CLI — a
dispatch needs no publisher rights — but by the separation of deliberation and
privilege, the least-privilege hygiene of local tooling identities, and the
server-side automation location.

## Invariants

- `release/<semver>` is never updated, rebased, or directly pushed by the
  controller.
- The controller only accepts a `main` workflow dispatch in the protected
  release environment.
- OIDC tokens and installation tokens are not printed, persisted, or stored as
  repository secrets.
- The reconciliation-publisher identity is separate from the
  release-automation identity and holds only the minimal candidate and PR
  publication permission.
- The Git transport header exists only in the local runner configuration scope
  and is removed before the job ends.
- The preparation branch carries the ticket reference, starts from the
  release ref, and is the only merge location for the current develop state.
- A conflict leads fail-closed to no shared-line mutation, no unresolved
  remote branch, and no PR.
- A recovery candidate must consist of exactly the release and pinned develop
  parents; arbitrary branch inputs are no trust proof.
- The resulting pull request targets `develop` and uses a merge commit.
- The controller creates PRs idempotently but never merges directly into
  `develop`; review and required checks remain binding.
- The reconciliation trigger is a deliberate admission; human, agent, and UI
  dispatch are trust-equivalent as long as the act remains separate and
  audited.
- The local CLI never triggers the controller as a side effect of a local
  mutation and holds no standing dispatch credential for the reconciliation
  lane.
- The regular trigger automation lies server-side in the delivery lifecycle;
  the manual dispatch remains the protected recovery entry.

## Consequences

- A workflow merged into `develop` can only be used privilegedly for a
  delivered release line after its governed main control-plane promotion.
- Dry-run calls remain strictly read-only and may trigger neither provider
  publishing nor pull request creation.
- The reconciliation remains traceable without changing the published release
  lineage.
- The normal path needs no manual operator start; its manual fallback remains
  available for controlled recovery.
- The trigger boundary is laid down as an admission act; the misreading that
  the local CLI could not execute the dispatch because it lacked publisher
  rights is ruled out — a dispatch only needs a dispatch-authorized operator
  identity.
