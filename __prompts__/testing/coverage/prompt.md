---
description: Enforce strict whitebox tests and complete Go coverage
alwaysApply: true
---

# Strict Whitebox Testing and Complete Coverage

- Every changed or new production path needs a direct same-package whitebox test
  for its invariants, branches, state transitions, errors, and cleanup paths.
- Keep external-package, integration, contract, race, or fuzz tests when the
  changed boundary needs them; they complement, never replace, whitebox tests.
- Add a regression test for every fixed defect and make assertions precise
  enough to fail when the defect returns.
- For every substantive Go change, format changed Go files and run:
  `go test ./...` and `go run ./cmd/check-coverage`.
- `cmd/check-coverage` is a required release gate: every executable Go package
  must reach exactly 100.0% statement coverage. Do not claim verification when
  this command was not run or did not pass.
- Follow the repository gate contracts in `CONTRIBUTING.md`,
  `README.md`, and `git-governance.quality.json`; preserve their command-array
  security model and do not hardcode project-specific checks into the CLI.
- The configured quality suite contains `go vet ./...`, but do not run any
  lint or static-analysis command unless the user explicitly requests it.
- If an environment prevents a required check, report it as unverified or
  blocked rather than weakening the test requirement.
