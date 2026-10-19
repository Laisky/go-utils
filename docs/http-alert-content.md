# HTTP alert content and admission

The default alert path forwards the application-chosen entry message and logger
name, plus every supported supplied Zap field, without an SDK content allowlist,
credential replacement, URL rewriting or payload truncation. This deliberately
changes the former default diagnostic policy. Applications that require that
policy must explicitly select `WithAlertDiagnosticRedaction()`; the existing
`WithAlertFieldAllowlist(...)` keeps its explicit selection/redaction contract.
`SanitizeDiagnosticText` remains available for application-owned policies.

## Attach the common alert core

```go
alert, err := log.NewAlert(ctx, endpoint,
    log.WithAlertType(alertType), log.WithAlertToken(token),
    log.WithAlertMaxMessageBytes(1<<20),
)
if err != nil { return err }
defer alert.Close()
logger := zap.New(zapcore.NewTee(localCore, alert.GetZapCore()))
logger.With(zap.Namespace("worker"), zap.String("url", originalURL)).Error(
    "conversion failed", zap.String("html", chosenInput), zap.Error(cause),
)
```

The core follows eager Zap `With` binding: object callbacks run at binding, and
mutable objects/byte slices are snapshotted then, without retaining the original
interfaces. Later `With` and call-site fields retain namespace ordering. Clones
are independent and can write concurrently. `Fields()` returns a fresh slice
containing an immutable Inline JSON snapshot; it does not expose the original
field types or mutable callbacks. It preserves JSON numbers and duplicate keys.

The old `GetZapHook` remains callable and preserves the fields actually supplied
to it. It is deprecated for contextual logging: the enclosing Zap core has already
encoded bound fields, and the fork hook supplies call-site fields before bound
fields. A hook cannot recover original namespace binding order or earlier mutable
object snapshots. Use the alert core directly, without also attaching the alert
hook around the tee (tee child contexts would be concatenated again).

## Representation and failure boundaries

Fields use Zap's standard JSON representation: Binary is base64; Duration uses
Zap's default nanosecond number; rich errors retain text, verbose text and causes;
objects, arrays, namespaces, nulls, empty containers and unfamiliar keys are kept.
Duplicate keys remain duplicate JSON keys; consumers must choose how to interpret
them. This is a JSON logging representation, not a general Go object round trip.
`zap.Reflect` explicitly selects standard `encoding/json` projection, including
JSON tags, omission, custom marshalers and standard Unicode normalization. Its
representation is compared with the ordinary Zap encoder in regression tests;
it is not replaced with an experimental JSON codec. Use Binary for arbitrary
original bytes. Native String/ByteString values, keys and metadata require valid
UTF-8; malformed text fails visibly rather than being replaced by the alert layer.

Failures or panics returned through SDK-invoked encoding callbacks, unknown field
types or unsupported reflected values return `ErrAlertEncoding` and reject the entire record before enqueueing;
partial object data and synthetic error markers are not sent. Invalid callbacks
are application errors. They may allocate or block themselves, as with normal
Zap, so the SDK does not claim to sandbox arbitrary application marshalers.

The default **complete formatted alert** limit is 16 KiB, configurable from
1 byte through 1 MiB using `WithAlertMaxMessageBytes`. Metadata, JSON escaping and
base64 expansion count toward it. `ErrAlertMessageTooLarge` rejects the whole
record; no field is silently cut, omitted or replaced with a size marker. A larger
configured budget allows complete larger inputs (actual HTTP tests cover 20 KiB).
This finite in-memory budget is independent of a downstream service's limits.
No application-owned file, archive, replay endpoint or storage directory is added.

`Send`/`Write` success acknowledges **local admission only** into the existing
20-message asynchronous queue. The queue can overflow; a configured rate limiter
can reject delivery. HTTP/GraphQL failures are asynchronous and are not retried or
claimed as recipient delivery. `Close` cancels, `Done` waits for reference release,
and `Sync` does not flush. The GraphQL mutation and textual metadata framing stay
unchanged, so existing receivers can parse requests, but receiver clipping remains
a release prerequisite.

## Receiver limitation and supported large-content proposal

The existing receiver's `TelegramMonitorAlert(type, token, msg)` clips `msg` at
3000 Go runes and appends a marker before sending. This is receiver application
policy, not Telegram's documented text limit. Raising only the SDK limit does
not make that receiver lossless. Do not release a crawler relying on full inputs
until the receiver change is qualified and authorized.

Telegram documents `sendMessage` as 1–4096 characters after entity parsing; the
limit is not a byte budget, and the documentation does not define that number
as Go runes. It documents `sendDocument` uploads up to 50 MB, with document captions
limited to 0–1024 characters after entity parsing. The MB convention is unspecified.
Sources checked 2026-10-18:
[sendMessage](https://core.telegram.org/bots/api#sendmessage),
[sendDocument](https://core.telegram.org/bots/api#senddocument).

Proposed receiver repair, **not implemented or delivery-qualified in this SDK PR**:
authenticate the same mutation, keep the same subscribers/bot credentials, send
short literal text without Markdown/HTML parsing, and send complete larger `msg`
bytes as an in-memory streamed `.txt` Document to each same subscriber. Use a fresh
reader/document for every recipient and retry; omit captions or use bounded fixed
ASCII metadata. The pinned telebot FileReader transport supports multipart streaming
without an application file. Do not switch GraphQL ingress to multipart (its upload
handler can spill files), add an archive destination, or reuse the unrelated
persistent Arweave upload route. Set and test an explicit receiver byte limit below
its qualified transport budget; reject larger inputs intact, before acknowledgement.
Proxy/runtime ingress limits remain to be verified. Cancellation, partial recipient
failure and remote acceptance must be reported honestly; successful enqueue or
HTTP acceptance is not proof that every recipient received the content.

Rollback restores the prior SDK version and its default sanitation/omission policy.
Explicitly selecting diagnostic redaction on the new version provides the prior
policy while retaining lifecycle fixes. No wire-schema or persisted-data migration
is required. Crawler and receiver releases remain separately coordinated.
