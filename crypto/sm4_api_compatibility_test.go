package crypto

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestSM4CompatibilityOriginalNames preserves the historical combined format in both directions.
func TestSM4CompatibilityOriginalNames(t *testing.T) {
	ins := new(Tongsuo)
	ctx := context.Background()
	key := []byte("0123456789abcdef")
	for _, n := range []int{0, 1, 15, 16, 17, 48, 1024} {
		plain := bytes.Repeat([]byte("a"), n)
		old := sm4TestLegacyEncrypt(t, key, plain)
		out, err := ins.DecryptBySm4Cbc(ctx, key, old)
		require.NoError(t, err, "existing callers must read historic ciphertext")
		require.True(t, bytes.Equal(plain, out))
		encoded, err := ins.EncryptBySm4Cbc(ctx, key, plain)
		require.NoError(t, err)
		require.Len(t, encoded, 16+(n/16+1)*16+32, "old readers require original framing")
		iv, ct, mac := encoded[:16], encoded[16:len(encoded)-32], encoded[len(encoded)-32:]
		expected := sm4TestRawCbcEncrypt(t, key, iv, sm4TestPkcs7Pad(plain))
		require.Equal(t, expected, ct)
		h := hmac.New(sha256.New, key)
		_, err = h.Write(ct)
		require.NoError(t, err)
		require.Equal(t, h.Sum(nil), mac, "old MAC scope and key must remain unchanged")
	}
}

// TestSM4CompatibilityBasicNames preserves ciphertext-only MACs and the optional-MAC legacy contract.
func TestSM4CompatibilityBasicNames(t *testing.T) {
	ins := new(Tongsuo)
	ctx := context.Background()
	key, iv, plain := []byte("0123456789abcdef"), []byte("abcdef0123456789"), []byte("legacy basic compatibility")
	ct := sm4TestRawCbcEncrypt(t, key, iv, sm4TestPkcs7Pad(plain))
	h := hmac.New(sha256.New, key)
	_, err := h.Write(ct)
	require.NoError(t, err)
	tag := h.Sum(nil)
	got, err := ins.DecryptBySm4CbcBaisc(ctx, key, ct, iv, tag)
	require.NoError(t, err)
	require.Equal(t, plain, got)
	got, err = ins.DecryptBySm4CbcBaisc(ctx, key, ct, iv, nil)
	require.NoError(t, err)
	require.Equal(t, plain, got)
	produced, mac, err := ins.EncryptBySm4CbcBaisc(ctx, key, plain, iv)
	require.NoError(t, err)
	require.Equal(t, ct, produced)
	require.Equal(t, tag, mac)
}
