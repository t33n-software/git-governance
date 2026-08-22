# Product Acceptance Matrix

This matrix is the repository-local source of truth for delivery status. It
does not rely on any external governance repository or unpublished rule set.

## Status legend

- `IMPLEMENTED`: source code exists.
- `VERIFIED`: automated tests or an actual local execution succeeded.
- `IN_PROGRESS`: a confirmed gap is actively being remediated.
- `PENDING`: intentionally planned but not yet delivered.
- `BLOCKED`: cannot be verified because an external prerequisite is absent.

## Verified baseline

| Item | Status | Evidence |
|---|---|---|
| Local repository | VERIFIED | `main` and `origin/main` are initialized; every audit and release gate begins by checking the current Git status |
| Go toolchain | VERIFIED | Go 1.26.6, Windows amd64 |
| Git client | VERIFIED | Git 2.53.0.windows.2 |
| Legacy scripts and copied hooks | VERIFIED | not present in this repository |
| Go module | VERIFIED | `github.com/t33n-software/git-governance`, language Go 1.26 and pinned toolchain Go 1.26.6 |

## Core domain and validation

| Capability | Status | Verification |
|---|---|---|
| Typed errors, remediation, exit codes | VERIFIED | whitebox tests |
| Ticket key, number, and ID grammar | VERIFIED | table tests and fuzzing |
| Syntax-only key policy | VERIFIED | unit tests |
| All 13 branch families | VERIFIED | parser and catalog tests |
| Slug, release SemVer, and support version parsing | VERIFIED | table tests and fuzzing |
| Publication-state and rewrite policy | VERIFIED | application tests |
| Conventional Commit parser | VERIFIED | header, body, footer, breaking, revert, and fuzz tests |
| Ticket-to-branch commit consistency | VERIFIED | application tests |
| JSON problem contract | VERIFIED | adapter and CLI tests |
| Human problem contract includes a safe actual value | IMPLEMENTED | non-sensitive actual values are rendered; sensitive values remain redacted |

## Git behavior

| Capability | Status | Verification |
|---|---|---|
| Argument-array Git process execution | VERIFIED | whitebox process-contract tests |
| Bounded stdout and stderr capture | VERIFIED | adapter tests |
| Context and timeout propagation | VERIFIED | adapter tests |
| Branch creation from remote target bases | VERIFIED | real local Git integration test |
| One official regular branch per ticket | VERIFIED | local/remote branch discovery, whitebox test, and real-Git regression test |
| Explicit staging only | VERIFIED | application and Git adapter tests |
| Commit creation through stdin | VERIFIED | real local Git integration test |
| Fail-closed Git transport authentication diagnostic | VERIFIED | non-interactive dry-run push adapter and same-package whitebox tests |
| First-push publication detection | VERIFIED | real local Git integration test |
| Base delta, merge, and rebase paths | VERIFIED | real local Git integration test |
| Scratch-to-official squash transfer | VERIFIED | whitebox, CLI-contract, Git-adapter, and real local Git integration tests |
| Structured commit-family composition | VERIFIED | canonical commit application module and same-package whitebox tests |
| Rebase, merge, and scratch-squash continuation after conflict resolution | VERIFIED | synchronizer, scratch merger, workflow, CLI interaction, and Git adapter whitebox tests |
| No automatic amend or force push | VERIFIED | absent from public command tree and application APIs |

## User-facing commands

| Command area | Status | Notes |
|---|---|---|
| `branch list`, `validate`, `create`, `merge-scratch`, `sync-base` | IMPLEMENTED | CLI contract tests cover help, JSON, flags, dry-run behavior, structured commit composition, and the governed `sync-base --resume` continuation of conflicted rebase and merge synchronizations |
| `commit create`, `validate` | IMPLEMENTED | explicit staging, branch-derived ticket context, and canonical family selection are enforced |
| `workflow ticket start` | IMPLEMENTED | optional scratch branch and provider-neutral PR intent |
| `workflow ticket publish` | IMPLEMENTED | reports conditional rebase state, runs final local quality only after synchronization, records revision-bound local Git metadata, resumes resolved rebase and scratch-transfer conflicts interactively or with `--resume`, and creates a PR only through an explicit configured provider |
| `workflow hotfix start` | IMPLEMENTED | affected-line selection is mandatory |
| hotfix publish and single-commit propagation | IMPLEMENTED | affected-line publish plus reviewed `cherry-pick -x` forward/backport workflow, including non-interactive `--resume` continuation |
| main hotfix release record and delivery verification | IMPLEMENTED / PROVISIONING REQUIRED | schema-validated ticket record, semantic commit budget, GraphQL merge evidence, immutable tag/release evidence verification, and protected `hotfix-delivery` controller exist; the lane-specific credential issuer and environment configuration remain external prerequisites |
| manifest hotfix propagation preparation | IMPLEMENTED | declared multi-commit SHA manifest creates and verifies a local resumable `fix/*` candidate without publication |
| manifest hotfix candidate publication | IMPLEMENTED / PROVISIONING REQUIRED | protected `hotfix-propagation.yml` controller, controller-only `--publish`, masked ephemeral transport, and dedicated `hotfix-propagation` broker contract exist; App, Secret, Cloud Run, OIDC/WIF, IAM, Environment, and Artifact Registry resources remain external prerequisites |
| `workflow release cut`, `request`, `execute-request`, `finalize-request`, `stabilize`, `align-promotion-base`, `align-reconciliation-base`, `promote`, `backmerge`, `support`, `cleanup` | IMPLEMENTED / PROVISIONING REQUIRED | local cut/support commands remain intent-only; the protected request controller persists a ticket-, SHA-, target-, expiry- and idempotency-bound record, the execution workflow receives only `request_id`, and an automatic read-only finalizer writes the authoritative completion state. The functional `release-request`, `release-execution`, and `release-credential-verification` lane configuration remains external. |
| GitHub App pull-request adapter | IMPLEMENTED | just-in-time App credential resolution, host/repository isolation, bounded REST responses, and idempotent existing-PR lookup |
| `auth login/status/logout github` | IMPLEMENTED | explicit Device Flow, redacted status, canonical native secret-store lifecycle scoped by host, account, and configured GitHub App client ID, repository-identity binding with capability discovery and stale-binding rebinding, legacy-storage rejection, and CLI contract tests |
| `validate pre-push` | IMPLEMENTED | parses every Git stdin ref update, validates the actual remote target, and reuses final local quality evidence only when it exactly matches the outgoing candidate |
| `config key` | IMPLEMENTED | OS configuration directory, atomic JSON storage |
| `policy describe`, `completion`, `version` | IMPLEMENTED | policy and environment inspection are read-only |
| `doctor` | IMPLEMENTED | Git version, remote, fail-closed Git transport dry-run authentication, Lefthook, policy, configuration, and in-progress-operation checks |
| Interactive Huh forms and accessible prompts | IMPLEMENTED | tested with accessible form input |
| Interactive field validation retries | VERIFIED | invalid ticket, slug, commit-subject, and breaking-change values show field diagnostics and retry in place |
| Workflow input failure summaries | VERIFIED | accepted command inputs accompany classified workflow and branch-creation failures |
| Git operation diagnostics | VERIFIED | operation context and bounded, credential-redacted Git diagnostics are rendered separately |
| Direct `git governance` invocation | IMPLEMENTED | available when `git-governance` is on `PATH` |

## Workflow policy

| Rule | Status | Behavior |
|---|---|---|
| Regular work starts from `origin/develop` | VERIFIED | direct remote base, no local `develop` checkout/pull required |
| Hotfix starts from actual affected line | VERIFIED | only `main`, `release/*`, or `support/*` accepted |
| Hotfix PR targets actual affected line | IMPLEMENTED | hotfix publish requires and uses the affected main/release/support line |
| Specialized workflow base metadata | VERIFIED | local Git metadata records hotfix, stabilization, and propagation bases for later sync and pre-push validation |
| First push checks basis freshness | VERIFIED | push is blocked when an unpublished branch misses base commits |
| Unpublished branch rebase | VERIFIED | only after a real base delta |
| Interactive and non-interactive conflict continuation | IMPLEMENTED | rebase, scratch-squash, and hotfix-propagation continuations remain explicit and can resume with `--resume` after manual conflict resolution |
| Published branch synchronization | VERIFIED | recommends or performs explicit merge, never routine rebase |
| Release promotion-base alignment | VERIFIED | a release-preparation branch with recorded release provenance alone may merge `origin/main`, resume an explicitly resolved exact-path conflict only against the current Main revision, run quality gates, and return through a PR to the frozen release line; ADR-0003 records the provider-neutral boundary and GitHub UI limitation |
| Trusted reconciliation control | IMPLEMENTED | protected Main workflow builds a trusted binary before release-derived branch checkout, uses ephemeral broker-backed transport, clears runner-local credentials on exit, and validates prepared conflict-recovery candidates before publication |
| Separate reconciliation publisher identity | IN PROGRESS | the protected `release-reconciliation` environment is wired for a dedicated broker-backed GitHub App that publishes validated candidates and Develop PRs without a Ruleset bypass |
| Functional release and hotfix control lanes | IMPLEMENTED / PROVISIONING REQUIRED | `release-control.yml`, `execute-protected-line-request.yml`, `tag-promoted-release.yml`, `publish-release-artifacts.yml`, `release-reconciliation.yml`, `hotfix-delivery.yml`, and `hotfix-propagation.yml` are thin, hash-verified callers of the centralized reusable payload family (`reusable-<capability>.yml`) owned by this repository; they map request, execution, credential verification, regular delivery, reconciliation, hotfix delivery, and hotfix propagation to separate lanes; GitHub environments and lane-specific cloud identities remain external prerequisites |
| Centralized release lifecycle family | IMPLEMENTED | the release and hotfix lifecycle exists exactly once as reusable payloads at `.github/workflows/reusable-<capability>.yml`; the canonical caller masters, the family contract, and the caller hash record live at `workflows/github/`; the read-only recovery finalization is a bound mode of the executor payload; the delivery variant binds as data (`cloud` or `github-only`); the delivery source verification runs the canonical quality gate as a pinned class-D tool, and tenant lanes build the governance CLI from their pinned tool module — no migration variable exists; the packaging contract tests prove the payload contract, the caller byte-identity, and the hash-record consistency; they additionally prove the `workflow_call` input-type boundary (only `string`, `number`, and `boolean`; never `choice` or `options`), the YAML well-formedness of every payload and caller master, and that the pinned home SHA is always reachable from the merged `origin/develop` line (never a pull-request-only commit) |
| Reconciliation conflict recovery | IMPLEMENTED | a resolved ticket-bound preparation merge can resume locally, while the protected Main controller validates exact release/develop merge-parent provenance before publishing the Develop PR |
| Automated delivery-to-reconciliation orchestration | PLANNED | successful delivery must dispatch the idempotent reconciliation controller; manual dispatch remains incident, retry, and recovery fallback |
| Scratch branch | VERIFIED | private local branch from the same-ticket official local branch; transfer resolves an existing local official target by ticket ID and squashes to one governed commit |
| Protected release- and support-line request lifecycle | IMPLEMENTED / PROVISIONING REQUIRED | request authorization, durable deployment-backed request record, bound execution, automatic ref/job finalization, and read-only recovery are covered by same-package adapter, workflow, CLI, and workflow-contract tests. The final `release-request` and `release-execution` environment configuration is external. |
| Protected-line request record anchoring | IMPLEMENTED | the durable deployment-backed request record anchors to the repository default branch rather than the source SHA, so record creation never depends on the source commit's branch relationship to the default branch; the authorized source revision remains bound immutably in the signed payload, which the executor and finalizer validate; same-package adapter whitebox tests pin the default-branch anchor and the payload authority |
| Delivery-gated release reconciliation | VERIFIED | GitHub verifies the merged promotion, exact tag, published release, and effective release-to-develop delta; backmerge is required only when that delta exists; adapter and workflow whitebox tests pass |
| Strict-base reconciliation alignment | IMPLEMENTED | a ticket-bound chore branch from release merges current Develop, validates the combined state, and prepares a Merge-Commit PR to Develop without modifying the delivered release ref |
| Release stabilization and completion | IMPLEMENTED | constrained stabilization, promotion intent, dispatch, delivery-gated conditional backmerge, cleanup, and support-tag provenance are present |

## Testing and quality

| Gate | Status | Latest local result |
|---|---|---|
| `go test ./...` | VERIFIED | passed after final remediation |
| `go run ./cmd/check-coverage` | VERIFIED | every Go package had a `_test.go` file; every package with executable statements reached 100.0 % |
| `go vet ./...` | VERIFIED | passed after final remediation |
| Domain whitebox coverage | VERIFIED | 100.0 % in every domain package |
| Git adapter whitebox coverage | VERIFIED | 100.0 % |
| Preferences whitebox coverage | VERIFIED | 100.0 % |
| Quality adapter whitebox coverage | VERIFIED | 100.0 % |
| Workflow whitebox coverage | VERIFIED | 100.0 % |
| Bootstrap CLI whitebox coverage | VERIFIED | 100.0 % |
| Terminal adapter whitebox coverage | VERIFIED | 100.0 % |
| Real Git integration | VERIFIED | passed against temporary local bare remotes |
| Bounded fuzzing | VERIFIED | ticket, branch, commit, and configuration targets passed |
| Race detection | VERIFIED | `CGO_ENABLED=1 go test -race ./...` passed locally with GCC 16.1.0 |
| Vulnerability scan | VERIFIED | `govulncheck` v1.5.0 reported no vulnerabilities |
| Windows amd64 native smoke | VERIFIED | version, policy, and branch-catalog commands passed; `doctor` is intentionally excluded because detached CI checkouts have no branch-bound Git credential |
| Windows/macOS/Linux cross-builds | VERIFIED | all six promised OS/architecture binaries compiled with `CGO_ENABLED=0` |
| Native primary-OS full-quality matrix | IMPLEMENTED | CI runs `cmd/build` natively on Linux, macOS, and Windows; each OS independently enforces lint, tests, uncached 100%-coverage, race, fuzz, and security gates |
| Native ARM64 smoke tests | IMPLEMENTED | CI matrix contains Ubuntu ARM64, Windows ARM64, and macOS ARM64 runners; remote execution requires the first push |
| macOS/Linux native smoke tests | IMPLEMENTED | CI executes native smoke tests; local Windows execution is intentionally not a prerequisite |
| Quality configuration schema v3 | VERIFIED | the quality runner strictly decodes schemaVersion 3 with the required pinned toolchain block and the optional project block (binary smoke contracts, fuzz budgets); schemaVersion 2 and unknown fields fail closed |

## Delivery and operations

| Capability | Status | Notes |
|---|---|---|
| Lefthook configuration | IMPLEMENTED | thin `commit-msg` and `pre-push` runners; no duplicated regex |
| Local Lefthook validation | VERIFIED | Lefthook v2.1.10 returned `All good` |
| Reproducible release configuration | VERIFIED | GoReleaser v2.16.0 installed locally and validated `.goreleaser.yaml` |
| Controlled Go execution | IMPLEMENTED | CI and release set `GOTOOLCHAIN=local`, `GOFLAGS=-mod=readonly`, and `GOVCS=*:off`, then verify Go 1.26.6 before running Go commands |
| Dependency admission review | IMPLEMENTED | immutable `actions/dependency-review-action` gate blocks dependency changes that introduce low-severity-or-higher findings across all dependency scopes |
| Dependency-review merge enforcement | VERIFIED | `Dependency admission review` is a required status check for `develop`, `main`, `release/*`, and `support/*`; the repository contract test binds the workflow name and every shared-line ruleset class variant |
| Periodic dependency re-evaluation | IMPLEMENTED | the CI workflow runs daily in addition to pull-request, push, and manual triggers |
| Dependency update intake | IMPLEMENTED | Dependabot opens daily reviewable update pull requests for the application module, the tools module, and GitHub Actions |
| Hosted runner major-version pinning | IMPLEMENTED | GitHub workflows use concrete Ubuntu and Windows runner labels rather than `*-latest` labels |
| GitHub Actions CI | IMPLEMENTED | immutable action commits, pinned tool versions, read-only module execution, native Linux/macOS/Windows full-quality gates, uncached coverage, race, fuzz, vulnerability, Lefthook, smoke, and release-config gates are configured |
| GitHub release artifacts | IMPLEMENTED | tag/manual-tag validation, checksums, SBOM, Cosign, provenance attestation, and Linux package formats are configured |
| CI-owned release tag lifecycle | IMPLEMENTED | merged same-repository `release/<semver> -> main` creates an immutable annotated tag and explicitly dispatches the artifact workflow because `GITHUB_TOKEN` tag pushes do not trigger `push` workflows |
| Live protected-line creation | VERIFIED | `release/1.0.0` was created by the protected workflow from the verified `develop` revision; the release Ruleset applies after creation |
| Live release delivery and reconciliation | VERIFIED | PR #30 promoted `release/1.0.1` to `main`; annotated `v1.0.1` targets `f8a54acc4e9f36af869f44737a527a03e5fbf2c5`; the artifact workflow and GitHub Release completed; PR #46 merged the controlled reconciliation to `develop` at `271557e7e66b79e54512e96d4e3a923277a21010` while the delivered release ref remained unchanged |
| Package-manager manifest templates | IMPLEMENTED | Homebrew, Scoop, and WinGet templates are version/checksum-driven under `packaging/` |
| Package-manager publication | BLOCKED | maintainer-controlled tap, bucket, WinGet submission, and publisher identities are external prerequisites |
| Platform-native signing and notarization | BLOCKED | Authenticode and Apple credentials are external publisher prerequisites; checksum Cosign signing remains configured |
| Internal Approved Proxy and registry admission | BLOCKED | intentionally deferred until the artifact-registry platform is provisioned; the repository does not change its current Go proxy configuration |
| Hermetic release build enclave | BLOCKED | requires the deferred Approved Proxy plus an immutable, pre-provisioned build image and network isolation outside repository configuration |
| Code owner review on shared lines | IMPLEMENTED / PROVISIONING REQUIRED | `.github/CODEOWNERS` binds the default owner and every shared-line ruleset class variant sets `require_code_owner_review: true`; importing the canonical organization rulesets remains an external prerequisite |

## Confirmed remediation work

| Gap | Target remediation | Status |
|---|---|---|
| Pre-push stdin discarded | parse and validate every outgoing update, including deletes and non-fast-forward updates | IMPLEMENTED |
| Hotfix publish target | retain the actual affected line and route the first PR to that line | IMPLEMENTED |
| Post-rebase verification | rerun branch, commit-series, policy, and configured quality validation | IMPLEMENTED |
| Release lifecycle | add stabilization, release-to-main intent, controlled propagation, and cleanup | IMPLEMENTED |
| Protected-line lifecycle | separate request authorization, one bound execution authorization, durable idempotent request state, automatic read-only finalizer, and recovery-only finalizer | IMPLEMENTED / PROVISIONING REQUIRED |
| Release reconciliation | require promotion/tag/delivery evidence, create a develop PR only for effective delta, and record `not-required` otherwise | IMPLEMENTED |
| Strict-base reconciliation alignment | retain the delivered release ref, merge Develop only into a ticket-bound preparation branch, then review a merge-commit PR to Develop | IMPLEMENTED |
| Tag-to-artifact trigger | explicitly dispatch the artifact workflow after a `GITHUB_TOKEN`-created immutable tag | IMPLEMENTED |
| Direct scratch selection | require/select an official ticket-branch base before creation | IMPLEMENTED |
| Application-level scratch base guard | reject remote-tracking scratch bases even for programmatic callers | IMPLEMENTED |
| Regular ticket exclusivity | reject a second official regular branch for one ticket after fetch | IMPLEMENTED |
| GitHub App session isolation | scope native refresh sessions, in-memory refresh state, authorization state, status, logout, and credential resolution by host, account, and configured client ID; reject legacy storage without selecting or migrating it | IMPLEMENTED |
| Project-agnostic quality gates | explicit repository-local command-array configuration; absent config reports `unconfigured` instead of pass | IMPLEMENTED |
| Final local publish quality | full suite runs after final synchronization; short-lived local Git metadata is reused only for an exact fresh candidate and otherwise falls back | IMPLEMENTED |

## Explicit non-goals in v1

- No live ticket-registry lookup.
- GitHub pull-request publication is supported only through the explicit GitHub adapter; other provider-specific APIs remain out of scope.
- No automatic self-update.
- No direct mutation of protected shared lines.
- No automatic shell-profile editing.
- No compiler or Go SDK requirement on end-user systems.
