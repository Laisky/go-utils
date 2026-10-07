package crypto

import (
	"bytes"
	"context"
	"encoding/hex"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// sm4TestTongsuoEnc runs the real `tongsuo enc -sm4-cbc` with a synthetic key and
// IV, exactly as releases before issue #44 did, feeding input through stdin. It
// encrypts when encrypt is true and decrypts otherwise, and returns stdout.
func sm4TestTongsuoEnc(t *testing.T, exePath string, encrypt bool, key, iv, input []byte) []byte {
	t.Helper()
	mode := "-d"
	if encrypt {
		mode = "-e"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	//nolint:gosec // G204: test-only invocation of the trusted tongsuo binary with synthetic keys.
	cmd := exec.CommandContext(ctx, exePath, "enc", "-sm4-cbc", mode,
		"-K", hex.EncodeToString(key), "-iv", hex.EncodeToString(iv))
	cmd.Stdin = bytes.NewReader(input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	require.NoError(t, err, "tongsuo enc failed: %s", stderr.String())

	return out
}

// TestTongsuoSm4CbcInteropWithTongsuoBinary verifies that the in-process SM4-CBC
// implementation is byte-for-byte compatible with `tongsuo enc -sm4-cbc -K -iv`
// in both directions, and that legacy envelopes built from real tongsuo output
// are still decryptable through the explicit legacy decoder; regression for
// issues #44 and #45. It is skipped when tongsuo is not installed.
func TestTongsuoSm4CbcInteropWithTongsuoBinary(t *testing.T) {
	t.Parallel()
	exePath, err := exec.LookPath("tongsuo")
	if err != nil {
		t.Skip("tongsuo binary not found in PATH")
	}

	ctx := context.Background()
	ins, err := NewTongsuo(exePath)
	require.NoError(t, err)

	lengths := append([]int{4096, 1 << 20}, sm4TestPlaintextLengths...)
	for _, n := range lengths {
		key, err := Salt(16)
		require.NoError(t, err)
		iv, err := Salt(16)
		require.NoError(t, err)
		plaintext, err := Salt(max(n, 1))
		require.NoError(t, err)
		plaintext = plaintext[:n]

		expected := sm4TestTongsuoEnc(t, exePath, true, key, iv, plaintext)
		ciphertext, tag, err := ins.EncryptBySm4CbcBaisc(ctx, key, plaintext, iv)
		require.NoError(t, err)
		require.Equal(t, expected, ciphertext, "ciphertext mismatch with tongsuo for %d bytes", n)

		decryptedByTongsuo := sm4TestTongsuoEnc(t, exePath, false, key, iv, ciphertext)
		require.True(t, bytes.Equal(plaintext, decryptedByTongsuo), "tongsuo failed to decrypt %d bytes", n)

		got, err := ins.DecryptBySm4CbcBaisc(ctx, key, expected, iv, nil)
		require.NoError(t, err)
		require.True(t, bytes.Equal(plaintext, got))
		got, err = ins.DecryptBySm4CbcBaisc(ctx, key, expected, iv, tag)
		require.NoError(t, err)
		require.True(t, bytes.Equal(plaintext, got))

		// Pre-#45 EncryptBySm4Cbc output: iv || tongsuo ciphertext || HMAC(key, ciphertext).
		legacy := sm4TestLegacyCombine(key, iv, expected)
		got, err = ins.DecryptBySm4CbcLegacy(ctx, key, legacy)
		require.NoError(t, err)
		require.True(t, bytes.Equal(plaintext, got))
		_, err = ins.DecryptBySm4Cbc(ctx, key, legacy)
		require.ErrorIs(t, err, ErrSm4CbcMalformedCiphertext)
	}
}
