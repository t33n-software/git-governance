# Hosting platform: GitHub — workflows
[INTENT: REFERENZ]

## Canonical source

The release and hotfix lifecycle workflow family for the `t33n-software`
organization is defined and managed once, centrally, in this repository. This
repository is the canonical source of truth for the family: it explains the
architecture, carries the payloads, and delivers the versioned, referenceable
artifacts.

A local copy, redefinition, or deviation in another repository is an
anti-pattern and forbidden (the redundancy and drift prohibition). A tenant
adopts the family exclusively through the thin, hash-verified callers.

## Artifact storage

- The **payloads** reside at the platform-enforced execution location
  [`.github/workflows/`](../../../../../.github/workflows/) and carry the
  `reusable-` prefix: `reusable-<capability>.yml`. GitHub reads reusable
  workflows exclusively from this location; the storage is not a choice but a
  platform constraint.
- The **canonical caller masters** and the **family work contract** reside in
  the family tree
  [`workflows/github/`](../../../../../workflows/github/CONTRACT.md) — the
  family is the purpose-named root; the platform (`github/`) sits one level
  below, mirrored to `rulesets/github/` and `properties/github/`.

Rationale for the axis form: this repository is the domain home of the Git
lifecycle; its root maps the primary artifact levels (`rulesets/`,
`properties/`, `workflows/`, the CLI). The hosting platform is the scaling
axis one level below, never a grouping parent above it.

## Naming convention

- **Payloads:** `reusable-<capability>.yml`, exclusively
  `on: workflow_call`, never self-triggering. The prefix encodes the role and
  prevents the name collision with the caller in the same directory.
- **Callers:** the caller file keeps its canonical identity
  (`release-control.yml`, `execute-protected-line-request.yml`,
  `tag-promoted-release.yml`, `publish-release-artifacts.yml`,
  `release-reconciliation.yml`, `hotfix-delivery.yml`,
  `hotfix-propagation.yml`) — workflow identity is contract, because the CLI
  and the fleet pins bind to it.
- **Check context:** a reusable call emits the context as the composite
  `<caller job name> / <callee job name>`; the caller job carries the lane
  identity, the callee job carries the gate identity.
- **Composite actions:** `.github/actions/<verb>-<object>/action.yml`.

## Convention documents

- [Provider error diagnostics](provider-error-diagnostics.md) — how the
  lifecycle lanes surface the provider's error diagnostic without redaction.
- [Environment-gate trigger context](environment-gate-trigger-context.md) —
  an environment-gated job never runs on a `pull_request` run; event-driven
  lanes detect on the pull request and execute on the main-bound dispatch.
- [Canonical copies, never symlinks](canonical-copies-not-symlinks.md) — why
  every execution location carries a byte-identical regular-file copy of the
  canonical caller and why a symbolic link cannot work.

## Management

- Management plane: the **organization** (`t33n-software`), never the
  individual repository level; the family is maintained once in this home and
  referenced by tenants via SHA pin.
- The delivery variant (`cloud` or `github-only`) is data — bound through the
  typed input, never through a forked payload.
- The seven workflow files of this repository under `.github/workflows/` are
  byte-identical to the canonical master callers (dogfooding); the contract
  test suite proves the identity on every change.
- Every deviation of a tenant from the canonical caller is a hash mismatch
  and fails closed.
