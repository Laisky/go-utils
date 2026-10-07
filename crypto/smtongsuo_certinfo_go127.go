//go:build go1.27

package crypto

import (
	"crypto/x509"

	"github.com/Laisky/errors/v2"
	"golang.org/x/crypto/cryptobyte"
	cryptobyte_asn1 "golang.org/x/crypto/cryptobyte/asn1"
)

// setX509RawSignatureAlgorithm fills cert.RawSignatureAlgorithm (added in Go
// 1.27) with the DER AlgorithmIdentifier inside cert.RawTBSCertificate, exactly
// as x509.ParseCertificate does. It returns an error if the TBS is malformed.
func setX509RawSignatureAlgorithm(cert *x509.Certificate) error {
	tbs := cryptobyte.String(cert.RawTBSCertificate)
	if !tbs.ReadASN1(&tbs, cryptobyte_asn1.SEQUENCE) {
		return errors.New("malformed tbs certificate")
	}
	if !tbs.SkipOptionalASN1(cryptobyte_asn1.Tag(0).Constructed().ContextSpecific()) {
		return errors.New("malformed tbs certificate version")
	}
	if !tbs.SkipASN1(cryptobyte_asn1.INTEGER) {
		return errors.New("malformed tbs certificate serial number")
	}

	var algorithm cryptobyte.String
	if !tbs.ReadASN1Element(&algorithm, cryptobyte_asn1.SEQUENCE) {
		return errors.New("malformed tbs certificate signature algorithm")
	}

	cert.RawSignatureAlgorithm = algorithm
	return nil
}
