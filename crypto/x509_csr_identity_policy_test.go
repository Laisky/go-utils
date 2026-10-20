package crypto

import (
	"crypto/x509"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestX509CSRIdentityAuthorization reproduces SPIFFE authorization preserving unrelated CSR identities.
func TestX509CSRIdentityAuthorization(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	id := "spiffe://example.org/workload"
	t.Run("authorized URI control", func(t *testing.T) {
		csr, err := NewX509SVIDCSR(key, id)
		require.NoError(t, err)
		der, err := NewX509SVIDByCSR(ca, key, csr, id)
		require.NoError(t, err)
		require.NotEmpty(t, der)
	})
	t.Run("wrong URI control", func(t *testing.T) {
		csr, err := NewX509SVIDCSR(key, "spiffe://example.org/other")
		require.NoError(t, err)
		der, err := NewX509SVIDByCSR(ca, key, csr, id)
		require.ErrorContains(t, err, "not the authorized")
		require.Nil(t, der)
	})
	t.Run("unrelated DNS and subject", func(t *testing.T) {
		csr, err := NewX509SVIDCSR(key, id, WithX509CSRCommonName("unapproved"), WithX509CSRDNSNames("unapproved.example.org"))
		require.NoError(t, err)
		request, err := x509.ParseCertificateRequest(csr)
		require.NoError(t, err)
		require.NoError(t, request.CheckSignature(), "possession control")
		der, err := NewX509SVIDByCSR(ca, key, csr, id)
		require.Error(t, err, "authorizing one URI must not implicitly authorize other identities")
		require.Nil(t, der)
	})
}
