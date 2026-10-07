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

// init creates the package-level default X509CertSerialNumberGenerator that certificate builders
// use when no serial number is supplied. It takes no parameters, returns nothing, and panics if the
// generator cannot be constructed.
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

// parseSans classifies each subject alternative name in sans for a certificate template. Each entry
// is tried, in order, as an IP address (net.ParseIP), an email address (net/mail.ParseAddress,
// keeping only the bare address), and a request URI (url.ParseRequestURI); any entry that matches
// none of these is treated as a DNS name. It returns a sansTemp whose DNSNames, EmailAddresses,
// IPAddresses and URIs fields hold the classified values in their original order.
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

// validPrikey checks whether prikey is a private key type this package can sign with, namely
// *rsa.PrivateKey, *ecdsa.PrivateKey or ed25519.PrivateKey as accepted by Privkey2Signer. It
// returns nil for a supported key and an error for any other type, including nil.
func validPrikey(prikey crypto.PrivateKey) error {
	if v := Privkey2Signer(prikey); v == nil {
		return errors.Errorf("not support this type of private key")
	}

	return nil
}

type oidContainsOption struct {
	prefix bool
}

// applyfs applies the option functions fs to o in order via gutils.Pipeline. It
// returns o on success, or nil and the first option error wrapped with context;
// application stops at that option.
func (o *oidContainsOption) applyfs(fs ...func(o *oidContainsOption) error) (*oidContainsOption, error) {
	o, err := gutils.Pipeline(fs, o)
	if err != nil {
		return nil, errors.Wrap(err, "apply OIDContains option")
	}

	return o, nil
}

// MatchPrefix treat prefix inclusion as a match as well
//
//	`1.2.3` contains `1.2.3.4`
//
// Prefixes are compared arc by arc, so 1.2.3 is not a prefix of 1.2.30, and an
// empty OID is not a prefix of anything.
func MatchPrefix() func(o *oidContainsOption) error {
	return func(o *oidContainsOption) error {
		o.prefix = true
		return nil
	}
}

// OIDContains reports whether oid is in oids. With MatchPrefix it also reports
// true when some element of oids lies under oid, compared arc by arc.
//
// OIDContains cannot return an error, so it fails closed: when any option
// returns an error, it logs that error and reports false (no match) instead of
// evaluating under a partially applied configuration.
func OIDContains(oids []asn1.ObjectIdentifier,
	oid asn1.ObjectIdentifier, opts ...func(o *oidContainsOption) error) bool {
	opt, err := new(oidContainsOption).applyfs(opts...)
	if err != nil {
		glog.Shared.Error("OIDContains option failed, reporting no match",
			zap.Stringer("oid", oid), zap.Error(err))
		return false
	}

	for i := range oids {
		if oids[i].Equal(oid) {
			return true
		}

		if opt.prefix && oidHasPrefix(oids[i], oid) {
			return true
		}
	}

	return false
}

// oidHasPrefix reports whether oid starts with the non-empty arc sequence
// prefix, comparing whole arcs rather than dotted strings. It returns false for
// an empty prefix.
func oidHasPrefix(oid, prefix asn1.ObjectIdentifier) bool {
	if len(prefix) == 0 || len(oid) < len(prefix) {
		return false
	}

	return oid[:len(prefix)].Equal(prefix)
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

	x509oid, err = x509.OIDFromInts(oids)
	if err != nil {
		return x509oid, errors.Wrapf(err, "convert oid %s", oid)
	}

	return x509oid, nil
}

// OidFromString convert string to x509 object identifier
func OidFromString(val string) (x509Oid x509.OID, err error) {
	asnOid, err := gutils.ParseObjectIdentifier(val)
	if err != nil {
		return x509Oid, errors.Wrapf(err, "parse oid %s", val)
	}

	return OidAsn2X509(asnOid)
}
