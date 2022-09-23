package utils

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"

	"github.com/pkg/errors"
)

type RSAPrikeyBits int

const (
	RSAPrikeyBits2048 RSAPrikeyBits = 2048
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
	ECDSACurveP224 ECDSACurve = "P224"
	ECDSACurveP256 ECDSACurve = "P256"
	ECDSACurveP384 ECDSACurve = "P384"
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
func NewEd25519Prikey() (*ed25519.PrivateKey, error) {
	_, pri, err := ed25519.GenerateKey(rand.Reader)
	return &pri, err
}

// Prikey2Der marshal private key by x509.8
func Prikey2Der(key any) ([]byte, error) {
	switch key.(type) {
	case *rsa.PrivateKey, *ecdsa.PrivateKey, *ed25519.PrivateKey:
	default:
		return nil, errors.Errorf("only support rsa/ecdsa/ed25519 private key")
	}

	return x509.MarshalPKCS8PrivateKey(key)
}

// Cert2Der marshal private key by x509.8
func Cert2Der(cert *x509.Certificate) []byte {
	return cert.Raw
}

func Der2Cert(certInDer []byte) (*x509.Certificate, error) {
	return x509.ParseCertificate(certInDer)
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
	commonName string
	dns        []string
	validFrom  time.Time
	validFor   time.Duration
	isCA       bool
}

func (o *tlsCertOption) fillDefault() *tlsCertOption {
	o.validFrom = time.Now()
	o.validFor = 7 * 24 * time.Hour

	return o
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
func GetPubkeyFromPrikey(priv any) any {
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

// TLSCertOption option to generate tls certificate
type TLSCertOption func(*tlsCertOption) error

// NewTLSCert new self sign tls cert
func NewTLSCert(prikey any, opts ...TLSCertOption) (certDer []byte, err error) {
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

func validPrikey(prikey any) error {
	switch prikey.(type) {
	case *rsa.PrivateKey, *ecdsa.PrivateKey, *ed25519.PrivateKey:
	default:
		return errors.Errorf("not support this type of private key")
	}

	return nil
}

// NewTLSCSR new CSR
func NewTLSCSR(prikey any, opts ...TLSCertOption) (csrDer []byte, err error) {
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
			Organization: []string{"Acme Co"},
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
func SignTLSCSR(ca *x509.Certificate, prikey any, csr *x509.CertificateRequest, opts ...TLSCertOption) (certDer []byte, err error) {
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
