package crypto

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatBig2Hex(t *testing.T) {
	t.Parallel()

	b := new(big.Int)
	b = b.SetInt64(490348974827092350)
	hex := FormatBig2Hex(b)

	t.Logf("%x, %v", b, hex)
	require.Equal(t, hex, fmt.Sprintf("%x", b))
}

func TestFormatBig2Base64(t *testing.T) {
	t.Parallel()

	b := new(big.Int)
	b = b.SetInt64(490348974827092350)
	r := FormatBig2Base64(b)
	require.Equal(t, r, "Bs4Ry2yLuX4=")
}

func TestParseHex2Big(t *testing.T) {
	t.Parallel()

	hex := "6ce11cb6c8bb97e"
	b, ok := ParseHex2Big(hex)
	require.True(t, ok)

	t.Logf("%x, %v", b, hex)
	require.Equal(t, hex, fmt.Sprintf("%x", b))
}

func TestParseBase642Big(t *testing.T) {
	t.Parallel()

	raw := "Bs4Ry2yLuX4="
	b, err := ParseBase642Big(raw)
	require.NoError(t, err)

	t.Log(b.String())
	require.Equal(t, "490348974827092350", b.String())
}

func TestECDSASignFormatAndParseByHex(t *testing.T) {
	t.Parallel()

	a := new(big.Int)
	a = a.SetInt64(490348974827092350)
	b := new(big.Int)
	b = b.SetInt64(9482039480932482)

	encoded := EncodeES256SignByHex(a, b)
	t.Logf("encoded: %v", encoded)

	a2, b2, err := DecodeES256SignByHex(encoded)
	require.NoError(t, err)

	require.Equal(t, 0, a2.Cmp(a))
	require.Equal(t, 0, b2.Cmp(b))
}

func TestECDSASignFormatAndParseByBase64(t *testing.T) {
	t.Parallel()

	a := new(big.Int)
	a = a.SetInt64(490348974827092350)
	b := new(big.Int)
	b = b.SetInt64(9482039480932482)

	encoded := EncodeES256SignByBase64(a, b)
	t.Logf("encoded: %v", encoded)

	a2, b2, err := DecodeES256SignByBase64(encoded)
	require.NoError(t, err)

	require.Equal(t, 0, a2.Cmp(a))
	require.Equal(t, 0, b2.Cmp(b))
}
