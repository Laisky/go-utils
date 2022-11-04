package encrypt

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"strings"

	"github.com/pkg/errors"
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

// Der2CSR parse crl der
func Der2CSR(csrDer []byte) (*x509.CertificateRequest, error) {
	return x509.ParseCertificateRequest(csrDer)
}

// Der2CRL parse crl der
func Der2CRL(crlDer []byte) (*x509.RevocationList, error) {
	return x509.ParseRevocationList(crlDer)
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
