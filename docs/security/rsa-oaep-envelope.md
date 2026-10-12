# RSA-OAEP whole-message integrity (#112)

## Security contract and wire compatibility

`RSAEncryptByOAEP` now writes a versioned hybrid envelope. A fresh random 32-byte
AES key encrypts the entire message with AES-256-GCM, and RSA-OAEP with SHA-256
wraps that key. The default decryptor authenticates the whole message before
returning any plaintext. Empty plaintext has an authenticated representation;
empty ciphertext, block deletion, duplication, permutation, truncation, appended
data, wrong keys and metadata changes are rejected.

**This is an intentional wire-format change, not a drop-in interoperability
change.** Method signatures remain compatible, but old raw/chunked OAEP ciphertext
is rejected, including a single block. There is no automatic fallback. Errors
for malformed or unauthenticated envelopes match `ErrInvalidRSAEnvelope` with
`errors.Is`; callers must not depend on the old `decrypt chunk` error text.

This is public-key encryption, not sender authentication or replay protection.
Anyone with the public key can create a new valid message. Applications needing
authenticated senders or freshness must enforce those properties separately.
The existing 1024-bit compatibility boundary is retained; use an appropriate
strong RSA key for new deployments rather than selecting that minimum.

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

## Migration and rollback

Inventory every writer, reader, stored ciphertext and external raw-OAEP consumer
before rollout. Upgrade readers and writers in a coordinated protocol migration.
Old binaries cannot read newly written envelopes. Do not roll back readers after
new ciphertext has been stored unless the application also restores a compatible
protocol/data version.

`RSADecryptByOAEPLegacy` is a deprecated, explicitly named offline migration aid.
Before calling it, authenticate the **complete original ciphertext** using a
trusted external MAC, signature, or equivalent integrity record. Legacy OAEP
cannot establish that no blocks were previously removed or reordered. Data with
no independent integrity record cannot be retrospectively authenticated.
After authorized migration, write the returned plaintext through the new
`RSAEncryptByOAEP`. Never catch a default decryption error and retry with the
legacy decoder, and never expose the legacy decoder to untrusted requests.
No legacy encryption API was added.

## Reproduction and regression evidence

On original revision `042437b6015f5ba8f185ef91b05b2bd3a5a420f3`, a disposable
2048-bit key and three 190-byte plaintext blocks demonstrated accepted complete
block removal, duplication, permutation and empty input. The intact round trip
and partial-block rejection served as controls. The same structural rejection
test passes after the repair.

Retained tests additionally flip every byte and try every truncated length in
bounded empty/nonempty fixtures, verify default downgrade rejection, exercise
explicit migration, and independently decode the envelope with standard-library
RSA and AES-GCM rather than relying only on a wrapper-to-wrapper round trip.
Existing key-validation and OAEP payload-boundary tests remain, updated to the
new documented format and uniform authentication-error contract.

Run `go test -race -count=1 -run 'TestRSAOAEP|TestSecurity68|TestRsaEncryptByOAEP|TestCVE_RSA_OAEP|TestRSAHelpers' ./crypto`.

Primary references: [Go RSA](https://pkg.go.dev/crypto/rsa#EncryptOAEP),
[Go AEAD](https://pkg.go.dev/crypto/cipher#AEAD), and
[RFC 3560 key transport](https://www.rfc-editor.org/rfc/rfc3560).
