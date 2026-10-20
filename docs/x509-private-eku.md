# Private extended key usages

Go certificate issuance supports arbitrary extended key usage OIDs through
`WithX509CertUnknownExtKeyUsage(...asn1.ObjectIdentifier)` and
`WithX509SignCSRUnknownExtKeyUsage(...asn1.ObjectIdentifier)`. These options add
purposes alongside the existing `x509.ExtKeyUsage` enum options:

```go
purpose := asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55555, 1}
der, err := crypto.NewX509Cert(key,
    crypto.WithX509CertCommonName("service.example"),
    crypto.WithX509CertExtKeyUsage(x509.ExtKeyUsageClientAuth),
    crypto.WithX509CertUnknownExtKeyUsage(purpose),
)
```

The OID options copy their input when created and each time applied. No arguments
is a no-op. An explicitly empty or malformed OID returns an error when the option
is applied; none of that option's purposes are added. Go classifies recognized
OIDs into `Certificate.ExtKeyUsage` when parsing the issued DER, even if supplied
through the OID option. Private OIDs appear in `UnknownExtKeyUsage`.

An explicit raw EKU extension passed through `WithX509CertExtraExtensions` or
`WithX509SignCSRExtraExtenstions` retains precedence over generated purposes in
either option order. Existing enum APIs and no-option behavior are unchanged.
Tongsuo rejects these new OID options; its existing options and defaults remain
unchanged.

CSR requests and authorization are separate. A CSR can request a raw EKU through
`WithX509CSRExtraExtension`; parsing retains it in `CertificateRequest.Extensions`.
Go CSR issuance does not automatically authorize or copy these requested EKUs.
The signer must select allowed purposes with the signing options or explicitly
authorize a raw extension. Checking a CSR signature proves possession and request
integrity, not permission to use its claimed identity or purposes.

This feature preserves purposes during issuance; it does not add a custom-purpose
chain verifier or change Go's certificate authorization rules. Applications must
enforce their purpose policy. Certificates retain ordinary X.509 DER encoding and
can be read by older versions through Go's `UnknownExtKeyUsage`. Rolling back the
library removes the convenience options, so callers must remove their use or
retain explicit raw-extension issuance. No persisted-data migration is required.
