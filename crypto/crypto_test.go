// package crypto contains some useful tools to deal with encryption/decryption
package crypto

import (
	"math/rand"
	"testing"

	gutils "github.com/Laisky/go-utils/v4"
	"github.com/stretchr/testify/require"
)

func TestRSAEncrypt(t *testing.T) {
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
		{"2", args{[]byte("43243242ffefewfwf"), gutils.HashTypeSha512}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := PasswordHash(tt.args.rawpassword, tt.args.hasher)
			require.NoError(t, err)

			err = VerifyHashedPassword(tt.args.rawpassword, h)
			require.NoError(t, err)
		})
	}
}
