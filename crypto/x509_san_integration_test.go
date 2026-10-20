package crypto

import (
	"crypto/x509"
	"encoding/asn1"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestX509SANPrivateEKUPathLen preserves merged private purposes alongside exact CA path limits.
func TestX509SANPrivateEKUPathLen(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	purpose := asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55555, 42}
	direct, err := NewX509Cert(key, WithX509CertCommonName("root"), WithX509CertIsCA(), WithX509CertCaMaxPathLen(0),
		WithX509CertUnknownExtKeyUsage(purpose), WithX509CertExtKeyUsage(x509.ExtKeyUsageAny))
	require.NoError(t, err)
	root := svidTestParse(t, direct)
	require.Equal(t, 0, root.MaxPathLen)
	require.True(t, root.MaxPathLenZero)
	require.Equal(t, []asn1.ObjectIdentifier{purpose}, root.UnknownExtKeyUsage)
	leafDER, err := NewX509SVID(key, svidTestID, WithX509CertParent(root), WithX509CertDNSNames("service.example.org"))
	require.NoError(t, err)
	_, err = VerifyX509SVIDForID([]*x509.Certificate{svidTestParse(t, leafDER)}, svidTestBundle("example.org", root), svidTestID)
	require.NoError(t, err, "zero CA limit still permits the leaf with mixed SANs")
	issuer := newCSRSignatureTestCA(t, key)
	requestDER, err := NewX509CSR(key, WithX509CSRCommonName("intermediate"))
	require.NoError(t, err)
	request, err := x509.ParseCertificateRequest(requestDER)
	require.NoError(t, err)
	issued, err := NewX509CertByCSRWithPolicy(issuer, key, requestDER, sanPolicyExact(request, request),
		WithX509SignCSRIsCA(), WithX509CaMaxPathLen(0), WithX509SignCSRUnknownExtKeyUsage(purpose))
	require.NoError(t, err)
	cert := svidTestParse(t, issued)
	require.Equal(t, 0, cert.MaxPathLen)
	require.True(t, cert.MaxPathLenZero)
	require.Equal(t, []asn1.ObjectIdentifier{purpose}, cert.UnknownExtKeyUsage)
	require.NoError(t, cert.CheckSignatureFrom(issuer))
}

// TestSANPolicySerialPrecedence retains the merged serial hook contract through both policy APIs.
func TestSANPolicySerialPrecedence(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.PureEd25519)
	ca := newCSRSignatureTestCA(t, key)
	serial := new(big.Int).Lsh(big.NewInt(1), 120)
	for _, svid := range []bool{false, true} {
		requestDER, claims := sanPolicyRequest(t, key, svid)
		for _, explicitFirst := range []bool{false, true} {
			generator := &countingSerialGenerator{value: 42}
			opts := []SignCSROption{WithX509SignCSRSeriaNumber(serial), WithX509SerialNumGenerator(generator)}
			if !explicitFirst {
				opts[0], opts[1] = opts[1], opts[0]
			}
			der, err := sanPolicyIssue(svid, ca, key, requestDER, sanPolicyExact(claims, claims), opts...)
			require.NoError(t, err)
			require.Zero(t, generator.calls)
			require.Zero(t, svidTestParse(t, der).SerialNumber.Cmp(serial))
		}
	}
}
