# ADR-0005: Separate Reconciliation Publisher Identity

- Status: Accepted
- Date: 2026-08-02
- Scope: Server-side publication of validated release-reconciliation candidates
- Decision makers: Release governance

## Context

A delivered release can require a ticket-bound `chore/*` reconciliation
candidate before a reviewed merge-commit pull request may target `develop`.
The controller must validate the candidate's ticket binding, immutable release
parent, pinned `develop` parent, delivery evidence, effective delta, and
quality result before publication.

The release-automation identity is deliberately limited to its release
lifecycle responsibilities. Granting it broad repository write access solely
to publish reconciliation candidates would combine protected-line lifecycle
dispatch with candidate publication and weaken separation of duties.

## Decision

The reconciliation controller uses a dedicated GitHub App installation token
minted by a separate private publisher broker deployment.

```text
release-control.yml on main
  -> protected release-reconciliation environment
  -> GitHub OIDC
  -> reconciliation publisher broker
  -> repository-bound publisher App installation token
  -> validated chore/* candidate publication and PR creation
  -> reviewed merge-commit PR to develop
```

The existing release-automation identity remains responsible for release-cut
and lifecycle operations. It does not receive reconciliation-publisher
repository write permission.

The publisher identity receives only the minimum GitHub permissions needed for
the controlled candidate path:

```text
Contents: Read & write
Pull requests: Read & write
Metadata: Read
```

It receives no Ruleset bypass, Actions write, Workflows write, administrative,
secret, or direct shared-line mutation permission.

The publisher broker:

- holds only the publisher App private key;
- accepts only the protected reconciliation workload identity;
- mints a short-lived token for exactly the approved repository;
- neither creates branches nor pull requests itself;
- never logs keys, installation tokens, workload tokens, or transport headers.

The controller validates a candidate before it uses the publisher token. It
may publish only a ticket-bound `chore/*` candidate whose merge parents prove
the immutable `release/<semver>` source and the pinned `develop` revision.
Rulesets continue to protect `main`, `develop`, `release/*`, and `support/*`;
the publisher identity receives no bypass.

## Consequences

- Reconciliation publication has an explicit, auditable least-privilege
  identity boundary.
- A compromise of the release-automation identity does not grant candidate
  publication permission.
- A compromise of the publisher identity cannot dispatch release lifecycle
  workflows or bypass shared-line Rulesets.
- The protected controller remains the only component that can turn a
  validated candidate into a pull request to `develop`.
- Deployment requires a dedicated broker service, secret, service accounts,
  workload-identity provider, Cloud Run Invoker binding, and GitHub
  environment variables.
- The normal delivery-to-reconciliation trigger remains server-side; manual
  dispatch is limited to incident, retry, and recovery.
