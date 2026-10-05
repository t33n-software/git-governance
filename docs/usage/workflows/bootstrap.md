# Bootstrap a repository

Birth an unborn repository under the governed lifecycle:

```powershell
git governance workflow bootstrap `
  --key ABC `
  --ticket 1 `
  --stage .
```

The workflow:

1. proves the unborn state (no HEAD commit, no references), the unborn HEAD
   targeting `main`, the idle Git state, the empty index, the signing gate
   with its canary, the committer identity, the Lefthook executable and
   `lefthook.yml`, the policy snapshot, the ticket key policy, and the content
   boundary of the explicit content set — read-only and fail-closed;
2. creates the signed `chore(ABC-1): initialize the governed repository`
   commit on `main`, creates `develop` from the same revision, and installs
   the canonical `commit-msg` and `pre-push` hook boundary;
3. proves the born refs against the governed validation, the shared revision,
   the verified signature, and the materialized hooks;
4. emits the genesis record — repository, revision, refs, signature status,
   policy snapshot, time, and actor — as the root of the evidence chain.

The content set is explicit: `--stage` is repeatable and the CLI never stages
implicitly. The genesis commit carries the mandatory canonical body. A born
repository is refused fail-closed with `REPOSITORY_ALREADY_BORN`, and the
local genesis stays reversible until publication.

The genesis mutation is budgeted internally: the per-process timeout of the
staging, commit, and branch-creation steps derives from the proven
content-file count of the preflight (a base ceiling plus a per-file
allowance, bounded), so large content sets do not need a `--timeout`
override. An explicitly supplied `--timeout` remains the upper bound that
caps the derived budget.

Every aborted genesis step is compensated: when the mutation or the
finalizer fails, the tool restores the proven pre-birth state — an empty
index and no references — before it reports the failure, leaving the unborn
HEAD and the working tree untouched. The failure record carries the
compensation fact, and the retry of the same endpoint stays idempotent. A
compensation that cannot complete fails closed with a named record that
blocks the retry until the partial birth is resolved.

Publication is a separately confirmed step:

```powershell
git governance workflow bootstrap `
  --key ABC `
  --ticket 1 `
  --stage . `
  --push
```

`--push` requires a bound remote and pushes `main` and `develop` with upstream
configuration. Without a bound remote, `--push` is refused with a named
remediation before any mutation; without `--push`, the genesis completes
locally.

For automation:

```powershell
git governance --interactive never --output json --yes workflow bootstrap `
  --key ABC `
  --ticket 1 `
  --stage .
```

Use `--dry-run` to inspect the complete preflight result and the full plan
without mutating Git.

## Recover the unborn pre-state

Before the birth, a repository can carry a foreign or aborted pre-staging
state — a populated index or stray references — that the birth preflight
refuses fail-closed. The recovery subcommand restores the proven pre-birth
state:

```powershell
git governance workflow bootstrap recover
```

The recovery:

1. proves the unborn topology — no HEAD commit, the unborn HEAD targeting
   `main`, no active Git operation — read-only and fail-closed;
2. empties the index and removes every reference through the same governed
   restoration the birth compensation uses; the working tree is never
   modified, so every staged file returns to its untracked form;
3. proves the restored state through a read-back (empty index, no
   references).

A born repository is refused with `REPOSITORY_ALREADY_BORN` — a born
repository is never reset by the recovery. The mutation requires explicit
confirmation or `--yes`; `--dry-run` renders the plan with the state
reductions the proof found. The recovery is idempotent: a clean unborn
repository recovers with a no-op restoration.

## Resume the publication of a born repository

A repository born without `--push` carries its genesis locally while its
shared lines are absent from the remote. The publication resume completes the
publication:

```powershell
git governance workflow bootstrap publish
```

The resume re-proves the complete birth topology before pushing — exactly one
commit reachable from HEAD, `main` and `develop` both pointing at that shared
genesis revision, the verified genesis signature, and the materialized
canonical hook boundary — and requires a bound remote. Outside that proven
state it fails closed with `BIRTH_STATE_INVALID`; it never pushes anything
else. The publication is separately confirmed (`--yes` or the interactive
prompt), and the underlying push crosses the pre-push boundary like every
other shared-line publication. `--dry-run` renders the proof plan and the
proven revision without pushing.
