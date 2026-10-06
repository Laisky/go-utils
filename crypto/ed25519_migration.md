# Ed25519 prehash formats

New prehash protocols should use `SignByEd25519ph` and `VerifyByEd25519ph`.
They stream the message into SHA-512 and select RFC 8032 Ed25519ph through
`ed25519.Options{Hash: crypto.SHA512}`. The context is explicitly empty; a
signature made with a nonempty context is rejected. These APIs accept a message
reader, not an already computed digest.

The historical `*ByEd25519WithSHA512` APIs instead sign the digest using **plain
Ed25519**. Their output is unchanged for compatibility, and they are deprecated
aliases for `SignByEd25519LegacySHA512` and `VerifyByEd25519LegacySHA512`. These
legacy functions must not be described as Ed25519ph. Use separate keys when a
legacy protocol coexists with a plain-Ed25519 signing service: a plain signature
on a digest is a valid legacy signature, which is precisely the cross-protocol
risk documented in issue #73.

Choose and authenticate the algorithm/version in the enclosing protocol or
trusted record metadata. During migration, old records select the explicit
legacy verifier and new records select Ed25519ph. Do not try one verifier after
the other fails. No helper does this fallback, and Ed25519ph rejects the legacy
signature even when the same test key and message are used.

Regression tests include the independent RFC 8032 section 7.3 `abc` vector, Go
standard-library interoperability, a frozen legacy signature, wrong keys and
contexts, malformed lengths, reader errors, empty input and fragmented input.
The original standards mismatch was reproduced on unchanged production source
before adding the new API. The old API deliberately remains legacy; the fix is
an explicit, tested migration path, not reinterpretation of existing signatures.

References:
- https://www.rfc-editor.org/rfc/rfc8032.html#section-7.3
- https://pkg.go.dev/crypto/ed25519#PrivateKey.Sign
- https://pkg.go.dev/crypto/ed25519#VerifyWithOptions
