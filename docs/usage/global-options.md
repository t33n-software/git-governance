# Global options and interaction

```text
--interactive auto|always|never   default: auto
--output human|json               default: human
--quiet                           suppress successful human output
--color auto|always|never         default: auto
--accessible                      use screen-reader-friendly prompts
--remote <name>                   default: origin
--repo <path>                     default: current directory
--config <path>                   explicit preferences file
--quality-config <path>           repository-local quality gate configuration
--dry-run                         generate a plan without Git mutation
--yes                             confirm a mutation non-interactively
--timeout <duration>              default: 30s
```

`--interactive`, `--output`, and `--quiet` have separate meanings:

- `--interactive never` disables all prompts and requires every mandatory
  value as a flag.
- `--interactive always` requires a real terminal on both standard input and
  standard output; it fails before a workflow starts when no terminal exists.
- `--output json` writes one versioned JSON result to stdout.
- `--quiet` suppresses successful human output only.
- `--color auto` emits ANSI color only to a terminal, `--color always` forces
  it, and `--color never` uses plain human output and line-oriented prompts.
- `--yes` is required for a mutating non-interactive command unless
  `--dry-run` is used.

For automation, use `--interactive never --output json`. A pull request is
created only when `--create-pull-request` is supplied to a workflow.
The hosting provider carries no flag: it is derived from the selected remote.
When that remote resolves to a GitHub hosting identity, the CLI uses the
GitHub App session bound to that repository or a managed credential broker,
and it fails closed with the authentication remediation when none is bound.
The interactive
`auth login github` command is intentionally unavailable in automation; see
[GitHub App authentication](authentication.md). Never pass a GitHub token as a
flag or save credentials in preferences.

When an interactive human workflow completes a remote refresh, its success
header starts with:

```text
🟢 Remote references fetched and stale references pruned from <remote> before this operation.
```

The status appears only after a successful `git fetch --prune <remote>` and is
absent from dry runs, `--interactive never`, JSON output, and `--quiet`.
Fetching refreshes configured remote-tracking references; it does not pull or
switch a local branch.

## Interactive input validation

Every interactive field explains its canonical input contract before accepting a
value. If a value is invalid, the command stays at that exact field, prints the
actual value (when safe), the violated rule, the expected format, a valid
example, and the correction. The prompt can be retried without limit; accepted
earlier values are retained and the workflow does not restart.

An error after accepted inputs—for example a repository or Git failure—includes
the input summary used by that command. Inline field diagnostics intentionally
show only the failing field so a correction remains focused. Git failures
separate operation context from the bounded Git diagnostic; the context is not
mislabelled as an input value.
