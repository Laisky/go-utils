package compress

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// reportArchive writes a ZIP holding the single tiny member report.txt.
// It takes the test handle and returns the archive path.
func reportArchive(t *testing.T) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "report.zip")
	file, err := os.Create(name)
	require.NoError(t, err)
	writer := zip.NewWriter(file)
	entry, err := writer.Create("report.txt")
	require.NoError(t, err)
	_, err = entry.Write([]byte("REPORT"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, file.Close())
	return name
}

// TestUnzipReportLinkRecipe follows the issue #46 recipe with a positive 1 KiB
// cap: a final link, a dangling final link and a linked parent directory must
// never redirect the write. Regression for issue #46 (already fixed by rooted
// publication; this test guards the behavior).
func TestUnzipReportLinkRecipe(t *testing.T) {
	cases := map[string]func(t *testing.T, dest, sentinel, outside string){
		"final-link": func(t *testing.T, dest, sentinel, _ string) {
			require.NoError(t, os.Symlink(sentinel, filepath.Join(dest, "report.txt")))
		},
		"dangling-link": func(t *testing.T, dest, _, outside string) {
			require.NoError(t, os.Symlink(filepath.Join(outside, "created.txt"), filepath.Join(dest, "report.txt")))
		},
	}
	for name, plant := range cases {
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			dest, outside := filepath.Join(parent, "extract"), filepath.Join(parent, "outside")
			require.NoError(t, os.Mkdir(dest, 0o700))
			require.NoError(t, os.Mkdir(outside, 0o700))
			sentinel := filepath.Join(outside, "sentinel.txt")
			require.NoError(t, os.WriteFile(sentinel, []byte("KEEP"), 0o600))
			plant(t, dest, sentinel, outside)

			_, err := Unzip(reportArchive(t), dest, UnzipWithMaxBytes(1024))
			require.NoError(t, err)
			data, err := os.ReadFile(sentinel)
			require.NoError(t, err)
			require.Equal(t, "KEEP", string(data))
			entries, err := os.ReadDir(outside)
			require.NoError(t, err)
			require.Len(t, entries, 1, "nothing may be created outside the destination")
			info, err := os.Lstat(filepath.Join(dest, "report.txt"))
			require.NoError(t, err)
			require.True(t, info.Mode().IsRegular(), "the link entry is replaced, not followed")
		})
	}

	t.Run("linked-parent", func(t *testing.T) {
		parent := t.TempDir()
		root, outside := filepath.Join(parent, "root"), filepath.Join(parent, "outside")
		require.NoError(t, os.Mkdir(root, 0o700))
		require.NoError(t, os.Mkdir(outside, 0o700))
		sentinel := filepath.Join(outside, "report.txt")
		require.NoError(t, os.WriteFile(sentinel, []byte("KEEP"), 0o600))
		require.NoError(t, os.Symlink(outside, filepath.Join(root, "extract")))

		archive := filepath.Join(t.TempDir(), "nested.zip")
		file, err := os.Create(archive)
		require.NoError(t, err)
		writer := zip.NewWriter(file)
		entry, err := writer.Create("extract/report.txt")
		require.NoError(t, err)
		_, err = entry.Write([]byte("REPORT"))
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		require.NoError(t, file.Close())

		_, err = Unzip(archive, root, UnzipWithMaxBytes(1024))
		require.Error(t, err)
		data, err := os.ReadFile(sentinel)
		require.NoError(t, err)
		require.Equal(t, "KEEP", string(data))
	})
}
