package utils

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"io"
	"math/big"
	"strings"

	"github.com/cespare/xxhash"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

// GeneratePasswordHash generate hashed password by origin password
func GeneratePasswordHash(password []byte) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
}

// ValidatePasswordHash validate password is match with hashedPassword
func ValidatePasswordHash(hashedPassword, password []byte) bool {
	return bcrypt.CompareHashAndPassword(hashedPassword, password) == nil
}

// HashSHA128String calculate string's hash by sha256
func HashSHA128String(val string) string {
	b := sha1.Sum([]byte(val))
	return hex.EncodeToString(b[:])
}

// HashSHA256String calculate string's hash by sha256
func HashSHA256String(val string) string {
	b := sha256.Sum256([]byte(val))
	return hex.EncodeToString(b[:])
}

// HashXxhashString calculate string's hash by sha256
func HashXxhashString(val string) string {
	b := xxhash.New().Sum([]byte(val))
	return hex.EncodeToString(b)
}

// EncodeECDSAPrivateKey encode ecdsa private key to pem bytes
func EncodeECDSAPrivateKey(privateKey *ecdsa.PrivateKey) ([]byte, error) {
	x509Encoded, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, errors.Wrap(err, "marshal private key")
	}

	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: x509Encoded}), nil
}

// EncodeECDSAPublicKey encode ecdsa public key to pem bytes
func EncodeECDSAPublicKey(publicKey *ecdsa.PublicKey) ([]byte, error) {
	x509EncodedPub, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return nil, errors.Wrap(err, "marshal public key")
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: x509EncodedPub}), nil
}

// DecodeECDSAPrivateKey decode ecdsa private key from pem bytes
func DecodeECDSAPrivateKey(pemEncoded []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(pemEncoded)
	privateKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "parse private key")
	}
	return privateKey, nil
}

// DecodeECDSAPrivateKey decode ecdsa public key from pem bytes
func DecodeECDSAPublicKey(pemEncodedPub []byte) (*ecdsa.PublicKey, error) {
	blockPub, _ := pem.Decode(pemEncodedPub)
	genericPublicKey, err := x509.ParsePKIXPublicKey(blockPub.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "parse public key")
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
		return nil, nil, errors.Wrap(err, "read contetn")
	}

	return ecdsa.Sign(rand.Reader, priKey, hasher.Sum(nil))
}

// VerifyReaderByECDSAWithSHA256 verify signature by ecdsa public key use sha256
func VerifyReaderByECDSAWithSHA256(pubKey *ecdsa.PublicKey, reader io.Reader, r, s *big.Int) (bool, error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return false, errors.Wrap(err, "read contetn")
	}

	return ecdsa.Verify(pubKey, hasher.Sum(nil), r, s), nil
}

const ecdsaSignDelimiter = "."

// FormatECDSASign format ecdsa sign to stirng
func FormatECDSASign(a, b *big.Int) string {
	return FormatBig2Hex(a) + ecdsaSignDelimiter + FormatBig2Hex(b)
}

// ParseECDSASign parse ecdsa sign string to two *big.Int
func ParseECDSASign(sign string) (a, b *big.Int, ok bool) {
	ss := strings.Split(sign, ecdsaSignDelimiter)
	if len(ss) != 2 {
		return nil, nil, false
	}
	if a, ok = ParseHex2Big(ss[0]); !ok {
		return
	}
	if b, ok = ParseHex2Big(ss[1]); !ok {
		return
	}

	return
}

// FormatBig2Hex format big to hex string
func FormatBig2Hex(b *big.Int) string {
	return b.Text(16)
}

// ParseHex2Big parse hex string to big
func ParseHex2Big(hex string) (b *big.Int, ok bool) {
	b = new(big.Int)
	return b.SetString(hex, 16)
}
