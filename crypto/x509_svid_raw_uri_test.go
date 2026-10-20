package crypto

import (
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// svidRawURIExtension encodes the original URI bytes without URL normalization.
func svidRawURIExtension(t *testing.T, value string) pkix.Extension {
	t.Helper()
	san, err := asn1.Marshal([]asn1.RawValue{{Class: 2, Tag: 6, Bytes: []byte(value)}})
	require.NoError(t, err)
	return pkix.Extension{Id: []int{2, 5, 29, 17}, Critical: true, Value: san}
}

// TestX509SVIDRawURI rejects original URI bytes that URL parsing would normalize.
func TestX509SVIDRawURI(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	id, err := ParseX509SVIDID(svidTestID)
	require.NoError(t, err)
	for _, rawID := range []string{svidTestID, svidTestID + "#", svidTestID + "#fragment", svidTestID + "?", svidTestID + "?#"} {
		t.Run(rawID, func(t *testing.T) {
			valid := rawID == svidTestID
			_, parseErr := ParseX509SVIDID(rawID)
			require.Equal(t, valid, parseErr == nil, "direct grammar control")
			ext := svidRawURIExtension(t, rawID)
			for _, rawFirst := range []bool{false, true} {
				certOpts := []X509CertOption{WithX509CertURIs(id.URL()), WithX509CertExtraExtensions(ext)}
				csrOpts := []X509CSROption{WithX509CSRURIs(id.URL()), WithX509CSRExtraExtension(ext)}
				if rawFirst {
					certOpts[0], certOpts[1] = certOpts[1], certOpts[0]
					csrOpts[0], csrOpts[1] = csrOpts[1], csrOpts[0]
				}
				certOpts = append(certOpts, WithX509CertParent(ca))
				t.Run(fmt.Sprintf("certificate/rawFirst=%t", rawFirst), func(t *testing.T) {
					der, err := NewX509SVID(key, svidTestID, certOpts...)
					if valid {
						require.NoError(t, err)
						require.NotEmpty(t, der)
					} else {
						require.Error(t, err, "final raw certificate URI must satisfy SPIFFE grammar")
						require.Nil(t, der)
					}
				})
				t.Run(fmt.Sprintf("request/rawFirst=%t", rawFirst), func(t *testing.T) {
					csrDER, err := NewX509SVIDCSR(key, svidTestID, csrOpts...)
					if valid {
						require.NoError(t, err)
						require.NotEmpty(t, csrDER)
					} else {
						require.Error(t, err, "final raw request URI must satisfy SPIFFE grammar")
						require.Nil(t, csrDER)
					}
				})
			}
			// The stdlib can sign these bytes; possession does not make the identity valid.
			csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{ExtraExtensions: []pkix.Extension{ext}}, key)
			require.NoError(t, err)
			csr, err := x509.ParseCertificateRequest(csrDER)
			require.NoError(t, err)
			require.NoError(t, csr.CheckSignature())
			t.Run("CSR issuance", func(t *testing.T) {
				_, err = NewX509SVIDByCSR(ca, key, csrDER, svidTestID)
				require.Equal(t, valid, err == nil, "issuer must validate original requested URI")
			})
			tpl := svidTestTemplate(t)
			tpl.ExtraExtensions = []pkix.Extension{ext}
			cert := svidTestSign(t, tpl, ca, key, key)
			require.NoError(t, cert.CheckSignatureFrom(ca))
			if rawID == svidTestID+"#" {
				require.Equal(t, svidTestID, cert.URIs[0].String(), "stdlib drops the delimiter")
				require.Contains(t, cert.Extensions, ext, "signed DER still contains the delimiter")
			}
			t.Run("profile", func(t *testing.T) {
				_, err = ValidateX509SVID(cert)
				require.Equal(t, valid, err == nil)
			})
			t.Run("trust", func(t *testing.T) {
				_, _, err = VerifyX509SVID([]*x509.Certificate{cert}, svidTestBundle("example.org", ca))
				require.Equal(t, valid, err == nil)
			})
		})
	}
}

// TestX509SVIDRawAuthorityURI checks the original optional signing-CA SPIFFE ID.
func TestX509SVIDRawAuthorityURI(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	for _, value := range []string{"spiffe://example.org", "spiffe://example.org#"} {
		t.Run(value, func(t *testing.T) {
			tpl := svidTestAuthority("root")
			tpl.ExtraExtensions = []pkix.Extension{svidRawURIExtension(t, value)}
			root := svidTestSign(t, tpl, tpl, key, key)
			leaf := svidTestSign(t, svidTestTemplate(t), root, key, key)
			_, _, err := VerifyX509SVID([]*x509.Certificate{leaf}, svidTestBundle("example.org", root))
			require.Equal(t, value == "spiffe://example.org", err == nil)
		})
	}
}

// TestX509SVIDChainPurposePolicy distinguishes SPIFFE identity trust from TLS-role policy.
func TestX509SVIDChainPurposePolicy(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	rootTpl := svidTestAuthority("root")
	root := svidTestSign(t, rootTpl, rootTpl, key, key)
	for _, usage := range []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning, x509.ExtKeyUsageServerAuth} {
		t.Run(fmt.Sprint(usage), func(t *testing.T) {
			interTpl := svidTestAuthority("purpose-constrained intermediate")
			interTpl.ExtKeyUsage = []x509.ExtKeyUsage{usage}
			inter := svidTestSign(t, interTpl, root, key, key)
			leaf := svidTestSign(t, svidTestTemplate(t), inter, key, key)
			_, _, err := VerifyX509SVID([]*x509.Certificate{leaf, inter}, svidTestBundle("example.org", root))
			require.NoError(t, err, "go-spiffe identity trust uses ExtKeyUsageAny")
			roots, intermediates := x509.NewCertPool(), x509.NewCertPool()
			roots.AddCert(root)
			intermediates.AddCert(inter)
			for _, role := range []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth} {
				_, err = leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, KeyUsages: []x509.ExtKeyUsage{role}})
				if usage == x509.ExtKeyUsageServerAuth && role == x509.ExtKeyUsageServerAuth {
					require.NoError(t, err, "matching TLS purpose is a positive control")
				} else {
					require.Error(t, err, "excluded TLS purpose is a negative control")
				}
			}
		})
	}
}
