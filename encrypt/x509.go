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

	"github.com/pkg/errors"
)

// NewX509CSR new CSR
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
		SignatureAlgorithm: x509.ECDSAWithSHA512,
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
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, errors.Wrap(err, "generate serial number")
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
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

	if opt.isCA {
		template.IsCA = true
		template.KeyUsage |= x509.KeyUsageCertSign
	}

	return template, nil
}

// SignX509CSR sign CSR to certificate
func SignX509CSR(
	ca *x509.Certificate,
	prikey crypto.PrivateKey,
	csr *x509.CertificateRequest,
	opts ...X509CertOption) (certDer []byte, err error) {
	if err = validPrikey(prikey); err != nil {
		return nil, err
	}

	opt, err := new(tlsCertOption).fillDefault().applyOpts(opts...)
	if err != nil {
		return nil, err
	}

	// create client certificate template
	tpl := &x509.Certificate{
		Signature:          csr.Signature,
		SignatureAlgorithm: csr.SignatureAlgorithm,

		PublicKeyAlgorithm: csr.PublicKeyAlgorithm,
		PublicKey:          csr.PublicKey,

		SerialNumber: big.NewInt(2),
		Issuer:       ca.Subject,
		Subject:      csr.Subject,
		NotBefore:    opt.validFrom,
		NotAfter:     opt.validFrom.Add(opt.validFor),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	if opt.isCA {
		tpl.IsCA = opt.isCA
		tpl.KeyUsage |= x509.KeyUsageCertSign
	}

	certDer, err = x509.CreateCertificate(rand.Reader, tpl, ca, GetPubkeyFromPrikey(prikey), prikey)
	if err != nil {
		return nil, errors.Wrap(err, "create certificate")
	}

	return certDer, nil
}

type tlsCertOption struct {
	commonName   string
	dns          []string
	ips          []net.IP
	validFrom    time.Time
	validFor     time.Duration
	isCA         bool
	organization []string
}

func (o *tlsCertOption) fillDefault() *tlsCertOption {
	o.validFrom = time.Now()
	o.validFor = 7 * 24 * time.Hour

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
	switch prikey.(type) {
	case *rsa.PrivateKey, *ecdsa.PrivateKey, ed25519.PrivateKey:
	default:
		return errors.Errorf("not support this type of private key")
	}

	return nil
}
