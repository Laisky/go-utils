package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	gutils "github.com/Laisky/go-utils/v4"
	gcrypto "github.com/Laisky/go-utils/v4/crypto"
)

func Test_signFileByRSA(t *testing.T) {
	dir, err := os.MkdirTemp("", "*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	prikeyFile := filepath.Join(dir, "prikey")
	pubkeyFile := filepath.Join(dir, "pub")

	t.Run("prepare keys", func(t *testing.T) {
		prikey, err := gcrypto.NewRSAPrikey(gcrypto.RSAPrikeyBits3072)
		require.NoError(t, err)

		prikeyPem, err := gcrypto.Prikey2Pem(prikey)
		require.NoError(t, err)
		err = os.WriteFile(prikeyFile, prikeyPem, 0644)
		require.NoError(t, err)

		pubkeyPem, err := gcrypto.Pubkey2Pem(&prikey.PublicKey)
		require.NoError(t, err)
		err = os.WriteFile(pubkeyFile, pubkeyPem, 0644)
		require.NoError(t, err)
	})

	dataFile := filepath.Join(dir, "data.txt")
	t.Run("write data file", func(t *testing.T) {
		data, err := gutils.RandomBytesWithLength(100)
		require.NoError(t, err)
		err = os.WriteFile(dataFile, data, 0644)
		require.NoError(t, err)
	})

	t.Run("sign", func(t *testing.T) {
		err := SignFileByRSA(prikeyFile, dataFile)
		require.NoError(t, err)
	})

	t.Run("verify", func(t *testing.T) {
		err := VerifyFileByRSA(pubkeyFile, dataFile)
		require.NoError(t, err)
	})

}
