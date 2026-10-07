package crypto

import (
	"crypto"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"net"
	"net/url"
	"time"

	"github.com/Laisky/errors/v2"
)

type x509V3CertOption struct {
	parent *x509.Certificate
	signCSROption
	x509CSROption
}

// X509CertOption option to generate tls certificate
type X509CertOption func(*x509V3CertOption) error

// WithX509CsrOptions set csr options
func WithX509CsrOptions(csrOpts []X509CSROption) X509CertOption {
	return func(o *x509V3CertOption) error {
		for _, f := range csrOpts {
			if err := f(&o.x509CSROption); err != nil {
				return err
			}
		}

		return nil
	}
}

// WithX509CertCaMaxPathLen set max path length
func WithX509CertCaMaxPathLen(maxPathLen int) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.maxPathLen = &maxPathLen
		return nil
	}
}

// WithX509CertExtentions set extensions
//
// Deprecated: this field is ignored in x509 v3 certificate,
// use WithX509CertExtraExtensions instead if you want to set extensions.
func WithX509CertExtentions(_ ...pkix.Extension) X509CertOption {
	return func(_ *x509V3CertOption) error {
		// o.signCSROption.extensions = append(o.signCSROption.extensions, exts...)
		return nil
	}
}

// WithX509CertExtraExtensions set extra extensions
func WithX509CertExtraExtensions(exts ...pkix.Extension) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.signCSROption.extraExtensions = append(o.signCSROption.extraExtensions, exts...)
		return nil
	}
}

// WithX509CertParent set issuer
func WithX509CertParent(parent *x509.Certificate) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.parent = parent
		return nil
	}
}

// WithX509CertPolicies set certificate policies
func WithX509CertPolicies(policies ...asn1.ObjectIdentifier) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.policies = append(o.policies, policies...)
		for _, policy := range policies {
			ox509OID, err := OidAsn2X509(policy)
			if err != nil {
				return errors.Wrap(err, "convert policy oid to x509 oid")
			}
			o.policyOIDs = append(o.policyOIDs, ox509OID)
		}
		return nil
	}
}

// WithX509CertOCSPServers set ocsp servers
func WithX509CertOCSPServers(ocsp ...string) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.ocsps = append(o.ocsps, ocsp...)
		return nil
	}
}

// WithX509CertSeriaNumber set certificate/CRL's serial number
//
// refer to RFC-5280 5.2.3 &
//
// # Args
//
// seriaNumber:
//   - (optional): generate certificate
//   - (required): generate CRL
func WithX509CertSeriaNumber(serialNumber *big.Int) X509CertOption {
	return func(o *x509V3CertOption) error {
		if serialNumber == nil {
			return errors.Errorf("serial number shoule not be empty")
		}

		o.serialNumber = serialNumber
		return nil
	}
}

// WithX509CertSerialNumGenerator set serial number generator
func WithX509CertSerialNumGenerator(gen X509CertSerialNumberGenerator) X509CertOption {
	return func(o *x509V3CertOption) error {
		if gen == nil {
			return errors.Errorf("serial number generator shoule not be empty")
		}

		o.serialNumGenerator = gen
		return nil
	}
}

// WithX509CertKeyUsage add key usage
func WithX509CertKeyUsage(usage ...x509.KeyUsage) X509CertOption {
	return func(o *x509V3CertOption) error {
		for i := range usage {
			o.keyUsage |= usage[i]
		}

		return nil
	}
}

// WithX509CertCRLs add crl endpoints
func WithX509CertCRLs(crlEndpoint ...string) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.crls = append(o.crls, crlEndpoint...)
		return nil
	}
}

// WithX509CertExtKeyUsage add ext key usage
func WithX509CertExtKeyUsage(usage ...x509.ExtKeyUsage) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.extKeyUsage = append(o.extKeyUsage, usage...)
		return nil
	}
}

// WithX509CertSignatureAlgorithm set signature algorithm
func WithX509CertSignatureAlgorithm(sigAlg x509.SignatureAlgorithm) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.signatureAlgorithm = sigAlg
		return nil
	}
}

// WithX509CertPublicKeyAlgorithm set signature algorithm
//
// Deprecated: this field is ignored in x509 v3 certificate
func WithX509CertPublicKeyAlgorithm(_ x509.PublicKeyAlgorithm) X509CertOption {
	return func(_ *x509V3CertOption) error {
		// o.publicKeyAlgorithm = pubkeyAlg
		return nil
	}
}

// WithX509Subject set subject name
func WithX509Subject(subject pkix.Name) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.subject = subject
		return nil
	}
}

// WithX509CertCommonName set common name
func WithX509CertCommonName(commonName string) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.subject.CommonName = commonName
		return nil
	}
}

// WithX509CertOrganization set organization
func WithX509CertOrganization(organization ...string) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.subject.Organization = append(o.subject.Organization, organization...)
		return nil
	}
}

// WithX509CertOrganizationUnit set organization unit
func WithX509CertOrganizationUnit(ou ...string) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.subject.OrganizationalUnit = append(o.subject.OrganizationalUnit, ou...)
		return nil
	}
}

// WithX509CertLocality set subject localities
func WithX509CertLocality(l ...string) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.subject.Locality = append(o.subject.Locality, l...)
		return nil
	}
}

// WithX509CertCountry set subject countries
func WithX509CertCountry(values ...string) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.subject.Country = append(o.subject.Country, values...)
		return nil
	}
}

// WithX509CertProvince set subject provinces
func WithX509CertProvince(values ...string) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.subject.Province = append(o.subject.Province, values...)
		return nil
	}
}

// WithX509CertStreetAddrs set subjuect street addresses
func WithX509CertStreetAddrs(addrs ...string) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.subject.StreetAddress = append(o.subject.StreetAddress, addrs...)
		return nil
	}
}

// WithX509CertPostalCode set subjuect postal codes
func WithX509CertPostalCode(codes ...string) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.subject.PostalCode = append(o.subject.PostalCode, codes...)
		return nil
	}
}

// WithX509CertSANS set certificate SANs
//
// refer to RFC-5280 4.2.1.6
//
// auto parse to ip/email/url/dns
func WithX509CertSANS(sans ...string) X509CertOption {
	return func(o *x509V3CertOption) error {
		parsedSANs := parseSans(sans)
		o.dnsNames = append(o.dnsNames, parsedSANs.DNSNames...)
		o.emailAddresses = append(o.emailAddresses, parsedSANs.EmailAddresses...)
		o.uris = append(o.uris, parsedSANs.URIs...)
		o.ipAddresses = append(o.ipAddresses, parsedSANs.IPAddresses...)

		return nil
	}
}

// WithX509CertDNSNames set dns sans
func WithX509CertDNSNames(dnsNames ...string) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.dnsNames = append(o.dnsNames, dnsNames...)
		return nil
	}
}

// WithX509CertEmailAddrs set email sans
func WithX509CertEmailAddrs(emailAddresses ...string) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.emailAddresses = append(o.emailAddresses, emailAddresses...)
		return nil
	}
}

// WithX509CertIPAddrs set ip sans
func WithX509CertIPAddrs(ipAddresses ...net.IP) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.ipAddresses = append(o.ipAddresses, ipAddresses...)
		return nil
	}
}

// WithX509CertURIs set uri sans
func WithX509CertURIs(uris ...*url.URL) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.uris = append(o.uris, uris...)
		return nil
	}
}

// WithX509CertValidFrom set valid from
//
// deprecated: use WithX509CertNotBefore instead
func WithX509CertValidFrom(validFrom time.Time) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.notBefore = validFrom
		return nil
	}
}

// WithX509CertNotBefore set not before
func WithX509CertNotBefore(notBefore time.Time) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.notBefore = notBefore
		return nil
	}
}

// WithX509CertValidFor set valid for duration
//
// deprecated: use WithX509CertNotAfter instead
func WithX509CertValidFor(validFor time.Duration) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.notAfter = o.notBefore.Add(validFor)
		return nil
	}
}

// WithX509CertNotAfter set not after
//
// default to 30 days later
func WithX509CertNotAfter(notAfter time.Time) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.notAfter = notAfter
		return nil
	}
}

// WithX509CertIsCA set is ca
func WithX509CertIsCA() X509CertOption {
	return func(o *x509V3CertOption) error {
		o.isCA = true
		o.keyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
		return nil
	}
}

// WithX509CertIsCRLCA set is ca to sign CRL
func WithX509CertIsCRLCA() X509CertOption {
	return func(o *x509V3CertOption) error {
		o.isCA = true
		o.keyUsage |= x509.KeyUsageCRLSign
		return nil
	}
}

// WithX509CertPubkey set new certs' pubkey
func WithX509CertPubkey(pubkey crypto.PublicKey) X509CertOption {
	return func(o *x509V3CertOption) error {
		if pubkey == nil {
			return errors.Errorf("pubkey is nil")
		}

		o.pubkey = pubkey
		return nil
	}
}

func (o *x509V3CertOption) applyOpts(opts ...X509CertOption) (
	*x509V3CertOption, error) {
	// fill default
	if _, err := o.signCSROption.applyOpts(nil); err != nil {
		return nil, errors.Wrap(err, "sign csr option")
	}

	o.fillDefault()

	// apply options
	if o.err != nil {
		return nil, o.err
	}

	for _, f := range opts {
		if err := f(o); err != nil {
			return nil, err
		}
	}

	if o.serialNumber == nil {
		// generate serial number by internal generator if not set
		o.serialNumber = big.NewInt(o.serialNumGenerator.SerialNum())
	}
	if o.subject.CommonName == "" {
		return nil, errors.Errorf("common name must be set")
	}

	return o, nil
}
