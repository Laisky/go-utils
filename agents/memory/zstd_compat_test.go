package memory

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

// TestZstdVersionInteroperability checks a frozen frame produced by the
// v1.20.1 writer and the current writer against the decoder.
func TestZstdVersionInteroperability(t *testing.T) {
	message := strings.Repeat("Cross-version memory archive: hello, zstd!\n", 1000)
	// This frame was produced by compressZstd with compress v1.20.1. The
	// v1.20.3 writer also produced these bytes during rollback qualification.
	const historicalFrame = "28b52ffd64f8a6ad0100b40243726f73732d76657273696f6e206d656d6f727920617263686976653a2068656c6c6f2c207a737464210a0154170533533eb9ec911024"

	oldFrame, err := hex.DecodeString(historicalFrame)
	require.NoError(t, err)
	newFrame, err := compressZstd(message)
	require.NoError(t, err)

	decoder, err := zstd.NewReader(nil)
	require.NoError(t, err)
	defer decoder.Close()
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"historical writer", oldFrame},
		{"current writer", newFrame},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decoder.DecodeAll(tc.data, nil)
			require.NoError(t, err)
			require.Equal(t, message, string(got))
		})
	}
}
