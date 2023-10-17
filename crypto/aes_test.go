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

	gutils "github.com/Laisky/go-utils/v4"
)

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
