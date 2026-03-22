package crypto

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConverterBehavior_NewRSAPrikeyInvalidBits verifies that unsupported RSA
// bit sizes are rejected with a descriptive error.
func TestConverterBehavior_NewRSAPrikeyInvalidBits(t *testing.T) {
	t.Parallel()

	for _, bits := range []RSAPrikeyBits{0, 512, 1024, 8192} {
		bits := bits
		t.Run("bits_"+string(rune(bits)), func(t *testing.T) {
			t.Parallel()
			_, err := NewRSAPrikey(bits)
			require.Error(t, err)
			require.Contains(t, err.Error(), "not support bits")
		})
	}
}

// TestConverterBehavior_NewECDSAPrikeyInvalidCurve verifies that unsupported
// ECDSA curves are rejected.
func TestConverterBehavior_NewECDSAPrikeyInvalidCurve(t *testing.T) {
	t.Parallel()

	for _, curve := range []ECDSACurve{"P224", "P128", "invalid", ""} {
		curve := curve
		t.Run("curve_"+string(curve), func(t *testing.T) {
			t.Parallel()
			_, err := NewECDSAPrikey(curve)
			require.Error(t, err)
			require.Contains(t, err.Error(), "unsupport curve")
		})
	}
}

// TestConverterBehavior_Prikey2DerUnsupportedType verifies that passing an
// unsupported key type to Prikey2Der returns an error.
func TestConverterBehavior_Prikey2DerUnsupportedType(t *testing.T) {
	t.Parallel()

	_, err := Prikey2Der("not-a-key")
	require.Error(t, err)
	require.Contains(t, err.Error(), "only support rsa/ecdsa/ed25519 private key")
}

// TestConverterBehavior_Pubkey2DerUnsupportedType verifies that passing an
// unsupported key type to Pubkey2Der returns an error.
func TestConverterBehavior_Pubkey2DerUnsupportedType(t *testing.T) {
	t.Parallel()

	_, err := Pubkey2Der("not-a-key")
	require.Error(t, err)
	require.Contains(t, err.Error(), "only support rsa/ecdsa/ed25519 public key")
}

// TestConverterBehavior_PrikeyPemRoundtrip verifies Prikey2Pem -> Pem2Prikey
// roundtrip for RSA, ECDSA, and Ed25519 keys.
func TestConverterBehavior_PrikeyPemRoundtrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		genKey func() (crypto.PrivateKey, error)
	}{
		{
			name: "RSA",
			genKey: func() (crypto.PrivateKey, error) {
				return NewRSAPrikey(RSAPrikeyBits2048)
			},
		},
		{
			name: "ECDSA",
			genKey: func() (crypto.PrivateKey, error) {
				return NewECDSAPrikey(ECDSACurveP256)
			},
		},
		{
			name: "Ed25519",
			genKey: func() (crypto.PrivateKey, error) {
				return NewEd25519Prikey()
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			key, err := tc.genKey()
			require.NoError(t, err)

			pemBytes, err := Prikey2Pem(key)
			require.NoError(t, err)
			require.NotEmpty(t, pemBytes)

			parsed, err := Pem2Prikey(pemBytes)
			require.NoError(t, err)
			require.NotNil(t, parsed)

			// Verify the parsed key is the same type
			switch key.(type) {
			case *rsa.PrivateKey:
				require.IsType(t, &rsa.PrivateKey{}, parsed)
			case *ecdsa.PrivateKey:
				require.IsType(t, &ecdsa.PrivateKey{}, parsed)
			case ed25519.PrivateKey:
				require.IsType(t, ed25519.PrivateKey{}, parsed)
			}
		})
	}
}

// TestConverterBehavior_PubkeyPemRoundtrip verifies Pubkey2Pem -> Pem2Pubkey
// roundtrip for RSA, ECDSA, and Ed25519 keys.
func TestConverterBehavior_PubkeyPemRoundtrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		genKey func() (crypto.PublicKey, error)
	}{
		{
			name: "RSA",
			genKey: func() (crypto.PublicKey, error) {
				k, err := NewRSAPrikey(RSAPrikeyBits2048)
				if err != nil {
					return nil, err
				}
				return &k.PublicKey, nil
			},
		},
		{
			name: "ECDSA",
			genKey: func() (crypto.PublicKey, error) {
				k, err := NewECDSAPrikey(ECDSACurveP256)
				if err != nil {
					return nil, err
				}
				return &k.PublicKey, nil
			},
		},
		{
			name: "Ed25519",
			genKey: func() (crypto.PublicKey, error) {
				k, err := NewEd25519Prikey()
				if err != nil {
					return nil, err
				}
				return k.Public(), nil
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			key, err := tc.genKey()
			require.NoError(t, err)

			pemBytes, err := Pubkey2Pem(key)
			require.NoError(t, err)
			require.NotEmpty(t, pemBytes)

			parsed, err := Pem2Pubkey(pemBytes)
			require.NoError(t, err)
			require.NotNil(t, parsed)

			switch key.(type) {
			case *rsa.PublicKey:
				require.IsType(t, &rsa.PublicKey{}, parsed)
			case *ecdsa.PublicKey:
				require.IsType(t, &ecdsa.PublicKey{}, parsed)
			case ed25519.PublicKey:
				require.IsType(t, ed25519.PublicKey{}, parsed)
			}
		})
	}
}

// TestConverterBehavior_Pem2DerInvalidPem verifies that garbage input to
// Pem2Der produces a "pem format invalid" error.
func TestConverterBehavior_Pem2DerInvalidPem(t *testing.T) {
	t.Parallel()

	_, err := Pem2Der([]byte("this is not valid pem data"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "pem format invalid")
}

// TestConverterBehavior_Pem2DersSingleAndMultiple verifies Pem2Ders correctly
// splits single and multiple PEM blocks into separate DER blocks.
func TestConverterBehavior_Pem2DersSingleAndMultiple(t *testing.T) {
	t.Parallel()

	// Generate two keys to get two distinct PEM blocks
	key1, err := NewEd25519Prikey()
	require.NoError(t, err)
	pem1, err := Prikey2Pem(key1)
	require.NoError(t, err)

	key2, err := NewEd25519Prikey()
	require.NoError(t, err)
	pem2, err := Prikey2Pem(key2)
	require.NoError(t, err)

	t.Run("single_block", func(t *testing.T) {
		t.Parallel()
		ders, err := Pem2Ders(pem1)
		require.NoError(t, err)
		require.Len(t, ders, 1)
	})

	t.Run("multiple_blocks", func(t *testing.T) {
		t.Parallel()
		combined := append(pem1, pem2...)
		ders, err := Pem2Ders(combined)
		require.NoError(t, err)
		require.Len(t, ders, 2)
	})

	t.Run("invalid_input", func(t *testing.T) {
		t.Parallel()
		_, err := Pem2Ders([]byte("garbage"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "pem format invalid")
	})
}

// TestConverterBehavior_SplitCertsPemChain verifies splitting a PEM chain and
// handling empty input.
func TestConverterBehavior_SplitCertsPemChain(t *testing.T) {
	t.Parallel()

	t.Run("empty_string", func(t *testing.T) {
		t.Parallel()
		result := SplitCertsPemChain("")
		require.Nil(t, result)
	})

	t.Run("two_certs", func(t *testing.T) {
		t.Parallel()

		// Generate two self-signed certs
		_, certDer1, err := NewRSAPrikeyAndCert(RSAPrikeyBits2048,
			WithX509CertCommonName("test-cert-1"))
		require.NoError(t, err)
		certPem1 := CertDer2Pem(certDer1)

		_, certDer2, err := NewRSAPrikeyAndCert(RSAPrikeyBits2048,
			WithX509CertCommonName("test-cert-2"))
		require.NoError(t, err)
		certPem2 := CertDer2Pem(certDer2)

		chain := string(certPem1) + string(certPem2)
		result := SplitCertsPemChain(chain)
		require.Len(t, result, 2)

		// Each result should contain BEGIN and END markers
		for _, p := range result {
			require.Contains(t, p, "-----BEGIN CERTIFICATE-----")
			require.Contains(t, p, "-----END CERTIFICATE-----")
		}
	})
}

// TestConverterBehavior_SecureCipherSuites verifies cipher suite filtering.
func TestConverterBehavior_SecureCipherSuites(t *testing.T) {
	t.Parallel()

	t.Run("nil_filter_returns_all", func(t *testing.T) {
		t.Parallel()
		suites := SecureCipherSuites(nil)
		require.NotEmpty(t, suites)
		// Should return all built-in secure cipher suites
		require.Equal(t, len(tls.CipherSuites()), len(suites))
	})

	t.Run("filter_returns_subset", func(t *testing.T) {
		t.Parallel()
		allSuites := SecureCipherSuites(nil)

		// Filter to only TLS 1.3 suites
		filtered := SecureCipherSuites(func(s *tls.CipherSuite) bool {
			for _, v := range s.SupportedVersions {
				if v == tls.VersionTLS13 {
					return true
				}
			}
			return false
		})

		require.NotEmpty(t, filtered)
		require.LessOrEqual(t, len(filtered), len(allSuites))
	})
}

// TestConverterBehavior_Der2PrikeyPKCS1Fallback verifies that an RSA key
// encoded in PKCS1 format can be parsed via the PKCS1 fallback path.
func TestConverterBehavior_Der2PrikeyPKCS1Fallback(t *testing.T) {
	t.Parallel()

	rsaKey, err := NewRSAPrikey(RSAPrikeyBits2048)
	require.NoError(t, err)

	// Marshal in PKCS1 format (not PKCS8)
	pkcs1Der := x509.MarshalPKCS1PrivateKey(rsaKey)

	parsed, err := Der2Prikey(pkcs1Der)
	require.NoError(t, err)
	require.IsType(t, &rsa.PrivateKey{}, parsed)
}

// TestConverterBehavior_Der2PubkeyPKIXFallback verifies that ECDSA and Ed25519
// public keys in PKIX format can be parsed via the PKIX fallback path.
func TestConverterBehavior_Der2PubkeyPKIXFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		genKey func() (crypto.PublicKey, error)
		want   interface{}
	}{
		{
			name: "ECDSA",
			genKey: func() (crypto.PublicKey, error) {
				k, err := NewECDSAPrikey(ECDSACurveP256)
				if err != nil {
					return nil, err
				}
				return &k.PublicKey, nil
			},
			want: &ecdsa.PublicKey{},
		},
		{
			name: "Ed25519",
			genKey: func() (crypto.PublicKey, error) {
				k, err := NewEd25519Prikey()
				if err != nil {
					return nil, err
				}
				return k.Public(), nil
			},
			want: ed25519.PublicKey{},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pubkey, err := tc.genKey()
			require.NoError(t, err)

			// Marshal in PKIX format
			pkixDer, err := x509.MarshalPKIXPublicKey(pubkey)
			require.NoError(t, err)

			parsed, err := Der2Pubkey(pkixDer)
			require.NoError(t, err)
			require.IsType(t, tc.want, parsed)
		})
	}
}

// TestConverterBehavior_DerPemBlockTypes verifies that the Der2Pem functions
// produce PEM blocks with correct type headers.
func TestConverterBehavior_DerPemBlockTypes(t *testing.T) {
	t.Parallel()

	dummyDer := []byte("dummy-der-data")

	tests := []struct {
		name     string
		fn       func([]byte) []byte
		expected string
	}{
		{"CertDer2Pem", CertDer2Pem, "CERTIFICATE"},
		{"PrikeyDer2Pem", PrikeyDer2Pem, "PRIVATE KEY"},
		{"PubkeyDer2Pem", PubkeyDer2Pem, "PUBLIC KEY"},
		{"CSRDer2Pem", CSRDer2Pem, "CERTIFICATE REQUEST"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := tc.fn(dummyDer)
			block, _ := pem.Decode(result)
			require.NotNil(t, block)
			require.Equal(t, tc.expected, block.Type)
		})
	}
}

// TestConverterBehavior_VerifyCertByPrikey verifies that matching cert+key
// succeeds and mismatched cert+key fails.
func TestConverterBehavior_VerifyCertByPrikey(t *testing.T) {
	t.Parallel()

	t.Run("matching_key", func(t *testing.T) {
		t.Parallel()

		prikeyPem, certDer, err := NewRSAPrikeyAndCert(RSAPrikeyBits2048,
			WithX509CertCommonName("test-match"))
		require.NoError(t, err)

		certPem := CertDer2Pem(certDer)
		err = VerifyCertByPrikey(certPem, prikeyPem)
		require.NoError(t, err)
	})

	t.Run("mismatched_key", func(t *testing.T) {
		t.Parallel()

		// Generate cert with one key
		_, certDer, err := NewRSAPrikeyAndCert(RSAPrikeyBits2048,
			WithX509CertCommonName("test-mismatch"))
		require.NoError(t, err)
		certPem := CertDer2Pem(certDer)

		// Generate a different key
		otherKey, err := NewRSAPrikey(RSAPrikeyBits2048)
		require.NoError(t, err)
		otherPrikeyPem, err := Prikey2Pem(otherKey)
		require.NoError(t, err)

		err = VerifyCertByPrikey(certPem, otherPrikeyPem)
		require.Error(t, err)
	})
}

// TestConverterBehavior_CertPemRoundtrip verifies Cert2Pem -> Pem2Cert
// roundtrip preserves certificate fields.
func TestConverterBehavior_CertPemRoundtrip(t *testing.T) {
	t.Parallel()

	_, certDer, err := NewRSAPrikeyAndCert(RSAPrikeyBits2048,
		WithX509CertCommonName("roundtrip-test"))
	require.NoError(t, err)

	cert, err := Der2Cert(certDer)
	require.NoError(t, err)

	certPem := Cert2Pem(cert)
	require.NotEmpty(t, certPem)

	parsed, err := Pem2Cert(certPem)
	require.NoError(t, err)
	require.Equal(t, "roundtrip-test", parsed.Subject.CommonName)
	require.Equal(t, cert.SerialNumber, parsed.SerialNumber)
	require.Equal(t, cert.NotBefore.Unix(), parsed.NotBefore.Unix())
	require.Equal(t, cert.NotAfter.Unix(), parsed.NotAfter.Unix())
}

// TestConverterBehavior_Pem2CertsMultiple verifies Pem2Certs can parse
// concatenated PEM certificates.
func TestConverterBehavior_Pem2CertsMultiple(t *testing.T) {
	t.Parallel()

	_, certDer1, err := NewRSAPrikeyAndCert(RSAPrikeyBits2048,
		WithX509CertCommonName("multi-cert-1"))
	require.NoError(t, err)
	cert1, err := Der2Cert(certDer1)
	require.NoError(t, err)

	_, certDer2, err := NewRSAPrikeyAndCert(RSAPrikeyBits2048,
		WithX509CertCommonName("multi-cert-2"))
	require.NoError(t, err)
	cert2, err := Der2Cert(certDer2)
	require.NoError(t, err)

	// Concatenate PEMs
	combined := append(Cert2Pem(cert1), Cert2Pem(cert2)...)

	certs, err := Pem2Certs(combined)
	require.NoError(t, err)
	require.Len(t, certs, 2)

	// Verify the common names are preserved (order may match)
	names := []string{certs[0].Subject.CommonName, certs[1].Subject.CommonName}
	require.Contains(t, names, "multi-cert-1")
	require.Contains(t, names, "multi-cert-2")
}

// TestConverterBehavior_SplitCertsPemChainWhitespace verifies edge cases for
// SplitCertsPemChain with whitespace-only input.
func TestConverterBehavior_SplitCertsPemChainWhitespace(t *testing.T) {
	t.Parallel()

	result := SplitCertsPemChain("   \n\t  ")
	require.Nil(t, result)
}

// TestConverterBehavior_Der2PubkeyRSAPKCS1 verifies that an RSA public key in
// PKCS1 format is parsed correctly via the primary path in Der2Pubkey.
func TestConverterBehavior_Der2PubkeyRSAPKCS1(t *testing.T) {
	t.Parallel()

	rsaKey, err := NewRSAPrikey(RSAPrikeyBits2048)
	require.NoError(t, err)

	// Marshal RSA public key in PKCS1 format
	pkcs1Der := x509.MarshalPKCS1PublicKey(&rsaKey.PublicKey)

	parsed, err := Der2Pubkey(pkcs1Der)
	require.NoError(t, err)
	require.IsType(t, &rsa.PublicKey{}, parsed)
}

// TestConverterBehavior_Pem2DerMultipleBlocks verifies that Pem2Der
// concatenates multiple PEM blocks into a single DER byte slice.
func TestConverterBehavior_Pem2DerMultipleBlocks(t *testing.T) {
	t.Parallel()

	_, certDer1, err := NewRSAPrikeyAndCert(RSAPrikeyBits2048,
		WithX509CertCommonName("pem2der-1"))
	require.NoError(t, err)

	_, certDer2, err := NewRSAPrikeyAndCert(RSAPrikeyBits2048,
		WithX509CertCommonName("pem2der-2"))
	require.NoError(t, err)

	pem1 := CertDer2Pem(certDer1)
	pem2 := CertDer2Pem(certDer2)
	combined := append(pem1, pem2...)

	der, err := Pem2Der(combined)
	require.NoError(t, err)
	// The concatenated DER should contain both certificates
	require.True(t, len(der) > len(certDer1))
	require.True(t, len(der) > len(certDer2))

	// Should be parseable as multiple certs
	certs, err := x509.ParseCertificates(der)
	require.NoError(t, err)
	require.Len(t, certs, 2)
}

// TestConverterBehavior_SecureCipherSuitesRejectAll verifies that a filter
// rejecting all suites returns an empty (nil) slice.
func TestConverterBehavior_SecureCipherSuitesRejectAll(t *testing.T) {
	t.Parallel()

	suites := SecureCipherSuites(func(s *tls.CipherSuite) bool {
		return false
	})
	require.Empty(t, suites)
}

// TestConverterBehavior_CRLDer2PemBlockType verifies the CRL PEM block type.
func TestConverterBehavior_CRLDer2PemBlockType(t *testing.T) {
	t.Parallel()

	result := CRLDer2Pem([]byte("dummy"))
	block, _ := pem.Decode(result)
	require.NotNil(t, block)
	require.Equal(t, "X509 CRL", block.Type)
}

// TestConverterBehavior_Pem2DerEmptyInput verifies that empty PEM input
// returns an error.
func TestConverterBehavior_Pem2DerEmptyInput(t *testing.T) {
	t.Parallel()

	_, err := Pem2Der([]byte(""))
	require.Error(t, err)
	require.Contains(t, err.Error(), "pem format invalid")
}

// TestConverterBehavior_PrikeyPemHasCorrectHeader verifies that Prikey2Pem
// outputs PEM with "PRIVATE KEY" header (PKCS8 format).
func TestConverterBehavior_PrikeyPemHasCorrectHeader(t *testing.T) {
	t.Parallel()

	key, err := NewEd25519Prikey()
	require.NoError(t, err)

	pemBytes, err := Prikey2Pem(key)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(pemBytes), "-----BEGIN PRIVATE KEY-----"))
}
