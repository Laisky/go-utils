package encrypt

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/mail"
	"net/url"
	"time"

	"github.com/Laisky/errors"
	"github.com/jinzhu/copier"

	gcounter "github.com/Laisky/go-utils/v3/counter"
)

var seriaCounter gcounter.Int64CounterItf

func init() {
	seriaCounter = gcounter.NewCounterFromN(time.Now().UnixNano())
}

// NewX509CSR new CSR
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

// NewX509CertTemplate new tls template
func NewX509CertTemplate(opts ...X509CertOption) (tpl *x509.Certificate, err error) {
	opt, err := new(tlsCertOption).fillDefault().applyOpts(opts...)
	if err != nil {
		return nil, err
	}

	notAfter := opt.validFrom.Add(opt.validFor)
	// serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	// serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, errors.Wrap(err, "generate serial number")
	}

	template := &x509.Certificate{
		SignatureAlgorithm: opt.signatureAlgorithm,
		SerialNumber:       big.NewInt(seriaCounter.Count()),
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
	commonName    string
	validFrom     time.Time
	validFor      time.Duration
	isCA, isCRLCA bool
	organization,
	organizationUnit,
	locality []string
	sans        []string
	keyUsage    x509.KeyUsage
	extKeyUsage []x509.ExtKeyUsage
	// signatureAlgorithm specific signature algorithm manually
	//
	// default to auto choose algorithm depends on certificate's algorithm
	signatureAlgorithm x509.SignatureAlgorithm
}

func (o *tlsCertOption) fillDefault() *tlsCertOption {
	o.validFrom = time.Now().UTC()
	o.validFor = 7 * 24 * time.Hour
	o.keyUsage |= x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature
	o.extKeyUsage = append(o.extKeyUsage, x509.ExtKeyUsageServerAuth)

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

// WithX509CertKeyUsage add key usage
func WithX509CertKeyUsage(usage ...x509.KeyUsage) X509CertOption {
	return func(o *tlsCertOption) error {
		for i := range usage {
			o.keyUsage |= usage[i]
		}

		return nil
	}
}

// WithX509ExtCertKeyUsage add key usage
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
//   - ca: CA to sign CRL
//   - prikey: prikey for CA
//   - revokeCerts: certifacates that will be revoked
func NewX509CRL(ca *x509.Certificate,
	prikey crypto.PrivateKey,
	revokeCerts []pkix.RevokedCertificate,
	opts ...X509CertOption) (crlDer []byte, err error) {
	if err = validPrikey(prikey); err != nil {
		return nil, err
	}

	ctpl, err := NewX509CertTemplate(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "parse options")
	}

	tpl := &x509.RevocationList{
		Number: big.NewInt(seriaCounter.Count()),
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
