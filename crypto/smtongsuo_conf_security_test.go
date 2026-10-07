package crypto

import (
	"bytes"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emmansun/gmsm/smx509"
	"github.com/stretchr/testify/require"
)

const (
	// confCanaryEnvForTest is the synthetic environment variable whose value
	// must never reach a Tongsuo artifact through caller-controlled fields.
	confCanaryEnvForTest = "GO_UTILS_TEST_MARKER"
	// confCanaryValueForTest is the synthetic canary value.
	confCanaryValueForTest = "synthetic-marker"
)

// literalConfValuesForTest lists subject values that OpenSSL configuration
// syntax would otherwise interpret: variable expansion, quoting, escaping,
// comments, padding, undefined variables and Unicode.
func literalConfValuesForTest() []string {
	return []string{
		"${ENV::" + confCanaryEnvForTest + "}",
		"$ENV::" + confCanaryEnvForTest,
		"$(ENV::" + confCanaryEnvForTest + ")",
		"$req::prompt",
		"${req::distinguished_name}",
		"${ENV::GO_UTILS_TEST_UNDEFINED_VARIABLE}",
		`"double quoted"`,
		`'single quoted'`,
		"`backtick`",
		`back\slash`,
		`trailing backslash\`,
		`escaped \n not newline`,
		"hash # comment",
		"#leading-hash",
		"  padded value  ",
		"dollar at end $",
		"semi;colon=equals,comma",
		"Unicode 中文 Grüße ✓",
		".",
	}
}

// requireNoCanaryForTest requires that neither data nor err mention the canary.
func requireNoCanaryForTest(t *testing.T, data []byte, err error) {
	t.Helper()

	require.NotContains(t, string(data), confCanaryValueForTest)
	if err != nil {
		require.NotContains(t, err.Error(), confCanaryValueForTest)
	}
}

// TestTongsuoNewX509CSRSubjectValuesAreLiteral generates real CSRs whose
// subject and SAN values contain OpenSSL configuration syntax and requires the
// parsed CSR to carry exactly the literal text, never the environment canary.
// Regression for issue #48.
func TestTongsuoNewX509CSRSubjectValuesAreLiteral(t *testing.T) {
	t.Setenv(confCanaryEnvForTest, confCanaryValueForTest)
	ins := newSecurityTestTongsuo(t)
	ctx := t.Context()

	prikeyPem, err := ins.NewPrikey(ctx)
	require.NoError(t, err)

	for _, value := range literalConfValuesForTest() {
		csrDer, err := ins.NewX509CSR(ctx, prikeyPem,
			WithX509CSRCommonName(value),
			WithX509CSROrganization(value),
			WithX509CSROrganizationUnit(value, "second-ou"),
		)
		requireNoCanaryForTest(t, csrDer, err)
		require.NoError(t, err, "value %q", value)

		csr, err := smx509.ParseCertificateRequest(csrDer)
		require.NoError(t, err)
		require.NoError(t, csr.CheckSignature())
		require.Equal(t, value, csr.Subject.CommonName)
		require.Equal(t, []string{value}, csr.Subject.Organization)
		require.Equal(t, []string{value, "second-ou"}, csr.Subject.OrganizationalUnit)
	}
}

// TestTongsuoNewX509CertSubjectValuesAreLiteral issues real self-signed
// certificates whose subject values contain OpenSSL configuration syntax and
// requires the parsed certificate to carry exactly the literal text.
// Regression for issue #48.
func TestTongsuoNewX509CertSubjectValuesAreLiteral(t *testing.T) {
	t.Setenv(confCanaryEnvForTest, confCanaryValueForTest)
	ins := newSecurityTestTongsuo(t)
	ctx := t.Context()

	prikeyPem, err := ins.NewPrikey(ctx)
	require.NoError(t, err)

	for _, value := range literalConfValuesForTest() {
		certDer, err := ins.NewX509Cert(ctx, prikeyPem,
			WithX509CertCommonName(value),
			WithX509CertOrganization(value),
			WithX509CertLocality(value),
		)
		requireNoCanaryForTest(t, certDer, err)
		require.NoError(t, err, "value %q", value)

		cert := parseSMCertForTest(t, certDer)
		require.Equal(t, value, cert.Subject.CommonName)
		require.Equal(t, []string{value}, cert.Subject.Organization)
		require.Equal(t, []string{value}, cert.Subject.Locality)
	}
}

// TestTongsuoSANValuesAreLiteral requires SAN values to be emitted as exactly
// one literal entry each: separators cannot add entries, variables cannot
// expand, and OpenSSL's special email keywords cannot change semantics.
// Regression for issue #48.
func TestTongsuoSANValuesAreLiteral(t *testing.T) {
	t.Setenv(confCanaryEnvForTest, confCanaryValueForTest)
	ins := newSecurityTestTongsuo(t)
	ctx := t.Context()

	prikeyPem, err := ins.NewPrikey(ctx)
	require.NoError(t, err)

	dnsNames := []string{
		"a.example,DNS:injected.example",
		"${ENV::" + confCanaryEnvForTest + "}.example",
		"hash#frag.example",
	}
	emails := []string{"user@example.com,email:injected@example.com"}

	csrDer, err := ins.NewX509CSR(ctx, prikeyPem,
		WithX509CSRCommonName("san-literal"),
		WithX509CSRDNSNames(dnsNames...),
		WithX509CSREmailAddrs(emails...),
	)
	requireNoCanaryForTest(t, csrDer, err)
	require.NoError(t, err)

	csr, err := smx509.ParseCertificateRequest(csrDer)
	require.NoError(t, err)
	require.Equal(t, dnsNames, csr.DNSNames)
	require.Equal(t, emails, csr.EmailAddresses)

	t.Run("openssl email keywords are rejected", func(t *testing.T) {
		for _, keyword := range []string{"copy", "move"} {
			csrDer, err := ins.NewX509CSR(ctx, prikeyPem,
				WithX509CSRCommonName("san-keyword"),
				WithX509CSREmailAddrs(keyword),
			)
			require.Error(t, err, keyword)
			require.Nil(t, csrDer)
		}
	})
}

// TestTongsuoSubprocessEnvironmentIsAllowlisted runs tongsuo with a config that
// deliberately references the canary through ${ENV::...} without any encoding,
// proving that inherited process environment never reaches the subprocess
// while explicitly passed variables still do. Regression for issue #48.
func TestTongsuoSubprocessEnvironmentIsAllowlisted(t *testing.T) {
	t.Setenv(confCanaryEnvForTest, confCanaryValueForTest)
	ins := newSecurityTestTongsuo(t)
	ctx := t.Context()

	prikeyPem, err := ins.NewPrikey(ctx)
	require.NoError(t, err)

	run := func(t *testing.T, envName string, extraEnv []string) ([]byte, error) {
		t.Helper()
		dir := t.TempDir()
		confPath := filepath.Join(dir, "raw.cnf")
		outPath := filepath.Join(dir, "csr.der")
		conf := strings.Join([]string{
			"[ req ]",
			"distinguished_name = dn",
			"prompt = no",
			"[ dn ]",
			"commonName = ${ENV::" + envName + "}",
			"",
		}, "\n")
		require.NoError(t, os.WriteFile(confPath, []byte(conf), 0o600))

		_, err := ins.runCMDWithEnv(ctx, []string{
			"req", "-new", "-outform", "DER", "-out", outPath,
			"-key", "/dev/stdin", "-sm3", "-config", confPath,
		}, prikeyPem, extraEnv)
		if err != nil {
			return nil, err
		}
		return os.ReadFile(outPath)
	}

	t.Run("inherited variable is not visible", func(t *testing.T) {
		csrDer, err := run(t, confCanaryEnvForTest, nil)
		requireNoCanaryForTest(t, csrDer, err)
		require.Error(t, err)
	})

	t.Run("explicit extra variable is visible", func(t *testing.T) {
		csrDer, err := run(t, "_TONGSUO_TEST_EXPLICIT", []string{"_TONGSUO_TEST_EXPLICIT=explicit-value"})
		require.NoError(t, err)
		csr, err := smx509.ParseCertificateRequest(csrDer)
		require.NoError(t, err)
		require.Equal(t, "explicit-value", csr.Subject.CommonName)
	})

	t.Run("password env path still works", func(t *testing.T) {
		encrypted, err := ins.NewPrikeyWithPassword(ctx, "synthetic-password")
		require.NoError(t, err)
		require.Contains(t, string(encrypted), "ENCRYPTED")
	})
}

// TestOpensslConfEncoderValues verifies the literal encoding of individual
// values: plain values stay unchanged, anything the OpenSSL parser would
// interpret is quoted with backslash and quote escaped, and the strict
// encoder rejects control characters while the legacy one strips them.
// Regression for issue #48.
func TestOpensslConfEncoderValues(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"example.com":                  "example.com",
		"Acme Corp, Inc.":              "Acme Corp, Inc.",
		"中文":                           "中文",
		"":                             "",
		"${ENV::X}":                    `"${ENV::X}"`,
		`a"b\c`:                        `"a\"b\\c"`,
		"it's":                         `"it's"`,
		"x # y":                        `"x # y"`,
		" padded ":                     `" padded "`,
		"`cmd`":                        "\"`cmd`\"",
		`trailing\`:                    `"trailing\\"`,
		"semi;colon":                   `"semi;colon"`,
		"https://x.example/a?b=c#frag": `"https://x.example/a?b=c#frag"`,
	}
	for in, want := range cases {
		got, err := opensslConfEncoder{}.value(in)
		require.NoError(t, err, in)
		require.Equal(t, want, got, in)
	}

	_, err := opensslConfEncoder{}.value("line\nbreak")
	require.Error(t, err)
	_, err = opensslConfEncoder{}.value(string([]byte{0xff, 0xfe}))
	require.Error(t, err)

	got, err := opensslConfEncoder{legacy: true}.value("line\nbreak $x")
	require.NoError(t, err)
	require.Equal(t, `"linebreak $x"`, got)
}

// TestTongsuoMultiValuedSubjectAndSANsRoundTrip verifies multi-valued subject
// attributes, street/postal attributes and IP/URI SANs round-trip as separate,
// literal values through the real tongsuo binary. Regression for issue #48.
func TestTongsuoMultiValuedSubjectAndSANsRoundTrip(t *testing.T) {
	t.Parallel()
	ins := newSecurityTestTongsuo(t)
	ctx := t.Context()

	prikeyPem, err := ins.NewPrikey(ctx)
	require.NoError(t, err)
	uri, err := url.Parse("https://svc.example/$path?q=${ENV::HOME}#frag")
	require.NoError(t, err)

	csrDer, err := ins.NewX509CSR(ctx, prikeyPem,
		WithX509CSRCommonName("multi"),
		WithX509CSROrganization("org-a", "org,b"),
		WithX509CSRStreetAddrs("1 Main St # 2"),
		WithX509CSRPostalCode("200000"),
		WithX509CSRIPAddrs(net.ParseIP("192.0.2.1"), net.ParseIP("2001:db8::1")),
		WithX509CSRURIs(uri),
	)
	require.NoError(t, err)

	csr, err := smx509.ParseCertificateRequest(csrDer)
	require.NoError(t, err)
	require.Equal(t, []string{"org-a", "org,b"}, csr.Subject.Organization)
	require.Equal(t, []string{"1 Main St # 2"}, csr.Subject.StreetAddress)
	require.Equal(t, []string{"200000"}, csr.Subject.PostalCode)
	require.Len(t, csr.IPAddresses, 2)
	require.True(t, csr.IPAddresses[1].Equal(net.ParseIP("2001:db8::1")))
	require.Len(t, csr.URIs, 1)
	require.Equal(t, uri.String(), csr.URIs[0].String())

	t.Run("unencodable values are rejected", func(t *testing.T) {
		t.Parallel()
		for name, opt := range map[string]X509CSROption{
			"control character": WithX509CSRCommonName("evil\n[ v3_ca ]"),
			"non-ascii dns":     WithX509CSRDNSNames("bücher.example"),
		} {
			csrDer, err := ins.NewX509CSR(ctx, prikeyPem, WithX509CSRCommonName("reject"), opt)
			require.Error(t, err, name)
			require.Nil(t, csrDer, name)
		}
	})
}

// TestTongsuoInheritedEnvOption verifies the explicit opt-in for passing named
// parent variables to tongsuo and the validation of variable names.
// Regression for issue #48.
func TestTongsuoInheritedEnvOption(t *testing.T) {
	t.Setenv(confCanaryEnvForTest, confCanaryValueForTest)
	exePath, err := exec.LookPath("tongsuo")
	if err != nil {
		t.Skip("tongsuo executable is not installed")
	}

	for _, bad := range []string{"", "A=B", "NUL\x00"} {
		_, err := NewTongsuo(exePath, WithTongsuoInheritedEnv(bad))
		require.Error(t, err, "%q", bad)
	}

	plain, err := NewTongsuo(exePath)
	require.NoError(t, err)
	require.NotContains(t, strings.Join(plain.subprocessEnv(nil), "\n"), confCanaryValueForTest)

	allowed, err := NewTongsuo(exePath, WithTongsuoInheritedEnv(confCanaryEnvForTest))
	require.NoError(t, err)
	require.Contains(t, allowed.subprocessEnv([]string{"EXTRA=1"}),
		confCanaryEnvForTest+"="+confCanaryValueForTest)
	require.Contains(t, allowed.subprocessEnv([]string{"EXTRA=1"}), "EXTRA=1")
}

// TestTongsuoNewX509CertByCSRRejectsTamperedCSR verifies CSR signature
// validation stays enforced in the Tongsuo signing path. Regression guard for
// issue #48.
func TestTongsuoNewX509CertByCSRRejectsTamperedCSR(t *testing.T) {
	t.Parallel()
	ins := newSecurityTestTongsuo(t)
	ctx := t.Context()

	caKey, caDer := newTongsuoSM2CAForTest(t, ins, "tamper-root")
	csrDer := newTongsuoSM2CSRForTest(t, ins, WithX509CSRCommonName("original-subject"))

	tampered := bytes.Replace(csrDer, []byte("original-subject"), []byte("tampered-subject"), 1)
	require.NotEqual(t, csrDer, tampered)

	certDer, err := ins.NewX509CertByCSR(ctx, caDer, caKey, tampered)
	require.Error(t, err)
	require.Nil(t, certDer)

	certDer, err = ins.NewX509CertByCSR(ctx, caDer, caKey, csrDer)
	require.NoError(t, err)
	require.Equal(t, "original-subject", parseSMCertForTest(t, certDer).Subject.CommonName)
}
