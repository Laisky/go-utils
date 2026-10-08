package cmd

import (
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/rivo/duplo"
	"github.com/stretchr/testify/require"
)

// writeSyntheticImage renders a deterministic 256x256 test picture (a gradient
// with a bright square at offset) and writes it to path as PNG or JPEG
// depending on the extension. The invert argument produces a visually
// different picture for negative controls.
func writeSyntheticImage(t *testing.T, path string, offset int, invert bool) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for y := range 256 {
		for x := range 256 {
			v := uint8((x + y) / 2)
			if x >= offset && x < offset+96 && y >= offset && y < offset+96 {
				v = 250
			}
			if invert {
				v = 255 - v
			}
			img.Set(x, y, color.RGBA{R: v, G: uint8(x), B: uint8(255 - y), A: 255})
		}
	}

	fp, err := os.Create(path)
	require.NoError(t, err)
	defer func() { require.NoError(t, fp.Close()) }()
	switch filepath.Ext(path) {
	case ".png":
		require.NoError(t, png.Encode(fp, img))
	default:
		require.NoError(t, jpeg.Encode(fp, img, &jpeg.Options{Quality: 95}))
	}
}

// remainingFiles returns the base names of the regular files left in dir.
func remainingFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// TestRemoveDuplicateDetectsSimilarImages verifies that the perceptual
// similarity check removes a re-encoded copy of an image while keeping a
// visually different image. The check previously returned before indexing any
// unmatched image, so the similarity index stayed empty and nothing was ever
// detected.
func TestRemoveDuplicateDetectsSimilarImages(t *testing.T) {
	dir := t.TempDir()
	writeSyntheticImage(t, filepath.Join(dir, "a.png"), 40, false)
	writeSyntheticImage(t, filepath.Join(dir, "b.jpg"), 40, false)
	writeSyntheticImage(t, filepath.Join(dir, "c.png"), 120, true)

	require.NoError(t, removeDuplicate(false, dir))

	left := remainingFiles(t, dir)
	require.Len(t, left, 2, "exactly one of the similar pair must be removed, got %v", left)
	require.Contains(t, left, "c.png", "a dissimilar image must be kept")
}

// TestRemoveDuplicateSimilarImagesDryRunKeepsFiles verifies that dry-run mode
// reports similar images without deleting anything.
func TestRemoveDuplicateSimilarImagesDryRunKeepsFiles(t *testing.T) {
	dir := t.TempDir()
	writeSyntheticImage(t, filepath.Join(dir, "a.png"), 40, false)
	writeSyntheticImage(t, filepath.Join(dir, "b.jpg"), 40, false)

	require.NoError(t, removeDuplicate(true, dir))
	require.Len(t, remainingFiles(t, dir), 2)
}

// TestSimilarImageReplacementKeepsIndexUsable verifies that processing a larger
// re-encoded image replaces a smaller predecessor and lets a later similar image
// match the kept file instead of consulting a deleted path.
func TestSimilarImageReplacementKeepsIndexUsable(t *testing.T) {
	dir := t.TempDir()
	pngPath := filepath.Join(dir, "picture.png")
	jpegPath := filepath.Join(dir, "picture.jpg")
	writeSyntheticImage(t, pngPath, 40, false)
	writeSyntheticImage(t, jpegPath, 40, false)
	pngInfo, err := os.Stat(pngPath)
	require.NoError(t, err)
	jpegInfo, err := os.Stat(jpegPath)
	require.NoError(t, err)
	require.NotEqual(t, pngInfo.Size(), jpegInfo.Size())

	smaller, larger := pngPath, jpegPath
	if pngInfo.Size() > jpegInfo.Size() {
		smaller, larger = jpegPath, pngPath
	}
	third := filepath.Join(dir, "later"+filepath.Ext(smaller))
	content, err := os.ReadFile(smaller)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(third, content, 0600))
	store := duplo.New()
	deleted, err := checkDupByImageSimilar(false, store, smaller)
	require.NoError(t, err)
	require.False(t, deleted)
	deleted, err = checkDupByImageSimilar(false, store, larger)
	require.NoError(t, err)
	require.True(t, deleted)
	require.NoFileExists(t, smaller)
	require.FileExists(t, larger)

	deleted, err = checkDupByImageSimilar(false, store, third)
	require.NoError(t, err, "replacement index must refer to the surviving image")
	require.True(t, deleted)
	require.NoFileExists(t, third)
	require.Equal(t, []string{filepath.Base(larger)}, remainingFiles(t, dir))
}
