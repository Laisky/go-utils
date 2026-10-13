package crypto

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"hash"
	"strconv"
	"strings"
	"testing"

	gutils "github.com/Laisky/go-utils/v6"
	"github.com/stretchr/testify/require"
)

// TestPasswordCompatibilityWriter verifies the selected legacy algorithm, salt and iteration
// contract with an independent historical-format decoder rather than a new-to-new round trip.
func TestPasswordCompatibilityWriter(t *testing.T) {
	for _, algorithm := range []gutils.HashType{gutils.HashTypeSha256, gutils.HashTypeSha512} {
		t.Run(algorithm.String(), func(t *testing.T) {
			t.Parallel()
			password := []byte("disposable compatibility password")
			record, err := PasswordHash(password, algorithm)
			require.NoError(t, err)
			parts := strings.Split(record, ".")
			require.Len(t, parts, 4, "old readers require the historical four-part layout")
			require.Equal(t, algorithm.String(), parts[0])
			iterations, err := strconv.Atoi(parts[1])
			require.NoError(t, err)
			require.GreaterOrEqual(t, iterations, MinPasswordHashIteration)
			require.Less(t, iterations, MaxPasswordHashIteration)
			salt, err := hex.DecodeString(parts[2])
			require.NoError(t, err)
			expected, err := hex.DecodeString(parts[3])
			require.NoError(t, err)
			var h hash.Hash = sha256.New()
			saltSize := 256
			if algorithm == gutils.HashTypeSha512 {
				h = sha512.New()
				saltSize = 512
			}
			require.Len(t, salt, saltSize)
			actual := append(append([]byte(nil), password...), salt...)
			for range iterations {
				h.Reset()
				_, err := h.Write(actual)
				require.NoError(t, err)
				actual = h.Sum(nil)
			}
			require.Equal(t, expected, actual, "the pre-Argon2 reader must verify a new write")
			require.NoError(t, VerifyHashedPassword(password, record))
		})
	}
}

// TestPasswordHashIterationCount retains the historical public writer's iteration-range assertion.
func TestPasswordHashIterationCount(t *testing.T) {
	t.Parallel()
	record, err := PasswordHash([]byte("password"), gutils.HashTypeSha256)
	require.NoError(t, err)
	parsed, err := parseHashedPassword(record)
	require.NoError(t, err)
	require.GreaterOrEqual(t, parsed.hashNum, MinPasswordHashIteration)
	require.Less(t, parsed.hashNum, MaxPasswordHashIteration)
}
