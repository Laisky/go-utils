package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"math/big"
	"strings"

	"github.com/Laisky/errors/v2"
)

const (
	// Historical ES256-named codecs are also used with Go's other NIST curves.
	maxECDSASignatureBits   = 521
	maxECDSAHexComponent    = 132
	maxECDSABase64Component = 88
)

// decodeBoundedECDSASignature decodes two positive NIST-curve-sized components.
// Length checks precede splitting, Base64 allocation, and big-integer parsing.
// Public-key-specific order checks remain the verifier's responsibility.
func decodeBoundedECDSASignature(signature string, useBase64 bool) (*big.Int, *big.Int, error) {
	limit := maxECDSAHexComponent
	if useBase64 {
		limit = maxECDSABase64Component
	}
	if len(signature) < 3 || len(signature) > 2*limit+1 {
		return nil, nil, errors.New("invalid ECDSA signature length")
	}
	left, right, found := strings.Cut(signature, ecdsaSignDelimiter)
	if !found || len(left) == 0 || len(right) == 0 || len(left) > limit || len(right) > limit ||
		strings.Contains(right, ecdsaSignDelimiter) {
		return nil, nil, errors.New("invalid ECDSA signature structure")
	}
	r, err := decodeECDSAComponent(left, useBase64)
	if err != nil {
		return nil, nil, errors.Wrap(err, "invalid ECDSA r component")
	}
	s, err := decodeECDSAComponent(right, useBase64)
	if err != nil {
		return nil, nil, errors.Wrap(err, "invalid ECDSA s component")
	}
	return r, s, nil
}

// decodeECDSAComponent parses an already length-bounded, strictly encoded integer.
func decodeECDSAComponent(raw string, useBase64 bool) (*big.Int, error) {
	value := new(big.Int)
	if useBase64 {
		// Go's decoder otherwise ignores newlines, even in Strict mode.
		for i := range len(raw) {
			if !isBase64URLByte(raw[i]) {
				return nil, errors.New("invalid Base64 alphabet")
			}
		}
		decoded, err := base64.URLEncoding.Strict().DecodeString(raw)
		if err != nil {
			return nil, errors.New("invalid Base64 encoding")
		}
		value.SetBytes(decoded)
	} else {
		for i := range len(raw) {
			if !isHexDigitByte(raw[i]) {
				return nil, errors.New("invalid hexadecimal encoding")
			}
		}
		if _, ok := value.SetString(raw, 16); !ok {
			return nil, errors.New("invalid hexadecimal component")
		}
	}
	if value.Sign() <= 0 || value.BitLen() > maxECDSASignatureBits {
		return nil, errors.New("ECDSA component outside supported range")
	}
	return value, nil
}

// validECDSAVerificationInputs checks a NIST public key and 0 < r,s < its order.
// Custom curves are unsupported; rejecting them avoids calling arbitrary curve
// implementations on lower-trust key coordinates during validation.
func validECDSAVerificationInputs(key *ecdsa.PublicKey, r, s *big.Int) bool {
	if key == nil || key.Curve == nil || r == nil || s == nil {
		return false
	}
	// Only nil-ness is read here; the coordinates are never modified or used for
	// arithmetic. PublicKey.Bytes below dereferences them, so nil must be rejected.
	if key.X == nil || key.Y == nil { //nolint:staticcheck // SA1019: read-only nil guard, see above.
		return false
	}
	switch key.Curve {
	case elliptic.P224(), elliptic.P256(), elliptic.P384(), elliptic.P521():
	default:
		return false
	}
	order := key.Curve.Params().N
	return r.Sign() > 0 && s.Sign() > 0 && r.Cmp(order) < 0 && s.Cmp(order) < 0 && ecdsaPublicKeyOnCurve(key)
}

// ecdsaPublicKeyOnCurve reports whether key holds a valid point on its NIST
// curve. It takes a key whose curve is P-224, P-256, P-384 or P-521 and returns
// false for negative, oversized or out-of-field coordinates, off-curve points
// and the point at infinity. PublicKey.Bytes (Go 1.25+) applies the same
// SEC 1 point validation as crypto/ecdh, replacing the deprecated
// elliptic.Curve.IsOnCurve with identical acceptance for these curves.
func ecdsaPublicKeyOnCurve(key *ecdsa.PublicKey) bool {
	_, err := key.Bytes()
	return err == nil
}

// isBase64URLByte reports whether c belongs to the padded URL-safe Base64
// alphabet (RFC 4648 section 5). It takes one byte and returns true for A-Z,
// a-z, 0-9, '-', '_' and the '=' padding character.
func isBase64URLByte(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' ||
		c == '-' || c == '_' || c == '='
}

// isHexDigitByte reports whether c is an ASCII hexadecimal digit in either
// letter case. It takes one byte and returns true only for 0-9, a-f and A-F.
func isHexDigitByte(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}
