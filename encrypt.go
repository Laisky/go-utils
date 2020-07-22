package utils

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"strings"

	"github.com/Laisky/zap"
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

// EncodeRSAPrivateKey encode rsa private key to pem bytes
func EncodeRSAPrivateKey(privateKey *rsa.PrivateKey) ([]byte, error) {
	x509Encoded := x509.MarshalPKCS1PrivateKey(privateKey)
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: x509Encoded}), nil
}

// EncodeRSAPublicKey encode rsa public key to pem bytes
func EncodeRSAPublicKey(publicKey *rsa.PublicKey) ([]byte, error) {
	x509EncodedPub := x509.MarshalPKCS1PublicKey(publicKey)
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: x509EncodedPub}), nil
}

// DecodeRSAPrivateKey decode rsa private key from pem bytes
func DecodeRSAPrivateKey(pemEncoded []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemEncoded)
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "parse rsa private key")
	}

	return privateKey, nil
}

// DecodeRSAPublicKey decode rsa public key from pem bytes
func DecodeRSAPublicKey(pemEncodedPub []byte) (*rsa.PublicKey, error) {
	blockPub, _ := pem.Decode(pemEncodedPub)
	pubkey, err := x509.ParsePKCS1PublicKey(blockPub.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "parse rsa public key")
	}

	return pubkey, nil
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

// SignByRSAWithSHA256 generate signature by rsa private key use sha256
func SignByRSAWithSHA256(priKey *rsa.PrivateKey, content []byte) ([]byte, error) {
	hashed := sha256.Sum256(content)
	return rsa.SignPKCS1v15(rand.Reader, priKey, crypto.SHA256, hashed[:])
}

// VerifyByRSAWithSHA256 verify signature by rsa public key use sha256
func VerifyByRSAWithSHA256(pubKey *rsa.PublicKey, content []byte, sig []byte) error {
	hash := sha256.Sum256(content)
	return rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hash[:], sig)
}

// SignReaderByRSAWithSHA256 generate signature by rsa private key use sha256
func SignReaderByRSAWithSHA256(priKey *rsa.PrivateKey, reader io.Reader) (sig []byte, err error) {
	hasher := sha256.New()
	if _, err = io.Copy(hasher, reader); err != nil {
		return nil, errors.Wrap(err, "read content")
	}

	return rsa.SignPKCS1v15(rand.Reader, priKey, crypto.SHA256, hasher.Sum(nil))
}

// VerifyReaderByRSAWithSHA256 verify signature by rsa public key use sha256
func VerifyReaderByRSAWithSHA256(pubKey *rsa.PublicKey, reader io.Reader, sig []byte) error {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return errors.Wrap(err, "read content")
	}

	return rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hasher.Sum(nil), sig)
}

const ecdsaSignDelimiter = "."

// FormatECDSASign encode es256 signature by hex
//
// Deprecated: replaced by EncodeES256SignByBase6e
var FormatECDSASign = EncodeES256SignByHex

// EncodeES256SignByHex format ecdsa sign to stirng
func EncodeES256SignByHex(a, b *big.Int) string {
	return FormatBig2Hex(a) + ecdsaSignDelimiter + FormatBig2Hex(b)
}

// ParseECDSASign encode es256 signature by base64
//
// Deprecated: replaced by EncodeES256SignByBase64
func ParseECDSASign(sign string) (a, b *big.Int, ok bool) {
	var err error
	if a, b, err = DecodeES256SignByHex(sign); err != nil {
		return nil, nil, false
	}

	return a, b, true
}

// DecodeES256SignByHex parse ecdsa signature string to two *big.Int
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

func expandAesSecret(secret []byte) []byte {
	var n int
	if len(secret) <= 16 {
		n = 16 - len(secret)
	} else if len(secret) <= 24 {
		n = 24 - len(secret)
	} else if len(secret) <= 32 {
		n = 32 - len(secret)
	} else {
		return secret[:32]
	}

	Logger.Debug("expand secuet", zap.Int("raw", len(secret)), zap.Int("expand", n))
	newSec := secret[:len(secret):len(secret)]
	return append(newSec, make([]byte, n)...)
}

// EncryptByAes encrypt bytes by aes with key
//
// inspired by https://tutorialedge.net/golang/go-encrypt-decrypt-aes-tutorial/
func EncryptByAes(secret []byte, cnt []byte) ([]byte, error) {
	if len(secret) == 0 {
		return nil, fmt.Errorf("secret is empty")
	}

	// generate a new aes cipher
	c, err := aes.NewCipher(expandAesSecret(secret))
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
	if len(secret) == 0 {
		return nil, fmt.Errorf("secret is empty")
	}

	// generate a new aes cipher
	c, err := aes.NewCipher(expandAesSecret(secret))
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

// AesReaderWrapper used to decrypt encrypted reader
type AesReaderWrapper struct {
	cnt []byte
	idx int
}

// NewAesReaderWrapper wrap reader by aes
func NewAesReaderWrapper(in io.Reader, key []byte) (*AesReaderWrapper, error) {
	cipher, err := ioutil.ReadAll(in)
	if err != nil {
		return nil, errors.Wrap(err, "read reader")
	}

	w := new(AesReaderWrapper)
	if w.cnt, err = DecryptByAes(key, cipher); err != nil {
		return nil, errors.Wrap(err, "decrypt")
	}

	return w, nil
}

func (w *AesReaderWrapper) Read(p []byte) (n int, err error) {
	if w.idx == len(w.cnt) {
		return 0, io.EOF
	}

	for n = range p {
		p[n] = w.cnt[w.idx]
		w.idx++
		if w.idx == len(w.cnt) {
			break
		}
	}

	return n + 1, nil
}
