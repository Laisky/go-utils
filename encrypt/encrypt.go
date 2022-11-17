// Package encrypt contains some useful tools to deal with encryption/decryption
package encrypt

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"io"
	"math/big"

	"github.com/Laisky/errors"
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
