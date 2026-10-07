package crypto

import (
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"time"

	"github.com/Laisky/errors/v2"
)

type x509CRLOption struct {
	// signatureAlgorithm specific signature algorithm manually
	//
	// default to auto choose algorithm depends on certificate's algorithm
	signatureAlgorithm x509.SignatureAlgorithm
	// thisUpdate (optional) default to now
	thisUpdate time.Time
	// nextUpdate (optional) default to 30days later
	nextUpdate time.Time
}

// applyOpts sets the CRL defaults on o (thisUpdate is the current UTC time and nextUpdate is 30
// days later) and then applies opts in order. It returns o on success, or the first option error
// wrapped with a stack trace.
func (o *x509CRLOption) applyOpts(opts ...X509CRLOption) (*x509CRLOption, error) {
	// fill default
	o.thisUpdate = time.Now().UTC()
	o.nextUpdate = o.thisUpdate.Add(30 * 24 * time.Hour)

	// apply options
	for i := range opts {
		if err := opts[i](o); err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return o, nil
}

// X509CRLOption options for create x509 CRL
type X509CRLOption func(*x509CRLOption) error

// WithX509CRLSignatureAlgorithm set signature algorithm
//
// default to auto choose algorithm depends on certificate's algorithm
func WithX509CRLSignatureAlgorithm(algo x509.SignatureAlgorithm) X509CRLOption {
	return func(o *x509CRLOption) error {
		o.signatureAlgorithm = algo
		return nil
	}
}

// WithX509CRLThisUpdate set this update
//
// default to now
func WithX509CRLThisUpdate(thisUpdate time.Time) X509CRLOption {
	return func(o *x509CRLOption) error {
		o.thisUpdate = thisUpdate
		return nil
	}
}

// WithX509CRLNextUpdate set next update
//
// default to 30 days later
func WithX509CRLNextUpdate(nextUpdate time.Time) X509CRLOption {
	return func(o *x509CRLOption) error {
		o.nextUpdate = nextUpdate
		return nil
	}
}

// NewX509CRL creates a CRL and signs it with the issuer CA and its private key.
// It returns the CRL in DER form, or an error.
//
// # Args
//
//   - ca: CA to sign CRL. It must assert the cRLSign key usage ([RFC5280 4.2.1.3]).
//   - prikey: prikey for CA. Its public key must equal ca.PublicKey.
//   - seriaNumber: the CRL number, required.
//   - revokeCerts: certifacates that will be revoked.
//
// The issuer checks fail closed: a CA without cRLSign, or a private key that does
// not belong to ca, is refused instead of producing a CRL that relying parties
// can never verify against ca.
//
// according to [RFC5280 5.2.3], X.509 v3 CRL could have a
// monotonically increasing sequence number as serial number.
//
// [RFC5280 4.2.1.3]: https://www.rfc-editor.org/rfc/rfc5280.html#section-4.2.1.3
// [RFC5280 5.2.3]: https://www.rfc-editor.org/rfc/rfc5280.html#section-5.2.3
func NewX509CRL(ca *x509.Certificate,
	prikey crypto.PrivateKey,
	seriaNumber *big.Int,
	revokeCerts []pkix.RevokedCertificate,
	opts ...X509CRLOption) (crlDer []byte, err error) {
	if ca == nil {
		return nil, errors.New("create CRL: nil issuer")
	}
	if err = validPrikey(prikey); err != nil {
		return nil, errors.WithStack(err)
	}
	if err = validCRLIssuer(ca, Privkey2Signer(prikey)); err != nil {
		return nil, errors.Wrap(err, "create CRL")
	}

	if seriaNumber == nil {
		return nil, errors.Errorf("seriaNumber is empty")
	}

	tpl, err := X509CrlOptions2Tpl(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "convert options to template")
	}

	tpl.Number = seriaNumber
	tpl.ExtraExtensions = ca.ExtraExtensions
	tpl.RevokedCertificateEntries, err = convertRevokedCertificateEntries(revokeCerts)
	if err != nil {
		return nil, errors.Wrap(err, "convert revoked certificate entries")
	}

	crlDer, err = x509.CreateRevocationList(rand.Reader, tpl, ca, Privkey2Signer(prikey))
	if err != nil {
		return nil, errors.Wrap(err, "create revocation list")
	}
	return crlDer, nil
}

// validCRLIssuer checks that ca may issue CRLs and that signer holds its key.
// The ca parameter is the issuer certificate and signer is the CA signer. It
// returns an error when ca does not assert the cRLSign key usage required by
// RFC 5280 section 4.2.1.3, or when the signer's public key differs from
// ca.PublicKey, and nil otherwise.
func validCRLIssuer(ca *x509.Certificate, signer crypto.Signer) error {
	if ca.KeyUsage&x509.KeyUsageCRLSign == 0 {
		return errors.New("issuer certificate does not assert the cRLSign key usage")
	}

	caPub, ok := ca.PublicKey.(interface{ Equal(crypto.PublicKey) bool })
	if !ok {
		return errors.Errorf("issuer certificate has unsupported public key type %T", ca.PublicKey)
	}
	if !caPub.Equal(signer.Public()) {
		return errors.New("private key does not match the issuer certificate public key")
	}

	return nil
}

// VerifyCRL checks that crl is signed by ca and that ca may issue CRLs. It
// returns nil when the signature verifies, or an error describing the failure.
func VerifyCRL(ca *x509.Certificate, crl *x509.RevocationList) error {
	if ca == nil || crl == nil {
		return errors.New("verify CRL: nil issuer or CRL")
	}
	if err := crl.CheckSignatureFrom(ca); err != nil {
		return errors.Wrap(err, "verify CRL signature")
	}

	return nil
}
