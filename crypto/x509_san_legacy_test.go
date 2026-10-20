package crypto

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"net"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

// sanLegacyURL parses a URI fixture and fails t if the fixture is invalid.
func sanLegacyURL(t *testing.T, value string) *url.URL {
	t.Helper()
	u, err := url.Parse(value)
	require.NoError(t, err)
	return u
}

// sanLegacyParse parses certificate DER and fails t if issuance produced invalid DER.
func sanLegacyParse(t *testing.T, der []byte) *x509.Certificate {
	t.Helper()
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert
}

// sanLegacyRequireFields compares all supported SAN types without depending on IPv4 slice width.
func sanLegacyRequireFields(t *testing.T, cert *x509.Certificate, dns, emails []string, ips []net.IP, uris []*url.URL) {
	t.Helper()
	require.Equal(t, dns, cert.DNSNames)
	require.Equal(t, emails, cert.EmailAddresses)
	require.Len(t, cert.IPAddresses, len(ips))
	for i := range ips {
		require.True(t, ips[i].Equal(cert.IPAddresses[i]), "IP SAN %d", i)
	}
	require.Equal(t, uris, cert.URIs)
}

// sanLegacyExtension creates an exact mixed SAN extension for raw override controls.
func sanLegacyExtension(t *testing.T) pkix.Extension {
	t.Helper()
	value, err := asn1.Marshal([]asn1.RawValue{
		{Class: asn1.ClassContextSpecific, Tag: 2, Bytes: []byte("raw.example.test")},
		{Class: asn1.ClassContextSpecific, Tag: 1, Bytes: []byte("raw@example.test")},
		{Class: asn1.ClassContextSpecific, Tag: 7, Bytes: net.ParseIP("2001:db8::42").To16()},
		{Class: asn1.ClassContextSpecific, Tag: 6, Bytes: []byte("https://raw.example.test/workload")},
	})
	require.NoError(t, err)
	return pkix.Extension{Id: asn1.ObjectIdentifier{2, 5, 29, 17}, Value: value}
}

// sanLegacyRequireRaw checks the final SAN bytes and their parsed identity fields.
func sanLegacyRequireRaw(t *testing.T, cert *x509.Certificate, extension pkix.Extension) {
	t.Helper()
	var found []pkix.Extension
	for _, ext := range cert.Extensions {
		if ext.Id.Equal(extension.Id) {
			found = append(found, ext)
		}
	}
	require.Equal(t, []pkix.Extension{extension}, found)
	sanLegacyRequireFields(t, cert, []string{"raw.example.test"}, []string{"raw@example.test"},
		[]net.IP{net.ParseIP("2001:db8::42")}, []*url.URL{sanLegacyURL(t, "https://raw.example.test/workload")})
}

// TestSANLegacyRoundTrip preserves typed and string SAN APIs across self, direct CA, and CSR issuance.
func TestSANLegacyRoundTrip(t *testing.T) {
	issuerKey := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, issuerKey)
	leafKey := newCSRSignatureTestKey(t, x509.PureEd25519)
	dns := []string{"service.example.test", "*.apps.example.test"}
	emails := []string{"workload@example.test"}
	ips := []net.IP{net.ParseIP("192.0.2.42"), net.ParseIP("2001:db8::42")}
	uris := []*url.URL{sanLegacyURL(t, "https://identity.example.test/workload")}
	strings := []string{dns[0], dns[1], ips[0].String(), ips[1].String(), emails[0], uris[0].String()}
	for _, typed := range []bool{false, true} {
		name := "strings"
		certOptions := []X509CertOption{WithX509CertCommonName("display-name")}
		csrOptions := []X509CSROption{WithX509CSRCommonName("display-name")}
		if typed {
			name = "typed"
			certOptions = append(certOptions, WithX509CertDNSNames(dns...), WithX509CertEmailAddrs(emails...),
				WithX509CertIPAddrs(ips...), WithX509CertURIs(uris...))
			csrOptions = append(csrOptions, WithX509CSRDNSNames(dns...), WithX509CSREmailAddrs(emails...),
				WithX509CSRIPAddrs(ips...), WithX509CSRURIs(uris...))
		} else {
			certOptions = append(certOptions, WithX509CertSANS(strings...))
			csrOptions = append(csrOptions, WithX509CSRSANS(strings...))
		}
		t.Run(name, func(t *testing.T) {
			t.Run("self", func(t *testing.T) {
				der, err := NewX509Cert(leafKey, certOptions...)
				require.NoError(t, err)
				cert := sanLegacyParse(t, der)
				sanLegacyRequireFields(t, cert, dns, emails, ips, uris)
				require.NoError(t, cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature))
			})
			t.Run("CA", func(t *testing.T) {
				options := append([]X509CertOption{}, certOptions...)
				options = append(options, WithX509CertParent(ca), WithX509CertPubkey(leafKey.Public()))
				der, err := NewX509Cert(issuerKey, options...)
				require.NoError(t, err)
				cert := sanLegacyParse(t, der)
				sanLegacyRequireFields(t, cert, dns, emails, ips, uris)
				require.NoError(t, cert.CheckSignatureFrom(ca))
			})
			t.Run("CSR", func(t *testing.T) {
				csrDER, err := NewX509CSR(leafKey, csrOptions...)
				require.NoError(t, err)
				csr, err := x509.ParseCertificateRequest(csrDER)
				require.NoError(t, err)
				require.NoError(t, csr.CheckSignature())
				sanLegacyRequireFields(t, &x509.Certificate{DNSNames: csr.DNSNames, EmailAddresses: csr.EmailAddresses,
					IPAddresses: csr.IPAddresses, URIs: csr.URIs}, dns, emails, ips, uris)
				der, err := NewX509CertByCSR(ca, issuerKey, csrDER)
				require.NoError(t, err)
				cert := sanLegacyParse(t, der)
				sanLegacyRequireFields(t, cert, dns, emails, ips, uris)
				require.Equal(t, csr.RawSubject, cert.RawSubject)
				require.Equal(t, csr.RawSubjectPublicKeyInfo, cert.RawSubjectPublicKeyInfo)
				require.NoError(t, cert.CheckSignatureFrom(ca))
			})
		})
	}
}

// TestSANLegacyRawOverride preserves raw SAN precedence in either option order and explicit signer overrides.
func TestSANLegacyRawOverride(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	ext := sanLegacyExtension(t)
	for _, rawFirst := range []bool{false, true} {
		name := "raw-last"
		if rawFirst {
			name = "raw-first"
		}
		t.Run(name, func(t *testing.T) {
			certOptions := []X509CertOption{WithX509CertCommonName("legacy"), WithX509CertDNSNames("typed.example.test"), WithX509CertExtraExtensions(ext)}
			csrOptions := []X509CSROption{WithX509CSRCommonName("legacy"), WithX509CSRDNSNames("typed.example.test"), WithX509CSRExtraExtension(ext)}
			if rawFirst {
				certOptions[1], certOptions[2] = certOptions[2], certOptions[1]
				csrOptions[1], csrOptions[2] = csrOptions[2], csrOptions[1]
			}
			der, err := NewX509Cert(key, certOptions...)
			require.NoError(t, err)
			sanLegacyRequireRaw(t, sanLegacyParse(t, der), ext)
			csrDER, err := NewX509CSR(key, csrOptions...)
			require.NoError(t, err)
			csr, err := x509.ParseCertificateRequest(csrDER)
			require.NoError(t, err)
			require.NoError(t, csr.CheckSignature())
			require.Contains(t, csr.Extensions, ext)
			der, err = NewX509CertByCSR(ca, key, csrDER)
			require.NoError(t, err)
			cert := sanLegacyParse(t, der)
			sanLegacyRequireFields(t, cert, []string{"raw.example.test"}, []string{"raw@example.test"},
				[]net.IP{net.ParseIP("2001:db8::42")}, []*url.URL{sanLegacyURL(t, "https://raw.example.test/workload")})
			require.NoError(t, cert.CheckSignatureFrom(ca))
		})
	}
	csrDER, err := NewX509CSR(key, WithX509CSRCommonName("legacy"), WithX509CSRDNSNames("requested.example.test"))
	require.NoError(t, err)
	der, err := NewX509CertByCSR(ca, key, csrDER, WithX509SignCSRExtraExtenstions(ext))
	require.NoError(t, err)
	sanLegacyRequireRaw(t, sanLegacyParse(t, der), ext)
}

// TestSANLegacyDNSIPVerification distinguishes trusted DNS/IP matching from URI and email authorization.
func TestSANLegacyDNSIPVerification(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	der, err := NewX509Cert(key, WithX509CertParent(ca), WithX509CertCommonName("not-a-hostname"),
		WithX509CertDNSNames("service.example.test", "*.apps.example.test"),
		WithX509CertIPAddrs(net.ParseIP("192.0.2.42"), net.ParseIP("2001:db8::42")),
		WithX509CertEmailAddrs("workload@example.test"), WithX509CertURIs(sanLegacyURL(t, "https://identity.example.test/workload")),
		WithX509CertExtKeyUsage(x509.ExtKeyUsageServerAuth))
	require.NoError(t, err)
	cert := sanLegacyParse(t, der)
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	for _, hostname := range []string{"service.example.test", "one.apps.example.test", "192.0.2.42", "2001:db8::42"} {
		t.Run(hostname, func(t *testing.T) {
			require.NoError(t, cert.VerifyHostname(hostname))
			_, err := cert.Verify(x509.VerifyOptions{Roots: roots, DNSName: hostname})
			require.NoError(t, err)
		})
	}
	for _, hostname := range []string{"wrong.example.test", "nested.one.apps.example.test", "192.0.2.43", "2001:db8::43", "not-a-hostname", "workload@example.test", "https://identity.example.test/workload", "identity.example.test"} {
		t.Run(hostname, func(t *testing.T) {
			_, err := cert.Verify(x509.VerifyOptions{Roots: roots})
			require.NoError(t, err, "wrong identity must still have a trusted chain")
			err = cert.VerifyHostname(hostname)
			var mismatch x509.HostnameError
			require.ErrorAs(t, err, &mismatch)
			_, err = cert.Verify(x509.VerifyOptions{Roots: roots, DNSName: hostname})
			require.ErrorAs(t, err, &mismatch)
		})
	}
}

// TestSANLegacyCNContract preserves the generic common-name requirement even when SANs are populated.
func TestSANLegacyCNContract(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	_, err := NewX509Cert(key, WithX509CertDNSNames("service.example.test"))
	require.ErrorContains(t, err, "common name")
	_, err = NewX509CSR(key, WithX509CSRDNSNames("service.example.test"))
	require.ErrorContains(t, err, "common name")
	der, err := NewX509Cert(key, WithX509CertCommonName("display"), WithX509CertDNSNames("service.example.test"))
	require.NoError(t, err)
	require.Equal(t, "display", sanLegacyParse(t, der).Subject.CommonName)
}

// TestSANLegacyMalformedInputs checks only malformed encodings rejected by the existing standard-library contract.
func TestSANLegacyMalformedInputs(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	for _, tc := range []struct {
		name string
		cert X509CertOption
		csr  X509CSROption
	}{
		{"non-ASCII DNS", WithX509CertDNSNames("\u00e9.example.test"), WithX509CSRDNSNames("\u00e9.example.test")},
		{"non-ASCII email", WithX509CertEmailAddrs("\u00e9@example.test"), WithX509CSREmailAddrs("\u00e9@example.test")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewX509Cert(key, WithX509CertCommonName("legacy"), tc.cert)
			require.Error(t, err)
			_, err = NewX509CSR(key, WithX509CSRCommonName("legacy"), tc.csr)
			require.Error(t, err)
		})
	}
}

// TestSANLegacyMalformedIPWire records the existing writer/parser boundary without tightening legacy options.
func TestSANLegacyMalformedIPWire(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	der, err := NewX509Cert(key, WithX509CertCommonName("legacy"), WithX509CertIPAddrs(net.IP{1, 2, 3}))
	require.NoError(t, err, "the legacy standard-library writer does not validate IP byte length")
	_, err = x509.ParseCertificate(der)
	require.ErrorContains(t, err, "IP")
	csrDER, err := NewX509CSR(key, WithX509CSRCommonName("legacy"), WithX509CSRIPAddrs(net.IP{1, 2, 3}))
	require.NoError(t, err)
	_, err = x509.ParseCertificateRequest(csrDER)
	require.ErrorContains(t, err, "IP")
}
