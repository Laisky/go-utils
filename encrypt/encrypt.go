// Package encrypt contains some useful tools to deal with encryption/decryption
package encrypt

import (
	"encoding/base64"
	"math/big"

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

// expandAesSecret expand or strip aes secret
//
// Deprecated: dangerous, do not use
// func expandAesSecret(secret []byte) []byte {
// 	var n int
// 	if len(secret) <= 16 {
// 		n = 16 - len(secret)
// 	} else if len(secret) <= 24 {
// 		n = 24 - len(secret)
// 	} else if len(secret) <= 32 {
// 		n = 32 - len(secret)
// 	} else {
// 		return secret[:32]
// 	}

// 	Logger.Debug("expand secuet", zap.Int("raw", len(secret)), zap.Int("expand", n))
// 	newSec := secret[:len(secret):len(secret)]
// 	return append(newSec, make([]byte, n)...)
// }

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
