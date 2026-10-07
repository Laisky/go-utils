package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSecurity64SourcePolicy accepts ordinary bytes but rejects final links and directories.
func TestSecurity64SourcePolicy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source")
	data := []byte("synthetic hash input")
	require.NoError(t, os.WriteFile(path, data, 0600))
	expected := sha256.Sum256(data)
	encoded := "sha256:" + hex.EncodeToString(expected[:])
	actual, err := FileHash(HashTypeSha256, path)
	require.NoError(t, err)
	require.Equal(t, expected[:], actual)
	require.NoError(t, VerifyFileHash(path, encoded))
	require.NoError(t, ValidateFileHash(path, encoded))
	require.Error(t, VerifyFileHash(dir, encoded))
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, err = FileHash(HashTypeSha256, link)
	require.Error(t, err, "file hashing must not follow a final symlink")
	require.Error(t, VerifyFileHash(link, encoded))
	require.Error(t, ValidateFileHash(link, encoded))
}
