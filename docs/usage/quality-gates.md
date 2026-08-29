# Project-agnostic quality gates

The CLI does not guess a project's language, package manager, build command,
or linter. Hardcoding `go test`, `npm test`, `mvn test`, or any other stack
command would make a generic Git tool architecturally incorrect.

Instead, a trusted repository can opt in with
`git-governance.quality.json` at its root, or an explicit
`--quality-config <path>`. Commands are executed as an executable plus argument
array; no shell command string is interpreted.

```json
{
  "schemaVersion": 4,
  "toolchain": { "language": "go", "version": "1.26.6" },
  "extends": [],
  "gates": [
    {
      "name": "unit-tests",
      "command": "go",
      "args": ["test", "./..."],
      "timeout": "2m"
    },
    {
      "name": "documentation-links",
      "command": "npm",
      "args": ["run", "docs:check"],
      "includeFamilies": ["docs"],
      "timeout": "2m"
    },
    {
      "name": "stress",
      "command": "./scripts/stress-test",
      "includeFamilies": ["feature", "perf"],
      "timeout": "2m"
    }
  ]
}
```

Each gate uses a repository-relative working directory and a positive Go
duration timeout. Paths that escape the repository, shell control characters,
unknown JSON fields, duplicate gate names, and unbounded configuration are
rejected.

The `toolchain` block is required and pins the toolchain identity by
`language` and `version` (for example `go` and `1.26.6`); a missing,
malformed, or unpinned identity is rejected. The optional `extends` list
declares capability pack references in the `<capability>@<major>` form (for
example `opentofu@1`); entries must be well-formed and unique, and the runner
validates the declaration without resolving the referenced packs. The
optional `project` block declares binary smoke contracts and fuzz execution
budgets as data; both are strictly validated.

If no configuration exists, the workflow reports `qualityStatus:
unconfigured`; it does not claim that project-specific checks passed. The
configuration is a trust boundary because running project-defined commands can
execute project code. Review it before using it in an unfamiliar repository.

When a valid configuration exists, each gate receives a typed branch-family
scope. A gate without `includeFamilies` or `excludeFamilies` inherits the
schema-owned default family set, which a repository may override through a
named `defaults` block. `includeFamilies` selects only the listed families;
`excludeFamilies` is applied afterward and removes specific families. A
multi-ref push runs every eligible gate once after its per-ref governance
checks pass.

For a governed publication, the full local suite runs after the final allowed
base synchronization. A successful run records a short-lived proof in
repository-local Git metadata, not in a tracked file. The proof binds the
outgoing ref and commit, target-base revision, remote, quality-configuration
digest, selected gates, toolchain, clean worktree, and creation time.

`validate pre-push` always performs structural validation for every update. It
can reuse the proof only when every binding remains exact and fresh. A missing,
expired, or mismatched proof triggers one full-suite fallback for the batch; a
corrupted or incomplete proof fails closed. This local optimization never
replaces remote CI, required checks, review, or branch protection, and it does
not permit `--no-verify` or hook disabling.

The recommended default includes every official working family:
`feature`, `fix`, `docs`, `refactor`, `chore`, `test`, `perf`, and `hotfix`.
`scratch` is absent from that default because it is private exploration. It is
not a hardcoded exception: a repository can include `scratch` for one small
gate without enabling expensive gates there. This lets a documentation branch
run link checks while skipping a stress test, or a performance branch run a
stress test that other families do not need.

## This repository's full-quality gate

This repository configures one `full-local-build` gate:

```text
go tool -modfile tools/go.mod quality-gate
```

The repository configuration, rather than the generic CLI, owns that Go
command. It ensures that governed publication runs the same complete local
picture as CI on the current native platform: formatting, linting, type checks,
tests, uncached 100%-coverage, race detection, static analysis, vulnerability
checks, fuzz smoke tests, Lefthook validation, and native binary smoke checks.
Cross-platform native quality remains a CI responsibility; see
[development verification](../development/verification.md).
