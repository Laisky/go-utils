package crypto

import (
	"crypto/x509"
	"encoding/asn1"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestReadableX509Cert verifies that ReadableX509Cert converts a certificate generated with a full
// set of subject, SAN, usage, validity, CRL, OCSP and policy options into a map whose "subject"
// entry reports the expected common name.
func TestReadableX509Cert(t *testing.T) {
	t.Parallel()

	validFrom := time.Unix(time.Now().Unix(), 0).UTC()
	_, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
		WithX509CertCommonName("laisky"),
		WithX509CertSANS("laisky.com"),
		WithX509CertSignatureAlgorithm(x509.SHA512WithRSA),
		WithX509CertOrganization("laisky-o"),
		WithX509CertOrganizationUnit("laisky-u"),
		WithX509CertLocality("local"),
		WithX509CertCountry("country"),
		WithX509CertProvince("province"),
		WithX509CertStreetAddrs("st-1", "st-2"),
		WithX509CertPostalCode("200233"),
		WithX509CertIsCA(),
		WithX509CertIsCRLCA(),
		WithX509CertSeriaNumber(big.NewInt(489238432420)),
		WithX509CertKeyUsage(x509.KeyUsageCRLSign),
		WithX509CertExtKeyUsage(x509.ExtKeyUsageCodeSigning),
		WithX509CertValidFrom(validFrom),
		WithX509CertValidFor(time.Hour),
		WithX509CertCRLs("crl"),
		WithX509CertOCSPServers("ocsp"),
		WithX509CertPolicies(asn1.ObjectIdentifier{1, 2, 3, 4}),
	)
	require.NoError(t, err)

	cert, err := Der2Cert(certder)
	require.NoError(t, err)

	m, err := ReadableX509Cert(cert)
	require.NoError(t, err)

	require.Equal(t, "laisky", m["subject"].(map[string]any)["common_name"])
}

// Test_ExtKeyUsage verifies how extended key usages set through WithX509CertExtKeyUsage and
// WithX509SignCSRExtKeyUsage affect x509 chain verification for ServerAuth. A certificate created
// without an explicit extended key usage, with ServerAuth, or with one of several requested usages
// verifies; a mismatching usage, or a CA in the chain restricted to CodeSigning, fails with
// "incompatible key usage", while a chain whose CA has no restriction verifies.
func Test_ExtKeyUsage(t *testing.T) {
	t.Parallel()

	t.Run("empty ext key usage", func(t *testing.T) {
		t.Parallel()
		_, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
			WithX509CertCommonName("laisky-test"))
		require.NoError(t, err)

		cert, err := Der2Cert(certder)
		require.NoError(t, err)

		root := x509.NewCertPool()
		root.AddCert(cert)
		_, err = cert.Verify(x509.VerifyOptions{
			Roots:     root,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		})
		require.NoError(t, err)
	})

	t.Run("ext key usage not match", func(t *testing.T) {
		t.Parallel()
		_, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
			WithX509CertCommonName("laisky-test"),
			WithX509CertExtKeyUsage(x509.ExtKeyUsageCodeSigning),
		)
		require.NoError(t, err)

		cert, err := Der2Cert(certder)
		require.NoError(t, err)

		root := x509.NewCertPool()
		root.AddCert(cert)
		_, err = cert.Verify(x509.VerifyOptions{
			Roots:     root,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		})
		require.ErrorContains(t, err, "certificate specifies an incompatible key usage")
	})

	t.Run("ext key usage match", func(t *testing.T) {
		t.Parallel()
		_, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
			WithX509CertCommonName("laisky-test"),
			WithX509CertExtKeyUsage(x509.ExtKeyUsageServerAuth),
		)
		require.NoError(t, err)

		cert, err := Der2Cert(certder)
		require.NoError(t, err)

		root := x509.NewCertPool()
		root.AddCert(cert)
		_, err = cert.Verify(x509.VerifyOptions{
			Roots:     root,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		})
		require.NoError(t, err)
	})

	t.Run("ext key usage match any", func(t *testing.T) {
		t.Parallel()
		_, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
			WithX509CertCommonName("laisky-test"),
			WithX509CertExtKeyUsage(x509.ExtKeyUsageServerAuth),
		)
		require.NoError(t, err)

		cert, err := Der2Cert(certder)
		require.NoError(t, err)

		root := x509.NewCertPool()
		root.AddCert(cert)
		_, err = cert.Verify(x509.VerifyOptions{
			Roots: root,
			KeyUsages: []x509.ExtKeyUsage{
				x509.ExtKeyUsageCodeSigning,
				x509.ExtKeyUsageServerAuth,
			},
		})
		require.NoError(t, err)
	})

	t.Run("not all cert in chain match ext key usage", func(t *testing.T) {
		t.Parallel()
		// new ca
		cakeyPem, caDer, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
			WithX509CertCommonName("laisky-test"),
			WithX509CertIsCA(),
			WithX509CertExtKeyUsage(x509.ExtKeyUsageCodeSigning),
		)
		require.NoError(t, err)
		ca, err := Der2Cert(caDer)
		require.NoError(t, err)
		cakey, err := Pem2Prikey(cakeyPem)
		require.NoError(t, err)

		// new leaf cert
		prikey, err := NewRSAPrikey(RSAPrikeyBits3072)
		require.NoError(t, err)
		csrDer, err := NewX509CSR(prikey, WithX509CSRCommonName("laisky-test"))
		require.NoError(t, err)
		certDer, err := NewX509CertByCSR(ca, cakey, csrDer,
			WithX509SignCSRExtKeyUsage(x509.ExtKeyUsageServerAuth),
		)
		require.NoError(t, err)
		cert, err := Der2Cert(certDer)
		require.NoError(t, err)
		prikeyPem, err := Prikey2Pem(prikey)
		require.NoError(t, err)
		require.NoError(t, VerifyCertByPrikey(CertDer2Pem(certDer), prikeyPem))

		// verify
		root := x509.NewCertPool()
		root.AddCert(ca)
		_, err = cert.Verify(x509.VerifyOptions{
			Roots: root,
			KeyUsages: []x509.ExtKeyUsage{
				x509.ExtKeyUsageServerAuth,
			},
		})
		require.ErrorContains(t, err, "certificate specifies an incompatible key usage")
	})

	t.Run("all cert in chain match ext key usage", func(t *testing.T) {
		t.Parallel()
		// new ca
		cakeyPem, caDer, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
			WithX509CertCommonName("laisky-test"),
			WithX509CertIsCA(),
		)
		require.NoError(t, err)
		ca, err := Der2Cert(caDer)
		require.NoError(t, err)
		cakey, err := Pem2Prikey(cakeyPem)
		require.NoError(t, err)

		// new leaf cert
		prikey, err := NewRSAPrikey(RSAPrikeyBits3072)
		require.NoError(t, err)
		csrDer, err := NewX509CSR(prikey, WithX509CSRCommonName("laisky-test"))
		require.NoError(t, err)
		certDer, err := NewX509CertByCSR(ca, cakey, csrDer,
			WithX509SignCSRExtKeyUsage(x509.ExtKeyUsageServerAuth),
		)
		require.NoError(t, err)
		cert, err := Der2Cert(certDer)
		require.NoError(t, err)

		// verify
		root := x509.NewCertPool()
		root.AddCert(ca)
		_, err = cert.Verify(x509.VerifyOptions{
			Roots: root,
			KeyUsages: []x509.ExtKeyUsage{
				x509.ExtKeyUsageServerAuth,
			},
		})
		require.NoError(t, err)
	})
}
