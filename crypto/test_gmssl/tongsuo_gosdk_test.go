package testgmssl

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	gmssl "github.com/GmSSL/GmSSL-Go"
	"github.com/stretchr/testify/require"
	tscrypto "github.com/tongsuo-project/tongsuo-go-sdk/crypto"

	gcrypto "github.com/Laisky/go-utils/v4/crypto"
)

func TestTongsuo_NewPrikeyWithPassword(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	ctx := context.Background()
	ins, err := gcrypto.NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	password := "test-password"
	prikeyPem, err := ins.NewPrikeyWithPassword(ctx, password)
	require.NoError(t, err)
	require.NotNil(t, prikeyPem)

	_, err = tscrypto.LoadPrivateKeyFromPEMWithPassword(prikeyPem, password)
	require.NoError(t, err)

	dir, err := os.MkdirTemp("", "tongsuo*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	prikeyPemPath := filepath.Join(dir, "prikey.pem")
	err = os.WriteFile(prikeyPemPath, []byte(prikeyPem), 0644)
	require.NoError(t, err)

	_, err = gmssl.ImportSm2EncryptedPrivateKeyInfoPem(password, prikeyPemPath)
	// require.NoError(t, err)
	require.ErrorContains(t, err, "Libgmssl inner error") // TODO: gmssl does not support prikey encrypted by tongsuo
}
