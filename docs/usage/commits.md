# Commit commands

The canonical header is:

```text
<type>(<KEY-NUMBER>)[!]: <subject>
```

Supported types:

```text
build chore ci docs feat fix perf refactor revert style test
```

Examples:

```text
feat(ABC-123): add export button
fix(ABC-123): address review feedback
docs(ABC-123): document export workflow
feat(ABC-123)!: replace the export contract
```

Create a commit:

```powershell
git governance --yes commit create `
  --type feat `
  --subject "add export button" `
  --stage cmd/git-governance/main.go `
  --stage README.md
```

Non-interactive:

```powershell
git governance --interactive never --output json --yes commit create `
  --type feat `
  --subject "add export button" `
  --stage cmd/git-governance/main.go `
  --stage README.md
```

The ticket defaults to the current ticket branch. An explicit ticket must
match that branch and is retained only as a compatibility check. All supported
commit-creation flows derive the ticket key and ticket ID from the current or
resolved target branch. Direct commits on `main`, `develop`, `release/*`, and
`support/*` remain forbidden.

The active local policy requires signed commits (`policy describe` reports
`commitSigning: required`). Every commit the CLI creates is verified against
the configured signing identity immediately after creation; an unsigned or
unverifiable commit fails closed with `COMMIT_SIGNATURE_REQUIRED`. Prove the
local signing setup with `doctor` before creating commits; the signing
configuration itself stays with Git (`commit.gpgsign`, `gpg.format`,
`user.signingkey`, `gpg.ssh.allowedSignersFile`).

In interactive mode, the CLI first shows that fixed context, then presents the
canonical commit-family list, and finally asks for the one-line description.
The description must be the non-empty, unpadded text after `: ` and must not
contain control characters. `commit validate --message` and `--message-file`
remain full-message validation inputs because hooks validate the exact message
that Git supplies.

Add a breaking change:

```powershell
git governance --yes commit create `
  --type feat `
  --subject "replace export contract" `
  --breaking `
  --breaking-description "Clients must read the new resource envelope." `
  --stage internal/domain/commitmsg/message.go
```

Validate a message file, for example from a hook:

```powershell
git governance --interactive never commit validate `
  --message-file .git/COMMIT_EDITMSG
```

The tool never runs `git add .`, `git commit --amend`, or `git push --force`.
