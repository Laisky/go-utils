package testgmssl

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	gmssl "github.com/GmSSL/GmSSL-Go"
	"github.com/stretchr/testify/require"

	gcrypto "github.com/Laisky/go-utils/v6/crypto"
)

// testSkipSmTongsuo reports whether the GmSSL/Tongsuo interoperability tests should be skipped
// because the tongsuo binary is not on PATH. It takes the calling test t, fails it if the PATH
// lookup errors for any reason other than exec.ErrNotFound, and returns true only when tongsuo
// is missing.
func testSkipSmTongsuo(t *testing.T) (skipped bool) {
	t.Helper()
	if _, err := exec.LookPath("tongsuo"); err != nil {
		require.ErrorIs(t, err, exec.ErrNotFound)
		return true
	}

	return false
}

// Test_HashBySm3 verifies interoperability of SM3 hashing: the digest of 1 MiB of random data
// computed by Tongsuo.HashBySm3 equals the digest computed by the GmSSL Go binding.
func Test_HashBySm3(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	raw, err := gcrypto.Salt(1024 * 1024)
	require.NoError(t, err)

	// signed by gmssl
	gmsslSm3 := gmssl.NewSm3()
	gmsslSm3.Update(raw)
	sigByGmssl := gmsslSm3.Digest()

	ctx := context.Background()
	ins, err := gcrypto.NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	// signed by tongsuo
	sigByTongsuo, err := ins.HashBySm3(ctx, raw)
	require.NoError(t, err)

	// should be equal
	require.Equal(t, sigByGmssl, sigByTongsuo)
}

// TestTOngsuo_EncryptBySm4Cbc verifies SM4-CBC interoperability between Tongsuo and GmSSL for
// 1 MiB of random data with a shared key and IV: ciphertext from Tongsuo.EncryptBySm4CbcBaisc
// decrypts with GmSSL, and ciphertext from GmSSL decrypts with Tongsuo.DecryptBySm4CbcBaisc
// when no HMAC tag is supplied.
func TestTOngsuo_EncryptBySm4Cbc(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	ctx := context.Background()
	ins, err := gcrypto.NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	key, err := gcrypto.Salt(16)
	require.NoError(t, err)
	iv, err := gcrypto.Salt(16)
	require.NoError(t, err)
	plaintext, err := gcrypto.Salt(1024 * 1024)
	require.NoError(t, err)

	t.Run("tongsuo -> gmssl", func(t *testing.T) {

		// encrypt by tongsuo
		cipher, _, err := ins.EncryptBySm4CbcBaisc(ctx, key, plaintext, iv)
		require.NoError(t, err)

		// decrypt by gmssl
		gmsslSm4, err := gmssl.NewSm4Cbc(key, iv, false)
		require.NoError(t, err)
		decrypted, err := gmsslSm4.Update(cipher)
		require.NoError(t, err)
		decrypted_last, err := gmsslSm4.Finish()
		require.NoError(t, err)
		decrypted = append(decrypted, decrypted_last...)
		require.Equal(t, plaintext, decrypted)
	})

	t.Run("gmssl -> tongsuo", func(t *testing.T) {
		// encrypt by gmssl
		gmsslSm4, err := gmssl.NewSm4Cbc(key, iv, true)
		require.NoError(t, err)
		cipher, err := gmsslSm4.Update(plaintext)
		require.NoError(t, err)
		cipher_last, err := gmsslSm4.Finish()
		require.NoError(t, err)
		cipher = append(cipher, cipher_last...)

		// decrypt by tongsuo
		decrypted, err := ins.DecryptBySm4CbcBaisc(ctx, key, cipher, iv, nil)
		require.NoError(t, err)
		require.Equal(t, plaintext, decrypted)
	})
}

// TestTongsuo_SignBySM2SM3 verifies SM2/SM3 signature interoperability between GmSSL and Tongsuo
// over 1 MiB of random data: a GmSSL signature verifies with Tongsuo.VerifyBySm2Sm3 and is
// rejected when the signature or plaintext is altered, and a Tongsuo.SignBySm2Sm3 signature
// verifies with GmSSL and is rejected when altered.
func TestTongsuo_SignBySM2SM3(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	dir, err := os.MkdirTemp("", "tongsuo*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ctx := context.Background()
	ins, err := gcrypto.NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	plaintext, err := gcrypto.Salt(1024 * 1024)
	require.NoError(t, err)

	t.Run("gmssl -> tongsuo", func(t *testing.T) {
		gmsslPrikey, err := gmssl.GenerateSm2Key()
		require.NoError(t, err)

		pubkeyPath := filepath.Join(dir, "pubkey.pem")
		err = gmsslPrikey.ExportPublicKeyInfoPem(pubkeyPath)
		require.NoError(t, err)

		pubkeyPem, err := os.ReadFile(pubkeyPath)
		require.NoError(t, err)

		// sign by gmssl
		gmsslSign, err := gmssl.NewSm2Signature(gmsslPrikey, gmssl.Sm2DefaultId, true)
		require.NoError(t, err)
		gmsslSign.Update(plaintext)
		signature, err := gmsslSign.Sign()
		require.NoError(t, err)

		// verify by tongsuo
		err = ins.VerifyBySm2Sm3(ctx, pubkeyPem, signature, plaintext)
		require.NoError(t, err)

		t.Run("invalid signature", func(t *testing.T) {
			err = ins.VerifyBySm2Sm3(ctx, pubkeyPem, append(signature[:len(signature)-1:len(signature)-1], 'a'), plaintext)
			require.ErrorContains(t, err, "Verification failure")
		})
		t.Run("invalid plaintext", func(t *testing.T) {
			err = ins.VerifyBySm2Sm3(ctx, pubkeyPem, signature, append(plaintext[:len(plaintext)-1:len(plaintext)-1], 'a'))
			require.ErrorContains(t, err, "Verification failure")
		})
	})

	t.Run("tongsuo -> gmssl", func(t *testing.T) {
		prikeyPem, err := ins.NewPrikey(ctx)
		require.NoError(t, err)

		// sign by tongsuo
		signature, err := ins.SignBySm2Sm3(ctx, prikeyPem, plaintext)
		require.NoError(t, err)

		pubkeyPem, err := ins.Prikey2Pubkey(ctx, prikeyPem)
		require.NoError(t, err)

		pubkeyPath := filepath.Join(dir, "pubkey.pem")
		err = os.WriteFile(pubkeyPath, []byte(pubkeyPem), 0644)
		require.NoError(t, err)

		gmsslPubKey, err := gmssl.ImportSm2PublicKeyInfoPem(pubkeyPath)
		require.NoError(t, err)

		// verify by gmssl
		gmsslSign, err := gmssl.NewSm2Signature(gmsslPubKey, gmssl.Sm2DefaultId, false)
		require.NoError(t, err)
		err = gmsslSign.Update(plaintext)
		require.NoError(t, err)
		ok := gmsslSign.Verify(signature)
		require.True(t, ok)

		t.Run("invalid signature", func(t *testing.T) {
			ok = gmsslSign.Verify(append(signature[:len(signature)-1:len(signature)-1], 'a'))
			require.False(t, ok)
		})
		t.Run("invalid plaintext", func(t *testing.T) {
			ok = gmsslSign.Verify(signature)
			require.False(t, ok)
		})
	})
}
