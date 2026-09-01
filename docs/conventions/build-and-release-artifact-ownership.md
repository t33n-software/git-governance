# Build and release artifact ownership

## Purpose

This convention separates versioned sources, volatile build inputs, and
publishable release artifacts. It prevents local smoke binaries or generated
packaging files from contaminating the artifact area managed by GoReleaser.

## Directory contract

| Path | Owner | Versioned | Permitted content |
| --- | --- | --- | --- |
| `docs/` | Product and operations documentation | Yes | Handwritten, reviewable documentation |
| `build/` | Build infrastructure, if needed | Yes | Recipes, scripts, or configuration, no outputs |
| `.build/` | Local and CI build steps | No | Smoke binaries and generated packaging inputs |
| `dist/` | GoReleaser | No | Archives, packages, checksums, SBOMs, signatures, and artifact metadata |
| `release/` | Release lifecycle term | Not as an output directory | Release workflow, immutable tags, GitHub Release, and reconciliation |

`.build/` and `dist/` are excluded through `.gitignore`. The leading dot in
`.build/` marks a tool-private workspace; Go package patterns like `./...`
additionally ignore directories with a leading dot.

## Build and release flow

```text
Versioned sources and docs/
        ↓
.build/bin/ and .build/generated/
        ↓
GoReleaser
        ↓
dist/
        ↓
GitHub Release, checksums, SBOMs, signatures, and attestations
```

The canonical quality gate (`go tool -modfile tools/go.mod quality-gate`) and
the native CI smoke tests write their executable binary into `.build/bin/`.
`cmd/generate-docs` produces shell completions and man pages under
`.build/generated/`. GoReleaser accepts these files as packaging input but
produces its own outputs exclusively in `dist/`.

## Binding rules

1. No preparatory generator, local smoke test, or manual build may write to
   `dist/`.
2. Only GoReleaser may create and manage `dist/` for release artifacts.
3. `release/` or `.release/` must not be used as a substitute for the
   volatile build workspace.
4. Handwritten documentation stays under `docs/`; only reproducible
   packaging inputs belong under `.build/generated/`.
5. Changes to these paths require a direct contract test update as well as a
   non-publishing GoReleaser snapshot check.

## 12-Factor classification

`.build/` belongs to the build phase and contains no publishable truth.
`dist/` is the artifact handover area between build and release. The release
combines the immutable tag with the release configuration; the run uses
exclusively published artifacts. No runtime process writes into `.build/` or
`dist/`.
