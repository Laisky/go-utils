package encrypt

import (
	"crypto/sha256"
	"io"

	"github.com/pkg/errors"
	"golang.org/x/crypto/hkdf"
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
