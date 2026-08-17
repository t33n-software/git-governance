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
branch validation, synchronization, and governed synchronization resume
commit creation and validation
ticket start and publication
hotfix start, record validation, delivery verification and propagation
release request, cut, stabilization, alignment, promotion, backmerge and support
authentication, diagnostics, policy inspection and pre-push validation
```

**Current State:**

The registry coverage named branch validation and synchronization without
binding the governed re-entry into a conflict-paused synchronization.

**Target State:**

The registry coverage binds branch validation, synchronization, and the
governed synchronization resume: the `branch sync-base` registry row covers
both the deliberate isolated base synchronization and the governed re-entry
into its conflict-paused rebase or merge operation, with quality re-verified
after mutation and after resume.

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
