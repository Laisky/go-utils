# Remote alert field policy

An alert is a different audience from the local log. By default, the Zap alert
hook sends only UTC time, level, logger name, and the entry's free-form message.
It does not forward structured fields, error strings, caller paths, or stacks.
Do not put secrets directly into the message or logger name.

`WithAlertFieldAllowlist("request_id", "error_code")` opts in to named top-level
string, bool, integer, float, and duration fields. This is an exact-name allowlist,
not a sensitive-key blacklist. Callers must ensure the selected values are safe.
Objects, arrays, errors, reflection, stringers, and all fields after a namespace
remain excluded even when their key is listed. Their marshalers are not invoked.
Use a reviewed scalar summary to expose selected facts about nested data.

Existing automatic field forwarding is intentionally removed. Callers that
need bound/call-site context must explicitly approve safe scalar keys. Direct
`Send` / `SendWithType` is an explicit free-form message API; it does not infer
or redact arbitrary application text.

The final queued message is capped at 16 KiB by default. Set a positive cap no
larger than 1 MiB with `WithAlertMaxMessageBytes`. Oversized or encoding-failed
messages return an error, never enqueue a partial message, and never fall back
to raw field formatting. Debug diagnostics omit message text, tokens, alert
labels, and raw transport/GraphQL errors. Endpoint diagnostics use only scheme
and authority. The existing 20-entry nonblocking queue and rate limiter remain.

`Close` is idempotent, cancels in-flight HTTP requests, and abandons pending work;
it does not flush. `Done` signals the actual end of the worker. The HTTP deadline
uses the positive configured `WithAlertPushTimeout`, preserving earlier context
deadlines. Custom rate limiters must return promptly.
