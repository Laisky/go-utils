package signature

import (
	"bytes"
	"testing"

	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/go-utils/v4/crypto"
	"github.com/stretchr/testify/require"
)

func TestVerifyBySHA256(t *testing.T) {
	total := 5
	threshold := 3

	keyShares, keyMeta, err := NewKeyShares(total, threshold, crypto.RSAPrikeyBits2048)
	require.NoError(t, err)

	// generate signature by k parts
	parts := gutils.RandomChoice(keyShares, threshold)
	content := gutils.RandomStringWithLength(1024)
	sig, err := SignatureBySHA256(bytes.NewReader([]byte(content)), parts, keyMeta)
	require.NoError(t, err)

	// verify signature
	err = VerifyBySHA256(bytes.NewReader([]byte(content)), keyMeta.PublicKey, sig)
	require.NoError(t, err)
}
