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
Extraction anchors the caller-selected destination with `os.Root`. All member
paths are canonical, local and have no trailing separator before rooted calls.
Parent links that resolve outside the root are rejected. A contained relative
link is allowed; leaf links/hardlinks are replaced rather than written through.
Each file's parent is pinned during temporary creation, publication and cleanup.
This addresses the ZIP sink in #46, not that issue's other filesystem/CLI/log sinks.

The caller still selects a trusted root. Mount points and moving an already
opened directory out of that root require external isolation; an open directory
handle continues to refer to its original directory. JavaScript targets fail
closed because their filesystem API cannot provide race-resistant rooting.
Use a supported, security-patched Go toolchain. In particular GO-2026-4970 affects
older Go `os.Root` calls with trailing-slash paths (fixed in Go 1.25.12/1.26.5).
This extractor strips those separators and tests that boundary even on 1.25.7;
that compatibility test is not a claim that the old toolchain is fully patched.
Applications must also impose outer input-size, time and isolation controls.
