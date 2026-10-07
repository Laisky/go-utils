package crypto

import (
	"crypto/sha256"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDeriveKeyByHKDF_RejectsOutOfRangeLength verifies that DeriveKeyByHKDF
// validates newKeyLength before allocating: a negative length used to panic in
// make, a zero length silently returned an empty key, and an oversized length
// allocated the whole buffer before HKDF refused it. Lengths from 1 to
// 255*sha256.Size bytes, the RFC 5869 limit for HKDF-SHA256, keep working.
// Regression found during the documentation-pass error-wrapping sweep.
func TestDeriveKeyByHKDF_RejectsOutOfRangeLength(t *testing.T) {
	t.Parallel()

	secret := []byte("raw-key-material")
	salt := []byte("salt")
	for _, length := range []int{-1, 0, 255*sha256.Size + 1, math.MaxInt} {
		require.NotPanics(t, func() {
			key, err := DeriveKeyByHKDF(secret, salt, length)
			require.ErrorContains(t, err, "key length")
			require.Nil(t, key)
		}, "length %d", length)
	}

	for _, length := range []int{1, 32, 255 * sha256.Size} {
		key, err := DeriveKeyByHKDF(secret, salt, length)
		require.NoError(t, err)
		require.Len(t, key, length)
	}
}
