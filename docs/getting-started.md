# Getting started

## Prerequisites for contributors

- Git 2.53 or newer recommended
- Go 1.26.6 (enforced by the `toolchain go1.26.6` directive)

Check the local environment:

```powershell
go version
git --version
```

Expected Go output begins with:

```text
go version go1.26.6
```

## Build from source

Clone or open the repository, then run the full build gate:

```powershell
go tool -modfile tools/go.mod quality-gate
```

The command is identical on macOS and Linux. The pinned quality-gate
orchestrator verifies root and build-tool module integrity, checks formatting,
runs Staticcheck, typechecks packages and tests, runs unit, contract,
integration, coverage, race, vet, vulnerability, fuzz, and Lefthook checks,
then builds and smoke-tests the native binary. It stops at the first failed
gate and writes `.build\bin\git-governance.exe` on Windows or
`.build/bin/git-governance` on macOS and Linux.

For a local development run without producing a binary:

```powershell
go run .\cmd\git-governance --help
```

To use the Git subcommand form locally, put the built binary in a directory
already on `PATH`:

```powershell
git governance --help
```

## GitHub App login

Pull-request publication uses a GitHub App, not a static personal token. Run
the explicit browser-assisted Device Flow from the project directory — or
pass it with `--repo` — and enter the public App client ID when the command
prompts for it, once per repository tenant, never again afterwards:

```powershell
git governance --interactive always auth login github
git governance auth status github
```

The client ID is stored with the protected session in the native operating
system secret store, and the session is bound to the canonical repository
identity of the selected remote. Switching between projects selects each
project's session automatically; no environment variable and no flag is
required at login or at publication time.

The complete prerequisite, secret-store, broker, logout, and Git transport
readiness contract is in [GitHub App authentication](usage/authentication.md).

## Install a released build

Consumers do not build from source. Install and update through the same
channel:

1. Download the release artifact for your platform from the release channel.
2. Verify its SHA-256 checksum and signature against the release manifest.
3. Place the binary in a directory already on your `PATH`.
4. Verify with `git-governance --version` and `git-governance doctor`.

Updates are the same channel re-run against a newer pinned version; there is
no automatic self-update. The delivery and update model is recorded in
[ADR-0009](architecture/ADR-0009-CHANNEL-OWNED-DELIVERY-AND-UPDATE-MODEL.md),
and the complete installation design — including the verification-first
script standard and the package-manager target level — is in
[installation and release](operations/installation-and-release.md).

Release installers and package-manager manifests are added by the release
pipeline. They are not yet published by this repository.
