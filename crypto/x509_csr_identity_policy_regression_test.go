package crypto

import (
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"net"
	"net/url"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

// sanPolicyRequest signs and parses a complete general or SPIFFE identity request.
func sanPolicyRequest(t *testing.T, key crypto.Signer, svid bool) ([]byte, *x509.CertificateRequest) {
	t.Helper()
	options := []X509CSROption{WithX509CSRSubject(pkix.Name{CommonName: "service", Organization: []string{"authorized"}}),
		WithX509CSRDNSNames("service.example.test"), WithX509CSREmailAddrs("service@example.test"),
		WithX509CSRIPAddrs(net.ParseIP("192.0.2.42"), net.ParseIP("2001:db8::42"))}
	var der []byte
	var err error
	if svid {
		der, err = NewX509SVIDCSR(key, svidTestID, options...)
	} else {
		options = append(options, WithX509CSRURIs(sanLegacyURL(t, "https://identity.example.test/workload"), sanLegacyURL(t, "urn:example:workload")))
		der, err = NewX509CSR(key, options...)
	}
	require.NoError(t, err)
	csr, err := x509.ParseCertificateRequest(der)
	require.NoError(t, err)
	require.NoError(t, csr.CheckSignature())
	return der, csr
}

// sanPolicyMatches checks every supported identity field against independently approved claims.
func sanPolicyMatches(subject []byte, dns, email []string, ips []net.IP, uris []*url.URL, expected *x509.CertificateRequest) bool {
	if !bytes.Equal(subject, expected.RawSubject) || !slices.Equal(dns, expected.DNSNames) ||
		!slices.Equal(email, expected.EmailAddresses) || len(ips) != len(expected.IPAddresses) || len(uris) != len(expected.URIs) {
		return false
	}
	for i := range ips {
		if !ips[i].Equal(expected.IPAddresses[i]) {
			return false
		}
	}
	for i := range uris {
		if uris[i].String() != expected.URIs[i].String() {
			return false
		}
	}
	return true
}

// sanPolicyExact creates two independent exact identity checks, rather than authorizing by possession.
func sanPolicyExact(request, certificate *x509.CertificateRequest) X509CSRIdentityPolicy {
	return X509CSRIdentityPolicy{
		AuthorizeRequest: func(csr *x509.CertificateRequest) error {
			if !sanPolicyMatches(csr.RawSubject, csr.DNSNames, csr.EmailAddresses, csr.IPAddresses, csr.URIs, request) {
				return errors.New("request identities are not authorized")
			}
			return nil
		},
		AuthorizeCertificate: func(cert *x509.Certificate) error {
			if !sanPolicyMatches(cert.RawSubject, cert.DNSNames, cert.EmailAddresses, cert.IPAddresses, cert.URIs, certificate) {
				return errors.New("certificate identities are not authorized")
			}
			return nil
		},
	}
}

// sanPolicyIssue selects the generic or SPIFFE policy entry point for shared controls.
func sanPolicyIssue(svid bool, ca *x509.Certificate, key crypto.PrivateKey, csr []byte, policy X509CSRIdentityPolicy, opts ...SignCSROption) ([]byte, error) {
	if svid {
		return NewX509SVIDByCSRWithPolicy(ca, key, csr, svidTestID, policy, opts...)
	}
	return NewX509CertByCSRWithPolicy(ca, key, csr, policy, opts...)
}

// TestSANPolicyMixedIdentities authorizes ordinary mixed SANs and a single SPIFFE URI with other SAN types.
func TestSANPolicyMixedIdentities(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.PureEd25519)
	ca := newCSRSignatureTestCA(t, key)
	for _, svid := range []bool{false, true} {
		t.Run(map[bool]string{false: "general", true: "SPIFFE"}[svid], func(t *testing.T) {
			der, claims := sanPolicyRequest(t, key, svid)
			issued, err := sanPolicyIssue(svid, ca, key, der, sanPolicyExact(claims, claims))
			require.NoError(t, err)
			cert := sanLegacyParse(t, issued)
			require.NoError(t, cert.CheckSignatureFrom(ca))
			require.True(t, sanPolicyMatches(cert.RawSubject, cert.DNSNames, cert.EmailAddresses, cert.IPAddresses, cert.URIs, claims))
			require.Equal(t, claims.RawSubjectPublicKeyInfo, cert.RawSubjectPublicKeyInfo)
			if svid {
				_, err := ValidateX509SVID(cert)
				require.NoError(t, err)
				require.Len(t, cert.URIs, 1)
			}
		})
	}
}

// TestSANPolicySubjects permits explicit CN-less DNs and requires critical SAN for the empty-Subject leaf encoding.
func TestSANPolicySubjects(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.PureEd25519)
	ca := newCSRSignatureTestCA(t, key)
	for _, subject := range []pkix.Name{{}, {Organization: []string{"authorized"}}} {
		der, err := NewX509CSR(key, WithX509CSRAllowEmptySubject(), WithX509CSRSubject(subject), WithX509CSRDNSNames("service.example.test"))
		require.NoError(t, err)
		csr, err := x509.ParseCertificateRequest(der)
		require.NoError(t, err)
		issued, err := NewX509CertByCSRWithPolicy(ca, key, der, sanPolicyExact(csr, csr))
		require.NoError(t, err)
		cert := sanLegacyParse(t, issued)
		require.Equal(t, csr.RawSubject, cert.RawSubject)
		for _, ext := range cert.Extensions {
			if ext.Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 17}) {
				require.Equal(t, len(subject.ToRDNSequence()) == 0, ext.Critical)
			}
		}
	}
}

// TestSANPolicyEachIdentityDenied rejects independently unauthorized Subject and each supported SAN field.
func TestSANPolicyEachIdentityDenied(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.PureEd25519)
	ca := newCSRSignatureTestCA(t, key)
	der, claims := sanPolicyRequest(t, key, false)
	for name, mutate := range map[string]func(*x509.CertificateRequest){
		"Subject": func(c *x509.CertificateRequest) { c.RawSubject = []byte{0x30, 0} },
		"DNS":     func(c *x509.CertificateRequest) { c.DNSNames[0] = "other.example.test" },
		"email":   func(c *x509.CertificateRequest) { c.EmailAddresses[0] = "other@example.test" },
		"IPv4":    func(c *x509.CertificateRequest) { c.IPAddresses[0] = net.ParseIP("192.0.2.43") },
		"IPv6":    func(c *x509.CertificateRequest) { c.IPAddresses[1] = net.ParseIP("2001:db8::43") },
		"URI":     func(c *x509.CertificateRequest) { c.URIs[0] = sanLegacyURL(t, "https://other.example.test/workload") },
	} {
		t.Run(name, func(t *testing.T) {
			other, err := x509.ParseCertificateRequest(bytes.Clone(der))
			require.NoError(t, err)
			mutate(other)
			for _, stage := range []string{"request", "certificate"} {
				t.Run(stage, func(t *testing.T) {
					policy := sanPolicyExact(other, claims)
					if stage == "certificate" {
						policy = sanPolicyExact(claims, other)
					}
					issued, err := NewX509CertByCSRWithPolicy(ca, key, der, policy)
					require.ErrorContains(t, err, stage+" identities are not authorized")
					require.Nil(t, issued)
				})
			}
		})
	}
}

// TestSANPolicySPIFFEIsNotOtherIdentityAuthorization requires separate approval of other claims beside a valid SPIFFE URI.
func TestSANPolicySPIFFEIsNotOtherIdentityAuthorization(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.PureEd25519)
	ca := newCSRSignatureTestCA(t, key)
	der, claims := sanPolicyRequest(t, key, true)
	for name, mutate := range map[string]func(*x509.CertificateRequest){
		"Subject": func(c *x509.CertificateRequest) { c.RawSubject = []byte{0x30, 0} },
		"DNS":     func(c *x509.CertificateRequest) { c.DNSNames[0] = "other.example.test" },
		"email":   func(c *x509.CertificateRequest) { c.EmailAddresses[0] = "other@example.test" },
		"IPv4":    func(c *x509.CertificateRequest) { c.IPAddresses[0] = net.ParseIP("192.0.2.43") },
		"IPv6":    func(c *x509.CertificateRequest) { c.IPAddresses[1] = net.ParseIP("2001:db8::43") },
	} {
		t.Run(name, func(t *testing.T) {
			other, err := x509.ParseCertificateRequest(bytes.Clone(der))
			require.NoError(t, err)
			mutate(other)
			issued, err := NewX509SVIDByCSRWithPolicy(ca, key, der, svidTestID, sanPolicyExact(other, claims))
			require.ErrorContains(t, err, "request identities are not authorized")
			require.Nil(t, issued)
			issued, err = NewX509SVIDByCSRWithPolicy(ca, key, der, svidTestID, sanPolicyExact(claims, other))
			require.ErrorContains(t, err, "certificate identities are not authorized")
			require.Nil(t, issued)
		})
	}
}

// TestSANPolicyStagesAndPossession requires both callbacks and authenticates signatures before invoking either.
func TestSANPolicyStagesAndPossession(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.PureEd25519)
	ca := newCSRSignatureTestCA(t, key)
	for _, svid := range []bool{false, true} {
		der, claims := sanPolicyRequest(t, key, svid)
		exact := sanPolicyExact(claims, claims)
		for _, policy := range []X509CSRIdentityPolicy{{}, {AuthorizeRequest: exact.AuthorizeRequest}, {AuthorizeCertificate: exact.AuthorizeCertificate}} {
			issued, err := sanPolicyIssue(svid, ca, key, der, policy)
			require.ErrorContains(t, err, "both CSR request and certificate identity policies")
			require.Nil(t, issued)
		}
		calls := 0
		policy := X509CSRIdentityPolicy{AuthorizeRequest: func(*x509.CertificateRequest) error { calls++; return nil },
			AuthorizeCertificate: func(*x509.Certificate) error { calls++; return nil }}
		tampered := bytes.Clone(der)
		tampered[len(tampered)-1] ^= 1
		issued, err := sanPolicyIssue(svid, ca, key, tampered, policy)
		require.ErrorContains(t, err, "possession")
		require.Nil(t, issued)
		require.Zero(t, calls)
		policy.AuthorizeRequest = func(*x509.CertificateRequest) error { return errors.New("request denied") }
		issued, err = sanPolicyIssue(svid, ca, key, der, policy)
		require.ErrorContains(t, err, "request denied")
		require.Nil(t, issued)
		require.Zero(t, calls, "request rejection must prevent signing/final authorization")
	}
}

// TestSANPolicySnapshotIsolation prevents policy snapshot mutations from replacing signed request or output identities.
func TestSANPolicySnapshotIsolation(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.PureEd25519)
	ca := newCSRSignatureTestCA(t, key)
	der, claims := sanPolicyRequest(t, key, false)
	original := bytes.Clone(der)
	policy := X509CSRIdentityPolicy{
		AuthorizeRequest: func(c *x509.CertificateRequest) error {
			c.Subject.Organization[0], c.DNSNames[0], c.EmailAddresses[0] = "mutated", "mutated.example.test", "mutated@example.test"
			c.IPAddresses[0][0] ^= 1
			c.URIs[0].Host = "mutated.example.test"
			c.Extensions[0].Value[0] ^= 1
			c.PublicKey.(ed25519.PublicKey)[0] ^= 1
			c.RawSubject[0] ^= 1
			c.Raw[0] ^= 1
			der[0] ^= 1
			return nil
		},
		AuthorizeCertificate: func(c *x509.Certificate) error {
			require.True(t, sanPolicyMatches(c.RawSubject, c.DNSNames, c.EmailAddresses, c.IPAddresses, c.URIs, claims))
			c.Subject.Organization[0], c.DNSNames[0], c.EmailAddresses[0] = "mutated", "mutated.example.test", "mutated@example.test"
			c.IPAddresses[0][0] ^= 1
			c.URIs[0].Host = "mutated.example.test"
			c.Extensions[0].Value[0] ^= 1
			c.PublicKey.(ed25519.PublicKey)[0] ^= 1
			c.RawSubject[0] ^= 1
			c.Raw[0] ^= 1
			return nil
		},
	}
	issued, err := NewX509CertByCSRWithPolicy(ca, key, der, policy)
	require.NoError(t, err)
	cert := sanLegacyParse(t, issued)
	require.True(t, sanPolicyMatches(cert.RawSubject, cert.DNSNames, cert.EmailAddresses, cert.IPAddresses, cert.URIs, claims))
	require.NoError(t, cert.CheckSignatureFrom(ca))
	require.Equal(t, claims.RawSubjectPublicKeyInfo, cert.RawSubjectPublicKeyInfo)
	_, err = x509.ParseCertificateRequest(original)
	require.NoError(t, err)
}

// TestSANPolicyFinalRawOverride requires separate final approval of issuer substitutions after request authorization.
func TestSANPolicyFinalRawOverride(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.PureEd25519)
	ca := newCSRSignatureTestCA(t, key)
	der, request := sanPolicyRequest(t, key, false)
	ext := sanLegacyExtension(t)
	finalDER, err := NewX509CSR(key, WithX509CSRSubject(request.Subject), WithX509CSRExtraExtension(ext))
	require.NoError(t, err)
	final, err := x509.ParseCertificateRequest(finalDER)
	require.NoError(t, err)
	issued, err := NewX509CertByCSRWithPolicy(ca, key, der, sanPolicyExact(request, request), WithX509SignCSRExtraExtenstions(ext))
	require.ErrorContains(t, err, "certificate identities are not authorized")
	require.Nil(t, issued)
	issued, err = NewX509CertByCSRWithPolicy(ca, key, der, sanPolicyExact(request, final), WithX509SignCSRExtraExtenstions(ext))
	require.NoError(t, err)
	sanLegacyRequireRaw(t, sanLegacyParse(t, issued), ext)
}

// TestSANPolicyRejectsUnsupportedWire rejects identities hidden from the supported typed SAN fields before authorization.
func TestSANPolicyRejectsUnsupportedWire(t *testing.T) {
	key := newCSRSignatureTestKey(t, x509.PureEd25519)
	ca := newCSRSignatureTestCA(t, key)
	allow := X509CSRIdentityPolicy{AuthorizeRequest: func(*x509.CertificateRequest) error { return nil }, AuthorizeCertificate: func(*x509.Certificate) error { return nil }}
	unknown, err := asn1.Marshal([]asn1.RawValue{{Class: 2, Tag: 8, Bytes: []byte{42, 3}}})
	require.NoError(t, err)
	for name, raw := range map[string][]byte{"unsupported registeredID": unknown, "empty GeneralNames": {0x30, 0}, "malformed DER": {0x30, 1, 0}} {
		t.Run(name, func(t *testing.T) {
			der, err := NewX509CSR(key, WithX509CSRCommonName("service"), WithX509CSRExtraExtension(pkix.Extension{Id: []int{2, 5, 29, 17}, Value: raw}))
			require.NoError(t, err)
			issued, err := NewX509CertByCSRWithPolicy(ca, key, der, allow)
			require.Error(t, err)
			require.Nil(t, issued)
		})
	}
	der, _ := sanPolicyRequest(t, key, false)
	ext := sanLegacyExtension(t)
	issued, err := NewX509CertByCSRWithPolicy(ca, key, der, allow, WithX509SignCSRExtraExtenstions(ext, ext))
	require.Error(t, err)
	require.Nil(t, issued)
	duplicateCSR, err := NewX509CSR(key, WithX509CSRCommonName("service"), WithX509CSRExtraExtension(ext), WithX509CSRExtraExtension(ext))
	require.NoError(t, err)
	issued, err = NewX509CertByCSRWithPolicy(ca, key, duplicateCSR, allow)
	require.Error(t, err)
	require.Nil(t, issued)
}
