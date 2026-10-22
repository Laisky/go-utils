package crypto

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"net/url"
	"testing"

	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"github.com/stretchr/testify/require"
)

// TestX509SVIDSelfSignedProfileContract distinguishes public profile construction from trust.
func TestX509SVIDSelfSignedProfileContract(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.PureEd25519)
	der, err := NewX509SVID(key, svidTestID, WithX509CertCommonName("self-signed profile"))
	require.NoError(t, err)
	cert := svidTestParse(t, der)
	require.Equal(t, cert.RawSubject, cert.RawIssuer)
	require.False(t, cert.IsCA)
	require.NoError(t, cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature))
	id, err := ValidateX509SVID(cert)
	require.NoError(t, err)
	require.Equal(t, svidTestID, id.String())
	// Ordinary X.509 trust can explicitly trust this leaf; SVID authentication
	// additionally requires a chain containing a signing authority.
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	_, err = cert.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}})
	require.NoError(t, err)
	bundles := svidTestBundle("example.org", cert)
	_, chains, err := VerifyX509SVID([]*x509.Certificate{cert}, bundles)
	require.ErrorContains(t, err, "trusted leaf is not a signing-authority chain")
	require.Nil(t, chains)
	chains, err = VerifyX509SVIDForID([]*x509.Certificate{cert}, bundles, svidTestID)
	require.ErrorContains(t, err, "trusted leaf is not a signing-authority chain")
	require.Nil(t, chains)
	der, err = NewX509SVID(key, svidTestID)
	require.ErrorContains(t, err, "nonempty Issuer DN")
	require.Nil(t, der)
	ca := newCSRSignatureTestCA(t, key)
	der, err = NewX509SVID(key, svidTestID, WithX509CertParent(ca))
	require.NoError(t, err)
	chains, err = VerifyX509SVIDForID([]*x509.Certificate{svidTestParse(t, der)}, svidTestBundle("example.org", ca), svidTestID)
	require.NoError(t, err)
	require.Len(t, chains[0], 2)
}

// TestX509SVIDAuthorityDiagnostics preserves rejection reasons without excluding valid alternate paths.
func TestX509SVIDAuthorityDiagnostics(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.PureEd25519)
	goodTemplate := svidTestAuthority("alternate review root")
	good := svidTestSign(t, goodTemplate, goodTemplate, key, key)
	badTemplate := svidTestAuthority("alternate review root")
	badTemplate.URIs = []*url.URL{{Scheme: "spiffe", Host: "example.org", Path: "/not-a-ca-id"}}
	bad := svidTestSign(t, badTemplate, badTemplate, key, key)
	leafDER, err := NewX509SVID(key, svidTestID, WithX509CertParent(good))
	require.NoError(t, err)
	leaf := svidTestParse(t, leafDER)
	_, rejected, err := VerifyX509SVID([]*x509.Certificate{leaf}, svidTestBundle("example.org", bad))
	require.ErrorContains(t, err, "no chain has valid X.509-SVID signing certificates")
	require.ErrorContains(t, err, "chain 0 authority 1")
	require.ErrorContains(t, err, "SPIFFE ID must have no path")
	require.Nil(t, rejected)
	for _, authorities := range [][]*x509.Certificate{{bad, good}, {good, bad}} {
		bundles := svidTestBundle("example.org", authorities...)
		_, candidates, err := x509svid.Verify([]*x509.Certificate{leaf}, bundles)
		require.NoError(t, err, "the standard chain verifier can build both candidate paths")
		require.Len(t, candidates, 2)
		_, chains, err := VerifyX509SVID([]*x509.Certificate{leaf}, bundles)
		require.NoError(t, err, "one rejected authority must not hide another valid chain")
		require.Len(t, chains, 1)
		require.Equal(t, good.Raw, chains[0][1].Raw)
	}
}

// TestX509SVIDPolicyAuthenticatedSnapshot protects the private issuance core from callback mutations.
func TestX509SVIDPolicyAuthenticatedSnapshot(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.PureEd25519)
	ca := newCSRSignatureTestCA(t, key)
	requestDER, claims := sanPolicyRequest(t, key, true)
	original := bytes.Clone(requestDER)
	requests, certificates, options := 0, 0, 0
	policy := X509CSRIdentityPolicy{
		AuthorizeRequest: func(snapshot *x509.CertificateRequest) error {
			requests++
			snapshot.Raw[0] ^= 1
			snapshot.RawSubject[0] ^= 1
			snapshot.URIs[0].Host = "mutated.example.org"
			snapshot.DNSNames[0] = "mutated.example.org"
			snapshot.EmailAddresses[0] = "mutated@example.org"
			snapshot.IPAddresses[0][0] ^= 1
			snapshot.PublicKey.(ed25519.PublicKey)[0] ^= 1
			requestDER[0] ^= 1
			return nil
		},
		AuthorizeCertificate: func(snapshot *x509.Certificate) error {
			certificates++
			require.True(t, sanPolicyMatches(snapshot.RawSubject, snapshot.DNSNames, snapshot.EmailAddresses,
				snapshot.IPAddresses, snapshot.URIs, claims))
			require.Equal(t, claims.RawSubjectPublicKeyInfo, snapshot.RawSubjectPublicKeyInfo)
			snapshot.Raw[0] ^= 1
			snapshot.RawSubject[0] ^= 1
			snapshot.URIs[0].Host = "mutated.example.org"
			return nil
		},
	}
	der, err := NewX509SVIDByCSRWithPolicy(ca, key, requestDER, svidTestID, policy,
		func(*signCSROption) error { options++; return nil })
	require.NoError(t, err)
	require.Equal(t, 1, requests)
	require.Equal(t, 1, certificates)
	require.Equal(t, 1, options)
	cert := svidTestParse(t, der)
	require.True(t, sanPolicyMatches(cert.RawSubject, cert.DNSNames, cert.EmailAddresses, cert.IPAddresses, cert.URIs, claims))
	require.Equal(t, claims.RawSubjectPublicKeyInfo, cert.RawSubjectPublicKeyInfo)
	require.NoError(t, cert.CheckSignatureFrom(ca))
	_, err = VerifyX509SVIDForID([]*x509.Certificate{cert}, svidTestBundle("example.org", ca), svidTestID)
	require.NoError(t, err)
	requests, certificates, options = 0, 0, 0
	tampered := bytes.Clone(original)
	tampered[len(tampered)-1] ^= 1
	der, err = NewX509SVIDByCSRWithPolicy(ca, key, tampered, svidTestID, policy,
		func(*signCSROption) error { options++; return nil })
	require.ErrorContains(t, err, "possession")
	require.Nil(t, der)
	require.Zero(t, requests)
	require.Zero(t, certificates)
	require.Zero(t, options)
}
