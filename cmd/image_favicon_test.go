package cmd

import (
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCropCenterSquare verifies that cropCenterSquare returns a centered square and respects dimensions.
// It uses the testing parameter to report failures.
func TestCropCenterSquare(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 120, 60))
	src.Set(10, 10, color.RGBA{R: 255, A: 255})

	square := cropCenterSquare(src)
	require.Equal(t, 60, square.Bounds().Dx())
	require.Equal(t, 60, square.Bounds().Dy())
}

// TestEncodeICO verifies that encodeICO produces a valid ICO header and offsets.
// It uses the testing parameter to report failures.
func TestEncodeICO(t *testing.T) {
	entries := []faviconEntry{
		{size: 16, data: []byte{1, 2, 3}},
		{size: 32, data: []byte{4, 5, 6, 7}},
	}

	data, err := encodeICO(entries)
	require.NoError(t, err)
	require.Greater(t, len(data), 6+16*len(entries))

	require.Equal(t, uint16(0), binary.LittleEndian.Uint16(data[0:2]))
	require.Equal(t, uint16(1), binary.LittleEndian.Uint16(data[2:4]))
	require.Equal(t, uint16(2), binary.LittleEndian.Uint16(data[4:6]))

	firstEntry := data[6 : 6+16]
	secondEntry := data[6+16 : 6+32]
	require.Equal(t, uint32(3), binary.LittleEndian.Uint32(firstEntry[8:12]))
	require.Equal(t, uint32(4), binary.LittleEndian.Uint32(secondEntry[8:12]))

	firstOffset := binary.LittleEndian.Uint32(firstEntry[12:16])
	secondOffset := binary.LittleEndian.Uint32(secondEntry[12:16])
	require.Equal(t, uint32(6+16*len(entries)), firstOffset)
	require.Equal(t, firstOffset+3, secondOffset)
}

// TestGenerateFaviconFile verifies that generateFaviconFile writes a new favicon file without changing the input.
// It uses the testing parameter to report failures.
func TestGenerateFaviconFile(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "source.png")
	sourceImg := image.NewRGBA(image.Rect(0, 0, 64, 32))

	inputFile, err := os.Create(inputPath)
	require.NoError(t, err)
	require.NoError(t, png.Encode(inputFile, sourceImg))
	require.NoError(t, inputFile.Close())

	originalBytes, err := os.ReadFile(inputPath)
	require.NoError(t, err)

	outputPath, err := generateFaviconFile(imageFaviconOptions{
		Input:  inputPath,
		Output: "",
		Sizes:  []int{16, 32},
		Format: faviconFormatICO,
		Force:  false,
	})
	require.NoError(t, err)
	require.NotEmpty(t, outputPath)
	require.FileExists(t, outputPath)
	require.NotEqual(t, inputPath, outputPath)
	require.Equal(t, filepath.Dir(inputPath), filepath.Dir(outputPath))

	afterBytes, err := os.ReadFile(inputPath)
	require.NoError(t, err)
	require.Equal(t, originalBytes, afterBytes)
}
