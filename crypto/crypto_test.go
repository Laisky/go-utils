// package crypto contains some useful tools to deal with encryption/decryption
package crypto

import (
	"math/rand"
	"testing"

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
