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
	return pri, errors.WithStack(err)
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

// Prikey2Pem marshal private key to pem, tailing with line break
func Prikey2Pem(key crypto.PrivateKey) ([]byte, error) {
	der, err := Prikey2Der(key)
	if err != nil {
		return nil, errors.WithStack(err)
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

// Pubkey2Pem marshal public key to pem, tailing with line break
func Pubkey2Pem(key crypto.PublicKey) ([]byte, error) {
	der, err := Pubkey2Der(key)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return PubkeyDer2Pem(der), nil
}

// Cert2Pem marshal x509 certificate to pem, tailing with line break
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
		return nil, errors.WithStack(err)
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
		return nil, errors.WithStack(err)
	}

	return Der2Cert(der)
}

// Pem2Certs parse multiple certificate in pem
func Pem2Certs(certInPem []byte) ([]*x509.Certificate, error) {
	certInPem = bytes.ReplaceAll(certInPem, []byte("----------BEGIN"), []byte("-----\n-----BEGIN"))
	der, err := Pem2Der(certInPem)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return x509.ParseCertificates(der)
}

// RSAPem2Prikey parse private key from x509 v1(rsa) pem
func RSAPem2Prikey(x509v1Pem []byte) (*rsa.PrivateKey, error) {
	der, err := Pem2Der(x509v1Pem)
	if err != nil {
		return nil, errors.WithStack(err)
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
		return nil, errors.WithStack(err)
	}

	return Der2Prikey(der)
}

// Pem2Pubkey parse public key from pem
func Pem2Pubkey(pubkeyPem []byte) (crypto.PublicKey, error) {
	der, err := Pem2Der(pubkeyPem)
	if err != nil {
		return nil, errors.WithStack(err)
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

// CertDer2Pem convert certificate in der to pem, tailing with line break
func CertDer2Pem(certInDer []byte) (certInPem []byte) {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certInDer})
}

// CSRDer2Pem convert CSR in der to pem, tailing with line break
func CSRDer2Pem(CSRInDer []byte) (CSRInPem []byte) {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: CSRInDer})
}

// Pem2Der convert pem to der
//
// support one or more certs
func Pem2Der(pemBytes []byte) (derBytes []byte, err error) {
	pemBytes = bytes.ReplaceAll(pemBytes, []byte("----------BEGIN"), []byte("-----\n-----BEGIN"))
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

	return derBytes, errors.WithStack(err)
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

	return dersBytes, errors.WithStack(err)
}

// VerifyCertByPrikey verify cert by prikey
func VerifyCertByPrikey(certPem []byte, prikeyPem []byte) error {
	_, err := tls.X509KeyPair(certPem, prikeyPem)
	return errors.WithStack(err)
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
	var altCnt string
	for i, v := range cert.DNSNames {
		altCnt += fmt.Sprintf("DNS.%d = %s\n", i+1, v)
	}
	for i, v := range cert.EmailAddresses {
		altCnt += fmt.Sprintf("email.%d = %s\n", i+1, v)
	}
	for i, v := range cert.IPAddresses {
		altCnt += fmt.Sprintf("IP.%d = %s\n", i+1, v.String())
	}
	for i, v := range cert.URIs {
		altCnt += fmt.Sprintf("URI.%d = %s\n", i+1, v.String())
	}
	if altCnt != "" {
		cnt += "\n"
		cnt += gutils.Dedent(`
			[ req_ext ]
			subjectAltName = @alt_names

			[ alt_names ]`)
		cnt += "\n"
		cnt += altCnt
		cnt = strings.ReplaceAll(cnt, "x509_extensions = v3_ca", "x509_extensions = v3_ca\nreq_extensions = req_ext")
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
	var sansCnt string
	for i, v := range csr.DNSNames {
		sansCnt += fmt.Sprintf("DNS.%d = %s\n", i+1, v)
	}
	for i, v := range csr.EmailAddresses {
		sansCnt += fmt.Sprintf("email.%d = %s\n", i+1, v)
	}
	for i, v := range csr.IPAddresses {
		sansCnt += fmt.Sprintf("IP.%d = %s\n", i+1, v.String())
	}
	for i, v := range csr.URIs {
		sansCnt += fmt.Sprintf("URI.%d = %s\n", i+1, v.String())
	}
	if sansCnt != "" {
		cnt += "\n"
		cnt += gutils.Dedent(`
			[ req_ext ]
			subjectAltName = @alt_names

			[ alt_names ]`)
		cnt += "\n"
		cnt += sansCnt

		cnt = strings.ReplaceAll(cnt, "string_mask = utf8only", "string_mask = utf8only\nreq_extensions = req_ext")
	}

	return []byte(cnt)
}

var (
	sortedKeyUsages = []string{
		"digitalSignature",
		"nonRepudiation",
		"keyEncipherment",
		"dataEncipherment",
		"keyAgreement",
		"keyCertSign",
		"cRLSign",
		"encipherOnly",
		"decipherOnly",
	}
	keyUsagesMap = map[string]x509.KeyUsage{
		"digitalSignature": x509.KeyUsageDigitalSignature,
		"nonRepudiation":   x509.KeyUsageContentCommitment, // nonRepudiation is also known as contentCommitment
		"keyEncipherment":  x509.KeyUsageKeyEncipherment,
		"dataEncipherment": x509.KeyUsageDataEncipherment,
		"keyAgreement":     x509.KeyUsageKeyAgreement,
		"keyCertSign":      x509.KeyUsageCertSign, // Corrected from "CertSign" to "keyCertSign"
		"cRLSign":          x509.KeyUsageCRLSign,
		"encipherOnly":     x509.KeyUsageEncipherOnly,
		"decipherOnly":     x509.KeyUsageDecipherOnly,
	}
	sortedExtKeyUsages = []string{
		"serverAuth",
		"clientAuth",
		"codeSigning",
		"emailProtection",
		"ipsecEndSystem",
		"ipsecTunnel",
		"ipsecUser",
		"timestamping",
		"ocspSigning",
		"microsoftServerGatedCrypto",
		"netscapeServerGatedCrypto",
		"microsoftCommercialCodeSigning",
		"microsoftKernelCodeSigning",
	}
	extKeyUsagesMap = map[string]x509.ExtKeyUsage{
		"serverAuth":                     x509.ExtKeyUsageServerAuth,
		"clientAuth":                     x509.ExtKeyUsageClientAuth,
		"codeSigning":                    x509.ExtKeyUsageCodeSigning,
		"emailProtection":                x509.ExtKeyUsageEmailProtection,
		"ipsecEndSystem":                 x509.ExtKeyUsageIPSECEndSystem,
		"ipsecTunnel":                    x509.ExtKeyUsageIPSECTunnel,
		"ipsecUser":                      x509.ExtKeyUsageIPSECUser,
		"timestamping":                   x509.ExtKeyUsageTimeStamping,
		"ocspSigning":                    x509.ExtKeyUsageOCSPSigning,
		"microsoftServerGatedCrypto":     x509.ExtKeyUsageMicrosoftServerGatedCrypto,
		"netscapeServerGatedCrypto":      x509.ExtKeyUsageNetscapeServerGatedCrypto,
		"microsoftCommercialCodeSigning": x509.ExtKeyUsageMicrosoftCommercialCodeSigning,
		"microsoftKernelCodeSigning":     x509.ExtKeyUsageMicrosoftKernelCodeSigning,
	}
)

// x509SignCsrOptions2OpensslConf marshal x509 csr to openssl conf
func x509SignCsrOptions2OpensslConf(opts ...SignCSROption) (opt *signCSROption, opensslConf []byte, err error) {
	opt, err = new(signCSROption).applyOpts(nil, opts...)
	if err != nil {
		return nil, nil, errors.Wrap(err, "apply options")
	}

	cnt := gutils.Dedent(`
		[req]
		x509_extensions = v3_ca

		[ v3_ca ]
		subjectKeyIdentifier = hash
		authorityKeyIdentifier = keyid:always, issuer
		basicConstraints = critical, CA:`)

	if opt.isCA {
		cnt += "TRUE\n"
	} else {
		cnt += "FALSE\n"
	}

	var extKeyUsages, keyUsages []string
	for _, name := range sortedKeyUsages {
		usage := keyUsagesMap[name]
		if opt.keyUsage&usage != 0 {
			keyUsages = append(keyUsages, name)
		}
	}
	if len(keyUsages) != 0 {
		cnt += fmt.Sprintf("keyUsage = %s\n", strings.Join(keyUsages, ", "))
	}

	if gutils.Contains(opt.extKeyUsage, x509.ExtKeyUsageAny) {
		// The main purpose of this function is to cater to the needs of non-compatible national SM2 standards.
		// Since Tongsuo does not support anyExtendedKeyUsage, so it is better to use enumeration instead.
		cnt += "extendedKeyUsage = serverAuth, clientAuth, codeSigning, emailProtection, ipsecEndSystem, " +
			"ipsecTunnel, ipsecUser, timestamping, ocspSigning, microsoftServerGatedCrypto, " +
			"netscapeServerGatedCrypto, microsoftCommercialCodeSigning, microsoftKernelCodeSigning\n"
	} else {
		for _, name := range sortedExtKeyUsages {
			usage := extKeyUsagesMap[name]
			if gutils.Contains(opt.extKeyUsage, usage) {
				extKeyUsages = append(extKeyUsages, name)
			}
		}
		if len(extKeyUsages) != 0 {
			cnt += fmt.Sprintf("extendedKeyUsage = %s\n", strings.Join(extKeyUsages, ", "))
		}
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

// SplitCertsPemChain split pem chain to multiple pem
func SplitCertsPemChain(pemChain string) (pems []string) {
	vs := strings.Split(pemChain, "-----END CERTIFICATE-----")
	for _, v := range vs {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}

		pems = append(pems, v+"\n-----END CERTIFICATE-----")
	}

	return
}

// X509CrlOptions2Tpl marshal x509 crl options to x509.RevocationList
func X509CrlOptions2Tpl(opts ...X509CRLOption) (*x509.RevocationList, error) {
	opt, err := new(x509CRLOption).applyOpts(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "apply options")
	}

	tpl := &x509.RevocationList{
		SignatureAlgorithm: opt.signatureAlgorithm,
		ThisUpdate:         opt.thisUpdate,
		NextUpdate:         opt.nextUpdate,
	}

	return tpl, nil
}
