# Resume ticket publication

Conflict resolution remains a human task. After resolving and staging every
conflict, the binary can resume the existing Git operation without prompts.
A resume never takes a branch input: during a paused rebase Git detaches
HEAD, so the branch is derived from the paused operation state, and any
`--branch` supplied together with `--resume` is rejected fail-closed.
Canonical convention:
[Resume branch derivation](../../../conventions/branches/resume-branch-derivation.md).

```powershell
git governance --interactive never --output json --yes workflow ticket publish `
  --resume `
  --push
```

For a paused scratch transfer, run the same command from the scratch branch
with its original commit input, and include `--target` when target resolution
is ambiguous:

```powershell
git governance --interactive never --output json --yes workflow ticket publish `
  --type feat `
  --subject "add export button" `
  --commit-body-file "<reported absolute path>" `
  --resume `
  --push
```

The commit body file originates from `message scaffold --kind commit-body`;
hand-created message files are contract violations.

Use `--create-pull-request` only together with `--push` and an explicitly
configured hosting provider. A failed resume leaves the Git operation intact
for further manual resolution.
