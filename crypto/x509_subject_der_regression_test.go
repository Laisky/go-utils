package crypto

import (
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// subjectDERUIDForTest returns a nonempty DN containing only an unrecognized UID attribute.
func subjectDERUIDForTest(t *testing.T) (pkix.Name, []byte) {
	t.Helper()
	subject := pkix.Name{ExtraNames: []pkix.AttributeTypeAndValue{{
		Type: asn1.ObjectIdentifier{0, 9, 2342, 19200300, 100, 1, 1}, Value: "approved-uid",
	}}}
	der, err := asn1.Marshal(subject.ToRDNSequence())
	require.NoError(t, err)
	require.NotEqual(t, []byte{0x30, 0}, der)
	return subject, der
}

// TestX509SubjectDERControls distinguishes original nonempty DER from lossy parsed names.
func TestX509SubjectDERControls(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	subject, rawSubject := subjectDERUIDForTest(t)
	for _, usage := range []x509.KeyUsage{x509.KeyUsageCertSign, x509.KeyUsageCRLSign} {
		tpl := &x509.Certificate{
			SerialNumber: big.NewInt(42), Subject: subject,
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
			IsCA: usage == x509.KeyUsageCertSign, BasicConstraintsValid: true,
			KeyUsage: usage, DNSNames: []string{"uid.example.org"},
		}
		der, err := x509.CreateCertificate(rand.Reader, tpl, ca, key.Public(), key)
		require.NoError(t, err, "standard writer accepts a UID-only DN")
		cert := svidTestParse(t, der)
		require.Equal(t, rawSubject, cert.RawSubject)
		require.Len(t, cert.Subject.Names, 1)
		require.Empty(t, cert.Subject.ToRDNSequence(), "parsed name reconstruction omits unknown UID")
		require.NoError(t, cert.CheckSignatureFrom(ca))
		roots := x509.NewCertPool()
		roots.AddCert(ca)
		_, err = cert.Verify(x509.VerifyOptions{Roots: roots, DNSName: "uid.example.org", KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}})
		require.NoError(t, err, "standard reader and trust-chain control for the original UID DN")
	}
	t.Run("legacy named UID", func(t *testing.T) {
		subject.CommonName = "legacy common name"
		der, err := NewX509Cert(key, WithX509Subject(subject), WithX509CertParent(ca), WithX509CertIsCA())
		require.NoError(t, err)
		cert := svidTestParse(t, der)
		require.Len(t, cert.Subject.Names, 2)
		require.NoError(t, cert.CheckSignatureFrom(ca))
	})
	t.Run("legacy CN requirement", func(t *testing.T) {
		uidOnly, _ := subjectDERUIDForTest(t)
		der, err := NewX509Cert(key, WithX509Subject(uidOnly), WithX509CertParent(ca), WithX509CertIsCA())
		require.ErrorContains(t, err, "common name must be set")
		require.Nil(t, der)
	})
}

// TestX509SubjectDERUIDDirect preserves nonempty unknown-attribute DNs under strict CA rules.
func TestX509SubjectDERUIDDirect(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	subject, rawSubject := subjectDERUIDForTest(t)
	for name, usage := range map[string]X509CertOption{
		"CA": WithX509CertIsCA(), "CRL signer": WithX509CertKeyUsage(x509.KeyUsageCRLSign),
	} {
		t.Run(name, func(t *testing.T) {
			der, err := NewX509Cert(key, WithX509CertAllowEmptySubject(), WithX509Subject(subject),
				WithX509CertParent(ca), WithX509CertDNSNames("uid.example.org"), usage)
			require.NoError(t, err, "nonempty original UID DN must not be mistaken for an empty Subject")
			cert := svidTestParse(t, der)
			require.Equal(t, rawSubject, cert.RawSubject)
			require.NoError(t, cert.CheckSignatureFrom(ca))
		})
	}
}

// TestX509SubjectDERUIDPolicy preserves the complete authorized request Subject in issued DER.
func TestX509SubjectDERUIDPolicy(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	subject, rawSubject := subjectDERUIDForTest(t)
	csrDER, err := NewX509CSR(key, WithX509CSRAllowEmptySubject(), WithX509CSRSubject(subject),
		WithX509CSRDNSNames("uid.example.org"))
	require.NoError(t, err)
	request, err := x509.ParseCertificateRequest(csrDER)
	require.NoError(t, err)
	require.NoError(t, request.CheckSignature())
	require.Equal(t, rawSubject, request.RawSubject)
	for name, opts := range map[string][]SignCSROption{
		"leaf":       nil,
		"CA":         {WithX509SignCSRIsCA()},
		"CRL signer": {WithX509SignCSRKeyUsage(x509.KeyUsageCRLSign)},
	} {
		t.Run(name, func(t *testing.T) {
			der, err := NewX509CertByCSRWithPolicy(ca, key, csrDER, sanPolicyExact(request, request), opts...)
			require.NoError(t, err, "policy must issue the original approved UID-only Subject")
			cert := svidTestParse(t, der)
			require.Equal(t, rawSubject, cert.RawSubject)
			require.NoError(t, cert.CheckSignatureFrom(ca))
		})
	}
}

// TestX509SubjectDEREmptySigningRejects retains rejection of genuinely empty CA and CRL DNs.
func TestX509SubjectDEREmptySigningRejects(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	for name, usage := range map[string]X509CertOption{
		"CA": WithX509CertIsCA(), "CRL signer": WithX509CertKeyUsage(x509.KeyUsageCRLSign),
	} {
		t.Run(name, func(t *testing.T) {
			der, err := NewX509Cert(key, WithX509CertAllowEmptySubject(), WithX509CertParent(ca),
				WithX509CertDNSNames("uid.example.org"), usage)
			require.ErrorContains(t, err, "nonempty Subject")
			require.Nil(t, der)
		})
	}
	csrDER, err := NewX509CSR(key, WithX509CSRAllowEmptySubject(), WithX509CSRDNSNames("uid.example.org"))
	require.NoError(t, err)
	request, err := x509.ParseCertificateRequest(csrDER)
	require.NoError(t, err)
	for name, usage := range map[string]SignCSROption{
		"CA": WithX509SignCSRIsCA(), "CRL signer": WithX509SignCSRKeyUsage(x509.KeyUsageCRLSign),
	} {
		t.Run("policy "+name, func(t *testing.T) {
			der, err := NewX509CertByCSRWithPolicy(ca, key, csrDER, sanPolicyExact(request, request), usage)
			require.ErrorContains(t, err, "nonempty Subject")
			require.Nil(t, der)
		})
	}
}

// TestX509SubjectDERLegacyCSR preserves the legacy named-Subject reconstruction contract.
func TestX509SubjectDERLegacyCSR(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	subject, _ := subjectDERUIDForTest(t)
	subject.CommonName = "legacy CSR common name"
	csrDER, err := NewX509CSR(key, WithX509CSRSubject(subject), WithX509CSRDNSNames("uid.example.org"))
	require.NoError(t, err)
	request, err := x509.ParseCertificateRequest(csrDER)
	require.NoError(t, err)
	require.NoError(t, request.CheckSignature())
	require.Len(t, request.Subject.Names, 2)
	der, err := NewX509CertByCSR(ca, key, csrDER)
	require.NoError(t, err)
	cert := svidTestParse(t, der)
	expected, err := asn1.Marshal(pkix.Name{CommonName: subject.CommonName}.ToRDNSequence())
	require.NoError(t, err)
	require.Equal(t, expected, cert.RawSubject, "the legacy API keeps its existing loss of unknown request DN attributes")
	require.NotEqual(t, request.RawSubject, cert.RawSubject)
	require.Len(t, cert.Subject.Names, 1)
	require.NoError(t, cert.CheckSignatureFrom(ca))
}

// TestX509SubjectDERSVIDPolicy preserves a UID-only Subject through SVID authorization and issuance.
func TestX509SubjectDERSVIDPolicy(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	subject, rawSubject := subjectDERUIDForTest(t)
	csrDER, err := NewX509SVIDCSR(key, svidTestID, WithX509CSRSubject(subject))
	require.NoError(t, err)
	request, err := x509.ParseCertificateRequest(csrDER)
	require.NoError(t, err)
	require.Equal(t, rawSubject, request.RawSubject)
	require.NoError(t, request.CheckSignature())
	der, err := NewX509SVIDByCSRWithPolicy(ca, key, csrDER, svidTestID, sanPolicyExact(request, request))
	require.NoError(t, err)
	cert := svidTestParse(t, der)
	require.Equal(t, rawSubject, cert.RawSubject)
	require.NoError(t, cert.CheckSignatureFrom(ca))
	_, err = VerifyX509SVIDForID([]*x509.Certificate{cert}, svidTestBundle("example.org", ca), svidTestID)
	require.NoError(t, err)
}

// TestX509SubjectDERMultivaluedPolicy preserves valid RDN grouping, order and string encodings.
func TestX509SubjectDERMultivaluedPolicy(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	rawSubject, err := asn1.Marshal(pkix.RDNSequence{
		{
			{Type: asn1.ObjectIdentifier{2, 5, 4, 3}, Value: asn1.RawValue{Tag: asn1.TagUTF8String, Bytes: []byte("grouped leaf")}},
			{Type: asn1.ObjectIdentifier{0, 9, 2342, 19200300, 100, 1, 1}, Value: "same-RDN-uid"},
		},
		{{Type: asn1.ObjectIdentifier{2, 5, 4, 10}, Value: "organization after CN"}},
	})
	require.NoError(t, err)
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		RawSubject: rawSubject, DNSNames: []string{"uid.example.org"},
	}, key)
	require.NoError(t, err, "standard writer control for valid custom RDN encoding")
	request, err := x509.ParseCertificateRequest(csrDER)
	require.NoError(t, err)
	require.NoError(t, request.CheckSignature())
	require.Equal(t, rawSubject, request.RawSubject)
	reconstructed, err := asn1.Marshal(request.Subject.ToRDNSequence())
	require.NoError(t, err)
	require.NotEqual(t, rawSubject, reconstructed, "fixture must detect lossy reconstruction")
	der, err := NewX509CertByCSRWithPolicy(ca, key, csrDER, sanPolicyExact(request, request))
	require.NoError(t, err)
	cert := svidTestParse(t, der)
	require.Equal(t, rawSubject, cert.RawSubject, "the new policy preserves exact original Subject DER")
	require.NoError(t, cert.CheckSignatureFrom(ca))
}
