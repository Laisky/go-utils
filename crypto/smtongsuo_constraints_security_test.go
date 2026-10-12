package crypto

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"testing"

	"github.com/stretchr/testify/require"
)

// constraintExtensionForTest returns a benign critical DER extension used only by disposable test certificates.
func constraintExtensionForTest() pkix.Extension {
	return pkix.Extension{Id: asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 112}, Critical: true, Value: []byte{0x05, 0x00}}
}

// TestTongsuoConstraintsConverters checks both public issuance paths represent explicitly requested CA path limits and raw extensions.
func TestTongsuoConstraintsConverters(t *testing.T) {
	ext := constraintExtensionForTest()
	t.Run("self_signed", func(t *testing.T) {
		_, tpl, err := x509CertOption2Template(WithX509CertCommonName("converter-root"), WithX509CertIsCA(), WithX509CertCaMaxPathLen(0), WithX509CertExtraExtensions(ext))
		require.NoError(t, err)
		conf, err := x509Cert2OpensslConf(tpl, opensslConfEncoder{})
		require.NoError(t, err)
		t.Run("pathlen", func(t *testing.T) { require.Contains(t, string(conf), "pathlen:0") })
		t.Run("extension", func(t *testing.T) { require.Contains(t, string(conf), ext.Id.String()+" = critical,DER:05:00") })
	})
	t.Run("csr", func(t *testing.T) {
		_, conf, err := x509SignCsrOptions2OpensslConf(WithX509SignCSRIsCA(), WithX509CaMaxPathLen(0), WithX509SignCSRExtraExtenstions(ext))
		require.NoError(t, err)
		t.Run("pathlen", func(t *testing.T) { require.Contains(t, string(conf), "pathlen:0") })
		t.Run("extension", func(t *testing.T) { require.Contains(t, string(conf), ext.Id.String()+" = critical,DER:05:00") })
	})
}

// TestTongsuoConstraintsSelfSigned checks actual SM2 certificate DER rather than tool display text.
func TestTongsuoConstraintsSelfSigned(t *testing.T) {
	ins := newSecurityTestTongsuo(t)
	ext := constraintExtensionForTest()
	_, der, err := ins.NewPrikeyAndCert(t.Context(), WithX509CertCommonName("disposable-root"), WithX509CertIsCA(), WithX509CertCaMaxPathLen(0), WithX509CertExtraExtensions(ext))
	require.NoError(t, err)
	cert := parseSMCertForTest(t, der)
	require.True(t, cert.IsCA)
	t.Run("pathlen", func(t *testing.T) { require.Equal(t, 0, cert.MaxPathLen); require.True(t, cert.MaxPathLenZero) })
	t.Run("extension", func(t *testing.T) { require.Contains(t, cert.Extensions, ext) })
}

// TestTongsuoConstraintsCSR checks pathLen=0 and an exact critical extension on an issued intermediate CA.
func TestTongsuoConstraintsCSR(t *testing.T) {
	ins := newSecurityTestTongsuo(t)
	parentKey, parentDER := newTongsuoSM2CAForTest(t, ins, "disposable-issuer")
	csr := newTongsuoSM2CSRForTest(t, ins, WithX509CSRCommonName("disposable-intermediate"))
	ext := constraintExtensionForTest()
	der, err := ins.NewX509CertByCSR(t.Context(), parentDER, parentKey, csr, WithX509SignCSRIsCA(), WithX509CaMaxPathLen(0), WithX509SignCSRExtraExtenstions(ext))
	require.NoError(t, err)
	cert := parseSMCertForTest(t, der)
	require.True(t, cert.IsCA)
	t.Run("pathlen", func(t *testing.T) { require.Equal(t, 0, cert.MaxPathLen); require.True(t, cert.MaxPathLenZero) })
	t.Run("extension", func(t *testing.T) { require.Contains(t, cert.Extensions, ext) })
}

// TestTongsuoConstraintsNativeControl proves the equivalent native-Go options already preserve the requested fields.
func TestTongsuoConstraintsNativeControl(t *testing.T) {
	ext := constraintExtensionForTest()
	_, der, err := NewECDSAPrikeyAndCert(ECDSACurveP256, WithX509CertCommonName("native-root"), WithX509CertIsCA(), WithX509CertCaMaxPathLen(0), WithX509CertExtraExtensions(ext))
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	t.Run("pathlen", func(t *testing.T) { require.Equal(t, 0, cert.MaxPathLen); require.True(t, cert.MaxPathLenZero) })
	t.Run("extension", func(t *testing.T) { require.Contains(t, cert.Extensions, ext) })
}
