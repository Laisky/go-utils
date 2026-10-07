package utils

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"hash"
	"io"
	"sync/atomic"

	"github.com/Laisky/errors/v2"
	"github.com/cespare/xxhash"

	"github.com/Laisky/go-utils/v6/log"
)

// HashSHA128String calculates the SHA-1 digest of val and returns it as a lowercase hex string.
//
// Despite its name, this function uses SHA-1 (a 160-bit digest), not SHA-256; SHA-1 is not
// collision resistant and must not be used for security decisions.
//
// Deprecated: use Hash instead
func HashSHA128String(val string) string {
	b := sha1.Sum([]byte(val))
	return hex.EncodeToString(b[:])
}

// HashSHA256String calculates the SHA-256 digest of val and returns it as a lowercase hex string.
//
// Deprecated: use Hash instead
func HashSHA256String(val string) string {
	b := sha256.Sum256([]byte(val))
	return hex.EncodeToString(b[:])
}

// HashXxhashString calculates the non-cryptographic xxhash (64-bit) digest of val and returns it
// as a lowercase hex string.
//
// Deprecated: use Hash instead
func HashXxhashString(val string) string {
	h := xxhash.New()
	_, _ = h.Write([]byte(val))
	return hex.EncodeToString(h.Sum(nil))
}

// HashTypeInterface selects a hash algorithm: String returns the algorithm name and Hasher
// returns a fresh hash state for it, or an error if the algorithm is unsupported.
type HashTypeInterface interface {
	String() string
	Hasher() (hash.Hash, error)
}

// HashType is the string name of a built-in hash algorithm such as HashTypeSha256.
type HashType string

// String returns the algorithm name stored in h.
func (h HashType) String() string {
	return string(h)
}

// Weak-algorithm diagnostics have a fixed per-algorithm, per-process budget.
// Claim the budget before logging: a logging hook can itself construct a hasher,
// so holding a sync.Once/mutex across the callback would permit a deadlock.
var (
	md5WarningLogged  atomic.Bool
	sha1WarningLogged atomic.Bool
)

// Hasher returns a fresh hash state for the selected algorithm.
// MD5 and SHA1 remain available for compatibility. Each emits at most one warning
// per process, independent of call count, work factors, and concurrent callers.
// These diagnostics do not make weak algorithms suitable for password storage.
func (h HashType) Hasher() (hash.Hash, error) {
	switch h {
	case HashTypeMD5:
		if md5WarningLogged.CompareAndSwap(false, true) {
			log.Shared.Warn("md5 is not safe for cryptographic use; legacy compatibility only (once per process)")
		}
		return md5.New(), nil
	case HashTypeSha1:
		if sha1WarningLogged.CompareAndSwap(false, true) {
			log.Shared.Warn("sha1 is not safe for cryptographic use; legacy compatibility only (once per process)")
		}
		return sha1.New(), nil
	case HashTypeSha256:
		return sha256.New(), nil
	case HashTypeSha512:
		return sha512.New(), nil
	case HashTypeXxhash:
		return xxhash.New(), nil
	default:
		return nil, errors.Errorf("unknon hasher %q", h.String())
	}
}

const (
	// HashTypeMD5 MD5
	HashTypeMD5 HashType = "md5"
	// HashTypeSha1 Sha1
	HashTypeSha1 HashType = "sha1"
	// HashTypeSha256 Sha256
	HashTypeSha256 HashType = "sha256"
	// HashTypeSha384 Sha384
	// HashTypeSha384 HashType = "sha384"
	// HashTypeSha512 Sha512
	HashTypeSha512 HashType = "sha512"
	// HashTypeXxhash Xxhash
	HashTypeXxhash HashType = "xxhash"

	// Added in go1.24
	// HashTypeSha3With256 Sha3With256
	// HashTypeSha3With256 HashType = "sha3-256"
	// HashTypeSha3With384 Sha3With384
	// HashTypeSha3With384 HashType = "sha3-384"
	// HashTypeSha3With512 Sha3With512
	// HashTypeSha3With512 HashType = "sha3-512"
)

// Hash computes the digest of content with the algorithm selected by hashType.
// It reads content until EOF and returns the raw digest bytes, or an error if the
// hasher is unknown or reading content fails.
func Hash(hashType HashTypeInterface, content io.Reader) (signature []byte, err error) {
	hasher, err := hashType.Hasher()
	if err != nil {
		return nil, errors.Wrap(err, "get hasher")
	}

	if _, err = io.Copy(hasher, content); err != nil {
		return nil, errors.Wrap(err, "read from content")
	}

	return hasher.Sum(nil), nil
}

// HashVerify computes the digest of content with the algorithm selected by hashType and
// compares it with signature in constant time. It returns nil when they match, or an error
// if the hasher is unknown, reading content fails, or the digests differ.
func HashVerify(hashType HashTypeInterface, content io.Reader, signature []byte) (err error) {
	hasher, err := hashType.Hasher()
	if err != nil {
		return errors.Wrap(err, "get hasher")
	}

	if _, err = io.Copy(hasher, content); err != nil {
		return errors.Wrap(err, "read from content")
	}

	if subtle.ConstantTimeCompare(hasher.Sum(nil), signature) != 1 {
		return errors.Errorf("signature not match")
	}

	return nil
}
