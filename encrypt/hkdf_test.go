package encrypt

import (
	"crypto/rand"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHKDFWithSHA256(t *testing.T) {
	key := make([]byte, 16)
	_, err := rand.Read(key)
	require.NoError(t, err)

	salt := make([]byte, sha256.Size)
	_, err = rand.Read(salt)
	require.NoError(t, err)

	results1 := make([][]byte, 10)
	for i := range results1 {
		results1[i] = make([]byte, 20)
	}
	HKDFWithSHA256(key, salt, nil, results1)

	results2 := make([][]byte, 10)
	for i := range results2 {
		results2[i] = make([]byte, 20)
	}
	HKDFWithSHA256(key, salt, nil, results2)

	// same key & salt will derivate same keys
	require.Len(t, results1[0], 20)
	require.Len(t, results2[0], 20)
	require.Equal(t, results1[0], results2[0])
	require.Equal(t, results1[1], results2[1])
	require.Equal(t, results1[2], results2[2])
}
