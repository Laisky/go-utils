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
		require.Contains(t, string(certinfo.Raw), "test-common-name")
		require.Contains(t, string(certinfo.Raw), "test org")
		require.Contains(t, string(certinfo.Raw), "CA:TRUE")
		require.Contains(t, string(certinfo.Raw), "1.3.6.1.4.1.59936.1.1.3")
		require.NotEmpty(t, certinfo.SerialNumber)
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
		require.Contains(t, string(certinfo.Raw), "test-common-name")
		require.Contains(t, string(certinfo.Raw), "test org")
		require.Contains(t, string(certinfo.Raw), "CA:FALSE")
		require.Contains(t, string(certinfo.Raw), "1.3.6.1.4.1.59936.1.1.3")
		require.Contains(t, string(certinfo.Raw), "1.3.6.1.4.1.59936.1.1.4")
		require.Contains(t, string(certinfo.Raw), notafter.UTC().Format("2006 GMT"))
		require.NotEmpty(t, certinfo.SerialNumber)
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
	rootCaPrikeyPem, rootCaDer, err := ins.NewPrikeyAndCert(ctx,
		WithX509CertCommonName("test-rootca"),
		WithX509CertIsCA())
	require.NoError(t, err)

	// new prikey
	prikeyPem, err := ins.NewPrikey(ctx)
	require.NoError(t, err)

	// new csr
	csrder, err := ins.NewX509CSR(ctx, prikeyPem,
		WithX509CSRCommonName("test-intermediate"),
		WithX509CSROrganization("test org"),
	)
	require.NoError(t, err)

	t.Run("sign csr as ca", func(t *testing.T) {
		certDer, err := ins.NewX509CertByCSR(ctx, rootCaDer, rootCaPrikeyPem, csrder,
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
		require.Contains(t, string(certinfo.Raw), "test-intermediate")
		require.Contains(t, string(certinfo.Raw), "test org")
		require.Contains(t, string(certinfo.Raw), "CA:TRUE")
		require.Contains(t, string(certinfo.Raw), "1.3.6.1.4.1.59936.1.1.3")
		require.Contains(t, string(certinfo.Raw), "1.3.6.1.4.1.59936.1.1.4")
		require.Contains(t, string(certinfo.Raw), "Issuer: CN = test-rootca")
		require.NotEmpty(t, certinfo.SerialNumber)
	})

	t.Run("sign csr as not ca", func(t *testing.T) {
		certDer, err := ins.NewX509CertByCSR(ctx, rootCaDer, rootCaPrikeyPem, csrder,
			WithX509SignCSRPolicies(
				asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 1, 3},
			),
		)
		require.NoError(t, err)

		// Verify that the generated certificate is valid
		certinfo, err := ins.ShowCertInfo(ctx, certDer)
		// t.Log(certinfo)
		require.NoError(t, err)
		require.Contains(t, string(certinfo.Raw), "test-intermediate")
		require.Contains(t, string(certinfo.Raw), "test org")
		require.Contains(t, string(certinfo.Raw), "CA:FALSE")
		require.Contains(t, string(certinfo.Raw), "1.3.6.1.4.1.59936.1.1.3")
		require.NotContains(t, string(certinfo.Raw), "1.3.6.1.4.1.59936.1.1.4")
		require.Contains(t, string(certinfo.Raw), "Issuer: CN = test-rootca")
		require.NotEmpty(t, certinfo.SerialNumber)
	})
}

func TestTongsuo_EncryptBySm4Baisc(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	ctx := context.Background()
	ins, err := NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	key, err := Salt(16)
	require.NoError(t, err)
	incorrectKey, err := Salt(16)
	require.NoError(t, err)
	plaintext := []byte("Hello, World!")
	iv, err := Salt(16)
	require.NoError(t, err)
	incorrectTag, err := Salt(32)
	require.NoError(t, err)

	t.Run("correct passphare", func(t *testing.T) {
		t.Parallel()

		ciphertext, tag, err := ins.EncryptBySm4Baisc(ctx, key, plaintext, iv)
		require.NoError(t, err)
		require.NotNil(t, ciphertext)
		require.Len(t, tag, 32)

		// Decrypt the ciphertext to verify the encryption
		decrypted, err := ins.DecryptBySm4Baisc(ctx, key, ciphertext, iv, tag)
		require.NoError(t, err)
		require.Equal(t, plaintext, decrypted)
		// require.Equal(t, len(plaintext), len(ciphertext))
	})

	t.Run("Decrypt the ciphertext with incorrect key", func(t *testing.T) {
		t.Parallel()

		ciphertext, tag, err := ins.EncryptBySm4Baisc(ctx, key, plaintext, iv)
		require.NoError(t, err)
		require.NotNil(t, ciphertext)

		_, err = ins.DecryptBySm4Baisc(ctx, incorrectKey, ciphertext, iv, tag)
		require.ErrorContains(t, err, "hmac not match")

		t.Run("key in incorrect length", func(t *testing.T) {
			_, err = ins.DecryptBySm4Baisc(ctx, append(key, 'd'), ciphertext, iv, tag)
			require.ErrorContains(t, err, "key should be 16 bytes")
		})

		t.Run("iv in incorrect length", func(t *testing.T) {
			_, err = ins.DecryptBySm4Baisc(ctx, key, ciphertext, append(iv, 'a'), tag)
			require.ErrorContains(t, err, "iv should be 16 bytes")
		})
	})

	t.Run("Decrypt the ciphertext with incorrect tag", func(t *testing.T) {
		t.Parallel()

		ciphertext, _, err := ins.EncryptBySm4Baisc(ctx, key, plaintext, iv)
		require.NoError(t, err)
		require.NotNil(t, ciphertext)

		_, err = ins.DecryptBySm4Baisc(ctx, key, ciphertext, iv, incorrectTag)
		require.ErrorContains(t, err, "hmac not match")

		t.Run("tag in incorrect length", func(t *testing.T) {
			_, err = ins.DecryptBySm4Baisc(ctx, key, ciphertext, iv, append(incorrectTag, []byte("123")...))
			require.ErrorContains(t, err, "hmac should be 0 or 32 bytes")
		})
	})

	t.Run("Decrypt the ciphertext with incorrect key and empty tag", func(t *testing.T) {
		t.Parallel()

		ciphertext, _, err := ins.EncryptBySm4Baisc(ctx, key, plaintext, iv)
		require.NoError(t, err)
		require.NotNil(t, ciphertext)

		_, err = ins.DecryptBySm4Baisc(ctx, incorrectKey, ciphertext, iv, nil)
		require.ErrorContains(t, err, "got bad decrypt")
	})
}

func TestTongsuo_DecryptBySm4(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	ctx := context.Background()
	ins, err := NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	key, err := Salt(16)
	require.NoError(t, err)
	plaintext := []byte("Hello, World!")

	cipher, err := ins.EncryptBySm4(ctx, key, plaintext)
	require.NoError(t, err)

	gotPlain, err := ins.DecryptBySm4(ctx, key, cipher)
	require.NoError(t, err)
	require.Equal(t, plaintext, gotPlain)
}

func TestTongsuo_CloneX509Csr(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	ctx := context.Background()
	ins, err := NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	prikeyOld, err := ins.NewPrikey(ctx)
	require.NoError(t, err)
	prikeyNew, err := ins.NewPrikey(ctx)
	require.NoError(t, err)

	csrder, err := ins.NewX509CSR(ctx, prikeyOld,
		WithX509CSRCommonName("test-common-name"),
		WithX509CSRCountry("CN"),
		WithX509CSROrganization("BBT"),
		WithX509CSRLocality("Shanghai"),
		WithX509CSRDNSNames("www.example.com", "www.example.net", "www.example.origin"),
		WithX509CSREmailAddrs("test@laisky.com"),
	)
	require.NoError(t, err)

	t.Run("valid csr info", func(t *testing.T) {
		t.Parallel()

		clonedCsr, err := ins.CloneX509Csr(ctx, prikeyNew, csrder)
		require.NoError(t, err)
		require.NotNil(t, clonedCsr)

		// Verify the generated cloned CSR
		clonedCsrInfo, err := ins.ShowCsrInfo(ctx, clonedCsr)
		require.NoError(t, err)
		require.Contains(t, clonedCsrInfo, "C = CN")
		require.Contains(t, clonedCsrInfo, "L = Shanghai")
		require.Contains(t, clonedCsrInfo, "O = BBT")
		require.Contains(t, clonedCsrInfo, "CN = test-common-name")
		require.Contains(t, clonedCsrInfo, "DNS:www.example.com")
		require.Contains(t, clonedCsrInfo, "DNS:www.example.net")
		require.Contains(t, clonedCsrInfo, "DNS:www.example.origin")
	})

}
