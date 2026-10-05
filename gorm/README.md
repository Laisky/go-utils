# GORM diagnostic logging

`NewLogger(formatter, logger)` records safe operation metadata by default: an
allowlisted operation name, duration in milliseconds, and a numeric affected-row
count when supplied. It does not record formatted SQL, bound parameters, raw
errors, caller paths, arbitrary event types, or extra formatter arguments.
SELECT events remain at Debug, recognized errors at Error, and other events at
Info. These are diagnostic logs, not a complete database audit or authorization
system. A formatter is application code and remains responsible for its own side
effects. No change is made to SQL execution or parameter binding.

The old `/*disable_log*/` text marker has no control effect, including when it
appears in a SQL comment, parameter, or error. Configure the downstream logger's
level and destinations through trusted application configuration instead. Do not
embed logging-control instructions in query data.

Applications intentionally needing verbose diagnostics can construct:

```go
logger := NewLogger(formatter, sink, WithUnsafeSQLLogging())
```

This explicitly permits **sensitive SQL text at Info/Debug/Error levels**. Only
string/byte formatter messages are emitted and each is capped at 16 KiB. Raw
argument/extra fields remain omitted. Limit use to trusted inputs and restricted,
short-lived diagnostic sinks. Debug level alone is not a redaction boundary.
The old two-argument constructor remains source compatible; its safer default
output is an intentional behavioral change.
