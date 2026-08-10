# ADR-0007: Functional Release and Hotfix Control Lanes

- Status: Accepted
- Date: 2026-08-10
- Scope: GitHub Actions environments, credential issuance, and controller responsibilities
- Decision makers: Release governance

## Context

The repository previously used a generic `release` environment for unrelated
responsibilities: release credential smoke verification, regular release
delivery, main-hotfix delivery, and hotfix propagation delivery verification.
That shared container obscures approval intent, widens the visible credential
surface, and prevents lane-specific audit, rotation, revocation, and
decommissioning.

GOV-50 already separates protected-line request authorization, protected-line
execution authorization, and the automatic read-only finalizer. The remaining
release and hotfix controllers require the same functional separation.

## Decision

Every active release or hotfix controller belongs to exactly one functional
lane:

```text
release-request
  -> authorize scope, version, source SHA, target ref, expiry, and idempotency

release-execution
  -> approve exactly one request-bound protected-line mutation

release-credential-verification
  -> verify the private release credential issuer without mutation

release-delivery
  -> immutable regular-release tag and artifact delivery

release-reconciliation
  -> publish provenance-validated chore/* reconciliation candidates

hotfix-delivery
  -> immutable main or support patch tag and delivery verification

hotfix-propagation
  -> publish provenance-validated fix/* propagation candidates
```

The automatic protected-line finalizer remains read-only and has no
reviewer-gated environment.

Request and execution use only their job-scoped platform tokens. A lane that
needs a private credential issuer receives only its own OIDC/WIF provider,
invocation identity, runtime identity, secret binding, and non-secret
environment variables. A controller never receives another lane's broker
variables or publisher credentials.

The generic `release` environment, its variables, and generic
federation/invocation bindings are not a compatibility layer. They must be
removed after every active workflow uses its assigned lane and the required
end-to-end evidence is complete.

## Naming convention

GitHub Environment names are the canonical human authorization-lane names:

```text
release-credential-verification
release-delivery
hotfix-delivery
hotfix-propagation
```

Google Cloud resources must share the same functional lane root, but must not
reuse the Environment name as an undifferentiated resource name. Their names
identify the resource role and platform scope:

```text
release-credential-verification
  -> GitHub Environment
release-credential-verification-broker
  -> private Cloud Run credential issuer
release-credential-verification-invoker
  -> invocation service account
github-release-credential-verification
  -> workload identity provider
```

This is semantic equivalence, not literal equality. The Environment expresses
an approval and governance boundary; Cloud resource names must additionally
express their executable resource type, identity role, and collision domain.
Variables use the same lane root in uppercase, for example
`GCP_RELEASE_CREDENTIAL_VERIFICATION_BROKER_URL`.

## Consequences

- Approval decisions are attributable to one concrete controller purpose.
- Credential issuer invocation is least-privilege and lane-scoped.
- Regular release delivery and hotfix delivery remain separate even when they
  dispatch the same immutable artifact workflow.
- Reconciliation and hotfix propagation retain their dedicated publisher
  identities and never receive shared release-delivery credentials.
- Documentation and workflow-contract tests must reject any active
  `environment: release` reference.
- GitHub environments and cloud identities require manual, lane-specific
  provisioning before the source contract is production-ready.
- Promotion, delivery, reconciliation, and cleanup remain independent
  lifecycle decisions; this ADR does not add an automatic merge path.
