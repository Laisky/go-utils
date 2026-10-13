# SM4 API compatibility

The original `EncryptBySm4Cbc` / `DecryptBySm4Cbc` and misspelled
`EncryptBySm4CbcBaisc` / `DecryptBySm4CbcBaisc` names keep the historical raw-key
CBC and HMAC-SHA256(key, ciphertext) format. All four are deprecated, not disabled.
The IV remains unauthenticated in that legacy format; use an external authenticated
protocol until an explicit migration can be coordinated. The old basic reader's
optional MAC behavior is preserved. No SM4 operation launches a child process.

`EncryptBySm4CbcEnvelope` / `DecryptBySm4CbcEnvelope` retain the versioned GUS4
format, key separation and mandatory authentication of header, IV and ciphertext.
`EncryptBySm4CbcBasicV1` / `DecryptBySm4CbcBasicV1` expose the post-hardening
basic MAC format explicitly. Its optional MAC parameter still means no integrity
when omitted; prefer the combined envelope. Neither new reader falls back to an
old MAC or envelope after authentication failure. Previously written GUS4 values
remain readable with the new envelope names, without rewriting their bytes.

Existing legacy callers need no format migration merely to upgrade. To adopt new
APIs, coordinate writers, readers, persisted format metadata and rollback readers.
Do not infer a trusted format by catching authentication errors. The previous
Legacy-only helpers did not preserve existing callers and are not a substitute
for restoring the original names' contracts.

Tests first reproduce two old-name interoperability failures, then preserve all
new-format IV/header/tag mutation, key-separation, malformed-input and subprocess
secrecy regressions on the explicit APIs. Independent CBC/HMAC construction and
real Tongsuo interoperability exercise old and new cryptographic primitives.
