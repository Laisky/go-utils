# X.509-SVIDs with the Go backend

The SVID APIs are opt-in. Existing certificate and CSR functions keep their
common-name requirement and extension behavior. Tongsuo has no new SVID API.

`NewX509SVID` creates a leaf certificate for an identity that the caller has
already authorized. Pass the issuer with `WithX509CertParent` and the workload's
public key with `WithX509CertPubkey` when the signing key belongs to a CA.
`NewX509SVIDCSR` creates a signed identity claim, including an empty Subject when
desired. Prefer `NewX509SVIDByCSRWithPolicy`: it requires an independently
authorized SPIFFE ID and two explicit issuer checks for the signed request and
actual final certificate. Each check must independently authorize every included
Subject, DNS, IP and email claim. The compatibility helper
`NewX509SVIDByCSR` checks only the SPIFFE ID and preserves other claims, so it is
deprecated in favor of that policy API. A CSR signature proves possession, not
enrollment authorization; requested raw extensions do not become issuer policy.

General DNS, IPv4/IPv6, email and ordinary URI SANs remain supported independently
of SPIFFE. See [general SAN and issuer policy](x509-san-policy.md) for mixed
issuance, CN-less subjects and DNS/IP/URI/email verification. A strict SVID may
include DNS/IP/email alongside its one SPIFFE URI; a second ordinary URI is
not permitted by the profile.

The builders validate original URI bytes in the final encoded certificate or
request after all options and raw overrides. URL parsing can drop an empty `#`
fragment delimiter, so checking only a parsed URL string is insufficient.
A leaf has exactly one canonical `spiffe` URI with a non-root
path, `CA=false`, critical digital-signature key usage, and no certificate/CRL
signing usage. Empty Subject requires a critical SAN extension. Other SAN types
are permitted. Builders include both TLS EKUs by default. Verification permits
an absent EKU (SPIFFE says SHOULD); an existing EKU must include clientAuth and
serverAuth. Conflicting raw overrides return an error and no DER.

`ValidateX509SVID` checks the profile in the certificate's DER only. It does not
authenticate the certificate. `VerifyX509SVID` additionally checks the real
X.509 chain and returns the authenticated ID. It uses a go-spiffe `x509bundle.Source`
to select the bundle associated with the leaf's trust domain, with no OS-root or
global-bundle fallback. `VerifyX509SVIDForID` also requires an exact authorized ID.
Applications may instead apply their own authorization policy to the returned ID.
Bundle rotation and federation are explicit changes to the domain-associated
bundle source. Signing CAs need not carry a SPIFFE URI; when they do, its path
must be empty, and its domain may differ from the leaf's domain.

```go
bundles := x509bundle.NewSet(x509bundle.FromX509Authorities(
    spiffeid.RequireTrustDomainFromString("example.org"), roots))
chains, err := crypto.VerifyX509SVIDForID(peerCertificates, bundles,
    "spiffe://example.org/service")
```

For TLS, run this verification from `tls.Config.VerifyConnection` on both sides
and require the peer's certificate. SPIFFE URI authorization replaces DNS-name
verification, so a client using `InsecureSkipVerify` must always install the full
chain, profile, and identity verification callback. The SPIFFE identity verifier
follows go-spiffe's `ExtKeyUsageAny` chain policy: it enforces the leaf profile,
but does not enforce TLS-role EKU restrictions on signing CAs. If the application's
policy requires those restrictions, additionally call `Certificate.Verify` with
the same trust-domain roots and intermediates, `KeyUsages` set to the required
clientAuth or serverAuth purpose, and no DNS name for URI-based authentication.
Do not use OS roots or pool unrelated trust domains for that additional check.
The mutual TLS regression
test demonstrates successful authentication and distinct wrong-identity and
wrong-domain failures without a network listener.

Existing PEM/PKCS8 and standard X.509 DER remain interoperable. Consumers can
roll back by returning to their previous generic APIs; no stored-format migration
is needed. Generic certificate APIs do not apply SVID policy automatically.
These APIs do not supply workload attestation, a Workload API daemon, JWT-SVIDs,
revocation infrastructure, or automatic credential rotation.

References: [X.509-SVID standard](https://github.com/spiffe/spiffe/blob/main/standards/X509-SVID.md),
[SPIFFE ID standard](https://github.com/spiffe/spiffe/blob/main/standards/SPIFFE-ID.md).
