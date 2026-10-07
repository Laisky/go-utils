package crypto

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTongsuo_EncryptBySm4Baisc verifies SM4-CBC encryption with an HMAC tag via
// EncryptBySm4CbcBaisc/DecryptBySm4CbcBaisc: the correct key, IV, and 32-byte tag round-trip the
// plaintext; a wrong key or tag fails with "hmac not match"; wrong key, IV, or tag lengths are
// rejected; and decrypting with a wrong key and no tag never yields the original plaintext.
func TestTongsuo_EncryptBySm4Baisc(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	ctx := context.Background()
	ins, err := NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	key, err := Salt(16)
	require.NoError(t, err)
	incorrectKey, err := Salt(16)
	require.NoError(t, err)
	plaintext := []byte("Hello, World!")
	iv, err := Salt(16)
	require.NoError(t, err)
	incorrectTag, err := Salt(32)
	require.NoError(t, err)

	t.Run("correct passphare", func(t *testing.T) {
		t.Parallel()

		ciphertext, tag, err := ins.EncryptBySm4CbcBaisc(ctx, key, plaintext, iv)
		require.NoError(t, err)
		require.NotNil(t, ciphertext)
		require.Len(t, tag, 32)

		// Decrypt the ciphertext to verify the encryption
		decrypted, err := ins.DecryptBySm4CbcBaisc(ctx, key, ciphertext, iv, tag)
		require.NoError(t, err)
		require.Equal(t, plaintext, decrypted)
		// require.Equal(t, len(plaintext), len(ciphertext))
	})

	t.Run("Decrypt the ciphertext with incorrect key", func(t *testing.T) {
		t.Parallel()

		ciphertext, tag, err := ins.EncryptBySm4CbcBaisc(ctx, key, plaintext, iv)
		require.NoError(t, err)
		require.NotNil(t, ciphertext)

		_, err = ins.DecryptBySm4CbcBaisc(ctx, incorrectKey, ciphertext, iv, tag)
		require.ErrorContains(t, err, "hmac not match")

		t.Run("key in incorrect length", func(t *testing.T) {
			_, err = ins.DecryptBySm4CbcBaisc(ctx, append(key, 'd'), ciphertext, iv, tag)
			require.ErrorContains(t, err, "key should be 16 bytes")
		})

		t.Run("iv in incorrect length", func(t *testing.T) {
			_, err = ins.DecryptBySm4CbcBaisc(ctx, key, ciphertext, append(iv, 'a'), tag)
			require.ErrorContains(t, err, "iv should be 16 bytes")
		})
	})

	t.Run("Decrypt the ciphertext with incorrect tag", func(t *testing.T) {
		t.Parallel()

		ciphertext, _, err := ins.EncryptBySm4CbcBaisc(ctx, key, plaintext, iv)
		require.NoError(t, err)
		require.NotNil(t, ciphertext)

		_, err = ins.DecryptBySm4CbcBaisc(ctx, key, ciphertext, iv, incorrectTag)
		require.ErrorContains(t, err, "hmac not match")

		t.Run("tag in incorrect length", func(t *testing.T) {
			_, err = ins.DecryptBySm4CbcBaisc(ctx, key, ciphertext, iv, append(incorrectTag, []byte("123")...))
			require.ErrorContains(t, err, "hmac should be 0 or 32 bytes")
		})
	})

	t.Run("Decrypt the ciphertext with incorrect key and empty tag", func(t *testing.T) {
		t.Parallel()

		ciphertext, _, err := ins.EncryptBySm4CbcBaisc(ctx, key, plaintext, iv)
		require.NoError(t, err)
		require.NotNil(t, ciphertext)

		// Without a MAC a wrong key is only detected by the padding check, and
		// random garbage has valid PKCS#7 padding with probability ~1/256.
		decrypted, err := ins.DecryptBySm4CbcBaisc(ctx, incorrectKey, ciphertext, iv, nil)
		if err != nil {
			require.ErrorContains(t, err, "bad decrypt")
		} else {
			require.NotEqual(t, plaintext, decrypted)
		}
	})
}

// TestTongsuo_DecryptBySm4 verifies that ciphertext produced by Tongsuo.EncryptBySm4Cbc with a
// random 16-byte key decrypts back to the original plaintext through Tongsuo.DecryptBySm4Cbc.
func TestTongsuo_DecryptBySm4(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	ctx := context.Background()
	ins, err := NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	key, err := Salt(16)
	require.NoError(t, err)
	plaintext := []byte("Hello, World!")

	cipher, err := ins.EncryptBySm4Cbc(ctx, key, plaintext)
	require.NoError(t, err)

	gotPlain, err := ins.DecryptBySm4Cbc(ctx, key, cipher)
	require.NoError(t, err)
	require.Equal(t, plaintext, gotPlain)
}

// TestTongsuo_SignBySM2SM3 verifies that an SM2/SM3 signature created by Tongsuo.SignBySm2Sm3 over
// 8 KiB of random data verifies with the matching public key through Tongsuo.VerifyBySm2Sm3.
func TestTongsuo_SignBySM2SM3(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	ctx := context.Background()
	ins, err := NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	prikeyPem, err := ins.NewPrikey(ctx)
	require.NoError(t, err)

	pubkeyPem, err := ins.Prikey2Pubkey(ctx, prikeyPem)
	require.NoError(t, err)

	raw, err := Salt(1024 * 8)
	require.NoError(t, err)

	signature, err := ins.SignBySm2Sm3(ctx, prikeyPem, raw)
	require.NoError(t, err)

	err = ins.VerifyBySm2Sm3(ctx, pubkeyPem, signature, raw)
	require.NoError(t, err)
}

// TestTongsuo_HashBySm3 verifies that Tongsuo.HashBySm3 returns a raw 32-byte digest without any
// "stdin" label, is deterministic for identical input, and changes when one byte of input changes.
func TestTongsuo_HashBySm3(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	ctx := context.Background()
	ins, err := NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	content := []byte("Hello, World!")

	hash, err := ins.HashBySm3(ctx, content)
	require.NoError(t, err)
	require.NotNil(t, hash)
	require.Len(t, hash, 32)
	require.NotContains(t, string(hash), "stdin")

	hash2, err := ins.HashBySm3(ctx, content)
	require.NoError(t, err)
	require.Equal(t, hash, hash2)

	hash3, err := ins.HashBySm3(ctx, append(content[:len(content)-1:len(content)-1], 'a'))
	require.NoError(t, err)
	require.NotEqual(t, hash, hash3)
}

// TestTongsuo_EncryptBySm2 verifies that Tongsuo.EncryptBySm2/DecryptBySm2 round-trip a plaintext
// with both an SM2 key pair and an RSA-2048 key pair, and that decrypting ciphertext with extra
// prepended or appended bytes fails with "operation error" instead of returning the plaintext.
func TestTongsuo_EncryptBySm2(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	ctx := context.Background()
	ins, err := NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	plaintext := []byte("Hello, World!")

	t.Run("sm2", func(t *testing.T) {
		prikeyPem, err := ins.NewPrikey(ctx)
		require.NoError(t, err)

		pubkeyPem, err := ins.Prikey2Pubkey(ctx, prikeyPem)
		require.NoError(t, err)

		// encrypt by sm2 pubkey
		ciphertext, err := ins.EncryptBySm2(ctx, pubkeyPem, plaintext)
		require.NoError(t, err)
		require.NotNil(t, ciphertext)

		// decrypt by sm2 prikey
		decrypted, err := ins.DecryptBySm2(ctx, prikeyPem, ciphertext)
		require.NoError(t, err)
		require.Equal(t, plaintext, decrypted)

		t.Run("ivalid ciphertext", func(t *testing.T) {
			decrypted, err := ins.DecryptBySm2(ctx, prikeyPem,
				append([]byte("halo"), ciphertext...))
			require.ErrorContains(t, err, "operation error")
			require.NotEqual(t, plaintext, decrypted)
		})
	})

	t.Run("compatable with rsa", func(t *testing.T) {
		prikey, err := NewRSAPrikey(RSAPrikeyBits2048)
		require.NoError(t, err)
		prikeyPem, err := Prikey2Pem(prikey)
		require.NoError(t, err)

		pubkey := Prikey2Pubkey(prikey)
		pubkeyPem, err := Pubkey2Pem(pubkey)
		require.NoError(t, err)

		// encrypt by rsa pubkey
		ciphertext, err := ins.EncryptBySm2(ctx, pubkeyPem, plaintext)
		require.NoError(t, err)
		require.NotNil(t, ciphertext)

		// decrypt by rsa prikey
		decrypted, err := ins.DecryptBySm2(ctx, prikeyPem, ciphertext)
		require.NoError(t, err)
		require.Equal(t, plaintext, decrypted)

		t.Run("ivalid ciphertext", func(t *testing.T) {
			decrypted, err := ins.DecryptBySm2(ctx, prikeyPem,
				append(ciphertext, []byte("halo")...))
			require.ErrorContains(t, err, "operation error")
			require.NotEqual(t, plaintext, decrypted)
		})
	})
}
