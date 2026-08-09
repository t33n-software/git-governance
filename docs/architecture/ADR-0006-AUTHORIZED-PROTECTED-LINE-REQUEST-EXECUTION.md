# ADR-0006: Authorized Protected-Line Request and Execution

- Status: Accepted
- Date: 2026-08-10
- Scope: Controlled creation of `release/<semver>` and `support/<major.minor>` lines
- Decision makers: Release governance

## Context

The prior protected-line workflow correlated a caller with
`create-protected-line.yml`, then waited for the child workflow to finish.
That design fails when the child waits for an environment approval longer than
the caller's bounded provider wait: the parent can report failure while the
child later creates the protected line successfully.

The problem is not solved by a manual deferred verification step. A regular
release cut needs distinct authorization of release scope, distinct
authorization of the one protected mutation, and an automatic independent
completion proof.

## Decision

Protected-line creation uses three controller responsibilities:

```text
Release Request Controller
  -> release-request environment authorization
  -> durable deployment-backed request record
  -> bound executor dispatch

Protected-Line Executor
  -> release-execution environment authorization
  -> validates request_id, ticket, source SHA, target, expiry and idempotency
  -> performs at most one protected ref mutation

Automatic Finalizer
  -> no reviewer environment
  -> verifies the correlated executor job and remote ref
  -> writes verified, failed or verification_pending
```

The immutable request payload binds:

```text
request ID
repository
ticket
operation and version
source ref and exact source SHA
target ref
requester and request-controller run
expected executor
expiry
idempotency key
provider deployment ID and audit states
```

The request controller uses a job-scoped token with only repository read,
workflow dispatch, and deployment-record permissions. It has no
`contents: write` permission. The executor receives only the ref-write and
request-status permissions needed for its bound mutation. The finalizer can
read workflows and refs and write request audit status, but cannot mutate a
Git ref.

`recover-protected-line-request.yml` is a read-only recovery path. It only
accepts a `verification_pending` request and never dispatches a new executor
or modifies a branch.

## Consequences

- A parent dispatch acknowledgement, child workflow success, or visible ref
  alone is no longer treated as a complete release-cut proof.
- Only `verified` is a regular protected-line-cut completion state.
- A local `workflow release cut --dispatch` or `support --dispatch` cannot
  bypass the request controller.
- Promotion, immutable tagging, artifact delivery, and reconciliation remain
  separate lifecycle decisions after a verified cut.
- GitHub must provision separate `release-request` and `release-execution`
  environments with the intended authorities before the new controller can be
  used in production.
- The current broker remains a separate credential boundary for broker smoke,
  release delivery, reconciliation, and other already-defined identities; the
  request, executor, and finalizer job tokens are scoped to their own jobs.
