# ADR-0009: Channel-Owned Delivery and Update Model

- Status: accepted
- Date: 2026-10-03
- Scope: distribution, installation, and update of the released binary
- Deciders: Repository governance

## Context

The release pipeline builds, signs, attests, and publishes immutable release
artifacts with SHA-256 checksums and an SBOM. The update model, however, was
unbound: whether the binary needs a self-updater, per-invocation version
banners, or an `update` subcommand, and how script-based and
package-manager-based delivery relate. The tool's hook paths
(`commit validate`, `validate pre-push`) must remain offline-capable, and the
repository is public: every delivery surface must stay organization-agnostic
and self-consistent for external consumers.

## Decision

Delivery and updates are owned by the installation channel, organized as a
delivery hierarchy. Highest level first:

1. **Operating-system package managers** — they own the installation
   location, `PATH`, upgrade, rollback, and update signaling. When this level
   is available for a consumer, lower mechanisms retire for that consumer.
2. **Verification-first install and update scripts** — the next-highest
   available level: resolve the version (explicit pinning or
   verified-latest), download the release artifact plus its SHA-256 checksum
   manifest and signature from the release channel, verify fail-closed before
   any mutation, install atomically with rollback, and report the `PATH`
   state.
3. **Signed release artifacts on the hosting platform** — the immutable
   artifact truth every channel consumes; checksums and signatures from the
   release manifest are the single verification truth.
4. **Source entrypoint (`go run`)** — the development clock only, never a
   delivery channel.

Binding update laws:

- Updates happen exclusively through the installing channel; an update is the
  idempotent re-run of the same installation mechanism against a newer pinned
  version.
- No automatic self-update in version 1, and no `update` subcommand: a
  self-updater would duplicate signature verification, proxy handling,
  rollback, channel choice, and package-manager ownership, and it would open
  a standing, unattended mutation channel without per-event proof.
- Self-update is architecturally correct only when the tool owns its own
  installation channel; when a package manager or a script channel owns the
  installation, that channel is the only update executor.
- Hook paths stay offline-capable: no network version checks inside
  `commit validate` or `validate pre-push`. A version-currency report is
  admissible only as an opt-in, fail-open, non-hook surface (a `doctor`
  extension candidate), and any notice may only display the installing
  channel's canonical update command.
- Deprecation awareness for this offline-isolated tool class comes from
  build-time support-window markers carried in the binary, never from runtime
  network calls.
- Notifications never auto-execute anything and are never the security
  boundary; verification-before-mutation always applies.
- Scripts use native operating-system commands, detect fetcher availability
  (POSIX: `curl`, then `wget`; Windows: native `Invoke-WebRequest` /
  `Invoke-RestMethod`), fail closed with actionable guidance, and document a
  manual zero-dependency download path. Documented one-liner bootstrap forms
  are convenience entries over the verification-first script, never a bypass
  of its verification.

The script-level working standard is canonical in
`docs/operations/installation-and-release.md`.

## Invariants

- Exactly one update authority per installation channel.
- Published artifacts are never replaced; versions are pinned or resolved
  against verified release metadata, never blindly.
- Checksums and signatures from the release manifest are the single
  verification truth; no second checksum scheme exists.
- Hook paths remain offline; currency is the channel's and the consumer's
  duty.
- No shell profile is edited without an explicit user request.

## Rejected alternatives

### Built-in self-updater

Rejected: it duplicates signature verification, proxy handling, rollback, and
channel choice behind the owning channel's back, creating a second update
truth and a standing unattended mutation channel.

### Per-invocation version banner with a network check

Rejected: hook paths must remain offline-capable; a network check inside the
commit-msg gate breaks the offline hard gate for a convenience signal. The
parallel to the offline help convention does not extend to network calls —
help is offline and deterministic; a version check is network.

### Automatic updates

Rejected: auto-update is a standing, unattended mutation channel without
per-event proof; it is incompatible with the verification-before-mutation
posture.

## Consequences

- Verification-first `install.sh` and `install.ps1` implement the script
  level; package-manager manifests are produced from published release
  artifacts once publisher identities exist.
- An `update` subcommand stays obsolete while a package manager or script
  channel owns updates.
- Consumers get update guidance exclusively through the channel's canonical
  update command; the tool stays offline in hook paths.
- The README and getting-started surfaces document this model for consumers.
