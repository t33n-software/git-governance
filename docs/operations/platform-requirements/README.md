# Platform requirements

This area carries the canonical operating-platform prerequisites that the
`git-governance` product needs on each supported operating system. It
complements the product installation design in
[installation and release](../installation-and-release.md): while that
document owns how the `git-governance` binary itself is installed, this area
owns what the host operating system must provide so authenticated features
work.

Each child document is the single canonical source for the platform
requirement it defines. Product surfaces reference these documents instead
of duplicating the prerequisite steps.

## Children

| Document | Platform | Requirement |
|---|---|---|
| [linux-secret-service.md](linux-secret-service.md) | Linux | `secret-tool` (`libsecret-tools`) for the freedesktop Secret Service protected-session store |

## Platform matrix

| Platform | Protected refresh-session store | Extra package required |
|---|---|---|
| Windows | DPAPI-protected session file | none |
| macOS | Keychain generic-password item | none |
| Linux | Secret Service through `secret-tool` | `libsecret-tools` (Debian/Ubuntu) or the distribution's `libsecret` package family |