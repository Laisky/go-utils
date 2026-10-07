package crypto

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	// oidNetscapeCommentForTest is the Netscape Comment extension OID used by
	// the display-spoofing fixtures.
	oidNetscapeCommentForTest = asn1.ObjectIdentifier{2, 16, 840, 1, 113730, 1, 13}
	// oidUnknownExtensionForTest is a private, unregistered extension OID.
	oidUnknownExtensionForTest = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55555, 7, 1}
)

// mustMarshalASN1ForTest DER-encodes v as an ASN.1 value with the given params
// and fails the test on error.
func mustMarshalASN1ForTest(t *testing.T, v any, params string) []byte {
	t.Helper()

	out, err := asn1.MarshalWithParams(v, params)
	require.NoError(t, err)
	return out
}

// TestTongsuoShowCertInfoAdversarialFixtures verifies that certificate metadata
// returned by ShowCertInfo comes from DER fields, never from display text that
// subject or extension values can forge. Regression for issue #61.
func TestTongsuoShowCertInfoAdversarialFixtures(t *testing.T) {
	t.Parallel()
	ins := newSecurityTestTongsuo(t)
	ctx := t.Context()

	t.Run("fixture 1: CA text in subject of a leaf", func(t *testing.T) {
		t.Parallel()
		der := newGoFixtureCertForTest(t, &x509.Certificate{
			Subject: pkix.Name{
				CommonName:         "CA: TRUE",
				OrganizationalUnit: []string{"CN = decoy-ou"},
			},
			BasicConstraintsValid: true,
			IsCA:                  false,
		}, nil)

		_, cert, err := ins.ShowCertInfo(ctx, der)
		require.NoError(t, err)
		require.False(t, cert.IsCA, "subject text must not synthesize CA status")
		require.Equal(t, "CA: TRUE", cert.Subject.CommonName)
	})

	t.Run("fixture 2: fake subject inside netscape comment", func(t *testing.T) {
		t.Parallel()
		der := newGoFixtureCertForTest(t, &x509.Certificate{
			Subject: pkix.Name{Organization: []string{"fixture-org"}},
			ExtraExtensions: []pkix.Extension{{
				Id:    oidNetscapeCommentForTest,
				Value: mustMarshalASN1ForTest(t, "Subject: CN = test-admin, X = y", "ia5"),
			}},
		}, nil)

		_, cert, err := ins.ShowCertInfo(ctx, der)
		require.NoError(t, err)
		require.Empty(t, cert.Subject.CommonName, "comment text must not synthesize a common name")
		require.Equal(t, []string{"fixture-org"}, cert.Subject.Organization)
	})

	t.Run("fixture 3: policy text in common name", func(t *testing.T) {
		t.Parallel()
		der := newGoFixtureCertForTest(t, &x509.Certificate{
			Subject: pkix.Name{
				CommonName:         "foo Policy: 1.2.3",
				OrganizationalUnit: []string{"CN = decoy-ou"},
			},
		}, nil)

		_, cert, err := ins.ShowCertInfo(ctx, der)
		require.NoError(t, err)
		require.Empty(t, cert.Policies, "subject text must not synthesize policies")
		require.Empty(t, cert.PolicyIdentifiers)
	})

	t.Run("fixture 4: usage text inside unknown extension", func(t *testing.T) {
		t.Parallel()
		fake := "\nX509v3 Key Usage: critical\n    Digital Signature, Certificate Sign\n" +
			"X509v3 Extended Key Usage:\n    TLS Web Server Authentication, Code Signing\n"
		der := newGoFixtureCertForTest(t, &x509.Certificate{
			Subject: pkix.Name{
				CommonName:         "fixture-usage",
				OrganizationalUnit: []string{"CN = decoy-ou"},
			},
			ExtraExtensions: []pkix.Extension{{
				Id:    oidUnknownExtensionForTest,
				Value: mustMarshalASN1ForTest(t, fake, "utf8"),
			}},
		}, nil)

		certinfo, cert, err := ins.ShowCertInfo(ctx, der)
		require.NoError(t, err)
		require.Contains(t, certinfo, "X509v3 Key Usage", "fixture must render the forged text")
		require.Zero(t, cert.KeyUsage, "extension text must not synthesize key usages")
		require.Empty(t, cert.ExtKeyUsage, "extension text must not synthesize ext key usages")
	})

	t.Run("fixture 5: SM2 is not reported as ECDSA", func(t *testing.T) {
		t.Parallel()
		_, der := newTongsuoSM2CAForTest(t, ins, "fixture-sm2")

		certinfo, cert, err := ins.ShowCertInfo(ctx, der)
		require.NoError(t, err)
		require.Contains(t, certinfo, "SM2")
		require.NotEqual(t, x509.ECDSA, cert.PublicKeyAlgorithm, "SM2 must not be labelled ECDSA")
		require.Equal(t, "fixture-sm2", cert.Subject.CommonName)
		require.True(t, cert.IsCA)
	})

	t.Run("unknown public key algorithm fails closed", func(t *testing.T) {
		t.Parallel()
		xkey, err := ecdh.X25519().GenerateKey(rand.Reader)
		require.NoError(t, err)
		spki, err := x509.MarshalPKIXPublicKey(xkey.PublicKey())
		require.NoError(t, err)
		der := newHandBuiltCertForTest(t, spki, pkix.Name{
			CommonName:         "fixture-x25519",
			OrganizationalUnit: []string{"CN = decoy-ou"},
		})

		_, cert, err := ins.ShowCertInfo(ctx, der)
		require.Error(t, err, "unsupported algorithms must not return ambiguous success")
		require.Nil(t, cert)
	})
}

// TestTongsuoShowCertInfoAgreesWithNativeParser verifies that supported RSA,
// ECDSA and Ed25519 certificates, including multi-valued and Unicode subjects,
// missing optional fields and critical unknown extensions, are returned exactly
// as crypto/x509 parses them. Regression for issue #61.
func TestTongsuoShowCertInfoAgreesWithNativeParser(t *testing.T) {
	t.Parallel()
	ins := newSecurityTestTongsuo(t)
	ctx := t.Context()

	rich := &x509.Certificate{
		Subject: pkix.Name{
			CommonName:         "Unicode 中文 Grüße",
			Organization:       []string{"org-one", "org-two"},
			OrganizationalUnit: []string{"ou, with = separators"},
		},
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageAny},
		PolicyIdentifiers:     []asn1.ObjectIdentifier{{1, 2, 3, 4}, {2, 23, 140, 1, 2, 1}},
		DNSNames:              []string{"a.example", "b.example"},
		EmailAddresses:        []string{"user@example.com"},
		ExtraExtensions: []pkix.Extension{{
			Id:       oidUnknownExtensionForTest,
			Critical: true,
			Value:    mustMarshalASN1ForTest(t, "critical unknown", "utf8"),
		}},
	}

	rsaKey, err := NewRSAPrikey(RSAPrikeyBits2048)
	require.NoError(t, err)
	edKey, err := NewEd25519Prikey()
	require.NoError(t, err)

	cases := []struct {
		name string
		der  []byte
		algo TongsuoPublicKeyAlgorithm
	}{
		{"ecdsa rich", newGoFixtureCertForTest(t, rich, nil), TongsuoPublicKeyAlgorithmECDSA},
		{"rsa minimal", newGoFixtureCertForTest(t, &x509.Certificate{
			Subject: pkix.Name{CommonName: "rsa-minimal"},
		}, rsaKey.Public()), TongsuoPublicKeyAlgorithmRSA},
		{"ed25519 without common name", newGoFixtureCertForTest(t, &x509.Certificate{
			Subject:  pkix.Name{Organization: []string{"no-cn"}},
			DNSNames: []string{"ed.example"},
		}, edKey.Public()), TongsuoPublicKeyAlgorithmEd25519},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			native, err := x509.ParseCertificate(tc.der)
			require.NoError(t, err)

			certinfo, cert, err := ins.ShowCertInfo(ctx, tc.der)
			require.NoError(t, err)
			require.NotEmpty(t, certinfo)
			require.Equal(t, native, cert)

			detail, err := ins.ShowCertInfoDetail(ctx, tc.der)
			require.NoError(t, err)
			require.Equal(t, tc.algo, detail.PublicKeyAlgorithm)
			require.False(t, detail.IsSM2())
			require.Equal(t, native.PublicKey, detail.PublicKey)
			require.Equal(t, native.SignatureAlgorithm.String(), detail.SignatureAlgorithm)
		})
	}
}

// TestParseTongsuoCertInfoSM2MatchesStructuredParser verifies the SM2 view is
// copied field by field from the SM2-capable DER parser and that SM2 is
// identified explicitly. Regression for issue #61.
func TestParseTongsuoCertInfoSM2MatchesStructuredParser(t *testing.T) {
	t.Parallel()
	ins := newSecurityTestTongsuo(t)

	_, der, err := ins.NewPrikeyAndCert(t.Context(),
		WithX509CertCommonName("sm2-structured"),
		WithX509CertOrganization("sm2 org"),
		WithX509CertIsCA(),
		WithX509CertPolicies(asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 59936, 1, 1, 3}),
	)
	require.NoError(t, err)
	sm := parseSMCertForTest(t, der)

	info, err := ParseTongsuoCertInfo(der)
	require.NoError(t, err)
	require.True(t, info.IsSM2())
	require.Empty(t, info.Display)
	require.Equal(t, sm.PublicKey, info.PublicKey)
	require.Equal(t, "SM2-SM3", info.SignatureAlgorithm)

	cert := info.Certificate
	require.Equal(t, x509.UnknownPublicKeyAlgorithm, cert.PublicKeyAlgorithm)
	require.Nil(t, cert.PublicKey)
	require.Equal(t, x509.UnknownSignatureAlgorithm, cert.SignatureAlgorithm)
	require.Equal(t, der, cert.Raw)
	require.Equal(t, sm.Subject, cert.Subject)
	require.Equal(t, sm.Issuer, cert.Issuer)
	require.Equal(t, 0, sm.SerialNumber.Cmp(cert.SerialNumber))
	require.True(t, sm.NotBefore.Equal(cert.NotBefore))
	require.True(t, sm.NotAfter.Equal(cert.NotAfter))
	require.True(t, cert.IsCA)
	require.Equal(t, x509.KeyUsageCertSign|x509.KeyUsageCRLSign, cert.KeyUsage)
	require.Equal(t, sm.SubjectKeyId, cert.SubjectKeyId)
	require.Equal(t, sm.AuthorityKeyId, cert.AuthorityKeyId)
	require.Equal(t, []asn1.ObjectIdentifier{{1, 3, 6, 1, 4, 1, 59936, 1, 1, 3}}, cert.PolicyIdentifiers)
	require.Len(t, cert.Policies, 1)
	require.Equal(t, "1.3.6.1.4.1.59936.1.1.3", cert.Policies[0].String())
}

// TestSmx509CertificateToX509Fidelity verifies the field-by-field conversion
// used for SM2 certificates reproduces crypto/x509 exactly (apart from the
// deliberately omitted public key) on a certificate both parsers understand.
// Regression for issue #61.
func TestSmx509CertificateToX509Fidelity(t *testing.T) {
	t.Parallel()

	der := newGoFixtureCertForTest(t, &x509.Certificate{
		Subject:               pkix.Name{CommonName: "fidelity", Organization: []string{"a", "b"}},
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            2,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageAny, x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageCodeSigning, x509.ExtKeyUsageEmailProtection, x509.ExtKeyUsageIPSECEndSystem,
			x509.ExtKeyUsageIPSECTunnel, x509.ExtKeyUsageIPSECUser, x509.ExtKeyUsageTimeStamping,
			x509.ExtKeyUsageOCSPSigning, x509.ExtKeyUsageMicrosoftServerGatedCrypto,
			x509.ExtKeyUsageNetscapeServerGatedCrypto, x509.ExtKeyUsageMicrosoftCommercialCodeSigning,
			x509.ExtKeyUsageMicrosoftKernelCodeSigning,
		},
		UnknownExtKeyUsage:    []asn1.ObjectIdentifier{{1, 3, 6, 1, 4, 1, 55555, 9}},
		PolicyIdentifiers:     []asn1.ObjectIdentifier{{1, 2, 3}},
		DNSNames:              []string{"fidelity.example"},
		PermittedDNSDomains:   []string{"example"},
		CRLDistributionPoints: []string{"http://crl.example/ca.crl"},
		OCSPServer:            []string{"http://ocsp.example"},
	}, nil)

	native, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	sm := parseSMCertForTest(t, der)

	view, err := smx509CertificateToX509(sm)
	require.NoError(t, err)

	expected := *native
	expected.PublicKeyAlgorithm = x509.UnknownPublicKeyAlgorithm
	expected.PublicKey = nil
	require.Equal(t, &expected, view)
}

// TestParseTongsuoCertInfoFailsClosed verifies malformed DER, trailing data and
// unsupported algorithms return errors (with a sentinel for algorithms) and no
// partial result. Regression for issue #61.
func TestParseTongsuoCertInfoFailsClosed(t *testing.T) {
	t.Parallel()

	valid := newGoFixtureCertForTest(t, &x509.Certificate{Subject: pkix.Name{CommonName: "valid"}}, nil)

	xkey, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	spki, err := x509.MarshalPKIXPublicKey(xkey.PublicKey())
	require.NoError(t, err)
	unsupported := newHandBuiltCertForTest(t, spki, pkix.Name{CommonName: "x25519"})

	info, err := ParseTongsuoCertInfo(unsupported)
	require.ErrorIs(t, err, ErrTongsuoUnsupportedPublicKeyAlgorithm)
	require.Nil(t, info)

	for name, der := range map[string][]byte{
		"empty":         nil,
		"garbage":       []byte("not a certificate"),
		"trailing data": append(append([]byte{}, valid...), 0x00),
		"pem":           CertDer2Pem(valid),
		"truncated":     valid[:len(valid)/2],
	} {
		info, err := ParseTongsuoCertInfo(der)
		require.Error(t, err, name)
		require.Nil(t, info, name)
	}
}

// TestTongsuo_ShowCertInfo checks ShowCertInfo against native parsing for
// RSA, ECDSA and Ed25519 and the explicit SM2 contract (issue #61).
func TestTongsuo_ShowCertInfo(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	ctx := context.Background()
	ins, err := NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	sno, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
	require.NoError(t, err)

	t.Run("test pubkey algorithm", func(t *testing.T) {
		t.Run("rsa", func(t *testing.T) {
			_, certDer, err := NewRSAPrikeyAndCert(RSAPrikeyBits2048,
				WithX509CertCommonName("test-rsa"),
				WithX509CertSeriaNumber(sno),
				WithX509CertKeyUsage(
					x509.KeyUsageDigitalSignature,
					x509.KeyUsageContentCommitment,
					x509.KeyUsageKeyEncipherment,
					x509.KeyUsageDataEncipherment,
					x509.KeyUsageKeyAgreement,
					x509.KeyUsageCertSign,
					x509.KeyUsageCRLSign,
					x509.KeyUsageEncipherOnly,
					x509.KeyUsageDecipherOnly,
				),
				WithX509CertExtKeyUsage(
					x509.ExtKeyUsageAny,
					x509.ExtKeyUsageServerAuth,
					x509.ExtKeyUsageClientAuth,
					x509.ExtKeyUsageCodeSigning,
					x509.ExtKeyUsageEmailProtection,
					x509.ExtKeyUsageIPSECEndSystem,
					x509.ExtKeyUsageIPSECTunnel,
					x509.ExtKeyUsageIPSECUser,
					x509.ExtKeyUsageTimeStamping,
					x509.ExtKeyUsageOCSPSigning,
					x509.ExtKeyUsageMicrosoftServerGatedCrypto,
					x509.ExtKeyUsageNetscapeServerGatedCrypto,
					x509.ExtKeyUsageMicrosoftCommercialCodeSigning,
					x509.ExtKeyUsageMicrosoftKernelCodeSigning,
				),
			)

			rawCert, err := Der2Cert(certDer)
			require.NoError(t, err)

			certinfo, cert, err := ins.ShowCertInfo(ctx, certDer)
			require.NoError(t, err, certinfo)

			t.Log(certinfo)
			require.Equal(t, x509.RSA, cert.PublicKeyAlgorithm)
			require.Equal(t, sno, cert.SerialNumber)
			require.Equal(t, rawCert.SubjectKeyId, cert.SubjectKeyId)
			require.Equal(t, rawCert.AuthorityKeyId, cert.AuthorityKeyId)
			require.Equal(t, rawCert.KeyUsage, cert.KeyUsage)
			require.Equal(t, rawCert.ExtKeyUsage, cert.ExtKeyUsage)
		})

		t.Run("ecdsa", func(t *testing.T) {
			_, certDer, err := NewECDSAPrikeyAndCert(ECDSACurveP256,
				WithX509CertCommonName("test-ecdsa"),
				WithX509CertSeriaNumber(sno),
			)
			require.NoError(t, err)

			_, cert, err := ins.ShowCertInfo(ctx, certDer)
			require.NoError(t, err)

			require.Equal(t, x509.ECDSA, cert.PublicKeyAlgorithm)
			require.Equal(t, sno, cert.SerialNumber)
		})

		t.Run("ed25519", func(t *testing.T) {
			_, certDer, err := NewEd25519PrikeyAndCert(
				WithX509CertCommonName("test-ed25519"),
				WithX509CertSeriaNumber(sno),
			)
			require.NoError(t, err)

			_, cert, err := ins.ShowCertInfo(ctx, certDer)
			require.NoError(t, err)

			require.Equal(t, x509.Ed25519, cert.PublicKeyAlgorithm)
			require.Equal(t, sno, cert.SerialNumber)
		})

		t.Run("sm2", func(t *testing.T) {
			certinfo, certDer, err := ins.NewPrikeyAndCert(ctx,
				WithX509CertCommonName("test-sm2"),
				WithX509CertSeriaNumber(sno),
			)
			require.NoError(t, err)

			_, cert, err := ins.ShowCertInfo(ctx, certDer)
			require.NoError(t, err)

			// SM2 is never labelled as plain ECDSA (issue #61)
			require.Equal(t, x509.UnknownPublicKeyAlgorithm, cert.PublicKeyAlgorithm)
			require.Nil(t, cert.PublicKey)
			require.Equal(t, sno, cert.SerialNumber, certinfo)

			detail, err := ins.ShowCertInfoDetail(ctx, certDer)
			require.NoError(t, err)
			require.True(t, detail.IsSM2())
			require.Equal(t, TongsuoPublicKeyAlgorithmSM2, detail.PublicKeyAlgorithm)
			require.Equal(t, "SM2-SM3", detail.SignatureAlgorithm)
		})
	})
}
