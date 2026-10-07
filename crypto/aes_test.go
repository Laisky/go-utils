package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	gutils "github.com/Laisky/go-utils/v6"
)

// TestAESEncryptFilesInDir verifies that AESEncryptFilesInDir with default options encrypts every
// file in a temporary directory into a sibling file with the ".enc" suffix, and that each output
// decrypts with AesDecrypt and the same 24-byte key back to the original content.
func TestAESEncryptFilesInDir(t *testing.T) {
	t.Parallel()

	dirName, err := os.MkdirTemp("", "go-utils-test-settings*")
	require.NoError(t, err)
	defer os.RemoveAll(dirName)

	cnt := []byte("12345")
	err = os.WriteFile(filepath.Join(dirName, "test1.toml"), cnt, 0640)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dirName, "test2.toml"), cnt, 0640)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dirName, "test3.toml"), cnt, 0640)
	require.NoError(t, err)

	secret := []byte("laiskyfwejfewjfewlijffed")
	err = AESEncryptFilesInDir(dirName, secret)
	require.NoError(t, err)

	for _, fname := range []string{"test1.toml.enc", "test2.toml.enc", "test3.toml.enc"} {
		fname = filepath.Join(dirName, fname)
		cipher, err := os.ReadFile(fname)
		require.NoError(t, err)

		got, err := AesDecrypt(secret, cipher)
		require.NoError(t, err)

		require.Equal(t, cnt, got)
	}
}

// TestEncryptByAes verifies that AesEncrypt output round-trips through AesDecrypt for a valid
// 32-byte key, and that AesEncrypt fails for empty content, for keys of 31 and 33 bytes, and for an
// empty key.
func TestEncryptByAes(t *testing.T) {
	t.Parallel()

	type args struct {
		secret []byte
		cnt    string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"", args{[]byte("fjdwudkwfjwiefweffewfewfjelwifew"), "mmm"}, false},
		{"", args{[]byte("fjdwudkwfjwiefweffewfewfjelwifew"), ""}, true},
		{"", args{[]byte("fjdwudkwfjwiefweffewfewfjelwifeww"), "mmm"}, true},
		{"", args{[]byte("fjdwudkwfjwiefweffewfewjelwifew"), "mmm"}, true},
		{"", args{[]byte(""), "mmm"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cipher, err := AesEncrypt(tt.args.secret, []byte(tt.args.cnt))
			if err != nil {
				if !tt.wantErr {
					t.Fatalf("EncryptByAes() error = %v, wantErr %v", err, tt.wantErr)
					return
				}

				return
			}

			decrypted, err := AesDecrypt(tt.args.secret, cipher)
			if err != nil {
				t.Fatalf("decrypt: %+v", err)
			}
			if string(decrypted) != tt.args.cnt {
				t.Fatalf("decrypted not equal to cnt")
			}
		})
	}
}

// TestNewAesReaderWrapper verifies that NewAesReaderWrapper authenticates and decrypts an
// AesEncrypt ciphertext read from an io.Reader, and that reading the wrapper to EOF yields the
// original plaintext.
func TestNewAesReaderWrapper(t *testing.T) {
	t.Parallel()

	raw := []byte("fjlf2fjjefjwijf93r23f")
	secret := []byte("fjefil2j3i2lfj32fl2defea")
	cipher, err := AesEncrypt(secret, raw)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	reader := bytes.NewReader(cipher)
	readerWraper, err := NewAesReaderWrapper(reader, secret)
	require.NoError(t, err)

	got, err := io.ReadAll(readerWraper)
	require.NoError(t, err)

	if !bytes.Equal(got, raw) {
		t.Fatalf("got: %s", string(got))
	}
}

// TestAEADDecrypt verifies that AEADEncrypt output decrypts with AEADDecrypt for plaintexts with
// and without additional data, that two encryptions of the same input differ because of the random
// IV, and that decryption with a wrong key or wrong additional data fails with a "message
// authentication failed" error.
func TestAEADDecrypt(t *testing.T) {
	t.Parallel()

	key := []byte(gutils.RandomStringWithLength(16))
	fakekey := []byte(gutils.RandomStringWithLength(16))

	type args struct {
		key            []byte
		plaintext      []byte
		additionalData []byte
	}
	tests := []struct {
		name string
		args args
	}{
		{"1", args{key, []byte("fhwkufhuweh"), []byte("laisky")}},
		{"2", args{key, []byte("31231"), nil}},
		{"3", args{key, []byte("31231"), []byte("laisky")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cipher, err := AEADEncrypt(tt.args.key, tt.args.plaintext, tt.args.additionalData)
			require.NoError(t, err)

			t.Run("different by random IV", func(t *testing.T) {
				cipher2, err := AEADEncrypt(tt.args.key, tt.args.plaintext, tt.args.additionalData)
				require.NoError(t, err)
				require.NotEqual(t, cipher, cipher2)
			})

			plain, err := AEADDecrypt(tt.args.key, cipher, tt.args.additionalData)
			require.NoError(t, err)
			require.Equal(t, tt.args.plaintext, plain)

			t.Run("wrong key", func(t *testing.T) {
				_, err := AEADDecrypt(fakekey, cipher, tt.args.additionalData)
				require.ErrorContains(t, err, "message authentication failed")
			})

			t.Run("wrong addional data", func(t *testing.T) {
				_, err := AEADDecrypt(tt.args.key, cipher, []byte("fake"))
				require.ErrorContains(t, err, "message authentication failed")
			})
		})
	}
}

// TestAEADBasic verifies the AEADEncryptBasic and AEADDecryptBasic pair: the ciphertext length
// equals the plaintext length, a different IV changes ciphertext and tag while the same IV
// reproduces them, the IV||ciphertext||tag concatenation decrypts with AEADDecrypt, a wrong key or
// wrong additional data fails authentication, and a wrongly sized IV fails with "iv size not
// match".
func TestAEADBasic(t *testing.T) {
	t.Parallel()

	key := []byte(gutils.RandomStringWithLength(16))
	fakekey := []byte(gutils.RandomStringWithLength(16))

	type args struct {
		key            []byte
		plaintext      []byte
		additionalData []byte
	}
	tests := []struct {
		name string
		args args
	}{
		{"1", args{key, []byte("fhwkufhuweh"), []byte("laisky")}},
		{"2", args{key, []byte("31231"), nil}},
		{"3", args{key, []byte("31231"), []byte("laisky")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iv := []byte(gutils.RandomStringWithLength(12))

			cipher, tag, err := AEADEncryptBasic(tt.args.key, tt.args.plaintext, iv, tt.args.additionalData)
			require.NoError(t, err)
			require.Equal(t, len(cipher), len(tt.args.plaintext))

			t.Run("different result by different IV", func(t *testing.T) {
				iv2 := []byte(gutils.RandomStringWithLength(12))
				cipher2, tag2, err := AEADEncryptBasic(tt.args.key, tt.args.plaintext, iv2, tt.args.additionalData)
				require.NoError(t, err)
				require.NotEqual(t, iv, iv2)
				require.NotEqual(t, cipher, cipher2)
				require.NotEqual(t, tag, tag2)
			})
			t.Run("same result by same IV", func(t *testing.T) {
				cipher2, tag2, err := AEADEncryptBasic(tt.args.key, tt.args.plaintext, iv, tt.args.additionalData)
				require.NoError(t, err)
				require.Equal(t, cipher, cipher2)
				require.Equal(t, tag, tag2)
			})

			plain, err := AEADDecryptBasic(tt.args.key, cipher, iv, tag, tt.args.additionalData)
			require.NoError(t, err)
			require.Equal(t, tt.args.plaintext, plain)

			t.Run("decrypt by sugar method", func(t *testing.T) {
				combindedCipher := append(iv, cipher...)
				combindedCipher = append(combindedCipher, tag...)

				plain, err = AEADDecrypt(tt.args.key, combindedCipher, tt.args.additionalData)
				require.NoError(t, err)
				require.Equal(t, tt.args.plaintext, plain)
			})

			t.Run("wrong key", func(t *testing.T) {
				_, err := AEADDecryptBasic(fakekey, cipher, iv, tag, tt.args.additionalData)
				require.ErrorContains(t, err, "message authentication failed")
			})

			t.Run("wrong addional data", func(t *testing.T) {
				_, err := AEADDecryptBasic(tt.args.key, cipher, iv, tag, []byte("fake"))
				require.ErrorContains(t, err, "message authentication failed")
			})

			t.Run("wrong iv", func(t *testing.T) {
				_, err := AEADDecryptBasic(tt.args.key, cipher, []byte("fake"), tag, tt.args.additionalData)
				require.ErrorContains(t, err, "iv size not match")
			})
		})
	}
}

// TestAEADDecryptBasicNoMutation verifies that AEADDecryptBasic does not write into the spare
// capacity of the caller's ciphertext slice: the bytes after the ciphertext in the shared backing
// buffer, pre-filled with 0xAA, are unchanged after a successful decryption.
func TestAEADDecryptBasicNoMutation(t *testing.T) {
	t.Parallel()

	key := []byte(gutils.RandomStringWithLength(16))
	iv := []byte(gutils.RandomStringWithLength(12))
	plaintext := []byte("no-mutation-check")

	ciphertext, tag, err := AEADEncryptBasic(key, plaintext, iv, nil)
	require.NoError(t, err)

	buf := make([]byte, len(ciphertext)+len(tag))
	copy(buf, ciphertext)

	pad := buf[len(ciphertext):]
	for i := range pad {
		pad[i] = 0xAA
	}

	_, err = AEADDecryptBasic(key, buf[:len(ciphertext)], iv, tag, nil)
	require.NoError(t, err)

	for i := range pad {
		require.Equal(t, byte(0xAA), pad[i])
	}
}

// TestGcmIvLength verifies that for 16-, 24- and 32-byte AES keys the standard library GCM nonce
// size and overhead equal the package constants AesGcmIvLen (12) and AesGcmTagLen (16).
func TestGcmIvLength(t *testing.T) {
	for _, keyLength := range []int{16, 24, 32} {
		key := []byte(gutils.RandomStringWithLength(keyLength))
		c, err := aes.NewCipher(key)
		require.NoError(t, err)

		gcm, err := cipher.NewGCM(c)
		require.NoError(t, err)

		require.Equal(t, AesGcmIvLen, gcm.NonceSize())
		require.Equal(t, AesGcmTagLen, gcm.Overhead())
	}
}

// TestAesCtrStream verifies AesCtrStreamEncrypt and AesCtrStreamDecrypt round trips for normal,
// empty, long, special-character, NUL-containing and Unicode messages with 16-, 24- and 32-byte
// keys and checks that the encrypted payload differs from the plaintext; it also verifies that 5-
// and 33-byte keys are rejected, that two encryptions of the same plaintext use different IVs and
// ciphertexts, and that decryption with a wrongly sized key fails.
func TestAesCtrStream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		plaintext []byte
		key       []byte
		wantErr   bool
	}{
		{
			name:      "normal case",
			plaintext: []byte("Hello, this is a test message!"),
			key:       []byte("0123456789abcdef"), // 16 bytes key
			wantErr:   false,
		},
		{
			name:      "empty message",
			plaintext: []byte(""),
			key:       []byte("0123456789abcdef"),
			wantErr:   false,
		},
		{
			name:      "long message",
			plaintext: bytes.Repeat([]byte("long message "), 1000),
			key:       []byte("0123456789abcdef"),
			wantErr:   false,
		},
		{
			name:      "invalid key size",
			plaintext: []byte("test message"),
			key:       []byte("short"),
			wantErr:   true,
		},
		{
			name:      "24 byte key",
			plaintext: []byte("message with longer key"),
			key:       bytes.Repeat([]byte("k"), 24),
			wantErr:   false,
		},
		{
			name:      "32 byte key",
			plaintext: []byte("message with longest key"),
			key:       bytes.Repeat([]byte("k"), 32),
			wantErr:   false,
		},
		{
			name:      "special characters",
			plaintext: []byte("!@#$%^&*()_+{}[]|\\:;\"'<>,.?/~`"),
			key:       []byte("0123456789abcdef"),
			wantErr:   false,
		},
		{
			name:      "null bytes in message",
			plaintext: []byte("hello\x00world\x00!"),
			key:       []byte("0123456789abcdef"),
			wantErr:   false,
		},
		{
			name:      "unicode characters",
			plaintext: []byte("Hello 世界! Здравствуйте! 👋"),
			key:       []byte("0123456789abcdef"),
			wantErr:   false,
		},
		{
			name:      "oversized key",
			plaintext: []byte("test message"),
			key:       bytes.Repeat([]byte("k"), 33),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create reader from plaintext
			reader := bytes.NewReader(tt.plaintext)

			// Encrypt
			encryptedReader, err := AesCtrStreamEncrypt(tt.key, reader)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Read encrypted data
			encryptedData, err := io.ReadAll(encryptedReader)
			require.NoError(t, err)

			// Verify that encrypted data is different from plaintext
			if len(tt.plaintext) > 0 {
				require.NotEqual(t, tt.plaintext, encryptedData[aes.BlockSize:])
			}

			// Decrypt
			decryptReader, err := AesCtrStreamDecrypt(tt.key, bytes.NewReader(encryptedData))
			require.NoError(t, err)

			// Read decrypted data
			decryptedData, err := io.ReadAll(decryptReader)
			require.NoError(t, err)

			// Verify decrypted data matches original plaintext
			require.Equal(t, tt.plaintext, decryptedData)
		})
	}

	t.Run("different IV produces different ciphertext", func(t *testing.T) {
		t.Parallel()

		plaintext := []byte("test message")
		key := []byte("0123456789abcdef")

		// First encryption
		reader1 := bytes.NewReader(plaintext)
		encrypted1, err := AesCtrStreamEncrypt(key, reader1)
		require.NoError(t, err)
		data1, err := io.ReadAll(encrypted1)
		require.NoError(t, err)

		// Second encryption
		reader2 := bytes.NewReader(plaintext)
		encrypted2, err := AesCtrStreamEncrypt(key, reader2)
		require.NoError(t, err)
		data2, err := io.ReadAll(encrypted2)
		require.NoError(t, err)

		// Verify different IVs were used
		require.NotEqual(t, data1[:aes.BlockSize], data2[:aes.BlockSize])
		// Verify ciphertexts are different
		require.NotEqual(t, data1[aes.BlockSize:], data2[aes.BlockSize:])
	})

	t.Run("wrong key size for decryption", func(t *testing.T) {
		t.Parallel()

		plaintext := []byte("test message")
		key := []byte("0123456789abcdef")

		// Encrypt with correct key
		reader := bytes.NewReader(plaintext)
		encryptedReader, err := AesCtrStreamEncrypt(key, reader)
		require.NoError(t, err)
		encryptedData, err := io.ReadAll(encryptedReader)
		require.NoError(t, err)

		// Try to decrypt with wrong key size
		wrongKey := []byte("wrong")
		_, err = AesCtrStreamDecrypt(wrongKey, bytes.NewReader(encryptedData))
		require.Error(t, err)
	})
}

// TestWithAESFilesInDirFileSuffix verifies that WithAESFilesInDirFileSuffix accepts suffixes
// starting with "." (including ".test.encrypted") and rejects empty or dotless suffixes with a
// "suffix should start with `.`" error, and that AESEncryptFilesInDir with a ".custom" suffix
// writes a test.toml.custom file that AesDecrypt restores.
func TestWithAESFilesInDirFileSuffix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		suffix    string
		wantError bool
		errMsg    string
	}{
		{
			name:      "valid suffix",
			suffix:    ".test",
			wantError: false,
		},
		{
			name:      "invalid suffix without dot",
			suffix:    "test",
			wantError: true,
			errMsg:    "suffix should start with `.`",
		},
		{
			name:      "empty suffix",
			suffix:    "",
			wantError: true,
			errMsg:    "suffix should start with `.`",
		},
		{
			name:      "multiple dots",
			suffix:    ".test.encrypted",
			wantError: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opt := new(encryptFilesOption)
			opt.fillDefault()

			err := WithAESFilesInDirFileSuffix(tt.suffix)(opt)
			if tt.wantError {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.errMsg)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.suffix, opt.suffix)
			}
		})
	}

	t.Run("practical usage with AESEncryptFilesInDir", func(t *testing.T) {
		dirName, err := os.MkdirTemp("", "go-utils-test-settings*")
		require.NoError(t, err)
		defer os.RemoveAll(dirName)

		// Create test file
		cnt := []byte("test content")
		err = os.WriteFile(filepath.Join(dirName, "test.toml"), cnt, 0640)
		require.NoError(t, err)

		// Custom suffix
		customSuffix := ".custom"
		// Use a proper AES key size (16 bytes for AES-128)
		secret := []byte("1234567890123456")
		err = AESEncryptFilesInDir(dirName, secret,
			WithAESFilesInDirFileExt(".toml"),
			WithAESFilesInDirFileSuffix(customSuffix))
		require.NoError(t, err)

		// Verify encrypted file exists with custom suffix
		encryptedFile := filepath.Join(dirName, "test.toml"+customSuffix)
		_, err = os.Stat(encryptedFile)
		require.NoError(t, err)

		// Verify content can be decrypted
		cipher, err := os.ReadFile(encryptedFile)
		require.NoError(t, err)

		got, err := AesDecrypt(secret, cipher)
		require.NoError(t, err)
		require.Equal(t, cnt, got)
	})
}
