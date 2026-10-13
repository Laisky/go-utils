package cmd

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	gcrypto "github.com/Laisky/go-utils/v6/crypto"
)

// TestAESCLICompatibilitySuffixInputs includes ordinary .enc plaintext in both
// modes by default and requires explicit opt-in for suffix-based exclusion.
func TestAESCLICompatibilitySuffixInputs(t *testing.T) {
	for _, passwordMode := range []bool{false, true} {
		name := "raw-key"
		if passwordMode {
			name = "password"
		}
		t.Run(name, func(t *testing.T) {
			setupAESCLITest(t)
			secrets, dir := t.TempDir(), t.TempDir()
			option, value := "--key-file", hex.EncodeToString(aesCLITestKey)
			if passwordMode {
				option, value = "--password-file", aesCLITestPassword
			}
			secretFile := writeSecretFile(t, secrets, "secret", value)
			input := filepath.Join(dir, "report.enc")
			require.NoError(t, os.WriteFile(input, []byte("ordinary plaintext"), 0600))
			require.NoError(t, runAESCLI(t, nil, "encrypt", "aes", "-i", dir, option, secretFile))
			output := input + aesEncryptedSuffix
			encoded, err := os.ReadFile(output)
			require.NoError(t, err, "the suffix must not hide plaintext")
			var plain []byte
			if passwordMode {
				plain, err = gcrypto.DecryptByPassword([]byte(aesCLITestPassword), encoded)
			} else {
				plain, err = gcrypto.AEADDecrypt(aesCLITestKey, encoded, nil)
			}
			require.NoError(t, err)
			require.Equal(t, "ordinary plaintext", string(plain))
			_, err = os.Lstat(output + aesEncryptedSuffix)
			require.ErrorIs(t, err, os.ErrNotExist, "outputs from this run are not fed back as inputs")
			require.NoError(t, runAESCLI(t, nil, "encrypt", "aes", "-i", dir, option, secretFile, "--skip-suffix-inputs"))
			after, err := os.ReadFile(output)
			require.NoError(t, err)
			require.Equal(t, encoded, after, "explicit suffix filtering must leave matching inputs untouched")
			require.ErrorContains(t, runAESCLI(t, nil, "encrypt", "aes", "-i", input, option, secretFile,
				"--skip-suffix-inputs"), "only supported for directory")
		})
	}
}
