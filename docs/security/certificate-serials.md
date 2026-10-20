# Certificate serial generation

Certificate creation through `NewX509Cert` and `NewX509CertByCSR` now uses
`GenerateX509CertificateSerialNumber` when no serial or legacy generator is
selected. It returns a uniformly random positive integer smaller than 2^159
from `crypto/rand`. Zero is retried, DER INTEGER content is at most 20 octets,
and an entropy failure returns an error rather than using a timestamp.

Serials remain `*big.Int` in certificate templates and standard certificate DER.
Consumers that narrowed serials to `int64` must retain the full integer or use
an explicitly selected legacy generator while they migrate. Standard Go X.509
parsing already supports these larger serials. Existing small serials remain
readable; returning to an older library does not change already issued DER.

Explicit serial options retain precedence over generators in either option
order. They consume no serial entropy and invoke no generator. Without an
explicit serial, a selected `X509CertSerialNumberGenerator` is called exactly
once. Its existing `SerialNum() int64` interface and both generator options
remain available. `DefaultX509CertSerialNumGenerator.SerialNum` retains its
int64 range, now retries zero, and panics on entropy failure because that
signature cannot return an error. Prefer the new error-returning API for new
code. Explicit serial options copy their inputs and can be reused safely.

This change preserves explicit serial validation policy, including zero being
passed to the underlying Go certificate implementation; it does not introduce
a new explicit-serial rejection rule. CRL numbers remain caller-selected.
Tongsuo root certificates share the template serial correction, but its CSR
issuance still uses the existing `-CAcreateserial` behavior.

RFC 5280 requires positive, issuer-unique certificate serials of at most 20
octets; it does not itself mandate randomness. Public TLS requirements and
private PKI policy are distinct. Random generation provides no mathematical
guarantee of global or issuer uniqueness. Issuers requiring a hard uniqueness
guarantee need durable reservation and collision handling.

Deploy this as a library update after checking consumers for int64 narrowing.
No data migration is needed for standard DER readers. Rollback preserves
existing certificates, but restores the previous default width and entropy
fallback behavior for newly issued certificates.
