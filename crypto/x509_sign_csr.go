package crypto

import (
	"crypto"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"time"

	"github.com/Laisky/errors/v2"
)

type signCSROption struct {
	notBefore    time.Time
	notAfter     time.Time
	isCA         bool
	keyUsage     x509.KeyUsage
	extKeyUsage  []x509.ExtKeyUsage
	serialNumber *big.Int
	// policies certificate policies
	//
	// refer to RFC-5280 4.2.1.4
	policies   []asn1.ObjectIdentifier
	policyOIDs []x509.OID
	// crls crl endpoints
	crls []string
	// ocsps ocsp servers
	ocsps []string
	// signatureAlgo the signature algorithm that parent certificate used to sign csr,
	// default to parent's signature algorithm
	signatureAlgo x509.SignatureAlgorithm
	// pubkeyAlgo    x509.PublicKeyAlgorithm

	// extensions,
	extraExtensions []pkix.Extension

	// pubkey csr will specific csr's pubkey, not use ca's pubkey
	pubkey             crypto.PublicKey
	serialNumGenerator X509CertSerialNumberGenerator
	// maxPathLen set CA path length constraint
	maxPathLen *int
}

// applyOpts fills the signing defaults into o and then applies opts in order. The defaults are a
// seven-day validity period starting now in UTC, digital-signature and key-encipherment key usage,
// and the package-level serial number generator. When csr is not nil, its ExtraExtensions are
// appended to o.extraExtensions before the options run. Afterwards a serial number is drawn from
// o.serialNumGenerator if none was set. It returns o on success, or the first option error.
func (o *signCSROption) applyOpts(
	// parent *x509.Certificate,
	csr *x509.CertificateRequest,
	opts ...SignCSROption,
) (*signCSROption, error) {
	// fill default
	o.notBefore = time.Now().UTC()
	o.notAfter = o.notBefore.Add(7 * 24 * time.Hour)
	o.keyUsage |= x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature
	o.serialNumGenerator = internalCertSerialNumGenerator

	// if parent != nil {
	// 	if o.signatureAlgo == x509.UnknownSignatureAlgorithm {
	// 		switch parent.PublicKeyAlgorithm {
	// 		case x509.RSA:
	// 			o.signatureAlgo = x509.SHA512WithRSA
	// 		case x509.DSA:
	// 			o.signatureAlgo = x509.DSAWithSHA256
	// 		case x509.ECDSA:
	// 			o.signatureAlgo = x509.ECDSAWithSHA512
	// 		case x509.Ed25519:
	// 			o.signatureAlgo = x509.PureEd25519
	// 		default:
	// 			return nil, errors.Errorf("unknown public key algorithm %q", parent.PublicKeyAlgorithm.String())
	// 		}
	// 	}
	// }

	if csr != nil {
		o.extraExtensions = append(o.extraExtensions, csr.ExtraExtensions...)
	}

	// apply options
	for _, f := range opts {
		if err := f(o); err != nil {
			return nil, errors.Wrap(err, "apply sign CSR option")
		}
	}

	if o.serialNumber == nil {
		// generate serial number by internal generator if not set
		o.serialNumber = big.NewInt(o.serialNumGenerator.SerialNum())
	}

	return o, nil
}

// SignCSROption options for create certificate from CRL
type SignCSROption func(*signCSROption) error

// WithX509CaMaxPathLen set max path length
//
// only CAs are allowed to specify MaxPathLen
func WithX509CaMaxPathLen(maxPathLen int) SignCSROption {
	return func(o *signCSROption) error {
		o.maxPathLen = &maxPathLen
		return nil
	}
}

// WithX509SerialNumGenerator set serial number generator
func WithX509SerialNumGenerator(gen X509CertSerialNumberGenerator) SignCSROption {
	return func(o *signCSROption) error {
		o.serialNumGenerator = gen
		return nil
	}
}

// WithX509SignSignatureAlgorithm set signature algorithm that parent certificate used to sign csr
func WithX509SignSignatureAlgorithm(algo x509.SignatureAlgorithm) SignCSROption {
	return func(o *signCSROption) error {
		o.signatureAlgo = algo
		return nil
	}
}

// WithX509SignPublicKeyAlgorithm set public key algorithm
//
// Deprecated: this field is ignored by golang built-in x509 library
func WithX509SignPublicKeyAlgorithm(_ x509.PublicKeyAlgorithm) SignCSROption {
	return func(_ *signCSROption) error {
		// o.pubkeyAlgo = algo
		return nil
	}
}

// WithX509SignCSRExtenstions set certificate extensions
//
// Extensions contains all requested extensions, in raw form. When parsing
// CSRs, this can be used to extract extensions that are not parsed by this
// package.
//
// Deprecated: this field is ignored by golang built-in x509 library,
// use WithX509SignCSRExtraExtenstions instead if you want to set extensions.
func WithX509SignCSRExtenstions(_ ...pkix.Extension) SignCSROption {
	return func(_ *signCSROption) error {
		// o.extensions = append(o.extensions, exts...)
		return nil
	}
}

// WithX509SignCSRExtraExtenstions set certificate extra extensions
//
// ExtraExtensions contains extensions to be copied, raw, into any CSR
// marshaled by CreateCertificateRequest. Values override any extensions
// that would otherwise be produced based on the other fields but are
// overridden by any extensions specified in Attributes.
//
// The ExtraExtensions field is not populated by ParseCertificateRequest,
// see Extensions instead.
func WithX509SignCSRExtraExtenstions(exts ...pkix.Extension) SignCSROption {
	return func(o *signCSROption) error {
		o.extraExtensions = append(o.extraExtensions, exts...)
		return nil
	}
}

// WithX509SignCSRPolicies set certificate policies
func WithX509SignCSRPolicies(policies ...asn1.ObjectIdentifier) SignCSROption {
	return func(o *signCSROption) error {
		o.policies = append(o.policies, policies...)
		for _, policy := range policies {
			ox509OID, err := OidAsn2X509(policy)
			if err != nil {
				return errors.Wrap(err, "convert policy oid to x509 oid")
			}
			o.policyOIDs = append(o.policyOIDs, ox509OID)
		}
		return nil
	}
}

// WithX509SignCSROCSPServers set ocsp servers
func WithX509SignCSROCSPServers(ocsp ...string) SignCSROption {
	return func(o *signCSROption) error {
		o.ocsps = append(o.ocsps, ocsp...)
		return nil
	}
}

// WithX509SignCSRSeriaNumber set certificate/CRL's serial number
//
// refer to RFC-5280 5.2.3 &
//
// # Args
//
// seriaNumber:
//   - (optional): generate certificate
//   - (required): generate CRL
func WithX509SignCSRSeriaNumber(serialNumber *big.Int) SignCSROption {
	return func(o *signCSROption) error {
		if serialNumber == nil {
			return errors.Errorf("serial number shoule not be empty")
		}

		o.serialNumber = serialNumber
		return nil
	}
}

// WithX509SignCSRKeyUsage add key usage
func WithX509SignCSRKeyUsage(usage ...x509.KeyUsage) SignCSROption {
	return func(o *signCSROption) error {
		for i := range usage {
			o.keyUsage |= usage[i]
		}

		return nil
	}
}

// WithX509SignCSRCRLs add crl endpoints
func WithX509SignCSRCRLs(crlEndpoint ...string) SignCSROption {
	return func(o *signCSROption) error {
		o.crls = append(o.crls, crlEndpoint...)
		return nil
	}
}

// WithX509SignCSRExtKeyUsage add ext key usage
func WithX509SignCSRExtKeyUsage(usage ...x509.ExtKeyUsage) SignCSROption {
	return func(o *signCSROption) error {
		o.extKeyUsage = append(o.extKeyUsage, usage...)
		return nil
	}
}

// WithX509SignCSRValidFrom set valid from
//
// deprecated: use WithX509SignCSRNotBefore instead
func WithX509SignCSRValidFrom(validFrom time.Time) SignCSROption {
	return func(o *signCSROption) error {
		o.notBefore = validFrom
		return nil
	}
}

// WithX509SignCSRNotBefore set valid from
func WithX509SignCSRNotBefore(notBefore time.Time) SignCSROption {
	return func(o *signCSROption) error {
		o.notBefore = notBefore
		return nil
	}
}

// WithX509SignCSRValidFor set valid for duration
//
// deprecated: use WithX509SignCSRNotAfter instead
func WithX509SignCSRValidFor(validFor time.Duration) SignCSROption {
	return func(o *signCSROption) error {
		o.notAfter = o.notBefore.Add(validFor)
		return nil
	}
}

// WithX509SignCSRNotAfter set valid for duration
func WithX509SignCSRNotAfter(notAfter time.Time) SignCSROption {
	return func(o *signCSROption) error {
		o.notAfter = notAfter
		return nil
	}
}

// WithX509SignCSRIsCA set is ca
func WithX509SignCSRIsCA() SignCSROption {
	return func(o *signCSROption) error {
		o.isCA = true
		o.keyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
		return nil
	}
}

// WithX509SignCSRIsCRLCA set is ca to sign CRL
func WithX509SignCSRIsCRLCA() SignCSROption {
	return func(o *signCSROption) error {
		o.isCA = true
		o.keyUsage |= x509.KeyUsageCRLSign
		return nil
	}
}

// NewX509CertByCSR verifies csrDer and signs it with the parent CA and prikey.
// It returns the certificate DER or an error. Signature verification establishes
// request integrity and proof of possession, not authorization for the requested
// subject or SANs; callers must enforce their enrollment policy.
//
// Depends on RFC-5280 4.2.1.12, empty ext key usage is as same as any key usage.
// so do not set any default ext key usages.
//
//   - https://github.com/golang/go/blob/1e9ff255a130200fcc4ec5e911d28181fce947d5/src/crypto/x509/verify.go#L1118
//
// but key usage is required in many cases:
//
//   - https://github.com/golang/go/blob/e04be8b24c20816f3429a8193c324ea67892e61f/src/crypto/x509/x509.go#L2165
func NewX509CertByCSR(
	parent *x509.Certificate,
	prikey crypto.PrivateKey,
	csrDer []byte,
	opts ...SignCSROption) (certDer []byte, err error) {
	if err = validPrikey(prikey); err != nil {
		return nil, errors.Wrap(err, "invalid CA private key")
	}

	csr, err := Der2CSR(csrDer)
	if err != nil {
		return nil, errors.Wrap(err, "parse csr")
	}

	// Parsing alone does not authenticate the request or prove key possession.
	if err := csr.CheckSignature(); err != nil {
		return nil, errors.Wrap(err, "verify csr signature")
	}

	opt, err := new(signCSROption).applyOpts(csr, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "apply options")
	}

	if !parent.IsCA || (parent.KeyUsage&x509.KeyUsageCertSign) == x509.KeyUsage(0) {
		return nil, errors.Errorf("parent certificate does not have CA flag or key usage")
	}

	certOpts := []X509CertOption{
		WithX509Subject(csr.Subject),
		WithX509CertParent(parent),
		WithX509CertNotBefore(opt.notBefore),
		WithX509CertNotAfter(opt.notAfter),
		WithX509CertPolicies(opt.policies...),
		WithX509CertCRLs(opt.crls...),
		WithX509CertOCSPServers(opt.ocsps...),
		WithX509CertKeyUsage(opt.keyUsage),
		WithX509CertExtKeyUsage(opt.extKeyUsage...),
		WithX509CertSeriaNumber(opt.serialNumber),
		WithX509CertDNSNames(csr.DNSNames...),
		WithX509CertEmailAddrs(csr.EmailAddresses...),
		WithX509CertIPAddrs(csr.IPAddresses...),
		WithX509CertURIs(csr.URIs...),
		WithX509CertPubkey(csr.PublicKey),
		WithX509CertSignatureAlgorithm(opt.signatureAlgo),
		// WithX509CertPublicKeyAlgorithm(opt.pubkeyAlgo),
		// WithX509CertExtentions(opt.extensions...),
		WithX509CertExtraExtensions(opt.extraExtensions...),
	}
	if opt.isCA {
		certOpts = append(certOpts, WithX509CertIsCA())
	}
	if opt.serialNumGenerator != nil {
		certOpts = append(certOpts, WithX509CertSerialNumGenerator(opt.serialNumGenerator))
	}
	if opt.maxPathLen != nil {
		certOpts = append(certOpts, WithX509CertCaMaxPathLen(*opt.maxPathLen))
	}

	return NewX509Cert(prikey, certOpts...)
}
