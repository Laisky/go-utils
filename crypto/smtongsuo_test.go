package crypto

import (
	"context"
	"crypto/x509"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSm2CrossAlgorithmSign verifies cross-algorithm issuance with Tongsuo: an SM2 root CA signs
// an RSA leaf CSR (the leaf keeps the RSA public key algorithm and its chain verifies, while an
// unrelated self-signed RSA certificate fails verification), and an RSA root CA signs an SM2
// leaf CSR whose chain verifies.
func TestSm2CrossAlgorithmSign(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	ctx := context.Background()
	ins, err := NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	t.Run("sm2 -> rsa", func(t *testing.T) {
		// root ca
		rootcaPrikeyPem, rootCaDer, err := ins.NewPrikeyAndCert(ctx,
			WithX509CertCommonName("sm2-rootca"),
			WithX509CertIsCA(),
		)
		require.NoError(t, err)

		// leaf cert & csr
		leafPrikey, err := NewRSAPrikey(RSAPrikeyBits2048)
		require.NoError(t, err)

		leafCsrDer, err := NewX509CSR(leafPrikey,
			WithX509CSRCommonName("leaf-rsa"),
		)
		require.NoError(t, err)

		// sign leaf cert by root ca
		leafCertDer, err := ins.NewX509CertByCSR(ctx, rootCaDer, rootcaPrikeyPem, leafCsrDer)
		require.NoError(t, err)

		leafCert, err := Der2Cert(leafCertDer)
		require.NoError(t, err, leafCert)
		require.Equal(t, x509.RSA, leafCert.PublicKeyAlgorithm)

		// print
		rootCaPem := CertDer2Pem(rootCaDer)
		// t.Logf("root ca: %s", rootCaPem)
		leafCertPem := CertDer2Pem(leafCertDer)
		// t.Logf("leaf cert: %s", leafCertPem)

		// verify
		err = ins.VerifyCertsChain(ctx, leafCertPem, nil, rootCaPem)
		require.NoError(t, err)

		t.Run("verify error", func(t *testing.T) {
			_, fakeLeafCertDer, err := NewRSAPrikeyAndCert(RSAPrikeyBits2048,
				WithX509CertCommonName("fake-leaf-rsa"),
			)
			require.NoError(t, err)

			fakeLeafCertPem := CertDer2Pem(fakeLeafCertDer)
			err = ins.VerifyCertsChain(ctx, fakeLeafCertPem, nil, rootCaPem)
			require.ErrorContains(t, err, "cannot verify certs chain")
		})
	})

	t.Run("rsa -> sm2", func(t *testing.T) {
		// root ca
		rootcaPrikeyPem, rootCaDer, err := NewRSAPrikeyAndCert(RSAPrikeyBits2048,
			WithX509CertCommonName("rsa-rootca"),
			WithX509CertIsCA(),
		)
		require.NoError(t, err)

		// leaf cert & csr
		leafPrikeyPem, err := ins.NewPrikey(ctx)
		require.NoError(t, err)
		leafCsrDer, err := ins.NewX509CSR(ctx, leafPrikeyPem,
			WithX509CSRCommonName("leaf-sm2"),
		)
		require.NoError(t, err)

		// sign leaf cert by root ca
		leafCertDer, err := ins.NewX509CertByCSR(ctx, rootCaDer, rootcaPrikeyPem, leafCsrDer)
		require.NoError(t, err)

		leafCertPem := CertDer2Pem(leafCertDer)
		// t.Logf("leaf cert: %s", leafCertPem)
		rootCaPem := CertDer2Pem(rootCaDer)

		// verify
		err = ins.VerifyCertsChain(ctx, leafCertPem, nil, rootCaPem)
		require.NoError(t, err)
	})
}

// Test_VerifyCertsChain verifies that Tongsuo.VerifyCertsChain accepts a leaf certificate signed
// through Tongsuo.NewX509CertByCSR for the SM2->SM2, RSA->SM2, and SM2->RSA root/leaf
// combinations.
func Test_VerifyCertsChain(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	ctx := context.Background()
	ins, err := NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	t.Run("sm2 -> sm2", func(t *testing.T) {
		rootcaPrikeyPem, rootcaCertDer, err := ins.NewPrikeyAndCert(ctx,
			WithX509CertCommonName("sm2-root-ca"),
			WithX509CertIsCA(),
		)
		require.NoError(t, err)

		leafPrikeyPem, err := ins.NewPrikey(ctx)
		require.NoError(t, err)

		leafCsrDer, err := ins.NewX509CSR(ctx, leafPrikeyPem,
			WithX509CSRCommonName("sm2-leaf"),
		)
		require.NoError(t, err)

		leafCertDer, err := ins.NewX509CertByCSR(ctx, rootcaCertDer, rootcaPrikeyPem, leafCsrDer)
		require.NoError(t, err)

		rootCertPem := CertDer2Pem(rootcaCertDer)
		leafCertPem := CertDer2Pem(leafCertDer)
		err = ins.VerifyCertsChain(ctx, leafCertPem, nil, rootCertPem)
		require.NoError(t, err)
	})

	t.Run("rsa -> sm2", func(t *testing.T) {
		rootcaPrikeyPem, rootcaCertDer, err := NewRSAPrikeyAndCert(RSAPrikeyBits2048,
			WithX509CertCommonName("rsa-root-ca"),
			WithX509CertIsCA(),
		)
		require.NoError(t, err)

		leafPrikeyPem, err := ins.NewPrikey(ctx)
		require.NoError(t, err)

		leafCsrDer, err := ins.NewX509CSR(ctx, leafPrikeyPem,
			WithX509CSRCommonName("sm2-leaf"),
		)
		require.NoError(t, err)

		leafCertDer, err := ins.NewX509CertByCSR(ctx, rootcaCertDer, rootcaPrikeyPem, leafCsrDer)
		require.NoError(t, err)

		rootCertPem := CertDer2Pem(rootcaCertDer)
		leafCertPem := CertDer2Pem(leafCertDer)
		err = ins.VerifyCertsChain(ctx, leafCertPem, nil, rootCertPem)
		require.NoError(t, err)
	})

	t.Run("sm2 -> rsa", func(t *testing.T) {
		rootcaPrikeyPem, rootcaCertDer, err := ins.NewPrikeyAndCert(ctx,
			WithX509CertCommonName("sm2-root-ca"),
			WithX509CertIsCA(),
		)
		require.NoError(t, err)

		leafPrikeyPem, err := NewRSAPrikey(RSAPrikeyBits2048)
		require.NoError(t, err)

		leafCsrDer, err := NewX509CSR(leafPrikeyPem,
			WithX509CSRCommonName("rsa-leaf"),
		)
		require.NoError(t, err)

		leafCertDer, err := ins.NewX509CertByCSR(ctx, rootcaCertDer, rootcaPrikeyPem, leafCsrDer)
		require.NoError(t, err)

		rootCertPem := CertDer2Pem(rootcaCertDer)
		leafCertPem := CertDer2Pem(leafCertDer)
		err = ins.VerifyCertsChain(ctx, leafCertPem, nil, rootCertPem)
		require.NoError(t, err)
	})

}

// testSkipSmTongsuo reports whether Tongsuo-dependent tests should be skipped because the
// tongsuo binary is not on PATH. It takes the calling test t, fails it if the PATH lookup errors
// for any reason other than exec.ErrNotFound, and returns true only when tongsuo is missing.
func testSkipSmTongsuo(t *testing.T) (skipped bool) {
	t.Helper()
	if _, err := exec.LookPath("tongsuo"); err != nil {
		require.ErrorIs(t, err, exec.ErrNotFound)
		return true
	}

	return false
}

// TestTongsuo_NewPrikeyWithPassword verifies that Tongsuo.NewPrikeyWithPassword returns an
// encrypted private key PEM for a non-empty password and rejects an empty password with
// "password should not be empty".
func TestTongsuo_NewPrikeyWithPassword(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	ctx := context.Background()
	ins, err := NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	t.Run("with password", func(t *testing.T) {
		prikeyPem, err := ins.NewPrikeyWithPassword(ctx, "test-password")
		require.NoError(t, err)
		require.NotNil(t, prikeyPem)
	})

	t.Run("without password", func(t *testing.T) {
		_, err := ins.NewPrikeyWithPassword(ctx, "")
		require.ErrorContains(t, err, "password should not be empty")
	})
}
