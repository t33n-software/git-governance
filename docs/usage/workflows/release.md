# Release and support workflows
## Release cut

```powershell
git governance --interactive never --output json --yes `
  workflow release cut `
  --version 2.8.0
```

The CLI validates the requested `release/2.8.0` line and emits an intent for
the protected request controller. It never creates, switches to, or pushes a
shared `release/*` branch itself.

`cut --dispatch` is deliberately rejected outside a dry run. A normal local
CLI invocation cannot dispatch the protected executor directly because that
would omit the durable, authorized release-request record.

The normal path is `release-control.yml` on `main` with
`operation=release-request`, `kind`, `version`, `ticket_key`, and `ticket`.
The request job runs in `release-request`, persists a durable request record,
then dispatches the bound executor. It finishes at
`awaiting_execution_approval`; it does not poll the executor and does not
claim that a release line exists.

## Functional release-control lanes

The repository does not use one generic `release` environment as a shared
approval or credential container. Each active controller belongs to one
functional lane:

| Lane | Controller responsibility | Human approval / credential boundary |
| --- | --- | --- |
| `release-request` | Authorize ticket, version, source SHA, target ref, expiry, and idempotency. | Request Authority; no external broker variables. |
| `release-execution` | Approve and perform at most one request-bound protected ref mutation. | Execution Authority; no external broker variables. |
| `release-credential-verification` | Prove the private release credential issuer accepts the approved repository and rejects an unapproved one. | Dedicated OIDC/WIF invocation of the release credential issuer. |
| `release-delivery` | Create the immutable regular-release tag and build, sign, attest, and publish its artifacts. | Delivery approval for controlled `main` source and the immutable tag namespace. |
| `release-reconciliation` | Publish only a provenance-validated `chore/*` reconciliation candidate and its PR. | Dedicated reconciliation-publisher App and broker. |
| `hotfix-delivery` | Create and verify a delivered main or support patch tag before artifact dispatch. | Dedicated OIDC/WIF invocation of the hotfix-delivery credential issuer. |
| `hotfix-propagation` | Publish only a provenance-validated `fix/*` propagation candidate and its PR. | Dedicated hotfix-propagation-publisher App and broker. |

`release-control.yml` uses `release-credential-verification` only for
`broker-smoke`. Configure only this lane's variables there:

```text
GCP_RELEASE_CREDENTIAL_VERIFICATION_BROKER_URL
GCP_RELEASE_CREDENTIAL_VERIFICATION_WIF_PROVIDER
GCP_RELEASE_CREDENTIAL_VERIFICATION_INVOKER_SERVICE_ACCOUNT
```

The request controller does not receive those variables. It uses only its
job-scoped GitHub Actions token with `actions: write`, `deployments: write`,
and `contents: read`; it has no `contents: write` capability.

Reconciliation publication uses the separately protected
`release-reconciliation` environment and its dedicated publisher identity:

```text
GCP_RECONCILIATION_PUBLISHER_BROKER_URL
GCP_RECONCILIATION_PUBLISHER_WIF_PROVIDER
GCP_RECONCILIATION_PUBLISHER_INVOKER_SERVICE_ACCOUNT
```

The reconciliation publisher App is limited to repository contents and pull
request publication for a provenance-validated `chore/*` candidate. It has no
Ruleset bypass, release-line dispatch, workflow-write, administration, or
shared-line mutation role.

The regular tag controller and `publish-release-artifacts.yml` run in
`release-delivery`.
`hotfix-delivery.yml` and the verification job of `hotfix-propagation.yml`
run in `hotfix-delivery` and consume only:

```text
GCP_HOTFIX_DELIVERY_BROKER_URL
GCP_HOTFIX_DELIVERY_WIF_PROVIDER
GCP_HOTFIX_DELIVERY_INVOKER_SERVICE_ACCOUNT
```

The propagation-publish job remains in `hotfix-propagation` and consumes only
its dedicated `GCP_HOTFIX_PROPAGATION_PUBLISHER_*` variables. Do not copy
broker variables, tokens, PEM values, or authorization headers between lanes.

First dispatch `broker-smoke`. It proves that the broker accepts the approved
`t33n-software/git-governance` repository request and rejects an unapproved request
without printing the returned installation token. A release owner then
authorizes a bound `release-request`; the separately approved
`release-execution` job performs at most one protected ref mutation.

`execute-protected-line-request.yml` receives only `request_id`. It loads and validates
the durable record, including ticket, operation, source SHA, target ref,
expiry, idempotency key, and expected executor. Its automatic, non-human
finalizer independently checks the executor job and the real remote ref, then
writes `verified`, `failed`, or `verification_pending`. Only `verified`
completes a release cut.

The read-only recovery path for an existing `verification_pending` record is a
bound mode of the executor: dispatch `execute-protected-line-request.yml` with
the `recovery` input set. It cannot dispatch an executor, push a ref, promote
a release, tag, publish artifacts, or reconcile branches.

### Required GitHub environments

Before production use, configure the functional lanes named above. Each lane
must have an explicit branch or tag policy, the required reviewer role,
administrator bypass disabled, and only its own non-secret variables. At
minimum:

```text
release-request
→ main
→ Release Request Authority

release-execution
→ main
→ Release Execution Authority

release-credential-verification
→ main
→ non-mutating release credential verification

release-delivery
→ main and the controlled immutable tag namespace

hotfix-delivery
→ main
→ main or support patch tag and delivery

hotfix-propagation
→ main
→ provenance-validated fix/* candidate publication

release-reconciliation
→ main
→ provenance-validated chore/* candidate publication
```

The normal finalizer intentionally has no reviewer environment because it is
read-only technical evidence rather than a third human approval. The request
and execution authorities must be distinct. If a single-person exception is
unavoidable, it must be explicitly recorded as a reduced separation-of-duties
exception; it is not an implicit fallback.

## Managed reconciliation control

Delivery-gated reconciliation uses the separate `release-reconciliation`
environment. It does not reuse the release-cut broker identity. The protected
controller consumes only these environment variables:

```text
GCP_RECONCILIATION_PUBLISHER_BROKER_URL
GCP_RECONCILIATION_PUBLISHER_WIF_PROVIDER
GCP_RECONCILIATION_PUBLISHER_INVOKER_SERVICE_ACCOUNT
```

The controller obtains an ephemeral OIDC audience token, asks the dedicated
publisher broker for a repository-bound installation token without printing it,
and only then creates or validates the non-shared `chore/*` preparation
candidate. It never mutates `release/<semver>` or `develop` directly.

For a delivered release whose Backmerge target requires a current PR head,
dispatch `reconciliation-align` from `release-reconciliation.yml` on `main`
with `release`, `ticket_key`, `ticket`, and `slug`. The workflow builds the
trusted control-plane binary before it switches the repository to the
release-derived preparation branch. It obtains a masked, short-lived installation token for the
ephemeral Git transport, removes that transport configuration before job exit,
and uses the binary to create, align, validate, push, and publish the reviewed
Preparation-Branch PR to `develop`.

The target architecture dispatches this operation automatically after the
release artifact workflow has confirmed the immutable tag, GitHub Release,
artifacts, signatures, SBOMs, and attestations. The controller then verifies
those facts again and either creates the reviewed PR or records
`not-required`. Manual `workflow_dispatch` remains an incident, retry, and
recovery fallback; it is not the normal delivery path.

If the controlled Develop merge conflicts, the controller stops before pushing
an unresolved branch or creating a PR. Resolve the conflict in a non-shared,
ticket-bound Preparation-Branch with
`workflow release align-reconciliation-base --resume --push`. Then dispatch
`reconciliation-resume` on `main` with `resolution_branch`; CI independently
verifies the exact release/develop merge-parent topology before it publishes
the candidate and Develop PR through the dedicated publisher broker. Do not
pass an arbitrary branch, use a local Device Flow session to create the PR,
update `release/*`, or select `ours`/`theirs` globally. See
[release reconciliation](release-reconciliation.md).

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

Adding `--dry-run` to promotion, backmerge, or any release workflow is strictly
read-only: it returns a plan but never pushes, invokes a provider preflight, or
creates a pull request.

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

For the protected production path, dispatch `reconciliation-align` from
`release-reconciliation.yml` on `main`. The workflow builds the trusted
control-plane binary before it creates the release-derived preparation branch,
then aligns that branch and opens the reviewed PR to Develop. Do not use a
local Device Flow session, GitHub **Update branch**, or a direct
Develop-to-Release merge as a substitute.

See [release reconciliation](release-reconciliation.md) for the complete
state and evidence contract.

Use the separate least-privilege controller identities for request,
execution, and finalization. The executor has only the ref-mutation
permission needed for its bound request; the finalizer has no ref-write
permission. Neither path starts a browser login. See
[GitHub App authentication](../authentication.md).

After the protected `release/<semver> -> main` pull request merges, GitHub
Actions creates the immutable annotated `v<semver>` tag at that exact merge
commit. The tag workflow then dispatches the artifact workflow for that tag.
The local CLI never tags or pushes `main`.

## Support line
```powershell
git governance --yes workflow release support `
  --version 2.8
```

The command requires a matching `v2.8.<patch>` release tag on `origin/main`
and emits an intent. A governed `release-request` with `kind=support` binds
the tagged `main` source SHA and `support/<major.minor>` target before the
separately authorized executor can create the remote support line.
