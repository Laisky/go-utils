package crypto

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509/pkix"
	"encoding/asn1"
	"os"
	"path/filepath"
	"slices"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/emmansun/gmsm/smx509"
	"golang.org/x/crypto/cryptobyte"
	cryptobyte_asn1 "golang.org/x/crypto/cryptobyte/asn1"

	glog "github.com/Laisky/go-utils/v6/log"
)

const (
	// maxTongsuoCRLDERSize bounds the CRL DER accepted by SignX509CRL.
	maxTongsuoCRLDERSize = 64 << 20
	// maxTongsuoCRLSignatureSize bounds the signature produced by the tool.
	maxTongsuoCRLSignatureSize = 16 << 10
	// minTongsuoCRLRSABits is the smallest RSA modulus accepted for CRL signing.
	minTongsuoCRLRSABits = 2048
)

var (
	oidSignatureSHA256WithRSAForCRL   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 11}
	oidSignatureECDSAWithSHA256ForCRL = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 2}
	oidSignatureECDSAWithSHA384ForCRL = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 3}
	oidSignatureECDSAWithSHA512ForCRL = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 4}
	oidSignatureSM2WithSM3ForCRL      = asn1.ObjectIdentifier{1, 2, 156, 10197, 1, 501}
)

// signedCertificateList mirrors the RFC 5280 CertificateList structure with
// the TBS kept as raw DER, so no signed field is ever re-encoded.
type signedCertificateList struct {
	TBSCertList        asn1.RawValue
	SignatureAlgorithm pkix.AlgorithmIdentifier
	SignatureValue     asn1.BitString
}

// tongsuoCRLSignatureAlgorithm selects the CRL signature algorithm and the
// matching tongsuo digest option from the actual signing public key, never
// from the input CRL. RSA keys of at least 2048 bits use SHA-256, ECDSA keys
// use the hash matching their curve and SM2 keys use SM3. It returns an error
// for any other key.
func tongsuoCRLSignatureAlgorithm(pub any) (pkix.AlgorithmIdentifier, string, error) {
	switch key := pub.(type) {
	case *rsa.PublicKey:
		if key.N.BitLen() < minTongsuoCRLRSABits {
			return pkix.AlgorithmIdentifier{}, "", errors.Errorf(
				"CRL signing RSA key must be at least %d bits", minTongsuoCRLRSABits)
		}
		return pkix.AlgorithmIdentifier{
			Algorithm: oidSignatureSHA256WithRSAForCRL, Parameters: asn1.NullRawValue,
		}, tongsuoDigestSHA256, nil
	case *ecdsa.PublicKey:
		switch {
		case isSM2PublicKey(key):
			return pkix.AlgorithmIdentifier{Algorithm: oidSignatureSM2WithSM3ForCRL}, tongsuoDigestSM3, nil
		case key.Curve == elliptic.P256():
			return pkix.AlgorithmIdentifier{Algorithm: oidSignatureECDSAWithSHA256ForCRL}, tongsuoDigestSHA256, nil
		case key.Curve == elliptic.P384():
			return pkix.AlgorithmIdentifier{Algorithm: oidSignatureECDSAWithSHA384ForCRL}, "-sha384", nil
		case key.Curve == elliptic.P521():
			return pkix.AlgorithmIdentifier{Algorithm: oidSignatureECDSAWithSHA512ForCRL}, "-sha512", nil
		}
	}

	return pkix.AlgorithmIdentifier{}, "", errors.Errorf("unsupported CRL signing key type %T", pub)
}

// replaceTBSCertListSignatureAlgorithm returns tbs with only its inner
// signature AlgorithmIdentifier replaced by alg. The version, issuer, update
// times, revoked entries and extensions keep their original DER bytes. It
// returns an error for a malformed TBSCertList.
func replaceTBSCertListSignatureAlgorithm(tbs []byte, alg pkix.AlgorithmIdentifier) ([]byte, error) {
	algDer, err := asn1.Marshal(alg)
	if err != nil {
		return nil, errors.Wrap(err, "marshal signature algorithm")
	}

	input := cryptobyte.String(tbs)
	var body cryptobyte.String
	if !input.ReadASN1(&body, cryptobyte_asn1.SEQUENCE) || !input.Empty() {
		return nil, errors.New("malformed tbs crl")
	}

	var version cryptobyte.String
	if body.PeekASN1Tag(cryptobyte_asn1.INTEGER) && !body.ReadASN1Element(&version, cryptobyte_asn1.INTEGER) {
		return nil, errors.New("malformed tbs crl version")
	}
	var oldAlg cryptobyte.String
	if !body.ReadASN1Element(&oldAlg, cryptobyte_asn1.SEQUENCE) {
		return nil, errors.New("malformed tbs crl signature algorithm")
	}

	var out cryptobyte.Builder
	out.AddASN1(cryptobyte_asn1.SEQUENCE, func(b *cryptobyte.Builder) {
		b.AddBytes(version)
		b.AddBytes(algDer)
		b.AddBytes(body)
	})

	result, err := out.Bytes()
	if err != nil {
		return nil, errors.Wrap(err, "marshal tbs crl")
	}
	return result, nil
}

// extensionsEqual reports whether two extension lists are identical in order,
// OID, criticality and value.
func extensionsEqual(a, b []pkix.Extension) bool {
	return slices.EqualFunc(a, b, func(x, y pkix.Extension) bool {
		return x.Id.Equal(y.Id) && x.Critical == y.Critical && bytes.Equal(x.Value, y.Value)
	})
}

// verifyResignedCRL requires that signed is exactly one DER CRL whose
// signature verifies under pub and whose issuer, number, update times, revoked
// entries and extensions equal those of input. It returns an error otherwise.
func verifyResignedCRL(input *smx509.RevocationList, signed []byte, pub any) error {
	out, err := smx509.ParseRevocationList(signed)
	if err != nil {
		return errors.Wrap(err, "parse signed crl")
	}
	if !bytes.Equal(out.Raw, signed) {
		return errors.New("signed crl has trailing data")
	}

	verifier := &smx509.Certificate{PublicKey: pub}
	if err = verifier.CheckSignature(out.SignatureAlgorithm, out.RawTBSRevocationList, out.Signature); err != nil {
		return errors.Wrap(err, "verify signed crl signature")
	}

	switch {
	case !bytes.Equal(out.RawIssuer, input.RawIssuer):
		return errors.New("signed crl issuer differs from the input")
	case (out.Number == nil) != (input.Number == nil) ||
		(out.Number != nil && out.Number.Cmp(input.Number) != 0):
		return errors.New("signed crl number differs from the input")
	case !out.ThisUpdate.Equal(input.ThisUpdate) || !out.NextUpdate.Equal(input.NextUpdate):
		return errors.New("signed crl update times differ from the input")
	case len(out.RevokedCertificateEntries) != len(input.RevokedCertificateEntries):
		return errors.New("signed crl revoked entries differ from the input")
	case !extensionsEqual(out.Extensions, input.Extensions):
		return errors.New("signed crl extensions differ from the input")
	}
	for i := range out.RevokedCertificateEntries {
		if !bytes.Equal(out.RevokedCertificateEntries[i].Raw, input.RevokedCertificateEntries[i].Raw) {
			return errors.Errorf("signed crl revoked entry %d differs from the input", i)
		}
	}

	return nil
}

// SignX509CRL re-signs a DER CRL with the CA private key through the tongsuo
// binary and returns the signed CRL as exactly one DER object.
//
// The input must be exactly one DER CRL (PEM and trailing data are rejected).
// The signature algorithm is chosen from the signing key: SHA-256 with RSA
// (at least 2048 bits), ECDSA with the hash matching P-256/P-384/P-521, or
// SM2 with SM3. Only the TBS signature algorithm is replaced; issuer, CRL
// number, update times, revoked entries and extensions keep their original
// DER bytes. The result is parsed, its signature verified under the public
// key derived from PrikeyPem and its fields compared with the input before it
// is returned; any subprocess failure or unexpected output returns an error
// and no bytes. Callers should still verify the CRL against the intended
// issuer certificate (for example smx509.RevocationList.CheckSignatureFrom)
// before distributing it.
func (t *Tongsuo) SignX509CRL(ctx context.Context,
	CrlDer []byte,
	PrikeyPem []byte,
) (signedCrlDer []byte, err error) {
	if len(CrlDer) == 0 || len(CrlDer) > maxTongsuoCRLDERSize {
		return nil, errors.Errorf("crl DER size must be between 1 and %d bytes", maxTongsuoCRLDERSize)
	}
	if len(PrikeyPem) == 0 {
		return nil, errors.New("crl signing key must not be empty")
	}

	input, err := smx509.ParseRevocationList(CrlDer)
	if err != nil {
		return nil, errors.Wrap(err, "parse input crl DER")
	}
	if !bytes.Equal(input.Raw, CrlDer) {
		return nil, errors.New("input crl has trailing data")
	}

	dir, err := os.MkdirTemp("", "tongsuo*")
	if err != nil {
		return nil, errors.Wrap(err, "generate temp dir")
	}
	defer t.removeAll(dir)

	// derive the public key from the private key, which stays on stdin
	pubPath := filepath.Join(dir, "pub.der")
	if _, err = t.runCMD(ctx, []string{
		"pkey", tongsuoFlagIn, tongsuoStdinPath, "-pubout",
		tongsuoFlagOutform, tongsuoFormatDER, tongsuoFlagOut, pubPath,
	}, PrikeyPem); err != nil {
		return nil, errors.Wrap(err, "derive crl signing public key")
	}
	pubDer, err := os.ReadFile(pubPath)
	if err != nil {
		return nil, errors.Wrap(err, "read crl signing public key")
	}
	pub, err := smx509.ParsePKIXPublicKey(pubDer)
	if err != nil {
		return nil, errors.Wrap(err, "parse crl signing public key")
	}

	alg, digest, err := tongsuoCRLSignatureAlgorithm(pub)
	if err != nil {
		return nil, errors.Wrap(err, "select crl signature algorithm")
	}
	tbs, err := replaceTBSCertListSignatureAlgorithm(input.RawTBSRevocationList, alg)
	if err != nil {
		return nil, errors.Wrap(err, "prepare tbs crl")
	}

	tbsPath := filepath.Join(dir, "tbs.der")
	if err = os.WriteFile(tbsPath, tbs, 0600); err != nil {
		return nil, errors.Wrap(err, "write tbs crl")
	}
	sigPath := filepath.Join(dir, "signature")
	if _, err = t.runCMD(ctx, []string{
		tongsuoCmdDgst, digest, "-sign", tongsuoStdinPath, tongsuoFlagOut, sigPath, tbsPath,
	}, PrikeyPem); err != nil {
		return nil, errors.Wrap(err, "sign tbs crl")
	}
	signature, err := os.ReadFile(sigPath)
	if err != nil {
		return nil, errors.Wrap(err, "read crl signature")
	}
	if len(signature) == 0 || len(signature) > maxTongsuoCRLSignatureSize {
		return nil, errors.Errorf("unexpected crl signature size %d", len(signature))
	}

	signed, err := asn1.Marshal(signedCertificateList{
		TBSCertList:        asn1.RawValue{FullBytes: tbs},
		SignatureAlgorithm: alg,
		SignatureValue:     asn1.BitString{Bytes: signature, BitLength: 8 * len(signature)},
	})
	if err != nil {
		return nil, errors.Wrap(err, "marshal signed crl")
	}

	if err = verifyResignedCRL(input, signed, pub); err != nil {
		return nil, errors.Wrap(err, "verify signed crl")
	}

	glog.Shared.Debug("signed crl with tongsuo",
		zap.String("signature_algorithm", alg.Algorithm.String()),
		zap.Int("revoked_entries", len(input.RevokedCertificateEntries)))
	return signed, nil
}
