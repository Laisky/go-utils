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

// TestAESFilesCompatibilityOverlappingInputs encrypts every original input before an overlapping output can replace it.
func TestAESFilesCompatibilityOverlappingInputs(t *testing.T) {
	inputs := map[string]string{"a": "first", "a.enc": "second", "a.enc.enc": "third"}
	for range 4 {
		dir := writeFilesTestDir(t, inputs)
		require.NoError(t, AESEncryptFilesInDir(dir, filesTestKey, WithAESFilesInDirMaxConcurrency(4)))
		for name, original := range inputs {
			ct, err := os.ReadFile(filepath.Join(dir, name+".enc"))
			require.NoError(t, err)
			plain, err := AesDecrypt(filesTestKey, ct)
			require.NoError(t, err)
			require.Equal(t, original, string(plain), name)
		}
		requireNoStagingFiles(t, dir)
	}
}
