# GitHub organization rulesets (canonical importable JSON)

This directory is the canonical source of truth for the organization-wide
GitHub rulesets of the `t33n-software` organization. The files are
organization ruleset bodies for:

- UI: **Organization Settings → Repository → Rulesets → New ruleset → Import a ruleset**
- API: `POST /orgs/{org}/rulesets`

They are managed once at the organization level and never redefined per
repository. Per-repository rulesets exist only as named, auditable exception
scenarios layered on top of this organization floor.

## Families and classes

The fleet is partitioned by the repository custom property `quality-gates`:

| Class | Property value | Meaning | Current members |
|---|---|---|---|
| full | `full` | Quality gates run on Linux, macOS, and Windows | `git-governance`, `license-hub` |
| linux-only | `linux-only` | Quality gates run on Linux only (Docker/CI/CD artifacts, no per-OS binaries) | all other governed repositories |

Class membership is assigned through the repository custom property, never by
editing a ruleset. The two class rulesets of the same shared line use mutually
exclusive selectors; organization rulesets aggregate and can only become more
restrictive, so a repository must never carry both classes. The property
definition itself is governed as a canonical artifact in
[`properties/github/`](../../properties/github/README.md) and is assigned per
repository only through the platform-instance bindings.

The classless rulesets apply to every governed repository:

- `00-push-protections.json` binds only private and internal repositories
  through the `visibility` system property, because push rulesets exist only
  there; public repositories rely on secret scanning with push protection plus
  the local quality gates. The push target covers the entire fork network and
  requires the Team plan on private repositories.
- `01-ticket-working-branches.json` binds every repository (`~ALL`) and
  protects published working-branch history (append-only after first push).

The classless tag rulesets (target `tag`) bind every repository (`~ALL`),
including public repositories — tag rulesets are not visibility-gated:

- `07-release-version-tags.json` binds the `refs/tags/v*` namespace to the
  release-automation GitHub App: creation, update, and deletion of version
  tags are restricted to that bypass identity, with the organization owner
  role as the named, audited break-glass path. Both bypass entries use
  `bypass_mode: always`, never `exempt`, so every bypass produces an audit
  entry. The bypass list is constitutive: without it the ruleset would block
  the governed release automation itself. The concrete app ID is this
  organization's reference binding of the logical release-automation
  identity; adopters substitute their own app ID exactly like `source`, and
  the steady-state projection binds the concrete ID from the instance
  bindings. Import it with `enforcement: disabled` until the governed tag
  workflows authenticate as the release-automation app; a tag push made with
  the repository `GITHUB_TOKEN` is rejected once the ruleset is active.
- `08-tag-namespace-floor.json` restricts creation and update of every
  non-`v*` tag (`refs/tags/*` minus `refs/tags/v*`) to the organization owner
  break-glass role, so no new ungoverned tag namespace can appear or move.
  It deliberately carries no `deletion` rule: existing non-`v*` tags remain
  cleanable, and the namespace can only shrink.

The shared-line rulesets `02-develop.*`, `03-main.*`, `04-release.*`, and
`05-support.*` require pull requests with `require_code_owner_review` against
`.github/CODEOWNERS` (default owner `@CyberT33N`), resolved review threads,
strict required status checks, and CodeQL code scanning with all alerts
blocking. `04-release.*` and `05-support.*` carry `do_not_enforce_on_create:
true` so the governed protected-line workflow can create a previously
nonexistent line; `02-develop.*` and `03-main.*` carry `false`.

## Naming contract

Ruleset title, repository selector, and file name form one machine-verifiable
triple: `<bounded-context>: <aggregate> [(quality-gates=<class>)]` ↔
`repository_property` value ↔ `<nn>-<line>[.quality-gates-<class>].json`.
Classless rulesets carry no class suffix and apply to the whole fleet.

## Import and activation order

1. Project the organization custom property `quality-gates` from its
   canonical definition artifact
   [`properties/github/quality-gates.json`](../../properties/github/README.md)
   (allowed values `full` and `linux-only` plus the onboarding value
   `pending`; `values_editable_by: org_actors`) and assign the class to each
   governed repository through the platform-instance bindings. A class
   ruleset whose selector property is missing or unassigned binds zero
   repositories — never activate the class rulesets before this step.
2. Import `00-push-protections.json` first, then
   `01-ticket-working-branches.json`, then the shared-line class variants in
   numeric order, then `08-tag-namespace-floor.json`, and finally
   `07-release-version-tags.json` (initially `disabled`; see above).
3. Start with **Evaluate** enforcement and review Rule Insights; switch to
   **Active** once the evaluation is clean. On the Team plan the `evaluate`
   status does not exist: import `active` in a controlled change window after
   the contract tests and selector review pass, monitor Rule Insights, and
   keep `disabled` as the documented rollback state.
4. A required status-check context may only be enforced on a repository whose
   workflows provably report that context on the exact target line; a
   repository that cannot emit a context must not carry the class property
   until its workflows are aligned.

Existing active rulesets are updated through the GitHub UI or the Rulesets
REST API; editing these files alone does not mutate the organization. After a
merged change to these sources, re-import the affected files.
