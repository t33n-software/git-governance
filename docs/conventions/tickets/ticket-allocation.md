# Ticket Allocation Governance (TAL)

This document is the canonical single source of truth for ticket-number
allocation across all governed surfaces. Ticket-key namespace governance
(classes, levels, repo admissibility) is consumed by rule IDs; this document
owns the NUMBER-allocation invariant.

## Rules

### TAL-R001: Uniqueness invariant
Every ticket number within a ticket-key namespace is allocated exactly once
across all governed allocation surfaces. The number is the audit anchor of
the governed workflow (branch, commit, pull request, release record all
reference it); a duplicate allocation breaks auditability and is a defect,
never a state.

### TAL-R002: Closed allocation-surface inventory
Allocation truth is derived from exactly these surfaces:

| # | Surface | Location | Read path | Grammar / evidence |
|---|---|---|---|---|
| 1 | Branch refs (local + remote) | Git refs | Git transport (platform-neutral, always) | `<type>/<KEY>-<NUMBER>-<slug>` (BGR-R002/KEY-R003) |
| 2 | Commit envelope history | Git objects, all refs | Git transport (always) | `type(KEY-NUMBER)[!]:` envelope (BGR-R003) |
| 3 | Hotfix release records | `.git-governance/hotfix-release-records/<KEY>-<NUMBER>.json` | Git/file transport (always) | schema-validated record |
| 4 | Genesis evidence record | repo evidence record (`workflow bootstrap`) | Git/file transport (always) | ticket-bound genesis envelope + record |
| 5 | PR / MR titles (open AND closed) | hosting platform | provider capability port (`PullRequestProvider`) | `<KEY>-<NUMBER>: <slug>` |
| 6 | Protected-line request records | hosting platform durable records | provider lifecycle port (`ReleaseLifecycleProvider`) | ticket-bound request record (REL-R019) |

Excluded surfaces (never allocation evidence, false-positive guard):
pull-request bodies, commit bodies, tags.

### TAL-R003: Derived state, never declared
Allocation truth is computed from the surface inventory at gate time. No
allocation registry, ledger, or stored claim state is maintained; a stored
copy drifts at the first allocation and splits the truth (fleet principle
P1, precedent D12). The inventory capability is a measurement, never a
store.

### TAL-R004: Fail-closed intake gates
Every ticket-consuming endpoint validates the requested KEY-NUMBER against a
fresh full-surface inventory — fetch first, inventory second, mutation
third — immediately before binding:

| Endpoint | Consumes | Gate |
|---|---|---|
| `workflow ticket start` | key + number → branch | mandatory |
| `workflow hotfix start` | key + number → hotfix branch | mandatory |
| `workflow release request` | key + number → request record + dispatch | mandatory (the surface that made RG-37 invisible) |
| `workflow release stabilize` | key + number → stabilization branch | mandatory |
| `workflow bootstrap` | key + number → genesis commit + record | mandatory |
| `branch create` | key + number → branch | mandatory |

A collision fails closed with `TICKET_NUMBER_ALREADY_ALLOCATED` (governance
category, exit 3).

### TAL-R005: Hosting-platform-agnostic architecture
Surfaces 1–4 are read through the Git transport and are platform-neutral by
construction. Platform surfaces (5–6) are read exclusively through provider
capability ports — never through platform-native tooling hardwired into the
core. A provider-less repository (`--pull-request-provider none`) runs the
git-core surfaces and fails closed (named degraded mode) on the
platform-only surfaces; a degraded scan never reports a platform-bound
number as "free". Future hosting platforms (GitLab, Bitbucket) are adopted
by implementing the same ports, never by adding a second core path.

The capability dimension extends the degraded mode to the configured
provider: a provider identity whose app class does not carry a surface's
read permission — measured fresh per inventory invocation from the public
app registration, never cached — runs that surface as named-absent with the
permission-class reason, and such a degraded scan never reports a
platform-bound number as "free". A genuine read failure while the
permission is carried stays fail-closed: only the permission-class fact
names an absence, never a failing read.

### TAL-R006: Canonical discovery
Next-free-number proposals are derived only from the governed inventory
capability (help-first re-anchored). PR-title heuristics, fixed page
windows, and title-only scans are forbidden evidence bases for any
allocation decision or proposal.

### TAL-R007: Inventory completeness
The inventory covers the full PR/MR surface through pagination (never a
fixed page window), the full branch-ref surface (local and remote), and the
complete reachable commit-envelope history across all refs, parsed with the
canonical domain grammars (never string matching). No silent truncation; a
surface that cannot be read completely fails the inventory closed.

### TAL-R008: Error contract
The collision problem record carries: `field: "ticket number"`,
`actual: <KEY-NUMBER>`, `holders: [<surface>, <locator>, <actor/requester>,
<timestamp>…]`, `expected`, `rule`, `example` (next free number),
`remediation` (continue the existing allocation, or claim the next free
number reported by the inventory). Shape follows `problem.Details`; the
holder list is the DX answer to "what is duplicated, and where".

### TAL-R009: Placement and reference-direction law
This convention lives exactly once in this open-source home. External
knowledge planes may re-reference these rule IDs; this home carries no
references to private knowledge planes (access-boundary rule: private →
public references are possible, public → private are not). Placement
disputes resolve through the existing placement conventions (fleet home
topology, class-E knowledge rules, hosting-platform layer model), not by
duplicating content.

### TAL-R010: Tracker layer
In the full organizational model, ticket IDs are authoritative tracker
namespaces (KEY-R001) and the tracker is the allocation instance. The CLI
remains tracker-agnostic; the repo-side inventory is the layer that makes
tracker-less operation safe and remains valid as repo-side enforcement
alongside a tracker (the layers complement, they do not compete).