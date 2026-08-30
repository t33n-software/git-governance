# Scratch transfer

## Create a private scratch branch directly

A scratch branch must start from the local official branch for the same ticket.
The interactive flow asks for that base; non-interactive use supplies it
explicitly:

```powershell
git governance --interactive never --yes branch create `
  --family scratch `
  --key ABC `
  --ticket 123 `
  --slug export-exploration `
  --base feature/ABC-123-add-export-button
```

`origin/feature/...`, a shared line, another scratch branch, and a branch for a
different ticket are rejected. Use `workflow ticket start --scratch` when a new
official branch and its private exploration branch should be created together.

## Squash a scratch branch into its official branch

`branch merge-scratch` transfers the current `scratch/*` branch as exactly one
governed commit to its local official ticket branch. It resolves the target by
ticket ID, not by the slug: `scratch/ABC-123-export-exploration` may therefore
transfer to `feature/ABC-123-add-export-button`, `fix/ABC-123-...`, or another
official family for `ABC-123`. The two descriptions do not need to match.

When exactly one local official branch has the same ticket, no target input is
needed. If no local official branch exists, the command stops with
`SCRATCH_TARGET_BRANCH_MISSING`. If manual work created multiple local official
branches for the ticket, specify the intended target explicitly:

```powershell
git governance branch merge-scratch `
  --target feature/ABC-123-add-export-button `
  --type feat `
  --subject "add export button" `
  --body "## Motivation`n`nDocuments the discarded experiment paths."
```

The standard human flow first displays the fixed target branch, ticket key, and
ticket ID. It then presents every supported commit family and asks for the
description that follows `: ` in the generated header and for the mandatory
body that documents the discarded experiment paths. The key and ticket are
derived from the resolved target and are never selectable in this flow.

Non-interactive automation uses the existing interaction contract rather than a
separate `--silent` flag:

```powershell
git governance --interactive never --yes branch merge-scratch `
  --type feat `
  --subject "add export button" `
  --body "## Motivation`n`nDocuments the discarded experiment paths."
```

The command switches to the official branch, applies `git merge --squash`, and
creates the generated Conventional Commit. It never runs `git add .`, pushes,
or deletes the scratch branch. The body is always mandatory for the squash
transfer: it is the only place that records the discarded experiment paths.
Optional `--footer`, `--breaking`, and `--breaking-description` inputs follow
the same structured contract as `commit create`. A squash-merge conflict
remains in Git for explicit user resolution; the direct command does not hide
or automatically discard it. Use `workflow ticket publish --resume` after
resolving and staging a paused scratch transfer.
