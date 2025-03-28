package signature

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	gutils "github.com/Laisky/go-utils/v5"
)

func TestVerifyBySHA256(t *testing.T) {
	t.Parallel()

	// Increase test timeout
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	total := 5
	threshold := 3

	// Generate key shares once
	keyShares, keyMeta, err := NewKeyShares(total, threshold, 1024) // Using smaller key size for tests
	require.NoError(t, err)

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

func TestNewKeyShares_IntegerOverflow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		total     int
		threshold int
		wantErr   string
	}{
		{
			name:      "valid values",
			total:     5,
			threshold: 3,
			wantErr:   "",
		},
		{
			name:      "overflow uint16 max",
			total:     70000,
			threshold: 65536,
			wantErr:   "threshold and total must not exceed 65535",
		},
		{
			name:      "large but valid values",
			total:     1000,
			threshold: 501,
			wantErr:   "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			shares, meta, err := NewKeyShares(tt.total, tt.threshold, 1024)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
				require.Nil(t, shares)
				require.Nil(t, meta)
			} else {
				require.NoError(t, err)
				require.NotNil(t, shares)
				require.NotNil(t, meta)
			}
		})
	}
}
