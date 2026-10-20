# General X.509 SANs and CSR issuer policy

DNS names, IPv4/IPv6 addresses, email mailboxes and ordinary URI SANs are already
supported by the generic certificate, CSR and CSR-signing APIs. Their typed
options preserve those identities; `WithX509CertSANS` / `WithX509CSRSANS` retain
their existing permissive string classification. SPIFFE is an optional URI
identity profile, not a replacement for these name forms.
Legacy CSR signing reconstructs parsed SAN fields; the additive policy APIs
below preserve the approved original SAN encoding without changing that contract.

## Explicit issuance authorization

A CSR signature proves possession and integrity. It does not authorize Subject,
DNS, IP, email, URI or requested extensions. `NewX509CertByCSRWithPolicy` requires
an `X509CSRIdentityPolicy` with two non-nil callbacks:

- `AuthorizeRequest` examines the signed request after signature validation and
  before signing. Reject any unapproved claim.
- `AuthorizeCertificate` examines the actual encoded certificate after all
  issuer options and raw overrides. Request approval does not approve a
  substituted final name.

Both callbacks receive independently parsed snapshots, including `RawSubject`
and `Extensions`. Mutation cannot change the bytes later issued or returned.
Errors return nil DER. Obtain approved identities from authenticated enrollment
policy, not from the untrusted CSR itself. For example:

```go
policy := crypto.X509CSRIdentityPolicy{
    AuthorizeRequest: func(request *x509.CertificateRequest) error {
        // Compare every Subject and SAN claim, including original extension
        // bytes where exact URI spelling matters, with the enrollment record.
        return enrollment.AuthorizeRequest(request)
    },
    AuthorizeCertificate: func(cert *x509.Certificate) error {
        return enrollment.AuthorizeCertificate(cert)
    },
}
der, err := crypto.NewX509CertByCSRWithPolicy(ca, caKey, csrDER, policy)
```

The policy APIs preserve the approved original SAN extension bytes, avoiding
lossy URL re-encoding. Empty-Subject issuance makes the SAN critical. Explicit
issuer SAN overrides retain precedence and require separate final approval;
other CSR-requested extensions are not automatically copied or authorized.

The policy entry points validate the four supported SAN forms and reject
unsupported GeneralName forms rather than silently dropping identities from
authorization. Duplicate extension OIDs, empty/malformed SAN sequences, invalid
IP lengths, relative URI references and mailbox display-name wrappers are
rejected. DNS names use ASCII preferred-name labels (including a single
leftmost wildcard); internationalized names require their ASCII representation.
Mailbox local-part spelling is preserved, including valid quoted forms.
Ordinary URIs can be opaque or hostless (`urn:`, `mailto:`, `file:`), and can
contain queries and fragments. URI normalization is not authorization.

Legacy `NewX509CertByCSR` keeps its existing behavior. The older
`NewX509SVIDByCSR` compatibility helper checks only the independently approved
SPIFFE ID and preserves other identities. Prefer `NewX509SVIDByCSRWithPolicy`:
it additionally requires both callbacks for all Subject/DNS/IP/email claims.
The strict SVID profile permits those other SAN types alongside exactly one
SPIFFE URI; an additional ordinary URI violates that profile. Generic
certificates can carry multiple ordinary or SPIFFE-form URI values without
automatically acquiring SVID semantics.

## CN-less and empty subjects

`WithX509CertAllowEmptySubject` and `WithX509CSRAllowEmptySubject` are explicit
Go-only options. They waive the legacy CN requirement and validate SAN inputs
before encoding, including nil URI and malformed IP checks. A populated DN
without CN is allowed. An actually empty leaf Subject requires a nonempty,
critical SAN in the issued certificate. An empty request DN requires SAN
claims; request criticality does not authorize issuer policy. CA/CRL signing
certificates require a populated Subject. Tongsuo rejects the new options;
its existing behavior and feature freeze remain unchanged.

These options and policy APIs opt into validation. Existing generic callers
retain their former encoding, raw-extension precedence and CN rules. Raw
extensions remain authoritative over typed options; new paths validate their
effective final contents. Standard certificate/CSR encodings remain unchanged.

## Verification and identity matching

Use `Certificate.Verify` with explicit roots, intermediates, time and usages for
chain validation. Set `VerifyOptions.DNSName` or call `VerifyHostname` for DNS/IP
matching. Trust alone does not mean the peer has the expected name.

`VerifyX509CertURI` and `VerifyX509CertEmail` verify original certificate DER with
the caller's `x509.VerifyOptions`, then require an exact authorized SAN match.
They do not use `VerifyHostname`. URI comparisons preserve original SAN bytes,
including an empty fragment delimiter; email comparisons preserve case and
mailbox spelling. This conservative exact policy does not normalize equivalent
URI/mailbox spellings. Select application-appropriate `KeyUsages` (for example,
`EmailProtection` for mail). Roots follow ordinary Go verification semantics,
including system roots when `Roots` is nil; configure explicit roots where
required. These helpers authorize only the identity being matched.

For a strict SPIFFE identity use `VerifyX509SVIDForID`, which checks the profile,
domain-selected bundle and expected SPIFFE ID. Independently verify DNS/IP or
other names if the application uses them. Neither generic CSR policy issuance
nor profile validation alone verifies the complete certificate chain.

## Tests and compatibility

Permanent tests cover mixed typed/string SANs through self/direct/CSR issuance,
signed request and final policy decisions for every field, raw overrides in both
orders, duplicate/unsupported extensions, malformed input, tampered signatures,
callback mutation isolation, empty-Subject criticality, DNS/IP/URI/email identity
positives and trusted-but-wrong-identity negatives. Path-length tests compare
omitted/`-1`/zero/positive DER values on direct and CSR issuance, and check exact
`TooManyIntermediates` failures at `N+1` with `N` acceptance controls.

Rollback can return to the previous generic API while retaining standard DER.
Do not remove issuer authorization checks as a rollback shortcut. No workload
attestation service, credential delivery service or new Tongsuo feature is added.

References: [RFC5280 SANs](https://www.rfc-editor.org/rfc/rfc5280#section-4.2.1.6),
[RFC3986 URIs](https://www.rfc-editor.org/rfc/rfc3986),
[RFC5321 mailbox grammar](https://www.rfc-editor.org/rfc/rfc5321#section-4.1.2),
[X.509-SVID](https://github.com/spiffe/spiffe/blob/main/standards/X509-SVID.md).
