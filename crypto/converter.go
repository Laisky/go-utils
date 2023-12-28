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
	"fmt"
	"strings"

	"github.com/Laisky/errors/v2"

	gutils "github.com/Laisky/go-utils/v4"
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
	switch key.(type) {
	case *rsa.PublicKey,
		*ecdsa.PublicKey,
		ed25519.PublicKey:
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
	return x509.ParseCertificate(certInDer)
}

// Der2Cert parse multiple certificates in der
func Der2Certs(certInDer []byte) ([]*x509.Certificate, error) {
	return x509.ParseCertificates(certInDer)
}

// Der2CSR parse crl der
func Der2CSR(csrDer []byte) (*x509.CertificateRequest, error) {
	return x509.ParseCertificateRequest(csrDer)
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
func Der2Prikey(prikeyDer []byte) (crypto.PrivateKey, error) {
	prikey, err := x509.ParsePKCS8PrivateKey(prikeyDer)
	if err != nil {
		if strings.Contains(err.Error(), "ParsePKCS1PrivateKey") {
			if prikey, err = x509.ParsePKCS1PrivateKey(prikeyDer); err != nil {
				return nil, errors.Wrap(err, "cannot parse by pkcs1 nor pkcs8")
			}

			return prikey, nil
		}

		return nil, errors.Wrap(err, "parse by pkcs8")
	}

	return prikey, nil
}

// Der2Pubkey parse public key from der in x509 pkcs1/pkix
func Der2Pubkey(pubkeyDer []byte) (crypto.PublicKey, error) {
	rsapubkey, err := x509.ParsePKCS1PublicKey(pubkeyDer)
	if err != nil {
		if strings.Contains(err.Error(), "ParsePKIXPublicKey") {
			pubkey, err := x509.ParsePKIXPublicKey(pubkeyDer)
			if err != nil {
				return nil, errors.Wrap(err, "cannot parse by pkcs1 nor pkix")
			}

			return pubkey, nil
		}

		return nil, errors.Wrap(err, "parse by pkcs1")
	}

	return rsapubkey, nil
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

// VerifyCertByPrikey verify cert by prikey
func VerifyCertByPrikey(certPem []byte, prikeyPem []byte) error {
	_, err := tls.X509KeyPair(certPem, prikeyPem)
	return err
}

// X509Cert2OpensslConf marshal x509
func X509Cert2OpensslConf(cert *x509.Certificate) (opensslConf []byte) {
	// set req & req_distinguished_name
	cnt := fmt.Sprintf(gutils.Dedent(`
		[ req ]
		distinguished_name = req_distinguished_name
		prompt = no
		string_mask = utf8only
		x509_extensions = v3_ca
		req_extensions = req_ext

		[ req_distinguished_name ]
		commonName = %s`), cert.Subject.CommonName)
	cnt += "\n"

	subjectMaps := map[string][]string{
		"countryName":            cert.Subject.Country,
		"stateOrProvinceName":    cert.Subject.Province,
		"localityName":           cert.Subject.Locality,
		"organizationName":       cert.Subject.Organization,
		"organizationalUnitName": cert.Subject.OrganizationalUnit,
	}

	for _, name := range []string{ // keep order
		"countryName",
		"stateOrProvinceName",
		"localityName",
		"organizationName",
		"organizationalUnitName",
	} {
		if len(subjectMaps[name]) != 0 {
			cnt += fmt.Sprintf("%s = %s\n", name, strings.Join(subjectMaps[name], ","))
		}
	}
	cnt += "\n"

	// set v3_ca
	cnt += gutils.Dedent(`
		[ v3_ca ]
		basicConstraints = critical, CA:`)
	if cert.IsCA {
		cnt += "TRUE\nkeyUsage = cRLSign, keyCertSign\n"
	} else {
		cnt += "FALSE\nkeyUsage = digitalSignature, nonRepudiation, keyEncipherment, dataEncipherment, keyAgreement\n"
		cnt += "extendedKeyUsage = anyExtendedKeyUsage\n"
	}
	cnt += "subjectKeyIdentifier = hash\nauthorityKeyIdentifier = keyid:always, issuer\n"

	// set policies
	if len(cert.PolicyIdentifiers) > 0 {
		cnt += "certificatePolicies = "

		var policySecions string
		for i, policy := range cert.PolicyIdentifiers {
			cnt += fmt.Sprintf("@policy-%d, ", i)
			policySecions += fmt.Sprintf("[ policy-%d ]\npolicyIdentifier = %s\n", i, policy.String())
		}

		cnt = strings.TrimRight(cnt, ", ")
		cnt += "\n\n" + policySecions
	}

	// set req_ext
	cnt += "\n"
	cnt += gutils.Dedent(`
			[ req_ext ]
			subjectAltName = @alt_names

			[ alt_names ]`)
	cnt += "\n"
	for i, v := range cert.DNSNames {
		cnt += fmt.Sprintf("DNS.%d = %s\n", i+1, v)
	}
	for i, v := range cert.EmailAddresses {
		cnt += fmt.Sprintf("email.%d = %s\n", i+1, v)
	}
	for i, v := range cert.IPAddresses {
		cnt += fmt.Sprintf("IP.%d = %s\n", i+1, v.String())
	}
	for i, v := range cert.URIs {
		cnt += fmt.Sprintf("URI.%d = %s\n", i+1, v.String())
	}

	return []byte(cnt)
}

// X509Csr2OpensslConf marshal x509 csr to openssl conf
//
// # Returns
//
//	[ req ]
//	distinguished_name = req_distinguished_name
//	prompt = no
//	string_mask = utf8only
//	req_extensions = req_ext
//
//	[ req_ext ]
//	subjectAltName = @alt_names
//
//	[ req_distinguished_name ]
//	commonName = Intermedia CA
//	countryName = CN
//	stateOrProvinceName = Shanghai
//	localityName = Shanghai
//	organizationName = BBT
//	organizationalUnitName = XSS
//
//	[ alt_names ]
//	DNS.1 = localhost
//	DNS.2 = example.com
func X509Csr2OpensslConf(csr *x509.CertificateRequest) (opensslConf []byte) {
	// set req & req_distinguished_name
	cnt := fmt.Sprintf(gutils.Dedent(`
		[ req ]
		distinguished_name = req_distinguished_name
		prompt = no
		string_mask = utf8only
		req_extensions = req_ext

		[ req_distinguished_name ]
		commonName = %s`), csr.Subject.CommonName)
	cnt += "\n"

	subjectMaps := map[string][]string{
		"countryName":            csr.Subject.Country,
		"stateOrProvinceName":    csr.Subject.Province,
		"localityName":           csr.Subject.Locality,
		"organizationName":       csr.Subject.Organization,
		"organizationalUnitName": csr.Subject.OrganizationalUnit,
	}

	for _, name := range []string{ // keep order
		"countryName",
		"stateOrProvinceName",
		"localityName",
		"organizationName",
		"organizationalUnitName",
	} {
		if len(subjectMaps[name]) != 0 {
			cnt += fmt.Sprintf("%s = %s\n", name, strings.Join(subjectMaps[name], ","))
		}
	}

	// set req_ext
	cnt += "\n"
	cnt += gutils.Dedent(`
		[ req_ext ]
		subjectAltName = @alt_names

		[ alt_names ]`)
	cnt += "\n"

	for i, v := range csr.DNSNames {
		cnt += fmt.Sprintf("DNS.%d = %s\n", i+1, v)
	}
	for i, v := range csr.EmailAddresses {
		cnt += fmt.Sprintf("email.%d = %s\n", i+1, v)
	}
	for i, v := range csr.IPAddresses {
		cnt += fmt.Sprintf("IP.%d = %s\n", i+1, v.String())
	}
	for i, v := range csr.URIs {
		cnt += fmt.Sprintf("URI.%d = %s\n", i+1, v.String())
	}

	return []byte(cnt)
}

// x509SignCsrOptions2OpensslConf marshal x509 csr to openssl conf
func x509SignCsrOptions2OpensslConf(opts ...SignCSROption) (opt *signCSROption, opensslConf []byte, err error) {
	opt, err = new(signCSROption).fillDefault(nil).applyOpts(opts...)
	if err != nil {
		return nil, nil, errors.Wrap(err, "apply options")
	}

	cnt := gutils.Dedent(`
		[req]
		x509_extensions = v3_ca
		req_extensions = req_ext

		[ v3_ca ]
		subjectKeyIdentifier = hash
		authorityKeyIdentifier = keyid:always, issuer
		basicConstraints = critical, CA:`)

	if opt.isCA {
		cnt += "TRUE\nkeyUsage = cRLSign, keyCertSign\n"
	} else {
		cnt += "FALSE\nkeyUsage = digitalSignature, nonRepudiation, keyEncipherment, dataEncipherment, keyAgreement\n"
		cnt += "extendedKeyUsage = anyExtendedKeyUsage\n"
	}

	if len(opt.policies) > 0 {
		cnt += "certificatePolicies = "

		var policySecions string
		for i, policy := range opt.policies {
			cnt += fmt.Sprintf("@policy-%d, ", i)
			policySecions += fmt.Sprintf("[ policy-%d ]\npolicyIdentifier = %s\n", i, policy.String())
		}

		cnt = strings.TrimRight(cnt, ", ")
		cnt += "\n\n" + policySecions
	}

	return opt, []byte(cnt), nil
}
