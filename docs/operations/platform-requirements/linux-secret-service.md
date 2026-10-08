# Linux Secret Service requirement (`secret-tool`)

## Purpose

`git-governance` protects its refresh sessions in the native operating system
secret store of each platform. On Linux the store is the freedesktop Secret
Service (D-Bus), and the CLI reaches it through the `secret-tool` client from
the `libsecret-tools` package.

This document is the canonical operating prerequisite for that Linux
integration. It lives under `docs/operations/platform-requirements/` because
it is a host-operating-system requirement of an authenticated feature, not a
CLI contract, project convention, or architecture decision.

## Dependency hierarchy (parent → child)

```text
git-governance (product)
└── GitHub App Device Flow authentication (feature)
    └── protected refresh-session store (no plaintext fallback)
        ├── Windows → DPAPI (no extra dependency)
        ├── macOS → Keychain (no extra dependency)
        └── Linux → freedesktop Secret Service (D-Bus)
            └── client: secret-tool
                └── Debian/Ubuntu package: libsecret-tools
```

The parent/child relation is a true DDD boundary model: the **feature**
(authentication) owns the requirement, the **platform adapter** (Linux) owns
the concrete store binding, and the **package** (`libsecret-tools`) is the
operating-system mechanism that satisfies the binding. The dependency is not
a build, runtime, or Go-module dependency of the binary; it is a host
integration dependency that exists only when the Linux secret store is used.

## Install

Debian / Ubuntu and Debian-derived distributions:

```sh
sudo apt-get install -y libsecret-tools
```

Other Linux distributions name the package differently (for example the
`libsecret` package family on Fedora/RHEL and Arch-based systems). Use the
distribution's package manager and package name.

## Verify availability

The client is available when the binary resolves:

```sh
command -v secret-tool
```

A missing or unavailable `secret-tool` makes `auth login github` and
pull-request publication fail closed on Linux: the CLI never falls back to a
plaintext session store. Re-run the install command above and verify again
with `command -v secret-tool` before retrying the login.

## Boundaries

- Keep this requirement in the operating platform domain: the instructions
  belong here, not duplicated inside the CLI conventions or the GitHub
  authentication usage guide. Those surfaces reference this document.
- Distribution-specific package names and install commands are the contents
  of this document; a single source keeps the guidance consistent.
- This requirement applies to Linux end devices only. Windows uses DPAPI and
  macOS uses Keychain; neither needs an additional package for the secret
  store.