package crypto

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	gutils "github.com/Laisky/go-utils/v4"
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
}

func Test_UseCaAsClientTlsCert(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rootprikeyPem, rootcaDer, err := NewRSAPrikeyAndCert(RSAPrikeyBits4096,
		WithX509CertCommonName("laisky-test"),
		WithX509CertIsCA(),
	)
	require.NoError(t, err)

	rootcaPrikey, err := Pem2Prikey(rootprikeyPem)
	require.NoError(t, err)

	rootca, err := Der2Cert(rootcaDer)
	require.NoError(t, err)

	rootcapool := x509.NewCertPool()
	rootcapool.AppendCertsFromPEM(CertDer2Pem(rootcaDer))

	gt := gutils.NewGoroutineTest(t, cancel)
	go func(t testing.TB) {
		prikey, err := NewRSAPrikey(RSAPrikeyBits4096)
		require.NoError(t, err)

		csrDer, err := NewX509CSR(prikey, WithX509CSRCommonName("laisky-test"))
		require.NoError(t, err)

		certDer, err := NewX509CertByCSR(rootca, rootcaPrikey, csrDer)
		require.NoError(t, err)

		ln, err := tls.Listen("tcp", "localhost:38443", &tls.Config{
			RootCAs:    rootcapool,
			ClientAuth: tls.RequireAndVerifyClientCert,
			Certificates: []tls.Certificate{
				{
					Certificate: [][]byte{certDer, rootcaDer},
					PrivateKey:  prikey,
				},
			},
		})
		require.NoError(t, err)

		for {
			conn, err := ln.Accept()
			require.NoError(t, err)

			go func() {
				buf := make([]byte, 4096)
				for {
					defer conn.Close()

					n, err := conn.Read(buf)
					if err != nil {
						t.Logf("failed to read: %v", err)
						break
					}

					if bytes.Equal(buf, []byte("close")) {
						t.Logf("close connection")
						break
					}

					_, err = conn.Write(buf[:n])
					if err != nil {
						t.Logf("failed to write: %v", err)
						break
					}
				}
			}()
		}
	}(gt)

	require.NoError(t, gutils.WaitTCPOpen(ctx, "localhost", 38443))

	t.Run("use ca as client tls cert", func(t *testing.T) {
		prikey, err := NewRSAPrikey(RSAPrikeyBits4096)
		require.NoError(t, err)

		csrDer, err := NewX509CSR(prikey, WithX509CSRCommonName("laisky-test"))
		require.NoError(t, err)

		certDer, err := NewX509CertByCSR(rootca, rootcaPrikey, csrDer,
			WithX509SignCSRIsCA(),
		)
		require.NoError(t, err)

		conn, err := tls.Dial("tcp", "localhost:38443", &tls.Config{
			RootCAs:            rootcapool,
			InsecureSkipVerify: true,
			Certificates: []tls.Certificate{
				{
					Certificate: [][]byte{certDer, rootcaDer},
					PrivateKey:  prikey,
				},
			},
		})
		require.NoError(t, err)
		defer conn.Close()

		_, err = conn.Write([]byte("hello"))
		require.NoError(t, err)
	})
}

func Test_UseCaAsServerTlsCert(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rootprikeyPem, rootcaDer, err := NewRSAPrikeyAndCert(RSAPrikeyBits4096,
		WithX509CertCommonName("laisky-test"),
		WithX509CertIsCA(),
	)
	require.NoError(t, err)

	rootcaPrikey, err := Pem2Prikey(rootprikeyPem)
	require.NoError(t, err)

	rootca, err := Der2Cert(rootcaDer)
	require.NoError(t, err)

	rootcapool := x509.NewCertPool()
	rootcapool.AppendCertsFromPEM(CertDer2Pem(rootcaDer))

	gt := gutils.NewGoroutineTest(t, cancel)
	go func(t testing.TB) {
		prikey, err := NewRSAPrikey(RSAPrikeyBits4096)
		require.NoError(t, err)

		csrDer, err := NewX509CSR(prikey, WithX509CSRCommonName("laisky-test"))
		require.NoError(t, err)

		certDer, err := NewX509CertByCSR(rootca, rootcaPrikey, csrDer,
			WithX509SignCSRIsCA(),
		)
		require.NoError(t, err)

		// cert, err := Der2Cert(certDer)
		// require.NoError(t, err)
		// t.Logf("cert: %+v", cert)

		ln, err := tls.Listen("tcp", "localhost:38444", &tls.Config{
			RootCAs:    rootcapool,
			ClientAuth: tls.RequireAndVerifyClientCert,
			Certificates: []tls.Certificate{
				{
					Certificate: [][]byte{certDer, rootcaDer},
					PrivateKey:  prikey,
				},
			},
		})
		require.NoError(t, err)

		for {
			conn, err := ln.Accept()
			require.NoError(t, err)

			go func() {
				buf := make([]byte, 4096)
				for {
					defer conn.Close()

					n, err := conn.Read(buf)
					if err != nil {
						t.Logf("failed to read: %v", err)
						break
					}

					if bytes.Equal(buf, []byte("close")) {
						t.Logf("close connection")
						break
					}

					_, err = conn.Write(buf[:n])
					if err != nil {
						t.Logf("failed to write: %v", err)
						break
					}
				}
			}()
		}
	}(gt)

	require.NoError(t, gutils.WaitTCPOpen(ctx, "localhost", 38444))

	t.Run("use leaf cert as client tls cert", func(t *testing.T) {
		prikey, err := NewRSAPrikey(RSAPrikeyBits4096)
		require.NoError(t, err)

		csrDer, err := NewX509CSR(prikey, WithX509CSRCommonName("laisky-test"))
		require.NoError(t, err)

		certDer, err := NewX509CertByCSR(rootca, rootcaPrikey, csrDer)
		require.NoError(t, err)

		conn, err := tls.Dial("tcp", "localhost:38444", &tls.Config{
			RootCAs:            rootcapool,
			InsecureSkipVerify: true,
			Certificates: []tls.Certificate{
				{
					Certificate: [][]byte{certDer, rootcaDer},
					PrivateKey:  prikey,
				},
			},
		})
		require.NoError(t, err)
		defer conn.Close()

		peercerts := conn.ConnectionState().PeerCertificates
		t.Log(peercerts)

		_, err = conn.Write([]byte("hello"))
		require.NoError(t, err)
	})
}

func TestX509Cert2OpensslConf(t *testing.T) {
	t.Parallel()

	t.Run("ca", func(t *testing.T) {
		t.Parallel()

		cert := &x509.Certificate{
			Subject: pkix.Name{
				CommonName:         "example.com",
				Province:           []string{"California"},
				Locality:           []string{"San Francisco"},
				Organization:       []string{"Acme Corp"},
				OrganizationalUnit: []string{"IT"},
			},
			IsCA:              true,
			PolicyIdentifiers: []asn1.ObjectIdentifier{[]int{2, 5, 29, 32}},
			DNSNames: []string{
				"localhost",
				"example.com",
			},
			IPAddresses: []net.IP{
				net.ParseIP("1.2.3.4"),
			},
		}

		expected := gutils.Dedent(`
			[ req ]
			distinguished_name = req_distinguished_name
			prompt = no
			string_mask = utf8only
			x509_extensions = v3_ca
			req_extensions = req_ext

			[ req_distinguished_name ]
			commonName = example.com
			stateOrProvinceName = California
			localityName = San Francisco
			organizationName = Acme Corp
			organizationalUnitName = IT

			[ v3_ca ]
			basicConstraints = critical, CA:TRUE
			keyUsage = cRLSign, keyCertSign
			subjectKeyIdentifier = hash
			authorityKeyIdentifier = keyid:always, issuer
			certificatePolicies = @policy-0

			[ policy-0 ]
			policyIdentifier = 2.5.29.32

			[ req_ext ]
			subjectAltName = @alt_names

			[ alt_names ]
			DNS.1 = localhost
			DNS.2 = example.com
			IP.1 = 1.2.3.4
			`)

		expected += "\n"

		opensslConf := X509Cert2OpensslConf(cert)
		t.Logf("got\n%s", string(opensslConf))
		require.Equal(t, expected, string(opensslConf))
	})

	t.Run("not ca", func(t *testing.T) {
		t.Parallel()

		cert := &x509.Certificate{
			Subject: pkix.Name{
				CommonName:         "example.com",
				Country:            []string{"US"},
				Province:           []string{"California"},
				Locality:           []string{"San Francisco"},
				Organization:       []string{"Acme Corp"},
				OrganizationalUnit: []string{"IT"},
			},
			IsCA: false,
			PolicyIdentifiers: []asn1.ObjectIdentifier{
				[]int{2, 5, 29, 32},
				[]int{1, 2, 3},
			},
			DNSNames: []string{
				"localhost",
				"example.com",
			},
			IPAddresses: []net.IP{
				net.ParseIP("1.2.3.4"),
			},
		}

		expected := gutils.Dedent(`
			[ req ]
			distinguished_name = req_distinguished_name
			prompt = no
			string_mask = utf8only
			x509_extensions = v3_ca
			req_extensions = req_ext

			[ req_distinguished_name ]
			commonName = example.com
			countryName = US
			stateOrProvinceName = California
			localityName = San Francisco
			organizationName = Acme Corp
			organizationalUnitName = IT

			[ v3_ca ]
			basicConstraints = critical, CA:FALSE
			keyUsage = digitalSignature, nonRepudiation, keyEncipherment, dataEncipherment, keyAgreement
			extendedKeyUsage = anyExtendedKeyUsage
			subjectKeyIdentifier = hash
			authorityKeyIdentifier = keyid:always, issuer
			certificatePolicies = @policy-0, @policy-1

			[ policy-0 ]
			policyIdentifier = 2.5.29.32
			[ policy-1 ]
			policyIdentifier = 1.2.3

			[ req_ext ]
			subjectAltName = @alt_names

			[ alt_names ]
			DNS.1 = localhost
			DNS.2 = example.com
			IP.1 = 1.2.3.4
			`)
		expected += "\n"

		opensslConf := X509Cert2OpensslConf(cert)
		t.Logf("got\n%s", string(opensslConf))
		require.Equal(t, expected, string(opensslConf))
	})
}

func TestX509Csr2OpensslConf(t *testing.T) {
	csr := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:         "example.com",
			Country:            []string{"US"},
			Province:           []string{"California"},
			Locality:           []string{"San Francisco"},
			Organization:       []string{"Acme Corp"},
			OrganizationalUnit: []string{"IT"},
		},
		DNSNames: []string{
			"localhost",
			"example.com",
		},
		IPAddresses: []net.IP{
			net.ParseIP("1.2.3.4"),
		},
	}

	expectedConf := gutils.Dedent(`
		[ req ]
		distinguished_name = req_distinguished_name
		prompt = no
		string_mask = utf8only
		req_extensions = req_ext

		[ req_distinguished_name ]
		commonName = example.com
		countryName = US
		stateOrProvinceName = California
		localityName = San Francisco
		organizationName = Acme Corp
		organizationalUnitName = IT

		[ req_ext ]
		subjectAltName = @alt_names

		[ alt_names ]
		DNS.1 = localhost
		DNS.2 = example.com
		IP.1 = 1.2.3.4
		`)
	expectedConf += "\n"

	opensslConf := X509Csr2OpensslConf(csr)
	t.Logf("got\n%s", string(opensslConf))
	require.Equal(t, expectedConf, string(opensslConf))
}

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
-----END CERTIFICATE-----`,
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
