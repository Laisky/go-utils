# Log pusher delivery and diagnostics

The default pusher has one worker, a 128-entry queue and a 64 KiB per-entry
limit. Enqueue never waits for a slow sender. A full queue returns
`ErrPusherQueueFull` and increments `Stats().Dropped`; oversized entries and
post-cancellation entries are rejected too. A zero-length queue is supported,
but accepts only when its receiver is already ready. The default is best effort,
not a durable logging or billing channel.

Every send receives a 30-second deadline unless the caller already has an earlier
one. `WithPusherSendTimeout` sets a different positive worker deadline. Direct
`PusherHTTPSender.Send` calls also impose the 30-second ceiling, even when the
supplied HTTP client has no timeout. The constructor copies client configuration
and headers and supplies a default client when nil is passed.

Custom senders must honor context cancellation. Custom formatters and filters
must return promptly. The library does not spawn a disposable goroutine for each
blocked callback or pretend it can forcibly stop arbitrary callback code.
`Close` is idempotent: it cancels the worker and abandons queued entries, without
flushing. `Done` signals actual worker termination. Keep durable delivery in a
separate acknowledged spool when it is required.

`Stats` exposes enqueued, delivered, failed and rejected entries without logging
back through a potentially recursive hook. Enqueued includes pending/in-flight
entries; shutdown-abandoned entries are not counted as enqueue rejections.

HTTP endpoint diagnostics show only scheme and authority. Userinfo, path, query,
fragment and malformed input are excluded. The private cause remains available
through explicit `errors.Is`/`errors.As` classification, but is not part of a
printable `Unwrap` chain. Do not log an original error extracted with `errors.As`.
HTTP response payloads and user-provided message bodies are separate data and
must not contain credentials intended to remain secret from their recipients.
