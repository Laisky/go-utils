package crypto

import (
	"bytes"
	"crypto/x509"

	"github.com/Laisky/errors/v2"
)

// VerifyX509CertURI verifies cert.Raw with the caller's chain options and requires
// an exact original URI SAN match. It does not apply SPIFFE profile/bundle rules.
// This is URI identity authorization, not VerifyHostname or authorization of other
// names in the certificate. The caller selects roots and application KeyUsages.
func VerifyX509CertURI(cert *x509.Certificate, authorizedURI string, opts x509.VerifyOptions) ([][]*x509.Certificate, error) {
	if !validX509URI(authorizedURI) {
		return nil, errors.New("authorized URI must be absolute")
	}
	parsed, chains, err := verifyX509IdentityChain(cert, opts)
	if err != nil {
		return nil, err
	}
	values, err := x509SVIDRawURIs(parsed.Extensions)
	if err != nil {
		return nil, err
	}
	for _, value := range values {
		if value == authorizedURI {
			return chains, nil
		}
	}
	return nil, errors.New("certificate URI SAN is not the authorized identity")
}

// VerifyX509CertEmail verifies cert.Raw with the caller's chain options and requires
// an exact bare email SAN match, including case. It does not use VerifyHostname or
// normalize mailbox identities. Callers select suitable roots and KeyUsages, such
// as EmailProtection, and independently authorize other names if used.
func VerifyX509CertEmail(cert *x509.Certificate, authorizedEmail string, opts x509.VerifyOptions) ([][]*x509.Certificate, error) {
	if !validX509Email(authorizedEmail) {
		return nil, errors.New("authorized email must be a bare mailbox")
	}
	parsed, chains, err := verifyX509IdentityChain(cert, opts)
	if err != nil {
		return nil, err
	}
	for _, value := range parsed.EmailAddresses {
		if value == authorizedEmail {
			return chains, nil
		}
	}
	return nil, errors.New("certificate email SAN is not the authorized identity")
}

// verifyX509IdentityChain reparses original DER before checking caller-selected trust.
func verifyX509IdentityChain(cert *x509.Certificate, opts x509.VerifyOptions) (*x509.Certificate, [][]*x509.Certificate, error) {
	if cert == nil {
		return nil, nil, errors.New("identity certificate is required")
	}
	parsed, err := x509.ParseCertificate(bytes.Clone(cert.Raw))
	if err != nil {
		return nil, nil, errors.Wrap(err, "parse identity certificate DER")
	}
	chains, err := parsed.Verify(opts)
	if err != nil {
		return nil, nil, errors.Wrap(err, "verify identity certificate chain")
	}
	return parsed, chains, nil
}
