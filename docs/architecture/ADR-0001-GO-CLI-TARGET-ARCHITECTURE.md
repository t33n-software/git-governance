# ADR-0001: Native Go CLI as the shared Git governance application

- Status: accepted and implemented in the local product core
- Date: 2026-07-10
- Decision type: new target architecture
- Product name: `git-governance`
- Primary invocation: `git governance ...`
- Direct invocation: `git-governance ...`

## 1. Result

The target architecture is a modular Go monolith as a single native binary. Branches, commits, workflows, and all associated syntactic validations use the same domain core. Lefthook remains the bindingly prescribed thin local hook orchestration and calls validation use cases of the same binary. CI uses the same machine-readable validation surfaces; server-side branch protection remains the binding final authority.

This produces neither parallel PowerShell/shell truths nor regex duplicates in hook files.

## 2. Standalone product contract

This architecture decision is the complete local authority for
`git-governance`. It defines the product boundaries, the branch and commit
conventions, the workflow, the installation form, and the verification
requirements itself. No external rule files are needed for use or further
development.

The product contract stipulates in particular:

- a single native Go binary for Windows, macOS, and Linux,
- a shared domain core for branches, commits, and workflows,
- the complete branch taxonomy from `main` to `scratch/*`,
- Conventional Commits with a mandatory ticket scope,
- `fetch --prune` and direct remote bases instead of local pull workarounds,
- a publication-dependent rebase and rewrite policy,
- Lefthook as the thin local hook runner,
- CI and remote branch protection as the binding enforcement authorities,
- an isolated syntax key policy with a later replaceable bundle adapter,
- prebuilt, signed release artifacts instead of compilers on end devices.

The existing shell/PowerShell code was regarded exclusively as an inventory
of existing capabilities. Its naming, structure, validation, user interface,
and installation logic are no target authority.

## 3. Classification

- Primary project class: cross-platform CLI
- Secondary classes: developer tooling, Git automation, local policy validation
- Business capability: rule-compliant Git work from ticket through branch and commit to PR preparation
- Execution: Windows, macOS, and Linux, interactive as well as non-interactive
- Delivery: signed native binary per operating system and architecture
- State: Git repository, user preferences, and in the future a local policy bundle
- Critical side effects: `fetch`, branch creation, `switch`, commit, push, and conditionally rebase or merge
- Non-goals: replacement for Git, Git hosting, CI, server-side branch protection, or the policy registry

## 4. Corrections to the initial assumptions

### 4.1 `develop` does not have to be checked out and pulled

For a new regular ticket branch, the following flow is canonical:

```text
git fetch --prune origin
git switch -c feature/ABC-123-add-export-button origin/develop
```

`git fetch` updates the remote-tracking reference `origin/develop`. The branch can be created directly from it. A preceding `git switch develop` with `git pull` unnecessarily mutates the local `develop` and fails more easily on local drift.

### 4.2 Rebase is not a general later workflow

A rebase is only permissible when all conditions are met:

1. The official working branch has never been published.
2. `git fetch --prune origin` was successful.
3. The actual target base is known.
4. `git log --oneline HEAD..origin/<target-base>` shows missing base commits.
5. The worktree is clean.
6. After the rebase, the configured validations run again.

After the first push, the official branch is append-only. Then routine rebase, amend, and force push are not permissible; required base changes are merged in a controlled manner. Private `scratch/*` branches are the explicitly separated exception.

### 4.3 Internal modules do not call their own CLI

Flags are the external automation surface. Internally, a workflow orchestrates application services directly in the same process. A self-invocation of the binary would produce process recursion, string/exit-code coupling, and a second error translation.

### 4.4 Non-interactive is not "silent"

The three axes remain separate:

- Interaction: `--interactive=auto|always|never`
- Output format: `--output=human|json`
- Output volume: `--quiet`

`--interactive=never` requires all necessary values as arguments or flags. "Silent" would be ambiguous and must not mean input mode and output format at the same time.

### 4.5 A continuous ticket-to-PR wizard is functionally wrong

Between branch start and pull request lies the actual development with commits, tests, and review preparation. Therefore two atomic, resumable use cases exist:

- `workflow ticket start`: fetch the current base, create the official branch, optionally create and switch to a scratch branch
- `workflow ticket publish`: validate the local state, check base freshness, prepare or execute the first push, and produce provider-neutral PR data

There is no long-lived workflow process and no hidden session state machine.

### 4.6 Release promotion base alignment is its own workflow

A structurally outdated `release/<semver> -> main` promotion is not a normal
`sync-base` case. The PR head can itself be a protected release line; a GitHub
**Update branch** or rebase would mutate this shared line directly.

The binary therefore treats this case as a bounded release preparation
workflow: a ticket-bound `chore/*` working branch is derived from the release
line, absorbs the approved main ref via an append-only merge, passes the
quality gates, and returns to the release line via PR. Only then is the
existing promotion PR re-verified against `main`. The canonical decision and
its provider-neutral boundary are defined in
[ADR-0003](ADR-0003-GOVERNED-RELEASE-PROMOTION-BASE-ALIGNMENT.md).

## 5. Hard gates

| Gate | Go binary + Lefthook | Two Go binaries + Lefthook | Remote policy service + client | Lefthook-only |
|---|---|---|---|---|
| Windows/macOS/Linux | PASS | PASS | PASS | CONDITIONAL |
| No required language runtime | PASS | PASS | PASS | PASS for Lefthook itself |
| One functional truth | PASS | PASS with a shared package | PASS | FAIL with YAML/scripts |
| Full interactive CLI | PASS | PASS | PASS | FAIL |
| Full non-interactive CLI | PASS | PASS | PASS | FAIL |
| Offline-capable `commit-msg` path | PASS | PASS | CONDITIONAL with cache | PASS only as a runner |
| Canonical Lefthook standard | PASS | PASS | PASS | PASS |
| Branch/commit/workflow creation | PASS | PASS | PASS | FAIL |
| Signable native delivery | PASS | PASS | PASS | not applicable to a missing domain app |

`Lefthook-only` is disqualified before the MCDA: Lefthook can execute arbitrary jobs but implements neither the domain model nor the required branch, commit, configuration, and workflow surface.

## 6. MCDA

The assessment uses the weights of the Architecture Decision Blueprint.

| Criterion | Weight | One Go binary | Two Go binaries | Remote service + client |
|---|---:|---:|---:|---:|
| Domain and problem fit | 20 % | 9.5 | 9.2 | 8.8 |
| Security, safety, and governance | 15 % | 9.0 | 8.8 | 8.0 |
| Correctness and contract strength | 12 % | 9.3 | 9.1 | 9.0 |
| Operability and reliability | 12 % | 9.3 | 8.2 | 7.2 |
| Deployment and portability | 10 % | 9.6 | 7.8 | 6.8 |
| Modularity and maintainability | 10 % | 9.2 | 9.6 | 9.2 |
| Performance and resources | 7 % | 9.0 | 9.1 | 7.5 |
| Verification and tooling | 6 % | 9.4 | 9.2 | 8.8 |
| Ecosystem and interoperability | 5 % | 9.0 | 8.8 | 8.3 |
| Longevity and lock-in | 3 % | 9.1 | 8.7 | 8.0 |
| Absolute fit | 100 % | **92.8/100** | **88.7/100** | **82.1/100** |

The normalized shares of the three permissible options sum to 100 %:

| Rank | Option | Normalized share | Assessment |
|---:|---|---:|---|
| 1 | One native Go binary with a modular domain, Lefthook and CI adapters | **35 %** | strongest overall fit |
| 2 | Separate Go binaries for workflow and validator with a shared policy package | **34 %** | valid, but a doubled release and installation surface |
| 3 | Local Go client with a remote policy service and offline cache | **31 %** | valid upon later proven centralization necessity |

The normalized value of 35 % is not a quality score. The absolute fit of the chosen architecture is 92.8/100; the 35 % only represents the share within a shortlist of three strong candidates.

## 7. Language, runtime, and framework decision

### 7.1 Language

Go remains the right language. The project-specific MCDA prioritizes Go for this cross-platform CLI with 44 %, Rust with 32 %, and TypeScript/Node with 24 %. The concrete requirements reinforce Go additionally:

- a single native binary without a required runtime
- very good cross-compilation for Windows, macOS, and Linux
- fast startup time for hooks
- standard library for processes, paths, JSON, signals, and configuration
- clear error and context semantics
- no necessity for `unsafe`, cgo, or plugins

Target state:

- Language version: Go 1.26
- Pinned build toolchain: Go 1.26.6
- cgo: disabled as long as no documented adapter needs it

The toolchain is installed on the development machine as Go 1.26.6. Domain,
adapter, application, CLI, and local Git integration tests are executed.
Cross-platform release smoke tests and signing remain separate release gates.

### 7.2 Delivery frameworks

- Command routing: Cobra
- Interactive terminal forms: Huh v2 behind a `Prompt` port
- Accessible mode: explicitly configurable and screenreader-compatible without TUI rendering
- Configuration: standard library and versioned JSON; no global Viper state
- Git integration: the installed `git` binary via `exec.CommandContext` and argument arrays

Cobra and Huh remain entry adapters. The domain and application layers do not import them.

## 8. Implemented structure

```text
cmd/
  git-governance/
    main.go
internal/
  adapters/
    browser/
    configfs/
    gitcli/
    github/
    quality/
    report/
    system/
    terminal/
  application/
    branch/
    commit/
    policy/
    port/
    workflow/
  bootstrap/
  domain/
    branch/
    commitmsg/
    problem/
    ticket/
  integration/
docs/
packaging/
```

This is a modular monolith, not a microservice architecture. The functional complexity justifies value objects and use cases but no distributed deployments.

## 9. Domain model and ports

### 9.1 Value objects

- `BranchFamily`
- `TicketKey`
- `TicketNumber`
- `TicketID`
- `BranchSlug`
- `BranchName`
- `SemanticVersion`
- `SupportVersion`
- `CommitType`
- `CommitMessage`
- `PublicationState`
- `TargetBase`

All value objects are only created valid. Regexes are implementation building blocks, not the entire domain model.

The commit families are derived from the canonical `CommitType` catalog. An
application module composes from it, together with the ticket ID derived from
the branch, a complete commit message. Thereby `commit create`, scratch
transfers, and synchronization merges use the same family selection and the
same header invariant without duplicating commit types or the ticket scope in
workflows.

### 9.2 Application use cases

- `CreateBranch`
- `ValidateBranch`
- `CreateCommit`
- `ValidateCommit`
- `StartTicketWorkflow`
- `PublishTicketWorkflow`
- `StartHotfixWorkflow`
- `CutReleaseWorkflow`
- `SyncTargetBase`
- `ValidatePrePush`
- `ManageKnownKeys`

### 9.3 Ports

- `GitRepository`: read repository state and execute explicit Git operations
- `KeyPolicy`: check a key syntactically or later against a signed bundle
- `PreferencesStore`: persist known keys and UX preferences
- `Prompt`: interactive input and selection
- `Reporter`: human or JSON output

The initial `SyntaxOnlyKeyPolicy` accepts every syntactically valid key. A later `BundleKeyPolicy` may replace it without changing the use cases.

### 9.4 GitHub App authentication

GitHub authentication is an external platform capability and remains outside
the ticket, branch, commit, and provider-neutral PR model.
`PullRequestPublisher` remains the application-owned port; the GitHub adapter
resolves its credentials itself immediately before REST calls.

- Local users use the explicit OAuth device flow via `auth login github`. The
  browser is only started by this command.
- The local client possesses neither the GitHub App private key nor the client
  secret. Therefore the device flow is the correct native client flow; PKCE
  does not replace a client secret in GitHub's authorization code exchange.
- Only a host-/account-/client-ID-bound refresh session is persisted in the
  native OS vault, supplemented by its binding to the canonical repository
  identity (host/owner/repository) of the sign-in working context. Access
  tokens remain in process memory. Local file paths are never part of the
  session keys.
- The resolver rotates access/refresh tokens in a controlled manner, isolates
  hosts, selects the session through the repository binding of the target
  repository, falls back without a binding to capability discovery across the
  stored host sessions, and finally always checks the concrete
  app/user/repository intersection.
- Managed CI workloads use a central broker. The broker holds the private key
  outside the client and mints repository-bound, short-lived installation
  tokens after workload policy verification.
- Git transport authentication remains separate. `doctor` verifies it with a
  non-mutating, non-interactive push dry run.

## 10. Canonical CLI surface

One binary provides separate subcommands for separate use cases:

```text
git governance branch list
git governance branch create
git governance branch validate
git governance branch sync-base

git governance commit create
git governance commit validate

git governance workflow ticket start
git governance workflow ticket publish
git governance workflow hotfix start
git governance workflow release cut
git governance workflow release backmerge

git governance validate pre-push

git governance auth login github
git governance auth status github
git governance auth logout github

git governance config key list
git governance config key add
git governance config key remove
git governance config key set-default

git governance doctor
git governance completion <shell>
```

There are no separate `mkbranch` and `mkcommit` products. Two binaries would duplicate versioning, installation, configuration, and error contracts without functional benefit. The subcommands nevertheless remain clearly separated and scriptable.

## 11. Workflow boundaries

### 11.1 Regular ticket start

1. Check repository, remote, and a clean worktree.
2. `git fetch --prune <remote>`.
3. Validate the ticket and the branch family.
4. Check local and selected remote-tracking branches for an existing official
   regular branch with the same ticket ID.
5. Create the official branch directly from `<remote>/develop`.
6. Optionally create a `scratch/*` branch from the local official branch.
7. End on the chosen working branch.

The interactive explanation must make clear:

- Scratch is only for uncertain exploration.
- Scratch is private and not a PR target.
- Stable work belongs on the official ticket branch.
- Scratch results are transferred in a controlled manner via squash or cherry-pick.

### 11.2 Ticket publication and PR preparation

1. Check the official branch and ticket consistency.
2. Run local governance and project-specific quality checks.
3. `fetch --prune`.
4. Check base freshness.
5. Rebase only with an unpublished branch and missing base commits.
6. Run the validations again.
7. Display the rebase outcome interactively; on conflicts, resume the paused
   rebase or a previous scratch squash after explicit resolution and retry, or
   non-interactively with `--resume`.
8. Confirm the first push with upstream interactively or request it explicitly
   non-interactively.
9. Produce provider-neutral PR data for the target `develop`; a real provider
   creation is only offered with a configured outbound adapter.

The GitHub adapter is an outbound adapter behind the application-owned
`PullRequestPublisher` port. The domain core continues to contain no `gh`,
GitHub, or GitLab dependency; further providers remain replaceable adapters.

### 11.3 Hotfix

A hotfix starts from the actually affected line: `main`, the same `release/*` line, or the same `support/*` line. The line is mandatory and is not set to `develop` out of convenience.

### 11.4 Release and support

`release/*` and `support/*` are not created through the normal branch wizard. The wizard displays them fully but refers to the governance-bound workflow commands. `main` and `develop` are explained but never offered as a normal working-branch choice.

`workflow release cut` and `workflow release support` locally only create an
intent. A normal `--dispatch` is rejected outside a dry run: the protected-line
executor must not accept a free version, branch, or source SHA input.

The server-side path separates request authorization, execution
authorization, and technical completion verification:

```text
release-request
→ durable request record
→ release-execution
→ exactly one bound ref mutation
→ automatic read-only finalizer
→ verified | failed | verification_pending
```

The finalizer checks the correlated executor job and the real remote ref
against the bound source SHA. It holds no ref mutation, promotion, tag,
delivery, or reconciliation authority. ADR-0006 describes the complete
request/execution/finalizer boundary and the read-only recovery path.

All active release and hotfix controllers additionally belong to exactly one
functional lane. Request, execution, credential verification, regular
delivery, reconciliation, hotfix delivery, and hotfix propagation share
neither a generic approval environment nor broker variables. ADR-0007
describes this lane, credential, and decommissioning boundary.

A backmerge is a mandatory reconciliation, not a blanket PR: only after the
merged main promotion, the exactly belonging tag, and the successful release
delivery does the hosting adapter evaluate the effective
`release/<semver>`-to-`develop` delta. Only a delta produces the backmerge PR;
without a delta, an auditable `not-required` result is delivered.

If the develop target enforces a current pull request head, the delivered
release ref remains unchanged. The controlled combination with the current
develop state happens exclusively on a ticket-bound preparation branch and is
reviewed via a merge-commit PR to develop. The protected main control-plane
workflow builds a trusted binary before switching to this branch and executes
the controlled reconciliation from there. ADR-0004 describes this execution
boundary; ADR-0005 separates the reconciliation publisher identity from the
release automation.

After fully confirmed delivery, the target path evaluates the reconciliation
programmatically and idempotently. With a delta, it creates the reviewable
backmerge PR; without a delta, it documents `not-required`. A manual start
remains exclusively the recovery fallback.

## 12. Lefthook: complement, not replacement

Lefthook is, per its own documentation, a Git hook manager: configurations are installed into `.git/hooks`, and `lefthook run <hook-name>` executes configured jobs. Custom hooks and interactive jobs are possible. This makes Lefthook a good runner but not a branch/commit/workflow domain product.

Weighted capability coverage of the product defined here:

| Capability area | Weight | Lefthook alone | Go CLI alone | Combined target |
|---|---:|---:|---:|---:|
| Policy and validation logic | 25 % | 0 % | 25 % | 25 % |
| Branch/commit mutations | 20 % | 0 % | 20 % | 20 % |
| Workflow orchestration | 20 % | 0 % | 20 % | 20 % |
| Interactive and machine UX | 10 % | 1 % | 10 % | 10 % |
| User preferences | 5 % | 0 % | 5 % | 5 % |
| Local hook orchestration | 10 % | 10 % | 0 % | 10 % |
| CI/enforcement contract | 10 % | 4 % | 6 % | 10 % |
| **Total** | **100 %** | **15 %** | **86 %** | **100 %** |

The values measure native responsibility, not the ability to start arbitrary third-party programs. When Lefthook calls an external Go program, that coverage stems from the Go program.

Target integration:

```yaml
commit-msg:
  jobs:
    - run: git-governance commit validate --message-file "{1}" --output human

pre-push:
  jobs:
    - run: git-governance --interactive never validate pre-push --remote "{1}" --output human
      use_stdin: true
```

No regex and no Git workflow logic is duplicated in `lefthook.yml`.

The local full suite is produced in the publish use case after the final
synchronization decision and is bound to the concrete publish candidate. The
hook remains responsible for every outgoing ref update and may only reuse this
proof on exact revision, base, configuration, toolchain, and worktree
agreement. Thereby the local deduplication reduces no policy verification and
replaces neither CI nor branch protection.

Sources:

- [Lefthook base model](https://lefthook.dev/)
- [Custom Lefthook hooks](https://lefthook.dev/configuration/Hook/)
- [`lefthook run`](https://lefthook.dev/usage/commands/run/)

## 13. Configuration and key storage

Known keys are UX preferences, not a policy registry. They are stored via `os.UserConfigDir()`:

- Linux: `$XDG_CONFIG_HOME/git-governance/config.json`, otherwise `$HOME/.config/git-governance/config.json`
- macOS: `$HOME/Library/Application Support/git-governance/config.json`
- Windows: `%AppData%\git-governance\config.json`

Therefore `$HOME/.config` is not the correct default on all operating systems.

Stored are:

- known keys
- an optional default key
- accessibility and display preferences
- the schema version

Ticket numbers are not reused as a global default. They are work-specific and are derived from the current branch on commits. This prevents accidental commits for an old ticket.

Source: [Go `os.UserConfigDir`](https://pkg.go.dev/os#UserConfigDir)

## 14. Error contract

Every functional error has:

- a stable code
- a category
- the affected field
- the observed value
- the violated rule
- the expected format
- a valid example
- a concrete remediation
- the causal technical error message, where present

Example:

```text
BRANCH_SLUG_INVALID
Field: slug
Value: add--Export
Rule: slugs are canonical kebab-case without empty segments.
Expected: [a-z0-9]+(?:-[a-z0-9]+)*
Example: add-export-button
Remediation: use lowercase letters and single hyphens.
```

Exit codes:

- `0`: success
- `2`: CLI usage or missing input
- `3`: governance/validation violation
- `4`: invalid repository state
- `5`: Git operation failed
- `6`: configuration or policy bundle invalid
- `7`: external adapter or network failed
- `130`: user cancellation

In JSON mode, exactly one versioned result goes to stdout; diagnostics go to stderr.

## 15. Security and reliability

- Git is called exclusively with argument arrays, never via shell command strings.
- Repository, ref, and path values are validated before process calls.
- No implicit `git add .`, `push`, rebase, merge, or force operations.
- Mutating steps support `--dry-run` and a visible plan phase.
- The worktree must be clean before switch, rebase, and merge.
- Cancellation is propagated to Git processes via `context.Context`.
- No fire-and-forget goroutines; Git steps deliberately run sequentially.
- Configuration files are replaced with a platform-appropriate recovery
  strategy and created with restrictive permissions.
- GitHub App tokens, refresh tokens, private keys, and authorization headers
  never appear in flags, preferences, logs, errors, or human or JSON reports.
  Without a native secret store or an authorized broker, the abort is
  fail-closed.
- Policy bundles will later require version, origin, signature/checksum, and a
  staleness rule.
- Hooks are local early verification; CI and remote protection remain binding.

## 16. Build, release, and installation

Release artifacts are built reproducibly in CI for the supported OS/architecture pairs, tested, SHA-256-checksummed, signed, and published together with SBOM and provenance. Package managers are the primary installation path; direct archives are the controlled fallback.

After a protected `release/<semver> -> main` merge, a privilege-minimized
GitHub Actions workflow creates `v<semver>` as an annotated, immutable tag on
exactly the merge commit. Because a tag created by `GITHUB_TOKEN` does not
trigger another push workflow, this workflow explicitly starts the existing
artifact workflow via `workflow_dispatch`.

After successful artifact and release publication, the GitHub lifecycle
adapter verifies the promotion merge, the exact tag, and the effective delta
from `release/<semver>` to `develop`. Only with a delta is a reviewable
backmerge PR created; otherwise the auditable result `not-required` is the
reconciliation completion.

Installation scripts must not modify `.bashrc`, `.zshrc`, or PowerShell profiles without being asked. Details reside in `docs/operations/installation-and-release.md`.

## 17. Verification

Mandatory gates:

- domain unit and table-driven tests for every valid and invalid grammar class
- fuzz tests for the branch and commit parsers
- contract tests for human and JSON errors
- integration tests against temporary real Git repositories
- tests for unpublished and published branches
- tests for no-op, rebase, and merge cases of base freshness
- `go test ./...`
- `go tool -modfile tools/go.mod check-coverage`
- `go test -race ./...`
- `go vet ./...`
- vulnerability and dependency scan
- builds and smoke tests on Windows, macOS, and Linux
- installation and upgrade tests per package manager

Current verification status: the modular Go core, the CLI, the local Git
integration, whitebox tests, and fuzz smokes are implemented in the
repository. The binding current evidence is maintained exclusively in
`docs/TRACEABILITY.md`. Release publication, native macOS/Linux smokes,
signing, package manager publication, and remote branch protection remain
separate delivery gates not yet locally provable.

## 18. Consequences

Positive:

- one functional truth
- identical behavior on Windows, macOS, and Linux
- the same use cases for interactive use, automation, Lefthook, and CI
- stable machine-readable errors
- later policy registry integration without rebuilding the workflows

Negative:

- native artifacts must be built and tested per OS/architecture
- Cobra and Huh extend the supply-chain surface
- Git remains a required external process dependency
- further provider-specific PR creation needs its own adapter

## 19. Discarded options

- Parallel `ps1`/`sh` implementations: duplicated functional truth
- Lefthook-only: a runner without domain and workflow capabilities
- Two independent commands `mkbranch` and `mkcommit`: duplicated delivery and configuration surface
- TypeScript/Node as the core: additional runtime surface without functional benefit
- Automatic editing of shell profiles: fragile, not idempotent, and unnecessary with package managers
- Blind `pull` or rebase: mutates state without a prior decision
- Internal CLI self-invocations: process coupling instead of application service composition
- A continuous ticket-to-PR wizard: an ill-fitting long-lived workflow across the development phase

## 20. Revisit triggers

The decision is re-evaluated when:

- a target operating system does not allow a Go binary
- hard real-time, FIPS, or new compliance gates emerge
- a central policy service with proven added value becomes binding
- offline capability ceases
- cross-provider PR creation becomes its own core product
- a stable published CLI contract forces a split into multiple artifacts

## 21. External references

- [Go Release History](https://go.dev/doc/devel/release)
- [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/)
- [Semantic Versioning 2.0.0](https://semver.org/)
- [Git Fetch](https://git-scm.com/docs/git-fetch)
- [Git Rebase](https://git-scm.com/docs/git-rebase)
- [Cobra](https://cobra.dev/)
- [Huh](https://github.com/charmbracelet/huh)
- [GoReleaser](https://goreleaser.com/)
