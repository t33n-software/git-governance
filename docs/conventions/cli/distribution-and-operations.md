# CLI conventions — Distribution and operations
[INTENT: REFERENCE]

## Canonical source

This document is the canonical source of truth for the distribution and
operations conventions of a command-line tool: the dependency-free artifact
form, cross-platform parity, the documented lifecycle, and telemetry
discipline. It binds every CLI of the organization, independent of
programming language and subject matter.

## 1. Dependency-free artifact form

The preferred form is a standalone, runtime-independent artifact (for
example a static binary), so the tool can be operated without ecosystem
coupling.

## 2. Cross-platform parity

All supported platforms offer the same commands, flags, and semantics;
unavoidable platform deviations are documented.

## 3. Documented lifecycle

Installation, upgrade, uninstallation, and version pinning are documented;
reproducible builds produce versioned, evidenced artifacts (see
[security-and-governance.md](security-and-governance.md)).

## 4. Telemetry discipline

Telemetry is disabled by default or absent; where present, it is documented
and explicitly opt-in.

## Positive example

The same command behaves identically on Windows, Linux, and macOS; the
installation guide names pinning to an exact version including checksum
verification.

## Negative example

A flag exists only on one operating system without the help naming this;
the tool sends usage data by default; an upgrade path is undocumented.
