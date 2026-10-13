package crypto

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAESFilesCompatibilitySuffixInputs encrypts ordinary plaintext even when its name ends with the output suffix.
func TestAESFilesCompatibilitySuffixInputs(t *testing.T) {
	for _, suffix := range []string{".enc", ".cipher"} {
		t.Run(suffix, func(t *testing.T) {
			name := "report" + suffix
			dir := writeFilesTestDir(t, map[string]string{name: "ordinary plaintext"})
			require.NoError(t, AESEncryptFilesInDir(dir, filesTestKey, WithAESFilesInDirFileSuffix(suffix)))
			ciphertext, err := os.ReadFile(filepath.Join(dir, name+suffix))
			require.NoError(t, err, "a suffix is not evidence that the source is encrypted")
			plaintext, err := AesDecrypt(filesTestKey, ciphertext)
			require.NoError(t, err)
			require.Equal(t, "ordinary plaintext", string(plaintext))
		})
	}
}
