# Automate release workflows

Use the same provider-neutral defaults in automation. `--interactive never`
requires every mandatory value as a flag; `--yes` authorizes only the bounded
operation exposed by that command.

```powershell
git governance --interactive never --output json --yes `
  workflow release cut `
  --version 2.8.0

git governance --interactive never --output json --yes workflow release stabilize `
  --release release/2.8.0 `
  --kind blocker `
  --key ABC `
  --ticket 999 `
  --slug release-blocker-timeout

git governance --interactive never --output json --yes workflow release publish-stabilization `
  --release release/2.8.0 `
  --push

git governance --interactive never --output json --yes `
  --pull-request-provider github workflow release promote `
  --release release/2.8.0 `
  --create-pull-request

git governance --interactive never --output json --yes `
  --pull-request-provider github workflow release backmerge `
  --release release/2.8.0 `
  --create-pull-request

git governance --interactive never --output json --yes `
  workflow release support `
  --version 2.8
```

For an actual GitHub PR during stabilization, promotion, or a required
backmerge, add `--create-pull-request`. Stabilization also requires `--push`;
promotion and backmerge do not.

## Protected-line request, execution, and finalization

`workflow release cut` and `workflow release support` prepare only a local
intent. A normal local `--dispatch` is rejected: the protected executor must
never receive an unbound version or branch input.

The normal server-side path is:

```text
release-control.yml / release-request
→ release-request environment approval
→ immutable GitHub Deployment request record
→ request_id dispatch to create-protected-line.yml
→ release-execution environment approval
→ exactly one bound ref mutation
→ automatic read-only finalizer
→ verified | failed | verification_pending
```

The request binds the governed ticket, operation, version, source line and
exact source SHA, target ref, requester, controller run, executor workflow,
expiry, and idempotency key. The request controller has no `contents: write`
permission; the executor has no authority beyond its one bound mutation; the
finalizer is read-only with respect to Git refs.

Only `verified` represents a complete protected release or support-line cut.
The automatic finalizer validates the correlated executor job and the actual
remote ref before writing that status. It does not promote, tag, deliver, or
reconcile the line.

`recover-protected-line-request.yml` is a read-only recovery path for a
record already in `verification_pending`. It cannot dispatch another executor
or mutate a ref. It is not the normal human verification step.

The request and execution jobs are separate from the non-mutating
`release-credential-verification` lane. Broker smoke consumes only the
verification lane's OIDC/WIF variables; request and execution do not receive
external credential-issuer variables.

Backmerge must run only after the main promotion, exact immutable tag, and
release artifact delivery are complete. It returns `not-required` instead of
creating an empty PR when no effective release-only delta remains. Automation
must use a managed credential broker or another least-privileged release
identity; it never starts a browser login. See
[GitHub App authentication](../authentication.md).

## Main-hotfix patch delivery

`hotfix-delivery.yml` is a separate main-bound controller for a merged
same-repository `hotfix/* -> main` pull request. It validates the reviewed
ticket record and ordered manifest through the `hotfix-delivery` lane before
creating an immutable patch tag. It then dispatches `release.yml` for that tag
and waits for the artifact workflow and published evidence.

The lane consumes only its own OIDC/WIF invocation variables. It does not
publish a propagation candidate, mutate `develop`, or grant local credentials
any release authority.

## Main-hotfix propagation

`hotfix-propagation.yml` is a separate, main-bound controller for one declared
additional target of a delivered main hotfix. It uses the protected
`hotfix-delivery` lane to recheck delivery, then the protected
`hotfix-propagation` lane, its own OIDC/WIF path, and a separate
least-privilege Publisher broker to publish the candidate:

```text
workflow hotfix propagate-manifest --publish
```

The source CLI prepares the ordered `cherry-pick -x` candidate, runs its
quality gates, pushes only the non-shared `fix/*` branch, and creates one PR
for the declared target. The controller does not merge that PR, mutate a
Shared Line directly, or reuse the release-automation or reconciliation
publisher identity.
