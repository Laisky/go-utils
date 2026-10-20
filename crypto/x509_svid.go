package crypto

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/asn1"
	"net/url"

	"github.com/Laisky/errors/v2"
	"github.com/spiffe/go-spiffe/v2/bundle/x509bundle"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
)

// ParseX509SVIDID parses a canonical SPIFFE workload ID with a non-root path.
// Identity parsing does not establish authority to issue or authenticate that ID.
func ParseX509SVIDID(value string) (spiffeid.ID, error) {
	id, err := spiffeid.FromString(value)
	if err != nil {
		return spiffeid.ID{}, errors.Wrap(err, "parse SPIFFE ID")
	}
	if id.Path() == "" || id.Path() == "/" {
		return spiffeid.ID{}, errors.New("X.509-SVID identity requires a non-root path")
	}
	return id, nil
}

// ValidateX509SVID validates the leaf profile encoded in cert.Raw and returns its ID.
// It does not verify signatures, validity periods, trust, or identity authorization.
// EKU is recommended by SPIFFE, not required; if present, both TLS purposes are required.
func ValidateX509SVID(cert *x509.Certificate) (spiffeid.ID, error) {
	if cert == nil {
		return spiffeid.ID{}, errors.New("X.509-SVID certificate is required")
	}
	parsed, err := x509.ParseCertificate(cert.Raw)
	if err != nil {
		return spiffeid.ID{}, errors.Wrap(err, "parse X.509-SVID certificate DER")
	}
	id, err := x509SVIDURI(parsed.URIs)
	if err != nil {
		return spiffeid.ID{}, err
	}
	if parsed.IsCA || parsed.KeyUsage&(x509.KeyUsageCertSign|x509.KeyUsageCRLSign) != 0 {
		return spiffeid.ID{}, errors.New("X.509-SVID leaf cannot sign certificates or CRLs")
	}
	if parsed.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return spiffeid.ID{}, errors.New("X.509-SVID requires digitalSignature key usage")
	}
	var criticalKU, criticalSAN, hasEKU bool
	for _, ext := range parsed.Extensions {
		switch {
		case ext.Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 15}):
			criticalKU = ext.Critical
		case ext.Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 17}):
			criticalSAN = ext.Critical
		case ext.Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 37}):
			hasEKU = true
		}
	}
	if !criticalKU {
		return spiffeid.ID{}, errors.New("X.509-SVID key usage extension must be critical")
	}
	if bytes.Equal(parsed.RawSubject, []byte{0x30, 0}) && !criticalSAN {
		return spiffeid.ID{}, errors.New("empty-subject X.509-SVID requires critical SAN")
	}
	if hasEKU {
		var client, server bool
		for _, usage := range parsed.ExtKeyUsage {
			client = client || usage == x509.ExtKeyUsageClientAuth
			server = server || usage == x509.ExtKeyUsageServerAuth
		}
		if !client || !server {
			return spiffeid.ID{}, errors.New("X.509-SVID EKU must include clientAuth and serverAuth")
		}
	}
	return id, nil
}

// x509SVIDURI validates exactly one URI SAN as a workload SPIFFE ID.
func x509SVIDURI(uris []*url.URL) (spiffeid.ID, error) {
	if len(uris) != 1 || uris[0] == nil {
		return spiffeid.ID{}, errors.New("X.509-SVID requires exactly one URI SAN")
	}
	return ParseX509SVIDID(uris[0].String())
}

// NewX509SVID creates a Go X.509 leaf for the caller-authorized SPIFFE ID.
// Options retain their usual meanings, but contradictory final DER is rejected.
// A parent CA and the leaf public key can be selected with the existing certificate options.
func NewX509SVID(prikey crypto.PrivateKey, authorizedID string, opts ...X509CertOption) ([]byte, error) {
	id, err := ParseX509SVIDID(authorizedID)
	if err != nil {
		return nil, err
	}
	defaults := []X509CertOption{
		WithX509CertURIs(id.URL()),
		WithX509CertExtKeyUsage(x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth),
		func(o *x509V3CertOption) error { o.allowEmptySubject = true; return nil },
	}
	der, err := NewX509Cert(prikey, append(defaults, opts...)...)
	if err != nil {
		return nil, errors.Wrap(err, "create X.509-SVID")
	}
	return validateIssuedX509SVID(der, id)
}

// NewX509SVIDCSR creates a signed request for a workload SPIFFE ID, allowing an empty subject.
// The requested ID is a claim; an issuer must independently authorize it before issuance.
func NewX509SVIDCSR(prikey crypto.PrivateKey, requestedID string, opts ...X509CSROption) ([]byte, error) {
	id, err := ParseX509SVIDID(requestedID)
	if err != nil {
		return nil, err
	}
	defaults := []X509CSROption{
		WithX509CSRURIs(id.URL()),
		func(o *x509CSROption) error { o.allowEmptySubject = true; return nil },
	}
	der, err := NewX509CSR(prikey, append(defaults, opts...)...)
	if err != nil {
		return nil, errors.Wrap(err, "create X.509-SVID CSR")
	}
	csr, err := x509.ParseCertificateRequest(der)
	if err != nil {
		return nil, errors.Wrap(err, "parse final X.509-SVID CSR")
	}
	actualID, err := x509SVIDURI(csr.URIs)
	if err != nil {
		return nil, err
	}
	if actualID != id {
		return nil, errors.New("final CSR SPIFFE ID differs from requested identity")
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, errors.Wrap(err, "verify final CSR signature")
	}
	return der, nil
}

// NewX509SVIDByCSR checks possession and requires the CSR's ID to equal authorizedID.
// Callers must obtain authorizedID from their enrollment policy, independently of the CSR.
// Requested raw extensions are not automatically issuance authorization.
func NewX509SVIDByCSR(parent *x509.Certificate, prikey crypto.PrivateKey, csrDER []byte,
	authorizedID string, opts ...SignCSROption) ([]byte, error) {
	id, err := ParseX509SVIDID(authorizedID)
	if err != nil {
		return nil, err
	}
	csr, err := x509.ParseCertificateRequest(csrDER)
	if err != nil {
		return nil, errors.Wrap(err, "parse X.509-SVID CSR")
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, errors.Wrap(err, "verify X.509-SVID CSR possession")
	}
	requestedID, err := x509SVIDURI(csr.URIs)
	if err != nil {
		return nil, err
	}
	if requestedID != id {
		return nil, errors.New("CSR SPIFFE ID is not the authorized identity")
	}
	defaults := []SignCSROption{WithX509SignCSRExtKeyUsage(x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth)}
	der, err := newX509CertByCSR(parent, prikey, csrDER, true, append(defaults, opts...)...)
	if err != nil {
		return nil, errors.Wrap(err, "issue X.509-SVID CSR")
	}
	return validateIssuedX509SVID(der, id)
}

// validateIssuedX509SVID validates final DER and its exact caller-authorized identity.
func validateIssuedX509SVID(der []byte, expected spiffeid.ID) ([]byte, error) {
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, errors.Wrap(err, "parse issued X.509-SVID")
	}
	id, err := ValidateX509SVID(cert)
	if err != nil {
		return nil, errors.Wrap(err, "validate issued X.509-SVID profile")
	}
	if id != expected {
		return nil, errors.New("issued SPIFFE ID differs from authorized identity")
	}
	return der, nil
}

// VerifyX509SVID verifies the DER leaf profile and its chain using the leaf ID's trust-domain bundle.
// It never uses OS roots. The returned authenticated ID still requires application authorization.
// The go-spiffe bundle source permits explicit federation and bundle rotation.
func VerifyX509SVID(certs []*x509.Certificate, bundles x509bundle.Source,
	opts ...x509svid.VerifyOption) (spiffeid.ID, [][]*x509.Certificate, error) {
	if len(certs) == 0 {
		return spiffeid.ID{}, nil, errors.New("X.509-SVID certificate chain is required")
	}
	parsed := make([]*x509.Certificate, len(certs))
	for i, cert := range certs {
		if cert == nil {
			return spiffeid.ID{}, nil, errors.New("nil X.509-SVID chain certificate")
		}
		var err error
		parsed[i], err = x509.ParseCertificate(cert.Raw)
		if err != nil {
			return spiffeid.ID{}, nil, errors.Wrap(err, "parse X.509-SVID chain DER")
		}
	}
	profileID, err := ValidateX509SVID(parsed[0])
	if err != nil {
		return spiffeid.ID{}, nil, err
	}
	if bundles == nil {
		return spiffeid.ID{}, nil, errors.New("X.509-SVID bundle source is required")
	}
	bundle, err := bundles.GetX509BundleForTrustDomain(profileID.TrustDomain())
	if err != nil {
		return spiffeid.ID{}, nil, errors.Wrap(err, "select X.509-SVID trust-domain bundle")
	}
	if bundle == nil || bundle.TrustDomain() != profileID.TrustDomain() {
		return spiffeid.ID{}, nil, errors.New("X.509-SVID bundle does not match leaf trust domain")
	}
	id, chains, err := x509svid.Verify(parsed, bundle, opts...)
	if err != nil {
		return spiffeid.ID{}, nil, errors.Wrap(err, "verify X.509-SVID trust")
	}
	validChains := make([][]*x509.Certificate, 0, len(chains))
	for _, chain := range chains {
		valid := true
		for _, authority := range chain[1:] {
			if err := validateX509SVIDAuthority(authority); err != nil {
				valid = false
				break
			}
		}
		if valid {
			validChains = append(validChains, chain)
		}
	}
	if len(validChains) == 0 {
		return spiffeid.ID{}, nil, errors.New("no chain has valid X.509-SVID signing certificates")
	}
	return id, validChains, nil
}

// validateX509SVIDAuthority checks signing profiles without requiring a CA SPIFFE URI.
func validateX509SVIDAuthority(cert *x509.Certificate) error {
	if !cert.IsCA || cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		return errors.New("X.509-SVID signing certificate requires CA and certSign")
	}
	criticalKU := false
	for _, ext := range cert.Extensions {
		if ext.Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 15}) {
			criticalKU = ext.Critical
		}
	}
	if !criticalKU {
		return errors.New("X.509-SVID signing certificate key usage must be critical")
	}
	for _, uri := range cert.URIs {
		// Signing certificates SHOULD be SVIDs; ordinary CAs can have unrelated URI SANs.
		if uri == nil || uri.Scheme != "spiffe" {
			continue
		}
		if len(cert.URIs) != 1 {
			return errors.New("SVID signing certificate requires exactly one URI SAN")
		}
		id, err := spiffeid.FromURI(uri)
		if err != nil {
			return errors.Wrap(err, "parse signing certificate SPIFFE ID")
		}
		if id.Path() != "" {
			return errors.New("X.509-SVID signing certificate SPIFFE ID must have no path")
		}
	}
	return nil
}

// VerifyX509SVIDForID additionally authorizes the exact expected workload ID after chain validation.
func VerifyX509SVIDForID(certs []*x509.Certificate, bundles x509bundle.Source, expectedID string,
	opts ...x509svid.VerifyOption) ([][]*x509.Certificate, error) {
	expected, err := ParseX509SVIDID(expectedID)
	if err != nil {
		return nil, err
	}
	id, chains, err := VerifyX509SVID(certs, bundles, opts...)
	if err != nil {
		return nil, err
	}
	if id != expected {
		return nil, errors.New("authenticated SPIFFE ID is not authorized")
	}
	return chains, nil
}
