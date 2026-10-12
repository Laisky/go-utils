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

// TestTongsuoConstraintsRejectAmbiguity rejects duplicate, malformed and reserved extension OIDs and invalid non-CA path constraints.
func TestTongsuoConstraintsRejectAmbiguity(t *testing.T) {
	ext := constraintExtensionForTest()
	cases := map[string]struct {
		isCA       bool
		maximum    int
		zero       bool
		extensions []pkix.Extension
	}{
		"non_ca_zero":            {maximum: 0, zero: true},
		"negative":               {isCA: true, maximum: -2},
		"inconsistent_zero_flag": {isCA: true, maximum: 1, zero: true},
		"duplicate":              {isCA: true, maximum: -1, extensions: []pkix.Extension{ext, ext}},
		"invalid_oid":            {isCA: true, maximum: -1, extensions: []pkix.Extension{{Id: asn1.ObjectIdentifier{4, 2}, Value: []byte{5, 0}}}},
		"empty_value":            {isCA: true, maximum: -1, extensions: []pkix.Extension{{Id: ext.Id}}},
		"override_ca":            {isCA: true, maximum: -1, extensions: []pkix.Extension{{Id: asn1.ObjectIdentifier{2, 5, 29, 19}, Value: []byte{0x30, 0}}}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			constraints, err := newTongsuoConstraints(tc.isCA, tc.maximum, tc.zero, tc.extensions)
			require.Error(t, err)
			require.Nil(t, constraints)
		})
	}
	require.Nil(t, X509Cert2OpensslConf(nil))
}

// TestTongsuoConstraintsOutputValidation proves missing or altered fields are rejected even if a tool returns parseable signed DER.
func TestTongsuoConstraintsOutputValidation(t *testing.T) {
	ext := constraintExtensionForTest()
	expected, err := newTongsuoConstraints(true, 0, true, []pkix.Extension{ext})
	require.NoError(t, err)
	for _, kind := range []string{"valid", "missing_pathlen", "wrong_pathlen", "missing_extension", "wrong_critical", "wrong_value", "not_ca"} {
		t.Run(kind, func(t *testing.T) {
			tpl := &x509.Certificate{IsCA: true, BasicConstraintsValid: true, MaxPathLen: 0, MaxPathLenZero: true, KeyUsage: x509.KeyUsageCertSign, ExtraExtensions: []pkix.Extension{constraintExtensionForTest()}}
			switch kind {
			case "missing_pathlen":
				tpl.MaxPathLen = -1
				tpl.MaxPathLenZero = false
			case "wrong_pathlen":
				tpl.MaxPathLen = 1
				tpl.MaxPathLenZero = false
			case "missing_extension":
				tpl.ExtraExtensions = nil
			case "wrong_critical":
				tpl.ExtraExtensions[0].Critical = false
			case "wrong_value":
				tpl.ExtraExtensions[0].Value = []byte{0x04, 0}
			case "not_ca":
				tpl.IsCA = false
				tpl.MaxPathLen = -1
				tpl.MaxPathLenZero = false
			}
			der := newGoFixtureCertForTest(t, tpl, nil)
			err := verifyTongsuoIssuedCert(der, tongsuoIssuanceExpectation{constraints: expected})
			if kind == "valid" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

// TestTongsuoConstraintsPositiveAndUnbounded retains positive and omitted path limits with noncritical raw extensions on both issuance paths.
func TestTongsuoConstraintsPositiveAndUnbounded(t *testing.T) {
	ins := newSecurityTestTongsuo(t)
	parentKey, parentDER := newTongsuoSM2CAForTest(t, ins, "positive-path-issuer")
	csr := newTongsuoSM2CSRForTest(t, ins, WithX509CSRCommonName("positive-path-subject"))
	for _, maximum := range []int{-1, 1, 3} {
		ext := constraintExtensionForTest()
		ext.Critical = false
		_, selfDER, err := ins.NewPrikeyAndCert(t.Context(), WithX509CertCommonName("positive-path-root"), WithX509CertIsCA(), WithX509CertCaMaxPathLen(maximum), WithX509CertExtraExtensions(ext))
		require.NoError(t, err)
		csrDER, err := ins.NewX509CertByCSR(t.Context(), parentDER, parentKey, csr, WithX509SignCSRIsCA(), WithX509CaMaxPathLen(maximum), WithX509SignCSRExtraExtenstions(ext))
		require.NoError(t, err)
		for _, der := range [][]byte{selfDER, csrDER} {
			cert := parseSMCertForTest(t, der)
			require.Equal(t, maximum, cert.MaxPathLen)
			require.False(t, cert.MaxPathLenZero)
			require.Contains(t, cert.Extensions, ext)
		}
	}
}
