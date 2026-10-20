package crypto

import (
	"crypto/rand"
	"crypto/x509"
	"math/big"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestX509SVIDEmptySubjectControl proves the standard library supports empty-subject SVIDs.
func TestX509SVIDEmptySubjectControl(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	id, err := url.Parse("spiffe://example.org/workload")
	require.NoError(t, err)
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{URIs: []*url.URL{id}}, key)
	require.NoError(t, err)
	csr, err := x509.ParseCertificateRequest(csrDER)
	require.NoError(t, err)
	require.NoError(t, csr.CheckSignature())
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), URIs: []*url.URL{id}, KeyUsage: x509.KeyUsageDigitalSignature,
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, key.Public(), key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	require.Equal(t, "", cert.Subject.String())
	for _, ext := range cert.Extensions {
		if ext.Id.Equal([]int{2, 5, 29, 17}) {
			require.True(t, ext.Critical)
		}
	}
}

// TestX509SVIDGenericCNCompatibility preserves the generic common-name restriction.
func TestX509SVIDGenericCNCompatibility(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	id, err := url.Parse("spiffe://example.org/workload")
	require.NoError(t, err)
	t.Run("CSR", func(t *testing.T) {
		_, err = NewX509CSR(key, WithX509CSRURIs(id))
		require.ErrorContains(t, err, "common name is required")
	})
	t.Run("certificate", func(t *testing.T) {
		_, err = NewX509Cert(key, WithX509CertURIs(id))
		require.ErrorContains(t, err, "common name must be set")
	})
}
