package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
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

// FormatECDSASign
//
// Deprecated: use EncodeES256SignByBase6e instead
var FormatECDSASign = EncodeES256SignByHex

// EncodeES256SignByHex format ecdsa sign to stirng
func EncodeES256SignByHex(a, b *big.Int) string {
	return FormatBig2Hex(a) + ecdsaSignDelimiter + FormatBig2Hex(b)
}

// ParseECDSASign(Deprecated)
func ParseECDSASign(sign string) (a, b *big.Int, ok bool) {
	var err error
	if a, b, err = DecodeES256SignByHex(sign); err != nil {
		return nil, nil, false
	}

	return a, b, true
}

// DecodeES256SignByHex parse ecdsa sign string to two *big.Int
func DecodeES256SignByHex(sign string) (a, b *big.Int, err error) {
	ss := strings.Split(sign, ecdsaSignDelimiter)
	if len(ss) != 2 {
		return nil, nil, fmt.Errorf("unknown format of signature `%s`, want `xxx.xxx`", sign)
	}
	var ok bool
	if a, ok = ParseHex2Big(ss[0]); !ok {
		return nil, nil, fmt.Errorf("invalidate hex `%s`", ss[0])
	}
	if b, ok = ParseHex2Big(ss[1]); !ok {
		return nil, nil, fmt.Errorf("invalidate hex `%s`", ss[1])
	}

	return
}

// EncodeES256SignByBase64 format ecdsa sign to stirng
func EncodeES256SignByBase64(a, b *big.Int) string {
	return FormatBig2Base64(a) + ecdsaSignDelimiter + FormatBig2Base64(b)
}

// DecodeES256SignByBase64 parse ecdsa sign string to two *big.Int
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

// FormatBig2Hex format big to hex string
func FormatBig2Hex(b *big.Int) string {
	return b.Text(16)
}

// ParseHex2Big parse hex string to big
func ParseHex2Big(hex string) (b *big.Int, ok bool) {
	b = new(big.Int)
	return b.SetString(hex, 16)
}

// FormatBig2Base64 format big to base64 string
func FormatBig2Base64(b *big.Int) string {
	return base64.URLEncoding.EncodeToString(b.Bytes())
}

// ParseBase642Big parse base64 string to big
func ParseBase642Big(raw string) (*big.Int, error) {
	bb, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}

	b := new(big.Int)
	b.SetBytes(bb)
	return b, nil
}

// EncryptByAES encrypt bytes by aes with key
//
// inspired by https://tutorialedge.net/golang/go-encrypt-decrypt-aes-tutorial/
func EncryptByAES(secret []byte, cnt []byte) ([]byte, error) {
	// generate a new aes cipher
	c, err := aes.NewCipher(secret)
	if err != nil {
		return nil, errors.Wrap(err, "new aes cipher")
	}

	// gcm or Galois/Counter Mode, is a mode of operation
	// for symmetric key cryptographic block ciphers
	// * https://en.wikipedia.org/wiki/Galois/Counter_Mode
	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, errors.Wrap(err, "new gcm")
	}

	// creates a new byte array the size of the nonce
	// which must be passed to Seal
	nonce := make([]byte, gcm.NonceSize())
	// populates our nonce with a cryptographically secure
	// random sequence
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, errors.Wrap(err, "load nonce")
	}

	// here we encrypt our text using the Seal function
	// Seal encrypts and authenticates plaintext, authenticates the
	// additional data and appends the result to dst, returning the updated
	// slice. The nonce must be NonceSize() bytes long and unique for all
	// time, for a given key.
	return gcm.Seal(nonce, nonce, cnt, nil), nil
}

// DecryptByAes encrypt bytes by aes with key
//
// inspired by https://tutorialedge.net/golang/go-encrypt-decrypt-aes-tutorial/
func DecryptByAes(secret []byte, encrypted []byte) ([]byte, error) {
	// generate a new aes cipher
	c, err := aes.NewCipher(secret)
	if err != nil {
		return nil, errors.Wrap(err, "new aes cipher")
	}

	// gcm or Galois/Counter Mode, is a mode of operation
	// for symmetric key cryptographic block ciphers
	// * https://en.wikipedia.org/wiki/Galois/Counter_Mode
	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, errors.Wrap(err, "new gcm")
	}

	nonceSize := gcm.NonceSize()
	if len(encrypted) < nonceSize {
		return nil, fmt.Errorf("encrypted too short")
	}

	nonce, encrypted := encrypted[:nonceSize], encrypted[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return nil, errors.Wrap(err, "gcm decrypt")
	}

	return plaintext, nil
}
