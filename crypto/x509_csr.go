package crypto

import (
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"net"
	"net/url"

	"github.com/Laisky/errors/v2"
)

type x509CSROption struct {
	err error

	subject pkix.Name

	dnsNames       []string
	emailAddresses []string
	ipAddresses    []net.IP
	uris           []*url.URL

	// extensions,
	extraExtensions []pkix.Extension

	// attributes contains the CSR attributes that can parse as
	// pkix.AttributeTypeAndValueSET.
	//
	// Deprecated: Use Extensions and ExtraExtensions instead for parsing and
	// generating the requestedExtensions attribute.
	attributes []pkix.AttributeTypeAndValueSET

	// signatureAlgorithm specific signature algorithm manually
	//
	// default to auto choose algorithm depends on certificate's algorithm
	signatureAlgorithm x509.SignatureAlgorithm
	// publicKeyAlgorithm specific publick key algorithm manually
	//
	// default to auto choose algorithm depends on certificate's algorithm
	// publicKeyAlgorithm x509.PublicKeyAlgorithm
}

// X509CSROption option to generate tls certificate
type X509CSROption func(*x509CSROption) error

// WithX509CSRExtension set extension
//
// Extensions contains raw X.509 extensions. When parsing certificates,
// this can be used to extract non-critical extensions that are not
// parsed by this package. When marshaling certificates, the Extensions
// field is ignored, see ExtraExtensions.
//
// Deprecated: this field is ignored by golang's built-in x509 library,
// use ExtraExtensions instead if you want to add custom extensions.
func WithX509CSRExtension(_ pkix.Extension) X509CSROption {
	return func(_ *x509CSROption) error {
		// o.extensions = append(o.extensions, ext)
		return nil
	}
}

// WithX509CSRExtraExtension set extra extension
//
// ExtraExtensions contains extensions to be copied, raw, into any
// marshaled certificates. Values override any extensions that would
// otherwise be produced based on the other fields. The ExtraExtensions
// field is not populated when parsing certificates, see Extensions.
func WithX509CSRExtraExtension(ext pkix.Extension) X509CSROption {
	return func(o *x509CSROption) error {
		o.extraExtensions = append(o.extraExtensions, ext)
		return nil
	}
}

// WithX509CSRAttribute set attribute
//
// Deprecated: Use Extensions and ExtraExtensions instead for parsing and
// generating the requestedExtensions attribute.
func WithX509CSRAttribute(attr pkix.AttributeTypeAndValueSET) X509CSROption {
	return func(o *x509CSROption) error {
		o.attributes = append(o.attributes, attr)
		return nil
	}
}

// WithX509CSRSignatureAlgorithm set signature algorithm
func WithX509CSRSignatureAlgorithm(sigAlg x509.SignatureAlgorithm) X509CSROption {
	return func(o *x509CSROption) error {
		o.signatureAlgorithm = sigAlg
		return nil
	}
}

// WithX509CSRPublicKeyAlgorithm set signature algorithm
//
// Deprecated: this field is ignored by golang's built-in x509 library
func WithX509CSRPublicKeyAlgorithm(_ x509.PublicKeyAlgorithm) X509CSROption {
	return func(_ *x509CSROption) error {
		// o.publicKeyAlgorithm = pubAlg
		return nil
	}
}

// WithX509CSRSubject set subject name
func WithX509CSRSubject(subject pkix.Name) X509CSROption {
	return func(o *x509CSROption) error {
		o.subject = subject
		return nil
	}
}

// WithX509CSRCommonName set common name
func WithX509CSRCommonName(commonName string) X509CSROption {
	return func(o *x509CSROption) error {
		o.subject.CommonName = commonName
		return nil
	}
}

// WithX509CSROrganization set organization
func WithX509CSROrganization(organization ...string) X509CSROption {
	return func(o *x509CSROption) error {
		o.subject.Organization = append(o.subject.Organization, organization...)
		return nil
	}
}

// WithX509CSROrganizationUnit set organization units
func WithX509CSROrganizationUnit(ou ...string) X509CSROption {
	return func(o *x509CSROption) error {
		o.subject.OrganizationalUnit = append(o.subject.OrganizationalUnit, ou...)
		return nil
	}
}

// WithX509CSRLocality set subject localities
func WithX509CSRLocality(l ...string) X509CSROption {
	return func(o *x509CSROption) error {
		o.subject.Locality = append(o.subject.Locality, l...)
		return nil
	}
}

// WithX509CSRCountry set subject countries
func WithX509CSRCountry(values ...string) X509CSROption {
	return func(o *x509CSROption) error {
		o.subject.Country = append(o.subject.Country, values...)
		return nil
	}
}

// WithX509CSRProvince set subject provinces
func WithX509CSRProvince(values ...string) X509CSROption {
	return func(o *x509CSROption) error {
		o.subject.Province = append(o.subject.Province, values...)
		return nil
	}
}

// WithX509CSRStreetAddrs set subjuect street addresses
func WithX509CSRStreetAddrs(addrs ...string) X509CSROption {
	return func(o *x509CSROption) error {
		o.subject.StreetAddress = append(o.subject.StreetAddress, addrs...)
		return nil
	}
}

// WithX509CSRPostalCode set subjuect postal codes
func WithX509CSRPostalCode(codes ...string) X509CSROption {
	return func(o *x509CSROption) error {
		o.subject.PostalCode = append(o.subject.PostalCode, codes...)
		return nil
	}
}

// WithX509CSRDNSNames set dns sans
func WithX509CSRDNSNames(dnsNames ...string) X509CSROption {
	return func(o *x509CSROption) error {
		o.dnsNames = append(o.dnsNames, dnsNames...)
		return nil
	}
}

// WithX509CSREmailAddrs set email sans
func WithX509CSREmailAddrs(emailAddresses ...string) X509CSROption {
	return func(o *x509CSROption) error {
		o.emailAddresses = append(o.emailAddresses, emailAddresses...)
		return nil
	}
}

// WithX509CSRIPAddrs set ip sans
func WithX509CSRIPAddrs(ipAddresses ...net.IP) X509CSROption {
	return func(o *x509CSROption) error {
		o.ipAddresses = append(o.ipAddresses, ipAddresses...)
		return nil
	}
}

// WithX509CSRURIs set uri sans
func WithX509CSRURIs(uris ...*url.URL) X509CSROption {
	return func(o *x509CSROption) error {
		o.uris = append(o.uris, uris...)
		return nil
	}
}

// WithX509CSRSANS returns an X509CSROption that adds the subject alternative names in sans to the
// CSR. Each value is classified by parseSans as an IP address, email address, URI or DNS name and
// appended to the matching field. The returned option never fails.
//
// refer to RFC-5280 4.2.1.6
//
// auto WithX509CSRSANS to ip/email/url/dns
func WithX509CSRSANS(sans ...string) X509CSROption {
	return func(o *x509CSROption) error {
		parsedSANs := parseSans(sans)
		o.dnsNames = append(o.dnsNames, parsedSANs.DNSNames...)
		o.emailAddresses = append(o.emailAddresses, parsedSANs.EmailAddresses...)
		o.uris = append(o.uris, parsedSANs.URIs...)
		o.ipAddresses = append(o.ipAddresses, parsedSANs.IPAddresses...)

		return nil
	}
}

// fillDefault fills default values into o and returns o for chaining. No CSR option currently has a
// default value, so it leaves o unchanged.
func (o *x509CSROption) fillDefault() *x509CSROption {
	return o
}

// applyOpts applies opts to o in order. It returns o on success, the previously recorded o.err if
// one is set, or the first option error wrapped with context.
func (o *x509CSROption) applyOpts(opts ...X509CSROption) (*x509CSROption, error) {
	if o.err != nil {
		return nil, errors.WithStack(o.err)
	}

	for _, f := range opts {
		if err := f(o); err != nil {
			return nil, errors.Wrap(err, "apply x509 CSR option")
		}
	}

	return o, nil
}

// X509CsrOption2Template convert X509CSROption to x509.CertificateRequest
func X509CsrOption2Template(opts ...X509CSROption) (tpl *x509.CertificateRequest, err error) {
	opt, err := new(x509CSROption).fillDefault().applyOpts(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "apply x509 CSR options")
	}

	tpl = &x509.CertificateRequest{
		SignatureAlgorithm: opt.signatureAlgorithm,
		Subject:            opt.subject,
		ExtraExtensions:    opt.extraExtensions,
		EmailAddresses:     opt.emailAddresses,
		DNSNames:           opt.dnsNames,
		IPAddresses:        opt.ipAddresses,
		URIs:               opt.uris,

		// these are fields that are not used by CreateCertificateRequest
		// PublicKeyAlgorithm: opt.publicKeyAlgorithm,
		// Extensions:      opt.extensions,
	}

	// Attributes backs the already-deprecated WithX509CSRAttribute option and is
	// kept only for compatibility.
	tpl.Attributes = opt.attributes //nolint:staticcheck // SA1019: see above.

	if opt.subject.CommonName == "" {
		return nil, errors.Errorf("common name is required")
	}

	return tpl, nil
}

// NewX509CSR new CSR
//
// # Arguments
//
// if prikey is not RSA private key, you must set SignatureAlgorithm by WithX509CertSignatureAlgorithm.
//
// Warning: CSR do not support set IsCA / KeyUsage / ExtKeyUsage,
// you should set these attributes in NewX509CertByCSR.
func NewX509CSR(prikey crypto.PrivateKey, opts ...X509CSROption) (csrDer []byte, err error) {
	if err = validPrikey(prikey); err != nil {
		return nil, err
	}

	csrTpl, err := X509CsrOption2Template(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "X509CsrOption2Template")
	}

	csrDer, err = x509.CreateCertificateRequest(rand.Reader, csrTpl, prikey)
	if err != nil {
		return nil, errors.Wrap(err, "create certificate")
	}

	return csrDer, nil
}
