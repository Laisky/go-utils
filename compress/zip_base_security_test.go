package compress

import (
	"archive/zip"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// security62Zip writes a tiny archive with synthetic content in a temporary directory.
func security62Zip(t *testing.T, parent string, names ...string) string {
	t.Helper()
	filename := filepath.Join(parent, "fixture.zip")
	file, err := os.Create(filename)
	require.NoError(t, err)
	writer := zip.NewWriter(file)
	for _, name := range names {
		header := &zip.FileHeader{Name: name, Method: zip.Store}
		header.SetMode(0o600)
		entry, err := writer.CreateHeader(header)
		require.NoError(t, err)
		_, err = entry.Write([]byte("TRAP"))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	require.NoError(t, file.Close())
	return filename
}

// TestSecurity62UnzipEmptyDestination reproduces the original sibling write in a disposable child.
func TestSecurity62UnzipEmptyDestination(t *testing.T) {
	if os.Getenv("GO_UTILS_SECURITY62_CHILD") == "1" {
		_, err := Unzip(os.Getenv("GO_UTILS_SECURITY62_ZIP"), "", UnzipWithMaxBytes(1024))
		require.Error(t, err)
		return
	}
	parent := t.TempDir()
	dest := filepath.Join(parent, "extract")
	require.NoError(t, os.Mkdir(dest, 0o700))
	sentinel := filepath.Join(parent, "outside.txt")
	require.NoError(t, os.WriteFile(sentinel, []byte("SAFE"), 0o600))
	archive := security62Zip(t, parent, "../outside.txt")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSecurity62UnzipEmptyDestination$")
	command.Dir = dest
	// Ensure the utility, rather than an optional stdlib ZIP policy, is tested.
	command.Env = append(os.Environ(), "GO_UTILS_SECURITY62_CHILD=1", "GO_UTILS_SECURITY62_ZIP="+archive, "GODEBUG=zipinsecurepath=1")
	output, runErr := command.CombinedOutput()
	contents, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	require.Equal(t, "SAFE", string(contents))
	require.NoError(t, runErr, string(output))
}

// TestSecurity62ZIPPreflight rejects hostile names before even a preceding safe member is extracted.
func TestSecurity62ZIPPreflight(t *testing.T) {
	for _, member := range []string{"../outside", "/outside", "a/../../outside", "a\\..\\outside", "C:/outside", "C:outside", "//host/share/file", "a//b", "./file"} {
		t.Run(member, func(t *testing.T) {
			parent := t.TempDir()
			dest := filepath.Join(parent, "extract")
			archive := security62Zip(t, parent, "first.txt", member)
			_, err := Unzip(archive, dest, UnzipWithMaxBytes(1024))
			require.Error(t, err)
			_, err = os.Stat(filepath.Join(dest, "first.txt"))
			require.True(t, os.IsNotExist(err))
		})
	}
	parent := t.TempDir()
	dest := filepath.Join(parent, "extract")
	archive := security62Zip(t, parent, "a/file.txt")
	files, err := Unzip(archive, dest, UnzipWithMaxBytes(1024))
	require.NoError(t, err)
	require.Len(t, files, 1)
	contents, err := os.ReadFile(filepath.Join(dest, "a/file.txt"))
	require.NoError(t, err)
	require.Equal(t, "TRAP", string(contents))
}

// TestSecurity62ZipDirectoryCompatibility preserves an empty virtual ZIP prefix
// while using an explicit trusted root internally and portable member names.
func TestSecurity62ZipDirectoryCompatibility(t *testing.T) {
	parent := t.TempDir()
	source := filepath.Join(parent, "source")
	require.NoError(t, os.MkdirAll(filepath.Join(source, "nested"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(source, "nested", "file.txt"), []byte("SAFE"), 0600))
	archive := filepath.Join(parent, "roundtrip.zip")
	require.NoError(t, ZipFiles(archive, []string{source}))
	reader, err := zip.OpenReader(archive)
	require.NoError(t, err)
	require.Len(t, reader.File, 1)
	require.Equal(t, "source/nested/file.txt", reader.File[0].Name)
	require.NoError(t, reader.Close())
	dest := filepath.Join(parent, "extract")
	_, err = Unzip(archive, dest, UnzipWithMaxBytes(1024))
	require.NoError(t, err)
	content, err := os.ReadFile(filepath.Join(dest, "source", "nested", "file.txt"))
	require.NoError(t, err)
	require.Equal(t, "SAFE", string(content))
}
