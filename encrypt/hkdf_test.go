package encrypt

import (
	"crypto/rand"
	"crypto/sha256"
	"reflect"
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

func TestExpandSecret(t *testing.T) {
	type args struct {
		secret    []byte
		expectLen int
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{"", args{[]byte("wefew"), 1}, []byte{30}, false},
		{"", args{[]byte("wefew"), 10}, []byte{30, 118, 34, 42, 107, 205, 110, 215, 121, 114}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandSecret(tt.args.secret, tt.args.expectLen)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExpandSecret() = %v, want %v", got, tt.want)
			}
		})
	}
}
