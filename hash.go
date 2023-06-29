package utils

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"hash"
	"io"
	"os"

	"github.com/Laisky/errors/v2"
	"github.com/cespare/xxhash"

	"github.com/Laisky/go-utils/v4/log"
)

// HashSHA128String calculate string's hash by sha256
//
// Deprecated: use Hash instead
func HashSHA128String(val string) string {
	b := sha1.Sum([]byte(val))
	return hex.EncodeToString(b[:])
}

// HashSHA256String calculate string's hash by sha256
//
// Deprecated: use Hash instead
func HashSHA256String(val string) string {
	b := sha256.Sum256([]byte(val))
	return hex.EncodeToString(b[:])
}

// HashXxhashString calculate string's hash by sha256
//
// Deprecated: use Hash instead
func HashXxhashString(val string) string {
	b := xxhash.New().Sum([]byte(val))
	return hex.EncodeToString(b)
}

// HashTypeInterface hashs
type HashTypeInterface interface {
	String() string
	Hasher() (hash.Hash, error)
}

// HashType hashs
type HashType string

// String name of hash
func (h HashType) String() string {
	return string(h)
}

// Hasher new hasher by hash type
func (h HashType) Hasher() (hash.Hash, error) {
	switch h {
	case HashTypeMD5:
		log.Shared.Warn("md5 is not safe or fast, use sha256 instead")
		return md5.New(), nil
	case HashTypeSha1:
		return sha1.New(), nil
	case HashTypeSha256:
		return sha256.New(), nil
	case HashTypeSha512:
		return sha512.New(), nil
	case HashTypeXxhash:
		return xxhash.New(), nil
	}

	return nil, errors.Errorf("unknon hasher %q", h.String())
}

const (
	// HashTypeMD5 MD5
	HashTypeMD5 HashType = "md5"
	// HashTypeSha1 Sha1
	HashTypeSha1 HashType = "sha1"
	// HashTypeSha256 Sha256
	HashTypeSha256 HashType = "sha256"
	// HashTypeSha512 Sha512
	HashTypeSha512 HashType = "sha512"
	// HashTypeXxhash Xxhash
	HashTypeXxhash HashType = "xxhash"
)

// Hash generate signature by hash
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

// FileHash generate file signature by hash
func FileHash(hashType HashTypeInterface, filepath string) (signature []byte, err error) {
	fp, err := os.Open(filepath)
	if err != nil {
		return nil, errors.Wrap(err, "open file")
	}
	defer fp.Close() // nolint: errcheck

	return Hash(hashType, fp)
}

// HashVerify verify by hash
func HashVerify(hashType HashTypeInterface, content io.Reader, signature []byte) (err error) {
	hasher, err := hashType.Hasher()
	if err != nil {
		return errors.Wrap(err, "get hasher")
	}

	if _, err = io.Copy(hasher, content); err != nil {
		return errors.Wrap(err, "read from content")
	}

	if !bytes.Equal(hasher.Sum(nil), signature) {
		return errors.Errorf("signature not match")
	}

	return nil
}
