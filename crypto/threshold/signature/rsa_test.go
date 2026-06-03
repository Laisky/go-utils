package signature

import (
	"bytes"
	"crypto/rsa"
	"testing"

	"github.com/niclabs/tcrsa"
	"github.com/stretchr/testify/require"

	gutils "github.com/Laisky/go-utils/v6"
	gcrypto "github.com/Laisky/go-utils/v6/crypto"
)

func TestVerifyBySHA256(t *testing.T) {
	t.Parallel()

	// Increase test timeout
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	total := 5
	threshold := 3

	// Generate key shares once. Use the minimum supported (and secure) key
	// size for tests; this is gated behind !testing.Short above because
	// 2048-bit threshold key generation is expensive.
	keyShares, keyMeta, err := NewKeyShares(total, threshold, gcrypto.RSAPrikeyBits2048)
	require.NoError(t, err)
	require.GreaterOrEqual(t, keyMeta.PublicKey.N.BitLen(), minRSAPublicKeyBits)

	t.Run("verify valid signature", func(t *testing.T) {
		content := gutils.RandomStringWithLength(128) // Reduced content size
		parts := gutils.RandomChoice(keyShares, threshold)
		sig, err := SignBySHA256(bytes.NewReader([]byte(content)), parts, keyMeta)
		require.NoError(t, err)

		err = VerifyBySHA256(bytes.NewReader([]byte(content)), keyMeta.PublicKey, sig)
		require.NoError(t, err)
	})

	t.Run("verify invalid content", func(t *testing.T) {
		content := gutils.RandomStringWithLength(128)
		parts := gutils.RandomChoice(keyShares, threshold)
		sig, err := SignBySHA256(bytes.NewReader([]byte(content)), parts, keyMeta)
		require.NoError(t, err)

		invalidContent := content + "invalid"
		err = VerifyBySHA256(bytes.NewReader([]byte(invalidContent)), keyMeta.PublicKey, sig)
		require.Error(t, err)
	})
}

func TestSignVerifyNilInputs(t *testing.T) {
	t.Parallel()

	t.Run("SignBySHA256 nil content", func(t *testing.T) {
		_, err := SignBySHA256(nil, nil, nil)
		require.ErrorContains(t, err, "content must not be nil")
	})
	t.Run("SignBySHA256 empty keyShares", func(t *testing.T) {
		_, err := SignBySHA256(bytes.NewReader([]byte("test")), nil, nil)
		require.ErrorContains(t, err, "keyShares must not be empty")
	})
	t.Run("SignBySHA256 nil keyMeta", func(t *testing.T) {
		_, err := SignBySHA256(bytes.NewReader([]byte("test")), make(tcrsa.KeyShareList, 1), nil)
		require.ErrorContains(t, err, "keyMeta and its PublicKey must not be nil")
	})
	t.Run("VerifyBySHA256 nil content", func(t *testing.T) {
		err := VerifyBySHA256(nil, nil, nil)
		require.ErrorContains(t, err, "content must not be nil")
	})
	t.Run("VerifyBySHA256 nil pubkey", func(t *testing.T) {
		err := VerifyBySHA256(bytes.NewReader([]byte("test")), nil, nil)
		require.ErrorContains(t, err, "pubkey must not be nil")
	})
	t.Run("VerifyBySHA256 empty signature", func(t *testing.T) {
		err := VerifyBySHA256(bytes.NewReader([]byte("test")), &rsa.PublicKey{}, nil)
		require.ErrorContains(t, err, "signature must not be empty")
	})
}

func TestNewKeyShares_IntegerOverflow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		total     int
		threshold int
		rsaBits   gcrypto.RSAPrikeyBits
		wantErr   string
	}{
		{
			// Bounds/validation cases only: each returns an error BEFORE the
			// expensive key-generation step, so this table stays fast. The
			// successful 2048-bit key-generation path is covered (once) by
			// TestVerifyBySHA256, which is gated behind !testing.Short.
			name:      "overflow uint16 max",
			total:     70000,
			threshold: 65536,
			rsaBits:   gcrypto.RSAPrikeyBits2048,
			wantErr:   "threshold and total must not exceed 65535",
		},
		{
			name:      "rsa bits too small (512)",
			total:     5,
			threshold: 3,
			rsaBits:   gcrypto.RSAPrikeyBits(512),
			wantErr:   "RSA bits must be at least 2048",
		},
		{
			// Security regression: 1024-bit RSA was previously accepted but is
			// deprecated/insecure and must now be rejected before any key is
			// generated.
			name:      "rsa bits 1024 now rejected",
			total:     5,
			threshold: 3,
			rsaBits:   gcrypto.RSAPrikeyBits(1024),
			wantErr:   "RSA bits must be at least 2048",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Generating 2048-bit threshold keys is expensive; only the
			// success cases hit key generation, so skip those in short mode.
			if tt.wantErr == "" && testing.Short() {
				t.Skip("skipping expensive 2048-bit key generation in short mode")
			}

			shares, meta, err := NewKeyShares(tt.total, tt.threshold, tt.rsaBits)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
				require.Nil(t, shares)
				require.Nil(t, meta)
			} else {
				require.NoError(t, err)
				require.NotNil(t, shares)
				require.NotNil(t, meta)
				require.GreaterOrEqual(t, meta.PublicKey.N.BitLen(), minRSAPublicKeyBits)
			}
		})
	}
}
