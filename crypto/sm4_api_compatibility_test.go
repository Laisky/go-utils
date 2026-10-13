package crypto

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSM4CompatibilityOriginalNames verifies both historical APIs against independent
// pre-envelope framing and MAC implementations at empty and multi-block boundaries.
func TestSM4CompatibilityOriginalNames(t *testing.T) {
	ins := newSm4TestTongsuo(t)
	ctx := context.Background()
	key := []byte("0123456789abcdef")
	iv := []byte("fedcba9876543210")
	for _, n := range []int{0, 1, 15, 16, 17, 65} {
		plain := bytes.Repeat([]byte{0x61}, n)
		ct := sm4TestRawCbcEncrypt(t, key, iv, sm4TestPkcs7Pad(plain))
		historical := sm4TestLegacyCombine(key, iv, ct)
		t.Run("reader-"+string(rune('A'+n)), func(t *testing.T) {
			got, err := ins.DecryptBySm4Cbc(ctx, key, historical)
			require.NoError(t, err, "the unchanged old name must read historical ciphertext")
			require.Equal(t, plain, got)
			got, err = ins.DecryptBySm4CbcBaisc(ctx, key, ct, iv, historical[len(historical)-32:])
			require.NoError(t, err)
			require.Equal(t, plain, got)
		})
		t.Run("writer-"+string(rune('A'+n)), func(t *testing.T) {
			out, tag, err := ins.EncryptBySm4CbcBaisc(ctx, key, plain, iv)
			require.NoError(t, err)
			require.Equal(t, ct, out)
			require.Equal(t, historical[len(historical)-32:], tag, "old readers must accept the returned MAC")
			combined, err := ins.EncryptBySm4Cbc(ctx, key, plain)
			require.NoError(t, err)
			require.Len(t, combined, 16+len(ct)+32)
			expected := sm4TestRawCbcEncrypt(t, key, combined[:16], sm4TestPkcs7Pad(plain))
			require.Equal(t, sm4TestLegacyCombine(key, combined[:16], expected), combined)
		})
	}
}

// TestSM4ExplicitAuthentication preserves post-change envelopes and rejects format downgrade.
func TestSM4ExplicitAuthentication(t *testing.T) {
	ins := newSm4TestTongsuo(t)
	ctx := context.Background()
	key := []byte("0123456789abcdef")
	plain := []byte("compatibility is not format autodetection")
	old := sm4TestLegacyEncrypt(t, key, plain)
	out, err := ins.DecryptBySm4CbcEnvelope(ctx, key, old)
	require.Error(t, err)
	require.Nil(t, out)
	envelope, err := ins.EncryptBySm4CbcEnvelope(ctx, key, plain)
	require.NoError(t, err)
	out, err = ins.DecryptBySm4CbcEnvelope(ctx, key, envelope)
	require.NoError(t, err)
	require.Equal(t, plain, out)
	out, err = ins.DecryptBySm4Cbc(ctx, key, envelope)
	require.Error(t, err)
	require.Nil(t, out)
	ct, tag, err := ins.EncryptBySm4CbcBasicAuthenticated(ctx, key, plain, old[:16])
	require.NoError(t, err)
	out, err = ins.DecryptBySm4CbcBasicAuthenticated(ctx, key, ct, old[:16], nil)
	require.Error(t, err)
	require.Nil(t, out)
	out, err = ins.DecryptBySm4CbcBasicAuthenticated(ctx, key, ct, old[:16], tag)
	require.NoError(t, err)
	require.Equal(t, plain, out)
}
