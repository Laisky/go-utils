package compress

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// security56Archive writes tiny stored members and returns their archive filename.
func security56Archive(t *testing.T, entries ...string) string {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for i, content := range entries {
		name := string(rune('a'+i)) + ".txt"
		h := &zip.FileHeader{Name: name, Method: zip.Store}
		h.SetMode(0600)
		w, err := zw.CreateHeader(h)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	path := filepath.Join(t.TempDir(), "input.zip")
	require.NoError(t, os.WriteFile(path, b.Bytes(), 0600))
	return path
}

// TestSecurity56DefaultBytes rejects advertised oversized members without inflating them.
func TestSecurity56DefaultBytes(t *testing.T) {
	path := security56Archive(t, "tiny")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	central := bytes.Index(data, []byte{'P', 'K', 1, 2})
	require.GreaterOrEqual(t, central, 0)
	// A small, syntactically valid directory advertises 128 MiB. No such payload
	// is allocated or decompressed in this bounded default-policy test.
	binary.LittleEndian.PutUint32(data[central+24:central+28], 128*1024*1024)
	require.NoError(t, os.WriteFile(path, data, 0600))
	dest := filepath.Join(t.TempDir(), "out")
	_, err = Unzip(path, dest)
	require.Error(t, err)
	_, statErr := os.Stat(dest)
	require.True(t, os.IsNotExist(statErr), "default byte policy must reject before filesystem writes")
}

// TestSecurity56BudgetPreservesTarget reproduces destructive overflow using ten bytes.
func TestSecurity56BudgetPreservesTarget(t *testing.T) {
	path := security56Archive(t, "0123456789")
	for _, exists := range []bool{false, true} {
		t.Run(fmt.Sprint(exists), func(t *testing.T) {
			dest := t.TempDir()
			target := filepath.Join(dest, "a.txt")
			if exists {
				require.NoError(t, os.WriteFile(target, []byte("SENTINEL"), 0600))
			}
			_, err := Unzip(path, dest, UnzipWithMaxBytes(7))
			require.Error(t, err)
			data, err := os.ReadFile(target)
			if exists {
				require.NoError(t, err)
				require.Equal(t, "SENTINEL", string(data))
			} else {
				require.True(t, os.IsNotExist(err))
			}
			temps, err := filepath.Glob(filepath.Join(dest, ".unzip-*"))
			require.NoError(t, err)
			require.Empty(t, temps)
		})
	}
}

// TestSecurity56ChecksumPreservesTarget never publishes bytes with a bad checksum.
func TestSecurity56ChecksumPreservesTarget(t *testing.T) {
	path := security56Archive(t, "bounded data")
	zr, err := zip.OpenReader(path)
	require.NoError(t, err)
	offset, err := zr.File[0].DataOffset()
	require.NoError(t, err)
	require.NoError(t, zr.Close())
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data[offset] ^= 1
	require.NoError(t, os.WriteFile(path, data, 0600))
	dest := t.TempDir()
	target := filepath.Join(dest, "a.txt")
	require.NoError(t, os.WriteFile(target, []byte("SENTINEL"), 0600))
	_, err = Unzip(path, dest)
	require.ErrorIs(t, err, zip.ErrChecksum)
	actual, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "SENTINEL", string(actual))
	names, err := os.ReadDir(dest)
	require.NoError(t, err)
	require.Len(t, names, 1)
}
