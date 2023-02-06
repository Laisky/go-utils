package encrypt

import (
	"crypto/rand"
	"crypto/sha256"
	"io"

	"github.com/Laisky/errors"
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
//	HKDFWithSHA256([]byte("your key"), nil, nil, results)
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
	salt := make([]byte, length)
	_, err := rand.Read(salt)
	if err != nil {
		return nil, errors.Wrap(err, "generate salt")
	}

	return salt, nil
}

// DeriveKey derive key by hkdf
//
// Deprecated: use DeriveKeyByHKDF instead
func DeriveKey(rawKey []byte, newKeyLength int) (newKey []byte, err error) {
	results := make([][]byte, 1)
	results[0] = make([]byte, newKeyLength)
	if err := HKDFWithSHA256(rawKey, nil, nil, results); err != nil {
		return nil, errors.Wrap(err, "derivative key by hkdf")
	}

	return results[0], nil
}

func DeriveKeyByHKDF(rawKey, salt []byte, newKeyLength int) (newKey []byte, err error) {
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
	return scrypt.Key(rawKey, salt, 32768, 16, 1, 32)
}
