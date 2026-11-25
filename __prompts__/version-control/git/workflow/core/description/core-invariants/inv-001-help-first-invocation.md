# Description: INV-001 Help-first invocation
[INTENT: CONTEXT]

This leaf owns exactly one behavioral invariant of the portable core: INV-001.
The invariant register and the package map live in the parent TOC surface
`description/core-invariants.md`.

---

## INV-001: Help-first invocation
[INTENT: SPECIFICATION]

**Type:** CONSTRAINT

**Description:**

Every endpoint is discovered immediately before use:

```text
git-governance <endpoint> --help
-> inspect the actual contract
-> execute one matching invocation
-> wait for the real result
```

The core names the necessary endpoint and the reason for calling it. It does
not freeze endpoint flags, value formats, regexes, technical limits or runtime
modes in prompt text.

The duty is scenario-classed: mutating and lane-start endpoints (class S1)
keep the per-invocation help immediately before each actual invocation
absolutely. Read-only idempotent endpoints (class S2) anchor their help once
per open contract epoch; an invalidation event of core [0.2.2] closes the
epoch and returns the endpoint to per-call duty, and an execution-rejected
call forces the error-driven re-anchor of core [0.2.3] with exactly one
corrected re-dispatch. Freshness is a ledger state, never a memory inference.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Help-first runtime contract | Sections [0.2], [0.2.1]–[0.2.3] |

---

## Conventions and Constraints
[INTENT: CONSTRAINT]

- This invariant is binding for every consuming agent and adapter.
- An invariant change is a material core-contract change and belongs in
  `core/CHANGELOG.md` with a semantic version, a compatibility assessment,
  and the affected surfaces.