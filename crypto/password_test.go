package crypto

import (
	"fmt"
	"testing"

	"github.com/Laisky/zap"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/log"
)

// TestPassword verifies that a hash produced by GeneratePasswordHash validates with
// ValidatePasswordHash for the original password and is rejected for a different password.
func TestPassword(t *testing.T) {
	t.Parallel()

	password := []byte("1234567890")
	hp, err := GeneratePasswordHash(password)
	require.NoError(t, err)

	t.Logf("got hashed password: %v", string(hp))

	require.True(t, ValidatePasswordHash(hp, password))
	require.False(t, ValidatePasswordHash(hp, []byte("dj23fij2f32")))
}

// ExampleGeneratePasswordHash demonstrates hashing a raw password with GeneratePasswordHash and
// checking the password against the stored hash with ValidatePasswordHash.
func ExampleGeneratePasswordHash() {
	// generate hashed password
	rawPassword := []byte("1234567890")
	hashedPassword, err := GeneratePasswordHash(rawPassword)
	if err != nil {
		log.Shared.Error("try to generate password got error", zap.Error(err))
		return
	}
	fmt.Printf("got new hashed pasword: %v\n", string(hashedPassword))

	// validate passowrd
	if !ValidatePasswordHash(hashedPassword, rawPassword) {
		log.Shared.Error("password invalidate", zap.Error(err))
		return
	}
}

// BenchmarkGeneratePasswordHash measures GeneratePasswordHash for a fixed password, and
// ValidatePasswordHash both against that password's hash and against the hash of a different
// password.
//
// goos: linux
// goarch: amd64
// pkg: github.com/Laisky/go-utils/v6/crypto
// cpu: Intel(R) Xeon(R) Gold 5320 CPU @ 2.20GHz
// BenchmarkGeneratePasswordHash
// BenchmarkGeneratePasswordHash/generate
// BenchmarkGeneratePasswordHash/generate-104         	       1	1256584728 ns/op	   19120 B/op	      16 allocs/op
// BenchmarkGeneratePasswordHash/validate
// BenchmarkGeneratePasswordHash/validate-104         	       1	1255534569 ns/op	   19216 B/op	      18 allocs/op
// BenchmarkGeneratePasswordHash/invalidate
// BenchmarkGeneratePasswordHash/invalidate-104       	       1	1253798232 ns/op	   19216 B/op	      18 allocs/op
func BenchmarkGeneratePasswordHash(b *testing.B) {
	pw := []byte("28jijf23f92of92o3jf23fjo2")
	ph, err := GeneratePasswordHash(pw)
	require.NoError(b, err)

	phw, err := GeneratePasswordHash([]byte("j23foj9foj29fj23fj"))
	require.NoError(b, err)

	b.Run("generate", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err = GeneratePasswordHash(pw)
			require.NoError(b, err)
		}
	})
	b.Run("validate", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			require.True(b, ValidatePasswordHash(ph, pw))
		}
	})
	b.Run("invalidate", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			require.False(b, ValidatePasswordHash(phw, pw))
		}
	})
}
