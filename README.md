# git-governance

`git-governance` is a native Go CLI for governed Git work on Windows, macOS,
and Linux. It creates and validates branches and commits, guides bounded
ticket workflows, and exposes the same validation core to Lefthook and CI.

Start with the [documentation index](docs/index.md), then use the
[CLI usage guide](docs/usage/index.md) for complete interactive and
non-interactive command contracts.

Hosting-platform contracts and their importable GitHub rulesets live under
[docs/hosting-platforms](docs/hosting-platforms/index.md). Do not redefine
those rules outside that tree.

## Command catalog

- `branch list`, `branch create`, `branch validate`, `branch merge-scratch`,
  and `branch sync-base`
- `commit create` and `commit validate`
- `workflow ticket start` and `workflow ticket publish`
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
`--create-pull-request`.

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
