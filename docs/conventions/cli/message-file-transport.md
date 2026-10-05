# CLI conventions — Message-file transport
[INTENT: REFERENCE]

## Canonical source

This document is the canonical source of truth for the message-file
transport of a command-line tool: every multi-line message part (commit
body, pull-request description) crosses the CLI boundary exclusively as a
file, and this file owns the transport contract, the line-ending contract,
and the diagnostic duty for rejected content. It binds every CLI of the
organization, independent of programming language and subject matter.

## 1. The transport contract

A multi-line message part is passed as an absolute path to an existing
plain UTF-8 text file. The loader checks, in order: the path is absolute,
the target is an existing regular file, the size stays within the transport
ceiling (1 MiB), and the content decodes as UTF-8 without a silent repair
attempt. The content is then carried verbatim into the validation layer —
the loader itself performs no content transformation.

The verbatim read is deliberate: the transport boundary is not the place
where a message becomes valid. Validation owns the content contract, and
the line-ending contract below owns the only normalization that happens.

## 2. The line-ending contract

Text entering a message part is valid with either line-ending convention:

- **LF** (`\n`, U+000A) — the canonical internal form;
- **CRLF** (`\r\n`) — accepted and normalized to LF before validation and
  before the message is assembled.

A lone carriage return (U+000D without a following line feed) is not a
line ending. It fails closed as corrupt input with a named error, never
silently repaired.

### Why both conventions are accepted

1. **Producers are operating-system native.** Editors, agent write tools,
   and shell chains emit the platform's native endings; Windows producers
   emit CRLF, and that behavior is not reliably controllable by the
   caller. A contract that mandates a single convention systematically
   fails one operating system's producers.
2. **The receiving boundary owns normalization.** Translation of a
   non-canonical input happens at the boundary that receives it, before
   validation runs — so every validation layer sees the same canonical
   form and no layer can disagree about line endings.
3. **Git carries LF internally.** Commit messages are stored with LF
   line endings; normalizing at ingestion keeps the stored artifact and
   the validated content identical.
4. **Cross-environment parity.** The same input file validates
   identically on Windows, Linux, and macOS, because the conversion is
   the tool's job — never the producer's discipline.

## 3. The diagnostic duty

When validation rejects message content, the failure record must name the
exact offending element: the code point (for example `U+000D CARRIAGE
RETURN`), its line, its byte offset, and a sanitized context line in which
control characters render as visible escapes. An error that says only
"control characters are forbidden" without naming the character forces the
consumer into a guess-and-retry loop against an invisible defect.

The diagnostic duty is why the line-ending normalization must happen
before validation: after normalization, every remaining control character
in an error record is a genuine content defect, never a line-ending
artifact.

## 4. The prompt boundary

The message grammar, the character inventory, and the line-ending contract
are owned by the tool: they live in its validation and in this document,
and they surface to agents through the endpoint help at invocation time.
Workflow prompts and adapters do not restate them. A prompt that copies
validation rules drifts silently when the tool evolves; the help, read
before every call, always carries the current contract.
