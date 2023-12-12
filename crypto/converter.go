package crypto

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/gmsm/gmtls"
	"github.com/Laisky/gmsm/sm2"
	smx509 "github.com/Laisky/gmsm/x509"
)

// SecureCipherSuites get golang built-in cipher suites without known insecure suites
func SecureCipherSuites(filter func(*tls.CipherSuite) bool) []uint16 {
	var cs []uint16
	for _, s := range tls.CipherSuites() {
		if filter == nil || filter(s) {
			cs = append(cs, s.ID)
		}
	}

	return cs
}

// RSAPrikeyBits width of rsa private key
type RSAPrikeyBits int

const (
	// RSAPrikeyBits2048 rsa private key with 2048 bits
	RSAPrikeyBits2048 RSAPrikeyBits = 2048
	// RSAPrikeyBits3072 rsa private key with 3072 bits
	RSAPrikeyBits3072 RSAPrikeyBits = 3072
	// RSAPrikeyBits4096 rsa private key with 4096 bits
	RSAPrikeyBits4096 RSAPrikeyBits = 4096
)

// NewRSAPrikey new rsa privat ekey
func NewRSAPrikey(bits RSAPrikeyBits) (*rsa.PrivateKey, error) {
	switch bits {
	case RSAPrikeyBits2048, RSAPrikeyBits3072, RSAPrikeyBits4096:
	default:
		return nil, errors.Errorf("not support bits %d", bits)
	}

	return rsa.GenerateKey(rand.Reader, int(bits))
}

// ECDSACurve algorithms
type ECDSACurve string

const (
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
	// case ECDSACurveP224:
	// 	return ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	case ECDSACurveP256:
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case ECDSACurveP384:
		return ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	case ECDSACurveP521:
		return ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	default:
		return nil, errors.Errorf("unsupport curve %s", curve)
	}
}

// NewSM2Prikey new sm2 private key
func NewSM2Prikey() (*sm2.PrivateKey, error) {
	return sm2.GenerateKey(rand.Reader)
}

// NewEd25519Prikey new ed25519 private key
func NewEd25519Prikey() (ed25519.PrivateKey, error) {
	_, pri, err := ed25519.GenerateKey(rand.Reader)
	return pri, err
}

// Prikey2Der marshal private key by x509.8
func Prikey2Der(key crypto.PrivateKey) ([]byte, error) {
	switch key := key.(type) {
	case *rsa.PrivateKey,
		*ecdsa.PrivateKey,
		ed25519.PrivateKey:
	case *sm2.PrivateKey:
		return smx509.MarshalSm2UnecryptedPrivateKey(key)
	default:
		return nil, errors.Errorf("only support rsa/ecdsa/ed25519 private key")
	}

	return x509.MarshalPKCS8PrivateKey(key)
}

// Prikey2Pubkey get public key from private key
func Prikey2Pubkey(prikey crypto.PrivateKey) (pubkey crypto.PublicKey) {
	return prikey.(interface{ Public() crypto.PublicKey }).Public() // nolint:forcetypeassert // panic if not support
}

// Prikey2Pem marshal private key to pem
func Prikey2Pem(key crypto.PrivateKey) ([]byte, error) {
	der, err := Prikey2Der(key)
	if err != nil {
		return nil, err
	}

	return PrikeyDer2Pem(der), nil
}

// Pubkey2Der marshal public key by pkix
func Pubkey2Der(key crypto.PublicKey) ([]byte, error) {
	switch key := key.(type) {
	case *rsa.PublicKey,
		*ecdsa.PublicKey,
		ed25519.PublicKey:
	case *sm2.PublicKey:
		return smx509.MarshalSm2PublicKey(key)
	default:
		return nil, errors.Errorf("only support rsa/ecdsa/ed25519 public key")
	}

	return x509.MarshalPKIXPublicKey(key)
}

// Pubkey2Pem marshal public key to pem
func Pubkey2Pem(key crypto.PublicKey) ([]byte, error) {
	der, err := Pubkey2Der(key)
	if err != nil {
		return nil, err
	}

	return PubkeyDer2Pem(der), nil
}

// Cert2Pem marshal x509 certificate to pem
func Cert2Pem(certs ...*x509.Certificate) (ret []byte) {
	for i := range certs {
		ret = append(ret, CertDer2Pem(Cert2Der(certs[i]))...)
	}

	return
}

// Cert2Der marshal private key by x509.8
func Cert2Der(cert ...*x509.Certificate) (ret []byte) {
	for i := range cert {
		ret = append(ret, cert[i].Raw...)
	}

	return
}

// Der2Cert parse sigle certificate in der
func Der2Cert(certInDer []byte) (*x509.Certificate, error) {
	var joinedErr error
	for _, loader := range []func([]byte) (*x509.Certificate, error){
		func(b []byte) (*x509.Certificate, error) { return x509.ParseCertificate(b) },
		func(b []byte) (*x509.Certificate, error) {
			smcert, err := smx509.ParseCertificate(b)
			if err != nil {
				return nil, errors.Wrap(err, "parse sm2 cert")
			}

			return smcert.ToX509Certificate(), nil
		},
	} {
		cert, err := loader(certInDer)
		if err != nil {
			joinedErr = errors.Join(joinedErr, err)
			continue
		}

		return cert, nil
	}

	return nil, errors.Wrap(joinedErr, "cannot parse certificate")
}

// Der2Cert parse multiple certificates in der
func Der2Certs(certInDer []byte) ([]*x509.Certificate, error) {
	var joinedErr error
	for _, loader := range []func([]byte) ([]*x509.Certificate, error){
		func(b []byte) ([]*x509.Certificate, error) { return x509.ParseCertificates(b) },
		func(b []byte) ([]*x509.Certificate, error) {
			smcerts, err := smx509.ParseCertificates(b)
			if err != nil {
				return nil, errors.Wrap(err, "parse sm2 certs")
			}

			var certs []*x509.Certificate
			for _, c := range smcerts {
				certs = append(certs, c.ToX509Certificate())
			}

			return certs, nil
		},
	} {
		certs, err := loader(certInDer)
		if err != nil {
			joinedErr = errors.Join(joinedErr, err)
			continue
		}

		return certs, nil
	}

	return nil, errors.Wrap(joinedErr, "cannot parse certificates")
}

// Der2CSR parse crl der
func Der2CSR(csrDer []byte) (*x509.CertificateRequest, error) {
	var joinedErr error
	for _, loader := range []func([]byte) (*x509.CertificateRequest, error){
		func(b []byte) (*x509.CertificateRequest, error) {
			return x509.ParseCertificateRequest(b)
		},
		func(b []byte) (*x509.CertificateRequest, error) {
			smcsr, err := smx509.ParseCertificateRequest(b)
			if err != nil {
				return nil, errors.Wrap(err, "parse sm2 csr")
			}

			return Sm2CertificateRequest(smcsr), nil
		},
	} {
		csr, err := loader(csrDer)
		if err != nil {
			joinedErr = errors.Join(joinedErr, err)
			continue
		}

		return csr, nil
	}

	return nil, errors.Wrap(joinedErr, "cannot parse certificate request")
}

// CSR2Der marshal csr to der
func CSR2Der(csr *x509.CertificateRequest) []byte {
	return csr.Raw
}

// Der2CRL parse crl der
func Der2CRL(crlDer []byte) (*x509.RevocationList, error) {
	return x509.ParseRevocationList(crlDer)
}

// CRLDer2Pem marshal crl to pem
func CRLDer2Pem(crlDer []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: crlDer})
}

// CRLPem2Der parse crl pem
func CRLPem2Der(crlPem []byte) ([]byte, error) {
	return Pem2Der(crlPem)
}

// Pem2CRL parse crl pem
func Pem2CRL(crlPem []byte) (*x509.RevocationList, error) {
	der, err := Pem2Der(crlPem)
	if err != nil {
		return nil, err
	}

	return Der2CRL(der)
}

// CRL2Der marshal crl to der
func CRL2Der(crl *x509.RevocationList) []byte {
	return crl.Raw
}

// CRL2Pem marshal crl to pem
func CRL2Pem(crl *x509.RevocationList) []byte {
	return CRLDer2Pem(CRL2Der(crl))
}

// Pem2CSR parse csr from pem
func Pem2CSR(csrInPem []byte) (*x509.CertificateRequest, error) {
	csrDer, err := Pem2Der(csrInPem)
	if err != nil {
		return nil, errors.Wrap(err, "parse csr pem")
	}

	return Der2CSR(csrDer)
}

// Pem2Cert parse single certificate in pem
func Pem2Cert(certInPem []byte) (*x509.Certificate, error) {
	der, err := Pem2Der(certInPem)
	if err != nil {
		return nil, err
	}

	return Der2Cert(der)
}

// Pem2Certs parse multiple certificate in pem
func Pem2Certs(certInPem []byte) ([]*x509.Certificate, error) {
	der, err := Pem2Der(certInPem)
	if err != nil {
		return nil, err
	}

	return x509.ParseCertificates(der)
}

// RSAPem2Prikey parse private key from x509 v1(rsa) pem
func RSAPem2Prikey(x509v1Pem []byte) (*rsa.PrivateKey, error) {
	der, err := Pem2Der(x509v1Pem)
	if err != nil {
		return nil, err
	}

	return RSADer2Prikey(der)
}

// RSADer2Prikey parse private key from x509 v1(rsa) der
func RSADer2Prikey(x509v1Der []byte) (*rsa.PrivateKey, error) {
	return x509.ParsePKCS1PrivateKey(x509v1Der)
}

// Pem2Prikey parse private key from x509 v8(general) pem
func Pem2Prikey(x509v8Pem []byte) (crypto.PrivateKey, error) {
	der, err := Pem2Der(x509v8Pem)
	if err != nil {
		return nil, err
	}

	return Der2Prikey(der)
}

// Pem2Pubkey parse public key from pem
func Pem2Pubkey(pubkeyPem []byte) (crypto.PublicKey, error) {
	der, err := Pem2Der(pubkeyPem)
	if err != nil {
		return nil, err
	}

	return Der2Pubkey(der)
}

// Der2Prikey parse private key from der in x509 v8/v1
func Der2Prikey(prikeyDer []byte) (prikey crypto.PrivateKey, err error) {
	var joinedErr error
	for _, loader := range []func([]byte) (crypto.PrivateKey, error){
		func(b []byte) (crypto.PrivateKey, error) { return x509.ParsePKCS1PrivateKey(b) },
		func(b []byte) (crypto.PrivateKey, error) { return x509.ParsePKCS8PrivateKey(b) },
		func(b []byte) (crypto.PrivateKey, error) { return smx509.ParsePKCS8UnecryptedPrivateKey(b) },
	} {
		if prikey, err = loader(prikeyDer); err != nil {
			joinedErr = errors.Join(joinedErr, err)
			continue
		}

		return prikey, nil
	}

	return nil, errors.Wrap(joinedErr, "cannot parse private key")
}

// Der2Pubkey parse public key from der in x509 pkcs1/pkix
func Der2Pubkey(pubkeyDer []byte) (pubkey crypto.PublicKey, err error) {
	var joinedErr error
	for _, loader := range []func([]byte) (crypto.PublicKey, error){
		func(b []byte) (crypto.PublicKey, error) { return x509.ParsePKCS1PublicKey(b) },
		func(b []byte) (crypto.PublicKey, error) { return x509.ParsePKIXPublicKey(b) },
		func(b []byte) (crypto.PublicKey, error) { return smx509.ParseSm2PublicKey(b) },
	} {
		if pubkey, err = loader(pubkeyDer); err != nil {
			joinedErr = errors.Join(joinedErr, err)
			continue
		}

		return pubkey, nil
	}

	return nil, errors.Wrap(joinedErr, "cannot parse public key")
}

// PrikeyDer2Pem convert private key in der to pem
func PrikeyDer2Pem(prikeyInDer []byte) (prikeyInPem []byte) {
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: prikeyInDer})
}

// PubkeyDer2Pem convert public key in der to pem
func PubkeyDer2Pem(pubkeyInDer []byte) (prikeyInPem []byte) {
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubkeyInDer})
}

// CertDer2Pem convert certificate in der to pem
func CertDer2Pem(certInDer []byte) (certInPem []byte) {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certInDer})
}

// CSRDer2Pem convert CSR in der to pem
func CSRDer2Pem(CSRInDer []byte) (CSRInPem []byte) {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: CSRInDer})
}

// Pem2Der convert pem to der
//
// support one or more certs
func Pem2Der(pemBytes []byte) (derBytes []byte, err error) {
	pemBytes = bytes.Trim(pemBytes, " \n")
	var (
		data = pemBytes
		blk  *pem.Block
	)
	for {
		blk, data = pem.Decode(data)
		if blk == nil {
			return nil, errors.Errorf("pem format invalid")
		}

		derBytes = append(derBytes, blk.Bytes...)
		if len(data) == 0 {
			break
		}
	}

	return derBytes, err
}

// Pem2Ders convert pem to ders
//
// support one or more certs
func Pem2Ders(pemBytes []byte) (dersBytes [][]byte, err error) {
	pemBytes = bytes.Trim(pemBytes, " \n")
	var (
		data = pemBytes
		blk  *pem.Block
	)
	for {
		blk, data = pem.Decode(data)
		if blk == nil {
			return nil, errors.Errorf("pem format invalid")
		}

		d := []byte{}
		d = append(d, blk.Bytes...)

		dersBytes = append(dersBytes, d)
		if len(data) == 0 {
			break
		}
	}

	return dersBytes, err
}

// GetPubkeyFromPrikey get pubkey from private key
func GetPubkeyFromPrikey(priv crypto.PrivateKey) crypto.PublicKey {
	//nolint:forcetypeassert // panic if not support
	return priv.(interface{ Public() crypto.PublicKey }).Public()
}

// VerifyCertByPrikey verify cert by prikey
func VerifyCertByPrikey(certPem []byte, prikeyPem []byte) error {
	var joinedErr error
	for _, loader := range []func([]byte, []byte) error{
		func(b1, b2 []byte) error {
			_, err := tls.X509KeyPair(b1, b2)
			return errors.Wrap(err, "verify cert by prikey")
		},
		func(b1, b2 []byte) error {
			_, err := gmtls.X509KeyPair(b1, b2)
			return errors.Wrap(err, "verify cert by sm2 prikey")
		},
	} {
		if err := loader(certPem, prikeyPem); err != nil {
			joinedErr = errors.Join(joinedErr, err)
			continue
		}

		return nil
	}

	return errors.Wrap(joinedErr, "cannot verify cert by prikey")
}

// SmCertificateRequest convert x509.CertificateRequest to smx509.CertificateRequest
func SmCertificateRequest(csr *x509.CertificateRequest) *smx509.CertificateRequest {
	return &smx509.CertificateRequest{
		Raw:                      csr.Raw,
		RawTBSCertificateRequest: csr.RawTBSCertificateRequest,
		Version:                  csr.Version,
		Signature:                csr.Signature,
		SignatureAlgorithm:       smx509.SM2WithSM3,
		PublicKeyAlgorithm:       smx509.PublicKeyAlgorithm(csr.PublicKeyAlgorithm),
		PublicKey:                csr.PublicKey,
		Subject:                  csr.Subject,
		// Attributes:               csr.Attributes,  // attributes is deprecated
		Extensions:      csr.Extensions,
		ExtraExtensions: csr.ExtraExtensions,
		DNSNames:        csr.DNSNames,
		EmailAddresses:  csr.EmailAddresses,
		IPAddresses:     csr.IPAddresses,
	}
}

// Sm2CertificateRequest convert smx509.CertificateRequest to x509.CertificateRequest
func Sm2CertificateRequest(csr *smx509.CertificateRequest) *x509.CertificateRequest {
	return &x509.CertificateRequest{
		Raw:                      csr.Raw,
		RawTBSCertificateRequest: csr.RawTBSCertificateRequest,
		Version:                  csr.Version,
		Signature:                csr.Signature,
		SignatureAlgorithm:       x509.SignatureAlgorithm(smx509.SM2WithSM3),
		PublicKeyAlgorithm:       x509.PublicKeyAlgorithm(csr.PublicKeyAlgorithm),
		PublicKey:                csr.PublicKey,
		Subject:                  csr.Subject,
		// Attributes:               csr.Attributes,  // attributes is deprecated
		Extensions:      csr.Extensions,
		ExtraExtensions: csr.ExtraExtensions,
		DNSNames:        csr.DNSNames,
		EmailAddresses:  csr.EmailAddresses,
		IPAddresses:     csr.IPAddresses,
	}
}
