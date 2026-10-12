# Local storage operation budgets (#110, #115)

`local.Config.Limits` is trusted application configuration, never an untrusted
per-request override. Zero fields select `DefaultLimits`; negative fields fail
construction. Defaults are 8 MiB returned per Read; 1024 search files; 8192
search-discovered entries; 64 MiB scanned; 8 MiB retained matching content; and a
five-second search deadline. The existing 4 MiB per-search-file limit, maximum
50 matches, and maximum traversal depth 32 remain. Exhaustion returns wrapped
`local.ErrBudgetExceeded` and no partial API result. Caller cancellation and
operation deadlines remain distinguishable with `errors.Is`.

Read now opens a rooted regular file, checks its requested range against the
budget before allocating, and reads only that range in at most 32 KiB pieces.
Length -1 means remaining content, but still obeys the byte limit; length 0 returns
empty data. An offset at EOF is valid, beyond EOF fails, and a missing file retains
its empty-result behavior. A tiny range of a large file is permitted: there is no
new file-size cap or storage quota. Concurrent truncation returns the available
prefix. Cancellation is checked between reads; it cannot preempt an OS call.

Search bounds discovery itself using incremental directory reads, with at most
one over-limit discovery probe, rather than allocating an entire unbounded
listing. Nonmatching files consume the same scan budget as matching files.
Symlinks and special files are skipped, and rooted filesystem protections remain.
Returned match content keeps its existing whole-file semantics within the retained
byte budget. This does not impose aggregate limits across separate concurrent
operations; applications still need tenant quotas and admission control.

On baseline `042437b6015f5ba8f185ef91b05b2bd3a5a420f3`, a one-byte range of a sparse
4 MiB file allocated 4,203,384 bytes. A 1025-file nonmatching fixture exceeded the
new default scan count; three independently allowed matching files exceeded the
retained-content budget. Regression tests preserve those bounded reproductions,
plus small configurable-counter, EOF, cancellation and typed-error controls.
Run `go test -race ./agents/memory/storage/local`.

Compatibility: formerly unbounded full reads/searches can now return budget errors.
Use explicit bounded ranges or configure reviewed limits for trusted maintenance
jobs; do not blindly retry exhausted requests. No on-disk migration is required.
Reverting restores old behavior but also restores the exhaustion risk.
