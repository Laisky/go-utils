package kms

import (
	"testing"

	"github.com/stretchr/testify/require"

	gutils "github.com/Laisky/go-utils/v4"
)

func TestEncryptedItem_Unmarshal(t *testing.T) {
	type args struct {
		KekID      uint16
		DekID      []byte
		Ciphertext []byte
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"1", args{1, []byte("213123"), []byte("2342342")}, false},
		{"2", args{1, []byte(gutils.RandomStringWithLength(1024)), []byte(gutils.RandomStringWithLength(1024))}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &EncryptedData{
				Version:    EncryptedItemVer1,
				KekID:      tt.args.KekID,
				DekID:      tt.args.DekID,
				Ciphertext: tt.args.Ciphertext,
			}

			data, err := e.Marshal()
			require.NoError(t, err)

			e2 := EncryptedData{}
			err = e2.Unmarshal(data)
			require.NoError(t, err)

			require.Equal(t, e.Version, e2.Version)
			require.Equal(t, e.KekID, e2.KekID)
			require.Equal(t, e.DekID, e2.DekID)
			require.Equal(t, e.Ciphertext, e2.Ciphertext)
		})
	}
}
