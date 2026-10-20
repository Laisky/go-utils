package crypto

import (
	"crypto/x509"
	"encoding/asn1"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestX509URIValidSyntaxControls preserves exact valid URI spelling and raw override precedence.
func TestX509URIValidSyntaxControls(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	for _, value := range []string{
		"https://example.org/workload?value=%2f#part",
		"https://example.org/workload?value=%2F#part",
		"https://example.org/workload#",
		"urn:example:workload%2fpart",
		"urn:example:workload%25GG",
		"urn:example:workload%3Cidentity%3E",
		"urn:example:AZaz09-._~!$&'()*+,;=:@/?:",
		"https://[2001:db8::1]/AZaz09-._~!$&'()*+,;=:@/?value=AZaz09-._~!$&'()*+,;=:@/?",
		"mailto:alice%2Bservice@example.org",
		"file:/workload%20identity",
	} {
		t.Run(value, func(t *testing.T) {
			raw := svidRawURIExtension(t, value)
			for _, rawFirst := range []bool{false, true} {
				options := []X509CertOption{WithX509CertURIs(nil), WithX509CertExtraExtensions(raw)}
				if rawFirst {
					options[0], options[1] = options[1], options[0]
				}
				der, err := NewX509Cert(key, append(options, WithX509CertParent(ca), WithX509CertAllowEmptySubject())...)
				require.NoError(t, err, "valid raw override controls unused typed URI inputs")
				cert := svidTestParse(t, der)
				values, err := x509SVIDRawURIs(cert.Extensions)
				require.NoError(t, err)
				require.Equal(t, []string{value}, values)
				_, err = VerifyX509CertURI(cert, value, x509.VerifyOptions{Roots: roots})
				require.NoError(t, err)
				if value == "https://example.org/workload?value=%2f#part" {
					_, err = VerifyX509CertURI(cert, "https://example.org/workload?value=%2F#part", x509.VerifyOptions{Roots: roots})
					require.Error(t, err, "valid percent escape spelling remains part of the exact identity")
				}
				if value == "https://example.org/workload#" {
					_, err = VerifyX509CertURI(cert, "https://example.org/workload", x509.VerifyOptions{Roots: roots})
					require.Error(t, err, "an empty fragment delimiter remains part of the exact identity")
				}
			}
			for _, rawFirst := range []bool{false, true} {
				options := []X509CSROption{WithX509CSRURIs(nil), WithX509CSRExtraExtension(raw)}
				if rawFirst {
					options[0], options[1] = options[1], options[0]
				}
				csrDER, err := NewX509CSR(key, append(options, WithX509CSRAllowEmptySubject())...)
				require.NoError(t, err)
				csr, err := x509.ParseCertificateRequest(csrDER)
				require.NoError(t, err)
				require.NoError(t, csr.CheckSignature())
				issued, err := NewX509CertByCSRWithPolicy(ca, key, csrDER, sanPolicyExact(csr, csr))
				require.NoError(t, err)
				values, err := x509SVIDRawURIs(svidTestParse(t, issued).Extensions)
				require.NoError(t, err)
				require.Equal(t, []string{value}, values)
			}
		})
	}
}

// TestX509URIRejectsMalformedSyntax rejects malformed escapes and forbidden literals accepted by net/url.
func TestX509URIRejectsMalformedSyntax(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	validRequest, err := NewX509CSR(key, WithX509CSRCommonName("valid policy request"),
		WithX509CSRURIs(&url.URL{Scheme: "https", Host: "example.org", Path: "/workload"}))
	require.NoError(t, err)
	for _, value := range []string{
		"https://example.org/workload?value=%",
		"https://example.org/workload?value=%2",
		"https://example.org/workload?value=%ZZ",
		"urn:example:workload%",
		"urn:example:workload%2",
		"urn:example:workload%GG",
		"urn:example:workload<",
		"urn:example:workload>",
		"urn:example:workload\\",
		"urn:example:workload\"",
		"urn:example:workload`",
		"urn:example:workload{",
		"urn:example:workload}",
		"urn:example:workload|",
		"https://example.org/workload?value=<",
		"https://example.org/workload?value=>",
		"https://example.org/workload?value=\\",
		"https://example.org/workload?value=\"",
		"https://example.org/workload?value=`",
		"https://example.org/workload?value={",
		"https://example.org/workload?value=}",
		"https://example.org/workload?value=|",
	} {
		t.Run(value, func(t *testing.T) {
			uri, err := url.Parse(value)
			require.NoError(t, err, "net/url parser accepts this malformed opaque/query syntax")
			require.Equal(t, value, uri.String())
			raw := svidRawURIExtension(t, value)
			legacyDER, err := NewX509Cert(key, WithX509CertParent(ca), WithX509CertCommonName("legacy URI"), WithX509CertExtraExtensions(raw))
			require.NoError(t, err, "legacy standard writer behavior is a compatibility control")
			legacy := svidTestParse(t, legacyDER)
			_, err = legacy.Verify(x509.VerifyOptions{Roots: roots})
			require.NoError(t, err, "the negative verification case has a valid signature and trusted chain")
			requestDER, err := NewX509CSR(key, WithX509CSRCommonName("legacy URI"), WithX509CSRExtraExtension(raw))
			require.NoError(t, err, "legacy request encoding remains compatible")
			request, err := x509.ParseCertificateRequest(requestDER)
			require.NoError(t, err)
			require.NoError(t, request.CheckSignature())
			t.Run("typed certificate", func(t *testing.T) {
				der, err := NewX509Cert(key, WithX509CertParent(ca), WithX509CertCommonName("strict URI"),
					WithX509CertAllowEmptySubject(), WithX509CertURIs(uri))
				require.Error(t, err)
				require.Nil(t, der)
			})
			t.Run("raw certificate override", func(t *testing.T) {
				der, err := NewX509Cert(key, WithX509CertParent(ca), WithX509CertCommonName("strict URI"),
					WithX509CertAllowEmptySubject(), WithX509CertExtraExtensions(raw))
				require.Error(t, err)
				require.Nil(t, der)
			})
			t.Run("typed request", func(t *testing.T) {
				der, err := NewX509CSR(key, WithX509CSRCommonName("strict URI"), WithX509CSRAllowEmptySubject(), WithX509CSRURIs(uri))
				require.Error(t, err)
				require.Nil(t, der)
			})
			t.Run("raw request override", func(t *testing.T) {
				der, err := NewX509CSR(key, WithX509CSRCommonName("strict URI"), WithX509CSRAllowEmptySubject(), WithX509CSRExtraExtension(raw))
				require.Error(t, err)
				require.Nil(t, der)
			})
			t.Run("policy request", func(t *testing.T) {
				calls := 0
				policy := X509CSRIdentityPolicy{
					AuthorizeRequest:     func(*x509.CertificateRequest) error { calls++; return nil },
					AuthorizeCertificate: func(*x509.Certificate) error { calls++; return nil },
				}
				der, err := NewX509CertByCSRWithPolicy(ca, key, requestDER, policy)
				require.Error(t, err)
				require.Nil(t, der)
				require.Zero(t, calls, "malformed requested identity must fail before authorization")
			})
			t.Run("policy final override", func(t *testing.T) {
				requests, certificates := 0, 0
				policy := X509CSRIdentityPolicy{
					AuthorizeRequest:     func(*x509.CertificateRequest) error { requests++; return nil },
					AuthorizeCertificate: func(*x509.Certificate) error { certificates++; return nil },
				}
				der, err := NewX509CertByCSRWithPolicy(ca, key, validRequest, policy, WithX509SignCSRExtraExtenstions(raw))
				require.Error(t, err)
				require.Nil(t, der)
				require.Equal(t, 1, requests)
				require.Zero(t, certificates, "malformed final identity must fail before final approval")
			})
			t.Run("exact URI verification", func(t *testing.T) {
				chains, err := VerifyX509CertURI(legacy, value, x509.VerifyOptions{Roots: roots})
				require.Error(t, err)
				require.Nil(t, chains)
			})
		})
	}
}

// TestX509URIRawOverrideRejectsMalformedEffectiveValue preserves raw SAN authority in either option order.
func TestX509URIRawOverrideRejectsMalformedEffectiveValue(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	uri, err := url.Parse("https://example.org/valid")
	require.NoError(t, err)
	raw := sanRawNamesForTest(t, asn1.RawValue{Class: 2, Tag: 6, Bytes: []byte("urn:example:invalid%")})
	for _, test := range []struct {
		name     string
		rawFirst bool
	}{{"raw last", false}, {"raw first", true}} {
		t.Run(test.name, func(t *testing.T) {
			t.Run("certificate", func(t *testing.T) {
				opts := []X509CertOption{WithX509CertURIs(uri), WithX509CertExtraExtensions(raw)}
				if test.rawFirst {
					opts[0], opts[1] = opts[1], opts[0]
				}
				der, err := NewX509Cert(key, append(opts, WithX509CertParent(ca), WithX509CertAllowEmptySubject())...)
				require.Error(t, err, "valid typed URI cannot conceal malformed effective raw SAN")
				require.Nil(t, der)
			})
			t.Run("request", func(t *testing.T) {
				opts := []X509CSROption{WithX509CSRURIs(uri), WithX509CSRExtraExtension(raw)}
				if test.rawFirst {
					opts[0], opts[1] = opts[1], opts[0]
				}
				der, err := NewX509CSR(key, append(opts, WithX509CSRAllowEmptySubject())...)
				require.Error(t, err)
				require.Nil(t, der)
			})
		})
	}
}
