# Branch conventions — Resume branch derivation

[INTENT: REFERENCE]

## Canonical source

This document is the canonical source of truth for how a paused Git
operation identifies the branch its resume continues. It binds every
`--resume` endpoint of this binary, present and future, independent of the
workflow lane that created the paused operation.

## The law

1. **A resume never requires an explicit branch flag.** During a paused
   rebase, Git detaches HEAD by design: the working checkout no longer
   reports a branch name, while the branch itself is recorded verbatim in
   the paused operation state (`rebase-merge/head-name`, falling back to
   `rebase-apply/head-name`). The resume therefore derives the branch from
   that state and from nothing else.
2. **A branch flag supplied with `--resume` is rejected.** Any `--branch`
   value passed together with `--resume` is rejected fail-closed at the
   command boundary, mirroring the existing `--resume` rejection of
   `--strategy` and merge inputs: the paused state already carries the only
   possible branch, a confirming value verifies nothing, and a deviating
   value is by definition an input error.
3. **No paused rebase, no derivation.** When no paused rebase exists, the
   resume path continues to resolve the checked-out branch, which is the
   externally-completed-verification case the resume endpoints already
   support.

## Why an explicit branch flag is architecturally wrong

- The paused operation fixes the checkout: while a rebase is paused, Git
  cannot switch branches, so exactly one branch can ever be resumed. A
  manually supplied value carries zero additional information.
- A supplied value would become a second truth surface competing with the
  operation state, splitting one fact across two sources.
- Requiring it creates a fail-open trap: the conflict error instructs the
  user to run `--resume`, while the same binary's branch preflight rejects
  the detached state that the conflict produced. This failure mode is the
  design reason for the rule and is recorded here so the requirement never
  returns as technical debt.
- Accepting it as an optional cross-check keeps a second truth surface alive
  without a second source: the match path verifies nothing (input equals
  input), the mismatch path rejects an input that could never change the
  outcome. Rejection is the consistent consequence of the fixed-state
  argument and closes the fail-open trap completely.

## Scope

- `workflow ticket publish --resume`
- `branch sync-base --resume`
- every future endpoint that continues a paused Git operation

## Do / Don't

| Do | Don't |
|---|---|
| Derive the branch from the paused rebase state of the addressed worktree | Require a branch flag to resume a paused rebase |
| Reject any branch input supplied together with `--resume` | Keep a cross-check path that cannot change the outcome |
| Fail closed when the cross-check deviates, naming the derived branch | Guess a branch from the worktree path or a stale reference |
| Keep checked-out-branch resolution for the no-paused-rebase path | Run the detached-HEAD checkout preflight before resume resolution |

## Verification

A resume implementation complies when: a paused rebase resumes without any
branch input; any branch input supplied with `--resume` is rejected
fail-closed with a remediation that names the derivation; and no code path
asks a paused worktree for its checked-out branch.
