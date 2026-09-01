# Installation and Release

## 1. Decision

`git-governance` is distributed as a prebuilt, signed native binary. End devices need Git, but neither Go nor Node.js, Python, PowerShell 7, or an additional language runtime.

Primary installation paths:

| Platform | Primary | Secondary | Controlled fallback |
|---|---|---|---|
| Windows | WinGet | Scoop | signed ZIP/MSI from the release |
| macOS | Homebrew tap | signed PKG once required | signed tar.gz |
| Linux | deb/rpm/apk or an organization-internal package channel | Homebrew/Nix as needed | signed tar.gz |

A package manager is the best-practice installation path because it owns the installation location, `PATH`, upgrade, and uninstall. The project does not edit `.bashrc`, `.zshrc`, the Fish configuration, or PowerShell profiles without being asked.

## 2. Boundary to NVM

NVM manages a runtime and multiple Node versions. To do so, it must configure shell initialization. `git-governance`, by contrast, is a single native application. A dedicated runtime/version manager would be additional complexity without functional benefit.

More fitting comparison models are native CLIs distributed via WinGet, Homebrew, Linux packages, or signed release archives.

## 3. Supported target matrix

Binding first release matrix:

| OS | Architectures | Artifact |
|---|---|---|
| Windows 10/11 | `amd64`, `arm64` | `.zip`, optional MSI |
| macOS, currently supported versions | `amd64`, `arm64` | `.tar.gz`, optional PKG |
| Linux | `amd64`, `arm64` | `.tar.gz`, deb, rpm, optional apk |

Additional operating systems are not an implicit commitment. They require target platform tests and a documented support gate.

Build properties:

- Go 1.26 as the language version
- pinned toolchain Go 1.26.6
- `go 1.26` plus `toolchain go1.26.6` in the module contract
- `CGO_ENABLED=0` as long as no documented adapter needs cgo
- no build or compiler installation on end devices
- reproducible version metadata from the immutable release commit

Cross-compilation alone is not sufficient. Every binding OS/architecture class requires at least one native smoke test; the three OS families require full integration paths.

## 4. Package names

- Executable: `git-governance` or `git-governance.exe`
- WinGet package ID: once the publisher is decided, `<Publisher>.GitGovernance`
- Homebrew formula: `git-governance`
- Debian/RPM/APK: `git-governance`
- Scoop manifest: `git-governance`

The name `git-flow` is avoided because it collides with existing Gitflow tools. `mk` is too generic; `git-tools` describes no stable product capability.

## 5. `PATH` behavior

### 5.1 Package managers

The respective package manager owns installation and `PATH` integration. The project appends no lines to user profiles.

### 5.2 Direct archives

Direct archives are a fallback:

- Unix: install the binary into a directory already on the `PATH`, preferably `~/.local/bin` for a user installation or an administratively managed system path.
- Windows: unpack the binary into a dedicated user program directory. A change to the user `PATH` happens only through a signed installer or after an explicit user action.

If the target path is not yet on the `PATH`, the documentation shows the necessary manual step. A general installation script must not guess and modify multiple shell profiles.

### 5.3 New terminals

A persistent `PATH` change is picked up by new processes. The installer must:

- clearly report whether the current terminal already knows the new path
- prompt to start a new terminal when needed
- verify with `git governance --version` and `git governance doctor`

## 6. User configuration

The binary determines the configuration root with Go `os.UserConfigDir()` and creates `git-governance/config.json` beneath it:

| Platform | Default |
|---|---|
| Linux | `$XDG_CONFIG_HOME/git-governance`, otherwise `$HOME/.config/git-governance` |
| macOS | `$HOME/Library/Application Support/git-governance` |
| Windows | `%AppData%\git-governance` |

The path is explicitly overridable via `--config`. A relative `XDG_CONFIG_HOME` is rejected, per the Go contract.

Configuration is:

- versioned
- validated typed
- replaced with a crash-safe write/recovery strategy; the exact atomicity
  guarantee follows the respective platform
- created with restrictive user permissions
- never used as a secret store

### 6.1 GitHub App credentials

The configuration file never stores GitHub tokens, refresh tokens, App private
keys, client secrets, broker credentials, or authorization headers. A local
user enters the public GitHub App client ID once at the interactive
`auth login github` prompt in a real terminal; no environment variable or flag
is involved at any point. The refresh session — including the public client
ID — is protected by DPAPI on Windows, Keychain on macOS, or Secret Service on
Linux and is bound to the canonical repository identity of the login's working
context; no plaintext fallback is permitted.

Managed CI does not reuse a developer refresh session. It supplies a
workload-identity token and a HTTPS credential-broker endpoint at deployment
time. The broker holds the GitHub App private key outside the repository and
mints only short-lived, repository-bound installation tokens. See
[GitHub App authentication](../usage/authentication.md) for the precise
runtime contract.

### 6.2 Repository quality gates

Project- and programming-language-dependent build, test, and lint commands
belong neither in the binary nor in the user configuration. A repository
optionally declares them in `git-governance.quality.json` or via
`--quality-config`.

The file contains only a name, an executable, an argument array, a
repository-relative working directory, and a timeout per gate. Shell strings
and paths outside the repository root are inadmissible. Without the file, the
result remains explicitly `unconfigured`; the CLI claims no passed
project-specific quality suite.

When a valid file exists, its gates are locally mandatory for every `pre-push`
with at least one official working branch. The configuration defines a default
scope for this and optional scopes per gate: `includeFamilies` selects
families, `excludeFamilies` removes them afterwards. A gate without a scope
inherits the repository default. The default set contains all official working
families; private `scratch/*` is not included but can be selected deliberately
for a single gate.

On a multi-ref push, the suite runs every entitled gate once after the
successful ref policy check. Thereby the tool remains project- and
language-agnostic: it knows no prescribed build or lint commands but reliably
enforces an existing explicit repository contract.

In the governed publish path, the full suite runs on the final candidate after
a possible synchronization. Its short local Git metadata proof binds the
revision, target base, remote, configuration digest, gate selection,
toolchain, and a clean worktree. Pre-push nevertheless always checks the
policy and only accepts the proof on an exact, fresh match. Without a matching
proof, the full suite is the raw-push fallback; corrupted proofs block
fail-closed.

Source: [Go `os.UserConfigDir`](https://pkg.go.dev/os#UserConfigDir)

## 7. Lefthook installation

Lefthook remains its own organization-wide standardized product. `git-governance` neither vendors nor copies its own Git hook scripts.

Repository setup:

1. Install Lefthook through the approved package channel.
2. Run `lefthook install` in the repository.
3. Run `lefthook validate`.
4. Run `git governance doctor`.

Lefthook itself is available as a standalone binary and supports Homebrew, WinGet, and Scoop, among others. That matches the same distribution principle.

Sources:

- [Lefthook installation](https://lefthook.dev/install/)
- [`lefthook install`](https://lefthook.dev/usage/commands/install/)

## 8. Release pipeline

The release pipeline strictly separates:

### 8.1 Build

- verify the exact Git tag and commit
- provision the exact Go toolchain and enforce it with `GOTOOLCHAIN=local`
- resolve dependencies with checksums
- check the module graph with `go mod tidy -diff` without mutation
- run build and test commands with `-mod=readonly`
- `go test ./...`
- `go tool -modfile tools/go.mod check-coverage` with uncached coverage, a
  mandatory `_test.go` file per Go package, and `100.0 %` for executable
  statements
- `go test -race ./...` on native test platforms
- `go vet ./...`
- static analysis and vulnerability scan
- dependency review on every pull request plus periodic CI re-evaluation
- build cross-platform binaries

#### 8.1.1 Local GoReleaser validation

The release configuration is verified in CI with the same hard-pinned
GoReleaser version as the release execution. Local validation is only
permissible with an already approved and verified GoReleaser binary. Ad-hoc
procurement via `go install ...@version` is not part of the release or
validation process.

The check validates exclusively the configuration. It publishes no artifacts
and needs no release credentials.

#### 8.1.2 Procurement boundary

This repository already enforces toolchain, read-only, and VCS fallback
controls in CI and release. The migration to an internal approved proxy and
the associated artifact registry admission is an external platform
prerequisite and is only activated once it is provided. Until then, the
existing Go proxy configuration remains unchanged.

#### 8.1.3 Dependency and runner governance

- Pull requests pass a dependency review pinned to an immutable commit; new
  vulnerability findings of any severity and in any dependency scope block the
  check.
- CI runs the same supply-chain gates daily in addition to pull requests,
  pushes, and manual executions.
- GitHub-hosted jobs use concrete, version-bound runner labels instead of
  `*-latest`. These labels reduce major-version drift but are no substitute
  for an immutable build enclave to be provided later.

### 8.2 Package

- produce Windows, macOS, and Linux artifacts
- produce shell completions and man pages
- include the license and notices
- produce a SHA-256 checksum per artifact
- produce an SBOM per artifact

Current artifact contract:

- Archives contain `README.md`, `CONTRIBUTING.md`, `LICENSE`, and `NOTICE`.
- The generator executed before the release creates Bash, Zsh, Fish, and
  PowerShell completions as well as Cobra man pages under `.build/generated/`;
  these files are included in every archive. `dist/` remains exclusively
  reserved for the release artifacts produced by GoReleaser.
- GoReleaser produces Linux packages in `deb`, `rpm`, and `apk`.
- Windows, Homebrew, Scoop, and WinGet publication continue to require
  concrete publisher, bucket, or tap identities; these are not invented and
  are only published after their configuration.
- Repository-local Homebrew, Scoop, and WinGet templates reside under
  `packaging/` and are only filled with the version and checksum data of an
  immutable GitHub release.

### 8.3 Verify

- native smoke tests per target class
- use real temporary Git repositories
- verify `version`, `doctor`, branch validation, and commit validation
- verify install/upgrade/uninstall per package format
- allow no unplanned profile or repository mutations

### 8.4 Sign and publish

- sign the checksum manifest, preferably with Sigstore/Cosign
- Windows code signing for the installer/binary
- macOS signing and notarization for the corresponding packages
- publish provenance/attestation
- publish the immutable release with artifacts
- produce package manager manifests only from the published artifact

All third-party actions in CI and release are pinned to full, immutable commit
IDs. GoReleaser, Syft, and Cosign are additionally bound to concrete versions.
Govulncheck, Lefthook, and Staticcheck come from the versioned
`tools/go.mod` module. A release is only built from a SemVer tag or a manual
run with an explicit existing SemVer tag.

The normal automated path reads:

```text
git-governance workflow release cut --version <semver>
-> machine-readable intent for execute-protected-line-request.yml
-> the authorized release-request controller binds ticket, source SHA, and target ref
-> the separately authorized execution creates release/<semver> from origin/develop
-> the automatic read-only finalizer verifies origin/release/<semver>
-> controlled stabilization and PR to main
release/<semver> -> protected merge to main
-> CI checks the merge commit
-> CI creates the annotated tag v<semver> on exactly this commit
-> CI starts the artifact workflow for this tag
-> GoReleaser builds, signs, attests, and publishes
-> the lifecycle adapter verifies promotion, tag, published delivery, and delta
-> with delta: reviewable backmerge PR to develop
-> without delta: auditable not-required, no empty PR
```

`execute-protected-line-request.yml` is dispatched exclusively by the bound
`release-request` controller with a `request_id`. The executor verifies the
version, the source line, the release tag for support lines, the nonexistence
of the target branch, and the durable request record. Its automatic read-only
finalizer verifies the created remote line. A GitHub ruleset or branch
protection rule must designate the workflow as the permitted creator of
`release/*` and `support/*`; the local CLI receives no push permission for
this.

When the `release/*` or `support/*` ruleset enforces required status checks,
its status check rule must set `do_not_enforce_on_create: true`. Otherwise
GitHub requires checks for a target branch before it even exists and blocks
the controlled release or support cut. The exception applies exclusively to
the first ref creation; all protection rules apply unchanged afterwards.

A tag created with `GITHUB_TOKEN` does not trigger another push workflow. The
tag workflow therefore explicitly starts the existing `workflow_dispatch` path
of the artifact workflow and passes the existing tag.

GoReleaser can orchestrate builds, archives, checksums, signatures, and package manager manifests. It remains a build/release tool and is not a runtime dependency.

Sources:

- [GoReleaser](https://goreleaser.com/)
- [GoReleaser checksums](https://goreleaser.com/customization/package/checksum/)
- [GoReleaser signing](https://goreleaser.com/customization/sign/sign/)
- [GoReleaser SBOM](https://goreleaser.com/customization/sbom/)

## 9. Version and update model

- SemVer 2.0.0
- Release tags: `v<semver>`
- The binary reports version, commit, and build provenance
- published artifacts are never replaced
- updates happen through the same channel as the installation
- no automatic self-update in version 1

A self-updater would duplicate signature verification, proxy, rollback, channel choice, and package manager ownership. It is only re-evaluated upon a later proven offline or fleet need.

## 10. Atomic installation and rollback

Package managers must handle upgrade and rollback according to their platform. Direct installers:

1. Verify the artifact and signature before mutation.
2. Write the new binary to a temporary path.
3. Run an executable smoke test.
4. Replace the target atomically, as far as the platform allows.
5. Preserve the previous version for a controlled rollback.

Configuration migrations are forward-compatible and must not silently destroy an older binary. Breaking schema changes need expand/contract or an explicit migration with backup.

## 11. Uninstall

Uninstall removes:

- the program owned by the package manager
- completions and man pages installed by the package

Uninstall does not remove by default:

- user configuration
- repository configuration
- Lefthook
- Git data

A separate `--purge` path may delete user configuration only after explicit confirmation.

## 12. Forbidden installation patterns

- editing multiple shell profiles without being asked
- `ExecutionPolicy Bypass` as a regular installation contract
- `curl | sh` without artifact verification
- downloading the latest unpinned binary on every start
- installing a Go toolchain on end devices
- parallel installers with their own functional logic per operating system
- mixing Lefthook, CLI, and repository policy installation
- changing published artifacts under the same tag
- mutable GitHub Action tags or unpinned `@latest` tool installations in CI
  and release workflows
- publishing to a Homebrew tap, Scoop bucket, WinGet, or platform signing
  channel without a provably maintainer-controlled target identity and
  credentials

## 13. Acceptance criteria

- fresh installation on every target platform
- invocation both as `git governance` and as `git-governance`
- a new terminal finds the binary
- an upgrade preserves the user configuration
- uninstall removes no user data without `--purge`
- checksum and signature are verifiable
- offline invocation of the local validation works
- no language runtime is required on the endpoint
- no shell profile is edited directly by the default installation
