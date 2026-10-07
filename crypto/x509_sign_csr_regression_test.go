package crypto

import (
	"crypto/x509"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewX509CertByCSRRejectsNilParent verifies that NewX509CertByCSR returns
// an error, instead of panicking on parent.IsCA, when no parent (issuer)
// certificate is supplied.
func TestNewX509CertByCSRRejectsNilParent(t *testing.T) {
	t.Parallel()
	prikey, err := NewECDSAPrikey(ECDSACurveP256)
	require.NoError(t, err)
	csrDer, err := NewX509CSR(prikey, WithX509CSRCommonName("leaf.example.invalid"))
	require.NoError(t, err)

	var certDer []byte
	require.NotPanics(t, func() {
		certDer, err = NewX509CertByCSR((*x509.Certificate)(nil), prikey, csrDer)
	})
	require.Error(t, err)
	require.Nil(t, certDer)
}
