# Conventions: Git-Governance Source-Repository Adapter
[INTENT: CONSTRAINT]

## 1. Adapter Boundary

`prompt.md` is a source-repository adapter, not the portable workflow source.
It must load `core/prompt.md` through the relative path from the same
directory and then bind only the executable prefix to this repository's Go
source entrypoint.

```text
portable core:
git-governance <endpoint> ...

this repository:
go run -mod=readonly ./cmd/git-governance <endpoint> ...
```

## 2. Mandatory Relative Loading

The adapter must always resolve:

```text
core/prompt.md
```

relative to the adapter file. It must not use:

```text
- absolute Windows, Linux or macOS paths;
- a user home directory;
- a separately checked-out AI-Base-Rules project;
- docs/, business files or source code as workflow-policy dependencies;
- an external RAG location.
```

If the core cannot be read fully, the adapter blocks rather than reintroducing
a local workflow copy.

## 3. Source Entrypoint Rules

The adapter uses only:

```text
go run -mod=readonly ./cmd/git-governance
```

It must:

```text
- preserve each endpoint chosen by the core;
- preserve the core's Help-first order;
- derive all runtime flags from current root and endpoint help;
- treat a failed source invocation as a blocked prerequisite;
- keep the source entrypoint separate from released binary behavior.
```

It must not:

```text
- build or inspect a dist binary as fallback;
- hardcode current global flags or endpoint arguments;
- add a Go-specific workflow into the core;
- replace a missing source capability with raw Git, gh or a static token;
- duplicate branch, commit, release, hotfix or quality policy.
```

## 4. Drift Prevention

The adapter owns one project-specific fact: this repository executes the
logical binary through the Go source entrypoint. The core owns all portable
workflow semantics. This separation prevents:

```text
source checkout behavior
!=
released binary behavior
```

from becoming a second workflow implementation.

Changes to the source entrypoint, Go module layout or adapter location require
an adapter update. Changes to generic workflow topology belong in the core.
Changes to CLI flags, validation formats or endpoint options belong in the
binary and are discovered through `--help`.

## 5. Required Agent-Workflow Conventions

The complete adapter-plus-core bundle must enforce all of the following:

```text
- Every git-governance endpoint is preceded by its current endpoint-specific
  --help invocation.
- The agent names the endpoint and workflow purpose, but learns required
  values, flags and validation details from current help and policy output.
- Only inputs that cannot be discovered from current help, policy or binary
  validation are proactively supplied by the workflow contract.
- A complete governed workflow defines state, proof gates, branch continuation,
  ticket intake, Scratch decisions, semantic commits, quality, publication,
  conflict handling, release, hotfix delivery and propagation boundaries.
- Scratch is conditional exploration, not an automatic workflow phase.
- Shared-line, release, hotfix and propagation work fails closed when the
  required governed endpoint or protected controller does not exist.
- The adapter and core preserve short audit logs without exposing private
  reasoning, credentials or large prompt payloads.
- The core is the complete portable contract; this adapter is the complete
  repository-local binding required to load and execute it.
```

## 6. Metadata Convention

The workflow directory maintains:

```text
prompt.md       source adapter
CONVENTIONS.md  adapter constraints
DESCRIPTION.md  adapter architecture
CHANGELOG.md    adapter version history
core/           portable workflow and its own metadata pair
```

The repository's stable Cursor rule entrypoint remains a relative Git symlink
to this adapter, not directly to the core, because the adapter is what binds
the portable workflow to this source repository.
