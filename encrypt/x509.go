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
	"time"

	"github.com/Laisky/errors"
	"github.com/jinzhu/copier"

	gcounter "github.com/Laisky/go-utils/v2/counter"
)

var seriaCounter gcounter.Counter

func init() {
	seriaCounter = *gcounter.NewCounterFromN(time.Now().UnixNano())
}

// NewX509CSR new CSR
//
// if prikey is not RSA private key, you must set SignatureAlgorithm by WithX509CertSignatureAlgorithm,
// default sig alg is x509.SHA256WithRSA.
func NewX509CSR(prikey crypto.PrivateKey, opts ...X509CertOption) (csrDer []byte, err error) {
	if err = validPrikey(prikey); err != nil {
		return nil, err
	}

	tpl, err := NewX509CertTemplate(opts...)
	if err != nil {
		return nil, err
	}

	csrTpl := &x509.CertificateRequest{
		Subject:            tpl.Subject,
		SignatureAlgorithm: x509.SHA256WithRSA,
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
		SerialNumber: big.NewInt(seriaCounter.Count()),
		Subject: pkix.Name{
			CommonName:   opt.commonName,
			Organization: opt.organization,
		},
		NotBefore: opt.validFrom,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              opt.dns,
		IPAddresses:           opt.ips,
	}

	switch {
	case opt.isCA:
		template.IsCA = true
		template.KeyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	case opt.isCRLCA:
		template.KeyUsage |= x509.KeyUsageCRLSign
	}

	return template, nil
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

	certDer, err = x509.CreateCertificate(rand.Reader, tpl, ca, GetPubkeyFromPrikey(prikey), prikey)
	if err != nil {
		return nil, errors.Wrap(err, "create certificate")
	}

	return certDer, nil
}

type tlsCertOption struct {
	commonName    string
	dns           []string
	ips           []net.IP
	validFrom     time.Time
	validFor      time.Duration
	isCA, isCRLCA bool
	organization  []string
	sigAlg        x509.SignatureAlgorithm
}

func (o *tlsCertOption) fillDefault() *tlsCertOption {
	o.validFrom = time.Now()
	o.validFor = 7 * 24 * time.Hour
	o.sigAlg = x509.ECDSAWithSHA512

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

// WithX509CertSignatureAlgorithm set signature algorithm
func WithX509CertSignatureAlgorithm(sigAlg x509.SignatureAlgorithm) X509CertOption {
	return func(o *tlsCertOption) error {
		o.sigAlg = sigAlg
		return nil
	}
}

// WithX509CertOrganization set organization
func WithX509CertOrganization(organization []string) X509CertOption {
	return func(o *tlsCertOption) error {
		o.organization = organization
		return nil
	}
}

// WithX509CertDNS set DNS SANs
func WithX509CertDNS(dns []string) X509CertOption {
	return func(o *tlsCertOption) error {
		o.dns = dns
		return nil
	}
}

// WithX509CertIPs set IP SANs
func WithX509CertIPs(ips []net.IP) X509CertOption {
	return func(o *tlsCertOption) error {
		o.ips = ips
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
		return nil
	}
}

// WithX509CertIsCRACA set is ca to sign CRL
func WithX509CertIsCRACA() X509CertOption {
	return func(o *tlsCertOption) error {
		o.isCRLCA = true
		return nil
	}
}

func (o *tlsCertOption) applyOpts(opts ...X509CertOption) (*tlsCertOption, error) {
	for _, f := range opts {
		if err := f(o); err != nil {
			return nil, err
		}
	}

	if len(o.dns) == 0 {
		o.dns = append(o.dns, o.commonName)
	}

	if len(o.ips) == 0 {
		for _, addr := range o.dns {
			if ip := net.ParseIP(addr); ip != nil {
				o.ips = append(o.ips, ip)
			}
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
	case *ed25519.PrivateKey:
		return privkey
	default:
		return nil
	}
}

// NewX509CRL create and sign CRL
//
// # Args
//
// ca: CA to sign CRL
//
// prikey: prikey for CA
//
// revokeCerts: certifacates that will be revoked
//
// opts: some CRL's attributes.
//
//   - WithX509CertValidFrom set CRL's `ThisUpdate`,
//   - WithX509CertValidFor set CRL's `NextUpdate`.
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
		Number:              big.NewInt(seriaCounter.Count()),
		Issuer:              ca.Subject,
		SignatureAlgorithm:  ctpl.SignatureAlgorithm,
		ThisUpdate:          ctpl.NotBefore,
		NextUpdate:          ca.NotAfter,
		Extensions:          ctpl.Extensions,
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
