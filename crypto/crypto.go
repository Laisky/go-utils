// Package crypto is a collection of cryptographic algorithms and protocols, providing
// hash functions, block and stream ciphers, public key cryptography and authentication.
// It also includes a cryptographically secure pseudo-random number generator.
//
// x.509
//
// This package provides many useful functions for x.509 certificate.
// you can build a PKI system with this package.
// including parsing and verification. it can be used to parse x.509 certificates,
// create x.509 certificate chains, verify x.509 certificate chains,
// and parse x.509 certificate revocation lists.
package crypto

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"golang.org/x/crypto/bcrypt"

	gutils "github.com/Laisky/go-utils/v6"
	glog "github.com/Laisky/go-utils/v6/log"
)

const (
	// DefaultPasswordDelay default delay time for password hashing and verifying
	DefaultPasswordDelay = 2 * time.Second
	// MaxPasswordHashIteration limit max hash iteration count
	MaxPasswordHashIteration = 1000000
	// MinPasswordHashIteration limit min hash iteration count
	MinPasswordHashIteration = 10000
	// legacyMinPasswordHashIteration keeps compatibility for already stored hashes.
	legacyMinPasswordHashIteration = 1
)

// HashedPassword salt hashed password
//
// generate by PasswordHash, verify by VerifyHashedPassword
type HashedPassword struct {
	salt           []byte
	hasher         gutils.HashTypeInterface
	hashNum        int
	hashedPassword []byte
}

// String convert to string
//
// can verify by VerifyHashedPassword
func (p HashedPassword) String() string {
	return fmt.Sprintf("%s.%d.%s.%s",
		p.hasher.String(),
		p.hashNum,
		hex.EncodeToString(p.salt),
		hex.EncodeToString(p.hashedPassword),
	)
}

func newHashedPassword(salt, rawpassword []byte,
	hasher gutils.HashTypeInterface,
	hashNum int) (h HashedPassword, err error) {
	return newHashedPasswordWithMinIteration(salt, rawpassword, hasher, hashNum, MinPasswordHashIteration)
}

// newHashedPasswordWithMinIteration builds a hashed password and validates the iteration range.
//
// Params:
//   - salt: random salt bytes appended to rawpassword.
//   - rawpassword: plaintext password bytes.
//   - hasher: hash implementation used to iteratively hash.
//   - hashNum: iteration count for hashing.
//   - minHashNum: minimum accepted iteration count.
//
// Returns:
//   - h: hashed password payload for serialization and verification.
//   - err: wrapped error when iterations are out of range or hashing fails.
func newHashedPasswordWithMinIteration(salt, rawpassword []byte,
	hasher gutils.HashTypeInterface,
	hashNum, minHashNum int) (h HashedPassword, err error) {
	if hashNum < minHashNum || hashNum > MaxPasswordHashIteration {
		return h, errors.Errorf("hashNum %d out of range [%d,%d]",
			hashNum, minHashNum, MaxPasswordHashIteration)
	}

	h.salt = salt
	h.hasher = hasher
	h.hashNum = hashNum

	h.hashedPassword = make([]byte, 0, len(rawpassword)+len(h.salt))
	h.hashedPassword = append(h.hashedPassword, rawpassword...)
	h.hashedPassword = append(h.hashedPassword, h.salt...)
	for i := 0; i < h.hashNum; i++ {
		h.hashedPassword, err = gutils.Hash(h.hasher, bytes.NewReader(h.hashedPassword))
		if err != nil {
			return h, errors.Wrap(err, "calculate password hash")
		}
	}

	return h, nil
}

func parseHashedPassword(hashedString string) (h HashedPassword, err error) {
	hs := strings.Split(hashedString, ".")
	if len(hs) != 4 {
		return h, errors.Errorf("hashedString must contains 4 parts")
	}

	h.hasher = gutils.HashType(hs[0])
	h.hashNum, err = strconv.Atoi(hs[1])
	if err != nil {
		return h, errors.Wrap(err, "parse hash num")
	}

	if h.hashNum > MaxPasswordHashIteration {
		return h, errors.Errorf("too many iterations %d > %d",
			h.hashNum, MaxPasswordHashIteration)
	} else if h.hashNum <= 0 {
		return h, errors.Errorf("invalid iterations %d", h.hashNum)
	}

	h.salt, err = hex.DecodeString(hs[2])
	if err != nil {
		return h, errors.Wrap(err, "decode salt")
	}

	h.hashedPassword, err = hex.DecodeString(hs[3])
	if err != nil {
		return h, errors.Wrap(err, "decode hashed password")
	}

	return h, nil
}

// VerifyHashedPassword verify HashedPassword
func VerifyHashedPassword(rawpassword []byte, hashedPassword string) (err error) {
	if len(rawpassword) == 0 || len(hashedPassword) == 0 {
		return errors.Errorf("rawpassword or hashedPassword is empty")
	}

	defer gutils.NewDelay(DefaultPasswordDelay).Wait()
	hp, err := parseHashedPassword(hashedPassword)
	if err != nil {
		return errors.Wrap(err, "parse hashed password")
	}

	if hp.hashNum < MinPasswordHashIteration {
		glog.Shared.Debug("verify legacy password hash with weak iteration count",
			zap.Int("hash_num", hp.hashNum),
			zap.Int("min_required", MinPasswordHashIteration))
	}

	rawH, err := newHashedPasswordWithMinIteration(
		hp.salt,
		rawpassword,
		hp.hasher,
		hp.hashNum,
		legacyMinPasswordHashIteration,
	)
	if err != nil {
		return errors.Wrap(err, "build hashed password by raw password")
	}

	if subtle.ConstantTimeCompare(hp.hashedPassword, rawH.hashedPassword) != 1 {
		return errors.Errorf("password not match")
	}

	return nil
}

// PasswordHash generate salted hash of password, can verify by VerifyHashedPassword
func PasswordHash(password []byte, hasher gutils.HashType) (hashedPassword string, err error) {
	if len(password) == 0 {
		return "", errors.Errorf("password is empty")
	}

	defer gutils.NewDelay(DefaultPasswordDelay).Wait()

	var salt []byte
	switch hasher {
	case gutils.HashTypeSha256:
		if salt, err = Salt(256); err != nil {
			return "", errors.Wrap(err, "generate salt")
		}
	case gutils.HashTypeSha512:
		if salt, err = Salt(512); err != nil {
			return "", errors.Wrap(err, "generate salt")
		}
	default:
		return "", errors.Errorf("only supprt sha256,sha512")
	}

	n, err := rand.Int(rand.Reader, big.NewInt(MaxPasswordHashIteration-MinPasswordHashIteration))
	if err != nil {
		return "", errors.Wrap(err, "generate hash count")
	}
	hashNum := int(n.Int64()) + MinPasswordHashIteration

	h, err := newHashedPassword(salt, password, hasher, hashNum)
	if err != nil {
		return "", errors.Wrap(err, "hashing password")
	}

	return h.String(), nil
}

// GeneratePasswordHash generate hashed password by origin password
//
// Deprecated: use PasswordHash instead
func GeneratePasswordHash(password []byte) ([]byte, error) {
	return bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
}

// ValidatePasswordHash validate password is match with hashedPassword
//
// Deprecated: use VerifyHashedPassword instead
func ValidatePasswordHash(hashedPassword, password []byte) bool {
	return bcrypt.CompareHashAndPassword(hashedPassword, password) == nil
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

var (
	// RSAEncrypt encrypt by RSAEncryptByPKCS1v15, for compatibility
	//
	// Deprecated: use RSAEncryptByPKCS1v15 or RsaEncryptByOAEP instead
	RSAEncrypt = RSAEncryptByPKCS1v15
	// RSADecrypt decrypt by RSADecryptByPKCS1v15, for compatibility
	//
	// Deprecated: use RSADecryptByPKCS1v15 or RSADecryptByOAEP instead
	RSADecrypt = RSADecryptByPKCS1v15
)

// RSAEncryptByPKCS1v15 encrypt by PKCS1v15
//
// This is not a deterministic encryption scheme,
// it will return different ciphertexts each time
// even if the same plaintext is encrypted multiple times.
func RSAEncryptByPKCS1v15(pubkey *rsa.PublicKey, plain []byte) (cipher []byte, err error) {
	chunk := make([]byte, pubkey.Size()-11) // will padding 11 bytes
	reader := bytes.NewReader(plain)
	for {
		n, err := reader.Read(chunk)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, errors.Wrap(err, "read chunk")
		}

		cipherChunk, err := rsa.EncryptPKCS1v15(rand.Reader, pubkey, chunk[:n])
		if err != nil {
			return nil, errors.Wrap(err, "encrypt chunk")
		}

		cipher = append(cipher, cipherChunk...)
	}

	return cipher, nil
}

// RSADecryptByPKCS1v15 decrypt by rsa PKCS1v15
//
// only accept cipher encrypted by RSAEncrypt
func RSADecryptByPKCS1v15(prikey *rsa.PrivateKey, cipher []byte) (plain []byte, err error) {
	chunk := make([]byte, prikey.Size())
	reader := bytes.NewReader(cipher)
	for {
		n, err := reader.Read(chunk)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, errors.Wrap(err, "read chunk")
		}

		plainChunk, err := rsa.DecryptPKCS1v15(rand.Reader, prikey, chunk[:n])
		if err != nil {
			return nil, errors.Wrap(err, "decrypt chunk")
		}

		plain = append(plain, plainChunk...)
	}

	return plain, nil
}

// RSAEncryptByOAEP encrypts by OAEP with SHA256
//
// This is not a deterministic encryption scheme,
// it will return different ciphertexts each time
// even if the same plaintext is encrypted multiple times.
func RSAEncryptByOAEP(pubkey *rsa.PublicKey, plain []byte) (cipher []byte, err error) {
	chunk := make([]byte, pubkey.Size()-2*sha256.Size-2)
	reader := bytes.NewReader(plain)
	for {
		n, err := reader.Read(chunk)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, errors.Wrap(err, "read chunk")
		}

		cipherChunk, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, pubkey, chunk[:n], nil)
		if err != nil {
			return nil, errors.Wrap(err, "encrypt chunk")
		}

		cipher = append(cipher, cipherChunk...)
	}

	return cipher, nil
}

// RSADecryptByOAEP decrypt by OAEP with SHA256
func RSADecryptByOAEP(prikey *rsa.PrivateKey, cipher []byte) (plain []byte, err error) {
	chunk := make([]byte, prikey.Size())
	reader := bytes.NewReader(cipher)
	for {
		n, err := reader.Read(chunk)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, errors.Wrap(err, "read chunk")
		}

		plainChunk, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, prikey, chunk[:n], nil)
		if err != nil {
			return nil, errors.Wrap(err, "decrypt chunk")
		}

		plain = append(plain, plainChunk...)
	}

	return plain, nil
}
