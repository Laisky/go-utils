package encrypt

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
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

	// generate root-ca
	prikeyPem, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
		WithX509CertIsCA())
	require.NoError(t, err)

	prikey, err := Pem2Prikey(prikeyPem)
	require.NoError(t, err)

	csrPrikey, err := NewRSAPrikey(RSAPrikeyBits3072)
	require.NoError(t, err)

	csrPrikeyPem, err := Prikey2Pem(csrPrikey)
	require.NoError(t, err)

	t.Run("sign ca-csr", func(t *testing.T) {
		csrder, err := NewX509CSR(csrPrikey,
			WithX509CertCommonName("laisky"),
			WithX509CertSANS("laisky.com"),
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
	})

	t.Run("set attribtues in non-ca csr", func(t *testing.T) {
		csrder, err := NewX509CSR(csrPrikey,
			WithX509CertCommonName("laisky"),
			WithX509CertSANS("laisky.com"),
		)
		require.NoError(t, err)

		ca, err := Der2Cert(certder)
		require.NoError(t, err)

		newCertDer, err := NewX509CertByCSR(ca, prikey, csrder,
			WithX509CertSignatureAlgorithm(x509.SHA512WithRSA),
		)
		require.NoError(t, err)

		newCert, err := Der2Cert(newCertDer)
		require.NoError(t, err)

		require.Equal(t, "laisky", newCert.Subject.CommonName)
		require.Contains(t, newCert.DNSNames, "laisky.com")
		require.False(t, newCert.IsCA)

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

	serialNum, err := RandomSerialNumber()
	require.NoError(t, err)

	var crlder []byte
	t.Run("without crl serial number", func(t *testing.T) {
		var err error
		crlder, err = NewX509CRL(ca, prikey,
			[]pkix.RevokedCertificate{
				{
					SerialNumber: serialNum,
				},
			})
		require.ErrorContains(t, err, "WithX509CertSeriaNumber() is required for NewX509CRL")
	})

	t.Run("with crl serial number", func(t *testing.T) {
		var err error
		crlder, err = NewX509CRL(ca, prikey,
			[]pkix.RevokedCertificate{
				{
					SerialNumber: serialNum,
				},
			},
			WithX509CertSeriaNumber(serialNum),
		)
		require.NoError(t, err)
	})

	crl, err := Der2CRL(crlder)
	require.NoError(t, err)

	t.Log(crl)

	err = crl.CheckSignatureFrom(ca)
	require.NoError(t, err)
}

func Test_OIDs(t *testing.T) {
	a1 := asn1.ObjectIdentifier{1, 2, 3}
	a2 := asn1.ObjectIdentifier{1, 2, 3}
	a3 := asn1.ObjectIdentifier{1, 2, 3, 4}
	require.Equal(t, a1, a2)
	require.NotEqual(t, a1, a3)
	require.NotEqual(t, a2, a3)

	_, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
		WithX509CertPolicies(a1, a2),
	)
	require.NoError(t, err)

	ca, err := Der2Cert(certder)
	require.NoError(t, err)

	require.Contains(t, ca.PolicyIdentifiers, a1)
	require.Contains(t, ca.PolicyIdentifiers, a2)
	require.NotContains(t, ca.PolicyIdentifiers, a3)
	require.True(t, OIDContains(ca.PolicyIdentifiers, a1))
	require.True(t, OIDContains(ca.PolicyIdentifiers, a2))
	require.False(t, OIDContains(ca.PolicyIdentifiers, a3))
}
