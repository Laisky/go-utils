package compress

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// security70Link creates only disposable links and skips hosts without link privileges.
func security70Link(t *testing.T, target, name string) {
	t.Helper()
	if err := os.Symlink(target, name); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
}

// TestSecurity70SourceLinks proves selected aliases do not disclose sibling files.
func TestSecurity70SourceLinks(t *testing.T) {
	for _, kind := range []string{"file", "directory", "internal", "dangling", "root"} {
		t.Run(kind, func(t *testing.T) {
			parent := t.TempDir()
			selected := filepath.Join(parent, "selected")
			require.NoError(t, os.Mkdir(selected, 0700))
			outside := filepath.Join(parent, "outside")
			require.NoError(t, os.Mkdir(outside, 0700))
			secret := filepath.Join(outside, "marker.txt")
			require.NoError(t, os.WriteFile(secret, []byte("SYNTHETIC_OUTSIDE_MARKER"), 0600))
			input := selected
			switch kind {
			case "file":
				security70Link(t, secret, filepath.Join(selected, "alias"))
			case "directory":
				security70Link(t, outside, filepath.Join(selected, "alias"))
			case "internal":
				require.NoError(t, os.WriteFile(filepath.Join(selected, "ordinary"), []byte("ordinary"), 0600))
				security70Link(t, "ordinary", filepath.Join(selected, "alias"))
			case "dangling":
				security70Link(t, filepath.Join(outside, "absent"), filepath.Join(selected, "alias"))
			case "root":
				input = filepath.Join(parent, "root-alias")
				security70Link(t, selected, input)
			}
			output := filepath.Join(parent, "archive.zip")
			require.NoError(t, os.WriteFile(output, []byte("KEEP_EXISTING_ARCHIVE"), 0600))
			err := ZipFiles(output, []string{input})
			require.Error(t, err, "source symlink must fail closed")
			after, readErr := os.ReadFile(output)
			require.NoError(t, readErr)
			require.Equal(t, "KEEP_EXISTING_ARCHIVE", string(after), "failure must not publish partial archive")
		})
	}
}

// TestSecurity70SelectedSymlink checks the exported incremental API's source policy.
func TestSecurity70SelectedSymlink(t *testing.T) {
	parent := t.TempDir()
	regular := filepath.Join(parent, "regular")
	require.NoError(t, os.WriteFile(regular, []byte("SYNTHETIC_OUTSIDE_MARKER"), 0600))
	alias := filepath.Join(parent, "alias")
	security70Link(t, regular, alias)
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	err := AddFileToZip(writer, alias, "")
	closeErr := writer.Close()
	require.NoError(t, closeErr)
	require.Error(t, err)
	reader, readErr := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, readErr)
	for _, file := range reader.File {
		src, openErr := file.Open()
		require.NoError(t, openErr)
		data, readErr := io.ReadAll(src)
		require.NoError(t, readErr)
		require.NoError(t, src.Close())
		require.NotContains(t, string(data), "SYNTHETIC_OUTSIDE_MARKER")
	}
}

// TestSecurity70OrdinaryRoundTrip preserves nested filenames and regular content.
func TestSecurity70OrdinaryRoundTrip(t *testing.T) {
	parent := t.TempDir()
	selected := filepath.Join(parent, "selected")
	require.NoError(t, os.MkdirAll(filepath.Join(selected, "nested"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(selected, "nested", "hello.txt"), []byte("hello"), 0600))
	output := filepath.Join(parent, "archive.zip")
	require.NoError(t, ZipFiles(output, []string{selected}))
	r, err := zip.OpenReader(output)
	require.NoError(t, err)
	defer r.Close()
	require.Len(t, r.File, 1)
	require.Equal(t, "selected/nested/hello.txt", r.File[0].Name)
	f, err := r.File[0].Open()
	require.NoError(t, err)
	data, err := io.ReadAll(f)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.Equal(t, "hello", string(data))
}

// TestSecurity70NoDisclosure checks actual archive bytes against outside sentinels.
func TestSecurity70NoDisclosure(t *testing.T) {
	for _, directory := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "directory"}[directory], func(t *testing.T) {
			parent := t.TempDir()
			selected := filepath.Join(parent, "selected")
			outside := filepath.Join(parent, "outside")
			require.NoError(t, os.Mkdir(selected, 0700))
			require.NoError(t, os.Mkdir(outside, 0700))
			marker := filepath.Join(outside, "marker")
			require.NoError(t, os.WriteFile(marker, []byte("SYNTHETIC_OUTSIDE_MARKER"), 0600))
			target := marker
			if directory {
				target = outside
			}
			security70Link(t, target, filepath.Join(selected, "alias"))
			output := filepath.Join(parent, "archive.zip")
			if err := ZipFiles(output, []string{selected}); err != nil {
				_, statErr := os.Lstat(output)
				require.True(t, os.IsNotExist(statErr), "rejected archive must not be published")
				return
			}
			r, err := zip.OpenReader(output)
			require.NoError(t, err)
			defer r.Close()
			for _, f := range r.File {
				reader, err := f.Open()
				require.NoError(t, err)
				data, err := io.ReadAll(reader)
				require.NoError(t, err)
				require.NoError(t, reader.Close())
				require.NotContains(t, string(data), "SYNTHETIC_OUTSIDE_MARKER", "archive disclosed unselected bytes")
			}
		})
	}
}
