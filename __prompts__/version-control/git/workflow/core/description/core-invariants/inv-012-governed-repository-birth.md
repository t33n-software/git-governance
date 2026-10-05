# Description: INV-012 Governed repository birth and the unborn branch context
[INTENT: CONTEXT]

This leaf owns exactly one behavioral invariant of the portable core: INV-012.
The invariant register and the package map live in the parent TOC surface
`description/core-invariants.md`.

---

## INV-012: Governed repository birth and the unborn branch context
[INTENT: SPECIFICATION]

**Type:** CONSTRAINT

**Description:**

A repository without commits and without references binds the branch context
`unborn` instead of `shared_line`: no shared line exists yet that the guard
could protect. The `unborn` context never activates the mutation embargo,
because creating the initial content set is the birth's input, not a mutation
of a shared line.

The birth itself runs only through the governed level-1 workflow
`workflow bootstrap`: it creates the signed genesis commit on `main`, creates
`develop` from the same revision, installs the canonical hook boundary, and
emits the genesis evidence record; the remote birth of the shared lines is a
separately confirmed `--push` step. Every other workflow stays blocked on an
unborn repository until the birth has completed.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Unborn branch context, birth routing, birth prohibitions | Sections [3.1], [3.2], [4.1], [4.3], [5.5], and [10] |

**Positive Example(s):**

```text
an unborn repository (no HEAD commit, no refs)
-> the branch context binds `unborn`; no embargo activates
-> the initial content set is created as the birth input
-> `workflow bootstrap` births main and develop under governance
```

**Negative Example(s):**

```text
an unborn repository is classified as a shared line
-> the mutation embargo blocks the initial content set
-> the birth is forced into raw Git outside the governed surface
```

---

## Conventions and Constraints
[INTENT: CONSTRAINT]

- This invariant is binding for every consuming agent and adapter.
- An invariant change is a material core-contract change and belongs in
  `core/CHANGELOG.md` with a semantic version, a compatibility assessment,
  and the affected surfaces.