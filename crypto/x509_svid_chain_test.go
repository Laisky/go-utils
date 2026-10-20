package crypto

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/spiffe/go-spiffe/v2/bundle/x509bundle"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"github.com/stretchr/testify/require"
)

// svidTestAuthority creates a signing template with normal X.509 constraints.
func svidTestAuthority(name string) *x509.Certificate {
	return &x509.Certificate{SerialNumber: big.NewInt(17), Subject: pkix.Name{CommonName: name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
}

// svidWrongDomainSource deliberately violates the bundle source contract for a negative control.
type svidWrongDomainSource struct{ bundle *x509bundle.Bundle }

// GetX509BundleForTrustDomain returns the incorrectly associated bundle for testing.
func (s svidWrongDomainSource) GetX509BundleForTrustDomain(spiffeid.TrustDomain) (*x509bundle.Bundle, error) {
	return s.bundle, nil
}

// TestX509SVIDChain validates real chains, federation, rotation, and trust-domain isolation.
func TestX509SVIDChain(t *testing.T) {
	rootKey := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	rootTpl := svidTestAuthority("root")
	root := svidTestSign(t, rootTpl, rootTpl, rootKey, rootKey)
	interKey := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	interTpl := svidTestAuthority("intermediate")
	interTpl.URIs = []*url.URL{{Scheme: "spiffe", Host: "other.org"}}
	inter := svidTestSign(t, interTpl, root, interKey, rootKey)
	leafKey := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	leaf := svidTestSign(t, svidTestTemplate(t), inter, leafKey, interKey)
	chain := []*x509.Certificate{leaf, inter}
	bundles := svidTestBundle("example.org", root)
	_, verified, err := VerifyX509SVID(chain, bundles)
	require.NoError(t, err, "root without URI and cross-domain intermediate are allowed")
	require.Len(t, verified[0], 3)
	_, _, err = VerifyX509SVID(chain, svidTestBundle("other.org", root))
	require.Error(t, err, "no global pooling across domains")
	_, err = VerifyX509SVIDForID(chain, bundles, "spiffe://example.org/other")
	require.ErrorContains(t, err, "not authorized")
	_, _, err = VerifyX509SVID(chain, bundles, x509svid.WithTime(time.Now().Add(48*time.Hour)))
	require.Error(t, err, "explicit verification time still checks validity")
	otherKey := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	otherRoot := newCSRSignatureTestCA(t, otherKey)
	_, _, err = VerifyX509SVID(chain, svidTestBundle("example.org", otherRoot))
	require.Error(t, err, "wrong root cannot authenticate even the right identity")
	// Rotation explicitly publishes both old and new anchors in one domain bundle.
	bundles.Add(x509bundle.FromX509Authorities(spiffeid.RequireTrustDomainFromString("example.org"), []*x509.Certificate{root, otherRoot}))
	_, _, err = VerifyX509SVID(chain, bundles)
	require.NoError(t, err)
	bundles.Add(x509bundle.FromX509Authorities(spiffeid.RequireTrustDomainFromString("example.org"), []*x509.Certificate{otherRoot}))
	_, _, err = VerifyX509SVID(chain, bundles)
	require.Error(t, err)
	_, _, err = VerifyX509SVID(chain, svidWrongDomainSource{x509bundle.FromX509Authorities(spiffeid.RequireTrustDomainFromString("other.org"), []*x509.Certificate{root})})
	require.ErrorContains(t, err, "does not match")
	_, _, err = VerifyX509SVID(chain, svidWrongDomainSource{})
	require.Error(t, err)
	_, _, err = VerifyX509SVID(nil, bundles)
	require.Error(t, err)
	_, _, err = VerifyX509SVID([]*x509.Certificate{nil}, bundles)
	require.Error(t, err)
	_, _, err = VerifyX509SVID(chain, nil)
	require.Error(t, err)
}

// TestX509SVIDChainConstraints covers signatures, pathLen, name constraints, and unknown critical extensions.
func TestX509SVIDChainConstraints(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	rootTpl := svidTestAuthority("root")
	root := svidTestSign(t, rootTpl, rootTpl, key, key)
	for name, mutate := range map[string]func(*x509.Certificate){
		"pathLen": func(c *x509.Certificate) { c.MaxPathLen = 0; c.MaxPathLenZero = true },
		"name constraints": func(c *x509.Certificate) {
			c.PermittedURIDomains = []string{"other.org"}
			c.PermittedDNSDomainsCritical = true
		},
		"unknown critical": func(c *x509.Certificate) {
			c.ExtraExtensions = []pkix.Extension{{Id: []int{1, 2, 3, 4}, Critical: true, Value: []byte{5, 0}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			tpl := svidTestAuthority("constrained root")
			mutate(tpl)
			issuer := svidTestSign(t, tpl, tpl, key, key)
			interTpl := svidTestAuthority("intermediate")
			inter := svidTestSign(t, interTpl, issuer, key, key)
			leaf := svidTestSign(t, svidTestTemplate(t), inter, key, key)
			_, _, err := VerifyX509SVID([]*x509.Certificate{leaf, inter}, svidTestBundle("example.org", issuer))
			require.Error(t, err)
		})
	}
	leaf := svidTestSign(t, svidTestTemplate(t), root, key, key)
	corrupted := append([]byte(nil), leaf.Raw...)
	corrupted[len(corrupted)-1] ^= 1
	_, _, err := VerifyX509SVID([]*x509.Certificate{svidTestParse(t, corrupted)}, svidTestBundle("example.org", root))
	require.Error(t, err)
	criticalLeafTpl := svidTestTemplate(t)
	criticalLeafTpl.ExtraExtensions = []pkix.Extension{{Id: []int{1, 2, 3, 4}, Critical: true, Value: []byte{5, 0}}}
	criticalLeaf := svidTestSign(t, criticalLeafTpl, root, key, key)
	_, err = ValidateX509SVID(criticalLeaf)
	require.NoError(t, err, "profile validation alone is not path authentication")
	_, _, err = VerifyX509SVID([]*x509.Certificate{criticalLeaf}, svidTestBundle("example.org", root))
	require.Error(t, err)
	for _, uri := range []string{"", "https://ca.example.org/info", "spiffe://other.org", "spiffe://other.org/not-a-ca-id"} {
		t.Run("CA URI "+uri, func(t *testing.T) {
			tpl := svidTestAuthority("URI root")
			if uri != "" {
				parsed, err := url.Parse(uri)
				require.NoError(t, err)
				tpl.URIs = []*url.URL{parsed}
			}
			issuer := svidTestSign(t, tpl, tpl, key, key)
			leaf := svidTestSign(t, svidTestTemplate(t), issuer, key, key)
			_, _, err := VerifyX509SVID([]*x509.Certificate{leaf}, svidTestBundle("example.org", issuer))
			if uri == "spiffe://other.org/not-a-ca-id" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// svidTestTLSCert issues a TLS certificate with a workload identity.
func svidTestTLSCert(t *testing.T, ca *x509.Certificate, issuerKey, leafKey crypto.Signer, id string) tls.Certificate {
	t.Helper()
	der, err := NewX509SVID(issuerKey, id, WithX509CertParent(ca), WithX509CertPubkey(leafKey.Public()))
	require.NoError(t, err)
	return tls.Certificate{Certificate: [][]byte{der, ca.Raw}, PrivateKey: leafKey}
}

// TestX509SVIDMTLS performs real TLS 1.3 mutual handshakes over local in-memory connections.
func TestX509SVIDMTLS(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	bundles := svidTestBundle("example.org", ca)
	serverID := "spiffe://example.org/server"
	clientID := "spiffe://example.org/client"
	serverCert := svidTestTLSCert(t, ca, key, key, serverID)
	for _, actual := range []string{clientID, "spiffe://example.org/other", "spiffe://other.org/client"} {
		t.Run(actual, func(t *testing.T) {
			clientCert := svidTestTLSCert(t, ca, key, key, actual)
			serverRaw, clientRaw := net.Pipe()
			t.Cleanup(func() { require.NoError(t, serverRaw.Close()); require.NoError(t, clientRaw.Close()) })
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			server := tls.Server(serverRaw, &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCert}, ClientAuth: tls.RequireAnyClientCert,
				VerifyConnection: func(state tls.ConnectionState) error {
					_, err := VerifyX509SVIDForID(state.PeerCertificates, bundles, clientID)
					return err
				}})
			client := tls.Client(clientRaw, &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{clientCert},
				// SPIFFE verifies the URI identity and domain bundle instead of DNS/OS roots.
				InsecureSkipVerify: true, //nolint:gosec // Complete verification is provided below.
				VerifyConnection: func(state tls.ConnectionState) error {
					_, err := VerifyX509SVIDForID(state.PeerCertificates, bundles, serverID)
					return err
				}})
			serverResult := make(chan error, 1)
			go func() { serverResult <- server.HandshakeContext(ctx) }()
			clientErr := client.HandshakeContext(ctx)
			// TLS 1.3 can finish the client handshake before the server authorizes its
			// certificate. Drain the alert so net.Pipe's synchronous write cannot block.
			alertResult := make(chan error, 1)
			go func() {
				var one [1]byte
				_, err := client.Read(one[:])
				alertResult <- err
			}()
			serverErr := <-serverResult
			require.NoError(t, clientRaw.Close())
			require.Error(t, <-alertResult)
			if actual == clientID {
				require.NoError(t, clientErr)
				require.NoError(t, serverErr)
			} else if actual == "spiffe://example.org/other" {
				require.ErrorContains(t, serverErr, "not authorized")
			} else {
				require.ErrorContains(t, serverErr, "trust-domain bundle")
			}
		})
	}
}
