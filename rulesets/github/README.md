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
restrictive, so a repository must never carry both classes.

The classless rulesets apply to every governed repository:

- `00-push-protections.json` binds only private and internal repositories
  through the `visibility` system property, because push rulesets exist only
  there; public repositories rely on secret scanning with push protection plus
  the local quality gates. The push target covers the entire fork network and
  requires the Team plan on private repositories.
- `01-ticket-working-branches.json` binds every repository (`~ALL`) and
  protects published working-branch history (append-only after first push).

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

1. Create the organization custom property `quality-gates` with the allowed
   values `full` and `linux-only`, then assign the class to each governed
   repository.
2. Import `00-push-protections.json` first, then
   `01-ticket-working-branches.json`, then the shared-line class variants in
   numeric order.
3. Start with **Evaluate** enforcement and review Rule Insights; switch to
   **Active** once the evaluation is clean.
4. A required status-check context may only be enforced on a repository whose
   workflows provably report that context on the exact target line; a
   repository that cannot emit a context must not carry the class property
   until its workflows are aligned.

Existing active rulesets are updated through the GitHub UI or the Rulesets
REST API; editing these files alone does not mutate the organization. After a
merged change to these sources, re-import the affected files.
