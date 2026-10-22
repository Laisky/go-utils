package crypto

import (
	"crypto/x509"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestX509SVIDSANBoundary rejects invalid ancillary names before either standard writer.
func TestX509SVIDSANBoundary(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	for name, bad := range map[string]struct {
		cert X509CertOption
		csr  X509CSROption
	}{
		"nil URI":   {WithX509CertURIs(nil), WithX509CSRURIs(nil)},
		"bad DNS":   {WithX509CertDNSNames("bad name"), WithX509CSRDNSNames("bad name")},
		"bad email": {WithX509CertEmailAddrs("Alice <alice@example.org>"), WithX509CSREmailAddrs("Alice <alice@example.org>")},
		"bad IP":    {WithX509CertIPAddrs(net.IP{1, 2, 3}), WithX509CSRIPAddrs(net.IP{1, 2, 3})},
	} {
		for _, cn := range []string{"", "named workload"} {
			t.Run(name+"/"+cn, func(t *testing.T) {
				var der []byte
				var err error
				t.Run("certificate", func(t *testing.T) {
					require.NotPanics(t, func() {
						der, err = NewX509SVID(key, svidTestID, WithX509CertParent(ca), WithX509CertCommonName(cn), bad.cert)
					})
					require.Error(t, err)
					require.Nil(t, der)
				})
				t.Run("CSR", func(t *testing.T) {
					require.NotPanics(t, func() {
						der, err = NewX509SVIDCSR(key, svidTestID, WithX509CSRCommonName(cn), bad.csr)
					})
					require.Error(t, err)
					require.Nil(t, der)
				})
			})
		}
	}
}

// TestX509SVIDSANBoundaryControls preserves valid mixed SANs, raw precedence and legacy encoding.
func TestX509SVIDSANBoundaryControls(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	for _, cn := range []string{"", "named workload"} {
		t.Run(cn, func(t *testing.T) {
			der, err := NewX509SVID(key, svidTestID, WithX509CertParent(ca), WithX509CertCommonName(cn),
				WithX509CertDNSNames("service.example.org"), WithX509CertEmailAddrs("alice@example.org"),
				WithX509CertIPAddrs(net.ParseIP("2001:db8::1")))
			require.NoError(t, err)
			cert := svidTestParse(t, der)
			require.NoError(t, cert.CheckSignatureFrom(ca))
			require.Equal(t, []string{"service.example.org"}, cert.DNSNames)
			require.Equal(t, []string{"alice@example.org"}, cert.EmailAddresses)
			require.True(t, cert.IPAddresses[0].Equal(net.ParseIP("2001:db8::1")))
			_, err = VerifyX509SVIDForID([]*x509.Certificate{cert}, svidTestBundle("example.org", ca), svidTestID)
			require.NoError(t, err)
			request, err := NewX509SVIDCSR(key, svidTestID, WithX509CSRCommonName(cn), WithX509CSRDNSNames("service.example.org"))
			require.NoError(t, err)
			parsed, err := x509.ParseCertificateRequest(request)
			require.NoError(t, err)
			require.NoError(t, parsed.CheckSignature())
			require.Equal(t, []string{"service.example.org"}, parsed.DNSNames)
		})
	}
	for _, rawFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "raw last", true: "raw first"}[rawFirst], func(t *testing.T) {
			ext := svidRawURIExtension(t, svidTestID)
			certOpts := []X509CertOption{WithX509CertURIs(nil), WithX509CertExtraExtensions(ext)}
			csrOpts := []X509CSROption{WithX509CSRURIs(nil), WithX509CSRExtraExtension(ext)}
			if rawFirst {
				certOpts[0], certOpts[1] = certOpts[1], certOpts[0]
				csrOpts[0], csrOpts[1] = csrOpts[1], csrOpts[0]
			}
			der, err := NewX509SVID(key, svidTestID, append(certOpts, WithX509CertParent(ca))...)
			require.NoError(t, err, "effective raw SAN overrides unused nil URI")
			require.NoError(t, svidTestParse(t, der).CheckSignatureFrom(ca))
			der, err = NewX509SVIDCSR(key, svidTestID, csrOpts...)
			require.NoError(t, err)
			request, err := x509.ParseCertificateRequest(der)
			require.NoError(t, err)
			require.NoError(t, request.CheckSignature())
		})
	}
	der, err := NewX509Cert(key, WithX509CertCommonName("legacy"), WithX509CertParent(ca), WithX509CertDNSNames("bad name"))
	require.NoError(t, err, "legacy generic SAN encoding remains permissive")
	require.Equal(t, []string{"bad name"}, svidTestParse(t, der).DNSNames)
	der, err = NewX509CSR(key, WithX509CSRCommonName("legacy"), WithX509CSRDNSNames("bad name"))
	require.NoError(t, err)
	request, err := x509.ParseCertificateRequest(der)
	require.NoError(t, err)
	require.NoError(t, request.CheckSignature())
}
