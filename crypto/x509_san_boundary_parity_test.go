package crypto

import (
	"crypto/x509"
	"encoding/asn1"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestX509SANMailboxEscapes preserves mailbox bytes and rejection boundaries across public APIs.
func TestX509SANMailboxEscapes(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	for _, tc := range []struct {
		value string
		valid bool
	}{
		{`"a\"b"@example.org`, true},
		{`"a\\b"@example.org`, true},
		{`""@example.org`, true},
		{`"a"b"@example.org`, false},
		{`"a\"@example.org`, false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			legacyDER, err := NewX509Cert(key, WithX509CertParent(ca),
				WithX509CertCommonName("mailbox"), WithX509CertEmailAddrs(tc.value))
			require.NoError(t, err, "legacy writer control")
			legacy := svidTestParse(t, legacyDER)
			require.Equal(t, []string{tc.value}, legacy.EmailAddresses)
			_, err = legacy.Verify(x509.VerifyOptions{Roots: roots})
			require.NoError(t, err, "trusted chain control")
			csrDER, err := NewX509CSR(key, WithX509CSRCommonName("mailbox"), WithX509CSREmailAddrs(tc.value))
			require.NoError(t, err, "legacy CSR writer control")
			request, err := x509.ParseCertificateRequest(csrDER)
			require.NoError(t, err)
			require.NoError(t, request.CheckSignature(), "possession control")
			der, certErr := NewX509Cert(key, WithX509CertParent(ca), WithX509CertAllowEmptySubject(),
				WithX509CertEmailAddrs(tc.value))
			strictCSR, csrErr := NewX509CSR(key, WithX509CSRAllowEmptySubject(), WithX509CSREmailAddrs(tc.value))
			issued, policyErr := NewX509CertByCSRWithPolicy(ca, key, csrDER, sanPolicyExact(request, request))
			_, verifyErr := VerifyX509CertEmail(legacy, tc.value, x509.VerifyOptions{Roots: roots})
			if !tc.valid {
				require.ErrorContains(t, certErr, "email SAN requires a bare mailbox")
				require.Nil(t, der)
				require.ErrorContains(t, csrErr, "email SAN requires a bare mailbox")
				require.Nil(t, strictCSR)
				require.ErrorContains(t, policyErr, "email SAN requires a bare mailbox")
				require.Nil(t, issued)
				require.ErrorContains(t, verifyErr, "authorized email must be a bare mailbox")
				return
			}
			require.NoError(t, certErr)
			require.Equal(t, []string{tc.value}, svidTestParse(t, der).EmailAddresses)
			require.NoError(t, csrErr)
			strictRequest, err := x509.ParseCertificateRequest(strictCSR)
			require.NoError(t, err)
			require.NoError(t, strictRequest.CheckSignature())
			require.Equal(t, []string{tc.value}, strictRequest.EmailAddresses)
			require.NoError(t, policyErr)
			require.Equal(t, []string{tc.value}, svidTestParse(t, issued).EmailAddresses)
			require.NoError(t, verifyErr)
		})
	}
}

// TestX509SANConstructedOtherName retains generic SAN encoding while policy rejects unsupported identities.
func TestX509SANConstructedOtherName(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.ECDSAWithSHA256)
	ca := newCSRSignatureTestCA(t, key)
	otherDER, err := asn1.Marshal(struct {
		TypeID asn1.ObjectIdentifier
		Value  asn1.RawValue
	}{asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55555, 1},
		asn1.RawValue{Class: 2, Tag: 0, IsCompound: true, Bytes: []byte{0x0c, 1, 'x'}}})
	require.NoError(t, err)
	var other asn1.RawValue
	_, err = asn1.Unmarshal(otherDER, &other)
	require.NoError(t, err)
	ext := sanRawNamesForTest(t, asn1.RawValue{Class: 2, Tag: 2, Bytes: []byte("service.example.org")},
		asn1.RawValue{Class: 2, Tag: 0, IsCompound: true, Bytes: other.Bytes})
	known := sanRawNamesForTest(t, asn1.RawValue{Class: 2, Tag: 2, Bytes: []byte("service.example.org")})
	for _, critical := range []bool{false, true} {
		ext.Critical = critical
		der, err := NewX509Cert(key, WithX509CertParent(ca), WithX509CertCommonName("service"),
			WithX509CertAllowEmptySubject(), WithX509CertExtraExtensions(ext))
		require.NoError(t, err, "generic strict encoder retains unsupported context-specific SAN")
		cert := svidTestParse(t, der)
		require.Contains(t, cert.Extensions, ext, "complete SAN bytes and criticality remain unchanged")
		csrDER, err := NewX509CSR(key, WithX509CSRCommonName("service"),
			WithX509CSRAllowEmptySubject(), WithX509CSRExtraExtension(ext))
		require.NoError(t, err)
		request, err := x509.ParseCertificateRequest(csrDER)
		require.NoError(t, err)
		require.NoError(t, request.CheckSignature())
		require.Contains(t, request.Extensions, ext)
		requestCalls, certCalls := 0, 0
		policy := X509CSRIdentityPolicy{
			AuthorizeRequest:     func(*x509.CertificateRequest) error { requestCalls++; return nil },
			AuthorizeCertificate: func(*x509.Certificate) error { certCalls++; return nil },
		}
		issued, err := NewX509CertByCSRWithPolicy(ca, key, csrDER, policy)
		require.ErrorContains(t, err, "identity policy supports only DNS, IP, email and URI SANs")
		require.Nil(t, issued)
		require.Zero(t, requestCalls)
		require.Zero(t, certCalls)
		knownCSR, err := NewX509CSR(key, WithX509CSRCommonName("service"), WithX509CSRExtraExtension(known))
		require.NoError(t, err)
		issued, err = NewX509CertByCSRWithPolicy(ca, key, knownCSR, policy,
			WithX509SignCSRExtraExtenstions(ext))
		require.ErrorContains(t, err, "identity policy supports only DNS, IP, email and URI SANs")
		require.Nil(t, issued)
		require.Equal(t, 1, requestCalls)
		require.Zero(t, certCalls)
	}
}
