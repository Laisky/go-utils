package encrypt

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewX509CSR(t *testing.T) {
	t.Run("sign by non-ca", func(t *testing.T) {
		prikeyPem, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072)
		require.NoError(t, err)

		prikey, err := Pem2Prikey(prikeyPem)
		require.NoError(t, err)

		csrPrikey, err := NewRSAPrikey(RSAPrikeyBits3072)
		require.NoError(t, err)

		csrder, err := NewX509CSR(csrPrikey,
			WithX509CertCommonName("laisky"),
		)
		require.NoError(t, err)

		ca, err := Der2Cert(certder)
		require.NoError(t, err)

		_, err = NewX509CertByCSR(ca, prikey, csrder,
			WithX509CertIsCA(),
			WithX509CertSignatureAlgorithm(x509.SHA512WithRSA),
		)
		require.Error(t, err)
	})

	prikeyPem, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
		WithX509CertIsCA())
	require.NoError(t, err)

	prikey, err := Pem2Prikey(prikeyPem)
	require.NoError(t, err)

	csrPrikey, err := NewRSAPrikey(RSAPrikeyBits3072)
	require.NoError(t, err)

	csrPrikeyPem, err := Prikey2Pem(csrPrikey)
	require.NoError(t, err)

	t.Run("set attribtues in csr", func(t *testing.T) {
		csrder, err := NewX509CSR(csrPrikey,
			WithX509CertCommonName("laisky"),
			WithX509CertSans("laisky.com"),
		)
		require.NoError(t, err)

		ca, err := Der2Cert(certder)
		require.NoError(t, err)

		newCertDer, err := NewX509CertByCSR(ca, prikey, csrder,
			WithX509CertIsCA(),
			WithX509CertSignatureAlgorithm(x509.SHA512WithRSA),
		)
		require.NoError(t, err)

		newCert, err := Der2Cert(newCertDer)
		require.NoError(t, err)

		require.Equal(t, "laisky", newCert.Subject.CommonName)
		require.Contains(t, newCert.DNSNames, "laisky.com")
		require.True(t, newCert.IsCA)

		t.Run("verify", func(t *testing.T) {
			roots := x509.NewCertPool()
			roots.AppendCertsFromPEM(CertDer2Pem(certder))
			_, err = newCert.Verify(x509.VerifyOptions{
				Roots: roots,
			})
			require.NoError(t, err)

			err = VerifyCertByPrikey(CertDer2Pem(newCertDer), csrPrikeyPem)
			require.NoError(t, err)
		})
	})

}

func TestNewX509CRL(t *testing.T) {
	prikeyPem, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
		WithX509CertIsCA())
	require.NoError(t, err)

	prikey, err := Pem2Prikey(prikeyPem)
	require.NoError(t, err)

	ca, err := Der2Cert(certder)
	require.NoError(t, err)

	crlder, err := NewX509CRL(ca, prikey,
		[]pkix.RevokedCertificate{
			{
				SerialNumber: big.NewInt(2),
			},
		})
	require.NoError(t, err)

	crl, err := Der2CRL(crlder)
	require.NoError(t, err)

	t.Log(crl)

	err = crl.CheckSignatureFrom(ca)
	require.NoError(t, err)
}
