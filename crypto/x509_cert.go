package crypto

import (
	"crypto"
	"crypto/rand"
	"crypto/x509"

	"github.com/Laisky/errors/v2"
)

// NewRSAPrikeyAndCert convient function to new rsa private key and cert
func NewRSAPrikeyAndCert(rsaBits RSAPrikeyBits, opts ...X509CertOption) (
	prikeyPem, certDer []byte, err error) {
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

// NewECDSAPrikeyAndCert convient function to new ecdsa private key and cert
func NewECDSAPrikeyAndCert(curve ECDSACurve, opts ...X509CertOption) (
	prikeyPem, certDer []byte, err error) {
	prikey, err := NewECDSAPrikey(curve)
	if err != nil {
		return nil, nil, errors.Wrap(err, "new ecdsa prikey")
	}

	prikeyPem, err = Prikey2Pem(prikey)
	if err != nil {
		return nil, nil, errors.Wrap(err, "convert prikey to pem")
	}

	certDer, err = NewX509Cert(prikey, opts...)
	return prikeyPem, certDer, errors.Wrap(err, "generate cert")
}

// NewEd25519PrikeyAndCert convient function to new ed25519 private key and cert
func NewEd25519PrikeyAndCert(opts ...X509CertOption) (
	prikeyPem, certDer []byte, err error) {
	prikey, err := NewEd25519Prikey()
	if err != nil {
		return nil, nil, errors.Wrap(err, "new ed25519 prikey")
	}

	prikeyPem, err = Prikey2Pem(prikey)
	if err != nil {
		return nil, nil, errors.Wrap(err, "convert prikey to pem")
	}

	certDer, err = NewX509Cert(prikey, opts...)
	return prikeyPem, certDer, errors.Wrap(err, "generate cert")
}

// x509CertOption2Template convert X509CertOption to x509.Certificate template
func x509CertOption2Template(opts ...X509CertOption) (
	opt *x509V3CertOption, certTemplate *x509.Certificate, err error) {
	if opt, err = new(x509V3CertOption).applyOpts(opts...); err != nil {
		return nil, nil, errors.Wrap(err, "apply options")
	}

	tpl := &x509.Certificate{
		SignatureAlgorithm:    opt.signatureAlgorithm,
		SerialNumber:          opt.serialNumber,
		Subject:               opt.subject,
		NotBefore:             opt.notBefore,
		NotAfter:              opt.notAfter,
		KeyUsage:              opt.keyUsage,
		ExtKeyUsage:           opt.extKeyUsage,
		BasicConstraintsValid: true,
		IsCA:                  opt.isCA,
		PolicyIdentifiers:     opt.policies,
		Policies:              append([]x509.OID(nil), opt.policyOIDs...),
		CRLDistributionPoints: opt.crls,
		OCSPServer:            opt.ocsps,
		EmailAddresses:        opt.emailAddresses,
		DNSNames:              opt.dnsNames,
		IPAddresses:           opt.ipAddresses,
		URIs:                  opt.uris,
		// Extensions:            opt.signCSROption.extensions,
		ExtraExtensions: opt.signCSROption.extraExtensions,
	}

	if opt.maxPathLen != nil {
		tpl.MaxPathLen = *opt.maxPathLen
		if tpl.MaxPathLen == 0 {
			tpl.MaxPathLenZero = true
		}
	}

	return opt, tpl, nil
}

// NewX509Cert new cert
func NewX509Cert(prikey crypto.PrivateKey, opts ...X509CertOption) (certDer []byte, err error) {
	if err = validPrikey(prikey); err != nil {
		return nil, errors.Wrap(err, "valid prikey")
	}

	opt, tpl, err := x509CertOption2Template(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "convert options to template")
	}

	if opt.pubkey == nil {
		opt.pubkey = Prikey2Pubkey(prikey)
	}

	// CreateCertificate x509.CreateCertificate will auto generate subject key id for ca template
	if !opt.isCA {
		if tpl.SubjectKeyId, err = X509CertSubjectKeyID(opt.pubkey); err != nil {
			return nil, errors.Wrap(err, "generate cert subject key id")
		}
	}

	parent := tpl
	if opt.parent != nil {
		parent = opt.parent
	}

	certDer, err = x509.CreateCertificate(rand.Reader, tpl, parent, opt.pubkey, prikey)
	if err != nil {
		return nil, errors.Wrap(err, "create certificate")
	}

	return certDer, nil
}
