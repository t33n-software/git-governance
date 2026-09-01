# Push protections: the boundary against secret-shaped artifacts
[INTENT: SPEZIFIKATION]

The `00-push-protections.json` ruleset blocks secret- and key-shaped
artifacts before they reach the commit graph. The push target applies to
every push into the repository and its entire fork network; there is no
branch binding.

## The availability boundary

Push rulesets exist only for **private and internal** repositories and
require the Team plan. Public repositories cannot carry this ruleset; their
boundary is secret scanning with push protection plus the local quality
gates. The selector therefore binds only private and internal repositories
through the `visibility` system property. The file nevertheless remains part
of the canonical set: it is the versioned definition of the boundary,
regardless of where it can be activated.

## Blocked artifact classes

- Key and keystore file extensions: `*.pem`, `*.key`, `*.p12`, `*.pfx`,
  `*.jks`, `*.keystore`, `*.kdbx`, `*.ppk`, `*.gpg`
- Environment and credential files as well as infrastructure states:
  `**/.env`, `**/.env.*`, `**/.envrc`, `**/credentials`, `**/credentials.*`,
  `**/*.tfstate`, `**/*.tfstate.*`

These classes are the artifact types that carry credential material in this
architecture. The list is a safe default; an extension happens only through a
reviewed change in the canonical repository.

## Why the entire `.env` family is blocked

Secret material is independent of the file suffix. `.env.development`,
`.env.test`, and every future dotted variant can carry production
credentials, and committed environment files are a primary harvest target.
The `**/.env.*` wildcard is therefore fail-closed against every current and
future variant; `**/.envrc` extends the boundary to direnv files. Because the
path restriction knows no exclusion list, this is intentional — and the
reason the template convention lives outside the blocked family.

## Template architecture

Shareable, non-secret environment defaults MUST live outside the blocked
family: names like `env.example`, `example.env`, or a `templates/env.*`
directory, exclusively with placeholder values. Real values come from the
secret manager or credential broker; local overrides stay in `.env*.local`,
which are both gitignored and push-blocked.

## Defense in depth

- The push rule prevents server-side, including the fork network.
- `.gitignore` prevents accidental staging on the client side.
- Secret scanning with push protection remains the detective layer — on
  public repositories the only one, because no push ruleset can exist there.
