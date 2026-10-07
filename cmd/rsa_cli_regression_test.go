package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	gcrypto "github.com/Laisky/go-utils/v6/crypto"
)

// runRSACLI executes the root command with args, capturing output, and resets
// the shared rsa flag variables afterwards. It returns the command error.
func runRSACLI(t *testing.T, args ...string) error {
	t.Helper()
	t.Cleanup(func() {
		rsaPrikeyPemFilepath, rsaPubkeyPemFilepath, fileWantToSignature = "", "", ""
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	})
	rootCmd.SetOut(&bytes.Buffer{})
	rootCmd.SetErr(&bytes.Buffer{})
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

// TestRSACLISignThenVerifyThroughFlags verifies that `rsa sign` and
// `rsa verify` both accept the data file through --file/-f. Previously only
// sign registered the flag, so verify could not be pointed at a file from the
// command line, and failures aborted the process through a logger panic.
func TestRSACLISignThenVerifyThroughFlags(t *testing.T) {
	dir := t.TempDir()
	prikey, err := gcrypto.NewRSAPrikey(gcrypto.RSAPrikeyBits3072)
	require.NoError(t, err)
	prikeyPem, err := gcrypto.Prikey2Pem(prikey)
	require.NoError(t, err)
	pubkeyPem, err := gcrypto.Pubkey2Pem(&prikey.PublicKey)
	require.NoError(t, err)

	prikeyFile := filepath.Join(dir, "prikey.pem")
	pubkeyFile := filepath.Join(dir, "pubkey.pem")
	dataFile := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(prikeyFile, prikeyPem, 0o600))
	require.NoError(t, os.WriteFile(pubkeyFile, pubkeyPem, 0o600))
	require.NoError(t, os.WriteFile(dataFile, []byte("signed payload"), 0o600))

	require.NoError(t, runRSACLI(t, "rsa", "sign", "-p", prikeyFile, "-f", dataFile))
	require.NoError(t, runRSACLI(t, "rsa", "verify", "-p", pubkeyFile, "-f", dataFile))

	// A tampered file must fail verification with an error, not a panic.
	require.NoError(t, os.WriteFile(dataFile, []byte("tampered payload"), 0o600))
	var verifyErr error
	require.NotPanics(t, func() {
		verifyErr = runRSACLI(t, "rsa", "verify", "-p", pubkeyFile, "-f", dataFile)
	})
	require.Error(t, verifyErr)
}
