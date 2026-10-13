# RSA-OAEP compatibility and whole-message integrity (#112, #123)

## API contracts

Existing public functions keep their historical behavior. A library upgrade must
not change their ciphertext format or make stored ciphertext unreadable.

| API pair | Format and empty-input behavior | Intended use |
| --- | --- | --- |
| `RSAEncryptByOAEP` / `RSADecryptByOAEP` | Concatenated RSA-OAEP/SHA-256 blocks, nil OAEP label; empty input returns `nil, nil` | Deprecated; retained for existing callers and externally authenticated legacy protocols |
| `RSAEncryptByOAEPEnvelope` / `RSADecryptByOAEPEnvelope` | Versioned RSA-OAEP/AES-256-GCM envelope; empty plaintext is authenticated, empty ciphertext is rejected | Explicit opt-in for new protocols and intentional migrations |

The original key validation, block boundaries and `decrypt chunk` error context
are preserved by the old functions. Deprecation is a warning, not removal or a
runtime refusal. Old writers interoperate with old readers in both directions.
Neither API pair automatically detects or falls back to the other format.

**Correction to #123:** that PR changed the old function names to use the envelope
format. Keeping their signatures did not preserve compatibility. The repair
restores their original implementations and original interoperability assertions;
the authenticated implementation is retained under the new explicit names.
Envelopes already written by #123 remain readable with
`RSADecryptByOAEPEnvelope`, with no change to the envelope bytes or version.
Callers that adopted #123's envelope behavior must select the new names explicitly.

## Security boundary

Independent legacy OAEP blocks do not authenticate message order or completeness.
Whole blocks can be removed, repeated or reordered. This limitation cannot be
removed from that format while preserving interoperability with old readers and
ciphertexts. The deprecated APIs therefore **do not** claim whole-message integrity;
applications must authenticate the complete legacy ciphertext externally before
accepting it across a trust boundary. The secure remediation is opt-in migration,
not silently reinterpreting an existing function.

The envelope uses a fresh random 32-byte AES key with AES-256-GCM for the payload;
RSA-OAEP/SHA-256 wraps the key. The header, wrapped key and nonce are authenticated
as associated data. Malformed or unauthenticated envelopes return no plaintext
and match `ErrInvalidRSAEnvelope` with `errors.Is`. The envelope reader never
falls back to legacy blocks, including single blocks. Tampering, truncation,
appended data, wrong keys and unknown versions remain rejected.

Public-key encryption does not establish sender identity or prevent replay.
Anyone holding the public key can create a fresh valid envelope; applications
must enforce sender authentication and freshness separately. The existing
1024-bit compatibility boundary remains, not a recommendation for new keys.

## Version 1 encoding

All integer lengths are unsigned big-endian. The exact format is:

| Field | Length |
| --- | --- |
| ASCII magic `GURSAENV` | 8 bytes |
| Version `0x01` | 1 byte |
| Wrapped-key length | 2 bytes, must equal the RSA modulus length |
| RSA-OAEP/SHA-256 wrapped AES key | RSA modulus length |
| Random GCM nonce | 12 bytes |
| GCM ciphertext and tag | plaintext length + 16 bytes |

The OAEP label is the UTF-8 string `go-utils RSA-OAEP AES-256-GCM envelope v1`.
Associated data is every byte preceding the GCM ciphertext, including the magic,
version, wrapped-key length, wrapped key and nonce. Version 1 fixes the algorithms;
unknown versions are rejected. Sizes are checked before slicing or allocation.

## Explicit migration and rollback

Existing legacy callers need no data migration merely to upgrade this library.
For new whole-message integrity guarantees, inventory writers, readers and stored
values, then explicitly adopt both `RSAEncryptByOAEPEnvelope` and
`RSADecryptByOAEPEnvelope`. This is not a drop-in rename for a mixed-version protocol.
Choose the format from trusted protocol/schema metadata, not a decryption error.
Never catch an envelope authentication failure and retry with an old decoder.

`RSADecryptByOAEPLegacy`, introduced by #123, remains available as a deprecated
strict offline migration helper. Its additional rejection of empty input and
partial blocks is unchanged; it does not replace the historical public function.
Before migration, independently authenticate the **complete original ciphertext**
using a trusted MAC, signature or equivalent integrity record. Legacy OAEP cannot
prove retrospectively that no blocks were removed or reordered. An envelope
cannot retroactively supply missing integrity evidence for historical plaintext.

Applications that wrote envelopes during the #123 window should use the explicit
envelope APIs for that data. Keep the required format-aware reader when rolling
back application code after new envelopes are stored. No automatic fallback,
stored-data rewrite or deployment is performed by this compatibility repair.

## Behavior-first regression evidence

Frozen fixtures include ciphertext generated by the actual pre-change module at
`042437b6015f5ba8f185ef91b05b2bd3a5a420f3` and envelopes generated from the exact
#123 merged source tree at `a389e7ad3e9103250acf5f5d5f7ec543a9522032`. Their
private key is disposable public test material and must never protect real data.

The restored original OAEP assertions and added historical-reader/writer/error
tests pass on the old implementation, fail on #123, and pass after API separation.
Tests independently decode with standard-library primitives and cover empty,
single-block, boundary and multi-block payloads, wrong keys, malformed blocks and
no partial plaintext on errors. They do not substitute a new-to-new round trip
for compatibility verification.

The envelope's byte-mutation, every-truncation, whole-block structural tamper,
wrong-key, fresh-nonce and downgrade-rejection tests now call the explicit APIs.
Frozen #123 envelopes prove that renaming does not discard the newly introduced
format. Existing key-validation tests remain on the deprecated functions, with
additional invalid/minimum-key coverage on the new functions.

Run `go test -race -count=1 -run 'TestRSA|TestSecurity68|TestRsaEncryptByOAEP|TestCVE_RSA_OAEP' ./crypto`.

Primary references: [Go RSA](https://pkg.go.dev/crypto/rsa#EncryptOAEP),
[Go AEAD](https://pkg.go.dev/crypto/cipher#AEAD),
[Go deprecation comments](https://go.dev/doc/comment#deprecations), and
[RFC 3560 key transport](https://www.rfc-editor.org/rfc/rfc3560).
