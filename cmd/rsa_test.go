package cmd

import (
	"os"
	"path/filepath"
	"testing"

	gutils "github.com/Laisky/go-utils/v3"
	gencrypt "github.com/Laisky/go-utils/v3/encrypt"
	"github.com/stretchr/testify/require"
)

func Test_signFileByRSA(t *testing.T) {
	dir, err := os.MkdirTemp("", "*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	prikeyFile := filepath.Join(dir, "prikey")
	pubkeyFile := filepath.Join(dir, "pub")

	t.Run("prepare keys", func(t *testing.T) {
		prikey, err := gencrypt.NewRSAPrikey(gencrypt.RSAPrikeyBits3072)
		require.NoError(t, err)

		prikeyPem, err := gencrypt.Prikey2Pem(prikey)
		require.NoError(t, err)
		err = os.WriteFile(prikeyFile, prikeyPem, 0644)
		require.NoError(t, err)

		pubkeyPem, err := gencrypt.Pubkey2Pem(&prikey.PublicKey)
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