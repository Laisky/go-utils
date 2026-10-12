package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/binary"
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

// openRSAEnvelopeWithStdlibForTest independently parses the wire format and authenticates it with standard-library primitives.
func openRSAEnvelopeWithStdlibForTest(t *testing.T, key *rsa.PrivateKey, encrypted []byte) []byte {
	t.Helper()
	require.GreaterOrEqual(t, len(encrypted), 11+key.Size()+12+16)
	require.Equal(t, "GURSAENV", string(encrypted[:8]))
	require.Equal(t, byte(1), encrypted[8])
	size := int(binary.BigEndian.Uint16(encrypted[9:11]))
	require.Equal(t, key.Size(), size)
	symmetric, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, key, encrypted[11:11+size], []byte("go-utils RSA-OAEP AES-256-GCM envelope v1"))
	require.NoError(t, err)
	require.Len(t, symmetric, 32)
	block, err := aes.NewCipher(symmetric)
	require.NoError(t, err)
	aead, err := cipher.NewGCM(block)
	require.NoError(t, err)
	prefix := 11 + size + 12
	plaintext, err := aead.Open(nil, encrypted[11+size:prefix], encrypted[prefix:], encrypted[:prefix])
	require.NoError(t, err)
	return plaintext
}

// TestRSAOAEPEnvelopeTampering verifies byte mutations, every truncation length, appended data, wrong keys, and authenticated empty messages.
func TestRSAOAEPEnvelopeTampering(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	for _, plain := range [][]byte{nil, []byte("bounded authenticated test payload")} {
		encrypted, err := RSAEncryptByOAEP(&key.PublicKey, plain)
		require.NoError(t, err)
		require.True(t, bytes.Equal(plain, openRSAEnvelopeWithStdlibForTest(t, key, encrypted)))
		out, err := RSADecryptByOAEP(key, encrypted)
		require.NoError(t, err)
		require.True(t, bytes.Equal(plain, out))
		for index := range encrypted {
			modified := bytes.Clone(encrypted)
			modified[index] ^= 1
			out, err := RSADecryptByOAEP(key, modified)
			require.ErrorIs(t, err, ErrInvalidRSAEnvelope, "accepted byte mutation at %d", index)
			require.Nil(t, out)
		}
		for length := 0; length < len(encrypted); length++ {
			out, err := RSADecryptByOAEP(key, encrypted[:length])
			require.ErrorIs(t, err, ErrInvalidRSAEnvelope, "accepted truncation to %d", length)
			require.Nil(t, out)
		}
		out, err = RSADecryptByOAEP(key, append(bytes.Clone(encrypted), 0))
		require.ErrorIs(t, err, ErrInvalidRSAEnvelope)
		require.Nil(t, out)
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	encrypted, err := RSAEncryptByOAEP(&key.PublicKey, []byte("wrong-key control"))
	require.NoError(t, err)
	out, err := RSADecryptByOAEP(other, encrypted)
	require.ErrorIs(t, err, ErrInvalidRSAEnvelope)
	require.Nil(t, out)
}

// TestRSAOAEPLegacyMigration requires explicit legacy decoding and rejects format downgrade through the default decryptor.
func TestRSAOAEPLegacyMigration(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	var legacy []byte
	for _, part := range [][]byte{[]byte("first"), []byte("second")} {
		block, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, &key.PublicKey, part, nil)
		require.NoError(t, err)
		legacy = append(legacy, block...)
	}
	for _, old := range [][]byte{legacy, legacy[:key.Size()]} {
		out, err := RSADecryptByOAEP(key, old)
		require.ErrorIs(t, err, ErrInvalidRSAEnvelope)
		require.Nil(t, out)
	}
	out, err := RSADecryptByOAEPLegacy(key, legacy)
	require.NoError(t, err)
	require.Equal(t, []byte("firstsecond"), out)
	upgraded, err := RSAEncryptByOAEP(&key.PublicKey, out)
	require.NoError(t, err)
	restored, err := RSADecryptByOAEP(key, upgraded)
	require.NoError(t, err)
	require.Equal(t, out, restored)
	out, err = RSADecryptByOAEPLegacy(key, legacy[:len(legacy)-1])
	require.Error(t, err)
	require.Nil(t, out)
}
