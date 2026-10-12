package cmd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	gcrypto "github.com/Laisky/go-utils/v6/crypto"
)

// TestAESDirectoryPasswordAdmissionContext verifies that cancellation reaches
// password-key admission before KDF validation or derivation can occur.
func TestAESDirectoryPasswordAdmissionContext(t *testing.T) {
	original := aesCLIPasswordKDFParams
	t.Cleanup(func() { aesCLIPasswordKDFParams = original })
	// Invalid options are a cheap sentinel: a discarded context would reach
	// their validation instead of returning cancellation at KDF admission.
	aesCLIPasswordKDFParams = gcrypto.PasswordKDFParams{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := encryptDirByAESCLI(ctx, t.TempDir(), &aesCLISecret{password: []byte("fixture")})
	require.ErrorIs(t, err, context.Canceled)
}
