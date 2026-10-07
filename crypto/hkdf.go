package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"io"

	"github.com/Laisky/errors/v2"
	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/scrypt"
)

// HKDFWithSHA256 derivative keys by HKDF with sha256.
// same key & salt will derivative same keys
//
// # Example
//
// derivative multiple keys:
//
//	results := make([][]byte, 10)
//	for i := range results {
//	    results[i] = make([]byte, 20)
//	}
//
//	HKDFWithSHA256([]byte("your key"), []byte("salt"), []byte("info"), results)
func HKDFWithSHA256(secret, salt, info []byte, results [][]byte) error {
	h := hkdf.New(sha256.New, secret, salt, info)
	for i := range results {
		if _, err := io.ReadFull(h, results[i]); err != nil {
			return errors.Wrap(err, "read from hkdf reader")
		}
	}

	return nil
}

// Salt generate random salt with specifiec length
func Salt(length int) ([]byte, error) {
	if length <= 0 {
		return nil, errors.Errorf("salt length must be positive, got %d", length)
	}

	salt := make([]byte, length)
	_, err := rand.Read(salt)
	if err != nil {
		return nil, errors.Wrap(err, "generate salt")
	}

	return salt, nil
}

// maxHKDFSHA256KeyLength is the RFC 5869 output limit of HKDF-SHA256, 255 hash blocks.
const maxHKDFSHA256KeyLength = 255 * sha256.Size

// DeriveKeyByHKDF derives a newKeyLength-byte key from rawKey and salt with
// HKDF-SHA256. newKeyLength must be between 1 and 8160 (255*32) bytes, the RFC
// 5869 limit; it is checked before any allocation. It returns the derived key,
// or an error for an out-of-range length or a failed derivation.
func DeriveKeyByHKDF(rawKey, salt []byte, newKeyLength int) (newKey []byte, err error) {
	if newKeyLength < 1 || newKeyLength > maxHKDFSHA256KeyLength {
		return nil, errors.Errorf("hkdf key length must be between 1 and %d bytes, got %d",
			maxHKDFSHA256KeyLength, newKeyLength)
	}

	results := make([][]byte, 1)
	results[0] = make([]byte, newKeyLength)
	if err := HKDFWithSHA256(rawKey, salt, nil, results); err != nil {
		return nil, errors.Wrap(err, "derivative key by hkdf")
	}

	return results[0], nil
}

// DeriveKeyBySMHF derive key by Stronger Key Derivation via Sequential Memory-Hard Functions
//
// https://pkg.go.dev/golang.org/x/crypto@v0.5.0/scrypt
func DeriveKeyBySMHF(rawKey, salt []byte) (newKey []byte, err error) {
	if newKey, err = scrypt.Key(rawKey, salt, 32768, 16, 1, 32); err != nil {
		return nil, errors.Wrap(err, "derive key by scrypt")
	}

	return newKey, nil
}
