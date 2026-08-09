# CLI usage

Use `git governance ...` when the binary is on `PATH`; the direct
`git-governance ...` form is equivalent.

## Command overview

```text
git governance branch list
git governance branch create
git governance branch validate
git governance branch merge-scratch
git governance branch sync-base

git governance commit create
git governance commit validate

git governance workflow ticket start
git governance workflow ticket publish
git governance workflow hotfix start
git governance workflow hotfix publish
git governance workflow hotfix propagate
git governance workflow release cut
git governance workflow release request
git governance workflow release stabilize
git governance workflow release publish-stabilization
git governance workflow release align-promotion-base
git governance workflow release promote
git governance workflow release backmerge
git governance workflow release align-reconciliation-base
git governance workflow release support
git governance workflow cleanup

git governance validate pre-push

git governance auth login github
git governance auth status github
git governance auth logout github

git governance config key list
git governance config key add
git governance config key remove
git governance config key set-default

git governance policy describe
git governance doctor
git governance completion <shell>
```

- [Global options and interaction](global-options.md)
- [GitHub App authentication](authentication.md)
- [Project-agnostic quality gates](quality-gates.md)
- [Branch taxonomy](branches/taxonomy.md)
- [Branch creation](branches/create.md)
- [Scratch transfer](branches/scratch-transfer.md)
- [Base synchronization](branches/synchronization.md)
- [Commit commands](commits.md)
- [Workflow commands](workflows/index.md)
- [Ticket-key configuration](configuration.md)
- [Lefthook integration](lefthook.md)
- [Diagnostics and errors](diagnostics.md)
