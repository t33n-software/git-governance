# Verification

```powershell
$env:GOTOOLCHAIN = "local"
$env:GOFLAGS = "-mod=readonly"
$env:GOVCS = "*:off"
go test -mod=readonly ./...
go run -mod=readonly .\cmd\check-coverage
CGO_ENABLED=1 go test -mod=readonly -race ./...
go vet -mod=readonly ./...
go test -mod=readonly ./internal/integration -count=1
go tool -modfile tools/go.mod govulncheck ./...
```

`check-coverage` executes
`go test -count=1 -p=1 -cover -covermode=atomic ./...`. It serializes package
test processes for reproducible aggregate coverage while preserving atomic
counters for concurrent code inside each process. It fails when a Go package
has no `_test.go` file or its executable statements do not reach `100.0%`
coverage.

## Platform verification model

Coverage is native-platform evidence, not a single cross-platform percentage.
Go build tags select different production files for Linux, macOS, and Windows;
a Windows coverage run therefore cannot prove branches that only compile under
`//go:build linux` or `//go:build darwin`.

The required model is:

1. A developer runs the complete local gate on the operating system they use:

   ```powershell
   go run -mod=readonly .\cmd\build
   ```

   During governed publication this same suite runs after the final allowed
   synchronization, not as stale evidence before a rebase or merge. The CLI
   records a short-lived, revision-bound local Git-metadata proof and pre-push
   may reuse it only for the exact final candidate. Raw pushes without a
   matching proof still run the suite locally.

2. Pull requests run the same complete build gate natively in CI on Linux,
   macOS, and Windows. Each runner independently executes linting, tests,
   uncached 100%-coverage, race detection, static analysis, vulnerability
   checks, fuzz smoke tests, Lefthook validation, and a native binary build.
3. Every build-tagged adapter has direct same-package tests for the operating
   system that compiles it. Linux, macOS, and Windows coverage must each reach
   `100.0%` independently.
4. Secondary architecture jobs remain native smoke checks. They prove that the
   binary starts on the target architecture but do not replace full native
   quality gates on the primary runner for each operating-system family.

No local Linux or macOS runner is required on a Windows developer workstation.
WSL2, containers, or remote CI can provide additional early feedback, but
cross-compiling with `GOOS` only proves that target code compiles; it cannot
execute target tests or establish target-native coverage. macOS behavior is
authoritatively tested on a macOS CI runner.

Repository branch protection must require every native `Quality gates (...)`
matrix result. The workflow configuration can create those checks, but the
hosting-provider ruleset is the external authority that prevents a merge when
one platform fails.

Go source and module metadata are canonical LF text through `.gitattributes`.
This keeps both `gofmt -l` and `go mod tidy -diff` semantically identical on
Windows and Unix runners instead of failing on a checkout-only line-ending
conversion.

Native binary smoke tests intentionally exclude `doctor`: a detached CI
checkout has neither a checked-out work branch nor a developer's Git transport
credential, while `doctor` must fail closed when either is absent. Its
behavior is covered by same-package and integration tests and is exercised in
real developer repositories before governed publication.

On Windows, the race detector needs a working C compiler and `CGO_ENABLED=1`.
If the local host cannot provide that compiler, use the Ubuntu CI gate rather
than claiming a skipped race run passed.

Fuzz smoke tests use a fixed iteration budget so runner scheduling cannot turn
a time budget into a false failure:

```powershell
go test ./internal/domain/ticket -run=^$ -fuzz=FuzzParseTicketValues -fuzztime=50000x -parallel=1
go test ./internal/domain/branch -run=^$ -fuzz=FuzzParseBranchValues -fuzztime=50000x -parallel=1
go test ./internal/domain/commitmsg -run=^$ -fuzz=FuzzParseCommitMessage -fuzztime=50000x -parallel=1
go test ./internal/adapters/configfs -run=^$ -fuzz=FuzzDecodePreferences -fuzztime=50000x -parallel=1
go test ./internal/adapters/quality -run=^$ -fuzz=FuzzDecodeQualityConfiguration -fuzztime=50000x -parallel=1
```
