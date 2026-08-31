# CLI conventions — Help contract
[INTENT: REFERENCE]

## Canonical source

This document is the canonical source of truth for the help contract of a
command-line tool: why the help surface is the authoritative contract
surface, which help area every command node must carry, how the four help
levels are structured, and why help is generated rather than
hand-maintained. It binds every CLI of the organization, independent of
programming language and subject matter.

## 1. The help surface is the authoritative contract surface

The help of a CLI tool is the only discovery channel that simultaneously
guarantees three properties by construction:

1. **zero side effects** — reading help mutates nothing;
2. **zero network dependency** — help works completely offline;
3. **zero version drift** — help is produced by the same installed binary
   that executes the call.

No other channel (documentation website, wiki, chat, error message)
guarantees all three at once. The help surface is therefore the
authoritative contract surface of the tool, and it is read by humans and
automation agents equally. Both consumer classes must be able to derive a
valid call from the help alone.

## 2. A complete help area on every node

Every CLI tool has a help argument, and every command and subcommand node —
recursively, independent of nesting depth — has its own complete help area.
There is no depth or importance discount: every node is its own area of
responsibility.

Binding rules:

1. `--help` and `-h` are answered on every level; a `help` subcommand is
   the recommended second access form; the root additionally carries a
   machine-readable `--version` (see
   [identity-and-discovery.md](identity-and-discovery.md)).
2. **Parent/child role split.** A parent node answers "what exists
   here?" — it lists its children with a one-line purpose and the path to
   their help. A child node answers "how do I call it correctly?" — it
   carries the complete input contract: usage line, flags with their value
   domains (see [value-domain-model.md](value-domain-model.md)), canonical
   examples, and exit behavior. Both directions are mandatory; neither
   replaces the other.
3. **No hidden consumer commands.** Commands intended for end users or
   automation consumers appear in the parent navigation (the visibility law
   of [identity-and-discovery.md](identity-and-discovery.md)).

## 3. The four help levels

```text
Level 0  root help        purpose of the tool, command families, global
                          flags, the discovery law
Level 1  group help       navigation: children with one-line purpose,
                          recursively per group level
Level 2  leaf help        full contract: usage, flags with value domains,
                          examples, exit behavior
Level 3  deep reference   specification / manpage — generated from the same
                          source, linked by identity
```

## 4. The generation mandate

Help texts are generated from the command registry of the tool; help that is
hand-maintained per command is forbidden, because it drifts from the
registry and the validation. The help is a projection of the same source
that also feeds validation, prompts, completion, and discovery (see
[single-source-of-truth.md](single-source-of-truth.md)).

## 5. Examples are valid and canonical

Every example in the help would pass the tool's own validation — consumers
copy examples verbatim. An example that the validation would reject destroys
the trust foundation of the level-2 surface.

## 6. Deprecations are shown in the help

Deprecated flags and commands carry the deprecation notice and the removal
horizon directly in the help text (see
[compatibility-and-lifecycle.md](compatibility-and-lifecycle.md)).

## Positive example

```text
$ tool --help
# purpose, command families with one-line purpose, global flags,
# hint "tool <command> --help"

$ tool release --help
# purpose of the group, children (cut, promote, ...) with one-line purpose

$ tool release cut --help
# complete input contract of the leaf, including every flag domain
```

## Negative example

```text
$ tool release cut --help
Error: unknown command "cut"
```

A registered subcommand that answers `--help` with an error, or merely
repeats its parent's text, violates the recursion claim: the node has no
help area of its own and its input contract is not discoverable.
