package crypto

import (
	"bytes"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// TestX509CSRIdentityAuthorization reproduces SPIFFE authorization preserving unrelated CSR identities.
func TestX509CSRIdentityAuthorization(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	id := "spiffe://example.org/workload"
	policy := X509CSRIdentityPolicy{
		AuthorizeRequest: func(csr *x509.CertificateRequest) error {
			if !bytes.Equal(csr.RawSubject, []byte{0x30, 0}) || len(csr.DNSNames)+len(csr.IPAddresses)+len(csr.EmailAddresses) != 0 {
				return errors.New("unapproved Subject or non-URI SAN")
			}
			return nil
		},
		AuthorizeCertificate: func(cert *x509.Certificate) error {
			if !bytes.Equal(cert.RawSubject, []byte{0x30, 0}) || len(cert.DNSNames)+len(cert.IPAddresses)+len(cert.EmailAddresses) != 0 {
				return errors.New("unapproved issued identity")
			}
			return nil
		},
	}
	t.Run("authorized URI control", func(t *testing.T) {
		csr, err := NewX509SVIDCSR(key, id)
		require.NoError(t, err)
		der, err := NewX509SVIDByCSRWithPolicy(ca, key, csr, id, policy)
		require.NoError(t, err)
		require.NotEmpty(t, der)
	})
	t.Run("wrong URI control", func(t *testing.T) {
		csr, err := NewX509SVIDCSR(key, "spiffe://example.org/other")
		require.NoError(t, err)
		der, err := NewX509SVIDByCSRWithPolicy(ca, key, csr, id, policy)
		require.ErrorContains(t, err, "not the authorized")
		require.Nil(t, der)
	})
	t.Run("unrelated DNS and subject", func(t *testing.T) {
		csr, err := NewX509SVIDCSR(key, id, WithX509CSRCommonName("unapproved"), WithX509CSRDNSNames("unapproved.example.org"))
		require.NoError(t, err)
		request, err := x509.ParseCertificateRequest(csr)
		require.NoError(t, err)
		require.NoError(t, request.CheckSignature(), "possession control")
		der, err := NewX509SVIDByCSRWithPolicy(ca, key, csr, id, policy)
		require.Error(t, err, "authorizing one URI must not implicitly authorize other identities")
		require.Nil(t, der)
		legacy, err := NewX509SVIDByCSR(ca, key, csr, id)
		require.NoError(t, err, "compatibility helper behavior is preserved")
		cert := svidTestParse(t, legacy)
		require.Equal(t, "unapproved", cert.Subject.CommonName)
		require.Equal(t, []string{"unapproved.example.org"}, cert.DNSNames)
	})
}

// TestX509CSRPolicyOriginalURI preserves authorized original URI octets during policy issuance.
func TestX509CSRPolicyOriginalURI(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	for _, value := range []string{"https://example.org/service#", "urn:example:service#"} {
		ext := svidRawURIExtension(t, value)
		requestDER, err := NewX509CSR(key, WithX509CSRCommonName("service"), WithX509CSRExtraExtension(ext))
		require.NoError(t, err)
		check := func(extensions []pkix.Extension) error {
			for _, actual := range extensions {
				if actual.Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 17}) && bytes.Equal(actual.Value, ext.Value) {
					return nil
				}
			}
			return errors.New("original URI SAN octets were not authorized")
		}
		policy := X509CSRIdentityPolicy{
			AuthorizeRequest:     func(csr *x509.CertificateRequest) error { return check(csr.Extensions) },
			AuthorizeCertificate: func(cert *x509.Certificate) error { return check(cert.Extensions) },
		}
		issued, err := NewX509CertByCSRWithPolicy(ca, key, requestDER, policy)
		require.NoError(t, err)
		require.NotEmpty(t, issued)
	}
}
