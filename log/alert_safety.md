# Remote alert diagnostics

The Zap alert hook forwards ordinary top-level string, bool, integer, float and
duration diagnostics by default, including fields bound with `logger.With`.
Unknown application stage/category strings are retained rather than reduced to
fixed enums. Error fields become sanitized `Error()` text, without stacks or
object formatting. This restores useful failure context when callers have not
selected an explicit field filter.

Default forwarding removes targeted credential and raw-payload field names and
sanitizes remaining strings, entry messages and logger names. It redacts
recognizable credential assignments, Bearer/Basic authentication, credential
key formats, URLs, labeled body/payload/header text and echoed alert tokens.
Counters such as `query_count`, `token_count` and `auth_failure_count` remain
ordinary diagnostics. `SanitizeDiagnosticText` provides the same text projection
for application source logs; it has no callbacks or configuration. Text above
1 MiB becomes a fixed oversized-diagnostic marker.

This is targeted sanitization, not a guarantee that arbitrary unlabeled prose
is free of user content. Applications must not log raw user content or place
credentials in misleading diagnostic names. Objects, arrays, reflection,
Stringers, binary/byte strings, complex values, time values and pointers remain
excluded. Unknown Zap types fail closed, and a namespace stops every later
field rather than flattening nested values. Object/array/Stringer marshalers
are never invoked. Default error formatting catches panics without exposing
panic values; applications should still provide promptly returning `Error()`
methods.

## Explicit-filter compatibility

`WithAlertFieldAllowlist("request_id", "error_code")` retains the legacy exact-name
filter. It replaces default forwarding and exports only the selected primitive
scalar fields. Targeted credential and payload redaction also applies to
selected values, entry messages and logger names. Errors and complex types remain
excluded even if their names are listed. An explicitly empty
`WithAlertFieldAllowlist()` preserves metadata-only alerts. Multiple allowlist
options replace each other rather than combining their keys. This explicit
mode preserves field and type selection; intentionally selected credential
or payload fields retain their names with a redacted marker.

Callers migrating from the previous metadata-only default may select the empty
filter to retain that behavior. Callers wanting ordinary diagnostic forwarding
should omit the allowlist option and any adapter that discards fields before
they reach the SDK. Direct `Send` / `SendWithType` remains an explicit free-form
message API and does not infer or redact arbitrary application text. Rolling
back the SDK restores the older default observability gap; explicit-filter
behavior and alert wire shape remain compatible.

## Delivery limits and lifecycle

The final queued message is capped at 16 KiB by default. Set a positive cap no
larger than 1 MiB with `WithAlertMaxMessageBytes`. Oversized or encoding-failed
messages return an error, never enqueue a partial message, and never fall back
to raw field formatting. Debug diagnostics omit message text, tokens, alert
labels, and raw transport/GraphQL errors. Endpoint diagnostics use only scheme
and authority. The existing 20-entry nonblocking queue and rate limiter remain.

`Close` is idempotent, cancels in-flight HTTP requests, and abandons pending work;
it does not flush. Admission and shutdown are serialized: after `Close` returns
or `Done` closes, no send can enter an orphaned queue. Pending payload/token
references are released before `Done` signals the actual end of the worker.
The HTTP deadline uses the positive configured `WithAlertPushTimeout`, preserving
earlier context deadlines. Custom rate limiters must return promptly.
