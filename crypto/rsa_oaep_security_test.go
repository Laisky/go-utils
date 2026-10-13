package crypto

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSecurity68InvalidOAEPKey verifies that invalid keys fail before allocation or loops.
func TestSecurity68InvalidOAEPKey(t *testing.T) {
	modulus := new(big.Int).Lsh(big.NewInt(1), 2047)
	keys := []*rsa.PublicKey{
		nil, {}, {N: big.NewInt(0), E: 65537}, {N: big.NewInt(-1), E: 65537},
		{N: new(big.Int).Lsh(big.NewInt(1), 511), E: 65537},
		{N: new(big.Int).Lsh(big.NewInt(1), 527), E: 65537},
		{N: modulus, E: 65537},
		{N: new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 1023), big.NewInt(1)), E: 65537},
		{N: modulus, E: 0}, {N: modulus, E: 2}, {N: modulus, E: 4},
	}
	for _, key := range keys {
		for _, plaintext := range [][]byte{nil, []byte("synthetic data")} {
			require.NotPanics(t, func() {
				ciphertext, err := RSAEncryptByOAEP(key, plaintext)
				require.Error(t, err)
				require.Nil(t, ciphertext)
			})
		}
	}
}

// TestSecurity68OAEPBoundaries decrypts every emitted block with the standard library.
func TestSecurity68OAEPBoundaries(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	capacity := key.Size() - 2*sha256.Size - 2
	for _, size := range []int{0, 1, capacity - 1, capacity, capacity + 1, 2 * capacity, 2*capacity + 29} {
		plaintext := bytes.Repeat([]byte{0x6a}, size)
		ciphertext, err := RSAEncryptByOAEP(&key.PublicKey, plaintext)
		require.NoError(t, err)
		wantBlocks := (size + capacity - 1) / capacity
		require.Len(t, ciphertext, wantBlocks*key.Size())
		recovered := make([]byte, 0, size)
		for pos := 0; pos < len(ciphertext); pos += key.Size() {
			chunk, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, key, ciphertext[pos:pos+key.Size()], nil)
			require.NoError(t, err)
			recovered = append(recovered, chunk...)
		}
		require.Equal(t, plaintext, recovered)
	}
	a, err := RSAEncryptByOAEP(&key.PublicKey, []byte("same input"))
	require.NoError(t, err)
	b, err := RSAEncryptByOAEP(&key.PublicKey, []byte("same input"))
	require.NoError(t, err)
	require.NotEqual(t, a, b)
}

// TestSecurity68NonpositiveCapacity separately reproduces both unsafe capacity boundaries.
// Empty plaintext makes the zero-capacity baseline case finite; no endless loop is launched.
func TestSecurity68NonpositiveCapacity(t *testing.T) {
	for _, bits := range []uint{512, 528} {
		t.Run(big.NewInt(int64(bits)).String(), func(t *testing.T) {
			key := &rsa.PublicKey{N: new(big.Int).Lsh(big.NewInt(1), bits-1), E: 65537}
			require.NotPanics(t, func() {
				ciphertext, err := RSAEncryptByOAEP(key, nil)
				require.Error(t, err)
				require.Nil(t, ciphertext)
			})
		})
	}
}

// TestSecurity68MinimumSupportedKey preserves the standard library's 1024-bit compatibility boundary.
func TestSecurity68MinimumSupportedKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)
	message := []byte("synthetic compatibility fixture")
	ciphertext, err := RSAEncryptByOAEP(&key.PublicKey, message)
	require.NoError(t, err)
	plaintext, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, key, ciphertext, nil)
	require.NoError(t, err)
	require.Equal(t, message, plaintext)
}
