# SPIFFE X.509 workload identities

## Why use this

Service addresses change as workloads move between hosts, containers and clouds.
SPIFFE gives a workload a service identity such as
`spiffe://example.org/payments`, independent of its current IP address. An
X.509-SVID binds that identity to a public key so services can authenticate each
other with certificates and explicitly authorize the expected peer.

The Go APIs here issue and verify that certificate profile. Workload attestation,
credential delivery and rotation belong to the surrounding identity system.

## Issue and verify an SVID

Save this complete program as `main.go` in a directory below the repository root
and run `GOTOOLCHAIN=local go run .` with Go 1.26 or newer.

The fixed ID represents a decision already made by trusted enrollment policy.
The CA signs the workload's **public key**; its private key remains separate.

```go
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"fmt"
	"time"

	gcrypto "github.com/Laisky/go-utils/v6/crypto"
	"github.com/spiffe/go-spiffe/v2/bundle/x509bundle"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
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

// main demonstrates profile validation, domain-selected trust and peer authorization.
func main() {
	const id = "spiffe://example.org/payments"
	now := time.Now().UTC()
	caKey, workloadKey := newKey(), newKey()
	ca := must(x509.ParseCertificate(must(gcrypto.NewX509Cert(caKey,
		gcrypto.WithX509CertCommonName("Example CA"), gcrypto.WithX509CertIsCA(),
		gcrypto.WithX509CertNotBefore(now.Add(-time.Minute)),
		gcrypto.WithX509CertNotAfter(now.Add(24*time.Hour))))))
	leaf := must(x509.ParseCertificate(must(gcrypto.NewX509SVID(caKey, id,
		gcrypto.WithX509CertParent(ca), gcrypto.WithX509CertPubkey(workloadKey.Public()),
		gcrypto.WithX509CertNotBefore(now.Add(-time.Minute)),
		gcrypto.WithX509CertNotAfter(now.Add(time.Hour))))))
	chain := []*x509.Certificate{leaf}
	bundleFor := func(domain string) *x509bundle.Set {
		return x509bundle.NewSet(x509bundle.FromX509Authorities(
			spiffeid.RequireTrustDomainFromString(domain), []*x509.Certificate{ca}))
	}
	bundles := bundleFor("example.org")
	_, err := gcrypto.ValidateX509SVID(leaf)
	check("profile valid", err == nil)
	_, err = gcrypto.VerifyX509SVIDForID(chain, bundles, id)
	check("trusted expected workload", err == nil)
	_, err = gcrypto.VerifyX509SVIDForID(chain, bundles, "spiffe://example.org/other")
	check("wrong identity rejected", err != nil)
	_, err = gcrypto.VerifyX509SVIDForID(chain, bundleFor("other.org"), id)
	check("missing trust-domain bundle rejected", err != nil)
	_, err = gcrypto.ParseX509SVIDID("spiffe://example.org/payments?role=admin")
	check("invalid SPIFFE syntax rejected", err != nil)
}
```

All five checks should print `ok`. `ValidateX509SVID` checks the encoded profile;
it does not establish trust. `VerifyX509SVID` also authenticates the chain and
returns its identity. `VerifyX509SVIDForID` additionally requires the expected
workload ID. The wrong-ID case tests authorization; the other-domain bundle case
tests trust-domain selection, even though both bundles contain the same CA.
SVID verification uses the leaf's domain bundle and never falls back to OS roots.

For CSR enrollment, use `NewX509SVIDCSR` and
`NewX509SVIDByCSRWithPolicy`, obtaining the authorized ID independently of the CSR
and approving every Subject/SAN claim as in the [issuer-policy example](x509-san-policy.md).
The deprecated `NewX509SVIDByCSR` checks the SPIFFE ID without authorizing other
claims. A strict SVID has one workload SPIFFE URI; DNS/IP/email SANs may accompany
it, but an additional ordinary URI may not.

For mTLS, call the complete chain/profile/expected-ID verifier from
`tls.Config.VerifyConnection` on both peers and require the peer certificate.
If setting `InsecureSkipVerify` for URI identities, that callback must perform
the full verification. SVID chain verification follows go-spiffe's
`ExtKeyUsageAny` policy; applications requiring TLS-role restrictions on signing
CAs must additionally use standard verification with the same domain roots and
the required `KeyUsages`. Named self-signed leaf construction can pass profile
validation but cannot satisfy SVID signing-authority-chain verification.

## Specification index

- [SPIFFE ID](https://github.com/spiffe/spiffe/blob/main/standards/SPIFFE-ID.md): identity syntax and trust domains.
- [X.509-SVID](https://github.com/spiffe/spiffe/blob/main/standards/X509-SVID.md): leaf/signing profiles and certificate validation.
- [Trust Domain and Bundle](https://github.com/spiffe/spiffe/blob/main/standards/SPIFFE_Trust_Domain_and_Bundle.md): domain-associated trust material.
- [RFC 5280](https://www.rfc-editor.org/rfc/rfc5280): X.509 certificate and path-validation rules.
