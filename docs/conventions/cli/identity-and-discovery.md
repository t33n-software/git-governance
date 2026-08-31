# CLI conventions — Identity and discovery
[INTENT: REFERENCE]

## Canonical source

This document is the canonical source of truth for the identity and
discovery conventions of a command-line tool: the stable binary name, the
machine-readable version, the grouping law of the command tree, and the
visibility law for commands. It binds every CLI of the organization,
independent of programming language and subject matter.

## 1. One stable binary name

Every CLI tool has exactly one stable binary name:

- lowercase, kebab-case;
- identical across repositories, packages, and distribution channels;
- identical on every supported platform.

Repository-specific alias names, renamed builds, or special-purpose binaries
of the same tool are forbidden: identity is how consumers, documentation,
scripts, and automation recognize the tool, and every alias fractures that
recognition.

## 2. Machine-readable version

The root command answers `--version` (and its declared short form, if any)
with a machine-readable version identifier:

- the output is stable in shape and parseable without heuristics;
- the version identifies the CLI contract (see
  [compatibility-and-lifecycle.md](compatibility-and-lifecycle.md)) — a
  consumer can derive contract compatibility from it;
- build metadata (commit, build date) may follow the version identifier but
  never replaces it.

## 3. Consistent command-tree grouping

The command tree follows one uniform grouping law on every level:

- the same ordering principle (for example noun-then-verb) applies to every
  group and every leaf;
- the same concept carries the same name in every tool of the organization;
- a command group is a navigation boundary, a leaf command is a use-case
  boundary — both are deliberate design acts, not accidental accumulation.

## 4. The visibility law

Commands intended for end users or automation consumers appear in the
navigation of their parent command. A hidden marker is reserved exclusively
for operator-internal or machine-internal endpoints (for example controller
callbacks that consumers never invoke directly). A consumer-relevant command
that is invisible in its parent's help is a discoverability defect.

## Positive example

```text
tool --version            # 2.8.0
tool <noun> <verb> ...    # the same grouping principle on every level
tool release --help       # lists cut, promote, ... — every consumer command visible
```

## Negative example

The same tool ships as `tool` in one repository and as `tool-cli` in
another; a consumer-facing command is marked hidden and appears in no parent
navigation; `--version` prints a decorative multi-line banner instead of a
parseable identifier. Each of these breaks a consumer's ability to discover
and pin the tool.
