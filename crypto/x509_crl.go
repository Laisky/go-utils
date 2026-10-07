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

// NewX509CRL create and sign CRL
//
// # Args
//
//   - ca: CA to sign CRL.
//   - prikey: prikey for CA.
//   - revokeCerts: certifacates that will be revoked.
//   - WithX509CertSeriaNumber() is required for NewX509CRL.
//
// according to [RFC5280 5.2.3], X.509 v3 CRL could have a
// monotonically increasing sequence number as serial number.
//
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

// VerifyCRL verify crl by ca
func VerifyCRL(ca *x509.Certificate, crl *x509.RevocationList) error {
	return crl.CheckSignatureFrom(ca)
}
