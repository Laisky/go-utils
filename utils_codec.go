package utils

import (
	"bytes"
	"crypto/md5"
	"encoding/asn1"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/go-utils/v6/json"
)

// MD5JSON calculate md5(jsonify(data))
func MD5JSON(data any) (string, error) {
	if NilInterface(data) {
		return "", errors.New("data is nil")
	}

	b, err := json.Marshal(data)
	if err != nil {
		return "", errors.Wrap(err, "marshal data to json")
	}

	return fmt.Sprintf("%x", md5.Sum(b)), nil
}

// EncodeByBase64 encode bytes to string by base64
func EncodeByBase64(raw []byte) string {
	return base64.URLEncoding.EncodeToString(raw)
}

// DecodeByBase64 decode string to bytes by base64
func DecodeByBase64(encoded string) ([]byte, error) {
	return base64.URLEncoding.DecodeString(encoded)
}

var (
	// EncodeByHex encode bytes to string by hex
	EncodeByHex = hex.EncodeToString
	// DecodeByHex decode string to bytes by hex
	DecodeByHex = hex.DecodeString
)

// RegexpOidFormat check if oid is valid
var RegexpOidFormat = regexp.MustCompile(`^\d(?:\.\d+){0,}$`)

// ParseObjectIdentifier parse oid
func ParseObjectIdentifier(val string) (oid asn1.ObjectIdentifier, err error) {
	if !RegexpOidFormat.MatchString(val) {
		return nil, errors.Errorf("invalid oid format: %q", val)
	}

	vals := strings.Split(val, ".")
	oid = make(asn1.ObjectIdentifier, len(vals))
	for i, v := range vals {
		oid[i], err = strconv.Atoi(v)
		if err != nil {
			return nil, errors.Wrapf(err, "parse oid %q", val)
		}
	}

	return oid, nil
}

// NewHasPrefixWithMagic create a func to check if s has prefix
//
// if the length of prefix is quite short, it will use magic number to check.
func NewHasPrefixWithMagic(prefix []byte) func(s []byte) bool {
	switch l := len(prefix); l {
	case 8:
		prefixMagicNumber := binary.NativeEndian.Uint64(prefix)
		return func(s []byte) bool {
			return len(s) >= l && binary.NativeEndian.Uint64(s[:8]) == prefixMagicNumber
		}
	case 4:
		prefixMagicNumber := binary.NativeEndian.Uint32(prefix)
		return func(s []byte) bool {
			return len(s) >= l && binary.NativeEndian.Uint32(s[:4]) == prefixMagicNumber
		}
	case 2:
		prefixMagicNumber := binary.NativeEndian.Uint16(prefix)
		return func(s []byte) bool {
			return len(s) >= l && binary.NativeEndian.Uint16(s[:2]) == prefixMagicNumber
		}
	case 0:
		return func(_ []byte) bool {
			return true
		}
	default:
		return func(s []byte) bool {
			return bytes.HasPrefix(s, prefix)
		}
	}
}
