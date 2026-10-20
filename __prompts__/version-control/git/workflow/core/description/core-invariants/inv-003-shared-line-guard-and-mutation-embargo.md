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

The mutation embargo is active from task-mode binding onward — before and
independent of branch detection; a skipped `BRANCH_CONTEXT_CHECK` never
leaves it unactivated. Immediately after branch detection, the core
classifies the checked-out branch as `shared_line`, `official_working`,
`scratch`, `unborn`, `detached`, or `unknown`; the classification refines
the embargo state and is never its precondition:

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
|| `core/prompt.md` | Embargo initial state, branch classes and embargo | Sections [2.2], [3.1] and [3.2] |

---

## Conventions and Constraints
[INTENT: CONSTRAINT]

- This invariant is binding for every consuming agent and adapter.
- An invariant change is a material core-contract change and belongs in
  `core/CHANGELOG.md` with a semantic version, a compatibility assessment,
  and the affected surfaces.