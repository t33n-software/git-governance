# Contributing to git-governance

## Development prerequisites

- Go 1.26.5; `go.mod` pins `toolchain go1.26.5`
- Git 2.53 or newer recommended

```powershell
$env:GOTOOLCHAIN = "local"
$env:GOFLAGS = "-mod=readonly"
$env:GOVCS = "*:off"
go version
git --version
go mod download
go -C tools mod download
```

Do not add a runtime requirement for end users. Go belongs to development and
CI; released artifacts are native binaries.

These variables affect only the current shell. Do not use `go env -w` for
repository policy because it mutates a developer's global Go configuration.

## Local development loop

```powershell
go test -mod=readonly ./...
go run -mod=readonly .\cmd\check-coverage
go vet -mod=readonly ./...
go run -mod=readonly .\cmd\git-governance --help
```

Format changed Go files:

```powershell
gofmt -w (Get-ChildItem -Recurse -Filter *.go | ForEach-Object FullName)
```

Use `gofmt` before every proposed commit. Avoid hand-formatting generated
completion output or Go module files.

## Architecture rules

- Domain packages must not import Cobra, Huh, Git CLI adapters, filesystem
  adapters, environment variables, or hosting-provider APIs.
- Application packages own interfaces consumed by use cases.
- Adapters implement those interfaces and never contain a second branch,
  commit, or ticket grammar.
- Workflows compose application services in-process. They must not execute
  `git-governance` as a child process or parse its text output.
- Git commands use argument arrays through the Git adapter. Do not introduce
  shell command strings.
- Add a new external dependency only when the standard library or existing
  dependencies cannot meet the contract.

## Testing requirements

Every behavior change requires tests at the lowest meaningful boundary:

| Change | Required evidence |
|---|---|
| Ticket, branch, version, or commit grammar | same-package whitebox table tests and fuzz seed |
| Domain error or exit code | unit and JSON/Human contract test |
| Git argument or parser behavior | Git adapter whitebox test |
| Git mutation behavior | temporary local repository integration test |
| CLI flags or help | bootstrap CLI contract test |
| Hook behavior | Lefthook configuration test and CLI validator test |
| User preference behavior | config adapter whitebox test |

Run the full local gate:

```powershell
go run -mod=readonly .\cmd\build
```

The build runner owns the complete ordered quality sequence and resolves its
pinned development tools from `tools/go.mod`; do not require globally installed
linters, vulnerability scanners, or Lefthook binaries.

`cmd/check-coverage` runs its coverage tests uncached. It rejects every Go
package reported without a `_test.go` file and every package with executable
statements below `100.0%` coverage.

Dependency updates belong in a separately reviewed update lane. That lane is
the only place allowed to run `go get` or a mutating `go mod tidy`; normal
development, CI, and release lanes must keep `go.mod` and `go.sum` read-only.

Run bounded fuzzing before changes to parser or config code:

```powershell
go test ./internal/domain/ticket -run=^$ -fuzz=FuzzParseTicketValues -fuzztime=2s -parallel=1
go test ./internal/domain/branch -run=^$ -fuzz=FuzzParseBranchValues -fuzztime=2s -parallel=1
go test ./internal/domain/commitmsg -run=^$ -fuzz=FuzzParseCommitMessage -fuzztime=2s -parallel=1
go test ./internal/adapters/configfs -run=^$ -fuzz=FuzzDecodePreferences -fuzztime=2s -parallel=1
go test ./internal/adapters/quality -run=^$ -fuzz=FuzzDecodeQualityConfiguration -fuzztime=2s -parallel=1
```

## Repository-local quality gates

`git-governance` is project- and language-agnostic. Do not hardcode build,
test, or lint commands in the CLI. A trusted repository may opt in through
`git-governance.quality.json`, using one executable and an argument array per
gate:

```json
{
  "schemaVersion": 2,
  "defaults": {
    "includeFamilies": [
      "feature", "fix", "docs", "refactor",
      "chore", "test", "perf", "hotfix"
    ]
  },
  "gates": [
    {
      "name": "unit-tests",
      "command": "go",
      "args": ["test", "./..."],
      "timeout": "2m"
    },
    {
      "name": "complete-statement-coverage",
      "command": "go",
      "args": ["run", "./cmd/check-coverage"],
      "timeout": "5m"
    }
  ]
}
```

The runner rejects shell strings, absolute or escaping working directories,
unknown fields, duplicate names, invalid family scopes, and invalid timeouts.
No file means `qualityStatus=unconfigured`, not a successful
project-quality result. Gates inherit the default family set unless they
declare `includeFamilies` and/or `excludeFamilies`; private `scratch` work is
included only by a gate that explicitly selects it.

## Manual CLI smoke test

```powershell
New-Item -ItemType Directory -Force .build\bin | Out-Null
go build -o .\.build\bin\git-governance.exe .\cmd\git-governance
.\.build\bin\git-governance.exe --version
.\.build\bin\git-governance.exe --output json branch list
.\.build\bin\git-governance.exe completion powershell
```

`.build/` is ignored and owns local build outputs. `dist/` is reserved for
GoReleaser release artifacts. Do not commit either directory.

## Commit and branch conventions

Use the contracts enforced by the CLI:

```text
feature/ABC-123-add-export-button
feat(ABC-123): add export button
```

Official published ticket branches are append-only:

- do not amend as a routine;
- do not force push;
- do not routine-rebase after first push;
- merge an updated target base when synchronization is necessary.

Before the first push, rebase only if the fetched target base contains commits
missing from the unpublished branch.

For normal ticket work, create exactly one official regular branch per ticket.
The CLI checks both local and selected remote-tracking official branches after
fetching. A second regular branch for the same ticket is rejected; governed
release stabilization and hotfix propagation are explicit lifecycle exceptions.

## Lefthook

Install the approved Lefthook binary, then:

```powershell
lefthook install
lefthook validate
```

The hook configuration is deliberately thin. Do not add regexes, live network
calls, direct `git pull`, rebase, merge, or business logic to `lefthook.yml`.

## Documentation

When a product contract changes, update the relevant local document:

- `README.md` for user-facing behavior;
- `docs/specification/POLICY-UND-VALIDIERUNG.md` for grammar and validation;
- `docs/specification/CLI-VERTRAG.md` for flags and commands;
- `docs/architecture/ADR-0001-GO-CLI-ZIELARCHITEKTUR.md` for architecture;
- `docs/operations/INSTALLATION-UND-RELEASE.md` for delivery;
- `docs/TRACEABILITY.md` for implementation and verification status.

The repository must remain self-contained. Do not add dependencies on external
rule files or unpublished documentation.

## Release handoff

The local CLI prepares release promotion only. After a protected
`release/<semver> -> main` merge, the `Tag Promoted Release` workflow creates
the annotated immutable `v<semver>` tag at the merge commit and dispatches the
artifact workflow. Do not create release tags from a developer workstation.

Package-manager templates live in `packaging/`. They require a
maintainer-controlled Homebrew tap, Scoop bucket, or WinGet submission target;
do not invent those external repositories or commit publisher credentials.
