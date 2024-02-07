package testgmssl

import (
	"context"
	"os/exec"
	"testing"

	gmssl "github.com/GmSSL/GmSSL-Go"
	"github.com/stretchr/testify/require"

	gcrypto "github.com/Laisky/go-utils/v4/crypto"
)

func testSkipSmTongsuo(t *testing.T) (skipped bool) {
	t.Helper()
	if _, err := exec.LookPath("tongsuo"); err != nil {
		require.ErrorIs(t, err, exec.ErrNotFound)
		return true
	}

	return false
}

func Test_HashBySm3(t *testing.T) {
	t.Parallel()
	if testSkipSmTongsuo(t) {
		return
	}

	raw, err := gcrypto.Salt(1024 * 1024)
	require.NoError(t, err)

	gmsslSm3 := gmssl.NewSm3()
	gmsslSm3.Update(raw)
	sigByGmssl := gmsslSm3.Digest()

	ctx := context.Background()
	ins, err := gcrypto.NewTongsuo("/usr/local/bin/tongsuo")
	require.NoError(t, err)

	sigByTongsuo, err := ins.HashBySm3(ctx, raw)
	require.NoError(t, err)

	require.Equal(t, sigByGmssl, sigByTongsuo)
}
