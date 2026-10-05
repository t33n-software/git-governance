# CLI conventions — Value-domain model
[INTENT: REFERENCE]

## Canonical source

This document is the canonical source of truth for the value-domain model of
a command-line tool: the eight value classes that classify every flag and
every positional argument, and the binding help duty that follows from the
class. It binds every CLI of the organization, independent of programming
language and subject matter.

## 1. The classification duty

Every flag and every positional argument of every CLI tool is assigned to
exactly one of the eight value classes below. The class determines what the
help must show and what the remaining channels project (see
[single-source-of-truth.md](single-source-of-truth.md)). A flag without a
class assignment is an architecture defect, because its help duty is
undefined and drift control cannot check it.

## 2. The eight value classes

| # | Class | Definition | Help duty (short form) |
|---|-------|-----------|------------------------|
| 1 | `closed-enum` | finite, fixed value set | complete list of the values accepted by this endpoint |
| 2 | `shaped` | grammar template with a fixed skeleton or prefix | grammar + canonical example + subset rules |
| 3 | `free-constrained` | free text with validation rules | compact rule set including the forbidden and an example |
| 4 | `structural-reference` | path, ref, or identifier with runtime-dependent validity | form + resolution rule; no full-prevention promise |
| 5 | `scalar-bounded` | number, duration, size | type, unit, range, default |
| 6 | `boolean-switch` | switch without a value | effect + default (+ negation form, if any) |
| 7 | `composite-token` | repeatable `TOKEN=VALUE` form | transport form + token grammar + example |
| 8 | `secret-reference` | reference to a secret | reference forms only; never a value-accepting argument |

## 3. `closed-enum` — the complete endpoint-specific value list

When a flag has a finite, fixed value set, the help MUST show 100 % of the
values accepted by this endpoint. Two precisions are binding:

1. **Endpoint reference, not global domain.** The shown set is the set that
   *this* endpoint accepts. If an endpoint accepts only a subset of a
   larger domain, the help shows the subset (or names the list plus the
   restriction). A help that lists values the endpoint rejects produces
   misbehavior; a help that omits accepted values does too.
2. **Derivation, not literal.** The list is rendered at registration time
   from the canonical value source, never duplicated as hand-maintained
   text (see [single-source-of-truth.md](single-source-of-truth.md)).

```text
--strategy string   check, auto, rebase, or merge (default "check")
```

Negative example: `--type string   change family` — the value set is fixed,
yet the help names not a single value; the consumer must guess and learns
the domain only through an error message. Equally forbidden: a list that
contains values this endpoint rejects (superset) or omits accepted values
(subset).

## 4. `free-constrained` — the compact rule set with allowed/forbidden

When a flag accepts free text whose validation can fail, the help MUST
proactively show the compact rule set, so that misbehavior is prevented
before the failed validation. The rule set comprises, as applicable to the
flag:

- allowed characters / character class;
- disallowed characters and forbidden content;
- length or size limits;
- the governing naming convention;
- the applied grammar or regex rule;
- one canonical valid example;
- the primary conditions under which validation fails.

The exhaustive grammar remains the property of the referenced specification
(help level 3, see [help-contract.md](help-contract.md)); the help carries
the operationally decisive rule set.

```text
--slug string   1-100 lowercase ASCII letters or digits, words joined by single
                hyphens (rejected unless matching ^[a-z0-9]+(?:-[a-z0-9]+)*$);
                example: add-export-button
```

## 5. The label law

The help distinguishes bindingly between "**rejected by validation**" (a
machine rule with hard rejection) and "**convention-violating**" (a content
contract that is not machine-enforced). Without this distinction the
consumer learns false expectations about rejection behavior. A rule text
that labels a convention rule as a validation rejection — or vice versa —
is an untrue contract and forbidden.

## 6. `shaped` — grammar, example, subsets

When a flag has a grammar form with a fixed skeleton (fixed prefix,
placeholder segments, versioned form), the help MUST show the grammar
itself, one canonical example, and any subset rules of this endpoint (which
prefixes or forms this endpoint accepts or excludes).

```text
--target-line string   declared develop, release/<semver>, or support/<major.minor>
                       target; example: support/2.7
```

The example simultaneously shows the endpoint-specific subset: it lists the
forms this endpoint accepts — and omits the form it rejects.

## 7. `structural-reference` — form and resolution rule, no full-prevention promise

When a flag references paths, refs, or identifiers whose validity is
decidable only at runtime (existence, registration, state), the help MUST
name the **form** and the **resolution rule** — and MUST NOT pretend it
could fully pre-validate. A help that promises an exhaustive list of valid
instances it cannot guarantee is an untrue contract and forbidden.

```text
--base string   canonical branch name or <remote>/<branch> on the selected remote;
                existence is resolved at runtime; example: origin/develop
```

## 8. The compact classes

1. **`scalar-bounded`:** the help shows type, unit, value range, and
   default.
2. **`boolean-switch`:** the help shows the effect of the set switch and
   the default; an existing negation form is named.
3. **`composite-token`:** repeatable key-value flags show the transport
   form (for example `TOKEN=VALUE`), the token grammar (allowed token
   characters, reserved tokens), the repeatability, and an example. If the
   CLI transport form differs from the grammar of the target artifact (for
   example `=` on the CLI versus `: ` in the artifact), exactly this mapping
   is shown.
4. **`secret-reference`:** secrets are NEVER accepted as a value argument
   (process-list and shell-history exposure). The help shows only the
   allowed reference forms (environment variable, file, broker) — a
   value-accepting secret argument is a contract breach per se (see
   [security-and-governance.md](security-and-governance.md)).

```text
--timeout duration   positive duration for external processes (default 30s)
--push               validate and push after committing (default false)
--footer strings     footer as TOKEN=VALUE; token: letters, digits, hyphens;
                     repeatable; example: --footer Refs=#123
--token-env string   environment variable that carries the access token (default TOOL_TOKEN)
```

## 9. Multi-value parameters — one value domain, one repeatable flag

A value domain that accepts multiple values is carried by exactly ONE flag
on exactly ONE endpoint, and that flag is repeatable:

1. **One domain, one flag.** Each occurrence appends one value. A second
   flag or a second endpoint for the same value domain is the
   duplicated-domain-surface antipattern: it doubles the command surface,
   splits the help, and drifts independently.
2. **Two transport forms, one domain.** Repetition (one value per
   occurrence) is the canonical form for agents — one value per flag
   avoids quoting hazards and keeps per-value shell completion. Comma-
   separated expansion within one occurrence is the compact form for
   humans. Both forms carry the same value set; they are transport
   variants of one domain, never two domains.
3. **Reserved delimiter.** The separator is reserved in comma-capable
   domains: a value containing it is not expressible in the comma form
   and fails closed at existence or resolution time. The hazard is
   disclosed in the flag's help duty.
4. **Help duty.** Every repeatable flag declares repeatability and both
   transport forms; the text is generated from the value register, never
   hand-maintained (see [single-source-of-truth.md](single-source-of-truth.md)).
5. **Error discipline.** A multi-value failure names the concrete failing
   element — the value and its position — never a bare count.

```text
--stage strings    repository-relative path; existence is resolved at runtime;
                   repeatable, one path per occurrence, or comma-separated
                   values in one occurrence
--footer strings   footer as TOKEN=VALUE; repeatable; example: --footer Refs=#123
```

Repetition is not an inefficiency: repeated flags are argument transport
within ONE process invocation, not repeated execution. Separate endpoints
per value count would multiply the command surface without any domain
value.
