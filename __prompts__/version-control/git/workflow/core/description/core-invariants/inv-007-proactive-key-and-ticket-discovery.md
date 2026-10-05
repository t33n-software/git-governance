# Description: INV-007 Proactive key and ticket discovery
[INTENT: CONTEXT]

This leaf owns exactly one behavioral invariant of the portable core: INV-007.
The invariant register and the package map live in the parent TOC surface
`description/core-invariants.md`.

---

## INV-007: Proactive key and ticket discovery
[INTENT: SPECIFICATION]

**Type:** WORKFLOW

**Description:**

When the active task requires a ticket and the user supplied neither key nor
ticket, the core runs an evidence-based discovery before the ticket stop
sequence. The discovery binds exactly one project — the explicitly passed
project (for example via `--repo`) or the current working directory — and
selects exactly one capability level in priority order:

```text
P1  the governed allocation-inventory capability of the binary (help-first;
    it measures the complete allocation-surface inventory of the bound
    project)
P2  available context tools that actually cover the complete surface
    inventory
P3  platform read paths of the binary's provider ports, only for
    proven-public repositories
P4  discovery unavailable (no capable level, or non-public repository
    without authenticated access) -> brief diagnostic status log, then the
    unchanged stop sequence
```

The proposal basis is the governed inventory capability: the complete
allocation-surface inventory across branch refs, commit-envelope history,
hotfix release records, the genesis record, pull-request titles, and
protected-line request records. A number is free only after a complete
inventory, a surface that cannot be read completely fails the inventory
closed, and projection-only bases are never valid allocation evidence. A
multi-decision matrix over task pattern, key distribution across the
inventoried surfaces, holder evidence per surface, the highest allocated
number, and the execution level produces a proposal for the level-1 workflow
or level-2 commands; collisions are additionally fail-closed by the binary's
intake gates. The proposal binds
nothing: only an explicit user confirmation or user-supplied replacement
values set `ticket_binding` to `user_provided` or `confirmed_proposal`. If
discovery fails or the proposal is declined without replacement values, the
`WAITING_FOR_TICKET` stop sequence applies unchanged.

**Affected Files:**

|| Path | Relevance | Elements |
||------|-----------|----------|
|| `core/prompt.md` | Discovery chain and proposal matrix | Section [4.5] |

---

## Conventions and Constraints
[INTENT: CONSTRAINT]

- This invariant is binding for every consuming agent and adapter.
- An invariant change is a material core-contract change and belongs in
  `core/CHANGELOG.md` with a semantic version, a compatibility assessment,
  and the affected surfaces.