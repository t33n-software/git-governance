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
go run .\cmd\build
```

On macOS or Linux:

```bash
go run ./cmd/build
```

`cmd/build` verifies root and build-tool module integrity, checks formatting,
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

Pull-request publication uses a GitHub App, not a static personal token. Set
the public App client ID in the current shell, then run the explicit
browser-assisted Device Flow:

```powershell
$env:GIT_GOVERNANCE_GITHUB_APP_CLIENT_ID = "<GitHub-App-client-ID>"
git governance --interactive always auth login github
git governance auth status github
```

The complete prerequisite, secret-store, broker, logout, and Git transport
readiness contract is in [GitHub App authentication](usage/authentication.md).

Release installers and package-manager manifests are added by the release
pipeline. They are not yet published by this repository.
