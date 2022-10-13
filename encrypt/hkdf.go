package encrypt

import (
	"crypto/sha256"
	"io"

	"golang.org/x/crypto/hkdf"
)

// HKDFWithSHA256 derivate keys by HKDF with sha256.
// same key & salt will derivate same keys
//
// # Example
//
// derivate multiple keys:
//
//	results := make([][]byte, 10)
//	for i := range results {
//	    results[i] = make([]byte, 20)
//	}
//	HKDFWithSHA256([]byte("your key"), nil, nil, results)
func HKDFWithSHA256(secret, salt, info []byte, results [][]byte) {
	h := hkdf.New(sha256.New, secret, salt, info)
	for i := range results {
		io.ReadFull(h, results[i])
	}

	return
}
