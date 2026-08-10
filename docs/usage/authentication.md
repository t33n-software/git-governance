# GitHub App authentication

`git-governance` creates GitHub pull requests with a GitHub App identity. It
does not accept a GitHub access token as a flag, store one in preferences, or
load a GitHub App private key on a developer workstation.

GitHub API authentication and Git transport authentication are intentionally
separate:

- GitHub App credentials authorize REST API calls that look up or create a pull
  request.
- SSH, Git Credential Manager, or another Git transport credential authorizes
  `git push`.

Both are required for an end-to-end publication.

## Prerequisites

For a local `github.com` login, the GitHub App owner must configure all of the
following before a user runs the CLI:

1. Install the App only for approved repositories.
2. Grant the least privilege needed for this feature: `Pull requests: Read and
   write` plus `Contents: Read and write` on the selected head repository.
   GitHub requires GitHub Apps that create pull requests to have write access
   to that head repository. Do not add Administration, Actions, Workflows, or
   broad repository access merely for pull-request creation.
3. Enable GitHub App Device Flow.
4. Keep expiring user-to-server tokens enabled. GitHub currently issues an
   eight-hour access token and a six-month refresh token in that mode.
5. Give the user access to the selected repository. A user token is limited to
   the intersection of the App's installation access and the user's access.
6. Supply the App's public client ID to the invoking process. It is not a
   secret:

   ```powershell
   $env:GIT_GOVERNANCE_GITHUB_APP_CLIENT_ID = "<GitHub-App-client-ID>"
   ```

The local CLI never receives the App ID, private key, or client secret. The
Authorization Code Flow is therefore not used locally: GitHub requires a
client secret for its code exchange even when PKCE is present. The Device Flow
is the secure native-client flow because it uses only the public client ID.

The local operating-system secret store must also be available:

| Platform | Protected refresh-session store |
|---|---|
| Windows | DPAPI-protected session file below the user configuration directory |
| macOS | Keychain generic-password item |
| Linux | Secret Service through `secret-tool` |

There is no plaintext-file fallback. An unavailable native store makes login
and publication fail closed.

## Create and install the GitHub App

Open [GitHub Apps settings](https://github.com/settings/apps) and select
**New GitHub App**. If the target repository belongs to an organization,
create the App under that organization (or have an organization App manager do
so), rather than under an unrelated personal account.

Configure the registration in this order:

1. **Identity**
   - Choose a short, unique name, for example `git-governance-pr`.
   - Add a clear description such as `Creates governed pull requests.`
   - Set **Homepage URL** to the project or repository URL.
   - Leave **Callback URL** and **Setup URL** empty. Device Flow does not use a
     callback URL.

2. **User authorization**
   - Leave **Expire user authorization tokens** enabled.
   - Leave **Request user authorization (OAuth) during installation**
     disabled. The CLI begins authorization explicitly through Device Flow; it
     must not silently redirect users during installation.
   - Enable **Device Flow**.

3. **Webhooks**
   - Disable **Active**. This integration does not receive webhooks.
   - Do not configure a webhook URL or webhook secret.

4. **Minimal permissions**
   - Under **Repository permissions**, set **Pull requests** and **Contents**
     to **Read and write**.
   - GitHub requires Contents write for a GitHub App that calls the pull
     request creation API against the head repository. The CLI does not use
     its App access token for Git transport.
   - Set every other repository, organization, and account permission to
     **No access**.
   - GitHub may show **Metadata: Read-only** automatically. It is an implicit
     baseline permission, not an additional business capability, and cannot
     normally be removed.
   - Do not enable Administration, Actions, Workflows, Issues, Commit
     statuses, Members, or any other permission for this use case.

5. **Installation boundary**
   - Under **Where can this GitHub App be installed?**, choose **Only on this
     account** for the account or organization that owns the target repository.
   - Click **Create GitHub App**.
   - On the resulting App page, choose **Install App**, select that same
     account or organization, choose **Only select repositories**, and select
     only the repository for which this CLI may create pull requests.

6. **Client configuration**
   - Copy the **Client ID** from the App settings page. It is public
     configuration; it is not the numeric App ID.
   - Do not generate or distribute an App private key or client secret to a
     developer workstation. The local Device Flow deliberately does not need
     either value.

The CLI uses the App access token only for GitHub pull-request API calls.
Keep SSH or Git Credential Manager configured separately for the developer's
Git transport identity; the CLI never passes its App access token to Git.

## Local login, step by step

Run the explicit interactive command from a real terminal:

```powershell
git governance --interactive always auth login github
```

`auth login github` rejects JSON output, `--interactive never`, and a missing
input or output terminal. A publication command never opens a browser or
starts login implicitly.

The command performs this sequence:

1. It sends the public client ID to GitHub's Device Authorization endpoint.
2. It prints the HTTPS verification URL and one-time user code, then attempts
   to open that URL in the native browser. If opening the browser fails, the
   printed URL and code remain sufficient for a manual browser login.
3. The user signs in to GitHub, verifies any required organization SSO session,
   and approves the GitHub App in the browser.
4. The CLI polls only at GitHub's requested interval. It honors
   `authorization_pending`, increases the interval for `slow_down`, and
   fails when the code expires or the user denies approval.
5. GitHub returns a short-lived user access token and a rotating refresh token.
   The CLI uses the access token only in memory to identify the account.
6. Only the host-bound refresh session, public client ID, account name, and
   refresh expiry are persisted in the native secret store. The access token,
   device code, authorization header, and refresh token are never written to
   preferences, logs, JSON output, errors, or command arguments.

Check the non-sensitive result afterwards:

```powershell
git governance auth status github
```

Status reports host, account, native-store source, and refresh-session expiry.
It deliberately does not display an access token or refresh token.

To remove the local session:

```powershell
git governance auth logout github
```

The Device Flow client removes its protected local refresh session. It cannot
remotely revoke that session because GitHub's revocation endpoint requires an
App client secret, which must not be present on a developer machine. Revoke
the App authorization in GitHub account settings when remote revocation is
required.

## Pull-request publication after login

With `--pull-request-provider github`, the publisher resolves credentials
immediately before every GitHub API call:

1. It parses the selected Git remote into host, owner, and repository.
2. For a local session, it accepts only `github.com`; a session for one host
   cannot be sent to another host.
3. It refreshes an expired or near-expiry access token exactly once per
   host/account profile while concurrent calls wait for the same result.
4. It checks the GitHub App installations and repositories visible to the
   authenticated user and rejects a repository that is not in that
   intersection.
5. It uses the in-memory token for the specific HTTPS request, then returns no
   credential value in the publication result.

The explicit publish command remains unchanged:

```powershell
git governance --interactive never --output json --yes `
  --pull-request-provider github workflow ticket publish `
  --push `
  --create-pull-request
```

The non-interactive publish path uses an existing session. It fails with an
actionable configuration error when no session exists; it never starts Device
Flow, opens a browser, or prompts.

## Safe GitHub API failure diagnostics

GitHub can return a structured `422 Unprocessable Entity` validation response
when a pull request cannot be created. The publisher maps only stable,
actionable categories and never prints raw GitHub response messages, response
bodies, access tokens, or Authorization headers.

The reported categories distinguish:

- App authorization or repository access;
- source-branch visibility;
- target-branch validity;
- pull-request title validity;
- an equivalent pull request already existing; and
- another GitHub validation failure that requires intent or repository-state
  review.

For App authentication failures, confirm that the App is installed only on the
selected repository and has both **Pull requests: Read and write** and
**Contents: Read and write**. GitHub documents the head-repository Contents
requirement for Apps that create pull requests in its
[Pull requests REST API](https://docs.github.com/en/rest/pulls/pulls) and
documents structured validation errors in its
[REST API troubleshooting guide](https://docs.github.com/en/rest/using-the-rest-api/troubleshooting-the-rest-api).

## Managed CI and GitHub Enterprise

Managed workloads use a central credential broker instead of a local user
refresh session. The broker alone holds the GitHub App private key in an HSM,
secret manager, or equivalent protected service. The CLI supports the
following explicit broker contract:

```text
POST <GIT_GOVERNANCE_GITHUB_CREDENTIAL_BROKER_URL>/v1/github/installations/token
Authorization: Bearer <GIT_GOVERNANCE_WORKLOAD_IDENTITY_TOKEN>
Content-Type: application/json

{
  "host": "github.example",
  "owner": "approved-owner",
  "repository": "approved-repository"
}

{
  "access_token": "<short-lived-installation-token>",
  "expires_at": "<RFC3339 timestamp>"
}
```

`GIT_GOVERNANCE_WORKLOAD_IDENTITY_TOKEN` is a short-lived workload identity,
not a GitHub credential. CI injects it from its identity provider; repository
files must not select the broker, token path, or credential command. The CLI
keeps both values in process memory only. The broker must authenticate the
workload, enforce repository policy, and mint a short-lived App installation
token for exactly the requested repository.

### Release lifecycle automation

Pull-request publication and protected release lifecycle operations have
different privilege boundaries. The local Device Flow App remains limited to
pull-request access. A separate, broker-backed release-automation identity is
required for:

- inspecting the promotion merge, immutable tag, GitHub Release, and
  release-to-develop comparison;
- creating a required backmerge pull request; and
- controlled release-line cleanup after its lifecycle is complete.

Protected release and support-line creation has three further ephemeral,
job-scoped boundaries:

```text
Release Request Controller
→ Actions: write, Deployments: write, Contents: read
→ no Contents: write

Protected-Line Executor
→ Contents: write and Deployments: write only for one bound request

Automatic Finalizer
→ Actions: read, Contents: read, Deployments: write
→ no ref-mutation permission
```

The request and executor jobs use separate protected environments
`release-request` and `release-execution`. They receive job-scoped
`GITHUB_TOKEN` values only in the controller process. Those values are
ephemeral controller credentials, not static repository secrets, and must
never be logged or copied to Git transport configuration.

### Functional release and hotfix lanes

No generic `release` environment is an authorization or credential container.
Each controller is bound to exactly one functional lane:

```text
release-request
→ request scope approval
→ no external credential issuer

release-execution
→ one request-bound protected ref mutation
→ no external credential issuer

release-credential-verification
→ read-only OIDC/WIF invocation of the release credential issuer

release-delivery
→ immutable regular-release tag and artifact publication

hotfix-delivery
→ immutable main or support patch tag and delivery verification

hotfix-propagation
→ provenance-validated fix/* candidate publication
```

`release-credential-verification` and `hotfix-delivery` each use only their
own lane-scoped broker URL, WIF provider, and invoker service-account
variables. A lane must not receive another lane's broker variables, private
key, installation token, or transport header.

Reconciliation candidate publication is a further, separate privilege boundary.
The protected `release-reconciliation` environment uses a dedicated publisher
App and broker. That App receives only the repository contents and pull request
permissions required to publish a provenance-validated `chore/*` candidate and
its reviewed pull request to `develop`. It receives no Actions write,
workflow-write, administration, Ruleset bypass, or shared-line mutation role.

Install that identity only for the approved repository and grant the minimum
GitHub permissions required by those exact API calls, including Actions
workflow dispatch and read access plus the limited Contents/Pull requests
access required by the controlled workflow. Do not grant ordinary developer
sessions a Ruleset bypass or reuse a broad administrator credential.

### Hotfix propagation publication

Main-hotfix propagation uses a third, separate broker-backed GitHub App
identity. The Hotfix-Propagation-Publisher App is installed only for the
approved repository and receives only:

```text
Contents: Read and write
Pull requests: Read and write
Metadata: Read-only
```

It receives no Actions, Workflows, Administration, Secrets, or Ruleset-bypass
permission. The protected `hotfix-propagation` environment supplies its own
OIDC/WIF invocation path to a dedicated Cloud Run broker. The controller
temporarily configures masked in-memory Git transport credentials only for the
provenance-validated `fix/*` candidate and removes them before the job exits.
The local Device-Flow App, the release-automation App, and the reconciliation
publisher App are not substitutes for this boundary.

When broker configuration is present, it is used for non-interactive
publication. The publisher derives `https://<host>/api/v3` for a GitHub
Enterprise remote; no legacy API-base environment variable is accepted.

## Doctor and Git transport readiness

`doctor` verifies Git transport authentication separately with:

```text
git push --dry-run --no-verify --porcelain <remote> HEAD:refs/heads/<current-branch>
```

It disables terminal prompts and skips Git hooks for that probe. A public read-only repository is
therefore not mistaken for authenticated push access. The dry run contacts the
remote and validates the configured credentials and write authorization, but
does not create or update a remote ref. A failed or unavailable Git transport
credential makes `doctor` return a classified error.

Run it after configuring SSH or Git Credential Manager:

```powershell
git governance doctor
```

`doctor` verifies Git transport readiness. `auth status github` verifies the
local protected session metadata; a GitHub App repository authorization check
occurs again immediately before pull-request publication.
