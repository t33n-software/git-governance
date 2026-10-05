# Description: INV-009 Current hotfix capability boundary
[INTENT: CONTEXT]

This leaf owns exactly one behavioral invariant of the portable core: INV-009.
The invariant register and the package map live in the parent TOC surface
`description/core-invariants.md`.

---

## INV-009: Current hotfix capability boundary
[INTENT: SPECIFICATION]

**Type:** CONSTRAINT

**Description:**

The core recognizes the current bounded hotfix endpoints:

```text
start
validate-record
publish
verify-merge
verify-delivery
single-commit propagation
manifest-candidate preparation
```

It requires a reviewed release record, a semantic commit budget, an ordered
manifest, immutable patch delivery evidence and target-local propagation
outcomes. It does not claim that an unavailable protected controller exists.
If a required manifest-publishing capability is absent, the core blocks
instead of replacing it with raw Git or a privileged local token.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Hotfix endpoint family | Section [5.3] |

---

## Conventions and Constraints
[INTENT: CONSTRAINT]

- This invariant is binding for every consuming agent and adapter.
- An invariant change is a material core-contract change and belongs in
  `core/CHANGELOG.md` with a semantic version, a compatibility assessment,
  and the affected surfaces.