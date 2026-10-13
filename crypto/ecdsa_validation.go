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

// validECDSAVerificationInputs checks a standard NIST public key, including
// equivalent curve wrappers, and the bounded signature components.
func validECDSAVerificationInputs(key *ecdsa.PublicKey, r, s *big.Int) bool {
	_, valid := canonicalECDSAVerificationKey(key, r, s)
	return valid
}

// canonicalECDSAVerificationKey returns a detached key using a trusted standard
// curve implementation. Wrapper arithmetic is never used for verification.
func canonicalECDSAVerificationKey(key *ecdsa.PublicKey, r, s *big.Int) (*ecdsa.PublicKey, bool) {
	//nolint:staticcheck // SA1019: read-only nil checks for legacy public coordinates before validation.
	if key == nil || key.Curve == nil || r == nil || s == nil || key.X == nil || key.Y == nil {
		return nil, false
	}
	curve := standardECDSACurve(key.Curve)
	if curve == nil {
		return nil, false
	}
	order := curve.Params().N
	if r.Sign() <= 0 || s.Sign() <= 0 || r.Cmp(order) >= 0 || s.Cmp(order) >= 0 {
		return nil, false
	}
	canonical := *key
	canonical.Curve = curve
	if !ecdsaPublicKeyOnCurve(&canonical) {
		return nil, false
	}
	return &canonical, true
}

// standardECDSACurve matches all mathematical parameters, rather than object
// identity or a curve name. Only Params is called on the wrapper; a broken Params
// implementation is rejected, and no wrapper arithmetic is trusted.
func standardECDSACurve(curve elliptic.Curve) (standard elliptic.Curve) {
	defer func() {
		if recover() != nil {
			standard = nil
		}
	}()
	params := curve.Params()
	if params == nil {
		return nil
	}
	for _, candidate := range []elliptic.Curve{elliptic.P224(), elliptic.P256(), elliptic.P384(), elliptic.P521()} {
		known := candidate.Params()
		if params.BitSize != known.BitSize {
			continue
		}
		match := true
		for _, pair := range [][2]*big.Int{
			{params.P, known.P}, {params.N, known.N}, {params.B, known.B},
			{params.Gx, known.Gx}, {params.Gy, known.Gy},
		} {
			if pair[0] == nil || pair[0].Cmp(pair[1]) != 0 {
				match = false
				break
			}
		}
		if match {
			return candidate
		}
	}
	return nil
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
