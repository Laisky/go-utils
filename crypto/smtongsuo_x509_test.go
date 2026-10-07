package crypto

import (
	"context"
	"crypto/x509"
	"encoding/asn1"
	"math/big"
	"testing"
	"time"

	"github.com/emmansun/gmsm/smx509"
	"github.com/stretchr/testify/require"
)

// TestTongsuo_NewPrikeyAndCert verifies that Tongsuo.NewPrikeyAndCert creates an SM2 key and a
// self-signed certificate honoring the requested options: for a CA, the certificate public key
// matches the private key and the subject, CA:TRUE flag, validity window, and policy OIDs are
// set; for a non-CA, CA:FALSE, the subject, the NotAfter year, and both policy OIDs are set.
func TestTongsuo_NewPrikeyAndCert(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	ctx := context.Background()
	ins, err := NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	t.Run("ca", func(t *testing.T) {
		t.Parallel()

		notbefore := time.Now().UTC().Truncate(time.Second)
		notafter := notbefore.Add(time.Hour * 24 * 7)
		opts := []X509CertOption{
			WithX509CertIsCA(),
			WithX509CertCommonName("test-common-name"),
			WithX509CertOrganization("test org"),
			WithX509CertPolicies(asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 1, 3}),
			WithX509CertPolicies(asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 2, 3}),
			WithX509CertNotBefore(notbefore),
			WithX509CertNotAfter(notafter),
		}

		prikeyPem, certDer, err := ins.NewPrikeyAndCert(context.Background(), opts...)
		require.NoError(t, err)
		require.NotNil(t, prikeyPem)
		require.NotNil(t, certDer)

		t.Run("verify pubkey", func(t *testing.T) {
			pubkeyFromPrikey, err := ins.Prikey2Pubkey(ctx, prikeyPem)
			require.NoError(t, err)

			pubkeyFromCert, err := ins.GetPubkeyFromCertPem(ctx, CertDer2Pem(certDer))
			require.NoError(t, err)

			require.Equal(t, pubkeyFromPrikey, pubkeyFromCert)
		})

		// Verify that the generated certificate is valid
		certinfo, cert, err := ins.ShowCertInfo(ctx, certDer)
		// t.Log(certinf))		require.NoError(t, err)
		require.Contains(t, certinfo, "test-common-name")
		require.Contains(t, certinfo, "test org")
		require.Contains(t, certinfo, "CA:TRUE")
		require.Contains(t, certinfo, "1.3.6.1.4.1.59936.1.1.3")
		require.Contains(t, certinfo, "1.3.6.1.4.1.59936.1.2.3")
		require.NotEmpty(t, cert.SerialNumber)
		require.Equal(t, notbefore, cert.NotBefore.UTC())
		require.Equal(t, notafter, cert.NotAfter.UTC())
		require.Equal(t, "test-common-name", cert.Subject.CommonName)
		require.True(t, cert.IsCA)
		oid, err := OidAsn2X509(asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 1, 3})
		require.NoError(t, err)
		require.Contains(t, cert.Policies, oid)
		oid, err = OidAsn2X509(asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 2, 3})
		require.NoError(t, err)
		require.Contains(t, cert.Policies, oid)
	})

	t.Run("not ca", func(t *testing.T) {
		t.Parallel()
		notafter := time.Now().Add(time.Hour * 24 * 365 * 10)

		opts := []X509CertOption{
			WithX509CertCommonName("test-common-name"),
			WithX509CertOrganization("test org"),
			WithX509CertNotAfter(notafter),
			WithX509CertPolicies(
				asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 1, 3},
				asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 1, 4},
			),
		}

		prikeyPem, certDer, err := ins.NewPrikeyAndCert(context.Background(), opts...)
		require.NoError(t, err)
		require.NotNil(t, prikeyPem)
		require.NotNil(t, certDer)

		// Verify that the generated certificate is valid
		certinfo, cert, err := ins.ShowCertInfo(ctx, certDer)
		// t.Log(certinfo)
		require.NoError(t, err)
		require.Contains(t, certinfo, "test-common-name")
		require.Contains(t, certinfo, "test org")
		require.Contains(t, certinfo, "CA:FALSE")
		require.Contains(t, certinfo, "1.3.6.1.4.1.59936.1.1.3")
		require.Contains(t, certinfo, "1.3.6.1.4.1.59936.1.1.4")
		require.Contains(t, certinfo, notafter.UTC().Format("2006 GMT"))
		require.NotEmpty(t, cert.SerialNumber)
		require.False(t, cert.IsCA)
		require.Equal(t, "test-common-name", cert.Subject.CommonName)
		oid, err := OidAsn2X509(asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 1, 3})
		require.NoError(t, err)
		require.Contains(t, cert.Policies, oid)
		oid, err = OidAsn2X509(asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 1, 4})
		require.NoError(t, err)
		require.Contains(t, cert.Policies, oid)
	})
}

// TestTongsuo_NewIntermediaCaByCsr verifies signing an SM2 CSR with an SM2 root CA through
// Tongsuo.NewX509CertByCSR: as a CA it yields an intermediate with the CSR subject, the root as
// issuer, and the requested policies, which can sign a second-level intermediate and a leaf whose
// chain verifies against a root bundle (but not against an unrelated root); as a non-CA it yields
// a certificate with IsCA false and the requested policy.
func TestTongsuo_NewIntermediaCaByCsr(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	ctx := context.Background()
	ins, err := NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	// new root ca
	rootCaPrikeyPem, rootCaDer, err := ins.NewPrikeyAndCert(ctx,
		WithX509CertCommonName("test-rootca"),
		WithX509CertIsCA())
	require.NoError(t, err)
	rootCaPem := CertDer2Pem(rootCaDer)

	// new prikey
	prikeyPem, err := ins.NewPrikey(ctx)
	require.NoError(t, err)

	// new csr
	csrder, err := ins.NewX509CSR(ctx, prikeyPem,
		WithX509CSRCommonName("test-intermediate"),
		WithX509CSROrganization("test org"),
	)
	require.NoError(t, err)

	t.Run("sign csr as ca", func(t *testing.T) {
		interL1, err := ins.NewX509CertByCSR(ctx, rootCaDer, rootCaPrikeyPem, csrder,
			WithX509SignCSRIsCA(),
			WithX509SignCSRPolicies(
				asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 1, 3},
				asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 1, 4},
			),
		)
		require.NoError(t, err)

		// Verify that the generated certificate is valid
		certinfo, cert, err := ins.ShowCertInfo(ctx, interL1)
		t.Logf("test log test-intermediate: %s", certinfo)
		require.NoError(t, err)
		require.Contains(t, certinfo, "test-intermediate")
		require.Contains(t, certinfo, "test org")
		require.Equal(t, "test-intermediate", cert.Subject.CommonName)
		require.Equal(t, []string{"test org"}, cert.Subject.Organization)
		require.Equal(t, "test-rootca", cert.Issuer.CommonName)
		require.True(t, cert.IsCA)
		require.Equal(t, []asn1.ObjectIdentifier{
			{1, 3, 6, 1, 4, 1, 59936, 1, 1, 3},
			{1, 3, 6, 1, 4, 1, 59936, 1, 1, 4},
		}, cert.PolicyIdentifiers)
		require.NotEmpty(t, cert.SerialNumber)

		t.Run("verify with multiple intermediates and roots", func(t *testing.T) {
			_, uselessRootDer, err := ins.NewPrikeyAndCert(ctx,
				WithX509CertCommonName("useless-root"),
				WithX509CertIsCA(),
			)
			require.NoError(t, err)

			rootsPem := CertDer2Pem(uselessRootDer)
			rootsPem = append(rootsPem, rootCaPem...)

			interL2PrikeyPem, err := ins.NewPrikey(ctx)
			require.NoError(t, err)

			interL2CsrDer, err := ins.NewX509CSR(ctx, interL2PrikeyPem,
				WithX509CSRCommonName("test-intermediate-l2"),
			)
			require.NoError(t, err)

			interL2, err := ins.NewX509CertByCSR(ctx, interL1, prikeyPem, interL2CsrDer,
				WithX509SignCSRIsCA(),
			)
			require.NoError(t, err)

			var intersPem []byte
			intersPem = append(intersPem, CertDer2Pem(interL1)...)
			intersPem = append(intersPem, CertDer2Pem(interL2)...)

			// leaf
			leafPrikeyPem, err := ins.NewPrikey(ctx)
			require.NoError(t, err)

			leafCsrDer, err := ins.NewX509CSR(ctx, leafPrikeyPem,
				WithX509CSRCommonName("test-leaf"),
			)
			require.NoError(t, err)

			leafDer, err := ins.NewX509CertByCSR(ctx, interL2, interL2PrikeyPem, leafCsrDer)
			require.NoError(t, err)
			leafPem := CertDer2Pem(leafDer)

			// Verify that the generated certificate is valid
			err = ins.VerifyCertsChain(ctx, leafPem, intersPem, rootsPem)
			require.NoError(t, err)

			t.Run("miss read root", func(t *testing.T) {
				invalidRootsPem := CertDer2Pem(uselessRootDer)
				err := ins.VerifyCertsChain(ctx, leafPem, intersPem, invalidRootsPem)
				require.ErrorContains(t, err, "cannot verify certs chain")
			})
		})
	})

	t.Run("sign csr as not ca", func(t *testing.T) {
		certDer, err := ins.NewX509CertByCSR(ctx, rootCaDer, rootCaPrikeyPem, csrder,
			WithX509SignCSRPolicies(
				asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 1, 3},
			),
		)
		require.NoError(t, err)

		// Verify that the generated certificate is valid
		certinfo, cert, err := ins.ShowCertInfo(ctx, certDer)
		// t.Log(certinfo)
		require.NoError(t, err)
		require.Contains(t, certinfo, "test-intermediate")
		require.Equal(t, "test-intermediate", cert.Subject.CommonName)
		require.Equal(t, []string{"test org"}, cert.Subject.Organization)
		require.Equal(t, "test-rootca", cert.Issuer.CommonName)
		require.False(t, cert.IsCA)
		require.Equal(t, []asn1.ObjectIdentifier{
			{1, 3, 6, 1, 4, 1, 59936, 1, 1, 3},
		}, cert.PolicyIdentifiers)
		require.NotEmpty(t, cert.SerialNumber)
	})

}

// TestTongsuo_CloneX509Csr verifies that Tongsuo.CloneX509Csr re-signs an existing CSR with a new
// SM2 private key while preserving its subject (CN, C, L, O), DNS names, and email addresses, and
// that the cloned CSR carries a valid self-signature.
func TestTongsuo_CloneX509Csr(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	ctx := context.Background()
	ins, err := NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	prikeyOld, err := ins.NewPrikey(ctx)
	require.NoError(t, err)
	prikeyNew, err := ins.NewPrikey(ctx)
	require.NoError(t, err)

	csrder, err := ins.NewX509CSR(ctx, prikeyOld,
		WithX509CSRCommonName("test-common-name"),
		WithX509CSRCountry("CN"),
		WithX509CSROrganization("BBT"),
		WithX509CSRLocality("Shanghai"),
		WithX509CSRDNSNames("www.example.com", "www.example.net", "www.example.origin"),
		WithX509CSREmailAddrs("test@laisky.com"),
	)
	require.NoError(t, err)

	t.Run("valid csr info", func(t *testing.T) {
		t.Parallel()

		clonedCsr, err := ins.CloneX509Csr(ctx, prikeyNew, csrder)
		require.NoError(t, err)
		require.NotNil(t, clonedCsr)

		// Verify the generated cloned CSR structurally; the display text format
		// differs between Tongsuo releases ("C = CN" vs "C=CN")
		clonedCsrInfo, err := ins.ShowCsrInfo(ctx, clonedCsr)
		require.NoError(t, err)
		require.Contains(t, clonedCsrInfo, "test-common-name")

		parsed, err := smx509.ParseCertificateRequest(clonedCsr)
		require.NoError(t, err)
		require.NoError(t, parsed.CheckSignature())
		require.Equal(t, "test-common-name", parsed.Subject.CommonName)
		require.Equal(t, []string{"CN"}, parsed.Subject.Country)
		require.Equal(t, []string{"Shanghai"}, parsed.Subject.Locality)
		require.Equal(t, []string{"BBT"}, parsed.Subject.Organization)
		require.Equal(t, []string{"www.example.com", "www.example.net", "www.example.origin"}, parsed.DNSNames)
		require.Equal(t, []string{"test@laisky.com"}, parsed.EmailAddresses)
	})
}

// TestTongsuo_NewX509CRL issues an SM2 leaf certificate, revokes it in a CRL
// signed by the SM2 CA through SignX509CRL and verifies the DER CRL under the
// CA certificate. It revives the formerly commented-out CRL test (issue #41).
func TestTongsuo_NewX509CRL(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	ctx := context.Background()
	ins, err := NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	// Generate a CA certificate
	caPrikeyPem, caCertDer, err := ins.NewPrikeyAndCert(ctx,
		WithX509CertCommonName("test-ca"),
		WithX509CertIsCA(),
	)
	require.NoError(t, err)
	caCert, err := smx509.ParseCertificate(caCertDer)
	require.NoError(t, err)

	// Generate a certificate to revoke
	certPrikeyPem, err := ins.NewPrikey(ctx)
	require.NoError(t, err)
	certCsrDer, err := ins.NewX509CSR(ctx, certPrikeyPem,
		WithX509CSRCommonName("test-cert"),
	)
	require.NoError(t, err)
	certDer, err := ins.NewX509CertByCSR(ctx, caCertDer, caPrikeyPem, certCsrDer)
	require.NoError(t, err)
	_, cert, err := ins.ShowCertInfo(ctx, certDer)
	require.NoError(t, err)

	// Build the CRL structure, then sign it with the SM2 CA key
	now := time.Now().UTC().Truncate(time.Second)
	crlNo := big.NewInt(1)
	unsignedCrlDer := newGoRevocationFixtureForTest(t, caCert.RawSubject, caCert.SubjectKeyId,
		crlNo, []x509.RevocationListEntry{{SerialNumber: cert.SerialNumber, RevocationTime: now}},
		now, now.Add(24*time.Hour))
	crlDer, err := ins.SignX509CRL(ctx, unsignedCrlDer, caPrikeyPem)
	require.NoError(t, err)
	require.NotNil(t, crlDer)

	// Verify the generated CRL
	crl, err := smx509.ParseRevocationList(crlDer)
	require.NoError(t, err)
	require.NoError(t, crl.CheckSignatureFrom(caCert))
	require.Equal(t, 0, crlNo.Cmp(crl.Number))
	require.Len(t, crl.RevokedCertificateEntries, 1)
	require.Equal(t, 0, cert.SerialNumber.Cmp(crl.RevokedCertificateEntries[0].SerialNumber))
}
