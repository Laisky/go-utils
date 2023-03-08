// package crypto contains some useful tools to deal with encryption/decryption
package crypto

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"golang.org/x/crypto/bcrypt"

	gutils "github.com/Laisky/go-utils/v4"
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
	h.salt = salt
	h.hasher = hasher
	h.hashNum = hashNum

	h.hashedPassword = append(rawpassword, h.salt...)
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

const defaultPasswordDelay = 2 * time.Second

// VerifyHashedPassword verify HashedPassword
func VerifyHashedPassword(rawpassword []byte, hashedPassword string) (err error) {
	if len(rawpassword) == 0 || len(hashedPassword) == 0 {
		return errors.Errorf("rawpassword or hashedPassword is empty")
	}

	defer gutils.NewDelay(defaultPasswordDelay).Wait()
	hp, err := parseHashedPassword(hashedPassword)
	if err != nil {
		return errors.Wrap(err, "parse hashed password")
	}

	rawH, err := newHashedPassword(hp.salt, rawpassword, hp.hasher, hp.hashNum)
	if err != nil {
		return errors.Wrap(err, "build hashed password by raw password")
	}

	if !bytes.Equal(hp.hashedPassword, rawH.hashedPassword) {
		return errors.Errorf("password not match")
	}

	return nil
}

// PasswordHash generate salted hash of password, can verify by VerifyHashedPassword
func PasswordHash(password []byte, hasher gutils.HashType) (hashedPassword string, err error) {
	if len(password) == 0 {
		return "", errors.Errorf("password is empty")
	}

	defer gutils.NewDelay(defaultPasswordDelay).Wait()

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

	n, err := rand.Int(rand.Reader, big.NewInt(10))
	if err != nil {
		return "", errors.Wrap(err, "generate hash count")
	}
	hashNum := int(n.Int64()) + 1

	h, err := newHashedPassword(salt, password, hasher, hashNum)
	if err != nil {
		return "", errors.Wrap(err, "hashing password")
	}

	return h.String(), nil
}

// GeneratePasswordHash generate hashed password by origin password
func GeneratePasswordHash(password []byte) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
}

// ValidatePasswordHash validate password is match with hashedPassword
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

// RSAEncrypt encrypt by PKCS1v15
//
// canbe decrypt by RSADecrypt
func RSAEncrypt(pubkey *rsa.PublicKey, plain []byte) (cipher []byte, err error) {
	chunk := make([]byte, pubkey.Size()-11) // will padding 11 bytes
	reader := bytes.NewReader(plain)
	for {
		n, err := reader.Read(chunk)
		if err != nil {
			if err == io.EOF {
				break
			}

			return nil, errors.Wrap(err, "read chunk")
		}

		cipherChunk, err := rsa.EncryptPKCS1v15(rand.Reader, pubkey, chunk[:n])
		if err != nil {
			return nil, errors.Wrap(err, "encrypt chunkd")
		}

		cipher = append(cipher, cipherChunk...)
	}

	return cipher, nil
}

// RSADecrypt decrypt by rsa PKCS1v15
//
// only accept cipher encrypted by RSAEncrypt
func RSADecrypt(prikey *rsa.PrivateKey, cipher []byte) (plain []byte, err error) {
	chunk := make([]byte, prikey.Size())
	reader := bytes.NewReader(cipher)
	for {
		n, err := reader.Read(chunk)
		if err != nil {
			if err == io.EOF {
				break
			}

			return nil, errors.Wrap(err, "read chunk")
		}

		plainChunk, err := rsa.DecryptPKCS1v15(rand.Reader, prikey, chunk[:n])
		if err != nil {
			return nil, errors.Wrap(err, "encrypt chunkd")
		}

		plain = append(plain, plainChunk...)
	}

	return plain, nil
}
