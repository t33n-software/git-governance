# Propagate a hotfix

Forward-port or backport one reviewed hotfix commit. The command creates a
short-lived `fix/*` branch from the requested target line, applies
`git cherry-pick -x`, validates it, and emits a pull-request intent:

```powershell
git governance --yes workflow hotfix propagate `
  --target-line develop `
  --commit 0123456789abcdef0123456789abcdef01234567 `
  --push
```

Non-interactive automation uses the same explicit inputs:

```powershell
git governance --interactive never --output json --yes workflow hotfix propagate `
  --source hotfix/ABC-999-payment-timeout `
  --target-line develop `
  --commit 0123456789abcdef0123456789abcdef01234567 `
  --push
```

Add `--pull-request-provider github --create-pull-request` to publish the
resulting PR after its branch is pushed.

If a cherry-pick pauses for conflicts, resolve and stage it, then resume the
already-created propagation branch:

```powershell
git governance --interactive never --output json --yes workflow hotfix propagate `
  --source hotfix/ABC-999-payment-timeout `
  --target-line develop `
  --branch fix/ABC-999-forward-port-payment-timeout `
  --resume `
  --push
```

## Reviewed multi-commit manifest

`workflow hotfix propagate` remains the reviewed one-commit primitive. A main
hotfix record can additionally declare an ordered full-SHA manifest and target
lines. `workflow hotfix propagate-manifest` creates and validates one local
candidate from a declared target without pushing it or creating a pull request:

```powershell
git governance --interactive never --output json --yes workflow hotfix propagate-manifest `
  --source hotfix/ABC-999-payment-timeout `
  --target-line develop
```

The command reads the ticket-bound record under
`.git-governance/hotfix-release-records/`, creates a workflow-managed `fix/*`
candidate from the target line, applies the manifest with ordered
`cherry-pick -x` operations, and runs the repository quality suite.

If one manifest commit conflicts, it leaves the candidate non-shared and
fail-closed. Resolve and explicitly stage only the conflicted paths, then
continue the original candidate:

```powershell
git governance --interactive never --output json --yes workflow hotfix propagate-manifest `
  --source hotfix/ABC-999-payment-timeout `
  --target-line develop `
  --branch fix/ABC-999-propagate-to-develop `
  --resume
```

This command deliberately has no generic `--push` or
`--create-pull-request` option. Its `--publish` mode is accepted only when the
protected Hotfix-Propagation-Publisher control plane supplies its dedicated
broker-backed workload identity:

```text
hotfix-propagation.yml on main
→ hotfix-propagation environment
→ OIDC and Workload Identity Federation
→ dedicated Hotfix-Propagation-Publisher broker and GitHub App
→ workflow hotfix verify-delivery
→ workflow hotfix propagate-manifest --publish
→ provenance-validated fix/* candidate and pull request
```

The controller creates the candidate itself, validates the delivered source
record, keeps the target branch non-shared, and uses the source CLI for every
cherry-pick, quality, push, and pull-request step. It never performs a raw
`git cherry-pick`, a direct Shared-Line mutation, or a local Device-Flow
fallback.

Until the dedicated GitHub App, Secret Manager key, Cloud Run broker, OIDC/WIF,
GitHub Environment, and least-privilege IAM boundaries are provisioned,
`--publish` fails closed. Local invocations remain candidate preparation or
resume only.
