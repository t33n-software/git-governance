# Description: Adapter Relationship
[INTENT: CONTEXT]

---

## 1. Scope Overview
[INTENT: CONTEXT]

This detail surface owns the relationship between the portable core and the
optional source-repository adapter that maps the logical binary invocation to
this repository's Go source entrypoint.

---

## 2. Information Register
[INTENT: REFERENCE]

|| ID | Type | Description | Change | Status |
||----|------|-------------|--------|--------|
|| ADP-001 | INFORMATION | Source adapter relationship and portability | No | Active |

---

## 3. Information Units
[INTENT: SPECIFICATION]

### 3.1 ADP-001: Source adapter relationship and portability
[INTENT: SPECIFICATION]

**Type:** INFORMATION

**Description:**

The parent workflow directory contains an optional source-repository adapter:

```text
../prompt.md
```

That adapter fully reads this core through the relative path `core/prompt.md`
and maps the logical binary invocation to the repository's Go source
entrypoint. It contains no duplicate workflow policy.

Other projects use this core directly with an installed `git-governance`
binary or supply their own narrow adapter. The core remains the single
workflow authority in both cases.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `../prompt.md` | Optional Go-source adapter for this repository | Adapter activation contract |
|| `../CONVENTIONS.md` | Adapter-specific source-binding conventions | Adapter conventions |

---

## 4. Conventions and Constraints
[INTENT: CONSTRAINT]

- An adapter may only bind binary resolution or transport; it never weakens
  endpoint semantics or governance order.
- The core never depends on a specific adapter, repository path, or checkout.

---

## 5. Path Index
[INTENT: REFERENCE]

|| # | Path | Relevance | Unit IDs |
||---|------|-----------|----------|
|| 1 | `../prompt.md` | Optional source adapter | ADP-001 |
|| 2 | `../CONVENTIONS.md` | Adapter conventions | ADP-001 |

---

## 6. Execution Context for LLM Agents
[INTENT: CONTEXT]

Load the core completely before applying any adapter. When the adapter and a
freshly read core disagree, the freshly read core wins and the adapter reports
the drift instead of improvising.
