# Description: INV-008 Scoped one-time provider-session verification
[INTENT: CONTEXT]

This leaf owns exactly one behavioral invariant of the portable core: INV-008.
The invariant register and the package map live in the parent TOC surface
`description/core-invariants.md`.

---

## INV-008: Scoped one-time provider-session verification
[INTENT: SPECIFICATION]

**Type:** WORKFLOW

**Description:**

When the classified task pattern implies provider publication (pull-request
creation in ticket work, hotfix or release provider steps), the core verifies
the provider session exactly once, immediately after pattern binding:

```text
task pattern bound and publication in scope
-> one auth status check (Help-first)
-> provider_session_verified bound to this scope
-> never re-probed within the same scope
```

A failed prefetch blocks early with the re-login remediation, before branch
or implementation steps begin. A mid-flight provider failure — for example a
session revoked or expired after the prefetch — is the affected endpoint's
fail-closed runtime path and is reported as `BLOCKED` with the re-login
remediation; the prompt deliberately carries no iterative re-check logic for
it. Patterns without provider effects (`diagnostic`, local-only `exploration`)
mark the surface `not_required` and skip the check entirely.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Provider-session prefetch | Section [4.1] |

---

## Conventions and Constraints
[INTENT: CONSTRAINT]

- This invariant is binding for every consuming agent and adapter.
- An invariant change is a material core-contract change and belongs in
  `core/CHANGELOG.md` with a semantic version, a compatibility assessment,
  and the affected surfaces.