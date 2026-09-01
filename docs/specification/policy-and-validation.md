# Policy and Validation Specification

## 1. Purpose and authority

This document is the executable, standalone convention contract for
`git-governance`. It is not a second policy registry. The initial
implementation validates ticket keys only syntactically; the later
registry/bundle check is added through the same `KeyPolicy` port.

The syntax is complete for the scope bound here:

- all 13 branch families defined in this document
- ticket, slug, release, and support names
- Conventional Commits / Angular style with a ticket reference
- breaking changes
- cross-field and repository state rules

"Complete" does not mean that a single regex can prove functional Git state.
The implementation uses a parser, small regexes per value object, and
subsequent semantic checks.

## 2. Validation pipeline

Every input passes through, in this order:

1. size and control-character limit
2. normalization only where it is lossless and explicit
3. lexical regex check
4. parsing into value objects
5. cross-field rules
6. Git reference check with `git check-ref-format --branch`
7. repository and publication state rules
8. optional policy registry/bundle check

Invalid inputs are never silently rewritten into other values. The
interactive UI shows for the affected field the safe actual value, the rule,
the expected format, a valid example, and the correction, and prompts for the
same value again. It neither leaves the field nor restarts the workflow; the
user can correct without limit. The UI may suggest an explicit correction but
requires the user's confirmation.

## 3. Ticket key

### 3.1 Decision

Keys may contain uppercase letters and digits but must begin with an
uppercase letter:

```regex
^[A-Z][A-Z0-9]*$
```

Examples:

- valid: `ABC`, `PLATFORM2`, `A1`
- invalid: `abc`, `2ABC`, `ABC-OPS`, `ABC_OPS`, empty

Digits must be allowed because a syntactic namespace should not be
unnecessarily restricted to pure letters. A leading letter keeps the
separation between key and ticket number unambiguous. Hyphens are forbidden
in the key because the first hyphen separates the key from the ticket number.

Additional technical limits:

- length: 1 to 32 ASCII characters
- no whitespace or control characters

The length limit is a protective limit of the CLI contract, not a statement
about a policy registry. Without a registry, every key that satisfies this
syntax is accepted; there is no allowlist.

### 3.2 Ticket number

```regex
^[1-9][0-9]*$
```

Additional technical limit: at most 18 digits.

Thereby `0`, negative values, signs, decimal places, and leading zeros are
not canonical. The number is treated as a string so that no integer overflow
enters the domain.

### 3.3 Ticket ID

```regex
^([A-Z][A-Z0-9]*)-([1-9][0-9]*)$
```

Examples:

- valid: `ABC-123`, `PLATFORM2-7`
- invalid: `ABC123`, `abc-123`, `ABC-001`, `ABC-0`

## 4. Branch slug

The slug is canonical ASCII `kebab-case`:

```regex
^[a-z0-9]+(?:-[a-z0-9]+)*$
```

Examples:

- valid: `add-export-button`
- valid: `oauth2-token-refresh`
- valid: `docs-v2`
- invalid: `Add-Export`
- invalid: `add--export`
- invalid: `-add-export`
- invalid: `add-export-`
- invalid: `feature/frontend`

Additional technical limit: 1 to 100 characters.

The old project's regex, `^[a-z0-9-]+$`, is not sufficient because it accepts
leading, trailing, and double hyphens.

## 5. Branch families

### 5.1 Complete taxonomy

| Family | Name form | Creation |
|---|---|---|
| `main` | exactly `main` | not through the normal wizard |
| `develop` | exactly `develop` | not through the normal wizard |
| `release` | `release/<semver>` | only the release workflow |
| `support` | `support/<major.minor>` | only the support/release workflow |
| `feature` | `feature/<ticket>-<slug>` | regular ticket workflow |
| `fix` | `fix/<ticket>-<slug>` | regular ticket workflow |
| `docs` | `docs/<ticket>-<slug>` | regular ticket workflow |
| `refactor` | `refactor/<ticket>-<slug>` | regular ticket workflow |
| `chore` | `chore/<ticket>-<slug>` | regular ticket workflow |
| `test` | `test/<ticket>-<slug>` | regular ticket workflow |
| `perf` | `perf/<ticket>-<slug>` | regular ticket workflow |
| `hotfix` | `hotfix/<ticket>-<slug>` | hotfix workflow |
| `scratch` | `scratch/<ticket>-<slug>` | private exploration |

`feature` is a branch family; `feat` is a commit type. Aliases like `feat/`,
misspellings like `featch/`, developer names, and additional path segments
are not accepted.

### 5.2 Official ticket, hotfix, and scratch branches

```regex
^(feature|fix|docs|refactor|chore|test|perf|hotfix|scratch)/([A-Z][A-Z0-9]*)-([1-9][0-9]*)-([a-z0-9]+(?:-[a-z0-9]+)*)$
```

Capture groups:

1. branch family
2. ticket key
3. ticket number
4. slug

Examples:

- `feature/ABC-123-add-export-button`
- `fix/PLATFORM2-7-handle-null-customer-id`
- `hotfix/ABC-999-payment-timeout`
- `scratch/ABC-123-export-button-exploration`

Not allowed:

- `feat/ABC-123-add-export-button`
- `feature/frontend/ABC-123-add-export-button`
- `feature/ABC-123/add-export-button`
- `feature/dennis/ABC-123-add-export-button`
- `feature/ABC-123-add--export-button`

### 5.3 Release branches

A release uses Semantic Versioning 2.0.0 without a leading `v`.

SemVer component:

```regex
^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-((?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$
```

Branch:

```text
release/<semver>
```

Examples:

- valid: `release/2.8.0`
- valid: `release/2.8.0-rc.1`
- valid: `release/2.8.0-rc.1+build.7`
- invalid: `release/v2.8.0`
- invalid: `release/02.8.0`
- invalid: `release/2.8`

The implementation checks the prefix first and then validates the version
with a dedicated `SemanticVersion` parser. A composite monster regex is not
used as the sole source of errors.

Source: [Semantic Versioning 2.0.0](https://semver.org/)

### 5.4 Support branches

Component:

```regex
^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$
```

Branch:

```text
support/<major.minor>
```

Examples:

- valid: `support/2.7`
- valid: `support/0.9`
- invalid: `support/2.7.1`
- invalid: `support/v2.7`

### 5.5 Shared lines

```regex
^(main|develop)$
```

Syntactic validity does not mean the user may work there. Direct commits and
pushes are blocked by repository state rules.

## 6. Branch-semantic rules

Regexes cannot prove the following rules and are therefore complemented by
domain and Git checks:

- regular ticket branches start from `origin/develop`
- hotfixes start from the actually affected `origin/main`, `origin/release/*`,
  or `origin/support/*` line
- release cuts start from `origin/develop`
- `scratch/*` is private and not a PR target
- a directly created `scratch/*` branch starts from a local official ticket
  branch with the same ticket ID
- a scratch transfer uses exclusively an existing local official ticket branch
  with the same ticket ID; the scratch and target slugs do not have to be
  identical
- exactly one local official candidate is chosen automatically; no candidate
  leads to `SCRATCH_TARGET_BRANCH_MISSING`; multiple candidates require an
  explicit target branch
- the transfer uses `git merge --squash` and produces a single
  ticket-consistent Conventional Commit on the official target branch
- `main`, `develop`, `release/*`, and `support/*` are shared lines
- per ticket, exactly one official working branch exists in the normal
  workflow
- ticket exclusivity is checked after `fetch --prune` against local and
  selected remote-tracking branches
- release stabilization and hotfix propagation are explicit active-line
  workflows and not regular ticket side branches
- before the first push, the actual target base is checked
- rebase is only permissible before the first push and only with missing base
  commits
- after the first push, an official branch is append-only
- force push is forbidden for official branches

The publication state is determined after a successful fetch through the
remote reference. If the state cannot be determined reliably, a
history-rewriting operation blocks safely instead of assuming an unpublished
branch.

### 6.0 Authorized protected-line creation

A release or support cut consists of separate, bound phases:

```text
Release Request Authorization
→ durable request record
→ Release Execution Authorization
→ exactly one protected ref mutation
→ automatic read-only finalizer
→ verified | failed | verification_pending
```

The request binds at least the ticket, the operation, the version, the source
ref, the exact source SHA, the target ref, the requester, the controller run,
the expected executor, the expiry, and the idempotency key. The request
authorization holds no `contents: write` permission.

The executor accepts no free-form version, branch, or source SHA inputs. It
only validates a non-expired, unconsumed request and may only create its
bound target ref exactly once. The execution authorization happens in the
separate `release-execution` environment.

The finalizer holds no ref mutation permission. It checks the correlated
executor job and the actual remote ref against the bound source SHA. Only
`verified` is a complete cut proof. A `verification_pending` recovery checks
the same request read-only and may not dispatch a new executor.

### 6.0.1 Functional release and hotfix control lanes

An active controller belongs to exactly one functional lane. The generic
`release` environment may neither authorize nor credentialize request,
execution, credential verification, regular delivery, or hotfix delivery or
propagation jointly.

| Lane | Permitted task | Not permitted |
| --- | --- | --- |
| `release-request` | scope, version, SHA, and target-ref authorization | ref mutation or external broker credentials |
| `release-execution` | exactly one bound protected-line mutation | promotion, tag, delivery, or reconciliation |
| `release-credential-verification` | read-only verification of the private release credential issuer | ref, tag, release, or candidate mutation |
| `release-delivery` | regular immutable tag and artifact delivery | request, execution, or candidate publication |
| `release-reconciliation` | provenance-validated `chore/*` candidate publication | direct shared-line mutation |
| `hotfix-delivery` | main or support patch tag and delivery verification | candidate publication |
| `hotfix-propagation` | provenance-validated `fix/*` candidate publication | tag, delivery, or shared-line mutation |

A lane with a credential issuer need receives only its own OIDC/WIF,
invocation, runtime, and secret boundary. Request and execution receive no
external credential issuer permission when their job-scoped platform
identities can execute the task with minimal rights.

### 6.1 Release stabilization

After a release cut, only three stabilizing branch categories are
permissible:

| Category | Branch family | Permitted purpose |
|---|---|---|
| `blocker` | `fix/*` | release-blocking defect |
| `docs` | `docs/*` | final technical or operative documentation |
| `release-prep` | `chore/*` | version, release preparation, or technical release work |

These branches start from `origin/release/<semver>` and target the same
release line via PR. Feature, general refactor, and off-topic work is not
permissible on the stabilization path.

### 6.1.1 Promotion base alignment

A promotion PR `release/<semver> -> main` can be structurally outdated under
strict main status checks. That is not a general rebase permission and never
allows a direct update of the `release/*` ref via GitHub **Update branch**, a
rebase, or a force push.

An alignment is only permissible when:

1. `main` actually requires a current promotion base;
2. the missing main commits are release-compatible;
3. a `release-prep` `chore/*` branch with the stored
   `origin/release/<semver>` base is active.

`workflow release align-promotion-base` verifies this provenance, merges
`origin/main` exclusively into the working branch, runs the repository
quality suite, and publishes a PR back onto the same release line. The
produced merge commit is ticket-bound and append-only. Only after its merge
are the existing promotion PR and its main gates re-evaluated.

Conflicts remain fail-closed on the preparation branch. A resolution may only
stage the exact conflict paths. The CLI resume path verifies before
continuing that the active merge still integrates the `origin/main` commit
current as of the fetch, and re-checks this base before quality, push, and
PR. With a moved-on main line, the candidate is discarded; a raw call of
`git merge --continue`, a rebase, or a release ref mutation are not
permissible recovery mechanisms.

GitHub rulesets protect the underlying release ref but cannot independently
hide the UI action based on its head branch family. Therefore direct ref
updates must be rejected server-side and the source-aware alignment must be
controlled by the workflow.

### 6.2 Support provenance

`support/<major.minor>` may only be created from `origin/main` when the
revision carries a matching release tag `v<major.minor.patch>`. Thereby a
support line does not begin from an unreleased integration state.

### 6.3 Hotfix forwarding

A hotfix PR targets the same affected line. When the same change is needed in
another active line, the tool creates a controlled `fix/*` branch there and
runs `git cherry-pick -x <sha>`. The provenance thereby remains visible in
the commit history.

A main hotfix receives a versioned record under
`.git-governance/hotfix-release-records/<KEY-NUMBER>.json` before the merge.
The record binds the ticket, the incident, the affected line, the previous
tag, the patch target version, the same-repository PR, the complete SHA
manifest, the commit budget exception, and all additional target lines. One
to three semantic manifest commits are normal; four need a justified
exception. Five or more additionally need an explicit release approval
referenced in the record; without it, the main merge is rejected fail-closed.

The single-commit surface `workflow hotfix propagate` remains for a reviewed
commit. `workflow hotfix propagate-manifest` may only use declared targets
and produces a local, non-shared `fix/*` candidate. It stores the resume
cursor exclusively in the local Git metadata resolution, not in the record
and not in the worktree. The candidate is published exclusively through the
separate hotfix propagation publisher boundary. The protected controller
revalidates delivery, record, manifest, target line, and quality, configures
only temporary masked Git transport credentials, and calls
`propagate-manifest --publish`. There is no local `--push` or PR bypass, no
blanket `main -> develop` merge, and no direct shared-line mutation.

A server-side main hotfix delivery controller revalidates the record, the PR
identity, the merge, the manifest, and the tag idempotency before it creates
the immutable patch tag. Afterwards, the release is only complete after a
non-draft release, payload, checksums, SBOM, the Sigstore bundle, and a
successful artifact workflow. A blanket `main -> develop` merge is neither
propagation nor reconciliation and remains forbidden.

### 6.4 Local workflow base metadata

Hotfix, release stabilization, and propagation workflows store their actual
remote base in the local Git configuration. The key is a JSON map under
`git-governance.workflow-bases`; it is not a global policy and is not
committed. `sync-base`, ticket publish, and `pre-push` read this base when no
explicit `--base` was passed. Thereby a hotfix or stabilization branch is not
mistakenly checked against `origin/develop`.

### 6.5 Repository quality gates

An existing, valid `git-governance.quality.json` is an explicit repository
contract. On all official working branches, its gates are mandatory before
every push. The pre-push validator always checks all actual ref updates
structurally. A push of multiple official refs runs the suite at most once.

The configuration uses a default scope and one scope per gate.
`includeFamilies` selects branch families; `excludeFamilies` is applied
afterwards and removes families. A gate without its own scope inherits the
default. Thereby a base suite can run on all official working families, a
documentation link check only on `docs/*`, and an elaborate stress test only
on `feature/*` and `perf/*`.

`scratch/*` is private exploration and not part of the default scope. A
concrete gate can, however, deliberately include scratch via
`includeFamilies`. A missing file always reads `unconfigured`, never
`passed`.

The final local suite of a publish workflow runs only after the last
permissible synchronization mutation. Its result is stored as a short-lived,
revision-bound record under `git-governance.final-quality-evidence` in the
local Git configuration, not in the versioned worktree. The record contains
no credentials and binds the ref and commit, the target base revision, the
remote, the configuration digest, the gate selection, the toolchain, a clean
worktree, and the creation time.

A matching proof only prevents the repetition of the same local suite. It
never replaces the structural pre-push policy, remote CI, required checks,
review, or shared-line protection. If the proof is missing, expired, or does
not match exactly, the suite runs once as a fallback. Corrupted or incomplete
proofs are rejected fail-closed; `--no-verify`, hook deactivation, and
unbound skip switches remain inadmissible.

### 6.6 Cleanup boundary

Remote deletion is not a CLI task. The hosting platform and CI control the
deletion of merged ticket and hotfix branches as well as the later release
cleanup after promotion, confirmed delivery, and reconciliation. The CLI
deletes exclusively:

- local `scratch/*` branches,
- never official ticket, hotfix, release, or support branches,
- never `main` or `develop`,
- never a remote branch.

On local deletion, the CLI removes the associated local
`git-governance.workflow-bases` metadata line. Merge, PR, and
forward/backport proofs belong to hosting/CI gates as long as no configured
hosting adapter can supply this data authoritatively.

### 6.7 Release reconciliation

After every release delivery, `release/<semver>` is reconciled against
`develop`. This check is mandatory, but a backmerge PR is only mandatory with
an effective delta:

- The lifecycle provider must prove the merged promotion PR, the exactly
  belonging immutable tag, and the successful release delivery.
- The provider then compares `release/<semver>` with `develop`.
- With an effective delta, `workflow release backmerge` creates the PR to
  `develop`.
- Without an effective delta, the command reports `not-required`; the
  auditable proof documents that no merge is needed.

An open promotion PR, a mere tag name, or a missing artifact delivery are not
permissible backmerge preconditions.

If the develop target enforces a current PR head, the published
`release/<semver>` ref must not be updated. In this case, the protected main
control plane creates a ticket-bound, release-derived `chore/*` preparation
branch. It builds the trusted CLI binary before the preparation branch
switch, receives a short-lived broker identity, and lets exclusively this
branch merge the current develop state. The workflow requires the stored
release base, an effective delta, and complete delivery evidence. Neither the
delivered release ref nor a local dry run may trigger a provider publish or a
pull request creation. A rebase, force push, platform update of the release
ref, or a `develop -> release/<semver>` PR violates the contract.

After confirmed delivery, the controller evaluation on the target path is
programmatic and idempotent. With an effective delta, it creates the
reviewable PR or documents `not-required`. A manual `workflow_dispatch` call
is exclusively the incident, retry, or recovery fallback and passes the same
delivery, ticket, audit, and quality checks.

### 6.7.1 Reconciliation conflict recovery

Conflicts during the controlled merge of `origin/develop` into a
release-derived preparation branch are fail-closed. The controller output is
the conflict proof; it contains the release, develop, ticket, branch, and
conflict-related Git diagnostics, but no credentials or confidential
transport data.

Not permissible are:

- updating, rebasing, or synchronizing `release/*` via a platform button;
- merging `develop` directly or creating an empty backmerge PR;
- a global `ours`/`theirs` strategy;
- treating a freely chosen branch as a privileged CI recovery entry.

An authorized resolution only arises in a non-shared, ticket-bound
`chore/*` preparation branch. After manual resolution and exact staging,
`align-reconciliation-base --resume` continues the existing merge and may
only push the candidate branch. The privileged `reconciliation-resume` path
only accepts the branch when:

1. branch, ticket, and slug match;
2. HEAD is an exact two-parent merge of the unchanged release ref and the
   current develop ref;
3. develop has not moved on since the resolution;
4. delivery, delta, quality, security, and review have passed again.

Only the protected controller publishes the PR to `develop`. It never merges
into `develop` itself.

The controller publication uses its own reconciliation publisher identity
from the protected `release-reconciliation` environment. This identity
receives exclusively the minimal contents and pull request rights for the
validated `chore/*` candidate path. It holds no ruleset bypass, no
release-line dispatch, and no direct shared-line mutation permission.

## 7. Commit types

Permitted canonical types:

| Type | Meaning |
|---|---|
| `feat` | new feature |
| `fix` | defect correction |
| `docs` | documentation only |
| `refactor` | restructuring without a feature or bugfix |
| `chore` | maintenance or tooling |
| `test` | tests |
| `perf` | performance improvement |
| `build` | build system or external dependencies |
| `ci` | CI configuration and scripts |
| `style` | formatting without a semantic change |
| `revert` | deliberate reversal, with a reference in the body/footer |

Branch and commit types are separate taxonomies. A `feature/*` branch
typically uses `feat` but can also use `test` or `docs` for functionally
separate test or documentation commits.

## 8. Commit header

The ticket reference is the mandatory scope:

```regex
^(build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test)\(([A-Z][A-Z0-9]*)-([1-9][0-9]*)\)(!)?: ([^\r\n]+)$
```

Capture groups:

1. commit type
2. ticket key
3. ticket number
4. optional breaking `!`
5. description

Additional rules for the description:

- not empty after `: `
- no leading or trailing whitespace
- no control characters
- exactly one header line
- technical upper bound: 200 Unicode code points
- no automatic case change
- never the metadata envelope (type, ticket, breaking marker) in header form —
  neither at the start nor embedded; the envelope is assembly property and is
  rejected fail-closed (canonical:
  `docs/conventions/commits/subject-contract.md`)

Examples:

```text
feat(ABC-123): add export button
fix(ABC-123): address review feedback on export validation
docs(ABC-123): document export workflow
feat(ABC-123)!: replace the export contract
```

## 9. Complete commit message

The message is parsed, not validated with a single multiline regex:

```text
<header>

[optional free body]

[optional footers]
```

Rules:

- The body and the footers each begin after a blank line.
- Footers follow the Git-trailer-like Conventional Commits syntax.
- `BREAKING CHANGE: <text>` and `BREAKING-CHANGE: <text>` are synonymous.
- A breaking change is present when the header contains `!` or a breaking
  footer.
- The create UI produces for breaking changes by default both `!` and an
  explanatory footer.
- Per Conventional Commits, the validator also accepts either of the two
  forms alone.
- `revert` needs at least one commit reference in the body or footer.

Example:

```text
feat(ABC-123)!: replace the export contract

The endpoint now returns a versioned export resource.

BREAKING CHANGE: clients must consume the new resource envelope.
Refs: 0123456789abcdef
```

Source: [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/)

## 10. Commit-to-branch consistency

On `feature/*`, `fix/*`, `docs/*`, `refactor/*`, `chore/*`, `test/*`,
`perf/*`, `hotfix/*`, and `scratch/*`, the ticket in the commit header must
exactly match the ticket in the branch.

```text
Branch: feature/ABC-123-add-export-button
Valid: feat(ABC-123): add export button
Invalid: feat(ABC-124): add export button
```

On shared lines, direct developer commits are forbidden regardless of their
syntax.

Local synchronization merges into an already published ticket branch must use
a compliant message, for example:

```text
chore(ABC-123): merge origin/develop
```

Hosting-side merge commits created on shared lines are not local ticket
commits. CI classifies them via parent count and PR metadata instead of
faking a normal ticket header.

All supported local commit creation paths have a known branch context:
`commit create` uses the current official ticket branch, scratch transfers
use the resolved official target branch, and synchronization merges use the
current official branch. The key and ticket ID are therefore derived from
this branch context and are not freely choosable commit inputs. A
non-canonical branch or a detached HEAD cannot execute a governed commit
creation path.

Interactive creation consists of two levels: first one of the canonical
commit families from section 7 is selected, then only the description for the
header is entered. The UI shows the derived key and ticket ID as fixed
information. The family is always an explicit decision: non-interactively,
`--type` is mandatory; interactively, the branch family expectation is a
preselection toward a confirmed decision, not a silent derivation. Complete
messages remain available for `commit validate`, because the body and footers
are part of the Git text being checked there.

## 11. Initial commit

The old capability of automatically creating `Initial commit` on an empty
repository is not carried over:

- it belongs to the repository bootstrap, not to branch creation
- it has no ticket reference
- it changes state surprisingly
- it mixes two use cases

An empty repository therefore leads to `REPOSITORY_HAS_NO_COMMITS` with a
separate bootstrap instruction.

## 12. Key preferences and the registry boundary

The user may store syntactically valid keys in their local preference file.
This speeds up selection but grants a key no organizational validity.

Today:

```text
SyntaxOnlyKeyPolicy
→ regex and size limit
→ no network
→ no allowlist
```

Later:

```text
BundleKeyPolicy
→ syntactic check
→ signed/versioned local JSON bundle
→ status, repository admissibility, and staleness
→ CI remains binding
```

Both adapters implement the same port; the branch, commit, and workflow use
cases do not change.

## 13. Error classes

At least the following stable codes are required:

- `TICKET_KEY_INVALID`
- `TICKET_NUMBER_INVALID`
- `TICKET_ID_INVALID`
- `BRANCH_FAMILY_INVALID`
- `BRANCH_SLUG_INVALID`
- `BRANCH_NAME_INVALID`
- `BRANCH_REF_INVALID`
- `BRANCH_BASE_INVALID`
- `BRANCH_ALREADY_EXISTS`
- `TICKET_BRANCH_ALREADY_EXISTS`
- `BRANCH_PUBLICATION_UNKNOWN`
- `SCRATCH_SOURCE_BRANCH_MISSING`
- `SCRATCH_TARGET_BRANCH_MISSING`
- `SCRATCH_TARGET_BRANCH_AMBIGUOUS`
- `SCRATCH_MERGE_EMPTY`
- `SHARED_LINE_MUTATION_FORBIDDEN`
- `REBASE_NOT_REQUIRED`
- `REBASE_AFTER_PUBLISH_FORBIDDEN`
- `FORCE_PUSH_FORBIDDEN`
- `COMMIT_TYPE_INVALID`
- `COMMIT_HEADER_INVALID`
- `COMMIT_DESCRIPTION_INVALID`
- `COMMIT_TICKET_MISMATCH`
- `BREAKING_CHANGE_INVALID`
- `WORKTREE_NOT_CLEAN`
- `REPOSITORY_HAS_NO_COMMITS`
- `POLICY_BUNDLE_MISSING`
- `POLICY_BUNDLE_STALE`

Every error names the observed value, the expected format, a valid example,
and a concrete remediation.

## 13.1 Pre-push update protocol

The pre-push validator processes every Git line according to this contract:

```text
<local-ref> <local-object-id> <remote-ref> <remote-object-id>
```

Object IDs must be complete SHA-1 or SHA-256 values. For every
`refs/heads/*` update:

- the actual remote target is parsed and validated;
- `HEAD:main` is therefore protected exactly like `main:main`;
- multiple pushes are checked line by line;
- deletions of shared lines are blocked;
- non-fast-forward updates on official working branches are blocked;
- on a first push, the base freshness is checked against the concrete local
  object ID, not against a randomly checked-out branch;
- non-branch refs such as tags are classified as outside branch governance
  and explicitly reported as skipped.

## 14. Test catalog

The implementation must check at least the following equivalence classes:

- each of the 13 branch families
- each of the 11 commit types
- minimal and maximal key length
- keys with digits and an invalid leading character
- minimal and maximal ticket numbers
- slugs with digits
- leading, trailing, and double hyphens
- additional slash segments
- SemVer core, pre-release, and build metadata
- SemVer with leading zeros and empty identifiers
- support versions with too many or too few segments
- commits without a scope, with a wrong ticket, and with a wrong type
- breaking changes with `!`, a footer, or both
- body and multiple footers
- CRLF and LF messages
- NUL and control characters
- an unpublished branch without a base delta
- an unpublished branch with a base delta
- a published branch with a base delta
- an unknown publication state
- direct mutation of every shared-line class

Parsers additionally receive fuzz tests. Regex snapshots alone do not count
as sufficient proof.
