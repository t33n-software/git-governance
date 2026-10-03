# Resume ticket publication

Conflict resolution remains a human task. After resolving and staging every
conflict, the binary can resume the existing Git operation without prompts:

```powershell
git governance --interactive never --output json --yes workflow ticket publish `
  --branch feature/ABC-123-add-export-button `
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
  --commit-body-file "$env:TEMP\commit-body-7c9e6679-7425-40de-944b-e07fc1f90ae7.txt" `
  --resume `
  --push
```

Use `--create-pull-request` only together with `--push` and an explicitly
configured hosting provider. A failed resume leaves the Git operation intact
for further manual resolution.
