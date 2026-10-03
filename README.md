# git-governance

`git-governance` is a native Go CLI for governed Git work on Windows, macOS,
and Linux. It creates and validates branches and commits, guides bounded
ticket workflows, and exposes the same validation core to Lefthook and CI.

Start with the [documentation index](docs/index.md), then use the
[CLI usage guide](docs/usage/index.md) for complete interactive and
non-interactive command contracts.

The canonical organization-wide GitHub rulesets live under
[rulesets/github](rulesets/github/README.md). They are managed once for the
whole organization and imported at organization level; do not redefine those
rules per repository.

## Installation

`git-governance` is distributed as a prebuilt, signed native binary through
its release channel; end devices need Git but no language runtime. Install
and update through the same channel:

- **Package managers** are the primary installation path once their publisher
  identities are configured; they own the installation location, `PATH`,
  upgrade, and rollback.
- **Release artifacts** are the direct path: download the artifact for your
  platform from the release channel, verify its SHA-256 checksum and
  signature against the release manifest, place the binary in a directory on
  your `PATH`, and verify with `git-governance --version` and
  `git-governance doctor`.
- **Updates** run through the installing channel — an update is the
  installation re-run against a newer pinned version. There is no automatic
  self-update and no `update` subcommand; the delivery and update model is
  recorded in
  [ADR-0009](docs/architecture/ADR-0009-CHANNEL-OWNED-DELIVERY-AND-UPDATE-MODEL.md).

Verification-first install scripts and package-manager manifests are added by
the release pipeline. The complete delivery, installation, and update design
is in [installation and release](docs/operations/installation-and-release.md);
a contributor build from source is described in
[getting started](docs/getting-started.md).

## Command catalog

- `branch list`, `branch create`, `branch validate`, `branch merge-scratch`,
  and `branch sync-base`
- `commit create` and `commit validate`
- `workflow ticket start` and `workflow ticket publish`
- `workflow bootstrap`
- `workflow hotfix start`, `workflow hotfix publish`, and
  `workflow hotfix propagate`
- `workflow release cut`, `workflow release stabilize`,
  `workflow release publish-stabilization`, `workflow release promote`,
  `workflow release backmerge`, and `workflow release support`
- `workflow cleanup`
- `validate pre-push`
- `auth login github`, `auth status github`, and `auth logout github`
- `config key list`, `config key add`, `config key remove`, and
  `config key set-default`
- `policy describe`, `doctor`, and `completion <shell>`

For automation, use `--interactive never --output json`, supply every required
value as a flag, and add `--yes` for mutations. GitHub pull-request creation is
an explicit opt-in through `--pull-request-provider github` and
`--create-pull-request`. After a created pull request, a local invocation
returns the workspace to the `develop` integration line and reports the
transition outcome; server-side controllers never switch their ephemeral
checkout.

For protected release or support lines, add `--dispatch` to the corresponding
release workflow. The GitHub lifecycle adapter waits for the authorized
workflow and verifies the resulting remote line. A release backmerge is
delivery-gated and creates a `develop` pull request only when an effective
release-only delta remains; see the
[release reconciliation guide](docs/usage/workflows/release-reconciliation.md).

GitHub API access uses an explicit GitHub App login or a managed credential
broker; it never accepts a GitHub token as a CLI flag or stores one in user
preferences. Read the [GitHub App authentication guide](docs/usage/authentication.md)
before requesting pull-request creation.

## License

This project is source-available under
`LicenseRef-git-governance-NoRepublish-1.0`: free to use, clone, build, and
modify for your own use, commercially and non-commercially; republishing the
project or substantially similar forks is prohibited. The canonical text lives
in `LICENSE` and `LICENSES/`, rendered from the organization's license hub and
pinned by `license.lock.json`.
