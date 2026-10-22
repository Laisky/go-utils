# X.509 identities and CSR authorization

## Why use this

A CA may serve many applications and tenants. A valid CSR proves control of a
private key, but it does not entitle that key to a production hostname, mailbox
or service URI. Issuing every name in a signed CSR lets one workload request
another workload's identity.

`NewX509CertByCSRWithPolicy` separates possession from enrollment authorization:
one callback approves the signed request, and another approves the actual issued
Subject and SANs after issuer options. Peer verification then checks both chain
trust and the identity the application expects. These Go APIs are opt-in;
existing generic builders keep their previous behavior.

## Issue and verify a service identity

Save this complete program as `main.go` in a directory below the repository root
and run `GOTOOLCHAIN=local go run .` with Go 1.26 or newer.

The enrollment record in this example permits an empty Subject and exactly
`urn:example:payments`. It is a trusted constant, not data taken from the CSR.
Comparing original SAN DER also rejects extra names and alternate URI spellings.

```go
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"net/url"
	"time"

	"github.com/Laisky/errors/v2"
	gcrypto "github.com/Laisky/go-utils/v6/crypto"
)

// must returns a successful result or stops this example on an unexpected error.
func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

// newKey creates an independent Ed25519 private key for the example.
func newKey() ed25519.PrivateKey {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	return must(key, err)
}

// check stops on an unexpected outcome and reports each successful check.
func check(name string, ok bool) {
	if !ok {
		panic(name)
	}
	fmt.Println(name + ": ok")
}

// main demonstrates enrollment approval, issuance and exact peer authorization.
func main() {
	const approvedURI = "urn:example:payments"
	now := time.Now().UTC()
	caKey, workloadKey := newKey(), newKey()
	ca := must(x509.ParseCertificate(must(gcrypto.NewX509Cert(caKey,
		gcrypto.WithX509CertCommonName("Example CA"), gcrypto.WithX509CertIsCA(),
		gcrypto.WithX509CertNotBefore(now.Add(-time.Minute)),
		gcrypto.WithX509CertNotAfter(now.Add(24*time.Hour))))))
	sanOID := asn1.ObjectIdentifier{2, 5, 29, 17}
	sanFor := func(identity string) []byte {
		return must(asn1.Marshal([]asn1.RawValue{{
			Class: asn1.ClassContextSpecific, Tag: 6, Bytes: []byte(identity),
		}}))
	}
	approvedSAN := sanFor(approvedURI)
	authorize := func(subject []byte, extensions []pkix.Extension) error {
		if bytes.Equal(subject, []byte{0x30, 0}) {
			for _, ext := range extensions {
				if ext.Id.Equal(sanOID) && bytes.Equal(ext.Value, approvedSAN) {
					return nil
				}
			}
		}
		return errors.New("Subject or SANs are outside the enrollment record")
	}
	policy := gcrypto.X509CSRIdentityPolicy{
		AuthorizeRequest: func(csr *x509.CertificateRequest) error {
			return authorize(csr.RawSubject, csr.Extensions)
		},
		AuthorizeCertificate: func(cert *x509.Certificate) error {
			return authorize(cert.RawSubject, cert.Extensions)
		},
	}
	request := func(extra ...gcrypto.X509CSROption) []byte {
		opts := []gcrypto.X509CSROption{gcrypto.WithX509CSRAllowEmptySubject(),
			gcrypto.WithX509CSRURIs(must(url.Parse(approvedURI))),
			gcrypto.WithX509CSRSignatureAlgorithm(x509.PureEd25519)}
		return must(gcrypto.NewX509CSR(workloadKey, append(opts, extra...)...))
	}
	issue := func(csr []byte, extra ...gcrypto.SignCSROption) ([]byte, error) {
		opts := []gcrypto.SignCSROption{
			gcrypto.WithX509SignCSRNotBefore(now.Add(-time.Minute)),
			gcrypto.WithX509SignCSRNotAfter(now.Add(time.Hour)),
			gcrypto.WithX509SignCSRExtKeyUsage(x509.ExtKeyUsageServerAuth)}
		return gcrypto.NewX509CertByCSRWithPolicy(ca, caKey, csr, policy, append(opts, extra...)...)
	}
	csr := request()
	cert := must(x509.ParseCertificate(must(issue(csr))))
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	verify := x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	_, err := gcrypto.VerifyX509CertURI(cert, approvedURI, verify)
	check("trusted expected URI", err == nil)
	_, err = gcrypto.VerifyX509CertURI(cert, "urn:example:other", verify)
	check("wrong peer rejected", err != nil)
	der, err := issue(request(gcrypto.WithX509CSRDNSNames("unapproved.example.org")))
	check("extra requested DNS denied", err != nil && der == nil)
	der, err = issue(csr, gcrypto.WithX509SignCSRExtraExtenstions(pkix.Extension{
		Id: sanOID, Critical: true, Value: sanFor("urn:example:other")}))
	check("issuer substitution denied", err != nil && der == nil)
	der, err = gcrypto.NewX509CSR(workloadKey, gcrypto.WithX509CSRAllowEmptySubject(),
		gcrypto.WithX509CSRURIs(must(url.Parse("https:///service"))))
	check("empty authority rejected", err != nil && der == nil)
}
```

All five checks should print `ok`; an unexpected result panics. The extra-DNS
request has a valid signature but fails enrollment approval. The issuer override
passes request approval and fails final certificate approval, which occurs
**after signing** and returns no DER. The wrong-peer case has trusted chain
material but fails the exact identity comparison.

For DNS/IP peers use `Certificate.Verify` with `VerifyOptions.DNSName`;
alternatively, verify the chain first and then call `VerifyHostname`.
For mail use `VerifyX509CertEmail` with the expected bare mailbox
and suitable `KeyUsages`. URI/mailbox helpers authorize the selected name only;
they do not authorize other identities in the certificate. Supply explicit roots
when system trust is inappropriate. Ordinary authority-free `urn:` and `file:`
URIs are allowed; an explicit `//` authority needs a host. SPIFFE identities add
the profile and trust-domain rules in [X.509-SVID usage](x509-svid.md).

## Specification index

- [RFC 5280 §4.1.2.6](https://www.rfc-editor.org/rfc/rfc5280#section-4.1.2.6): Subject names and empty-Subject certificates.
- [RFC 5280 §4.2.1.6](https://www.rfc-editor.org/rfc/rfc5280#section-4.2.1.6): DNS, IP, email and URI SANs.
- [RFC 3986](https://www.rfc-editor.org/rfc/rfc3986): URI syntax and authority components.
- [RFC 5321 §4.1.2](https://www.rfc-editor.org/rfc/rfc5321#section-4.1.2): mailbox syntax.
