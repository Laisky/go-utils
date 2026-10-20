package crypto

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"

	"github.com/Laisky/errors/v2"
)

// X509CSRIdentityPolicy explicitly authorizes every identity in a signed request
// and in the final certificate. Both callbacks are required and must return an
// error to deny issuance. Their separately parsed DER snapshots include raw
// Subject and extensions; modifying a snapshot cannot modify the issued DER.
// The policy APIs support DNS, IP, email and URI GeneralNames and reject other
// SAN forms rather than silently omitting identities from authorization.
type X509CSRIdentityPolicy struct {
	AuthorizeRequest     func(*x509.CertificateRequest) error
	AuthorizeCertificate func(*x509.Certificate) error
}

// NewX509CertByCSRWithPolicy checks possession, authorizes the request before
// signing, and authorizes final encoded identities after all issuer options and
// raw overrides. It permits a DN without CN, or an empty leaf Subject with a
// nonempty critical SAN. It returns no DER if either policy check fails.
// Legacy NewX509CertByCSR behavior is unchanged. This is an issuance policy API,
// not a chain verifier or an automatic source of identity authorization.
func NewX509CertByCSRWithPolicy(parent *x509.Certificate, prikey crypto.PrivateKey,
	csrDER []byte, policy X509CSRIdentityPolicy, opts ...SignCSROption) ([]byte, error) {
	requestDER, request, err := authorizeX509CSRRequest(csrDER, policy)
	if err != nil {
		return nil, err
	}
	options := append(append([]SignCSROption(nil), opts...), preserveX509CSRSubjectAltName(request))
	der, err := newX509CertByCSR(parent, prikey, requestDER, true, options...)
	if err != nil {
		return nil, err
	}
	return authorizeX509IssuedCertificate(der, policy)
}

// NewX509SVIDByCSRWithPolicy requires the independently authorized SPIFFE ID and
// explicit policy approval for all Subject and SAN claims, including DNS, IP and
// email. Other SAN types may accompany the single SPIFFE URI; a second ordinary
// URI is incompatible with the SVID profile. Both policy checks must succeed.
func NewX509SVIDByCSRWithPolicy(parent *x509.Certificate, prikey crypto.PrivateKey,
	csrDER []byte, authorizedID string, policy X509CSRIdentityPolicy, opts ...SignCSROption) ([]byte, error) {
	requestDER, request, err := authorizeX509CSRRequest(csrDER, policy)
	if err != nil {
		return nil, err
	}
	options := append(append([]SignCSROption(nil), opts...), preserveX509CSRSubjectAltName(request))
	der, err := NewX509SVIDByCSR(parent, prikey, requestDER, authorizedID, options...)
	if err != nil {
		return nil, err
	}
	return authorizeX509IssuedCertificate(der, policy)
}

// preserveX509CSRSubjectAltName retains authorized original SAN bytes unless the
// issuer explicitly supplies a SAN override, which still needs final approval.
// Other requested extensions do not become issuance options.
func preserveX509CSRSubjectAltName(request *x509.CertificateRequest) SignCSROption {
	return func(o *signCSROption) error {
		oid := asn1.ObjectIdentifier{2, 5, 29, 17}
		for _, ext := range o.extraExtensions {
			if ext.Id.Equal(oid) {
				return nil
			}
		}
		for _, ext := range request.Extensions {
			if ext.Id.Equal(oid) {
				ext = pkix.Extension{Id: oid, Critical: ext.Critical, Value: bytes.Clone(ext.Value)}
				if len(request.Subject.ToRDNSequence()) == 0 {
					ext.Critical = true
				}
				o.extraExtensions = append(o.extraExtensions, ext)
				break
			}
		}
		return nil
	}
}

// authorizeX509CSRRequest authenticates request bytes and authorizes a detached snapshot.
func authorizeX509CSRRequest(csrDER []byte, policy X509CSRIdentityPolicy) ([]byte, *x509.CertificateRequest, error) {
	if policy.AuthorizeRequest == nil || policy.AuthorizeCertificate == nil {
		return nil, nil, errors.New("both CSR request and certificate identity policies are required")
	}
	der := bytes.Clone(csrDER)
	csr, err := x509.ParseCertificateRequest(der)
	if err != nil {
		return nil, nil, errors.Wrap(err, "parse policy CSR")
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, nil, errors.Wrap(err, "verify policy CSR possession")
	}
	if err := validateX509IdentitySAN(csr.RawSubject, csr.Extensions, false, true); err != nil {
		return nil, nil, err
	}
	snapshot, err := x509.ParseCertificateRequest(bytes.Clone(der))
	if err != nil {
		return nil, nil, errors.Wrap(err, "parse request policy snapshot")
	}
	if err := policy.AuthorizeRequest(snapshot); err != nil {
		return nil, nil, errors.Wrap(err, "authorize CSR identities")
	}
	return der, csr, nil
}

// authorizeX509IssuedCertificate validates and authorizes a detached final DER snapshot.
func authorizeX509IssuedCertificate(der []byte, policy X509CSRIdentityPolicy) ([]byte, error) {
	cert, err := x509.ParseCertificate(bytes.Clone(der))
	if err != nil {
		return nil, errors.Wrap(err, "parse issued identity policy certificate")
	}
	if err := validateX509IdentitySAN(cert.RawSubject, cert.Extensions, true, true); err != nil {
		return nil, err
	}
	if len(cert.RawSubject) == 2 && (cert.IsCA || cert.KeyUsage&x509.KeyUsageCRLSign != 0) {
		return nil, errors.New("CA and CRL signing certificates require a nonempty Subject")
	}
	if err := policy.AuthorizeCertificate(cert); err != nil {
		return nil, errors.Wrap(err, "authorize issued certificate identities")
	}
	return der, nil
}
