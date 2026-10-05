# Description: INV-003 Shared-line guard and mutation embargo
[INTENT: CONTEXT]

This leaf owns exactly one behavioral invariant of the portable core: INV-003.
The invariant register and the package map live in the parent TOC surface
`description/core-invariants.md`.

---

## INV-003: Shared-line guard and mutation embargo
[INTENT: SPECIFICATION]

**Type:** CONSTRAINT

**Description:**

Immediately after branch detection, the core classifies the checked-out branch
as `shared_line`, `official_working`, `scratch`, `unborn`, `detached`, or
`unknown`.
On `main`, `develop`, `release/*`, and `support/*`, a binding mutation embargo
activates before any other work:

```text
- no file creation, edit, rename, or deletion;
- no staging of any kind;
- no `commit create` or any other commit creation;
- no `branch create` or self-directed branch switching;
- no raw Git mutation.
```

The embargo lifts only after a governed level-1 workflow has created or
confirmed the official working branch and the branch context has been
re-verified. Pre-existing uncommitted damage found on a shared line escalates
to a user decision instead of an autonomous repair through `branch create`
carry-over.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Branch classes and embargo | Sections [3.1] and [3.2] |

---

## Conventions and Constraints
[INTENT: CONSTRAINT]

- This invariant is binding for every consuming agent and adapter.
- An invariant change is a material core-contract change and belongs in
  `core/CHANGELOG.md` with a semantic version, a compatibility assessment,
  and the affected surfaces.