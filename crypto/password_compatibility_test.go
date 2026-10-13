package crypto

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	gutils "github.com/Laisky/go-utils/v6"
)

// TestPasswordCompatibilityWriter verifies both requested algorithms with an independent pre-Argon2 reader.
func TestPasswordCompatibilityWriter(t *testing.T) {
	for _, hasher := range []gutils.HashType{gutils.HashTypeSha256, gutils.HashTypeSha512} {
		t.Run(hasher.String(), func(t *testing.T) {
			t.Parallel()
			password := []byte("disposable compatibility password")
			encoded, err := PasswordHash(password, hasher)
			require.NoError(t, err)
			fields := strings.Split(encoded, ".")
			require.Len(t, fields, 4, "old readers require the requested digest record, not PHC")
			require.Equal(t, hasher.String(), fields[0])
			count, err := strconv.Atoi(fields[1])
			require.NoError(t, err)
			require.GreaterOrEqual(t, count, MinPasswordHashIteration)
			require.Less(t, count, MaxPasswordHashIteration)
			salt, err := hex.DecodeString(fields[2])
			require.NoError(t, err)
			expectedSaltSize := 256
			if hasher == gutils.HashTypeSha512 {
				expectedSaltSize = 512
			}
			require.Len(t, salt, expectedSaltSize)
			value := append(append([]byte{}, password...), salt...)
			for range count {
				if hasher == gutils.HashTypeSha256 {
					digest := sha256.Sum256(value)
					value = digest[:]
				} else {
					digest := sha512.Sum512(value)
					value = digest[:]
				}
			}
			require.Equal(t, hex.EncodeToString(value), fields[3], "new writers must remain readable by old instances")
			require.NoError(t, VerifyHashedPassword(password, encoded))
		})
	}
}
