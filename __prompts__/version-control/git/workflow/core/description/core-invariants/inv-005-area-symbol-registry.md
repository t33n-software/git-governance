# Description: INV-005 Area-symbol registry
[INTENT: CONTEXT]

This leaf owns exactly one behavioral invariant of the portable core: INV-005.
The invariant register and the package map live in the parent TOC surface
`description/core-invariants.md`.

---

## INV-005: Area-symbol registry
[INTENT: SPECIFICATION]

**Type:** CONSTRAINT

**Description:**

All visible status messages and audit records carry a centralized
area-specific UTF-8 symbol defined in the core's symbol registry. Thirteen
architectural areas (context and guard, environment and policy, intake and
decision binding, branch provisioning, implementation, verification, commit,
publication and pull request, release/support lifecycle, hotfix lifecycle,
conflict recovery, waiting and blocked, completion and cleanup) each own one
distinct symbol. Symbols are derived from the architectural area of the
current step, never invented ad hoc, and never replace textual gate results.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Symbol registry and audit records | Sections [0.3] and [9] |

---

## Conventions and Constraints
[INTENT: CONSTRAINT]

- This invariant is binding for every consuming agent and adapter.
- An invariant change is a material core-contract change and belongs in
  `core/CHANGELOG.md` with a semantic version, a compatibility assessment,
  and the affected surfaces.