# Release and support workflows
## Release cut

```powershell
git governance --interactive never --output json --yes `
  --pull-request-provider github workflow release cut `
  --version 2.8.0 `
  --dispatch
```

The CLI validates the requested `release/2.8.0` line and emits an intent for
the protected `create-protected-line.yml` GitHub Actions workflow. With
`--dispatch`, the configured GitHub lifecycle adapter starts that workflow,
waits for its successful correlated run, fetches the remote, and verifies that
`origin/release/2.8.0` exists. The CLI never creates, switches to, or pushes a
shared `release/*` branch itself.

A successful dispatch response is only transport acknowledgement. The adapter
accepts a successful HTTP `2xx` dispatch response, then requires the
correlated workflow run to succeed and the requested remote line to exist
before it reports the release cut as complete.

Without `--dispatch`, `cut` remains an intent-only plan. It is useful for
review or a manually operated release process, but it does not prove that a
release line exists and cannot advance the governed release lifecycle.

## Managed broker release control

The repository-local `release-control.yml` workflow is the only supported
GitHub Actions entrypoint for a managed credential broker. It runs in the
protected `release` environment and uses GitHub OIDC plus Google Workload
Identity Federation to obtain a short-lived Cloud Run ID token. The workflow
passes that token only in process memory to `git-governance`; it never stores a
GitHub App key, an installation token, or a Google service-account key in
GitHub.

Configure these GitHub repository variables:

```text
GCP_BROKER_URL
GCP_BROKER_WIF_PROVIDER
GCP_BROKER_INVOKER_SERVICE_ACCOUNT
```

First dispatch `broker-smoke`. It proves that the broker accepts the approved
`CyberT33N/git-governance` repository request and rejects an unapproved request
without printing the returned installation token. Only after that smoke test
passes may a release owner dispatch `release-cut` with a concrete SemVer value.

## Stabilization

Only release-blocking fixes, final documentation, and release preparation are
allowed after a cut. Create the corresponding short-lived stabilization branch:

```powershell
git governance --yes workflow release stabilize `
  --release release/2.8.0 `
  --kind blocker `
  --key ABC `
  --ticket 999 `
  --slug release-blocker-timeout
```

Publish its pull-request intent back to the frozen release line:

```powershell
git governance --yes workflow release publish-stabilization `
  --release release/2.8.0 `
  --push
```

For non-interactive execution, use `--interactive never --output json --yes`.
After resolving and staging a paused rebase, add `--resume` to
`workflow release publish-stabilization`; the original `--release` value is
still required.

## Promotion-base alignment

When a release-to-main PR is structurally out of date and the `main` ruleset
requires current status checks, do not use GitHub's **Update branch**, rebase
the frozen release line, or weaken the main ruleset. Create a `release-prep`
stabilization branch from the frozen line, then run the dedicated alignment:

```powershell
git governance --interactive never --output json --yes `
  --pull-request-provider github workflow release align-promotion-base `
  --release release/2.8.0 `
  --push `
  --create-pull-request
```

The command accepts only the checked-out `chore/*` release-preparation branch
whose stored workflow base is the supplied `release/<semver>` line. It merges
the current `origin/main` into that working branch with a ticket-scoped
governed merge commit, runs the configured quality suite, and opens a PR back
to the frozen release line. Merge that stabilization PR with a merge commit;
then rerun the existing release-to-main PR checks and approval. This preserves
the original promotion PR and never mutates a shared line directly.

If the controlled merge conflicts, it remains fail-closed in the non-shared
preparation branch. Resolve only the exact conflicted paths and stage them
explicitly, then continue through the CLI:

```powershell
git governance --interactive never --output json --yes `
  --pull-request-provider github workflow release align-promotion-base `
  --release release/2.8.0 `
  --resume `
  --push `
  --create-pull-request
```

The resume path fetches `origin/main`, requires the active merge target to
match that current revision, continues Git's existing ticket-scoped merge, and
then rechecks that Main did not advance before quality, push, or PR
publication. A raw `git merge --continue`, GitHub **Update branch**, rebase,
or force push is not a substitute.

## Promotion, delivery, and conditional backmerge

After approval, create the release promotion:

```powershell
git governance workflow release promote --release release/2.8.0
```

The default output is a provider-neutral intent. To create the main pull
request through GitHub, use explicit provider configuration and confirmation:

```powershell
git governance --interactive never --output json --yes `
  --pull-request-provider github workflow release promote `
  --release release/2.8.0 `
  --create-pull-request
```

Do not invoke `backmerge` merely because the promotion PR exists. It is
permitted only after the promotion merged, the immutable `v2.8.0` tag points
to that merge commit, and the required release artifacts and GitHub Release
were published successfully.

```powershell
git governance --interactive never --output json --yes `
  --pull-request-provider github workflow release backmerge `
  --release release/2.8.0 `
  --create-pull-request
```

The command verifies those delivery facts with GitHub and compares
`release/2.8.0` with `develop`:

- `status=required`: it creates or returns the reviewed
  `release/2.8.0 -> develop` PR.
- `status=not-required`: no effective release-only delta remains, so it
  creates no empty PR. Record this result before release-branch cleanup.

When Develop requires a current pull-request head, do not update the delivered
release line. First create a `release-prep` stabilization branch from the
release line, then align that branch with Develop:

```powershell
git governance --interactive never --output json --yes `
  workflow release stabilize `
  --release release/2.8.0 `
  --kind release-prep `
  --key ABC `
  --ticket 999 `
  --slug align-release-reconciliation-base
```

```powershell
git governance --interactive never --output json --yes `
  workflow release align-reconciliation-base `
  --release release/2.8.0 `
  --push `
  --create-pull-request
```

The command accepts only the checked-out, ticket-bound `chore/*` preparation
branch created from the stated release line. It verifies delivery and an
effective delta, merges current Develop into the preparation branch, runs
quality gates, and opens a merge-commit PR from that branch to Develop. It
never updates `release/2.8.0`.

See [release reconciliation](release-reconciliation.md) for the complete
state and evidence contract.

Use a least-privileged release-automation identity for protected-line dispatch
and delivery verification. A managed credential broker is the preferred
non-interactive mechanism; publication and dispatch never start login
implicitly. See [GitHub App authentication](../authentication.md).

After the protected `release/<semver> -> main` pull request merges, GitHub
Actions creates the immutable annotated `v<semver>` tag at that exact merge
commit. The tag workflow then dispatches the artifact workflow for that tag.
The local CLI never tags or pushes `main`.

## Support line
```powershell
git governance --yes --pull-request-provider github workflow release support `
  --version 2.8 `
  --dispatch
```

The command requires a matching `v2.8.<patch>` release tag on `origin/main`
and dispatches the same protected-line workflow. The privileged workflow
creates and the CLI verifies the remote support line from the tagged
`origin/main` revision; support lines cannot be created from an untagged
integration state.
