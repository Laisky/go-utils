package crypto

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestX509SANIssuerControls preserves valid empty-Subject issuance and raw-name precedence.
func TestX509SANIssuerControls(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	t.Run("populated CN-less self-signed DN", func(t *testing.T) {
		der, err := NewX509Cert(key, WithX509CertAllowEmptySubject(),
			WithX509Subject(pkix.Name{Organization: []string{"issuer organization"}}))
		require.NoError(t, err)
		cert := svidTestParse(t, der)
		require.NotEqual(t, []byte{0x30, 0}, cert.RawIssuer)
		require.Empty(t, cert.Issuer.CommonName)
		require.Equal(t, []string{"issuer organization"}, cert.Issuer.Organization)
		require.NoError(t, cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature))
	})
	t.Run("empty leaf with populated original parent DN", func(t *testing.T) {
		parent := *ca
		parent.Subject = pkix.Name{} // Original DER still takes precedence.
		der, err := NewX509Cert(key, WithX509CertAllowEmptySubject(),
			WithX509CertParent(&parent), WithX509CertDNSNames("service.example.org"))
		require.NoError(t, err)
		cert := svidTestParse(t, der)
		require.Equal(t, []byte{0x30, 0}, cert.RawSubject)
		require.Equal(t, ca.RawSubject, cert.RawIssuer)
		require.NoError(t, cert.CheckSignatureFrom(ca))
		roots := x509.NewCertPool()
		roots.AddCert(ca)
		_, err = cert.Verify(x509.VerifyOptions{Roots: roots, DNSName: "service.example.org"})
		require.NoError(t, err)
	})
	t.Run("legacy issuer behavior", func(t *testing.T) {
		parent := *ca
		parent.RawSubject = []byte{0x30, 0}
		der, err := NewX509Cert(key, WithX509CertCommonName("legacy"), WithX509CertParent(&parent))
		require.NoError(t, err)
		cert := svidTestParse(t, der)
		require.Equal(t, []byte{0x30, 0}, cert.RawIssuer)
		require.NoError(t, cert.CheckSignatureFrom(ca))
	})
}

// TestX509SVIDIssuerRejectsEmpty applies the same issuer rule to the final SVID profile.
func TestX509SVIDIssuerRejectsEmpty(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	der, err := NewX509SVID(key, svidTestID, WithX509CertParent(ca))
	require.NoError(t, err, "valid empty-Subject SVID control")
	_, err = ValidateX509SVID(svidTestParse(t, der))
	require.NoError(t, err)
	t.Run("direct issuance", func(t *testing.T) {
		der, err := NewX509SVID(key, svidTestID)
		require.ErrorContains(t, err, "nonempty Issuer DN")
		require.Nil(t, der)
	})
	parent := *ca
	parent.RawSubject = []byte{0x30, 0}
	id, err := ParseX509SVIDID(svidTestID)
	require.NoError(t, err)
	// The generic writer remains a compatibility control for nonconformant DER.
	der, err = NewX509Cert(key, WithX509CertCommonName("legacy SVID fixture"),
		WithX509CertParent(&parent), WithX509CertURIs(id.URL()),
		WithX509CertExtKeyUsage(x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth))
	require.NoError(t, err)
	cert := svidTestParse(t, der)
	require.Equal(t, []byte{0x30, 0}, cert.RawIssuer)
	cert.Issuer = ca.Subject // Parsed-cache mutation must not bypass the encoded check.
	_, err = ValidateX509SVID(cert)
	require.ErrorContains(t, err, "nonempty Issuer DN")
}

// TestX509SANIssuerRejectsEmpty requires nonempty final Issuer DER in opt-in issuance.
func TestX509SANIssuerRejectsEmpty(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	parent := *ca
	parent.RawSubject = []byte{0x30, 0} // A populated cached Subject cannot override this DER.
	for name, opts := range map[string][]X509CertOption{
		"self-signed empty Subject":    {WithX509CertDNSNames("service.example.org")},
		"explicit empty raw parent DN": {WithX509CertCommonName("strict"), WithX509CertParent(&parent)},
	} {
		t.Run(name, func(t *testing.T) {
			der, err := NewX509Cert(key, append(opts, WithX509CertAllowEmptySubject())...)
			if err == nil {
				cert := svidTestParse(t, der)
				require.Equal(t, []byte{0x30, 0}, cert.RawIssuer, "behavioral reproduction: empty encoded Issuer")
				require.NoError(t, cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature))
			}
			require.ErrorContains(t, err, "nonempty Issuer DN")
			require.Nil(t, der)
		})
	}
	csrDER, err := NewX509CSR(key, WithX509CSRCommonName("policy leaf"), WithX509CSRDNSNames("service.example.org"))
	require.NoError(t, err)
	svidCSR, err := NewX509SVIDCSR(key, "spiffe://example.org/service")
	require.NoError(t, err)
	for name, issue := range map[string]func(*x509.Certificate, X509CSRIdentityPolicy) ([]byte, error){
		"generic policy": func(parent *x509.Certificate, policy X509CSRIdentityPolicy) ([]byte, error) {
			return NewX509CertByCSRWithPolicy(parent, key, csrDER, policy)
		},
		"SVID policy": func(parent *x509.Certificate, policy X509CSRIdentityPolicy) ([]byte, error) {
			return NewX509SVIDByCSRWithPolicy(parent, key, svidCSR, "spiffe://example.org/service", policy)
		},
	} {
		t.Run(name, func(t *testing.T) {
			requests, certificates := 0, 0
			policy := X509CSRIdentityPolicy{
				AuthorizeRequest:     func(*x509.CertificateRequest) error { requests++; return nil },
				AuthorizeCertificate: func(*x509.Certificate) error { certificates++; return nil },
			}
			der, err := issue(ca, policy)
			require.NoError(t, err, "valid issuer and policy control")
			require.NoError(t, svidTestParse(t, der).CheckSignatureFrom(ca))
			require.Equal(t, 1, requests)
			require.Equal(t, 1, certificates)
			der, err = issue(&parent, policy)
			require.ErrorContains(t, err, "nonempty Issuer DN")
			require.Nil(t, der)
			require.Equal(t, 2, requests)
			require.Equal(t, 1, certificates, "invalid final issuer must fail before final authorization")
		})
	}
}
