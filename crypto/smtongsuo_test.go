package crypto

import (
	"context"
	"crypto/x509"
	"encoding/asn1"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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

		notbefore := time.Now().UTC().Truncate(time.Second)
		notafter := notbefore.Add(time.Hour * 24 * 7)
		opts := []X509CertOption{
			WithX509CertIsCA(),
			WithX509CertCommonName("test-common-name"),
			WithX509CertOrganization("test org"),
			WithX509CertPolicies(asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 1, 3}),
			WithX509CertPolicies(asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 2, 3}),
			WithX509CertNotBefore(notbefore),
			WithX509CertNotAfter(notafter),
		}

		prikeyPem, certDer, err := ins.NewPrikeyAndCert(context.Background(), opts...)
		require.NoError(t, err)
		require.NotNil(t, prikeyPem)
		require.NotNil(t, certDer)

		t.Run("verify pubkey", func(t *testing.T) {
			pubkeyFromPrikey, err := ins.Prikey2Pubkey(ctx, prikeyPem)
			require.NoError(t, err)

			pubkeyFromCert, err := ins.GetPubkeyFromCertPem(ctx, CertDer2Pem(certDer))
			require.NoError(t, err)

			require.Equal(t, pubkeyFromPrikey, pubkeyFromCert)
		})

		// Verify that the generated certificate is valid
		certinfo, err := ins.ShowCertInfo(ctx, certDer)
		// t.Log(string(certinfo.Raw))
		require.NoError(t, err)
		require.Contains(t, string(certinfo.Raw), "test-common-name")
		require.Contains(t, string(certinfo.Raw), "test org")
		require.Contains(t, string(certinfo.Raw), "CA:TRUE")
		require.Contains(t, string(certinfo.Raw), "1.3.6.1.4.1.59936.1.1.3")
		require.Contains(t, string(certinfo.Raw), "1.3.6.1.4.1.59936.1.2.3")
		require.NotEmpty(t, certinfo.SerialNumber)
		require.Equal(t, notbefore, certinfo.NotBefore.UTC())
		require.Equal(t, notafter, certinfo.NotAfter.UTC())
		require.Equal(t, "test-common-name", certinfo.Subject.CommonName)
		require.True(t, certinfo.IsCa)
		require.Contains(t, certinfo.Policies, asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 1, 3})
		require.Contains(t, certinfo.Policies, asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 2, 3})
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
		require.False(t, certinfo.IsCa)
		require.Equal(t, "test-common-name", certinfo.Subject.CommonName)
		require.Contains(t, certinfo.Policies, asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 1, 3})
		require.Contains(t, certinfo.Policies, asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 1, 4})
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
	rootCaPem := CertDer2Pem(rootCaDer)

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
		interL1, err := ins.NewX509CertByCSR(ctx, rootCaDer, rootCaPrikeyPem, csrder,
			WithX509SignCSRIsCA(),
			WithX509SignCSRPolicies(
				asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 1, 3},
				asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 1, 4},
			),
		)
		require.NoError(t, err)

		// Verify that the generated certificate is valid
		certinfo, err := ins.ShowCertInfo(ctx, interL1)
		t.Logf("test log test-intermediate: %s", string(certinfo.Raw))
		require.NoError(t, err)
		require.Contains(t, string(certinfo.Raw), "Subject: CN = test-intermediate")
		require.Contains(t, string(certinfo.Raw), "test org")
		require.Contains(t, string(certinfo.Raw), "CA:TRUE")
		require.Contains(t, string(certinfo.Raw), "1.3.6.1.4.1.59936.1.1.3")
		require.Contains(t, string(certinfo.Raw), "1.3.6.1.4.1.59936.1.1.4")
		require.Contains(t, string(certinfo.Raw), "Issuer: CN = test-rootca")
		require.NotEmpty(t, certinfo.SerialNumber)

		t.Run("verify with multiple intermediates and roots", func(t *testing.T) {
			_, uselessRootDer, err := ins.NewPrikeyAndCert(ctx,
				WithX509CertCommonName("useless-root"),
				WithX509CertIsCA(),
			)
			require.NoError(t, err)

			rootsPem := CertDer2Pem(uselessRootDer)
			rootsPem = append(rootsPem, rootCaPem...)

			interL2PrikeyPem, err := ins.NewPrikey(ctx)
			require.NoError(t, err)

			interL2CsrDer, err := ins.NewX509CSR(ctx, interL2PrikeyPem,
				WithX509CSRCommonName("test-intermediate-l2"),
			)
			require.NoError(t, err)

			interL2, err := ins.NewX509CertByCSR(ctx, interL1, prikeyPem, interL2CsrDer,
				WithX509SignCSRIsCA(),
			)
			require.NoError(t, err)

			var intersPem []byte
			intersPem = append(intersPem, CertDer2Pem(interL1)...)
			intersPem = append(intersPem, CertDer2Pem(interL2)...)

			// leaf
			leafPrikeyPem, err := ins.NewPrikey(ctx)
			require.NoError(t, err)

			leafCsrDer, err := ins.NewX509CSR(ctx, leafPrikeyPem,
				WithX509CSRCommonName("test-leaf"),
			)
			require.NoError(t, err)

			leafDer, err := ins.NewX509CertByCSR(ctx, interL2, interL2PrikeyPem, leafCsrDer)
			require.NoError(t, err)
			leafPem := CertDer2Pem(leafDer)

			// Verify that the generated certificate is valid
			err = ins.VerifyCertsChain(ctx, leafPem, intersPem, rootsPem)
			require.NoError(t, err)

			t.Run("miss read root", func(t *testing.T) {
				invalidRootsPem := CertDer2Pem(uselessRootDer)
				err := ins.VerifyCertsChain(ctx, leafPem, intersPem, invalidRootsPem)
				require.ErrorContains(t, err, "cannot verify certs chain")
			})
		})
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
		require.Contains(t, string(certinfo.Raw), "Subject: CN = test-intermediate")
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

		ciphertext, tag, err := ins.EncryptBySm4CbcBaisc(ctx, key, plaintext, iv)
		require.NoError(t, err)
		require.NotNil(t, ciphertext)
		require.Len(t, tag, 32)

		// Decrypt the ciphertext to verify the encryption
		decrypted, err := ins.DecryptBySm4CbcBaisc(ctx, key, ciphertext, iv, tag)
		require.NoError(t, err)
		require.Equal(t, plaintext, decrypted)
		// require.Equal(t, len(plaintext), len(ciphertext))
	})

	t.Run("Decrypt the ciphertext with incorrect key", func(t *testing.T) {
		t.Parallel()

		ciphertext, tag, err := ins.EncryptBySm4CbcBaisc(ctx, key, plaintext, iv)
		require.NoError(t, err)
		require.NotNil(t, ciphertext)

		_, err = ins.DecryptBySm4CbcBaisc(ctx, incorrectKey, ciphertext, iv, tag)
		require.ErrorContains(t, err, "hmac not match")

		t.Run("key in incorrect length", func(t *testing.T) {
			_, err = ins.DecryptBySm4CbcBaisc(ctx, append(key, 'd'), ciphertext, iv, tag)
			require.ErrorContains(t, err, "key should be 16 bytes")
		})

		t.Run("iv in incorrect length", func(t *testing.T) {
			_, err = ins.DecryptBySm4CbcBaisc(ctx, key, ciphertext, append(iv, 'a'), tag)
			require.ErrorContains(t, err, "iv should be 16 bytes")
		})
	})

	t.Run("Decrypt the ciphertext with incorrect tag", func(t *testing.T) {
		t.Parallel()

		ciphertext, _, err := ins.EncryptBySm4CbcBaisc(ctx, key, plaintext, iv)
		require.NoError(t, err)
		require.NotNil(t, ciphertext)

		_, err = ins.DecryptBySm4CbcBaisc(ctx, key, ciphertext, iv, incorrectTag)
		require.ErrorContains(t, err, "hmac not match")

		t.Run("tag in incorrect length", func(t *testing.T) {
			_, err = ins.DecryptBySm4CbcBaisc(ctx, key, ciphertext, iv, append(incorrectTag, []byte("123")...))
			require.ErrorContains(t, err, "hmac should be 0 or 32 bytes")
		})
	})

	t.Run("Decrypt the ciphertext with incorrect key and empty tag", func(t *testing.T) {
		t.Parallel()

		ciphertext, _, err := ins.EncryptBySm4CbcBaisc(ctx, key, plaintext, iv)
		require.NoError(t, err)
		require.NotNil(t, ciphertext)

		_, err = ins.DecryptBySm4CbcBaisc(ctx, incorrectKey, ciphertext, iv, nil)
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

	cipher, err := ins.EncryptBySm4Cbc(ctx, key, plaintext)
	require.NoError(t, err)

	gotPlain, err := ins.DecryptBySm4Cbc(ctx, key, cipher)
	require.NoError(t, err)
	require.Equal(t, plaintext, gotPlain)
}

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

func TestTongsuo_SignBySM2SM3(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	ctx := context.Background()
	ins, err := NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	prikeyPem, err := ins.NewPrikey(ctx)
	require.NoError(t, err)

	pubkeyPem, err := ins.Prikey2Pubkey(ctx, prikeyPem)
	require.NoError(t, err)

	raw, err := Salt(1024 * 8)
	require.NoError(t, err)

	signature, err := ins.SignBySm2Sm3(ctx, prikeyPem, raw)
	require.NoError(t, err)

	err = ins.VerifyBySm2Sm3(ctx, pubkeyPem, signature, raw)
	require.NoError(t, err)
}

func TestTongsuo_HashBySm3(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	ctx := context.Background()
	ins, err := NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	content := []byte("Hello, World!")

	hash, err := ins.HashBySm3(ctx, content)
	require.NoError(t, err)
	require.NotNil(t, hash)
	require.Len(t, hash, 32)
	require.NotContains(t, string(hash), "stdin")

	hash2, err := ins.HashBySm3(ctx, content)
	require.NoError(t, err)
	require.Equal(t, hash, hash2)

	hash3, err := ins.HashBySm3(ctx, append(content[:len(content)-1:len(content)-1], 'a'))
	require.NoError(t, err)
	require.NotEqual(t, hash, hash3)
}

func TestTongsuo_ShowCertInfo(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	ctx := context.Background()
	ins, err := NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	t.Run("test pubkey algorithm", func(t *testing.T) {
		t.Run("rsa", func(t *testing.T) {
			_, certDer, err := NewRSAPrikeyAndCert(RSAPrikeyBits2048,
				WithX509CertCommonName("test-rsa"),
			)

			certinfo, err := ins.ShowCertInfo(ctx, certDer)
			require.NoError(t, err)

			require.Equal(t, x509.RSA, certinfo.PublicKeyAlgorithm)
		})

		t.Run("ecdsa", func(t *testing.T) {
			_, certDer, err := NewECDSAPrikeyAndCert(ECDSACurveP256,
				WithX509CertCommonName("test-ecdsa"),
			)
			require.NoError(t, err)

			certinfo, err := ins.ShowCertInfo(ctx, certDer)
			require.NoError(t, err)

			require.Equal(t, x509.ECDSA, certinfo.PublicKeyAlgorithm)
		})

		t.Run("ed25519", func(t *testing.T) {
			_, certDer, err := NewEd25519PrikeyAndCert(
				WithX509CertCommonName("test-ed25519"),
			)
			require.NoError(t, err)

			certinfo, err := ins.ShowCertInfo(ctx, certDer)
			require.NoError(t, err)

			require.Equal(t, x509.Ed25519, certinfo.PublicKeyAlgorithm)
		})

		t.Run("sm2", func(t *testing.T) {
			_, certDer, err := ins.NewPrikeyAndCert(ctx,
				WithX509CertCommonName("test-sm2"),
			)
			require.NoError(t, err)

			certinfo, err := ins.ShowCertInfo(ctx, certDer)
			require.NoError(t, err)

			require.Equal(t, x509.ECDSA, certinfo.PublicKeyAlgorithm)
		})
	})
}
