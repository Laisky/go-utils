package crypto

import (
	"context"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"testing"

	"github.com/stretchr/testify/require"
)

// privateEKUTestExtension encodes purposes as a raw EKU extension for controls.
func privateEKUTestExtension(t *testing.T, purposes ...asn1.ObjectIdentifier) pkix.Extension {
	t.Helper()
	der, err := asn1.Marshal(purposes)
	require.NoError(t, err)
	return pkix.Extension{Id: asn1.ObjectIdentifier{2, 5, 29, 37}, Value: der}
}

// TestPrivateEKUIssuance verifies typed purposes in direct and CSR certificate DER.
func TestPrivateEKUIssuance(t *testing.T) {
	t.Parallel()
	privateOne := asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55555, 1}
	privateTwo := asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55555, 2}
	serverAuth := asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 1}
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	csrDER, err := NewX509CSR(key, WithX509CSRCommonName("private-eku"))
	require.NoError(t, err)
	for _, tc := range []struct {
		name    string
		known   []x509.ExtKeyUsage
		oids    []asn1.ObjectIdentifier
		want    []x509.ExtKeyUsage
		unknown []asn1.ObjectIdentifier
	}{
		{"private", nil, []asn1.ObjectIdentifier{privateOne, privateTwo}, nil, []asn1.ObjectIdentifier{privateOne, privateTwo}},
		{"mixed", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, []asn1.ObjectIdentifier{privateOne}, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, []asn1.ObjectIdentifier{privateOne}},
		{"any_and_private", []x509.ExtKeyUsage{x509.ExtKeyUsageAny}, []asn1.ObjectIdentifier{privateOne}, []x509.ExtKeyUsage{x509.ExtKeyUsageAny}, []asn1.ObjectIdentifier{privateOne}},
		{"known_oid", nil, []asn1.ObjectIdentifier{serverAuth, privateOne}, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, []asn1.ObjectIdentifier{privateOne}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, viaCSR := range []bool{false, true} {
				var der []byte
				if viaCSR {
					der, err = NewX509CertByCSR(ca, key, csrDER, WithX509SignCSRExtKeyUsage(tc.known...),
						WithX509SignCSRUnknownExtKeyUsage(tc.oids...))
				} else {
					der, err = NewX509Cert(key, WithX509CertCommonName("private-eku"), WithX509CertParent(ca),
						WithX509CertExtKeyUsage(tc.known...), WithX509CertUnknownExtKeyUsage(tc.oids...))
				}
				require.NoError(t, err)
				cert, err := x509.ParseCertificate(der)
				require.NoError(t, err)
				require.Equal(t, tc.want, cert.ExtKeyUsage)
				require.Equal(t, tc.unknown, cert.UnknownExtKeyUsage)
				require.NoError(t, cert.CheckSignatureFrom(ca))
				var raw []asn1.ObjectIdentifier
				for _, ext := range cert.Extensions {
					if ext.Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 37}) {
						rest, err := asn1.Unmarshal(ext.Value, &raw)
						require.NoError(t, err)
						require.Empty(t, rest)
					}
				}
				require.Len(t, raw, len(tc.known)+len(tc.oids))
			}
		})
	}
}

// TestPrivateEKURawOverride preserves raw EKU precedence in either option order.
func TestPrivateEKURawOverride(t *testing.T) {
	t.Parallel()
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	csrDER, err := NewX509CSR(key, WithX509CSRCommonName("private-eku"))
	require.NoError(t, err)
	typed := asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55555, 1}
	raw := asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55555, 9}
	ext := privateEKUTestExtension(t, raw)
	for _, rawFirst := range []bool{false, true} {
		certOpts := []X509CertOption{WithX509CertUnknownExtKeyUsage(typed), WithX509CertExtraExtensions(ext)}
		signOpts := []SignCSROption{WithX509SignCSRUnknownExtKeyUsage(typed), WithX509SignCSRExtraExtenstions(ext)}
		if rawFirst {
			certOpts[0], certOpts[1] = certOpts[1], certOpts[0]
			signOpts[0], signOpts[1] = signOpts[1], signOpts[0]
		}
		certOpts = append(certOpts, WithX509CertCommonName("private-eku"), WithX509CertExtKeyUsage(x509.ExtKeyUsageServerAuth))
		directDER, err := NewX509Cert(key, certOpts...)
		require.NoError(t, err)
		signOpts = append(signOpts, WithX509SignCSRExtKeyUsage(x509.ExtKeyUsageServerAuth))
		signedDER, err := NewX509CertByCSR(ca, key, csrDER, signOpts...)
		require.NoError(t, err)
		for _, der := range [][]byte{directDER, signedDER} {
			cert, err := x509.ParseCertificate(der)
			require.NoError(t, err)
			require.Equal(t, []asn1.ObjectIdentifier{raw}, cert.UnknownExtKeyUsage)
			require.Empty(t, cert.ExtKeyUsage)
		}
	}
}

// TestPrivateEKUCSRRequestsAreNotAuthorization preserves requests without granting purposes.
func TestPrivateEKUCSRRequestsAreNotAuthorization(t *testing.T) {
	t.Parallel()
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	requested := asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55555, 1}
	authorized := asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55555, 2}
	ext := privateEKUTestExtension(t, requested)
	csrDER, err := NewX509CSR(key, WithX509CSRCommonName("private-eku"), WithX509CSRExtraExtension(ext))
	require.NoError(t, err)
	csr, err := x509.ParseCertificateRequest(csrDER)
	require.NoError(t, err)
	require.NoError(t, csr.CheckSignature())
	require.Contains(t, csr.Extensions, ext)
	require.Empty(t, csr.ExtraExtensions)
	for _, explicit := range []bool{false, true} {
		var opts []SignCSROption
		if explicit {
			opts = append(opts, WithX509SignCSRUnknownExtKeyUsage(authorized))
		}
		der, err := NewX509CertByCSR(ca, key, csrDER, opts...)
		require.NoError(t, err)
		cert, err := x509.ParseCertificate(der)
		require.NoError(t, err)
		require.NoError(t, cert.CheckSignatureFrom(ca))
		require.Empty(t, cert.ExtKeyUsage)
		if explicit {
			require.Equal(t, []asn1.ObjectIdentifier{authorized}, cert.UnknownExtKeyUsage)
		} else {
			require.Empty(t, cert.UnknownExtKeyUsage)
		}
	}
}

// TestPrivateEKUValidation rejects malformed purposes atomically in both option types.
func TestPrivateEKUValidation(t *testing.T) {
	t.Parallel()
	valid := asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55555, 1}
	for _, invalid := range []asn1.ObjectIdentifier{nil, {}, {1}, {3, 1}, {1, 40}, {-1, 1}, {1, 2, -1}} {
		certOpt := &x509V3CertOption{}
		signOpt := &signCSROption{}
		require.NoError(t, WithX509CertUnknownExtKeyUsage(valid)(certOpt))
		require.NoError(t, WithX509SignCSRUnknownExtKeyUsage(valid)(signOpt))
		require.Error(t, WithX509CertUnknownExtKeyUsage(valid, invalid)(certOpt))
		require.Error(t, WithX509SignCSRUnknownExtKeyUsage(valid, invalid)(signOpt))
		require.Equal(t, []asn1.ObjectIdentifier{valid}, certOpt.unknownExtKeyUsage)
		require.Equal(t, []asn1.ObjectIdentifier{valid}, signOpt.unknownExtKeyUsage)
	}
	certOpt := &x509V3CertOption{}
	signOpt := &signCSROption{}
	require.NoError(t, WithX509CertUnknownExtKeyUsage(nil...)(certOpt))
	require.NoError(t, WithX509SignCSRUnknownExtKeyUsage(nil...)(signOpt))
	require.Nil(t, certOpt.unknownExtKeyUsage)
	require.Nil(t, signOpt.unknownExtKeyUsage)
}

// TestPrivateEKUOptionCopies isolates input mutations and reusable option results.
func TestPrivateEKUOptionCopies(t *testing.T) {
	t.Parallel()
	input := []asn1.ObjectIdentifier{{1, 3, 6, 1, 4, 1, 55555, 1}}
	want := []asn1.ObjectIdentifier{append(asn1.ObjectIdentifier(nil), input[0]...)}
	certOption := WithX509CertUnknownExtKeyUsage(input...)
	signOption := WithX509SignCSRUnknownExtKeyUsage(input...)
	input[0][7] = 99
	input[0] = asn1.ObjectIdentifier{1, 2, 3}
	certFirst, certSecond := &x509V3CertOption{}, &x509V3CertOption{}
	signFirst, signSecond := &signCSROption{}, &signCSROption{}
	require.NoError(t, certOption(certFirst))
	require.NoError(t, signOption(signFirst))
	require.Equal(t, want, certFirst.unknownExtKeyUsage)
	require.Equal(t, want, signFirst.unknownExtKeyUsage)
	certFirst.unknownExtKeyUsage[0][7] = 88
	signFirst.unknownExtKeyUsage[0][7] = 77
	require.NoError(t, certOption(certSecond))
	require.NoError(t, signOption(signSecond))
	require.Equal(t, want, certSecond.unknownExtKeyUsage)
	require.Equal(t, want, signSecond.unknownExtKeyUsage)
	_, firstTemplate, err := x509CertOption2Template(WithX509CertCommonName("private-eku"), certOption)
	require.NoError(t, err)
	firstTemplate.UnknownExtKeyUsage[0][7] = 66
	_, secondTemplate, err := x509CertOption2Template(WithX509CertCommonName("private-eku"), certOption)
	require.NoError(t, err)
	require.Equal(t, want, secondTemplate.UnknownExtKeyUsage)
}

// TestPrivateEKUTongsuoBoundary rejects new Go-only options without invoking a binary.
func TestPrivateEKUTongsuoBoundary(t *testing.T) {
	t.Parallel()
	purpose := asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55555, 1}
	der, err := new(Tongsuo).NewX509Cert(context.Background(), nil,
		WithX509CertCommonName("private-eku"), WithX509CertUnknownExtKeyUsage(purpose))
	require.ErrorContains(t, err, "only supported by Go issuance")
	require.Nil(t, der)
	_, conf, err := x509SignCsrOptions2OpensslConf(WithX509SignCSRUnknownExtKeyUsage(purpose))
	require.ErrorContains(t, err, "only supported by Go issuance")
	require.Nil(t, conf)
	_, conf, err = x509SignCsrOptions2OpensslConf(WithX509SignCSRUnknownExtKeyUsage())
	require.NoError(t, err)
	require.NotEmpty(t, conf)
}

// TestPrivateEKULegacyEnumControl retains rejection of undefined enum values.
func TestPrivateEKULegacyEnumControl(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	der, err := NewX509Cert(key, WithX509CertCommonName("private-eku"),
		WithX509CertExtKeyUsage(x509.ExtKeyUsage(1000)))
	require.ErrorContains(t, err, "unknown extended key usage")
	require.Nil(t, der)
}

// TestPrivateEKURawExtensionControl proves Go and the wrapper can encode private purposes.
func TestPrivateEKURawExtensionControl(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	purpose := asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55555, 1}
	der, err := NewX509Cert(key, WithX509CertCommonName("private-eku"),
		WithX509CertExtraExtensions(privateEKUTestExtension(t, purpose)))
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	require.Equal(t, []asn1.ObjectIdentifier{purpose}, cert.UnknownExtKeyUsage)
	require.Empty(t, cert.ExtKeyUsage)
	require.NoError(t, cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature))
}
