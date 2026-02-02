package crypto

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestVerifyHashedPassword_DoS(t *testing.T) {
	t.Parallel()
	t.Run("too many iterations", func(t *testing.T) {
		t.Parallel()
		// A malicious hashed password string with a very large iteration count
		// Format: {hasher}.{hashNum}.{salt}.{hashedPassword}
		maliciousHash := "sha256.10000000.00.00"

		start := time.Now()
		err := VerifyHashedPassword([]byte("password"), maliciousHash)
		duration := time.Since(start)

		t.Logf("Duration: %v", duration)
		require.Error(t, err)
		require.Contains(t, err.Error(), "too many iterations")

		// Ensure it failed because of the limit, and didn't take much longer than the default delay (2s)
		require.Less(t, duration, 3*time.Second)
	})

	t.Run("negative iterations", func(t *testing.T) {
		t.Parallel()
		maliciousHash := "sha256.-1.00.00"
		err := VerifyHashedPassword([]byte("password"), maliciousHash)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid iterations")
	})
}
