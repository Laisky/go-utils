# SM4 compatibility and explicit authenticated formats

The original EncryptBySm4Cbc/DecryptBySm4Cbc methods again use
`iv || SM4-CBC(raw key,iv,plaintext) || HMAC-SHA256(raw key,ciphertext)`.
The original `Baisc` methods retain raw CBC and ciphertext-only HMAC; an empty
tag on the old basic decryptor explicitly skips authentication. These APIs are
deprecated, not disabled. Their historical IV-malleability limitation remains.
Separate Legacy helpers alone did not preserve existing callers, so original names
now preserve their wire contract as well. In-process cryptography is retained:
restoring compatibility does not restore secret keys in subprocess arguments.

New protocols explicitly select EncryptBySm4CbcEnvelope/DecryptBySm4CbcEnvelope.
They preserve the previously introduced GUS4 version 1 bytes, key separation,
header/IV/ciphertext authentication and strict no-fallback parsing. The new
BasicAuthenticated pair supports separate ciphertext/IV/tag fields, requires
a nonempty tag, and authenticates the IV with the previously introduced derived MAC key.
Historical Basic tags are not accepted by these explicit authenticated readers.

Upgrade old readers before switching writers; select formats using trusted
protocol metadata. Do not catch authentication failures and retry a legacy reader.
Independently authenticate historical IVs before treating migrated plaintext as
trusted: a new envelope cannot retroactively establish integrity of old data.
No persisted-data rewrite or automatic migration is included. Existing callers
using the original format need no migration just to upgrade the library. Callers
using the short-lived changed format must explicitly select the new methods.

Behavior tests independently construct and decode old framing at empty, block
boundary and multi-block sizes. The same tests fail before this correction and
pass after it. All IV/tag/header tamper and malformed-padding regression assertions
remain, applied to the explicit new format; raw no-tag controls use the old basic API.
