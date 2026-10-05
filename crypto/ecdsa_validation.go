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
	if !found || len(left) == 0 || len(right) == 0 || len(left) > limit || len(right) > limit || strings.Contains(right, ecdsaSignDelimiter) {
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
			c := raw[i]
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
				(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '=') {
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
			c := raw[i]
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
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
	if key == nil || key.Curve == nil || key.X == nil || key.Y == nil || r == nil || s == nil {
		return false
	}
	switch key.Curve {
	case elliptic.P224(), elliptic.P256(), elliptic.P384(), elliptic.P521():
	default:
		return false
	}
	order := key.Curve.Params().N
	return r.Sign() > 0 && s.Sign() > 0 && r.Cmp(order) < 0 && s.Cmp(order) < 0 && key.Curve.IsOnCurve(key.X, key.Y)
}
