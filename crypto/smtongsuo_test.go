package crypto

import (
	"context"
	"encoding/asn1"
	"os/exec"
	"testing"
	"time"

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
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	ctx := context.Background()
	ins, err := NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	t.Run("ca", func(t *testing.T) {
		t.Parallel()
		opts := []X509CertOption{
			WithX509CertIsCA(),
			WithX509CertCommonName("test-common-name"),
			WithX509CertOrganization("test org"),
			WithX509CertPolicies(asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 1, 3}),
		}

		prikeyPem, certDer, err := ins.NewPrikeyAndCert(context.Background(), opts...)
		require.NoError(t, err)
		require.NotNil(t, prikeyPem)
		require.NotNil(t, certDer)

		// Verify that the generated certificate is valid
		certinfo, err := ins.ShowCertInfo(ctx, certDer)
		// t.Log(certinfo)
		require.NoError(t, err)
		require.Contains(t, certinfo, "test-common-name")
		require.Contains(t, certinfo, "test org")
		require.Contains(t, certinfo, "CA:TRUE")
		require.Contains(t, certinfo, "1.3.6.1.4.1.59936.1.1.3")
	})

	t.Run("not ca", func(t *testing.T) {
		t.Parallel()
		notafter := time.Now().Add(time.Hour * 24 * 365 * 10)

		opts := []X509CertOption{
			WithX509CertCommonName("test-common-name"),
			WithX509CertOrganization("test org"),
			WithX509CertNotAfter(notafter),
			WithX509CertPolicies(
				asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 1, 3},
				asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 1, 4},
			),
		}

		prikeyPem, certDer, err := ins.NewPrikeyAndCert(context.Background(), opts...)
		require.NoError(t, err)
		require.NotNil(t, prikeyPem)
		require.NotNil(t, certDer)

		// Verify that the generated certificate is valid
		certinfo, err := ins.ShowCertInfo(ctx, certDer)
		// t.Log(certinfo)
		require.NoError(t, err)
		require.Contains(t, certinfo, "test-common-name")
		require.Contains(t, certinfo, "test org")
		require.Contains(t, certinfo, "CA:FALSE")
		require.Contains(t, certinfo, "1.3.6.1.4.1.59936.1.1.3")
		require.Contains(t, certinfo, "1.3.6.1.4.1.59936.1.1.4")
		require.Contains(t, certinfo, notafter.UTC().Format("2006 GMT"))
	})
}

func TestTongsuo_NewIntermediaCaByCsr(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	ctx := context.Background()
	ins, err := NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	// new root ca
	caPrikeyPem, caDer, err := ins.NewPrikeyAndCert(ctx,
		WithX509CertCommonName("test-common-name"),
		WithX509CertIsCA())
	require.NoError(t, err)

	// new prikey
	prikeyPem, err := ins.NewPrikey(ctx)
	require.NoError(t, err)

	// new csr
	csrder, err := ins.NewX509CSR(ctx, prikeyPem,
		WithX509CSRCommonName("test-common-name"),
		WithX509CSROrganization("test org"),
	)
	require.NoError(t, err)

	t.Run("sign csr as ca", func(t *testing.T) {
		certDer, err := ins.NewX509CertByCSR(ctx, caDer, caPrikeyPem, csrder,
			WithX509SignCSRIsCA(),
			WithX509SignCSRPolicies(
				asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 1, 3},
				asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 1, 4},
			),
		)
		require.NoError(t, err)

		// Verify that the generated certificate is valid
		certinfo, err := ins.ShowCertInfo(ctx, certDer)
		// t.Log(certinfo)
		require.NoError(t, err)
		require.Contains(t, certinfo, "test-common-name")
		require.Contains(t, certinfo, "test org")
		require.Contains(t, certinfo, "CA:TRUE")
		require.Contains(t, certinfo, "1.3.6.1.4.1.59936.1.1.3")
		require.Contains(t, certinfo, "1.3.6.1.4.1.59936.1.1.4")
		require.Contains(t, certinfo, "Issuer: CN = test-common-name")
	})

	t.Run("sign csr as not ca", func(t *testing.T) {
		certDer, err := ins.NewX509CertByCSR(ctx, caDer, caPrikeyPem, csrder,
			WithX509SignCSRPolicies(
				asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 1, 3},
			),
		)
		require.NoError(t, err)

		// Verify that the generated certificate is valid
		certinfo, err := ins.ShowCertInfo(ctx, certDer)
		// t.Log(certinfo)
		require.NoError(t, err)
		require.Contains(t, certinfo, "test-common-name")
		require.Contains(t, certinfo, "test org")
		require.Contains(t, certinfo, "CA:FALSE")
		require.Contains(t, certinfo, "1.3.6.1.4.1.59936.1.1.3")
		require.NotContains(t, certinfo, "1.3.6.1.4.1.59936.1.1.4")
		require.Contains(t, certinfo, "Issuer: CN = test-common-name")
	})
}
