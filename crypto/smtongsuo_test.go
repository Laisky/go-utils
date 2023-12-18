package crypto

import (
	"context"
	"crypto/x509"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func testSkipSmTongsuo(t *testing.T) (skipped bool) {
	t.Helper()
	if _, err := exec.LookPath("tongsuo"); err != nil {
		require.ErrorIs(t, err, exec.ErrNotFound)
		return true
	}

	return false
}

func TestTongsuo_NewPrikeyAndCert(t *testing.T) {
	if testSkipSmTongsuo(t) {
		return
	}

	t.Parallel()

	tongsuo := &Tongsuo{} // Create an instance of Tongsuo

	// Define the X509CertOption options for the test
	opts := []X509CertOption{
		WithX509CertCommonName("test"),
		WithX509CertOrganization("test org"),
	}

	prikeyPem, certDer, err := tongsuo.NewPrikeyAndCert(context.Background(), opts...)
	require.NoError(t, err)
	require.NotNil(t, prikeyPem)
	require.NotNil(t, certDer)

	// Verify that the generated certificate is valid
	cert, err := x509.ParseCertificate(certDer)
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.Equal(t, "test", cert.Subject.CommonName)
	require.Equal(t, "test org", cert.Subject.Organization[0])
}
