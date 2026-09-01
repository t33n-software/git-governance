# Tag governance: the version-tag namespace and the namespace floor
[INTENT: SPEZIFIKATION]

This document describes independently what the two tag rulesets of the family
enforce and why. The importable JSON definitions reside under
[`rulesets/github/`](../../../../../rulesets/github/README.md).

## Overview

| Ruleset | Target refs | Class |
|---|---|---|
| `tag-governance: release version tags` | `refs/tags/v*` | classless, `~ALL` |
| `tag-governance: tag namespace floor` | `refs/tags/*` without `refs/tags/v*` | classless, `~ALL` |

## Why two artifacts

A ruleset carries exactly one target, exactly one ref selector set, and
exactly one bypass list. The `v*` namespace and the remaining namespace need
different rules and different bypass actors; two classless artifacts of the
same family are the correct form — never a mixed-together ruleset.

## `07-release-version-tags`: the evidence namespace

Version tags (`v<semver>`) are the immutable evidence anchors of the delivery
chain: the immutable release tag references the approved promotion merge
commit, and the release attestation binds tag, commit, and assets. Therefore:

- **creation**, **update**, **deletion**: only through bypass actors.
- Bypass actors: the release-automation GitHub App (`Integration`,
  `bypass_mode: always`) and the organization owner role
  (`OrganizationAdmin`, `always`) as the named, audited break-glass path. The
  hotfix delivery lane uses the same release-automation identity for tag
  operations; one integration bypass covers both lanes.
- `bypass_mode` is never `exempt`: every bypass MUST produce an audit entry.
- The bypass list is constitutive, not an exception: without it, the ruleset
  would block the governed release automation itself. The concrete app ID in
  the artifact is the owning organization's reference binding to the logical
  release-automation identity; adopters substitute their own app ID (like
  `source` and the check contexts), and the steady-state projection binds the
  concrete ID from the instance bindings.

**Activation precondition (blocking).** A creation-restricted tag ruleset
fails closed for every identity that is not a bypass actor. The governed tag
workflows MUST therefore authenticate as the release-automation app before
activation (broker-minted installation token); a tag push with the repository
`GITHUB_TOKEN` is rejected after activation. Until then, the artifact is
imported with `enforcement: disabled` and only activated after a verified tag
creation through the app identity.

**Relationship to release immutability.** Defense in depth: release
immutability locks the tags of **published** releases; the tag ruleset locks
the entire `v*` namespace regardless of whether a release object exists, and
binds creation to the automation identity.

## `08-tag-namespace-floor`: the floor for the rest

Tags outside `v*` have no canonical role in this architecture: staging is an
artifact environment, releases are `v*`. An ungoverned look-alike tag is a
confusion and supply-chain vector. Therefore:

- **creation** and **update**: only through the organization owner role
  (break-glass).
- Deliberately **no** `deletion`: existing non-`v*` tags remain cleanable;
  the namespace can only shrink, never grow or move.

## Deliberately not included

- **No `tag_name_pattern`:** the tag grammar is enforced by the governed
  release automation at creation time; since only that identity can create
  `v*` tags, a pattern rule would be redundant. The pattern rule types are
  also bound to the Enterprise Cloud entitlement.
- **No required status checks:** tag rulesets carry no
  `required_status_checks`; `do_not_enforce_on_create` does not apply, and
  `bypass_mode: pull_request` exists only for branch rulesets.
- **No classes:** tag governance is class-independent; both artifacts remain
  classless on `~ALL`. Tag rulesets — unlike push rulesets — also exist on
  public repositories.

## The naming triple

Title, selector, and file name form the machine-verifiable triple:
`tag-governance: <aggregate>` ↔ `~ALL` ↔ `07|08-<name>.json`. The tag family
deliberately carries no `quality-gates` class.
