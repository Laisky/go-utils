package crypto

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHMAC(t *testing.T) {
	t.Parallel()

	for _, keyLen := range []int{
		16, 1024, 10240,
	} {
		keyLen := keyLen
		key, err := Salt(keyLen)
		require.NoError(t, err)

		t.Run(fmt.Sprintf("keyLen=%d", keyLen), func(t *testing.T) {
			t.Parallel()

			for _, plainLen := range []int{
				16, 1024, 10240,
			} {
				plainLen := plainLen
				plain, err := Salt(plainLen)
				require.NoError(t, err)

				t.Run(fmt.Sprintf("plainLen=%d", plainLen), func(t *testing.T) {
					t.Parallel()

					hmac1, err := HMACSha256(key, bytes.NewReader(plain))
					require.NoError(t, err)

					hmac2, err := HMACSha256(key, bytes.NewReader(plain))
					require.NoError(t, err)
					require.Equal(t, hmac1, hmac2)

					t.Run("incorrect plain", func(t *testing.T) {
						newplain, err := Salt(plainLen)
						require.NoError(t, err)

						hmacIncorrect, err := HMACSha256(key, bytes.NewReader(newplain))
						require.NoError(t, err)
						require.NotEqual(t, hmac1, hmacIncorrect)
					})

					t.Run("incorrect key", func(t *testing.T) {
						newkey, err := Salt(keyLen)
						require.NoError(t, err)

						hmacIncorrect, err := HMACSha256(newkey, bytes.NewReader(plain))
						require.NoError(t, err)
						require.NotEqual(t, hmac1, hmacIncorrect)
					})
				})
			}
		})
	}
}

func TestVerifyHMACSha256(t *testing.T) {
	t.Parallel()

	key, err := Salt(32)
	require.NoError(t, err)
	plain, err := Salt(128)
	require.NoError(t, err)

	mac, err := HMACSha256(key, bytes.NewReader(plain))
	require.NoError(t, err)

	t.Run("valid", func(t *testing.T) {
		t.Parallel()
		err := VerifyHMACSha256(key, bytes.NewReader(plain), mac)
		require.NoError(t, err)
	})

	t.Run("wrong data", func(t *testing.T) {
		t.Parallel()
		wrongPlain, err := Salt(128)
		require.NoError(t, err)
		err = VerifyHMACSha256(key, bytes.NewReader(wrongPlain), mac)
		require.ErrorContains(t, err, "hmac verification failed")
	})

	t.Run("wrong key", func(t *testing.T) {
		t.Parallel()
		wrongKey, err := Salt(32)
		require.NoError(t, err)
		err = VerifyHMACSha256(wrongKey, bytes.NewReader(plain), mac)
		require.ErrorContains(t, err, "hmac verification failed")
	})

	t.Run("wrong mac", func(t *testing.T) {
		t.Parallel()
		wrongMAC := make([]byte, 32)
		err := VerifyHMACSha256(key, bytes.NewReader(plain), wrongMAC)
		require.ErrorContains(t, err, "hmac verification failed")
	})
}
