# Description: Endpoint Registry
[INTENT: CONTEXT]

---

## 1. Scope Overview
[INTENT: CONTEXT]

This detail surface owns the role and the coverage of the core's endpoint
registry: which endpoint families the registry maps, which questions it
answers for every endpoint, and why it deliberately carries no option list.

---

## 2. Information Register
[INTENT: REFERENCE]

|| ID | Type | Description | Change | Status |
||----|------|-------------|--------|--------|
|| REG-001 | INFORMATION | Registry purpose and coverage | Yes | Active |
|| REG-002 | INFORMATION | Questions the registry answers per endpoint | No | Active |
|| REG-003 | CONSTRAINT | The registry never duplicates the binary's option list | No | Active |

---

## 3. Information Units
[INTENT: SPECIFICATION]

### 3.1 REG-001: Registry purpose and coverage
[INTENT: SPECIFICATION]

**Type:** INFORMATION

**Description:**

The core contains an endpoint registry for:

```text
repository birth of unborn repositories through the governed bootstrap
workflow
branch validation, synchronization, governed synchronization resume, and the
on-demand shared-line refresh of local checkouts
commit creation and validation
read-only ticket-number allocation inventory (next-free number and holders
before ticket intake)
ticket start and publication
task worktree acquisition, inventory, proven-completion prune and removal
lifecycle
hotfix start, record validation, delivery verification and propagation
release request, cut, stabilization, alignment, promotion, backmerge and support
authentication, diagnostics, policy inspection and pre-push validation
```

**Current State:**

The registry coverage binds branch validation, synchronization, the governed
synchronization resume, the on-demand shared-line refresh, the read-only
allocation inventory of ticket numbers, and the complete task worktree
lifecycle: the `workflow worktree start`, `workflow worktree list`, `workflow
worktree remove`, and `workflow worktree prune` registry rows cover the
governed acquisition of a detached task worktree, the worktree inventory,
the fail-closed worktree removal, and the evidence-based prune of
proven-complete worktrees under hybrid completion evidence. The repository
birth family carries the complete birth lifecycle: the `workflow bootstrap`
row covers the governed birth itself, the `workflow bootstrap recover`
and `workflow bootstrap publish` rows cover the governed pre-birth recovery
of a foreign or aborted pre-staging state and the governed publication
resume of a repository born without `--push`, and the `validate pre-push`
row carries the state-based recognition of the governed remote birth that
completes the birth publication path.

**Target State:**

The register rows describe implemented binary endpoints; the enumeration of
the allocation-inventory surfaces matches the implemented inventory,
including the task-worktree registry that holds pre-start ticket numbers
before any branch, commit, or pull request exists.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Endpoint registry | Section [5] |

---

### 3.2 REG-002: Questions the registry answers per endpoint
[INTENT: SPECIFICATION]

**Type:** INFORMATION

**Description:**

The registry answers:

```text
- which endpoint is required;
- when it is allowed;
- which execution level it belongs to (workflow, bounded command, read-only);
- which workflow transition it supports;
- which evidence must exist before and after it;
- when an unavailable capability must block instead of being bypassed.
```

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Endpoint registry | Section [5] |

---

### 3.3 REG-003: The registry never duplicates the binary's option list
[INTENT: SPECIFICATION]

**Type:** CONSTRAINT

**Description:**

The registry intentionally does not duplicate the binary's option list.
Endpoint identifiers are stable workflow context; every concrete flag, value
form, and interaction mode is derived from the immediately preceding help
result at runtime.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Registry boundary | Section [5] |
|| `core/CONVENTIONS.md` | Authoring rules for this boundary | Sections 2 and 4 |

**Positive Example(s):**

```text
| endpoint | level | when it is required | result boundary |
```

A registry row binds identity, level, timing, and evidence without naming a
single flag.

**Negative Example(s):**

```text
| endpoint | flags copied from a point-in-time help output |
```

A row that freezes flags drifts against the binary the moment the binary
changes its contract.

---

## 4. Conventions and Constraints
[INTENT: CONSTRAINT]

- A capability that is not registered is not part of the governed topology.
- A new governed capability is registered only after the binary implements it;
  the registry never portrays an unimplemented endpoint as available.

---

## 5. Path Index
[INTENT: REFERENCE]

|| # | Path | Relevance | Unit IDs |
||---|------|-----------|----------|
|| 1 | `core/prompt.md` | Endpoint registry | REG-001, REG-002, REG-003 |
|| 2 | `core/CONVENTIONS.md` | Authoring and runtime constraints | REG-003 |

---

## 6. Execution Context for LLM Agents
[INTENT: CONTEXT]

Use this surface to decide whether a capability is part of the governed
topology. Derive every invocation form from the current help of the running
binary, never from this document.
