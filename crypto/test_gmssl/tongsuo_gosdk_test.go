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
// The key is a PKCS#8 "ENCRYPTED PRIVATE KEY" (PBES2: PBKDF2-HMAC-SM3 with a
// high iteration count + SM4-CBC). tongsuo-go-sdk must decrypt it with the
// right password, reject a wrong one, and yield a usable SM2 key: its public
// key must match Tongsuo.Prikey2Pubkey, and a Tongsuo.SignBySm2Sm3 signature
// made with it must verify in GmSSL.
//
// GmSSL 3.1.1 cannot import it: Tongsuo (like OpenSSL 3) omits the optional
// [0] curve parameters from the inner ECPrivateKey, as RFC 5915 section 3
// recommends for PKCS#8, while GmSSL's sm2_private_key_from_der requires them;
// GmSSL also names HMAC-SM3 with the older OID 1.2.156.10197.1.401.2. The
// rejection is asserted next to a positive control showing GmSSL imports its
// own PKCS#8 key, so a future GmSSL that accepts the format is noticed. The
// PBKDF2-HMAC-SM3 content itself is cross-checked with the independent gmsm
// parser in the parent module (TestTongsuoNewPrikeyWithPasswordUsesPKCS8PBES2).
func TestTongsuo_NewPrikeyWithPassword(t *testing.T) {
	t.Parallel()
	ins := newTongsuo(t)
	ctx := t.Context()

	password := "test-password"
	encryptedPem, err := ins.NewPrikeyWithPassword(ctx, password)
	require.NoError(t, err)

	block, rest := pem.Decode(encryptedPem)
	require.NotNil(t, block)
	require.Empty(t, strings.TrimSpace(string(rest)))
	require.Equal(t, "ENCRYPTED PRIVATE KEY", block.Type)
	require.Empty(t, block.Headers)

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

	t.Run("gmssl rejects pkcs8 without ECPrivateKey parameters", func(t *testing.T) {
		dir := t.TempDir()

		// Positive control: GmSSL round-trips its own PKCS#8 encrypted key, so
		// the rejection below is caused by the key encoding, not the import path.
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
