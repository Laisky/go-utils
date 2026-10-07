package crypto

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/asn1"
	"math/big"
	"net"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	gutils "github.com/Laisky/go-utils/v6"
	glog "github.com/Laisky/go-utils/v6/log"
)

// X509CertSerialNumberGenerator x509 certificate serial number generator
type X509CertSerialNumberGenerator interface {
	SerialNum() int64
}

var (
	internalCertSerialNumGenerator X509CertSerialNumberGenerator
)

func init() {
	var err error
	if internalCertSerialNumGenerator, err = NewDefaultX509CertSerialNumGenerator(); err != nil {
		glog.Shared.Panic("new default cert serial number generator", zap.Error(err))
	}
}

// DefaultX509CertSerialNumGenerator generates cryptographically random
// certificate serial numbers per RFC 5280 Section 4.1.2.2.
type DefaultX509CertSerialNumGenerator struct{}

// NewDefaultX509CertSerialNumGenerator new DefaultX509CertSerialNumGenerator
func NewDefaultX509CertSerialNumGenerator() (*DefaultX509CertSerialNumGenerator, error) {
	return &DefaultX509CertSerialNumGenerator{}, nil
}

// SerialNum generates a cryptographically random positive int64 serial number.
//
// Uses crypto/rand to ensure unpredictability as required by RFC 5280.
// The result is always positive (63 bits of entropy).
func (g *DefaultX509CertSerialNumGenerator) SerialNum() int64 {
	// Generate 63 bits of randomness (positive int64)
	maxSerial := new(big.Int).SetInt64(1<<63 - 1)
	n, err := rand.Int(rand.Reader, maxSerial)
	if err != nil {
		// Fall back to timestamp-based if crypto/rand fails (extremely unlikely)
		return time.Now().UnixNano()
	}
	return n.Int64()
}

// NewX509CertTemplate new tls template with common default values
// func NewX509CertTemplate(opts ...X509CertOption) (tpl *x509.Certificate, err error) {
// 	opt, err := new(x509V3CertOption).fillDefault().applyOpts(opts...)
// 	if err != nil {
// 		return nil, err
// 	}

// 	notAfter := opt.validFrom.Add(opt.validFor)
// 	tpl = &x509.Certificate{
// 		SignatureAlgorithm: opt.signatureAlgorithm,
// 		SerialNumber:       opt.serialNumber,
// 		Subject: pkix.Name{
// 			CommonName:         opt.commonName,
// 			Organization:       opt.organization,
// 			OrganizationalUnit: opt.organizationUnit,
// 			Locality:           opt.locality,
// 			Country:            opt.country,
// 			Province:           opt.province,
// 			StreetAddress:      opt.streetAddrs,
// 			PostalCode:         opt.PostalCode,
// 		},
// 		NotBefore: opt.validFrom,
// 		NotAfter:  notAfter,

// 		KeyUsage:              opt.keyUsage,
// 		ExtKeyUsage:           opt.extKeyUsage,
// 		BasicConstraintsValid: true,
// 		IsCA:                  opt.isCA,
// 		PolicyIdentifiers:     opt.policies,
// 		CRLDistributionPoints: opt.crls,
// 		OCSPServer:            opt.ocsps,
// 	}

// 	sansTpl := parseSans(opt.sans)
// 	tpl.DNSNames = sansTpl.DNSNames
// 	tpl.EmailAddresses = sansTpl.EmailAddresses
// 	tpl.IPAddresses = sansTpl.IPAddresses
// 	tpl.URIs = sansTpl.URIs

// 	return tpl, nil
// }

type sansTemp struct {
	DNSNames       []string
	EmailAddresses []string
	IPAddresses    []net.IP
	URIs           []*url.URL
}

func parseSans(sans []string) (tpl sansTemp) {
	for i := range sans {
		if ip := net.ParseIP(sans[i]); ip != nil {
			tpl.IPAddresses = append(tpl.IPAddresses, ip)
		} else if email, err := mail.ParseAddress(sans[i]); err == nil && email != nil {
			tpl.EmailAddresses = append(tpl.EmailAddresses, email.Address)
		} else if uri, err := url.ParseRequestURI(sans[i]); err == nil && uri != nil {
			tpl.URIs = append(tpl.URIs, uri)
		} else {
			tpl.DNSNames = append(tpl.DNSNames, sans[i])
		}
	}

	return tpl
}

// Privkey2Signer convert privkey to signer
func Privkey2Signer(privkey crypto.PrivateKey) crypto.Signer {
	switch privkey := privkey.(type) {
	case *rsa.PrivateKey:
		return privkey
	case *ecdsa.PrivateKey:
		return privkey
	case ed25519.PrivateKey:
		return privkey
	default:
		return nil
	}
}

func validPrikey(prikey crypto.PrivateKey) error {
	if v := Privkey2Signer(prikey); v == nil {
		return errors.Errorf("not support this type of private key")
	}

	return nil
}

type oidContainsOption struct {
	prefix bool
}

func (o *oidContainsOption) applyfs(fs ...func(o *oidContainsOption) error) *oidContainsOption {
	o, _ = gutils.Pipeline(fs, o)
	return o
}

// MatchPrefix treat prefix inclusion as a match as well
//
//	`1.2.3` contains `1.2.3.4`
func MatchPrefix() func(o *oidContainsOption) error {
	return func(o *oidContainsOption) error {
		o.prefix = true
		return nil
	}
}

// OIDContains is oid in oids
func OIDContains(oids []asn1.ObjectIdentifier,
	oid asn1.ObjectIdentifier, opts ...func(o *oidContainsOption) error) bool {
	opt := new(oidContainsOption).applyfs(opts...)

	for i := range oids {
		if oids[i].Equal(oid) {
			return true
		}

		if opt.prefix && strings.HasPrefix(oids[i].String(), oid.String()) {
			return true
		}
	}

	return false
}

// X509CertSubjectKeyID generate subject key id for pubkey
//
// if x509 certificate template is a CA, subject key id will generated by golang automatelly
//
//   - https://cs.opensource.google/go/go/+/refs/tags/go1.19.5:src/crypto/x509/x509.go;l=1476
func X509CertSubjectKeyID(pubkey crypto.PublicKey) ([]byte, error) {
	keyBytes, err := Pubkey2Der(pubkey)
	if err != nil {
		return nil, errors.Wrap(err, "marshal pubkeu")
	}

	hasher := sha1.New()
	if _, err := hasher.Write(keyBytes); err != nil {
		return nil, errors.Wrap(err, "hash pubkey")
	}

	return hasher.Sum(nil), nil
}

// OidAsn2X509 convert asn1 object identifier to x509 object identifier
func OidAsn2X509(oid asn1.ObjectIdentifier) (x509oid x509.OID, err error) {
	if len(oid) == 0 {
		return x509oid, nil // Return an empty x509.OID without error
	}

	oids := make([]uint64, 0, len(oid))
	for i := range oid {
		// Check for negative numbers or numbers too large for uint64
		if oid[i] < 0 {
			return x509oid, errors.New("invalid oid")
		}
		oids = append(oids, uint64(oid[i])) //nolint:gosec // G115: integer overflow // impossible
	}

	return x509.OIDFromInts(oids)
}

// OidFromString convert string to x509 object identifier
func OidFromString(val string) (x509Oid x509.OID, err error) {
	asnOid, err := gutils.ParseObjectIdentifier(val)
	if err != nil {
		return x509Oid, errors.Wrapf(err, "parse oid %s", val)
	}

	return OidAsn2X509(asnOid)
}
