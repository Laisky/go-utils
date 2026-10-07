package testgmssl

import (
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gmssl "github.com/GmSSL/GmSSL-Go"
	"github.com/stretchr/testify/require"
	tscrypto "github.com/tongsuo-project/tongsuo-go-sdk/crypto"
)

// TestTongsuo_NewPrikeyWithPassword verifies the password-encrypted SM2 private
// key produced by Tongsuo.NewPrikeyWithPassword.
//
// The key uses OpenSSL "traditional" PEM encryption ("EC PRIVATE KEY" with
// Proc-Type/DEK-Info SM4-CBC headers). tongsuo-go-sdk must decrypt it with the
// right password, reject a wrong one, and yield a usable SM2 key: its public
// key must match Tongsuo.Prikey2Pubkey, and a Tongsuo.SignBySm2Sm3 signature
// made with it must verify in GmSSL. GmSSL itself only imports PKCS#8
// "ENCRYPTED PRIVATE KEY" (PBKDF2-HMAC-SM3 + SM4-CBC), so it is expected to
// reject this format even after the PEM is re-encoded without any extra text.
func TestTongsuo_NewPrikeyWithPassword(t *testing.T) {
	t.Parallel()
	ins := newTongsuo(t)
	ctx := t.Context()

	password := "test-password"
	encryptedPem, err := ins.NewPrikeyWithPassword(ctx, password)
	require.NoError(t, err)

	block, _ := pem.Decode(encryptedPem)
	require.NotNil(t, block)
	require.Equal(t, "EC PRIVATE KEY", block.Type)
	require.Equal(t, "4,ENCRYPTED", block.Headers["Proc-Type"])
	require.True(t, strings.HasPrefix(block.Headers["DEK-Info"], "SM4-CBC,"),
		"unexpected DEK-Info %q", block.Headers["DEK-Info"])

	t.Run("tongsuo-go-sdk", func(t *testing.T) {
		ctx := t.Context()
		key, err := tscrypto.LoadPrivateKeyFromPEMWithPassword(encryptedPem, password)
		require.NoError(t, err)
		_, err = tscrypto.LoadPrivateKeyFromPEMWithPassword(encryptedPem, "wrong-"+password)
		require.Error(t, err)

		plainPrikeyPem, err := key.MarshalPKCS1PrivateKeyPEM()
		require.NoError(t, err)
		pubkeyPem, err := key.MarshalPKIXPublicKeyPEM()
		require.NoError(t, err)

		wantPubkeyPem, err := ins.Prikey2Pubkey(ctx, plainPrikeyPem)
		require.NoError(t, err)
		require.Equal(t, pemBlockBytes(t, wantPubkeyPem), pemBlockBytes(t, pubkeyPem))

		msg := randomBytes(t, 4096)
		signature, err := ins.SignBySm2Sm3(ctx, plainPrikeyPem, msg)
		require.NoError(t, err)
		require.True(t, gmsslSm2Verify(t, pubkeyPem, msg, signature))
	})

	t.Run("gmssl rejects traditional format", func(t *testing.T) {
		dir := t.TempDir()

		// Positive control: GmSSL round-trips its own PKCS#8 encrypted key, so
		// the rejection below is caused by the key format, not the import path.
		gmsslKey, err := gmssl.GenerateSm2Key()
		require.NoError(t, err)
		pkcs8Path := filepath.Join(dir, "gmssl-pkcs8.pem")
		require.NoError(t, gmsslKey.ExportEncryptedPrivateKeyInfoPem(password, pkcs8Path))
		_, err = gmssl.ImportSm2EncryptedPrivateKeyInfoPem(password, pkcs8Path)
		require.NoError(t, err)

		prikeyPath := filepath.Join(dir, "prikey.pem")
		require.NoError(t, os.WriteFile(prikeyPath, pem.EncodeToMemory(block), 0o600))
		_, err = gmssl.ImportSm2EncryptedPrivateKeyInfoPem(password, prikeyPath)
		require.ErrorContains(t, err, "Libgmssl inner error")
	})
}
