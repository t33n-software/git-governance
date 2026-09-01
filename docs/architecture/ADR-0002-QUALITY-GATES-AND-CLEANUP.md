# ADR-0002: Quality Gate Default and Cleanup Responsibility

- Status: accepted
- Date: 2026-07-10
- Scope: local push verification, project-specific quality gates, and branch cleanup

## Decision

An existing, valid `git-governance.quality.json` mandatorily activates the
local quality suite for every push that updates at least one official working
branch:

```text
feature  fix  docs  refactor  chore  test  perf  hotfix
```

`scratch/*` is not part of this suite by default. A repository may explicitly
include scratch in a single, fitting gate scope.

The configuration file itself is the opt-in. Without the file, no project- or
language-specific build, test, or lint assumption is made; the status then
reads `unconfigured`, never `passed`.

For a regular publication, the full local suite runs only after the branch
has been synchronized against its current target base. A passing run produces
a short-lived, revision-bound proof in the local Git metadata resolution under
`git-governance.final-quality-evidence`. The proof contains no secrets and is
not committed. It binds at least the outgoing ref and commit, the remote and
target base revision, the quality configuration digest, the gate selection,
the toolchain, a clean worktree, and the creation time.

The pre-push path continues to check every actual ref update structurally. It
may only reuse the proof when all bindings still match exactly and the proof
is fresh. If it is missing, expired, or mismatched, the full suite runs once
as a fallback. A corrupted or incomplete proof blocks fail-closed. Neither
`--no-verify` nor hook deactivation or an unbound skip switch is a permissible
deduplication.

This repository additionally uses the platform-neutral coverage gate through
the pinned tool (`go tool -modfile tools/go.mod check-coverage`). It runs
`go test -count=1 -cover ./...` and aborts when a Go package contains no
`_test.go` file or a package with executable statements does not reach exactly
`100.0 %`. The same gate runs locally, in the pre-push path, and in CI.

The CLI by default deletes only local `scratch/*` branches and removes their
local workflow base metadata in the process. It never deletes remote branches
and does not delete shared lines. The remote deletion of ticket and hotfix
branches lies with GitHub, GitLab, or an equivalent hosting automation. A
release branch is removed by CI or hosting automation only after promotion to
`main`, confirmed tag/artifact delivery, and completed reconciliation to
`develop`. An auditable `not-required` is a valid reconciliation completion
when no effective delta remains.

## Decision assessment

| Rank | Model | Absolute fit | Normalized share |
|---:|---|---:|---:|
| 1 | Repository configuration activates mandatory gates on all official working branches; scratch is an explicit opt-in | 94.0/100 | 53 % |
| 2 | Repository configuration activates gates only in explicit workflow commands | 82.0/100 | 46 % |
| 3 | The CLI contains hard-wired build/lint commands for every repository | disqualified | 1 % |

The chosen model wins because it prevents local bypass through a direct
`git push` without guessing a language, a build system, or a package manager.
It keeps the functional governance in the product core and the
project-specific commands in an explicitly verifiable repository contract.

The normalized shares apply only to these three options. They are neither
market shares nor general quality scores.

## Quality gate contract

```text
valid quality file
→ pre-push checks all actual Git ref updates
→ shared-line, rewrite, and base rules pass
→ a matching final proof is only used for exactly the same updates
→ otherwise entitled gates run once per push
→ the push may proceed
```

A gate may be scoped more narrowly through `includeFamilies` and
`excludeFamilies` — for example a documentation check only on `docs/*` or a
load test on `feature/*` and `perf/*`. This explicit scoping rule does not
change the default: on every official working-branch update, the configured
suite applicable to it runs.

Untrusted repository configuration is not executed. The runner only accepts
an executable with an argument array, a repository-relative working directory,
and a positive timeout. Shell strings and paths outside the repository root
are excluded.

## Cleanup contract

| Branch class | Local CLI deletion | Remote deletion | Responsible party |
|---|---|---|---|
| `scratch/*` | yes | not intended | Developer / CLI |
| regular ticket branches | no | after PR merge | Hosting platform |
| `hotfix/*` | no | after target merge and forwarding | Hosting platform / CI |
| `release/*` | no | only after main promotion, confirmed delivery, and completed reconciliation | CI / hosting automation |
| `main`, `develop`, active `support/*` | no | no | Branch protection |

The CLI must not claim a merge, pull request, or forwarding completion as long
as no authoritative hosting adapter provides that proof. A general local or
remote deletion function would cross this trust boundary.

## Tag lifecycle

```text
release/<semver> → pull request to main → protected merge
→ CI checks the main merge commit
→ CI creates the annotated immutable tag v<semver>
→ CI starts the artifact build, signing, and publication
→ the lifecycle adapter verifies promotion, tag, delivery, and the effective delta
→ with delta: release/<semver> → pull request to develop
→ without delta: auditable not-required
→ CI or hosting automation cleans up the completed release line
```

The local CLI creates neither tags nor direct `main`, `release/*`, or
`support/*` pushes. It exclusively creates provider-neutral intents that a
protected, authorized CI workflow executes.
