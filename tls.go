package utils

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// RSAPrikeyBits width of rsa private key
type RSAPrikeyBits int

const (
	// RSAPrikeyBits2048 rsa private key with 2048 bits
	RSAPrikeyBits2048 RSAPrikeyBits = 2048
	// RSAPrikeyBits3072 rsa private key with 3072 bits
	RSAPrikeyBits3072 RSAPrikeyBits = 3072
)

// NewRSAPrikey new rsa privat ekey
func NewRSAPrikey(bits RSAPrikeyBits) (*rsa.PrivateKey, error) {
	switch bits {
	case RSAPrikeyBits2048, RSAPrikeyBits3072:
	default:
		return nil, errors.Errorf("not support bits %d", bits)
	}

	return rsa.GenerateKey(rand.Reader, int(bits))
}

// ECDSACurve algorithms
type ECDSACurve string

const (
	// ECDSACurveP224 ecdsa with P224
	ECDSACurveP224 ECDSACurve = "P224"
	// ECDSACurveP256 ecdsa with P256
	ECDSACurveP256 ECDSACurve = "P256"
	// ECDSACurveP384 ecdsa with P384
	ECDSACurveP384 ECDSACurve = "P384"
	// ECDSACurveP521 ecdsa with P521
	ECDSACurveP521 ECDSACurve = "P521"
)

// NewECDSAPrikey new ecdsa private key
func NewECDSAPrikey(curve ECDSACurve) (*ecdsa.PrivateKey, error) {
	switch curve {
	case ECDSACurveP224:
		return ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	case ECDSACurveP256:
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case ECDSACurveP384:
		return ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	case ECDSACurveP521:
		return ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	default:
		return nil, errors.Errorf("unknown curve %s", curve)
	}
}

// NewEd25519Prikey new ed25519 private key
func NewEd25519Prikey() (ed25519.PrivateKey, error) {
	_, pri, err := ed25519.GenerateKey(rand.Reader)
	return pri, err
}

// Prikey2Der marshal private key by x509.8
func Prikey2Der(key crypto.PrivateKey) ([]byte, error) {
	switch key.(type) {
	case *rsa.PrivateKey,
		*ecdsa.PrivateKey,
		ed25519.PrivateKey:
	default:
		return nil, errors.Errorf("only support rsa/ecdsa/ed25519 private key")
	}

	return x509.MarshalPKCS8PrivateKey(key)
}

// Prikey2Pem marshal private key to pem
func Prikey2Pem(key crypto.PrivateKey) ([]byte, error) {
	der, err := Prikey2Der(key)
	if err != nil {
		return nil, err
	}

	return PrikeyDer2Pem(der), nil
}

// Cert2Pem marshal x509 certificate to pem
func Cert2Pem(cert *x509.Certificate) []byte {
	return CertDer2Pem(Cert2Der(cert))
}

// Cert2Der marshal private key by x509.8
func Cert2Der(cert *x509.Certificate) []byte {
	return cert.Raw
}

// Der2Cert parse certificate in der
func Der2Cert(certInDer []byte) (*x509.Certificate, error) {
	return x509.ParseCertificate(certInDer)
}

// Pem2Cert parse certificate in pem
func Pem2Cert(certInPem []byte) (*x509.Certificate, error) {
	return Der2Cert(Pem2Der(certInPem))
}

// RSAPem2Prikey parse private key from x509 v1(rsa) pem
func RSAPem2Prikey(x509v1Pem []byte) (*rsa.PrivateKey, error) {
	return RSADer2Prikey(Pem2Der(x509v1Pem))
}

// RSADer2Prikey parse private key from x509 v1(rsa) der
func RSADer2Prikey(x509v1Der []byte) (*rsa.PrivateKey, error) {
	return x509.ParsePKCS1PrivateKey(x509v1Der)
}

// Pem2Prikey parse private key from x509 v8(general) pem
func Pem2Prikey(x509v8Pem []byte) (crypto.PrivateKey, error) {
	return Der2Prikey(Pem2Der(x509v8Pem))
}

// Der2Prikey parse private key from der in x509 v8/v1
func Der2Prikey(prikeyDer []byte) (crypto.PrivateKey, error) {
	prikey, err := x509.ParsePKCS8PrivateKey(prikeyDer)
	if err != nil && strings.Contains(err.Error(), "ParsePKCS1PrivateKey") {
		if prikey, err = x509.ParsePKCS1PrivateKey(prikeyDer); err != nil {
			return nil, errors.Wrap(err, "cannot parse by pkcs1 nor pkcs8")
		}

		return prikey, nil
	}

	return prikey, nil
}

// PrikeyDer2Pem convert private key in der to pem
func PrikeyDer2Pem(prikeyInDer []byte) (prikeyInDem []byte) {
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: prikeyInDer})
}

// CertDer2Pem convert certificate in der to pem
func CertDer2Pem(certInDer []byte) (certInDem []byte) {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certInDer})
}

// Pem2Der convert pem to der
func Pem2Der(pemBytes []byte) (derBytes []byte) {
	blk, _ := pem.Decode(pemBytes)
	return blk.Bytes
}

type tlsCertOption struct {
	commonName   string
	dns          []string
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

// TLSCertOption option to generate tls certificate
type TLSCertOption func(*tlsCertOption) error

// WithTLSCommonName set common name
func WithTLSCommonName(commonName string) TLSCertOption {
	return func(o *tlsCertOption) error {
		o.commonName = commonName
		return nil
	}
}

// WithTLSOrganization set organization
func WithTLSOrganization(organization []string) TLSCertOption {
	return func(o *tlsCertOption) error {
		o.organization = organization
		return nil
	}
}

// WithTLSDNS set dnses
func WithTLSDNS(dns []string) TLSCertOption {
	return func(o *tlsCertOption) error {
		o.dns = dns
		return nil
	}
}

// WithTLSValidFrom set valid from
func WithTLSValidFrom(validFrom time.Time) TLSCertOption {
	return func(o *tlsCertOption) error {
		o.validFrom = validFrom
		return nil
	}
}

// WithTLSValidFor set valid for duration
func WithTLSValidFor(validFor time.Duration) TLSCertOption {
	return func(o *tlsCertOption) error {
		o.validFor = validFor
		return nil
	}
}

// WithTLSIsCA set is ca
func WithTLSIsCA() TLSCertOption {
	return func(o *tlsCertOption) error {
		o.isCA = true
		return nil
	}
}

func (o *tlsCertOption) applyOpts(opts ...TLSCertOption) (*tlsCertOption, error) {
	for _, f := range opts {
		if err := f(o); err != nil {
			return nil, err
		}
	}

	if len(o.dns) == 0 {
		o.dns = append(o.dns, o.commonName)
	}

	return o, nil
}

// GetPubkeyFromPrikey get pubkey from private key
func GetPubkeyFromPrikey(priv crypto.PrivateKey) crypto.PublicKey {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &k.PublicKey
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	case ed25519.PrivateKey:
		return k.Public().(ed25519.PublicKey)
	default:
		return nil
	}
}

// NewTLSCert new self sign tls cert
func NewTLSCert(prikey crypto.PrivateKey, opts ...TLSCertOption) (certDer []byte, err error) {
	if err = validPrikey(prikey); err != nil {
		return nil, err
	}

	tpl, err := NewTLSTemplate(opts...)
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

// NewTLSCSR new CSR
func NewTLSCSR(prikey crypto.PrivateKey, opts ...TLSCertOption) (csrDer []byte, err error) {
	if err = validPrikey(prikey); err != nil {
		return nil, err
	}

	tpl, err := NewTLSTemplate(opts...)
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

// NewTLSTemplate new tls template
func NewTLSTemplate(opts ...TLSCertOption) (tpl *x509.Certificate, err error) {
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
	}

	if opt.isCA {
		template.IsCA = true
		template.KeyUsage |= x509.KeyUsageCertSign
	}

	return template, nil
}

// SignTLSCSR sign CSR to certificate
func SignTLSCSR(ca *x509.Certificate, prikey crypto.PrivateKey, csr *x509.CertificateRequest, opts ...TLSCertOption) (certDer []byte, err error) {
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
