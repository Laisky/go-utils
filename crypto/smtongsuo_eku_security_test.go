package crypto

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emmansun/gmsm/smx509"
	"github.com/stretchr/testify/require"
)

// TestX509SignCsrOptionsExtKeyUsageAnyIsExact verifies that requesting
// x509.ExtKeyUsageAny emits only the anyExtendedKeyUsage OID instead of an
// enumeration of concrete, sensitive usages. Regression for issue #39.
func TestX509SignCsrOptionsExtKeyUsageAnyIsExact(t *testing.T) {
	t.Parallel()

	_, conf, err := x509SignCsrOptions2OpensslConf(WithX509SignCSRExtKeyUsage(x509.ExtKeyUsageAny))
	require.NoError(t, err)
	require.Contains(t, string(conf), "extendedKeyUsage = anyExtendedKeyUsage\n")
	for _, known := range opensslExtKeyUsages {
		if known.usage == x509.ExtKeyUsageAny {
			continue
		}
		require.NotContains(t, string(conf), known.token, "Any must not expand into %s", known.token)
	}
	for _, legacy := range []string{"timestamping", "ocspSigning", "microsoftServerGatedCrypto"} {
		require.NotContains(t, string(conf), legacy)
	}
}

// TestX509SignCsrOptionsExplicitExtKeyUsagesAreExact verifies that explicit
// single and multiple EKU requests are emitted exactly, deterministically and
// without duplicates, and that unknown EKU values fail closed. Regression for
// issue #39.
func TestX509SignCsrOptionsExplicitExtKeyUsagesAreExact(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		usage []x509.ExtKeyUsage
		want  string
	}{
		{"single", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, "extendedKeyUsage = serverAuth\n"},
		{"multiple unordered with duplicate", []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth,
		}, "extendedKeyUsage = serverAuth, clientAuth\n"},
		{"any plus explicit", []x509.ExtKeyUsage{
			x509.ExtKeyUsageCodeSigning, x509.ExtKeyUsageAny,
		}, "extendedKeyUsage = anyExtendedKeyUsage, codeSigning\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for range 3 { // deterministic across invocations
				_, conf, err := x509SignCsrOptions2OpensslConf(WithX509SignCSRExtKeyUsage(tc.usage...))
				require.NoError(t, err)
				require.Contains(t, string(conf), tc.want)
				require.Equal(t, 1, strings.Count(string(conf), "extendedKeyUsage"))
			}
		})
	}

	t.Run("unknown usage fails closed", func(t *testing.T) {
		t.Parallel()
		_, conf, err := x509SignCsrOptions2OpensslConf(
			WithX509SignCSRExtKeyUsage(x509.ExtKeyUsageServerAuth, x509.ExtKeyUsage(9999)))
		require.Error(t, err)
		require.Nil(t, conf)
	})
}

// smExtKeyUsagesForTest converts crypto/x509 EKU values into the matching
// smx509 values used to compare against parsed SM2 certificates.
func smExtKeyUsagesForTest(usages ...x509.ExtKeyUsage) []smx509.ExtKeyUsage {
	out := make([]smx509.ExtKeyUsage, 0, len(usages))
	for _, u := range usages {
		out = append(out, smx509.ExtKeyUsage(u))
	}
	return out
}

// tongsuoVerifyPurposeForTest runs `tongsuo verify -purpose` for leafDer under
// the trust anchor caDer and returns the verifier error, if any.
func tongsuoVerifyPurposeForTest(t *testing.T, ins *Tongsuo, purpose string, caDer, leafDer []byte) error {
	t.Helper()

	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	leafPath := filepath.Join(dir, "leaf.pem")
	require.NoError(t, os.WriteFile(caPath, CertDer2Pem(caDer), 0o600))
	require.NoError(t, os.WriteFile(leafPath, CertDer2Pem(leafDer), 0o600))

	_, err := ins.runCMD(t.Context(), []string{
		"verify", "-purpose", purpose, "-CAfile", caPath, leafPath,
	}, nil)
	return err
}

// TestTongsuoNewX509CertByCSRExtKeyUsageDER issues real certificates with the
// tongsuo binary and inspects the DER EKU OIDs: Any must stay exactly the
// anyExtendedKeyUsage OID and explicit requests must stay exact. It also
// records how the Tongsuo purpose verifier treats Any versus serverAuth.
// Regression for issue #39.
func TestTongsuoNewX509CertByCSRExtKeyUsageDER(t *testing.T) {
	t.Parallel()
	ins := newSecurityTestTongsuo(t)
	ctx := t.Context()

	version, err := ins.runCMD(ctx, []string{"version"}, nil)
	require.NoError(t, err)
	t.Logf("tongsuo version: %s", strings.TrimSpace(string(version)))

	caKey, caDer := newTongsuoSM2CAForTest(t, ins, "eku-root")
	csrDer := newTongsuoSM2CSRForTest(t, ins, WithX509CSRCommonName("eku-leaf"))

	cases := []struct {
		name  string
		usage []x509.ExtKeyUsage
	}{
		{"any", []x509.ExtKeyUsage{x509.ExtKeyUsageAny}},
		{"server auth", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}},
		{"server and client auth", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}},
	}
	issued := map[string][]byte{}
	for _, tc := range cases {
		leafDer, err := ins.NewX509CertByCSR(ctx, caDer, caKey, csrDer,
			WithX509SignCSRExtKeyUsage(tc.usage...))
		require.NoError(t, err, tc.name)

		leaf := parseSMCertForTest(t, leafDer)
		require.Equal(t, smExtKeyUsagesForTest(tc.usage...), leaf.ExtKeyUsage, tc.name)
		require.Empty(t, leaf.UnknownExtKeyUsage, tc.name)
		issued[tc.name] = leafDer
	}

	// OpenSSL-derived purpose checks do not treat anyExtendedKeyUsage as
	// serverAuth; that is exactly why Any must never be silently expanded.
	require.Error(t, tongsuoVerifyPurposeForTest(t, ins, "sslserver", caDer, issued["any"]))
	require.NoError(t, tongsuoVerifyPurposeForTest(t, ins, "sslserver", caDer, issued["server auth"]))
}

// TestTongsuoNewX509CertExtKeyUsageDER verifies the self-signed issuance path
// honors explicit EKU requests exactly and keeps Any as the single
// anyExtendedKeyUsage OID. Regression for issue #39.
func TestTongsuoNewX509CertExtKeyUsageDER(t *testing.T) {
	t.Parallel()
	ins := newSecurityTestTongsuo(t)
	ctx := t.Context()

	cases := []struct {
		name  string
		usage []x509.ExtKeyUsage
		want  []x509.ExtKeyUsage
	}{
		{"default leaf keeps any", nil, []x509.ExtKeyUsage{x509.ExtKeyUsageAny}},
		{"explicit any", []x509.ExtKeyUsage{x509.ExtKeyUsageAny}, []x509.ExtKeyUsage{x509.ExtKeyUsageAny}},
		{"explicit server auth", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}},
		{"explicit client and server auth",
			[]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
			[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts := []X509CertOption{WithX509CertCommonName("eku-self-signed")}
			if len(tc.usage) != 0 {
				opts = append(opts, WithX509CertExtKeyUsage(tc.usage...))
			}

			_, certDer, err := ins.NewPrikeyAndCert(ctx, opts...)
			require.NoError(t, err)

			cert := parseSMCertForTest(t, certDer)
			require.Equal(t, smExtKeyUsagesForTest(tc.want...), cert.ExtKeyUsage)
			require.Empty(t, cert.UnknownExtKeyUsage)
		})
	}
}

// TestTongsuoExtKeyUsageTokensEncodeExactOIDs requests every supported
// concrete EKU through the real tongsuo binary and requires each OpenSSL
// token to encode exactly the intended OID. Regression for issue #39.
func TestTongsuoExtKeyUsageTokensEncodeExactOIDs(t *testing.T) {
	t.Parallel()
	ins := newSecurityTestTongsuo(t)
	ctx := t.Context()

	var all []x509.ExtKeyUsage
	for _, known := range opensslExtKeyUsages {
		if known.usage != x509.ExtKeyUsageAny {
			all = append(all, known.usage)
		}
	}

	caKey, caDer := newTongsuoSM2CAForTest(t, ins, "eku-all-root")
	csrDer := newTongsuoSM2CSRForTest(t, ins, WithX509CSRCommonName("eku-all-leaf"))

	leafDer, err := ins.NewX509CertByCSR(ctx, caDer, caKey, csrDer, WithX509SignCSRExtKeyUsage(all...))
	require.NoError(t, err)
	require.Equal(t, smExtKeyUsagesForTest(all...), parseSMCertForTest(t, leafDer).ExtKeyUsage)

	_, selfDer, err := ins.NewPrikeyAndCert(ctx,
		WithX509CertCommonName("eku-all-self"), WithX509CertExtKeyUsage(all...))
	require.NoError(t, err)
	require.Equal(t, smExtKeyUsagesForTest(all...), parseSMCertForTest(t, selfDer).ExtKeyUsage)
}

// TestTongsuoExplicitExtKeyUsageOverridesCSRRequest verifies that an explicit
// signer EKU request replaces an EKU extension requested inside the CSR, so the
// issued certificate carries exactly the signer's usages. Regression for issue
// #39.
func TestTongsuoExplicitExtKeyUsageOverridesCSRRequest(t *testing.T) {
	t.Parallel()
	ins := newSecurityTestTongsuo(t)
	ctx := t.Context()

	caKey, caDer := newTongsuoSM2CAForTest(t, ins, "eku-override-root")

	leafKey, err := NewECDSAPrikey(ECDSACurveP256)
	require.NoError(t, err)
	codeSigningEKU, err := asn1.Marshal([]asn1.ObjectIdentifier{{1, 3, 6, 1, 5, 5, 7, 3, 3}})
	require.NoError(t, err)
	csrDer, err := NewX509CSR(leafKey,
		WithX509CSRCommonName("eku-override-leaf"),
		WithX509CSRExtraExtension(pkix.Extension{Id: asn1.ObjectIdentifier{2, 5, 29, 37}, Value: codeSigningEKU}),
	)
	require.NoError(t, err)

	leafDer, err := ins.NewX509CertByCSR(ctx, caDer, caKey, csrDer,
		WithX509SignCSRExtKeyUsage(x509.ExtKeyUsageServerAuth))
	require.NoError(t, err)

	leaf, err := x509.ParseCertificate(leafDer)
	require.NoError(t, err)
	require.Equal(t, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, leaf.ExtKeyUsage)
	require.Empty(t, leaf.UnknownExtKeyUsage)
}
