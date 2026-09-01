# CLI Contract for `git-governance`

## 1. Invocation and naming

The release binary is called:

```text
git-governance
```

Git recognizes executables matching the pattern `git-<name>` as a subcommand. Therefore both forms are equivalent:

```text
git governance branch create
git-governance branch create
```

The primary documentation uses `git governance ...`.

The old names `mkbranch` and `mkcommit` do not become the target surface. A single root command is necessary because branches, commits, validation, configuration, and workflows use the same domain objects and the same release version. Functionally separate use cases remain separate subcommands.

## 2. Global options

```text
--interactive auto|always|never   default: auto
--output human|json              default: human
--quiet                          only necessary output
--color auto|always|never        default: auto
--accessible                     simplified screenreader surface
--remote <name>                  default: origin
--repo <path>                    default: current directory
--config <path>                  explicit configuration file
--quality-config <path>          explicit repository quality gate file
--pull-request-provider none|github
--dry-run                        show the plan, mutate nothing
--yes                            approve confirmable steps
--timeout <duration>             limit for external processes
```

Rules:

- `auto` starts forms only with a present TTY and missing mandatory values.
- `never` never reads interactively; missing values are a usage error.
- `always` fails clearly when no TTY is available.
- `always` is inadmissible with `--output=json`, because JSON contains no prompts.
- `--yes` does not replace missing functional values.
- `--quiet` changes neither interaction nor validation.
- `--color=auto` uses color only on terminal output; `always` enforces ANSI
  color and `never` uses plain text output.
- In JSON mode, prompts are forbidden; with `--interactive=auto`, JSON therefore behaves like `never`.
- Secrets are managed neither via flags nor via this configuration file.
- `--pull-request-provider=github` activates exclusively the GitHub adapter;
  it resolves a GitHub App session or a managed credential broker only
  immediately before the API call.
- `--create-pull-request` is an explicit workflow flag, additionally requires
  `--push` in publish workflows, and produces no silent fallback without a
  provider.

Interactive text fields show their full canonical contract before input. On a
functionally invalid input, the UI stays on this field: it shows the safe
actual value, the violated rule, the expected format, a valid example, and the
correction, and prompts for the same value again. There is no retry limit and
no return to the workflow start.

If a command fails only after accepted inputs, the human/JSON error output
contains an ordered input overview. The overview comprises the values used in
the command; security-marked values are redacted. On Git errors, `context` and
`diagnostic` are separated from the `actual` field so that the operation
context is not mistaken for user input.

`--quality-config` is not a language setting. It points to a
repository-local, explicitly trusted JSON contract of executable
command/argument arrays. If the file is missing, the result reads
`qualityStatus=unconfigured`; it is never reported as a passed build or lint.

When a valid configuration exists, `validate pre-push` determines the scope of
each gate against the actual branch families in the update stream. A gate
without its own scope inherits `defaults.includeFamilies`; a gate with
`includeFamilies` restricts itself to these families, and `excludeFamilies`
subtracts families afterwards. Every gate entitled thereby runs at most once
on a multi-ref push.

A final local quality run binds its proof to the outgoing revisions, the
target base revision, the remote, the configuration digest, the gate
selection, the toolchain, and a clean worktree. The proof resides only in the
local Git metadata resolution under
`git-governance.final-quality-evidence`, contains no credentials, and is not
committed. `validate pre-push` continues to always check all structural rules.
It uses the proof only on an exact, fresh match; with a missing, expired, or
mismatched proof, the repo-defined full suite runs once as a fallback.
Corrupted or incomplete proofs are rejected fail-closed.

The recommended default set contains all official working families:
`feature`, `fix`, `docs`, `refactor`, `chore`, `test`, `perf`, and `hotfix`.
`scratch` is therefore not selected by default but can be deliberately
included for a single lightweight gate. This is not a global special rule but
the same scope semantics as for documentation, test, performance, or stress
gates.

## 3. Flag help and value domains

The help is the authoritative contract surface of this CLI: everything the
validation knows is discoverable through the help before the call
(discoverable-closed). A human and an LLM agent must be able to derive a valid
call from the help alone. The canonical, language- and topic-agnostic
conventions reside in `docs/conventions/cli/`; this section binds their duties
to the concrete runtime of this project.

**Value class duty.** Every flag and every positional argument belongs to
exactly one of the eight value classes per
`docs/conventions/cli/value-domain-model.md`:

- `closed-enum`: the help shows 100 % of the values accepted for this
  endpoint (the endpoint subset, never the entire domain and never a hand
  copy).
- `free-constrained`: the help shows the compact rule set — character class,
  length, naming convention, regex/grammar, a canonical example, and the
  primary rejection conditions.
- `shaped`: the help shows the grammar, a canonical example, and the
  endpoint's subset rules.
- `structural-reference`: the help shows the form and the resolution rule and
  promises no runtime-independent full prevention.
- `scalar-bounded`, `boolean-switch`, `composite-token`, and
  `secret-reference` follow the compact duties of the convention.

**Label law.** The help bindingly distinguishes between "is rejected by the
validation" (a machine rule with a hard rejection) and "violates the
convention" (a content contract without machine enforcement).

**Single source of truth.** Every value domain is defined exactly once in the
canonical register of the application layer (`internal/application/cliparam`).
Static help, interactive prompts, error contracts, shell completion, and the
discovery endpoint `policy describe` render from this register. Literal
duplicates are forbidden; endpoint subsets are produced through declared
filters (e.g. `DirectlyCreatable`), never through hand copies. Drift between
projection and source is a release-blocking defect: contract tests pin every
projection against the register, property-based tests prove acceptance and
rejection per documented value and per documented rule, and a help-first
consumer test proves that a valid call is derivable from the help alone —
canonically laid down in
`docs/conventions/cli/testing-and-verification.md`.

**Deep reference.** The help of every value-carrying endpoint carries exactly
one reference line to the discovery endpoints `policy describe` (value domains
and limits) and `branch list` (family patterns).

## 4. Command tree

```text
git governance
├── branch
│   ├── list
│   ├── create
│   ├── validate
│   ├── merge-scratch
│   └── sync-base
├── commit
│   ├── create
│   └── validate
├── workflow
│   ├── ticket
│   │   ├── start
│   │   └── publish
│   ├── hotfix
│   │   ├── start
│   │   ├── validate-record
│   │   ├── verify-merge
│   │   ├── verify-delivery
│   │   ├── publish
│   │   ├── propagate
│   │   └── propagate-manifest
│   ├── release
│   │   ├── cut
│   │   ├── stabilize
│   │   ├── publish-stabilization
│   │   ├── align-promotion-base
│   │   ├── promote
│   │   ├── backmerge
│   │   ├── align-reconciliation-base
│   │   └── support
│   └── cleanup
├── validate
│   └── pre-push
├── auth
│   ├── login
│   │   └── github
│   ├── status
│   │   └── github
│   └── logout
│       └── github
├── config
│   └── key
│       ├── list
│       ├── add
│       ├── remove
│       └── set-default
├── policy
│   └── describe
├── completion
└── doctor
```

## 5. `auth`

```text
git governance auth login github
git governance auth status github
git governance auth logout github
```

`auth login github` is an explicit interactive GitHub App device flow. It
requires human output and a real TTY, prints only the verification URL and the
one-time code, and opens a browser exclusively in this command.
`--interactive never` and JSON output are invalid for it. The local client
stores exclusively a protected refresh session in the native operating system
vault; access tokens, refresh tokens, private keys, and client secrets are
never displayed, persisted, or accepted as a flag.

`auth status github` is not interactive and prints only the host, the account,
the credential source, the bound repository of the working context, and the
refresh expiry state. The session selection follows the canonical repository
identity of the working directory or `--repo`; outside a resolvable GitHub
repository context, the most recently used host session applies.
`auth logout github` deletes the local vault session bound to the selected
repository context, including the binding. A device flow session is not
revoked remotely, because a local client must not possess a GitHub App client
secret. The complete flow, the binding and discovery model, and the broker
contract reside in
[`docs/usage/authentication.md`](../usage/authentication.md).

## 6. `branch list`

Shows all branch families including shared lines and governance-bound lines:

- `main`
- `develop`
- `release`
- `support`
- `feature`
- `fix`
- `docs`
- `refactor`
- `chore`
- `test`
- `perf`
- `hotfix`
- `scratch`

Every entry contains:

- the role
- the naming form
- the permitted start base
- the typical PR target
- the protection/rewrite rule
- whether the family is created via `branch create` or a workflow

`branch list` is the complete information surface. `branch create` shows only
selectable families for the concrete context and explains why other families
must not be created directly.

## 7. `branch create`

### 5.1 Purpose

Creates exactly one branch from an explicitly determined base. The command
validates and mutates Git; it contains no ticket-to-PR overall workflow.

### 5.2 Options

```text
--family feature|fix|docs|refactor|chore|test|perf|scratch
--key <KEY>
--ticket <NUMBER>
--slug <kebab-case>
--base <remote-ref>
--switch                        default: true
```

Rules:

- If `--family` is missing interactively, a selection with an explanation of
  every family appears.
- For regular ticket families, the default base is `<remote>/develop`.
- After `fetch --prune`, this remote-tracking base must exist. If, for
  example, `origin/develop` is missing, the creation is rejected as
  `BRANCH_BASE_INVALID` with the missing base before Git attempts a branch
  switch.
- Before a real creation, the command checks after `fetch --prune` whether a
  local or selected remote-tracking branch already exists for the same ticket.
  A second regular official ticket branch is rejected.
- `hotfix` requires the actually affected base and is created via `workflow
  hotfix start`.
- `scratch` is created from a local official ticket branch of the same ticket;
  on direct selection, this base is prompted for.
- `scratch` accepts no remote-tracking reference, shared line, other scratch
  base, or ticket-foreign base.
- `release` and `support` refer to governance-bound workflow commands.
- `main` and `develop` are not selectable working branches.
- The command never executes `git add`, commit, amend, or force push.

### 5.3 Non-interactive example

```text
git governance branch create \
  --interactive never \
  --family feature \
  --key ABC \
  --ticket 123 \
  --slug add-export-button \
  --output json
```

The generated name is:

```text
feature/ABC-123-add-export-button
```

### 5.4 Mutation plan

Before confirmation or with `--dry-run`, a plan is displayed:

```text
Update remote: git fetch --prune origin
Check base: refs/remotes/origin/develop
Create branch: feature/ABC-123-add-export-button
Start point: origin/develop
Switch working branch: yes
```

### 5.5 `branch merge-scratch`

```text
git governance branch merge-scratch \
  [--branch scratch/<ticket>-<slug>] \
  [--target <official-ticket-branch>] \
  --type <commit-family> --subject <description> --body <text> \
  [--footer <TOKEN=VALUE>]... [--breaking [--breaking-description <text>]]
```

Without `--branch`, the current branch is the scratch source. The command only
accepts a local `scratch/*` branch and transfers its content as exactly one
squash commit into a local official ticket branch.

The target resolution uses the ticket ID, not the branch slug:

- `scratch/ABC-123-export-exploration` and
  `feature/ABC-123-add-export-button` belong together even though their
  descriptions differ.
- Exactly one local `feature`, `fix`, `docs`, `refactor`, `chore`, `test`,
  `perf`, or `hotfix` branch for the same ticket is used automatically.
- If this local branch is missing, the command ends with
  `SCRATCH_TARGET_BRANCH_MISSING`; a remote-tracking ref alone is not a
  mergeable local target branch.
- With multiple local candidates, `--target` is mandatory; the command does
  not guess between branch families.

Interactively, it first shows the scratch source and target branch as well as
the derived, non-editable ticket key and ticket ID. Then it shows the complete
canonical commit family view (`build`, `chore`, `ci`, `docs`, `feat`, `fix`,
`perf`, `refactor`, `revert`, `style`, `test`) and subsequently asks for the
description and the mandatory body. The description is exactly the non-empty,
unpadded text after `: ` in the produced header; it must not contain control
characters, may be at most 200 Unicode code points long, and must never carry
the metadata envelope in header form. The body documents the discarded
experiment paths of the scratch course.

For automation, the existing global options are authoritative:

```text
git governance --interactive never --yes branch merge-scratch \
  --type feat \
  --subject "add export button" \
  --body "## Motivation\n\nDocuments the discarded experiment paths."
```

The execution switches to the target, runs `git merge --squash`, and creates
the resulting ticket-consistent Conventional Commit. It never runs `git add .`,
push, or scratch deletion. The body is always mandatory on the scratch squash
transfer: it is the only place that documents the discarded experiment paths.
On a conflict, the normal Git conflict state remains for explicit resolution.
The direct command does not continue an automatic merge; within `workflow
ticket publish`, the same conflict state is instead continued through a retry
choice after the user has resolved and staged it.

## 8. `branch validate`

```text
git governance branch validate [<branch-name>]
```

Without an argument, the current branch is used. The command checks:

- the complete branch grammar
- the value object rules
- `git check-ref-format --branch`
- family-specific rules
- optionally the key policy/bundle
- with a present repository, the permissible working context

It mutates nothing and is suitable for local diagnostics and CI.

## 9. `branch sync-base`

### 7.1 Purpose

Determines whether the current official working branch misses commits of its
actual target base. The command replaces no merge queue and executes no blind
rebase. An optional `--branch` is an explicit expectation of the current
branch and must match it; the command never silently switches to another
branch.

```text
git governance branch sync-base \
  --strategy check|auto|rebase|merge

git governance branch sync-base --resume
```

For `--strategy merge`, the interactive surface produces the same structured
commit flow with the fixed branch ticket. Non-interactive calls use
`--merge-type <family>` and `--merge-subject <description>`; optionally
`--merge-body`, `--merge-footer`, `--merge-breaking`, and
`--merge-breaking-description` complement the merge commit.

`--resume` resumes a rebase or merge paused by this command after all conflict
paths have been resolved and explicitly staged. The re-entry deliberately
accepts no strategy, no dry run, and no merge commit inputs: the active Git
operation determines the continuation. Before resuming, the command checks the
branch grammar, the publication state, and the active operation; a merge is
additionally only resumed when no unresolved conflicts remain open and it
still points to the fetched base revision. After resuming, base freshness,
branch validation, and the configured quality checks run again.

### 7.2 Decision logic

1. check the current branch and the optional `--branch` against each other
2. determine the actual target base
3. check a clean worktree
4. `git fetch --prune <remote>`
5. check the publication state
6. determine missing base commits
7. apply the policy

| State | Result |
|---|---|
| no missing base commits | `BASE_UP_TO_DATE`, no mutation |
| unpublished with a base delta | rebase permitted |
| published with a base delta | rebase forbidden; optionally a controlled merge |
| publication state unknown | block the history rewrite |
| shared line or scratch | its own family contract |

`auto` does not mean "always mutate":

- unpublished: rebase only on a delta
- published: without an explicit merge approval, only output the action plan

After a mutation, the governance checks and the configured quality checks run
again. If a direct `branch sync-base` rebase or merge fails due to a conflict,
Git remains in the normal rebase or merge state and is not hidden. After
resolution and explicit staging, `branch sync-base --resume` resumes the
paused operation in a governed manner. In `workflow ticket publish`, the same
state is additionally presented as a retry step: after resolution and staging,
retry resumes the existing rebase.

## 10. `commit create`

### 8.1 Options

```text
--type build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test
                                 mandatory in non-interactive mode
--ticket <KEY-NUMBER>            compatibility check against the branch
--subject <text>                 commit description (never with the metadata envelope)
--body <text>                    mandatory with breaking, on the hotfix lane, on
                                 release stabilization, and on the scratch squash transfer
--breaking
--breaking-description <text>
--footer <token=value>           repeatable
--stage <path>                   repeatable
--push                           default: false
```

### 8.2 Defaults and derivations

- The ticket is derived from the branch name on a ticket branch.
- An explicit `--ticket` must exactly match the branch and does not change the
  derived scope.
- The commit type is always an explicit decision: non-interactively `--type`
  is mandatory, and if it is missing, the call fails closed; a silent
  derivation from the branch family does not exist.
- Interactively, the fixed branch, key, and ticket ID context line is shown,
  then the canonical commit family view with the branch family expectation as
  the preselection, and finally the description. The preselection is a
  proposal that the author explicitly confirms or changes. Key and ticket are
  not selectable in this flow.
- The description never carries the metadata envelope (type, ticket, breaking
  marker) in header form — neither at the start nor embedded; the validation
  rejects envelope content fail-closed. The canonical conventions reside in
  `docs/conventions/commits/subject-contract.md` and
  `docs/conventions/commits/family-selection.md`.
- The body is the default: with `--breaking`, on the hotfix lane, on a release
  stabilization branch, and on the scratch squash transfer it is mandatory,
  and if it is missing, the creation fails closed.
- The command checks whether changes are staged.
- Without `--stage`, `git add .` is never executed automatically.
- `--stage` accepts explicit paths and shows them before the mutation.
- `--push` is optional and runs through the same pre-push validation as
  Lefthook.

### 8.3 Breaking changes

With `--breaking`, the UI produces by default:

```text
feat(ABC-123)!: replace the export contract

## Motivation

The export contract changed incompatibly.

BREAKING CHANGE: clients must consume the new resource envelope.
```

A breaking commit without a body is inadmissible: the migration impact resides
in the footer, the functional reason in the body.

The user receives an explanation:

- Breaking means an incompatible public contract change.
- The marker must not be misused for internal refactors.
- The description must name the concrete migration impact.

### 8.4 Amend and force push

`commit create` offers no amend flag. Before the first push, a local amend
would in principle be permissible per the reference governance, but it is not
a necessary product use case. After the first push, amend is forbidden as a
routine. Force push is offered by no command.

## 11. `commit validate`

```text
git governance commit validate --message-file <path>
git governance commit validate --message <text>
```

Checks:

- header grammar
- commit type
- ticket ID
- the description including envelope freedom (no metadata envelope in the
  subject)
- body/footer structure
- breaking-change semantics
- ticket consistency with the current branch
- shared-line rules
- optional key policy

For `commit-msg`, `--message-file` is always used. The file is read bounded;
NUL and inadmissible control characters are rejected.

## 12. `workflow ticket start`

### 10.1 Purpose

Starts regular ticket work and ends on the official or the optional scratch
branch.

```text
git governance workflow ticket start \
  --family feature \
  --key ABC \
  --ticket 123 \
  --slug add-export-button \
  --scratch
```

### 10.2 Flow

1. Check the repository and the Git version.
2. Check the worktree and running Git operations.
3. Validate the ticket inputs.
4. `git fetch --prune origin`.
5. Create the official branch directly from `origin/develop`.
6. Optionally show the scratch question with an explanation.
7. On approval, create `scratch/<ticket>-<scratch-slug>` from the official
   branch.
8. End on the chosen branch.

`--scratch` explicitly creates a private exploration. Without the flag, the
interactive mode asks; non-interactively, no scratch branch is created without
the flag.

### 10.3 Scratch explanation in the UI

The UI must display, in effect, before the selection:

```text
Scratch branches are private, short-lived exploration lines.
Use scratch only when the solution path or experiment is uncertain.
Do not create a pull request from scratch and do not share it as an official
working branch. Transfer stable results later in a controlled manner via
squash or cherry-pick into the official ticket branch.
```

## 13. `workflow ticket publish`

This command is called after development and local tests. It is not an
automatically continuing part of `ticket start`.

```text
git governance workflow ticket publish \
  --push --draft
```

Flow:

1. resolve the current branch
2. on `scratch/*`, determine the local official target branch via the ticket
   ID, show the scratch, the target, and the squash commit, and confirm
3. with a confirmed scratch path, execute the same `branch merge-scratch` use
   case and continue on the official branch
4. check the official ticket branch and a clean state
5. validate the branch and the commit series
6. check base freshness
7. rebase with an unpublished branch and a base delta
8. after a rebase, run the branch/policy check and the commit series again
9. run the project-defined full suite on the final publish candidate and
   produce the revision-bound local proof
10. in the interactive view, show whether a rebase happened or why it did not
11. with a paused scratch squash or rebase, resolve the conflicts and stage;
    the matching retry choice resumes exactly this Git operation instead of
    starting the workflow anew
12. confirm interactively before the first push or set `--push` explicitly
    non-interactively
13. check the pre-push policy against the actual update and reuse the proof
    only on an exact binding
14. after a push with a configured provider, confirm the PR creation
    interactively; non-interactively set `--create-pull-request` explicitly;
    without a provider, only output the provider-neutral PR intent

For a scratch start, the non-interactive mode needs a commit family, a
description, the mandatory commit body, and the existing mutation approval:

```text
git governance --interactive never --yes workflow ticket publish \
  --type feat \
  --subject "add export button" \
  --commit-body "## Motivation\n\nDocuments the discarded experiment paths." \
  --push
```

`--target <official-ticket-branch>` is only permissible on `scratch/*` and
resolves manual ambiguity. On an official branch, `--target` and the scratch
transfer inputs `--type`, `--subject`, `--commit-body`, `--commit-footer`,
`--commit-breaking`, and `--commit-breaking-description` remain invalid.

Without a provider adapter, no hosting API call is invented. The JSON output
is a stable handover surface for GitHub, GitLab, Bitbucket, or other adapters.
A user confirmation can therefore only produce a real PR when such an adapter
is configured at runtime.

For a GitHub PR, the automation sets `--pull-request-provider github`,
`--push`, and `--create-pull-request`. It uses an already existing GitHub App
session or a CI credential broker; it never starts a browser and accepts no
static GitHub token. The adapter derives host, owner, and repository from the
selected Git remote, verifies the exact repository authorization, and returns
an already open similar PR idempotently.

After manual conflict resolution and staging, the continuation is also
available without a TTY:

```text
git governance --interactive never --yes workflow ticket publish \
  --branch feature/ABC-123-add-export \
  --resume --push
```

On `scratch/*`, the original `--type`/`--subject` and `--commit-body` inputs
remain required; with ambiguity, `--target` remains mandatory.

## 14. `workflow hotfix start`

Mandatory options:

```text
--key <KEY>
--ticket <NUMBER>
--slug <slug>
--affected-line main|release/<semver>|support/<major.minor>
```

Flow:

1. validate the affected line
2. `fetch --prune`
3. check the remote line and a clean worktree
4. create `hotfix/<ticket>-<slug>` directly from the affected remote line
5. set the target PR onto the same line

A hotfix never starts automatically from `develop`.

### 13.1 `workflow hotfix publish`

```text
git governance workflow hotfix publish \
  --affected-line main|release/<semver>|support/<major.minor> \
  --push
```

The command requires the actually affected line again, validates the hotfix
against the same base, and produces the PR intent onto exactly that line. A
hotfix is never silently rerouted to `develop`. `--create-pull-request` is
only permissible together with `--push`. After a manual rebase conflict
resolution, `--resume` resumes the same hotfix publication without interactive
inputs.

### 13.2 `workflow hotfix validate-record`

```text
git governance workflow hotfix validate-record \
  --branch hotfix/<KEY-NUMBER>-<slug> \
  [--record .git-governance/hotfix-release-records/<KEY-NUMBER>.json]
```

The read-only command only loads the ticket-bound JSON record from the
controlled repository directory. It requires schema version 1, a main hotfix,
a stable patch successor of the previous tag, the exact hotfix PR binding, an
ordered complete SHA manifest, and declared additional propagation targets.

### 13.3 `workflow hotfix verify-merge` and `verify-delivery`

```text
git governance --pull-request-provider github workflow hotfix verify-merge \
  --branch hotfix/<KEY-NUMBER>-<slug>

git governance --pull-request-provider github workflow hotfix verify-delivery \
  --branch hotfix/<KEY-NUMBER>-<slug>
```

`verify-merge` checks the merged same-repository main PR, the exact GraphQL
merge commit, the ordered commit manifest, and the absence of the new
immutable tag. `verify-delivery` additionally checks that the tag points
exactly to the merge, that a non-draft GitHub release with payload, checksums,
SBOM, and the Sigstore bundle exists, and that the artifact workflow was
successful. Both commands are read-only and receive their short-lived identity
only in the protected controller.

### 13.4 `workflow hotfix propagate`

```text
git governance workflow hotfix propagate \
  --target-line main|develop|release/<semver>|support/<major.minor> \
  --commit <sha> \
  --push
```

The command creates a controlled `fix/*` branch from the target line, runs
`git cherry-pick -x <sha>`, and prepares the PR against this target line.
Thereby the provenance of a forward or backport remains provable. With a
paused cherry-pick, the user resolves the conflicts and then resumes with
`--source`, `--target-line`, the produced `--branch`, and `--resume`.
`--commit` is not required again when resuming.

### 13.5 `workflow hotfix propagate-manifest`

```text
git governance workflow hotfix propagate-manifest \
  --source hotfix/<KEY-NUMBER>-<slug> \
  --target-line develop|release/<semver>|support/<major.minor> \
  [--publish]
```

The command accepts exclusively a target line declared in the verified record.
It produces a workflow-managed `fix/*` candidate, stores its local resume
cursor in Git metadata, applies the declared SHA series in the given order,
and runs the quality suite. On a conflict, `--resume` requires the identical
source, target, and candidate branch. `--push` and `--create-pull-request`
remain deliberately unavailable. `--publish` is only permissible within the
protected hotfix propagation publisher controller: it requires the dedicated
broker workload identity, creates the candidate from the declared target line,
re-verifies it, pushes only the non-shared `fix/*` branch, and creates its PR.
Without this server-side boundary, `--publish` ends fail-closed; local
candidates remain non-publishing.

## 15. Release commands

### 13.1 `workflow release cut`

```text
git governance workflow release cut \
  --version 2.8.0
```

The command:

- requires an explicit governance confirmation
- checks the local release request and produces a machine-readable intent for
  the protected release request controller
- creates, switches, or pushes no local `release/*` branch
- rejects a normal `--dispatch` outside a dry run; an unbound local CLI call
  must not start the protected-line executor
- deliberately remains at an intent without `--dispatch`; it can prove no
  subsequent release state
- explains the limited stabilization permitted afterwards

### 13.1.1 Controller-internal release request and finalizer commands

The following subcommands are not local normal operations. They require the
explicit, short-lived GitHub Actions controller mode and a job-scoped
`GITHUB_TOKEN`:

```text
workflow release request
workflow release execute-request
workflow release finalize-request
```

`request` binds the ticket, the operation, the version, the source ref, the
exact source SHA, the target ref, the requester, the parent run, the expiry,
and the idempotency to a durable provider record. `execute-request` accepts
exclusively its `request_id` and the correlated executor run.
`finalize-request` checks read-only the executor and the actual remote ref and
only writes the audit status. `--recovery` is exclusively permissible for an
existing `verification_pending` record; it never starts a new mutation.

The regular flow is triggered by `release-control.yml` with
`operation=release-request` and the separate environments `release-request`
and `release-execution`. Only `verified` is a complete protected-line cut
proof.

The CLI models no generic `release` collection lane. The protected workflow
binds request, execution, credential verification, regular delivery,
reconciliation, hotfix delivery, and hotfix propagation each to its own
functional lane. Only the workflow lane with the concrete credential issuer
need receives its lane-specific OIDC/WIF and invocation variables; the local
controller commands accept no such credentials as inputs.

### 13.2 `workflow release stabilize`

```text
git governance workflow release stabilize \
  --release release/<semver> \
  --kind blocker|docs|release-prep \
  --key <KEY> --ticket <NUMBER> --slug <kebab-case>
```

Only the three named categories are permissible on a frozen release line. New
features, general refactors, and off-topic tickets have no selectable
stabilization category.

### 13.3 `workflow release publish-stabilization`

This command validates a stabilization branch against
`origin/release/<semver>` and produces its PR intent onto the same release
line. `--create-pull-request` requires `--push`; after a manual rebase
conflict resolution, `--resume` resumes the existing stabilization.

### 13.4 `workflow release align-promotion-base`

```text
git governance --pull-request-provider github workflow release align-promotion-base \
  --release release/<semver> \
  [--branch chore/<KEY-NUMBER>-<slug>] \
  [--resume] \
  [--push --create-pull-request]
```

The command is exclusively permissible for a `chore/*` release preparation
branch created by `workflow release stabilize --kind release-prep` from the
given `release/<semver>` line. It checks the stored release base, requires the
checked-out branch, and executes exclusively there a ticket-scoped merge of
`origin/main`. After the quality suite, it can push the working branch and
create its PR back onto the release line. Thereby a strict main ruleset
fulfills the freshness check without `Update branch`, rebase, or direct
mutation of a shared line.

On a conflict, the running merge operation remains on the same non-shared
preparation branch. `--resume` requires an active conflict-free merge
operation with explicitly staged resolution paths, compares `MERGE_HEAD` with
the `origin/main` current as of the fetch, resumes only this merge, and
re-checks the main base before quality, push, and PR. If main has evolved,
the candidate ends fail-closed. `--resume` is not permissible with
`--dry-run`.

### 13.5 `workflow release promote`

This command produces the provider-neutral PR intent:

```text
release/<semver> -> main
```

Tagging and artifact creation follow only after the protected merge in the
release pipeline. The CI workflow creates `v<semver>` directly on the merge
commit and then starts the artifact workflow for exactly this immutable tag. A
real provider PR is only possible with `--pull-request-provider github
--create-pull-request` and the explicit mutation approval.

### 13.6 `workflow release backmerge`

It produces no silent direct commits and no empty ritual PR. Outside a dry
run, it requires a configured release lifecycle provider that proves before
the PR creation:

1. `release/<semver> -> main` was merged;
2. `v<semver>` points exactly to this promotion merge;
3. the required GitHub release and artifact delivery succeeded;
4. an effective release delta is still missing in `develop`.

```text
release/<semver> -> develop
```

If the delta is present, it delivers `status=required` and creates the PR with
`--create-pull-request`. If no effective delta is present, the command
delivers `status=not-required`, the delivery proof, and no PR. A real provider
PR follows the same explicit GitHub adapter configuration as the promotion.

### 13.6.1 `workflow release align-reconciliation-base`

This workflow handles exclusively a backmerge whose target policy requires a
current pull request head. It only accepts a currently checked-out,
ticket-bound `chore/*` preparation branch with the stored workflow base
`origin/release/<semver>`.

The workflow verifies the complete release delivery and the effective delta,
verifies the current `origin/develop` base, merges it exclusively into the
preparation branch, runs the quality gates, and optionally publishes a
merge-commit PR to `develop`.

On a conflict, the normal merge state remains in the non-shared, ticket-bound
preparation branch. After explicit resolution and staging of the exact
conflict paths, `--resume` resumes exclusively this merge; the command makes
no automatic `ours`/`theirs` decision.

```text
git governance workflow release align-reconciliation-base \
  --release release/<semver> \
  [--branch chore/<KEY-NUMBER>-<slug>] \
  [--resume] \
  [--prepared] \
  [--push --create-pull-request]
```

`--resume` and `--prepared` are mutually exclusive:

- `--resume` requires an active, conflict-free, and fully staged merge in the
  same local preparation branch. After the merge, `origin/develop` must still
  be contained; otherwise the candidate branch is rejected fail-closed.
- `--prepared` is the server-side recovery entry for an already
  conflict-cleaned, pushed preparation branch. Missing local workflow metadata
  is only permissible when the CLI independently proves that HEAD has exactly
  the unchanged release ref as the first and the current develop ref as the
  second merge parent.

An arbitrary branch name never suffices as a recovery proof. The protected
`reconciliation-resume` workflow revalidates the ticket, the slug, the branch
grammar, the merge provenance, the delivery, and the quality before it creates
a PR to `develop`.

The server-side publication uses a dedicated reconciliation publisher identity
from the `release-reconciliation` environment. The local resolution workspace
does not receive this identity. The publisher identity is restricted to the
validated `chore/*` candidate and its PR; it holds no ruleset bypass and no
direct shared-line mutation permission. `release/<semver>` remains unchanged.
In a dry run, no CLI workflow executes a fetch, merge, push, provider
preflight, or provider publish.

### 13.7 `workflow release support`

`support/<major.minor>` may only be requested when the currently fetched
`origin/main` revision carries a matching `v<major.minor.patch>` release tag.
The CLI produces an intent. The normal execution then happens only through the
protected release request controller with `kind=support`; the request binds
the approved main revision before the separate execution workflow may create
the remote support line.

### 13.8 `workflow cleanup`

`workflow cleanup` never deletes remote branches. Remote deletion and
lifecycle proofs belong to GitHub, GitLab, or CI:

- Ticket and hotfix remote branches are removed by the hosting platform after
  the matching PR merge.
- A release remote branch is preserved until the main promotion, the
  tag/artifact workflow, and the completed reconciliation to `develop`. A
  `status=not-required` is thereby a valid completion without an empty
  backmerge PR; afterwards, hosting automation or CI deletes it.
- `main`, `develop`, `release/*`, and `support/*` are never local CLI cleanup
  targets.

The CLI permits exclusively local `scratch/*` cleanup and removes their local
workflow base metadata. Official ticket, hotfix, release, and support branches
are no local CLI cleanup targets. The command does not claim to be able to
prove a hosting merge or forward/backport completion.

## 16. `validate pre-push`

This command is the Lefthook and manual pre-push surface.

```text
git governance validate pre-push \
  --remote origin
```

It reads the ref list supplied by Git bounded from stdin and checks:

- every four-field Git ref update instead of only the currently checked-out
  branch
- target branch grammar and the key policy
- the shared-line push prohibition
- commit-ticket consistency
- deletions, non-fast-forward/rewrite attempts, and multiple updates
- bundle presence and freshness once the bundle adapter is active
- baseline freshness before the first push
- the final local quality proof only with a matching outgoing revision, base,
  configuration, toolchain, gate selection, and a fresh clean worktree state

The validator never executes a rebase or merge itself. If a matching final
proof is missing, the configured full suite runs as the local raw-push
fallback. It blocks with a concrete, policy-compliant instruction.

## 17. Configuration commands

```text
git governance config key add PLATFORM2
git governance config key list
git governance config key set-default PLATFORM2
git governance config key remove PLATFORM2
```

Rules:

- only syntactically valid keys are stored
- storage is deduplicated and recoverable in a platform-appropriate manner
- a stored key is not automatically registry-admitted
- ticket numbers are not stored as a global default
- commits derive the ticket from the current branch

## 18. `policy describe`

Outputs the active executable policy versioned:

```text
git governance policy describe --output json
```

Included are:

- the policy schema version
- the branch families
- the commit types
- the regex/grammar IDs
- the active key policy (`syntax-only` or `bundle`)
- the technical limits
- the error codes

Documentation and conformance tests use this output so that no second regex
truth arises in hooks or examples.

## 19. `doctor`

Read-only diagnostics:

- supported operating system and architecture
- Git present and minimum version met
- repository detected
- remote present, without disclosing its URL in the human output
- Git transport authentication through a non-interactive `push --dry-run
  --porcelain` of the current branch
- user configuration readable
- Lefthook present
- Lefthook configuration present
- policy bundle status, when enabled
- no running merge/rebase/cherry-pick operation

A missing Git transport authentication is a classified error, not merely a
warning check. The dry run contacts the remote but changes no remote
reference, skips Git hooks, and must not open a credential prompt. GitHub App
API sessions remain separate from this and are checked through
`auth status github` as well as the publish preflight.

`doctor` installs, repairs, or mutates nothing without a separate explicit
command.

## 20. Human and JSON output

### 19.1 Human

After a successful `git fetch --prune <remote>`, the interactive human
completion message begins with:

```text
🟢 Remote references fetched and stale references pruned from <remote> before this operation.
```

The display is only printed after an actually successfully completed fetch,
not with `--dry-run`, `--interactive=never`, JSON output, or `--quiet`. Fetch
updates the configured remote-tracking references; a local branch is not
pulled or switched by it.

Error presentation:

```text
Error [COMMIT_TICKET_MISMATCH]

Actual value:
  ABC-124

What is wrong?
  The commit uses ABC-124, but the current branch belongs to ABC-123.

Expected:
  All commits of an official ticket branch use its ticket ID.

Valid example:
  feat(ABC-123): add export button

How to fix it:
  Use ABC-123 or switch to the branch belonging to the commit.
```

### 19.2 JSON

```json
{
  "schemaVersion": 1,
  "ok": false,
  "error": {
    "code": "COMMIT_TICKET_MISMATCH",
    "category": "governance",
    "field": "ticket",
    "actual": "ABC-124",
    "expected": "ABC-123",
    "rule": "commit ticket must equal branch ticket",
    "example": "feat(ABC-123): add export button",
    "remediation": "use ABC-123 or switch branches"
  }
}
```

JSON field names and exit codes are public contracts and are versioned
compatibly.

## 21. Internal composition

Delivery adapters collect inputs and produce commands. Workflows call
application services directly:

```text
Cobra/Huh
→ StartTicketWorkflow
  → FetchRemote
  → CreateBranch
  → optional CreateScratchBranch
→ Reporter
```

Not permissible:

```text
workflow command
→ starts git-governance branch create as a child process
→ parses its text output
```

Only external consumers and automation use the CLI surface.

## 22. Adoption from the previous tool

| Existing capability | Target decision |
|---|---|
| interactive branch selection | adopt, but with the complete canonical taxonomy |
| interactive commit type selection | adopt and complete |
| ticket key history | adopt as an OS-conformant user preference |
| ticket number input | adopt, but do not reuse globally |
| branch slug input | adopt with stricter kebab-case |
| confirmation before mutation | adopt plus `--dry-run` |
| switch to the new branch | adopt |
| optional push after a commit | only explicitly and through pre-push validation |
| checkout and pull of `develop` | replace with fetch and a direct base reference |
| dev/user suffixes in branch names | discard |
| automatic initial commit | discard |
| custom hook copy scripts | replace with the Lefthook standard |
| parallel PowerShell/shell logic | discard completely |
