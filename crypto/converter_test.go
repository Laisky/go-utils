package crypto

import (
	"crypto"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	testCertChain = `-----BEGIN CERTIFICATE-----
MIIE3DCCA0SgAwIBAgIUKvzFXZamgum1ss+T490hiYDoszAwDQYJKoZIhvcNAQEM
BQAwTDELMAkGA1UEBhMCVVMxCzAJBgNVBAgTAkNBMRYwFAYDVQQHEw1TYW4gRnJh
bmNpc2NvMRgwFgYDVQQDEw9zZ3gtY29vcmRpbmF0b3IwHhcNMjIwOTI4MDcxMzAw
WhcNMzIwOTI1MDcxMzAwWjBSMQswCQYDVQQGEwJVUzELMAkGA1UECBMCQ0ExFjAU
BgNVBAcTDVNhbiBGcmFuY2lzY28xHjAcBgNVBAMTFXNneC1jb29yZGluYXRvci1p
bnRlcjCCAaIwDQYJKoZIhvcNAQEBBQADggGPADCCAYoCggGBAMjh9A4Wmsy5LHQp
DjikniH/jqIsJJRg7TBUqdiNgCoQbWAPWj+a3huQ7AEKgQH+MdKvFwRIoOftAV7r
uNrX+a4Q/b1Kx1EvjNgCs8zSQYw3s/UBfw9BnXcrwGplj7wsanHFreS8Ul7VQ5NV
Fb5G20yw31tbXpb0LGj3t5hFU+v578soorJGB0OXFZm6HYs77FxdvHZFfluTA6aK
4ThutDqgwmhZydMVuuO95fe01DUFvwR7gXxkRJwIumJaoYYBGI2WBrD1BmRzrWBx
LoQU0AWUl/joV2qPLechpnVZuMb8nAM5/epPEkf6CF0Caj2+PY6VoZnM4iSafgzC
eu8oKKbyEWEaRz0f8TezUwpFl/ROa3JS9v0b3yILV1Jp1wFcPsIdF0925hOTM6/m
H0iCuFChzfKakEsE0I5DoVlgxHXq0ruOsmuY32Lp5vBJk7N5JNpnfUrELGGToDFm
ZqgbCRFdBXv5xVRFT2fdyrSpI6KvrUekVpsfe4FByEUfBPSqUQIDAQABo4GvMIGs
MA4GA1UdDwEB/wQEAwIBBjAPBgNVHRMBAf8EBTADAQH/MB0GA1UdDgQWBBRBJsMa
3GclgkZnPM0s2AHelW47uDAfBgNVHSMEGDAWgBSPUheLFd1VIGK858UuSYrMe42+
VDAPBgNVHREECDAGhwR/AAABMDgGA1UdHwQxMC8wLaAroCmGJ2h0dHBzOi8vczMu
bGFpc2t5LmNvbS9wdWJsaWMvbGFpc2t5LmNybDANBgkqhkiG9w0BAQwFAAOCAYEA
KKbLRHfaG/mEB3az4qoKBAQYy3SIDBSvBT5jT+AqLMzivLHAw5oHoF1AkfsGxcea
XQcFcqIVm49cS8x6hhY7RSCAnCzOcSOu5oGEuDvzqbc5O9DUtDEkh46kiVSnJzny
k2DJFpP0aXfRszSehEa58nQmWQMf9YmIGo/ZTKrO7Er0jXnXdWKTx4bZHbRYKnXG
MPC7YwtLB65kTab13Ln0/c9gsb0yFjfg6Niz6uEGDCFnriB5L1mGuPzB7pUVXQmn
YWpmmLsprvVNNySy3BDtGqyxKDxqTTaMX0iOKQ1AEt+bE+mqE/+GajPMp89NEqnL
UVGpNBYHMtuO30mf1W/BXXkHa+n9MMrbx0Kx+sZMMNJEjRddFJvVzZExFcIzw6un
2PBvgd0kWUOgTspjIPHpBVnuOYmp6I2+g7G2NfPf5NXg5e8ilp5OIqvwbvlrtsLa
PTuL0dTl0RFO5wsyAokn8EUjfzJhfz+8xEUo28CjO9Ku8JsOfCJN2stzSmK6stJl
-----END CERTIFICATE-----
-----BEGIN CERTIFICATE-----
MIIEeTCCAuGgAwIBAgIUEV77hRsKEOh5u65RVaQNvW+gXW0wDQYJKoZIhvcNAQEM
BQAwTDELMAkGA1UEBhMCVVMxCzAJBgNVBAgTAkNBMRYwFAYDVQQHEw1TYW4gRnJh
bmNpc2NvMRgwFgYDVQQDEw9zZ3gtY29vcmRpbmF0b3IwHhcNMjIwOTI4MDcxMDAw
WhcNMzIwOTI1MDcxMDAwWjBMMQswCQYDVQQGEwJVUzELMAkGA1UECBMCQ0ExFjAU
BgNVBAcTDVNhbiBGcmFuY2lzY28xGDAWBgNVBAMTD3NneC1jb29yZGluYXRvcjCC
AaIwDQYJKoZIhvcNAQEBBQADggGPADCCAYoCggGBANwjjvzxyUBNuQYDuboFDgFu
qtOCkuCK+JZd6+ITzaI473YCNP8SLjL0nJFV//ofzUl+IvErSZT55E97DKi1I4gu
tJK72eQfEbgd6BFJ+kHqu3uAKbjNGyrAOs6MgcKZNzINYSlA5fk9c4oX1nV/4sOc
8fx4232pjeRnUwiDc0ZSF/RBNOErnUHbYdHBoVhDXjMLb2JZGsmPFD6FapFqOJCF
3rfXEUOlkOzsdjbUXnTjXVLKv3u6yqOvetJhGVdq9/iLLnz6U4gTtcuUOimWS9eP
ArWYR883vHPsctBfaqsBkv4HcAQTvhQrS4FdhF/DKjw61kFfIVjZlsZbLZvIAqbT
HhqFxebUPMMXIRSxuaxXiQbxesZXsHjkoaOW8Xly2dlOdW57FPCzxHhivggSYMzf
7daIqJ0E9Jl2OIHCZieVo5KGsjDmR6gSp4MVqf7wYhvucPzcZqNHaVuOH23BrlQ4
k/UovQ9IRobES4i5pCJifS65DBcib4ryPX+KNOZJcQIDAQABo1MwUTAOBgNVHQ8B
Af8EBAMCAQYwDwYDVR0TAQH/BAUwAwEB/zAdBgNVHQ4EFgQUj1IXixXdVSBivOfF
LkmKzHuNvlQwDwYDVR0RBAgwBocEfwAAATANBgkqhkiG9w0BAQwFAAOCAYEAIUrE
O/Q13nDHE12zl1pnY1smqBRRAIpHpIJPRNJvAnbi5REMk1JisJepTZRq5dbuZK0m
PNEjCIagl9mmnO73dEyCaEOz7OQOaQ9yPTpwAk9DkXuNGX2BzhLYqzH7apeLyEyD
SEaIEHyhcPUAkmjWqxWLrgM0dL5LmXR0yKLuzbw6sDKfWWQFQRg1wOqvJs1B/oE0
xXc/NNXJu2BhU+VTPhGqa/Vvd7nCkr4aVSiVr8q7dWM3GKAA4ZvxLoRv0NJyETmn
WQjpFVscMRBKZp/QbpaGPv71K8ZyqxvO8GTMS6g5t5s7O5ZgJeafxftgVeFZC+6o
4cOdHScy5GiqDvuHfybhQ7B/9U7XNvrPXuA9zhghO7FB5axp8KdXslhFc2rMUHC6
689h6LJZOpVsoUN+8qpzvcGOjlM/m4IIppnq2jKAx8aSCf05B/1yLn+KIa81wYap
emCoppSZz2o5Go8jmqJYBJJEv0lst+cGTuUErhx08DoADfUveAQkgzVdE9/z
-----END CERTIFICATE-----
`
)

// TestTLSPrivatekey verifies private key conversions. NewRSAPrikey and NewECDSAPrikey reject an
// unsupported size and curve. For every key from testAsymmetricPrikeys, DER and PEM forms
// round-trip through Prikey2Der, Prikey2Pem, Pem2Der, Pem2Ders, PrikeyDer2Pem, Pem2Prikey and
// Der2Prikey (plus RSADer2Prikey and RSAPem2Prikey for PKCS#1 RSA keys), PEM output ends with a
// newline, and a certificate created from the key round-trips through Cert2Pem and Pem2Cert.
func TestTLSPrivatekey(t *testing.T) {
	t.Parallel()
	t.Run("err", func(t *testing.T) {
		_, err := NewRSAPrikey(RSAPrikeyBits(123))
		require.Error(t, err)

		_, err = NewECDSAPrikey(ECDSACurve("123"))
		require.Error(t, err)
	})

	for _, prikey := range testAsymmetricPrikeys(t) {
		if rsaPrikey, ok := prikey.(*rsa.PrivateKey); ok {
			prider := x509.MarshalPKCS1PrivateKey(rsaPrikey)
			pripem := PrikeyDer2Pem(prider)
			prider2, err := Pem2Der(pripem)
			require.NoError(t, err)
			require.Equal(t, prider, prider2)
			key2, err := RSADer2Prikey(prider)
			require.NoError(t, err)
			require.True(t, rsaPrikey.Equal(key2))
			key2, err = RSAPem2Prikey(pripem)
			require.NoError(t, err)
			require.True(t, rsaPrikey.Equal(key2))
		}

		der, err := Prikey2Der(prikey)
		require.NoError(t, err)

		pem, err := Prikey2Pem(prikey)
		require.NoError(t, err)
		require.Equal(t, "\n", string(pem[len(pem)-1]))

		_, err = Pem2Der(append(pem, '\n'))
		require.NoError(t, err)

		der2, err := Pem2Der(pem)
		require.NoError(t, err)
		require.Equal(t, pem, PrikeyDer2Pem(der2))
		require.Equal(t, der, der2)
		der22, err := Pem2Der(pem)
		require.NoError(t, err)
		require.Equal(t, der, der22)

		ders, err := Pem2Ders(pem)
		require.NoError(t, err)
		require.Equal(t, pem, PrikeyDer2Pem(ders[0]))
		require.Equal(t, der, der2)

		prikey, err = Pem2Prikey(pem)
		require.NoError(t, err)
		der2, err = Prikey2Der(prikey)
		require.NoError(t, err)
		require.Equal(t, der, der2)

		prikey, err = Der2Prikey(der)
		require.NoError(t, err)
		der2, err = Prikey2Der(prikey)
		require.NoError(t, err)
		require.Equal(t, der, der2)

		require.NotNil(t, Prikey2Pubkey(prikey))

		t.Run("cert", func(t *testing.T) {
			der, err := NewX509Cert(prikey,
				WithX509CertCommonName("laisky"),
				WithX509CertSANS("laisky"),
				WithX509CertIsCA(),
				WithX509CertOrganization("laisky"),
				WithX509CertValidFrom(time.Now()),
				WithX509CertValidFor(time.Second),
			)
			require.NoError(t, err)

			cert, err := Der2Cert(der)
			require.NoError(t, err)

			pem := Cert2Pem(cert)
			require.Equal(t, "\n", string(pem[len(pem)-1]))
			cert, err = Pem2Cert(pem)
			require.NoError(t, err)
			require.Equal(t, der, Cert2Der(cert))
		})
	}
}

// testAsymmetricPrikeys generates one private key of each supported type for table-driven tests:
// RSA-2048, RSA-3072, ECDSA P-256, P-384 and P-521, and Ed25519. The t parameter is used to fail
// the calling test if key generation fails. It returns the keys indexed by a short algorithm name
// such as "rsa2048" or "es256".
func testAsymmetricPrikeys(t *testing.T) (prikeys map[string]crypto.PrivateKey) {
	t.Helper()

	rsa2048, err := NewRSAPrikey(RSAPrikeyBits2048)
	require.NoError(t, err)
	rsa3072, err := NewRSAPrikey(RSAPrikeyBits3072)
	require.NoError(t, err)
	es256, err := NewECDSAPrikey(ECDSACurveP256)
	require.NoError(t, err)
	es384, err := NewECDSAPrikey(ECDSACurveP384)
	require.NoError(t, err)
	es521, err := NewECDSAPrikey(ECDSACurveP521)
	require.NoError(t, err)
	edkey, err := NewEd25519Prikey()
	require.NoError(t, err)

	return map[string]crypto.PrivateKey{
		"rsa2048": rsa2048,
		"rsa3072": rsa3072,
		"es256":   es256,
		"es384":   es384,
		"es521":   es521,
		"ed25519": edkey,
	}
}

// TestTLSPublickey verifies that Pubkey2Der rejects a nil key and that, for the public key of each
// key from testAsymmetricPrikeys, DER and PEM forms round-trip through Pubkey2Der, Pubkey2Pem,
// Pem2Der, PubkeyDer2Pem, Pem2Pubkey and Der2Pubkey, with PEM output ending in a newline.
func TestTLSPublickey(t *testing.T) {
	t.Parallel()

	_, err := Pubkey2Der(nil)
	require.Error(t, err)

	for _, prikey := range testAsymmetricPrikeys(t) {
		pubkey := Prikey2Pubkey(prikey)

		require.NotNil(t, pubkey)
		der, err := Pubkey2Der(pubkey)
		require.NoError(t, err)

		pem, err := Pubkey2Pem(pubkey)
		require.NoError(t, err)
		require.Equal(t, "\n", string(pem[len(pem)-1]))

		der2, err := Pem2Der(pem)
		require.NoError(t, err)
		require.Equal(t, pem, PubkeyDer2Pem(der2))
		require.Equal(t, der, der2)
		der22, err := Pem2Der(pem)
		require.NoError(t, err)
		require.Equal(t, der, der22)

		pubkey, err = Pem2Pubkey(pem)
		require.NoError(t, err)
		der2, err = Pubkey2Der(pubkey)
		require.NoError(t, err)
		require.Equal(t, der, der2)

		pubkey, err = Der2Pubkey(der)
		require.NoError(t, err)
		der2, err = Pubkey2Der(pubkey)
		require.NoError(t, err)
		require.Equal(t, der, der2)
	}
}

// TestPem2Der_multi_certs verifies that Pem2Der concatenates the DER of a two-certificate PEM
// chain, that Der2Certs parses it back in order, and that Cert2Der and Cert2Pem over the parsed
// certificates reproduce the same DER.
func TestPem2Der_multi_certs(t *testing.T) {
	t.Parallel()

	der, err := Pem2Der([]byte(testCertChain))
	require.NoError(t, err)
	cs, err := Der2Certs(der)
	require.NoError(t, err)

	require.Equal(t, "sgx-coordinator-inter", cs[0].Subject.CommonName)
	require.Equal(t, "sgx-coordinator", cs[1].Subject.CommonName)

	gotder := Cert2Der(cs...)
	require.Equal(t, der, gotder)

	gotder, err = Pem2Der(Cert2Pem(cs...))
	require.NoError(t, err)
	require.Equal(t, der, gotder)
}

// TestSecureCipherSuites verifies that SecureCipherSuites returns every secure suite when the
// filter is nil or always true, and none when the filter always returns false.
func TestSecureCipherSuites(t *testing.T) {
	t.Parallel()

	raw := SecureCipherSuites(nil)
	filtered := SecureCipherSuites(func(cs *tls.CipherSuite) bool {
		return true
	})
	require.Equal(t, len(raw), len(filtered))

	filtered = SecureCipherSuites(func(cs *tls.CipherSuite) bool {
		return false
	})
	require.Zero(t, len(filtered))
}

// TestVerifyCertByPrikey verifies that VerifyCertByPrikey accepts a PEM certificate together with
// its own PEM private key, that CertDer2Pem output ends with a newline, and that a certificate
// generated for a different key is rejected.
func TestVerifyCertByPrikey(t *testing.T) {
	t.Parallel()

	prikey, certDer, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
		WithX509CertCommonName("TestVerifyCertByPrikey"),
	)
	require.NoError(t, err)

	certPem := CertDer2Pem(certDer)
	require.Equal(t, "\n", string(certPem[len(certPem)-1]))

	err = VerifyCertByPrikey(certPem, prikey)
	require.NoError(t, err)

	t.Run("different cert", func(t *testing.T) {
		_, certDer2, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
			WithX509CertCommonName("laisky-test"),
		)
		require.NoError(t, err)
		certPem2 := CertDer2Pem(certDer2)
		err = VerifyCertByPrikey(certPem2, prikey)
		require.Error(t, err)
	})
}

// TestDer2CSR verifies that CSRs created by NewX509CSR for each key from testAsymmetricPrikeys
// parse identically from DER (Der2CSR) and from PEM (CSRDer2Pem and Pem2CSR), and that an
// OpenSSL-generated CSR with an empty subject parses successfully.
func TestDer2CSR(t *testing.T) {
	t.Parallel()

	for algo, prikey := range testAsymmetricPrikeys(t) {
		t.Logf("test algo: %v", algo)
		csrDer, err := NewX509CSR(prikey,
			WithX509CSRCommonName("laisky"),
		)
		require.NoError(t, err)

		csr, err := Der2CSR(csrDer)
		require.NoError(t, err)

		pem := CSRDer2Pem(csrDer)
		require.Equal(t, "\n", string(pem[len(pem)-1]))

		csr2, err := Pem2CSR(pem)
		require.NoError(t, err)

		require.Equal(t, csr, csr2)
	}

	t.Run("parse openssl generated csr", func(t *testing.T) {
		csrder, err := base64.StdEncoding.DecodeString("MIICRTCCAS0CAQAwADCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBALx3QQ3rtGhaiuIDr8ZlifeN4iZ7X2Cc5DH5BnkYRbFohDFntG7hEEhg+Oci04i/bxH+x8cN9rTNhkczXxDdjQlsUy4gVrBEM0CK2VMfX5nbb2lRHFz5v7Q32vfFEYjwivpgl7zDspbxKYQGnfOrMt8vyYNJbD1lqa3XVcv1gzdwsCnAMBU9uRCr4MFT4IdXZDS4TCSNPCL3EpII3NOhVGqriTeSfxTpmvD2F3cHOSK34w96v3n6B1Ss5I8GGCkW96VRxPlHTFt52XuQ2O054o2AfReqIrDrM7fnWWS0cH4b2xg/PeHhyz//mEOXeHMkswFddBz3m/zrGK4VmvKEZqkCAwEAAaAAMA0GCSqGSIb3DQEBCwUAA4IBAQAcKp1P04R/P8728IlQuOSlrOBgeXgWRgpVN7K260R81MdTnjEuv9qhYbUy3pPyY8XN22/FIgk40jkrg5pxYKFcZ7pvCorDvlpW3nrl/UCgikhIFTfif5QpfT9vov0jHWB4jgMKbE22Nt/T7X8eHA914y/M1YB1KYkYt++1HlzlCj+FRakb1IRy9tAGj5uPW3KNRZ9iHd/Q9MTrm+cCKBJukBAtvJPiTg0oB3mFacYfyYExaIknYtlhgB1s4rzXIzggvGbqqH8vFc0djOSF3NUjqb9gNDGESzznnmENayzRXjABmzYKVA1COIi2dn9DWsLNlQJIy+xhZR6LbVkO7eOW")
		require.NoError(t, err)

		csr, err := Der2CSR(csrder)
		require.NoError(t, err)
		require.Equal(t, "", csr.Subject.CommonName)
	})
}

// TestSplitCertsPemChain verifies that SplitCertsPemChain splits a PEM chain into one trimmed block
// per certificate for single and multiple certificates, and returns nil for an empty chain.
func TestSplitCertsPemChain(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		pemChain string
		expected []string
	}{
		{
			name:     "Single Certificate",
			pemChain: "-----BEGIN CERTIFICATE-----\nCERT1\n-----END CERTIFICATE-----",
			expected: []string{"-----BEGIN CERTIFICATE-----\nCERT1\n-----END CERTIFICATE-----"},
		},
		{
			name: "Multiple Certificates",
			pemChain: `-----BEGIN CERTIFICATE-----
CERT1
-----END CERTIFICATE-----
-----BEGIN CERTIFICATE-----
CERT2
-----END CERTIFICATE-----
-----BEGIN CERTIFICATE-----
CERT3
-----END CERTIFICATE-----
`,
			expected: []string{
				"-----BEGIN CERTIFICATE-----\nCERT1\n-----END CERTIFICATE-----",
				"-----BEGIN CERTIFICATE-----\nCERT2\n-----END CERTIFICATE-----",
				"-----BEGIN CERTIFICATE-----\nCERT3\n-----END CERTIFICATE-----",
			},
		},
		{
			name:     "Empty Chain",
			pemChain: "",
			expected: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := SplitCertsPemChain(tc.pemChain)
			require.Equal(t, tc.expected, got)
		})
	}
}
