package encrypt

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"math"
	"math/big"
	"net"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/Laisky/errors"

	gutils "github.com/Laisky/go-utils/v3"
)

type x509CSROption struct {
	err error

	commonName string
	organization, organizationUnit,
	locality, country, province, streetAddrs, PostalCode []string
	sans []string
	// signatureAlgorithm specific signature algorithm manually
	//
	// default to auto choose algorithm depends on certificate's algorithm
	signatureAlgorithm x509.SignatureAlgorithm
	// publicKeyAlgorithm specific publick key algorithm manually
	//
	// default to auto choose algorithm depends on certificate's algorithm
	publicKeyAlgorithm x509.PublicKeyAlgorithm
}

// X509CSROption option to generate tls certificate
type X509CSROption func(*x509CSROption) error

// WithX509CSRCommonName set common name
func WithX509CSRCommonName(commonName string) X509CSROption {
	return func(o *x509CSROption) error {
		o.commonName = commonName
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
func WithX509CSRPublicKeyAlgorithm(pubAlg x509.PublicKeyAlgorithm) X509CSROption {
	return func(o *x509CSROption) error {
		o.publicKeyAlgorithm = pubAlg
		return nil
	}
}

// WithX509CSROrganization set organization
func WithX509CSROrganization(organization ...string) X509CSROption {
	return func(o *x509CSROption) error {
		o.organization = append(o.organization, organization...)
		return nil
	}
}

// WithX509CSROrganizationUnit set organization units
func WithX509CSROrganizationUnit(ou ...string) X509CSROption {
	return func(o *x509CSROption) error {
		o.organizationUnit = append(o.organizationUnit, ou...)
		return nil
	}
}

// WithX509CSRLocality set subject localities
func WithX509CSRLocality(l ...string) X509CSROption {
	return func(o *x509CSROption) error {
		o.locality = append(o.locality, l...)
		return nil
	}
}

// WithX509CSRCountry set subject countries
func WithX509CSRCountry(values ...string) X509CSROption {
	return func(o *x509CSROption) error {
		o.country = append(o.country, values...)
		return nil
	}
}

// WithX509CSRProvince set subject provinces
func WithX509CSRProvince(values ...string) X509CSROption {
	return func(o *x509CSROption) error {
		o.province = append(o.province, values...)
		return nil
	}
}

// WithX509CSRStreetAddrs set subjuect street addresses
func WithX509CSRStreetAddrs(addrs ...string) X509CSROption {
	return func(o *x509CSROption) error {
		o.streetAddrs = append(o.streetAddrs, addrs...)
		return nil
	}
}

// WithX509CSRPostalCode set subjuect postal codes
func WithX509CSRPostalCode(codes ...string) X509CSROption {
	return func(o *x509CSROption) error {
		o.PostalCode = append(o.PostalCode, codes...)
		return nil
	}
}

// WithX509CertSANS set certificate SANs
//
// refer to RFC-5280 4.2.1.6
//
// auto WithX509CSRSANS to ip/email/url/dns
func WithX509CSRSANS(sans ...string) X509CSROption {
	return func(o *x509CSROption) error {
		o.sans = append(o.sans, sans...)
		return nil
	}
}

func (o *x509CSROption) applyOpts(opts ...X509CSROption) (*x509CSROption, error) {
	if o.err != nil {
		return nil, o.err
	}

	for _, f := range opts {
		if err := f(o); err != nil {
			return nil, err
		}
	}

	return o, nil
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

	opt, err := new(x509CSROption).applyOpts(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "apply options")
	}

	csrTpl := &x509.CertificateRequest{
		SignatureAlgorithm: opt.signatureAlgorithm,
		PublicKeyAlgorithm: opt.publicKeyAlgorithm,
		Subject: pkix.Name{
			Country:            opt.country,
			Organization:       opt.organization,
			OrganizationalUnit: opt.organizationUnit,
			Locality:           opt.locality,
			Province:           opt.province,
			StreetAddress:      opt.streetAddrs,
			PostalCode:         opt.PostalCode,
			CommonName:         opt.commonName,
		},
	}

	sansTpl := parseSans(opt.sans)
	csrTpl.DNSNames = sansTpl.DNSNames
	csrTpl.EmailAddresses = sansTpl.EmailAddresses
	csrTpl.IPAddresses = sansTpl.IPAddresses
	csrTpl.URIs = sansTpl.URIs

	csrDer, err = x509.CreateCertificateRequest(rand.Reader, csrTpl, prikey)
	if err != nil {
		return nil, errors.Wrap(err, "create certificate")
	}

	return csrDer, nil
}

// RandomSerialNumber generate random serial number
func RandomSerialNumber() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).SetInt64(math.MaxInt64))
}

// NewX509CertTemplate new tls template with common default values
func NewX509CertTemplate(opts ...X509CertOption) (tpl *x509.Certificate, err error) {
	opt, err := new(x509V3CertOption).fillDefault().applyOpts(opts...)
	if err != nil {
		return nil, err
	}

	notAfter := opt.validFrom.Add(opt.validFor)
	tpl = &x509.Certificate{
		SignatureAlgorithm: opt.signatureAlgorithm,
		SerialNumber:       opt.serialNumber,
		Subject: pkix.Name{
			CommonName:         opt.commonName,
			Organization:       opt.organization,
			OrganizationalUnit: opt.organizationUnit,
			Locality:           opt.locality,
			Country:            opt.country,
			Province:           opt.province,
			StreetAddress:      opt.streetAddrs,
			PostalCode:         opt.PostalCode,
		},
		NotBefore: opt.validFrom,
		NotAfter:  notAfter,

		KeyUsage:              opt.keyUsage,
		ExtKeyUsage:           opt.extKeyUsage,
		BasicConstraintsValid: true,
		IsCA:                  opt.isCA,
		PolicyIdentifiers:     opt.policies,
		CRLDistributionPoints: opt.crls,
		OCSPServer:            opt.ocsps,
	}

	sansTpl := parseSans(opt.sans)
	tpl.DNSNames = sansTpl.DNSNames
	tpl.EmailAddresses = sansTpl.EmailAddresses
	tpl.IPAddresses = sansTpl.IPAddresses
	tpl.URIs = sansTpl.URIs

	return tpl, nil
}

type sansTemp struct {
	DNSNames       []string
	EmailAddresses []string
	IPAddresses    []net.IP
	URIs           []*url.URL
}

func parseSans(sans []string) (tpl sansTemp) {
	for i := range sans {
		if ip := net.ParseIP(sans[i]); ip != nil {
			tpl.IPAddresses = append(tpl.IPAddresses, ip)
		} else if email, err := mail.ParseAddress(sans[i]); err == nil && email != nil {
			tpl.EmailAddresses = append(tpl.EmailAddresses, email.Address)
		} else if uri, err := url.ParseRequestURI(sans[i]); err == nil && uri != nil {
			tpl.URIs = append(tpl.URIs, uri)
		} else {
			tpl.DNSNames = append(tpl.DNSNames, sans[i])
		}
	}

	return tpl
}

type signCSROption struct {
	err error

	validFrom     time.Time
	validFor      time.Duration
	isCA, isCRLCA bool
	keyUsage      x509.KeyUsage
	extKeyUsage   []x509.ExtKeyUsage
	serialNumber  *big.Int
	// customSerialNum 不是自动生成的随机序列号，而是外部传入的用户指定的序列号
	customSerialNum bool
	// policies certificate policies
	//
	// refer to RFC-5280 4.2.1.4
	policies []asn1.ObjectIdentifier
	// crls crl endpoints
	crls []string
	// ocsps ocsp servers
	ocsps []string
}

func (o *signCSROption) fillDefault() *signCSROption {
	o.validFrom = time.Now().UTC()
	o.validFor = 7 * 24 * time.Hour
	o.keyUsage |= x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature
	o.extKeyUsage = append(o.extKeyUsage, x509.ExtKeyUsageServerAuth)

	if o.serialNumber, o.err = RandomSerialNumber(); o.err != nil {
		o.err = errors.Wrap(o.err, "generate random serial number")
	}

	return o
}

func (o *signCSROption) applyOpts(opts ...SignCSROption) (*signCSROption, error) {
	if o.err != nil {
		return nil, errors.WithStack(o.err)
	}

	for _, f := range opts {
		if err := f(o); err != nil {
			return nil, err
		}
	}

	return o, nil
}

// SignCSROption options for create certificate from CRL
type SignCSROption func(*signCSROption) error

// WithX509SignCSRPolicies set certificate policies
func WithX509SignCSRPolicies(policies ...asn1.ObjectIdentifier) SignCSROption {
	return func(o *signCSROption) error {
		o.policies = append(o.policies, policies...)
		return nil
	}
}

// WithX509SignCSROCSPServers set ocsp servers
func WithX509SignCSROCSPServers(ocsp ...string) SignCSROption {
	return func(o *signCSROption) error {
		o.ocsps = append(o.ocsps, ocsp...)
		return nil
	}
}

// WithX509SignCSRSeriaNumber set certificate/CRL's serial number
//
// refer to RFC-5280 5.2.3 &
//
// # Args
//
// seriaNumber:
//   - (optional): generate certificate
//   - (required): generate CRL
func WithX509SignCSRSeriaNumber(serialNumber *big.Int) SignCSROption {
	return func(o *signCSROption) error {
		if serialNumber == nil {
			return errors.Errorf("serial number shoule not be empty")
		}

		o.customSerialNum = true
		o.serialNumber = serialNumber
		return nil
	}
}

// WithX509SignCSRKeyUsage add key usage
func WithX509SignCSRKeyUsage(usage ...x509.KeyUsage) SignCSROption {
	return func(o *signCSROption) error {
		for i := range usage {
			o.keyUsage |= usage[i]
		}

		return nil
	}
}

// WithX509SignCSRCRLs add crl endpoints
func WithX509SignCSRCRLs(crlEndpoint ...string) SignCSROption {
	return func(o *signCSROption) error {
		o.crls = append(o.crls, crlEndpoint...)
		return nil
	}
}

// WithX509SignCSRExtKeyUsage add ext key usage
func WithX509SignCSRExtKeyUsage(usage ...x509.ExtKeyUsage) SignCSROption {
	return func(o *signCSROption) error {
		o.extKeyUsage = append(o.extKeyUsage, usage...)
		return nil
	}
}

// WithX509SignCSRValidFrom set valid from
func WithX509SignCSRValidFrom(validFrom time.Time) SignCSROption {
	return func(o *signCSROption) error {
		o.validFrom = validFrom
		return nil
	}
}

// WithX509SignCSRValidFor set valid for duration
func WithX509SignCSRValidFor(validFor time.Duration) SignCSROption {
	return func(o *signCSROption) error {
		o.validFor = validFor
		return nil
	}
}

// WithX509SignCSRIsCA set is ca
func WithX509SignCSRIsCA() SignCSROption {
	return func(o *signCSROption) error {
		o.isCA = true
		o.keyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
		return nil
	}
}

// WithX509SignCSRIsCRLCA set is ca to sign CRL
func WithX509SignCSRIsCRLCA() SignCSROption {
	return func(o *signCSROption) error {
		o.isCRLCA = true
		o.keyUsage |= x509.KeyUsageCRLSign
		return nil
	}
}

// NewX509CertByCSR sign CSR to certificate
func NewX509CertByCSR(
	ca *x509.Certificate,
	prikey crypto.PrivateKey,
	csrDer []byte,
	opts ...SignCSROption) (certDer []byte, err error) {
	if err = validPrikey(prikey); err != nil {
		return nil, err
	}

	opt, err := new(signCSROption).fillDefault().applyOpts(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "apply options")
	}

	if !ca.IsCA || (ca.KeyUsage&x509.KeyUsageCertSign) == x509.KeyUsage(0) {
		return nil, errors.Errorf("ca is invalid to sign cert")
	}

	csr, err := Der2CSR(csrDer)
	if err != nil {
		return nil, errors.Wrap(err, "parse csr")
	}

	notAfter := opt.validFrom.Add(opt.validFor)
	tpl := &x509.Certificate{
		Issuer:             ca.Subject,
		SignatureAlgorithm: csr.SignatureAlgorithm,
		PublicKeyAlgorithm: csr.PublicKeyAlgorithm,
		SerialNumber:       opt.serialNumber,
		Subject:            csr.Subject,

		NotBefore: opt.validFrom,
		NotAfter:  notAfter,

		KeyUsage:              opt.keyUsage,
		ExtKeyUsage:           opt.extKeyUsage,
		BasicConstraintsValid: true,
		IsCA:                  opt.isCA,
		PolicyIdentifiers:     opt.policies,
		CRLDistributionPoints: opt.crls,
		OCSPServer:            opt.ocsps,

		DNSNames:       csr.DNSNames,
		EmailAddresses: csr.EmailAddresses,
		IPAddresses:    csr.IPAddresses,
		URIs:           csr.URIs,
	}

	certDer, err = x509.CreateCertificate(rand.Reader, tpl, ca, csr.PublicKey, prikey)
	if err != nil {
		return nil, errors.Wrap(err, "create certificate")
	}

	return certDer, nil
}

type x509V3CertOption struct {
	err error

	signCSROption
	x509CSROption
}

func (o *x509V3CertOption) fillDefault() *x509V3CertOption {
	o.validFrom = time.Now().UTC()
	o.validFor = 7 * 24 * time.Hour
	o.keyUsage |= x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature
	o.extKeyUsage = append(o.extKeyUsage, x509.ExtKeyUsageServerAuth)

	if o.serialNumber, o.err = RandomSerialNumber(); o.err != nil {
		o.err = errors.Wrap(o.err, "generate random serial number")
	}

	return o
}

// X509CertOption option to generate tls certificate
type X509CertOption func(*x509V3CertOption) error

// WithX509CertCommonName set common name
func WithX509CertCommonName(commonName string) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.commonName = commonName
		return nil
	}
}

// WithX509CertPolicies set certificate policies
func WithX509CertPolicies(policies ...asn1.ObjectIdentifier) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.policies = append(o.policies, policies...)
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

		o.customSerialNum = true
		o.serialNumber = serialNumber
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

// WithX509CertOrganization set organization
func WithX509CertOrganization(organization ...string) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.organization = append(o.organization, organization...)
		return nil
	}
}

// WithX509CertOrganizationUnit set organization unit
func WithX509CertOrganizationUnit(ou ...string) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.organizationUnit = append(o.organizationUnit, ou...)
		return nil
	}
}

// WithX509CertLocality set subject localities
func WithX509CertLocality(l ...string) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.locality = append(o.locality, l...)
		return nil
	}
}

// WithX509CertCountry set subject countries
func WithX509CertCountry(values ...string) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.country = append(o.country, values...)
		return nil
	}
}

// WithX509CertProvince set subject provinces
func WithX509CertProvince(values ...string) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.province = append(o.province, values...)
		return nil
	}
}

// WithX509CertStreetAddrs set subjuect street addresses
func WithX509CertStreetAddrs(addrs ...string) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.streetAddrs = append(o.streetAddrs, addrs...)
		return nil
	}
}

// WithX509CertPostalCode set subjuect postal codes
func WithX509CertPostalCode(codes ...string) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.PostalCode = append(o.PostalCode, codes...)
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
		o.sans = append(o.sans, sans...)
		return nil
	}
}

// WithX509CertValidFrom set valid from
func WithX509CertValidFrom(validFrom time.Time) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.validFrom = validFrom
		return nil
	}
}

// WithX509CertValidFor set valid for duration
func WithX509CertValidFor(validFor time.Duration) X509CertOption {
	return func(o *x509V3CertOption) error {
		o.validFor = validFor
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
		o.isCRLCA = true
		o.keyUsage |= x509.KeyUsageCRLSign
		return nil
	}
}

func (o *x509V3CertOption) applyOpts(opts ...X509CertOption) (*x509V3CertOption, error) {
	if o.err != nil {
		return nil, o.err
	}

	for _, f := range opts {
		if err := f(o); err != nil {
			return nil, err
		}
	}

	return o, nil
}

// NewRSAPrikeyAndCert convient function to new rsa private key and cert
func NewRSAPrikeyAndCert(rsaBits RSAPrikeyBits, opts ...X509CertOption) (prikeyPem, certDer []byte, err error) {
	prikey, err := NewRSAPrikey(rsaBits)
	if err != nil {
		return nil, nil, errors.Wrap(err, "new rsa prikey")
	}

	prikeyPem, err = Prikey2Pem(prikey)
	if err != nil {
		return nil, nil, errors.Wrap(err, "convert prikey to pem")
	}

	certDer, err = NewX509Cert(prikey, opts...)
	return prikeyPem, certDer, errors.Wrap(err, "generate cert")
}

// Privkey2Signer convert privkey to signer
func Privkey2Signer(privkey crypto.PrivateKey) crypto.Signer {
	switch privkey := privkey.(type) {
	case *rsa.PrivateKey:
		return privkey
	case *ecdsa.PrivateKey:
		return privkey
	case ed25519.PrivateKey:
		return privkey
	default:
		return nil
	}
}

// NewX509CRL create and sign CRL
//
// # Args
//
//   - ca: CA to sign CRL.
//   - prikey: prikey for CA.
//   - revokeCerts: certifacates that will be revoked.
//   - WithX509CertSeriaNumber() is required for NewX509CRL.
//
// according to [RFC5280 5.2.3], X.509 v3 CRL could have a
// monotonically increasing sequence number as serial number.
//
// [RFC5280 5.2.3]: https://www.rfc-editor.org/rfc/rfc5280.html#section-5.2.3
func NewX509CRL(ca *x509.Certificate,
	prikey crypto.PrivateKey,
	revokeCerts []pkix.RevokedCertificate,
	opts ...X509CertOption) (crlDer []byte, err error) {
	if err = validPrikey(prikey); err != nil {
		return nil, err
	}

	if opt, err := new(x509V3CertOption).fillDefault().applyOpts(opts...); err != nil {
		return nil, err
	} else if !opt.customSerialNum {
		// do not use random serial number for CRL
		return nil, errors.Errorf("WithX509CertSeriaNumber() is required for NewX509CRL")
	}

	ctpl, err := NewX509CertTemplate(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "parse options")
	}

	tpl := &x509.RevocationList{
		Number: ctpl.SerialNumber,
		// Issuer:              ca.Subject,
		SignatureAlgorithm: ctpl.SignatureAlgorithm,
		ThisUpdate:         ctpl.NotBefore,
		NextUpdate:         ca.NotAfter,
		// Extensions:          ctpl.Extensions,
		ExtraExtensions:     ca.ExtraExtensions,
		RevokedCertificates: revokeCerts,
	}

	return x509.CreateRevocationList(rand.Reader, tpl, ca, Privkey2Signer(prikey))
}

// NewX509Cert new self sign tls cert
func NewX509Cert(prikey crypto.PrivateKey, opts ...X509CertOption) (certDer []byte, err error) {
	if err = validPrikey(prikey); err != nil {
		return nil, err
	}

	tpl, err := NewX509CertTemplate(opts...)
	if err != nil {
		return nil, err
	}

	certDer, err = x509.CreateCertificate(rand.Reader, tpl, tpl, GetPubkeyFromPrikey(prikey), prikey)
	if err != nil {
		return nil, errors.Wrap(err, "create certificate")
	}

	return certDer, nil
}

func validPrikey(prikey crypto.PrivateKey) error {
	if v := Privkey2Signer(prikey); v == nil {
		return errors.Errorf("not support this type of private key")
	}

	return nil
}

// VerifyCRLPkix verify crl by ca
func VerifyCRLPkix(ca *x509.Certificate, crl *pkix.CertificateList) error {
	return ca.CheckCRLSignature(crl)
}

// VerifyCRL verify crl by ca
func VerifyCRL(ca *x509.Certificate, crl *x509.RevocationList) error {
	return crl.CheckSignatureFrom(ca)
}

type oidContainsOption struct {
	prefix bool
}

func (o *oidContainsOption) applyfs(fs ...func(o *oidContainsOption) error) *oidContainsOption {
	o, _ = gutils.Pipeline(fs, o)
	return o
}

// MatchPrefix treat prefix inclusion as a match as well
//
//	`1.2.3` contains `1.2.3.4`
func MatchPrefix() func(o *oidContainsOption) error {
	return func(o *oidContainsOption) error {
		o.prefix = true
		return nil
	}
}

// OIDContains is oid in oids
func OIDContains(oids []asn1.ObjectIdentifier,
	oid asn1.ObjectIdentifier, opts ...func(o *oidContainsOption) error) bool {
	opt := new(oidContainsOption).applyfs(opts...)

	for i := range oids {
		if oids[i].Equal(oid) {
			return true
		}

		if opt.prefix && strings.HasPrefix(oids[i].String(), oid.String()) {
			return true
		}
	}

	return false
}

// ReadableX509Cert convert x509 certificate to readable jsonable map
func ReadableX509Cert(cert *x509.Certificate) (map[string]any, error) {
	return map[string]any{
		"subject": map[string]any{
			"country":             cert.Subject.Country,
			"organization":        cert.Subject.Organization,
			"organizational_unit": cert.Subject.OrganizationalUnit,
			"locality":            cert.Subject.Locality,
			"province":            cert.Subject.Province,
			"street_address":      cert.Subject.StreetAddress,
			"postal_code":         cert.Subject.PostalCode,
			"serial_number":       cert.Subject.SerialNumber,
			"common_name":         cert.Subject.CommonName,
		},
		"issuer": map[string]any{
			"country":             cert.Issuer.Country,
			"organization":        cert.Issuer.Organization,
			"organizational_unit": cert.Issuer.OrganizationalUnit,
			"locality":            cert.Issuer.Locality,
			"province":            cert.Issuer.Province,
			"street_address":      cert.Issuer.StreetAddress,
			"postal_code":         cert.Issuer.PostalCode,
			"serial_number":       cert.Issuer.SerialNumber,
			"common_name":         cert.Issuer.CommonName,
		},
		"signature_algorithm": cert.SignatureAlgorithm.String(),
		"publicKey_algorithm": cert.PublicKeyAlgorithm.String(),
		"not_before":          cert.NotBefore.Format(time.RFC3339),
		"not_after":           cert.NotAfter.Format(time.RFC3339),
		"key_usage":           ReadableX509KeyUsage(cert.KeyUsage),
		"ext_key_usage":       ReadableX509ExtKeyUsage(cert.ExtKeyUsage),
		"is_ca":               fmt.Sprintf("%t", cert.IsCA),
		"serial_number":       cert.SerialNumber.String(),
		"sans": map[string]any{
			"dns_names":       cert.DNSNames,
			"email_addresses": cert.EmailAddresses,
			"ip_addresses":    cert.IPAddresses,
			"uris":            cert.URIs,
		},
		"ocsps":              cert.OCSPServer,
		"cris":               cert.CRLDistributionPoints,
		"policy_identifiers": ReadableOIDs(cert.PolicyIdentifiers),
	}, nil
}

// ReadableX509KeyUsage convert x509 certificate key usages to readable strings
func ReadableX509KeyUsage(usage x509.KeyUsage) (usageNames []string) {
	for name, u := range map[string]x509.KeyUsage{
		"KeyUsageDigitalSignature":  x509.KeyUsageDigitalSignature,
		"KeyUsageContentCommitment": x509.KeyUsageContentCommitment,
		"KeyUsageKeyEncipherment":   x509.KeyUsageKeyEncipherment,
		"KeyUsageDataEncipherment":  x509.KeyUsageDataEncipherment,
		"KeyUsageKeyAgreement":      x509.KeyUsageKeyAgreement,
		"KeyUsageCertSign":          x509.KeyUsageCertSign,
		"KeyUsageCRLSign":           x509.KeyUsageCRLSign,
		"KeyUsageEncipherOnly":      x509.KeyUsageEncipherOnly,
		"KeyUsageDecipherOnly":      x509.KeyUsageDecipherOnly,
	} {
		if usage&u != 0 {
			usageNames = append(usageNames, name)
		}
	}

	return usageNames
}

// ReadableX509ExtKeyUsage convert x509 certificate ext key usages to readable strings
func ReadableX509ExtKeyUsage(usages []x509.ExtKeyUsage) (usageNames []string) {
	for _, u1 := range usages {
		for name, u2 := range map[string]x509.ExtKeyUsage{
			"ExtKeyUsageAny":                            x509.ExtKeyUsageAny,
			"ExtKeyUsageServerAuth":                     x509.ExtKeyUsageServerAuth,
			"ExtKeyUsageClientAuth":                     x509.ExtKeyUsageClientAuth,
			"ExtKeyUsageCodeSigning":                    x509.ExtKeyUsageCodeSigning,
			"ExtKeyUsageEmailProtection":                x509.ExtKeyUsageEmailProtection,
			"ExtKeyUsageIPSECEndSystem":                 x509.ExtKeyUsageIPSECEndSystem,
			"ExtKeyUsageIPSECTunnel":                    x509.ExtKeyUsageIPSECTunnel,
			"ExtKeyUsageIPSECUser":                      x509.ExtKeyUsageIPSECUser,
			"ExtKeyUsageTimeStamping":                   x509.ExtKeyUsageTimeStamping,
			"ExtKeyUsageOCSPSigning":                    x509.ExtKeyUsageOCSPSigning,
			"ExtKeyUsageMicrosoftServerGatedCrypto":     x509.ExtKeyUsageMicrosoftServerGatedCrypto,
			"ExtKeyUsageNetscapeServerGatedCrypto":      x509.ExtKeyUsageNetscapeServerGatedCrypto,
			"ExtKeyUsageMicrosoftCommercialCodeSigning": x509.ExtKeyUsageMicrosoftCommercialCodeSigning,
			"ExtKeyUsageMicrosoftKernelCodeSigning":     x509.ExtKeyUsageMicrosoftKernelCodeSigning,
		} {
			if u1 == u2 {
				usageNames = append(usageNames, name)
				break
			}
		}
	}

	return usageNames
}

// ReadableX509ExtKeyUsage convert objectids to readable strings
func ReadableOIDs(oids []asn1.ObjectIdentifier) (names []string) {
	for i := range oids {
		names = append(names, oids[i].String())
	}

	return names
}
