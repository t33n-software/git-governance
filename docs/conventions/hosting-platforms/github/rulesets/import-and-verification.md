# Import, activation, and verification
[INTENT: ANFORDERUNG]

## Prerequisites

1. The organization carries at least the **Team plan**; otherwise the
   organization ruleset surface does not exist, and push rulesets for private
   repositories remain dependent on it as well.
2. The `quality-gates` custom property is created at the organization level
   with the values `full` and `linux-only`, and every governed repository
   carries exactly one of the two values.
3. `.github/CODEOWNERS` is merged before shared-line rulesets with
   `require_code_owner_review` become active; without the contract, every
   shared-line PR blocks fail-closed.
4. A required check context is only enforced on a live line once the emitting
   workflow provably reports on a real PR against exactly that target line —
   including the no-change path, which MUST report success instead of
   pending.

## Import order

1. `00-push-protections.json` (binds only private/internal repositories)
2. `01-ticket-working-branches.json`
3. `02-develop.*` (both classes)
4. `03-main.*` (both classes)
5. `04-release.*` (both classes; check `do_not_enforce_on_create: true`)
6. `05-support.*` (both classes; check `do_not_enforce_on_create: true`)
7. `08-tag-namespace-floor.json` (active; no delivery lane affected)
8. `07-release-version-tags.json` (initially `disabled`; activation only
   after verified tag creation through the release-automation identity, see
   [Tag governance](tag-governance.md))

Graphically through **Organization Settings → Repository → Rulesets → New
ruleset → Import a ruleset**, or programmatically through
`POST /orgs/{org}/rulesets` with an organization owner identity. Tokens are
never passed through project files, command history, or source code.

## Activation

- On **Enterprise Cloud**, new or changed organization rulesets start in the
  **Evaluate** status; the Rule Insights evaluation shows what would have
  happened. Only after a clean evaluation is the switch to **Active** made.
- On the **Team plan**, the **Evaluate** status does not exist
  (Enterprise-only): the import happens after passing contract tests and
  selector review directly as **Active** in a controlled change window;
  monitoring runs through Rule Insights (the organization dashboard or the
  rule-suites API), and **Disabled** is the documented rollback state. A
  ruleset with an open activation precondition — for example
  `07-release-version-tags.json` before the verified app identity — is
  imported as **Disabled** and only activated after the precondition is met.
- An import alone does not mutate existing state: already active rulesets are
  updated through the UI or the REST API; after every merged change to the
  canonical sources, a re-import follows.
- Bypass actors are only configured for an audited release or emergency
  process, never as a convenient creation path.

## Verification checklist

- A merged `feature/*` PR deletes its remote head branch.
- The deletion protection prevents the deletion of `main`, `develop`,
  `release/*`, and `support/*`.
- A `develop` PR allows only merge, rebase, and squash; a PR onto `main`,
  `release/*`, or `support/*` allows only merge commits.
- A shared-line PR without the approval of the bound CODEOWNERS owner is
  blocked; open review threads block as well.
- The required checks report on every shared-line PR and block on failure; a
  PR without changes reports success instead of "pending".
- CodeQL results are present and blocking.
- On a private repository, the push ruleset rejects a push containing a
  secret- or key-shaped artifact (for example `.pem` or `.env`) — including
  the fork network.
- No repository carries a copy of a general ruleset outside a named exception
  scenario; every general ruleset exists exactly once at the organization
  level.
- The visibility selection of the push ruleset and the class selectors
  address exactly the intended repositories.
- A manual push of `refs/tags/v0.0.0-test` is rejected after the activation
  of `07`; the release-automation identity continues to create `v*` tags.
- `08` rejects the creation of a non-`v*` tag by a non-owner identity; an
  existing non-`v*` tag remains deletable.
- Every bypass in Rule Insights produces an audit entry (nowhere
  `bypass_mode: exempt`).
