package gorm

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGzTextScanDecompressionBombProtection(t *testing.T) {
	t.Parallel()

	t.Run("normal data", func(t *testing.T) {
		t.Parallel()
		original := "hello world"
		gz := GzText(original)
		val, err := gz.Value()
		require.NoError(t, err)

		var result GzText
		err = result.Scan(val)
		require.NoError(t, err)
		require.Equal(t, GzText(original), result)
	})

	t.Run("empty data", func(t *testing.T) {
		t.Parallel()
		var result GzText
		err := result.Scan([]byte{})
		require.NoError(t, err)
		require.Equal(t, GzText(""), result)
	})

	t.Run("exceeds max decompressed size", func(t *testing.T) {
		t.Parallel()
		// Create a gzip payload that decompresses to > 64 MiB
		// by compressing a large repeated string (compresses very well)
		const maxSize = 64 * 1024 * 1024
		bigData := strings.Repeat("A", maxSize+1024)

		var buf bytes.Buffer
		w := gzip.NewWriter(&buf)
		_, err := w.Write([]byte(bigData))
		require.NoError(t, err)
		require.NoError(t, w.Close())

		var result GzText
		err = result.Scan(buf.Bytes())
		require.ErrorContains(t, err, "decompressed data exceeds maximum size")
	})

	t.Run("string input", func(t *testing.T) {
		t.Parallel()
		original := "test string input"
		gz := GzText(original)
		val, err := gz.Value()
		require.NoError(t, err)

		// Scan from string type
		var result GzText
		err = result.Scan(string(val.([]byte)))
		require.NoError(t, err)
		require.Equal(t, GzText(original), result)
	})
}
