package crypto

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// csrSignatureTestAlgorithm describes a supported CSR signing algorithm.
type csrSignatureTestAlgorithm struct {
	name string
	algo x509.SignatureAlgorithm
}

// csrSignatureTestAlgorithms returns the algorithms covered by issuance tests.
func csrSignatureTestAlgorithms() []csrSignatureTestAlgorithm {
	return []csrSignatureTestAlgorithm{
		{"RSA_PKCS1", x509.SHA256WithRSA},
		{"RSA_PSS", x509.SHA256WithRSAPSS},
		{"ECDSA_P256", x509.ECDSAWithSHA256},
		{"ECDSA_P384", x509.ECDSAWithSHA384},
		{"ECDSA_P521", x509.ECDSAWithSHA512},
		{"Ed25519", x509.PureEd25519},
	}
}

// newCSRSignatureTestKey generates a private key for algo and fails t on error.
func newCSRSignatureTestKey(t *testing.T, algo x509.SignatureAlgorithm) crypto.Signer {
	t.Helper()

	switch algo {
	case x509.SHA1WithRSA, x509.SHA256WithRSA, x509.SHA256WithRSAPSS:
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		return key
	case x509.PureEd25519:
		_, key, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		return key
	default:
		curves := map[x509.SignatureAlgorithm]elliptic.Curve{
			x509.ECDSAWithSHA256: elliptic.P256(),
			x509.ECDSAWithSHA384: elliptic.P384(),
			x509.ECDSAWithSHA512: elliptic.P521(),
		}
		curve, ok := curves[algo]
		require.True(t, ok, "unsupported test algorithm: %v", algo)
		key, err := ecdsa.GenerateKey(curve, rand.Reader)
		require.NoError(t, err)
		return key
	}
}

// newCSRSignatureTestCA creates a self-signed issuer for key and fails t on error.
func newCSRSignatureTestCA(t *testing.T, key crypto.Signer) *x509.Certificate {
	t.Helper()

	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "CSR regression CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	require.NoError(t, err)
	ca, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return ca
}

// newCSRSignatureTestRequest signs a request with every supported SAN type.
func newCSRSignatureTestRequest(t *testing.T, key crypto.Signer, algo x509.SignatureAlgorithm) []byte {
	t.Helper()

	uri, err := url.Parse("spiffe://example.test/original")
	require.NoError(t, err)
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		SignatureAlgorithm: algo,
		Subject: pkix.Name{
			CommonName:   "original-subject",
			Organization: []string{"CSR regression"},
		},
		DNSNames:       []string{"original.example"},
		EmailAddresses: []string{"original@example.test"},
		IPAddresses:    []net.IP{net.IPv4(192, 0, 2, 10)},
		URIs:           []*url.URL{uri},
	}, key)
	require.NoError(t, err)
	return der
}

// replaceCSRSignatureTestBytes replaces exactly one equal-length DER field.
func replaceCSRSignatureTestBytes(t *testing.T, der, old, replacement []byte) []byte {
	t.Helper()

	require.Len(t, replacement, len(old))
	require.Equal(t, 1, bytes.Count(der, old), "fixture must identify exactly one field")
	return bytes.Replace(der, old, replacement, 1)
}

// requireCSRSignatureTestCertificate checks the issuer signature and copied fields.
func requireCSRSignatureTestCertificate(t *testing.T, der []byte, ca *x509.Certificate, csr *x509.CertificateRequest) {
	t.Helper()

	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	require.NoError(t, cert.CheckSignatureFrom(ca))
	require.Equal(t, csr.Subject, cert.Subject)
	require.Equal(t, csr.RawSubjectPublicKeyInfo, cert.RawSubjectPublicKeyInfo)
	require.Equal(t, csr.DNSNames, cert.DNSNames)
	require.Equal(t, csr.EmailAddresses, cert.EmailAddresses)
	require.Equal(t, csr.IPAddresses, cert.IPAddresses)
	require.Equal(t, csr.URIs, cert.URIs)

	roots := x509.NewCertPool()
	roots.AddCert(ca)
	_, err = cert.Verify(x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	})
	require.NoError(t, err)
}

// TestNewX509CertByCSRRejectsTampering proves invalid but parseable requests cannot issue certificates.
func TestNewX509CertByCSRRejectsTampering(t *testing.T) {
	t.Parallel()

	caKey := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, caKey)
	for _, algorithm := range csrSignatureTestAlgorithms() {
		t.Run(algorithm.name, func(t *testing.T) {
			t.Parallel()

			key := newCSRSignatureTestKey(t, algorithm.algo)
			der := newCSRSignatureTestRequest(t, key, algorithm.algo)
			csr, err := Der2CSR(der)
			require.NoError(t, err)
			require.NoError(t, csr.CheckSignature())

			replacementKey := newCSRSignatureTestKey(t, algorithm.algo)
			replacementSPKI, err := x509.MarshalPKIXPublicKey(replacementKey.Public())
			require.NoError(t, err)
			badSignature := bytes.Clone(csr.Signature)
			badSignature[len(badSignature)-1] ^= 1

			var san []byte
			for _, extension := range csr.Extensions {
				if extension.Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 17}) {
					san = extension.Value
				}
			}
			require.NotEmpty(t, san)

			cases := []struct {
				name        string
				old         []byte
				replacement []byte
			}{
				{"subject", csr.RawSubject, replaceCSRSignatureTestBytes(t, csr.RawSubject, []byte("original-subject"), []byte("tampered-subject"))},
				{"DNS_SAN", san, replaceCSRSignatureTestBytes(t, san, []byte("original.example"), []byte("tampered.example"))},
				{"email_SAN", san, replaceCSRSignatureTestBytes(t, san, []byte("original@example.test"), []byte("tampered@example.test"))},
				{"IP_SAN", san, replaceCSRSignatureTestBytes(t, san, []byte{192, 0, 2, 10}, []byte{192, 0, 2, 11})},
				{"URI_SAN", san, replaceCSRSignatureTestBytes(t, san, []byte("spiffe://example.test/original"), []byte("spiffe://example.test/tampered"))},
				{"public_key", csr.RawSubjectPublicKeyInfo, replacementSPKI},
				{"signature", csr.Signature, badSignature},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					tamperedDER := replaceCSRSignatureTestBytes(t, der, tc.old, tc.replacement)
					// Parsing must succeed: a DER syntax error would not reproduce this vulnerability.
					tamperedCSR, err := Der2CSR(tamperedDER)
					require.NoError(t, err)
					require.Error(t, tamperedCSR.CheckSignature())

					optionCalled := false
					certDER, err := NewX509CertByCSR(ca, caKey, tamperedDER, func(_ *signCSROption) error {
						optionCalled = true
						return nil
					})
					if len(certDER) > 0 {
						// On the vulnerable revision, verify actual CA issuance, not just a missing error.
						requireCSRSignatureTestCertificate(t, certDER, ca, tamperedCSR)
						t.Log("vulnerable behavior: CA-signed certificate issued from a CSR with an invalid signature")
					}
					require.ErrorContains(t, err, "verify csr signature")
					require.Nil(t, certDER)
					require.False(t, optionCalled, "unverified requests must not reach signing options")
				})
			}
		})
	}
}

// TestNewX509CertByCSRValidSignatures checks valid requests and cross-algorithm issuers.
func TestNewX509CertByCSRValidSignatures(t *testing.T) {
	t.Parallel()

	issuers := []csrSignatureTestAlgorithm{
		{"RSA", x509.SHA256WithRSA},
		{"ECDSA", x509.ECDSAWithSHA256},
		{"Ed25519", x509.PureEd25519},
	}
	for _, issuer := range issuers {
		t.Run(issuer.name, func(t *testing.T) {
			t.Parallel()

			caKey := newCSRSignatureTestKey(t, issuer.algo)
			ca := newCSRSignatureTestCA(t, caKey)
			for _, algorithm := range csrSignatureTestAlgorithms() {
				t.Run(algorithm.name, func(t *testing.T) {
					t.Parallel()

					key := newCSRSignatureTestKey(t, algorithm.algo)
					der := newCSRSignatureTestRequest(t, key, algorithm.algo)
					csr, err := Der2CSR(der)
					require.NoError(t, err)
					require.NoError(t, csr.CheckSignature())

					optionCalled := false
					certDER, err := NewX509CertByCSR(ca, caKey, der, func(_ *signCSROption) error {
						optionCalled = true
						return nil
					})
					require.NoError(t, err)
					require.True(t, optionCalled)
					requireCSRSignatureTestCertificate(t, certDER, ca, csr)
				})
			}
		})
	}
}

// TestNewX509CertByCSRMalformedRequests retains parse errors before signing options.
func TestNewX509CertByCSRMalformedRequests(t *testing.T) {
	t.Parallel()

	caKey := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, caKey)
	for name, der := range map[string][]byte{
		"nil":       nil,
		"empty":     {},
		"truncated": {0x30, 0x82, 0x01},
	} {
		t.Run(name, func(t *testing.T) {
			optionCalled := false
			certDER, err := NewX509CertByCSR(ca, caKey, der, func(_ *signCSROption) error {
				optionCalled = true
				return nil
			})
			require.ErrorContains(t, err, "parse csr")
			require.Nil(t, certDER)
			require.False(t, optionCalled)
		})
	}
}

// csrSignatureTestWireRequest preserves signed request bytes while changing the algorithm identifier.
type csrSignatureTestWireRequest struct {
	Info      asn1.RawValue
	Algorithm pkix.AlgorithmIdentifier
	Signature asn1.BitString
}

// TestNewX509CertByCSRUnsupportedSignature preserves the underlying verification error.
func TestNewX509CertByCSRUnsupportedSignature(t *testing.T) {
	t.Parallel()

	caKey := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, caKey)
	der := newCSRSignatureTestRequest(t, caKey, x509.ECDSAWithSHA256)
	var wire csrSignatureTestWireRequest
	rest, err := asn1.Unmarshal(der, &wire)
	require.NoError(t, err)
	require.Empty(t, rest)
	wire.Algorithm.Algorithm = asn1.ObjectIdentifier{1, 2, 3, 4}
	der, err = asn1.Marshal(wire)
	require.NoError(t, err)
	csr, err := Der2CSR(der)
	require.NoError(t, err)
	require.ErrorIs(t, csr.CheckSignature(), x509.ErrUnsupportedAlgorithm)

	optionCalled := false
	certDER, err := NewX509CertByCSR(ca, caKey, der, func(_ *signCSROption) error {
		optionCalled = true
		return nil
	})
	require.ErrorContains(t, err, "verify csr signature")
	require.ErrorIs(t, err, x509.ErrUnsupportedAlgorithm)
	require.Nil(t, certDER)
	require.False(t, optionCalled)
}
