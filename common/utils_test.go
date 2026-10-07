package common

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

// TestStr2BytesSharesMemory verifies the zero-copy contract of Str2Bytes:
// identical bytes, len == cap == len(s), and the same backing memory.
func TestStr2BytesSharesMemory(t *testing.T) {
	t.Parallel()
	s := string([]byte("zero-copy conversion"))
	b := Str2Bytes(s)

	require.Equal(t, []byte(s), b)
	require.Len(t, b, len(s))
	require.Equal(t, len(s), cap(b))
	require.Equal(t, unsafe.StringData(s), unsafe.SliceData(b))
}

// TestBytes2StrSharesMemory verifies the zero-copy contract of Bytes2Str.
func TestBytes2StrSharesMemory(t *testing.T) {
	t.Parallel()
	b := []byte("zero-copy conversion")
	s := Bytes2Str(b)

	require.Equal(t, string(b), s)
	require.Equal(t, unsafe.SliceData(b), unsafe.StringData(s))
}

// TestUnsafeConversionsEmptyInputs verifies empty and nil inputs.
func TestUnsafeConversionsEmptyInputs(t *testing.T) {
	t.Parallel()
	require.Empty(t, Str2Bytes(""))
	require.NotNil(t, Str2Bytes(""))
	require.Empty(t, Bytes2Str(nil))
	require.Empty(t, Bytes2Str([]byte{}))
}

// TestUnsafeConversionsRoundTrip verifies round trips across many lengths.
func TestUnsafeConversionsRoundTrip(t *testing.T) {
	t.Parallel()
	for n := range 300 {
		raw := make([]byte, n)
		for i := range raw {
			raw[i] = byte(i * 7)
		}
		s := string(raw)
		require.Equal(t, s, Bytes2Str(Str2Bytes(s)))
	}
}
