# Ticket-key preferences

Keys are convenience preferences, not organizational authorization. v1 accepts
every syntactically valid key:

```regex
^[A-Z][A-Z0-9]*$
```

Manage keys:

```powershell
git governance config key add --key PLATFORM2
git governance config key set-default --key PLATFORM2
git governance config key list
git governance config key remove --key PLATFORM2
```

All key commands are scriptable:

```powershell
git governance --interactive never --output json config key add --key PLATFORM2
git governance --interactive never --output json config key set-default --key PLATFORM2
git governance --interactive never --output json config key list
git governance --interactive never --output json config key remove --key PLATFORM2
```

Configuration location uses Go `os.UserConfigDir()`:

| Platform | Default application directory |
|---|---|
| Linux | `$XDG_CONFIG_HOME/git-governance` or `$HOME/.config/git-governance` |
| macOS | `$HOME/Library/Application Support/git-governance` |
| Windows | `%AppData%\git-governance` |

The file is versioned JSON and uses a platform-aware recoverable replacement
strategy. Unix replacement uses same-directory rename semantics; Windows keeps
and restores a temporary `.bak` recovery copy if an interrupted replacement
leaves the target absent. It never stores secrets or a global default ticket
number.

GitHub authentication is not stored in this preference file. The non-secret
GitHub App client ID is entered once at the interactive `git governance auth
login github` prompt and is stored with the protected refresh session in the
operating system's native secret store, bound to the canonical repository
identity of the working directory or `--repo`. It is never stored in this
file and never required again — not as an environment variable, not as a
flag. API access tokens remain in memory and are renewed just in time.

Managed CI uses a broker endpoint and workload identity supplied by its
deployment environment, not by this file:

```text
GIT_GOVERNANCE_GITHUB_CREDENTIAL_BROKER_URL
GIT_GOVERNANCE_WORKLOAD_IDENTITY_TOKEN
```

See [GitHub App authentication](authentication.md) for the complete local
login, broker, host-isolation, rotation, and logout contract.
