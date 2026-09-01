# Hosting platform: GitHub — workflows — provider error diagnostics
[INTENT: REFERENZ]

## Canonical source

This file is the canonical source of truth for the convention of how the
release and hotfix lifecycle family surfaces the provider's error diagnostics
in the CLI surface. The business code re-references this file as the
authority and carries at the affected location only the brief note that — and
why — no redaction happens; the complete rationale lives exclusively here.

## What comes back from the provider

The lifecycle lanes call the provider's REST API (GitHub). A failed operation
returns the error envelope with these properties:

- `message`: the provider-generated plaintext diagnostic text of the
  operation.
- `errors[]`: optional validation entries with `resource`, `field`, `code`,
  and a detail `message`.
- `documentation_url`: a public reference to the provider documentation.
- `status`: the status code.

None of these properties contains credentials, tokens, private keys,
authorization headers, or secrets. The success response carries exclusively
public metadata (refs, SHAs, URLs, user and app master data including the
public `client_id`). The request side — the authorization header and the
request payload — is never read or printed by the diagnostic surface.

## Why no redaction happens

The diagnostic surface reproduces the provider diagnostic text unchanged.
There is deliberately **no** redaction, because the channel is secret-free by
construction — not because a net cleans it afterwards:

1. The request to the provider is fully typed and has no free-text field into
   which a secret could enter; it carries exclusively refs, SHAs, named
   constants, and the request record.
2. The credential lives exclusively in the authorization header; it is never
   written into the body and never echoed back by the provider.
3. The provider's `message` text is a public diagnostic text about the API
   operation and contains no credentials.

The 100% safety is therefore not achieved by enumerating possible contents,
but through the construction proof: a secret-free request, a header-isolated
credential, a provider-side diagnostic text. A redaction mechanism on top of
this channel would be an untenable layer: pattern matching is necessarily
incomplete, and over a channel that carries no secrets by construction, it
carries no control power. The convention rejects such a layer and justifies
the safety through the construction.

## Source re-referencing

The binding source for the error envelope and its properties is the
provider's (GitHub's) official REST API documentation for the deployments and
error surface: it defines the status codes (201, 202, 409, 422) and the error
envelope with `message`, `errors[]` (with `resource`, `field`, `code`,
`message`), and `documentation_url`. This convention re-references that
contract instead of defining it again.

## Contract for the business code

- The diagnostic surface reads a length-bound prefix of the response body,
  extracts exclusively `message` and `errors[].message` (for non-JSON, the
  raw text), and writes the result into the non-sensitive `Diagnostic` field.
- The comment at the affected location only records that no redaction happens
  and re-references this file as the canonical source; it deliberately does
  not carry the complete rationale.
- Request or response headers, tokens, the request payload, or any secret are
  never read or printed.
