# Code quality and coverage gates in organization ownership
[INTENT: ANWEISUNG]

## Convention

The organization MUST define and enforce its own code quality and code
coverage gates, and these gates MUST be programming-language-agnostic at the
contract surface. The quality and coverage gate MUST NOT be handed over to
the hosting platform. The hosting controls `code_quality` and `code_coverage`
MUST appear neither in the versioned ruleset sources nor become an
organization default.

## The four layers of the convention

1. **Contract surface (language-agnostic).** The organization rulesets
   require only stable, identical check contexts — in the composed era the
   composite form `Quality gates / linux-amd64` and
   `Dependency review / Dependency admission review` (the `linux-only` class;
   the `full` class carries the inline-era form `Quality gates (<os>)` plus
   `Dependency admission review` until the caller adoption of its
   repositories). Every repository — Go, Python, Node.js, or a future
   ecosystem — emits the same contexts; the organization-wide contract never
   names a language-specific tool.
2. **Gate content (producer-owned).** Each repository's own build gate
   computes quality and coverage with its native toolchain — for example
   golangci-lint plus an exact 100% statement coverage check for Go, ruff
   plus coverage.py for Python, eslint plus c8 for Node.js. Evidence is
   produced and enforced at the authoritative producer: the language
   toolchain that computes them in the build.
3. **Sovereignty boundary.** The definition of what counts as a quality or
   coverage violation is organization-owned, versioned, and reviewable in the
   repository. A hosting gate would subject merges to the platform's generic
   preview heuristics: it could fail closed on findings the organization does
   not classify as defects; it would cap the organization at the platform's
   quality convention instead of the stricter project-specific criteria; and
   its rule state would lie outside the versioned JSON sources as a
   non-auditable, UI-only drift surface.
4. **Exclusion of the hosting controls.** Both controls are missing from the
   importable ruleset schema and from the REST rule vocabulary (verified
   against the organization and repository schemas); both are feature-gated
   behind GitHub Code Quality in public preview; coverage is additionally
   redundant because the exact 100% statement coverage gate in the required
   check already enforces a stricter boundary from the same CI evidence
   producer.

## Decision record (normalized)

| Option | Share | Assessment |
|---|---|---|
| Organization-owned gates | 50 % | The only form with producer authority, language uniformity, sovereignty over the error definition, and fail-closed safety |
| Hosting platform controls | 26 % | Fails the portability gate (not versionable) and the sovereignty gate |
| Dual enforcement (both) | 24 % | Fails the redundancy gate: same evidence, no independent producer |

## Safety of the project-owned model

The project-owned model remains safe because the organization ruleset
enforces the context as a merge condition and every change to the emitting
workflow passes the same review and code owner boundary. The existence and
green status of the gate are therefore organization-enforced, while the
content remains project-authoritative.

## Exception and revisit

A single repository MAY enable GitHub Code Quality through the UI as a named,
auditable exception — after verifying availability, plan, and actual PR
results — never as a fleet default and never in the versioned sources. The
re-evaluation of this exclusion happens only when all revisit triggers fire:
the controls enter the importable schema, the feature leaves preview, and —
for coverage — the evidence producer is independent of the existing quality
gate. The expected landing zone would then be exclusively the shared-line
families in both classes, activated through `evaluate` before `active`.
