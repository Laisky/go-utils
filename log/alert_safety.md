# Remote alert content and explicit policies

The default now preserves application-chosen messages and supported Zap fields.
It does not select a content filter or redact credential-shaped values on the
application's behalf. See [HTTP alert content and admission](../docs/http-alert-content.md)
for the complete representation, migration, size, delivery and receiver contract.

`WithAlertDiagnosticRedaction()` explicitly opts into the former default policy:
sanitized entry messages/logger names and primitive diagnostic fields, sanitized
error text, no complex marshalers, and namespace-stop behavior. Its established
1 MiB diagnostic marker is part of that explicitly selected projection only.
`WithAlertFieldAllowlist(...)` retains the previous explicit exact-name/type filter
and targeted redaction. An empty list selects metadata only; later allowlists
replace earlier ones. The allowlist takes precedence over diagnostic redaction
regardless of option order. `SanitizeDiagnosticText` remains an application-owned
text projection. Tests keep the historical assertions under these explicit modes.

Attach `GetZapCore()` directly using a Zap tee to preserve eager `With` snapshots
and namespace ordering. `GetZapHook()` is retained but deprecated for bound-context
use because it cannot recover earlier encoding/order from the enclosing core.

The complete alert is capped at 16 KiB by default, configurable through 1 MiB.
Whole-entry overflow or encoding failure rejects admission; the faithful default
never substitutes a marker or omits a field to fit. The existing nonblocking
20-message queue, optional rate limiter and asynchronous HTTP sender remain.
Local admission is not recipient delivery. `Close` cancels rather than flushes;
`Done` follows release of queued payload references. Transport diagnostics remain
fixed messages without application payload, alert token or raw remote error.
