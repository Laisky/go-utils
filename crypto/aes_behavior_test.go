package crypto

import (
	"bytes"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAesBehavior_AEADAllKeySizes verifies that AEADEncrypt and AEADDecrypt round-trip the same
// plaintext with random 16-, 24- and 32-byte keys (AES-128, AES-192 and AES-256), each in its own
// parallel subtest.
func TestAesBehavior_AEADAllKeySizes(t *testing.T) {
	t.Parallel()

	keySizes := []struct {
		name string
		size int
	}{
		{"AES-128/16-byte-key", 16},
		{"AES-192/24-byte-key", 24},
		{"AES-256/32-byte-key", 32},
	}

	plaintext := []byte("hello aead key size test")

	for _, ks := range keySizes {
		ks := ks
		t.Run(ks.name, func(t *testing.T) {
			t.Parallel()

			key := make([]byte, ks.size)
			_, err := rand.Read(key)
			require.NoError(t, err)

			ciphertext, err := AEADEncrypt(key, plaintext, nil)
			require.NoError(t, err)
			require.NotEmpty(t, ciphertext)

			got, err := AEADDecrypt(key, ciphertext, nil)
			require.NoError(t, err)
			require.Equal(t, plaintext, got)
		})
	}
}

// TestAesBehavior_AEADEncryptEmptyPlaintext verifies that AEADEncrypt rejects an empty plaintext
// with a "content is empty" error.
func TestAesBehavior_AEADEncryptEmptyPlaintext(t *testing.T) {
	t.Parallel()

	key := make([]byte, 16)
	_, err := rand.Read(key)
	require.NoError(t, err)

	_, err = AEADEncrypt(key, []byte{}, nil)
	require.ErrorContains(t, err, "content is empty")
}

// TestAesBehavior_AEADDecryptEmptyCiphertext verifies that AEADDecrypt rejects an empty ciphertext
// with a "ciphertext is empty" error.
func TestAesBehavior_AEADDecryptEmptyCiphertext(t *testing.T) {
	t.Parallel()

	key := make([]byte, 16)
	_, err := rand.Read(key)
	require.NoError(t, err)

	_, err = AEADDecrypt(key, []byte{}, nil)
	require.ErrorContains(t, err, "ciphertext is empty")
}

// TestAesBehavior_AEADDecryptTruncatedCiphertext verifies that AEADDecrypt rejects a ciphertext of
// AesGcmIvLen-1 random bytes, which is shorter than the 12-byte GCM nonce, with a "ciphertext too
// short" error.
func TestAesBehavior_AEADDecryptTruncatedCiphertext(t *testing.T) {
	t.Parallel()

	key := make([]byte, 16)
	_, err := rand.Read(key)
	require.NoError(t, err)

	// Nonce size for AES-GCM is 12 bytes; provide fewer bytes.
	shortCiphertext := make([]byte, AesGcmIvLen-1)
	_, err = rand.Read(shortCiphertext)
	require.NoError(t, err)

	_, err = AEADDecrypt(key, shortCiphertext, nil)
	require.ErrorContains(t, err, "ciphertext too short")
}

// TestAesBehavior_AEADDecryptTamperedCiphertext verifies that flipping one byte of the encrypted
// payload that follows the IV prefix of an AEADEncrypt output makes AEADDecrypt fail with a
// "message authentication failed" error.
func TestAesBehavior_AEADDecryptTamperedCiphertext(t *testing.T) {
	t.Parallel()

	key := make([]byte, 16)
	_, err := rand.Read(key)
	require.NoError(t, err)

	plaintext := []byte("tamper detection test")
	ciphertext, err := AEADEncrypt(key, plaintext, nil)
	require.NoError(t, err)

	// Flip a byte in the ciphertext portion (after the IV prefix).
	tampered := make([]byte, len(ciphertext))
	copy(tampered, ciphertext)
	tampered[AesGcmIvLen+1] ^= 0xFF

	_, err = AEADDecrypt(key, tampered, nil)
	require.Error(t, err)
	require.ErrorContains(t, err, "message authentication failed")
}

// TestAesBehavior_AEADEncryptBasicWrongIVSize verifies that AEADEncryptBasic rejects IVs of 0, 8,
// 11, 13, 16 and 24 bytes, i.e. any length other than the 12-byte GCM nonce, with an "iv size not
// match" error.
func TestAesBehavior_AEADEncryptBasicWrongIVSize(t *testing.T) {
	t.Parallel()

	key := make([]byte, 16)
	_, err := rand.Read(key)
	require.NoError(t, err)

	plaintext := []byte("wrong iv size test")

	wrongIVSizes := []int{0, 8, 11, 13, 16, 24}
	for _, ivLen := range wrongIVSizes {
		iv := make([]byte, ivLen)
		_, _ = rand.Read(iv)

		_, _, err := AEADEncryptBasic(key, plaintext, iv, nil)
		require.ErrorContains(t, err, "iv size not match",
			"expected error for IV length %d", ivLen)
	}
}

// TestAesBehavior_AEADEncryptBasicEmptyPlaintext verifies that AEADEncryptBasic rejects an empty
// plaintext with a "content is empty" error even when the key and the 12-byte IV are valid.
func TestAesBehavior_AEADEncryptBasicEmptyPlaintext(t *testing.T) {
	t.Parallel()

	key := make([]byte, 16)
	_, err := rand.Read(key)
	require.NoError(t, err)

	iv := make([]byte, AesGcmIvLen)
	_, err = rand.Read(iv)
	require.NoError(t, err)

	_, _, err = AEADEncryptBasic(key, []byte{}, iv, nil)
	require.ErrorContains(t, err, "content is empty")
}

// TestAesBehavior_AEADDecryptBasicEmptyCiphertext verifies that AEADDecryptBasic rejects an empty
// ciphertext with a "ciphertext is empty" error even when the key, IV and tag sizes are valid.
func TestAesBehavior_AEADDecryptBasicEmptyCiphertext(t *testing.T) {
	t.Parallel()

	key := make([]byte, 16)
	_, err := rand.Read(key)
	require.NoError(t, err)

	iv := make([]byte, AesGcmIvLen)
	_, err = rand.Read(iv)
	require.NoError(t, err)

	tag := make([]byte, AesGcmTagLen)

	_, err = AEADDecryptBasic(key, []byte{}, iv, tag, nil)
	require.ErrorContains(t, err, "ciphertext is empty")
}

// TestAesBehavior_AEADDecryptBasicTamperedTag verifies that AEADDecryptBasic fails with a "message
// authentication failed" error when the first byte of the authentication tag returned by
// AEADEncryptBasic is flipped.
func TestAesBehavior_AEADDecryptBasicTamperedTag(t *testing.T) {
	t.Parallel()

	key := make([]byte, 16)
	_, err := rand.Read(key)
	require.NoError(t, err)

	iv := make([]byte, AesGcmIvLen)
	_, err = rand.Read(iv)
	require.NoError(t, err)

	plaintext := []byte("tag tamper test")
	ciphertext, tag, err := AEADEncryptBasic(key, plaintext, iv, nil)
	require.NoError(t, err)

	// Flip a byte in the tag.
	tamperedTag := make([]byte, len(tag))
	copy(tamperedTag, tag)
	tamperedTag[0] ^= 0xFF

	_, err = AEADDecryptBasic(key, ciphertext, iv, tamperedTag, nil)
	require.Error(t, err)
	require.ErrorContains(t, err, "message authentication failed")
}

// TestAesBehavior_AEADDecryptBasicTamperedCiphertext verifies that AEADDecryptBasic fails with a
// "message authentication failed" error when the first ciphertext byte returned by AEADEncryptBasic
// is flipped while the IV and tag are left intact.
func TestAesBehavior_AEADDecryptBasicTamperedCiphertext(t *testing.T) {
	t.Parallel()

	key := make([]byte, 16)
	_, err := rand.Read(key)
	require.NoError(t, err)

	iv := make([]byte, AesGcmIvLen)
	_, err = rand.Read(iv)
	require.NoError(t, err)

	plaintext := []byte("ciphertext tamper test")
	ciphertext, tag, err := AEADEncryptBasic(key, plaintext, iv, nil)
	require.NoError(t, err)

	// Flip a byte in the ciphertext.
	tamperedCiphertext := make([]byte, len(ciphertext))
	copy(tamperedCiphertext, ciphertext)
	tamperedCiphertext[0] ^= 0xFF

	_, err = AEADDecryptBasic(key, tamperedCiphertext, iv, tag, nil)
	require.Error(t, err)
	require.ErrorContains(t, err, "message authentication failed")
}

// TestAesBehavior_AesCtrStreamLargeData verifies that 10 MiB of random data encrypted by
// AesCtrStreamEncrypt with a 32-byte key decrypts back to identical bytes through
// AesCtrStreamDecrypt.
func TestAesBehavior_AesCtrStreamLargeData(t *testing.T) {
	t.Parallel()

	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)

	// 10 MB of random data.
	const size = 10 * 1024 * 1024
	plaintext := make([]byte, size)
	_, err = rand.Read(plaintext)
	require.NoError(t, err)

	encReader, err := AesCtrStreamEncrypt(key, bytes.NewReader(plaintext))
	require.NoError(t, err)

	encData, err := io.ReadAll(encReader)
	require.NoError(t, err)

	decReader, err := AesCtrStreamDecrypt(key, bytes.NewReader(encData))
	require.NoError(t, err)

	decData, err := io.ReadAll(decReader)
	require.NoError(t, err)

	require.Equal(t, plaintext, decData)
}

// TestAesBehavior_AesCtrStreamWrongKeyDecryption verifies that decrypting an AesCtrStreamEncrypt
// stream with a different random key returns no error, because CTR mode is unauthenticated, but
// yields output that differs from the original plaintext.
func TestAesBehavior_AesCtrStreamWrongKeyDecryption(t *testing.T) {
	t.Parallel()

	keyA := make([]byte, 16)
	_, err := rand.Read(keyA)
	require.NoError(t, err)

	keyB := make([]byte, 16)
	_, err = rand.Read(keyB)
	require.NoError(t, err)

	plaintext := []byte("CTR wrong key test - should produce garbled output")

	encReader, err := AesCtrStreamEncrypt(keyA, bytes.NewReader(plaintext))
	require.NoError(t, err)

	encData, err := io.ReadAll(encReader)
	require.NoError(t, err)

	// Decrypt with wrong key B - CTR mode does not error, just gives wrong plaintext.
	decReader, err := AesCtrStreamDecrypt(keyB, bytes.NewReader(encData))
	require.NoError(t, err)

	decData, err := io.ReadAll(decReader)
	require.NoError(t, err)

	require.NotEqual(t, plaintext, decData,
		"decryption with wrong key should not produce original plaintext")
}

// TestAesBehavior_AesCtrStreamEmptyReader verifies that an empty input stream encrypted by
// AesCtrStreamEncrypt, which still emits the IV, decrypts through AesCtrStreamDecrypt to empty
// output without error.
func TestAesBehavior_AesCtrStreamEmptyReader(t *testing.T) {
	t.Parallel()

	key := make([]byte, 16)
	_, err := rand.Read(key)
	require.NoError(t, err)

	encReader, err := AesCtrStreamEncrypt(key, bytes.NewReader(nil))
	require.NoError(t, err)

	encData, err := io.ReadAll(encReader)
	require.NoError(t, err)

	decReader, err := AesCtrStreamDecrypt(key, bytes.NewReader(encData))
	require.NoError(t, err)

	decData, err := io.ReadAll(decReader)
	require.NoError(t, err)

	require.Empty(t, decData)
}

// TestAesBehavior_AESEncryptFilesInDirExtensionFilter verifies that AESEncryptFilesInDir with
// WithAESFilesInDirFileExt(".json") encrypts config.json into a config.json.enc that AesDecrypt
// restores, while config.toml gets no encrypted counterpart.
func TestAesBehavior_AESEncryptFilesInDirExtensionFilter(t *testing.T) {
	t.Parallel()

	dir, err := os.MkdirTemp("", "aes-ext-filter-*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	jsonContent := []byte("json content here")
	tomlContent := []byte("toml content here")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), jsonContent, 0640))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), tomlContent, 0640))

	key := make([]byte, 16)
	_, err = rand.Read(key)
	require.NoError(t, err)

	err = AESEncryptFilesInDir(dir, key, WithAESFilesInDirFileExt(".json"))
	require.NoError(t, err)

	// The .json file should have been encrypted.
	encBytes, err := os.ReadFile(filepath.Join(dir, "config.json.enc"))
	require.NoError(t, err)

	got, err := AesDecrypt(key, encBytes)
	require.NoError(t, err)
	require.Equal(t, jsonContent, got)

	// The .toml file should NOT have an encrypted counterpart.
	_, err = os.Stat(filepath.Join(dir, "config.toml.enc"))
	require.True(t, os.IsNotExist(err),
		"config.toml.enc should not exist because .toml was not in the filter")
}

// TestAesBehavior_WithAESFilesInDirFileExtInvalid verifies that WithAESFilesInDirFileExt rejects
// extensions without a leading dot ("json", "toml" and the empty string) with an "ext should start
// with `.`" error.
func TestAesBehavior_WithAESFilesInDirFileExtInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ext  string
	}{
		{"no leading dot", "json"},
		{"empty string", ""},
		{"plain word", "toml"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opt := new(encryptFilesOption)
			opt.fillDefault()

			err := WithAESFilesInDirFileExt(tt.ext)(opt)
			require.Error(t, err)
			require.ErrorContains(t, err, "ext should start with `.`")
		})
	}
}
