package encrypt

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"io"
	"math/big"
	"strings"

	"github.com/pkg/errors"
)

// EncodeECDSAPrivateKey encode ecdsa private key to pem bytes
func EncodeECDSAPrivateKey(privateKey *ecdsa.PrivateKey) ([]byte, error) {
	x509Encoded, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, errors.Wrap(err, "marshal ecdsa private key")
	}

	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: x509Encoded}), nil
}

// EncodeECDSAPublicKey encode ecdsa public key to pem bytes
func EncodeECDSAPublicKey(publicKey *ecdsa.PublicKey) ([]byte, error) {
	x509EncodedPub, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return nil, errors.Wrap(err, "marshal ecdsa public key")
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: x509EncodedPub}), nil
}

// DecodeECDSAPrivateKey decode ecdsa private key from pem bytes
func DecodeECDSAPrivateKey(pemEncoded []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(pemEncoded)
	privateKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "parse ecdsa private key")
	}

	return privateKey, nil
}

// DecodeECDSAPublicKey decode ecdsa public key from pem bytes
func DecodeECDSAPublicKey(pemEncodedPub []byte) (*ecdsa.PublicKey, error) {
	blockPub, _ := pem.Decode(pemEncodedPub)
	genericPublicKey, err := x509.ParsePKIXPublicKey(blockPub.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "parse ecdsa public key")
	}

	return genericPublicKey.(*ecdsa.PublicKey), nil
}

// SignByECDSAWithSHA256 generate signature by ecdsa private key use sha256
func SignByECDSAWithSHA256(priKey *ecdsa.PrivateKey, content []byte) (r, s *big.Int, err error) {
	hash := sha256.Sum256(content)
	return ecdsa.Sign(rand.Reader, priKey, hash[:])
}

// VerifyByECDSAWithSHA256 verify signature by ecdsa public key use sha256
func VerifyByECDSAWithSHA256(pubKey *ecdsa.PublicKey, content []byte, r, s *big.Int) bool {
	hash := sha256.Sum256(content)
	return ecdsa.Verify(pubKey, hash[:], r, s)
}

// SignReaderByECDSAWithSHA256 generate signature by ecdsa private key use sha256
func SignReaderByECDSAWithSHA256(priKey *ecdsa.PrivateKey, reader io.Reader) (r, s *big.Int, err error) {
	hasher := sha256.New()
	if _, err = io.Copy(hasher, reader); err != nil {
		return nil, nil, errors.Wrap(err, "read content")
	}

	return ecdsa.Sign(rand.Reader, priKey, hasher.Sum(nil))
}

// VerifyReaderByECDSAWithSHA256 verify signature by ecdsa public key use sha256
func VerifyReaderByECDSAWithSHA256(pubKey *ecdsa.PublicKey, reader io.Reader, r, s *big.Int) (bool, error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return false, errors.Wrap(err, "read content")
	}

	return ecdsa.Verify(pubKey, hasher.Sum(nil), r, s), nil
}

const ecdsaSignDelimiter = "."

// EncodeES256SignByHex format ecdsa sign to stirng
func EncodeES256SignByHex(a, b *big.Int) string {
	return FormatBig2Hex(a) + ecdsaSignDelimiter + FormatBig2Hex(b)
}

// DecodeES256SignByHex parse ecdsa signature string to two *big.Int
func DecodeES256SignByHex(sign string) (a, b *big.Int, err error) {
	ss := strings.Split(sign, ecdsaSignDelimiter)
	if len(ss) != 2 {
		return nil, nil, errors.Errorf("unknown format of signature `%s`, want `xxx.xxx`", sign)
	}
	var ok bool
	if a, ok = ParseHex2Big(ss[0]); !ok {
		return nil, nil, errors.Errorf("invalidate hex `%s`", ss[0])
	}
	if b, ok = ParseHex2Big(ss[1]); !ok {
		return nil, nil, errors.Errorf("invalidate hex `%s`", ss[1])
	}

	return
}

// EncodeES256SignByBase64 format ecdsa signature to stirng
func EncodeES256SignByBase64(a, b *big.Int) string {
	return FormatBig2Base64(a) + ecdsaSignDelimiter + FormatBig2Base64(b)
}

// DecodeES256SignByBase64 parse ecdsa signature string to two *big.Int
func DecodeES256SignByBase64(sign string) (a, b *big.Int, err error) {
	ss := strings.Split(sign, ecdsaSignDelimiter)
	if len(ss) != 2 {
		return nil, nil, errors.Wrapf(err, "unknown format of signature `%s`, expect is `xxxx.xxxx`", sign)
	}

	if a, err = ParseBase642Big(ss[0]); err != nil {
		return nil, nil, errors.Wrapf(err, "`%s` is not validate base64", ss[0])
	}

	if b, err = ParseBase642Big(ss[1]); err != nil {
		return nil, nil, errors.Wrapf(err, "`%s` is not validate base64", ss[1])
	}

	return
}
