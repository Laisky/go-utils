package crypto

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"

	gutils "github.com/Laisky/go-utils/v6"
)

// TestHKDFWithSHA256 verifies that two HKDFWithSHA256 calls with the same random key and salt fill
// ten 20-byte result buffers with identical keys, checking the length and value of the first three
// outputs.
func TestHKDFWithSHA256(t *testing.T) {
	t.Parallel()

	key := make([]byte, 16)
	_, err := rand.Read(key)
	require.NoError(t, err)

	salt, err := Salt(16)
	require.NoError(t, err)

	results1 := make([][]byte, 10)
	for i := range results1 {
		results1[i] = make([]byte, 20)
	}
	require.NoError(t, HKDFWithSHA256(key, salt, nil, results1))

	results2 := make([][]byte, 10)
	for i := range results2 {
		results2[i] = make([]byte, 20)
	}
	require.NoError(t, HKDFWithSHA256(key, salt, nil, results2))

	// same key & salt will derivative same keys
	require.Len(t, results1[0], 20)
	require.Len(t, results2[0], 20)
	require.Equal(t, results1[0], results2[0])
	require.Equal(t, results1[1], results2[1])
	require.Equal(t, results1[2], results2[2])
}

// TestSalt_InvalidLength verifies that Salt rejects zero and negative lengths with a "salt length
// must be positive" error.
func TestSalt_InvalidLength(t *testing.T) {
	t.Parallel()

	_, err := Salt(0)
	require.ErrorContains(t, err, "salt length must be positive")

	_, err = Salt(-1)
	require.ErrorContains(t, err, "salt length must be positive")
}

// TestDeriveKey verifies DeriveKeyByHKDF with a nil salt against base64-encoded known-answer
// vectors for several secrets and output lengths, checking both the output length and value.
func TestDeriveKey(t *testing.T) {
	t.Parallel()

	type args struct {
		secret    []byte
		expectLen int
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{"0", args{[]byte("wefew"), 1}, "Hg==", false},
		{"1", args{[]byte("wefew"), 10}, "HnYiKmvNbtd5cg==", false},
		{"2", args{[]byte("dqwdq"), 10}, "NVj26CZZyWBZeQ==", false},
		{"3", args{[]byte("dqwdq"), 10}, "NVj26CZZyWBZeQ==", false},
		{"5", args{[]byte("dqwde"), 10}, "ZOMpRJ3GeNQF4w==", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DeriveKeyByHKDF(tt.args.secret, nil, tt.args.expectLen)
			if !tt.wantErr {
				require.NoErrorf(t, err, "[%s]", tt.name)
			}

			require.Lenf(t, got, tt.args.expectLen, "[%s]", tt.name)
			require.Equalf(t, tt.want, gutils.EncodeByBase64(got), "[%s]", tt.name)
		})
	}
}

// TestDeriveKeyByHKDF verifies DeriveKeyByHKDF against base64-encoded known-answer vectors for
// combinations of secret, salt (including nil) and output length, showing that identical inputs
// yield identical keys and that changing the secret or salt changes the key.
func TestDeriveKeyByHKDF(t *testing.T) {
	t.Parallel()

	type args struct {
		secret, salt []byte
		expectLen    int
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{"0", args{[]byte("wefew"), nil, 1}, "Hg==", false},
		{"1", args{[]byte("wefew"), nil, 10}, "HnYiKmvNbtd5cg==", false},
		{"2", args{[]byte("dqwdq"), []byte("dqwdq"), 10}, "F5a7I4KXb-zYoQ==", false},
		{"3", args{[]byte("dqwdq"), []byte("dqwdq"), 10}, "F5a7I4KXb-zYoQ==", false},
		{"4", args{[]byte("dqwdq"), []byte("dqwde"), 10}, "ZXuIt9wvOADp7A==", false},
		{"5", args{[]byte("dqwde"), []byte("dqwdq"), 10}, "tR9B7aMZl6prFA==", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DeriveKeyByHKDF(tt.args.secret, tt.args.salt, tt.args.expectLen)
			if !tt.wantErr {
				require.NoErrorf(t, err, "[%s]", tt.name)
			}

			require.Lenf(t, got, tt.args.expectLen, "[%s]", tt.name)
			require.Equalf(t, tt.want, gutils.EncodeByBase64(got), "[%s]", tt.name)
		})
	}
}

// TestDeriveKeyBySMHF verifies DeriveKeyBySMHF (scrypt) against base64-encoded known-answer vectors
// for several secret and salt combinations, including a nil salt, so that the derivation stays
// deterministic and its output is pinned.
func TestDeriveKeyBySMHF(t *testing.T) {
	t.Parallel()

	type args struct {
		secret, salt []byte
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{"0", args{[]byte("wefew"), nil}, "8WhliJaHIKLFH26MJxNbrt-EeS1E9gRcJy_Rgn8reaM=", false},
		{"1", args{[]byte("wefew"), nil}, "8WhliJaHIKLFH26MJxNbrt-EeS1E9gRcJy_Rgn8reaM=", false},
		{"2", args{[]byte("dqwdq"), []byte("dqwdq")}, "oJoV1zG4OD3DacQMKSUuR7pEuC_3KOAVxbA2g17H-H0=", false},
		{"3", args{[]byte("dqwdq"), []byte("dqwdq")}, "oJoV1zG4OD3DacQMKSUuR7pEuC_3KOAVxbA2g17H-H0=", false},
		{"4", args{[]byte("dqwdq"), []byte("dqwde")}, "iARWAZNeqM7KCaUaKMiFK4LxV0OGTOq5IT-m3VscZGg=", false},
		{"5", args{[]byte("dqwde"), []byte("dqwdq")}, "6x06Uy39lid583AXwDdDcLDAKSrgjPHOKdrrcHxKdOY=", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DeriveKeyBySMHF(tt.args.secret, tt.args.salt)
			if !tt.wantErr {
				require.NoErrorf(t, err, "[%s]", tt.name)
			}

			require.Equalf(t, tt.want, gutils.EncodeByBase64(got), "[%s]", tt.name)
		})
	}
}
