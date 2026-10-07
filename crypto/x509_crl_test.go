package crypto

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newTestSeriaNo returns a fresh random serial number from a new DefaultX509CertSerialNumGenerator
// for use in CRL tests. The t parameter is used to fail the calling test if the generator cannot be
// created.
func newTestSeriaNo(t *testing.T) *big.Int {
	g, err := NewDefaultX509CertSerialNumGenerator()
	require.NoError(t, err)

	return big.NewInt(g.SerialNum())
}

// TestNewX509CRL verifies NewX509CRL: a CA certificate that does not assert the cRLSign key usage
// is refused; a CA created with only WithX509CertIsCA asserts cRLSign and can sign a verifiable
// CRL; a nil CRL serial number is rejected; CRLs signed by a WithX509CertIsCRLCA certificate verify with
// VerifyCRL and preserve every revoked serial number and revocation time (at second precision); DER
// and PEM conversions round-trip; and a revoked entry with a zero revocation time is rejected.
func TestNewX509CRL(t *testing.T) {
	t.Parallel()

	t.Run("ca without crl sign key usage", func(t *testing.T) {
		t.Parallel()

		// This package's CA options always add cRLSign, so build the issuer with
		// crypto/x509 directly to get a CA that asserts only keyCertSign.
		ca, prikey := newCRLIssuerCertForTest(t, x509.KeyUsageCertSign|x509.KeyUsageDigitalSignature)
		require.Zero(t, ca.KeyUsage&x509.KeyUsageCRLSign)

		serialNum := newTestSeriaNo(t)
		crlDer, err := NewX509CRL(ca, prikey, serialNum,
			[]pkix.RevokedCertificate{
				{
					RevocationTime: time.Now().UTC(),
					SerialNumber:   serialNum,
				},
			},
		)
		require.Error(t, err, "RFC 5280 4.2.1.3 requires a CRL issuer to assert cRLSign")
		require.Contains(t, strings.ToLower(err.Error()), "crlsign")
		require.Nil(t, crlDer)
	})

	t.Run("ca created by WithX509CertIsCA asserts crl sign", func(t *testing.T) {
		t.Parallel()
		prikeyPem, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
			WithX509CertCommonName("laisky-test"),
			WithX509CertIsCA())
		require.NoError(t, err)

		prikey, err := Pem2Prikey(prikeyPem)
		require.NoError(t, err)

		ca, err := Der2Cert(certder)
		require.NoError(t, err)
		require.NotZero(t, ca.KeyUsage&x509.KeyUsageCRLSign)

		serialNum := newTestSeriaNo(t)
		crlDer, err := NewX509CRL(ca, prikey, serialNum,
			[]pkix.RevokedCertificate{
				{
					RevocationTime: time.Now().UTC(),
					SerialNumber:   serialNum,
				},
			},
		)
		require.NoError(t, err)

		crl, err := Der2CRL(crlDer)
		require.NoError(t, err)
		require.NoError(t, VerifyCRL(ca, crl))
	})

	// Setup CA with CRL signing capability
	prikeyPem, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
		WithX509CertCommonName("laisky-test"),
		WithX509CertIsCRLCA())
	require.NoError(t, err)

	prikey, err := Pem2Prikey(prikeyPem)
	require.NoError(t, err)

	ca, err := Der2Cert(certder)
	require.NoError(t, err)

	serialNum := newTestSeriaNo(t)
	revokeTime := time.Now().UTC()

	t.Run("without crl serial number", func(t *testing.T) {
		t.Parallel()
		_, err := NewX509CRL(ca, prikey, nil,
			[]pkix.RevokedCertificate{
				{
					RevocationTime: revokeTime,
					SerialNumber:   serialNum,
				},
			})
		require.ErrorContains(t, err, "seriaNumber is empty")
	})

	t.Run("with crl serial number", func(t *testing.T) {
		t.Parallel()
		crlDer, err := NewX509CRL(ca, prikey, serialNum,
			[]pkix.RevokedCertificate{
				{
					RevocationTime: revokeTime,
					SerialNumber:   serialNum,
				},
			},
		)
		require.NoError(t, err)

		crl, err := Der2CRL(crlDer)
		require.NoError(t, err)

		err = VerifyCRL(ca, crl)
		require.NoError(t, err)

		require.Equal(t, serialNum, crl.RevokedCertificates[0].SerialNumber)
		require.Equal(t, revokeTime.Unix(), crl.RevokedCertificates[0].RevocationTime.Unix())
	})

	t.Run("with multiple revoked certificates", func(t *testing.T) {
		t.Parallel()
		serialNum2 := newTestSeriaNo(t)
		revokeTime2 := time.Now().UTC()

		crlDer, err := NewX509CRL(ca, prikey, serialNum,
			[]pkix.RevokedCertificate{
				{
					RevocationTime: revokeTime,
					SerialNumber:   serialNum,
				},
				{
					RevocationTime: revokeTime2,
					SerialNumber:   serialNum2,
				},
			},
		)
		require.NoError(t, err)

		crl, err := Der2CRL(crlDer)
		require.NoError(t, err)

		require.Len(t, crl.RevokedCertificates, 2)
		require.Equal(t, serialNum2, crl.RevokedCertificates[1].SerialNumber)
	})

	t.Run("crl convert", func(t *testing.T) {
		t.Parallel()
		crlDer, err := NewX509CRL(ca, prikey, serialNum,
			[]pkix.RevokedCertificate{
				{
					RevocationTime: revokeTime,
					SerialNumber:   serialNum,
				},
			},
		)
		require.NoError(t, err)

		pem := CRLDer2Pem(crlDer)
		gotDer, err := CRLPem2Der(pem)
		require.NoError(t, err)
		require.Equal(t, crlDer, gotDer)

		crl, err := Pem2CRL(pem)
		require.NoError(t, err)
		pem2 := CRL2Pem(crl)
		require.Equal(t, pem, pem2)

		der2 := CRL2Der(crl)
		require.Equal(t, crlDer, der2)
	})

	t.Run("invalid revocation time", func(t *testing.T) {
		t.Parallel()
		_, err := NewX509CRL(ca, prikey, serialNum,
			[]pkix.RevokedCertificate{
				{
					RevocationTime: time.Time{}, // zero time
					SerialNumber:   serialNum,
				},
			},
		)
		require.Error(t, err)
		require.Contains(t, err.Error(), "zero RevocationTime field")
	})
}
