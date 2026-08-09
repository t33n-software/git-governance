# Hotfix workflows

## Start

```powershell
git governance --yes workflow hotfix start `
  --key ABC `
  --ticket 999 `
  --slug payment-timeout `
  --affected-line main
```

The affected line must be `main`, `release/<semver>`, or
`support/<major.minor>`. A hotfix never starts from `develop` by default.
The selected remote base is stored only in local Git metadata so later
publish, sync, and pre-push validation use the same affected line.

Non-interactive start:

```powershell
git governance --interactive never --output json --yes workflow hotfix start `
  --key ABC `
  --ticket 999 `
  --slug payment-timeout `
  --affected-line main
```

## Publish

Publish the hotfix to the same affected line:

```powershell
git governance --yes workflow hotfix publish `
  --affected-line main `
  --push
```

For automation:

```powershell
git governance --interactive never --output json --yes workflow hotfix publish `
  --affected-line main `
  --push
```

Add `--pull-request-provider github --create-pull-request` to create the
explicit provider-backed pull request after the push. The target remains the
specified affected line; it is never silently redirected to `develop`.
Authenticate before local publication with `auth login github`, or configure
the managed credential broker for automation. See
[GitHub App authentication](../authentication.md).

After resolving and staging a paused rebase, continue the same publication:

```powershell
git governance --interactive never --output json --yes workflow hotfix publish `
  --affected-line main `
  --resume `
  --push
```

## Main patch-delivery record

Before a `hotfix/* -> main` pull request is merged, the hotfix branch must
contain one reviewed record at:

```text
.git-governance/hotfix-release-records/<KEY-NUMBER>.json
```

The record is versioned with the hotfix and binds the incident, affected line,
previous immutable tag, patch target, reviewed source/target pull-request
identity, ordered full-SHA semantic manifest, optional four-commit exception,
and every declared additional propagation target.

```json
{
  "schemaVersion": 1,
  "ticket": "ABC-999",
  "incident": "INC-999",
  "affectedLine": "main",
  "targetVersion": "1.0.2",
  "previousTag": "v1.0.1",
  "expectedPullRequest": {
    "source": "hotfix/ABC-999-payment-timeout",
    "target": "main"
  },
  "manifest": [
    "0123456789abcdef0123456789abcdef01234567"
  ],
  "commitBudgetException": "",
  "scopeEscalationApproval": "",
  "propagationTargets": [
    "develop"
  ]
}
```

The normal semantic budget is one to three commits. Four or more commits need
a concise non-empty `commitBudgetException`. Five or more additionally require
a one-line `scopeEscalationApproval` reference documenting explicit release
approval before the Main merge.

Validate the record locally or in a CI candidate checkout:

```powershell
git governance --interactive never --output json workflow hotfix validate-record `
  --branch hotfix/ABC-999-payment-timeout
```

The Linux quality gate validates this record for same-repository hotfix PRs
that target `main`. It is therefore evaluated through the existing required
`Quality gates (linux-amd64)` check rather than through a bypassable local
convention.

## Main patch-delivery controller

After a same-repository hotfix PR is merged into `main`, the protected
[`hotfix-delivery.yml`](../../../.github/workflows/hotfix-delivery.yml)
controller independently verifies the record, merged pull request, ordered
manifest, and tag idempotency with the release broker identity. It then creates
the immutable patch tag, dispatches the existing artifact workflow, and waits
for the non-draft GitHub Release, checksums, payload, SBOM, Sigstore bundle,
and successful artifact workflow.

The read-only verification commands are available to the trusted controller and
for diagnosis:

```powershell
git governance --interactive never --output json --pull-request-provider github `
  workflow hotfix verify-merge `
  --branch hotfix/ABC-999-payment-timeout

git governance --interactive never --output json --pull-request-provider github `
  workflow hotfix verify-delivery `
  --branch hotfix/ABC-999-payment-timeout
```

They do not create a tag, push a branch, or publish a pull request. Local
device-flow credentials and static tokens are not a replacement for the
protected controller.

## Manifest propagation publisher

After the immutable main patch delivery is verified, each propagation target
declared in the reviewed record is handled by the separate
`hotfix-propagation.yml` controller. It builds trusted `main` source, verifies
the delivery again, creates the target-derived candidate with
`workflow hotfix propagate-manifest --publish`, and publishes only through the
dedicated Hotfix-Propagation-Publisher identity.

That identity is separate from release automation and reconciliation
publication. It may publish a provenance-validated `fix/*` candidate and its
reviewable pull request, but it has no Actions, Workflows, Administration,
Secrets, Ruleset-bypass, or direct Shared-Line authority. A local workflow
command cannot substitute a Device-Flow token, a static token, or a raw Git
push for this control plane.
