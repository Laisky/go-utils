package crypto

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"math/big"
	"net"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	gutils "github.com/Laisky/go-utils/v4"
	gcounter "github.com/Laisky/go-utils/v4/counter"
	glog "github.com/Laisky/go-utils/v4/log"
)

// X509CertSerialNumberGenerator x509 certificate serial number generator
type X509CertSerialNumberGenerator interface {
	SerialNum() int64
}

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
	return func(o *x509CSROption) error {
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
	return func(o *x509CSROption) error {
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

// WithX509CertSANS set certificate SANs
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

func (o *x509CSROption) fillDefault() *x509CSROption {
	return o
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

// X509CsrOption2Template convert X509CSROption to x509.CertificateRequest
func X509CsrOption2Template(opts ...X509CSROption) (tpl *x509.CertificateRequest, err error) {
	opt, err := new(x509CSROption).fillDefault().applyOpts(opts...)
	if err != nil {
		return nil, err
	}

	tpl = &x509.CertificateRequest{
		SignatureAlgorithm: opt.signatureAlgorithm,
		Subject:            opt.subject,
		ExtraExtensions:    opt.extraExtensions,
		Attributes:         opt.attributes,
		EmailAddresses:     opt.emailAddresses,
		DNSNames:           opt.dnsNames,
		IPAddresses:        opt.ipAddresses,
		URIs:               opt.uris,

		// these are fields that are not used by CreateCertificateRequest
		// PublicKeyAlgorithm: opt.publicKeyAlgorithm,
		// Extensions:      opt.extensions,
	}

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

var (
	internalCertSerialNumGenerator X509CertSerialNumberGenerator
)

func init() {
	var err error
	if internalCertSerialNumGenerator, err = NewDefaultX509CertSerialNumGenerator(); err != nil {
		glog.Shared.Panic("new default cert serial number generator", zap.Error(err))
	}
}

// DefaultX509CertSerialNumGenerator default cert serial number generator base on epoch time and random int
type DefaultX509CertSerialNumGenerator struct {
	counter *gcounter.RotateCounter
}

// NewDefaultX509CertSerialNumGenerator new DefaultX509CertSerialNumGenerator
func NewDefaultX509CertSerialNumGenerator() (*DefaultX509CertSerialNumGenerator, error) {
	serialCounter, err := gcounter.NewRotateCounter(10000)
	if err != nil {
		return nil, errors.Wrap(err, "new counter")
	}

	return &DefaultX509CertSerialNumGenerator{
		counter: serialCounter,
	}, nil
}

// SerialNum get randon serial number
func (g *DefaultX509CertSerialNumGenerator) SerialNum() int64 {
	return time.Now().UnixMilli()*10000 + g.counter.Count()
}

// NewX509CertTemplate new tls template with common default values
// func NewX509CertTemplate(opts ...X509CertOption) (tpl *x509.Certificate, err error) {
// 	opt, err := new(x509V3CertOption).fillDefault().applyOpts(opts...)
// 	if err != nil {
// 		return nil, err
// 	}

// 	notAfter := opt.validFrom.Add(opt.validFor)
// 	tpl = &x509.Certificate{
// 		SignatureAlgorithm: opt.signatureAlgorithm,
// 		SerialNumber:       opt.serialNumber,
// 		Subject: pkix.Name{
// 			CommonName:         opt.commonName,
// 			Organization:       opt.organization,
// 			OrganizationalUnit: opt.organizationUnit,
// 			Locality:           opt.locality,
// 			Country:            opt.country,
// 			Province:           opt.province,
// 			StreetAddress:      opt.streetAddrs,
// 			PostalCode:         opt.PostalCode,
// 		},
// 		NotBefore: opt.validFrom,
// 		NotAfter:  notAfter,

// 		KeyUsage:              opt.keyUsage,
// 		ExtKeyUsage:           opt.extKeyUsage,
// 		BasicConstraintsValid: true,
// 		IsCA:                  opt.isCA,
// 		PolicyIdentifiers:     opt.policies,
// 		CRLDistributionPoints: opt.crls,
// 		OCSPServer:            opt.ocsps,
// 	}

// 	sansTpl := parseSans(opt.sans)
// 	tpl.DNSNames = sansTpl.DNSNames
// 	tpl.EmailAddresses = sansTpl.EmailAddresses
// 	tpl.IPAddresses = sansTpl.IPAddresses
// 	tpl.URIs = sansTpl.URIs

// 	return tpl, nil
// }

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
	notBefore    time.Time
	notAfter     time.Time
	isCA         bool
	keyUsage     x509.KeyUsage
	extKeyUsage  []x509.ExtKeyUsage
	serialNumber *big.Int
	// policies certificate policies
	//
	// refer to RFC-5280 4.2.1.4
	policies []asn1.ObjectIdentifier
	// crls crl endpoints
	crls []string
	// ocsps ocsp servers
	ocsps         []string
	signatureAlgo x509.SignatureAlgorithm
	// pubkeyAlgo    x509.PublicKeyAlgorithm

	// extensions,
	extraExtensions []pkix.Extension

	// pubkey csr will specific csr's pubkey, not use ca's pubkey
	pubkey             crypto.PublicKey
	serialNumGenerator X509CertSerialNumberGenerator
	// maxPathLen set CA path length constraint
	maxPathLen *int
}

func (o *signCSROption) fillDefault(csr *x509.CertificateRequest) *signCSROption {
	o.notBefore = time.Now().UTC()
	o.notAfter = o.notBefore.Add(7 * 24 * time.Hour)
	o.keyUsage |= x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature
	o.serialNumGenerator = internalCertSerialNumGenerator

	if csr != nil {
		if o.signatureAlgo == x509.UnknownSignatureAlgorithm {
			o.signatureAlgo = csr.SignatureAlgorithm
		}

		// if o.pubkeyAlgo == x509.UnknownPublicKeyAlgorithm {
		// 	o.pubkeyAlgo = csr.PublicKeyAlgorithm
		// }

		// o.extensions = append(o.extensions, csr.Extensions...)
		o.extraExtensions = append(o.extraExtensions, csr.ExtraExtensions...)
	}

	return o
}

func (o *signCSROption) applyOpts(opts ...SignCSROption) (*signCSROption, error) {
	for _, f := range opts {
		if err := f(o); err != nil {
			return nil, err
		}
	}

	switch {
	case o.serialNumber == nil:
		// generate serial number by internal generator if not set
		o.serialNumber = big.NewInt(o.serialNumGenerator.SerialNum())
	}

	return o, nil
}

// SignCSROption options for create certificate from CRL
type SignCSROption func(*signCSROption) error

// WithX509CaMaxPathLen set max path length
//
// only CAs are allowed to specify MaxPathLen
func WithX509CaMaxPathLen(maxPathLen int) SignCSROption {
	return func(o *signCSROption) error {
		o.maxPathLen = &maxPathLen
		return nil
	}
}

// WithX509SerialNumGenerator set serial number generator
func WithX509SerialNumGenerator(gen X509CertSerialNumberGenerator) SignCSROption {
	return func(o *signCSROption) error {
		o.serialNumGenerator = gen
		return nil
	}
}

// WithX509SignSignatureAlgorithm set signature algorithm
func WithX509SignSignatureAlgorithm(algo x509.SignatureAlgorithm) SignCSROption {
	return func(o *signCSROption) error {
		o.signatureAlgo = algo
		return nil
	}
}

// WithX509SignPublicKeyAlgorithm set public key algorithm
//
// Deprecated: this field is ignored by golang built-in x509 library
func WithX509SignPublicKeyAlgorithm(_ x509.PublicKeyAlgorithm) SignCSROption {
	return func(o *signCSROption) error {
		// o.pubkeyAlgo = algo
		return nil
	}
}

// WithX509SignCSRExtenstions set certificate extensions
//
// Extensions contains all requested extensions, in raw form. When parsing
// CSRs, this can be used to extract extensions that are not parsed by this
// package.
//
// Deprecated: this field is ignored by golang built-in x509 library,
// use WithX509SignCSRExtraExtenstions instead if you want to set extensions.
func WithX509SignCSRExtenstions(_ ...pkix.Extension) SignCSROption {
	return func(o *signCSROption) error {
		// o.extensions = append(o.extensions, exts...)
		return nil
	}
}

// WithX509SignCSRExtraExtenstions set certificate extra extensions
//
// ExtraExtensions contains extensions to be copied, raw, into any CSR
// marshaled by CreateCertificateRequest. Values override any extensions
// that would otherwise be produced based on the other fields but are
// overridden by any extensions specified in Attributes.
//
// The ExtraExtensions field is not populated by ParseCertificateRequest,
// see Extensions instead.
func WithX509SignCSRExtraExtenstions(exts ...pkix.Extension) SignCSROption {
	return func(o *signCSROption) error {
		o.extraExtensions = append(o.extraExtensions, exts...)
		return nil
	}
}

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
//
// deprecated: use WithX509SignCSRNotBefore instead
func WithX509SignCSRValidFrom(validFrom time.Time) SignCSROption {
	return func(o *signCSROption) error {
		o.notBefore = validFrom
		return nil
	}
}

// WithX509SignCSRNotBefore set valid from
func WithX509SignCSRNotBefore(notBefore time.Time) SignCSROption {
	return func(o *signCSROption) error {
		o.notBefore = notBefore
		return nil
	}
}

// WithX509SignCSRValidFor set valid for duration
//
// deprecated: use WithX509SignCSRNotAfter instead
func WithX509SignCSRValidFor(validFor time.Duration) SignCSROption {
	return func(o *signCSROption) error {
		o.notAfter = o.notBefore.Add(validFor)
		return nil
	}
}

// WithX509SignCSRNotAfter set valid for duration
func WithX509SignCSRNotAfter(notAfter time.Time) SignCSROption {
	return func(o *signCSROption) error {
		o.notAfter = notAfter
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
		o.isCA = true
		o.keyUsage |= x509.KeyUsageCRLSign
		return nil
	}
}

// NewX509CertByCSR sign CSR to certificate
//
// Depends on RFC-5280 4.2.1.12, empty ext key usage is as same as any key usage.
// so do not set any default ext key usages.
//
//   - https://github.com/golang/go/blob/1e9ff255a130200fcc4ec5e911d28181fce947d5/src/crypto/x509/verify.go#L1118
//
// but key usage is required in many cases:
//
//   - https://github.com/golang/go/blob/e04be8b24c20816f3429a8193c324ea67892e61f/src/crypto/x509/x509.go#L2165
func NewX509CertByCSR(
	parent *x509.Certificate,
	prikey crypto.PrivateKey,
	csrDer []byte,
	opts ...SignCSROption) (certDer []byte, err error) {
	if err = validPrikey(prikey); err != nil {
		return nil, err
	}

	csr, err := Der2CSR(csrDer)
	if err != nil {
		return nil, errors.Wrap(err, "parse csr")
	}

	opt, err := new(signCSROption).fillDefault(csr).applyOpts(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "apply options")
	}

	if !parent.IsCA || (parent.KeyUsage&x509.KeyUsageCertSign) == x509.KeyUsage(0) {
		return nil, errors.Errorf("parent certificate does not have CA flag or key usage")
	}

	certOpts := []X509CertOption{
		WithX509Subject(csr.Subject),
		WithX509CertParent(parent),
		WithX509CertNotBefore(opt.notBefore),
		WithX509CertNotAfter(opt.notAfter),
		WithX509CertPolicies(opt.policies...),
		WithX509CertCRLs(opt.crls...),
		WithX509CertOCSPServers(opt.ocsps...),
		WithX509CertKeyUsage(opt.keyUsage),
		WithX509CertExtKeyUsage(opt.extKeyUsage...),
		WithX509CertSeriaNumber(opt.serialNumber),
		WithX509CertDNSNames(csr.DNSNames...),
		WithX509CertEmailAddrs(csr.EmailAddresses...),
		WithX509CertIPAddrs(csr.IPAddresses...),
		WithX509CertURIs(csr.URIs...),
		WithX509CertPubkey(csr.PublicKey),
		WithX509CertSignatureAlgorithm(opt.signatureAlgo),
		// WithX509CertPublicKeyAlgorithm(opt.pubkeyAlgo),
		// WithX509CertExtentions(opt.extensions...),
		WithX509CertExtraExtensions(opt.extraExtensions...),
	}
	if opt.isCA {
		certOpts = append(certOpts, WithX509CertIsCA())
	}
	if opt.serialNumGenerator != nil {
		certOpts = append(certOpts, WithX509CertSerialNumGenerator(opt.serialNumGenerator))
	}
	if opt.maxPathLen != nil {
		certOpts = append(certOpts, WithX509CertCaMaxPathLen(*opt.maxPathLen))
	}

	return NewX509Cert(prikey, certOpts...)
}

type x509V3CertOption struct {
	parent *x509.Certificate
	signCSROption
	x509CSROption
}

func (o *x509V3CertOption) fillDefault() *x509V3CertOption {
	o.signCSROption.fillDefault(nil)
	o.x509CSROption.fillDefault()

	return o
}

// X509CertOption option to generate tls certificate
type X509CertOption func(*x509V3CertOption) error

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
	return func(o *x509V3CertOption) error {
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
	return func(o *x509V3CertOption) error {
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

// NewRSAPrikeyAndCert convient function to new rsa private key and cert
func NewRSAPrikeyAndCert(rsaBits RSAPrikeyBits, opts ...X509CertOption) (
	prikeyPem, certDer []byte, err error) {
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

// NewECDSAPrikeyAndCert convient function to new ecdsa private key and cert
func NewECDSAPrikeyAndCert(curve ECDSACurve, opts ...X509CertOption) (
	prikeyPem, certDer []byte, err error) {
	prikey, err := NewECDSAPrikey(curve)
	if err != nil {
		return nil, nil, errors.Wrap(err, "new ecdsa prikey")
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

type x509CRLOption struct {
	// signatureAlgorithm specific signature algorithm manually
	//
	// default to auto choose algorithm depends on certificate's algorithm
	signatureAlgorithm x509.SignatureAlgorithm
	// thisUpdate (optional) default to now
	thisUpdate time.Time
	// nextUpdate (optional) default to 30days later
	nextUpdate time.Time
}

func (o *x509CRLOption) fillDefault() *x509CRLOption {
	o.thisUpdate = gutils.Clock.GetUTCNow()
	o.nextUpdate = o.thisUpdate.Add(30 * 24 * time.Hour)
	return o
}

func (o *x509CRLOption) applyOpts(opts ...X509CRLOption) (*x509CRLOption, error) {
	for i := range opts {
		if err := opts[i](o); err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return o, nil
}

// X509CRLOption options for create x509 CRL
type X509CRLOption func(*x509CRLOption) error

// WithX509CRLSignatureAlgorithm set signature algorithm
//
// default to auto choose algorithm depends on certificate's algorithm
func WithX509CRLSignatureAlgorithm(algo x509.SignatureAlgorithm) X509CRLOption {
	return func(o *x509CRLOption) error {
		o.signatureAlgorithm = algo
		return nil
	}
}

// WithX509CRLThisUpdate set this update
//
// default to now
func WithX509CRLThisUpdate(thisUpdate time.Time) X509CRLOption {
	return func(o *x509CRLOption) error {
		o.thisUpdate = thisUpdate
		return nil
	}
}

// WithX509CRLNextUpdate set next update
//
// default to 30 days later
func WithX509CRLNextUpdate(nextUpdate time.Time) X509CRLOption {
	return func(o *x509CRLOption) error {
		o.nextUpdate = nextUpdate
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
	seriaNumber *big.Int,
	revokeCerts []pkix.RevokedCertificate,
	opts ...X509CRLOption) (crlDer []byte, err error) {
	if err = validPrikey(prikey); err != nil {
		return nil, errors.WithStack(err)
	}

	if seriaNumber == nil {
		return nil, errors.Errorf("seriaNumber is empty")
	}

	opt, err := new(x509CRLOption).fillDefault().applyOpts(opts...)
	if err != nil {
		return nil, err
	}

	tpl := &x509.RevocationList{
		Number:              seriaNumber,
		SignatureAlgorithm:  opt.signatureAlgorithm,
		ThisUpdate:          opt.thisUpdate,
		NextUpdate:          opt.nextUpdate,
		ExtraExtensions:     ca.ExtraExtensions,
		RevokedCertificates: revokeCerts,
	}

	return x509.CreateRevocationList(rand.Reader, tpl, ca, Privkey2Signer(prikey))
}

// x509CertOption2Template convert X509CertOption to x509.Certificate template
func x509CertOption2Template(opts ...X509CertOption) (
	opt *x509V3CertOption, certTemplate *x509.Certificate, err error) {
	if opt, err = new(x509V3CertOption).fillDefault().applyOpts(opts...); err != nil {
		return nil, nil, errors.Wrap(err, "apply options")
	}

	tpl := &x509.Certificate{
		SignatureAlgorithm:    opt.signatureAlgorithm,
		SerialNumber:          opt.serialNumber,
		Subject:               opt.subject,
		NotBefore:             opt.notBefore,
		NotAfter:              opt.notAfter,
		KeyUsage:              opt.keyUsage,
		ExtKeyUsage:           opt.extKeyUsage,
		BasicConstraintsValid: true,
		IsCA:                  opt.isCA,
		PolicyIdentifiers:     opt.policies,
		CRLDistributionPoints: opt.crls,
		OCSPServer:            opt.ocsps,
		EmailAddresses:        opt.emailAddresses,
		DNSNames:              opt.dnsNames,
		IPAddresses:           opt.ipAddresses,
		URIs:                  opt.uris,
		// Extensions:            opt.signCSROption.extensions,
		ExtraExtensions: opt.signCSROption.extraExtensions,
	}

	if opt.maxPathLen != nil {
		tpl.MaxPathLen = *opt.maxPathLen
		if tpl.MaxPathLen == 0 {
			tpl.MaxPathLenZero = true
		}
	}

	return opt, tpl, nil
}

// NewX509Cert new cert
func NewX509Cert(prikey crypto.PrivateKey, opts ...X509CertOption) (certDer []byte, err error) {
	if err = validPrikey(prikey); err != nil {
		return nil, errors.Wrap(err, "valid prikey")
	}

	opt, tpl, err := x509CertOption2Template(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "convert options to template")
	}

	if opt.pubkey == nil {
		opt.pubkey = Prikey2Pubkey(prikey)
	}

	// CreateCertificate x509.CreateCertificate will auto generate subject key id for ca template
	if !opt.isCA {
		if tpl.SubjectKeyId, err = X509CertSubjectKeyID(opt.pubkey); err != nil {
			return nil, errors.Wrap(err, "generate cert subject key id")
		}
	}

	parent := tpl
	if opt.parent != nil {
		parent = opt.parent
	}

	certDer, err = x509.CreateCertificate(rand.Reader, tpl, parent, opt.pubkey, prikey)
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
	pubkey, err := Pubkey2Pem(cert.PublicKey)
	if err != nil {
		return nil, errors.Wrap(err, "convert public key to pem")
	}

	v := map[string]any{
		"subject":                 ReadablePkixName(cert.Subject),
		"issuer":                  ReadablePkixName(cert.Issuer),
		"subject_key_id_base64":   gutils.EncodeByBase64(cert.SubjectKeyId),
		"authority_key_id_base64": gutils.EncodeByBase64(cert.AuthorityKeyId),
		"signature_algorithm":     cert.SignatureAlgorithm.String(),
		"public_key_algorithm":    cert.PublicKeyAlgorithm.String(),
		"not_before":              cert.NotBefore.Format(time.RFC3339),
		"not_after":               cert.NotAfter.Format(time.RFC3339),
		"key_usage":               ReadableX509KeyUsage(cert.KeyUsage),
		"ext_key_usage":           ReadableX509ExtKeyUsage(cert.ExtKeyUsage),
		"is_ca":                   fmt.Sprintf("%t", cert.IsCA),
		"serial_number":           cert.SerialNumber.String(),
		"public_key":              string(pubkey),
		"sans": map[string]any{
			"dns_names":       cert.DNSNames,
			"email_addresses": cert.EmailAddresses,
			"ip_addresses":    cert.IPAddresses,
			"uris":            cert.URIs,
		},
		"ocsps":              cert.OCSPServer,
		"cris":               cert.CRLDistributionPoints,
		"policy_identifiers": ReadableOIDs(cert.PolicyIdentifiers),
	}
	return gutils.RemoveEmptyVal(v), nil
}

// ReadableX509CSR convert x509 certificate request to readable jsonable map
func ReadableX509CSR(csr *x509.CertificateRequest) (map[string]any, error) {
	pubkey, err := Pubkey2Pem(csr.PublicKey)
	if err != nil {
		return nil, errors.Wrap(err, "convert public key to pem")
	}

	v := map[string]any{
		"subject":              ReadablePkixName(csr.Subject),
		"signature_algorithm":  csr.SignatureAlgorithm.String(),
		"public_key_algorithm": csr.PublicKeyAlgorithm.String(),
		"public_key":           string(pubkey),
		"sans": map[string]any{
			"dns_names":       csr.DNSNames,
			"email_addresses": csr.EmailAddresses,
			"ip_addresses":    csr.IPAddresses,
			"uris":            csr.URIs,
		},
	}
	return gutils.RemoveEmptyVal(v), nil
}

// ReadableX509Extention convert x509 certificate extension to readable jsonable map
func ReadableX509Extention(ext *pkix.Extension) (map[string]any, error) {
	v := map[string]any{
		"oid":           ext.Id.String(),
		"critical":      fmt.Sprintf("%t", ext.Critical),
		"raw_value_b64": gutils.EncodeByBase64(ext.Value),
	}
	return gutils.RemoveEmptyVal(v), nil
}

// ReadableX509KeyUsage convert x509 certificate key usages to readable strings
func ReadableX509KeyUsage(usage x509.KeyUsage) (usageNames []string) {
	for name, u := range map[string]x509.KeyUsage{
		"DigitalSignature":  x509.KeyUsageDigitalSignature,
		"ContentCommitment": x509.KeyUsageContentCommitment,
		"KeyEncipherment":   x509.KeyUsageKeyEncipherment,
		"DataEncipherment":  x509.KeyUsageDataEncipherment,
		"KeyAgreement":      x509.KeyUsageKeyAgreement,
		"CertSign":          x509.KeyUsageCertSign,
		"CRLSign":           x509.KeyUsageCRLSign,
		"EncipherOnly":      x509.KeyUsageEncipherOnly,
		"DecipherOnly":      x509.KeyUsageDecipherOnly,
	} {
		if usage&u != 0 {
			usageNames = append(usageNames, name)
		}
	}

	return usageNames
}

// ReadablePkixName convert pkix.Name to readable map with strings
func ReadablePkixName(name pkix.Name) map[string]any {
	m := map[string]any{
		"country":             name.Country,
		"organization":        name.Organization,
		"organizational_unit": name.OrganizationalUnit,
		"locality":            name.Locality,
		"province":            name.Province,
		"street_address":      name.StreetAddress,
		"postal_code":         name.PostalCode,
		"serial_number":       name.SerialNumber,
		"common_name":         name.CommonName,
	}

	return gutils.RemoveEmptyVal(m)
}

// ReadableX509ExtKeyUsage convert x509 certificate ext key usages to readable strings
func ReadableX509ExtKeyUsage(usages []x509.ExtKeyUsage) (usageNames []string) {
	for _, u1 := range usages {
		for name, u2 := range map[string]x509.ExtKeyUsage{
			"Any":                            x509.ExtKeyUsageAny,
			"ServerAuth":                     x509.ExtKeyUsageServerAuth,
			"ClientAuth":                     x509.ExtKeyUsageClientAuth,
			"CodeSigning":                    x509.ExtKeyUsageCodeSigning,
			"EmailProtection":                x509.ExtKeyUsageEmailProtection,
			"IPSECEndSystem":                 x509.ExtKeyUsageIPSECEndSystem,
			"IPSECTunnel":                    x509.ExtKeyUsageIPSECTunnel,
			"IPSECUser":                      x509.ExtKeyUsageIPSECUser,
			"TimeStamping":                   x509.ExtKeyUsageTimeStamping,
			"OCSPSigning":                    x509.ExtKeyUsageOCSPSigning,
			"MicrosoftServerGatedCrypto":     x509.ExtKeyUsageMicrosoftServerGatedCrypto,
			"NetscapeServerGatedCrypto":      x509.ExtKeyUsageNetscapeServerGatedCrypto,
			"MicrosoftCommercialCodeSigning": x509.ExtKeyUsageMicrosoftCommercialCodeSigning,
			"MicrosoftKernelCodeSigning":     x509.ExtKeyUsageMicrosoftKernelCodeSigning,
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

// X509CertSubjectKeyID generate subject key id for pubkey
//
// if x509 certificate template is a CA, subject key id will generated by golang automatelly
//
//   - https://cs.opensource.google/go/go/+/refs/tags/go1.19.5:src/crypto/x509/x509.go;l=1476
func X509CertSubjectKeyID(pubkey crypto.PublicKey) ([]byte, error) {
	keyBytes, err := Pubkey2Der(pubkey)
	if err != nil {
		return nil, errors.Wrap(err, "marshal pubkeu")
	}

	hasher := sha1.New()
	hasher.Sum(keyBytes)
	return hasher.Sum(nil), nil
}
