package crypto

import (
	"crypto/x509"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestX509SANVerifyIdentities separates chain trust from exact URI and email authorization.
func TestX509SANVerifyIdentities(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	uri, err := url.Parse("https://example.org/service?x=1#part")
	require.NoError(t, err)
	der, err := NewX509Cert(key, WithX509CertParent(ca), WithX509CertAllowEmptySubject(),
		WithX509CertURIs(uri), WithX509CertEmailAddrs("alice@example.org"),
		WithX509CertDNSNames("service.example.org"),
		WithX509CertExtKeyUsage(x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageEmailProtection))
	require.NoError(t, err)
	cert := svidTestParse(t, der)
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	opts := x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	_, err = cert.Verify(opts)
	require.NoError(t, err, "chain trust positive control")
	_, err = VerifyX509CertURI(cert, uri.String(), opts)
	require.NoError(t, err)
	_, err = VerifyX509CertEmail(cert, "alice@example.org", x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageEmailProtection}})
	require.NoError(t, err)
	chains, err := VerifyX509CertURI(cert, "https://example.org/other", opts)
	require.ErrorContains(t, err, "not the authorized identity")
	require.Nil(t, chains)
	chains, err = VerifyX509CertEmail(cert, "mallory@example.org", opts)
	require.ErrorContains(t, err, "not the authorized identity")
	require.Nil(t, chains)
	_, err = VerifyX509CertEmail(cert, "Alice@example.org", opts)
	require.Error(t, err, "exact matching does not silently fold mailbox case")
	// Parsed-field substitution cannot change the signed identity used by the helper.
	cert.URIs[0].Path = "/substitution"
	cert.EmailAddresses[0] = "mallory@example.org"
	_, err = VerifyX509CertURI(cert, "https://example.org/substitution?x=1#part", opts)
	require.Error(t, err)
	_, err = VerifyX509CertEmail(cert, "mallory@example.org", opts)
	require.Error(t, err)
	_, err = VerifyX509CertEmail(cert, "alice@example.org", opts)
	require.NoError(t, err)
	_, err = VerifyX509CertURI(cert, uri.String(), x509.VerifyOptions{Roots: x509.NewCertPool()})
	var unknown x509.UnknownAuthorityError
	require.ErrorAs(t, err, &unknown)
	_, err = VerifyX509CertEmail(cert, "alice@example.org", x509.VerifyOptions{Roots: roots, CurrentTime: time.Now().Add(30 * 24 * time.Hour)})
	var invalid x509.CertificateInvalidError
	require.ErrorAs(t, err, &invalid)
	require.Equal(t, x509.Expired, invalid.Reason)
	_, err = VerifyX509CertURI(nil, uri.String(), opts)
	require.Error(t, err)
	_, err = VerifyX509CertEmail(nil, "alice@example.org", opts)
	require.Error(t, err)
	_, err = VerifyX509CertURI(cert, "/relative", opts)
	require.Error(t, err)
	_, err = VerifyX509CertEmail(cert, "Alice <alice@example.org>", opts)
	require.Error(t, err)
}

// TestX509SANVerifyOriginalURI prevents URL normalization from broadening authorization.
func TestX509SANVerifyOriginalURI(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	for _, value := range []string{"https://example.org/workload#", "urn:example:workload", "mailto:alice@example.org"} {
		der, err := NewX509Cert(key, WithX509CertCommonName("service"), WithX509CertParent(ca),
			WithX509CertExtraExtensions(svidRawURIExtension(t, value)))
		require.NoError(t, err)
		cert := svidTestParse(t, der)
		_, err = VerifyX509CertURI(cert, value, x509.VerifyOptions{Roots: roots})
		require.NoError(t, err)
		if value == "https://example.org/workload#" {
			_, err = VerifyX509CertURI(cert, "https://example.org/workload", x509.VerifyOptions{Roots: roots})
			require.Error(t, err, "the original empty-fragment delimiter is part of exact URI policy")
		}
	}
}

// TestX509SANMailboxGrammar retains quoted mailboxes and SMTP address literals without header normalization.
func TestX509SANMailboxGrammar(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	for _, value := range []string{`"service account"@example.org`, `"alice"@example.org`, `"a@b"@example.org`, "alice@[192.0.2.1]", "alice@[IPv6:2001:db8::1]"} {
		t.Run(value, func(t *testing.T) {
			legacyDER, err := NewX509Cert(key, WithX509CertCommonName("mailbox"), WithX509CertParent(ca), WithX509CertEmailAddrs(value))
			require.NoError(t, err, "standard writer control")
			cert := svidTestParse(t, legacyDER)
			require.Equal(t, []string{value}, cert.EmailAddresses)
			_, err = cert.Verify(x509.VerifyOptions{Roots: roots})
			require.NoError(t, err, "chain control")
			_, err = VerifyX509CertEmail(cert, value, x509.VerifyOptions{Roots: roots})
			require.NoError(t, err)
			der, err := NewX509Cert(key, WithX509CertAllowEmptySubject(), WithX509CertEmailAddrs(value))
			require.NoError(t, err)
			require.Equal(t, []string{value}, svidTestParse(t, der).EmailAddresses)
			csrDER, err := NewX509CSR(key, WithX509CSRAllowEmptySubject(), WithX509CSREmailAddrs(value))
			require.NoError(t, err)
			request, err := x509.ParseCertificateRequest(csrDER)
			require.NoError(t, err)
			issued, err := NewX509CertByCSRWithPolicy(ca, key, csrDER, sanPolicyExact(request, request))
			require.NoError(t, err)
			require.Equal(t, []string{value}, svidTestParse(t, issued).EmailAddresses)
		})
	}
	for _, value := range []string{"Alice <alice@example.org>", "alice(comment)@example.org", "alice@bad_domain", "alice@-bad.example", "alice@*.example.org", "alice@bad..example", "a..b@example.org", "alice\n@example.org", `"unclosed@example.org`, "alice@[IPv6:192.0.2.1]"} {
		t.Run("invalid "+value, func(t *testing.T) {
			require.False(t, validX509Email(value))
		})
	}
}
