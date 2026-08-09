# Automate release workflows

Use the same provider-neutral defaults in automation. `--interactive never`
requires every mandatory value as a flag; `--yes` authorizes the bounded
mutation or protected-line request.

```powershell
git governance --interactive never --output json --yes `
  --pull-request-provider github workflow release cut `
  --version 2.8.0 `
  --dispatch

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
  --pull-request-provider github workflow release support `
  --version 2.8 `
  --dispatch
```

For an actual GitHub PR during stabilization, promotion, or a required
backmerge, add `--create-pull-request`. Stabilization also requires `--push`;
promotion and backmerge do not. `cut --dispatch` and `support --dispatch`
wait for protected-line creation and verify the resulting remote line.

When `release-control.yml` dispatches `release-cut`, it uses a bounded
deferred dispatch so an independently approved child workflow cannot outlive
the parent wait. The controller records a non-secret correlation ID, exits
after dispatch acceptance, and requires a later `release-cut-verify` operation
with that same ID. The verification operation is the only step that confirms
the child succeeded and the remote protected line exists; it does not dispatch
a second child workflow.

Backmerge must run only after the main promotion, exact immutable tag, and
release artifact delivery are complete. It returns `not-required` instead of
creating an empty PR when no effective release-only delta remains. Automation
must use a managed credential broker or another least-privileged release
identity; it never starts a browser login. See
[GitHub App authentication](../authentication.md).

## Main-hotfix patch delivery

`hotfix-delivery.yml` is a separate main-bound controller for a merged
same-repository `hotfix/* -> main` pull request. It validates the reviewed
ticket record and ordered manifest through the managed release broker before
creating an immutable patch tag. It then dispatches `release.yml` for that tag
and waits for the artifact workflow and published evidence.

The controller may use the release-automation boundary only for tag and
artifact delivery. It does not publish a propagation candidate, mutate
`develop`, or grant local credentials any release authority.

## Main-hotfix propagation

`hotfix-propagation.yml` is a separate, main-bound controller for one declared
additional target of a delivered main hotfix. It uses the protected
`hotfix-propagation` environment, its own OIDC/WIF path, and a separate
least-privilege Publisher broker. The controller independently rechecks the
delivery record before invoking:

```text
workflow hotfix propagate-manifest --publish
```

The source CLI prepares the ordered `cherry-pick -x` candidate, runs its
quality gates, pushes only the non-shared `fix/*` branch, and creates one PR
for the declared target. The controller does not merge that PR, mutate a
Shared Line directly, or reuse the release-automation or reconciliation
publisher identity.
