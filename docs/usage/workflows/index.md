# Workflow commands

- [Repository bootstrap](bootstrap.md)
- [Ticket start](tickets/start.md)
- [Ticket publish](tickets/publish.md)
- [Resume ticket publication](tickets/resume.md)
- [Hotfix workflows](hotfix.md)
- [Hotfix propagation](propagation.md)
- [Release and support workflows](release.md)
- [Release reconciliation](release-reconciliation.md)
- [Automate release workflows](release-automation.md)
- [Scratch cleanup](cleanup.md)

Every workflow supports the common interaction contract. For automation, use
`--interactive never --output json`, supply all mandatory flags, and add
`--yes` before any mutating operation. Use `--dry-run` to inspect a plan
without mutating Git.
