package crypto

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRSAOAEPMessageStructure rejects removal, repetition and permutation of whole ciphertext blocks before returning any plaintext.
func TestRSAOAEPMessageStructure(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	capacity := key.Size() - 2*sha256.Size - 2
	plain := append(bytes.Repeat([]byte("a"), capacity), bytes.Repeat([]byte("b"), capacity)...)
	plain = append(plain, bytes.Repeat([]byte("c"), capacity)...)
	ciphertext, err := RSAEncryptByOAEP(&key.PublicKey, plain)
	require.NoError(t, err)
	got, err := RSADecryptByOAEP(key, ciphertext)
	require.NoError(t, err)
	require.Equal(t, plain, got)
	size := key.Size()
	cases := map[string][]byte{
		"remove_first": append([]byte(nil), ciphertext[size:]...),
		"remove_last":  append([]byte(nil), ciphertext[:len(ciphertext)-size]...),
		"duplicate":    append(append([]byte(nil), ciphertext[:size]...), ciphertext...),
		"permute":      append(append(append([]byte(nil), ciphertext[size:2*size]...), ciphertext[:size]...), ciphertext[2*size:]...),
		"partial":      append([]byte(nil), ciphertext[:len(ciphertext)-1]...),
		"empty":        nil,
	}
	for name, corrupted := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := RSADecryptByOAEP(key, corrupted)
			require.Error(t, err, "accepted structurally modified message containing %d plaintext bytes", len(out))
			require.Empty(t, out)
		})
	}
}
