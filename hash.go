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

	"github.com/Laisky/errors/v2"
	"github.com/cespare/xxhash"
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

type HashTypeInterface interface {
	String() string
	Hasher() (hash.Hash, error)
}

type HashType string

func (h HashType) String() string {
	return string(h)
}

func (h HashType) Hasher() (hash.Hash, error) {
	switch h {
	case HashTypeMD5:
		return md5.New(), nil
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
	HashTypeMD5    HashType = "md5"
	HashTypeSha256 HashType = "sha256"
	HashTypeSha512 HashType = "sha512"
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
