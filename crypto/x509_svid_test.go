package crypto

import (
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/spiffe/go-spiffe/v2/bundle/x509bundle"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"github.com/stretchr/testify/require"
)

const svidTestID = "spiffe://example.org/workload"

// svidTestBundle returns a bundle explicitly associated with td and its roots.
func svidTestBundle(td string, roots ...*x509.Certificate) *x509bundle.Set {
	return x509bundle.NewSet(x509bundle.FromX509Authorities(spiffeid.RequireTrustDomainFromString(td), roots))
}

// svidTestParse parses a certificate and fails t on errors.
func svidTestParse(t *testing.T, der []byte) *x509.Certificate {
	t.Helper()
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert
}

// svidTestTemplate returns an empty-subject workload leaf template.
func svidTestTemplate(t *testing.T) *x509.Certificate {
	t.Helper()
	id, err := url.Parse(svidTestID)
	require.NoError(t, err)
	return &x509.Certificate{SerialNumber: big.NewInt(42), NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(time.Hour), URIs: []*url.URL{id},
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}}
}

// svidTestSign signs tpl with issuerKey for leafKey and parses the resulting DER.
func svidTestSign(t *testing.T, tpl, issuer *x509.Certificate, leafKey, issuerKey crypto.Signer) *x509.Certificate {
	t.Helper()
	der, err := x509.CreateCertificate(rand.Reader, tpl, issuer, leafKey.Public(), issuerKey)
	require.NoError(t, err)
	return svidTestParse(t, der)
}

// TestX509SVIDIdentityGrammar checks canonical grammar and the required workload path.
func TestX509SVIDIdentityGrammar(t *testing.T) {
	for _, value := range []string{"", "https://example.org/workload", "spiffe://example.org", "spiffe://example.org/",
		"spiffe://EXAMPLE.org/workload", "spiffe://user@example.org/workload", "spiffe://example.org:443/workload",
		"spiffe://example.org/a/../b", "spiffe://example.org/a//b", "spiffe://example.org/a%2Fb",
		"spiffe://example.org/a?query", "spiffe://example.org/a#fragment", "spiffe://example.org/./a"} {
		t.Run(value, func(t *testing.T) { _, err := ParseX509SVIDID(value); require.Error(t, err) })
	}
	id2048 := "spiffe://example.org/" + strings.Repeat("a", 2048-len("spiffe://example.org/"))
	id, err := ParseX509SVIDID(id2048)
	require.NoError(t, err)
	require.Len(t, id.String(), 2048)
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	csr, err := NewX509SVIDCSR(key, id2048)
	require.NoError(t, err)
	parsed, err := x509.ParseCertificateRequest(csr)
	require.NoError(t, err)
	require.Equal(t, id2048, parsed.URIs[0].String())
}

// TestX509SVIDIssuance exercises empty/populated subjects, every supported key family, and interoperability.
func TestX509SVIDIssuance(t *testing.T) {
	for _, algo := range []x509.SignatureAlgorithm{x509.SHA256WithRSA, x509.ECDSAWithSHA256, x509.PureEd25519} {
		t.Run(algo.String(), func(t *testing.T) {
			key := newCSRSignatureTestKey(t, algo)
			ca := newCSRSignatureTestCA(t, key)
			bundles := svidTestBundle("example.org", ca)
			for _, cn := range []string{"", "optional display name"} {
				opts := []X509CertOption{WithX509CertParent(ca), WithX509CertCommonName(cn), WithX509CertDNSNames("service.example.org")}
				der, err := NewX509SVID(key, svidTestID, opts...)
				require.NoError(t, err)
				cert := svidTestParse(t, der)
				require.Equal(t, cn, cert.Subject.CommonName)
				require.Equal(t, []string{"service.example.org"}, cert.DNSNames)
				id, err := ValidateX509SVID(cert)
				require.NoError(t, err)
				require.Equal(t, svidTestID, id.String())
				require.NoError(t, cert.CheckSignatureFrom(ca))
				_, err = VerifyX509SVIDForID([]*x509.Certificate{cert}, bundles, svidTestID)
				require.NoError(t, err)
				_, _, err = x509svid.Verify([]*x509.Certificate{cert}, bundles)
				require.NoError(t, err, "go-utils writer/go-spiffe reader")
				pemKey, err := Prikey2Pem(key)
				require.NoError(t, err)
				decoded, err := Pem2Prikey(pemKey)
				require.NoError(t, err)
				require.Equal(t, key, decoded)
			}
			csrDER, err := NewX509SVIDCSR(key, svidTestID)
			require.NoError(t, err)
			der, err := NewX509SVIDByCSR(ca, key, csrDER, svidTestID)
			require.NoError(t, err)
			_, err = VerifyX509SVIDForID([]*x509.Certificate{svidTestParse(t, der)}, bundles, svidTestID)
			require.NoError(t, err)
			// An existing stdlib writer is accepted by the new profile and verifier.
			legacy := svidTestSign(t, svidTestTemplate(t), ca, key, key)
			_, _, err = VerifyX509SVID([]*x509.Certificate{legacy}, bundles)
			require.NoError(t, err, "existing writer/new reader")
		})
	}
}

// TestX509SVIDProfile validates actual DER and rejects contradictory raw extensions.
func TestX509SVIDProfile(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	cases := map[string]func(*x509.Certificate){
		"no URI":              func(c *x509.Certificate) { c.URIs = nil },
		"two URI":             func(c *x509.Certificate) { c.URIs = append(c.URIs, c.URIs[0]) },
		"root path":           func(c *x509.Certificate) { c.URIs[0].Path = "" },
		"wrong scheme":        func(c *x509.Certificate) { c.URIs[0].Scheme = "https" },
		"CA":                  func(c *x509.Certificate) { c.IsCA = true },
		"certSign":            func(c *x509.Certificate) { c.KeyUsage |= x509.KeyUsageCertSign },
		"crlSign":             func(c *x509.Certificate) { c.KeyUsage |= x509.KeyUsageCRLSign },
		"no digitalSignature": func(c *x509.Certificate) { c.KeyUsage = x509.KeyUsageKeyEncipherment },
		"noncritical KU": func(c *x509.Certificate) {
			c.ExtraExtensions = []pkix.Extension{{Id: []int{2, 5, 29, 15}, Value: []byte{3, 2, 7, 128}}}
		},
		"noncritical empty-subject SAN": func(c *x509.Certificate) {
			value, err := asn1.Marshal([]asn1.RawValue{{Class: 2, Tag: 6, Bytes: []byte(svidTestID)}})
			require.NoError(t, err)
			c.ExtraExtensions = []pkix.Extension{{Id: []int{2, 5, 29, 17}, Value: value}}
		},
		"one EKU": func(c *x509.Certificate) { c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth} },
		"Any EKU": func(c *x509.Certificate) { c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageAny} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			tpl := svidTestTemplate(t)
			mutate(tpl)
			cert := svidTestSign(t, tpl, ca, key, key)
			_, err := ValidateX509SVID(cert)
			require.Error(t, err)
		})
	}
	tpl := svidTestTemplate(t)
	tpl.ExtKeyUsage = nil
	cert := svidTestSign(t, tpl, ca, key, key)
	_, _, err := VerifyX509SVID([]*x509.Certificate{cert}, svidTestBundle("example.org", ca))
	require.NoError(t, err, "EKU presence is SHOULD, not MUST")
	// Parsed fields are not an authorization channel: DER is the source of truth.
	cert.URIs = nil
	_, err = ValidateX509SVID(cert)
	require.NoError(t, err)
	_, err = ValidateX509SVID(nil)
	require.Error(t, err)
}

// TestX509SVIDFinalOutput checks option-order independence, raw overrides, and CSR authorization.
func TestX509SVIDFinalOutput(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	wrongSAN, err := asn1.Marshal([]asn1.RawValue{{Class: 2, Tag: 6, Bytes: []byte("spiffe://example.org/other")}})
	require.NoError(t, err)
	badExtensions := []pkix.Extension{
		{Id: []int{2, 5, 29, 17}, Critical: true, Value: wrongSAN},
		{Id: []int{2, 5, 29, 37}, Value: []byte{0x30, 0}},
		{Id: []int{2, 5, 29, 19}, Critical: true, Value: []byte{0x30, 3, 1, 1, 255}},
		{Id: []int{2, 5, 29, 15}, Critical: true, Value: []byte{3, 2, 1, 6}},
	}
	for _, ext := range badExtensions {
		for _, reverse := range []bool{false, true} {
			opts := []X509CertOption{WithX509CertExtraExtensions(ext),
				WithX509CertKeyUsage(x509.KeyUsageDigitalSignature),
				WithX509CertExtKeyUsage(x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth)}
			if reverse {
				opts[0], opts[2] = opts[2], opts[0]
			}
			der, err := NewX509SVID(key, svidTestID, append(opts, WithX509CertParent(ca))...)
			require.Error(t, err)
			require.Nil(t, der)
		}
	}
	_, err = NewX509SVID(key, svidTestID, WithX509CertParent(ca), WithX509CertIsCA())
	require.Error(t, err)
	csr, err := NewX509SVIDCSR(key, svidTestID)
	require.NoError(t, err)
	_, err = NewX509SVIDByCSR(ca, key, csr, "spiffe://example.org/other")
	require.ErrorContains(t, err, "not the authorized identity")
	tampered := append([]byte(nil), csr...)
	tampered[len(tampered)-1] ^= 1
	_, err = NewX509SVIDByCSR(ca, key, tampered, svidTestID)
	require.Error(t, err)
	_, err = NewX509SVIDCSR(key, svidTestID, WithX509CSRExtraExtension(badExtensions[0]))
	require.Error(t, err)
	// A CA request in the CSR is never implicitly authorized by possession.
	claim, err := NewX509SVIDCSR(key, svidTestID, WithX509CSRExtraExtension(badExtensions[2]))
	require.NoError(t, err)
	der, err := NewX509SVIDByCSR(ca, key, claim, svidTestID)
	require.NoError(t, err)
	require.False(t, svidTestParse(t, der).IsCA)
	for _, ext := range badExtensions {
		for _, reverse := range []bool{false, true} {
			opts := []SignCSROption{WithX509SignCSRExtraExtenstions(ext),
				WithX509SignCSRKeyUsage(x509.KeyUsageDigitalSignature),
				WithX509SignCSRExtKeyUsage(x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth)}
			if reverse {
				opts[0], opts[2] = opts[2], opts[0]
			}
			der, err = NewX509SVIDByCSR(ca, key, csr, svidTestID, opts...)
			require.Error(t, err)
			require.Nil(t, der)
		}
	}
}
