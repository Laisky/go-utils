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
	"math"
	"math/big"
	"net"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/Laisky/errors"
	"github.com/jinzhu/copier"

	gutils "github.com/Laisky/go-utils/v3"
)

// NewX509CSR new CSR
//
// # Arguments
//
// if prikey is not RSA private key, you must set SignatureAlgorithm by WithX509CertSignatureAlgorithm.
//
// Warning: CSR do not support set IsCA / KeyUsage / ExtKeyUsage,
// you should set these attributes in NewX509CertByCSR.
func NewX509CSR(prikey crypto.PrivateKey, opts ...X509CertOption) (csrDer []byte, err error) {
	if err = validPrikey(prikey); err != nil {
		return nil, err
	}

	tpl, err := NewX509CertTemplate(opts...)
	if err != nil {
		return nil, err
	}

	if tpl.IsCA {
		return nil, errors.Errorf("CSR do not support CA, should set CA in NewX509CertByCSR")
	}

	csrTpl := &x509.CertificateRequest{}
	if err = copier.Copy(csrTpl, tpl); err != nil {
		return nil, errors.Wrap(err, "copy attributes from options to template")
	}

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

// NewX509CertTemplate new tls template
func NewX509CertTemplate(opts ...X509CertOption) (tpl *x509.Certificate, err error) {
	opt, err := new(tlsCertOption).fillDefault().applyOpts(opts...)
	if err != nil {
		return nil, err
	}

	notAfter := opt.validFrom.Add(opt.validFor)
	template := &x509.Certificate{
		SignatureAlgorithm: opt.signatureAlgorithm,
		SerialNumber:       opt.serialNumber,
		Subject: pkix.Name{
			CommonName:         opt.commonName,
			Organization:       opt.organization,
			OrganizationalUnit: opt.organizationUnit,
			Locality:           opt.locality,
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
	parseAndFillSans(template, opt.sans)
	return template, nil
}

func parseAndFillSans(tpl *x509.Certificate, sans []string) {
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
}

// NewX509CertByCSR sign CSR to certificate
//
// csr's attributes will overweite option's attributes.
// you need verify csr manually before invoke this function.
func NewX509CertByCSR(
	ca *x509.Certificate,
	prikey crypto.PrivateKey,
	csrDer []byte,
	opts ...X509CertOption) (certDer []byte, err error) {
	if err = validPrikey(prikey); err != nil {
		return nil, err
	}

	if !ca.IsCA || (ca.KeyUsage&x509.KeyUsageCertSign) == x509.KeyUsage(0) {
		return nil, errors.Errorf("ca is invalid to sign cert")
	}

	csr, err := Der2CSR(csrDer)
	if err != nil {
		return nil, errors.Wrap(err, "parse csr")
	}

	// create client certificate template
	tpl, err := NewX509CertTemplate(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "new cert template")
	}
	tpl.Issuer = ca.Subject
	if err = copier.Copy(tpl, csr); err != nil {
		return nil, errors.Wrap(err, "copy from csr")
	}

	certDer, err = x509.CreateCertificate(rand.Reader, tpl, ca, csr.PublicKey, prikey)
	if err != nil {
		return nil, errors.Wrap(err, "create certificate")
	}

	return certDer, nil
}

type tlsCertOption struct {
	err error

	commonName    string
	validFrom     time.Time
	validFor      time.Duration
	isCA, isCRLCA bool
	organization,
	organizationUnit,
	locality []string
	sans         []string
	keyUsage     x509.KeyUsage
	extKeyUsage  []x509.ExtKeyUsage
	serialNumber *big.Int
	// customSerialNum 不是自动生成的随机序列号，而是外部传入的用户指定的序列号
	customSerialNum bool
	// signatureAlgorithm specific signature algorithm manually
	//
	// default to auto choose algorithm depends on certificate's algorithm
	signatureAlgorithm x509.SignatureAlgorithm
	// policies certificate policies
	//
	// refer to RFC-5280 4.2.1.4
	policies []asn1.ObjectIdentifier
	// crls crl endpoints
	crls []string
	// ocsps ocsp servers
	ocsps []string
}

func (o *tlsCertOption) fillDefault() *tlsCertOption {
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
type X509CertOption func(*tlsCertOption) error

// WithX509CertCommonName set common name
func WithX509CertCommonName(commonName string) X509CertOption {
	return func(o *tlsCertOption) error {
		o.commonName = commonName
		return nil
	}
}

// WithX509CertPolicies set certificate policies
func WithX509CertPolicies(policies ...asn1.ObjectIdentifier) X509CertOption {
	return func(o *tlsCertOption) error {
		o.policies = append(o.policies, policies...)
		return nil
	}
}

// WithX509CertOCSPServers set ocsp servers
func WithX509CertOCSPServers(ocsp ...string) X509CertOption {
	return func(o *tlsCertOption) error {
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
	return func(o *tlsCertOption) error {
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
	return func(o *tlsCertOption) error {
		for i := range usage {
			o.keyUsage |= usage[i]
		}

		return nil
	}
}

// WithX509CertCRLs add crl endpoints
func WithX509CertCRLs(crlEndpoint ...string) X509CertOption {
	return func(o *tlsCertOption) error {
		o.crls = append(o.crls, crlEndpoint...)
		return nil
	}
}

// WithX509ExtCertKeyUsage add ext key usage
func WithX509ExtCertKeyUsage(usage ...x509.ExtKeyUsage) X509CertOption {
	return func(o *tlsCertOption) error {
		o.extKeyUsage = append(o.extKeyUsage, usage...)
		return nil
	}
}

// WithX509CertSignatureAlgorithm set signature algorithm
func WithX509CertSignatureAlgorithm(sigAlg x509.SignatureAlgorithm) X509CertOption {
	return func(o *tlsCertOption) error {
		o.signatureAlgorithm = sigAlg
		return nil
	}
}

// WithX509CertOrganization set organization
func WithX509CertOrganization(organization ...string) X509CertOption {
	return func(o *tlsCertOption) error {
		o.organization = append(o.organization, organization...)
		return nil
	}
}

// WithX509CertOrganizationUnit set organization unit
func WithX509CertOrganizationUnit(ou ...string) X509CertOption {
	return func(o *tlsCertOption) error {
		o.organizationUnit = append(o.organizationUnit, ou...)
		return nil
	}
}

// WithX509CertLocality set organization unit
func WithX509CertLocality(l ...string) X509CertOption {
	return func(o *tlsCertOption) error {
		o.locality = append(o.locality, l...)
		return nil
	}
}

// WithX509CertSANS set certificate SANs
//
// refer to RFC-5280 4.2.1.6
//
// auto parse to ip/email/url/dns
func WithX509CertSANS(sans ...string) X509CertOption {
	return func(o *tlsCertOption) error {
		o.sans = append(o.sans, sans...)
		return nil
	}
}

// WithX509CertValidFrom set valid from
func WithX509CertValidFrom(validFrom time.Time) X509CertOption {
	return func(o *tlsCertOption) error {
		o.validFrom = validFrom
		return nil
	}
}

// WithX509CertValidFor set valid for duration
func WithX509CertValidFor(validFor time.Duration) X509CertOption {
	return func(o *tlsCertOption) error {
		o.validFor = validFor
		return nil
	}
}

// WithX509CertIsCA set is ca
func WithX509CertIsCA() X509CertOption {
	return func(o *tlsCertOption) error {
		o.isCA = true
		o.keyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
		return nil
	}
}

// WithX509CertIsCRLCA set is ca to sign CRL
func WithX509CertIsCRLCA() X509CertOption {
	return func(o *tlsCertOption) error {
		o.isCRLCA = true
		o.keyUsage |= x509.KeyUsageCRLSign
		return nil
	}
}

func (o *tlsCertOption) applyOpts(opts ...X509CertOption) (*tlsCertOption, error) {
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

	if opt, err := new(tlsCertOption).fillDefault().applyOpts(opts...); err != nil {
		return nil, err
	} else if !opt.customSerialNum {
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
