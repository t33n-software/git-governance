# Diagnostics, policy, errors, and completions

```powershell
git governance doctor
git governance --output json policy describe
git governance completion powershell > git-governance.ps1
```

Generate scripts for `bash`, `zsh`, `fish`, or `powershell`.

`doctor` is read-only with respect to Git refs and the worktree. It reports the
Git version, repository and history state, selected remote availability, active
rebase/merge/cherry-pick state, Git transport authentication, commit-signing
readiness, Lefthook binary/configuration status, local policy mode, and user
configuration status.

The Git authentication check performs a non-interactive
`git push --dry-run --no-verify --porcelain` for the current branch and
selected remote. It disables terminal prompts and skips hooks, validates real push authorization rather than
anonymous public read access, and never updates a remote ref. A missing,
expired, or unauthorized Git transport credential makes `doctor` return a
classified error. GitHub App API authentication remains separate; inspect it
with `auth status github` and see [GitHub App authentication](authentication.md).

The commit-signing checks prove that the environment can produce governed,
verifiable commit signatures before any commit is created:

- `Commit signing configuration` reads the effective Git configuration and
  requires `commit.gpgsign=true`, `gpg.format=ssh`, a readable
  `user.signingkey` file, a configured `user.email` as the verification
  principal, and a readable `gpg.ssh.allowedSignersFile`.
- `Commit signing proof` signs a bounded canary payload with the configured
  key through the configured signing program (`gpg.ssh.program`, defaulting to
  `ssh-keygen`) and verifies that signature against the allowed-signers file.
  The canary uses a temporary workspace and never mutates the repository, its
  refs, or its objects.
- `Lane signing identity` applies only when the process environment carries a
  Git configuration injection (`GIT_CONFIG_COUNT`): a commit-producing lane
  must inject the machine-identity signing keys (`commit.gpgsign`,
  `gpg.format`, `user.signingkey`, `user.name`, and `user.email`).

Every signing check is fail-closed: a missing or unprovable precondition makes
`doctor` return a classified error, so a broken signing setup surfaces locally
instead of at the remote boundary.

## Errors and exit codes

All errors have a stable code, field, non-sensitive actual value, rule, valid
example, and remediation. Sensitive values are redacted in both human and JSON
output. In JSON mode, errors have this shape:

```json
{
  "schemaVersion": 1,
  "ok": false,
  "error": {
    "code": "COMMIT_TICKET_MISMATCH",
    "category": "governance",
    "field": "ticket"
  }
}
```

Exit codes:

| Code | Meaning |
|---:|---|
| 0 | success |
| 1 | internal failure |
| 2 | CLI usage or missing input |
| 3 | governance or policy violation |
| 4 | repository state failure |
| 5 | Git operation failure |
| 6 | configuration failure |
| 7 | external adapter failure |
| 130 | cancellation |

## Further documentation

- [Architecture Decision Record](../architecture/ADR-0001-GO-CLI-ZIELARCHITEKTUR.md)
- [Policy and validation contract](../specification/POLICY-UND-VALIDIERUNG.md)
- [CLI contract](../specification/CLI-VERTRAG.md)
- [Installation and release design](../operations/INSTALLATION-UND-RELEASE.md)
- [Product acceptance matrix](../TRACEABILITY.md)
- [Contributing](../../CONTRIBUTING.md)
- [Package-manager publication templates](../../packaging/README.md)
