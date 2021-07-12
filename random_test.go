package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRandomStringWithLength(t *testing.T) {
	for i := 0; i < 10; i++ {
		n, err := SecRandInt(1000)
		if err != nil {
			require.NoError(t, err)
		}

		ret := RandomStringWithLength(n)
		require.Len(t, ret, n)

		ret, err = SecRandomStringWithLength(n)
		require.NoError(t, err)
		require.Len(t, ret, n)
	}
}
