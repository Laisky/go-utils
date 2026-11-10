# Memory archive integrity

Memory maintenance now reads each written archive back and checks that its
decoded compressed frame is byte-for-byte equal to the frame made from the raw
shard. A write acknowledgement alone never authorizes deleting a nonempty raw
shard. A failed read, malformed envelope or different frame returns an error and
retains the raw shard. This detects truncation and corruption across the actual
storage transport without unbounded decompression of returned data.

## Existing binary archives

The zero value of `Config.ArchiveFormat` is `ArchiveFormatZstd`. It preserves the
historical `.jsonl.zst` path and standard binary zstd bytes. Local storage remains
compatible with existing readers, rolling upgrades and rollback. Existing files
are not migrated or reinterpreted. `DecodeArchive(ArchiveFormatZstd, content)`
returns those same bytes for a standard zstd decoder.

The MCP FileIO contract in `docs/ref/mcp_fileio.md` is **UTF-8 text only**. A raw
zstd frame is not UTF-8. The old path converted it to a string, then JSON encoding
replaced invalid bytes. Consequently default-format maintenance over MCP now
fails readback verification and retains the raw shard instead of silently
deleting its recoverable source. A custom byte-preserving local backend can keep
using the default format. Do not pretend the FileIO service accepts a binary or
base64 content encoding that its contract does not support.

## Explicit portable format

To archive through MCP, explicitly set:

```go
engine, err := memory.NewEngine(storage, memory.Config{
    ArchiveFormat: memory.ArchiveFormatZstdBase64V1,
})
```

The new representation uses a separate `.jsonl.zst.b64.json` name. Its UTF-8 JSON
object contains `format: "zstd-base64-v1"` and `data`, a standard padded base64
encoding of the unchanged zstd frame. Storage's text Read/Write contracts remain
unchanged. `DecodeArchive(ArchiveFormatZstdBase64V1, content)` validates that
format and returns the compressed frame. It does not decompress or authenticate
data; restoration tools must bound decompression and validate the recovered
JSONL. There is no automatic fallback to the legacy decoder on malformed input.
The envelope adds base64 and JSON size overhead and remains subject to backend
request/response quotas. A quota or read-size error retains the raw shard.

## Migration, rolling upgrades and rollback

1. Update restoration tools to recognize both distinct filenames and select the
   matching explicit decoder. Keep immutable backups of existing raw/archive
   files. Deploy those readers before enabling the new writer configuration.
2. Stop older maintenance workers for the affected sessions before opting into
   the portable format. Older binaries cannot decode its envelope; they must not
   be allowed to run the known-unsafe binary MCP archival path.
3. Enable the format and verify end-to-end restoration of a test shard. Existing
   binary archives stay where they are; portable archives do not overwrite them.
4. Rollback can stop writing portable archives, but must retain a compatible
   restoration tool and the portable files. To return to an old binary-only
   reader, explicitly decode and validate portable archives into standard zstd
   files using a binary-safe destination first. Never rename base64 JSON to `.zst`.
   Do not resume an older MCP maintenance implementation that can delete raw data
   after corrupting its archive.

## Existing damage and concurrency limits

Already-corrupted MCP archives cannot generally be reconstructed: replacement
characters have destroyed byte information. Retain those files for diagnosis,
check other raw/legacy logs or backups, and restore from a verified source. No
automatic repair or claim of recovered production history is made here.

Readback detects a corrupted stored frame; it is not a transaction or a guarantee
against a backend lying consistently or corrupting data later. Maintenance must
not race writers to the same raw shard. The separate AfterTurn/maintenance
transaction problem in #145 is not solved by this check, and exclusive session
ownership remains necessary until a storage-backed commit protocol exists.

## Regression evidence

- The original implementation fails `TestArchiveLegacyMCPRetainsRaw`: the real
  MCPClient/MCPStorage JSON transport returns success after damaging binary
  bytes and removes the raw shard. The fixed implementation errors and retains it.
- A portable archive crosses the same real JSON transport, decompresses exactly,
  and deletes the raw source only after verification. Corrupt readback keeps it.
- Read errors retain raw data. Unknown formats and malformed envelopes fail closed.
- Real local storage retains frozen historical zstd bytes and emits new standard
  binary zstd that the historical reading path can decode.

The bounded transport fixtures use synthetic data and no production endpoint.
