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

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Help-first runtime contract | Section [0.2] |

---

## Conventions and Constraints
[INTENT: CONSTRAINT]

- This invariant is binding for every consuming agent and adapter.
- An invariant change is a material core-contract change and belongs in
  `core/CHANGELOG.md` with a semantic version, a compatibility assessment,
  and the affected surfaces.