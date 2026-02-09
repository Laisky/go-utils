// package crypto contains some useful tools to deal with encryption/decryption
package crypto

import (
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	gutils "github.com/Laisky/go-utils/v6"
)

func TestRSAEncrypt(t *testing.T) {
	t.Parallel()
	prikey, err := NewRSAPrikey(RSAPrikeyBits3072)
	require.NoError(t, err)

	plain := make([]byte, 102400)
	_, err = rand.Read(plain)
	require.NoError(t, err)

	cipher, err := RSAEncrypt(&prikey.PublicKey, plain)
	require.NoError(t, err)

	gotPlain, err := RSADecrypt(prikey, cipher)
	require.NoError(t, err)

	require.Equal(t, plain, gotPlain)
}

func TestVerifyHashedPassword(t *testing.T) {
	t.Parallel()
	type args struct {
		rawpassword []byte
		hasher      gutils.HashType
	}
	tests := []struct {
		name string
		args args
	}{
		{"0", args{[]byte("fewfewfewfh"), gutils.HashTypeSha256}},
		{"1", args{[]byte("43243242"), gutils.HashTypeSha256}},
		{"2", args{[]byte("32ifh23fu21f2h3"), gutils.HashTypeSha512}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := PasswordHash(tt.args.rawpassword, tt.args.hasher)
			require.NoError(t, err)

			err = VerifyHashedPassword(tt.args.rawpassword, h)
			require.NoError(t, err)

			t.Logf("hashed password: %q", h)
		})
	}
}

func TestPasswordHashIterationCount(t *testing.T) {
	t.Parallel()
	h, err := PasswordHash([]byte("password"), gutils.HashTypeSha256)
	require.NoError(t, err)

	hp, err := parseHashedPassword(h)
	require.NoError(t, err)

	require.GreaterOrEqual(t, hp.hashNum, MinPasswordHashIteration)
	require.Less(t, hp.hashNum, MaxPasswordHashIteration)
}

func TestVerifyHashedPassword_LegacyIterationCompatibility(t *testing.T) {
	t.Parallel()
	rawPassword := []byte("legacy-password")
	salt := []byte("legacy-salt")
	legacyIteration := MinPasswordHashIteration - 1

	hp, err := newHashedPasswordWithMinIteration(
		salt,
		rawPassword,
		gutils.HashTypeSha256,
		legacyIteration,
		legacyMinPasswordHashIteration,
	)
	require.NoError(t, err)
	require.Less(t, legacyIteration, MinPasswordHashIteration)

	err = VerifyHashedPassword(rawPassword, hp.String())
	require.NoError(t, err)

	err = VerifyHashedPassword([]byte("wrong-password"), hp.String())
	require.ErrorContains(t, err, "password not match")
}

func TestNewHashedPassword_RejectWeakIterationForNewHashes(t *testing.T) {
	t.Parallel()

	_, err := newHashedPassword(
		[]byte("salt"),
		[]byte("password"),
		gutils.HashTypeSha256,
		MinPasswordHashIteration-1,
	)
	require.ErrorContains(t, err, "out of range")
}

func TestRsaEncryptByOAEP(t *testing.T) {
	t.Parallel()

	prikey, err := NewRSAPrikey(RSAPrikeyBits3072)
	require.NoError(t, err)

	for _, plainSize := range []int{
		1, 1024, 10240,
	} {
		plainSize := plainSize
		t.Run(fmt.Sprintf("plainSize=%d", plainSize), func(t *testing.T) {
			t.Parallel()

			plain, err := Salt(plainSize)
			require.NoError(t, err)

			cipher, err := RSAEncryptByOAEP(&prikey.PublicKey, plain)
			require.NoError(t, err)
			require.NotEqual(t, cipher, plain)

			t.Run("correct", func(t *testing.T) {
				gotPlain, err := RSADecryptByOAEP(prikey, cipher)
				require.NoError(t, err)
				require.Equal(t, plain, gotPlain)
			})

			t.Run("wrong cipher", func(t *testing.T) {
				_, err = RSADecryptByOAEP(prikey, []byte("fewfew"))
				require.Error(t, err)
			})

			t.Run("wrong key", func(t *testing.T) {
				newPrikey, err := NewRSAPrikey(RSAPrikeyBits3072)
				require.NoError(t, err)

				_, err = RSADecryptByOAEP(newPrikey, cipher)
				require.Error(t, err)
				require.ErrorContains(t, err, "decrypt chunk")
			})

			t.Run("cipher should be different", func(t *testing.T) {
				cipher1, err := RSAEncryptByOAEP(&prikey.PublicKey, plain)
				require.NoError(t, err)

				cipher2, err := RSAEncryptByOAEP(&prikey.PublicKey, plain)
				require.NoError(t, err)

				require.NotEqual(t, cipher1, cipher2)
			})
		})
	}
}

func TestVerifyHashedPassword_DoS(t *testing.T) {
	t.Parallel()
	// This should fail quickly once the limit is implemented.
	// We use a very large iteration count to simulate a DoS attack.
	largeIterationHash := "sha256.1000001.73616c74.68617368"
	err := VerifyHashedPassword([]byte("password"), largeIterationHash)
	require.Error(t, err)
	require.ErrorContains(t, err, "too many iterations")
}
