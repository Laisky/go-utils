# ZIP extraction budgets

`Unzip` now defaults to 64 MiB aggregate decompressed output, 16 MiB per file,
100,000 entries and a 32 KiB copy buffer. This intentionally rejects archives
which the old unlimited byte default accepted. For larger trusted archives, set
both `UnzipWithMaxBytes` and `UnzipWithMaxFileBytes` to explicit finite values.
`UnzipWithUnsafeUnlimitedBytes` is a deliberately named escape hatch for trusted
inputs only. It disables both byte limits, not the entry or copy-buffer limit.
Options apply in order; later finite options restore their respective limits.
Copy buffers must be between one byte and one MiB.

Every member path and advertised size is checked before extraction. Actual
streamed bytes are independently counted; archive headers are not trusted as a
substitute. At most one extra byte is read into memory to detect overflow, and
no byte beyond the output budget is written. A member is written to a private
temporary file, read through EOF to check its CRC, closed, and then renamed to
its destination. Any failed member leaves an existing destination untouched
and its temporary output is removed. Successful overwrites retain the prior
permission clamp (no group/other write, special bits removed).

This is per-file publication, not an archive-wide transaction: earlier completed
members and created directories may remain after a later I/O/checksum failure.
Rename guarantees depend on the operating system. A close/removal/rename failure
is returned, not silently ignored. No durability/fsync guarantee is added.

These budgets constrain output and in-flight copy memory, not elapsed CPU time,
compressed-input size or memory used by the ZIP reader to parse its directory.
Parent-directory symlink/race confinement remains a separate issue (#46).
Applications must use a trusted extraction directory and appropriate outer
input-size, time and isolation controls for hostile archives.
