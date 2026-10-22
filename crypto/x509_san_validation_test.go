package crypto

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// sanRawNamesForTest encodes GeneralNames without normalizing their original bytes.
func sanRawNamesForTest(t *testing.T, names ...asn1.RawValue) pkix.Extension {
	t.Helper()
	der, err := asn1.Marshal(names)
	require.NoError(t, err)
	return pkix.Extension{Id: asn1.ObjectIdentifier{2, 5, 29, 17}, Critical: true, Value: der}
}

// TestX509SANEmptySubject checks additive CN-less issuance and critical final SAN rules.
func TestX509SANEmptySubject(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	for _, subject := range []pkix.Name{{}, {Organization: []string{"approved organization"}}} {
		der, err := NewX509Cert(key, WithX509CertParent(ca), WithX509CertAllowEmptySubject(), WithX509Subject(subject), WithX509CertDNSNames("service.example.org"))
		require.NoError(t, err)
		cert := svidTestParse(t, der)
		require.Empty(t, cert.Subject.CommonName)
		if len(subject.ToRDNSequence()) == 0 {
			for _, ext := range cert.Extensions {
				if ext.Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 17}) {
					require.True(t, ext.Critical)
				}
			}
		}
		require.NoError(t, cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature))
		csrDER, err := NewX509CSR(key, WithX509CSRAllowEmptySubject(), WithX509CSRSubject(subject), WithX509CSRDNSNames("service.example.org"))
		require.NoError(t, err)
		csr, err := x509.ParseCertificateRequest(csrDER)
		require.NoError(t, err)
		require.NoError(t, csr.CheckSignature())
	}
	der, err := NewX509Cert(key, WithX509CertAllowEmptySubject())
	require.ErrorContains(t, err, "requires nonempty SAN")
	require.Nil(t, der)
	der, err = NewX509CSR(key, WithX509CSRAllowEmptySubject())
	require.ErrorContains(t, err, "requires nonempty SAN")
	require.Nil(t, der)
	raw := sanRawNamesForTest(t, asn1.RawValue{Class: 2, Tag: 2, Bytes: []byte("service.example.org")})
	raw.Critical = false
	der, err = NewX509Cert(key, WithX509CertAllowEmptySubject(), WithX509CertExtraExtensions(raw))
	require.ErrorContains(t, err, "critical SAN")
	require.Nil(t, der)
	der, err = NewX509Cert(key, WithX509CertAllowEmptySubject(), WithX509CertIsCA(), WithX509CertDNSNames("ca.example.org"))
	require.ErrorContains(t, err, "nonempty Subject")
	require.Nil(t, der)
}

// TestX509SANStrictInput rejects malformed new-path inputs without changing legacy contracts.
func TestX509SANStrictInput(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	relative := &url.URL{Path: "/relative"}
	for name, options := range map[string]struct {
		cert X509CertOption
		csr  X509CSROption
	}{
		"empty DNS":       {WithX509CertDNSNames(""), WithX509CSRDNSNames("")},
		"bad DNS":         {WithX509CertDNSNames("bad name"), WithX509CSRDNSNames("bad name")},
		"IP length":       {WithX509CertIPAddrs(net.IP{1, 2, 3}), WithX509CSRIPAddrs(net.IP{1, 2, 3})},
		"nil URI":         {WithX509CertURIs(nil), WithX509CSRURIs(nil)},
		"relative URI":    {WithX509CertURIs(relative), WithX509CSRURIs(relative)},
		"display mailbox": {WithX509CertEmailAddrs("Alice <alice@example.org>"), WithX509CSREmailAddrs("Alice <alice@example.org>")},
	} {
		t.Run(name, func(t *testing.T) {
			der, err := NewX509Cert(key, WithX509CertCommonName("strict"), WithX509CertAllowEmptySubject(), options.cert)
			require.Error(t, err)
			require.Nil(t, der)
			der, err = NewX509CSR(key, WithX509CSRCommonName("strict"), WithX509CSRAllowEmptySubject(), options.csr)
			require.Error(t, err)
			require.Nil(t, der)
		})
	}
	for name, raw := range map[string]pkix.Extension{
		"empty sequence":  sanRawNamesForTest(t),
		"constructed DNS": sanRawNamesForTest(t, asn1.RawValue{Class: 2, Tag: 2, IsCompound: true, Bytes: []byte("service.example.org")}),
		"wrong class":     sanRawNamesForTest(t, asn1.RawValue{Class: 0, Tag: asn1.TagIA5String, Bytes: []byte("service.example.org")}),
		"trailing data":   {Id: asn1.ObjectIdentifier{2, 5, 29, 17}, Critical: true, Value: []byte{0x30, 0, 0}},
	} {
		t.Run(name, func(t *testing.T) {
			der, err := NewX509Cert(key, WithX509CertCommonName("strict"), WithX509CertAllowEmptySubject(), WithX509CertExtraExtensions(raw))
			require.Error(t, err)
			require.Nil(t, der)
			der, err = NewX509CSR(key, WithX509CSRCommonName("strict"), WithX509CSRAllowEmptySubject(), WithX509CSRExtraExtension(raw))
			require.Error(t, err)
			require.Nil(t, der)
		})
	}
	valid := sanRawNamesForTest(t, asn1.RawValue{Class: 2, Tag: 2, Bytes: []byte("service.example.org")})
	der, err := NewX509Cert(key, WithX509CertParent(ca), WithX509CertAllowEmptySubject(), WithX509CertURIs(nil), WithX509CertExtraExtensions(valid))
	require.NoError(t, err, "raw override preserves standard precedence and avoids the unused nil URI")
	require.Equal(t, []string{"service.example.org"}, svidTestParse(t, der).DNSNames)
}

// TestX509SANOrdinaryURIs retains valid opaque, hostless, query and fragment URI forms.
func TestX509SANOrdinaryURIs(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	for _, value := range []string{"https://example.org/service?a=b#part", "urn:example:service", "mailto:alice@example.org", "file:/service"} {
		t.Run(value, func(t *testing.T) {
			uri, err := url.Parse(value)
			require.NoError(t, err)
			der, err := NewX509Cert(key, WithX509CertParent(ca), WithX509CertAllowEmptySubject(), WithX509CertURIs(uri))
			require.NoError(t, err)
			require.Equal(t, value, svidTestParse(t, der).URIs[0].String())
			csrDER, err := NewX509CSR(key, WithX509CSRAllowEmptySubject(), WithX509CSRURIs(uri))
			require.NoError(t, err)
			csr, err := x509.ParseCertificateRequest(csrDER)
			require.NoError(t, err)
			require.Equal(t, value, csr.URIs[0].String())
		})
	}
}

// TestX509SANFrozenTongsuo rejects new Go-only options before any tool invocation.
func TestX509SANFrozenTongsuo(t *testing.T) {
	var backend Tongsuo
	der, err := backend.NewX509Cert(t.Context(), nil, WithX509CertCommonName("test"), WithX509CertAllowEmptySubject())
	require.ErrorContains(t, err, "Go-only SAN")
	require.Nil(t, der)
	der, err = backend.NewX509CSR(t.Context(), nil, WithX509CSRCommonName("test"), WithX509CSRAllowEmptySubject())
	require.ErrorContains(t, err, "Go-only SAN")
	require.Nil(t, der)
}

// TestX509SANDNSLength checks the full wildcard name rather than only its suffix.
func TestX509SANDNSLength(t *testing.T) {
	maximum := strings.Join([]string{strings.Repeat("a", 63), strings.Repeat("b", 63), strings.Repeat("c", 63), strings.Repeat("d", 61)}, ".")
	require.Len(t, maximum, 253)
	require.True(t, validX509DNSName(maximum))
	require.True(t, validX509DNSName(maximum+"."))
	require.True(t, validX509DNSName("*."+maximum[:len(maximum)-2]))
	require.False(t, validX509DNSName("*."+maximum))
}
