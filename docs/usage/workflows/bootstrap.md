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
