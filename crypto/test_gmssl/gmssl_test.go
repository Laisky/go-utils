package testgmssl

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	gmssl "github.com/GmSSL/GmSSL-Go"
	"github.com/stretchr/testify/require"

	gcrypto "github.com/Laisky/go-utils/v6/crypto"
)

// Test_HashBySm3 verifies SM3 interoperability: for inputs that cover the empty
// message, the SM3 padding boundaries (55, 56, 64 bytes) and 1 MiB of random
// data, the digest computed by Tongsuo.HashBySm3 (the tongsuo binary) equals the
// digest computed by the GmSSL C library.
func Test_HashBySm3(t *testing.T) {
	t.Parallel()
	ins := newTongsuo(t)

	for _, n := range []int{0, 1, 55, 56, 64, 1 << 20} {
		t.Run(fmt.Sprintf("len=%d", n), func(t *testing.T) {
			raw := randomBytes(t, n)

			gmsslSm3 := gmssl.NewSm3()
			gmsslSm3.Update(raw)
			digestByGmssl := gmsslSm3.Digest()

			digestByTongsuo, err := ins.HashBySm3(t.Context(), raw)
			require.NoError(t, err)
			require.Len(t, digestByTongsuo, 32)
			require.Equal(t, digestByGmssl, digestByTongsuo)
		})
	}
}

// TestTongsuo_SignBySM2SM3 verifies SM2-with-SM3 signature interoperability
// (default user ID "1234567812345678") over 1 MiB of random data in both
// directions: a GmSSL signature verifies with Tongsuo.VerifyBySm2Sm3, and a
// Tongsuo.SignBySm2Sm3 signature verifies with GmSSL against the public key
// from Tongsuo.Prikey2Pubkey. Each side must also reject a modified signature
// and a modified message.
func TestTongsuo_SignBySM2SM3(t *testing.T) {
	t.Parallel()
	ins := newTongsuo(t)
	plaintext := randomBytes(t, 1<<20)

	t.Run("gmssl -> go-utils", func(t *testing.T) {
		ctx := t.Context()
		gmsslPrikey, err := gmssl.GenerateSm2Key()
		require.NoError(t, err)

		pubkeyPath := filepath.Join(t.TempDir(), "pubkey.pem")
		require.NoError(t, gmsslPrikey.ExportPublicKeyInfoPem(pubkeyPath))
		pubkeyPem, err := os.ReadFile(pubkeyPath)
		require.NoError(t, err)

		signer, err := gmssl.NewSm2Signature(gmsslPrikey, gmssl.Sm2DefaultId, true)
		require.NoError(t, err)
		require.NoError(t, signer.Update(plaintext))
		signature, err := signer.Sign()
		require.NoError(t, err)

		require.NoError(t, ins.VerifyBySm2Sm3(ctx, pubkeyPem, signature, plaintext))

		err = ins.VerifyBySm2Sm3(ctx, pubkeyPem, flipLastByte(signature), plaintext)
		require.ErrorIs(t, err, gcrypto.ErrSm2SignatureVerification)
		err = ins.VerifyBySm2Sm3(ctx, pubkeyPem, signature, flipLastByte(plaintext))
		require.ErrorIs(t, err, gcrypto.ErrSm2SignatureVerification)
	})

	t.Run("go-utils -> gmssl", func(t *testing.T) {
		ctx := t.Context()
		prikeyPem, err := ins.NewPrikey(ctx)
		require.NoError(t, err)
		signature, err := ins.SignBySm2Sm3(ctx, prikeyPem, plaintext)
		require.NoError(t, err)
		pubkeyPem, err := ins.Prikey2Pubkey(ctx, prikeyPem)
		require.NoError(t, err)

		require.True(t, gmsslSm2Verify(t, pubkeyPem, plaintext, signature))
		// Every negative check uses a fresh GmSSL verifier; a consumed verifier
		// rejects even a valid signature, which would make these checks vacuous.
		require.False(t, gmsslSm2Verify(t, pubkeyPem, plaintext, flipLastByte(signature)))
		require.False(t, gmsslSm2Verify(t, pubkeyPem, flipLastByte(plaintext), signature))
	})
}
