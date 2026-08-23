# Hosting platform: GitHub — workflows — Canonical copies, never symlinks
[INTENT: REFERENZ]

## Canonical source

This file is the canonical source of truth for the convention that the
canonical caller masters and every execution copy of them — this repository's
own `.github/workflows/` callers and every tenant's callers — are
byte-identical regular files, never symbolic links. The caller workflows and
the verification surfaces re-reference this file as the authority; the
complete rationale lives exclusively here.

## The architectural foundation

The release and hotfix lifecycle exists exactly once as canonical caller
masters under `workflows/github/callers/release-lifecycle/`. Every execution
location carries a byte-identical regular-file copy, and the caller hash
record plus the packaging contract tests prove that identity fail-closed on
every change. The copy is the execution materialization; the hash record is
the drift proof. Reference over copy applies to the payload logic (the
`uses:` call); the caller itself is the thin, hash-verified file the platform
executes.

## Why the copy MUST happen

GitHub Actions reads workflow files as regular files from the repository tree
at the platform-enforced location `.github/workflows/`. A workflow that does
not physically exist there as a regular file is never discovered and never
runs. The caller copy is therefore not a convenience duplication but the
platform-mandated execution materialization, and the byte identity keeps the
copy provably equal to the master without any drift window.

## Why a symlink does not work

1. **The platform does not resolve symlinks for workflows.** GitHub Actions
   does not follow symlinks when it discovers or executes workflow files; a
   symlinked caller would never be discovered and never run. This is
   documented platform behavior, not a configuration gap.
2. **The Windows checkout materializes a text file.** With the default
   `core.symlinks=false`, a tracked symlink checks out on Windows without
   Developer Mode or elevation as a plain text file containing the target
   path — the local verification, the dogfooding copy, and the hash proof all
   break deterministically on the fleet's development machines.
3. **The verification model binds content, not link text.** A symlink is
   stored in the Git tree as the target path string (mode `120000`); the hash
   proof would then prove the link text instead of the content, degrading the
   drift guarantee for zero governance gain.

## The sanctioned evolution

Drift between a master and a copy is a fail-closed defect surfaced by the
hash record and the contract tests, never a silent state. If the manual copy
step itself must be automated, the bound answer is a governed sync or render
command that re-materializes the copies from the masters and recomputes the
hash record — never a symlink.
