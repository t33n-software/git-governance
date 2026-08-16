# GitHub branch rulesets (importable JSON)

These JSON files are the executable, repository-specific source for GitHub
branch protection. They match the GitHub repository ruleset body used by:

- UI: **Settings → Rules → Rulesets → New ruleset → Import a ruleset**
- API: `POST /repos/{owner}/{repo}/rulesets`

Schema authority:

- [REST API endpoints for rules](https://docs.github.com/en/rest/repos/rules)
- [Available rules for rulesets](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets)
- [Managing rulesets for a repository](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/managing-rulesets-for-a-repository)

The rulesets enforce remote behavior; they do not redefine branch taxonomy,
ticket grammar, or local workflow policy.

## Files

| File | Targets | Intent |
|---|---|---|
| [`00-push-protections.json`](00-push-protections.json) | every push (no branch targets) | push protection: blocks secret- and key-shaped artifacts from entering the commit graph |
| [`01-ticket-working-branches.json`](01-ticket-working-branches.json) | all official ticket and `hotfix/*` working branches | light protection: block force push; allow direct commits and deletion after merge |
| [`02-develop.json`](02-develop.json) | `develop` | strong shared line: PR only, no force push, no deletion, CI/review gates, explicitly permits all three repository-approved PR merge methods |
| [`03-main.json`](03-main.json) | `main` | maximal shared line: PR only, no force push, no deletion, CI/review gates, merge commits only |
| [`04-release.json`](04-release.json) | `release/*` | very strong release line: PR only, no force push, no deletion, CI/review gates, merge commits only |
| [`05-support.json`](05-support.json) | `support/*` | very strong long-lived maintenance line: PR only, no force push, no deletion, CI/review gates, merge commits only |

## Branch-family coverage

| Branch family | Enforcement location | Reason |
|---|---|---|
| `main` | `03-main.json` | Protected production truth; PR-only and not rewriteable |
| `develop` | `02-develop.json` | Protected integration line; PR-only and not rewriteable |
| `release/*` | `04-release.json` | Frozen shared release line; PR-only and not rewriteable |
| `support/*` | `05-support.json` | Long-lived shared maintenance line; PR-only and not rewriteable |
| `feature/*`, `fix/*`, `docs/*`, `refactor/*`, `chore/*`, `test/*`, `perf/*` | `01-ticket-working-branches.json` | Official working branches: direct commits allowed, force pushes prohibited after publication |
| `hotfix/*` | `01-ticket-working-branches.json` plus its protected target line | Official working branch: direct commits allowed, force pushes prohibited; the PR lands on `main`, `release/*`, or `support/*` |
| `scratch/*` | Explicitly no GitHub ruleset | Private, short-lived exploration permits local rebase/amend/force-push behavior and is never a PR source |
| `staging` | Not applicable | The rules define staging as an artifact environment, not a branch |

The absence of a `scratch/*` JSON file is intentional policy coverage, not a
gap. A no-op GitHub ruleset would add no enforcement; a restrictive one would
contradict the private-exploration contract.

## Push protections

`00-push-protections.json` is a push Ruleset: it applies to every push to the
repository and its entire fork network and carries no branch targeting. It
blocks secret- and key-shaped artifacts (private-key and key-store extensions,
environment files, credential files, and infrastructure state files) from
entering the commit graph.

Push Rulesets exist only for private and internal repositories with the Team
plan. This repository is public and cannot carry one; the file documents the
boundary and stands ready if the repository is ever reclassified as private.
The public secret-material boundary is secret scanning with push protection
plus the local quality gates. The file is therefore excluded from the import
sequences below while the repository stays public.

The file mirrors the official GitHub export envelope because the import
validates against that schema: a `source` field with the repository's own
identity, an explicit `conditions: null` (push Rulesets carry no branch
conditions), and restricted file extensions in glob form (`*.pem`, not `pem`).

## Code owner review

`.github/CODEOWNERS` is the versioned ownership contract. Its default `*`
entry binds every path to `@CyberT33N`, the maintainer of this
single-maintainer organization. The shared-line Rulesets `02-develop.json`,
`03-main.json`, `04-release.json`, and `05-support.json` set
`require_code_owner_review: true`, so a shared-line pull request requires an
approval from the bound owner in addition to the existing review count and
resolved review threads.

GitHub CODEOWNERS entries may reference users, organization teams
(`@org/team`), or email addresses; a bare organization cannot own code. A team
reference such as `@t33n-software/<team>` is added by a governed change once
the organization establishes its first team.

After a merged change to these Ruleset sources, re-import the affected files
through the GitHub UI so the live rules match the versioned sources.

## Hotfix, Scratch, and Staging

### Hotfix

`hotfix/*` is a short-lived official working branch for an incident on the
line that actually carries the defect. It is already covered by
[`01-ticket-working-branches.json`](01-ticket-working-branches.json), which
blocks force pushes while preserving direct implementation commits.

There is deliberately no duplicate hotfix-only Ruleset: it would enforce the
same `non_fast_forward` rule twice. The pull request is instead protected by
the Ruleset for its actual destination:

- `hotfix/*` → `main`: [`03-main.json`](03-main.json)
- `hotfix/*` → `release/*`: [`04-release.json`](04-release.json)
- `hotfix/*` → `support/*`: [`05-support.json`](05-support.json)

This preserves the actual incident lineage and avoids treating every hotfix as
if it belonged to `main`.

For a same-repository `hotfix/* -> main` pull request, the existing required
Linux quality job also validates the ticket-bound release record under
`.git-governance/hotfix-release-records/`. This keeps the pre-merge record
gate inside an already required main check instead of creating an optional
hotfix-only status context. The record and manifest remain domain/controller
responsibilities; Rulesets alone cannot validate their semantic content.

After immutable Main-hotfix delivery, each declared multi-commit propagation
candidate is published only by the protected `hotfix-propagation` controller.
Its dedicated Publisher App may push the provenance-validated `fix/*`
candidate and create a PR for the declared target line, but it has no Ruleset
bypass, Actions, Workflows, Administration, Secrets, or direct Shared-Line
permission. Rulesets continue to enforce the target-line review and Required
Checks; the controller never merges a propagation PR.

### Scratch

`scratch/*` is private, short-lived exploration. It is neither an official
pull-request source nor a shared integration line. Its purpose is to allow a
developer to experiment safely before stable work is transferred to an
official ticket branch.

No GitHub Ruleset is created for `scratch/*`: its intended private workflow
permits local rebase, amend, and force-push behavior. A Ruleset that blocked
those operations would defeat the reason the branch exists.

### Staging

`staging` is not a branch and therefore has no Ruleset. It is an environment
fed from release-candidate artifacts built from `release/*`. Treating it as a
permanent branch would introduce an unsupported branch-per-environment flow
and obscure the release artifact that was actually deployed.

## Supply-chain fortress gates

The shared-line Rulesets `02-develop.json`, `03-main.json`,
`04-release.json`, and `05-support.json` require the following gates:

| Gate | Configured threshold | Why it applies |
|---|---|---|
| Dependency admission review | fail dependency additions at `low` severity or above across `runtime`, `development`, and `unknown` scopes | Every dependency change reaches a shared-line PR through the immutable `actions/dependency-review-action` gate. |
| CodeQL code scanning | `alerts_threshold: all`; `security_alerts_threshold: all` | Every unresolved CodeQL alert, including the lowest reported severity, blocks a pull request. |
| GitHub Code Quality | `severity: all` | Every unresolved Code Quality result blocks a pull request. |
| GitHub Code Quality coverage | `minimum_coverage: 100`; `max_coverage_drop: 0` | Any aggregate coverage below 100% or any coverage decrease blocks a pull request. |
| Project statement coverage | exactly `100.0%` | The existing repository quality gate rejects executable Go packages below complete statement coverage. |

These gates intentionally target **shared PR destination lines only**:
`develop`, `main`, `release/*`, and `support/*`. They do not target
`feature/*`, other regular ticket branches, or `hotfix/*` directly. Those
branches are PR sources; their changes are analyzed as part of the pull
request to the protected destination line. Applying these gates to source
branches would block normal direct development pushes without increasing the
merge-time protection.

In particular, **never** add `code_coverage`, `code_scanning`, or
`code_quality` to `01-ticket-working-branches.json`. GitHub coverage checks
require a PR merge through the API or UI, so applying them to a working branch
would reject normal branch pushes before a PR can exist.

### Required CodeQL and Code Quality prerequisites

Before importing these Rulesets with active enforcement, the repository
**MUST** have:

1. a successful CodeQL workflow that reports results for pull requests to each
   shared destination line;
2. GitHub Code Quality enabled for the repository and a successful
   `CodeQL - Code Quality` result on a pull request;
3. an authorized plan for GitHub Code Quality. This GitHub feature is currently
   a preview capability and requires GitHub Team or Enterprise Cloud;
4. a controlled exception path for a proven emergency, with audited,
   least-privileged bypass access rather than a broad administrator bypass.

Without the first two prerequisites, GitHub will block every pull request
because the Rulesets require results that do not exist yet.

### Activate CodeQL

This repository supplies the versioned advanced CodeQL workflow at
[`../../../../.github/workflows/codeql.yml`](../../../../.github/workflows/codeql.yml).
It scans Go code on pull requests to and pushes on every shared line, then
uploads CodeQL results with only `security-events: write` in addition to
read-only repository permissions.

To activate it in GitHub:

1. Open **Settings → Security → Code scanning** for the repository.
2. If CodeQL setup is requested, choose **Advanced** setup so GitHub uses the
   committed workflow rather than creating a second default-setup scanner.
3. Merge or update a pull request containing `codeql.yml`.
4. Verify that **CodeQL (go)** succeeds and that Code Scanning records its
   results for the pull request target.

The Code Scanning Ruleset gate will remain pending until that first successful
analysis uploads results. Do not use `pull_request_target` for this workflow:
untrusted pull-request code must run with the normal `pull_request` event.

### Required GitHub Code Quality coverage configuration

The shared-line JSON files serialize GitHub's **Restrict code coverage** rule
with the following strict values:

```text
minimum_coverage: 100
max_coverage_drop: 0
```

`Maximum coverage drop` is not a maximum coverage value. Setting it to `100`
would permit a decline of up to 100 percentage points and is therefore the
least strict meaningful setting. A value of `0` rejects every coverage
decrease. Before importing these active Rulesets, the repository **MUST**
upload a Cobertura XML report for pull requests and the default branch with
`code-quality: write` permission. The existing project coverage gate remains
the authoritative immediate 100% enforcement until that upload path is live.

## Enforcement boundary

Rulesets are the final remote enforcement layer, not a replacement for the
full governance system. GitHub cannot encode every repository governance
constraint faithfully in static branch rulesets:

- ticket-scoped branch grammar and the equality between branch ticket and
  Conventional Commit scope;
- the mandatory, conditional rebase before an official branch's first push;
- the prohibition of feature work on a frozen `release/*` line;
- the actual affected line, explicit propagation, and `cherry-pick -x`
  provenance of a hotfix;
- a merge-method choice conditional on the PR source branch or on whether a
  ticket history contains internal noise;
- protected-line creation, tag-to-artifact triggering, and deletion only after
  release promotion, confirmed delivery, and documented reconciliation.

Those controls remain in the `git-governance` domain/application workflow,
Lefthook, CI, and human release governance. Treating GitHub Rulesets as the
only architecture would both leave these constraints unenforced and duplicate
the canonical policy in a less expressive mechanism.

## Release lifecycle automation

Rulesets protect the release boundary but cannot execute or prove the complete
release lifecycle. The GitHub lifecycle adapter and Actions workflows must
provide these separate controls:

1. `workflow release cut` emits only a local intent. The protected
   `release-request` controller binds the ticket, source SHA, and target ref,
   then dispatches `execute-protected-line-request.yml` with its durable
   `request_id`. The executor and automatic finalizer prove the resulting
   `origin/release/<semver>` reference. The CLI never pushes a shared line.
2. The merged `release/<semver> -> main` PR triggers the tag workflow. Because
   a tag pushed with `GITHUB_TOKEN` cannot trigger another `push` workflow,
   that workflow explicitly dispatches `publish-release-artifacts.yml` with the immutable tag.
3. `workflow release backmerge` verifies the merged promotion, exact tag, and
   published release before comparing `release/<semver>` with `develop`. It
   opens a PR only for an effective delta; `not-required` is the auditable
   completion result when no delta remains.
4. Release cleanup occurs only after delivery and either the backmerge merge or
   the recorded `not-required` reconciliation result.

When the Develop Ruleset requires a current pull-request head, GitHub's
**Update branch** action must not update the delivered `release/<semver>` ref.
The governed reconciliation path instead uses a ticket-bound `chore/*`
preparation branch created from the release line, merges current Develop only
into that working branch, validates it, and opens a merge-commit PR from the
working branch to Develop.

The protected Main release-control workflow builds a trusted binary before it
creates the release-derived preparation branch. It uses a short-lived, masked
broker installation token only in the ephemeral runner Git transport, clears
that configuration on exit, and opens the reviewed merge-commit PR to Develop
without updating the delivered release ref.

If the controlled Develop merge conflicts, Rulesets cannot resolve the content
or prove an arbitrary submitted branch is safe. The controller fails closed,
does not push an unresolved candidate, and records the release/develop/ticket
context through the workflow run. A `reconciliation-resume` recovery run
accepts only a ticket-bound `chore/*` candidate whose merge parents prove the
immutable release ref and the current Develop ref; it then reruns delivery,
quality, review, and publication gates. Neither GitHub **Update branch** nor a
Ruleset bypass is a replacement for that source-aware provenance check.

The reconciliation publisher is a dedicated GitHub App and broker identity,
separate from release-line automation. It may publish only the
provenance-validated ticket-bound `chore/*` candidate and its reviewed PR to
`develop`. It has no Ruleset bypass, release-line dispatch, workflow-write, or
direct shared-line mutation role.

The normal target path dispatches this controller automatically after verified
release delivery. It remains idempotent, revalidates all delivery evidence,
and creates a PR only for effective delta. Manual dispatch is retained only
for incident, retry, and recovery; it cannot bypass Ruleset, review, quality,
or audit gates.

### Initial protected-line creation

`04-release.json` and `05-support.json` set
`required_status_checks.do_not_enforce_on_create` to `true`. A newly created
`release/*` or `support/*` ref cannot have its own status-check results before
it exists, so requiring them at creation time would reject the controlled
workflow by construction.

This exception is limited to first creation. The resulting shared line retains
the deletion, non-fast-forward, pull-request, review, status-check, CodeQL,
Code Quality, and coverage protections for every subsequent update. Do not use
a broad Ruleset bypass merely to create a protected line.

The automation identity must be narrowly scoped and auditable. Do not use an
ordinary developer or a broad administrator bypass to create protected release
lines, dispatch release workflows, or remove them after completion.

## Merge strategy mapping

GitHub rulesets target the **destination branch**, not the PR source. They
cannot enforce different merge methods for different source families targeting
the same line.

| Destination | Canonical workflow result | Ruleset behavior |
|---|---|---|
| `develop` | Regular ticket PRs may use rebase merge, normal merge, or a selective squash; the release backmerge must remain possible | Explicitly allows `merge`, `rebase`, and `squash` |
| `main` | Release promotion and hotfix lineage prefer a merge commit | `merge` only |
| `release/*` | Controlled stabilization and hotfix lineage prefer a merge commit | `merge` only |
| `support/*` | Controlled maintenance and hotfix lineage prefer a merge commit | `merge` only |

`required_linear_history` is deliberately absent: it would prohibit the merge
commits explicitly preferred for release and hotfix lineage. A GitHub **rebase
merge** is not the same operation as rebasing the shared target branch; the
reference workflow permits it for a regular ticket PR into `develop`, but does
not make it a global or universal requirement. GitHub rulesets cannot express
the remaining conditional decision (“squash only for internal noise”) because
they cannot select a merge method by PR source branch or commit quality.

### Pull request creation versus merge selection

GitHub records both the PR source and target branch, for example
`feature/GOV-3-example` → `develop`. Creating a pull request does not select
its merge method. The authorized person who merges it selects the method after
reviews and checks are satisfied.

For a regular ticket PR targeting `develop`, use this decision order:

| PR history | Merge method |
|---|---|
| Each commit is independently meaningful and should remain visible | Rebase merge |
| The branch boundary or a release backmerge must remain explicit | Merge commit |
| The branch contains internal intermediate commits that should not enter the integration history | Selective squash merge |

All three methods are deliberately available on `develop`; availability is not
permission to choose arbitrarily. GitHub Rulesets cannot select a method from
the source branch or commit history, so that context decision remains with the
authorized merger.

### Auto-merge

**Allow auto-merge MUST remain disabled** under **Settings → General → Pull
Requests**. Auto-merge preserves a method selected in advance and executes it
later after requirements pass. It does not bypass Rulesets, reviews, or
checks, but it removes the final deliberate review of the context-dependent
merge-method decision. Reconsider it only if a controlled merger service
enforces the same source- and history-aware decision matrix.

## Bypass, review, and lifecycle decisions

`bypass_actors` is intentionally empty. GitHub omits bypass lists when
exporting a ruleset, and the repository cannot safely invent team or app IDs.
Do not grant ordinary developers bypass.

`release/*` is protected from premature deletion. Before a completed release
line can be deleted after its promotion, confirmed delivery, and completed
reconciliation, configure an
explicit, audited release-maintainer or release-automation bypass through the
GitHub UI or API. The same identity decision is needed if release or support
branch cleanup requires an otherwise unavailable operation. Routine initial
release or support branch creation uses the narrow
`do_not_enforce_on_create` exception instead of a Ruleset-wide bypass. GitHub
bypass is ruleset-wide, so use a dedicated, least-privileged actor rather than
an unrestricted administrator bypass when a bypass is genuinely required.

The rulesets require one approval and resolved review threads, which implement
the governance recommendations. Code owner review is active on every shared
line through the versioned ownership contract; see "Code owner review". Signed
commits remain a recommended, separate decision and are not silently imposed
here.

## Branch cleanup after merge

GitHub Rulesets can restrict deletion; they cannot automatically delete a
branch after a pull request lifecycle completes.

For normal ticket branches, enable **Settings → General → Pull Requests →
Automatically delete head branches**. The working-branch Ruleset intentionally
has no `deletion` rule, allowing GitHub to remove merged
`feature/*`, `fix/*`, `docs/*`, `refactor/*`, `chore/*`, `test/*`, and
`perf/*` remote branches automatically.

Shared lines remain deletion-protected. `release/*` requires a controlled
cleanup only after promotion to `main`, confirmed delivery, and either a
merged backmerge to `develop` or an audited `not-required` reconciliation;
active
`support/*`, `main`, and `develop` are never automatically deleted. A
`hotfix/*` branch is deleted only after its merge and documented
forward-/backport decision, so its lifecycle is controlled by hosting or CI
rather than an unconditional deletion rule.

Automatic remote cleanup does not delete local branches. The governed CLI
never cleans official local branches; `workflow cleanup` is reserved for
local `scratch/*` branches.

## Import (UI)

1. Open the repository on GitHub.
2. **Settings → Rules → Rulesets**.
3. **New ruleset → Import a ruleset**.
4. Select one JSON file from this directory.
5. Review targets and rules, then **Create**.
6. Repeat for each branch ruleset file in numeric order.

## Import (API)

```powershell
gh api --method POST repos/{owner}/{repo}/rulesets --input docs/hosting-platforms/github/rulesets/01-ticket-working-branches.json
gh api --method POST repos/{owner}/{repo}/rulesets --input docs/hosting-platforms/github/rulesets/02-develop.json
gh api --method POST repos/{owner}/{repo}/rulesets --input docs/hosting-platforms/github/rulesets/03-main.json
gh api --method POST repos/{owner}/{repo}/rulesets --input docs/hosting-platforms/github/rulesets/04-release.json
gh api --method POST repos/{owner}/{repo}/rulesets --input docs/hosting-platforms/github/rulesets/05-support.json
```

## Status checks

Shared-line rulesets require the CI job names from `.github/workflows/ci.yml`:

- `Quality gates (linux-amd64)`
- `Quality gates (macos-arm64)`
- `Quality gates (windows-amd64)`
- `Dependency admission review`

The dependency-admission workflow emits its required check for pull requests
targeting `develop`, `main`, `release/**`, and `support/**`. It has no path
filter because a skipped required workflow leaves its check pending and blocks
the pull request.

If any of those check names or target branches change, update the
`required_status_checks` arrays before import, or adjust them in the GitHub UI
after import. Enable a required check on a live shared-line Ruleset only after
the corresponding workflow can report that check for pull requests to the
target line.

For `04-release.json` and `05-support.json`, retain
`do_not_enforce_on_create: true` while updating those arrays. It is required
for the controlled first creation of a previously nonexistent protected line;
it does not weaken checks on an existing line.

## Required repository merge settings

GitHub's repository settings expose merge-method **capabilities** globally;
Rulesets then restrict their use by destination branch. This repository
**MUST** enable all three settings under **Settings → General → Pull Requests**
so that no supported workflow is blocked:

| Repository setting | Why it must be enabled | Where Rulesets permit it |
|---|---|---|
| **Allow merge commits** | Preserves visible release and hotfix lineage. | `main`, `release/*`, and `support/*` allow only `merge`; `develop` also permits it for a normal merge or release backmerge. |
| **Allow rebase merging** | Preserves a deliberately reviewed regular ticket commit series when it lands on the integration line. | `develop` only. |
| **Allow squash merging** | Supports the exceptional case in which a ticket PR contains internal noise that should become one reviewable integration commit. | `develop` only. |

The global settings do not grant every method to every branch. The effective
method is the intersection of the repository capability and the Ruleset for
the pull request's destination branch:

```text
globally enabled merge methods ∩ destination-branch allowed_merge_methods
```

`02-develop.json` explicitly allows all three methods. That does **not** make
rebase the global default and does not authorize a rebase of `develop` itself.
`03-main.json`, `04-release.json`, and `05-support.json` constrain their
targets to merge commits, preserving release and hotfix lineage despite the
repository-wide capability settings.

No ruleset-only configuration can enforce the source-dependent merge decision
for `develop`; retain the `git-governance` workflow, review policy, and CI as
the authoritative controls for that remaining semantic rule.

## Promotion-base alignment on release PR heads

GitHub may show **Update branch** for a pull request. That action updates the
PR head ref; it is not a harmless metadata refresh. For a promotion PR whose
head is `release/<semver>`, an update or rebase would directly mutate a
protected shared release line.

`04-release.json` therefore protects the underlying `release/*` ref through
PR-only updates, `non_fast_forward`, required review, required checks, and
merge-only PR methods. GitHub Rulesets cannot independently hide the UI action
based on the source branch family, so the expected behavior is that a direct
attempt is rejected by the protected ref controls.

When the `main` Ruleset requires a current promotion head, use the governed
release-preparation flow:

```text
release/<semver>
  -> ticket-bound chore/* release-preparation branch
  -> controlled merge of origin/main
  -> quality gates and review
  -> merge-commit PR back to release/<semver>
  -> re-evaluate the existing release/<semver> -> main PR
```

The alignment controller must verify that the incoming Main commits are
release-compatible. It may be the repository CLI, a protected GitHub Actions
workflow, or a narrowly scoped GitHub App, but it must never bypass the
Ruleset to update an existing `release/*` ref directly.
