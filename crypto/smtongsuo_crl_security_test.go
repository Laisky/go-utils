package crypto

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"math/big"
	"testing"
	"time"

	"github.com/emmansun/gmsm/smx509"
	"github.com/stretchr/testify/require"
)

// crlEntriesForTest returns two revoked entries with reason codes and
// revocation times truncated to whole seconds.
func crlEntriesForTest(now time.Time) []x509.RevocationListEntry {
	return []x509.RevocationListEntry{
		{SerialNumber: big.NewInt(1001), RevocationTime: now.Add(-time.Hour), ReasonCode: 1},
		{SerialNumber: big.NewInt(1002), RevocationTime: now.Add(-2 * time.Hour), ReasonCode: 4},
	}
}

// requireCRLFieldsSurviveForTest requires that the signed CRL kept the input
// issuer, number, update times, revoked entries and extensions.
func requireCRLFieldsSurviveForTest(t *testing.T, input []byte, signed *smx509.RevocationList) {
	t.Helper()

	in, err := smx509.ParseRevocationList(input)
	require.NoError(t, err)
	require.Equal(t, in.RawIssuer, signed.RawIssuer)
	require.Equal(t, 0, in.Number.Cmp(signed.Number))
	require.True(t, in.ThisUpdate.Equal(signed.ThisUpdate))
	require.True(t, in.NextUpdate.Equal(signed.NextUpdate))
	require.Equal(t, in.AuthorityKeyId, signed.AuthorityKeyId)
	require.Equal(t, in.Extensions, signed.Extensions)
	require.Len(t, signed.RevokedCertificateEntries, len(in.RevokedCertificateEntries))
	for i := range in.RevokedCertificateEntries {
		want, got := in.RevokedCertificateEntries[i], signed.RevokedCertificateEntries[i]
		require.Equal(t, 0, want.SerialNumber.Cmp(got.SerialNumber))
		require.True(t, want.RevocationTime.Equal(got.RevocationTime))
		require.Equal(t, want.ReasonCode, got.ReasonCode)
	}
}

// TestTongsuoSignX509CRLReturnsVerifiedDER signs CRLs with the real tongsuo
// binary and requires DER output that verifies under the issuer key while
// preserving every CRL field. Regression for issue #41.
func TestTongsuoSignX509CRLReturnsVerifiedDER(t *testing.T) {
	t.Parallel()
	ins := newSecurityTestTongsuo(t)
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Second)

	t.Run("sm2 issuer", func(t *testing.T) {
		t.Parallel()
		caKey, caDer := newTongsuoSM2CAForTest(t, ins, "crl-sm2-root")
		ca := parseSMCertForTest(t, caDer)

		input := newGoRevocationFixtureForTest(t, ca.RawSubject, ca.SubjectKeyId,
			big.NewInt(7), crlEntriesForTest(now), now, now.Add(24*time.Hour))

		signed, err := ins.SignX509CRL(ctx, input, caKey)
		require.NoError(t, err)
		require.False(t, bytes.HasPrefix(bytes.TrimSpace(signed), []byte("-----BEGIN")), "output must be DER, not PEM")

		crl, err := smx509.ParseRevocationList(signed)
		require.NoError(t, err)
		require.Equal(t, signed, crl.Raw, "output must be exactly one DER CRL")
		require.Equal(t, smx509.SM2WithSM3, crl.SignatureAlgorithm)
		require.NoError(t, crl.CheckSignatureFrom(ca))
		requireCRLFieldsSurviveForTest(t, input, crl)

		// a different SM2 CA must not verify the signature
		_, otherDer := newTongsuoSM2CAForTest(t, ins, "crl-sm2-root")
		require.Error(t, crl.CheckSignatureFrom(parseSMCertForTest(t, otherDer)))
	})

	t.Run("rsa issuer", func(t *testing.T) {
		t.Parallel()
		caKeyPem, caDer, err := NewRSAPrikeyAndCert(RSAPrikeyBits2048,
			WithX509CertCommonName("crl-rsa-root"), WithX509CertIsCA())
		require.NoError(t, err)
		ca, err := x509.ParseCertificate(caDer)
		require.NoError(t, err)

		input := newGoRevocationFixtureForTest(t, ca.RawSubject, ca.SubjectKeyId,
			big.NewInt(8), crlEntriesForTest(now), now, now.Add(24*time.Hour))

		signed, err := ins.SignX509CRL(ctx, input, caKeyPem)
		require.NoError(t, err)

		crl, err := x509.ParseRevocationList(signed)
		require.NoError(t, err)
		require.Equal(t, x509.SHA256WithRSA, crl.SignatureAlgorithm)
		require.NoError(t, crl.CheckSignatureFrom(ca))

		smCRL, err := smx509.ParseRevocationList(signed)
		require.NoError(t, err)
		requireCRLFieldsSurviveForTest(t, input, smCRL)
	})

	t.Run("ecdsa issuer", func(t *testing.T) {
		t.Parallel()
		caKeyPem, caDer, err := NewECDSAPrikeyAndCert(ECDSACurveP384,
			WithX509CertCommonName("crl-ecdsa-root"), WithX509CertIsCA())
		require.NoError(t, err)
		ca, err := x509.ParseCertificate(caDer)
		require.NoError(t, err)

		input := newGoRevocationFixtureForTest(t, ca.RawSubject, ca.SubjectKeyId,
			big.NewInt(9), nil, now, now.Add(time.Hour))

		signed, err := ins.SignX509CRL(ctx, input, caKeyPem)
		require.NoError(t, err)

		crl, err := x509.ParseRevocationList(signed)
		require.NoError(t, err)
		require.Equal(t, x509.ECDSAWithSHA384, crl.SignatureAlgorithm)
		require.NoError(t, crl.CheckSignatureFrom(ca))
		require.Empty(t, crl.RevokedCertificateEntries)
	})
}

// TestTongsuoSignX509CRLRejectsInvalidInput verifies malformed CRLs, PEM
// input, trailing data and unusable keys yield errors and no partial output.
// Regression for issue #41.
func TestTongsuoSignX509CRLRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	ins := newSecurityTestTongsuo(t)
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Second)

	caKey, caDer := newTongsuoSM2CAForTest(t, ins, "crl-invalid-root")
	ca := parseSMCertForTest(t, caDer)
	input := newGoRevocationFixtureForTest(t, ca.RawSubject, ca.SubjectKeyId,
		big.NewInt(1), nil, now, now.Add(time.Hour))

	weakRSA, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)
	weakRSAPem, err := Prikey2Pem(weakRSA)
	require.NoError(t, err)
	edKey, err := NewEd25519Prikey()
	require.NoError(t, err)
	edPem, err := Prikey2Pem(edKey)
	require.NoError(t, err)

	cases := []struct {
		name string
		crl  []byte
		key  []byte
	}{
		{"rsa key below 2048 bits", input, weakRSAPem},
		{"unsupported ed25519 key", input, edPem},
		{"empty crl", nil, caKey},
		{"garbage crl", []byte("not a crl"), caKey},
		{"pem crl", CRLDer2Pem(input), caKey},
		{"trailing data", append(append([]byte{}, input...), 0x00), caKey},
		{"empty key", input, nil},
		{"garbage key", input, []byte("-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----\n")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, err := ins.SignX509CRL(ctx, tc.crl, tc.key)
			require.Error(t, err)
			require.Nil(t, out)
		})
	}
}
