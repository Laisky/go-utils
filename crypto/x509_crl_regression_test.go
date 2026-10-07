package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newCRLIssuerCertForTest creates a self-signed ECDSA P-256 CA certificate whose
// KeyUsage is exactly keyUsage. It uses crypto/x509 directly because this
// package's CA options always add cRLSign. When keyUsage is zero the
// certificate carries no key usage extension at all. It returns the parsed
// certificate and its private key, failing t on any error.
func newCRLIssuerCertForTest(t *testing.T, keyUsage x509.KeyUsage) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()

	prikey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	skid, err := X509CertSubjectKeyID(&prikey.PublicKey)
	require.NoError(t, err)

	now := time.Now().UTC()
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano()),
		Subject:               pkix.Name{CommonName: "crl-issuer-under-test"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              keyUsage,
		SubjectKeyId:          skid,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &prikey.PublicKey, prikey)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	require.Equal(t, keyUsage, cert.KeyUsage)
	require.True(t, cert.IsCA)
	return cert, prikey
}

// newRevokedEntriesForTest returns a single revoked-certificate entry with a
// fresh serial number and the current UTC time, for use as NewX509CRL input.
func newRevokedEntriesForTest(t *testing.T) []pkix.RevokedCertificate {
	t.Helper()

	return []pkix.RevokedCertificate{{
		SerialNumber:   newTestSeriaNo(t),
		RevocationTime: time.Now().UTC(),
	}}
}

// TestNewX509CRL_RefusesIssuerWithoutCRLSign verifies that NewX509CRL refuses
// to sign a CRL with a CA certificate that does not assert the cRLSign key
// usage, as RFC 5280 section 4.2.1.3 requires of every CRL issuer, both when
// the key usage extension lacks the bit and when the extension is absent. It
// is a regression guard for the documentation-pass finding that the
// "ca without crl sign key usage" subtest never built such a CA.
func TestNewX509CRL_RefusesIssuerWithoutCRLSign(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		keyUsage x509.KeyUsage
	}{
		{name: "cert sign without crl sign", keyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature},
		{name: "no key usage extension", keyUsage: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ca, prikey := newCRLIssuerCertForTest(t, tc.keyUsage)
			crlDer, err := NewX509CRL(ca, prikey, newTestSeriaNo(t), newRevokedEntriesForTest(t))
			require.Error(t, err)
			require.Nil(t, crlDer)
			require.Contains(t, strings.ToLower(err.Error()), "crlsign")
		})
	}

	t.Run("crl sign asserted is accepted", func(t *testing.T) {
		t.Parallel()

		ca, prikey := newCRLIssuerCertForTest(t, x509.KeyUsageCertSign|x509.KeyUsageCRLSign)
		crlDer, err := NewX509CRL(ca, prikey, newTestSeriaNo(t), newRevokedEntriesForTest(t))
		require.NoError(t, err)

		crl, err := Der2CRL(crlDer)
		require.NoError(t, err)
		require.NoError(t, VerifyCRL(ca, crl))
	})
}

// TestNewX509CRL_RefusesMismatchedIssuerKey verifies that NewX509CRL refuses a
// private key that does not belong to the issuer certificate instead of
// emitting a CRL that names the CA as issuer but can never verify against it.
// This mirrors the check x509.CreateCertificate performs for certificates.
func TestNewX509CRL_RefusesMismatchedIssuerKey(t *testing.T) {
	t.Parallel()

	ca, _ := newCRLIssuerCertForTest(t, x509.KeyUsageCertSign|x509.KeyUsageCRLSign)
	otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	crlDer, err := NewX509CRL(ca, otherKey, newTestSeriaNo(t), newRevokedEntriesForTest(t))
	require.ErrorContains(t, err, "does not match")
	require.Nil(t, crlDer)

	t.Run("different key algorithm", func(t *testing.T) {
		t.Parallel()

		edKey, err := NewEd25519Prikey()
		require.NoError(t, err)

		crlDer, err := NewX509CRL(ca, edKey, newTestSeriaNo(t), newRevokedEntriesForTest(t))
		require.ErrorContains(t, err, "does not match")
		require.Nil(t, crlDer)
	})
}
